package static

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Descriptor atoms are Runtime structural identities or opaque Target
// identities. The high bit is reserved for opaque classes; Runtime handles
// are one-based uint32 values and therefore cannot collide with it.
const opaqueAtomBit uint64 = 1 << 63

type classDescriptor struct {
	owner       *ClassSet
	atoms       []uint64 // deterministic maximal principal basis, in universe order.
	key         string   // portable basis identity; never a dense-handle encoding.
	identity    keyspace.ContentID
	rank        uint64
	nilable     bool
	fingerprint uint64
	closed      []byte // optional exact union encoding for derived closed values.
}

func opaqueClassAtom(index uint32) uint64 { return opaqueAtomBit | uint64(index) }

func runtimeClassAtom(index uint32) uint64 { return uint64(index) }

type descriptorUniverseRow struct {
	atom uint64
	key  string
	id   keyspace.ContentID
}

// sealDescriptorUniverse publishes exactly the finite observation universe P:
// every closed structural Runtime row plus every opaque Class row. Ordering is
// by portable structural/opaque bytes, never by construction-local handles.
// No subtype pair table or coverage bitset survives this pass.
func (s *ClassSet) sealDescriptorUniverse(runtime *typeauthority.Runtime) error {
	if s == nil || runtime == nil || len(s.rows) == 0 {
		return errors.New("static: descriptor universe source unavailable")
	}
	rows := make([]descriptorUniverseRow, 0, runtime.Count()+len(s.rows))
	for index := 1; index <= runtime.Count(); index++ {
		inner, ok := runtime.InnerAtIndex(uint32(index))
		if !ok {
			return errors.New("static: Runtime universe row unavailable")
		}
		encoded, closed := runtime.CanonicalEncoding(inner)
		if !closed {
			// A bound/open structural child cannot participate in Runtime.Subtype
			// as an independent judgment. Its enclosing closed row remains in P.
			continue
		}
		key := "\x00" + string(encoded)
		rows = append(rows, descriptorUniverseRow{
			atom: runtimeClassAtom(uint32(index)),
			key:  key,
			id:   descriptorAtomID(key),
		})
		if form, valid := runtime.Form(inner); valid && form == typeauthority.FormUnknown {
			s.unknownAtom = runtimeClassAtom(uint32(index))
		}
	}
	for index := 1; index < len(s.rows); index++ {
		if s.rows[index].kind != ClassOpaque {
			continue
		}
		key := "\x01" + string(s.rows[index].encoded)
		rows = append(rows, descriptorUniverseRow{
			atom: opaqueClassAtom(uint32(index)),
			key:  key,
			id:   descriptorAtomID(key),
		})
	}
	if len(rows) == 0 || uint64(len(rows)) == ^uint64(0) || s.unknownAtom == 0 {
		return errors.New("static: empty, overflowing, or top-less descriptor universe")
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].key != rows[right].key {
			return rows[left].key < rows[right].key
		}
		return rows[left].atom < rows[right].atom
	})
	s.universe = make([]uint64, len(rows))
	s.universeIDs = make([]keyspace.ContentID, len(rows))
	for index, row := range rows {
		if !row.id.Available() || index > 0 && row.atom == rows[index-1].atom {
			return errors.New("static: malformed descriptor universe identity")
		}
		s.universe[index] = row.atom
		s.universeIDs[index] = row.id
	}
	return nil
}

