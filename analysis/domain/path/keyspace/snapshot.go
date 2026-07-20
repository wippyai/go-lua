package keyspace

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Snapshot is an immutable, solve-independent structural key collection.
// Keys are sorted and deduplicated by their complete structural identity;
// solve-local root and segment intern ids are never retained.
type Snapshot struct {
	keys  []SnapshotKey
	valid bool
}

// SnapshotKey is one immutable structural key in a Snapshot. Its variable-size
// storage is private and exposed only through allocation-free scalar accessors.
type SnapshotKey struct {
	kind       KeyKind
	sym        symbol.ID
	version    uint32
	root       uint32
	canon      bool
	namedRoot  string
	formalRoot formal.Root
	segments   []segment.Segment
}

// canonicalSnapshotKeyKindCount is deliberately independent of keyKindCount:
// the paired compile-time array bounds make key-namespace growth fail until
// this canonical owner is reviewed and updated.
const canonicalSnapshotKeyKindCount = 10

var (
	_ [canonicalSnapshotKeyKindCount - int(keyKindCount)]struct{}
	_ [int(keyKindCount) - canonicalSnapshotKeyKindCount]struct{}
)

// FreezeKey validates and decodes one key into its immutable, solve-independent
// structural identity. Callers must encode SnapshotKey's scalar fields; they
// must not interpret private KeyKind values or dense owner-local root IDs.
func FreezeKey(ctx context.Context, source *KeySpace, key Key) (SnapshotKey, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return SnapshotKey{}, err
	}
	if source == nil {
		return SnapshotKey{}, fmt.Errorf("keyspace: nil key source")
	}
	return freezeSnapshotKey(ctx, source, key)
}

// FreezeSnapshot validates keys against source, deep-copies their structural
// identity, and returns a canonically sorted unique snapshot. Cancellation or
// invalid/foreign input returns the zero snapshot and no partial authority.
func FreezeSnapshot(ctx context.Context, source *KeySpace, keys []Key) (Snapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if source == nil {
		return Snapshot{}, fmt.Errorf("keyspace: nil snapshot source")
	}
	if len(keys) == 0 {
		return Snapshot{valid: true}, nil
	}

	owned := make([]SnapshotKey, len(keys))
	for index, key := range keys {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return Snapshot{}, err
			}
		}
		frozen, err := FreezeKey(ctx, source, key)
		if err != nil {
			return Snapshot{}, err
		}
		owned[index] = frozen
	}
	if err := sortSnapshotKeys(ctx, owned); err != nil {
		return Snapshot{}, err
	}

	unique := 0
	for index := range owned {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return Snapshot{}, err
			}
		}
		if unique != 0 && compareSnapshotKeys(owned[unique-1], owned[index]) == 0 {
			continue
		}
		owned[unique] = owned[index]
		unique++
	}
	clear(owned[unique:])
	owned = owned[:unique]
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	return Snapshot{keys: owned, valid: true}, nil
}

func freezeSnapshotKey(ctx context.Context, source *KeySpace, key Key) (SnapshotKey, error) {
	if key.Kind == KindInvalid || !source.validKey(key) {
		return SnapshotKey{}, fmt.Errorf("keyspace: invalid or foreign snapshot key")
	}
	segments := source.segments(key.Segs)
	for index, item := range segments {
		if index&63 == 0 {
			if err := ctx.Err(); err != nil {
				return SnapshotKey{}, err
			}
		}
		if !validSnapshotSegment(item) {
			return SnapshotKey{}, fmt.Errorf("keyspace: malformed snapshot segment")
		}
	}

	out := SnapshotKey{
		kind: key.Kind, sym: key.Sym, version: key.Ver, root: key.Root, canon: key.Canon,
		segments: append([]segment.Segment(nil), segments...),
	}
	switch key.Kind {
	case KindResolverSym:
		if key.Sym == 0 || key.Ver == 0 || key.Root != 0 || key.Canon {
			return SnapshotKey{}, fmt.Errorf("keyspace: malformed resolver snapshot key")
		}
	case KindUnversionedSym, KindStableSym:
		if key.Sym == 0 || key.Ver != 0 || key.Root != 0 || key.Canon {
			return SnapshotKey{}, fmt.Errorf("keyspace: malformed symbol snapshot key")
		}
	case KindPlaceholder, KindRetSlot:
		if key.Sym != 0 || key.Ver != 0 {
			return SnapshotKey{}, fmt.Errorf("keyspace: malformed slot snapshot key")
		}
	case KindNamed:
		if key.Sym != 0 || key.Ver != 0 {
			return SnapshotKey{}, fmt.Errorf("keyspace: malformed named snapshot key")
		}
		out.namedRoot = source.rootName(rootID(key.Root))
		if out.namedRoot == "" {
			return SnapshotKey{}, fmt.Errorf("keyspace: empty or foreign named snapshot root")
		}
		out.root = 0
	case kindFormalRoot:
		if key.Sym != 0 || key.Ver != 0 || key.Canon || key.Root == 0 {
			return SnapshotKey{}, fmt.Errorf("keyspace: malformed formal snapshot key")
		}
		root := source.formalRootEntries[key.Root]
		if !root.Valid() {
			return SnapshotKey{}, fmt.Errorf("keyspace: invalid formal snapshot descriptor")
		}
		out.formalRoot = root
		out.root = 0
	case kindBoundaryExistential:
		if key.Sym != 0 || key.Ver != 0 || key.Canon || key.Root == 0 {
			return SnapshotKey{}, fmt.Errorf("keyspace: malformed boundary existential snapshot key")
		}
		out.namedRoot = encodeBoundaryExistentialDescriptor(source.existentialEntries[key.Root])
		out.root = 0
	case KindRootlessSuffix:
		if key.Sym != 0 || key.Ver != 0 || key.Root != 0 || key.Canon || len(segments) == 0 {
			return SnapshotKey{}, fmt.Errorf("keyspace: malformed rootless snapshot key")
		}
	default:
		return SnapshotKey{}, fmt.Errorf("keyspace: unknown snapshot key kind %d", key.Kind)
	}
	return out, nil
}

