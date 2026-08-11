package static

import (
	"crypto/sha256"
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
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
func (s *ClassSet) Identity(class Class) (keyspace.ContentID, bool) {
	if !s.owns(class) || !s.id.Available() {
		return keyspace.ContentID{}, false
	}
	if class.descriptor != nil {
		id := class.descriptor.identity
		return id, id.Available()
	}
	if uint64(class.index) >= uint64(len(s.identities)) {
		return keyspace.ContentID{}, false
	}
	id := s.identities[class.index]
	return id, id.Available()
}

func classIdentityDescriptorID(descriptorKey string) (id keyspace.ContentID) {
	prefix := []byte("wippy.analysis.static/class-descriptor\x00\x02")
	input := make([]byte, 0, len(prefix)+len(descriptorKey))
	input = append(input, prefix...)
	input = append(input, descriptorKey...)
	return sha256.Sum256(input)
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
		id := classIdentityDescriptorID(s.descriptors[index].key)
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
	s.identities = make([]keyspace.ContentID, len(s.rows))
	seen := make(map[keyspace.ContentID]string, len(s.rows))
	for index := range s.rows {
		if index >= len(s.descriptors) || s.descriptors[index].key == "" {
			return errors.New("static: unavailable class semantic descriptor")
		}
		key := s.descriptors[index].key
		id := classIdentityID(key)
		if !id.Available() {
			return errors.New("static: unavailable class identity")
		}
		if prior, duplicate := seen[id]; duplicate && prior != key {
			return errors.New("static: class identity collision")
		}
		seen[id] = key
		s.identities[index] = id
	}
	return nil
}

func classIdentityID(descriptorKey string) (id keyspace.ContentID) {
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.analysis.static/class\x00\x02"))
	_, _ = h.Write([]byte(descriptorKey))
	copy(id[:], h.Sum(nil))
	return id
}
