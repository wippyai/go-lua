package keyspace

import "github.com/wippyai/go-lua/analysis/domain/formal"

// WithFormalRoot imports source's immutable suffix from from and replaces its
// root with the complete typed formal identity. It does not infer which
// concrete root a formal root denotes; that finite binding is owned by the
// caller's sealed relation schema.
func (ks *KeySpace) WithFormalRoot(from *KeySpace, source Key, root formal.Root) (Key, bool) {
	if !ks.validSpace() || from == nil || !from.validKey(source) || source.Kind == KindRootlessSuffix || !root.Valid() {
		return Key{}, false
	}
	segments, ok := from.SegmentsView(source)
	if !ok {
		return Key{}, false
	}
	target, ok := ks.formalRootKey(root)
	if !ok {
		return Key{}, false
	}
	target.Segs = ks.internSegments(segments)
	return ks.bindKey(target), true
}

// WithStructuralRoot imports source's immutable suffix from from and replaces
// its root with target, which must be an exact root already owned by ks.  It is
// the direction-neutral root-substitution primitive used by formal transport
// and its injective publication inverse.
func (ks *KeySpace) WithStructuralRoot(from *KeySpace, source, target Key) (Key, bool) {
	if !ks.validSpace() || from == nil || !from.validKey(source) || source.Kind == KindRootlessSuffix || !ks.validKey(target) {
		return Key{}, false
	}
	root, ok := ks.StructuralRoot(target)
	if !ok || root != target {
		return Key{}, false
	}
	segments, ok := from.SegmentsView(source)
	if !ok {
		return Key{}, false
	}
	root.Segs = ks.internSegments(segments)
	return ks.bindKey(root), true
}
