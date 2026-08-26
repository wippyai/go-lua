package relationconstructor

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func readingSpec(key schema.Key, reads ...schema.Key) rule.Spec {
	joins := make([]ruleprogram.JoinDecl, 0, len(reads))
	for port, read := range reads {
		joins = append(joins, ruleprogram.JoinDecl{
			Read: ruleprogram.ReadDecl{Input: ruleprogram.InputRef(port)},
			Key: member.ProjectionRef{
				Axis:   schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"},
				Member: read,
			},
		})
	}
	return rule.Spec{Key: key, Program: ruleprogram.Program{Joins: joins}}
}

// TestTwoDerivationsOfOneRuleNameTheSameScopes is the stability law. The names
// are a function of the declaration alone, so two compilations of one catalog
// place a rule at the same scopes without carrying a scope table between them.
// Without this, two mounts of one program would answer in address spaces that
// only happen to agree.
func TestTwoDerivationsOfOneRuleNameTheSameScopes(t *testing.T) {
	spec := readingSpec("heap/allocation/closed", "heap/route-key", "heap/source-key")
	first, ok := DecisionScopes(spec)
	if !ok {
		t.Fatal("the first derivation refused a declared rule")
	}
	second, ok := DecisionScopes(readingSpec("heap/allocation/closed", "heap/route-key", "heap/source-key"))
	if !ok {
		t.Fatal("the second derivation refused the same rule")
	}
	if first.Candidate != second.Candidate {
		t.Fatalf("candidate scope differs: %v vs %v", first.Candidate, second.Candidate)
	}
	if len(first.Ports) != len(second.Ports) {
		t.Fatalf("port count differs: %d vs %d", len(first.Ports), len(second.Ports))
	}
	for index := range first.Ports {
		if first.Ports[index] != second.Ports[index] {
			t.Fatalf("port %d differs: %v vs %v", index, first.Ports[index], second.Ports[index])
		}
	}
}

// TestACandidateScopeIsTheRuleOwnKeyOnTheScopeSurface states what the name is,
// so a reader can spell it without calling this function. It also states that
// the scope does not collide with the rule's own entry: they are the same key
// on two surfaces, which is exactly why a scope needs no minted identity.
func TestACandidateScopeIsTheRuleOwnKeyOnTheScopeSurface(t *testing.T) {
	spec := readingSpec("heap/allocation/closed")
	placement, ok := DecisionScopes(spec)
	if !ok {
		t.Fatal("a rule with no declared port was refused")
	}
	if placement.Candidate.Entry.Surface != schema.SurfaceKindStructure {
		t.Fatal("the candidate scope is not a structure-surface entry")
	}
	if placement.Candidate.Entry.Key != spec.Key {
		t.Fatalf("candidate scope key = %q, want the rule's own key", placement.Candidate.Entry.Key)
	}
	if placement.Candidate.Entry.Surface == schema.SurfaceKindRule {
		t.Fatal("the candidate scope took the rule's own surface")
	}
	if len(placement.Ports) != 0 {
		t.Fatalf("a rule declaring no port was placed at %d port scopes", len(placement.Ports))
	}
}

// TestOnePortScopePerDeclaredPortDistinguishedByItsRead states that the port
// vector is total over the declared ports and that two ports are visibly
// different scopes. A derivation that named ports by ordinal alone would place
// two different reads at names a reader cannot tell apart.
func TestOnePortScopePerDeclaredPortDistinguishedByItsRead(t *testing.T) {
	placement, ok := DecisionScopes(readingSpec("heap/allocation/closed", "heap/route-key", "heap/source-key"))
	if !ok {
		t.Fatal("a two-port rule was refused")
	}
	if len(placement.Ports) != 2 {
		t.Fatalf("placed at %d port scopes, want 2", len(placement.Ports))
	}
	if placement.Ports[0] == placement.Ports[1] {
		t.Fatal("two ports were placed at one scope")
	}
	for index, port := range placement.Ports {
		if port.Entry.Surface != schema.SurfaceKindStructure {
			t.Fatalf("port %d is not a structure-surface entry", index)
		}
		if port == placement.Candidate {
			t.Fatalf("port %d took the candidate's own scope", index)
		}
	}
	// The read's declared key is carried, so changing only the read changes
	// only that port's scope.
	other, ok := DecisionScopes(readingSpec("heap/allocation/closed", "heap/route-key", "heap/other-key"))
	if !ok {
		t.Fatal("the varied rule was refused")
	}
	if other.Ports[0] != placement.Ports[0] {
		t.Fatal("an unchanged port changed scope")
	}
	if other.Ports[1] == placement.Ports[1] {
		t.Fatal("a port reading a different key kept its scope")
	}
}

// TestTwoRulesNeverShareAScope states the separation the whole derivation
// rests on: the rule's own key prefixes every scope it is placed at, so no two
// rules decide their candidates or observe their ports at one scope.
func TestTwoRulesNeverShareAScope(t *testing.T) {
	first, ok := DecisionScopes(readingSpec("heap/allocation/closed", "heap/route-key"))
	if !ok {
		t.Fatal("the first rule was refused")
	}
	second, ok := DecisionScopes(readingSpec("heap/allocation/empty", "heap/route-key"))
	if !ok {
		t.Fatal("the second rule was refused")
	}
	if first.Candidate == second.Candidate {
		t.Fatal("two rules decide their candidates at one scope")
	}
	if first.Ports[0] == second.Ports[0] {
		t.Fatal("two rules observe one port scope")
	}
}

// TestAMalformedPlacementRefuses states that a rule is placed wholly or not at
// all. An unnamed rule has no namespace to derive in, and two joins claiming
// one port would leave that port with two reads and no single scope.
func TestAMalformedPlacementRefuses(t *testing.T) {
	if _, ok := DecisionScopes(rule.Spec{}); ok {
		t.Fatal("an unnamed rule was placed")
	}
	contested := rule.Spec{Key: "heap/allocation/closed", Program: ruleprogram.Program{
		Joins: []ruleprogram.JoinDecl{
			{Read: ruleprogram.ReadDecl{Input: 0}, Key: member.ProjectionRef{
				Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}, Member: "heap/route-key"}},
			{Read: ruleprogram.ReadDecl{Input: 0}, Key: member.ProjectionRef{
				Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "heap"}, Member: "heap/source-key"}},
		},
	}}
	if _, ok := DecisionScopes(contested); ok {
		t.Fatal("two joins claimed one port and the rule was still placed")
	}
}
