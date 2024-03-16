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

	"github.com/UnownHash/Fletchling/app_config"
	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/filters"
	"github.com/UnownHash/Fletchling/version"
)

const (
	LOGFILE_NAME                  = "fletchling-db-refresher.log"
	DEFAULT_CONFIG_FILENAME       = "./configs/fletchling.toml"
	DEFAULT_NESTS_MIGRATIONS_PATH = "./db_store/sql"
)

func usage(flagSet *flag.FlagSet, output io.Writer) {
	fmt.Fprintf(output, `** A wild Fletchling has appeared. Version %s **
Usage: %s [-help] [-debug] [-f configfile] [-all-spawnpoints]

%s is used to re-run the filtering of nests in the DB.
This will activate and deactivate nests based on your current configuration
under the 'filters' section.

As a part of this, spawnpoints will be computed for a nest if they are not
already known or if the -all-spawnpoints option was given. (This occurs only
if the Golbat DB is configured.)

This does not reload Fletchling afterwards. You will need to issue the API
call.

`,
		version.APP_VERSION, os.Args[0], os.Args[0])

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
	allSpawnpointsFlag := flagSet.Bool("all-spawnpoints", false, "re-compute spawnpoints for all nests, even if they are already known")
	versionFlag := flagSet.Bool("version", false, "print the version of this tool and exit")

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

	if *versionFlag {
		fmt.Fprintf(os.Stdout, "%s\n", version.APP_VERSION)
		os.Exit(0)
	}

	if args := flagSet.Args(); len(args) != 0 {
		usage(flagSet, os.Stdout)
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

	logger, err := cfg.CreateLogger(true, os.Stdout)
	if err != nil {
		log.Fatal(err)
	}
	logger.Infof("STARTUP: Version %s. Config loaded.", version.APP_VERSION)

	// check destination first before we attempt to load
	// area fences.
	nestsDBStore, err := db_store.NewNestsDBStore(cfg.NestsDb, logger)
	if err != nil {
		logger.Errorf("failed to init nests db for db importer: %v", err)
		os.Exit(1)
	}

	if _, _, err := nestsDBStore.CheckMigrate(DEFAULT_NESTS_MIGRATIONS_PATH); err != nil {
		logger.Errorf("error initing nests db: %v", err)
		os.Exit(1)
	}

	var golbatDBStore *db_store.GolbatDBStore

	if cfg.GolbatDb == nil {
		logger.Warnf("Skipping spawnpoint count gathering and filtering: no golbat_db configured")
	} else {
		// check destination first before we attempt to load
		// area fences.
		var err error
		golbatDBStore, err = db_store.NewGolbatDBStore(*cfg.GolbatDb, logger)
		if err != nil {
			logger.Errorf("failed to init golbat db for spawnpoint counts: %v", err)
			os.Exit(1)
		}
	}

	dbRefresher := filters.NewDBRefresher(logger, nestsDBStore, golbatDBStore)

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

	cfg.Filters.Log(logger, "Using config: ")

	logger.Infof("Gathering missing spawnpoints, running filters, and activating/deactivating nests...")
	refreshConfig := filters.RefreshNestConfig{
		FiltersConfig:           cfg.Filters,
		Concurrency:             cfg.Filters.Concurrency,
		ForceSpawnpointsRefresh: *allSpawnpointsFlag,
	}

	err = dbRefresher.RefreshAllNests(ctx, refreshConfig)
	if err == nil {
		logger.Infof("Done activating/deactivating nests")
	} else {
		logger.Fatalf("failed to filter and activate/deactivate nests: %v", err)
	}

	logger.Infof("Done. Don't forget to tell Fletchling to reload!")
}
