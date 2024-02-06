package processor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"

	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/koji_client"
	"github.com/UnownHash/Fletchling/processor/models"
)

type NestLoader interface {
	LoaderName() string
	LoadNests(context.Context) ([]*models.Nest, error)
}

type NestProcessorManagerConfig struct {
	Logger            *logrus.Logger
	NestLoader        NestLoader
	NestsDBStore      *db_store.NestsDBStore
	GolbatDBStore     *db_store.GolbatDBStore
	KojiClient        *koji_client.APIClient
	KojiProjectName   string
	NestingPokemonURL string
}

type NestProcessorManager struct {
	logger          *logrus.Logger
	nestsDBStore    *db_store.NestsDBStore
	golbatDBStore   *db_store.GolbatDBStore
	nestLoader      NestLoader
	kojiCli         *koji_client.APIClient
	kojiProjectName string

	reloadCh    chan struct{}
	reloadMutex sync.Mutex
	config      Config

	processedCount atomic.Uint64

	nestProcessorMutex sync.Mutex
	nestProcessor      *NestProcessor
}

// GetNestProcessor returns an object representing
// the current configuration and stats. This may be
// stale as soon as it is retrieved if a reload happens.
// But it's fine because nests that still exist in the
// new nestProcessor will have been given the stats structures
// from this one.
func (mgr *NestProcessorManager) GetNestProcessor() *NestProcessor {
	mgr.nestProcessorMutex.Lock()
	defer mgr.nestProcessorMutex.Unlock()
	return mgr.nestProcessor
}

func (mgr *NestProcessorManager) GetConfig() Config {
	return mgr.GetNestProcessor().GetConfig()
}

func (mgr *NestProcessorManager) GetNestByID(nestId int64) *models.Nest {
	return mgr.GetNestProcessor().GetNestByID(nestId)
}

func (mgr *NestProcessorManager) GetNests() []*models.Nest {
	return mgr.GetNestProcessor().GetNests()
}

func (mgr *NestProcessorManager) ProcessPokemon(pokemon *models.Pokemon) {
	mgr.GetNestProcessor().AddPokemon(pokemon)
	mgr.processedCount.Add(1)
}

func (mgr *NestProcessorManager) processStats(ctx context.Context, nestProcessor *NestProcessor) {
	mgr.logger.Infof("Rotating stats...")
	statsCollection := nestProcessor.RotateStats()
	mgr.logger.Infof("Done rotating stats.")
	if statsCollection != nil {
		go nestProcessor.ProcessStatsCollection(statsCollection)
	}
}

// Run runs the processor until `ctx` is cancelled. One must load
// a config via LoadConfig() before calling Run().
func (mgr *NestProcessorManager) Run(ctx context.Context) {
	nestProcessor := mgr.GetNestProcessor()
	if nestProcessor == nil {
		mgr.logger.Fatal("coding error: NestProcessorManager requires config loaded before calling Run()")
	}

	statsTimerStopped := false
	rotationInterval := nestProcessor.config.RotationInterval()
	statsTimerStart := time.Now()
	statsTimer := time.NewTimer(rotationInterval)
	defer func() {
		if !statsTimerStopped && !statsTimer.Stop() {
			<-statsTimer.C
		}
	}()

	logTimerStopped := false
	logInterval := time.Minute
	logTimer := time.NewTimer(logInterval)
	defer func() {
		if !logTimerStopped && !logTimer.Stop() {
			<-logTimer.C
		}
	}()

	// eat any pending reload signal.
	select {
	case <-mgr.reloadCh:
	default:
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-mgr.reloadCh:
			nestProcessor = mgr.GetNestProcessor()
			if newInterval := nestProcessor.config.RotationInterval(); newInterval != rotationInterval {
				mgr.logger.Infof("RELOAD: processing interval changed from %s to %s",
					rotationInterval, newInterval,
				)
				rotationInterval = newInterval
				passed := time.Now().Sub(statsTimerStart)
				statsTime := false
				// since timer hasn't fired, we have to stop first.
				if !statsTimer.Stop() {
					// we raced.
					<-statsTimer.C
					statsTime = true
				}
				statsTimerStopped = true
				if statsTime || passed > rotationInterval {
					mgr.logger.Infof("RELOAD: processing time hit during reload. Will process stats now.")
					mgr.processStats(ctx, nestProcessor)
					passed = 0
					statsTimerStart = time.Now()
				}
				statsTimer.Reset(rotationInterval - passed)
				statsTimerStopped = false
				mgr.logger.Infof("RELOAD: next processing time set for %s from now", (rotationInterval - passed).Truncate(time.Second))
			}
		case <-logTimer.C:
			logTimerStopped = true
			cnt := mgr.processedCount.Swap(0)
			mgr.logger.Infof("PROCESSOR: processed %d pokemon", cnt)
			logTimer.Reset(logInterval)
			logTimerStopped = false
		case <-statsTimer.C:
			statsTimerStopped = true
			mgr.processStats(ctx, nestProcessor)
			statsTimerStart = time.Now()
			statsTimer.Reset(rotationInterval)
			statsTimerStopped = false
		}
	}
}

