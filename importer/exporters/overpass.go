package exporters

import (
	"context"
	"fmt"

	"github.com/UnownHash/Fletchling/overpass"
	"github.com/sirupsen/logrus"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"
	"github.com/paulmach/osm/osmgeojson"
)

type OverpassExporter struct {
	logger             *logrus.Logger
	overpassCli        *overpass.Client
	bound              orb.Bound
	parentPolygon      *orb.Polygon
	parentMultiPolygon *orb.MultiPolygon
	parentName         string
}

func (*OverpassExporter) ExporterName() string {
	return "overpass"
}

func (exporter *OverpassExporter) ExportFeatures(ctx context.Context) ([]*geojson.Feature, error) {
	osm_data, err := exporter.overpassCli.GetPossibleNestLocations(ctx, exporter.bound)
	if err != nil {
		return nil, fmt.Errorf("failed to query overpass: %w", err)
	}

	fc, err := osmgeojson.Convert(osm_data)
	if err != nil {
		return nil, fmt.Errorf("error converting osm to geojson: %w", err)
	}

	if len(fc.Features) == 0 {
		return nil, nil
	}

	features := make([]*geojson.Feature, len(fc.Features))
	idx := 0

	for _, feature := range fc.Features {
		overpass.AdjustFeatureProperties(feature)
		name, _ := feature.Properties["name"].(string)
		if name == "" {
			exporter.logger.Warnf("skipping osm feature with no name")
			continue
		}

		if exporter.parentPolygon != nil {
			geometry := feature.Geometry
			center, _ := planar.CentroidArea(geometry)
			if !planar.PolygonContains(*exporter.parentPolygon, center) {
				exporter.logger.Warnf("skipping osm feature '%s': result not within area", name)
				continue
			}
		} else if exporter.parentMultiPolygon != nil {
			geometry := feature.Geometry
			center, _ := planar.CentroidArea(geometry)
			if !planar.MultiPolygonContains(*exporter.parentMultiPolygon, center) {
				exporter.logger.Warnf("skipping osm feature '%s': result not within area", name)
				continue
			}
		}

		if exporter.parentName != "" {
			feature.Properties["parent"] = exporter.parentName
		}

		features[idx] = feature
		idx++
	}

	features = features[:idx]

	return features, nil
}

func NewOverpassExporter(logger *logrus.Logger, overpassCli *overpass.Client, feature *geojson.Feature) (*OverpassExporter, error) {
	bound := feature.Geometry.Bound()
	parentName, _ := feature.Properties["name"].(string)

	var polygonPtr *orb.Polygon
	var multiPolygonPtr *orb.MultiPolygon

	geometry := feature.Geometry
	switch typ := geometry.GeoJSONType(); typ {
	case "Polygon":
		polygon, ok := geometry.(orb.Polygon)
		if ok {
			polygonPtr = &polygon
		}
	case "MultiPolygon":
		multiPolygon, ok := geometry.(orb.MultiPolygon)
		if ok {
			multiPolygonPtr = &multiPolygon
		}
	}

	loader := &OverpassExporter{
		logger:             logger,
		overpassCli:        overpassCli,
		parentPolygon:      polygonPtr,
		parentMultiPolygon: multiPolygonPtr,
		bound:              bound,
		parentName:         parentName,
	}
	return loader, nil
}
