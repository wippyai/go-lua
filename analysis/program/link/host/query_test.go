package host

import "testing"

func TestHostQueriesRebindPortableBootIdentity(t *testing.T) {
	project, boundary, module := hostFixture(t)
	draft, err := Build(Input{Project: project, Boundary: boundary, Module: module})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	root, ok := component.BootRoots().At(0)
	if !ok {
		t.Fatal("boot root unavailable")
	}
	id, ok := component.BootRoots().ID(root)
	if !ok {
		t.Fatal("boot root identity unavailable")
	}
	if _, _, ok := component.BootRoots().Mapping(root); !ok {
		t.Fatal("BootRoot Mapping did not return the issued handle")
	}
	if _, _, ok := component.BootRoots().Mapping(BootRoot{}); ok {
		t.Fatal("zero BootRoot identity resolved")
	}
	if got, ok := component.BootRoots().ID(root); !ok || got != id {
		t.Fatal("BootRoot ID did not remain available")
	}
}
