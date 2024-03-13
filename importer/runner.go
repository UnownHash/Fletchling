package importer

import (
	"context"
	"fmt"

	orb_geo "github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/exporters"
	"github.com/UnownHash/Fletchling/geo"
	"github.com/UnownHash/Fletchling/importers"
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
		if err := ctx.Err(); err != nil {
			return err
		}

		if feature.Geometry == nil || feature.Properties == nil {
			runner.logger.Warnf("ImportRunner: skipping feature with no geometry or properties")
			continue
		}

		if name, _ := feature.Properties["name"].(string); name == "" {
			if config.DefaultName == "" {
				runner.logger.Warnf("ImportRunner: skipping feature with no name and no default name configured")
				continue
			}
			name = config.DefaultName
			if config.DefaultNameLocation {
				labelPoint := geo.GetPolygonLabelPoint(feature.Geometry)
				name += fmt.Sprintf(" at %0.5f,%0.5f", labelPoint.Lat(), labelPoint.Lon())
			}
			feature.Properties["name"] = name
		}

		name, areaName, _, err := geo.NameAndIntIdFromFeature(feature)
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

		switch geometryType := geometry.GeoJSONType(); geometryType {
		case "Polygon":
		case "MultiPolygon":
		default:
			runner.logger.Warnf("ImportRunner: skipping feature '%s': unsupported shape: %s",
				fullName,
				geometryType,
			)
			continue
		}

		area := orb_geo.Area(geometry)

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

func NewImportRunner(logger *logrus.Logger, config Config, importer importers.Importer, exporter exporters.Exporter) (*ImportRunner, error) {
	runner := &ImportRunner{
		logger:   logger,
		config:   config,
		importer: importer,
		exporter: exporter,
	}
	return runner, nil
}
