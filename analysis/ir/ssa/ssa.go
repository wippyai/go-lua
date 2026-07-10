// Package ssa defines reusable SSA value identifiers and phi metadata over CFG points.
//
// CFG construction owns graph topology. This package is a small theory leaf so
// higher layers can attach versions and phi nodes to cfg.Point values when SSA
// wiring is enabled, without adding SSA state back to the CFG package.
package ssa

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Version represents a stable SSA version of a variable.
//
// In SSA (Static Single Assignment) form, each assignment creates a new version
// of a variable. The Version type captures both the variable identity and which
// assignment created this particular value:
//
//	local x = 1     -- x@1
//	x = x + 1       -- x@2 (new version)
//	if cond then
//	    x = 10      -- x@3 in then-branch
//	else
//	    x = 20      -- x@4 in else-branch
//	end
//	-- x@5 = phi(x@3, x@4) at join point
//
// Version components:
//   - Root: The variable name used for display/debug output
//   - Symbol: The symbol.ID of the declaration (distinguishes shadowed names)
//   - ID: The version number (0 = undefined, 1+ = specific assignment)
type Version struct {
	Root   string    // Variable name used for display/debug output
	Symbol symbol.ID // Declaration identity (distinguishes same-named variables in different scopes)
	ID     int       // Version number within the function (0 = undefined/uninitialized)
}

// IsZero returns true if this is an undefined/uninitialized version.
// Zero versions indicate the variable has not been assigned on this path.
func (v Version) IsZero() bool {
	return v.ID == 0
}

// String returns a human-readable representation of the version.
func (v Version) String() string {
	return v.Root + "@" + strconv.Itoa(v.ID)
}

// PhiOperand represents one incoming value for a phi node.
//
// At a join point where multiple control flow paths merge, a phi node needs
// to know which version of a variable comes from each predecessor. Each
// PhiOperand pairs a predecessor point with the version visible at that point.
type PhiOperand struct {
	From    cfg.Point // Predecessor point where this version comes from
	Version Version   // The SSA version from that predecessor
}

// PhiNode represents a phi function at a control flow join point.
//
// The type is intentionally kept as a standalone IR surface so dominance and
// SSA wiring can consume it later without changing its shape.
//
// Phi nodes are the mechanism for merging variable versions after control
// flow divergence. When an if/else or loop creates multiple definitions of
// a variable, a phi node at the join point unifies them into a single version.
//
// Example:
//
//	if cond then
//	    x = "hello"  -- x@1
//	else
//	    x = "world"  -- x@2
//	end
//	-- phi node: x@3 = phi(x@1 from then-branch, x@2 from else-branch)
//	print(x)  -- uses x@3
//
// For type checking, the phi node's type is typically the union of all
// operand types.
type PhiNode struct {
	Point    cfg.Point    // CFG point where the phi is located
	Target   Version      // The new version created by this phi
	Operands []PhiOperand // Incoming versions from each predecessor
}
