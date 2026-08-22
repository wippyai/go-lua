package equation

import (
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/internal/canonical"
)

// Equation identity is the row-address plane of the whole engine: a Query key
// is the snapshot row a declared query publishes under, so it outlives the
// construction that minted it. A construction-path change may move who calls
// deriveQueryKey and when, but it must not move the key a fixed declaration
// derives to, and it must not silently reshape the preimage.
//
// identityVersionFence is the version identityKey frames every equation
// preimage under. Raising it declares every retained equation identity
// unreadable, so it may only change together with the pinned digests below.
const identityVersionFence = 18

// queryKeyDomainFence is the domain tag deriveQueryKey frames its preimage
// under (identity.go, deriveQueryKey). The tag separates a Query key from every
// other equation identity space, so it is part of the address, not a label.
const queryKeyDomainFence = "analysis/engine/equation/query"

// queryKeyPreimageFenceHex pins the whole deriveQueryKey preimage in one
// literal: the domain tag, identityVersion, and the event order
//
//	writeContentID(Context), writeKey(Family), writeKey(point.key), writeScope(point.Scope()),
//	Count(len(Surfaces)), then per surface in slice order
//	writeSurface(surface) followed by writeSurfaceCatalog(surface, catalog)
//
// replayed over a fixed declaration. Any change to that order, to the framing
// any writeKey/writeScope/writeSurface helper emits, or to identityVersion
// moves this digest.
const queryKeyPreimageFenceHex = "4e03ca1388f9c360c545af66afd50e4503e61b42ddead82d6b6c1741b77ec14a"

func TestEquationIdentityVersionIsFenced(t *testing.T) {
	if identityVersion != identityVersionFence {
		t.Fatalf("identityVersion is %d, the fence pins %d; raising it invalidates every retained equation identity and must move the pinned preimage digests in the same change", identityVersion, identityVersionFence)
	}
}

// fenceQuerySurfaces is the fixed surface vector the preimage is pinned over.
// It carries one ordinal-space exact read and one content-space anchored read,
// which is the distinction identityVersion 18 exists for: writeSurfaceLocal
// must frame the space tag ahead of either payload, so the two spaces cannot
// collide in one preimage.
func fenceQuerySurfaces() []Surface {
	anchored := Surface{Factor: fenceIdentityKey(72), Form: SurfaceReadExact, Mode: TargetModeNone}
	anchored.Content = fenceContent(9)
	return []Surface{
		{Factor: fenceIdentityKey(71), Form: SurfaceReadExact, Local: 1, Mode: TargetModeNone},
		anchored,
	}
}

func fenceIdentityKey(value byte) composition.Key {
	var id composition.ID
	id[0] = value
	return composition.Key{ID: id, Version: 1}
}

func fenceContent(value byte) [32]byte {
	var content [32]byte
	content[0] = value
	return content
}

func fenceContext(value byte) identity.ContentID {
	var content identity.ContentID
	content[0] = value
	return content
}

// TestQueryKeyPreimageShapeIsFenced replays deriveQueryKey's preimage through
// the same identityKey framing and the same writer helpers, in the same order,
// and pins the result. deriveQueryKey itself needs a sealed topology to reach;
// this law fences the part of it a construction-path edit can touch, which is
// the preimage.
func TestQueryKeyPreimageShapeIsFenced(t *testing.T) {
	family, point := fenceIdentityKey(61), fenceIdentityKey(62)
	surfaces := fenceQuerySurfaces()
	key, ok := identityKey(queryKeyDomainFence, func(writer *canonical.DigestWriter) bool {
		if !writeContentID(writer, fenceContext(60)) || !writeKey(writer, family) || !writeKey(writer, point) || !writeScope(writer, EmptyScope()) || writer.Count(uint64(len(surfaces))) != nil {
			return false
		}
		for _, surface := range surfaces {
			if !writeSurface(writer, surface) || !writeSurfaceCatalog(writer, surface, topologyCatalog{}) {
				return false
			}
		}
		return true
	})
	if !ok || !key.Available() {
		t.Fatal("the fenced query preimage no longer derives a key")
	}
	if key.Version != identityVersionFence {
		t.Fatalf("derived query key carries version %d, the fence pins %d", key.Version, identityVersionFence)
	}
	if got := hex.EncodeToString(key.ID[:]); got != queryKeyPreimageFenceHex {
		t.Fatalf("fenced query preimage digest is %s, the fence pins %s; a construction-path edit must not reshape the preimage", got, queryKeyPreimageFenceHex)
	}
}

// TestQueryKeyPreimageIsPositional records why the digest above is a fence and
// not a snapshot: every position in the preimage is load-bearing. Swapping the
// two surfaces, swapping family for point, or reading a surface under the other
// coordinate space each moves the key.
func TestQueryKeyPreimageIsPositional(t *testing.T) {
	family, point := fenceIdentityKey(61), fenceIdentityKey(62)
	derive := func(first, second composition.Key, surfaces []Surface) composition.Key {
		key, ok := identityKey(queryKeyDomainFence, func(writer *canonical.DigestWriter) bool {
			if !writeContentID(writer, fenceContext(60)) || !writeKey(writer, first) || !writeKey(writer, second) || !writeScope(writer, EmptyScope()) || writer.Count(uint64(len(surfaces))) != nil {
				return false
			}
			for _, surface := range surfaces {
				if !writeSurface(writer, surface) || !writeSurfaceCatalog(writer, surface, topologyCatalog{}) {
					return false
				}
			}
			return true
		})
		if !ok {
			t.Fatal("the fenced query preimage no longer derives a key")
		}
		return key
	}
	surfaces := fenceQuerySurfaces()
	base := derive(family, point, surfaces)
	if base == derive(point, family, surfaces) {
		t.Fatal("family and point are interchangeable in the query preimage")
	}
	swapped := []Surface{surfaces[1], surfaces[0]}
	if base == derive(family, point, swapped) {
		t.Fatal("surface order does not reach the query key")
	}
	// The ordinal and content coordinate spaces must not collide: an ordinal
	// surface whose Local equals the first byte of a content surface is a
	// different declaration and must derive a different key.
	collide := []Surface{surfaces[0], {Factor: surfaces[1].Factor, Form: SurfaceReadExact, Local: 9, Mode: TargetModeNone}}
	if base == derive(family, point, collide) {
		t.Fatal("the surface coordinate space tag no longer separates ordinal from content payloads")
	}
	if base == derive(family, point, surfaces[:1]) {
		t.Fatal("the surface count does not reach the query key")
	}
}

// TestQueryKeyDomainSeparatesItsIdentitySpace pins that the query tag is its own
// address space. A construction-path edit that reused another equation tag for a
// query row would alias two identity spaces under one key.
func TestQueryKeyDomainSeparatesItsIdentitySpace(t *testing.T) {
	encode := func(writer *canonical.DigestWriter) bool {
		return writeKey(writer, fenceIdentityKey(61))
	}
	query, queryOK := identityKey(queryKeyDomainFence, encode)
	other, otherOK := identityKey(queryKeyDomainFence+"-not", encode)
	if !queryOK || !otherOK {
		t.Fatal("domain-separated identity keys no longer derive")
	}
	if query == other {
		t.Fatal("the query domain tag does not reach identity")
	}
}
