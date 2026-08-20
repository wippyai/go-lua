package factor

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

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
	if !ok || !present || rows != 1 || binary.BigEndian.Uint64(payload[:8]) != effectResultFormat || binary.BigEndian.Uint64(payload[10:18]) != 2 {
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
			if !ok || present || rows != uint64(test.rows) || len(payload) != effectResultHeaderSize {
				t.Fatalf("EncodeResult absent observation = present:%v rows:%d payload:%d ok:%v", present, rows, len(payload), ok)
			}
			if binary.BigEndian.Uint64(payload[:8]) != effectResultFormat || payload[8] != 0 || payload[9] != 0 || binary.BigEndian.Uint64(payload[10:18]) != 0 {
				t.Fatal("EncodeResult produced invalid absent result payload")
			}
		})
	}
}
