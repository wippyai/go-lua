package flow

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
)

// CallBodyTarget is Flow's formal, portable identity for one existing Body.
// It is deliberately independent of a mount, Link, or Call selector; the
// exact sealed Body boundary issues it, while Link later adds its ModuleKey.
type CallBodyTarget struct{ context identity.ContentID }

// CallBodyTarget returns the formal target for the Body owned by one existing
// Function boundary. The boundary is the issued callable proof; callers do
// not reconstruct a raw Body term or rejoin it at Program altitude.
func (view View) CallBodyTarget(function FunctionBoundary) (CallBodyTarget, bool) {
	if !view.available() || !function.Available() {
		return CallBodyTarget{}, false
	}
	boundaries := view.FunctionBoundaries()
	if !boundaries.OwnsFunction(function) {
		return CallBodyTarget{}, false
	}
	body, bodyOK := function.Body()
	boundary, boundaryOK := boundaries.ForBody(body)
	if !bodyOK || !boundaryOK || !boundary.Available() || !boundaries.OwnsBody(boundary) {
		return CallBodyTarget{}, false
	}
	target := CallBodyTarget{context: boundary.ContextID()}
	return target, target.Valid()
}

func (target CallBodyTarget) Valid() bool { return target.context.Available() }

func (target CallBodyTarget) ContextID() identity.ContentID {
	if !target.Valid() {
		return identity.ContentID{}
	}
	return target.context
}

const (
	callBodyTargetVersion uint64 = 1
	callBodyTargetTag            = "pcallbod"
)

// ID is the closed role/version identity of this formal. The preimage is
// fixed and contains only the Flow Body ContextID; no raw term or ordinal
// participates. Keep this codec byte-identical to the former Program query.
func (target CallBodyTarget) ID() (identity.ContentID, bool) {
	if !target.Valid() {
		return identity.ContentID{}, false
	}
	var payload [8 + 8 + 32]byte
	copy(payload[:8], callBodyTargetTag)
	binary.BigEndian.PutUint64(payload[8:16], callBodyTargetVersion)
	copy(payload[16:], target.context[:])
	id := identity.ContentID(sha256.Sum256(payload[:]))
	return id, id.Available()
}
