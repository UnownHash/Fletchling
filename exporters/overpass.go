package exporters

import (
	"context"
	"fmt"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"
	"github.com/paulmach/osm/osmgeojson"
	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/geo"
	"github.com/UnownHash/Fletchling/overpass"
)

var sportMappings = map[string]string{
	"american_football": "American Football Field",
	"baseball":          "Baseball Field",
	"basketball":        "Basketball Court",
	"beachvolleyball":   "Volleyball Court",
	"equestrian":        "Equestrian Area",
	"football":          "Football Field",
	"golf":              "Golf Course",
	"horseshoes":        "Horseshoes Area",
	"multi":             "Multipurpose Area",
	"skateboard":        "Skate Park",
	"soccer":            "Soccer Field",
	"softball":          "Softball Field",
	"tennis":            "Tennis Court",
	"volleyball":        "Volleyball Court",
}

var leisureMappings = map[string]string{
	"park":           "Park",
	"garden":         "Garden",
	"golf_course":    "Golf Course",
	"nature_reserve": "Nature Reserve",
	"playground":     "Playground",
}

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
		geometry := feature.Geometry
		featureCenter := geo.GetPolygonLabelPoint(geometry)

		overpass.AdjustFeatureProperties(feature)

		id, _ := feature.Properties["id"]
		if id == nil {
			exporter.logger.Debugf("skipping osm feature with no id")
			continue
		}

		var wasNoName bool

		name, _ := feature.Properties["name"].(string)
		if name == "" {
			var mapping string

			leisure, _ := feature.Properties["leisure"].(string)
			if leisure == "pitch" {
				sport, _ := feature.Properties["sport"].(string)
				mapping, _ = sportMappings[sport]
			} else {
				mapping, _ = leisureMappings[leisure]
			}

			// if we still don't have a name, leave it with no name
			if mapping != "" {
				name = "Unknown " + mapping
				feature.Properties[name] = name
				wasNoName = true
			}
		}

		if exporter.parentPolygon != nil {
			if !planar.PolygonContains(*exporter.parentPolygon, featureCenter) {
				exporter.logger.Debugf("skipping osm feature '%s': %0.5f,%0.5f not within area", name, featureCenter.Lat(), featureCenter.Lon())
				continue
			}
		} else if exporter.parentMultiPolygon != nil {
			if !planar.MultiPolygonContains(*exporter.parentMultiPolygon, featureCenter) {
				exporter.logger.Debugf("skipping osm feature '%s': %0.5f,%0.5f not within area", name, featureCenter.Lat(), featureCenter.Lon())
				continue
			}
		}

		if exporter.parentName == "" {
			delete(feature.Properties, "parent")
		} else {
			feature.Properties["parent"] = exporter.parentName
		}

		if wasNoName {
			exporter.logger.Infof("osm feature id '%v' at %0.5f,%0.5f had no name: using '%s'", id, featureCenter.Lat(), featureCenter.Lon(), name)
		}

		features[idx] = feature
		idx++
	}

	features = features[:idx]

	return features, nil
}

func NewOverpassExporter(logger *logrus.Logger, overpassCli *overpass.Client, area *geojson.Feature) (*OverpassExporter, error) {
	bound := area.Geometry.Bound()
	areaName, _ := area.Properties["name"].(string)
	parentName, _ := area.Properties["parent"].(string)
	if parentName != "" && areaName != "" {
		areaName = parentName + "/" + areaName
	}

	var polygonPtr *orb.Polygon
	var multiPolygonPtr *orb.MultiPolygon

	geometry := area.Geometry
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
	default:
		return nil, fmt.Errorf("area '%s' has unsupported geometry type '%s'", areaName, typ)
	}

	loader := &OverpassExporter{
		logger:             logger,
		overpassCli:        overpassCli,
		parentPolygon:      polygonPtr,
		parentMultiPolygon: multiPolygonPtr,
		bound:              bound,
		parentName:         areaName,
	}
	return loader, nil
}
