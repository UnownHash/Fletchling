package filters

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

const (
	DEFAULT_FILTER_CONCURRENCY      = 4
	DEFAULT_MIN_SPAWNPOINTS         = 10
	DEFAULT_MAX_SPAWNPOINT_AGE_DAYS = 7
	DEFAULT_MIN_AREA_M2             = float64(100)
	DEFAULT_MAX_AREA_M2             = float64(10000000)
	DEFAULT_MAX_OVERLAP_PERCENT     = float64(60)
)

type FiltersConfig struct {
	// how many threads to use for filtering.
	Concurrency int `koanf:"concurrency" json:"concurrency"`
	// how many spawnpoints required in the geofence in order to track.
	MinSpawnpoints int64 `koanf:"min_spawnpoints" json:"min_spawnpoints"`
	// when querying spawnpoints, ignore spawnpoints older than this may days.
	MaxSpawnpointAgeDays int `koanf:"max_spawnpoint_age_days" json:"max_spawnpoint_age_days"`
	// minimum area required in order to track.
	MinAreaM2 float64 `koanf:"min_area_m2" json:"min_area_m2"`
	// maximum area that cannot be exceeded in order to track.
	MaxAreaM2 float64 `koanf:"max_area_m2" json:"max_area_m2"`
	// nests are allowed to overlap larger nests by at most this percentage. 0 = no overlap allowed at all.
	MaxOverlapPercent float64 `koanf:"max_overlap_percent" json:"max_overlap_percent"`
}

func (cfg *FiltersConfig) Validate() error {
	if val := cfg.Concurrency; val < 1 {
		return fmt.Errorf("concurrency should probably be at least 1, not %d", val)
	}
	if val := cfg.MinSpawnpoints; val < 0 {
		return fmt.Errorf("min_spawnpoints should probably be at least 0, not %d", val)
	}
	if val := cfg.MaxSpawnpointAgeDays; val < 1 {
		return fmt.Errorf("max_spawnpoint_age_days should probably be at least 1, not %d", val)
	}
	if val := cfg.MinSpawnpoints; val < 1 {
		return fmt.Errorf("min_spawnpoints should probably be at least 1, not %d", val)
	}
	if val := cfg.MinAreaM2; val < 0 {
		return fmt.Errorf("min_area_m2 should probably be at least 0, not %0.3f", val)
	}
	if val := cfg.MaxAreaM2; val < 0 {
		return fmt.Errorf("max_area_m2 should probably be at least 0, not %0.3f", val)
	}
	if val := cfg.MaxOverlapPercent; val < 0 {
		return fmt.Errorf("max_overlap_percent should probably be at least 0, not %0.3f", val)
	}
	return nil
}

func (cfg *FiltersConfig) Log(logger *logrus.Logger, prefix string) {
	logger.Infof("%sconcurrency=%d, min_spawnpoints=%d, max_spawnpoint_age_days=%d, min_area_m2=%0.3f max_area_m2=%0.3f",
		prefix,
		cfg.Concurrency,
		cfg.MinSpawnpoints,
		cfg.MaxSpawnpointAgeDays,
		cfg.MinAreaM2,
		cfg.MaxAreaM2,
	)
}

func DefaultFiltersConfig() FiltersConfig {
	return FiltersConfig{
		Concurrency:          DEFAULT_FILTER_CONCURRENCY,
		MinSpawnpoints:       DEFAULT_MIN_SPAWNPOINTS,
		MaxSpawnpointAgeDays: DEFAULT_MAX_SPAWNPOINT_AGE_DAYS,
		MinAreaM2:            DEFAULT_MIN_AREA_M2,
		MaxAreaM2:            DEFAULT_MAX_AREA_M2,
		MaxOverlapPercent:    DEFAULT_MAX_OVERLAP_PERCENT,
	}
}
