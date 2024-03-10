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

	"github.com/paulmach/orb/geojson"

	"github.com/UnownHash/Fletchling/app_config"
	"github.com/UnownHash/Fletchling/areas"
	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/exporters"
	"github.com/UnownHash/Fletchling/filters"
	"github.com/UnownHash/Fletchling/importer"
	"github.com/UnownHash/Fletchling/importers"
	"github.com/UnownHash/Fletchling/overpass"
	"github.com/UnownHash/Fletchling/version"
)

const (
	LOGFILE_NAME                  = "fletchling-osm-importer.log"
	DEFAULT_CONFIG_FILENAME       = "./configs/fletchling.toml"
	DEFAULT_NESTS_MIGRATIONS_PATH = "./db_store/sql"
)

func usage(flagSet *flag.FlagSet, output io.Writer) {
	fmt.Fprintf(output, "** A wild Fletchling has appeared. Version %s **\n", version.APP_VERSION)
	fmt.Fprintf(output, "Usage: %s [-help] [-debug] [-skip-activation] [-all-areas] [-f configfile] [<area-name>]\n", os.Args[0])
	fmt.Fprint(output, "\n")
	fmt.Fprintf(output, "%s is used to import data from overpass into your nests db. This is generally ", os.Args[0])
	fmt.Fprint(output, "something you do once for all of your areas.\n")
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "<area-name>:\n")
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "This argument is required if you do not use the '-all-areas' option.\n")
	fmt.Fprint(output, "This is used to import a single area only.\n")
	fmt.Fprint(output, "\n")

	fmt.Fprint(output, "Options:\n")
	flagSet.SetOutput(output)
	flagSet.PrintDefaults()

	fmt.Fprint(output, "\nExamples (follow unix shell escaping rules!):\n")
	fmt.Fprintf(output, "%s -all-areas\n", os.Args[0])
	fmt.Fprintf(output, "%s 'My Area Name'\n", os.Args[0])
	fmt.Fprintf(output, "%s \"My Area's Name\"\n", os.Args[0])
	fmt.Fprint(output, "\n")
}

func main() {
	flagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	helpFlag := flagSet.Bool("help", false, "help!")
	debugFlag := flagSet.Bool("debug", false, "override config and turn on debug logging")
	flagSet.BoolVar(helpFlag, "h", false, "help!")
	configFileFlag := flagSet.String("f", DEFAULT_CONFIG_FILENAME, "config file to use")
	allAreasFlag := flagSet.Bool("all-areas", false, "import all areas found in your areas source")
	skipActivateFlag := flagSet.Bool("skip-activation", false, "skips the final spawnpoint count gathering, filtering, and activation of new nests")

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

	args := flagSet.Args()
	l := len(args)

	if (*allAreasFlag && l != 0) || (!*allAreasFlag && l != 1) {
		if l == 0 {
			usage(flagSet, os.Stdout)
			os.Exit(0)
		}
		if *allAreasFlag && l == 1 {
			fmt.Fprint(os.Stderr, "Error: do not specify an area if '-all-areas' is given\n")
			fmt.Fprintf(os.Stderr, "Try %s -help for help.\n", os.Args[0])
		}
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

	importerImpl, err := importers.NewDBImporter(logger, nestsDBStore)
	if err != nil {
		logger.Errorf("Error: couldn't create importer: %v", err)
		os.Exit(1)
	}

	areasLoader, err := areas.NewAreasLoader(logger, cfg.Areas)
	if err != nil {
		logger.Errorf("Error: couldn't create areas loader: %v", err)
		os.Exit(1)
	}

	var dbRefresher *filters.DBRefresher

	if !*skipActivateFlag {
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

		dbRefresher = filters.NewDBRefresher(logger, nestsDBStore, golbatDBStore)
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

	if err := areasLoader.ReloadAreas(ctx); err != nil {
		logger.Errorf("Failed to load areas: %v.", err)
		os.Exit(1)
	}

	var overpassAreas []*geojson.Feature

	if *allAreasFlag {
		overpassAreas = areasLoader.GetAllAreas(ctx)
		if len(overpassAreas) == 0 {
			logger.Errorf("No areas were loaded/returned from source.")
			os.Exit(1)
		}
		logger.Infof("%d area geofence(s) loaded from source", len(overpassAreas))
	} else {
		areaName := args[0]
		overpassArea := areasLoader.GetArea(ctx, areaName)
		if overpassArea == nil {
			logger.Errorf("Could not find area '%s'", areaName)
			os.Exit(1)
		}

		logger.Info("1 area geofence loaded from source")

		overpassAreas = []*geojson.Feature{overpassArea}
	}

	for _, feature := range overpassAreas {
		areaName, _ := feature.Properties["name"].(string)

		overpassCli, err := overpass.NewClient(logger, cfg.Overpass.Url)
		if err != nil {
			logger.Fatalf("failed to create overpass client for area %s: %v", areaName, err)
		}

		exporterImpl, err := exporters.NewOverpassExporter(logger, overpassCli, feature)
		if err != nil {
			logger.Fatalf("failed to create overpass exporter for area %s: %v", areaName, err)
		}

		logger.Infof("Importing area %s...", areaName)

		runner, err := importer.NewImportRunner(logger, cfg.Importer, importerImpl, exporterImpl)
		if err != nil {
			logger.Fatal(err)
		}

		err = runner.Import(ctx)
		if err != nil {
			logger.Fatal(err)
		}
	}

	if dbRefresher != nil {
		logger.Infof("Gathering missing spawnpoints, running filters, and activating/deactivating nests...")
		refreshConfig := filters.RefreshNestConfig{
			Concurrency:       cfg.Filters.Concurrency,
			MinAreaM2:         cfg.Filters.MinAreaM2,
			MaxAreaM2:         cfg.Filters.MaxAreaM2,
			MinSpawnpoints:    cfg.Filters.MinSpawnpoints,
			MaxOverlapPercent: cfg.Filters.MaxOverlapPercent,
		}
		err := dbRefresher.RefreshAllNests(ctx, refreshConfig)
		if err == nil {
			logger.Infof("Done activating/deactivating nests")
		} else {
			logger.Fatalf("failed to filter and activate/deactivate nests: %v", err)
		}
	}

	logger.Infof("Done.")
}
