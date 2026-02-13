package main

import (
	"fmt"
	"strings"
)

// healthKeys are property names that get semantic coloring
var healthKeys = map[string]bool{
	"Health":      true,
	"HealthRollup": true,
	"State":       true,
	"Status":      true,
}

// formatValue renders a Go value with color coding
func formatValue(v any) string {
	if v == nil {
		return nullStyle.Render("null")
	}
	switch val := v.(type) {
	case string:
		return stringStyle.Render(fmt.Sprintf("%q", val))
	case bool:
		if val {
			return trueStyle.Render("true")
		}
		return falseStyle.Render("false")
	case float64:
		if val == float64(int64(val)) {
			return numberStyle.Render(fmt.Sprintf("%d", int64(val)))
		}
		return numberStyle.Render(fmt.Sprintf("%g", val))
	default:
		return fmt.Sprintf("%v", val)
	}
}

// formatHealthValue renders health/state values with semantic colors
func formatHealthValue(name string, v any) string {
	if !healthKeys[name] {
		return formatValue(v)
	}
	s, ok := v.(string)
	if !ok {
		return formatValue(v)
	}
	upper := strings.ToUpper(s)
	switch {
	case upper == "OK" || upper == "ENABLED" || upper == "UP":
		return healthOKStyle.Render(s)
	case upper == "WARNING" || upper == "STANDBYOFFLINE" || upper == "STARTING":
		return healthWarningStyle.Render(s)
	case upper == "CRITICAL" || upper == "DISABLED" || upper == "ABSENT":
		return healthCriticalStyle.Render(s)
	default:
		return formatValue(v)
	}
}

// formatPlainValue renders a value without ANSI codes (for measuring widths)
func formatPlainValue(v any) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%v", val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
