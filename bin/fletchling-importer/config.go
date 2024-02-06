package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/toml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/sirupsen/logrus"

	"github.com/UnownHash/Fletchling/db_store"
	"github.com/UnownHash/Fletchling/importer"
	"github.com/UnownHash/Fletchling/logging"
)

type KojiConfig struct {
	Url   string `koanf:"url"`
	Token string `koanf:"token"`
}

func (cfg *KojiConfig) Validate() error {
	if cfg == nil {
		return nil
	}

	const fcStr = "/api/v1/geofence/feature-collection/"

	idx := strings.Index(cfg.Url, fcStr)
	if idx >= 0 {
		cfg.Url = cfg.Url[:idx]
	}

	if cfg.Url == "" {
		return errors.New("No koji url configured")
	}

	return nil
}

type OverpassConfig struct {
	Url string `json:"url"`
}

func (cfg *OverpassConfig) Validate() error {
	if cfg == nil {
		return nil
	}

	if cfg.Url == "" {
		return errors.New("No overpass url configured")
	}

	return nil
}

type Config struct {
	Logging logging.Config `koanf:"logging"`

	Importer importer.Config `koanf:"importer"`

	OverpassExporter *OverpassConfig `koanf:"overpass_exporter"`

	KojiOverpassSrc *KojiConfig `koanf:"koji_overpass_source"`
	KojiImporter    *KojiConfig `koanf:"koji_importer"`
	KojiExporter    *KojiConfig `koanf:"koji_exporter"`

	DBImporter *db_store.DBConfig `koanf:"db_importer"`
	DBExporter *db_store.DBConfig `koanf:"db_exporter"`
}

func (cfg *Config) CreateLogger() *logrus.Logger {
	return cfg.Logging.CreateLogger(nil, true)
}

func (cfg *Config) Validate() error {
	if err := cfg.OverpassExporter.Validate(); err != nil {
		return err
	}

	if err := cfg.KojiOverpassSrc.Validate(); err != nil {
		return err
	}

	if err := cfg.KojiImporter.Validate(); err != nil {
		return err
	}

	if err := cfg.KojiExporter.Validate(); err != nil {
		return err
	}

	if cfg.DBImporter != nil {
		if err := cfg.DBImporter.Validate(); err != nil {
			return err
		}
	}

	if cfg.DBExporter != nil {
		if err := cfg.DBExporter.Validate(); err != nil {
			return err
		}
	}

	if err := cfg.Logging.Validate(); err != nil {
		return err
	}

	return nil
}

var defaultConfig = Config{
	Logging: logging.Config{
		Filename:   filepath.FromSlash("logs/fletchling-importer.log"),
		MaxSizeMB:  100,
		MaxAgeDays: 7,
		MaxBackups: 20,
		Compress:   false,
	},
}

func LoadConfig(filename string) (*Config, error) {
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
