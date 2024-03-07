package processor

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/processor/models"
)

// NestProcessor is the pokemon nest processor.
// The manager creates one for each instance of
// the configuration. I.e., a new one one will be
// created by the Manager and swapped in on reloads,
// preserving the pointer to the history.
type NestProcessor struct {
	logger       *logrus.Logger
	nestsDBStore *db_store.NestsDBStore

	nestMatcher *NestMatcher

	statsCollection *StatsCollection
	webhookSender   WebhookSender

	config Config
}

func (np *NestProcessor) LogConfiguration(prefix string, numNests int) {
	var buf bytes.Buffer

	buf.WriteString(prefix)
	buf.WriteString(fmt.Sprintf("numNests: %d, ", numNests))
	np.config.writeConfiguration(&buf)

	np.logger.Info(buf.String())
}

func (np *NestProcessor) AddPokemon(pokemon *models.Pokemon) AddPokemonStats {
	nests := np.nestMatcher.GetMatchingNests(pokemon.Lat, pokemon.Lon)
	wasCounted := np.statsCollection.AddPokemon(pokemon, nests)
	return AddPokemonStats{
		WasCounted:      wasCounted,
		NumNestsMatched: uint64(len(nests)),
	}
}

func (np *NestProcessor) RotateStats() *FrozenStatsCollection {
	return np.statsCollection.Rotate(np.config.MaxHistoryDuration(), np.config.SkipPeriodMinGlobalSpawnPct)
}

func (np *NestProcessor) KeepRecentStats(keepDuration time.Duration) (int, time.Duration) {
	return np.statsCollection.KeepRecent(keepDuration)
}

func (np *NestProcessor) PurgeNewestStats(purgeDuration time.Duration, includeCurrent bool) (int, time.Duration) {
	return np.statsCollection.PurgeNewest(purgeDuration, includeCurrent)
}

func (np *NestProcessor) PurgeOldestStats(purgeDuration time.Duration) (int, time.Duration) {
	return np.statsCollection.PurgeOldest(purgeDuration)
}

