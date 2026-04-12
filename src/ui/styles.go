// Package ui provides styled terminal output for Takumi CLI commands.
package ui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// --- Colors ---

// Takumi brand palette — warm, craftsman-inspired tones.
var (
	ColorPrimary   = lipgloss.Color("#E8A87C") // warm amber
	ColorSecondary = lipgloss.Color("#95DAC1") // sage green
	ColorAccent    = lipgloss.Color("#C49BBB") // soft purple
	ColorSuccess   = lipgloss.Color("#73D2A0") // mint green
	ColorWarning   = lipgloss.Color("#F4C95D") // golden yellow
	ColorError     = lipgloss.Color("#E76F51") // terracotta
	ColorMuted     = lipgloss.Color("#7C7C7C") // gray
	ColorBright    = lipgloss.Color("#FAFAFA") // near-white
)

// --- Text styles ---

var (
	// Bold is bold bright text.
	Bold = lipgloss.NewStyle().Bold(true).Foreground(ColorBright)

	// Muted is dimmed text for secondary info.
	Muted = lipgloss.NewStyle().Foreground(ColorMuted)

	// Primary is the main brand color text.
	Primary = lipgloss.NewStyle().Foreground(ColorPrimary)

	// Success text.
	Success = lipgloss.NewStyle().Foreground(ColorSuccess)

	// Warning text.
	Warning = lipgloss.NewStyle().Foreground(ColorWarning)

	// Error text.
	Error = lipgloss.NewStyle().Foreground(ColorError)

	// Accent text.
	Accent = lipgloss.NewStyle().Foreground(ColorAccent)
)

// --- Composite elements ---

var (
	// Banner is the Takumi header style.
	Banner = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		MarginBottom(1)

	// SectionHeader for grouping output.
	SectionHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSecondary).
			MarginTop(1)

	// Bullet is the prefix for list items.
	BulletStyle = lipgloss.NewStyle().
			Foreground(ColorAccent)

	// FilePathStyle highlights file/directory paths.
	FilePathStyle = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Italic(true)

	// CommandStyle highlights CLI commands.
	CommandStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	// BoxStyle for framed content blocks.
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2)
)

// --- Helper functions ---

// Check renders a success checkmark.
func Check(msg string) string {
	return Success.Render("✓") + " " + msg
}

// Cross renders a failure mark.
func Cross(msg string) string {
	return Error.Render("✗") + " " + msg
}

// Warn renders a warning bullet.
func Warn(msg string) string {
	return Warning.Render("!") + " " + msg
}

// Bullet renders a styled bullet point.
func Bullet(msg string) string {
	return BulletStyle.Render("→") + " " + msg
}

// FilePath renders a highlighted file path.
func FilePath(path string) string {
	return FilePathStyle.Render(path)
}

// Command renders a highlighted command.
func Command(cmd string) string {
	return CommandStyle.Render(cmd)
}

// Header renders the Takumi banner.
func Header() string {
	return Banner.Render("匠 Takumi")
}

// StepDone renders a completed step.
func StepDone(msg string) string {
	return "  " + Check(msg)
}

// StepInfo renders an info step.
func StepInfo(msg string) string {
	return "  " + Bullet(msg)
}

// Summary renders a boxed summary block.
func Summary(title, body string) string {
	header := Bold.Render(title)
	return BoxStyle.Render(header + "\n" + body)
}

// Divider renders a subtle horizontal line.
func Divider() string {
	return Muted.Render("─────────────────────────────────────")
}

// FormatCount renders a count with unit, e.g. "3 packages".
func FormatCount(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}
