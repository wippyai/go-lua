package program

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/program/keyspace"
)

const (
	callBodyTargetVersion uint64 = 1
	callBodyTargetTag            = "pcallbod"
)

// CallBodyTarget is Program's formal, portable identity for one existing
// Body. It is deliberately independent of a mount, Link, or Call selector;
// the exact Body proof issues it, while Link later adds its ModuleKey.
type CallBodyTarget struct{ context keyspace.ContentID }

// CallTarget returns the formal Call target for this exact Program Body.
func (body Body) CallTarget() (CallBodyTarget, bool) {
	if !body.Available() || !body.ContextID().Available() {
		return CallBodyTarget{}, false
	}
	target := CallBodyTarget{context: body.ContextID()}
	return target, target.Valid()
}

func (target CallBodyTarget) Valid() bool { return target.context.Available() }

func (target CallBodyTarget) ContextID() keyspace.ContentID {
	if !target.Valid() {
		return keyspace.ContentID{}
	}
	return target.context
}

// ID is the closed role/version identity of this formal. The preimage is
// fixed and contains only the Program Body ContextID; no raw term or ordinal
// participates.
func (target CallBodyTarget) ID() (keyspace.ContentID, bool) {
	if !target.Valid() {
		return keyspace.ContentID{}, false
	}
	var payload [8 + 8 + 32]byte
	copy(payload[:8], callBodyTargetTag)
	binary.BigEndian.PutUint64(payload[8:16], callBodyTargetVersion)
	copy(payload[16:], target.context[:])
	id := keyspace.ContentID(sha256.Sum256(payload[:]))
	return id, id.Available()
}
