package keyspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"
	"unsafe"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

func TestSnapshotCanonicalBytesIgnoreInternAndInsertionOrder(t *testing.T) {
	leftSpace, leftKeys := snapshotFixture(t, false)
	rightSpace, rightKeys := snapshotFixture(t, true)
	left, err := FreezeSnapshot(context.Background(), leftSpace, leftKeys)
	if err != nil {
		t.Fatal(err)
	}
	right, err := FreezeSnapshot(context.Background(), rightSpace, rightKeys)
	if err != nil {
		t.Fatal(err)
	}
	if left.Len() != right.Len() || left.Len() != 10 {
		t.Fatalf("snapshot lengths = %d/%d, want 10 unique keys", left.Len(), right.Len())
	}
	if !bytes.Equal(snapshotBytes(t, left), snapshotBytes(t, right)) {
		t.Fatal("equivalent independent keyspaces encoded differently")
	}
	for index := 0; index < left.Len(); index++ {
		assertSnapshotKeysEqual(t, left.KeyAt(index), right.KeyAt(index))
	}
}

func TestSnapshotPreservesAdversarialStructuralIdentity(t *testing.T) {
	space := New()
	fieldDot, _ := space.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "a.b"}})
	twoFields, _ := space.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "a"}, {Kind: segment.SegmentField, Name: "b"}})
	field, _ := space.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "name"}})
	stringIndex, _ := space.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentIndexString, Name: "name"}})
	resolver, _ := space.FromResolverKey(7, 3, []segment.Segment{{Kind: segment.SegmentField, Name: "x"}})
	resolverNext := resolver
	resolverNext.Ver = 4
	resolverMax := space.ownKey(Key{Kind: KindResolverSym, Sym: math.MaxUint64, Ver: math.MaxUint32})
	unversioned, _ := space.FromResolverKey(7, 0, []segment.Segment{{Kind: segment.SegmentField, Name: "x"}})
	stable, _ := space.FromStableSymbol(7, []segment.Segment{{Kind: segment.SegmentField, Name: "x"}})
	placeholder := space.FromPath(pathdom.Path{Root: "$0", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "x"}}})
	placeholderMax := space.ownKey(Key{Kind: KindPlaceholder, Root: math.MaxUint32})
	ret := space.FromPath(pathdom.Path{Root: "ret[0]", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "x"}}})
	retMax := space.ownKey(Key{Kind: KindRetSlot, Root: math.MaxUint32})
	named := space.FromPath(pathdom.Path{Root: "sym7@3", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "x"}}})
	namedCanonical := named
	namedCanonical.Canon = true

	keys := []Key{
		fieldDot, twoFields, field, stringIndex,
		resolver, resolverNext, resolverMax, unversioned, stable,
		placeholder, placeholderMax, ret, retMax, named, namedCanonical,
	}
	snapshot, err := FreezeSnapshot(context.Background(), space, keys)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Len() != len(keys) {
		t.Fatalf("snapshot collapsed adversarial identities: got %d want %d", snapshot.Len(), len(keys))
	}
	for left := range keys {
		oneLeft, err := FreezeSnapshot(context.Background(), space, []Key{keys[left]})
		if err != nil {
			t.Fatal(err)
		}
		for right := left + 1; right < len(keys); right++ {
			oneRight, err := FreezeSnapshot(context.Background(), space, []Key{keys[right]})
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(snapshotBytes(t, oneLeft), snapshotBytes(t, oneRight)) {
				t.Fatalf("structural keys %d and %d encoded identically", left, right)
			}
		}
	}

	minIndex, _ := space.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentIndexInt, Index: math.MinInt}})
	maxIndex, _ := space.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentIndexInt, Index: math.MaxInt}})
	indexed, err := FreezeSnapshot(context.Background(), space, []Key{maxIndex, minIndex})
	if err != nil || indexed.Len() != 2 || indexed.KeyAt(0).SegmentAt(0).Index != math.MinInt || indexed.KeyAt(1).SegmentAt(0).Index != math.MaxInt {
		t.Fatalf("signed integer segment identity/order lost: %#v, %v", indexed, err)
	}
}

