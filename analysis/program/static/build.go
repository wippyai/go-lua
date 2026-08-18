package static

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Build validates and compacts authored static syntax, including the
// parser/binder-produced TypeRef disposition. It accepts no inferred or domain
// resolution; those have no place in this owner.
func Build(input Input) (*Draft, error) {
	if !validCounts(input.Counts) {
		return nil, errors.New("program/static: inconsistent authored cardinality")
	}
	census, ok := staticCensus(input)
	if !ok {
		return nil, errors.New("program/static: inconsistent authored cardinality")
	}
	component := &Component{census: census, types: typeStore{
		primitive: append([]Primitive(nil), input.Types.Primitive...),
		literal:   append([]Literal(nil), input.Types.Literal...),
		optional:  append([]Optional(nil), input.Types.Optional...),
		array:     append([]Array(nil), input.Types.Array...),
		mapType:   append([]Map(nil), input.Types.Map...),
		field:     append([]Field(nil), input.Types.Field...),
	}}
	if err := compactReferences(component, census, input.References); err != nil {
		return nil, err
	}
	if err := compactTypes(component, census, input.Types); err != nil {
		return nil, err
	}
	if err := compactDeclarations(component, census, input.Declarations); err != nil {
		return nil, err
	}
	if err := compactSignatures(component, census, input.Signatures); err != nil {
		return nil, err
	}
	if err := compactContracts(component, census, input.Contracts); err != nil {
		return nil, err
	}
	if !completeTypeParamOwnership(component, census) {
		return nil, errors.New("program/static: incomplete or multiply-owned type parameter")
	}
	if err := compactOperators(component, census, input.Operators); err != nil {
		return nil, err
	}
	if err := compactOperands(component, census, input.Operands); err != nil {
		return nil, err
	}
	if err := compactPublications(component, census, input.Publications); err != nil {
		return nil, err
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
	if !validCommitTerms(input.TypeOf, keyspace.FamilyTypeOf, len(component.operators.typeOf)) {
		return errors.New("program/static: invalid TypeOf input")
	}
	if !validCommitTerms(input.Annotations, keyspace.FamilyAnnotation, len(component.operands.annotations)) {
		return errors.New("program/static: invalid Annotation input")
	}
	if !validCommitTerms(input.Publications, keyspace.FamilyTypePublication, len(component.publications)) {
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

func appendTerms(pool *[]keyspace.Term, values []keyspace.Term) (poolRange, bool) {
	start := len(*pool)
	if uint64(start)+uint64(len(values)) > uint64(math.MaxUint32) {
		return poolRange{}, false
	}
	*pool = append(*pool, values...)
	return poolRange{Start: uint32(start), End: uint32(len(*pool))}, true
}

func validTerm(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal != 0 && ordinal <= counts[family]
}

func hasFamily(counts [keyspace.FamilyCount]uint32, term keyspace.Term, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 && keyspace.TermOrdinal(term) <= counts[family]
}
