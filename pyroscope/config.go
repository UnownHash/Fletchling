package pyroscope

type Config struct {
	ApplicationName      string `koanf:"application_name"`
	ServerAddress        string `koanf:"server_address"`
	ApiKey               string `koanf:"api_key"`
	MutexProfileFraction int    `koanf:"mutex_profile_fraction"`
	BlockProfileRate     int    `koanf:"block_profile_rate"`
}
