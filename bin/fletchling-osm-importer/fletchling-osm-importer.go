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
	"github.com/UnownHash/Fletchling/logging"
	"github.com/UnownHash/Fletchling/overpass"
)

const (
	DEFAULT_OVERPASS_URL   = "https://overpass-api.de/api/interpreter"
	DEFAULT_MIGRATION_PATH = "./db_store/sql"
	DEFAULT_MIN_AREA       = 100.0
	DEFAULT_MAX_AREA       = 10000000.0
)

func usage(flagSet *flag.FlagSet, output io.Writer) {
	fmt.Fprintf(output, "Usage: %s [options] <areas-source> <destination-uri> [<area-name>]\n", os.Args[0])
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "<areas-source>:\n")
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "This is where to get your area geofences. This can be a Koji url or a filename.\n")
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "For koji, this should be a url of this form:\n")
	fmt.Fprint(output, "  http://:KOJI-SECRET@127.0.0.1:8080/api/v1/geofence/feature-collection/PROJECT-NAME\n")
	fmt.Fprint(output, "You can simplify it if you don't have a secret:\n")
	fmt.Fprint(output, "  http://127.0.0.1:8080/api/v1/geofence/feature-collection/PROJECT-NAME\n")
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "If you specify a filename, it can be of the poracle-style geofences.json or it can be a geojson FeatureCollection.\n")
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "<destination-uri>:\n")
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "This is where overpass results will be imported in the form of nests. This should be a uri.\n")
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "If you are going to import into a database: db://USERNAME:PASSWORD@HOSTNAME/DATABASE\n")
	fmt.Fprint(output, "If you are going to import into Koji, see what a Koji url looks like above\n")
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "<area-name>:\n")
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "This argument is required if you do not use the '-all-areas' option.\n")
	fmt.Fprint(output, "This is used to import a single area only.\n")
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "Options:\n")
	flagSet.SetOutput(output)
	flagSet.PrintDefaults()
	fmt.Fprint(output, "\nExamples:\n")
	fmt.Fprint(output, "Import from overpass using all areas from MyAreas project in koji into golbat DB:\n")
	fmt.Fprintf(output, "%s -all-areas 'http://:bearertoken@127.0.0.1:8080/api/v1/geofence/feature-collection/MyAreas' 'db://user:password@dbhost/golbat'\n", os.Args[0])
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "Import from overpass using 'my area name' geofence from MyAreas project in koji into golbat DB:\n")
	fmt.Fprintf(output, "%s 'http://:bearertoken@127.0.0.1:8080/api/v1/geofence/feature-collection/MyAreas' 'db://user:password@dbhost/golbat' 'my area name'\n", os.Args[0])
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "Import from overpass using all areas from MyAreas project in koji into koji MyNests project:\n")
	fmt.Fprint(output, "(you can import into different koji instance, even)\n")
	fmt.Fprintf(output, "%s -all-areas 'http://:bearertoken@kojihost1:8080/api/v1/geofence/feature-collection/MyAreas' 'http://:bearertoken@kojihost2:8080/api/v1/geofence/feature-collection/MyNests'\n", os.Args[0])
	fmt.Fprint(output, "\n")
	fmt.Fprint(output, "Import from overpass using all areas from file '/path/to/geofences.json' into golbat DB:\n")
	fmt.Fprintf(output, "%s -all-areas 'path/to/geofences.json' 'db://user:password@dbhost/golbat'\n", os.Args[0])
	fmt.Fprint(output, "\n")
}

