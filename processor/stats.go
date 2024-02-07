package processor

import (
	"sort"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/processor/models"
)

type AddPokemonStats struct {
	NumPokemonProcessed uint64
	NumNestsMatched     uint64
}

// This is protected by the other structures using it and
// doesn't require locking.
type CountsByPokemon struct {
	Total     uint64                       `json:"total"`
	ByPokemon map[models.PokemonKey]uint64 `json:"by_pokemon"`
}

// returns true if empty now
func (counts *CountsByPokemon) subtract(logger *logrus.Logger, other *CountsByPokemon) bool {
	counts.Total -= other.Total

	if counts.Total <= 0 {
		if counts.Total < 0 {
			logger.Errorf("Total count in %#v has gone negative after del of %#v",
				*counts,
				*other,
			)
		}
		counts.ByPokemon = nil
		return true
	}

	for k, v := range other.ByPokemon {
		counts.ByPokemon[k] -= v

		if counts.ByPokemon[k] <= 0 {
			if counts.ByPokemon[k] < 0 {
				logger.Errorf("Total count for pokemon %s gone negative (%d) after del of %d",
					k,
					counts.ByPokemon[k],
					v,
				)
			}
			delete(counts.ByPokemon, k)
			continue
		}
	}

	return false
}

func (counts *CountsByPokemon) mostSpawningPokemon() (models.PokemonKey, float64) {
	var pokemon models.PokemonKey
	var maxCount uint64

	if counts.Total == 0 {
		return pokemon, 0
	}

	for pokemonKey, count := range counts.ByPokemon {
		if count > maxCount {
			maxCount = count
			pokemon = pokemonKey
		}
	}

	return pokemon, 100 * float64(maxCount) / float64(counts.Total)
}

func (counts *CountsByPokemon) clone() *CountsByPokemon {
	nCounts := NewCountsByPokemon()
	nCounts.Total = counts.Total
	for k, v := range counts.ByPokemon {
		nCounts.ByPokemon[k] = v
	}
	return nCounts
}

func NewCountsByPokemon() *CountsByPokemon {
	return &CountsByPokemon{
		ByPokemon: make(map[models.PokemonKey]uint64),
	}
}

// CountsForTImePeriod contain pokemon counts for a specific time
// period. StartTime is set upon creation of the struct. EndTime will
// be set when the next time period is created.
type CountsForTimePeriod struct {
	logger *logrus.Logger
	mutex  sync.RWMutex

	Frozen       bool                       `json:"frozen"`
	StartTime    time.Time                  `json:"start_time"`
	EndTime      time.Time                  `json:"end_time"`
	NestCounts   map[int64]*CountsByPokemon `json:"nest_counts"`
	GlobalCounts *CountsByPokemon           `json:"global_counts"`
}

func (tpCounts *CountsForTimePeriod) clone(endTime time.Time) *CountsForTimePeriod {
	if !tpCounts.Frozen {
		tpCounts.mutex.RLock()
		defer tpCounts.mutex.RUnlock()
	}

	ntpCounts := NewCountsForTimePeriod(tpCounts.logger, tpCounts.StartTime)
	ntpCounts.EndTime = endTime
	for k, v := range tpCounts.NestCounts {
		ntpCounts.NestCounts[k] = v.clone()
	}
	ntpCounts.GlobalCounts = tpCounts.GlobalCounts.clone()
	return ntpCounts
}

func (tpCounts *CountsForTimePeriod) subtract(logger *logrus.Logger, other *CountsForTimePeriod) {
	if !tpCounts.Frozen {
		tpCounts.mutex.Lock()
		defer tpCounts.mutex.Unlock()
	}

	tpCounts.GlobalCounts.subtract(logger, other.GlobalCounts)
	for nestId, delNestCount := range other.NestCounts {
		if tpCounts.NestCounts[nestId].subtract(logger, delNestCount) {
			delete(tpCounts.NestCounts, nestId)
		}
	}
}

func (tpCounts *CountsForTimePeriod) Duration() time.Duration {
	endTime := tpCounts.EndTime
	if endTime.IsZero() {
		endTime = time.Now()
	}
	return endTime.Sub(tpCounts.StartTime).Truncate(time.Minute)
}