func (mgr *NestProcessorManager) updateNestInDb(ctx context.Context, nest *models.Nest, partial *db_store.NestPartialUpdate, msg string) error {
	fullName := nest.FullName()

	if nest.ExistsInDb {
		if partial == nil {
			return nil
		}
		mgr.logger.Infof("NEST-LOAD[%s]: Updating nest in DB: %s", fullName, msg)
		err := mgr.nestsDBStore.UpdateNestPartial(ctx, nest.Id, partial)
		if err != nil {
			mgr.logger.Infof("NEST-LOAD[%s]: Failed to updating nest in DB: %s: %v", fullName, msg, err)
		}
		return err
	}
	return nil
}

func (mgr *NestProcessorManager) addOrUpdateNestInDb(ctx context.Context, nest *models.Nest, partial *db_store.NestPartialUpdate, msg string) error {
	if nest.SyncedToDb {
		return mgr.updateNestInDb(ctx, nest, partial, msg)
	}
	// came from a non-DB source. Sync up with DB.

	fullName := nest.FullName()
	mgr.logger.Infof("NEST-LOAD[%s]: Saving/Updating non-DB-sourced nest to DB", fullName)

	dbNest := nest.AsDBStoreNest()
	if err := mgr.nestsDBStore.InsertOrUpdateNest(ctx, dbNest); err == nil {
		nest.SyncedToDb = true
		nest.ExistsInDb = true
	} else {
		mgr.logger.Warnf("NEST-LOAD[%s]: Failed to insert/update non-DB-sourced nest to DB: %v", fullName, err)
		return err
	}
	return nil
}

