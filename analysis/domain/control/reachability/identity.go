package reachability

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program"
)

const semanticVersion = 1

// semantic identifies one reachability equation family. Its Program anchor
// identifies the concrete occurrence, so it must not encode a Link shard or
// declaration ordinal.
func semantic(kind string) engine.SemanticKey {
	hash := sha256.New()
	_, _ = hash.Write([]byte("analysis/domain/control/reachability/" + kind))
	var id program.ContentID
	copy(id[:], hash.Sum(nil))
	return engine.SemanticKey{ID: id, Version: semanticVersion}
}
