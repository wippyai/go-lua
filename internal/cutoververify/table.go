package cutoververify

import (
	"fmt"
	"strings"
)

// FormatTable renders results as a plain-text, left-aligned table with
// stable column widths derived from the given rows: CHECK, STATUS, NOTE.
// No color, no emoji, and the row order is exactly the input order.
func FormatTable(results []Result) string {
	nameWidth := len("CHECK")
	statusWidth := len("STATUS")
	for _, r := range results {
		if len(r.Name) > nameWidth {
			nameWidth = len(r.Name)
		}
		if len(r.Status) > statusWidth {
			statusWidth = len(r.Status)
		}
	}

	var b strings.Builder
	writeRow := func(name, status, note string) {
		fmt.Fprintf(&b, "%-*s  %-*s  %s\n", nameWidth, name, statusWidth, status, note)
	}
	writeRow("CHECK", "STATUS", "NOTE")
	writeRow(strings.Repeat("-", nameWidth), strings.Repeat("-", statusWidth), strings.Repeat("-", len("NOTE")))
	for _, r := range results {
		writeRow(r.Name, string(r.Status), r.Note)
	}
	return b.String()
}

// Overall reports whether every result passes, honoring treatWarnAsFail for
// StatusWarn rows, and the "RESULT: PASS"/"RESULT: FAIL" line to print
// beneath the table.
func Overall(results []Result, treatWarnAsFail bool) (bool, string) {
	pass := true
	for _, r := range results {
		if !r.Pass(treatWarnAsFail) {
			pass = false
			break
		}
	}
	if pass {
		return true, "RESULT: PASS"
	}
	return false, "RESULT: FAIL"
}
