package pyroscope

import (
	"os"
	"runtime"

	"github.com/grafana/pyroscope-go"
)

func Run(config Config) error {
	runtime.SetMutexProfileFraction(config.MutexProfileFraction)
	runtime.SetBlockProfileRate(config.BlockProfileRate)

	pyroscopeConfig := pyroscope.Config{
		ApplicationName: config.ApplicationName,
		ServerAddress:   config.ServerAddress,
		Tags:            map[string]string{"hostname": os.Getenv("HOSTNAME")},
		ProfileTypes: []pyroscope.ProfileType{
			pyroscope.ProfileCPU,
			pyroscope.ProfileAllocObjects,
			pyroscope.ProfileAllocSpace,
			pyroscope.ProfileInuseObjects,
			pyroscope.ProfileInuseSpace,

			pyroscope.ProfileGoroutines,
			pyroscope.ProfileMutexCount,
			pyroscope.ProfileMutexDuration,
			pyroscope.ProfileBlockCount,
			pyroscope.ProfileBlockDuration,
		},
	}

	if config.ApiKey != "" {
		pyroscopeConfig.AuthToken = config.ApiKey
	}

	_, err := pyroscope.Start(pyroscopeConfig)
	return err
}
