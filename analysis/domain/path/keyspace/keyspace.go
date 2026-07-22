// Package keyspace builds a structural, comparable replacement for the string
// abstract-state key (pathdom.PathKey) used across analysis state lanes.
//
// A Key is a small comparable struct usable directly as a Go map key. It
// captures everything that makes two old PathKey strings equal or unequal, so a
// per-analysis KeySpace can reproduce the exact old string spelling
// (Format) while also answering the structural map/prefix/rebase/order queries
// without hashing strings.
//
// Identity scope: SegmentsID and root-interning ids are dense ids that are
// meaningful ONLY within the KeySpace that produced them. They MUST NOT be
// serialized, compared across KeySpaces, or persisted. Two KeySpaces may assign
// different ids to the same structural value.
package keyspace

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// KeyKind flavors the root spelling of the old PathKey string. Every flavor that
// Path.Key(), the resolver, and the address package can emit is represented.
type KeyKind uint8

const (
	// KindInvalid is the zero value and represents no key.
	KindInvalid KeyKind = iota
	// KindResolverSym is the verbose resolver symbol root with an SSA version:
	// sym<id>@<ver>. This is the point-local state key spelling.
	KindResolverSym
	// KindUnversionedSym is the bare verbose symbol root with no version:
	// sym<id>. Path.Key() emits this when Version == 0.
	KindUnversionedSym
	// KindStableSym is the compact, version-insensitive symbol root: s<id>.
	// It occupies a distinct spelling space from KindResolverSym/KindUnversionedSym.
	KindStableSym
	// KindPlaceholder is a function-refinement placeholder root: $<i>.
	KindPlaceholder
	// KindRetSlot is a return-slot root: ret[<i>].
	KindRetSlot
	// KindNamed is an arbitrary named (global) root spelled verbatim.
	KindNamed
	// KindRootlessSuffix is the deliberately rootless static-member heap key:
	// bare FormatSegments(segments) with no root.
	KindRootlessSuffix
	// kindFormalRoot is a private typed relational root. Root holds an
	// authority-local dense index into exact formal.Root storage.
	kindFormalRoot
	// kindBoundaryExistential is a private, structural lexical root minted only
	// by boundary transport. It is deliberately disjoint from user named roots.
	kindBoundaryExistential
	// keyKindCount is the owner-side inventory fence. Canonical key codecs pin
	// this cardinality at compile time, so adding a key namespace requires an
	// explicit codec update rather than silently falling through downstream.
	keyKindCount
)

// SegmentsID is the interned id for an immutable segment list. 0 means empty.
type SegmentsID uint32

// KeyHandle is a dense, KeySpace-local reference to a bound Key. Zero is the
// invalid handle. It is meaningful only with the KeySpace that minted it.
type KeyHandle uint32

// rootID is the interned id for a named-root spelling. 0 means none.
type rootID uint32

// keyUniverse is the unforgeable identity of one KeySpace's dense intern
// tables. It is deliberately private: callers may copy Keys, but only a
// KeySpace can mint a Key belonging to its universe.
type keyUniverse struct {
	space *KeySpace
	keys  []Key
	byKey map[keyShape]KeyHandle
}

// Key is a structural, comparable abstract-state key. It is usable directly as
// a Go map key. Field meaning depends on Kind:
//
//   - KindResolverSym:    Sym set, Ver set (>0), Segs.
//   - KindUnversionedSym: Sym set, Ver==0, Segs.
//   - KindStableSym:      Sym set, Ver==0, Segs.
//   - KindPlaceholder:    Root holds the placeholder index, Segs.
//   - KindRetSlot:        Root holds the return-slot index, Segs.
//   - KindNamed:          Root holds the interned named-root id, Segs.
//   - KindRootlessSuffix: Segs only (must be non-empty).
//   - private formal root: Root holds an interned typed formal descriptor id,
//     Segs.
//   - private boundary existential: Root holds an interned structural
//     descriptor id, Segs.
//
// Canon selects the spelling of stable named roots (KindNamed, KindPlaceholder,
// KindRetSlot). When false, Format spells the root verbatim, matching Path.Key()
// (the syntax-facing key). When true, Format applies the canonical stable
// encoding (n<len>:<root> when the verbatim spelling is ambiguous), matching
// Stable.Key(). The two spellings are deliberately distinct map keys, mirroring
// the old string world where a syntax key and its canonicalized address key do
// not collide. Canon has no effect on non-stable-named kinds.
type Key struct {
	Sym   symbol.ID
	owner *keyUniverse
	Ver   uint32
	Root  uint32
	Segs  SegmentsID
	Kind  KeyKind
	Canon bool
}

