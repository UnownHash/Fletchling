package filters

import "fmt"

type Config struct {
	// how many threads to use for filtering.
	Concurrency int `koanf:"concurrency" json:"concurrency"`
	// how many spawnpoints required in the geofence in order to track.
	MinSpawnpoints int64 `koanf:"min_spawnpoints" json:"min_spawnpoints"`
	// minimum area required in order to track.
	MinAreaM2 float64 `koanf:"min_area_m2" json:"min_area_m2"`
	// maximum area that cannot be exceeded in order to track.
	MaxAreaM2 float64 `koanf:"max_area_m2" json:"max_area_m2"`
	// nests are allowed to overlap larger nests by at most this percentage. 0 = no overlap allowed at all.
	MaxOverlapPercent float64 `koanf:"max_overlap_percent" json:"max_overlap_percent"`
}

func (cfg *Config) Validate() error {
	if val := cfg.Concurrency; val < 1 {
		return fmt.Errorf("concurrency should probably be at least 1, not %d", val)
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
