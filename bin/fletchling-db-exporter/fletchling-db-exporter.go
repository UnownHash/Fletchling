package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/UnownHash/Fletchling/app_config"
	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/version"
	"github.com/paulmach/orb/geojson"
)

const (
	LOGFILE_NAME                  = "fletchling-db-exporter.log"
	DEFAULT_CONFIG_FILENAME       = "./configs/fletchling.toml"
	DEFAULT_NESTS_MIGRATIONS_PATH = "./db_store/sql"
)

func usage(flagSet *flag.FlagSet, output io.Writer) {
	fmt.Fprintf(output, `** A wild Fletchling has appeared. Version %s **
Usage: %s [-help] [-debug] [-f configfile] [-include-inactive] [-all-areas] [-area <area-name>]

%s is used to export multiple nests from the DB as a geojson FeatureCollection
or a single nest as a geojson Feature.

Output will go to stdout, so just redirect output to a file. Logging
will go to stderr and to a logfile.
`,
		version.APP_VERSION, os.Args[0], os.Args[0])

	fmt.Fprint(output, "Options:\n")
	flagSet.SetOutput(output)
	flagSet.PrintDefaults()

	fmt.Fprintf(output, `
Examples (follow unix shell escaping rules!):
%s -all-areas > all-areas.geojson
%s -area 'My Area Name' > myarea.geojson
%s -area "My Area's Name" > myarea.geojson
%s -nest-id 12345 > mynest.geojson
`,
		os.Args[0], os.Args[0], os.Args[0], os.Args[0])
}

func main() {
	flagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	helpFlag := flagSet.Bool("help", false, "help!")
	debugFlag := flagSet.Bool("debug", false, "override config and turn on debug logging")
	flagSet.BoolVar(helpFlag, "h", false, "help!")
	configFileFlag := flagSet.String("f", DEFAULT_CONFIG_FILENAME, "config file to use")
	includeInactiveFlag := flagSet.Bool("include-inactive", false, "include inactive nests, also")
	allAreasFlag := flagSet.Bool("all-areas", false, "import all areas found in your areas source")
	onlyAreaFlag := flagSet.String("area", "", "export only 1 area")
	nestIdFlag := flagSet.Int64("nest-id", 0, "export only 1 nest")
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

	if len(flagSet.Args()) > 0 {
		usage(flagSet, os.Stderr)
		os.Exit(0)
	}

	if *allAreasFlag {
		if *onlyAreaFlag != "" || *nestIdFlag != 0 {
			fmt.Fprint(os.Stderr, "Error: do not specify an '-area' or '-nest-id' if '-all-areas' is given\n")
			fmt.Fprintf(os.Stderr, "Try %s -help for help.\n", os.Args[0])
			os.Exit(1)
		}
	} else if *onlyAreaFlag != "" && *nestIdFlag != 0 {
		fmt.Fprint(os.Stderr, "Error: do not specify an '-area' if '-nest-id' is given\n")
		fmt.Fprintf(os.Stderr, "Try %s -help for help.\n", os.Args[0])
		os.Exit(1)
	} else if *onlyAreaFlag == "" && *nestIdFlag == 0 {
		fmt.Fprint(os.Stderr, "Error: one of '-all-areas', '-area', or '-nest-id' is required\n")
		fmt.Fprintf(os.Stderr, "Try %s -help for help.\n", os.Args[0])
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

	logger, err := cfg.CreateLogger(true, os.Stderr)
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

	encoder := json.NewEncoder(os.Stdout)

	if *nestIdFlag != 0 {
		nest, err := nestsDBStore.GetNestById(ctx, *nestIdFlag)

		if err != nil {
			logger.Fatalf("failed to get nest from DB: %v", err)
		}

		if nest == nil {
			logger.Fatalf("nest %d not found", *nestIdFlag)
		}

		feature, err := nest.AsFeature()
		if err != nil {
			logger.Fatal(err)
		}

		err = encoder.Encode(feature)
		if err != nil {
			logger.Fatal(err)
		}
	}

	areasProcessed := make(map[string]int)
	featureCollection := geojson.NewFeatureCollection()

	nestsCh := make(chan db_store.Nest, 64)

	var streamWg sync.WaitGroup
	defer streamWg.Wait()

	streamWg.Add(1)
	go func() {
		defer streamWg.Done()

		for {
			var nest db_store.Nest
			var ok bool

			select {
			case <-ctx.Done():
				return
			case nest, ok = <-nestsCh:
				if !ok {
					return
				}
			}

			if !*includeInactiveFlag && !nest.Active.ValueOrZero() {
				continue
			}

			areaName := nest.AreaName.ValueOrZero()
			if splitted := strings.Split(areaName, "/"); len(splitted) == 2 {
				areaName = splitted[1]
			}

			if *onlyAreaFlag != "" && *onlyAreaFlag != areaName {
				continue
			}

			feature, err := nest.AsFeature()
			if err != nil {
				logger.Warn(err)
				continue
			}

			areasProcessed[areaName]++

			featureCollection.Append(feature)
		}
	}()

	logger.Infof("Starting export...")

	err = nestsDBStore.StreamNests(ctx, db_store.StreamNestsOpts{IncludePolygon: true}, nestsCh)
	if err != nil {
		logger.Fatal(err)
	}

	streamWg.Wait()

	err = encoder.Encode(featureCollection)
	if err != nil {
		logger.Fatal(err)
	}
}
