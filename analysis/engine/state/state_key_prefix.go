package state

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
)

func stateKeyHasPrefix(ks *keyspace.KeySpace, key, prefix keyspace.Key) bool {
	if ks == nil {
		return false
	}
	if ks.HasPrefix(key, prefix) {
		return true
	}
	if !sameStateResolverRoot(key, prefix) {
		return false
	}
	keySegments, ok := ks.SegmentsView(key)
	if !ok {
		return false
	}
	prefixSegments, ok := ks.SegmentsView(prefix)
	if !ok {
		return false
	}
	return segmentsHavePrefix(keySegments, prefixSegments)
}

func stateKeyHasStrictPrefix(ks *keyspace.KeySpace, key, prefix keyspace.Key) bool {
	if ks == nil {
		return false
	}
	if ks.HasStrictPrefix(key, prefix) {
		return true
	}
	if !sameStateResolverRoot(key, prefix) {
		return false
	}
	keySegments, ok := ks.SegmentsView(key)
	if !ok {
		return false
	}
	prefixSegments, ok := ks.SegmentsView(prefix)
	if !ok {
		return false
	}
	return len(keySegments) > len(prefixSegments) && segmentsHavePrefix(keySegments, prefixSegments)
}

func sameStateResolverRoot(a, b keyspace.Key) bool {
	if !stateResolverSymbolKey(a) || !stateResolverSymbolKey(b) || a.Sym != b.Sym {
		return false
	}
	if a.Kind == keyspace.KindResolverSym && b.Kind == keyspace.KindResolverSym {
		return a.Ver == b.Ver
	}
	return true
}

func stateResolverSymbolKey(k keyspace.Key) bool {
	return k.Kind == keyspace.KindResolverSym || k.Kind == keyspace.KindUnversionedSym
}

func segmentsHavePrefix(segments []segment.Segment, prefix []segment.Segment) bool {
	if len(prefix) > len(segments) {
		return false
	}
	for i := range prefix {
		if segments[i] != prefix[i] {
			return false
		}
	}
	return true
}
