package authorityprojection

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestProjectCarriesScopeRegionIntoResolvedDeclaration(t *testing.T) {
	fixture := newProjectionFixture(t)
	registry := projectionRegistry(t, fixture)
	if err := Project(registry, fixture.catalog, projectionResolver(fixture)); err != nil {
		t.Fatalf("project: %v", err)
	}

	scope, ok := fixture.catalog.ScopeAt(0)
	if !ok {
		t.Fatal("catalog scope unavailable")
	}
	declaration := registry.Declaration(model.SchemaID{})
	if len(declaration.Scopes) != 1 {
		t.Fatalf("projected scopes = %d, want one", len(declaration.Scopes))
	}
	projected := declaration.Scopes[0]
	if projected.Region().Identity() != scope.Region().Identity() {
		t.Fatalf("projected region identity = %v, want %v", projected.Region().Identity(), scope.Region().Identity())
	}
}
