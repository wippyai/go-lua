package program

import "testing"

func TestExecutableRootCatalogIsExplicitlyEmptyForNoExecutableRoots(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-executable-root-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	body, ok := published.BodyAt(0)
	if !ok {
		t.Fatal("BodyAt(0) unavailable")
	}
	roots, ok := body.ExecutableRoots()
	if !ok || !roots.Available() {
		t.Fatal("ExecutableRoots did not publish an explicit empty catalog")
	}
	if roots.Count() != 0 || body.ExecutableRootCount() != 0 {
		t.Fatalf("empty executable catalog counts = %d/%d", roots.Count(), body.ExecutableRootCount())
	}
	if published.OwnsExecutableRoots(roots) == false {
		t.Fatal("Program rejected its own empty executable-root catalog")
	}
	if _, ok := roots.At(0); ok {
		t.Fatal("ExecutableRootAt accepted an out-of-range row")
	}
	if _, ok := body.ExecutableRootAt(-1); ok {
		t.Fatal("ExecutableRootAt accepted a negative index")
	}
}
