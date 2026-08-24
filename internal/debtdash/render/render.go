// Package render turns debt-dashboard measurements into the printable
// table and gate verdict every wave boundary is checked against. It is
// the only debtdash package the CLI imports; measure's Report type is
// reachable here as an alias, and Measure is reachable as a thin
// pass-through, so the CLI never reaches around this package into measure
// directly.
package render

import "github.com/wippyai/go-lua/internal/debtdash/measure"

// Report is the measurement snapshot render formats and gates against.
type Report = measure.Report

// LOC is a non-test/test line-count split for one authored area.
type LOC = measure.LOC

// AreaLOC names one immediate subdirectory of domain/ together with its
// authored non-test/test line counts.
type AreaLOC = measure.AreaLOC

// Measure runs the measurement pass over root and returns its Report.
func Measure(root string) (Report, error) {
	return measure.Measure(root)
}

// Labeled pairs one commit label with its measured Report.
type Labeled struct {
	Commit string
	Report Report
}
