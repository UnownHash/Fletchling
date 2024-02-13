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
	"github.com/UnownHash/Fletchling/stats_collector"
)

type NestLoader interface {
	LoaderName() string
	LoadNests(context.Context) ([]*models.Nest, error)
}

type WebhookSender interface {
	AddNestWebhook(*models.Nest, *models.NestingPokemonInfo)
}

type NestProcessorManagerConfig struct {
	Logger            *logrus.Logger
	NestLoader        NestLoader
	NestsDBStore      *db_store.NestsDBStore
	GolbatDBStore     *db_store.GolbatDBStore
	KojiClient        *koji_client.APIClient
	KojiProjectName   string
	NestingPokemonURL string
	StatsCollector    stats_collector.StatsCollector
	WebhookSender     WebhookSender
}

type NestProcessorManager struct {
	logger          *logrus.Logger
	nestsDBStore    *db_store.NestsDBStore
	golbatDBStore   *db_store.GolbatDBStore
	nestLoader      NestLoader
	kojiCli         *koji_client.APIClient
	kojiProjectName string
	statsCollector  stats_collector.StatsCollector
	webhookSender   WebhookSender

	reloadCh    chan struct{}
	reloadMutex sync.Mutex
	config      Config

	pokemonProcessedCount atomic.Uint64
	nestsMatchedCount     atomic.Uint64

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
	resp := mgr.GetNestProcessor().AddPokemon(pokemon)
	mgr.pokemonProcessedCount.Add(1)
	// webhook handler adds pokemon processed to statsCollector.
	mgr.nestsMatchedCount.Add(resp.NumNestsMatched)
	mgr.statsCollector.AddNestsMatched(resp.NumNestsMatched)
	if resp.NumNestsMatched > 0 {
		mgr.statsCollector.AddPokemonMatched(1)
	}
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
			pokemonCnt := mgr.pokemonProcessedCount.Swap(0)
			nestsCnt := mgr.nestsMatchedCount.Swap(0)
			mgr.logger.Infof("PROCESSOR: last minute: processed %d pokemon, matched %d nest(s)", pokemonCnt, nestsCnt)
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
		if partial.Updated == nil {
			now := time.Now()
			nest.SetUpdatedAt(now)
			nowInt := null.IntFrom(now.Unix())
			partial.Updated = &nowInt
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

	nest.SetUpdatedAt(time.Now())
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

// LoadConfig queries the db (or koji merged with db) for the geofences
// to use, filters them (min spawnpoints), etc, and sets up a new NestProcessor
// for them. The stats and some state will be preserved if there was an existing
// NestProcessor. LoadConfig() is used for both the initial configuration load as
// well as reloads.
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

	// first load all of the geofences into a new matcher. No spawnpoint caching anymore.
	nestMatcher := NewNestMatcher(mgr.logger, true)

	curNestProcessor := mgr.nestProcessor

	if mgr.golbatDBStore == nil {
		mgr.logger.Warnf("NEST-LOAD[]: No golbat DB configured. Missing spawnpoint counts in nests will not be able to be retrieved and will not be filtered")
	}

	for _, nest := range nests {
		fullName := nest.FullName()

		// area will be rechecked, other reasons we'll leave alone.
		if !nest.Active && nest.Discarded != "area" {
			mgr.logger.Warnf("NEST-LOAD[%s]: Nest is disabled (skipping this one).", fullName)
			continue
		}

		if _, ok := nestsById[nest.Id]; ok {
			// won't happen if nests come from DB. but possibly from other sources. we need
			// the nestsById mapping anyway, so might as well check.
			mgr.logger.Warnf("NEST-LOAD[%s]: Nest already exists (skipping this one).", fullName)
			continue
		}

		// do this before the spawnpoints query! Also do not save to db unless it's there already
		// and set to Active. Only update active/discarded.
		if err := filter.FilterArea(nest.AreaM2); err != nil {
			mgr.logger.Warnf("NEST-LOAD[%s]: skipping nest due to filter: %s", fullName, err)
			if !nest.Active {
				continue
			}
			nest.Active = false
			nest.Discarded = "area"
			discarded := null.StringFrom(nest.Discarded)
			var nullInt null.Int
			mgr.updateNestInDb(ctx, nest, &db_store.NestPartialUpdate{
				Active:      &nest.Active,
				Discarded:   &discarded,
				PokemonId:   &nullInt,
				PokemonForm: &nullInt,
			}, "disabling due to area filter.")
			continue
		}

		_, dbUpdatedAt := nest.GetNestingPokemon()

		dbNeedsSpawnpoints := nest.Spawnpoints == nil
		// The spawnpoints column has a DEFAULT 0. Depending on how they are inserted initially,
		// the default may have been used. But the default for Updated is NULL, so we can use
		// that, also. This ends up as a zero time in the Nest model.
		if !dbNeedsSpawnpoints && *nest.Spawnpoints == 0 && dbUpdatedAt.IsZero() {
			nest.Spawnpoints = nil
			dbNeedsSpawnpoints = true
		}

		if dbUpdatedAt.IsZero() {
			dbUpdatedAt = time.Now()
		}

		if dbNeedsSpawnpoints && mgr.golbatDBStore != nil {
			mgr.logger.Infof("NEST-LOAD[%s]: number of spawnpoints is unknown. Will query golbat for them.", fullName)

			numSpawnpoints, err := mgr.golbatDBStore.GetSpawnpointsCount(ctx, nest.Geometry)
			if err == nil {
				nest.Spawnpoints = &numSpawnpoints
			} else {
				mgr.logger.Warnf("NEST-LOAD[%s]: couldn't query spawnpoints: %#v", fullName, err)
			}
		}

		if nest.Spawnpoints == nil {
			mgr.logger.Warnf("NEST-LOAD[%s]: allowing nest with unknown number of spawnpoints due to no golbat DB config or query error", fullName)
		} else {
			if err := filter.FilterSpawnpoints(*nest.Spawnpoints); err != nil {
				mgr.logger.Warnf("NEST-LOAD[%s]: skipping nest: %s", fullName, err)

				// update only if the nest WAS active... or if we didn't
				// have the number of spawnpoints before.
				if !nest.Active && !dbNeedsSpawnpoints {
					continue
				}

				// go ahead and store the current spawnpoints so we won't
				// query again.
				nest.Active = false
				nest.Discarded = "spawnpoints"
				discarded := null.StringFrom(nest.Discarded)
				var nullInt null.Int
				mgr.addOrUpdateNestInDb(ctx, nest, &db_store.NestPartialUpdate{
					Spawnpoints: nest.Spawnpoints,
					Active:      &nest.Active,
					Discarded:   &discarded,
					PokemonId:   &nullInt,
					PokemonForm: &nullInt,
				}, "updating spawnpoints, disabling due to spawnpoints filter.")

				continue
			}
		}

		if curNestProcessor != nil {
			// safe to access map from previous config load. It is read only.
			oldNest := curNestProcessor.nestIdsToNests[nest.Id]
			if oldNest != nil {
				nest.NestStatsInfo = oldNest.NestStatsInfo
			}
		}

		nest.Discarded = ""

		// update if !active in DB.. or if db didn't have spawnpoints but we have them now
		if !nest.Active || (dbNeedsSpawnpoints && nest.Spawnpoints != nil) {
			nest.Active = true
			var discarded null.String
			mgr.addOrUpdateNestInDb(ctx, nest, &db_store.NestPartialUpdate{
				Spawnpoints: nest.Spawnpoints,
				Active:      &nest.Active,
				Discarded:   &discarded,
			}, "updating spawnpoints (if they were fetched) and enabling")
		}

		if err := nestMatcher.AddNest(nest, nil); err != nil {
			mgr.logger.Warnf("NEST-LOAD[%s]: Failed to add nest to matcher: %v", fullName, err)
			continue
		}

		if nest.Spawnpoints == nil {
			mgr.logger.Infof("NEST-LOAD[%s]: Nest loaded and active with unknown number of spawnpoints", fullName)
		} else {
			mgr.logger.Infof("NEST-LOAD[%s]: Nest loaded and active with %d spawnpoint(s)", fullName, *nest.Spawnpoints)
		}

		nestsById[nest.Id] = nest
	}

	nestProcessor := NewNestProcessor(mgr.nestProcessor, mgr.logger, mgr.nestsDBStore, nestMatcher, nestsById, mgr.webhookSender, config)
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
		statsCollector:  config.StatsCollector,
		webhookSender:   config.WebhookSender,
		reloadCh:        make(chan struct{}, 1),
	}
	return mgr, nil
}
