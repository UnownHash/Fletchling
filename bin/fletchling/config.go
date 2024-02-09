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
	"github.com/UnownHash/Fletchling/httpserver"
	"github.com/UnownHash/Fletchling/logging"
	"github.com/UnownHash/Fletchling/processor"
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
	Processor processor.Config `koanf:"processor"`

	Logging logging.Config    `koanf:"logging"`
	HTTP    httpserver.Config `koanf:"http"`

	Koji *KojiConfig `koanf:"koji"`

	NestsDb  db_store.DBConfig  `koanf:"nests_db"`
	GolbatDb *db_store.DBConfig `koanf:"golbat_db"`
}

func (cfg *Config) CreateLogger(rotate bool) *logrus.Logger {
	return cfg.Logging.CreateLogger(rotate, true)
}

func (cfg *Config) Validate() error {
	if err := cfg.Koji.Validate(); err != nil {
		return err
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

	return nil
}

var defaultConfig = Config{
	Processor: processor.GetDefaultConfig(),

	Logging: logging.Config{
		Filename:   filepath.FromSlash("logs/fletchling.log"),
		MaxSizeMB:  500,
		MaxAgeDays: 7,
		MaxBackups: 20,
		Compress:   true,
	},

	HTTP: httpserver.Config{
		Addr: "127.0.0.1:9042",
	},

	NestsDb: db_store.DBConfig{
		Addr:           "127.0.0.1:3306",
		Db:             "fletchling",
		MigrationsPath: "./db_store/sql",
	},

	/*
		GolbatDb: db_store.DBConfig{
			Addr: "127.0.0.1:3306",
			Db:   "golbat",
		},
	*/
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
