package main

import "github.com/charmbracelet/lipgloss"

// All styles use ANSI colors 0–15 so they follow the terminal's theme
// (Solarized, Dracula, Gruvbox, etc. all remap these).
//
//   0: black    8: bright black (dark gray)
//   1: red      9: bright red
//   2: green   10: bright green
//   3: yellow  11: bright yellow
//   4: blue    12: bright blue
//   5: magenta 13: bright magenta
//   6: cyan    14: bright cyan
//   7: white   15: bright white

var (
	// Panel borders
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.ANSIColor(8))

	// Status bar
	statusStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.ANSIColor(11)).
			Reverse(true).
			Padding(0, 1)

	// Breadcrumb
	breadcrumbStyle     = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(7))
	breadcrumbSepStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))
	breadcrumbLastStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.ANSIColor(15))

	// Help bar
	helpKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))
	helpDescStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(7))

	// Tree items
	cursorStyle    = lipgloss.NewStyle().Reverse(true).Bold(true)
	childStyle     = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(12)) // Bright blue
	objectStyle    = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(5))  // Magenta
	arrayStyle     = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(5))  // Magenta
	linkStyle      = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3))  // Yellow
	propNameStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))  // Green
	indicatorStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))  // Dark gray

	// Values
	stringStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))  // Green
	numberStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(4))  // Blue
	nullStyle   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))  // Dark gray
	trueStyle   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(10)) // Bright green
	falseStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1))  // Red

	// Health/status semantic colors
	healthOKStyle       = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(10)) // Bright green
	healthWarningStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(11)) // Bright yellow
	healthCriticalStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(9))  // Bright red

	// Details panel
	detailLabelStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true) // Yellow
	detailValueStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(7))            // White

	// Search overlay
	searchPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3)).Bold(true) // Yellow
	searchMatchStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(6))            // Cyan

	// Action overlay
	actionTitleStyle   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)).Bold(true) // Red
	actionNameStyle    = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3))            // Yellow
	actionTargetStyle  = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(4))            // Blue
	actionConfirmStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1)).Bold(true) // Red
	actionSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))            // Green
	actionErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1))            // Red

	// Loading
	loadingStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8)).Italic(true)

	// Overlay panel (search/action modals)
	overlayStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.ANSIColor(3)).
			Padding(0, 1)

	// Separator between tree and details
	separatorStyle = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(8))
)
