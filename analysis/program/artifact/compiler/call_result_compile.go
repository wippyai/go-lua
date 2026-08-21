package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// callResultGeometryWitness is the compiler-local, target-independent witness
// for one authored Call output. It is deliberately not retained in Program:
// CallResult is the only sealed projection, and it reuses the Call identity.
type callResultGeometryWitness struct {
	values       identity.ContentID
	value        identity.ContentID
	tail         identity.ContentID
	position     uint32
	form         programschema.CallResultForm
	multiplicity programschema.CallResultMultiplicity
	count        uint32
}

// callResultGeometry resolves the one authored output context of a Call. A
// statement Call is intentionally resultless: Lua discards its output and no
// Value/Values coordinate is issued. The index is built once from the authored
// Values relation, so each subsequent Call lookup is O(1) rather than rescanning
// every Values row and member. The target outcome/result ordinal is deliberately
// supplied later by Boundary and therefore never guessed here.
func (compiler *compiler) callResultGeometry(term keyspace.Term, callID identity.ContentID) (programschema.CallResult, bool, bool) {
	if compiler == nil || compiler.input == nil || !callID.Available() || keyspace.TermFamily(term) != keyspace.FamilyCall || keyspace.TermOrdinal(term) == 0 {
		return programschema.CallResult{}, false, false
	}
	if !compiler.callResultGeometryComputed {
		compiler.callResultGeometryByTerm, compiler.callResultGeometryOK = compiler.buildCallResultGeometryIndex()
		compiler.callResultGeometryComputed = true
	}
	if !compiler.callResultGeometryOK {
		return programschema.CallResult{}, false, false
	}
	witness, found := compiler.callResultGeometryByTerm[term]
	if !found {
		return programschema.CallResult{}, false, true
	}
	result, resultOK := programschema.NewCallResultWithMultiplicity(callID, witness.values, witness.value, witness.tail, witness.position, witness.form, witness.multiplicity, witness.count)
	return result, true, resultOK
}

// buildCallResultGeometryIndex makes the one compiler-local inverse from an
// authored Call term to its exact Values coordinate. Every Values row and
// member is visited once. Duplicate Call terms, or a malformed Call term that
// appears in Values without a canonical authored Call row, fail the whole
// geometry plane closed.
func (compiler *compiler) buildCallResultGeometryIndex() (map[keyspace.Term]callResultGeometryWitness, bool) {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() {
		return nil, false
	}
	flowView := compiler.input.Flow()
	index := make(map[keyspace.Term]callResultGeometryWitness)
	ok := flowView.VisitCallResultGeometry(func(geometry flow.CallResultGeometry) bool {
		if _, duplicate := index[geometry.Call]; duplicate {
			return false
		}
		index[geometry.Call] = callResultGeometryWitness{
			values: geometry.Values, value: geometry.Value, tail: geometry.Tail,
			position: geometry.Position, form: geometry.Form, multiplicity: geometry.Multiplicity, count: geometry.Count,
		}
		return true
	})
	return index, ok
}
