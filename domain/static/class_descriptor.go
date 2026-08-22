package static

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/type/kind"
)

// Descriptor atoms are Runtime structural identities or opaque Target
// identities. The high bit is reserved for opaque classes; Runtime handles
// are one-based uint32 values and therefore cannot collide with it.
const opaqueAtomBit uint64 = 1 << 63

// classDescriptor is one immutable extensional lower set. Coverage is the
// canonical form and the sole content identity: a maximal-basis presentation
// is reconstructed from it where a closed type projection is consumed, never
// carried through a join.
type classDescriptor struct {
	owner      *ClassSet
	coverage   []uint64 // packed bitset over dense universe positions.
	coverageID identity.ContentID
	identity   identity.ContentID
	rank       uint64
	// derivedKinds is the runtime-kind projection of a derived descriptor.
	// A sealed row answers from ClassSet's dense row table instead.
	derivedKinds runtimekind.Set
	nilable      bool
	fingerprint  uint64
}

func opaqueClassAtom(index uint32) uint64 { return opaqueAtomBit | uint64(index) }

func runtimeClassAtom(index uint32) uint64 { return uint64(index) }

type descriptorUniverseRow struct {
	atom uint64
	key  string
	id   identity.ContentID
}

// sealDescriptorUniverse publishes exactly the finite observation universe P:
// every closed structural Runtime row plus every opaque Class row. Ordering is
// by portable structural/opaque bytes, never by construction-local handles.
func (s *ClassSet) sealDescriptorUniverse(runtime *typeauthority.Runtime) error {
	if s == nil || runtime == nil || len(s.rows) == 0 {
		return errors.New("static: descriptor universe source unavailable")
	}
	rows := make([]descriptorUniverseRow, 0, runtime.Count()+len(s.rows))
	unknownFound := false
	for index := 1; index <= runtime.Count(); index++ {
		inner, ok := runtime.InnerAtIndex(uint32(index))
		if !ok {
			return errors.New("static: Runtime universe row unavailable")
		}
		canonicalID, closed := runtime.CanonicalIdentity(inner)
		if !closed {
			// A bound/open structural child cannot participate in Runtime.Subtype
			// as an independent judgment. Its enclosing closed row remains in P.
			continue
		}
		key := "\x00" + string(canonicalID[:])
		rows = append(rows, descriptorUniverseRow{
			atom: runtimeClassAtom(uint32(index)),
			key:  key,
			id:   descriptorAtomID(key),
		})
		if rowKind, valid := runtime.Kind(inner); valid && rowKind == kind.Unknown {
			unknownFound = true
		}
	}
	for index := 1; index < len(s.rows); index++ {
		if s.rows[index].kind != ClassOpaque {
			continue
		}
		key := "\x01" + string(s.rows[index].opaqueID)
		rows = append(rows, descriptorUniverseRow{
			atom: opaqueClassAtom(uint32(index)),
			key:  key,
			id:   descriptorAtomID(key),
		})
	}
	if !unknownFound {
		return errors.New("static: top-less descriptor universe")
	}
	if err := s.installDescriptorUniverse(rows); err != nil {
		return err
	}
	return s.sealDescriptorPrincipals(s.atomSubtype)
}

