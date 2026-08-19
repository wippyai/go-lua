// Package contracts owns the authored static sidecars attached to Flow's
// Function and Call terms.
//
// The package is independent of the enclosing Static component. It retains
// only static syntax: Flow remains the sole owner of callable evaluation,
// formals, bodies, callee selection, and runtime argument occurrences. Both
// relations are dense by their canonical Flow ordinal, and an empty row is
// meaningful - it distinguishes an omitted return clause from an explicit
// known-empty one without inventing a second Function or Call identity.
package contracts

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// FunctionContract is the authored static sidecar of one Flow Function.
type FunctionContract struct {
	TypeParams   []keyspace.Term
	ReturnsKnown bool
	Returns      []keyspace.Term
}

// CallContract contains only authored static type arguments. Runtime actual
// occurrences remain owned by Flow's Values relation.
type CallContract struct{ TypeArguments []keyspace.Term }

// Input is dense by canonical Function and Call ordinal.
type Input struct {
	Function []FunctionContract
	Call     []CallContract
}

// FunctionContractRow is the sealed form of a FunctionContract.
type FunctionContractRow struct {
	TypeParams   rows.Span
	ReturnsKnown bool
	Returns      rows.Span
}

// CallContractRow is the sealed form of a CallContract.
type CallContractRow struct{ TypeArguments rows.Span }

// Table is the sealed immutable contract relation set.
//
// callTypeArguments is the sealed width of the call type-argument column: the
// length of the term segment the call windows cover. Build assigns it once so
// the denominator is read rather than re-walked per call.
//
// callArgumentID is the immutable per-call type-argument column identity.
// Authored terms live in the shared column; this is only their stable content
// identity, and it is authored-derived rather than a query derivative.
type Table struct {
	function          rows.Table[FunctionContractRow]
	call              rows.Table[CallContractRow]
	callArgumentID    rows.Table[identity.ContentID]
	terms             rows.Pool[keyspace.Term]
	callTypeArguments uint32
}

// Count reports the sealed row denominator of one contract family.
func (table Table) Count(family keyspace.Family) int {
	switch family {
	case keyspace.FamilyFunction:
		return table.function.Count()
	case keyspace.FamilyCall:
		return table.call.Count()
	default:
		return 0
	}
}

// CallTypeArgumentWidth is the sealed total width of the call type-argument
// column. It is a denominator the schema publishes, read rather than recounted.
func (table Table) CallTypeArgumentWidth() int { return int(table.callTypeArguments) }

// VisitFunctionTypeParams emits every Function-contract-owned type parameter
// claim against the Flow Function that owns it, the third column of the one
// TypeParam ownership law.
func (table Table) VisitFunctionTypeParams(claim func(owner, param keyspace.Term) bool) bool {
	if claim == nil {
		return false
	}
	for owner, row := range table.function.Terms() {
		for _, param := range table.terms.All(row.TypeParams) {
			if !claim(owner, param) {
				return false
			}
		}
	}
	return true
}

// VisitContainment emits the authored children this vertical hangs on opaque
// Flow parents. It inspects no body, value, or control flow. attachReturn
// carries a return-position child so the enclosing owner can record direct
// return evidence for the bound-assertion law.
func (table Table) VisitContainment(attach, attachReturn func(parent, child keyspace.Term) bool) bool {
	if attach == nil || attachReturn == nil {
		return false
	}
	for parent, row := range table.function.Terms() {
		for _, child := range table.terms.All(row.Returns) {
			if !attachReturn(parent, child) {
				return false
			}
		}
	}
	for parent, row := range table.call.Terms() {
		for _, child := range table.terms.All(row.TypeArguments) {
			if !attach(parent, child) {
				return false
			}
		}
	}
	return true
}
