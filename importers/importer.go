package importers

import (
	"context"

	"github.com/paulmach/orb/geojson"
)

type Importer interface {
	ImporterName() string
	ImportFeatures(context.Context, []*geojson.Feature) error
}
