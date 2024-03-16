package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/UnownHash/Fletchling/filters"
	"github.com/UnownHash/Fletchling/pyroscope"
	"github.com/UnownHash/Fletchling/stats_collector"
	"github.com/UnownHash/Fletchling/version"
	"github.com/UnownHash/Fletchling/webhook_sender"

	"github.com/UnownHash/Fletchling/app_config"
	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/httpserver"
	"github.com/UnownHash/Fletchling/processor"
	"github.com/UnownHash/Fletchling/processor/nest_loader"
)

const (
	LOGFILE_NAME                  = "fletchling.log"
	DEFAULT_CONFIG_FILENAME       = "configs/fletchling.toml"
	DEFAULT_NESTS_MIGRATIONS_PATH = "./db_store/sql"
)

func usage(flagSet *flag.FlagSet, output io.Writer) {
	fmt.Fprintf(output, "** A wild Fletchling has appeared. Version %s **\n", version.APP_VERSION)
	fmt.Fprintf(output, "Usage: %s [-debug] [-help] [-f <config-filename>]\n", os.Args[0])
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "Options:\n")
	flagSet.SetOutput(output)
	flagSet.PrintDefaults()
	fmt.Fprint(output, "\n")
}

func main() {
	flagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	helpFlag := flagSet.Bool("help", false, "help!")
	debugFlag := flagSet.Bool("debug", false, "override config and turn on debug logging")
	flagSet.BoolVar(helpFlag, "h", false, "help!")
	configFileFlag := flagSet.String("f", DEFAULT_CONFIG_FILENAME, "config file to use")

	err := flagSet.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s", err)
		usage(flagSet, os.Stderr)
		os.Exit(2)
	}

	if *helpFlag {
		usage(flagSet, os.Stdout)
		os.Exit(0)
	}

	if len(flagSet.Args()) != 0 {
		usage(flagSet, os.Stderr)
		os.Exit(1)
	}

	defaultConfig := app_config.GetDefaultConfig()
	configFilename := *configFileFlag
	cfg, err := app_config.LoadConfig(configFilename, defaultConfig)
	if err != nil {
		log.Fatal(err)
	}
	cfg.Logging.Filename = LOGFILE_NAME

	if *debugFlag {
		cfg.Logging.Debug = true
	}

	logger := cfg.CreateLogger(true)
	logger.Infof("STARTUP: Version %s. Config loaded.", version.APP_VERSION)

	statsCollector := stats_collector.GetStatsCollector(cfg)
	logger.Infof("STARTUP: using %s stats collector", statsCollector.Name())

	if cfg.Pyroscope.ServerAddress != "" {
		if err := pyroscope.Run(cfg.Pyroscope); err != nil {
			logger.Errorf("STARTUP: Failed to Initialized pyroscope: %v", err)
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

	nestsDBStore, err := db_store.NewNestsDBStore(cfg.NestsDb, logger)
	if err != nil {
		logger.Fatalf("failed to create nests dbStore: %v", err)
	}

	if err := nestsDBStore.Migrate(DEFAULT_NESTS_MIGRATIONS_PATH); err != nil {
		logger.Fatalf("failed to run nests db migrations: %v", err)
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

	nestLoader := nest_loader.NewDBNestLoader(logger, nestsDBStore)
	logger.Debugf("STARTUP: nest loader (db) inited.")

	dbRefresher := filters.NewDBRefresher(
		logger,
		nestsDBStore,
		golbatDBStore,
	)

	var poracleWebhookSender *webhook_sender.PoracleSender
	var webhookSender processor.WebhookSender

	if len(cfg.Webhooks) > 0 {
		poracleWebhookSender, err = webhook_sender.NewPoracleSender(logger, cfg.Webhooks, cfg.WebhookSettings)
		if err != nil {
			logger.Fatal(err)
		}
		webhookSender = poracleWebhookSender
	} else {
		webhookSender = webhook_sender.NewNoopSender()
	}

	processorManagerConfig := processor.NestProcessorManagerConfig{
		Logger:         logger,
		NestsDBStore:   nestsDBStore,
		GolbatDBStore:  golbatDBStore,
		NestLoader:     nestLoader,
		StatsCollector: statsCollector,
		WebhookSender:  webhookSender,
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

	var filtersConfigMutex sync.Mutex
	filtersConfig := cfg.Filters

	getFiltersConfigFn := func() filters.FiltersConfig {
		filtersConfigMutex.Lock()
		defer filtersConfigMutex.Unlock()
		return filtersConfig
	}

	cfg.Filters.Log(logger, "STARTUP: Filters config loaded: ")

	reloadFn := func() error {
		cfg, err := app_config.LoadConfig(configFilename, defaultConfig)
		if err != nil {
			return fmt.Errorf("failed to reload config file: %w", err)
		}
		err = processorManager.LoadConfig(ctx, cfg.Processor)
		if err != nil {
			return fmt.Errorf("failed to reload processor manager: %w", err)
		}
		cfg.Filters.Log(logger, "Filters config reloaded: ")
		filtersConfigMutex.Lock()
		defer filtersConfigMutex.Unlock()
		filtersConfig = cfg.Filters
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

	if poracleWebhookSender == nil {
		logger.Debugf("STARTUP: Not starting webhook sender due to no webhooks configured.")
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancelFn()

			poracleWebhookSender.Run(ctx)
			logger.Debugf("STARTUP: webhook sender started.")
		}()
	}

	httpServer, err := httpserver.NewHTTPServer(logger, processorManager, statsCollector, dbRefresher, reloadFn, getFiltersConfigFn)
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
