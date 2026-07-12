// Package observation defines lowering-owned, neutral diagnostic evidence
// occurrence identities. Durable artifact projection composes these body-local
// IDs with the canonical StaticArtifactID/debug-map digest.
package observation

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

type Kind uint8

const (
	Invalid Kind = iota
	CallInvocation
	Assignment
	CallArgument
	CallResult
)

type Occurrence struct {
	Point wir.DebugPointID
	Kind  Kind
	Slot  uint32
}

func (o Occurrence) Valid() bool { return o.Point.Valid() && o.Kind > Invalid && o.Kind <= CallResult }

func (o Occurrence) Less(other Occurrence) bool {
	if o.Point.Ordinal != other.Point.Ordinal {
		return o.Point.Ordinal < other.Point.Ordinal
	}
	if o.Point.Phase != other.Point.Phase {
		return o.Point.Phase < other.Point.Phase
	}
	if o.Kind != other.Kind {
		return o.Kind < other.Kind
	}
	return o.Slot < other.Slot
}

type InvocationID [sha256.Size]byte

// ExtendInvocation is valid only for finite acyclic provenance. Recursive SCC
// activation requires a later cycle-normalized observer closure; semantic
// admission remains hard-false in this tranche.
func ExtendInvocation(parent InvocationID, caller lexicalidentity.StableLexicalBodyID, call Occurrence) (InvocationID, bool) {
	if caller == (lexicalidentity.StableLexicalBodyID{}) || !call.Valid() || call.Kind != CallInvocation {
		return InvocationID{}, false
	}
	h := sha256.New()
	_, _ = h.Write([]byte("wippy.observation.invocation.v1"))
	_, _ = h.Write(parent[:])
	_, _ = h.Write(caller[:])
	var raw [10]byte
	binary.BigEndian.PutUint32(raw[:4], call.Point.Ordinal)
	raw[4], raw[5] = byte(call.Point.Phase), byte(call.Kind)
	binary.BigEndian.PutUint32(raw[6:], call.Slot)
	_, _ = h.Write(raw[:])
	var out InvocationID
	copy(out[:], h.Sum(nil))
	return out, true
}
