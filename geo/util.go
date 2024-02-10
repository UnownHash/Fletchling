package geo

import (
	"fmt"
	"strconv"

	venise_geo "github.com/dernise/venise/geo"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geo"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"
	"gopkg.in/guregu/null.v4"
)

func idIsValid(id any) (int64, error) {
	var nestId int64

	switch v := id.(type) {
	case string:
		var err error
		nestId, err = strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("id '%s' can't be parsed as int", v)
		}
	case int:
		nestId = int64(v)
	case uint:
		nestId = int64(v)
	case int64:
		nestId = v
	case uint64:
		nestId = int64(v)
	default:
		return 0, fmt.Errorf("id '%v' type '%T' not supported", v, v)
	}

	return nestId, nil
}

func NameAndIntIdFromFeature(feature *geojson.Feature) (string, null.String, int64, error) {
	var areaName null.String

	props := feature.Properties

	name, ok := props["name"].(string)
	if !ok {
		return "<unknown>", areaName, 0, fmt.Errorf("feature has no name")
	}

	fullName := name

	if parent, _ := props["parent"].(string); parent != "" {
		areaName = null.StringFrom(parent)
		fullName = parent + "/" + name
	}

	id, ok := props["id"]
	if !ok {
		return name, areaName, 0, fmt.Errorf("feature '%s' has no id", fullName)
	}

	nestId, err := idIsValid(id)
	if err != nil {
		return name, areaName, 0, fmt.Errorf("feature '%s': %s", fullName, err)
	}

	return name, areaName, nestId, nil
}

func convertToVenisePolygon(orbPolygon orb.Polygon) venise_geo.Polygon {
	polygon := venise_geo.Polygon{
		Rings: make([][]venise_geo.Point, len(orbPolygon)),
	}
	for ringIdx, ring := range orbPolygon {
		ringPoints := make([]venise_geo.Point, len(ring))
		for ptsIdx, coord := range ring {
			ringPoints[ptsIdx] = venise_geo.Point(coord)
		}
		polygon.Rings[ringIdx] = ringPoints
	}
	return polygon
}

func GetPolygonLabelPoint(geometry orb.Geometry) orb.Point {
	center, _ := planar.CentroidArea(geometry)
	switch typedGeometry := geometry.(type) {
	case orb.Polygon:
		if !planar.PolygonContains(typedGeometry, center) {
			point := venise_geo.Polylabel(convertToVenisePolygon(typedGeometry), 0.000001, false)
			return orb.Point(point)
		}
	case orb.MultiPolygon:
		if !planar.MultiPolygonContains(typedGeometry, center) {
			if len(typedGeometry) < 1 {
				break
			}

			bestPoly := typedGeometry[0]
			maxArea := geo.Area(bestPoly)

			for _, poly := range typedGeometry[1:] {
				area := geo.Area(poly)
				if area > maxArea {
					maxArea = area
					bestPoly = poly
				}
			}

			point := venise_geo.Polylabel(convertToVenisePolygon(bestPoly), 0.000001, false)
			return orb.Point(point)
		}
	}
	return center
}
