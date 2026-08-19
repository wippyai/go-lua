package static

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

type ClassKind uint8

const (
	ClassAnyValue ClassKind = iota
	ClassConcrete
	ClassOpaque
	// ClassDerived is an immutable normalized union descriptor which was not
	// one of the singleton rows admitted during Static sealing.  Derived
	// classes carry no new owner or type authority: their descriptor is an
	// owner-fenced value assembled from the sealed Runtime atoms.
	ClassDerived
)

type Class struct {
	owner      *ClassSet
	index      uint32
	descriptor *classDescriptor
}

type classRow struct {
	kind    ClassKind
	encoded []byte
	input   typeauthority.RuntimeInput
	inner   typeauthority.RuntimeInner // construction-only; zero for opaque/AnyValue.
}

// ClassSet is the Link-scoped Pack value classification. Sealed singleton
// rows are complemented by immutable owner-fenced lower-set descriptors. The
// finite Runtime/opaque atom universe bounds rank and descriptor width; every
// relational judgment is recomputed by a finite scan. It owns no Pack
// values and derives declaration classes directly from Target.
type ClassSet struct {
	authority        *Authority
	id               identity.ContentID
	rows             []classRow // zero is AnyValue
	byBytes          map[string]Class
	byStatic         map[uint32]Class
	byTarget         map[vocabulary.Type]Class
	nilable          []bool
	runtimeKinds     []runtimekind.Set // sealed Class index -> may-runtime-kind mask.
	runtimeAtomKinds []runtimekind.Set // sealed Runtime atom index -> may-runtime-kind mask.
	ranks            []uint64
	universe         []uint64             // structurally ordered finite observation universe P.
	universeIDs      []identity.ContentID // portable identity parallel to universe.
	universeID       identity.ContentID   // identity of the ordered universe as a whole.
	unknownAtom      uint64               // preferred representative of universal principals.

	// Direct atom-to-position tables over the dense universe, indexed by the
	// Runtime row and the opaque Class row respectively; -1 is absent.
	runtimeAtomPositions []int32
	opaqueAtomPositions  []int32

	// The sealed principal relation. principals packs one coverage row D(a)
	// per universe position, principalSizes is its popcount, and opaqueMask
	// marks the opaque positions.
	coverageStride int
	principals     []uint64
	principalSizes []uint32
	opaqueMask     []uint64

	runtime     *typeauthority.Runtime
	descriptors []classDescriptor
	// coverageIndex addresses sealed descriptors by coverage hash. A hot join
	// probes it without materializing its result.
	coverageIndex map[uint64][]uint32
	nil           Class
	identities    []identity.ContentID
}

