package importer

type Config struct {
	MinAreaM2           float64 `koanf:"min_area_m2"`
	MaxAreaM2           float64 `koanf:"max_area_m2"`
	DefaultName         string  `koanf:"default_name"`
	DefaultNameLocation bool    `koanf:"default_name_location"`
	AllowContained      bool    `koanf:"allow_contained"`
}

func (cfg *Config) Validate() error {
	return nil
}
