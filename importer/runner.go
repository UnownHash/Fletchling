package importer

import (
	"context"
	"fmt"

	"github.com/UnownHash/Fletchling/geo"
	orb_geo "github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"

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

	fullNameFn := func(name string, areaName null.String) string {
		if areaName.Valid {
			return areaName.String + "/" + name
		}
		return name
	}

	var rtree *geo.FenceRTree[*geojson.Feature]

	if !config.AllowContained {
		rtree = geo.NewFenceRTree[*geojson.Feature]()
	}

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
				centroid := geo.GetPolygonLabelPoint(feature.Geometry)

				name += fmt.Sprintf(" at %0.5f,%0.5f", centroid.Lat(), centroid.Lon())
			}
			feature.Properties["name"] = name
		}

		name, areaName, _, err := geo.NameAndIntIdFromFeature(feature)
		if err != nil {
			// exporters should deal with some of this, so only logging debug.
			runner.logger.Debugf("ImportRunner: skipping feature: %v", err)
			continue
		}

		fullName := fullNameFn(name, areaName)
		geometry := feature.Geometry
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

		if rtree != nil {
			rtree.InsertGeometry(feature.Geometry, feature)
		}

		features[idx] = feature
		idx++
	}

	features = features[:idx]

	if rtree != nil {
		nonOverlappingFeatures := make([]*geojson.Feature, len(features))
		idx = 0
		for _, feature := range features {
			var skip bool

			center := geo.GetPolygonLabelPoint(feature.Geometry)
			matches := rtree.GetMatches(center.Lat(), center.Lon())
			for _, match := range matches {
				if match == feature {
					// ourself.
					continue
				}

				name, areaName, _, _ := geo.NameAndIntIdFromFeature(feature)
				fullName := fullNameFn(name, areaName)

				matchName, matchAreaName, _, _ := geo.NameAndIntIdFromFeature(match)
				matchFullName := fullNameFn(matchName, matchAreaName)

				runner.logger.Warnf(
					"ImportRunner: skipping feature '%s': center at %0.5f,%0.5f appears contained by feature '%s'",
					fullName,
					center.Lat(),
					center.Lon(),
					matchFullName,
				)
				skip = true
				break
			}
			if skip {
				continue
			}

			nonOverlappingFeatures[idx] = feature
			idx++
		}
		features = nonOverlappingFeatures[:idx]
	}

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
