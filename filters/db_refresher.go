package filters

import (
	"context"
	"math"
	"time"

	orb_geo "github.com/paulmach/orb/geo"
	"github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"

	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/geo"
)

type RefreshNestConfig struct {
	Concurrency             int
	ForceSpawnpointsRefresh bool

	MinAreaM2         float64
	MaxAreaM2         float64
	MinSpawnpoints    int64
	MaxOverlapPercent float64
}

type DBRefresher struct {
	logger        *logrus.Logger
	nestsDBStore  *db_store.NestsDBStore
	golbatDBStore *db_store.GolbatDBStore
}

func (refresher *DBRefresher) refreshNest(ctx context.Context, config RefreshNestConfig, nest db_store.Nest) (db_store.Nest, error) {
	fullName := nest.FullName()

	var partialUpdate *db_store.NestPartialUpdate

	makePartialUpdate := func() {
		if partialUpdate == nil {
			partialUpdate = &db_store.NestPartialUpdate{}
		}
	}

	active := null.BoolFrom(false)

	origNest := nest
	m2 := nest.M2
	areaUpdated := false
	spawnpoints := nest.Spawnpoints

	jsonGeometry, err := nest.Geometry()
	if err == nil {
		geometry := jsonGeometry.Geometry()
		if geo.GeometrySupported(geometry) {
			area := orb_geo.Area(geometry)

			if m2.Valid {
				if math.Abs(m2.Float64-area) > 100 {
					refresher.logger.Infof("DB-REFRESHER[%s]: area is %0.3f m², but DB says %0.3f m², will update.", fullName, area, m2.Float64)
					m2.Float64 = area
					areaUpdated = true
				}
			} else {
				// compute area, if it is unknown yet
				refresher.logger.Infof("DB-REFRESHER[%s]: area was computed to be %0.3f m².", fullName, area)
				m2 = null.FloatFrom(area)
			}
		} else {
			refresher.logger.Warnf("DB-REFRESHER[%s]: found unsupported geometry: %s",
				fullName,
				geometry.GeoJSONType(),
			)
			m2.Valid = false
			spawnpoints.Valid = false
		}
	} else {
		refresher.logger.Warnf("DB-REFRESHER[%s]: found invalid geometry: %v",
			fullName,
			err,
		)
		jsonGeometry = nil
		m2.Valid = false
		spawnpoints.Valid = false
	}

	// compute spawnpoints if golbat_db is configured
	// only get the count for ones we don't know yet, but can be forced.
	// m2 is checked because if it's not valid, it means we already know
	// the geometry is bad. And if it is valid, we don't want to query
	// spawnpoints for areas that are too large.
	if m2.Valid && (config.MaxAreaM2 <= 0 || m2.Float64 <= config.MaxAreaM2) && (!spawnpoints.Valid || config.ForceSpawnpointsRefresh) && (refresher.golbatDBStore != nil) {
		var err error

		if !spawnpoints.Valid {
			refresher.logger.Infof("DB-REFRESHER[%s]: number of spawnpoints is unknown. Will query golbat for them.", fullName)
		}
		numSpawnpoints, err := refresher.golbatDBStore.GetSpawnpointsCount(ctx, jsonGeometry)
		if err == nil {
			if spawnpoints.Valid {
				refresher.logger.Infof("DB-REFRESHER[%s]: spawnpoint count changed from %d to %d", fullName, spawnpoints.Int64, numSpawnpoints)
			} else {
				refresher.logger.Infof("DB-REFRESHER[%s]: spawnpoint count initial value is %d", fullName, numSpawnpoints)
			}
			spawnpoints = null.IntFrom(numSpawnpoints)
		} else {
			if spawnpoints.Valid {
				refresher.logger.Warnf("DB-REFRESHER[%s]: couldn't query spawnpoints (using current value of %d): %v", fullName, spawnpoints.Int64, err)
			} else {
				refresher.logger.Warnf("DB-REFRESHER[%s]: couldn't query spawnpoints (skipping filtering): %v", fullName, err)
			}
		}
	}

	var discarded null.String

	if !m2.Valid {
		discarded = null.StringFrom("invalid")
		if !discarded.Equal(nest.Discarded) {
			refresher.logger.Warnf("DB-REFRESHER[%s]: Deactivating due to invalid geometry",
				fullName,
			)
		}
	} else if area := m2.Float64; (area < config.MinAreaM2) || (config.MaxAreaM2 > 0 && area > config.MaxAreaM2) {
		discarded = null.StringFrom("area")
		if !discarded.Equal(nest.Discarded) {
			if area < config.MinAreaM2 {
				refresher.logger.Warnf("DB-REFRESHER[%s]: Deactivating due to min area filter (%0.3f < %0.3f)",
					fullName,
					area,
					config.MinAreaM2,
				)
			} else {
				refresher.logger.Warnf("DB-REFRESHER[%s]: Deactivating due to max area filter (%0.3f > %0.3f)",
					fullName,
					area,
					config.MaxAreaM2,
				)
			}
		}
	} else if spawnpoints.Valid && spawnpoints.Int64 < config.MinSpawnpoints {
		discarded = null.StringFrom("spawnpoints")
		if !discarded.Equal(nest.Discarded) {
			refresher.logger.Warnf("DB-REFRESHER[%s]: Deactivating due to spawnpoints filter (%d < %d)",
				fullName,
				spawnpoints.Int64,
				config.MinSpawnpoints,
			)
		}
	} else {
		active = null.BoolFrom(true)
		if !active.Equal(nest.Active) {
			refresher.logger.Infof("DB-REFRESHER[%s]: Activating nest (might still be disabled by overlap filter later)", fullName)
		}
	}

	if !discarded.Equal(nest.Discarded) {
		makePartialUpdate()
		partialUpdate.Discarded = &discarded
		nest.Discarded = discarded
	}

	if !m2.Equal(nest.M2) || areaUpdated {
		makePartialUpdate()
		partialUpdate.M2 = &m2
		nest.M2 = m2
	}

	if !spawnpoints.Equal(nest.Spawnpoints) {
		makePartialUpdate()
		partialUpdate.Spawnpoints = &spawnpoints
		nest.Spawnpoints = spawnpoints
	}

	if !active.Equal(nest.Active) {
		makePartialUpdate()
		partialUpdate.Active = active.Ptr()
		nest.Active = active
	}

	if !active.Bool && nest.PokemonId.Valid {
		makePartialUpdate()
		nest.PokemonId.Valid = false
		partialUpdate.PokemonId = &nest.PokemonId
		nest.PokemonForm.Valid = false
		partialUpdate.PokemonForm = &nest.PokemonForm
		nest.PokemonAvg.Valid = false
		partialUpdate.PokemonAvg = &nest.PokemonAvg
		nest.PokemonCount.Valid = false
		partialUpdate.PokemonCount = &nest.PokemonCount
	}

	if partialUpdate == nil {
		// nothing to do
		return origNest, nil
	}

	nest.Updated = null.IntFrom(time.Now().Unix())
	partialUpdate.Updated = &nest.Updated

	err = refresher.nestsDBStore.UpdateNestPartial(ctx, nest.NestId, partialUpdate)
	if err != nil {
		discardedStr := discarded.ValueOrZero()
		if !discarded.Valid {
			discardedStr = "<nil>"
		}
		refresher.logger.Errorf(
			"DB-REFRESHER[%s]: Failed to update nest to active=%t, discarded=%s: %v",
			fullName,
			active.Bool,
			discardedStr,
			err,
		)
		return origNest, err
	}

	return nest, nil
}

