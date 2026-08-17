package stdlib

import "testing"

func TestCatalogueIdentityNameAndMountLaws(t *testing.T) {
	libraries := Libraries()
	if len(libraries) == 0 {
		t.Fatal("empty standard-library catalogue")
	}
	if libraries[0].ID() != Package || libraries[1].ID() != Base {
		t.Fatalf("open prefix = [%q, %q], want [package, base]",
			libraries[0].ID(), libraries[1].ID())
	}

	ids := make(map[ID]bool, len(libraries))
	names := make(map[string]bool, len(libraries))
	for _, library := range libraries {
		if library.ID() == "" {
			t.Fatal("catalogue contains an empty identity")
		}
		if ids[library.ID()] {
			t.Fatalf("duplicate identity %q", library.ID())
		}
		ids[library.ID()] = true
		if names[library.Name()] {
			t.Fatalf("duplicate module name %q", library.Name())
		}
		names[library.Name()] = true

		if library.ID() == Base {
			if library.Name() != "" || library.Mount() != MountGlobals {
				t.Fatalf("base = name %q mount %d, want global mount", library.Name(), library.Mount())
			}
			continue
		}
		if library.Name() == "" || library.Mount() != MountModule {
			t.Fatalf("%q = name %q mount %d, want named module mount",
				library.ID(), library.Name(), library.Mount())
		}
	}

	copyOfCatalogue := Libraries()
	copyOfCatalogue[0] = Library{}
	if Libraries()[0].ID() != Package {
		t.Fatal("Libraries returned aliased catalogue storage")
	}
}

func TestBindRequiresExactCoverageAndPreservesCatalogueOrder(t *testing.T) {
	values := make(map[ID]string)
	for _, library := range Libraries() {
		values[library.ID()] = "provider:" + string(library.ID())
	}
	bound, err := Bind(values)
	if err != nil {
		t.Fatal(err)
	}
	for index, item := range bound {
		want := Libraries()[index]
		if item.Library.ID() != want.ID() || item.Value != "provider:"+string(want.ID()) {
			t.Fatalf("binding %d = (%q, %q), want %q", index,
				item.Library.ID(), item.Value, want.ID())
		}
	}

	delete(values, Math)
	if _, err := Bind(values); err == nil {
		t.Fatal("Bind admitted a missing provider")
	}
	values[Math] = "provider:math"
	values[ID("foreign")] = "provider:foreign"
	if _, err := Bind(values); err == nil {
		t.Fatal("Bind admitted a foreign provider")
	}
}
