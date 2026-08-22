package factor

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/plane"
)

// The wire offsets this family's payload is read at are derived from its
// sealed layout, never spelled: format, layout digest, row count, then the one
// unkeyed row record and its variable extent.
var (
	effectHeaderSize = 8 + 32 + 8
	effectStateAt    = effectHeaderSize
	effectTopAt      = effectHeaderSize + 1
	effectOffsetsAt  = effectHeaderSize + ExactResultLayout.RowWidth()
	effectTailAt     = effectOffsetsAt + 2*8
)

// TestExactResultLayoutSeals states that the family's declaration is
// admissible: an unsealed layout would refuse every answer at publication.
func TestExactResultLayoutSeals(t *testing.T) {
	if !ExactResultLayout.Available() || ExactResultLayout.Family() != ExactResultFamily {
		t.Fatal("the effect-exact layout did not seal")
	}
	if ExactResultLayout.RowWidth() != 2 {
		t.Fatalf("row width = %d, want the state byte plus the top flag", ExactResultLayout.RowWidth())
	}
	variable, declared := ExactResultLayout.Variable()
	if !declared || variable != ExactColumnAtoms {
		t.Fatalf("variable column = %d/%v, want the declared atom vector", variable, declared)
	}
}

func effectCodecID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func TestEncodeEffectResultDropsPrivateAlgebraAccumulator(t *testing.T) {
	atoms := []identity.ContentID{effectCodecID(1), effectCodecID(67)}
	observation := EffectObservation{Atoms: atoms, Rows: 1, Present: true, Valid: true, seal: sealAtoms(atoms), joined: Value{}}
	present, rows, payload, ok := EncodeResult(observation)
	if !ok || !present || rows != 1 || binary.BigEndian.Uint64(payload[:8]) != plane.Format ||
		binary.BigEndian.Uint64(payload[effectOffsetsAt+8:effectOffsetsAt+16]) != 2*32 {
		t.Fatal("effect result codec refused canonical observation")
	}
	before := append([]byte(nil), payload...)
	observation.Atoms[0][0] ^= 0xff
	if string(payload) != string(before) {
		t.Fatal("encoded Effect payload aliases the observation")
	}
}

func TestEncodeEffectResultRejectsUnauthenticatedProjection(t *testing.T) {
	observation := EffectObservation{Atoms: []identity.ContentID{effectCodecID(1)}, Rows: 1, Present: true, Valid: true}
	if _, _, _, ok := EncodeResult(observation); ok {
		t.Fatal("unauthenticated Effect atom projection encoded")
	}
}

func TestEncodeEffectResultAcceptsAbsentWithoutSeal(t *testing.T) {
	for _, test := range []struct {
		name string
		rows uint32
	}{
		{name: "one row", rows: 1},
		{name: "zero rows", rows: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			observation := EffectObservation{Rows: test.rows, Valid: true}
			present, rows, payload, ok := EncodeResult(observation)
			if !ok || present || rows != uint64(test.rows) || len(payload) != effectTailAt {
				t.Fatalf("EncodeResult absent observation = present:%v rows:%d payload:%d ok:%v", present, rows, len(payload), ok)
			}
			if binary.BigEndian.Uint64(payload[:8]) != plane.Format || payload[effectStateAt] != 0 || payload[effectTopAt] != 0 ||
				binary.BigEndian.Uint64(payload[effectOffsetsAt+8:effectOffsetsAt+16]) != 0 {
				t.Fatal("EncodeResult produced invalid absent result payload")
			}
		})
	}
}
