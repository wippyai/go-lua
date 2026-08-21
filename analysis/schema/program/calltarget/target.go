// Package calltarget owns the canonical cold proof connecting one closure
// allocation to its callable body.
package calltarget

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
)

// Target is the exact closure-allocation-to-callable-body proof: which
// allocation is called, which body it enters, and the context and formal that
// body was compiled with. Its fields are intentionally private so every
// published row passes validation.
type Target struct {
	allocation identity.ContentID
	body       identity.ContentID
	context    identity.ContentID
	function   identity.ContentID
	formal     identity.ContentID
}

// NewTarget admits one complete call-target proof.
func NewTarget(allocation, body, context, function, formal identity.ContentID) (Target, bool) {
	row := Target{allocation: allocation, body: body, context: context, function: function, formal: formal}
	return row, row.Available()
}

// Available reports whether the row names a complete proof.
func (row Target) Available() bool {
	return row.allocation.Available() && row.body.Available() && row.context.Available() &&
		row.function.Available() && row.formal.Available()
}

func (row Target) AllocationID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.allocation
}

func (row Target) BodyID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row Target) ContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.context
}

func (row Target) FunctionID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.function
}

func (row Target) FormalID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.formal
}

// Family is the canonical manifest binding for call-target rows.
func Family() programfamily.Family[Target] {
	return programfamily.New[Target](programcatalog.CallTarget())
}
