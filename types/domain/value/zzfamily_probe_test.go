package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// zzStoreFamily builds a recursive named "Store" family whose body is a record
// with the given field name carrying a self-typed function, modeling the two
// distinct cross-module Store classes (one with field "cache", one with field
// "sessions") that must NOT canonicalize to one representative.
func zzStoreFamily(field string, valueType typ.Type) typ.Type {
	return typ.NewRecursive("Store", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field(field, valueType).
			Field("get", typ.Func().Param("self", self).Returns(typ.String).Build()).
			Build()
	})
}

// zzStoreFamilySameNames builds two Stores with the SAME field names
// (cache/sessions both named "data") but DIFFERENT value types, mirroring the
// real fixtures where the map value differs (string vs Snapshot record) but the
// method shapes (get/put returning self) are identical.
func zzStoreFamilySameShape(mapValue typ.Type) typ.Type {
	return typ.NewRecursive("Store", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("data", typ.NewMap(typ.String, mapValue)).
			Field("get", typ.Func().Param("self", self).Param("k", typ.String).Returns(typ.NewOptional(mapValue)).Build()).
			Field("put", typ.Func().Param("self", self).Param("v", mapValue).Returns(self).Build()).
			Build()
	})
}

// zzStoreBehindEdge builds a Store whose ONLY differing content (a field typed by
// returnType) appears solely inside a self-recursive method return, i.e. behind
// the recursion edge. Two such families share field names and arities at every
// level reachable without crossing the recursion variable; their difference is
// only observable by following self. This models the real degraded class flow
// where the discriminating body sits behind the self back-edge.
func zzStoreBehindEdge(returnType typ.Type) typ.Type {
	return typ.NewRecursive("Store", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("next", typ.Func().Param("self", self).Returns(self).Build()).
			Field("value", typ.Func().Param("self", self).Returns(returnType).Build()).
			Build()
	})
}

func TestZZFamily_DifferenceBehindRecursionEdge(t *testing.T) {
	a := zzStoreBehindEdge(typ.String)
	b := zzStoreBehindEdge(typ.Number)
	repA := CanonicalRecursiveFamily(a)
	repB := CanonicalRecursiveFamily(b)
	t.Logf("A rep: %s", repA.String())
	t.Logf("B rep: %s", repB.String())
	t.Logf("phash a=%d b=%d", typ.ProductFamilyHash(a), typ.ProductFamilyHash(b))
	if repA == repB {
		t.Errorf("two families differing only in a method return type collapsed to one rep")
	}
}

// zzAddStore mirrors the clean "other" module: Store{add(self), items:{[number]:string}}.
func zzAddStore() typ.Type {
	return typ.NewRecursive("Store", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("add", typ.Func().Param("self", self).Param("v", typ.String).Returns().Build()).
			Field("items", typ.NewMap(typ.Number, typ.String)).
			Build()
	})
}

// zzCacheStore mirrors the self-method module: Store{cache, get, put}.
func zzCacheStore() typ.Type {
	return typ.NewRecursive("Store", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("cache", typ.NewMap(typ.String, typ.String)).
			Field("get", typ.Func().Param("self", self).Param("k", typ.String).Returns(typ.NewOptional(typ.String)).Build()).
			Field("put", typ.Func().Param("self", self).Param("k", typ.String).Param("v", typ.String).Returns(self).Build()).
			Build()
	})
}

func TestZZFamily_RealCleanModulesDoNotCollide(t *testing.T) {
	other := zzAddStore()
	self := zzCacheStore()
	t.Logf("other body: %s", other.(*typ.Recursive).Body.String())
	t.Logf("self  body: %s", self.(*typ.Recursive).Body.String())
	t.Logf("SameConvergedFact=%v", SameConvergedFact(other, self))
	t.Logf("factTypeMetadataEqual=%v", factTypeMetadataEqual(other, self, nil))
	repOther := CanonicalRecursiveFamily(other)
	repSelf := CanonicalRecursiveFamily(self)
	if repOther == repSelf {
		t.Errorf("two clean distinct Store modules collapsed: rep=%s", repOther.String())
	}
}

func TestZZFamily_SameShapeDifferentValueDoNotCollide(t *testing.T) {
	strStore := zzStoreFamilySameShape(typ.String)
	recStore := zzStoreFamilySameShape(typ.NewRecord().Field("id", typ.String).Field("flags", typ.NewMap(typ.String, typ.Boolean)).Build())

	repStr := CanonicalRecursiveFamily(strStore)
	repRec := CanonicalRecursiveFamily(recStore)
	t.Logf("str rep:  %s", repStr.String())
	t.Logf("rec rep:  %s", repRec.String())
	t.Logf("phash str=%d rec=%d", typ.ProductFamilyHash(strStore), typ.ProductFamilyHash(recStore))
	if repStr == repRec {
		t.Errorf("two Store families that differ only in map value type collapsed to one rep")
	}
}

func TestZZFamily_DistinctStoresDoNotCollide(t *testing.T) {
	cacheStore := zzStoreFamily("cache", typ.NewMap(typ.String, typ.String))
	snapStore := zzStoreFamily("sessions", typ.NewMap(typ.String, typ.NewRecord().Field("id", typ.String).Build()))

	repCache := CanonicalRecursiveFamily(cacheStore)
	repSnap := CanonicalRecursiveFamily(snapStore)

	t.Logf("cache rep: %s", repCache.String())
	t.Logf("snap rep:  %s", repSnap.String())
	t.Logf("phash cache=%d snap=%d", typ.ProductFamilyHash(cacheStore), typ.ProductFamilyHash(snapStore))

	if repCache == repSnap {
		t.Errorf("distinct Store families canonicalized to the SAME representative (collision): %s", repCache.String())
	}
	// And re-interning the cache store must return the cache rep, not the snap rep.
	repCache2 := CanonicalRecursiveFamily(zzStoreFamily("cache", typ.NewMap(typ.String, typ.String)))
	if repCache2 != repCache {
		t.Errorf("re-intern of cache Store gave a different rep")
	}
}