func (tpCounts *CountsForTimePeriod) AddPokemon(pokemon *models.Pokemon, nests []*models.Nest) (resp AddPokemonStats) {
	tpCounts.mutex.Lock()
	defer tpCounts.mutex.Unlock()

	if tpCounts.Frozen {
		// only happens if we have a bug!
		return
	}

	pokemonKey := pokemon.Key()
	tpCounts.GlobalCounts.Total++
	tpCounts.GlobalCounts.ByPokemon[pokemonKey]++

	for _, nest := range nests {
		nestCount := tpCounts.NestCounts[nest.Id]
		if nestCount == nil {
			nestCount = NewCountsByPokemon()
		}
		nestCount.Total++
		nestCount.ByPokemon[pokemonKey]++
		tpCounts.NestCounts[nest.Id] = nestCount
	}

	resp.NumPokemonProcessed++
	resp.NumNestsMatched += uint64(len(nests))

	return
}

func (tpCounts *CountsForTimePeriod) GetOrderedGlobalPokemon() PokemonCountAndTotals {
	if !tpCounts.Frozen {
		tpCounts.mutex.RLock()
		defer tpCounts.mutex.RUnlock()
	}

	statsList := make(PokemonCountAndTotals, len(tpCounts.GlobalCounts.ByPokemon))

	total := tpCounts.GlobalCounts.Total
	idx := 0
	for pokemonKey, count := range tpCounts.GlobalCounts.ByPokemon {
		statsList[idx] = PokemonCountAndTotal{
			PokemonKey: pokemonKey,
			Rank:       idx + 1,
			Count:      count,
			Total:      total,
		}
		idx++
	}

	sort.Sort(statsList)

	return statsList
}

// GetSummaryForNests() returns info about the pokemon existing in the nest, sorted by spawn %/count DESC.
// Since this object is also abused to keep totals for all time periods and there can be gaps due to
// throwing out time periods, the StartTime/EndTime in this object won't reflect an accurate duration. This
// must be passed in.
func (tpCounts *CountsForTimePeriod) GetSummaryForNest(nest *models.Nest, duration time.Duration) *models.NestTimePeriodSummary {
	if !tpCounts.Frozen {
		tpCounts.mutex.RLock()
		defer tpCounts.mutex.RUnlock()
	}

	nestCounts, ok := tpCounts.NestCounts[nest.Id]
	if !ok {
		return nil
	}

	globalCounts := tpCounts.GlobalCounts

	countsAndTotals := make(models.NestPokemonCountsAndTotals, len(nestCounts.ByPokemon))
	idx := 0
	for pokemonKey, count := range nestCounts.ByPokemon {
		countsAndTotals[idx] = models.NestPokemonCountAndTotal{
			Rank:        idx + 1,
			PokemonKey:  pokemonKey,
			Count:       count,
			Total:       nestCounts.Total,
			Global:      globalCounts.ByPokemon[pokemonKey],
			GlobalTotal: globalCounts.Total,
		}
		idx++
	}

	sort.Sort(countsAndTotals)

	return &models.NestTimePeriodSummary{
		Nest:                   nest,
		PokemonCountsAndTotals: countsAndTotals,
		StartTime:              tpCounts.StartTime,
		EndTime:                tpCounts.EndTime,
		Duration:               duration,
	}
}

func NewCountsForTimePeriod(logger *logrus.Logger, startTime time.Time) *CountsForTimePeriod {
	return &CountsForTimePeriod{
		logger:       logger,
		StartTime:    startTime,
		NestCounts:   make(map[int64]*CountsByPokemon),
		GlobalCounts: NewCountsByPokemon(),
	}
}

type FrozenStatsCollection struct {
	// Duration is the sum of the durations of all
	// time periods.
	Duration time.Duration
	// CountsByTimePeriod is the list of stats for each time period
	CountsByTimePeriod []*CountsForTimePeriod
	// Totals are the sums of stats from all time periods.
	Totals *CountsForTimePeriod
}

func (fstats *FrozenStatsCollection) Len() int {
	return len(fstats.CountsByTimePeriod)
}

func (fstats *FrozenStatsCollection) LatestEntry() *CountsForTimePeriod {
	// there's always an entry
	return fstats.CountsByTimePeriod[len(fstats.CountsByTimePeriod)-1]
}

