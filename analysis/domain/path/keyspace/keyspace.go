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
)

// SegmentsID is the interned id for an immutable segment list. 0 means empty.
type SegmentsID uint32

// rootID is the interned id for a named-root spelling. 0 means none.
type rootID uint32

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
//
// Canon selects the spelling of stable named roots (KindNamed, KindPlaceholder,
// KindRetSlot). When false, Format spells the root verbatim, matching Path.Key()
// (the syntax-facing key). When true, Format applies the canonical stable
// encoding (n<len>:<root> when the verbatim spelling is ambiguous), matching
// Stable.Key(). The two spellings are deliberately distinct map keys, mirroring
// the old string world where a syntax key and its canonicalized address key do
// not collide. Canon has no effect on non-stable-named kinds.
type Key struct {
	Kind  KeyKind
	Sym   symbol.ID
	Ver   uint32
	Root  uint32
	Segs  SegmentsID
	Canon bool
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
	segEntries []segmentsEntry
	segByKey   map[string]SegmentsID
	segByOne   map[segment.Segment]SegmentsID
	segByTwo   map[segmentPairKey]SegmentsID
	segByThree map[segmentTripleKey]SegmentsID

	rootEntries []rootEntry
	rootByName  map[string]rootID

	formatByKey map[Key]string
}

// New returns an empty KeySpace. Index 0 of each intern table is reserved as the
// empty/none sentinel.
func New() *KeySpace {
	ks := &KeySpace{
		segByKey:    make(map[string]SegmentsID),
		segByOne:    make(map[segment.Segment]SegmentsID),
		segByTwo:    make(map[segmentPairKey]SegmentsID),
		segByThree:  make(map[segmentTripleKey]SegmentsID),
		rootByName:  make(map[string]rootID),
		formatByKey: make(map[Key]string),
	}
	ks.segEntries = append(ks.segEntries, segmentsEntry{})
	ks.rootEntries = append(ks.rootEntries, rootEntry{})
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
	if !ks.validSegmentsID(k.Segs) {
		return false
	}
	if k.Kind == KindNamed && !ks.validRootID(rootID(k.Root)) {
		return false
	}
	return true
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
