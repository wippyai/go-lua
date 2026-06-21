package keyspace

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

// isLocal reports whether a key participates in the point-local address space,
// matching address.StructuralKeyFromPathKey's local branch (versioned resolver
// symbol only).
func (k Key) isLocal() bool {
	return k.Kind == KindResolverSym
}

// isStable reports whether a key participates in the stable address space,
// matching address.StableFromKey (stable-symbol, named, placeholder, ret slot).
// Unversioned resolver symbols and rootless suffixes are not structural keys.
func (k Key) isStable() bool {
	switch k.Kind {
	case KindStableSym, KindNamed, KindPlaceholder, KindRetSlot:
		return true
	default:
		return false
	}
}

// isStructural reports whether a key is comparable in either address space.
func (k Key) isStructural() bool {
	return k.isLocal() || k.isStable()
}

// isStableNamed reports whether a key is a stable named root whose canonical
// spelling may differ from its verbatim spelling (encoded n<len>: form).
func (k Key) isStableNamed() bool {
	switch k.Kind {
	case KindNamed, KindPlaceholder, KindRetSlot:
		return true
	default:
		return false
	}
}

// sameStableRoot reports stable root equality. Symbol and named roots never
// collide because their spellings live in disjoint spaces.
func (a Key) sameStableRoot(b Key) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KindStableSym:
		return a.Sym == b.Sym
	case KindNamed, KindPlaceholder, KindRetSlot:
		return a.Root == b.Root
	default:
		return false
	}
}

// HasPrefix reports whether prefix is k or one of its structural ancestors,
// mirroring address.PathKeyHasPrefix(Format(k), Format(prefix)).
func (ks *KeySpace) HasPrefix(k, prefix Key) bool {
	if !k.isStructural() || !prefix.isStructural() {
		return false
	}
	if k.isLocal() || prefix.isLocal() {
		return k.isLocal() && prefix.isLocal() &&
			k.Sym == prefix.Sym && k.Ver == prefix.Ver &&
			ks.segmentsHasPrefixEquiv(k.Segs, prefix.Segs)
	}
	return k.sameStableRoot(prefix) && ks.segmentsHasPrefixExact(k.Segs, prefix.Segs)
}

// HasStrictPrefix reports whether prefix is a strict structural ancestor of k,
// mirroring address.PathKeyHasStrictPrefix.
func (ks *KeySpace) HasStrictPrefix(k, prefix Key) bool {
	remainder, ok := ks.remainderAfterPrefix(k, prefix)
	return ok && len(remainder) > 0
}

// remainderAfterPrefix returns the suffix segments of k below prefix, mirroring
// StructuralKey.RemainderAfterPrefix.
func (ks *KeySpace) remainderAfterPrefix(k, prefix Key) ([]segment.Segment, bool) {
	if !k.isStructural() || !prefix.isStructural() {
		return nil, false
	}
	if k.isLocal() || prefix.isLocal() {
		if !k.isLocal() || !prefix.isLocal() ||
			k.Sym != prefix.Sym || k.Ver != prefix.Ver ||
			!ks.segmentsHasPrefixEquiv(k.Segs, prefix.Segs) {
			return nil, false
		}
		return ks.segmentsTail(k.Segs, ks.segLen(prefix.Segs)), true
	}
	if !k.sameStableRoot(prefix) || !ks.segmentsHasPrefixExact(k.Segs, prefix.Segs) {
		return nil, false
	}
	return ks.segmentsTail(k.Segs, ks.segLen(prefix.Segs)), true
}

func (ks *KeySpace) segLen(id SegmentsID) int {
	return len(ks.segments(id))
}

func (ks *KeySpace) segmentsTail(id SegmentsID, from int) []segment.Segment {
	segs := ks.segments(id)
	if from >= len(segs) {
		return nil
	}
	return append([]segment.Segment(nil), segs[from:]...)
}

// segmentsHasPrefixExact mirrors Suffix.HasPrefix (exact == on segments).
func (ks *KeySpace) segmentsHasPrefixExact(segs, prefix SegmentsID) bool {
	if segs == prefix {
		return true
	}
	s := ks.segments(segs)
	p := ks.segments(prefix)
	if len(p) > len(s) {
		return false
	}
	for i := range p {
		if s[i] != p[i] {
			return false
		}
	}
	return true
}