// keyShape is the complete structural identity of a Key. It backs the
// owner-side handle index without increasing Key's hot-map footprint.
type keyShape struct {
	Sym   symbol.ID
	owner *keyUniverse
	Ver   uint32
	Root  uint32
	Segs  SegmentsID
	Kind  KeyKind
	Canon bool
}

func keyShapeOf(k Key) keyShape {
	return keyShape{
		Sym: k.Sym, owner: k.owner, Ver: k.Ver, Root: k.Root, Segs: k.Segs,
		Kind: k.Kind, Canon: k.Canon,
	}
}

// Handle returns this key's dense, KeySpace-local handle. It returns zero for
// an unbound or invalid key.
func (k Key) Handle() KeyHandle {
	if k.owner == nil {
		return 0
	}
	handle, ok := k.owner.byKey[keyShapeOf(k)]
	if !ok || handle == 0 {
		return 0
	}
	canonical, ok := k.owner.KeyByHandle(handle)
	if !ok || canonical != k {
		return 0
	}
	return handle
}

// KeyByHandle resolves a KeySpace-local handle to its canonical bound Key.
func (ks *KeySpace) KeyByHandle(handle KeyHandle) (Key, bool) {
	if !ks.validSpace() {
		return Key{}, false
	}
	return ks.owner.KeyByHandle(handle)
}

func (u *keyUniverse) KeyByHandle(handle KeyHandle) (Key, bool) {
	if u == nil || handle == 0 || int(handle) >= len(u.keys) {
		return Key{}, false
	}
	key := u.keys[handle]
	return key, key.owner == u
}

// segmentsEntry holds an interned segment list together with its canonical
// FormatSegments spelling so Format and Less never rebuild the suffix string.
type segmentsEntry struct {
	segments []segment.Segment
	suffix   string
}

// rootEntry holds an interned named root together with its verbatim spelling.
type rootEntry struct {
	name string
}

type boundaryExistentialDescriptor struct {
	namespace  ExistentialNamespace
	sourceKind KeyKind
	sym        symbol.ID
	version    uint32
	slot       uint32
	canon      bool
	namedRoot  string
}

type segmentPairKey struct {
	first  segment.Segment
	second segment.Segment
}

type segmentTripleKey struct {
	first  segment.Segment
	second segment.Segment
	third  segment.Segment
}

// KeySpace interns segment lists and named roots per analysis. It is the oracle
// and structural engine for Key values. It is not safe for concurrent use.
type KeySpace struct {
	owner *keyUniverse

	segEntries []segmentsEntry
	segByKey   map[string]SegmentsID
	segByOne   map[segment.Segment]SegmentsID
	segByTwo   map[segmentPairKey]SegmentsID
	segByThree map[segmentTripleKey]SegmentsID

	rootEntries []rootEntry
	rootByName  map[string]rootID

	formalRootEntries []formal.Root
	formalRootByValue map[formal.Root]uint32

	existentialEntries      []boundaryExistentialDescriptor
	existentialByDescriptor map[boundaryExistentialDescriptor]uint32

	// formatByKey is the single owner-sealed spelling table. Every valid key has
	// exactly one entry, installed only after all structural fields are final.
	// Format and Less are pure readers of this same representation.
	formatByKey map[Key]string
}

