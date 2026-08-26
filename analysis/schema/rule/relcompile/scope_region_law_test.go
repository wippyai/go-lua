package relcompile

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

func TestInstallScopeRetainsExactRegionInDeclaration(t *testing.T) {
	registry := NewRegistry()
	owner := axisEntry("scope-region")
	if err := registry.InstallOwner(owner, issue(t, "scope-region/owner")); err != nil {
		t.Fatalf("install owner: %v", err)
	}
	name := NewName(owner, "scope")
	formula := region.True()
	if err := registry.InstallScope(name, issue(t, "scope-region/scope"), formula); err != nil {
		t.Fatalf("install scope: %v", err)
	}
	declaration := registry.Declaration(model.SchemaID{})
	if len(declaration.Scopes) != 1 {
		t.Fatalf("projected scopes = %d, want one", len(declaration.Scopes))
	}
	projected := declaration.Scopes[0]
	if projected.Region().Identity() != formula.Identity() {
		t.Fatalf("projected region identity = %v, want %v", projected.Region().Identity(), formula.Identity())
	}
	if projected.Region().Identity() == (region.False()).Identity() {
		t.Fatal("projected region was replaced with a fallback formula")
	}
}

func TestInstallScopeRefusesUnavailableRegionWithoutClaimingIdentity(t *testing.T) {
	registry := NewRegistry()
	owner := axisEntry("scope-region-retry")
	if err := registry.InstallOwner(owner, issue(t, "scope-region-retry/owner")); err != nil {
		t.Fatalf("install owner: %v", err)
	}
	name := NewName(owner, "scope")
	token := issue(t, "scope-region-retry/scope")
	if err := registry.InstallScope(name, token, region.Region{}); err == nil || refusalOf(t, err).Reason != ReasonUnavailable {
		t.Fatalf("unavailable region refusal = %v, want unavailable", err)
	}
	if err := registry.InstallScope(name, token, region.True()); err != nil {
		t.Fatalf("valid retry after unavailable region: %v", err)
	}
}
