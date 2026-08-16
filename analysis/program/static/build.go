package static

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// Build validates and compacts authored static syntax, including the
// parser/binder-produced TypeRef disposition. It accepts no inferred or domain
// resolution; those have no place in this owner.
func Build(input Input) (*Draft, error) {
	if !validCounts(input.Counts) || !matchingCounts(input) {
		return nil, errors.New("program/static: inconsistent authored cardinality")
	}
	component := &Component{types: typeStore{
		primitive: append([]Primitive(nil), input.Types.Primitive...),
		literal:   append([]Literal(nil), input.Types.Literal...),
		optional:  append([]Optional(nil), input.Types.Optional...),
		array:     append([]Array(nil), input.Types.Array...),
		mapType:   append([]Map(nil), input.Types.Map...),
		field:     append([]Field(nil), input.Types.Field...),
	}}
	if err := compactReferences(component, input.Counts, input.References); err != nil {
		return nil, err
	}
	if err := compactTypes(component, input.Counts, input.Types); err != nil {
		return nil, err
	}
	if err := compactDeclarations(component, input.Counts, input.Declarations); err != nil {
		return nil, err
	}
	if err := compactSignatures(component, input.Counts, input.Signatures); err != nil {
		return nil, err
	}
	if err := compactContracts(component, input.Counts, input.Contracts); err != nil {
		return nil, err
	}
	if !completeTypeParamOwnership(component, input.Counts) {
		return nil, errors.New("program/static: incomplete or multiply-owned type parameter")
	}
	if err := compactOperators(component, input.Counts, input.Operators); err != nil {
		return nil, err
	}
	if err := compactOperands(component, input.Counts, input.Operands); err != nil {
		return nil, err
	}
	if err := compactPublications(component, input.Counts, input.Publications); err != nil {
		return nil, err
	}
	localProof, ok := localForest(component, input.Counts)
	if !ok {
		return nil, errors.New("program/static: cyclic, shared, or incomplete static declaration child")
	}
	component.staticTypes = buildStaticTypeIndex(component)
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

// Commit consumes the claimed finalizer exactly once. The receipt is checked
// against this draft's authored denominators before publication. Both a
// valid and an invalid receipt are terminal: construction views and copied
// finalizers expire before the result is returned, and no receipt data is
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
	// rejected receipt terminal as well as a successful one, and expires every
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
		return errors.New("program/static: invalid TypeOf receipt")
	}
	if !validCommitTerms(input.Annotations, keyspace.FamilyAnnotation, len(component.operands.annotations)) {
		return errors.New("program/static: invalid Annotation receipt")
	}
	if !validCommitTerms(input.Publications, keyspace.FamilyTypePublication, len(component.publications)) {
		return errors.New("program/static: invalid Publication receipt")
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

func compactReferences(component *Component, counts [keyspace.FamilyCount]uint32, input ReferencesInput) error {
	store := &component.references
	for _, row := range input.TypeRef {
		if !validTypeRef(counts, row) {
			return errors.New("program/static: invalid type reference")
		}
		source, ok := appendKeys(&store.source, row.Source)
		if !ok {
			return errors.New("program/static: oversized type reference source")
		}
		canonical, ok := appendKeys(&store.canonical, row.Canonical)
		if !ok {
			return errors.New("program/static: oversized type reference canonical path")
		}
		store.rows = append(store.rows, typeRefRow{
			resolution: row.Resolution,
			target:     row.Target,
			root:       row.Root,
			source:     source,
			canonical:  canonical,
		})
	}
	return nil
}

func validTypeRef(counts [keyspace.FamilyCount]uint32, row TypeRef) bool {
	if !validKeys(row.Source, 1) || !validTypeRefRoot(counts, row.Source, row.Root) {
		return false
	}
	switch row.Resolution {
	case TypeRefUnresolved:
		return row.Target == 0 && len(row.Canonical) == 0
	case TypeRefDeclaration:
		return staticrole.TypeReferenceTarget(counts, row.Target) && len(row.Canonical) == 0
	case TypeRefCanonicalPath:
		return row.Target == 0 && validKeys(row.Canonical, 1)
	default:
		return false
	}
}

func validTypeRefRoot(counts [keyspace.FamilyCount]uint32, source []keyspace.Key, root keyspace.Term) bool {
	if len(source) == 1 {
		return root == 0
	}
	return hasFamily(counts, root, keyspace.FamilyCell)
}

func validKeys(keys []keyspace.Key, minimum int) bool {
	if len(keys) < minimum {
		return false
	}
	for _, key := range keys {
		if key == 0 {
			return false
		}
	}
	return true
}

func countEquals(count uint32, length int) bool {
	return length >= 0 && uint64(length) <= uint64(keyspace.MaxTermOrdinal) && uint64(count) == uint64(length)
}

func appendTerms(pool *[]keyspace.Term, values []keyspace.Term) (poolRange, bool) {
	start := len(*pool)
	if uint64(start)+uint64(len(values)) > uint64(math.MaxUint32) {
		return poolRange{}, false
	}
	*pool = append(*pool, values...)
	return poolRange{Start: uint32(start), End: uint32(len(*pool))}, true
}

func appendKeys(pool *[]keyspace.Key, values []keyspace.Key) (poolRange, bool) {
	start := len(*pool)
	if uint64(start)+uint64(len(values)) > uint64(math.MaxUint32) {
		return poolRange{}, false
	}
	*pool = append(*pool, values...)
	return poolRange{Start: uint32(start), End: uint32(len(*pool))}, true
}

// writeReferencesContent owns authored spelling and binder disposition. The
// range offsets are implementation-only; the encoded paths are exact.
func writeReferencesContent(writer *framing.Writer, store referenceStore) error {
	if err := writer.Count(uint64(len(store.rows))); err != nil {
		return err
	}
	for _, row := range store.rows {
		if err := writer.Uint(uint64(row.resolution)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.target)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.root)); err != nil {
			return err
		}
		if err := writeReferenceKeysContent(writer, store.source[row.source.Start:row.source.End]); err != nil {
			return err
		}
		if err := writeReferenceKeysContent(writer, store.canonical[row.canonical.Start:row.canonical.End]); err != nil {
			return err
		}
	}
	return nil
}

func writeReferenceKeysContent(writer *framing.Writer, keys []keyspace.Key) error {
	if err := writer.Count(uint64(len(keys))); err != nil {
		return err
	}
	for _, key := range keys {
		if err := writer.Uint(uint64(key)); err != nil {
			return err
		}
	}
	return nil
}

func validTerm(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal != 0 && ordinal <= counts[family]
}

func hasFamily(counts [keyspace.FamilyCount]uint32, term keyspace.Term, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 && keyspace.TermOrdinal(term) <= counts[family]
}
