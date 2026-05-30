package product

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func zzStore(mapValue typ.Type) typ.Type {
	return typ.NewRecursive("Store", func(self typ.Type) typ.Type {
		return typ.NewRecord().
			Field("data", typ.NewMap(typ.String, mapValue)).
			Field("get", typ.Func().Param("self", self).Param("k", typ.String).Returns(typ.NewOptional(mapValue)).Build()).
			Field("put", typ.Func().Param("self", self).Param("v", mapValue).Returns(self).Build()).
			Build()
	})
}

func TestZZIntern_DistinctStoreProductsStayDistinct(t *testing.T) {
	strStore := zzStore(typ.String)
	recStore := zzStore(typ.NewRecord().Field("id", typ.String).Field("flags", typ.NewMap(typ.String, typ.Boolean)).Build())

	avStr := FromType(strStore)
	avRec := FromType(recStore)

	projStr := avStr.ProjectValue()
	projRec := avRec.ProjectValue()
	t.Logf("proj str: %s", projStr.String())
	t.Logf("proj rec: %s", projRec.String())

	if typ.TypeEquals(projStr, projRec) {
		t.Errorf("distinct Store products projected to TypeEquals values (contamination): str=%s rec=%s", projStr.String(), projRec.String())
	}
	// Re-project the str store fresh; must still be the str body.
	avStr2 := FromType(zzStore(typ.String))
	if !typ.TypeEquals(avStr2.ProjectValue(), projStr) {
		t.Errorf("re-projection of str Store changed: %s vs %s", avStr2.ProjectValue().String(), projStr.String())
	}
}