// Current stats collection. Duration is only set when returning
// copies of the stats collection.
type StatsCollection struct {
	logger *logrus.Logger

	// Totals are the sums of stats from all time periods.
	Totals *CountsForTimePeriod

	// mutex protects only the below addresses being
	// changed and they only change on rotation or purge.
	// CountsByTimePeriod is guarded by its own locks,
	// as is Totals above.
	mutex sync.RWMutex

	// Duration is the sum of the durations of all
	// time periods except for the current one.
	Duration time.Duration
	// CountsByTimePeriod is the list of stats for each time period
	CountsByTimePeriod []*CountsForTimePeriod
}

// requires stats.mutex be write locked. purges from the front.
func (stats *StatsCollection) keepRecentStats(keepDuration time.Duration) (int, time.Duration) {
	counts := stats.CountsByTimePeriod
	l := len(counts)

	var durPurged time.Duration
	numPurged := 0

	for ; l > 0 && stats.Duration > keepDuration; l-- {
		var del *CountsForTimePeriod

		del, counts = counts[0], counts[1:]

		numPurged++
		durPurged += del.Duration()

		stats.Duration -= del.Duration()
		stats.Totals.subtract(stats.logger, del)
	}

	if l == 0 {
		counts = append(counts, NewCountsForTimePeriod(stats.logger, time.Now()))
	}

	stats.CountsByTimePeriod = counts
	stats.Totals.StartTime = counts[0].StartTime

	return numPurged, durPurged
}

// requires stats.mutex be write locked
func (stats *StatsCollection) purgeOldestStats(purgeDuration time.Duration) (int, time.Duration) {
	counts := stats.CountsByTimePeriod

	var durPurged time.Duration
	numPurged := 0

	// don't purge periods that overlap purgeDuration only
	// partially. always keep current period.
	for l := len(counts); l > 1 && (durPurged+counts[0].Duration()) < purgeDuration; l-- {
		var del *CountsForTimePeriod

		del, counts = counts[0], counts[1:]

		numPurged++
		durPurged += del.Duration()

		stats.Duration -= del.Duration()
		stats.Totals.subtract(stats.logger, del)
	}

	stats.CountsByTimePeriod = counts
	stats.Totals.StartTime = counts[0].StartTime

	return numPurged, durPurged
}

// requires stats.mutex be write locked
func (stats *StatsCollection) purgeNewestStats(purgeDuration time.Duration, includeCurrent bool) (int, time.Duration) {
	counts := stats.CountsByTimePeriod
	l := len(counts)

	var current *CountsForTimePeriod

	if !includeCurrent {
		current = counts[l-1]
		counts = counts[:l-1]
		l--
	}

	var durPurged time.Duration
	numPurged := 0

	// don't purge periods that overlap purgeDuration only
	// partially.
	for ; l > 0 && (durPurged+counts[0].Duration()) < purgeDuration; l-- {
		var del *CountsForTimePeriod

		del, counts = counts[l-1], counts[:l-1]

		numPurged++
		durPurged += del.Duration()

		stats.Duration -= del.Duration()
		stats.Totals.subtract(stats.logger, del)
	}

	if current != nil {
		// put this back
		counts = append(counts, current)
	} else if numPurged > 0 {
		// current was removed.
		counts = append(counts, NewCountsForTimePeriod(stats.logger, time.Now()))
	}

	stats.CountsByTimePeriod = counts
	stats.Totals.StartTime = counts[0].StartTime

	return numPurged, durPurged
}

func (stats *StatsCollection) Len() int {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()

	return len(stats.CountsByTimePeriod)
}

func (stats *StatsCollection) AddPokemon(pokemon *models.Pokemon, nests []*models.Nest) AddPokemonStats {
	// Yes, we'll be writing, but this lock only protects rotation and purges.
	// Each time period has its own locking that to protect its structures.
	stats.mutex.RLock()
	defer stats.mutex.RUnlock()

	// there's always an entry
	latest := stats.CountsByTimePeriod[len(stats.CountsByTimePeriod)-1]
	resp := latest.AddPokemon(pokemon, nests)
	if resp.NumPokemonProcessed == 0 {
		stats.logger.Warnf("time period unexpectedly frozen when adding pokemon")
		return resp
	}
	stats.Totals.AddPokemon(pokemon, nests)
	return resp
}

