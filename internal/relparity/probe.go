// Package relparity is the external two-binary parity harness for the sealed
// relational engine cut (docs/architecture/relation-engine.md, Wave 4C).
//
// The harness builds a baseline observation binary from one git revision and a
// replacement observation binary from another, runs each as its own process
// over a shared fixture tree, and compares the two dumps row by row under a
// stable key.
//
// It imports no analyzer package. That is the Wave 4C fence made structural:
// a harness that cannot link a runtime cannot link two of them, so the only
// contact either runtime has with the comparison is its process stdout. The
// fence is enforced by a law over this package's own import list.
package relparity

import "time"

// Probe is the observation contract both sides must satisfy: one command that
// takes a fixture name and a dump verb and writes an accessor=value dump to
// stdout.
//
// The contract is deliberately the one the baseline already implements, so the
// harness runs against a pinned historical revision without that revision
// carrying harness-specific code. A replacement revision satisfies parity by
// implementing the same command, not by teaching the harness a second shape.
type Probe struct {
	// Package is the go package pattern of the observation command, built
	// inside each side's checkout.
	Package string
	// Verbs are the dump verbs run per fixture, in comparison order.
	Verbs []string
	// Timeout bounds one (fixture, verb) process.
	Timeout time.Duration
}

// DefaultProbe is the observation contract as it stands at the baseline: the
// inspector session command, over the four verbs that together carry stable
// key, scope, value, outcome, diagnostic and canonical lineage.
//
//	target    the compiled target and its declared surface
//	publish   published families, cells, native rows with provenance, findings
//	why       the derivation chain each family's answer was concluded through
//	rows      the solved row inventory
func DefaultProbe() Probe {
	return Probe{
		Package: "./cmd/solvedump",
		Verbs:   []string{"target", "rows", "publish", "why"},
		Timeout: 40 * time.Second,
	}
}

// Side is one separately built runtime under comparison.
type Side struct {
	// Name is the side's role in the report.
	Name string
	// Ref is the git revision the binary was built from.
	Ref string
	// Commit is the resolved full SHA of Ref.
	Commit string
	// Binary is the absolute path of the built observation binary. It lives
	// outside every checkout so a later checkout reset cannot invalidate it.
	Binary string
	// Env is appended to the process environment of every run of this side.
	Env []string
}

// Roles are the two fixed side names.
const (
	RoleBaseline    = "baseline"
	RoleReplacement = "replacement"
)
