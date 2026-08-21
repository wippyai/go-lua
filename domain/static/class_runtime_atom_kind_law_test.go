package static

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/typ"
)

// TestRuntimeAtomKindProjectionSeparatesAbstentionFromCorruption states which
// atom earns the whole runtime vocabulary and which one earns nothing at all.
//
// An open atom publishes no canonical encoding. It has stated no structure, so
// every runtime family remains possible and All is the sound answer, the same
// abstention an opaque Class row earns.
//
// An atom that publishes an encoding has stated its structure. If that
// encoding does not decode, the seal has learned nothing about the atom and
// cannot answer for it: All would be a claim the atom never made, and it is the
// one mask under which every kind test passes. The projection must refuse.
func TestRuntimeAtomKindProjectionSeparatesAbstentionFromCorruption(t *testing.T) {
	t.Run("open atom abstains", func(t *testing.T) {
		mask, err := runtimeAtomKindMask(nil, false)
		if err != nil {
			t.Fatalf("open atom projection = %v, want the whole vocabulary", err)
		}
		if mask != runtimekind.All {
			t.Fatalf("open atom projection = %d, want %d", mask, runtimekind.All)
		}
	})
	t.Run("published encoding is projected", func(t *testing.T) {
		encoded, err := typ.EncodeCanonical(context.Background(), typ.String)
		if err != nil {
			t.Fatalf("encode a closed atom: %v", err)
		}
		mask, err := runtimeAtomKindMask(encoded, true)
		if err != nil {
			t.Fatalf("closed atom projection: %v", err)
		}
		if want := runtimekind.Bit(runtimekind.String); mask != want {
			t.Fatalf("closed string atom projection = %d, want %d", mask, want)
		}
	})
	t.Run("undecodable encoding refuses", func(t *testing.T) {
		mask, err := runtimeAtomKindMask([]byte("this is not a canonical type encoding"), true)
		if err == nil {
			t.Fatalf("undecodable atom projection = %d, want a refusal", mask)
		}
		if mask == runtimekind.All {
			t.Fatalf("undecodable atom projection returned the whole vocabulary %d, which proves membership in every runtime family", mask)
		}
	})
}