// installDescriptorUniverse fixes the dense universe order and the direct
// atom-to-position tables. Ordering is portable, so a permuted declaration
// order installs the same positions and the same atom identities.
func (s *ClassSet) installDescriptorUniverse(rows []descriptorUniverseRow) error {
	if s == nil || len(rows) == 0 || uint64(len(rows)) >= uint64(math.MaxInt32) {
		return errors.New("static: empty or overflowing descriptor universe")
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].key != rows[right].key {
			return rows[left].key < rows[right].key
		}
		return rows[left].atom < rows[right].atom
	})
	var maximumRuntime, maximumOpaque uint64
	for _, row := range rows {
		if row.atom&opaqueAtomBit != 0 {
			if index := row.atom &^ opaqueAtomBit; index > maximumOpaque {
				maximumOpaque = index
			}
			continue
		}
		if row.atom > maximumRuntime {
			maximumRuntime = row.atom
		}
	}
	construction := &descriptorConstruction{
		universe:             make([]uint64, len(rows)),
		runtimeAtomPositions: make([]int32, maximumRuntime+1),
		opaqueAtomPositions:  make([]int32, maximumOpaque+1),
	}
	for index := range construction.runtimeAtomPositions {
		construction.runtimeAtomPositions[index] = -1
	}
	for index := range construction.opaqueAtomPositions {
		construction.opaqueAtomPositions[index] = -1
	}
	atomIDs := make([]identity.ContentID, len(rows))
	for index, row := range rows {
		if !row.id.Available() {
			return errors.New("static: malformed descriptor universe identity")
		}
		table, offset := construction.runtimeAtomPositions, row.atom
		if row.atom&opaqueAtomBit != 0 {
			table, offset = construction.opaqueAtomPositions, row.atom&^opaqueAtomBit
		}
		if table[offset] >= 0 {
			return errors.New("static: duplicate descriptor universe atom")
		}
		table[offset] = int32(index)
		construction.universe[index] = row.atom
		atomIDs[index] = row.id
	}
	s.construction = construction
	s.universeSize = len(rows)
	s.coverageStride = (len(rows) + 63) / 64
	s.opaqueMask = make([]uint64, s.coverageStride)
	for index, atom := range construction.universe {
		if atom&opaqueAtomBit != 0 {
			coverageSet(s.opaqueMask, index)
		}
	}
	s.universeID = descriptorUniverseID(atomIDs)
	if !s.universeID.Available() {
		return errors.New("static: unavailable descriptor universe identity")
	}
	return nil
}

// sealDescriptorPrincipals materializes D(a) for every atom of P as a packed
// bitset row. It is the only consumer of the atom relation: after this pass
// no ClassSet judgment asks a subtype question.
func (s *ClassSet) sealDescriptorPrincipals(relation func(left, right uint64) (bool, bool)) error {
	if s == nil || s.construction == nil || relation == nil || len(s.construction.universe) == 0 || s.coverageStride == 0 {
		return errors.New("static: descriptor principal source unavailable")
	}
	universe := s.construction.universe
	s.construction.principals = make([]uint64, len(universe)*s.coverageStride)
	for upper, upperAtom := range universe {
		row := s.construction.principals[upper*s.coverageStride : (upper+1)*s.coverageStride]
		size := uint32(0)
		for lower, lowerAtom := range universe {
			answer, decided := relation(lowerAtom, upperAtom)
			if !decided {
				return errors.New("static: undecided descriptor atom relation")
			}
			if !answer {
				continue
			}
			coverageSet(row, lower)
			size++
		}
		if size == 0 {
			return errors.New("static: non-reflexive descriptor principal")
		}
	}
	return nil
}

func descriptorAtomID(key string) identity.ContentID {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/class-atom\x00\x01"))
	_, _ = h.Write([]byte(key))
	var id identity.ContentID
	copy(id[:], h.Sum(nil))
	return id
}

func descriptorUniverseID(atoms []identity.ContentID) identity.ContentID {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/class-universe\x00\x01"))
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(len(atoms)))
	_, _ = h.Write(word[:])
	for _, atom := range atoms {
		_, _ = h.Write(atom[:])
	}
	var id identity.ContentID
	copy(id[:], h.Sum(nil))
	return id
}

