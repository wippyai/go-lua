package keyspace

import pathdom "github.com/wippyai/go-lua/analysis/domain/path"

// StatePath decodes an interned state-address key into its syntax-facing path
// form. It accepts only state-address roots: verbose resolver symbols,
// placeholders, return slots, and named roots. Compact stable symbols and
// rootless static-member suffixes are not source paths and are deliberately
// rejected.
func (ks *KeySpace) StatePath(k Key) (pathdom.Path, bool) {
	if ks == nil || k.Kind == KindInvalid || !ks.validKey(k) {
		return pathdom.Path{}, false
	}
	segments := ks.Segments(k)
	switch k.Kind {
	case KindResolverSym:
		return pathdom.Path{Symbol: k.Sym, Version: int(k.Ver), Segments: segments}, true
	case KindUnversionedSym:
		return pathdom.Path{Symbol: k.Sym, Segments: segments}, true
	case KindPlaceholder:
		return pathdom.NewPlaceholder(int(k.Root)).AppendSegments(segments), true
	case KindRetSlot, KindNamed:
		root := ks.namedRootString(k)
		if root == "" {
			return pathdom.Path{}, false
		}
		return pathdom.Path{Root: root, Segments: segments}, true
	default:
		return pathdom.Path{}, false
	}
}