// LoadConfig queries koji for the geofences to use based on the configured
// project and sets up all of the associated objects and state. This is
// used for both the initial configuration load as well as reloads.
// When an error occurs during reload, the previous configuration continues
// running.
//
// A config must be loaded prior to calling Run().
func (mgr *NestProcessorManager) LoadConfig(ctx context.Context, config Config) error {
	mgr.reloadMutex.Lock()
	defer mgr.reloadMutex.Unlock()

	filter := Filter{
		MinSpawnpoints: int64(config.MinSpawnpoints),
		MinArea:        config.MinAreaM2,
		MaxArea:        config.MaxAreaM2,
	}

	// nests will be filtered here.
	nests, err := mgr.nestLoader.LoadNests(ctx)
	if err != nil {
		return fmt.Errorf("failed to load nests: %w", err)
	}

	mgr.logger.Infof("NEST-LOAD[]: Got %d nest(s) from loader", len(nests))

	nestsById := make(map[int64]*models.Nest)

	// first load all of the geofences into a new matcher.
	nestMatcher := NewNestMatcher(mgr.logger, mgr.golbatDBStore == nil)

	curNestProcessor := mgr.nestProcessor

	if mgr.golbatDBStore == nil {
		mgr.logger.Warnf("NEST-LOAD[]: No golbat DB configured. Spawnpoints will be unavailable")
	}

	for _, nest := range nests {
		fullName := nest.FullName()

		// area and spawnpoints will be rechecked, other reasons we'll leave alone.
		if !nest.Active && nest.Discarded != "area" && nest.Discarded != "spawnpoints" {
			mgr.logger.Warnf("NEST-LOAD[%s]: Nest is disabled (skipping this one).", fullName)
			continue
		}

		if _, ok := nestsById[nest.Id]; ok {
			// shouldn't happen
			mgr.logger.Warnf("NEST-LOAD[%s]: Nest already exists (skipping this one).", fullName)
			continue
		}

		// do this before the spawnpoints query! Also do not save to db unless it's there already,
		// in which case only update active/discarded.
		if err := filter.FilterArea(nest.AreaM2); err != nil {
			nest.Active = false
			nest.Discarded = "area"
			discarded := null.StringFrom(nest.Discarded)
			mgr.updateNestInDb(ctx, nest, &db_store.NestPartialUpdate{
				Active:    &nest.Active,
				Discarded: &discarded,
			}, "disabling due to area filter.")
			mgr.logger.Warnf("NEST-LOAD[%s]: skipping nest due to filter: %s", fullName, err)
			continue
		}

		var numSpawnpoints int64
		var spawnpointIds []uint64

		if mgr.golbatDBStore != nil {
			var err error

			spawnpointIds, err = mgr.golbatDBStore.GetContainedSpawnpoints(ctx, nest.Geometry)
			if err != nil {
				mgr.logger.Warnf("NEST-LOAD[%s]: skipping nest: couldn't query spawnpoints: %#v", fullName, err)
				continue
			}

			numSpawnpoints = int64(len(spawnpointIds))
			nest.Spawnpoints = numSpawnpoints

			if err := filter.FilterSpawnpoints(numSpawnpoints); err != nil {
				nest.Active = false
				nest.Discarded = "spawnpoints"
				discarded := null.StringFrom(nest.Discarded)
				mgr.addOrUpdateNestInDb(ctx, nest, &db_store.NestPartialUpdate{
					Spawnpoints: &numSpawnpoints,
					Active:      &nest.Active,
					Discarded:   &discarded,
				}, "updating spawnpoints, disabling due to spawnpoints filter.")
				mgr.logger.Warnf("NEST-LOAD[%s]: skipping nest: %s", fullName, err)
				continue
			}
		}

		if curNestProcessor != nil {
			oldNest := mgr.nestProcessor.nestIdsToNests[nest.Id]
			if oldNest != nil {
				nest.NestStatsInfo = oldNest.NestStatsInfo
			}
		}

		discarded := null.StringFrom("")
		nest.Discarded = ""
		nest.Active = true

		if mgr.golbatDBStore == nil {
			mgr.addOrUpdateNestInDb(ctx, nest, &db_store.NestPartialUpdate{
				Active:    &nest.Active,
				Discarded: &discarded,
			}, "ensuring enabled")
		} else {
			mgr.addOrUpdateNestInDb(ctx, nest, &db_store.NestPartialUpdate{
				Spawnpoints: &numSpawnpoints,
				Active:      &nest.Active,
				Discarded:   &discarded,
			}, "updating spawnpoints, enabling")
		}

		if err := nestMatcher.AddNest(nest, spawnpointIds); err != nil {
			mgr.logger.Warnf("NEST-LOAD[%s]: Failed to add nest to matcher: %v", fullName, err)
			continue
		}

		mgr.logger.Infof("NEST-LOAD[%s]: Nest loaded and active with %d spawnpoint(s)", fullName, numSpawnpoints)
		nestsById[nest.Id] = nest
	}

	nestProcessor := NewNestProcessor(mgr.nestProcessor, mgr.logger, mgr.nestsDBStore, nestMatcher, nestsById, config)
	nestProcessor.LogConfiguration("Config loaded: ", len(nestsById))

	// now we can swap in the new state
	mgr.nestProcessorMutex.Lock()
	defer mgr.nestProcessorMutex.Unlock()

	mgr.nestProcessor = nestProcessor

	// if we can't push one, there's already one.
	select {
	case mgr.reloadCh <- struct{}{}:
	default:
	}

	return nil
}

func NewNestProcessorManager(config NestProcessorManagerConfig) (*NestProcessorManager, error) {
	mgr := &NestProcessorManager{
		logger:          config.Logger,
		nestsDBStore:    config.NestsDBStore,
		golbatDBStore:   config.GolbatDBStore,
		kojiCli:         config.KojiClient,
		kojiProjectName: config.KojiProjectName,
		nestLoader:      config.NestLoader,
		reloadCh:        make(chan struct{}, 1),
	}
	return mgr, nil
}
