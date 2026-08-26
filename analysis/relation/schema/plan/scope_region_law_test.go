package plan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

func scopeRegionLawToken(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relation/schema/plan/scope-region-law/v1", []byte(label))
	if !ok {
		t.Fatalf("derive %q", label)
	}
	return value
}

func TestScopeRegionParticipatesInExecutionSchemaDigest(t *testing.T) {
	owner, ok := model.IssueOwnerID(scopeRegionLawToken(t, "owner"))
	if !ok {
		t.Fatal("issue owner")
	}
	scopeID, ok := model.IssueScopeID(owner, scopeRegionLawToken(t, "scope"))
	if !ok {
		t.Fatal("issue scope")
	}
	schemaID, ok := model.IssueSchemaID(owner, scopeRegionLawToken(t, "schema"))
	if !ok {
		t.Fatal("issue schema")
	}

	build := func(value model.ScopeSchema) ExecutionSchema {
		t.Helper()
		builder := NewBuilder(schemaID)
		if !builder.AddScope(value) {
			t.Fatal("add scope")
		}
		compiled, buildOK := builder.Build()
		if !buildOK || !compiled.Available() {
			t.Fatal("build execution schema")
		}
		return compiled
	}

	whole := model.DefineScopeSchema(scopeID, nil, region.True())
	never := model.DefineScopeSchema(scopeID, nil, region.False())
	if whole.Region().Identity() == never.Region().Identity() {
		t.Fatal("test regions unexpectedly share an identity")
	}
	first, second := build(whole), build(never)
	if first.Digest() == second.Digest() {
		t.Fatal("scope region was omitted from execution-schema digest")
	}
	if got := first.Scopes()[0].Region().Identity(); got != whole.Region().Identity() {
		t.Fatal("execution schema did not retain the exact scope region")
	}
}
