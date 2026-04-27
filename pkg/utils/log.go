package utils

import (
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// BootstrapLogrus level-peeks at os.Args before formal flag parsing to set the Logrus level.
// This solves the bootstrap paradox where we need to log initialization errors or
// raw arguments before the cloud-provider framework's flag parsing is complete.
// The log-level flag supported by cloud-provider framework
// -v, --v Level
//
//	number for the log level verbosity
//
// It maps the klog '-v' or '--v' verbosity levels to Logrus levels:
//
//	v >= 5: TraceLevel (Logs raw os.Args)
//	v >= 3: DebugLevel
//	v < 3:  InfoLevel (Default)
func BootstrapLogrus() {
	level := logrus.InfoLevel

	for i, arg := range os.Args {
		if strings.HasPrefix(arg, "-v=") || strings.HasPrefix(arg, "--v=") {
			// Cut splits at the first occurrence of "="
			_, value, found := strings.Cut(arg, "=")
			if found {
				level = MapK8sLevelToLogrus(value)
			}
		} else if (arg == "-v" || arg == "--v") && i+1 < len(os.Args) {
			level = MapK8sLevelToLogrus(os.Args[i+1])
		}
	}

	logrus.SetLevel(level)
}

// MapK8sLevelToLogrus converts a klog verbosity string to a logrus Level.
func MapK8sLevelToLogrus(vString string) logrus.Level {
	v, err := strconv.Atoi(vString)
	if err != nil {
		return logrus.InfoLevel
	}

	switch {
	case v >= 5:
		return logrus.TraceLevel
	case v >= 3:
		return logrus.DebugLevel
	default:
		return logrus.InfoLevel
	}
}