func (s *ClassSet) sealDescriptors(runtime *typeauthority.Runtime) error {
	if s == nil || runtime == nil || len(s.rows) == 0 {
		return errors.New("static: ClassSet descriptor source unavailable")
	}
	s.runtime = runtime
	if err := s.sealDescriptorUniverse(runtime); err != nil {
		return err
	}
	s.descriptors = make([]classDescriptor, len(s.rows))

	// ClassAnyValue is the total finite coverage, not an empty descriptor.
	total := make([]uint64, s.coverageStride)
	for position := 0; position < s.universeSize; position++ {
		coverageSet(total, position)
	}
	s.descriptors[0] = classDescriptor{owner: s, coverage: total}

	for index := 1; index < len(s.rows); index++ {
		row := s.rows[index]
		coverage := make([]uint64, s.coverageStride)
		switch row.kind {
		case ClassConcrete:
			if !runtime.Equal(row.inner, row.inner) {
				return errors.New("static: foreign concrete descriptor")
			}
			count := runtime.DescriptorCount(row.inner)
			if count == 0 {
				return errors.New("static: empty concrete semantic descriptor")
			}
			for atomIndex := 0; atomIndex < count; atomIndex++ {
				atom, valid := runtime.DescriptorAt(row.inner, atomIndex)
				if !valid {
					return errors.New("static: malformed Runtime semantic descriptor")
				}
				runtimeIndex, indexOK := runtime.Index(atom)
				if !indexOK {
					return errors.New("static: Runtime semantic atom index unavailable")
				}
				if !s.addPrincipal(coverage, runtimeClassAtom(runtimeIndex)) {
					return errors.New("static: concrete descriptor coverage unavailable")
				}
			}
		case ClassOpaque:
			if !s.addPrincipal(coverage, opaqueClassAtom(uint32(index))) {
				return errors.New("static: opaque descriptor coverage unavailable")
			}
		default:
			return errors.New("static: invalid ClassSet descriptor row")
		}
		s.descriptors[index] = classDescriptor{owner: s, coverage: coverage}
	}
	return s.finalizeDescriptors()
}

// finalizeDescriptors closes the descriptor table once every row coverage is
// materialized: it mints coverage identity, rank, and nil admission, collapses
// extensionally equal rows, and publishes the coverage index the hot join
// probes.
func (s *ClassSet) finalizeDescriptors() error {
	if s == nil || len(s.descriptors) != len(s.rows) {
		return errors.New("static: malformed descriptor finalization source")
	}
	if uint64(s.nil.index) >= uint64(len(s.descriptors)) || s.descriptors[s.nil.index].coverage == nil {
		return errors.New("static: nil descriptor unavailable")
	}
	for index := range s.descriptors {
		descriptor := &s.descriptors[index]
		id, valid := s.coverageIdentity(descriptor.coverage)
		if !valid {
			return errors.New("static: descriptor portable identity unavailable")
		}
		covered := coveragePopcount(descriptor.coverage)
		rank, ranked := s.coverageRank(covered)
		if !ranked {
			return errors.New("static: descriptor ideal-complement rank unavailable")
		}
		descriptor.coverageID = id
		descriptor.identity = classIdentityID(id)
		descriptor.rank = rank
		descriptor.fingerprint = descriptorFingerprint(id)
		descriptor.nilable = index == 0 || s.coverageNilable(descriptor.coverage)
		if !descriptor.identity.Available() {
			return errors.New("static: descriptor class identity unavailable")
		}
	}
	if err := s.mergeEquivalentDescriptors(); err != nil {
		return err
	}
	s.coverageIndex = make(map[uint64][]uint32, len(s.descriptors))
	for index := range s.descriptors {
		descriptor := &s.descriptors[index]
		descriptor.owner = s
		hash := coverageHash(descriptor.coverage)
		s.coverageIndex[hash] = append(s.coverageIndex[hash], uint32(index))
	}
	return nil
}

// universePosition is the direct atom-to-position table that replaces the
// former linear universe scan.
func (s *ClassSet) universePosition(atom uint64) (int, bool) {
	if s == nil || s.construction == nil {
		return 0, false
	}
	table, offset := s.construction.runtimeAtomPositions, atom
	if atom&opaqueAtomBit != 0 {
		table, offset = s.construction.opaqueAtomPositions, atom&^opaqueAtomBit
	}
	if offset >= uint64(len(table)) || table[offset] < 0 {
		return 0, false
	}
	return int(table[offset]), true
}

func (s *ClassSet) addPrincipal(coverage []uint64, atom uint64) bool {
	position, ok := s.universePosition(atom)
	if !ok || s.construction == nil || len(coverage) != s.coverageStride {
		return false
	}
	row := s.construction.principals[position*s.coverageStride : (position+1)*s.coverageStride]
	for index := range coverage {
		coverage[index] |= row[index]
	}
	return true
}