func validSnapshotSegment(item segment.Segment) bool {
	switch item.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return item.Index == 0
	case segment.SegmentIndexInt:
		return item.Name == ""
	default:
		return false
	}
}

// Len reports the number of canonical unique keys.
func (s Snapshot) Len() int { return len(s.keys) }

// KeyAt returns the i-th key in canonical structural order.
func (s Snapshot) KeyAt(i int) SnapshotKey { return s.keys[i] }

// Kind returns the key namespace.
func (k SnapshotKey) Kind() KeyKind { return k.kind }

// Symbol returns the exact symbolic root, or zero for non-symbol keys.
func (k SnapshotKey) Symbol() symbol.ID { return k.sym }

// Version returns the exact SSA version, or zero for non-versioned keys.
func (k SnapshotKey) Version() uint32 { return k.version }

// RootIndex returns the placeholder or return-slot index. Its interpretation is
// selected by Kind; zero is a valid slot index.
func (k SnapshotKey) RootIndex() uint32 { return k.root }

// Canonical reports the exact stable-named spelling bit.
func (k SnapshotKey) Canonical() bool { return k.canon }

// NamedRoot returns the exact named root, or empty for non-named keys.
func (k SnapshotKey) NamedRoot() string { return k.namedRoot }

// FormalRoot returns the exact typed relational descriptor, or false for a
// key from any other namespace.
func (k SnapshotKey) FormalRoot() (formal.Root, bool) {
	if k.kind != kindFormalRoot || !k.formalRoot.Valid() {
		return formal.Root{}, false
	}
	return k.formalRoot, true
}

// SegmentLen reports the number of exact structural path segments.
func (k SnapshotKey) SegmentLen() int { return len(k.segments) }

// SegmentAt returns the i-th exact structural segment by value.
func (k SnapshotKey) SegmentAt(i int) segment.Segment { return k.segments[i] }

func sortSnapshotKeys(ctx context.Context, keys []SnapshotKey) error {
	if len(keys) < 2 {
		return ctx.Err()
	}
	scratch := make([]SnapshotKey, len(keys))
	source, target := keys, scratch
	comparisons := uint64(0)
	for width := 1; width < len(keys); width *= 2 {
		for start := 0; start < len(keys); start += 2 * width {
			middle := min(start+width, len(keys))
			end := min(start+2*width, len(keys))
			left, right, out := start, middle, start
			for left < middle && right < end {
				comparisons++
				if comparisons&63 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
				}
				if compareSnapshotKeys(source[left], source[right]) <= 0 {
					target[out] = source[left]
					left++
				} else {
					target[out] = source[right]
					right++
				}
				out++
			}
			out += copy(target[out:end], source[left:middle])
			copy(target[out:end], source[right:end])
		}
		source, target = target, source
	}
	if len(source) != 0 && &source[0] != &keys[0] {
		copy(keys, source)
	}
	return ctx.Err()
}

func compareSnapshotKeys(left, right SnapshotKey) int {
	if result := compareUint64(uint64(left.kind), uint64(right.kind)); result != 0 {
		return result
	}
	if result := compareUint64(uint64(left.sym), uint64(right.sym)); result != 0 {
		return result
	}
	if result := compareUint64(uint64(left.version), uint64(right.version)); result != 0 {
		return result
	}
	if left.canon != right.canon {
		if !left.canon {
			return -1
		}
		return 1
	}
	if result := compareUint64(uint64(left.root), uint64(right.root)); result != 0 {
		return result
	}
	if left.namedRoot < right.namedRoot {
		return -1
	}
	if left.namedRoot > right.namedRoot {
		return 1
	}
	if result := formal.Compare(left.formalRoot, right.formalRoot); result != 0 {
		return result
	}
	for index := 0; index < min(len(left.segments), len(right.segments)); index++ {
		if result := compareSegments(left.segments[index], right.segments[index]); result != 0 {
			return result
		}
	}
	return compareUint64(uint64(len(left.segments)), uint64(len(right.segments)))
}

func compareSegments(left, right segment.Segment) int {
	if result := compareUint64(uint64(left.Kind), uint64(right.Kind)); result != 0 {
		return result
	}
	if left.Name < right.Name {
		return -1
	}
	if left.Name > right.Name {
		return 1
	}
	if left.Index < right.Index {
		return -1
	}
	if left.Index > right.Index {
		return 1
	}
	return 0
}

func compareUint64(left, right uint64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
