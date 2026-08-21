package static

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// This is an adversarial probe on the Runtime atom seal, not a unit suite for
// the projection. The seal reads one atom's canonical encoding and turns it
// into the may-set of runtime families a value of that atom can carry. That
// may-set is what every kind test, every discriminated-union narrowing, and
// every runtime guard the JIT is allowed to delete is decided against.
//
// The seal's stated law separates two answers. An atom that publishes no
// encoding has stated nothing, so All - every family remains possible - is the
// sound abstention. An atom that publishes an encoding has stated its
// structure, so an encoding that does not decode has to be refused: answering
// it with a mask would be a claim the atom never made.
//
// These probes attack the boundary between those two answers, and then attack
// the encoding itself.

func runtimeAtomProbeEncode(t *testing.T, value typ.Type) []byte {
	t.Helper()
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil {
		t.Fatalf("encode %s: %v", value, err)
	}
	if len(encoded) == 0 {
		t.Fatalf("encode %s produced no bytes", value)
	}
	return encoded
}

// TestRuntimeAtomSealRefusesCorruptedEncodings sweeps the shapes a damaged
// encoding actually arrives in. A truncated write, a payload with a tail
// appended, and a payload whose head was replaced are the three ways a stored
// or transmitted encoding degrades, and none of them announce themselves.
//
// Each one published an encoding, so each one falls under the seal's stated
// obligation to refuse rather than answer.
func TestRuntimeAtomSealRefusesCorruptedEncodings(t *testing.T) {
	encoded := runtimeAtomProbeEncode(t, typ.String)
	for _, probe := range []struct {
		name  string
		bytes []byte
	}{
		{name: "truncated to one byte", bytes: encoded[:1]},
		{name: "truncated by half", bytes: encoded[:len(encoded)/2]},
		{name: "truncated by its last byte", bytes: encoded[:len(encoded)-1]},
		{name: "trailing bytes appended", bytes: append(append([]byte(nil), encoded...), 0x7f, 0x7f, 0x7f)},
		{name: "leading bytes prepended", bytes: append([]byte{0x7f, 0x7f}, encoded...)},
		{name: "zero bytes", bytes: make([]byte, len(encoded))},
	} {
		t.Run(probe.name, func(t *testing.T) {
			if len(probe.bytes) == 0 {
				t.Fatalf("%s produced no bytes, so this probe publishes nothing to refuse", probe.name)
			}
			mask, err := runtimeAtomKindMask(probe.bytes, true)
			if err != nil {
				return
			}
			t.Fatalf("a published encoding %s projected to the may-set %d instead of being refused; the atom never stated that structure",
				probe.name, mask)
		})
	}
}

// TestRuntimeAtomSealRefusesASilentlyRewrittenEncoding is the sharper attack.
// The probes above damage an encoding into something unreadable. This one
// damages it into something readable but different: one bit is flipped and the
// bytes still decode, to another type, whose runtime families are not the ones
// the atom declared.
//
// Nothing is refused there, because nothing is wrong with the document - it is
// a well-formed encoding of the wrong type. That is the finding: the canonical
// encoding carries no witness of its own content, so a corrupted atom does not
// fail, it answers confidently with the wrong families. A value the atom said
// was a string is read as belonging to whatever family the flipped byte names,
// and every kind test and guard decision downstream is decided from it.
func TestRuntimeAtomSealRefusesASilentlyRewrittenEncoding(t *testing.T) {
	for _, probe := range []struct {
		name  string
		value typ.Type
	}{
		{name: "string", value: typ.String},
		{name: "number", value: typ.Number},
		{name: "boolean", value: typ.Boolean},
	} {
		t.Run(probe.name, func(t *testing.T) {
			encoded := runtimeAtomProbeEncode(t, probe.value)
			declared, err := runtimeAtomKindMask(encoded, true)
			if err != nil {
				t.Fatalf("the undamaged encoding of %s was refused: %v", probe.value, err)
			}
			rewritten := make([]string, 0)
			for position := range encoded {
				for bit := 0; bit < 8; bit++ {
					damaged := append([]byte(nil), encoded...)
					damaged[position] ^= 1 << bit
					mask, maskErr := runtimeAtomKindMask(damaged, true)
					if maskErr != nil || mask == declared {
						continue
					}
					rewritten = append(rewritten, fmt.Sprintf("byte %d bit %d: may-set %d, declared %d", position, bit, mask, declared))
				}
			}
			if len(rewritten) != 0 {
				t.Fatalf("%d single-bit changes to the encoding of %s decoded cleanly into a different runtime may-set:\n%s",
					len(rewritten), probe.value, strings.Join(rewritten, "\n"))
			}
		})
	}
}

// TestRuntimeAtomSealKeepsAStatedAtomDistinctFromAbstention pins the precision
// the seal exists to provide. All is the answer an atom that stated nothing
// earns. An atom that did state a closed primitive structure must project to
// that primitive's family alone: if it projects to All instead, its statement
// has been discarded and it is indistinguishable from the atom that made none,
// so no kind test against it can ever be decided.
//
// This is the direction a repair is most likely to break. Refusing corruption
// is easy to buy by widening what counts as corrupt and answering All for the
// rest; this probe is what makes that trade visible.
func TestRuntimeAtomSealKeepsAStatedAtomDistinctFromAbstention(t *testing.T) {
	for _, probe := range []struct {
		name  string
		value typ.Type
		want  runtimekind.Set
	}{
		{name: "string", value: typ.String, want: runtimekind.Bit(runtimekind.String)},
		{name: "number", value: typ.Number, want: runtimekind.Bit(runtimekind.Number)},
		{name: "boolean", value: typ.Boolean, want: runtimekind.Bit(runtimekind.Boolean)},
		{name: "nil", value: typ.Nil, want: runtimekind.Bit(runtimekind.Nil)},
	} {
		t.Run(probe.name, func(t *testing.T) {
			encoded := runtimeAtomProbeEncode(t, probe.value)
			mask, err := runtimeAtomKindMask(encoded, true)
			if err != nil {
				t.Fatalf("the encoding of the closed atom %s was refused: %v", probe.value, err)
			}
			if !mask.Valid() {
				t.Fatalf("the atom %s projected to %d, which is outside the closed runtime vocabulary", probe.value, mask)
			}
			if mask == runtimekind.All {
				t.Fatalf("the closed atom %s projected to the whole vocabulary, so it is read as an atom that stated nothing", probe.value)
			}
			if mask != probe.want {
				t.Fatalf("the closed atom %s projected to %d, want exactly %d", probe.value, mask, probe.want)
			}
		})
	}
}

// TestRuntimeAtomSealAbstainsOnlyForAnUnpublishedAtom pins the abstention side
// of the law so a later repair cannot buy refusal by widening what counts as
// corruption. An atom that published nothing has stated nothing and earns All;
// that is the same answer an opaque Class row earns and it stays.
func TestRuntimeAtomSealAbstainsOnlyForAnUnpublishedAtom(t *testing.T) {
	mask, err := runtimeAtomKindMask(nil, false)
	if err != nil {
		t.Fatalf("an atom that published no encoding was refused: %v", err)
	}
	if mask != runtimekind.All {
		t.Fatalf("an unpublished atom projected to %d, want the whole vocabulary %d", mask, runtimekind.All)
	}
}
