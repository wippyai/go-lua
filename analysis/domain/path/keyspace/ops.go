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
	case KindStableSym, KindNamed, KindPlaceholder, KindRetSlot, kindFormalRoot, kindBoundaryExistential:
		return true
	default:
		return false
	}
}

// isStructural reports whether a key is comparable in either address space.
func (k Key) isStructural() bool {
	return k.isLocal() || k.isStable()
}

// HasPathPrefix compares keys in the path-evidence namespace. Unlike
// HasPrefix, whose contract is intentionally limited to resolver/stable
// address keys, this law also admits unversioned lexical paths: those are the
// canonical keys stored by local path evidence and must participate in
// subtree/descendant invalidation. Root identity remains exact and static
// field/index-string segments retain their Lua equivalence.
func (ks *KeySpace) HasPathPrefix(candidate, prefix Key) bool {
	if ks == nil || !ks.validKey(candidate) || !ks.validKey(prefix) || candidate.Kind != prefix.Kind ||
		candidate.Sym != prefix.Sym || candidate.Ver != prefix.Ver || candidate.Root != prefix.Root || candidate.Canon != prefix.Canon {
		return false
	}
	candidateSegments, prefixSegments := ks.segments(candidate.Segs), ks.segments(prefix.Segs)
	if len(prefixSegments) > len(candidateSegments) {
		return false
	}
	for index := range prefixSegments {
		if !segmentsEquivalent(candidateSegments[index], prefixSegments[index]) {
			return false
		}
	}
	return true
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
	case KindNamed, KindPlaceholder, KindRetSlot, kindFormalRoot, kindBoundaryExistential:
		return a.Root == b.Root
	default:
		return false
	}
}

