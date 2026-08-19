package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/functionboundary"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Boundary handles.  Each name below is the single functionboundary row type
// published under its public name; the private package keeps every
// constructor unexported, so the capability fence is unchanged.
type (
	// FunctionCapture is one ordered pair of existing captured Cells.
	FunctionCapture = functionboundary.Capture
	// OutcomeExit is one typed existing Outcome owned by a Body boundary.
	OutcomeExit = functionboundary.OutcomeExit
	// FunctionBoundary is an opaque exact-quartet-fenced handle for one
	// existing Function's Body, formal/capture Cells, vararg Cell, and
	// owning Outcomes.
	FunctionBoundary = functionboundary.Boundary
	// BodyBoundary is an opaque handle for one existing Body and its full
	// ordered Outcome range. It never exposes Function-only formal/capture
	// semantics.
	BodyBoundary = functionboundary.BodyBoundary
	// RootBoundary is the explicit assembly-entry Body boundary. It has no
	// Function/formal/capture accessors by design.
	RootBoundary = functionboundary.RootBoundary
)

// FunctionBoundaries is Flow's immutable Function/Body-boundary projection.
type FunctionBoundaries struct{ result *functionboundary.Result }

// OwnsBody authenticates an exact Body handle issued by this live projection.
// Equal handles from equivalent replay deliberately do not pass this fence.
func (view FunctionBoundaries) OwnsBody(boundary BodyBoundary) bool {
	return view.result.OwnsBody(boundary)
}

// OwnsFunction authenticates one Function boundary issued by this exact live
// projection. Equivalent replay remains Equal but is not hot-owner admitted.
func (view FunctionBoundaries) OwnsFunction(boundary FunctionBoundary) bool {
	return view.result.OwnsFunction(boundary)
}

func (view FunctionBoundaries) Count() int { return view.result.Count() }

func (view FunctionBoundaries) At(index int) (FunctionBoundary, bool) {
	return view.result.At(index)
}

// For resolves an existing Function directly by its canonical ordinal.
func (view FunctionBoundaries) For(function keyspace.Term) (FunctionBoundary, bool) {
	return view.result.For(function)
}

// ForFunctionBody resolves the Function owning an existing Function Body.
func (view FunctionBoundaries) ForFunctionBody(body keyspace.Term) (FunctionBoundary, bool) {
	return view.result.ForFunctionBody(body)
}

// ForFunctionOutcome resolves the Function owning an existing Function-body Outcome.
func (view FunctionBoundaries) ForFunctionOutcome(outcome keyspace.Term) (FunctionBoundary, bool) {
	return view.result.ForFunctionOutcome(outcome)
}

// ForBody resolves any existing Body, including the root/chunk Body. A root
// Body returns a BodyBoundary rather than a fabricated FunctionBoundary.
func (view FunctionBoundaries) ForBody(body keyspace.Term) (BodyBoundary, bool) {
	return view.result.ForBody(body)
}

// ForOutcome resolves the Body owning any existing Outcome, including a
// top-level Outcome.
func (view FunctionBoundaries) ForOutcome(outcome keyspace.Term) (BodyBoundary, bool) {
	return view.result.ForOutcome(outcome)
}

// Root returns the explicit assembly-entry Body boundary.
func (view FunctionBoundaries) Root() (RootBoundary, bool) { return view.result.Root() }

// ResolveContextID performs exact-quartet-fenced Function lookup.
func (view FunctionBoundaries) ResolveContextID(id identity.ContentID) (FunctionBoundary, bool) {
	return view.result.ResolveContextID(id)
}

// ResolveBodyContextID performs exact-quartet-fenced Body lookup.
func (view FunctionBoundaries) ResolveBodyContextID(id identity.ContentID) (BodyBoundary, bool) {
	return view.result.ResolveBodyContextID(id)
}
