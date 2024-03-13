package db_store

import (
	"errors"
	"fmt"
	"net/url"

	"github.com/jmoiron/sqlx"
)

type DBConfig struct {
	Addr     string `koanf:"addr"`
	User     string `koanf:"user"`
	Password string `koanf:"password"`
	Db       string `koanf:"db"`
	Port	 string `koanf:"port"`

	MaxPool int `koanf:"max_pool"`
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
	return fmt.Sprintf("%s:%s@(%s:%s)/%s", cfg.User, cfg.Password, cfg.Addr, cfg.Port, cfg.Db)
}

func (cfg *DBConfig) Validate() error {
	_, err := sqlx.Connect("mysql", cfg.AsDSN())
	return err
}
