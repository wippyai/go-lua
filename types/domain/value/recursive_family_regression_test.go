package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// recursiveStoreFamily builds a recursive named "Store" family whose body is a record
// with the given field name carrying a self-typed function, modeling the two
// distinct cross-module Store classes (one with field "cache", one with field
// "sessions") that must NOT canonicalize to one representative.
func recursiveStoreFamily(field string, valueType typ.Type) typ.Type {
	return typ.NewRecursive("Store", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field(field, valueType).
			Field("get", typ.Func().Param("self", self).Returns(typ.String).Build()).
			Build()
	})
}

// recursiveStoreFamilySameShape builds two Stores with the SAME field names
// (cache/sessions both named "data") but DIFFERENT value types, mirroring the
// real fixtures where the map value differs (string vs Snapshot record) but the
// method shapes (get/put returning self) are identical.
func recursiveStoreFamilySameShape(mapValue typ.Type) typ.Type {
	return typ.NewRecursive("Store", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("data", typ.NewMap(typ.String, mapValue)).
			Field("get", typ.Func().Param("self", self).Param("k", typ.String).Returns(typ.NewOptional(mapValue)).Build()).
			Field("put", typ.Func().Param("self", self).Param("v", mapValue).Returns(self).Build()).
			Build()
	})
}

// recursiveStoreBehindEdge builds a Store whose ONLY differing content (a field typed by
// returnType) appears solely inside a self-recursive method return, i.e. behind
// the recursion edge. Two such families share field names and arities at every
// level reachable without crossing the recursion variable; their difference is
// only observable by following self. This models the real degraded class flow
// where the discriminating body sits behind the self back-edge.
func recursiveStoreBehindEdge(returnType typ.Type) typ.Type {
	return typ.NewRecursive("Store", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("next", typ.Func().Param("self", self).Returns(self).Build()).
			Field("value", typ.Func().Param("self", self).Returns(returnType).Build()).
			Build()
	})
}

func TestRecursiveFamily_DifferenceBehindRecursionEdge(t *testing.T) {
	a := recursiveStoreBehindEdge(typ.String)
	b := recursiveStoreBehindEdge(typ.Number)
	repA := CanonicalRecursiveFamily(a)
	repB := CanonicalRecursiveFamily(b)
	if repA == repB {
		t.Errorf("two families differing only in a method return type collapsed to one rep")
	}
}

// addStoreFamily mirrors the clean "other" module: Store{add(self), items:{[number]:string}}.
func addStoreFamily() typ.Type {
	return typ.NewRecursive("Store", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("add", typ.Func().Param("self", self).Param("v", typ.String).Returns().Build()).
			Field("items", typ.NewMap(typ.Number, typ.String)).
			Build()
	})
}

// cacheStoreFamily mirrors the self-method module: Store{cache, get, put}.
func cacheStoreFamily() typ.Type {
	return typ.NewRecursive("Store", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("cache", typ.NewMap(typ.String, typ.String)).
			Field("get", typ.Func().Param("self", self).Param("k", typ.String).Returns(typ.NewOptional(typ.String)).Build()).
			Field("put", typ.Func().Param("self", self).Param("k", typ.String).Param("v", typ.String).Returns(self).Build()).
			Build()
	})
}

func TestRecursiveFamily_RealCleanModulesDoNotCollide(t *testing.T) {
	other := addStoreFamily()
	self := cacheStoreFamily()
	repOther := CanonicalRecursiveFamily(other)
	repSelf := CanonicalRecursiveFamily(self)
	if repOther == repSelf {
		t.Errorf("two clean distinct Store modules collapsed: rep=%s", repOther.String())
	}
}

func TestRecursiveFamily_SameShapeDifferentValueDoNotCollide(t *testing.T) {
	strStore := recursiveStoreFamilySameShape(typ.String)
	recStore := recursiveStoreFamilySameShape(typ.NewRecord().Field("id", typ.String).Field("flags", typ.NewMap(typ.String, typ.Boolean)).Build())

	repStr := CanonicalRecursiveFamily(strStore)
	repRec := CanonicalRecursiveFamily(recStore)
	if repStr == repRec {
		t.Errorf("two Store families that differ only in map value type collapsed to one rep")
	}
}

func TestRecursiveFamily_DistinctStoresDoNotCollide(t *testing.T) {
	cacheStore := recursiveStoreFamily("cache", typ.NewMap(typ.String, typ.String))
	snapStore := recursiveStoreFamily("sessions", typ.NewMap(typ.String, typ.NewRecord().Field("id", typ.String).Build()))

	repCache := CanonicalRecursiveFamily(cacheStore)
	repSnap := CanonicalRecursiveFamily(snapStore)

	if repCache == repSnap {
		t.Errorf("distinct Store families canonicalized to the SAME representative (collision): %s", repCache.String())
	}
	// And re-interning the cache store must return the cache rep, not the snap rep.
	repCache2 := CanonicalRecursiveFamily(recursiveStoreFamily("cache", typ.NewMap(typ.String, typ.String)))
	if repCache2 != repCache {
		t.Errorf("re-intern of cache Store gave a different rep")
	}
}
