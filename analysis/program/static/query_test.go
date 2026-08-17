package static

import "testing"

func TestStaticQueryRootProjectsOwnedVerticalsAndFailsClosed(t *testing.T) {
	component := staticContentComponent(t, publicationFixture(t))
	view := component.View()
	if !view.Available() || view.Types().Primitives().Count() != 1 ||
		view.References().Count() != 2 || view.Declarations().Aliases().Count() != 1 ||
		view.Publications().Count() != 1 {
		t.Fatal("View did not project the authored top-level verticals")
	}
	var nilComponent *Component
	empty := nilComponent.View()
	if empty.Available() || empty.Types().Primitives().Count() != 0 ||
		empty.References().Count() != 0 || empty.Declarations().Aliases().Count() != 0 ||
		empty.Publications().Count() != 0 {
		t.Fatal("nil Component View did not fail closed")
	}
}