// atomSubtype extends Runtime's sealed closed relation with opaque atoms. It
// runs once per atom pair while the universe principals are materialized and
// is never reached by a recurrent query.
//
// Opaque atoms prove only reflexivity, except that Any and Unknown are the
// universal structural supertypes and therefore cover opaque residuals too.
func (s *ClassSet) atomSubtype(left, right uint64) (answer, decided bool) {
	if s == nil || s.runtime == nil {
		return false, false
	}
	if left == right {
		return true, true
	}
	if right&opaqueAtomBit != 0 {
		return false, true
	}
	rightInner, rightOK := s.runtime.InnerAtIndex(uint32(right))
	if !rightOK {
		return false, false
	}
	if rowKind, ok := s.runtime.Kind(rightInner); ok && (rowKind == kind.Any || rowKind == kind.Unknown) {
		return true, true
	}
	if left&opaqueAtomBit != 0 {
		return false, true
	}
	leftInner, leftOK := s.runtime.InnerAtIndex(uint32(left))
	if !leftOK {
		return false, false
	}
	return s.runtime.Subtype(leftInner, rightInner)
}

// coverageRank is 1+|P\C|. Strict coverage growth strictly increases the
// popcount and therefore strictly descends this measure.
func (s *ClassSet) coverageRank(covered uint64) (uint64, bool) {
	if s == nil || s.universeSize == 0 || covered == 0 || covered > uint64(s.universeSize) {
		return 0, false
	}
	return 1 + uint64(s.universeSize) - covered, true
}

func (s *ClassSet) coverageHasOpaque(coverage []uint64) bool {
	if s == nil || len(coverage) != len(s.opaqueMask) {
		return false
	}
	for index, word := range coverage {
		if word&s.opaqueMask[index] != 0 {
			return true
		}
	}
	return false
}

// coverageNilable is exact: an opaque residual has no proven shape and may be
// nil, and any other coverage admits nil exactly when it observes the nil
// class.
func (s *ClassSet) coverageNilable(coverage []uint64) bool {
	if s == nil || uint64(s.nil.index) >= uint64(len(s.descriptors)) {
		return false
	}
	if s.coverageHasOpaque(coverage) {
		return true
	}
	return coverageSubset(s.descriptors[s.nil.index].coverage, coverage)
}

// coverageIdentity is the portable content identity of one coverage. The
// sealed universe identity pins the exact ordered atom vocabulary the bitset
// positions mean, so the pair is a complete portable statement.
func (s *ClassSet) coverageIdentity(coverage []uint64) (identity.ContentID, bool) {
	if s == nil || len(coverage) != s.coverageStride || !s.universeID.Available() {
		return identity.ContentID{}, false
	}
	if coveragePopcount(coverage) == 0 {
		return identity.ContentID{}, false
	}
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/class-coverage\x00\x01"))
	_, _ = h.Write(s.universeID[:])
	var word [8]byte
	for _, part := range coverage {
		binary.BigEndian.PutUint64(word[:], part)
		_, _ = h.Write(word[:])
	}
	var id identity.ContentID
	copy(id[:], h.Sum(nil))
	return id, id.Available()
}

