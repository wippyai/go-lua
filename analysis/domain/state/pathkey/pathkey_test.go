package pathkey

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
)

func TestDeleteSubtreeVersionedKeysUseSegmentBoundaries(t *testing.T) {
	reg := standard.Registry()
	present := presentValue(reg)
	in := map[pathdom.PathKey]product.Value{
		pathdom.PathKey("sym40@3"):            present,
		pathdom.PathKey("sym40@3.field"):      present,
		pathdom.PathKey("sym40@3.field.deep"): present,
		pathdom.PathKey("sym40@3.fieldish"):   present,
		pathdom.PathKey("sym40.field.deep"):   present,
		pathdom.PathKey("sym40@4.field.deep"): present,
		pathdom.PathKey("sym41@3.field.deep"): present,
	}

	out, changed, ok := DeleteSubtree(in, pathdom.PathKey("sym40@3.field"))
	if !ok || !changed {
		t.Fatalf("DeleteSubtree(versioned prefix) = changed=%v ok=%v, want true/true", changed, ok)
	}
	assertAbsent(t, out, pathdom.PathKey("sym40@3.field"))
	assertAbsent(t, out, pathdom.PathKey("sym40@3.field.deep"))
	assertPresent(t, out, pathdom.PathKey("sym40@3"))
	assertPresent(t, out, pathdom.PathKey("sym40@3.fieldish"))
	assertPresent(t, out, pathdom.PathKey("sym40.field.deep"))
	assertPresent(t, out, pathdom.PathKey("sym40@4.field.deep"))
	assertPresent(t, out, pathdom.PathKey("sym41@3.field.deep"))
	assertPresent(t, in, pathdom.PathKey("sym40@3.field.deep"))
}

func TestDeleteSubtreeSupportsPlaceholderAndStableRoots(t *testing.T) {
	reg := standard.Registry()
	present := presentValue(reg)
	in := map[pathdom.PathKey]product.Value{
		pathdom.PathKey("$0.field"):          present,
		pathdom.PathKey("$0.field.deep"):     present,
		pathdom.PathKey("$0.fieldish"):       present,
		pathdom.PathKey("ret[1].field"):      present,
		pathdom.PathKey("ret[1].field.deep"): present,
		pathdom.PathKey("ret[1].fieldish"):   present,
	}

	out, changed, ok := DeleteSubtree(in, pathdom.PathKey("$0.field"))
	if !ok || !changed {
		t.Fatalf("DeleteSubtree(placeholder) = changed=%v ok=%v, want true/true", changed, ok)
	}
	assertAbsent(t, out, pathdom.PathKey("$0.field"))
	assertAbsent(t, out, pathdom.PathKey("$0.field.deep"))
	assertPresent(t, out, pathdom.PathKey("$0.fieldish"))
	assertPresent(t, out, pathdom.PathKey("ret[1].field"))

	out, changed, ok = DeleteSubtree(in, pathdom.PathKey("ret[1].field"))
	if !ok || !changed {
		t.Fatalf("DeleteSubtree(stable) = changed=%v ok=%v, want true/true", changed, ok)
	}
	assertAbsent(t, out, pathdom.PathKey("ret[1].field"))
	assertAbsent(t, out, pathdom.PathKey("ret[1].field.deep"))
	assertPresent(t, out, pathdom.PathKey("ret[1].fieldish"))
	assertPresent(t, out, pathdom.PathKey("$0.field"))
}

func TestDeleteDescendantsKeepsContainerButRemovesChildren(t *testing.T) {
	reg := standard.Registry()
	present := presentValue(reg)
	in := map[pathdom.PathKey]product.Value{
		pathdom.PathKey("sym60@2.item"):            present,
		pathdom.PathKey("sym60@2.item.count"):      present,
		pathdom.PathKey("sym60@2.item.name.first"): present,
		pathdom.PathKey("sym60@2.itemized.count"):  present,
		pathdom.PathKey("$0.item"):                 present,
		pathdom.PathKey("$0.item.count"):           present,
	}

	out, changed, ok := DeleteDescendants(in, pathdom.PathKey("sym60@2.item"))
	if !ok || !changed {
		t.Fatalf("DeleteDescendants(versioned prefix) = changed=%v ok=%v, want true/true", changed, ok)
	}
	assertPresent(t, out, pathdom.PathKey("sym60@2.item"))
	assertAbsent(t, out, pathdom.PathKey("sym60@2.item.count"))
	assertAbsent(t, out, pathdom.PathKey("sym60@2.item.name.first"))
	assertPresent(t, out, pathdom.PathKey("sym60@2.itemized.count"))
	assertPresent(t, out, pathdom.PathKey("$0.item.count"))

	out, changed, ok = DeleteDescendants(in, pathdom.PathKey("$0.item"))
	if !ok || !changed {
		t.Fatalf("DeleteDescendants(placeholder prefix) = changed=%v ok=%v, want true/true", changed, ok)
	}
	assertPresent(t, out, pathdom.PathKey("$0.item"))
	assertAbsent(t, out, pathdom.PathKey("$0.item.count"))
}

func TestDeleteRejectsInvalidSpellings(t *testing.T) {
	reg := standard.Registry()
	present := presentValue(reg)
	in := map[pathdom.PathKey]product.Value{
		pathdom.PathKey("sym1@1.field"): present,
	}

	for _, prefix := range []pathdom.PathKey{
		pathdom.PathKey(""),
		pathdom.PathKey(".field"),
		pathdom.PathKey("sym1@1[bad]"),
		pathdom.PathKey("sym1@1."),
		pathdom.PathKey("sym1.field"),
		pathdom.PathKey("ret[1"),
	} {
		if out, changed, ok := DeleteSubtree(in, prefix); ok || changed || !sameMap(out, in) {
			t.Fatalf("DeleteSubtree(%q) = changed=%v ok=%v, want unchanged/false", prefix, changed, ok)
		}
		if out, changed, ok := DeleteDescendants(in, prefix); ok || changed || !sameMap(out, in) {
			t.Fatalf("DeleteDescendants(%q) = changed=%v ok=%v, want unchanged/false", prefix, changed, ok)
		}
	}
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func assertPresent(t *testing.T, m map[pathdom.PathKey]product.Value, key pathdom.PathKey) {
	t.Helper()
	if _, ok := m[key]; !ok {
		t.Fatalf("expected %s to remain present", key)
	}
}

func assertAbsent(t *testing.T, m map[pathdom.PathKey]product.Value, key pathdom.PathKey) {
	t.Helper()
	if _, ok := m[key]; ok {
		t.Fatalf("expected %s to be removed", key)
	}
}

func sameMap(a, b map[pathdom.PathKey]product.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || av != bv {
			return false
		}
	}
	return true
}
