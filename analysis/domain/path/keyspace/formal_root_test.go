package keyspace

import (
	"bytes"
	"context"
	"math"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func TestFormalRootInterningIsExactAndKeySpaceLocal(t *testing.T) {
	firstOwner := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("keyspace-formal-owner")), 1)
	secondOwner := firstOwner
	secondOwner[len(secondOwner)-1] ^= 1 // prove the complete 32-byte owner participates
	wide := uint64(math.MaxUint32) + 23
	firstRoot := formal.NewRoot(firstOwner, wide, formal.Input)
	variants := []formal.Root{
		firstRoot,
		formal.NewRoot(firstOwner, wide, formal.Output),
		formal.NewRoot(firstOwner, wide+1, formal.Input),
		formal.NewRoot(secondOwner, wide, formal.Input),
	}

	left := New()
	leftKeys := make([]Key, len(variants))
	for index, root := range variants {
		key, ok := left.InternFormalRoot(root)
		if !ok {
			t.Fatalf("InternFormalRoot(%d)", index)
		}
		leftKeys[index] = key
		if got, ok := left.DescribeFormalRoot(key); !ok || got != root {
			t.Fatalf("formal descriptor %d = %#v/%t", index, got, ok)
		}
	}
	seen := make(map[Key]struct{}, len(leftKeys))
	for _, key := range leftKeys {
		seen[key] = struct{}{}
	}
	if len(seen) != len(leftKeys) {
		t.Fatal("owner, ordinal, or vocabulary variants collided")
	}

	// Perturb dense insertion order. Structural import and snapshot bytes must
	// remain identical even though Key.Root is authority-local.
	right := New()
	for index := len(variants) - 1; index >= 0; index-- {
		if _, ok := right.InternFormalRoot(variants[index]); !ok {
			t.Fatal("right formal root")
		}
	}
	imported, ok := right.ImportKey(left, leftKeys[0])
	if !ok || imported.Root == leftKeys[0].Root {
		t.Fatalf("cross-KeySpace import retained dense root: %#v/%#v", leftKeys[0], imported)
	}
	if got, ok := right.DescribeFormalRoot(imported); !ok || got != firstRoot {
		t.Fatalf("imported descriptor = %#v/%t", got, ok)
	}
	leftSnapshot, err := FreezeSnapshot(context.Background(), left, leftKeys)
	if err != nil {
		t.Fatal(err)
	}
	rightKeys := make([]Key, len(variants))
	for index, root := range variants {
		rightKeys[index], _ = right.InternFormalRoot(root)
	}
	rightSnapshot, err := FreezeSnapshot(context.Background(), right, rightKeys)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshotBytes(t, leftSnapshot), snapshotBytes(t, rightSnapshot)) {
		t.Fatal("formal canonical snapshot depends on dense interning order")
	}
	wantRoots := make(map[formal.Root]struct{}, len(variants))
	for _, root := range variants {
		wantRoots[root] = struct{}{}
	}
	for index := 0; index < leftSnapshot.Len(); index++ {
		leftRoot, leftOK := leftSnapshot.KeyAt(index).FormalRoot()
		rightRoot, rightOK := rightSnapshot.KeyAt(index).FormalRoot()
		if _, expected := wantRoots[leftRoot]; !leftOK || !rightOK || !expected || leftRoot != rightRoot {
			t.Fatalf("snapshot formal roots %d = %#v/%t and %#v/%t", index, leftRoot, leftOK, rightRoot, rightOK)
		}
	}
}

func TestFormalRootSuffixOrderFormatAndNamedNamespaceAreDisjoint(t *testing.T) {
	space := New()
	owner := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte("keyspace-formal-format")), 1)
	root := formal.NewRoot(owner, math.MaxUint64, formal.Middle)
	key, ok := space.InternFormalRoot(root)
	if !ok {
		t.Fatal("formal root")
	}
	child, ok := space.AppendSegment(key, segment.Segment{Kind: segment.SegmentField, Name: "member"})
	if !ok || !space.HasPrefix(child, key) || !space.Less(key, child) {
		t.Fatal("formal root did not participate in structural suffix/order laws")
	}
	spelling := string(space.FormatReadOnly(key))
	if !strings.HasPrefix(spelling, formalRootPrefix) || !strings.Contains(spelling, owner.String()) {
		t.Fatalf("formal format lost complete descriptor: %q", spelling)
	}
	named := space.FromPath(pathdom.Path{Root: spelling})
	if named == key || space.FormatReadOnly(named) == space.FormatReadOnly(key) {
		t.Fatal("user named root collided with private typed formal namespace")
	}
	if _, ok := space.DescribeFormalRoot(named); ok {
		t.Fatal("named spelling was decoded as typed formal identity")
	}
}

func TestFormalRootStateKeyRoundTripRetainsTypedDescriptor(t *testing.T) {
	space := New()
	owner := lexicalidentity.StableLexicalBodyID{1, 2, 3, 4}
	want := formal.NewRoot(owner, 37, formal.Middle)
	root, ok := space.InternFormalRoot(want)
	if !ok {
		t.Fatal("InternFormalRoot")
	}
	child, ok := space.appendSegments(root, []segment.Segment{{Kind: segment.SegmentField, Name: "member"}})
	if !ok {
		t.Fatal("append formal suffix")
	}
	roundTrip, ok := space.FromStateKey(space.FormatReadOnly(child))
	if !ok {
		t.Fatal("formal StateKey round trip rejected")
	}
	if got, exact := space.DescribeFormalRoot(roundTrip); !exact || got != want {
		t.Fatalf("formal descriptor = %#v/%t, want %#v", got, exact, want)
	}
	if !space.HasPrefix(roundTrip, root) {
		t.Fatal("formal suffix was not retained")
	}
}

func TestMalformedReservedFormalRootStateKeyFailsClosed(t *testing.T) {
	space := New()
	for _, malformed := range []pathdom.PathKey{
		pathdom.PathKey(formalRootPrefix + "00:in:0000000000000001"),
		pathdom.PathKey(formalRootPrefix + strings.Repeat("0", 64) + ":invalid:0000000000000001"),
		pathdom.PathKey(formalRootPrefix + strings.Repeat("0", 64) + ":in:0000000000000000"),
		pathdom.PathKey(formalRootPrefix + "A" + strings.Repeat("0", 63) + ":in:0000000000000001"),
	} {
		if got, ok := space.FromStateKey(malformed); ok || got != (Key{}) {
			t.Fatalf("FromStateKey(%q) = %#v/%t, want rejected", malformed, got, ok)
		}
	}
}

func TestFormalRootDenseIndexRejectsOverflowBeforeCast(t *testing.T) {
	if id, ok := nextFormalRootID(math.MaxUint32); !ok || id != math.MaxUint32 {
		t.Fatalf("last representable dense id = %d/%t", id, ok)
	}
	if id, ok := nextFormalRootID(uint64(math.MaxUint32) + 1); ok || id != 0 {
		t.Fatalf("overflow dense id = %d/%t", id, ok)
	}
}
