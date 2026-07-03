package keyspace

import (
	"strconv"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// FromPath produces the structural key whose Format reproduces p.Key() byte for
// byte. An empty path yields the invalid (zero) key, matching Path.Key() == "".
//
// Path.Key() spells a symbol-rooted path as sym<id>[@<ver>]<segs> and a
// non-symbol path as <Root><segs> verbatim. To make Format reproduce that, the
// non-symbol Root is classified into its spelling flavor (placeholder, return
// slot, or arbitrary named) but always re-emitted verbatim.
func (ks *KeySpace) FromPath(p pathdom.Path) Key {
	if p.IsEmpty() {
		return Key{}
	}
	segs := ks.internSegments(p.Segments)
	if p.Symbol != 0 {
		if p.Version != 0 {
			return Key{Kind: KindResolverSym, Sym: p.Symbol, Ver: uint32(p.Version), Segs: segs}
		}
		return Key{Kind: KindUnversionedSym, Sym: p.Symbol, Segs: segs}
	}
	return ks.namedRootKey(p.Root, segs)
}

// namedRootKey classifies a verbatim root spelling into a flavor and interns it
// when it is an arbitrary name. Placeholder ($N) and return slot (ret[N]) roots
// carry their index in Root; arbitrary names carry an interned root id.
func (ks *KeySpace) namedRootKey(root string, segs SegmentsID) Key {
	if idx, ok := placeholderIndex(root); ok {
		return Key{Kind: KindPlaceholder, Root: uint32(idx), Segs: segs}
	}
	if idx, ok := retSlotIndex(root); ok {
		return Key{Kind: KindRetSlot, Root: uint32(idx), Segs: segs}
	}
	return Key{Kind: KindNamed, Root: uint32(ks.internRoot(root)), Segs: segs}
}

// FromResolverKey produces the structural key for a verbose resolver root, the
// spelling Resolver.KeyForVersion emits. version > 0 yields KindResolverSym;
// version == 0 yields KindUnversionedSym.
func (ks *KeySpace) FromResolverKey(sym symbol.ID, version int, segments []segment.Segment) (Key, bool) {
	if sym == 0 {
		return Key{}, false
	}
	segs := ks.internSegments(segments)
	if version > 0 {
		return Key{Kind: KindResolverSym, Sym: sym, Ver: uint32(version), Segs: segs}, true
	}
	if version != 0 {
		return Key{}, false
	}
	return Key{Kind: KindUnversionedSym, Sym: sym, Segs: segs}, true
}

// FromPathKey parses a point-local resolver key string (sym<id>@<ver><segs>)
// into the interned local key, mirroring address.LocalKeyFromPathKey. Only
// versioned resolver keys participate in the point-local value lane; every other
// spelling (unversioned, stable, named, rootless) yields false.
func (ks *KeySpace) FromPathKey(key pathdom.PathKey) (Key, bool) {
	sym, version, suffix, ok := pathaddr.ParseResolverPath(key)
	if !ok || version <= 0 {
		return Key{}, false
	}
	segments, ok := segment.InternFormattedSegments(suffix)
	if !ok {
		return Key{}, false
	}
	return Key{Kind: KindResolverSym, Sym: sym, Ver: uint32(version), Segs: ks.internSegments(segments)}, true
}

// FromStateKey parses any verbose state path-key spelling the resolver emits
// (RootOrVisibleKeyAt / KeyAt): a versioned resolver symbol (sym<id>@<ver>), a
// bare unversioned resolver symbol (sym<id>, the root spelling Path.Key() emits
// when Version == 0), or an arbitrary named/placeholder/return-slot root spelled
// verbatim. Unlike FromPathKey it does not require a version, so it covers the
// numeric and length floor lanes whose root keys are unversioned. Compact stable
// symbol spellings are not interpreted as stable symbol keys here; when accepted
// by the plain named-root grammar they remain ordinary named roots. Rootless
// suffix spellings are not state keys and yield false.
func (ks *KeySpace) FromStateKey(key pathdom.PathKey) (Key, bool) {
	if key == "" {
		return Key{}, false
	}
	if sym, version, suffix, ok := pathaddr.ParseResolverPath(key); ok {
		segments, segOK := segment.InternFormattedSegments(suffix)
		if !segOK {
			return Key{}, false
		}
		segs := ks.internSegments(segments)
		if version > 0 {
			return Key{Kind: KindResolverSym, Sym: sym, Ver: uint32(version), Segs: segs}, true
		}
		return Key{Kind: KindUnversionedSym, Sym: sym, Segs: segs}, true
	}
	root, segments, ok := parsePlainNamedRoot(string(key))
	if !ok {
		return Key{}, false
	}
	return ks.namedRootKey(root, ks.internSegments(segments)), true
}

// InternStateKey interns an already-validated state-key carrier into the hot
// structural key representation used by state lanes. Prefer this at boundaries
// that already narrowed a raw PathKey to address.StateKey.
func (ks *KeySpace) InternStateKey(key pathaddr.StateKey) (Key, bool) {
	if key == "" {
		return Key{}, false
	}
	return ks.FromStateKey(key.PathKey())
}

// FromStableSymbol produces the compact stable symbol key (s<id><segs>), the
// spelling address.SymbolStableKey emits.
func (ks *KeySpace) FromStableSymbol(sym symbol.ID, segments []segment.Segment) (Key, bool) {
	if sym == 0 {
		return Key{}, false
	}
	return Key{Kind: KindStableSym, Sym: sym, Segs: ks.internSegments(segments)}, true
}

// FromRootlessSuffix produces the rootless static-member heap key, the spelling
// address.RelativeStaticMemberSuffixKey emits. An empty segment list has no
// rootless key and yields false.
func (ks *KeySpace) FromRootlessSuffix(segments []segment.Segment) (Key, bool) {
	if len(segments) == 0 {
		return Key{}, false
	}
	return Key{Kind: KindRootlessSuffix, Segs: ks.internSegments(segments)}, true
}

func placeholderIndex(root string) (int, bool) {
	idx := pathdom.PlaceholderIndexFromString(root)
	if idx < 0 {
		return 0, false
	}
	// Canonical round-trip guard: $<idx> must spell back to root exactly so
	// non-canonical spellings ($00, $0x...) stay arbitrary named roots.
	if "$"+strconv.Itoa(idx) != root {
		return 0, false
	}
	return idx, true
}

func retSlotIndex(root string) (int, bool) {
	idx := pathdom.ReturnSlotIndexFromString(root)
	if idx < 0 {
		return 0, false
	}
	if "ret["+strconv.Itoa(idx)+"]" != root {
		return 0, false
	}
	return idx, true
}
