package overpass

import "errors"

type Config struct {
	Url string `koanf:"url" json:"url"`
}

func (cfg *Config) Validate() error {
	if cfg.Url == "" {
		return errors.New("No overpass url configured")
	}

	return nil
}
