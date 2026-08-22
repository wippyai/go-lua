// Package diagnostic owns the one-shot construction of the Program diagnostic
// publication. It consumes only the exact immutable rows and owner bundles
// issued by sibling compiler phases; it never imports the parent compiler.
package diagnostic

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/allocation"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/bodyboundary"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
	programdiagnostic "github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
)

// Input is the complete immutable construction boundary for diagnostic rows.
// The child receives canonical rows from earlier compiler phases and exact
// owner bundles for body boundaries and allocations. No parent-private draft
// or resolver callback crosses this boundary.
type Input struct {
	Program       *program.Program
	Values        []programschema.Values
	ValuesMembers []programschema.ValuesMember
	Calls         []programschema.Call
	CallArguments []programschema.CallArgument
	BodyBoundary  *bodyboundary.Bundle
	Allocations   *allocation.Bundle
}

func (input Input) Available() bool {
	return input.Program != nil && input.Program.Available()
}

type callArgumentSource struct {
	term  keyspace.Term
	index int
}

type compiler struct {
	input        Input
	bodyBoundary *bodyboundary.Bundle
	allocations  *allocation.Bundle
	calls        []programschema.Call

	diagnosticObservations    []programdiagnostic.DiagnosticObservation
	diagnosticEvidence        []programdiagnostic.DiagnosticEvidence
	diagnosticPaths           []programdiagnostic.DiagnosticPath
	diagnosticObservationByID map[identity.ContentID]int
	diagnosticEvidenceScratch map[identity.ContentID]struct{}
	diagnosticPointScratch    []identity.ContentID

	branchScopeRewriteComputed   bool
	branchScopeRewriteWellFormed bool
	branchScopeRewriteOwners     map[keyspace.Term]struct{}

	callArgumentSources map[identity.ContentID]callArgumentSource
}

// pointPaths copies one owner-issued Causal span into reusable construction
// scratch. Diagnostic identity and publication consume the slice before the
// next call; no borrowed owner storage or per-observation allocation escapes.
func (compiler *compiler) pointPaths(paths causal.SitePointPaths) ([]identity.ContentID, bool) {
	if compiler == nil || !paths.Available() {
		return nil, false
	}
	compiler.diagnosticPointScratch = compiler.diagnosticPointScratch[:0]
	for index := 0; index < paths.Count(); index++ {
		point, ok := paths.At(index)
		if !ok {
			return nil, false
		}
		compiler.diagnosticPointScratch = append(compiler.diagnosticPointScratch, point)
	}
	return compiler.diagnosticPointScratch, true
}

// Compile admits all diagnostic observations and returns the one canonical
// publication. The returned publication owns the only slices retained by the
// parent; all indexes and scratch state die with this child transaction.
func Compile(input Input) (programdiagnostic.Publication, programconstruction.Fault) {
	if input.Program == nil || !input.Program.Available() ||
		input.BodyBoundary == nil || input.Allocations == nil {
		return programdiagnostic.Publication{}, programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticInvalidInput, -1, -1)
	}
	compiler := &compiler{
		input:                     input,
		bodyBoundary:              input.BodyBoundary,
		allocations:               input.Allocations,
		calls:                     input.Calls,
		diagnosticObservationByID: make(map[identity.ContentID]int),
	}
	if !compiler.indexCallArgumentSources() {
		return programdiagnostic.Publication{}, programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticCall, -1, -1)
	}
	if fault := compiler.copyDiagnosticObservationsFailure(); fault.Available() {
		return programdiagnostic.Publication{}, fault
	}
	identity.SortByContentID(compiler.diagnosticObservations, func(row programdiagnostic.DiagnosticObservation) identity.ContentID { return row.ID() })
	for index := 1; index < len(compiler.diagnosticObservations); index++ {
		if compiler.diagnosticObservations[index-1].ID() == compiler.diagnosticObservations[index].ID() {
			return programdiagnostic.Publication{}, programconstruction.New(programcatalog.DiagnosticObservation(), programconstruction.IssueDiagnosticDuplicate, index, -1)
		}
	}
	return programdiagnostic.Publication{
		DiagnosticObservations: compiler.diagnosticObservations,
		DiagnosticEvidence:     compiler.diagnosticEvidence,
		DiagnosticPaths:        compiler.diagnosticPaths,
	}, programconstruction.Fault{}
}

func (compiler *compiler) indexCallArgumentSources() bool {
	if compiler == nil || compiler.input.Program == nil {
		return false
	}
	compiler.callArgumentSources = make(map[identity.ContentID]callArgumentSource)
	view := compiler.input.Program.Flow()
	calls := view.Authored().Calls()
	values := view.Authored().Values()
	for index := 0; index < calls.Count(); index++ {
		term, termOK := calls.At(index)
		_, _, _, actuals, relationOK := calls.Get(term)
		width, widthOK := values.Len(actuals)
		if !termOK || !relationOK || !widthOK || width < 0 {
			return false
		}
		for position := 0; position < width; position++ {
			member, memberOK := values.Member(actuals, position)
			argumentID, argumentOK := view.CallArgumentID(term, position)
			if !memberOK || !argumentOK || !argumentID.Available() {
				return false
			}
			if prior, duplicate := compiler.callArgumentSources[argumentID]; duplicate && (prior.term != member || prior.index != index) {
				return false
			}
			compiler.callArgumentSources[argumentID] = callArgumentSource{term: member, index: index}
		}
	}
	return true
}

func (compiler *compiler) valueRowForTerm(term keyspace.Term) (programschema.Values, bool) {
	if compiler == nil || keyspace.TermFamily(term) != keyspace.FamilyValues || keyspace.TermOrdinal(term) == 0 {
		return programschema.Values{}, false
	}
	index := int(keyspace.TermOrdinal(term)) - 1
	if index < 0 || index >= len(compiler.input.Values) {
		return programschema.Values{}, false
	}
	row := compiler.input.Values[index]
	return row, row.Available()
}

func (compiler *compiler) valueMemberAt(row programschema.Values, index int) (programschema.ValuesMember, bool) {
	if compiler == nil || !row.Available() || index < 0 {
		return programschema.ValuesMember{}, false
	}
	offset, count, spanOK := row.MemberSpan()
	if !spanOK || index >= int(count) || uint64(offset)+uint64(index) >= uint64(len(compiler.input.ValuesMembers)) {
		return programschema.ValuesMember{}, false
	}
	member := compiler.input.ValuesMembers[int(offset)+index]
	return member, member.Available()
}

func (compiler *compiler) callArgumentSource(id identity.ContentID) (keyspace.Term, int, bool) {
	if compiler == nil || !id.Available() {
		return 0, -1, false
	}
	source, ok := compiler.callArgumentSources[id]
	return source.term, source.index, ok && source.term != 0 && source.index >= 0
}
