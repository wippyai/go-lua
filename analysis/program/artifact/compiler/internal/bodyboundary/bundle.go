// Package bodyboundary owns the compiler's complete Body/Outcome and callable
// boundary publication. It accepts canonical Program input and retains all
// construction indexes privately until the parent freezes the planes.
package bodyboundary

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// Input is the exact canonical boundary for body publication. Values and the
// site-to-point relation are already issued by the parent; no callback or
// compiler authority crosses this package boundary.
type Input struct {
	Program   *program.Program
	ProgramID identity.ContentID
	Values    []programschema.Values
}

// Planes is the one canonical transfer from this child to publication.
type Planes struct {
	Bodies              []programschema.Body
	BodyEntries         []programschema.BodyEntry
	BodyRoots           []programschema.BodyRoot
	EntryBodyID         identity.ContentID
	Outcomes            []programschema.Outcome
	OutcomeReturnValues []programschema.OutcomeReturnValue
	OutcomePoints       []programschema.OutcomePoint
	FunctionBoundaries  []programschema.FunctionBoundary
	FunctionFormals     []programschema.FunctionFormal
	FunctionVarargs     []programschema.FunctionVararg
	FunctionCaptures    []programschema.FunctionCapture
}

// Bundle owns all canonical planes and private lookup indexes. Methods expose
// immutable construction reads only; callers cannot append or replace rows.
type Bundle struct {
	bodies                 []programschema.Body
	bodyEntries            []programschema.BodyEntry
	bodyRoots              []programschema.BodyRoot
	entryBodyID            identity.ContentID
	outcomes               []programschema.Outcome
	outcomeReturnValues    []programschema.OutcomeReturnValue
	outcomePoints          []programschema.OutcomePoint
	functionBoundaries     []programschema.FunctionBoundary
	functionFormals        []programschema.FunctionFormal
	functionVarargs        []programschema.FunctionVararg
	functionCaptures       []programschema.FunctionCapture
	functionIDsByTerm      map[keyspace.Term]identity.ContentID
	functionBoundaryByBody map[identity.ContentID]programschema.FunctionBoundary
	taken                  bool
}

func (bundle *Bundle) available() bool { return bundle != nil && !bundle.taken }

func (bundle *Bundle) Bodies() []programschema.Body {
	if !bundle.available() {
		return nil
	}
	return bundle.bodies
}
func (bundle *Bundle) BodyEntries() []programschema.BodyEntry {
	if !bundle.available() {
		return nil
	}
	return bundle.bodyEntries
}
func (bundle *Bundle) BodyRoots() []programschema.BodyRoot {
	if !bundle.available() {
		return nil
	}
	return bundle.bodyRoots
}
func (bundle *Bundle) EntryBodyID() identity.ContentID {
	if !bundle.available() {
		return identity.ContentID{}
	}
	return bundle.entryBodyID
}
func (bundle *Bundle) Outcomes() []programschema.Outcome {
	if !bundle.available() {
		return nil
	}
	return bundle.outcomes
}
func (bundle *Bundle) OutcomeReturnValues() []programschema.OutcomeReturnValue {
	if !bundle.available() {
		return nil
	}
	return bundle.outcomeReturnValues
}
func (bundle *Bundle) OutcomePoints() []programschema.OutcomePoint {
	if !bundle.available() {
		return nil
	}
	return bundle.outcomePoints
}
func (bundle *Bundle) FunctionBoundaries() []programschema.FunctionBoundary {
	if !bundle.available() {
		return nil
	}
	return bundle.functionBoundaries
}
func (bundle *Bundle) FunctionFormals() []programschema.FunctionFormal {
	if !bundle.available() {
		return nil
	}
	return bundle.functionFormals
}
func (bundle *Bundle) FunctionVarargs() []programschema.FunctionVararg {
	if !bundle.available() {
		return nil
	}
	return bundle.functionVarargs
}
func (bundle *Bundle) FunctionCaptures() []programschema.FunctionCapture {
	if !bundle.available() {
		return nil
	}
	return bundle.functionCaptures
}

func (bundle *Bundle) FunctionID(term keyspace.Term) (identity.ContentID, bool) {
	if !bundle.available() {
		return identity.ContentID{}, false
	}
	id, ok := bundle.functionIDsByTerm[term]
	return id, ok && id.Available()
}

func (bundle *Bundle) FunctionFormalAt(boundary programschema.FunctionBoundary, index int) (programschema.FunctionFormal, bool) {
	if !bundle.available() || !boundary.Available() || index < 0 || index >= boundary.FormalCount() {
		return programschema.FunctionFormal{}, false
	}
	offset, _, ok := boundary.FormalSpan()
	if !ok || uint64(offset)+uint64(index) >= uint64(len(bundle.functionFormals)) {
		return programschema.FunctionFormal{}, false
	}
	row := bundle.functionFormals[int(offset)+index]
	return row, row.Available()
}

func (bundle *Bundle) FunctionVararg(boundary programschema.FunctionBoundary) (programschema.FunctionVararg, bool) {
	if !bundle.available() || !boundary.Available() || !boundary.HasVararg() {
		return programschema.FunctionVararg{}, false
	}
	offset, count, ok := boundary.VarargSpan()
	if !ok || count != 1 || uint64(offset) >= uint64(len(bundle.functionVarargs)) {
		return programschema.FunctionVararg{}, false
	}
	row := bundle.functionVarargs[offset]
	return row, row.Available()
}

func (bundle *Bundle) FunctionCaptureAt(boundary programschema.FunctionBoundary, index int) (programschema.FunctionCapture, bool) {
	if !bundle.available() || !boundary.Available() || index < 0 || index >= boundary.CaptureCount() {
		return programschema.FunctionCapture{}, false
	}
	offset, _, ok := boundary.CaptureSpan()
	if !ok || uint64(offset)+uint64(index) >= uint64(len(bundle.functionCaptures)) {
		return programschema.FunctionCapture{}, false
	}
	row := bundle.functionCaptures[int(offset)+index]
	return row, row.Available() && row.InnerBodyID() == boundary.BodyID()
}

func (bundle *Bundle) FunctionBoundaryForBody(bodyID identity.ContentID) (programschema.FunctionBoundary, bool) {
	if !bundle.available() || !bodyID.Available() {
		return programschema.FunctionBoundary{}, false
	}
	row, ok := bundle.functionBoundaryByBody[bodyID]
	return row, ok && row.Available()
}

// TakeCanonicalPlanes transfers the only publication representation once.
func (bundle *Bundle) TakeCanonicalPlanes() (Planes, bool) {
	if bundle == nil || bundle.taken {
		return Planes{}, false
	}
	bundle.taken = true
	return Planes{
		Bodies: bundle.bodies, BodyEntries: bundle.bodyEntries, BodyRoots: bundle.bodyRoots,
		EntryBodyID: bundle.entryBodyID,
		Outcomes:    bundle.outcomes, OutcomeReturnValues: bundle.outcomeReturnValues, OutcomePoints: bundle.outcomePoints,
		FunctionBoundaries: bundle.functionBoundaries, FunctionFormals: bundle.functionFormals,
		FunctionVarargs: bundle.functionVarargs, FunctionCaptures: bundle.functionCaptures,
	}, true
}