func descriptorAtomID(key string) keyspace.ContentID {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/class-atom\x00\x01"))
	_, _ = h.Write([]byte(key))
	var id keyspace.ContentID
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
	s.ranks = make([]uint64, len(s.rows))
	s.nilable = make([]bool, len(s.rows))

	// ClassAnyValue is the total finite coverage, not an empty descriptor.
	anyAtoms, ok := s.normalizeDescriptorAtoms(append([]uint64(nil), s.universe...))
	if !ok {
		return errors.New("static: total Class descriptor unavailable")
	}
	s.descriptors[0] = classDescriptor{owner: s, atoms: anyAtoms}
	s.nilable[0] = true

	for index := 1; index < len(s.rows); index++ {
		row := s.rows[index]
		var atoms []uint64
		switch row.kind {
		case ClassConcrete:
			if !runtime.Equal(row.inner, row.inner) {
				return errors.New("static: foreign concrete descriptor")
			}
			for atomIndex := 0; atomIndex < runtime.DescriptorCount(row.inner); atomIndex++ {
				atom, valid := runtime.DescriptorAt(row.inner, atomIndex)
				if !valid {
					return errors.New("static: malformed Runtime semantic descriptor")
				}
				atomIndex, indexOK := runtime.Index(atom)
				if !indexOK {
					return errors.New("static: Runtime semantic atom index unavailable")
				}
				atoms = append(atoms, runtimeClassAtom(atomIndex))
			}
			if len(atoms) == 0 {
				return errors.New("static: empty concrete semantic descriptor")
			}
		case ClassOpaque:
			atoms = []uint64{opaqueClassAtom(uint32(index))}
		default:
			return errors.New("static: invalid ClassSet descriptor row")
		}
		normalized, valid := s.normalizeDescriptorAtoms(atoms)
		if !valid {
			return errors.New("static: concrete descriptor coverage unavailable")
		}
		s.descriptors[index] = classDescriptor{owner: s, atoms: normalized}
	}

	if uint64(s.nil.index) >= uint64(len(s.descriptors)) || len(s.descriptors[s.nil.index].atoms) == 0 {
		return errors.New("static: nil descriptor unavailable")
	}
	for index := range s.descriptors {
		descriptor := &s.descriptors[index]
		key, valid := s.descriptorKeyAtoms(descriptor.atoms)
		if !valid {
			return errors.New("static: descriptor portable identity unavailable")
		}
		rank, ranked := s.descriptorIdealRank(descriptor.atoms)
		if !ranked {
			return errors.New("static: descriptor ideal-complement rank unavailable")
		}
		descriptor.key = key
		descriptor.rank = rank
		descriptor.fingerprint = descriptorFingerprint(key)
		s.ranks[index] = rank
		if index != 0 {
			nilable := descriptorHasOpaque(descriptor.atoms) || s.descriptorCoverageSubset(s.descriptors[s.nil.index].atoms, descriptor.atoms)
			descriptor.nilable = nilable
			s.nilable[index] = nilable
		} else {
			descriptor.nilable = true
		}
	}
	if err := s.mergeEquivalentDescriptors(); err != nil {
		return err
	}
	s.descriptorRows = make(map[string]Class, len(s.descriptors))
	for index := range s.descriptors {
		descriptor := &s.descriptors[index]
		descriptor.owner = s
		s.descriptorRows[descriptor.key] = Class{owner: s, index: uint32(index)}
	}
	return nil
}

func (s *ClassSet) universePosition(atom uint64) (int, bool) {
	if s == nil {
		return 0, false
	}
	for index, candidate := range s.universe {
		if candidate == atom {
			return index, true
		}
	}
	return 0, false
}

