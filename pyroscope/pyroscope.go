package pyroscope

import (
	"github.com/grafana/pyroscope-go"
	"os"
	"runtime"
)

type Status struct {
	Started bool
	Error   error
}

func Run(config Config) Status {
	if config.ServerAddress != "" {
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

		if err != nil {
			return Status{
				Started: false,
				Error:   err,
			}
		}
		return Status{
			Started: true,
			Error:   nil,
		}
	}
	return Status{
		Started: false,
		Error:   nil,
	}
}