func (stats *StatsCollection) GetSnapshot() *FrozenStatsCollection {
	fstats := &FrozenStatsCollection{}

	stats.mutex.RLock()
	defer stats.mutex.RUnlock()

	counts := stats.CountsByTimePeriod[:]

	now := time.Now()
	// we have to clone the last entry, as it will continue to
	// be updated after we're done with our lock.
	lastEntry := counts[len(counts)-1].clone(now)
	counts[len(counts)-1] = lastEntry

	fstats.CountsByTimePeriod = counts
	fstats.Totals = stats.Totals.clone(now)
	// add the partial period we have to Duration
	fstats.Duration = stats.Duration + lastEntry.EndTime.Sub(lastEntry.StartTime)

	return fstats
}

func (stats *StatsCollection) KeepRecent(keepDuration time.Duration) (int, time.Duration) {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()

	return stats.keepRecentStats(keepDuration)
}

// Appends a new empty entry and ensures there's at most 'maxHistory' entries.
// Returns the current entries unless the current period is being thrown out,
// in which case, nil will be returned.
func (stats *StatsCollection) Rotate(maxHistoryDuration time.Duration, skipPeriodMinGlobalSpawnPct float64) *FrozenStatsCollection {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()

	// all other mutexs can be skipped as they are all
	// done behind a read lock on stats.mutex and no one
	// can have that now.

	now := time.Now()

	// The last time period ends now. We don't set the
	// Totals EndTime until we clone it, and only set it
	// in the copy we'll return below. It is assumed here
	// that there's always at least one entry in the history
	// and we do ensure that in NewStatsCollection() as well
	// as in here.
	counts := stats.CountsByTimePeriod
	lastIdx := len(counts) - 1
	latestEntry := counts[lastIdx]

	latestEntry.EndTime = now
	// locks no longer necessary on this time period.
	latestEntry.Frozen = true

	var currentStats *FrozenStatsCollection

	// check if we're going to throw this period away.
	pokemonKey, maxGblPct := latestEntry.GlobalCounts.mostSpawningPokemon()
	if skipPeriodMinGlobalSpawnPct > 0 && maxGblPct > skipPeriodMinGlobalSpawnPct {
		stats.logger.Infof("Throwing away current time period: %s is spawning at %0.3f%%",
			pokemonKey,
			maxGblPct,
		)

		stats.Totals.subtract(stats.logger, latestEntry)
		counts[lastIdx] = NewCountsForTimePeriod(stats.logger, now)
		// in case lastIdx == 0:
		stats.Totals.StartTime = counts[0].StartTime
		// fall through to rotate old history out in case config settings
		// changed.
	} else {
		stats.Duration += latestEntry.Duration()
		currentStats = &FrozenStatsCollection{
			Duration: stats.Duration,
			// nothing will write to the arrays and maps in this
			// time series anymore, so we don't need to clone these
			CountsByTimePeriod: counts[:],
			// we have to clone these because the stats.Totals map
			// will continue to be updated.
			Totals: stats.Totals.clone(now),
		}
		counts = append(counts, NewCountsForTimePeriod(stats.logger, now))
		stats.CountsByTimePeriod = counts
	}

	stats.keepRecentStats(maxHistoryDuration)

	return currentStats
}

func (stats *StatsCollection) PurgeOldest(purgeDuration time.Duration) (int, time.Duration) {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()

	return stats.purgeOldestStats(purgeDuration)
}

func (stats *StatsCollection) PurgeNewest(purgeDuration time.Duration, includeCurrent bool) (int, time.Duration) {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()

	return stats.purgeNewestStats(purgeDuration, includeCurrent)
}

func NewStatsCollection(logger *logrus.Logger) *StatsCollection {
	h := &StatsCollection{
		logger: logger,
		// set up the first entry.
		CountsByTimePeriod: append(
			make([]*CountsForTimePeriod, 0, 8),
			NewCountsForTimePeriod(logger, time.Now()),
		),
		Totals: NewCountsForTimePeriod(logger, time.Now()),
	}
	return h
}
