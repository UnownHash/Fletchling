package geo

import (
	"fmt"
	"sync"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/planar"
	"github.com/tidwall/rtree"
)

type FenceRTreeEntry[V any] struct {
	feature      *geojson.Feature
	polygon      orb.Polygon
	multiPolygon orb.MultiPolygon
	value        V
	containsFn   func(orb.Point) bool
}

func (e FenceRTreeEntry[V]) polygonContains(p orb.Point) bool {
	return planar.PolygonContains(e.polygon, p)
}

func (e FenceRTreeEntry[V]) multiPolygonContains(p orb.Point) bool {
	return planar.MultiPolygonContains(e.multiPolygon, p)
}

func (e FenceRTreeEntry[V]) Contains(p orb.Point) bool {
	return e.containsFn(p)
}

type FenceRTree[V any] struct {
	mutex sync.RWMutex
	rtree rtree.RTreeG[FenceRTreeEntry[V]]
}

func (rt *FenceRTree[V]) insertEntry(bbox orb.Bound, entry FenceRTreeEntry[V]) {
	rt.mutex.Lock()
	defer rt.mutex.Unlock()
	rt.rtree.Insert(bbox.Min, bbox.Max, entry)
}

func (rt *FenceRTree[V]) insertPolygon(polygon orb.Polygon, value V) {
	entry := FenceRTreeEntry[V]{
		polygon: polygon,
		value:   value,
	}
	entry.containsFn = entry.polygonContains
	rt.insertEntry(polygon.Bound(), entry)
}

func (rt *FenceRTree[V]) insertMultiPolygon(multiPolygon orb.MultiPolygon, value V) {
	entry := FenceRTreeEntry[V]{
		multiPolygon: multiPolygon,
		value:        value,
	}
	entry.containsFn = entry.multiPolygonContains
	rt.insertEntry(multiPolygon.Bound(), entry)
}

func (rt *FenceRTree[V]) InsertGeometry(geometry orb.Geometry, value V) error {
	switch typ := geometry.GeoJSONType(); typ {
	case "Polygon":
		polygon, ok := geometry.(orb.Polygon)
		if !ok {
			return fmt.Errorf("GeoJSONType is %s but Geometry type is %T not orb.Polygon", typ, geometry)
		}
		rt.insertPolygon(polygon, value)
	case "MultiPolygon":
		multiPolygon, ok := geometry.(orb.MultiPolygon)
		if !ok {
			return fmt.Errorf("GeoJSONType is %s but Geometry type is %T not orb.MultiPolygon", typ, geometry)
		}
		rt.insertMultiPolygon(multiPolygon, value)
	default:
		return fmt.Errorf("GeoJSONType %s is not supported", typ)
	}
	return nil
}

func (rt *FenceRTree[V]) InsertFeature(feature *geojson.Feature, value V) error {
	return rt.InsertGeometry(feature.Geometry, value)
}

func (rt *FenceRTree[V]) GetMatches(lat, lon float64) []V {
	matches := make([]V, 0, 2)

	p := orb.Point{lon, lat}

	rt.mutex.RLock()
	defer rt.mutex.RUnlock()
	rt.rtree.Search(p, p, func(min, max [2]float64, entry FenceRTreeEntry[V]) bool {
		if entry.Contains(p) {
			matches = append(matches, entry.value)
		}
		return true
	})

	return matches
}

func NewFenceRTree[V any]() *FenceRTree[V] {
	return &FenceRTree[V]{}
}