// atomSubtype extends Runtime's closed structural relation with opaque atoms.
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
	if form, ok := s.runtime.Form(rightInner); ok && (form == typeauthority.FormAny || form == typeauthority.FormUnknown) {
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

func (s *ClassSet) descriptorCovers(atoms []uint64, candidate uint64) (bool, bool) {
	if s == nil || len(atoms) == 0 {
		return false, false
	}
	for _, upper := range atoms {
		answer, decided := s.atomSubtype(candidate, upper)
		if !decided {
			return false, false
		}
		if answer {
			return true, true
		}
	}
	return false, true
}

func (s *ClassSet) descriptorCoverageSubset(leftAtoms, rightAtoms []uint64) bool {
	if s == nil || len(leftAtoms) == 0 || len(rightAtoms) == 0 || len(s.universe) == 0 {
		return false
	}
	for _, candidate := range s.universe {
		left, leftOK := s.descriptorCovers(leftAtoms, candidate)
		right, rightOK := s.descriptorCovers(rightAtoms, candidate)
		if !leftOK || !rightOK || left && !right {
			return false
		}
	}
	return true
}

func (s *ClassSet) descriptorCoverageEqual(leftAtoms, rightAtoms []uint64) bool {
	return s.descriptorCoverageSubset(leftAtoms, rightAtoms) && s.descriptorCoverageSubset(rightAtoms, leftAtoms)
}

func (s *ClassSet) principalSubset(left, right uint64) (bool, bool) {
	if s == nil || len(s.universe) == 0 {
		return false, false
	}
	for _, witness := range s.universe {
		inLeft, leftOK := s.atomSubtype(witness, left)
		inRight, rightOK := s.atomSubtype(witness, right)
		if !leftOK || !rightOK {
			return false, false
		}
		if inLeft && !inRight {
			return false, true
		}
	}
	return true, true
}

// normalizeDescriptorAtoms computes the canonical basis of one extensional
// coverage C. It considers every admitted principal D(a), retains those
// contained in C which are inclusion-maximal, and chooses one deterministic
// representative for equal principals. It uses O(|P|) transient scratch and
// O(|P|^2 + |P|^2*k) subtype scans, where k is the resulting incomparable
// maximal frontier (worst-case cubic, normally tiny). No pair relation or
// coverage representation is retained.
func (s *ClassSet) normalizeDescriptorAtoms(atoms []uint64) ([]uint64, bool) {
	if s == nil || len(atoms) == 0 || len(s.universe) == 0 {
		return nil, false
	}
	for _, atom := range atoms {
		if _, ok := s.universePosition(atom); !ok {
			return nil, false
		}
	}
	coverage := make([]bool, len(s.universe))
	for index, witness := range s.universe {
		covered, ok := s.descriptorCovers(atoms, witness)
		if !ok {
			return nil, false
		}
		coverage[index] = covered
	}
	type principalCandidate struct {
		atom     uint64
		size     int
		position int
	}
	candidates := make([]principalCandidate, 0, len(s.universe))
	for candidatePosition, candidate := range s.universe {
		contained := true
		size := 0
		for witnessIndex, witness := range s.universe {
			inPrincipal, ok := s.atomSubtype(witness, candidate)
			if !ok {
				return nil, false
			}
			if inPrincipal {
				size++
				if !coverage[witnessIndex] {
					contained = false
				}
			}
		}
		if contained {
			candidates = append(candidates, principalCandidate{atom: candidate, size: size, position: candidatePosition})
		}
	}
	if len(candidates) == 0 {
		return nil, false
	}
	// A strict principal superset has strictly greater finite cardinality.
	// Visiting larger principals first means each later candidate need only be
	// compared with the already-retained maximal frontier, rather than with all
	// candidate pairs. Equal-sized equal principals put Unknown first, then the
	// portable universe order supplies the deterministic representative.
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].size != candidates[right].size {
			return candidates[left].size > candidates[right].size
		}
		if (candidates[left].atom == s.unknownAtom) != (candidates[right].atom == s.unknownAtom) {
			return candidates[left].atom == s.unknownAtom
		}
		return candidates[left].position < candidates[right].position
	})
	basis := make([]uint64, 0, len(candidates))
	for _, candidate := range candidates {
		maximal := true
		for _, prior := range basis {
			candidateToPrior, ok := s.principalSubset(candidate.atom, prior)
			if !ok {
				return nil, false
			}
			if candidateToPrior {
				maximal = false
				break
			}
		}
		if maximal {
			basis = append(basis, candidate.atom)
		}
	}
	if len(basis) == 0 || !s.descriptorCoverageEqual(atoms, basis) {
		return nil, false
	}
	// candidates follows universe order, but Unknown may replace an earlier
	// equal principal. Restore the one portable order explicitly.
	sort.Slice(basis, func(left, right int) bool {
		leftPosition, _ := s.universePosition(basis[left])
		rightPosition, _ := s.universePosition(basis[right])
		return leftPosition < rightPosition
	})
	return basis, true
}

// descriptorIdealRank is 1+|P\C(A)|. Strict coverage inclusion therefore
// strictly descends, including direct joins whose basis cardinality is equal.
func (s *ClassSet) descriptorIdealRank(atoms []uint64) (uint64, bool) {
	if s == nil || len(atoms) == 0 || len(s.universe) == 0 || uint64(len(s.universe)) == ^uint64(0) {
		return 0, false
	}
	covered := uint64(0)
	for _, candidate := range s.universe {
		answer, ok := s.descriptorCovers(atoms, candidate)
		if !ok {
			return 0, false
		}
		if answer {
			covered++
		}
	}
	if covered == 0 || covered > uint64(len(s.universe)) {
		return 0, false
	}
	return 1 + uint64(len(s.universe)) - covered, true
}

