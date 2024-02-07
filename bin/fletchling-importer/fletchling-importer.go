package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/paulmach/orb/geojson"

	"github.com/UnownHash/Fletchling/db_store"
	np_geo "github.com/UnownHash/Fletchling/geo"
	"github.com/UnownHash/Fletchling/importer"
	"github.com/UnownHash/Fletchling/importer/exporters"
	"github.com/UnownHash/Fletchling/importer/importers"
	"github.com/UnownHash/Fletchling/koji_client"
	"github.com/UnownHash/Fletchling/overpass"
)

const (
	DEFAULT_CONFIG_FILENAME = "configs/fletchling-importer.toml"
)

func main() {
	flagSet := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	helpOpt := flagSet.Bool("help", false, "help!")
	help2Opt := flagSet.Bool("h", false, "help!")
	configFlag := flagSet.String("config", DEFAULT_CONFIG_FILENAME, fmt.Sprintf("config filename to use (default: %s)", DEFAULT_CONFIG_FILENAME))
	kojiExporterProjectFlag := flagSet.String("koji-src-project", "", "project name when loading areas from koji")
	kojiImporterProjectFlag := flagSet.String("koji-dest-project", "", "project name when saving nests to koji")
	kojiCreatePropsFlag := flagSet.Bool("koji-create-properties", false, "create missing properties in koji when saving nests to koji")
	overpassKojiProjectFlag := flagSet.String("overpass-koji-project", "", "project name when --overpass-src is 'koji'")
	overpassAreasSrcFlag := flagSet.String("overpass-areas-src", "", "where to get areas to search ('koji', or filename)")
	overpassAreaFlag := flagSet.String("overpass-area", "", "the area for which to find nests from --overpass-src")

	flagSet.Parse(os.Args[1:])

	if *helpOpt || *help2Opt {
		fmt.Printf("Usage: %s [options] <src> <dest>\n", os.Args[0])
		fmt.Printf("\n<src> -- one of: overpass,koji,db\n")
		fmt.Printf("\n<dest> -- one of: koji,db\n\n")
		fmt.Printf("Options:\n")
		flagSet.SetOutput(os.Stdout)
		flagSet.PrintDefaults()
		os.Exit(0)
	}

	args := flagSet.Args()

	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <configfile> <src> <dest>\n", os.Args[0])
		os.Exit(1)
	}

	ctx := context.Background()

	configFilename := *configFlag
	exporterName := args[0]
	importerName := args[1]

	cfg, err := LoadConfig(configFilename)
	if err != nil {
		log.Fatal(err)
	}

	logger := cfg.CreateLogger()

	var areaName string
	var exporterImpl exporters.Exporter
	var importerImpl importers.Importer

	switch exporterName {
	case "overpass":
		if cfg.OverpassExporter == nil {
			logger.Fatal("'overpass' selected as source, but no 'overpass_exporter' section in config")
		}
		if *overpassAreaFlag == "" {
			logger.Fatal("'overpass' selected as source, but no --overpass-area given")
		}

		var overpassAreas []*geojson.Feature

		switch *overpassAreasSrcFlag {
		case "koji":
			if cfg.KojiOverpassSrc == nil {
				logger.Fatal("'overpass' selected as source, but no 'koji_overpass_source' section in config")
			}

			if *overpassKojiProjectFlag == "" {
				logger.Fatal("'overpass' selected as source, but no --overpass-koji-project' given")
			}

			kojiCli, err := koji_client.NewAPIClient(logger, cfg.KojiOverpassSrc.Url, cfg.KojiOverpassSrc.Token)
			if err != nil {
				logger.Fatalf("failed to create koji client for overpass exporter: %v", err)
			}

			fc, err := kojiCli.GetFeatureCollection(*overpassKojiProjectFlag)
			if err != nil {
				logger.Fatalf("failed to get areas from koji for overpass exporter: %v", err)
			}
			if l := len(fc.Features); l == 0 {
				logger.Fatal("no geofence areas found in koji to use for overpass")
			} else {
				logger.Infof("loaded %d area(s) from koji", l)
			}
			overpassAreas = fc.Features
		case "":
			logger.Fatalf("'overpass' selected as source, but no --overpass-src given.")
		default:
			features, err := np_geo.LoadGeofencesFile(*overpassAreasSrcFlag)
			if err != nil {
				logger.Fatalf("--overpass-src should be 'koji' or a filename: %v", err)
			}
			if l := len(features); l == 0 {
				logger.Fatalf("no geofence areas found in file %s", *overpassAreasSrcFlag)
			} else {
				logger.Infof("loaded %d area(s) from %s", l, *overpassAreasSrcFlag)
			}
			overpassAreas = features
		}

		var overpassArea *geojson.Feature

		for _, feature := range overpassAreas {
			if name, _ := feature.Properties["name"].(string); name != "" && name == *overpassAreaFlag {
				overpassArea = feature
				break
			}
		}

		if overpassArea == nil {
			logger.Fatalf("area '%s' not found in loaded areas for overpass exporter", *overpassAreaFlag)
		}

		overpassCli, err := overpass.NewClient(logger, cfg.OverpassExporter.Url)
		if err != nil {
			logger.Fatalf("failed to create overpass client for overpass exporter: %v", err)
		}

		exporter, err := exporters.NewOverpassExporter(logger, overpassCli, overpassArea)
		if err != nil {
			logger.Fatalf("failed to create overpass exporter: %v", err)
		}

		exporterImpl = exporter
		areaName = *overpassAreaFlag

		// fallthrough and check the rest after importer is verified
	case "koji":
		if cfg.KojiExporter == nil {
			logger.Fatal("'koji' selected as source, but no 'koji_exporter' section in config")
		}

		if *kojiExporterProjectFlag == "" {
			logger.Fatal("'koji' selected as source, but no --koji-src-project given")
		}

		projectName := *kojiExporterProjectFlag

		kojiCli, err := koji_client.NewClient(logger, cfg.KojiExporter.Url, cfg.KojiExporter.Token)
		if err != nil {
			logger.Fatalf("failed to create koji client for koji exporter: %v", err)
		}

		_, err = kojiCli.GetProjectByName(projectName)
		if err != nil {
			logger.Fatalf("failed to get koji-src-project '%s': %v", projectName, err)
		}

		kojiExporter, err := exporters.NewKojiExporter(logger, kojiCli.APIClient, projectName)
		if err != nil {
			logger.Fatalf("failed to create koji exporter: %v", err)
		}

		exporterImpl = kojiExporter
	case "db":
		if cfg.DBExporter == nil {
			logger.Fatal("'db' selected as source, but no 'db_exporter' section in config")
		}

		nestsDBStore, err := db_store.NewNestsDBStore(*cfg.DBExporter, logger, "")
		if err != nil {
			logger.Fatalf("failed to init db for db exporter: %v", err)
		}

		dbExporter, err := exporters.NewDBExporter(logger, nestsDBStore)
		if err != nil {
			logger.Fatalf("failed to create db exporter: %v", err)
		}

		exporterImpl = dbExporter
	default:
		logger.Fatalf("Unknown source '%s'", exporterName)
	}

	switch importerName {
	case "koji":
		if cfg.KojiImporter == nil {
			logger.Fatal("'koji' selected as destination, but no 'koji_importer' section in config")
		}

		if *kojiImporterProjectFlag == "" {
			logger.Fatal("'koji' selected as destination, but no --koji-dest-project given")
		}

		if exporterName == "koji" {
			if cfg.KojiExporter.Url == cfg.KojiImporter.Url && *kojiExporterProjectFlag == *kojiImporterProjectFlag {
				logger.Fatal("'koji' as both source and destination requires different urls or different projects, of course, unless you like an expensive no-op.")
			}
		}

		projectName := *kojiImporterProjectFlag

		kojiCli, err := koji_client.NewAdminClient(logger, cfg.KojiImporter.Url, cfg.KojiImporter.Token)
		if err != nil {
			logger.Fatalf("failed to create koji client for koji importer: %v", err)
		}

		if areaName != "" {
			geofences, err := kojiCli.GetAllGeofences()
			if err != nil {
				logger.Fatalf("failed to get all koji geofences for koji importer to verify parents: %v", err)
			}
			existingNames := make(map[string]struct{})
			for _, geofence := range geofences {
				existingNames[geofence.Name] = struct{}{}
			}
			if _, ok := existingNames[areaName]; !ok {
				logger.Fatalf("area to be used as parent in koji does not exist in koji: %s", areaName)
			}
		}

		_, err = kojiCli.GetProjectByName(*kojiImporterProjectFlag)
		if err != nil {
			logger.Fatalf("failed to get koji-dest-project '%s': %v", *kojiImporterProjectFlag, err)
		}

		kojiImporter, err := importers.NewKojiImporter(logger, kojiCli, projectName, *kojiCreatePropsFlag)
		if err != nil {
			logger.Fatalf("failed to create koji importer: %v", err)
		}

		importerImpl = kojiImporter
	case "db":
		if cfg.DBImporter == nil {
			logger.Fatal("'db' selected as destination, but no 'db_importer' section in config")
		}

		if exporterName == "db" {
			if cfg.DBExporter.Addr == cfg.DBImporter.Addr && cfg.DBExporter.Db == cfg.DBImporter.Db {
				logger.Fatal("'db' as both source and destination requires different addrs or different dbs, of course, unless you like an expensive no-op.")
			}
		}
		nestsDBStore, err := db_store.NewNestsDBStore(*cfg.DBImporter, logger, "")
		if err != nil {
			logger.Fatalf("failed to init db for db importer: %v", err)
		}

		dbImporter, err := importers.NewDBImporter(logger, nestsDBStore)
		if err != nil {
			logger.Fatalf("failed to create db importer: %v", err)
		}

		importerImpl = dbImporter
	default:
		logger.Fatalf("Unknown destination '%s'", importerName)
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancelFn := context.WithCancel(ctx)
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

	logger.Infof("running...")

	runner, err := importer.NewImportRunner(cfg.Importer, logger, importerImpl, exporterImpl)
	if err != nil {
		logger.Fatal(err)
	}

	err = runner.Import(ctx)
	if err != nil {
		logger.Fatal(err)
	}

	logger.Infof("Done.")
}
