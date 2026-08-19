package program

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// programSemanticID is Program's owner-neutral codec for identities derived
// entirely from canonical authored structure. It retains no row or proof.
func programSemanticID(domain string, write func(*framing.Writer) bool) identity.ContentID {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || write == nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

// programRoleID is Program's owner-fenced companion for identities whose
// canonical structure is reusable but still belongs to one published root.
func programRoleID(domain string, owner identity.ContentID, write func(*framing.Writer) bool) identity.ContentID {
	if !owner.Available() || write == nil {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || writer.Bytes(owner[:]) != nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
