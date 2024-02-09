package importer

import (
	"context"
	"fmt"

	"github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"
	"github.com/sirupsen/logrus"

	np_geo "github.com/UnownHash/Fletchling/geo"
	"github.com/UnownHash/Fletchling/importer/exporters"
	"github.com/UnownHash/Fletchling/importer/importers"
)

type ImportRunner struct {
	logger *logrus.Logger
	config Config

	importer importers.Importer
	exporter exporters.Exporter
	areaName string
}

func (runner *ImportRunner) Import(ctx context.Context) error {
	baseFeatures, err := runner.exporter.ExportFeatures(ctx)
	if err != nil {
		return fmt.Errorf("failed to get features from exporter: %w", err)
	}

	features := make([]*geojson.Feature, len(baseFeatures))
	idx := 0

	config := runner.config

	for _, feature := range baseFeatures {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if name, _ := feature.Properties["name"].(string); name == "" {
			if config.DefaultName == "" {
				runner.logger.Warnf("ImportRunner: skipping feature with no name and no default name configured")
				continue
			}
			name = config.DefaultName
			if config.DefaultNameLocation {
				centroid, _ := planar.CentroidArea(feature.Geometry)
				name += fmt.Sprintf(" at %0.5f,%0.5f", centroid.Lat(), centroid.Lon())
			}
			feature.Properties["name"] = name
		}

		name, areaName, _, err := np_geo.NameAndIntIdFromFeature(feature)
		if err != nil {
			// exporters should deal with some of this, so only logging debug.
			runner.logger.Debugf("ImportRunner: skipping feature: %v", err)
			continue
		}

		fullName := name
		if areaName.Valid {
			fullName = areaName.String + "/" + name
		}

		geometry := feature.Geometry
		area := geo.Area(geometry)

		if area < config.MinAreaM2 {
			runner.logger.Warnf(
				"ImportRunner: skipping feature '%s': area too small (%0.3f < %0.3f)",
				fullName,
				area,
				config.MinAreaM2,
			)
			continue
		}

		if config.MaxAreaM2 > 0 && area > config.MaxAreaM2 {
			runner.logger.Warnf(
				"ImportRunner: skipping feature '%s': area too large (%0.3f > %0.3f)",
				fullName,
				area,
				config.MaxAreaM2,
			)
			continue
		}

		features[idx] = feature
		idx++
	}

	features = features[:idx]

	return runner.importer.ImportFeatures(ctx, features)
}

func NewImportRunner(config Config, logger *logrus.Logger, importer importers.Importer, exporter exporters.Exporter) (*ImportRunner, error) {
	runner := &ImportRunner{
		logger:   logger,
		config:   config,
		importer: importer,
		exporter: exporter,
	}
	return runner, nil
}
