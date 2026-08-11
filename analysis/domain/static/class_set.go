package static

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/target"
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
	inner   typeauthority.RuntimeInner // construction-only; zero for opaque/AnyValue.
}

// ClassSet is the Link-scoped Pack value classification. Sealed singleton
// rows are complemented by immutable owner-fenced lower-set descriptors. The
// finite Runtime/opaque atom universe bounds rank and descriptor width; every
// relational judgment is recomputed by a finite scan. It owns no Pack
// values and derives declaration classes directly from Target.
type ClassSet struct {
	authority        *Authority
	target           *target.Contract
	id               keyspace.ContentID
	rows             []classRow // zero is AnyValue
	byBytes          map[string]Class
	byStatic         map[uint32]Class
	byTarget         map[target.Type]Class
	nilable          []bool
	runtimeKinds     []runtimekind.Set // sealed Class index -> may-runtime-kind mask.
	runtimeAtomKinds []runtimekind.Set // sealed Runtime atom index -> may-runtime-kind mask.
	ranks            []uint64
	universe         []uint64             // structurally ordered finite observation universe P.
	universeIDs      []keyspace.ContentID // portable identity parallel to universe.
	unknownAtom      uint64               // preferred representative of universal principals.
	runtime          *typeauthority.Runtime
	descriptors      []classDescriptor
	descriptorRows   map[string]Class
	nil              Class
	identities       []keyspace.ContentID
}

