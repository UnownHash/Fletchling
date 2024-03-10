package areas

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	KojiUrl   string `koanf:"koji_url"`
	KojiToken string `koanf:"koji_token"`
	Filename  string `koanf:"filename"`

	CacheDir      string `koanf:"cache_dir"`
	CacheFilename string `koanf:"cache_filename"`

	// computed during validation
	KojiBaseUrl string `koanf:"-"`
	KojiProject string `koanf:"-"`
}

func (cfg *Config) Validate() error {
	if cfg.KojiUrl != "" {
		if cfg.Filename != "" {
			return errors.New("only specify one of 'areas.koji_url' or 'areas.filename'")
		}

		uri, err := url.Parse(cfg.KojiUrl)
		if err != nil {
			return fmt.Errorf("'areas.koji_url' looks malformed: %w", err)
		}

		const fcStr = "/api/v1/geofence/feature-collection/"

		if !strings.HasPrefix(uri.Path, fcStr) {
			return fmt.Errorf("'areas.koji_url' looks malformed: '%s'(%s) does not start with '%s'", uri.Path, uri.String(), fcStr)
		}

		cfg.KojiProject = uri.Path[len(fcStr):]
		if cfg.KojiProject == "" {
			return fmt.Errorf("'areas.koji_url' looks malformed: the project is missing")
		}

		uri.Path = ""
		cfg.KojiBaseUrl = uri.String()

		return nil
	}

	if cfg.Filename == "" {
		return errors.New("One of 'areas.koji_url' or 'areas.filename' must be configured")
	}

	f, err := os.Open(cfg.Filename)
	if err != nil {
		return fmt.Errorf("'areas.filename' is '%s', which is missing or not accessible: %w", cfg.Filename, err)
	}

	f.Close()

	return nil
}

func GetDefaultConfig() Config {
	return Config{
		CacheDir:      filepath.FromSlash("./.cache"),
		CacheFilename: "areas-cache.json",
	}
}
