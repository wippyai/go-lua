package typeauthority

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/domain/type/transform"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// Family is a sealed, Link-independent Runtime input vocabulary. Its owner
// admits one ordered set of closed types once, and every Runtime seal then
// reads the same canonically encoded receipts instead of cloning and encoding
// the vocabulary again.
//
// A Family is the general form of the fixed primitive seed rows: those eight
// graphs are already shared, un-cloned and un-re-encoded, by every Runtime
// seal in the process. Sharing is sound for exactly the same reason here. A
// member is closed, canonical, and never mutated after admission, so the
// Link fence it is later bound to carries no information about it.
//
// Family is immutable after SealFamily and safe for concurrent readers.
type Family struct {
	id      identity.ContentID
	members []familyMember
	prefix  *familyPrefix
}

type familyMember struct {
	canonicalID identity.ContentID
	graph       typ.CanonicalGraphReceipt
}

// SealFamily admits one ordered closed type vocabulary. domain names the
// owner that authored the order, so two owners never mint one identity for
// two different vocabularies. Each value is cloned once here and never again:
// the Family owns the resulting graph outright.
func SealFamily(domain string, values []typ.Type) (*Family, error) {
	if domain == "" {
		return nil, errors.New("typeauthority: unnamed Runtime family")
	}
	family := &Family{members: make([]familyMember, len(values))}
	hash := sha256.New()
	_, _ = hash.Write([]byte("wippy.domain.type/runtime-family/v1"))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(domain))
	var word [8]byte
	binary.BigEndian.PutUint64(word[:], uint64(len(values)))
	_, _ = hash.Write(word[:])
	for index, value := range values {
		if value == nil {
			return nil, errors.New("typeauthority: nil Runtime family member")
		}
		graph, err := typ.EncodeCanonicalGraph(context.Background(), transform.Clone(value))
		if err != nil {
			return nil, err
		}
		root, rootOK := graph.Root()
		if !rootOK || !root.Closed {
			return nil, errors.New("typeauthority: open Runtime family member root")
		}
		sealed, sealedOK := graph.Seal()
		if !sealedOK {
			return nil, errors.New("typeauthority: unsealable Runtime family member")
		}
		digest, digestOK := sealed.Digest()
		canonicalID := identity.ContentID(digest)
		if !digestOK || !canonicalID.Available() {
			return nil, errors.New("typeauthority: Runtime family member identity unavailable")
		}
		family.members[index] = familyMember{canonicalID: canonicalID, graph: sealed}
		_, _ = hash.Write(canonicalID[:])
	}
	copy(family.id[:], hash.Sum(nil))
	if !family.id.Available() {
		return nil, errors.New("typeauthority: unavailable Runtime family identity")
	}
	prefix, err := newFamilyPrefix(family.members)
	if err != nil {
		return nil, err
	}
	family.prefix = prefix
	return family, nil
}

// ContentID is the portable identity of the whole ordered vocabulary.
func (f *Family) ContentID() identity.ContentID {
	if f == nil {
		return identity.ContentID{}
	}
	return f.id
}

// Count is the number of admitted members.
func (f *Family) Count() int {
	if f == nil {
		return 0
	}
	return len(f.members)
}

// CanonicalIdentity is the owner-neutral identity of one admitted member.
func (f *Family) CanonicalIdentity(index int) (identity.ContentID, bool) {
	if f == nil || index < 0 || index >= len(f.members) {
		return identity.ContentID{}, false
	}
	id := f.members[index].canonicalID
	return id, id.Available()
}

// Input binds one already-encoded member to the Authority that will seal it.
// It performs no clone and no canonical encoding: the whole point of a sealed
// Family is that both were paid once by its owner. The Link fence still
// applies, because only this call can name the sealing Authority.
func (f *Family) Input(index int, types *Authority) (RuntimeInput, bool) {
	if f == nil || types == nil || !types.LinkID().Available() || index < 0 || index >= len(f.members) {
		return RuntimeInput{}, false
	}
	member := f.members[index]
	if !member.graph.Sealed() || f.prefix == nil {
		return RuntimeInput{}, false
	}
	return RuntimeInput{authority: types, graph: member.graph, prefix: f.prefix, prefixMember: index}, true
}