// mergeEquivalentDescriptors gives one Class row to one extensional coverage.
// Static Values remain exact and distinct; only their Pack Class projection is
// remapped. Coverage identity is the merge key and the retained coverage is
// compared word for word, so a digest is never treated as equality.
func (s *ClassSet) mergeEquivalentDescriptors() error {
	if s == nil || len(s.rows) != len(s.descriptors) {
		return errors.New("static: malformed descriptor merge source")
	}
	keys := make(map[identity.ContentID]uint32, len(s.rows))
	oldToNew := make([]uint32, len(s.rows))
	keepers := make([]uint32, 0, len(s.rows))
	for old := range s.rows {
		id := s.descriptors[old].coverageID
		if !id.Available() {
			return errors.New("static: descriptor identity unavailable during merge")
		}
		if prior, exists := keys[id]; exists {
			if !coverageEqual(s.descriptors[keepers[prior]].coverage, s.descriptors[old].coverage) {
				return errors.New("static: descriptor coverage identity collision")
			}
			oldToNew[old] = prior
			continue
		}
		keys[id] = uint32(len(keepers))
		oldToNew[old] = uint32(len(keepers))
		keepers = append(keepers, uint32(old))
	}
	if len(keepers) == len(s.rows) {
		return nil
	}
	rows := make([]classRow, len(keepers))
	descriptors := make([]classDescriptor, len(keepers))
	for next, old := range keepers {
		rows[next] = s.rows[old]
		descriptors[next] = s.descriptors[old]
	}
	remap := func(class Class) (Class, error) {
		if class.owner != s || uint64(class.index) >= uint64(len(oldToNew)) {
			return Class{}, errors.New("static: foreign descriptor merge member")
		}
		return Class{owner: s, index: oldToNew[class.index]}, nil
	}
	for key, class := range s.byStatic {
		mapped, err := remap(class)
		if err != nil {
			return err
		}
		s.byStatic[key] = mapped
	}
	for key, class := range s.byTarget {
		mapped, err := remap(class)
		if err != nil {
			return err
		}
		s.byTarget[key] = mapped
	}
	nilClass, err := remap(s.nil)
	if err != nil {
		return err
	}
	s.nil = nilClass
	s.rows, s.descriptors = rows, descriptors
	s.byCanonical = nil
	return nil
}

func descriptorFingerprint(id identity.ContentID) uint64 {
	return binary.BigEndian.Uint64(id[:8])
}

// classForCoverage and classForJoinedCoverage resolve an already sealed row
// for a coverage. Both probe the owner's coverage index by hash and confirm
// the candidate word for word, so neither materializes an intermediate value.
func (s *ClassSet) classForCoverage(coverage []uint64) (Class, bool) {
	if s == nil {
		return Class{}, false
	}
	for _, index := range s.coverageIndex[coverageHash(coverage)] {
		if coverageEqual(s.descriptors[index].coverage, coverage) {
			return Class{owner: s, index: index}, true
		}
	}
	return Class{}, false
}

func (s *ClassSet) classForJoinedCoverage(left, right []uint64) (Class, bool) {
	if s == nil {
		return Class{}, false
	}
	for _, index := range s.coverageIndex[coverageJoinHash(left, right)] {
		if coverageEqualsJoin(s.descriptors[index].coverage, left, right) {
			return Class{owner: s, index: index}, true
		}
	}
	return Class{}, false
}

func (s *ClassSet) derivedJoin(left, right []uint64, runtimeKinds runtimekind.Set) Class {
	coverage := make([]uint64, s.coverageStride)
	if !coverageOr(coverage, left, right) {
		panic("static: malformed ClassSet descriptor join")
	}
	return s.derivedClass(coverage, runtimeKinds)
}

// derivedClass takes ownership of one freshly built coverage. A derived class
// exists only for a coverage no sealed row already states.
func (s *ClassSet) derivedClass(coverage []uint64, runtimeKinds runtimekind.Set) Class {
	if existing, found := s.classForCoverage(coverage); found {
		return existing
	}
	id, ok := s.coverageIdentity(coverage)
	if !ok {
		panic("static: derived descriptor identity unavailable")
	}
	covered := coveragePopcount(coverage)
	rank, ranked := s.coverageRank(covered)
	if !ranked {
		panic("static: derived descriptor ideal-complement rank unavailable")
	}
	descriptor := &classDescriptor{
		owner:        s,
		coverage:     coverage,
		coverageID:   id,
		identity:     classIdentityID(id),
		rank:         rank,
		derivedKinds: runtimeKinds,
		nilable:      s.coverageNilable(coverage),
		fingerprint:  descriptorFingerprint(id),
	}
	return Class{owner: s, index: ^uint32(0), descriptor: descriptor}
}