func sealClassSet(authority *Authority) (*ClassSet, *typeauthority.Runtime, error) {
	set := &ClassSet{authority: authority, rows: []classRow{{kind: ClassAnyValue}},
		byBytes: make(map[string]Class), byStatic: make(map[uint32]Class),
		byTarget: make(map[vocabulary.Type]Class)}
	nilClass, err := set.addConcrete(typ.Nil)
	if err != nil {
		return nil, nil, err
	}
	set.nil = nilClass
	for index := 2; index < len(authority.results); index++ {
		row := authority.results[index]
		switch row.kind {
		case KindClosed:
			decoded, decodeErr := typ.DecodeCanonicalStructural(context.Background(), row.closed)
			if decodeErr != nil {
				return nil, nil, decodeErr
			}
			class, addErr := set.addConcreteInput(decoded, row.runtime)
			if addErr != nil {
				return nil, nil, addErr
			}
			set.byStatic[uint32(index)] = class
		case KindSymbolic:
			if row.symbolic.reason == ReasonOpenFormal && row.symbolic.reference.Valid() {
				materialized, found := authority.types.Resolve(row.symbolic.reference)
				input, closed := authority.types.RuntimeInputForType(materialized)
				if found && closed {
					class, addErr := set.addConcreteInput(materialized, input)
					if addErr != nil {
						return nil, nil, fmt.Errorf("static: materialize open class: %w", addErr)
					}
					set.byStatic[uint32(index)] = class
					continue
				}
			}
			h := sha256.New()
			writeSymbolic(h, row.symbolic)
			ordinal, ordinalErr := denseOrdinal(len(set.rows))
			if ordinalErr != nil {
				return nil, nil, fmt.Errorf("static: opaque class handle: %w", ordinalErr)
			}
			class := Class{owner: set, index: ordinal}
			set.rows = append(set.rows, classRow{kind: ClassOpaque, encoded: h.Sum(nil)})
			set.byStatic[uint32(index)] = class
		}
	}
	contract := authority.target
	if contract == nil || !contract.ContentID().Available() {
		return nil, nil, errors.New("static: Link target unavailable")
	}
	seenOperations := make(map[vocabulary.Operation]struct{}, contract.Operations.OperationCount())
	for index := 0; index < contract.Operations.OperationCount(); index++ {
		operation, valid := contract.Operations.OperationAt(index)
		if !valid {
			return nil, nil, errors.New("static: malformed operation family")
		}
		// Target owns the complete declaration denominator.  Static classifies
		// every callable endpoint directly; Link application selection is a
		// later Rule concern and is deliberately absent here.
		seenOperations[operation] = struct{}{}
		if err := set.addOperation(contract, operation); err != nil {
			return nil, nil, err
		}
	}
	runtime, err := set.sealRuntime()
	if err != nil {
		return nil, nil, err
	}
	set.runtime = runtime
	if err := set.sealClassOrder(runtime); err != nil {
		return nil, nil, err
	}
	if err := set.sealDescriptors(runtime); err != nil {
		return nil, nil, err
	}
	authority.id = authority.contentID()
	if !authority.id.Available() {
		return nil, nil, errors.New("static: unavailable content identity")
	}
	set.id = set.contentID(runtime, seenOperations)
	if !set.id.Available() {
		return nil, nil, errors.New("static: unavailable ClassSet identity")
	}
	if err := set.finalizeDescriptorIdentities(); err != nil {
		return nil, nil, err
	}
	if err := set.sealClassIdentities(); err != nil {
		return nil, nil, err
	}
	if err := set.sealRuntimeKinds(); err != nil {
		return nil, nil, err
	}
	if err := set.sealRuntimeAtomKinds(); err != nil {
		return nil, nil, err
	}
	set.byBytes = nil
	for index := range set.rows {
		set.rows[index].encoded = nil
		set.rows[index].input = typeauthority.RuntimeInput{}
		set.rows[index].inner = typeauthority.RuntimeInner{}
	}
	return set, runtime, nil
}

func (s *ClassSet) ContentID() identity.ContentID {
	if s == nil {
		return identity.ContentID{}
	}
	return s.id
}

// Owns authenticates both sealed singleton rows and immutable derived
// descriptors. Pack may carry a derived class without importing or extending
// Static's row authority.
func (s *ClassSet) Owns(class Class) bool { return s.owns(class) }

func (s *ClassSet) AnyValue() Class { return Class{owner: s} }
func (s *ClassSet) Nil() Class {
	if s == nil {
		return Class{}
	}
	return s.nil
}
func (s *ClassSet) Kind(class Class) (ClassKind, bool) {
	if !s.owns(class) {
		return ClassAnyValue, false
	}
	if class.descriptor != nil {
		return ClassDerived, true
	}
	return s.rows[class.index].kind, true
}
func (s *ClassSet) ClassForStatic(value Value) (Class, bool) {
	if s == nil || value.owner != s.authority {
		return Class{}, false
	}
	if value.isDerived() && value.class.owner == s {
		return value.class, true
	}
	if value.index == 1 {
		return s.AnyValue(), true
	}
	if value.index == 0 {
		return Class{}, false
	}
	class, ok := s.byStatic[value.index]
	return class, ok
}

// ClassForTarget projects a Target-owned type handle. The Contract pointer is
// part of the capability: the raw ordinal alone is not an owner identity and
// must not admit an equal-numbered type from another sealed Target.
func (s *ClassSet) ClassForTarget(contract *target.Contract, value vocabulary.Type) (Class, bool) {
	if s == nil || s.authority == nil || contract == nil || contract != s.authority.target {
		return Class{}, false
	}
	class, ok := s.byTarget[value]
	return class, ok
}

func (s *ClassSet) Equal(left, right Class) bool {
	if !s.owns(left) || !s.owns(right) {
		return false
	}
	if left.descriptor == nil && right.descriptor == nil && left.index == right.index {
		return true
	}
	leftCoverage, leftOK := s.classCoverage(left)
	rightCoverage, rightOK := s.classCoverage(right)
	return leftOK && rightOK && coverageEqual(leftCoverage, rightCoverage)
}

// Compare returns the exact owner-local semantic order of two classes. It is
// the coverage order rather than a truncated fingerprint, so consumers can
// order derived classes without turning a hash into equality.
func (s *ClassSet) Compare(left, right Class) int {
	if !s.owns(left) || !s.owns(right) {
		return 0
	}
	leftCoverage, leftOK := s.classCoverage(left)
	rightCoverage, rightOK := s.classCoverage(right)
	if !leftOK || !rightOK {
		return 0
	}
	return coverageCompare(leftCoverage, rightCoverage)
}

