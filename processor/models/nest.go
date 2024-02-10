package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"
	"gopkg.in/guregu/null.v4"

	"github.com/UnownHash/Fletchling/db_store"
)

// NestingPokemonInfo contains info about a nesting pokemon. 'Count'
// values are for the PokemonKey. 'Total' values are for all pokemon,
// including the PokemonKey.
type NestingPokemonInfo struct {
	PokemonKey PokemonKey `json:"pokemon"`

	StatsDurationMinutes uint64 `json:"stats_duration_minutes"`

	NestCount       uint64  `json:"nest_count"`
	NestTotal       uint64  `json:"nest_total"`
	NestHourlyCount float64 `json:"nest_hourly_count"`
	NestHourlyTotal float64 `json:"nest_hourly_total"`

	GlobalCount       uint64  `json:"global_count"`
	GlobalTotal       uint64  `json:"global_total"`
	GlobalHourlyCount float64 `json:"global_hourly_count"`
	GlobalHourlyTotal float64 `json:"global_hourly_total"`

	DetectedAt time.Time `json:"detected_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (ni *NestingPokemonInfo) NestPct() float64 {
	if ni.NestTotal == 0 {
		return 0
	}
	return 100 * float64(ni.NestCount) / float64(ni.NestTotal)
}

func (ni *NestingPokemonInfo) NestRatio() float64 {
	if ni.NestCount == ni.NestTotal {
		return 0
	}
	return float64(ni.NestCount) / float64(ni.NestTotal-ni.NestCount)
}

func (ni *NestingPokemonInfo) GlobalPct() float64 {
	if ni.GlobalTotal == 0 {
		return 0
	}
	return 100 * float64(ni.GlobalCount) / float64(ni.GlobalTotal)
}

func (ni *NestingPokemonInfo) GlobalRatio() float64 {
	if ni.GlobalCount == ni.GlobalTotal {
		return 0
	}
	return float64(ni.GlobalCount) / float64(ni.GlobalTotal-ni.GlobalCount)
}

type NestStatsInfo struct {
	mutex sync.Mutex

	// updatedAt here is used as the Updated time
	// in the db when we write. It's inside this struct
	// because we may be writing to it during stats
	// processing and this is where we have the locking.
	updatedAt      time.Time
	nestingPokemon *NestingPokemonInfo
}

func (si *NestStatsInfo) GetNestingPokemon() (*NestingPokemonInfo, time.Time) {
	si.mutex.Lock()
	defer si.mutex.Unlock()

	return si.nestingPokemon, si.updatedAt
}

func (si *NestStatsInfo) SetUpdatedAt(updatedAt time.Time) time.Time {
	si.mutex.Lock()
	defer si.mutex.Unlock()

	old := si.updatedAt
	si.updatedAt = updatedAt
	return old
}

// SetNestingPokemon sets the current nesting mon (or no mon). Returns the intended
// db Updated column value. The Updated column value only changes if there's a nesting
// mon, as we'll be writing out new stats. If there's no nesting mon, it is not updated
// if we think something is no longer nesting, because we'll continue to leave it in the
// DB for a while.
// Side effect: can update ni.DetectedAt.
func (si *NestStatsInfo) SetNestingPokemon(ni *NestingPokemonInfo, updatedAt time.Time) (*NestingPokemonInfo, time.Time) {
	si.mutex.Lock()
	defer si.mutex.Unlock()

	old := si.nestingPokemon

	// if we have no nesting mon, we delay updating
	// until there's been no mon for a period of time.
	if ni != nil {
		if updatedAt.IsZero() {
			updatedAt = time.Now()
		}
		// we'll be writing to the DB, so update this,
		// as this is what will be used for Updated column.
		si.updatedAt = updatedAt

		if old != nil && old.PokemonKey == ni.PokemonKey {
			// same mon, so copy the DetectedAt.
			ni.DetectedAt = old.DetectedAt
		}
	}

	si.nestingPokemon = ni

	return old, si.updatedAt
}

type Nest struct {
	SyncedToDb bool
	ExistsInDb bool

	Id          int64
	Name        string
	Center      orb.Point
	Geometry    *geojson.Geometry
	AreaName    *string
	Spawnpoints *int64
	AreaM2      float64
	Active      bool
	Discarded   string

	// broken out so we can just copy this pointer to
	// new Nests without needing to lock.
	*NestStatsInfo
}

func (nest *Nest) FullName() string {
	var namePrefix string
	if nest.AreaName != nil {
		namePrefix = *nest.AreaName + "/"
	}
	return fmt.Sprintf("%s%s(NestId:%d)", namePrefix, nest.Name, nest.Id)
}

func (nest *Nest) String() string {
	center := nest.Center
	return fmt.Sprintf("'%s' centered at %0.5f,%0.5f", nest.FullName(), center.Lat(), center.Lon())
}

func (nest *Nest) AsDBStoreNest() *db_store.Nest {
	center := nest.Center

	ni, updatedAt := nest.GetNestingPokemon()

	polygon, _ := json.Marshal(nest.Geometry)

	discarded := null.StringFrom(nest.Discarded)
	if nest.Active {
		discarded.Valid = false
	}

	dbNest := &db_store.Nest{
		NestId:      nest.Id,
		Lat:         center.Lat(),
		Lon:         center.Lon(),
		Name:        nest.Name,
		Polygon:     polygon,
		AreaName:    null.StringFromPtr(nest.AreaName),
		Spawnpoints: null.IntFromPtr(nest.Spawnpoints),
		M2:          null.FloatFrom(nest.AreaM2),
		Active:      null.BoolFrom(nest.Active),
		Updated:     null.IntFrom(updatedAt.Unix()),
		Discarded:   discarded,
	}

	if ni != nil {
		dbNest.PokemonId = null.IntFrom(int64(ni.PokemonKey.PokemonId))
		dbNest.PokemonForm = null.IntFrom(int64(ni.PokemonKey.FormId))
		dbNest.PokemonCount = null.FloatFrom(float64(ni.NestCount))
		dbNest.PokemonAvg = null.FloatFrom(ni.NestHourlyCount)
		// nestcollector uses pct, and I agree it is better than 'ratio'.
		dbNest.PokemonRatio = null.FloatFrom(ni.NestPct())
	}

	return dbNest
}

func (nest *Nest) AsStorePartialUpdatePokemon(updatedAt time.Time) *db_store.NestPartialUpdate {
	var updated null.Int

	dbNest := nest.AsDBStoreNest()
	if updatedAt.IsZero() {
		updated = dbNest.Updated
	} else {
		updated = null.IntFrom(updatedAt.Unix())
	}

	discarded := null.StringFrom(nest.Discarded)
	if nest.Active {
		discarded.Valid = false
	}

	return &db_store.NestPartialUpdate{
		Updated:      &updated,
		Discarded:    &discarded,
		PokemonId:    &dbNest.PokemonId,
		PokemonForm:  &dbNest.PokemonForm,
		PokemonCount: &dbNest.PokemonCount,
		PokemonAvg:   &dbNest.PokemonAvg,
		PokemonRatio: &dbNest.PokemonRatio,
	}
}

func NestingPokemonInfoFromDBStore(dbNest *db_store.Nest) (*NestingPokemonInfo, time.Time) {
	// preserve nesting pokemon in DB if it looks ok. But if there's no Updated, set to
	// now.
	var dbUpdatedAt time.Time
	if epoch := dbNest.Updated.ValueOrZero(); epoch > 0 {
		dbUpdatedAt = time.Unix(epoch, 0)
	}

	updatedAtOrNow := dbUpdatedAt
	if updatedAtOrNow.IsZero() {
		updatedAtOrNow = time.Now()
	}

	if pokemonId := dbNest.PokemonId.ValueOrZero(); pokemonId > 0 {
		// When we load existing nest from the DB, we don't care about the existing
		// stats so much. If we already have this nest in memory, the current
		// stats will be copied to it. If we're just starting up, we really only
		// look at PokemonId,FormId to log when nesting mon changes.
		count := dbNest.PokemonCount.ValueOrZero()
		pct := dbNest.PokemonAvg.ValueOrZero()
		total := float64(0)
		if pct > 0 {
			total = 100 * count / pct
		}
		ni := &NestingPokemonInfo{
			PokemonKey: PokemonKey{
				PokemonId: int(pokemonId),
				FormId:    int(dbNest.PokemonForm.ValueOrZero()),
			},
			NestHourlyCount: count,
			NestHourlyTotal: total,
			DetectedAt:      updatedAtOrNow,
			UpdatedAt:       updatedAtOrNow,
		}
		return ni, dbUpdatedAt
	}
	return nil, dbUpdatedAt
}

func NewNestFromDBStore(storeNest *db_store.Nest) (*Nest, error) {
	nestStatsInfo := &NestStatsInfo{}
	nestStatsInfo.SetNestingPokemon(NestingPokemonInfoFromDBStore(storeNest))

	geometry, err := storeNest.Geometry()
	if err != nil {
		return nil, err
	}

	return &Nest{
		SyncedToDb: true,
		ExistsInDb: true,

		Id:            storeNest.NestId,
		Name:          storeNest.Name,
		Center:        orb.Point{storeNest.Lon, storeNest.Lat},
		Geometry:      geometry,
		AreaName:      storeNest.AreaName.Ptr(),
		Spawnpoints:   storeNest.Spawnpoints.Ptr(),
		AreaM2:        storeNest.M2.ValueOrZero(),
		Active:        storeNest.Active.ValueOrZero(),
		Discarded:     storeNest.Discarded.ValueOrZero(),
		NestStatsInfo: nestStatsInfo,
	}, nil
}

func NewNestFromKojiFeature(feature *geojson.Feature) (*Nest, error) {
	props := feature.Properties

	name, ok := props["name"].(string)
	if !ok {
		return nil, fmt.Errorf("feature has no name")
	}

	fullName := name
	var areaName null.String

	if parent, _ := props["parent"].(string); parent != "" {
		areaName = null.StringFrom(parent)
		fullName = parent + "/" + name
	}

	id, ok := props["id"]
	if !ok {
		return nil, fmt.Errorf("feature '%s' has no id", fullName)
	}

	var nestId int64

	switch v := id.(type) {
	case string:
		var err error
		nestId, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("feature '%s': id '%s' can't be parsed as int", fullName, v)
		}
	case int64:
		nestId = v
	case uint64:
		nestId = int64(v)
	default:
		return nil, fmt.Errorf("feature '%s': id '%s' type '%T' not supported", fullName, v)
	}

	geometry := feature.Geometry
	jsonGeometry := geojson.NewGeometry(geometry)
	center, _ := planar.CentroidArea(geometry)
	area := geo.Area(geometry)

	return &Nest{
		Id:       nestId,
		Name:     name,
		Center:   center,
		Geometry: jsonGeometry,
		AreaName: areaName.Ptr(),
		AreaM2:   area,

		// default to true
		Active: true,

		NestStatsInfo: &NestStatsInfo{
			updatedAt: time.Now(),
		},
	}, nil
}
