package ui

import (
	"testing"

	"github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

func TestSetVerbose_On(t *testing.T) {
	SetVerbose(true)
	assert.Equal(t, log.DebugLevel, Log.GetLevel())
	// Reset
	SetVerbose(false)
}

func TestSetVerbose_Off(t *testing.T) {
	SetVerbose(false)
	assert.Equal(t, log.InfoLevel, Log.GetLevel())
}

func TestLoggerInitialized(t *testing.T) {
	assert.NotNil(t, Log, "global logger should be initialized by init()")
}

func TestTakumiLogStyles(t *testing.T) {
	s := takumiLogStyles()
	assert.NotNil(t, s)
	assert.NotNil(t, s.Levels[log.InfoLevel])
	assert.NotNil(t, s.Levels[log.WarnLevel])
	assert.NotNil(t, s.Levels[log.ErrorLevel])
	assert.NotNil(t, s.Levels[log.DebugLevel])
}