func (np *NestProcessor) logPokemonAndComputeNesting(summary models.NestTimePeriodSummary, pokStats models.NestPokemonCountAndTotal, onlyLog bool, logPrefix string) *models.NestingPokemonInfo {
	nest := summary.Nest

	nestPct := pokStats.NestPct()
	gblPct := pokStats.GlobalPct()
	var nestPctToGblPct float64

	if gblPct != 0 {
		nestPctToGblPct = nestPct / gblPct
	}

	cfg := np.config

	ni, reason := func() (*models.NestingPokemonInfo, string) {
		if onlyLog {
			return nil, ""
		}

		// The more interesting checks are first to see what they look like in logs.

		if nestPct < cfg.MinNestPokemonPct {
			return nil, fmt.Sprintf("this pokemon's percent in the nest (%0.3f) too small (< %0.3f)", nestPct, np.config.MinNestPokemonPct)
		}

		if nestPct < gblPct {
			return nil, fmt.Sprintf("this pokemon's percent in the nest (%0.3f) is less than global spawn percent (%0.3f)", nestPct, gblPct)
		}

		if nestPctToGblPct < cfg.MinNestPctToGlobalPctRatio {
			return nil, fmt.Sprintf("this pokemon's ratio (%0.3f) of nest percent (%0.3f) to global percent (%0.3f) is too small (< %0.3f)", nestPctToGblPct, nestPct, gblPct, cfg.MinNestPctToGlobalPctRatio)
		}

		// if a mon is spawning enough globally, ignore it. but I find the nestPct:gblPct ratio better.
		if maxPct := cfg.MaxGlobalSpawnPct; maxPct > 0 {
			if pct := pokStats.GlobalPct(); pct > maxPct {
				return nil, fmt.Sprintf("this pokemon's global spawn pct is too high (%0.3f > %0.3f)", pct, maxPct)
			}
		}

		if pokStats.Total < uint64(cfg.MinTotalPokemon) {
			return nil, fmt.Sprintf("not enough pokemon seen overall (%d < %d)", pokStats.Total, cfg.MinTotalPokemon)
		}

		if pokStats.Count < uint64(cfg.MinNestPokemon) {
			return nil, fmt.Sprintf("not enough of this pokemon seen (%d < %d)", pokStats.Count, cfg.MinNestPokemon)
		}

		if minHistory := np.config.MinHistoryDuration(); summary.Duration < minHistory {
			return nil, "not enough stats history yet"
		}

		hours := float64(summary.Duration) / float64(time.Hour)

		return &models.NestingPokemonInfo{
			PokemonKey: pokStats.PokemonKey,

			StatsDurationMinutes: uint64(summary.Duration / time.Minute),

			UpdatedAt:  summary.EndTime,
			DetectedAt: summary.EndTime,

			NestCount:       pokStats.Count,
			NestTotal:       pokStats.Total,
			NestHourlyCount: float64(pokStats.Count) / hours,
			NestHourlyTotal: float64(pokStats.Total) / hours,

			GlobalCount:       pokStats.Global,
			GlobalTotal:       pokStats.GlobalTotal,
			GlobalHourlyCount: float64(pokStats.Global) / hours,
			GlobalHourlyTotal: float64(pokStats.GlobalTotal) / hours,
		}, "nesting!"
	}()

	if logPrefix != "" {
		fmt := "%s NEST [%s] #%02d: %d:%d nest: %d/%d (%0.3f%%), global: %d/%d (%0.3f%%), nestPctToGlobalPctRatio: %0.3f)"
		if reason != "" {
			fmt += ": " + reason
		}
		np.logger.Infof(fmt,
			logPrefix,
			nest,
			pokStats.Rank,
			pokStats.PokemonKey.PokemonId,
			pokStats.PokemonKey.FormId,
			pokStats.Count,
			pokStats.Total,
			nestPct,
			pokStats.Global,
			pokStats.GlobalTotal,
			gblPct,
			nestPctToGblPct,
		)
	}

	return ni
}

func (np *NestProcessor) processTimePeriodSummary(summary models.NestTimePeriodSummary, logPrefix string) *models.NestingPokemonInfo {
	var nestingPokemonInfo *models.NestingPokemonInfo

	// XXX: It's probably more interesting to look at these as a whole. For
	// example, we can possibly reason about things if we compared against
	// each other. For example, these are sorted by % in the nest. If there's
	// a tie, the one spawning the least globally wins. But, let's say we have
	// 22% in nest spawning @ 3% globally.. and.. 20% in nest spawning at 0.5%
	// globally. It is more likely that the 20% is the nesting pokemon.
	//
	// So, we're currently relying on the 22% to get thrown out and ignored by
	// its own individual stats. And it would by some of the global configs like:
	//
	// 1) you could configure to ignore pokemon spawning at 2.5%+. That might be
	//    reasonable. From eyeballing prometheus over the past number of months,
	//    it seems Niantic doesn't go much over 10% for a single mon and often
	//    looks like around 7%. As I write this, there's only 10 pokemon spawning
	//    at more than 2%. The highest is ~6% and #2 is ~4.75%.
	// 2) you could configure the nest_pct_to_global_pct ratio accordingly. The
	//    default for this one is currently 8, and that's enough to get the 22%
	//    in the above example thrown out (ratio is ~7). The default of 10 has seemed
	//    like a good number for this as I've been testing things. The obvious nesting
	//    mons can have some absurd ratios.
	//
	// Or it might be interesting to look at the top 2 choices and compare those. Or
	// some sort of scoring system.
	for idx, pokStats := range summary.PokemonCountsAndTotals {
		// stop at 10 pokemon
		if idx > 9 {
			if logPrefix != "" {
				np.logger.Infof(
					"%s NEST [%s] Stopping at %d out of %d pokemon",
					logPrefix,
					summary.Nest,
					idx,
					len(summary.PokemonCountsAndTotals),
				)
			}
			break
		}
		if pokStats.GlobalTotal <= 0 || pokStats.Global <= 0 ||
			pokStats.Total <= 0 || pokStats.Count <= 0 {
			np.logger.Warnf("PROCESSOR: Got unexpected stats when processing time period: %#v", pokStats)
			continue
		}

		res := np.logPokemonAndComputeNesting(summary, pokStats, nestingPokemonInfo != nil, logPrefix)
		if res != nil {
			nestingPokemonInfo = res
		}
	}

	return nestingPokemonInfo
}

