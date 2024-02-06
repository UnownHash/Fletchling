package processor

import (
	"context"
	"fmt"

	"github.com/paulmach/orb/geojson"
)

type Filter struct {
	MinSpawnpoints int64
	MinArea        float64
	MaxArea        float64
}

func (f Filter) FilterSpawnpoints(spawnpoints int64) error {
	if spawnpoints < f.MinSpawnpoints {
		return fmt.Errorf("spawnpoints %d < min_spawnpoints %d", spawnpoints, f.MinSpawnpoints)
	}
	return nil
}

func (f Filter) FilterArea(area float64) error {
	if area < f.MinArea {
		return fmt.Errorf("area %0.3f < min_area %0.3f", area, f.MinArea)
	}

	if f.MaxArea > 0 && area > f.MaxArea {
		return fmt.Errorf("area %0.3f > max_area %0.3f", area, f.MaxArea)
	}
	return nil
}

type SpawnpointGetter interface {
	GetContainedSpawnpoints(context.Context, *geojson.Geometry) ([]uint64, error)
}