// mergeEquivalentDescriptors gives one Class row to one extensional coverage.
// Static Values remain exact and distinct; only their Pack Class projection is
// remapped. Because every descriptor is normalized first, portable basis key
// equality is precisely coverage equality.
func (s *ClassSet) mergeEquivalentDescriptors() error {
	if s == nil || len(s.rows) != len(s.descriptors) || len(s.ranks) != len(s.rows) || len(s.nilable) != len(s.rows) {
		return errors.New("static: malformed descriptor merge source")
	}
	keys := make(map[string]uint32, len(s.rows))
	oldToNew := make([]uint32, len(s.rows))
	keepers := make([]uint32, 0, len(s.rows))
	for old := range s.rows {
		key := s.descriptors[old].key
		if key == "" {
			return errors.New("static: descriptor key unavailable during merge")
		}
		if prior, exists := keys[key]; exists {
			oldToNew[old] = prior
			continue
		}
		keys[key] = uint32(len(keepers))
		oldToNew[old] = uint32(len(keepers))
		keepers = append(keepers, uint32(old))
	}
	if len(keepers) == len(s.rows) {
		return nil
	}
	rows := make([]classRow, len(keepers))
	descriptors := make([]classDescriptor, len(keepers))
	ranks := make([]uint64, len(keepers))
	nilable := make([]bool, len(keepers))
	for next, old := range keepers {
		rows[next] = s.rows[old]
		descriptors[next] = s.descriptors[old]
		ranks[next] = s.ranks[old]
		nilable[next] = s.nilable[old]
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
	s.rows, s.descriptors, s.ranks, s.nilable = rows, descriptors, ranks, nilable
	s.byBytes = nil
	return nil
}

func (s *ClassSet) descriptorKeyAtoms(atoms []uint64) (string, bool) {
	if s == nil || len(atoms) == 0 || len(s.universe) != len(s.universeIDs) {
		return "", false
	}
	key := make([]byte, 8+len(atoms)*len(keyspace.ContentID{}))
	binary.BigEndian.PutUint64(key[:8], uint64(len(atoms)))
	for index, atom := range atoms {
		position, ok := s.universePosition(atom)
		if !ok || !s.universeIDs[position].Available() {
			return "", false
		}
		copy(key[8+index*len(keyspace.ContentID{}):], s.universeIDs[position][:])
	}
	return string(key), true
}

func descriptorFingerprint(key string) uint64 {
	digest := sha256.Sum256([]byte(key))
	return binary.BigEndian.Uint64(digest[:8])
}

func descriptorHasOpaque(atoms []uint64) bool {
	for _, atom := range atoms {
		if atom&opaqueAtomBit != 0 {
			return true
		}
	}
	return false
}

func (s *ClassSet) joinDescriptorAtoms(left, right Class) ([]uint64, bool) {
	leftAtoms, leftOK := s.classAtoms(left)
	rightAtoms, rightOK := s.classAtoms(right)
	if !leftOK || !rightOK {
		return nil, false
	}
	atoms := make([]uint64, 0, len(leftAtoms)+len(rightAtoms))
	atoms = append(atoms, leftAtoms...)
	atoms = append(atoms, rightAtoms...)
	return s.normalizeDescriptorAtoms(atoms)
}

func (s *ClassSet) derivedClass(atoms []uint64) Class {
	key, ok := s.descriptorKeyAtoms(atoms)
	if !ok {
		panic("static: derived descriptor identity unavailable")
	}
	if existing, found := s.descriptorRows[key]; found {
		return existing
	}
	descriptor := &classDescriptor{
		owner:       s,
		atoms:       append([]uint64(nil), atoms...),
		key:         key,
		identity:    classIdentityDescriptorID(key),
		fingerprint: descriptorFingerprint(key),
	}
	if uint64(s.nil.index) < uint64(len(s.descriptors)) {
		descriptor.nilable = descriptorHasOpaque(descriptor.atoms) || s.descriptorCoverageSubset(s.descriptors[s.nil.index].atoms, descriptor.atoms)
	}
	rank, ranked := s.descriptorIdealRank(descriptor.atoms)
	if !ranked {
		panic("static: derived descriptor ideal-complement rank unavailable")
	}
	descriptor.rank = rank
	descriptor.closed = s.derivedDescriptorEncoding(descriptor.atoms)
	return Class{owner: s, index: ^uint32(0), descriptor: descriptor}
}

// derivedDescriptorEncoding reconstructs a closed representative only when
// every maximal basis atom is Runtime-owned. Opaque residuals remain exact
// Classes but intentionally have no ClosedType projection.
func (s *ClassSet) derivedDescriptorEncoding(atoms []uint64) []byte {
	if s == nil || s.runtime == nil || len(atoms) == 0 {
		return nil
	}
	members := make([]typ.Type, 0, len(atoms))
	for _, atom := range atoms {
		if atom&opaqueAtomBit != 0 {
			return nil
		}
		inner, ok := s.runtime.InnerAtIndex(uint32(atom))
		if !ok {
			return nil
		}
		encoded, ok := s.runtime.CanonicalEncoding(inner)
		if !ok || len(encoded) == 0 {
			return nil
		}
		member, err := typ.DecodeCanonicalStructural(context.Background(), encoded)
		if err != nil || member == nil {
			return nil
		}
		members = append(members, member)
	}
	union := typeexpr.Union(members...)
	encoded, err := typ.EncodeCanonical(context.Background(), union)
	if err != nil || len(encoded) == 0 {
		return nil
	}
	return encoded
}
