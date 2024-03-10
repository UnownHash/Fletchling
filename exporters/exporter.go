package exporters

import (
	"context"

	"github.com/paulmach/orb/geojson"
)

type Exporter interface {
	ExporterName() string
	ExportFeatures(context.Context) ([]*geojson.Feature, error)
}
