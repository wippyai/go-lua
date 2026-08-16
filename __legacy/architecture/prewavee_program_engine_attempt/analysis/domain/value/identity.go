package value

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program"
)

const sourceLiteralSemanticVersion = 1

func registryAuthority(registry *axis.Registry) (axis.SchemaIdentity, bool) {
	if registry == nil {
		return axis.SchemaIdentity{}, false
	}
	plan, err := registry.CanonicalPlan()
	if err != nil {
		return axis.SchemaIdentity{}, false
	}
	return plan.AuthorityIdentity()
}

// semanticFactor identifies the source-literal semantics carrier. The
// canonical registry authority is part of the identity because it determines
// every product lane's meaning.
func semanticFactor(authority axis.SchemaIdentity) engine.SemanticKey {
	return semantic("factor", authority, nil)
}

// semanticOccurrence identifies one exact typed Program occurrence. Program
// ContentID and the complete tagged Term are sufficient and remain stable if
// Link shard numbering or unrelated module discovery changes.
func semanticOccurrence(authority axis.SchemaIdentity, source *program.Program, term program.Term) engine.SemanticKey {
	return semantic("occurrence", authority, source, uint64(term))
}

func semantic(kind string, authority axis.SchemaIdentity, source *program.Program, values ...uint64) engine.SemanticKey {
	hash := sha256.New()
	_, _ = hash.Write([]byte("analysis/domain/value/source-literal/" + kind))
	_, _ = hash.Write(authority[:])
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
	return engine.SemanticKey{ID: id, Version: sourceLiteralSemanticVersion}
}