func (np *NestProcessor) logLatestEntry(lastEntry *CountsForTimePeriod) {
	np.logger.Infof("LAST-PERIOD: dur: %s, nests_processed: %d, global_mons: %d",
		lastEntry.EndTime.Sub(lastEntry.StartTime).Truncate(time.Second),
		len(lastEntry.NestCounts),
		lastEntry.GlobalCounts.Total,
	)

	for nestId := range lastEntry.NestCounts {
		nest := np.GetNestById(nestId)
		if nest == nil {
			// a reload happened and nest was removed
			np.logger.Warnf("LAST-PERIOD: Ignoring missing nest %d", nestId)
			continue
		}
		summary := lastEntry.GetSummaryForNest(nest, lastEntry.EndTime.Sub(lastEntry.StartTime))
		if summary == nil {
			np.logger.Warnf("LAST-PERIOD: No summary for nest %s", nest)
			continue
		}
		np.processTimePeriodSummary(*summary, "LAST-PERIOD:")
	}
}

func (np *NestProcessor) GetConfig() Config {
	return np.config
}

func (np *NestProcessor) GetNestById(nestId int64) *models.Nest {
	// nests don't change once loaded into this object, so no locking.
	return np.nestMatcher.GetNestById(nestId)
}

func (np *NestProcessor) GetNests() []*models.Nest {
	return np.nestMatcher.GetAllNests()
}

func (np *NestProcessor) GetStatsSnapshot() *FrozenStatsCollection {
	return np.statsCollection.GetSnapshot()
}