// segmentsHasPrefixEquiv mirrors address.SegmentsHasPrefix (field and string
// index segments with equal names are equivalent).
func (ks *KeySpace) segmentsHasPrefixEquiv(segs, prefix SegmentsID) bool {
	s := ks.segments(segs)
	p := ks.segments(prefix)
	if len(p) > len(s) {
		return false
	}
	for i := range p {
		if !segmentsEquivalent(s[i], p[i]) {
			return false
		}
	}
	return true
}

func segmentsEquivalent(a, b segment.Segment) bool {
	if a == b {
		return true
	}
	if (a.Kind == segment.SegmentField || a.Kind == segment.SegmentIndexString) &&
		(b.Kind == segment.SegmentField || b.Kind == segment.SegmentIndexString) {
		return a.Name == b.Name
	}
	return false
}

// appendSegments returns the key reached by appending suffix segments,
// preserving Kind and root. It mirrors StructuralKey.Append.
func (ks *KeySpace) appendSegments(k Key, suffix []segment.Segment) (Key, bool) {
	if !k.isStructural() {
		return Key{}, false
	}
	if len(suffix) == 0 {
		return k, true
	}
	base := ks.segments(k.Segs)
	next := make([]segment.Segment, len(base)+len(suffix))
	copy(next, base)
	copy(next[len(base):], suffix)
	out := k
	out.Segs = ks.internSegments(next)
	return out, true
}

// Rebase rewrites k from structural prefix from to prefix to, in the same
// address space, mirroring address.RebasePathKey. It returns false when from is
// not a prefix of k, when from and to are different kinds, or when the result
// would be empty or unchanged.
func (ks *KeySpace) Rebase(k, from, to Key) (Key, bool) {
	if !k.isStructural() || !from.isStructural() || !to.isStructural() {
		return Key{}, false
	}
	if from.isLocal() != to.isLocal() {
		return Key{}, false
	}
	remainder, ok := ks.remainderAfterPrefix(k, from)
	if !ok {
		return Key{}, false
	}
	rebased, ok := ks.appendSegments(to, remainder)
	if !ok || rebased.Kind == KindInvalid {
		return Key{}, false
	}
	rebased.Canon = rebased.isStableNamed()
	out := ks.Format(rebased)
	if out == "" || out == ks.Format(k) {
		return Key{}, false
	}
	return rebased, true
}

// FieldCanonical returns the equivalent key whose static string-index segments
// use field spelling, mirroring address.FieldCanonicalPathKey. It returns false
// when k is not a recognized structural key, when nothing changes, or when the
// canonical key equals the original.
func (ks *KeySpace) FieldCanonical(k Key) (Key, bool) {
	if !k.isStructural() {
		return Key{}, false
	}
	segments, changed := fieldCanonicalSegments(ks.segments(k.Segs))
	if !changed {
		return Key{}, false
	}
	out := k
	out.Segs = ks.internSegments(segments)
	out.Canon = out.isStableNamed()
	formatted := ks.Format(out)
	if formatted == "" || formatted == ks.Format(k) {
		return Key{}, false
	}
	return out, true
}

func fieldCanonicalSegments(segments []segment.Segment) ([]segment.Segment, bool) {
	var out []segment.Segment
	changed := false
	for i, seg := range segments {
		if seg.Kind != segment.SegmentIndexString {
			continue
		}
		if !changed {
			out = append([]segment.Segment(nil), segments...)
			changed = true
		}
		out[i] = segment.Segment{Kind: segment.SegmentField, Name: seg.Name}
	}
	if !changed {
		return segments, false
	}
	return out, true
}

// SuffixSegments returns the static-member suffix segments for a rootless heap
// key, mirroring address.RelativeStaticMemberSuffixSegments. Only rootless keys
// with non-empty segments have a suffix; all others return false.
func (ks *KeySpace) SuffixSegments(k Key) ([]segment.Segment, bool) {
	if k.Kind != KindRootlessSuffix || k.Segs == 0 {
		return nil, false
	}
	return append([]segment.Segment(nil), ks.segments(k.Segs)...), true
}
