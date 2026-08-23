package factor

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/plane"
)

// The payload every published effect answer carries for the three arms below.
// A layout digest pins what the declaration is; these pin what the declaration
// produces, so a change in the walk that leaves the layout intact is caught
// here rather than by a consumer that can no longer read a stored answer.
const (
	exactPresentPayloadPin = "564d54b596ac18594897bc7ea807b6c41dbfb60d4a05e1b556a6b8e31a65a4c8"
	exactTopPayloadPin     = "9f4f97db7ac4aa6728b0f2e958c43129faa3b1e9a8c7d6f35e501e77cf139d04"
	exactAbsentPayloadPin  = "6633603f241c5e96cfcabe90bfd2c80fea03793f473ea3e8ce6148f15de8ea58"
)

// TestExactPublicationPayloadIsPinned states that the wire image of an effect
// answer is a function of the declaration alone. The family authors a
// projection and the plane driver writes it; neither may change the bytes a
// stored answer was written as.
func TestExactPublicationPayloadIsPinned(t *testing.T) {
	atoms := []identity.ContentID{effectCodecID(1), effectCodecID(67)}
	for _, pinned := range []struct {
		name        string
		observation EffectObservation
		length      int
		pin         string
	}{
		{"present", EffectObservation{Atoms: atoms, Rows: 1, Present: true, Valid: true, seal: sealAtoms(atoms)}, 130, exactPresentPayloadPin},
		{"top", EffectObservation{Rows: 1, Present: true, Top: true, Valid: true, seal: sealAtoms(nil)}, 66, exactTopPayloadPin},
		{"absent", EffectObservation{Valid: true}, 66, exactAbsentPayloadPin},
	} {
		_, _, payload, ok := plane.Publish(exactResultTestLayout, exactPublicationProjection, pinned.observation)
		if !ok {
			t.Fatalf("the %s fixture did not publish", pinned.name)
		}
		if len(payload) != pinned.length {
			t.Fatalf("%s payload = %d bytes, want the pinned %d", pinned.name, len(payload), pinned.length)
		}
		digest := sha256.Sum256(payload)
		if got := hex.EncodeToString(digest[:]); got != pinned.pin {
			t.Fatalf("%s payload digest = %s, want the pinned image %s", pinned.name, got, pinned.pin)
		}
	}
}