func featuresFromKoji(logger *logrus.Logger, uri url.URL) ([]*geojson.Feature, error) {
	var token string

	if ui := uri.User; ui != nil {
		token, _ = ui.Password()
	}
	uri.User = nil

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

func getImporter(logger *logrus.Logger, flagSet *flag.FlagSet, uriStr string, initDb bool) (importers.Importer, error) {
	destUri, err := url.Parse(uriStr)
	if err != nil {
		return nil, err
	}

	if destUri.Scheme == "db" {
		var dbConfig db_store.DBConfig

		if err := dbConfig.SetFromUri(destUri); err != nil {
			logger.Errorf("bad database uri?: %s", err)
			os.Exit(1)
		}

		var migrationPath string
		if initDb {
			migrationPath = DEFAULT_MIGRATION_PATH
		}

		nestsDBStore, err := db_store.NewNestsDBStore(dbConfig, logger, migrationPath)
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

	return nil, errors.New("unsupported uri")
}

func main() {
	flagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	helpFlag := flagSet.Bool("help", false, "help!")
	flagSet.BoolVar(helpFlag, "h", false, "help!")
	overpassUrlFlag := flagSet.String("overpass-url", DEFAULT_OVERPASS_URL, "the overpass api url to use")
	minAreaFlag := flagSet.Float64("min-area", DEFAULT_MIN_AREA, "don't import nest if it's not at least this size in meters squared")
	maxAreaFlag := flagSet.Float64("max-area", DEFAULT_MAX_AREA, "don't import nest if at least this size in meters squared")
	allAreasFlag := flagSet.Bool("all-areas", false, "import all areas found in your areas source")
	debugFlag := flagSet.Bool("debug", false, "turn on debug logging")
	initDbFlag := flagSet.Bool("init-db", false, "use this to run db migrations and create nests table, if needed (requires the migrations be at ./db_store/sql).")

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

	logFormatter := &logging.PlainFormatter{
		TimestampFormat: "2006-01-02 15:04:05",
		LevelDesc:       []string{"PANC", "FATL", "ERRO", "WARN", "INFO", "DEBG"},
	}

	logger := logrus.New()
	logger.SetFormatter(logFormatter)
	if *debugFlag {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}
	log.SetOutput(logger.Writer())

	args := flagSet.Args()
	l := len(args)

	if (*allAreasFlag && l != 2) || (!*allAreasFlag && l != 3) {
		if *allAreasFlag && l == 3 {
			logger.Errorf("do not specify an area if '-all-areas' is given")
			logger.Fatalf("Try %s -help for help.", os.Args[0])
		} else if !*allAreasFlag && l == 2 {
			logger.Errorf("if you do not use '-all-areas', specify an area name")
			logger.Fatalf("Try %s -help for help.", os.Args[0])
		}
		usage(flagSet, os.Stdout)
		os.Exit(1)
	}

	cfg := importer.Config{
		MinAreaM2: *minAreaFlag,
		MaxAreaM2: *maxAreaFlag,
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

	var overpassAreas []*geojson.Feature

	// check destination first before we attempt to load
	// area fences.
	importerImpl, err := getImporter(logger, flagSet, args[1], *initDbFlag)
	if err != nil {
		logger.Error(err)
		logger.Errorf("Try %s -help for help.", os.Args[0])
		os.Exit(1)
	}

	// try source areas as file first.
	overpassAreas, fileLoadErr := geo.LoadFeaturesFromFile(args[0])
	if fileLoadErr != nil {
		sourceUri, err := url.Parse(args[0])
		if err != nil {
			logger.Errorf("source failed to load as a file and doesn't look like a uri: %s", fileLoadErr)
			logger.Errorf("Try %s -help for help.", os.Args[0])
			os.Exit(1)
		}
		if sourceUri.Scheme == "file" {
			overpassAreas, err = geo.LoadFeaturesFromFile(sourceUri.Path)
			if err != nil {
				logger.Error(err)
				logger.Errorf("Try %s -help for help.", os.Args[0])
				os.Exit(1)
			}
		}
		overpassAreas, err = featuresFromKoji(logger, *sourceUri)
		if err != nil {
			logger.Errorf("source failed to load as a file: %s", fileLoadErr)
			logger.Errorf("source also looks like uri but failed to get area geofences from it: %s", err)
			logger.Errorf("Try %s -help for help.", os.Args[0])
			os.Exit(1)
		}
	}

	if len(overpassAreas) == 0 {
		logger.Fatal("The source does not seem to contain any geofences for areas")
	}

	if *allAreasFlag {
		logger.Infof("%d area geofence(s) loaded from source", len(overpassAreas))
	} else {
		var overpassArea *geojson.Feature
		areaName := args[2]

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

	for _, feature := range overpassAreas {
		areaName, _ := feature.Properties["name"].(string)

		overpassCli, err := overpass.NewClient(logger, *overpassUrlFlag)
		if err != nil {
			logger.Fatalf("failed to create overpass client for area %s: %v", areaName, err)
		}

		exporterImpl, err := exporters.NewOverpassExporter(logger, overpassCli, feature)
		if err != nil {
			logger.Fatalf("failed to create overpass exporter for area %s: %v", areaName, err)
		}

		logger.Infof("Importing area %s...", areaName)

		runner, err := importer.NewImportRunner(cfg, logger, importerImpl, exporterImpl)
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