// New returns an empty KeySpace. Index 0 of each intern table is reserved as the
// empty/none sentinel.
func New() *KeySpace {
	ks := &KeySpace{
		segByKey:                make(map[string]SegmentsID),
		segByOne:                make(map[segment.Segment]SegmentsID),
		segByTwo:                make(map[segmentPairKey]SegmentsID),
		segByThree:              make(map[segmentTripleKey]SegmentsID),
		rootByName:              make(map[string]rootID),
		formalRootByValue:       make(map[formal.Root]uint32),
		existentialByDescriptor: make(map[boundaryExistentialDescriptor]uint32),
		formatByKey:             make(map[Key]string),
	}
	ks.segEntries = append(ks.segEntries, segmentsEntry{})
	ks.rootEntries = append(ks.rootEntries, rootEntry{})
	ks.formalRootEntries = append(ks.formalRootEntries, formal.Root{})
	ks.existentialEntries = append(ks.existentialEntries, boundaryExistentialDescriptor{})
	ks.owner = &keyUniverse{space: ks, keys: []Key{{}}, byKey: make(map[keyShape]KeyHandle)}
	return ks
}

// internSegments returns the dense id for a segment list. Interning is keyed by
// a lossless structural encoding, not by FormatSegments: a field named "a.b" and
// the two fields "a","b" share a FormatSegments spelling, so spelling is NOT an
// injective intern key. The structural encoding records kind and value per
// segment so distinct lists never collide.
func (ks *KeySpace) internSegments(segments []segment.Segment) SegmentsID {
	if len(segments) == 0 {
		return 0
	}
	switch len(segments) {
	case 1:
		key := segments[0]
		if id, ok := ks.segByOne[key]; ok {
			return id
		}
		id := ks.addSegmentsEntry(segments)
		ks.segByOne[key] = id
		return id
	case 2:
		key := segmentPairKey{first: segments[0], second: segments[1]}
		if id, ok := ks.segByTwo[key]; ok {
			return id
		}
		id := ks.addSegmentsEntry(segments)
		ks.segByTwo[key] = id
		return id
	case 3:
		key := segmentTripleKey{first: segments[0], second: segments[1], third: segments[2]}
		if id, ok := ks.segByThree[key]; ok {
			return id
		}
		id := ks.addSegmentsEntry(segments)
		ks.segByThree[key] = id
		return id
	}
	internKey := structuralSegmentsKey(segments)
	if id, ok := ks.segByKey[internKey]; ok {
		return id
	}
	id := ks.addSegmentsEntry(segments)
	ks.segByKey[internKey] = id
	return id
}

func (ks *KeySpace) addSegmentsEntry(segments []segment.Segment) SegmentsID {
	id := SegmentsID(len(ks.segEntries))
	stored := append([]segment.Segment(nil), segments...)
	suffix := segment.FormatSegments(segments)
	ks.segEntries = append(ks.segEntries, segmentsEntry{segments: stored, suffix: suffix})
	return id
}

// structuralSegmentsKey builds an injective intern key for a segment list:
// each step is encoded as kind, a length-prefixed name (for field/string-index),
// or the signed integer value (for int index), so distinct structural lists map
// to distinct keys regardless of FormatSegments ambiguity.
func structuralSegmentsKey(segments []segment.Segment) string {
	var b strings.Builder
	for _, seg := range segments {
		switch seg.Kind {
		case segment.SegmentField:
			b.WriteByte('f')
			b.WriteString(strconv.Itoa(len(seg.Name)))
			b.WriteByte(':')
			b.WriteString(seg.Name)
		case segment.SegmentIndexString:
			b.WriteByte('q')
			b.WriteString(strconv.Itoa(len(seg.Name)))
			b.WriteByte(':')
			b.WriteString(seg.Name)
		case segment.SegmentIndexInt:
			b.WriteByte('i')
			b.WriteString(strconv.Itoa(seg.Index))
			b.WriteByte(';')
		}
	}
	return b.String()
}

func (ks *KeySpace) internRoot(name string) rootID {
	if id, ok := ks.rootByName[name]; ok {
		return id
	}
	id := rootID(len(ks.rootEntries))
	ks.rootEntries = append(ks.rootEntries, rootEntry{name: name})
	ks.rootByName[name] = id
	return id
}

func (ks *KeySpace) seg(id SegmentsID) segmentsEntry {
	if id == 0 || int(id) >= len(ks.segEntries) {
		return segmentsEntry{}
	}
	return ks.segEntries[id]
}

func (ks *KeySpace) rootName(id rootID) string {
	if id == 0 || int(id) >= len(ks.rootEntries) {
		return ""
	}
	return ks.rootEntries[id].name
}

