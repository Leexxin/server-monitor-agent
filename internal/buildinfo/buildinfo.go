package buildinfo

import "runtime"

var (
	Version   = "dev"
	Revision  = "unknown"
	BuildTime = "unknown"
)

type Info struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	BuildTime string `json:"buildTime"`
	GoVersion string `json:"goVersion"`
}

func Current() Info {
	return Info{
		Version:   Version,
		Revision:  Revision,
		BuildTime: BuildTime,
		GoVersion: runtime.Version(),
	}
}
