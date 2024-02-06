package httpserver

import "errors"

type Config struct {
	Addr string `koanf:"addr"`
}

func (cfg *Config) Validate() error {
	if cfg.Addr == "" {
		return errors.New("no http addr configured")
	}
	return nil
}
