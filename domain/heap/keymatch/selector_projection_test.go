package keymatch_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/composite"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	keymatch "github.com/wippyai/go-lua/domain/heap/keymatch"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestSelectorProjectionIsCompleteUniqueCanonicalAndOwnerFenced(t *testing.T) {
	heap, values, _ := fixture(t, "keymatch_selector_projection", selectorProjectionSource(12))
	projection, sealed := keymatch.NewSelectorProjection(heap, values)
	if !sealed {
		t.Fatal("selector projection")
	}

	// The atom-level relation remains the independently meaningful baseline:
	// every valid atom projection must appear, while equal Heap selectors must
	// appear only once in the canonical relation projection.
	want := directDistinctSelectors(t, heap, values, values.Top())
	got := projectedSelectors(t, projection, values.Top())
	if !sameSelectorSequence(got, want) {
		t.Fatalf("selector projection differs from exact atom image: got=%d want=%d", len(got), len(want))
	}
	if selectorOccurrences(got, heapdomain.KeySelectorKinds, runtimekind.Bit(runtimekind.Table)) != 1 {
		t.Fatal("many table-summary atoms did not collapse to one Kinds(Table) selector")
	}
	if hasDuplicateSelector(got) {
		t.Fatal("selector projection emitted a duplicate Heap selector")
	}

	var support []valuedomain.Atom
	if !values.VisitSupport(values.Top(), func(atom valuedomain.Atom) { support = append(support, atom) }) {
		t.Fatal("Top support")
	}
	for left, right := 0, len(support)-1; left < right; left, right = left+1, right-1 {
		support[left], support[right] = support[right], support[left]
	}
	permuted, permutedOK := values.Alternatives(support...)
	if !permutedOK {
		t.Fatal("permuted Value relation")
	}
	if again := projectedSelectors(t, projection, permuted); !sameSelectorSequence(again, got) {
		t.Fatal("selector projection order depends on input construction order")
	}

	_, foreignValues, _ := fixture(t, "keymatch_selector_projection_foreign", selectorProjectionSource(2))
	if _, ok := keymatch.NewSelectorProjection(heap, foreignValues); ok {
		t.Fatal("selector projection accepted foreign Value owner")
	}
	if projection.Visit(foreignValues.Top(), func(heapdomain.KeySelector) bool { return true }) {
		t.Fatal("selector projection consumed a foreign Value relation")
	}
	if projection.Visit(values.Top(), nil) {
		t.Fatal("selector projection accepted nil visitor")
	}
}

func TestSelectorProjectionRejectsSameLinkResealedHeap(t *testing.T) {
	heap, values, linked := fixture(t, "keymatch_selector_projection_resealed", selectorProjectionSource(4))
	if !values.OwnsHeapSchema(heap) {
		t.Fatal("Value did not retain the exact Heap schema handle")
	}
	projection, ok := keymatch.NewSelectorProjection(heap, values)
	if !ok || projection == nil {
		t.Fatal("exact Value/Heap schema pair was rejected")
	}

	compilation, compilationOK := composite.Build()
	if !compilationOK {
		t.Fatal("keymatch compilation")
	}
	resealed, resealedFailure := heapdomain.SealWithArtifacts(linked, keymatchHeapMounts(t, linked, compilation))
	if resealedFailure != heapdomain.SealFailureNone || values.OwnsHeapSchema(resealed) {
		t.Fatal("independently resealed same-Link Heap was not distinguished")
	}
	if _, ok := keymatch.NewSelectorProjection(resealed, values); ok {
		t.Fatal("selector projection accepted independently resealed Heap")
	}
	atom, atomOK := values.OpaqueKind(runtimekind.Number)
	if !atomOK || !atom.TableKeyValidity().MayBeValid() {
		t.Fatal("Value key atom")
	}
	if _, ok := keymatch.Project(resealed, values, atom); ok {
		t.Fatal("Project accepted independently resealed Heap")
	}
}

// TestSelectorProjectionCollapsesRepeatedKindsAtScale captures the concrete
// former RawGet blow-up: every table allocation contributes a Summary atom,
// but all of those atoms denote the one Kinds(Table) selector.  The sizes are
// far apart solely to exercise arbitrary finite support; they are not a
// cardinality limit or a semantic cutoff.
func TestSelectorProjectionCollapsesRepeatedKindsAtScale(t *testing.T) {
	for _, count := range []int{64, 1024} {
		t.Run(selectorProjectionScaleName(count), func(t *testing.T) {
			heap, values, _ := fixture(t, "keymatch_selector_projection_scale", selectorProjectionSource(count))
			projection, sealed := keymatch.NewSelectorProjection(heap, values)
			if !sealed {
				t.Fatal("selector projection")
			}
			rawTableKinds := 0
			if !values.VisitSupport(values.Top(), func(atom valuedomain.Atom) {
				alternative, ok := keymatch.Project(heap, values, atom)
				if ok && alternative.Selector().Kind() == heapdomain.KeySelectorKinds && alternative.Selector().RuntimeKinds() == runtimekind.Bit(runtimekind.Table) {
					rawTableKinds++
				}
			}) {
				t.Fatal("Top atom projection")
			}
			selectors := projectedSelectors(t, projection, values.Top())
			if tableKinds := selectorOccurrences(selectors, heapdomain.KeySelectorKinds, runtimekind.Bit(runtimekind.Table)); tableKinds != 1 {
				t.Fatalf("scale %d emitted Kinds(Table) %d times, want once", count, tableKinds)
			}
			if rawTableKinds < count || rawTableKinds <= 1 {
				t.Fatalf("scale %d did not construct repeated raw table selectors: %d", count, rawTableKinds)
			}
			if hasDuplicateSelector(selectors) {
				t.Fatalf("scale %d emitted duplicate selectors", count)
			}
		})
	}
}

