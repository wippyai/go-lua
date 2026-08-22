package static

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
)

func (s *ClassSet) descriptorFor(class Class) (*classDescriptor, bool) {
	if !s.owns(class) {
		return nil, false
	}
	if class.descriptor != nil {
		return class.descriptor, true
	}
	if uint64(class.index) >= uint64(len(s.descriptors)) {
		return nil, false
	}
	return &s.descriptors[class.index], true
}

// Fingerprint is the allocation-free identity of an admitted Pack class.
// Structural runtime-type identity belongs to typeauthority.Runtime instead.
func (s *ClassSet) Fingerprint(class Class) uint64 {
	descriptor, ok := s.descriptorFor(class)
	if !ok {
		return 0
	}
	return descriptor.fingerprint
}

// Identity is the portable identity of one Pack class. It is deliberately
// distinct from runtime structural identity and remains owned by ClassSet.
func (s *ClassSet) Identity(class Class) (identity.ContentID, bool) {
	if s == nil || !s.id.Available() {
		return identity.ContentID{}, false
	}
	descriptor, ok := s.descriptorFor(class)
	if !ok {
		return identity.ContentID{}, false
	}
	id := descriptor.identity
	return id, id.Available()
}

// classIdentityID consumes the canonical coverage identity. Sealed and
// derived classes use this one identity language.
func classIdentityID(coverageID identity.ContentID) (id identity.ContentID) {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/class\x00\x03"))
	_, _ = h.Write(coverageID[:])
	copy(id[:], h.Sum(nil))
	return id
}