func (s *ClassSet) LessOrEq(left, right Class) bool {
	if !s.owns(left) || !s.owns(right) {
		return false
	}
	if left.descriptor == nil && right.descriptor == nil && left.index == right.index {
		return true
	}
	leftCoverage, leftOK := s.classCoverage(left)
	rightCoverage, rightOK := s.classCoverage(right)
	return leftOK && rightOK && coverageSubset(leftCoverage, rightCoverage)
}

// Join is the coverage union. A recurrent widening step whose result is
// already a sealed row is answered by bitwise containment and one indexed
// probe; only a coverage no sealed row states constructs a derived value.
func (s *ClassSet) Join(left, right Class) Class {
	if !s.owns(left) || !s.owns(right) {
		panic("static: foreign Pack class")
	}
	leftCoverage, leftOK := s.classCoverage(left)
	rightCoverage, rightOK := s.classCoverage(right)
	if !leftOK || !rightOK {
		panic("static: malformed ClassSet coverage")
	}
	if coverageSubset(leftCoverage, rightCoverage) {
		return right
	}
	if coverageSubset(rightCoverage, leftCoverage) {
		return left
	}
	if joined, found := s.classForJoinedCoverage(leftCoverage, rightCoverage); found {
		return joined
	}
	return s.derivedJoin(leftCoverage, rightCoverage)
}
func (s *ClassSet) Rank(class Class) uint64 {
	if !s.owns(class) {
		return 0
	}
	if class.descriptor != nil {
		return uint64(class.descriptor.rank)
	}
	if uint64(class.index) >= uint64(len(s.ranks)) {
		return 0
	}
	return uint64(s.ranks[class.index])
}

func (s *ClassSet) CanBeNil(class Class) bool {
	if !s.owns(class) {
		return false
	}
	if s.isAny(class) {
		return true
	}
	if class.descriptor != nil {
		return class.descriptor.nilable
	}
	if s.rows[class.index].kind == ClassOpaque {
		return true
	}
	if s.Equal(s.nil, class) {
		return true
	}
	return s.nilable[class.index]
}

// MayRuntimeKinds returns the presealed Lua runtime-kind over-approximation
// for one exact Class.  The query is an immutable slice lookup: it never
// decodes types, walks recursive graphs, or allocates during recurrent solve
// work.  Applied classes share their declared row's runtime representation.
//
// A zero mask is a valid answer for a concrete bottom class.  The bool only
// distinguishes that result from a foreign or malformed Class handle.
func (s *ClassSet) MayRuntimeKinds(class Class) (runtimekind.Set, bool) {
	if !s.owns(class) {
		return 0, false
	}
	if class.descriptor != nil {
		return class.descriptor.derivedKinds, true
	}
	if uint64(class.index) >= uint64(len(s.runtimeKinds)) {
		return 0, false
	}
	return s.runtimeKinds[class.index], true
}

func (s *ClassSet) owns(class Class) bool {
	if s == nil || class.owner != s {
		return false
	}
	if class.descriptor != nil {
		return class.descriptor.owner == s && len(class.descriptor.coverage) == s.coverageStride && class.descriptor.covered != 0
	}
	return uint64(class.index) < uint64(len(s.rows))
}

func (s *ClassSet) isAny(class Class) bool {
	return s != nil && class.owner == s && class.descriptor == nil && class.index == 0
}

func (s *ClassSet) classCoverage(class Class) ([]uint64, bool) {
	if !s.owns(class) {
		return nil, false
	}
	if class.descriptor != nil {
		return class.descriptor.coverage, true
	}
	if uint64(class.index) >= uint64(len(s.descriptors)) {
		return nil, false
	}
	return s.descriptors[class.index].coverage, true
}

func (s *ClassSet) addConcrete(value typ.Type) (Class, error) {
	value = typ.UnwrapStructuralWrappers(value)
	if value == nil {
		return Class{}, errors.New("static: nil concrete class")
	}
	input, ok := s.authority.types.RuntimeInputForType(value)
	if !ok {
		return Class{}, errors.New("static: concrete class lacks scoped Runtime input")
	}
	return s.addConcreteInput(value, input)
}

