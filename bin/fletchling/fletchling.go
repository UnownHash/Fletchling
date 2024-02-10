package main

import (
	"context"
	"fmt"
	"github.com/UnownHash/Fletchling/pyroscope"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/httpserver"
	"github.com/UnownHash/Fletchling/koji_client"
	"github.com/UnownHash/Fletchling/processor"
	"github.com/UnownHash/Fletchling/processor/nest_loader"
)

const (
	DEFAULT_CONFIG_FILENAME       = "configs/fletchling.toml"
	DEFAULT_NESTS_MIGRATIONS_PATH = "./db_store/sql"
)

func main() {
	var configFilename string

	switch len(os.Args) {
	case 1:
		configFilename = DEFAULT_CONFIG_FILENAME
	case 2:
		configFilename = os.Args[1]
	default:
		log.Fatalf("Usage: %s [<config-filename>]", os.Args[0])
	}

	cfg, err := LoadConfig(configFilename)
	if err != nil {
		log.Fatal(err)
	}

	logger := cfg.CreateLogger(true)

	logger.Infof("STARTUP: config loaded.")

	pyroscopeStatus := pyroscope.Run(cfg.Pyroscope)
	if pyroscopeStatus.Started {
		if pyroscopeStatus.Error != nil {
			logger.Error("STARTUP: Failed to Initialized pyroscope")
		} else {
			logger.Info("STARTUP: Initialized pyroscope")
		}
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancelFn()

		sig_ch := make(chan os.Signal, 1)
		signal.Notify(sig_ch, syscall.SIGINT, syscall.SIGTERM)
		select {
		case <-ctx.Done():
			// something else told us to exit
		case sig := <-sig_ch:
			logger.Infof("received signal '%s'", sig.String())
		}
	}()

	logger.Debugf("STARTUP: signal handler installed.")

	nestsDBStore, err := db_store.NewNestsDBStore(cfg.NestsDb, logger, DEFAULT_NESTS_MIGRATIONS_PATH)
	if err != nil {
		logger.Fatalf("failed to create nests dbStore: %v", err)
	}

	var golbatDBStore *db_store.GolbatDBStore

	if cfg.GolbatDb != nil {
		dbStore, err := db_store.NewGolbatDBStore(*cfg.GolbatDb, logger)
		if err != nil {
			logger.Fatalf("failed to create golbat dbStore: %v", err)
		}
		golbatDBStore = dbStore
	}

	logger.Debugf("STARTUP: store inited.")

	var nestLoader processor.NestLoader

	if cfg.Koji == nil {
		nestLoader = nest_loader.NewDBNestLoader(logger, nestsDBStore)
	} else {
		kojiClient, err := koji_client.NewClient(logger, cfg.Koji.Url, cfg.Koji.Token)
		if err != nil {
			logger.Fatal(err)
		}
		nestLoader = nest_loader.NewKojiNestLoader(logger, kojiClient.APIClient, cfg.Koji.Project, nestsDBStore)
	}

	logger.Debugf("STARTUP: koji client inited.")

	processorManagerConfig := processor.NestProcessorManagerConfig{
		Logger:        logger,
		NestsDBStore:  nestsDBStore,
		GolbatDBStore: golbatDBStore,
		NestLoader:    nestLoader,
	}

	logger.Debugf("STARTUP: initializing processor.")

	processorManager, err := processor.NewNestProcessorManager(processorManagerConfig)
	if err != nil {
		logger.Fatalf("failed to create processor manager: %v", err)
	}

	if err := processorManager.LoadConfig(ctx, cfg.Processor); err != nil {
		logger.Fatalf("failed to load config into NestProcessorManager: %v", err)
	}

	logger.Debugf("STARTUP: processor initialized.")

	reloadFn := func() error {
		cfg, err := LoadConfig(configFilename)
		if err != nil {
			return fmt.Errorf("failed to reload config file: %w", err)
		}
		err = processorManager.LoadConfig(ctx, cfg.Processor)
		if err != nil {
			return fmt.Errorf("failed to reload processor manager: %w", err)
		}
		return nil
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer cancelFn()

		sig_ch := make(chan os.Signal, 1)
		signal.Notify(sig_ch, syscall.SIGHUP)
		for {
			select {
			case <-ctx.Done():
				// something else told us to exit
				return
			case sig := <-sig_ch:
				logger.Infof("received signal '%s' -- Reloading config.", sig.String())
				err := reloadFn()
				if err == nil {
					logger.Infof("processor config reloaded")
				} else {
					logger.Error(err)
				}
			}
		}
	}()
	logger.Debugf("STARTUP: installed reload (SIGHUP) handler")

	wg.Add(1)
	go func() {
		defer wg.Done()
		// shut down everything else if this bails early
		defer cancelFn()

		processorManager.Run(ctx)
	}()

	logger.Debugf("STARTUP: processor started.")

	httpServer, err := httpserver.NewHTTPServer(logger, processorManager, reloadFn)
	if err != nil {
		logger.Fatalf("failed to create http server: %v", err)
	}

	logger.Infof("STARTUP: starting http server (final step)")
	err = httpServer.Run(ctx, cfg.HTTP.Addr, time.Second*5)
	if err != nil {
		logger.Fatalf("failed to run http server: %v", err)
	}

	// http server could have shut down early or not started. The defers
	// above will cancel and wait for things to shutdown cleanly.
}
