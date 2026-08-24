// Package cutoververify mechanizes the cutover landing ritual: index
// cleanliness, a standalone build in a cached clone, a protocol-zero grep,
// targeted tests, and an optional ladder-fixture regression diff.
package cutoververify

// Status is the outcome of one check.
type Status string

const (
	StatusPass Status = "PASS"
	StatusFail Status = "FAIL"
	StatusWarn Status = "WARN"
	StatusSkip Status = "SKIP"
)

// Result is one row of the verification report. Detail holds the text a
// human needs to act on a non-PASS outcome; it is printed as the check runs
// and is not repeated in the summary table.
type Result struct {
	Name   string
	Status Status
	Note   string
	Detail string
}

// Pass reports whether the result should count toward an overall PASS.
// StatusWarn counts as passing unless the caller has asked for it to be
// treated as fatal.
func (r Result) Pass(treatWarnAsFail bool) bool {
	switch r.Status {
	case StatusPass, StatusSkip:
		return true
	case StatusWarn:
		return !treatWarnAsFail
	default:
		return false
	}
}
