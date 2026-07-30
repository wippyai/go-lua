package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// RekeyKeySpace structurally imports every keyspace-owned State lane into to.
// Lane ownership and rekey operations live in the lane catalog, so adding a
// State axis without declaring its keyspace policy fails at catalog creation.
// The operation is transactional and validates keys even when from == to. Nil
// provenance succeeds only when every enabled owned lane is structurally free.
func (s State) RekeyKeySpace(from, to *keyspace.KeySpace) (State, error) {
	if from != nil && !from.Valid() || to != nil && !to.Valid() {
		return s, fmt.Errorf("state: cannot rekey with invalid keyspace authority")
	}
	out := s
	for _, spec := range defaultLaneCatalog.specs {
		if spec.keySpaceMode == laneKeySpaceOwned && s.laneEnabled(spec.bit) {
			next, ok := spec.rekey(out, from, to)
			if !ok {
				return s, fmt.Errorf("state: cannot structurally import keyspace-owned lane %q", spec.id)
			}
			out = next
		}
	}
	return out, nil
}