// HasPrefix reports whether prefix is k or one of its structural ancestors,
// mirroring address.PathKeyHasPrefix(Format(k), Format(prefix)).
func (ks *KeySpace) HasPrefix(k, prefix Key) bool {
	if ks == nil || !ks.validKey(k) || !ks.validKey(prefix) || !k.isStructural() || !prefix.isStructural() {
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

// StructuralRoot returns k with its complete suffix removed through the same
// sealed mint boundary as every other derived Key. Rootless suffix keys have no
// structural root and fail closed.
func (ks *KeySpace) StructuralRoot(k Key) (Key, bool) {
	if ks == nil || !ks.validKey(k) || k.Kind == KindRootlessSuffix {
		return Key{}, false
	}
	if k.Segs == 0 {
		return k, true
	}
	k.Segs = 0
	return ks.bindKey(k), true
}

// ExactRemainderAfterPrefix returns the suffix segments of k below prefix using
// exact segment equality, rather than local field/string-index equivalence. Use
// this for callers whose old PathKey flow compared decoded segment structs.
func (ks *KeySpace) ExactRemainderAfterPrefix(k, prefix Key) ([]segment.Segment, bool) {
	if ks == nil || !ks.validKey(k) || !ks.validKey(prefix) || !k.isStructural() || !prefix.isStructural() {
		return nil, false
	}
	if k.isLocal() || prefix.isLocal() {
		if !k.isLocal() || !prefix.isLocal() ||
			k.Sym != prefix.Sym || k.Ver != prefix.Ver ||
			!ks.segmentsHasPrefixExact(k.Segs, prefix.Segs) {
			return nil, false
		}
		return ks.segmentsTail(k.Segs, ks.segLen(prefix.Segs)), true
	}
	if !k.sameStableRoot(prefix) || !ks.segmentsHasPrefixExact(k.Segs, prefix.Segs) {
		return nil, false
	}
	return ks.segmentsTail(k.Segs, ks.segLen(prefix.Segs)), true
}

// remainderAfterPrefix returns the suffix segments of k below prefix, mirroring
// StructuralKey.RemainderAfterPrefix.
func (ks *KeySpace) remainderAfterPrefix(k, prefix Key) ([]segment.Segment, bool) {
	if ks == nil || !ks.validKey(k) || !ks.validKey(prefix) || !k.isStructural() || !prefix.isStructural() {
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

// SegmentLen returns the number of static-member segments in k. It is the
// keyspace-owned measure used by finite alias-closure algorithms; callers
// should prefer it over copying Segments only to count them.
func (ks *KeySpace) SegmentLen(k Key) (int, bool) {
	if ks == nil || !ks.validKey(k) {
		return 0, false
	}
	return ks.segLen(k.Segs), true
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
	return ks.bindKey(out), true
}

// AppendSegment returns the key reached by appending one segment, preserving
// the root/address space. It is the keyspace-owned fast path for callers that
// already have a canonical key and should not rebuild a syntax Path only to ask
// for a child key.
func (ks *KeySpace) AppendSegment(k Key, seg segment.Segment) (Key, bool) {
	if ks == nil || !ks.validKey(k) || !k.isStructural() {
		return Key{}, false
	}
	segs := ks.segments(k.Segs)
	var next SegmentsID
	switch len(segs) {
	case 0:
		next = ks.internSegments([]segment.Segment{seg})
	case 1:
		next = ks.internSegments([]segment.Segment{segs[0], seg})
	case 2:
		next = ks.internSegments([]segment.Segment{segs[0], segs[1], seg})
	default:
		combined := make([]segment.Segment, len(segs)+1)
		copy(combined, segs)
		combined[len(segs)] = seg
		next = ks.internSegments(combined)
	}
	out := k
	out.Segs = next
	return ks.bindKey(out), true
}

// Rebase rewrites k from structural prefix from to prefix to, in the same
// address space, mirroring address.RebasePathKey. It returns false when from is
// not a prefix of k, when from and to are different kinds, or when the result
// would be empty or unchanged.
func (ks *KeySpace) Rebase(k, from, to Key) (Key, bool) {
	if ks == nil || !ks.validKey(k) || !ks.validKey(from) || !ks.validKey(to) ||
		!k.isStructural() || !from.isStructural() || !to.isStructural() {
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
	rebased = ks.bindKey(rebased)
	out := ks.Format(rebased)
	if out == "" || out == ks.Format(k) {
		return Key{}, false
	}
	return rebased, true
}

// RebaseToExistential preserves k's exact suffix while substituting a local
// or stable structural root with a boundary existential root. Ordinary Rebase
// keeps local and stable address spaces disjoint; boundary transport is the
// sole authority allowed to cross that seam.
func (ks *KeySpace) RebaseToExistential(k, from, to Key) (Key, bool) {
	if ks == nil || !ks.validKey(k) || !ks.validKey(from) || !ks.validKey(to) ||
		!k.isStructural() || !from.isStructural() || to.Kind != kindBoundaryExistential {
		return Key{}, false
	}
	remainder, ok := ks.remainderAfterPrefix(k, from)
	if !ok {
		return Key{}, false
	}
	rebased, ok := ks.appendSegments(to, remainder)
	if !ok || rebased.Kind == KindInvalid || ks.Format(rebased) == "" || ks.Format(rebased) == ks.Format(k) {
		return Key{}, false
	}
	return rebased, true
}

// FieldCanonical returns the equivalent key whose static string-index segments
// use field spelling, mirroring address.FieldCanonicalPathKey. It returns false
// when k is not a recognized structural key, when nothing changes, or when the
// canonical key equals the original.
func (ks *KeySpace) FieldCanonical(k Key) (Key, bool) {
	if ks == nil || !ks.validKey(k) || !k.isStructural() {
		return Key{}, false
	}
	segments, changed := fieldCanonicalSegments(ks.segments(k.Segs))
	if !changed {
		return Key{}, false
	}
	out := k
	out.Segs = ks.internSegments(segments)
	out.Canon = out.isStableNamed()
	out = ks.bindKey(out)
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
	segments, ok := ks.SuffixSegmentsView(k)
	if !ok {
		return nil, false
	}
	return append([]segment.Segment(nil), segments...), true
}

// SuffixSegmentsView returns a rootless static-member suffix as a read-only
// borrowed segment view. The returned slice is owned by the KeySpace and must
// not be mutated. Use SuffixSegments when the caller needs an owned slice.
func (ks *KeySpace) SuffixSegmentsView(k Key) ([]segment.Segment, bool) {
	if ks == nil || !ks.validKey(k) || k.Kind != KindRootlessSuffix || k.Segs == 0 {
		return nil, false
	}
	return ks.segments(k.Segs), true
}
