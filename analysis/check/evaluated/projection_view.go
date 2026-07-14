package evaluated

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"

	"github.com/wippyai/go-lua/analysis/engine/operationplan"
)

type ProjectionMode uint8

const (
	ProjectionModeInvalid ProjectionMode = iota
	ProjectionWithoutCallOutcome
	ProjectionWithCallOutcome
)

type ProjectionViewDigest [sha256.Size]byte

func (d ProjectionViewDigest) Available() bool { return d != (ProjectionViewDigest{}) }

// ProjectionView is the exact selected requirement universe. ConsumerInventory
// identifies the full sealed inventory; View distinguishes Entries(false) from
// Entries(true), including mode and exact canonical slot count.
type ProjectionView struct {
	Mode   ProjectionMode
	Slots  uint32
	Digest ProjectionViewDigest
}

func (v ProjectionView) Valid() bool {
	return (v.Mode == ProjectionWithoutCallOutcome || v.Mode == ProjectionWithCallOutcome) && v.Slots != 0 && v.Digest.Available()
}

func SealProjectionView(requirements operationplan.ObservationRequirements, callOutcome bool) (ProjectionView, error) {
	if !requirements.Sealed() {
		return ProjectionView{}, fmt.Errorf("evaluated: projection view requirements are not sealed")
	}
	mode := ProjectionWithoutCallOutcome
	if callOutcome {
		mode = ProjectionWithCallOutcome
	}
	h := sha256.New()
	writeProjectionBytes(h, []byte("wippy.evaluated.projection-view.v1"))
	writeProjectionUint(h, uint64(mode))
	entries := requirements.Entries(callOutcome)
	writeProjectionUint(h, uint64(len(entries)))
	for _, requirement := range entries {
		writeProjectionBytes(h, []byte(requirement.Projection()))
		writeProjectionUint(h, uint64(requirement.Stage()))
		writeProjectionUint(h, uint64(requirement.Point()))
		to, hasTo := requirement.EdgeTarget()
		writeProjectionUint(h, uint64(to))
		writeProjectionUint(h, boolUint(hasTo))
		anchor, hasAnchor := requirement.Anchor()
		writeProjectionUint(h, uint64(anchor.Point.Ordinal))
		writeProjectionUint(h, uint64(anchor.Point.Phase))
		writeProjectionUint(h, uint64(anchor.Kind))
		writeProjectionUint(h, uint64(anchor.Slot))
		writeProjectionUint(h, boolUint(hasAnchor))
		writeProjectionUint(h, boolUint(requirement.RequiresCallOutcome()))
	}
	var digest ProjectionViewDigest
	copy(digest[:], h.Sum(nil))
	view := ProjectionView{Mode: mode, Slots: uint32(len(entries)), Digest: digest}
	if !view.Valid() {
		return ProjectionView{}, fmt.Errorf("evaluated: empty projection view")
	}
	return view, nil
}

func writeProjectionBytes(h hash.Hash, value []byte) {
	writeProjectionUint(h, uint64(len(value)))
	_, _ = h.Write(value)
}

func writeProjectionUint(h hash.Hash, value uint64) {
	var raw [8]byte
	binary.BigEndian.PutUint64(raw[:], value)
	_, _ = h.Write(raw[:])
}

func boolUint(value bool) uint64 {
	if value {
		return 1
	}
	return 0
}
