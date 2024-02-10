package util

import (
	venise_geo "github.com/dernise/venise/geo"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/planar"
)

func convertToPolygon(orbPolygon orb.Polygon) venise_geo.Polygon {
	var polygon venise_geo.Polygon
	for _, ring := range orbPolygon {
		var ringPoints []venise_geo.Point
		for _, coord := range ring {
			ringPoints = append(ringPoints, venise_geo.Point{coord[0], coord[1]})
		}
		polygon.Rings = append(polygon.Rings, ringPoints)
	}
	return polygon
}

func GetPolygonLabelPoint(feature orb.Geometry) orb.Point {
	center, _ := planar.CentroidArea(feature)
	orbPolygon, ok := feature.(orb.Polygon)
	if ok {
		if !planar.PolygonContains(orbPolygon, center) {
			point := venise_geo.Polylabel(convertToPolygon(orbPolygon), 0.000001, false)
			return orb.Point(point)
		}
	}
	return center
}
