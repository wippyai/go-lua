// Package solver provides a pluggable theory-solver seam over the numeric
// constraint IR. A Solver is a single theory backend (e.g. difference logic);
// a Portfolio runs a cheapest-first list of backends and decides entailment by
// refutation.
//
// Variables are pathdom.PathKey carried by the numeric constraints. A backend
// returns decision.Unknown for any constraint or goal outside its theory and
// never decision.Invalid unless it is a complete decision procedure.
package solver

import (
	"github.com/wippyai/go-lua/analysis/domain/constraint/decision"
	"github.com/wippyai/go-lua/analysis/domain/constraint/numeric"
)

// Solver is a single theory backend.
//
// Assert feeds an asserted constraint into the backend's state. Constraints
// outside the backend's theory are ignored. Entails reports whether the
// asserted set entails goal: Valid when proven, Invalid only when a complete
// backend refutes it, Unknown otherwise.
type Solver interface {
	Assert(c numeric.NumericConstraint)
	Entails(goal numeric.NumericConstraint) decision.Result
}
