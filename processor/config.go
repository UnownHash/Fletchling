package processor

import (
	"bytes"
	"fmt"
	"time"
)

const (
	DEFAULT_ROTATION_INTERVAL_MINUTES        = 15
	DEFAULT_MIN_HISTORY_DURATION_HOURS       = 1
	DEFAULT_MAX_HISTORY_DURATION_HOURS       = 12
	DEFAULT_MIN_NEST_POKEMON                 = 4
	DEFAULT_MIN_NEST_POKEMON_PCT             = float64(12)
	DEFAULT_MIN_TOTAL_POKEMON                = 12
	DEFAULT_MAX_GLOBAL_SPAWN_PCT             = 15
	DEFAULT_MIN_NEST_PCT_TO_GLOBAL_PCT_RATIO = float64(8)
	DEFAULT_SKIP_PERIOD_MIN_GLOBAL_SPAWN_PCT = float64(40)
	DEFAULT_LOG_LAST_STATS_PERIOD            = false
	DEFAULT_NO_NESTING_POKEMON_AGE_HOURS     = 12
)

type Config struct {
	// Whether to log the last stats period when processing
	LogLastStatsPeriod bool `koanf:"log_last_stats_period" json:"log_last_stats_period"`
	// how often to rotate stats
	RotationIntervalMinutes int `koanf:"rotation_interval_minutes" json:"rotation_interval_minutes"`
	// Require this many horus of stats in order to produce the nesting pokemon and update the DB.
	MinHistoryDurationHours int `koanf:"min_history_duration_hours" json:"min_history_duration_hours"`
	// Hold stats covering at most this many hours.
	MaxHistoryDurationHours int `koanf:"max_history_duration_hours" json:"max_history_duration_hours"`
	// number of a particular pokemon type seen needed to count as nesting.
	MinNestPokemon int `koanf:"min_nest_pokemon" json:"min_nest_pokemon"`
	// pct of particular pokemon type seen needed to count as nesting.
	MinNestPokemonPct float64 `koanf:"min_nest_pokemon_pct" json:"min_nest_pokemon_pct"`
	// number of total pokemon seen in nest needed to count as nesting.
	MinTotalPokemon int `koanf:"min_total_pokemon" json:"min_total_pokemon"`
	// if > 0, ignore pokemon that are spawning globally more than this percent.
	MaxGlobalSpawnPct float64 `koanf:"max_global_spawn_pct" json:"max_global_spawn_pct"`
	// Mininium required pokemon NestPct/GlobalPct ratio.
	MinNestPctToGlobalPctRatio float64 `koanf:"min_nest_pct_to_global_pct_ratio" json:"min_nest_pct_to_global_pct_ratio"`
	// Throw out a whole time period if a single mon spawns at more than this percent globally.
	SkipPeriodMinGlobalSpawnPct float64 `koanf:"skip_period_min_global_spawn_pct" json:"skip_period_min_global_spawn_pct"`
	// How many hours without seeing a nesting pokemon before we unset it in DB.
	NoNestingPokemonAgeHours int `koanf:"no_nesting_pokemon_age_hours" json:"no_nesting_pokemon_age_hours"`
}

func (cfg *Config) writeConfiguration(buf *bytes.Buffer) {
	buf.WriteString(fmt.Sprintf("log_last_stats_period: %t, ", cfg.LogLastStatsPeriod))
	buf.WriteString(fmt.Sprintf("rotation_interval_minutes: %d(%s), ", cfg.RotationIntervalMinutes, cfg.RotationInterval()))
	buf.WriteString(fmt.Sprintf("min_history_duration_hours: %d(%s), ", cfg.MinHistoryDurationHours, cfg.MinHistoryDuration()))
	buf.WriteString(fmt.Sprintf("max_history_duration_hours: %d(%s), ", cfg.MaxHistoryDurationHours, cfg.MaxHistoryDuration()))
	buf.WriteString(fmt.Sprintf("min_nest_pokemon: %d, ", cfg.MinNestPokemon))
	buf.WriteString(fmt.Sprintf("min_nest_pokemon_pct: %0.3f, ", cfg.MinNestPokemonPct))
	buf.WriteString(fmt.Sprintf("min_total_pokemon: %d, ", cfg.MinTotalPokemon))
	buf.WriteString(fmt.Sprintf("max_global_spawn_pct: %0.3f, ", cfg.MaxGlobalSpawnPct))
	buf.WriteString(fmt.Sprintf("min_nest_pct_to_global_pct_ratio: %0.3f, ", cfg.MinNestPctToGlobalPctRatio))
	buf.WriteString(fmt.Sprintf("skip_period_min_global_spawn_pct: %0.3f, ", cfg.SkipPeriodMinGlobalSpawnPct))
	buf.WriteString(fmt.Sprintf("no_nesting_pokemon_age_hours: %0.3f", cfg.SkipPeriodMinGlobalSpawnPct))
}