func TestSnapshotOwnsSourceStorage(t *testing.T) {
	space := New()
	key := space.FromPath(pathdom.Path{Root: "mutable", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "before"}}})
	snapshot, err := FreezeSnapshot(context.Background(), space, []Key{key})
	if err != nil {
		t.Fatal(err)
	}
	want := snapshotBytes(t, snapshot)

	borrowed, ok := space.SegmentsView(key)
	if !ok {
		t.Fatal("source segment view missing")
	}
	borrowed[0] = segment.Segment{Kind: segment.SegmentIndexInt, Index: 99}
	space.rootEntries[rootID(key.Root)].name = "changed"
	key.Kind = KindRootlessSuffix

	if got := snapshotBytes(t, snapshot); !bytes.Equal(got, want) {
		t.Fatal("source mutation changed frozen snapshot")
	}
	item := snapshot.KeyAt(0).SegmentAt(0)
	item.Name = "caller-change"
	if got := snapshot.KeyAt(0).SegmentAt(0).Name; got != "before" {
		t.Fatalf("returned segment mutation changed snapshot: %q", got)
	}
}

func TestSnapshotCancellationReturnsNoSnapshotOrBytes(t *testing.T) {
	space, keys := benchmarkSnapshotKeys(t, 128)
	ctx := &cancelAfterChecks{cancelAt: 5}
	snapshot, err := FreezeSnapshot(ctx, space, keys)
	if !errors.Is(err, context.Canceled) || snapshot.Len() != 0 {
		t.Fatalf("FreezeSnapshot cancellation = len %d, %v", snapshot.Len(), err)
	}

	complete, err := FreezeSnapshot(context.Background(), space, keys[:8])
	if err != nil {
		t.Fatal(err)
	}
	encodeCtx, cancel := context.WithCancel(context.Background())
	var writer canonical.Writer
	if err := writer.ResetBuffer(encodeCtx, "keyspace.snapshot", 1); err != nil {
		t.Fatal(err)
	}
	primeSnapshotCancellation(t, &writer)
	cancel()
	if err := complete.EncodeCanonical(&writer); !errors.Is(err, context.Canceled) {
		t.Fatalf("EncodeCanonical error = %v", err)
	}
	if encoded, err := writer.FinishBytes(); !errors.Is(err, context.Canceled) || encoded != nil {
		t.Fatalf("FinishBytes after cancellation = %x, %v", encoded, err)
	}
}

func TestSnapshotRejectsForeignAndMalformedKeys(t *testing.T) {
	owner := New()
	foreign := New()
	foreignKey, _ := foreign.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "member"}})
	if snapshot, err := FreezeSnapshot(context.Background(), owner, []Key{foreignKey}); err == nil || snapshot.Len() != 0 {
		t.Fatalf("foreign key snapshot = %#v, %v", snapshot, err)
	}
	malformed := Key{Kind: KindResolverSym, Sym: 1, Ver: 1, Canon: true}
	if snapshot, err := FreezeSnapshot(context.Background(), owner, []Key{malformed}); err == nil || snapshot.Len() != 0 {
		t.Fatalf("malformed key snapshot = %#v, %v", snapshot, err)
	}
}

func TestSnapshotRejectsForeignKeysWithCollidingDenseIDs(t *testing.T) {
	owner := New()
	foreign := New()

	ownerSegment, _ := owner.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "owner-member"}})
	foreignSegment, _ := foreign.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "foreign-member"}})
	if ownerSegment.Segs != foreignSegment.Segs {
		t.Fatalf("test setup did not collide segment ids: %d != %d", ownerSegment.Segs, foreignSegment.Segs)
	}
	if snapshot, err := FreezeSnapshot(context.Background(), owner, []Key{foreignSegment}); err == nil || snapshot.Len() != 0 {
		t.Fatalf("foreign colliding segment key snapshot = %#v, %v", snapshot, err)
	}

	ownerNamed := owner.FromPath(pathdom.Path{Root: "owner-root"})
	foreignNamed := foreign.FromPath(pathdom.Path{Root: "foreign-root"})
	if ownerNamed.Root != foreignNamed.Root {
		t.Fatalf("test setup did not collide root ids: %d != %d", ownerNamed.Root, foreignNamed.Root)
	}
	if snapshot, err := FreezeSnapshot(context.Background(), owner, []Key{foreignNamed}); err == nil || snapshot.Len() != 0 {
		t.Fatalf("foreign colliding named-root key snapshot = %#v, %v", snapshot, err)
	}
}

