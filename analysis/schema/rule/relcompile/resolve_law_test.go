package relcompile_test

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	arithmetic "github.com/wippyai/go-lua/domain/value/arithmetic/program"
)

func refusalOf(t *testing.T, err error) relcompile.Refusal {
	t.Helper()
	var refusal relcompile.Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("error %v is not a Refusal", err)
	}
	return refusal
}

// TestAuthoredDeclarationResolvesAndLowers states that a whole authored rule
// declaration - candidate, ordered equijoins, scope per observed input port,
// one typed Apply, one carried alternative and one publication - resolves
// through the canonical registry and lowers into the closed algebra.
func TestAuthoredDeclarationResolvesAndLowers(t *testing.T) {
	surfaces := newOwners(t)
	spec := arithmetic.RuleEntry()
	placement := surfaces.install(spec)

	rules, err := relcompile.Resolve(surfaces.registry, spec, placement)
	if err != nil {
		t.Fatalf("resolve %s: %v", spec.Key, err)
	}
	if len(rules) != 1 {
		t.Fatalf("resolved rules = %d, want one per published column", len(rules))
	}
	resolved := rules[0]
	if len(resolved.Joins) != 2 {
		t.Fatalf("joins = %d, want the two authored reads", len(resolved.Joins))
	}
	for index, join := range resolved.Joins {
		if len(join.LeftColumns) != 1 || len(join.RightColumns) != 1 {
			t.Fatalf("join %d is not a single-column oriented equijoin", index)
		}
		if join.RightColumns[0].Relation() != join.Relation {
			t.Fatalf("join %d pairs against a column the joined relation does not own", index)
		}
		if !join.Scope.Available() {
			t.Fatalf("join %d observes no decision scope", index)
		}
		if join.Complete != nil {
			t.Fatalf("join %d fabricated a completion for an explicitly sparse read", index)
		}
	}
	if resolved.Carry == nil {
		t.Fatal("the authored identity carry was dropped")
	}
	if resolved.Carry.Transform != nil {
		t.Fatal("an identity carry named a transform")
	}
	if resolved.Publish == nil || resolved.Carry.Relation != resolved.Publish.Relation {
		t.Fatal("the carried alternative is not the published destination")
	}
}

// TestCarriedDerivationLowersToMerge states that the authored whole-output
// carry becomes the destination key's Merge of two derivations, never a
// carry form of its own.
func TestCarriedDerivationLowersToMerge(t *testing.T) {
	surfaces := newOwners(t)
	spec := arithmetic.RuleEntry()
	placement := surfaces.install(spec)
	rules, err := relcompile.Resolve(surfaces.registry, spec, placement)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	owner, err := surfaces.registry.Owner(relcompile.Site{Path: "test"}, schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: spec.Writes})
	if err != nil {
		t.Fatalf("resolve owner: %v", err)
	}
	schemaID, ok := model.IssueSchemaID(owner, surfaces.token("schema", relcompile.EntryName(schema.SurfaceKindRule, spec.Key)))
	if !ok {
		t.Fatal("issue schema identity")
	}
	declaration := surfaces.registry.Declaration(schemaID)
	declaration.Rules = rules
	compiled, err := relcompile.Compile(declaration)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(compiled.Expressions()) != 1 {
		t.Fatalf("expressions = %d, want one per publication", len(compiled.Expressions()))
	}
	root := compiled.Expressions()[0].Expression()
	published, ok := root.(algebra.Publish)
	if !ok {
		t.Fatalf("root = %T, want Publish", root)
	}
	merged, ok := published.Child().(algebra.Merge)
	if !ok {
		t.Fatalf("published child = %T, want Merge of the produced and carried derivations", published.Child())
	}
	if len(merged.Inputs()) != 2 {
		t.Fatalf("merge inputs = %d, want the produced rows and the carried rows", len(merged.Inputs()))
	}
	if merged.Contract().Key() != published.Contract().Key() {
		t.Fatal("the carried derivation merges under a key other than the publication key")
	}
	if _, ok := merged.Inputs()[0].(algebra.Apply); !ok {
		t.Fatalf("first merge input = %T, want the semantic Apply", merged.Inputs()[0])
	}
}

// TestUnresolvedReferenceNamesTheRuleAndSite states that a reference the
// owning surface never installed refuses at the authored path inside the rule
// that named it, and reports which owner statement is missing.
func TestUnresolvedReferenceNamesTheRuleAndSite(t *testing.T) {
	surfaces := newOwners(t)
	spec := arithmetic.RuleEntry()
	placement := surfaces.install(spec)

	partial := newOwners(t)
	_, err := relcompile.Resolve(partial.registry, spec, placement)
	if err == nil {
		t.Fatal("a declaration resolved against a registry no owner installed into")
	}
	refusal := refusalOf(t, err)
	if refusal.Reason != relcompile.ReasonUnknown {
		t.Fatalf("reason = %v, want unknown", refusal.Reason)
	}
	if refusal.Site.Rule != spec.Key {
		t.Fatalf("refusal rule = %q, want %q", refusal.Site.Rule, spec.Key)
	}
	if refusal.Site.Path == "" {
		t.Fatal("the refusal names no declaration path")
	}
	if refusal.Kind == relcompile.KindInvalid {
		t.Fatal("the refusal names no missing owner statement")
	}
}
