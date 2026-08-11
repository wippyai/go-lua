package coverage

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/engine"
)

const (
	conclusionProtocol uint64 = 0x434f_4e43_4c55_534e
	conclusionVersion  uint16 = 1
)

// DeriveConclusion gives one Factor owner a private, closed conclusion
// namespace without a global registry. Ordinal and revision belong to that
// Factor's contract package; Rule identity deliberately does not participate.
func DeriveConclusion(owner engine.SemanticKey, ordinal, revision uint16) (engine.SemanticKey, bool) {
	if !owner.Available() || ordinal == 0 || revision == 0 {
		return engine.SemanticKey{}, false
	}
	var frame [8 + 2 + 32 + 8 + 2 + 2]byte
	binary.BigEndian.PutUint64(frame[0:8], conclusionProtocol)
	binary.BigEndian.PutUint16(frame[8:10], conclusionVersion)
	digest := owner.Digest()
	copy(frame[10:42], digest[:])
	binary.BigEndian.PutUint64(frame[42:50], owner.Version())
	binary.BigEndian.PutUint16(frame[50:52], ordinal)
	binary.BigEndian.PutUint16(frame[52:54], revision)
	return engine.NewSemanticKey(sha256.Sum256(frame[:]), uint64(revision))
}
