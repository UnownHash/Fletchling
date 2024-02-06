package db_store

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

type DBConfig struct {
	Addr     string `koanf:"addr"`
	User     string `koanf:"user"`
	Password string `koanf:"password"`
	Db       string `koanf:"db"`

	MaxPool int `koanf:"max_pool"`
}

func (cfg *DBConfig) AsDSN() string {
	return fmt.Sprintf("%s:%s@(%s)/%s", cfg.User, cfg.Password, cfg.Addr, cfg.Db)
}

func (cfg *DBConfig) Validate() error {
	_, err := sqlx.Connect("mysql", cfg.AsDSN())
	return err
}
