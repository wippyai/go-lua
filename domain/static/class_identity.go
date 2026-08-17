package static

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
)

// Fingerprint is the allocation-free identity of an admitted Pack class.
// Structural runtime-type identity belongs to typeauthority.Runtime instead.
func (s *ClassSet) Fingerprint(class Class) uint64 {
	if !s.owns(class) {
		return 0
	}
	if class.descriptor != nil {
		return class.descriptor.fingerprint
	}
	return uint64(class.index) + 1
}

// Identity is the portable identity of one Pack class. It is deliberately
// distinct from runtime structural identity and remains owned by ClassSet.
func (s *ClassSet) Identity(class Class) (identity.ContentID, bool) {
	if !s.owns(class) || !s.id.Available() {
		return identity.ContentID{}, false
	}
	if class.descriptor != nil {
		id := class.descriptor.identity
		return id, id.Available()
	}
	if uint64(class.index) >= uint64(len(s.identities)) {
		return identity.ContentID{}, false
	}
	id := s.identities[class.index]
	return id, id.Available()
}

// classIdentityDescriptorID and classIdentityID both consume the coverage
// identity of a descriptor. The \x03 law states that input language: a class
// identity is its extensional coverage, not an ordered basis encoding.
func classIdentityDescriptorID(coverageID identity.ContentID) (id identity.ContentID) {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/class-descriptor\x00\x03"))
	_, _ = h.Write(coverageID[:])
	copy(id[:], h.Sum(nil))
	return id
}

// finalizeDescriptorIdentities is intentionally separate from descriptor
// sealing: ClassSet's content identity is the input to every descriptor
// identity, so it cannot be minted while the descriptor rows are still being
// assembled.
func (s *ClassSet) finalizeDescriptorIdentities() error {
	if s == nil || !s.id.Available() || len(s.descriptors) == 0 {
		return errors.New("static: unavailable descriptor identity source")
	}
	for index := range s.descriptors {
		id := classIdentityDescriptorID(s.descriptors[index].coverageID)
		if !id.Available() {
			return errors.New("static: unavailable descriptor identity")
		}
		s.descriptors[index].identity = id
	}
	return nil
}

func (s *ClassSet) sealClassIdentities() error {
	if s == nil || !s.id.Available() {
		return errors.New("static: unavailable class identity projection")
	}
	s.identities = make([]identity.ContentID, len(s.rows))
	seen := make(map[identity.ContentID]identity.ContentID, len(s.rows))
	for index := range s.rows {
		if index >= len(s.descriptors) || !s.descriptors[index].coverageID.Available() {
			return errors.New("static: unavailable class semantic descriptor")
		}
		coverageID := s.descriptors[index].coverageID
		id := classIdentityID(coverageID)
		if !id.Available() {
			return errors.New("static: unavailable class identity")
		}
		if prior, duplicate := seen[id]; duplicate && prior != coverageID {
			return errors.New("static: class identity collision")
		}
		seen[id] = coverageID
		s.identities[index] = id
	}
	return nil
}

func classIdentityID(coverageID identity.ContentID) (id identity.ContentID) {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/class\x00\x03"))
	_, _ = h.Write(coverageID[:])
	copy(id[:], h.Sum(nil))
	return id
}
