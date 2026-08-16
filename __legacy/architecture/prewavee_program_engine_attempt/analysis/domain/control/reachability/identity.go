package reachability

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program"
)

const semanticVersion = 1

// semantic identifies a process-wide reachability family, such as the one
// Factor installed by this domain.
func semantic(kind string) engine.SemanticKey {
	return semanticProgram(kind, nil)
}

// semanticProgram identifies one cacheable Program equation. It is stable
// when Link shard numbering or module discovery order changes, while keeping
// each equation distinct within one Program cache section.
func semanticProgram(kind string, source *program.Program, values ...uint64) engine.SemanticKey {
	hash := sha256.New()
	_, _ = hash.Write([]byte("analysis/domain/control/reachability/" + kind))
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
	return engine.SemanticKey{ID: id, Version: semanticVersion}
}