func (refresher *DBRefresher) RefreshNest(ctx context.Context, config RefreshNestConfig, nest db_store.Nest) (db_store.Nest, error) {
	return refresher.refreshNest(ctx, config, nest)
}

func (refresher *DBRefresher) RefreshAllNests(ctx context.Context, config RefreshNestConfig) error {
	err := refresher.nestsDBStore.IterateNestsConcurrently(
		ctx,
		db_store.IterateNestsConcurrentlyOpts{
			Concurrency:    config.Concurrency,
			IncludePolygon: true,
		},
		func(nest db_store.Nest) error {
			_, err := refresher.refreshNest(ctx, config, nest)
			return err
		},
	)

	if err != nil {
		return err
	}

	if config.MaxOverlapPercent < 0 || config.MaxOverlapPercent >= 100 {
		refresher.logger.Infof("DB-REFRESHER: Skipping overlap disablement due to overlap_max_percent=%0.3f", config.MaxOverlapPercent)
	} else {
		refresher.logger.Infof("DB-REFRESHER: Starting overlap disabling... this may take a while...")
		numDisabled, err := refresher.nestsDBStore.DisableOverlappingNests(ctx, config.MaxOverlapPercent)
		if err != nil {
			refresher.logger.Errorf("DB-REFRESHER: Overlap disablement errored: %v", err)
			return err
		}
		refresher.logger.Infof("DB-REFRESHER: Finished overlap disablement. Disabled %d nest(s)", numDisabled)
	}

	return nil
}

func NewDBRefresher(logger *logrus.Logger, nestsDBStore *db_store.NestsDBStore, golbatDBStore *db_store.GolbatDBStore) *DBRefresher {
	return &DBRefresher{
		logger:        logger,
		nestsDBStore:  nestsDBStore,
		golbatDBStore: golbatDBStore,
	}
}