func (cfg *Config) MinHistoryDuration() time.Duration {
	return time.Hour * time.Duration(cfg.MinHistoryDurationHours)
}

func (cfg *Config) MaxHistoryDuration() time.Duration {
	return time.Hour * time.Duration(cfg.MaxHistoryDurationHours)
}

func (cfg *Config) RotationInterval() time.Duration {
	return time.Minute * time.Duration(cfg.RotationIntervalMinutes)
}

func (cfg *Config) NoNestingPokemonAge() time.Duration {
	return time.Hour * time.Duration(cfg.NoNestingPokemonAgeHours)
}

func GetDefaultConfig() Config {
	return Config{
		LogLastStatsPeriod:          DEFAULT_LOG_LAST_STATS_PERIOD,
		RotationIntervalMinutes:     DEFAULT_ROTATION_INTERVAL_MINUTES,
		MinHistoryDurationHours:     DEFAULT_MIN_HISTORY_DURATION_HOURS,
		MaxHistoryDurationHours:     DEFAULT_MAX_HISTORY_DURATION_HOURS,
		MinNestPokemon:              DEFAULT_MIN_NEST_POKEMON,
		MinNestPokemonPct:           DEFAULT_MIN_NEST_POKEMON_PCT,
		MinTotalPokemon:             DEFAULT_MIN_TOTAL_POKEMON,
		MaxGlobalSpawnPct:           DEFAULT_MAX_GLOBAL_SPAWN_PCT,
		MinNestPctToGlobalPctRatio:  DEFAULT_MIN_NEST_PCT_TO_GLOBAL_PCT_RATIO,
		SkipPeriodMinGlobalSpawnPct: DEFAULT_SKIP_PERIOD_MIN_GLOBAL_SPAWN_PCT,
		NoNestingPokemonAgeHours:    DEFAULT_NO_NESTING_POKEMON_AGE_HOURS,
	}
}

func (cfg *Config) Validate() error {
	if val := cfg.RotationIntervalMinutes; val < 1 {
		return fmt.Errorf("invalid rotation_interval_minutes '%d': must be > 0", val)
	}

	if val := cfg.MinHistoryDurationHours; val < 1 || val > 12 {
		return fmt.Errorf("invalid min_history_duration_hours '%d': must be > %d and <= %d", val, 0, 12)
	}

	if val := cfg.MaxHistoryDurationHours; val <= 0 || val > 7*24 {
		return fmt.Errorf("invalid max_history_duration_hours '%d': must be > %d and <= %d", val, 0, 7*24)
	}

	if min, max := cfg.MinHistoryDurationHours, cfg.MaxHistoryDurationHours; min > max {
		return fmt.Errorf("min_history_duration_hours(%d) > max_history_duration_hours(%d)", min, max)
	}

	if maxGlobalSpawnPct := cfg.MaxGlobalSpawnPct; maxGlobalSpawnPct > 0 && maxGlobalSpawnPct < 1 {
		return fmt.Errorf("max_global_spawn_pct is too low (%0.3f < 1)", maxGlobalSpawnPct)
	}

	if skipPeriodMinGlobalSpawnPct := cfg.SkipPeriodMinGlobalSpawnPct; skipPeriodMinGlobalSpawnPct > 0 && skipPeriodMinGlobalSpawnPct < 3 {
		return fmt.Errorf("skip_period_min_global_spawn_pct is too low (%0.3f < 3)", skipPeriodMinGlobalSpawnPct)
	}

	return nil
}
