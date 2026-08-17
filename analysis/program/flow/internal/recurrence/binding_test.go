package recurrence

import "testing"

func TestBindingCapabilitiesRequireAClaimedPlanAndResult(t *testing.T) {
	var binding Binding
	if binding.Matches(nil, nil) {
		t.Fatal("empty Binding matched absent Plan and Result")
	}
	if route, ok := binding.Claim(nil, 0); ok || route.Valid() {
		t.Fatal("empty Binding issued a route claim")
	}
	if rows, ok := binding.CompleteAndTakeDirectory(nil); ok || rows != nil {
		t.Fatalf("empty Binding transferred a directory: rows=%v ok=%t", rows, ok)
	}
	if hierarchy, ok := binding.CompleteAndTakeHierarchy(nil); ok || hierarchy.events != nil {
		t.Fatalf("empty Binding transferred a hierarchy: hierarchy=%v ok=%t", hierarchy, ok)
	}
	var component Component
	if head, ok := component.Head(); ok || head != 0 {
		t.Fatal("empty recurrence Component exposed a head")
	}
	if path, ok := component.HeadPath(); ok || path.Available() {
		t.Fatal("empty recurrence Component exposed a path")
	}
}
