package exporters

import (
	"context"

	"github.com/paulmach/orb/geojson"
)

type MultiExporter []Exporter

func (MultiExporter) ExporterName() string {
	return "multi"
}

func (mExporter *MultiExporter) Append(exporter Exporter) {
	*mExporter = append(*mExporter, exporter)
}

func (mExporter MultiExporter) ExportFeatures(ctx context.Context) ([]*geojson.Feature, error) {
	allFeatures := make([]*geojson.Feature, 0)

	for _, exporter := range mExporter {
		features, err := exporter.ExportFeatures(ctx)
		if err != nil {
			return nil, err
		}
		allFeatures = append(allFeatures, features...)
	}
	return allFeatures, nil
}
