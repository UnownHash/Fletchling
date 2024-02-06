package exporters

import (
	"context"

	"github.com/UnownHash/Fletchling/db_store"
	"github.com/paulmach/orb/geojson"
	"github.com/sirupsen/logrus"
)

type DBExporter struct {
	logger       *logrus.Logger
	nestsDBStore *db_store.NestsDBStore
}

func (*DBExporter) ExporterName() string {
	return "koji"
}

func (exporter *DBExporter) ExportFeatures(ctx context.Context) ([]*geojson.Feature, error) {
	nests, err := exporter.nestsDBStore.GetAllNests(ctx)
	if err != nil {
		return nil, err
	}

	features := make([]*geojson.Feature, len(nests))
	idx := 0

	for _, nest := range nests {
		if nest.Polygon == nil {
			exporter.logger.Warnf("DBExporter: skipping nest '%s': invalid polygon", nest.Name)
			continue
		}

		jsonGeometry, err := nest.Geometry()
		if err != nil {
			exporter.logger.Warnf("DBExporter: skipping nest '%s': invalid geometry: %v", nest.Name, err)
			continue
		}
		geometry := jsonGeometry.Geometry()
		feature := geojson.NewFeature(geometry)
		feature.Properties["name"] = nest.Name
		feature.Properties["id"] = nest.NestId
		if areaName := nest.AreaName.ValueOrZero(); areaName != "" {
			feature.Properties["parent"] = areaName
		}

		features[idx] = feature
		idx++
	}

	features = features[:idx]

	return features, nil
}

func NewDBExporter(logger *logrus.Logger, nestsDBStore *db_store.NestsDBStore) (*DBExporter, error) {
	exporter := &DBExporter{
		logger:       logger,
		nestsDBStore: nestsDBStore,
	}
	return exporter, nil
}
