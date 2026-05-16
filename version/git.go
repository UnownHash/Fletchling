package version

import "runtime/debug"

func GetGitSHA() string {
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return "unknown"
}