func TestKeySpaceShallowCopyCannotMintOrUseKeys(t *testing.T) {
	source := New()
	sourceSegment, _ := source.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "source-member"}})
	sourceNamed := source.FromPath(pathdom.Path{Root: "source-root"})

	segmentClone := *source
	segmentClone.segEntries = append([]segmentsEntry(nil), source.segEntries[:1]...)
	segmentClone.segByKey = make(map[string]SegmentsID)
	segmentClone.segByOne = make(map[segment.Segment]SegmentsID)
	segmentClone.segByTwo = make(map[segmentPairKey]SegmentsID)
	segmentClone.segByThree = make(map[segmentTripleKey]SegmentsID)
	segmentClone.formatByKey = make(map[Key]string)
	cloneSegment, ok := segmentClone.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "clone-member"}})
	if ok || cloneSegment != (Key{}) {
		t.Fatalf("shallow-copy segment key minted with original authority: %#v/%v", cloneSegment, ok)
	}
	if segmentClone.Format(sourceSegment) != "" {
		t.Fatal("shallow copy used an original-space segment key")
	}
	if imported, ok := segmentClone.ImportKey(source, sourceSegment); ok || imported != (Key{}) {
		t.Fatalf("shallow copy imported with original authority: %#v/%v", imported, ok)
	}

	rootClone := *source
	rootClone.rootEntries = append([]rootEntry(nil), source.rootEntries[:1]...)
	rootClone.rootByName = make(map[string]rootID)
	rootClone.formatByKey = make(map[Key]string)
	cloneNamed := rootClone.FromPath(pathdom.Path{Root: "clone-root"})
	if cloneNamed != (Key{}) {
		t.Fatalf("shallow-copy named key minted with original authority: %#v", cloneNamed)
	}
	if rootClone.Format(sourceNamed) != "" {
		t.Fatal("shallow copy used an original-space named key")
	}
	if snapshot, err := FreezeSnapshot(context.Background(), &rootClone, []Key{sourceNamed}); err == nil || snapshot.Len() != 0 {
		t.Fatalf("shallow-copy source snapshot = %#v, %v", snapshot, err)
	}
}

func TestKeyProvenanceSurvivesCopiesAndTransforms(t *testing.T) {
	space := New()
	base := space.FromPath(pathdom.Path{Root: "root", Segments: []segment.Segment{{Kind: segment.SegmentIndexString, Name: "member"}}})
	copied := base
	if _, err := FreezeSnapshot(context.Background(), space, []Key{copied}); err != nil {
		t.Fatalf("same-owner copy rejected: %v", err)
	}

	canonicalKey, ok := space.FieldCanonical(copied)
	if !ok || !space.validKey(canonicalKey) {
		t.Fatalf("field canonicalization lost provenance: %#v/%v", canonicalKey, ok)
	}
	appended, ok := space.AppendSegment(canonicalKey, segment.Segment{Kind: segment.SegmentField, Name: "child"})
	if !ok || !space.validKey(appended) {
		t.Fatalf("append lost provenance: %#v/%v", appended, ok)
	}

	from := space.FromPath(pathdom.Path{Root: "root", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "member"}}})
	to := space.FromPath(pathdom.Path{Root: "other"})
	rebased, ok := space.Rebase(appended, from, to)
	if !ok || !space.validKey(rebased) {
		t.Fatalf("rebase lost provenance: %#v/%v", rebased, ok)
	}

	destination := New()
	imported, ok := destination.ImportKey(space, rebased)
	if !ok || !destination.validKey(imported) {
		t.Fatalf("import did not rebind provenance: %#v/%v", imported, ok)
	}
	if space.validKey(imported) || space.Format(imported) != "" {
		t.Fatal("imported key retained or forged source provenance")
	}
}

func TestKeyProvenanceDoesNotGrowHotMapKey(t *testing.T) {
	if got := unsafe.Sizeof(Key{}); got != 32 {
		t.Fatalf("sizeof(Key) = %d, want preserved 32-byte hot map key", got)
	}
}

