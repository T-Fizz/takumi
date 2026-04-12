package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheck(t *testing.T) {
	result := Check("done")
	assert.Contains(t, result, "done")
	assert.Contains(t, result, "✓")
}

func TestCross(t *testing.T) {
	result := Cross("failed")
	assert.Contains(t, result, "failed")
	assert.Contains(t, result, "✗")
}

func TestWarn(t *testing.T) {
	result := Warn("caution")
	assert.Contains(t, result, "caution")
	assert.Contains(t, result, "!")
}

func TestBullet(t *testing.T) {
	result := Bullet("item")
	assert.Contains(t, result, "item")
	assert.Contains(t, result, "→")
}

func TestFilePath(t *testing.T) {
	result := FilePath("src/main.go")
	assert.Contains(t, result, "src/main.go")
}

func TestCommand(t *testing.T) {
	result := Command("takumi build")
	assert.Contains(t, result, "takumi build")
}

func TestHeader(t *testing.T) {
	result := Header()
	assert.Contains(t, result, "匠")
	assert.Contains(t, result, "Takumi")
}

func TestStepDone(t *testing.T) {
	result := StepDone("created file")
	assert.Contains(t, result, "✓")
	assert.Contains(t, result, "created file")
}

func TestStepInfo(t *testing.T) {
	result := StepInfo("info message")
	assert.Contains(t, result, "→")
	assert.Contains(t, result, "info message")
}

func TestSummary(t *testing.T) {
	result := Summary("Title", "body text")
	assert.Contains(t, result, "Title")
	assert.Contains(t, result, "body text")
}

func TestDivider(t *testing.T) {
	result := Divider()
	assert.Contains(t, result, "─")
}

func TestFormatCount(t *testing.T) {
	assert.Equal(t, "1 package", FormatCount(1, "package", "packages"))
	assert.Equal(t, "0 packages", FormatCount(0, "package", "packages"))
	assert.Equal(t, "3 packages", FormatCount(3, "package", "packages"))
	assert.Equal(t, "1 error", FormatCount(1, "error", "errors"))
	assert.Equal(t, "5 errors", FormatCount(5, "error", "errors"))
}
