package ui

import (
	"os"

	"github.com/charmbracelet/log"
)

// Log is the global Takumi logger.
var Log *log.Logger

func init() {
	Log = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
	})
	Log.SetStyles(takumiLogStyles())
}

// SetVerbose enables debug-level logging.
func SetVerbose(on bool) {
	if on {
		Log.SetLevel(log.DebugLevel)
	} else {
		Log.SetLevel(log.InfoLevel)
	}
}

// takumiLogStyles returns custom log styles matching the Takumi palette.
func takumiLogStyles() *log.Styles {
	s := log.DefaultStyles()
	s.Levels[log.InfoLevel] = Success.Bold(true)
	s.Levels[log.WarnLevel] = Warning.Bold(true)
	s.Levels[log.ErrorLevel] = Error.Bold(true)
	s.Levels[log.DebugLevel] = Muted.Bold(true)
	return s
}