func sealClassSet(authority *Authority) (*ClassSet, *typeauthority.Runtime, error) {
	set := &ClassSet{authority: authority, rows: []classRow{{kind: ClassAnyValue}},
		byBytes: make(map[string]Class), byStatic: make(map[uint32]Class),
		byTarget: make(map[target.Type]Class)}
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
			class, addErr := set.addConcrete(decoded)
			if addErr != nil {
				return nil, nil, addErr
			}
			set.byStatic[uint32(index)] = class
		case KindSymbolic:
			if row.symbolic.reason == ReasonOpenFormal && row.symbolic.reference.Valid() {
				materialized, found := authority.types.Resolve(row.symbolic.reference)
				if found {
					class, addErr := set.addConcrete(materialized)
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
	contract, ok := authority.source.Boundary().Target()
	if !ok {
		return nil, nil, errors.New("static: Link target unavailable")
	}
	set.target = contract
	seenOperations := make(map[target.Operation]struct{}, contract.OperationCount())
	for index := 0; index < contract.OperationCount(); index++ {
		operation, valid := contract.OperationAt(index)
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
		set.rows[index].inner = typeauthority.RuntimeInner{}
	}
	return set, runtime, nil
}

func (s *ClassSet) ContentID() keyspace.ContentID {
	if s == nil {
		return keyspace.ContentID{}
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
func (s *ClassSet) ClassForTarget(contract *target.Contract, value target.Type) (Class, bool) {
	if s == nil || contract == nil || contract != s.target {
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
	leftAtoms, leftOK := s.classAtoms(left)
	rightAtoms, rightOK := s.classAtoms(right)
	return leftOK && rightOK && s.descriptorCoverageEqual(leftAtoms, rightAtoms)
}

// Compare returns the exact owner-local semantic order of two classes. It is
// deliberately descriptor/atom based rather than a truncated fingerprint, so
// consumers can order derived classes without turning a hash into equality.
func (s *ClassSet) Compare(left, right Class) int {
	if !s.owns(left) || !s.owns(right) {
		return 0
	}
	if s.Equal(left, right) {
		return 0
	}
	leftAtoms, leftOK := s.classAtoms(left)
	rightAtoms, rightOK := s.classAtoms(right)
	if !leftOK || !rightOK {
		return 0
	}
	for index := 0; index < len(leftAtoms) && index < len(rightAtoms); index++ {
		leftPosition, leftFound := s.universePosition(leftAtoms[index])
		rightPosition, rightFound := s.universePosition(rightAtoms[index])
		if !leftFound || !rightFound {
			return 0
		}
		if leftPosition < rightPosition {
			return -1
		}
		if leftPosition > rightPosition {
			return 1
		}
	}
	if len(leftAtoms) < len(rightAtoms) {
		return -1
	}
	return 1
}

func (s *ClassSet) LessOrEq(left, right Class) bool {
	if !s.owns(left) || !s.owns(right) {
		return false
	}
	if left.descriptor == nil && right.descriptor == nil && left.index == right.index {
		return true
	}
	leftAtoms, leftOK := s.classAtoms(left)
	rightAtoms, rightOK := s.classAtoms(right)
	return leftOK && rightOK && s.descriptorCoverageSubset(leftAtoms, rightAtoms)
}
func (s *ClassSet) Join(left, right Class) Class {
	if !s.owns(left) || !s.owns(right) {
		panic("static: foreign Pack class")
	}
	// Most recurrent joins are widening steps where one lower set already
	// contains the other. Resolve those without constructing a descriptor.
	leftToRight := s.LessOrEq(left, right)
	rightToLeft := s.LessOrEq(right, left)
	if leftToRight && rightToLeft {
		atoms, ok := s.classAtoms(left)
		if !ok {
			panic("static: malformed equal ClassSet coverage")
		}
		key, ok := s.descriptorKeyAtoms(atoms)
		if !ok {
			panic("static: malformed equal ClassSet descriptor")
		}
		if canonical, found := s.descriptorRows[key]; found {
			return canonical
		}
		return s.derivedClass(atoms)
	}
	if leftToRight {
		return right
	}
	if rightToLeft {
		return left
	}
	atoms, ok := s.joinDescriptorAtoms(left, right)
	if !ok {
		panic("static: malformed ClassSet descriptor join")
	}
	key, keyOK := s.descriptorKeyAtoms(atoms)
	if !keyOK {
		panic("static: malformed ClassSet descriptor identity")
	}
	if joined, found := s.descriptorRows[key]; found {
		return joined
	}
	return s.derivedClass(atoms)
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
		var kinds runtimekind.Set
		for _, atom := range class.descriptor.atoms {
			if atom&opaqueAtomBit != 0 {
				// Opaque Target/formal atoms have no proven runtime shape.
				kinds |= runtimekind.All
				continue
			}
			if uint64(atom) < uint64(len(s.runtimeAtomKinds)) {
				kinds |= s.runtimeAtomKinds[atom]
			}
		}
		return kinds, true
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
		return class.descriptor.owner == s && len(class.descriptor.atoms) != 0
	}
	return uint64(class.index) < uint64(len(s.rows))
}

func (s *ClassSet) isAny(class Class) bool {
	return s != nil && class.owner == s && class.descriptor == nil && class.index == 0
}

func (s *ClassSet) classAtoms(class Class) ([]uint64, bool) {
	if !s.owns(class) {
		return nil, false
	}
	if class.descriptor != nil {
		return class.descriptor.atoms, true
	}
	if uint64(class.index) >= uint64(len(s.descriptors)) {
		return nil, false
	}
	return s.descriptors[class.index].atoms, true
}

func (s *ClassSet) addConcrete(value typ.Type) (Class, error) {
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
	s.rows = append(s.rows, classRow{kind: ClassConcrete, encoded: append([]byte(nil), encoded...)})
	s.byBytes[string(encoded)] = class
	return class, nil
}

func (s *ClassSet) addTarget(contract *target.Contract, value target.Type) error {
	if _, exists := s.byTarget[value]; exists {
		return nil
	}
	encoded, ok := contract.TypeBytes(value)
	if !ok {
		return errors.New("static: Target type bytes unavailable")
	}
	decoded, err := typ.DecodeCanonicalFormals(context.Background(), encoded, nil)
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
	identity := make([]byte, 0, len(contractID)+8+len(encoded))
	identity = append(identity, contractID[:]...)
	var ordinal [8]byte
	binary.BigEndian.PutUint64(ordinal[:], uint64(value))
	identity = append(identity, ordinal[:]...)
	identity = append(identity, encoded...)
	index, ordinalErr := denseOrdinal(len(s.rows))
	if ordinalErr != nil {
		return fmt.Errorf("static: Target class handle: %w", ordinalErr)
	}
	class := Class{owner: s, index: index}
	s.rows = append(s.rows, classRow{kind: ClassOpaque, encoded: identity})
	s.byTarget[value] = class
	return nil
}

func (s *ClassSet) addValues(contract *target.Contract, values target.Values) error {
	for index := 0; index < contract.ValuesCount(values); index++ {
		value, ok := contract.ValuesAt(values, index)
		if !ok {
			return errors.New("static: malformed Target Values")
		}
		if err := s.addTarget(contract, value); err != nil {
			return err
		}
	}
	for index := 0; index < contract.ValuesSuffixCount(values); index++ {
		value, ok := contract.ValuesSuffixAt(values, index)
		if !ok {
			return errors.New("static: malformed Target Values suffix")
		}
		if err := s.addTarget(contract, value); err != nil {
			return err
		}
	}
	if value, ok := contract.ValuesTailType(values); ok {
		return s.addTarget(contract, value)
	}
	return nil
}

func (s *ClassSet) addOperation(contract *target.Contract, operation target.Operation) error {
	input, ok := contract.Input(operation)
	if !ok {
		return errors.New("static: operation input unavailable")
	}
	if err := s.addValues(contract, input); err != nil {
		return err
	}
	for index := 0; index < contract.TypeFormalCount(operation); index++ {
		if value, ok := contract.TypeFormalConstraint(operation, target.TypeFormal(index)); ok {
			if err := s.addTarget(contract, value); err != nil {
				return err
			}
		}
	}
	for index := 0; index < contract.ValuesVarCount(operation); index++ {
		value, ok := contract.ValuesVarType(operation, target.ValuesVar(index))
		if !ok {
			return errors.New("static: ValuesVar type unavailable")
		}
		if err := s.addTarget(contract, value); err != nil {
			return err
		}
	}
	for index := 0; index < contract.OutcomeCount(operation); index++ {
		_, values, ok := contract.OutcomeAt(operation, index)
		if !ok {
			return errors.New("static: malformed outcome")
		}
		if err := s.addValues(contract, values); err != nil {
			return err
		}
	}
	kinds := [...]flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel}
	for index := 0; index < contract.CallbackCount(operation); index++ {
		callback, ok := contract.CallbackAt(operation, index)
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
	for index := 0; index < contract.ResumeCount(operation); index++ {
		resume, ok := contract.ResumeIDAt(operation, index)
		if !ok {
			return errors.New("static: malformed resume")
		}
		_, _, _, values, ok := contract.Resume(resume)
		if !ok {
			return errors.New("static: resume arguments unavailable")
		}
		if err := s.addValues(contract, values); err != nil {
			return err
		}
	}
	return nil
}

func (s *ClassSet) contentID(runtime *typeauthority.Runtime, operations map[target.Operation]struct{}) (id keyspace.ContentID) {
	if runtime == nil || !runtime.ContentID().Available() {
		return keyspace.ContentID{}
	}
	h := sha256.New()
	h.Write([]byte("wippy.analysis.static/class-set/v9"))
	linkID := s.authority.source.ContentID()
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
				return keyspace.ContentID{}
			}
			h.Write(innerID[:])
			continue
		}
		binary.BigEndian.PutUint64(word[:], uint64(len(row.encoded)))
		h.Write(word[:])
		h.Write(row.encoded)
	}
	contract, _ := s.authority.source.Boundary().Target()
	targetID := contract.ContentID()
	h.Write(targetID[:])
	// Target handle order, not map iteration, fixes selected-operation identity.
	for index := 0; index < contract.OperationCount(); index++ {
		operation, _ := contract.OperationAt(index)
		if _, ok := operations[operation]; ok {
			operationID, valid := contract.OperationContentID(operation)
			if !valid {
				return keyspace.ContentID{}
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
	// Extensional lower-set descriptors, rather than declaration-order pair
	// tables, are the complete Pack carrier identity.
	binary.BigEndian.PutUint64(word[:], uint64(len(s.descriptors)))
	h.Write(word[:])
	for _, descriptor := range s.descriptors {
		binary.BigEndian.PutUint64(word[:], uint64(len(descriptor.key)))
		h.Write(word[:])
		h.Write([]byte(descriptor.key))
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
				return keyspace.ContentID{}
			}
			key := s.descriptors[index].key
			binary.BigEndian.PutUint64(word[:], uint64(len(key)))
			h.Write(word[:])
			h.Write([]byte(key))
		}
	}
	for index, rank := range s.ranks {
		if index >= len(s.descriptors) {
			return keyspace.ContentID{}
		}
		key := s.descriptors[index].key
		binary.BigEndian.PutUint64(word[:], uint64(len(key)))
		h.Write(word[:])
		h.Write([]byte(key))
		binary.BigEndian.PutUint64(word[:], uint64(rank))
		h.Write(word[:])
	}
	copy(id[:], h.Sum(nil))
	return id
}
