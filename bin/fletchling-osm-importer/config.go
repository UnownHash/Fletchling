package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/importer"
	"github.com/UnownHash/Fletchling/logging"
)

const (
	DEFAULT_NEST_NAME    = "Unknown Nest"
	DEFAULT_OVERPASS_URL = "https://overpass-api.de/api/interpreter"
	DEFAULT_MIN_AREA_M2  = 100.0
	DEFAULT_MAX_AREA_M2  = 10000000.0
)

type AreasConfig struct {
	KojiUrl   string `koanf:"koji_url"`
	KojiToken string `koanf:"koji_token"`
	Filename  string `koanf:"filename"`
}

func (cfg *AreasConfig) Validate() error {
	if cfg.KojiUrl == "" && cfg.Filename == "" {
		return errors.New("areas config is missing one of 'koji_url' or 'filename'")
	}

	if cfg.KojiUrl != "" {
		if cfg.Filename != "" {
			return errors.New("areas config should only have one of 'koji_url' or 'filename'")
		}
		_, err := url.Parse(cfg.KojiUrl)
		if err != nil {
			return fmt.Errorf("'areas.koji_url' config is not valid: %w", err)
		}
	}

	return nil
}

type OverpassConfig struct {
	Url string `json:"url"`
}

func (cfg *OverpassConfig) Validate() error {
	if cfg.Url == "" {
		return errors.New("No overpass url configured")
	}

	return nil
}

type Config struct {
	Importer importer.Config `koanf:"importer"`

	Areas AreasConfig `koanf:"areas"`

	NestsDB  db_store.DBConfig  `koanf:"nests_db"`
	GolbatDB *db_store.DBConfig `koanf:"golbat_db"`

	Overpass OverpassConfig `koanf:"overpass"`

	Logging logging.Config `koanf:"logging"`
}

func (cfg *Config) CreateLogger(rotate bool) *logrus.Logger {
	return cfg.Logging.CreateLogger(rotate, true)
}

func (cfg *Config) Validate() error {
	if err := cfg.Importer.Validate(); err != nil {
		return err
	}

	if err := cfg.Areas.Validate(); err != nil {
		return err
	}

	if err := cfg.NestsDB.Validate(); err != nil {
		return err
	}

	if cfg.GolbatDB != nil {
		if err := cfg.GolbatDB.Validate(); err != nil {
			return err
		}
	}

	if err := cfg.Overpass.Validate(); err != nil {
		return err
	}

	if err := cfg.Logging.Validate(); err != nil {
		return err
	}

	return nil
}

var defaultConfig = Config{
	Importer: importer.Config{
		MinAreaM2:   DEFAULT_MIN_AREA_M2,
		MaxAreaM2:   DEFAULT_MAX_AREA_M2,
		DefaultName: DEFAULT_NEST_NAME,
	},
	NestsDB: db_store.DBConfig{
		Addr: "127.0.0.1:3306",
		Db:   "fletchling",
	},
	Overpass: OverpassConfig{
		Url: DEFAULT_OVERPASS_URL,
	},
	Logging: logging.Config{
		Filename:   filepath.FromSlash("logs/fletchling-osm-importer.log"),
		MaxSizeMB:  100,
		MaxAgeDays: 7,
		MaxBackups: 20,
		Compress:   false,
	},
}

func LoadConfig(filename string) (*Config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
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
