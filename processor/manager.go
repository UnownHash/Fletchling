package processor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"

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

func (mgr *NestProcessorManager) GetNestById(nestId int64) *models.Nest {
	return mgr.GetNestProcessor().GetNestById(nestId)
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

// LoadConfig loads all active nests from the nests db and sets up a new
// NestProcessor for them. No filtering is performed. Whatever is active
// in the DB will be loaded.
// LoadConfig is used for both the initial configuration load as well as
// reloads.
// If this is a reload, any nests that were already active will have the
// existing stats copied to these freshly loaded nests.
// Any errors that occur during reload will not affect the running processing.
//
// A config must be loaded prior to calling Run().
func (mgr *NestProcessorManager) LoadConfig(ctx context.Context, config Config) error {
	// allow only 1 reload at a time. While mgr.nestProcessor has its own lock, it
	// can never change without holding the reloadMutex, so we can access it safely.
	// If reload succeeds, we'll grab the other lock to replace mgr.nestProcessor.
	mgr.reloadMutex.Lock()
	defer mgr.reloadMutex.Unlock()

	dbNests, err := mgr.nestsDBStore.GetActiveNests(ctx)
	if err != nil {
		return fmt.Errorf("failed to load active nests from the DB: %w", err)
	}

	mgr.logger.Infof("NEST-LOAD[]: Loaded %d active nest(s) from the DB", len(dbNests))

	nestMatcher := NewNestMatcher(mgr.logger)
	curNestProcessor := mgr.nestProcessor

	for _, dbNest := range dbNests {
		nest, err := models.NewNestFromDBStore(dbNest)
		if err != nil {
			mgr.logger.Warnf("NEST-LOAD[%s]: skipping nest id '%d': %v", dbNest.Name, dbNest.NestId, err)
			continue
		}

		if curNestProcessor != nil {
			oldNest := curNestProcessor.nestMatcher.GetNestById(nest.Id)
			if oldNest != nil {
				nest.NestStatsInfo = oldNest.NestStatsInfo
			}
		}

		fullName := nest.FullName()

		if err := nestMatcher.AddNest(nest); err != nil {
			mgr.logger.Warnf("NEST-LOAD[%s]: Failed to add nest to matcher: %v", fullName, err)
			continue
		}

		var spawnpointsStr string

		if nest.Spawnpoints == nil {
			spawnpointsStr = "unknown number of spawnpoints"
		} else {
			spawnpointsStr = fmt.Sprintf("%d spawnpoint(s)", *nest.Spawnpoints)
		}

		mgr.logger.Infof("NEST-LOAD[%s]: Nest loaded with %s covering %0.3f meters squared", fullName, spawnpointsStr, nest.AreaM2)
	}

	nestProcessor := NewNestProcessor(mgr.nestProcessor, mgr.logger, mgr.nestsDBStore, nestMatcher, mgr.webhookSender, config)
	nestProcessor.LogConfiguration("Config loaded: ", nestMatcher.Len())

	// now we can swap in the new state
	mgr.nestProcessorMutex.Lock()
	defer mgr.nestProcessorMutex.Unlock()

	mgr.nestProcessor = nestProcessor

	// Notify the background goroutine that config was reloaded. If we can't
	// push into the channel, it means there's already a reload queued up, so
	// just move on.
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