func (s *ClassSet) addConcreteInput(value typ.Type, input typeauthority.RuntimeInput) (Class, error) {
	value = typ.UnwrapStructuralWrappers(value)
	if value == nil {
		return Class{}, errors.New("static: nil concrete class")
	}
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil {
		return Class{}, err
	}
	if class, ok := s.byBytes[string(encoded)]; ok {
		return class, nil
	}
	index, err := denseOrdinal(len(s.rows))
	if err != nil {
		return Class{}, fmt.Errorf("static: concrete class handle: %w", err)
	}
	class := Class{owner: s, index: index}
	s.rows = append(s.rows, classRow{kind: ClassConcrete, encoded: append([]byte(nil), encoded...), input: input})
	s.byBytes[string(encoded)] = class
	return class, nil
}

func (s *ClassSet) addTarget(contract *target.Contract, value vocabulary.Type) error {
	if _, exists := s.byTarget[value]; exists {
		return nil
	}
	declaration, ok := contract.Operations.TypeDeclaration(value)
	if !ok {
		return errors.New("static: Target type declaration unavailable")
	}
	decoded, err := domaincontract.Decode(context.Background(), declaration, nil)
	if err == nil && decoded != nil {
		class, addErr := s.addConcrete(decoded)
		if addErr != nil {
			return addErr
		}
		s.byTarget[value] = class
		return nil
	}
	// A scoped endpoint with operation formals is still a finite opaque class.
	// Static does not manufacture a parallel Target-formal authority to decode it.
	contractID := contract.ContentID()
	digest := declaration.Digest()
	identity := make([]byte, 0, len(contractID)+8+len(digest))
	identity = append(identity, contractID[:]...)
	var ordinal [8]byte
	binary.BigEndian.PutUint64(ordinal[:], uint64(value))
	identity = append(identity, ordinal[:]...)
	identity = append(identity, digest[:]...)
	index, ordinalErr := denseOrdinal(len(s.rows))
	if ordinalErr != nil {
		return fmt.Errorf("static: Target class handle: %w", ordinalErr)
	}
	class := Class{owner: s, index: index}
	s.rows = append(s.rows, classRow{kind: ClassOpaque, encoded: identity})
	s.byTarget[value] = class
	return nil
}

func (s *ClassSet) addValues(contract *target.Contract, values vocabulary.Values) error {
	for index := 0; index < contract.Operations.ValuesCount(values); index++ {
		value, ok := contract.Operations.ValuesAt(values, index)
		if !ok {
			return errors.New("static: malformed Target Values")
		}
		if err := s.addTarget(contract, value); err != nil {
			return err
		}
	}
	for index := 0; index < contract.Operations.ValuesSuffixCount(values); index++ {
		value, ok := contract.Operations.ValuesSuffixAt(values, index)
		if !ok {
			return errors.New("static: malformed Target Values suffix")
		}
		if err := s.addTarget(contract, value); err != nil {
			return err
		}
	}
	if value, ok := contract.Operations.ValuesTailType(values); ok {
		return s.addTarget(contract, value)
	}
	return nil
}

func (s *ClassSet) addOperation(contract *target.Contract, operation vocabulary.Operation) error {
	input, ok := contract.Operations.Input(operation)
	if !ok {
		return errors.New("static: operation input unavailable")
	}
	if err := s.addValues(contract, input); err != nil {
		return err
	}
	for index := 0; index < contract.Operations.TypeFormalCount(operation); index++ {
		if value, ok := contract.Operations.TypeFormalConstraint(operation, vocabulary.TypeFormal(index)); ok {
			if err := s.addTarget(contract, value); err != nil {
				return err
			}
		}
	}
	for index := 0; index < contract.Operations.ValuesVarCount(operation); index++ {
		value, ok := contract.Operations.ValuesVarType(operation, vocabulary.ValuesVar(index))
		if !ok {
			return errors.New("static: ValuesVar type unavailable")
		}
		if err := s.addTarget(contract, value); err != nil {
			return err
		}
	}
	for index := 0; index < contract.Operations.OutcomeCount(operation); index++ {
		_, values, ok := contract.Operations.OutcomeAt(operation, index)
		if !ok {
			return errors.New("static: malformed outcome")
		}
		if err := s.addValues(contract, values); err != nil {
			return err
		}
	}
	kinds := [...]flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel}
	for index := 0; index < contract.Operations.CallbackCount(operation); index++ {
		callback, ok := contract.Operations.CallbackAt(operation, index)
		if !ok {
			return errors.New("static: malformed callback")
		}
		values, ok := contract.CallbackArguments(callback)
		if !ok {
			return errors.New("static: callback arguments unavailable")
		}
		if err := s.addValues(contract, values); err != nil {
			return err
		}
		for _, kind := range kinds {
			if values, found := contract.CallbackOutcome(callback, kind); found {
				if err := s.addValues(contract, values); err != nil {
					return err
				}
			}
		}
	}
	for index := 0; index < contract.SubedgeCount(operation); index++ {
		edge, ok := contract.SubedgeAt(operation, index)
		if !ok {
			return errors.New("static: malformed subedge")
		}
		values, ok := contract.SubedgeArguments(edge)
		if !ok {
			return errors.New("static: subedge arguments unavailable")
		}
		if err := s.addValues(contract, values); err != nil {
			return err
		}
		for _, kind := range kinds {
			if values, found := contract.SubedgeTerminal(edge, kind); found {
				if err := s.addValues(contract, values); err != nil {
					return err
				}
			}
		}
		if values, found := contract.AdmissionFailure(edge); found {
			if err := s.addValues(contract, values); err != nil {
				return err
			}
		}
	}
	for index := 0; index < contract.Operations.ResumeCount(operation); index++ {
		resume, ok := contract.Operations.ResumeIDAt(operation, index)
		if !ok {
			return errors.New("static: malformed resume")
		}
		_, _, _, values, ok := contract.Operations.Resume(resume)
		if !ok {
			return errors.New("static: resume arguments unavailable")
		}
		if err := s.addValues(contract, values); err != nil {
			return err
		}
	}
	return nil
}

