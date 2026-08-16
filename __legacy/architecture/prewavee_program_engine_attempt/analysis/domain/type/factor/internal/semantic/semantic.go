// Package semantic owns the stable identities of the Type Factor and its
// first-class Program equations. These are cold cache identities, never fact
// keys or a runtime rule-kind registry.
package semantic

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
)

// version 3 changes the Factor terminal from a Pack to the sealed
// Pack×Origin carrier.  Persisted equation identities must never make the
// old terminal appear compatible with this provenance-aware factor.
const version = 3

func Factor(source *link.Link) engine.SemanticKey { return identity("factor", source, 0) }

func Literal(source *link.Link, value link.Value) engine.SemanticKey {
	return identity("literal", source, uint64(value))
}

func Values(source *link.Link, value link.Value) engine.SemanticKey {
	return identity("values", source, uint64(value))
}

func identity(kind string, source *link.Link, values ...uint64) engine.SemanticKey {
	hash := sha256.New()
	_, _ = hash.Write([]byte("analysis/domain/type/factor/" + kind))
	if source != nil {
		content := source.ContentID()
		_, _ = hash.Write(content[:])
	}
	var encoded [8]byte
	for _, value := range values {
		binary.BigEndian.PutUint64(encoded[:], value)
		_, _ = hash.Write(encoded[:])
	}
	var id program.ContentID
	copy(id[:], hash.Sum(nil))
	return engine.SemanticKey{ID: id, Version: version}
}
