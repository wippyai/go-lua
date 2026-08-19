package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticcontracts "github.com/wippyai/go-lua/analysis/program/static/contracts"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	staticpubs "github.com/wippyai/go-lua/analysis/program/static/publications"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	staticsig "github.com/wippyai/go-lua/analysis/program/static/signatures"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

// Build seals authored static syntax into an immutable Component. It is a
// pure constructor: nothing is mutated across the pipeline. Each vertical
// validates its own authored rows and returns a sealed table by value, and
// the laws that span two verticals then run over those sealed values through
// the columns their owners publish. It accepts no inferred or domain
// resolution; those have no place in this owner.
//
// The stages are ordered by what they read, not by convenience:
//
//	census      the one cardinality column, sealed from the authored input
//	independent every vertical that needs nothing but the census
//	dependent   the verticals that consume an already-sealed sibling table
//	joint       the laws no single vertical can close alone
//	identity    the content digest over the sealed sections
func Build(input Input) (*Draft, error) {
	if !validCounts(input.Counts) {
		return nil, errors.New("program/static: inconsistent authored cardinality")
	}
	census, ok := staticCensus(input)
	if !ok {
		return nil, errors.New("program/static: inconsistent authored cardinality")
	}

	typeTable, err := statictypes.Build(input.Types, census)
	if err != nil {
		return nil, err
	}
	referenceTable, err := staticrefs.Build(input.References, census)
	if err != nil {
		return nil, err
	}
	declarationTable, err := staticdecl.Build(input.Declarations, census)
	if err != nil {
		return nil, err
	}
	signatureTable, err := staticsig.Build(input.Signatures, census)
	if err != nil {
		return nil, err
	}
	contractTable, err := staticcontracts.Build(input.Contracts, census)
	if err != nil {
		return nil, err
	}
	operatorTable, err := staticoperators.Build(input.Operators, census)
	if err != nil {
		return nil, err
	}

	// Publications admits only a reference disposition References published;
	// Operands admits a runtime type target only on what Types and References
	// published. Both take those tables as sealed values.
	publicationTable, err := staticpubs.Build(input.Publications, census, referenceTable)
	if err != nil {
		return nil, err
	}
	operandTable, err := staticoperands.Build(input.Operands, census, typeTable, referenceTable)
	if err != nil {
		return nil, err
	}

	component := &Component{
		census:       census,
		types:        typeTable,
		references:   referenceTable,
		declarations: declarationTable,
		signatures:   signatureTable,
		contracts:    contractTable,
		operators:    operatorTable,
		operands:     operandTable,
		publications: publicationTable,
	}
	if !interfaceMethodScopes(component) {
		return nil, errors.New("program/static: interface method signature scope mismatch")
	}
	if !completeTypeParamOwnership(component, census) {
		return nil, errors.New("program/static: incomplete or multiply-owned type parameter")
	}
	localProof, ok := localForest(component)
	if !ok {
		return nil, errors.New("program/static: cyclic, shared, or incomplete static declaration child")
	}

	component.contentID = contentID(component)
	if !component.contentID.Available() {
		return nil, errors.New("program/static: unavailable content identity")
	}
	return &Draft{state: &draftState{
		component:        component,
		localContainment: localProof,
		phase:            draftOpen,
	}}, nil
}

// Finalizer claims the authored component for the single owner-defined
// publication transaction. The returned capability exposes only lifecycle-
// bound View and LocalContainment validation surfaces; Commit or Abort must
// terminate it before another copy can act.
func (draft *Draft) Finalizer() (Finalizer, error) {
	if draft == nil || draft.state == nil {
		return Finalizer{}, errors.New("program/static: invalid draft finalizer")
	}
	state := draft.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftOpen || state.component == nil || !state.component.contentID.Available() {
		return Finalizer{}, errors.New("program/static: draft already finalized")
	}
	state.phase = draftClaimed
	return Finalizer{state: state}, nil
}

// View returns the immutable authored Static surface used by the coordinating
// finalizer. It contains no mutation or publication operation.
func (finalizer Finalizer) View() View {
	if finalizer.state == nil {
		return View{}
	}
	return View{state: finalizer.state}
}

// Commit consumes the claimed finalizer exactly once. The input is checked
// against this draft's authored denominators before publication. Both a
// valid and an invalid input are terminal: construction views and copied
// finalizers expire before the result is returned, and no input data is
// retained in Component, ContentID, or Cold.
func (finalizer Finalizer) Commit(input CommitInput) (*Component, error) {
	if finalizer.state == nil {
		return nil, errors.New("program/static: invalid finalizer commit")
	}
	state := finalizer.state
	state.mu.Lock()
	if state.phase != draftClaimed || state.component == nil {
		state.mu.Unlock()
		return nil, errors.New("program/static: finalizer is terminal")
	}
	component := state.component
	// Close the lifecycle before validating caller input. This makes a
	// rejected input terminal as well as a successful one, and expires every
	// copied construction view at the same state transition.
	state.component = nil
	state.localContainment = nil
	state.phase = draftCommitted
	state.mu.Unlock()

	if err := validateCommitInput(component, input); err != nil {
		return nil, err
	}
	return component, nil
}

func validateCommitInput(component *Component, input CommitInput) error {
	if component == nil {
		return errors.New("program/static: missing authored component")
	}
	if !validCommitTerms(input.TypeOf, keyspace.FamilyTypeOf, component.operators.Count(keyspace.FamilyTypeOf)) {
		return errors.New("program/static: invalid TypeOf input")
	}
	if !validCommitTerms(input.Annotations, keyspace.FamilyAnnotation, component.operands.Count(keyspace.FamilyAnnotation)) {
		return errors.New("program/static: invalid Annotation input")
	}
	if !validCommitTerms(input.Publications, keyspace.FamilyTypePublication, component.publications.Count()) {
		return errors.New("program/static: invalid Publication input")
	}
	return nil
}

func validCommitTerms(terms []keyspace.Term, family keyspace.Family, count int) bool {
	if !countEquals(uint32(count), len(terms)) {
		return false
	}
	for index, term := range terms {
		if term != keyspace.MakeTerm(family, uint32(index+1)) {
			return false
		}
	}
	return true
}

// Abort terminates the claimed publication without publishing a component.
// It returns an error when another copied capability has already won the
// terminal action, making lifecycle races observable to the coordinator.
func (finalizer Finalizer) Abort() error {
	if finalizer.state == nil {
		return errors.New("program/static: invalid finalizer abort")
	}
	state := finalizer.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftClaimed || state.component == nil {
		return errors.New("program/static: finalizer is terminal")
	}
	state.component = nil
	state.localContainment = nil
	state.phase = draftAborted
	return nil
}

func validCounts(counts [keyspace.FamilyCount]uint32) bool {
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return false
	}
	for _, count := range counts {
		if count > keyspace.MaxTermOrdinal {
			return false
		}
	}
	return true
}