func (s *ClassSet) contentID(runtime *typeauthority.Runtime, operations map[vocabulary.Operation]struct{}) (id identity.ContentID) {
	if runtime == nil || !runtime.ContentID().Available() {
		return identity.ContentID{}
	}
	h := sha256.New()
	// v10 states the coverage descriptor law: a Class identity is its
	// extensional coverage over the sealed universe, not an ordered basis.
	h.Write([]byte("wippy.analysis.static/class-set/v10"))
	linkID := s.authority.LinkID()
	h.Write(linkID[:])
	runtimeID := runtime.ContentID()
	h.Write(runtimeID[:])
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(len(s.rows)))
	h.Write(word[:])
	for _, row := range s.rows {
		h.Write([]byte{byte(row.kind)})
		if row.kind == ClassConcrete {
			innerID, ok := runtime.Identity(row.inner)
			if !ok {
				return identity.ContentID{}
			}
			h.Write(innerID[:])
			continue
		}
		binary.BigEndian.PutUint64(word[:], uint64(len(row.encoded)))
		h.Write(word[:])
		h.Write(row.encoded)
	}
	contract := s.authority.target
	if contract == nil {
		return identity.ContentID{}
	}
	targetID := contract.ContentID()
	h.Write(targetID[:])
	// Target handle order, not map iteration, fixes selected-operation identity.
	for index := 0; index < contract.Operations.OperationCount(); index++ {
		operation, _ := contract.Operations.OperationAt(index)
		if _, ok := operations[operation]; ok {
			operationID, valid := contract.OperationContentID(operation)
			if !valid {
				return identity.ContentID{}
			}
			h.Write(operationID[:])
		}
	}
	// The raw structurally canonical universe is part of the algebra: coverage
	// and rank are interpreted over exactly this finite P.
	binary.BigEndian.PutUint64(word[:], uint64(len(s.universeIDs)))
	h.Write(word[:])
	for _, atomID := range s.universeIDs {
		h.Write(atomID[:])
	}
	// Extensional coverage descriptors, rather than declaration-order pair
	// tables or ordered bases, are the complete Pack carrier identity.
	binary.BigEndian.PutUint64(word[:], uint64(len(s.descriptors)))
	h.Write(word[:])
	for _, descriptor := range s.descriptors {
		h.Write(descriptor.coverageID[:])
	}
	nilableCount := 0
	for _, answer := range s.nilable {
		if answer {
			nilableCount++
		}
	}
	binary.BigEndian.PutUint64(word[:], uint64(nilableCount))
	h.Write(word[:])
	for index, answer := range s.nilable {
		if answer {
			if index >= len(s.descriptors) {
				return identity.ContentID{}
			}
			h.Write(s.descriptors[index].coverageID[:])
		}
	}
	for index, rank := range s.ranks {
		if index >= len(s.descriptors) {
			return identity.ContentID{}
		}
		h.Write(s.descriptors[index].coverageID[:])
		binary.BigEndian.PutUint64(word[:], uint64(rank))
		h.Write(word[:])
	}
	copy(id[:], h.Sum(nil))
	return id
}
