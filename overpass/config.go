package overpass

import "errors"

type Config struct {
	Url       string `koanf:"url"`
	UserAgent string `koanf:"user_agent"`
}

func (cfg *Config) Validate() error {
	if cfg.Url == "" {
		return errors.New("No overpass url configured")
	}

	return nil
}