func TestSelectorProjectionWarmVisitAllocatesNothing(t *testing.T) {
	heap, values, _ := fixture(t, "keymatch_selector_projection_warm", selectorProjectionSource(256))
	projection, sealed := keymatch.NewSelectorProjection(heap, values)
	if !sealed {
		t.Fatal("selector projection")
	}
	want := len(projectedSelectors(t, projection, values.Top()))
	if want == 0 {
		t.Fatal("selector image")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		count := 0
		if !projection.Visit(values.Top(), func(heapdomain.KeySelector) bool {
			count++
			return true
		}) || count != want {
			panic("warm selector projection")
		}
	}); allocations != 0 {
		t.Fatalf("warm selector projection allocated %v times", allocations)
	}
}

func projectedSelectors(t testing.TB, projection *keymatch.SelectorProjection, value valuedomain.Value) []heapdomain.KeySelector {
	t.Helper()
	var selectors []heapdomain.KeySelector
	if projection == nil || !projection.Visit(value, func(selector heapdomain.KeySelector) bool {
		selectors = append(selectors, selector)
		return true
	}) {
		t.Fatal("selector projection visit")
	}
	return selectors
}

func directDistinctSelectors(t testing.TB, heap heapdomain.Schema, values *valuedomain.Schema, value valuedomain.Value) []heapdomain.KeySelector {
	t.Helper()
	var selectors []heapdomain.KeySelector
	if values == nil || !values.VisitSupport(value, func(atom valuedomain.Atom) {
		alternative, ok := keymatch.Project(heap, values, atom)
		if !ok || containsSelector(selectors, alternative.Selector()) {
			return
		}
		selectors = append(selectors, alternative.Selector())
	}) {
		t.Fatal("direct atom selector projection")
	}
	return selectors
}

func sameSelectorSequence(left, right []heapdomain.KeySelector) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !sameSelector(left[index], right[index]) {
			return false
		}
	}
	return true
}

func hasDuplicateSelector(selectors []heapdomain.KeySelector) bool {
	for index := range selectors {
		if containsSelector(selectors[:index], selectors[index]) {
			return true
		}
	}
	return false
}

func containsSelector(selectors []heapdomain.KeySelector, want heapdomain.KeySelector) bool {
	for _, selector := range selectors {
		if sameSelector(selector, want) {
			return true
		}
	}
	return false
}

func sameSelector(left, right heapdomain.KeySelector) bool {
	if left.Kind() != right.Kind() || left.RuntimeKinds() != right.RuntimeKinds() || left.ExactCount() != right.ExactCount() || left.ReferenceCount() != right.ReferenceCount() {
		return false
	}
	for index := 0; index < left.ExactCount(); index++ {
		leftKey, leftOK := left.ExactAt(index)
		rightKey, rightOK := right.ExactAt(index)
		if !leftOK || !rightOK || leftKey != rightKey {
			return false
		}
	}
	for index := 0; index < left.ReferenceCount(); index++ {
		leftReference, leftOK := left.ReferenceAt(index)
		rightReference, rightOK := right.ReferenceAt(index)
		if !leftOK || !rightOK || leftReference != rightReference {
			return false
		}
	}
	return true
}

func selectorOccurrences(selectors []heapdomain.KeySelector, kind heapdomain.KeySelectorKind, kinds runtimekind.Set) int {
	count := 0
	for _, selector := range selectors {
		if selector.Kind() == kind && selector.RuntimeKinds() == kinds {
			count++
		}
	}
	return count
}

func selectorProjectionSource(count int) string {
	if count < 1 {
		return "return nil"
	}
	var source strings.Builder
	for index := 0; index < count; index++ {
		source.WriteString("local item")
		source.WriteString(strconv.Itoa(index))
		source.WriteString(" = {}\n")
	}
	source.WriteString("return item0")
	return source.String()
}

func selectorProjectionScaleName(count int) string {
	if count == 64 {
		return "small"
	}
	if count == 1024 {
		return "large"
	}
	return "unexpected"
}
