package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/paulmach/orb/geojson"
	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/geo"
	"github.com/UnownHash/Fletchling/importer"
	"github.com/UnownHash/Fletchling/importer/exporters"
	"github.com/UnownHash/Fletchling/importer/importers"
	"github.com/UnownHash/Fletchling/koji_client"
	"github.com/UnownHash/Fletchling/overpass"
)

const (
	DEFAULT_CONFIG_FILENAME = "./configs/fletchling-osm-importer.toml"
)

func usage(flagSet *flag.FlagSet, output io.Writer) {
	fmt.Fprintf(output, "Usage: %s [-help] [-all-areas] [-f configfile] [<area-name>]\n", os.Args[0])
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

func featuresFromKoji(logger *logrus.Logger, urlStr, token string) ([]*geojson.Feature, error) {
	// already checked by config validator
	uri, _ := url.Parse(urlStr)

	var baseUrl, project string

	const fcStr = "/feature-collection/"
	idx := strings.Index(uri.Path, fcStr)
	if idx >= 0 {
		// get base url and project
		project = uri.Path[idx+len(fcStr):]
		uri.Path = ""
		baseUrl = uri.String()
	}

	if baseUrl == "" || project == "" {
		return nil, errors.New("there's a problem with your koji url")
	}

	kojiCli, err := koji_client.NewAPIClient(logger, baseUrl, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create koji client for area geofences: %v", err)
	}

	fc, err := kojiCli.GetFeatureCollection(project)
	if err != nil {
		return nil, fmt.Errorf("failed to get area geofences from koji: %v", err)
	}
	return fc.Features, nil
}

func getImporter(cfg Config, logger *logrus.Logger) (importers.Importer, error) {
	nestsDBStore, err := db_store.NewNestsDBStore(cfg.NestsDB, logger, "")
	if err != nil {
		logger.Errorf("failed to init db for db importer: %v", err)
		os.Exit(1)
	}

	dbImporter, err := importers.NewDBImporter(logger, nestsDBStore)
	if err != nil {
		logger.Fatalf("failed to create db importer: %v", err)
	}
	return dbImporter, nil
}

func loadAreas(cfg Config, logger *logrus.Logger) ([]*geojson.Feature, error) {
	if filename := cfg.Areas.Filename; filename != "" {
		return geo.LoadFeaturesFromFile(filename)
	} else {
		return featuresFromKoji(logger, cfg.Areas.KojiUrl, cfg.Areas.KojiToken)
	}
}

func main() {
	flagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	helpFlag := flagSet.Bool("help", false, "help!")
	debugFlag := flagSet.Bool("debug", false, "override config and turn on debug logging")
	flagSet.BoolVar(helpFlag, "h", false, "help!")
	configFileFlag := flagSet.String("f", DEFAULT_CONFIG_FILENAME, "config file to use")
	allAreasFlag := flagSet.Bool("all-areas", false, "import all areas found in your areas source")

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

	cfg, err := LoadConfig(*configFileFlag)
	if err != nil {
		log.Fatal(err)
	}

	if *debugFlag {
		cfg.Logging.Debug = true
	}
	logger := cfg.CreateLogger(true)

	// check destination first before we attempt to load
	// area fences.
	importerImpl, err := getImporter(*cfg, logger)
	if err != nil {
		logger.Errorf("Error: couldn't create importer: %s", err)
		logger.Errorf("Try %s -help for help.", os.Args[0])
		os.Exit(1)
	}

	overpassAreas, err := loadAreas(*cfg, logger)
	if len(overpassAreas) == 0 {
		logger.Errorf("No areas were loaded/returned from source.")
		logger.Errorf("Try %s -help for help.", os.Args[0])
		os.Exit(1)
	}

	if *allAreasFlag {
		logger.Infof("%d area geofence(s) loaded from source", len(overpassAreas))
	} else {
		var overpassArea *geojson.Feature

		areaName := args[0]
		for _, feature := range overpassAreas {
			if name, _ := feature.Properties["name"].(string); name != "" && name == areaName {
				overpassArea = feature
				break
			}
		}

		if overpassArea == nil {
			logger.Fatalf("Area '%s' was not found in the source", areaName)
		}

		logger.Info("1 area geofence loaded from source")

		overpassAreas[0] = overpassArea
		overpassAreas = overpassAreas[:1]
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

		runner, err := importer.NewImportRunner(cfg.Importer, logger, importerImpl, exporterImpl)
		if err != nil {
			logger.Fatal(err)
		}

		err = runner.Import(ctx)
		if err != nil {
			logger.Fatal(err)
		}
	}

	logger.Infof("Done.")
}
