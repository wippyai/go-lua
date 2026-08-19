package programschema

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// StorageCellIdentity returns the canonical root-fenced identity of one
// authored storage Cell. Callers remain responsible for proving that the Cell
// belongs to their exact sealed Program; this function owns only the shared
// schema identity equation.
func StorageCellIdentity(programID identity.ContentID, term keyspace.Term) (identity.ContentID, bool) {
	if !programID.Available() || keyspace.TermFamily(term) == keyspace.FamilyInvalid || keyspace.TermOrdinal(term) == 0 {
		return identity.ContentID{}, false
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/transformer/storage-cell", 1) != nil ||
		writer.Record(1) != nil || writer.Bytes(programID[:]) != nil ||
		writer.Uint(uint64(keyspace.TermFamily(term))) != nil ||
		writer.Uint(uint64(keyspace.TermOrdinal(term))) != nil || writer.Finish() != nil {
		return identity.ContentID{}, false
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id, id.Available()
}
