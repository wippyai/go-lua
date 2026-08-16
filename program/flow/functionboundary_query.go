package flow

import (
	"github.com/wippyai/go-lua/program/flow/internal/functionboundary"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// FunctionCapture is one ordered pair of existing captured Cells.
type FunctionCapture struct {
	Inner     keyspace.Term
	Outer     keyspace.Term
	InnerBody keyspace.Term
	OuterBody keyspace.Term
}

// OutcomeExit is one typed existing Outcome owned by a Body boundary.
type OutcomeExit struct {
	Outcome keyspace.Term
	Body    keyspace.Term
	Kind    kind.OutcomeKind
	Target  keyspace.Term
}

// FunctionBoundaries is Flow's immutable Function/Body-boundary projection.
type FunctionBoundaries struct{ result *functionboundary.Result }

// OwnsBody authenticates an exact Body handle issued by this live projection.
// Equal handles from equivalent replay deliberately do not pass this fence.
func (view FunctionBoundaries) OwnsBody(boundary BodyBoundary) bool {
	return view.result != nil && view.result.OwnsBody(boundary.boundary)
}

// OwnsFunction authenticates one Function boundary issued by this exact live
// projection. Equivalent replay remains Equal but is not hot-owner admitted.
func (view FunctionBoundaries) OwnsFunction(boundary FunctionBoundary) bool {
	return view.result != nil && view.result.OwnsFunction(boundary.boundary)
}

func (view FunctionBoundaries) Count() int {
	if view.result == nil {
		return 0
	}
	return view.result.Count()
}

func (view FunctionBoundaries) At(index int) (FunctionBoundary, bool) {
	if view.result == nil {
		return FunctionBoundary{}, false
	}
	boundary, ok := view.result.At(index)
	return FunctionBoundary{boundary: boundary}, ok
}

// For resolves an existing Function directly by its canonical ordinal.
func (view FunctionBoundaries) For(function keyspace.Term) (FunctionBoundary, bool) {
	if view.result == nil {
		return FunctionBoundary{}, false
	}
	boundary, ok := view.result.For(function)
	return FunctionBoundary{boundary: boundary}, ok
}

// ForFunctionBody resolves the Function owning an existing Function Body.
func (view FunctionBoundaries) ForFunctionBody(body keyspace.Term) (FunctionBoundary, bool) {
	if view.result == nil {
		return FunctionBoundary{}, false
	}
	boundary, ok := view.result.ForFunctionBody(body)
	return FunctionBoundary{boundary: boundary}, ok
}

// ForFunctionOutcome resolves the Function owning an existing Function-body Outcome.
func (view FunctionBoundaries) ForFunctionOutcome(outcome keyspace.Term) (FunctionBoundary, bool) {
	if view.result == nil {
		return FunctionBoundary{}, false
	}
	boundary, ok := view.result.ForFunctionOutcome(outcome)
	return FunctionBoundary{boundary: boundary}, ok
}

// ForBody resolves any existing Body, including the root/chunk Body. A root
// Body returns a BodyBoundary rather than a fabricated FunctionBoundary.
func (view FunctionBoundaries) ForBody(body keyspace.Term) (BodyBoundary, bool) {
	if view.result == nil {
		return BodyBoundary{}, false
	}
	boundary, ok := view.result.ForBody(body)
	return BodyBoundary{boundary: boundary}, ok
}

// ForOutcome resolves the Body owning any existing Outcome, including a
// top-level Outcome.
func (view FunctionBoundaries) ForOutcome(outcome keyspace.Term) (BodyBoundary, bool) {
	if view.result == nil {
		return BodyBoundary{}, false
	}
	boundary, ok := view.result.ForOutcome(outcome)
	return BodyBoundary{boundary: boundary}, ok
}

// Root returns the explicit assembly-entry Body boundary.
func (view FunctionBoundaries) Root() (RootBoundary, bool) {
	if view.result == nil {
		return RootBoundary{}, false
	}
	boundary, ok := view.result.Root()
	return RootBoundary{boundary: boundary}, ok
}

// ResolveContextID performs exact-quartet-fenced Function lookup.
func (view FunctionBoundaries) ResolveContextID(id keyspace.ContentID) (FunctionBoundary, bool) {
	if view.result == nil {
		return FunctionBoundary{}, false
	}
	boundary, ok := view.result.ResolveContextID(id)
	return FunctionBoundary{boundary: boundary}, ok
}

// ResolveBodyContextID performs exact-quartet-fenced Body lookup.
func (view FunctionBoundaries) ResolveBodyContextID(id keyspace.ContentID) (BodyBoundary, bool) {
	if view.result == nil {
		return BodyBoundary{}, false
	}
	boundary, ok := view.result.ResolveBodyContextID(id)
	return BodyBoundary{boundary: boundary}, ok
}

// FunctionBoundary is an opaque exact-quartet-fenced handle for one existing
// Function's Body, formal/capture Cells, vararg Cell, and owning Outcomes.
type FunctionBoundary struct{ boundary functionboundary.Boundary }

func (boundary FunctionBoundary) Available() bool { return boundary.boundary.Available() }
func (boundary FunctionBoundary) Equal(other FunctionBoundary) bool {
	return boundary.boundary.Equal(other.boundary)
}
func (boundary FunctionBoundary) ContextID() keyspace.ContentID {
	return boundary.boundary.ContextID()
}
func (boundary FunctionBoundary) Function() (keyspace.Term, bool) {
	return boundary.boundary.Function()
}
func (boundary FunctionBoundary) Owner() (keyspace.Term, bool) {
	return boundary.boundary.Owner()
}
func (boundary FunctionBoundary) Body() (keyspace.Term, bool) {
	return boundary.boundary.Body()
}
func (boundary FunctionBoundary) Entry() (keyspace.Term, bool) {
	return boundary.boundary.Entry()
}
func (boundary FunctionBoundary) Vararg() (keyspace.Term, bool) {
	return boundary.boundary.Vararg()
}
func (boundary FunctionBoundary) FormalCount() int {
	return boundary.boundary.FormalCount()
}
func (boundary FunctionBoundary) FormalAt(index int) (keyspace.Term, bool) {
	return boundary.boundary.FormalAt(index)
}
func (boundary FunctionBoundary) CaptureCount() int {
	return boundary.boundary.CaptureCount()
}
func (boundary FunctionBoundary) CaptureAt(index int) (FunctionCapture, bool) {
	capture, ok := boundary.boundary.CaptureAt(index)
	return FunctionCapture{Inner: capture.Inner, Outer: capture.Outer, InnerBody: capture.InnerBody, OuterBody: capture.OuterBody}, ok
}
func (boundary FunctionBoundary) OutcomeCount() int {
	return boundary.boundary.OutcomeCount()
}
func (boundary FunctionBoundary) OutcomeAt(index int) (OutcomeExit, bool) {
	exit, ok := boundary.boundary.OutcomeAt(index)
	return OutcomeExit{Outcome: exit.Outcome, Body: exit.Body, Kind: exit.Kind, Target: exit.Target}, ok
}

// BodyBoundary is an opaque handle for one existing Body and its full ordered
// Outcome range. It never exposes Function-only formal/capture semantics.
type BodyBoundary struct{ boundary functionboundary.BodyBoundary }

func (boundary BodyBoundary) Available() bool { return boundary.boundary.Available() }
func (boundary BodyBoundary) Equal(other BodyBoundary) bool {
	return boundary.boundary.Equal(other.boundary)
}
func (boundary BodyBoundary) ContextID() keyspace.ContentID {
	return boundary.boundary.ContextID()
}
func (boundary BodyBoundary) Body() (keyspace.Term, bool) {
	return boundary.boundary.Body()
}
func (boundary BodyBoundary) Entry() (keyspace.Term, bool) {
	return boundary.boundary.Entry()
}
func (boundary BodyBoundary) OutcomeCount() int {
	return boundary.boundary.OutcomeCount()
}
func (boundary BodyBoundary) OutcomeAt(index int) (OutcomeExit, bool) {
	exit, ok := boundary.boundary.OutcomeAt(index)
	return OutcomeExit{Outcome: exit.Outcome, Body: exit.Body, Kind: exit.Kind, Target: exit.Target}, ok
}
func (boundary BodyBoundary) OutcomeForTerm(outcome keyspace.Term) (OutcomeExit, int, bool) {
	exit, ordinal, ok := boundary.boundary.OutcomeForTerm(outcome)
	return OutcomeExit{Outcome: exit.Outcome, Body: exit.Body, Kind: exit.Kind, Target: exit.Target}, ordinal, ok
}

// RootBoundary is the explicit assembly-entry Body boundary. It has no
// Function/formal/capture accessors by design.
type RootBoundary struct{ boundary functionboundary.RootBoundary }

func (boundary RootBoundary) Available() bool { return boundary.boundary.Available() }
func (boundary RootBoundary) Equal(other RootBoundary) bool {
	return boundary.boundary.Equal(other.boundary)
}
func (boundary RootBoundary) ContextID() keyspace.ContentID {
	return boundary.boundary.ContextID()
}
func (boundary RootBoundary) Body() (keyspace.Term, bool) {
	return boundary.boundary.Body()
}
func (boundary RootBoundary) Entry() (keyspace.Term, bool) {
	return boundary.boundary.Entry()
}
func (boundary RootBoundary) OutcomeCount() int {
	return boundary.boundary.OutcomeCount()
}
func (boundary RootBoundary) OutcomeAt(index int) (OutcomeExit, bool) {
	exit, ok := boundary.boundary.OutcomeAt(index)
	return OutcomeExit{Outcome: exit.Outcome, Body: exit.Body, Kind: exit.Kind, Target: exit.Target}, ok
}