func TestSnapshotSealDistinguishesSuccessfulEmptyFromZero(t *testing.T) {
	var zeroWriter canonical.Writer
	if err := zeroWriter.ResetBuffer(context.Background(), "keyspace.snapshot", 1); err != nil {
		t.Fatal(err)
	}
	if err := (Snapshot{}).EncodeCanonical(&zeroWriter); err == nil {
		t.Fatal("unsealed zero snapshot encoded with authority")
	}

	empty, err := FreezeSnapshot(context.Background(), New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if encoded := snapshotBytes(t, empty); len(encoded) == 0 {
		t.Fatal("successfully sealed empty snapshot did not encode")
	}
}

func TestSnapshotConcurrentReads(t *testing.T) {
	space, keys := benchmarkSnapshotKeys(t, 64)
	snapshot, err := FreezeSnapshot(context.Background(), space, keys)
	if err != nil {
		t.Fatal(err)
	}
	want := snapshotBytes(t, snapshot)

	const readers = 16
	var group sync.WaitGroup
	group.Add(readers)
	for reader := 0; reader < readers; reader++ {
		go func() {
			defer group.Done()
			for pass := 0; pass < 100; pass++ {
				for index := 0; index < snapshot.Len(); index++ {
					key := snapshot.KeyAt(index)
					for segmentIndex := 0; segmentIndex < key.SegmentLen(); segmentIndex++ {
						_ = key.SegmentAt(segmentIndex)
					}
				}
				got, err := encodeSnapshotBytes(snapshot)
				if err != nil {
					t.Errorf("concurrent snapshot encoding: %v", err)
					return
				}
				if !bytes.Equal(got, want) {
					t.Errorf("concurrent snapshot bytes changed")
					return
				}
			}
		}()
	}
	group.Wait()
}

func TestSnapshotPublicSurfaceHasNoRawStorageEscape(t *testing.T) {
	for _, value := range []any{Snapshot{}, SnapshotKey{}} {
		typeOf := reflect.TypeOf(value)
		for fieldIndex := 0; fieldIndex < typeOf.NumField(); fieldIndex++ {
			field := typeOf.Field(fieldIndex)
			switch field.Type.Kind() {
			case reflect.Slice, reflect.Map, reflect.Pointer, reflect.UnsafePointer:
				if field.IsExported() {
					t.Fatalf("%s.%s exposes raw storage", typeOf, field.Name)
				}
			}
		}
		for methodIndex := 0; methodIndex < typeOf.NumMethod(); methodIndex++ {
			method := typeOf.Method(methodIndex)
			for output := 0; output < method.Type.NumOut(); output++ {
				switch method.Type.Out(output).Kind() {
				case reflect.Slice, reflect.Map, reflect.Pointer, reflect.UnsafePointer:
					t.Fatalf("%s.%s returns raw storage %s", typeOf, method.Name, method.Type.Out(output))
				}
			}
		}
	}
}

var (
	benchmarkSnapshotSink Snapshot
	benchmarkSnapshotSum  uint64
	benchmarkKeySink      Key
	benchmarkBoolSink     bool
	benchmarkUintSink     uint64
)

func BenchmarkKeyProvenanceHotPath(b *testing.B) {
	space := New()
	segments := []segment.Segment{{Kind: segment.SegmentField, Name: "member"}}
	key, ok := space.FromStableSymbol(42, segments)
	if !ok {
		b.Fatal("failed to build benchmark key")
	}
	values := map[Key]uint64{key: 7}

	b.Run("construct-cached", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			benchmarkKeySink, _ = space.FromStableSymbol(42, segments)
		}
	})
	b.Run("validate", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			benchmarkBoolSink = space.validKey(key)
		}
	})
	b.Run("map-lookup", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			benchmarkUintSink = values[key]
		}
	})
}

func BenchmarkFreezeSnapshot(b *testing.B) {
	for _, count := range []int{1, 8, 64} {
		space, keys := benchmarkSnapshotKeys(b, count)
		b.Run(fmt.Sprintf("keys=%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				snapshot, err := FreezeSnapshot(context.Background(), space, keys)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkSnapshotSink = snapshot
			}
		})
	}
}

func BenchmarkSnapshotTraversal(b *testing.B) {
	space, keys := benchmarkSnapshotKeys(b, 64)
	snapshot, err := FreezeSnapshot(context.Background(), space, keys)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	var sum uint64
	for range b.N {
		for index := 0; index < snapshot.Len(); index++ {
			key := snapshot.KeyAt(index)
			sum += uint64(key.Kind()) + uint64(key.Symbol()) + uint64(key.Version()) + uint64(key.RootIndex())
			for segmentIndex := 0; segmentIndex < key.SegmentLen(); segmentIndex++ {
				item := key.SegmentAt(segmentIndex)
				sum += uint64(item.Kind) + uint64(item.Index)
			}
		}
	}
	benchmarkSnapshotSum = sum
}

func BenchmarkSnapshotCanonicalEncoding(b *testing.B) {
	space, keys := benchmarkSnapshotKeys(b, 16)
	snapshot, err := FreezeSnapshot(context.Background(), space, keys)
	if err != nil {
		b.Fatal(err)
	}
	var writer canonical.Writer
	b.ReportAllocs()
	for range b.N {
		if err := writer.Reset(context.Background(), io.Discard, "keyspace.snapshot", 1); err != nil {
			b.Fatal(err)
		}
		if err := snapshot.EncodeCanonical(&writer); err != nil {
			b.Fatal(err)
		}
		if err := writer.Finish(); err != nil {
			b.Fatal(err)
		}
	}
}

