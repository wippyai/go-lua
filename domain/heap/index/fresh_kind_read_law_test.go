package index_test

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// A fresh root's kind is the same for every fresh root, and the catalog that
// holds those rows proved them when it was built. Reading a fresh Key's kind
// is therefore a membership question the count answers, and it must not build
// the Root row out of the catalog only to read a constant off it and throw
// the row away. Key.Kind runs once per published query row per evaluation, so
// a materialization there is paid at the rate of the whole publication.
func TestFreshKeyKindReadsNoCatalogRow(t *testing.T) {
	linked := moduleExportTopologyLink(t)
	heap, _, _, _ := indexSchemas(t, linked)

	var freshKey heapdomain.Key
	for index := 0; index < heap.FreshCount(); index++ {
		_, candidate, candidateOK := heap.FreshAt(index)
		if !candidateOK || !candidate.Valid() {
			continue
		}
		freshKey = candidate
		break
	}
	if !freshKey.Valid() {
		t.Fatalf("fixture published no fresh root (fresh=%d)", heap.FreshCount())
	}

	heapdomain.DbgHeapReset()
	if kind := freshKey.Kind(); kind != heapdomain.RootAllocation {
		t.Fatalf("fresh root kind = %v, want RootAllocation", kind)
	}
	if built := heapdomain.DbgHeap().FreshRootMaterializations; built != 0 {
		t.Errorf("reading one fresh Key's kind built %d catalog rows: the kind is constant over fresh rows and the row's existence is the catalog's count", built)
	}
}