func (ks *KeySpace) validSegmentsID(id SegmentsID) bool {
	return id == 0 || int(id) < len(ks.segEntries)
}

func (ks *KeySpace) validRootID(id rootID) bool {
	return id == 0 || int(id) < len(ks.rootEntries)
}

func (ks *KeySpace) validKey(k Key) bool {
	if !ks.validSpace() || k.owner == nil || k.owner != ks.owner {
		return false
	}
	if k.Handle() == 0 {
		return false
	}
	if !ks.validSegmentsID(k.Segs) {
		return false
	}
	if k.Kind == KindNamed && !ks.validRootID(rootID(k.Root)) {
		return false
	}
	if k.Kind == kindFormalRoot && (k.Root == 0 || int(k.Root) >= len(ks.formalRootEntries)) {
		return false
	}
	if k.Kind == kindBoundaryExistential && (k.Root == 0 || int(k.Root) >= len(ks.existentialEntries)) {
		return false
	}
	_, sealed := ks.formatByKey[k]
	return sealed
}

// validSpace reports whether ks is the original KeySpace bound to its dense
// intern-table universe. A shallow struct copy shares the token but not this
// self identity and therefore has no authority to mint or interpret keys.
func (ks *KeySpace) validSpace() bool {
	return ks != nil && ks.owner != nil && ks.owner.space == ks
}

// ownKey binds a newly constructed key to this KeySpace's intern-table
// universe. Derived keys retain provenance naturally when copied.
func (ks *KeySpace) ownKey(k Key) Key {
	if !ks.validSpace() {
		return Key{}
	}
	return ks.bindKey(k)
}

// bindKey binds after the public minting boundary has already proved
// validSpace. Keeping that proof at the boundary prevents shallow copies from
// mutating intern tables while avoiding a duplicate check on every hot mint.
func (ks *KeySpace) bindKey(k Key) Key {
	k.owner = ks.owner
	if k.Kind == KindInvalid {
		return Key{}
	}
	shape := keyShapeOf(k)
	if handle, ok := ks.owner.byKey[shape]; ok {
		return ks.owner.keys[handle]
	}
	handle := KeyHandle(len(ks.owner.keys))
	ks.owner.keys = append(ks.owner.keys, k)
	ks.owner.byKey[shape] = handle
	ks.sealSpelling(k)
	return k
}

// sealSpelling records the one canonical byte spelling of a fully constructed
// key. It is a minting operation, never a formatting or comparison operation.
func (ks *KeySpace) sealSpelling(k Key) string {
	if existing, ok := ks.formatByKey[k]; ok {
		return existing
	}
	var b strings.Builder
	ks.writeRoot(&b, k)
	b.WriteString(ks.suffix(k.Segs))
	spelling := b.String()
	ks.formatByKey[k] = spelling
	return spelling
}

// segments returns the interned, immutable segment list for a key. The slice
// must not be mutated; callers that expose it must copy.
func (ks *KeySpace) segments(id SegmentsID) []segment.Segment {
	if id == 0 {
		return nil
	}
	return ks.seg(id).segments
}

// Segments returns a fresh copy of a key's segment list, the structural
// equivalent of reading address.LocalPathFromKey(...).Segments. The copy is
// owned by the caller and safe to retain or mutate.
func (ks *KeySpace) Segments(k Key) []segment.Segment {
	if ks == nil || !ks.validKey(k) {
		return nil
	}
	interned := ks.segments(k.Segs)
	if len(interned) == 0 {
		return nil
	}
	out := make([]segment.Segment, len(interned))
	copy(out, interned)
	return out
}

// SegmentsView returns the key's interned segment list as a read-only borrowed
// view. The returned slice is owned by the KeySpace and must not be mutated.
// Use Segments when the caller needs an owned slice.
func (ks *KeySpace) SegmentsView(k Key) ([]segment.Segment, bool) {
	if ks == nil || !ks.validKey(k) {
		return nil, false
	}
	return ks.segments(k.Segs), true
}

// suffix returns the canonical FormatSegments spelling for a segment id.
func (ks *KeySpace) suffix(id SegmentsID) string {
	if id == 0 {
		return ""
	}
	return ks.seg(id).suffix
}
