// Package ui holds the terminal screens and renderers of Finux.
package ui

import (
	"charm.land/lipgloss/v2"

	"finux/src/account"
)

var (
	colorAccent = lipgloss.Color("2")
	colorDim    = lipgloss.Color("8")
	colorError  = lipgloss.Color("1")

	styleAccent = lipgloss.NewStyle().Foreground(colorAccent)
	styleTitle  = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleBold   = lipgloss.NewStyle().Bold(true)
	styleDim    = lipgloss.NewStyle().Foreground(colorDim)
	styleError  = lipgloss.NewStyle().Foreground(colorError)
	styleBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorDim).Padding(1, 2)
)

// swatch renders text in a palette color. Unknown keys give unstyled text.
func swatch(key, text string) string {
	color, ok := account.ColorByKey(key)
	if !ok {
		return text
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(color.Hex)).Render(text)
}

// Success formats the feedback line of an action that worked.
func Success(message string) string {
	return styleAccent.Render("✓") + " " + message
}

// Failure formats the feedback line of an action that did not work.
func Failure(message string) string {
	return styleError.Render("✗") + " " + message
}

// Hint formats secondary text.
func Hint(message string) string {
	return styleDim.Render(message)
}
