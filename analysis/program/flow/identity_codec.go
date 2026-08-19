package flow

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/framing"
)

// flowSemanticID encodes identities derived entirely from canonical Flow
// structure. Each public owner query supplies its own stable domain.
func flowSemanticID(domain string, write func(*framing.Writer) bool) identity.ContentID {
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, domain, 1) != nil || writer.Record(1) != nil || write == nil || !write(&writer) || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}