func snapshotFixture(t testing.TB, reverse bool) (*KeySpace, []Key) {
	t.Helper()
	space := New()
	builders := []func() Key{
		func() Key {
			key, _ := space.FromResolverKey(11, 3, []segment.Segment{{Kind: segment.SegmentField, Name: "member"}})
			return key
		},
		func() Key {
			key, _ := space.FromResolverKey(11, 0, []segment.Segment{{Kind: segment.SegmentIndexString, Name: "member"}})
			return key
		},
		func() Key {
			key, _ := space.FromStableSymbol(12, []segment.Segment{{Kind: segment.SegmentIndexInt, Index: -7}})
			return key
		},
		func() Key {
			return space.FromPath(pathdom.Path{Root: "$2", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "x"}}})
		},
		func() Key {
			key := space.FromPath(pathdom.Path{Root: "$2", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "x"}}})
			key.Canon = true
			return key
		},
		func() Key {
			return space.FromPath(pathdom.Path{Root: "ret[3]", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "x"}}})
		},
		func() Key {
			return space.FromPath(pathdom.Path{Root: "alpha", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "a.b"}}})
		},
		func() Key {
			key := space.FromPath(pathdom.Path{Root: "omega", Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "a"}, {Kind: segment.SegmentField, Name: "b"}}})
			key.Canon = true
			return key
		},
		func() Key {
			key, _ := space.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentField, Name: "raw"}})
			return key
		},
		func() Key {
			key, _ := space.FromRootlessSuffix([]segment.Segment{{Kind: segment.SegmentIndexString, Name: "raw"}})
			return key
		},
	}
	if reverse {
		for left, right := 0, len(builders)-1; left < right; left, right = left+1, right-1 {
			builders[left], builders[right] = builders[right], builders[left]
		}
	}
	keys := make([]Key, 0, len(builders)+1)
	for _, build := range builders {
		keys = append(keys, build())
	}
	keys = append(keys, keys[0])
	return space, keys
}

func benchmarkSnapshotKeys(t testing.TB, count int) (*KeySpace, []Key) {
	t.Helper()
	space := New()
	keys := make([]Key, count)
	for index := range keys {
		key, ok := space.FromRootlessSuffix([]segment.Segment{
			{Kind: segment.SegmentField, Name: fmt.Sprintf("field-%03d", count-index)},
			{Kind: segment.SegmentIndexInt, Index: index - count/2},
		})
		if !ok {
			t.Fatalf("failed to build benchmark key %d", index)
		}
		keys[index] = key
	}
	return space, keys
}

func snapshotBytes(t testing.TB, snapshot Snapshot) []byte {
	t.Helper()
	encoded, err := encodeSnapshotBytes(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func encodeSnapshotBytes(snapshot Snapshot) ([]byte, error) {
	var writer canonical.Writer
	if err := writer.ResetBuffer(context.Background(), "keyspace.snapshot", 1); err != nil {
		return nil, err
	}
	if err := snapshot.EncodeCanonical(&writer); err != nil {
		return nil, err
	}
	encoded, err := writer.FinishBytes()
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func assertSnapshotKeysEqual(t testing.TB, left, right SnapshotKey) {
	t.Helper()
	if left.Kind() != right.Kind() || left.Symbol() != right.Symbol() || left.Version() != right.Version() ||
		left.RootIndex() != right.RootIndex() || left.Canonical() != right.Canonical() || left.NamedRoot() != right.NamedRoot() ||
		left.SegmentLen() != right.SegmentLen() {
		t.Fatalf("snapshot keys differ: %#v/%#v", left, right)
	}
	for index := 0; index < left.SegmentLen(); index++ {
		if left.SegmentAt(index) != right.SegmentAt(index) {
			t.Fatalf("snapshot segment %d differs: %#v/%#v", index, left.SegmentAt(index), right.SegmentAt(index))
		}
	}
}

func primeSnapshotCancellation(t testing.TB, writer *canonical.Writer) {
	t.Helper()
	for range 60 {
		if err := writer.Nil(); err != nil {
			t.Fatal(err)
		}
	}
}

type cancelAfterChecks struct {
	checks   int
	cancelAt int
}

func (c *cancelAfterChecks) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *cancelAfterChecks) Done() <-chan struct{}       { return nil }
func (c *cancelAfterChecks) Value(any) any               { return nil }
func (c *cancelAfterChecks) Err() error {
	c.checks++
	if c.checks >= c.cancelAt {
		return context.Canceled
	}
	return nil
}

var _ context.Context = (*cancelAfterChecks)(nil)