func (np *NestProcessor) ProcessStatsCollection(statsCollection *FrozenStatsCollection) {
	np.LogConfiguration("PROCESSOR: time period processing starting with configuration: ", np.nestMatcher.Len())
	defer np.logger.Infof("PROCESSOR: time period processing ending")

	totals := statsCollection.Totals

	// This duration is the sum of all the time periods we have.
	// This may be shorter than (EndTime - StartTime) if periods
	// were thrown out.
	duration := statsCollection.Duration
	fullDuration := totals.EndTime.Sub(totals.StartTime).Truncate(time.Minute)
	var withGaps string
	if fullDuration > duration {
		withGaps = " with gaps"
	}

	np.logger.Infof("PROCESSOR: Processing %d time period(s) (%s (%s to %s%s)): %d nests with pokemon, total mons globally: %d",
		statsCollection.Len(),
		duration,
		statsCollection.Totals.StartTime.Format(time.RFC3339),
		statsCollection.Totals.EndTime.Format(time.RFC3339),
		withGaps,
		len(totals.NestCounts),
		totals.GlobalCounts.Total,
	)

	if np.config.LogLastStatsPeriod {
		np.logLatestEntry(statsCollection.LatestEntry())
	}

	now := time.Now()

	logPrefix := fmt.Sprintf("ALL-PERIODS(%d):", statsCollection.Len())
	for nestId := range totals.NestCounts {
		nest := np.nestMatcher.GetNestById(nestId)
		if nest == nil {
			// a reload happened and nest was removed
			np.logger.Warnf("PROCESSOR: Ignoring missing nest %d", nestId)
			continue
		}

		summary := totals.GetSummaryForNest(nest, duration)
		if summary == nil {
			np.logger.Warnf("PROCESSOR: No summary for nest %s", nest)
			continue
		}

		ni := np.processTimePeriodSummary(
			*summary,
			logPrefix,
		)

		if minHistory := np.config.MinHistoryDuration(); summary.Duration < minHistory {
			continue
		}

		// side effect: updates ni.DetectedAt.
		old_ni, dbUpdatedAt := nest.SetNestingPokemon(ni, now)

		if ni == nil {
			if old_ni == nil {
				np.logger.Infof("PROCESSOR[%s]: still does not have a nesting pokemon",
					nest,
				)
			} else {
				np.logger.Infof("PROCESSOR[%s]: NEST-END: nesting pokemon was %s",
					nest,
					old_ni.PokemonKey,
				)
			}

			if now.After(dbUpdatedAt.Add(np.config.NoNestingPokemonAge())) {
				continue
			}

			np.logger.Infof("PROCESSOR[%s]: Unsetting nesting pokemon in DB",
				nest,
			)

			partialNest := nest.AsStorePartialUpdatePokemon(now)
			if err := np.nestsDBStore.UpdateNestPartial(context.Background(), nest.Id, partialNest); err != nil {
				np.logger.Errorf("PROCESSOR[%s]: failed to update DB to unset nesting pokemon: %v",
					nest,
					err,
				)
			}
			nest.SetUpdatedAt(now)
			continue
		}

		if old_ni == nil {
			np.logger.Infof("PROCESSOR[%s]: NEST-START: nesting pokemon is %s",
				nest,
				ni.PokemonKey,
			)
			np.webhookSender.AddNestWebhook(nest, ni)
		} else if ni.PokemonKey != old_ni.PokemonKey {
			np.logger.Infof("PROCESSOR[%s]: NEST-CHANGE: nesting pokemon has changed from %s to %s",
				nest,
				old_ni.PokemonKey,
				ni.PokemonKey,
			)
			np.webhookSender.AddNestWebhook(nest, ni)
		}

		var nestToGlobalPctRatio float64
		if gblPct := ni.GlobalPct(); gblPct > 0 {
			nestToGlobalPctRatio = ni.NestPct() / gblPct
		}

		np.logger.Infof("PROCESSOR[%s]: NESTING: %s (nestingFor:%s, statsDuration:%s, cnt:%d/%d, nestHourlyRate:%0.3f, nestPct:%0.3f, gblHourlyRate:%0.3f, gblPct:%0.3f, nestPctToGlobalPctRatio:%0.3f ",
			nest,
			ni.PokemonKey,
			time.Since(ni.DetectedAt),
			time.Minute*time.Duration(ni.StatsDurationMinutes),
			ni.NestCount,
			ni.NestTotal,
			ni.NestHourlyCount,
			ni.NestPct(),
			ni.GlobalHourlyCount,
			ni.GlobalPct(),
			nestToGlobalPctRatio,
		)

		partialNest := nest.AsStorePartialUpdatePokemon(now)
		if err := np.nestsDBStore.UpdateNestPartial(context.Background(), nest.Id, partialNest); err != nil {
			np.logger.Errorf("PROCESSOR[%s]: failed to update DB to set nesting pokemon: %v",
				nest,
				err,
			)
			continue
		}
		nest.SetUpdatedAt(now)
	}
}

func NewNestProcessor(oldNestProcessor *NestProcessor, logger *logrus.Logger, nestsDBStore *db_store.NestsDBStore, nestMatcher *NestMatcher, webhookSender WebhookSender, config Config) *NestProcessor {
	nestProcessor := &NestProcessor{
		logger:        logger,
		nestsDBStore:  nestsDBStore,
		nestMatcher:   nestMatcher,
		webhookSender: webhookSender,
		config:        config,
	}
	if oldNestProcessor == nil {
		// startup.
		nestProcessor.statsCollection = NewStatsCollection(logger)
	} else {
		// reload.
		// these will still contain deleted nests, but those will be
		// skipped when nesting mon is computed. they'll eventually
		// cycle out.
		nestProcessor.statsCollection = oldNestProcessor.statsCollection
	}
	return nestProcessor
}
