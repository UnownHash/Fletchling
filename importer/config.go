package importer

type Config struct {
	DefaultName         string `koanf:"default_name"`
	DefaultNameLocation bool   `koanf:"default_name_location"`

	// below here will be populated from filters.Config

	// minimum area required in order to track.
	MinAreaM2 float64 `koanf:"-" json:"-"`
	// maximum area that cannot be exceeded in order to track.
	MaxAreaM2 float64 `koanf:"-" json:"-"`
}

func (cfg *Config) Validate() error {
	return nil
}
