package app_config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/areas"
	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/filters"
	"github.com/UnownHash/Fletchling/httpserver"
	"github.com/UnownHash/Fletchling/importer"
	"github.com/UnownHash/Fletchling/logging"
	"github.com/UnownHash/Fletchling/overpass"
	"github.com/UnownHash/Fletchling/processor"
	"github.com/UnownHash/Fletchling/pyroscope"
	"github.com/UnownHash/Fletchling/stats_collector"
	"github.com/UnownHash/Fletchling/webhook_sender"
)

const (
	DEFAULT_OVERPASS_URL = "https://overpass-api.de/api/interpreter"

	DEFAULT_FILTER_CONCURRENCY  = 4
	DEFAULT_MIN_SPAWNPOINTS     = 10
	DEFAULT_MIN_AREA_M2         = float64(100)
	DEFAULT_MAX_AREA_M2         = float64(10000000)
	DEFAULT_MAX_OVERLAP_PERCENT = float64(60)
	DEFAULT_NEST_NAME           = "Unknown Nest"
)

type KojiConfig struct {
	Url     string `koanf:"url"`
	Token   string `koanf:"token"`
	Project string `koanf:"project"`
}

func (cfg *KojiConfig) Validate() error {
	if cfg == nil {
		return nil
	}

	const fcStr = "/api/v1/geofence/feature-collection/"

	idx := strings.Index(cfg.Url, fcStr)
	if idx >= 0 {
		// get base url and project
		cfg.Project = cfg.Url[idx+len(fcStr):]
		cfg.Url = cfg.Url[:idx]
	}

	if cfg.Url == "" {
		return errors.New("No koji url configured")
	}
	if cfg.Project == "" {
		return errors.New("No koji project configured")
	}

	return nil
}

type Config struct {
	NestsDb         db_store.DBConfig                `koanf:"nests_db"`
	GolbatDb        *db_store.DBConfig               `koanf:"golbat_db"`
	Overpass        overpass.Config                  `koanf:"overpass"`
	Filters         filters.Config                   `koanf:"filters"`
	Importer        importer.Config                  `koanf:"importer"`
	Areas           areas.Config                     `koanf:"areas"`
	WebhookSettings webhook_sender.SettingsConfig    `koanf:"webhook_settings"`
	Webhooks        webhook_sender.WebhooksConfig    `koanf:"webhooks"`
	HTTP            httpserver.Config                `koanf:"http"`
	Logging         logging.Config                   `koanf:"logging"`
	Processor       processor.Config                 `koanf:"processor"`
	Pyroscope       pyroscope.Config                 `koanf:"pyroscope"`
	Prometheus      stats_collector.PrometheusConfig `koanf:"prometheus"`
}

func (cfg *Config) GetPrometheusConfig() stats_collector.PrometheusConfig {
	return cfg.Prometheus
}

func (cfg *Config) CreateLogger(rotate bool, teeWriter io.Writer) (*logrus.Logger, error) {
	return cfg.Logging.CreateLogger(rotate, true, teeWriter)
}

func (cfg *Config) Validate() error {
	if err := cfg.Areas.Validate(); err != nil {
		return err
	}

	if err := cfg.Filters.Validate(); err != nil {
		return err
	}

	cfg.Importer.MinAreaM2 = cfg.Filters.MinAreaM2
	cfg.Importer.MaxAreaM2 = cfg.Filters.MaxAreaM2

	if err := cfg.Importer.Validate(); err != nil {
		return err
	}

	if err := cfg.Webhooks.Validate(); err != nil {
		return err
	}

	if len(cfg.Webhooks) > 0 {
		if err := cfg.WebhookSettings.Validate(); err != nil {
			return err
		}
	}

	if err := cfg.Logging.Validate(); err != nil {
		return err
	}

	if err := cfg.HTTP.Validate(); err != nil {
		return err
	}

	if err := cfg.NestsDb.Validate(); err != nil {
		return err
	}

	if cfg.GolbatDb != nil {
		if err := cfg.GolbatDb.Validate(); err != nil {
			return err
		}
	}

	if err := cfg.Processor.Validate(); err != nil {
		return err
	}

	if err := cfg.Prometheus.Validate(); err != nil {
		return err
	}

	return nil
}

func GetDefaultConfig() Config {
	return Config{
		Areas: areas.GetDefaultConfig(),

		Processor: processor.GetDefaultConfig(),

		Overpass: overpass.Config{
			Url: DEFAULT_OVERPASS_URL,
		},

		Importer: importer.Config{
			DefaultName: DEFAULT_NEST_NAME,
			MinAreaM2:   DEFAULT_MIN_AREA_M2,
			MaxAreaM2:   DEFAULT_MAX_AREA_M2,
		},

		Filters: filters.Config{
			Concurrency:       DEFAULT_FILTER_CONCURRENCY,
			MinSpawnpoints:    DEFAULT_MIN_SPAWNPOINTS,
			MinAreaM2:         DEFAULT_MIN_AREA_M2,
			MaxAreaM2:         DEFAULT_MAX_AREA_M2,
			MaxOverlapPercent: DEFAULT_MAX_OVERLAP_PERCENT,
		},

		WebhookSettings: webhook_sender.SettingsConfig{
			FlushIntervalSeconds: 1,
		},

		Logging: logging.Config{
			LogDir:     filepath.FromSlash("./logs"),
			MaxSizeMB:  500,
			MaxAgeDays: 7,
			MaxBackups: 20,
			Compress:   true,
		},

		HTTP: httpserver.Config{
			Addr: "127.0.0.1:9042",
		},

		NestsDb: db_store.DBConfig{
			Addr: "127.0.0.1:3306",
			Db:   "fletchling",
		},

		Pyroscope: pyroscope.Config{
			ApplicationName:      "fletchling",
			MutexProfileFraction: 5,
			BlockProfileRate:     5,
		},

		Prometheus: stats_collector.GetDefaultPrometheusConfig(),
	}
}

func LoadConfig(filename string, defaultConfig Config) (*Config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("couldn't open '%s': %w", filename, err)
	}
	defer f.Close()

	k := koanf.New(".")
	err = k.Load(structs.Provider(defaultConfig, "koanf"), nil)
	if err != nil {
		return nil, fmt.Errorf("couldn't load default config: %w", err)
	}

	err = k.Load(file.Provider(filename), toml.Parser())
	if err != nil {
		return nil, fmt.Errorf("failed to load config file: %w", err)
	}

	var cfg Config

	err = k.Unmarshal("", &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	err = cfg.Validate()
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}
