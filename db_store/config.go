package db_store

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"github.com/jmoiron/sqlx"
)

type DBConfig struct {
	Addr     string `koanf:"addr"`
	User     string `koanf:"user"`
	Password string `koanf:"password"`
	Db       string `koanf:"db"`

	MaxPool        int    `koanf:"max_pool"`
	MigrationsPath string `koanf:"migrations_path"`
}

func (cfg *DBConfig) SetFromUri(uri *url.URL) error {
	if ui := uri.User; ui != nil {
		cfg.User = ui.Username()
		cfg.Password, _ = ui.Password()
	}
	cfg.Addr = uri.Host
	cfg.Db = uri.Path
	for len(cfg.Db) > 0 && cfg.Db[0] == '/' {
		cfg.Db = cfg.Db[1:]
	}
	if cfg.Db == "" {
		return errors.New("no database name in uri path")
	}
	return nil
}

func (cfg *DBConfig) AsDSN() string {
	return fmt.Sprintf("%s:%s@(%s)/%s", cfg.User, cfg.Password, cfg.Addr, cfg.Db)
}

func (cfg *DBConfig) Validate() error {
	if path := cfg.MigrationsPath; path != "" {
		fi, err := os.Stat(cfg.MigrationsPath)
		if err != nil {
			return fmt.Errorf("'migrations' path '%s' looks not great: %s", path, err)
		}
		if !fi.IsDir() {
			return fmt.Errorf("'migrations' path '%s' is not a directory", path)
		}
	}
	_, err := sqlx.Connect("mysql", cfg.AsDSN())
	return err
}
