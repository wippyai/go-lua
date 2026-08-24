package generator

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
)

// memberSetDefinition adds one nested ordered member set to the self-provided
// specimen: a second relation whose rows are the members one candidate row
// carries, addressed by its own directory.
func memberSetDefinition() definition.Definition {
	source := selfProviderDefinition()
	owner := definition.GoType{PackagePath: "example/self", Name: "Schema"}
	candidate := definition.GoType{PackagePath: "example/self", Name: "Candidate"}
	port := definition.GoType{PackagePath: "example/self", Name: "Port"}
	axis := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "self"}
	method := func(name string, receiver definition.GoType) definition.GoSymbol {
		return definition.GoSymbol{PackagePath: owner.PackagePath, Name: name, Receiver: receiver, ResultIndex: 0}
	}
	source.Carriers = append(source.Carriers, definition.Carrier{Name: "Port", Key: "carrier/self/port", Type: port})
	source.Relations = append(source.Relations, definition.Relation{
		Name: "Ports", Key: "self/ports", Subject: "Port",
		CandidateProvider: member.RelationRef{Axis: axis, Member: "self/ports"},
		CandidateResolver: method("PortForOccurrence", owner),
		CandidateOrdinal:  method("PortOrdinal", owner),
		CandidateAt:       method("PortAt", owner),
		MemberParent:      member.RelationRef{Axis: axis, Member: "self/candidates"},
		MemberCount:       method("PortCount", candidate),
		MemberAt:          method("PortAt", candidate),
	})
	source.Projections = append(source.Projections, definition.Projection{
		Name: "PortKey", Key: "self/port/key", Relation: "Ports", Role: member.Key, Result: "Key",
		CandidateProvider: member.RelationRef{Axis: axis, Member: "self/ports"},
		Accessor:          method("Key", port),
	})
	return source
}

// TestANestedMemberSetIsThreeRowsOrNone states the declaration is one
// statement. A parent with no accessors names a set nothing can read, and
// accessors with no parent name members of nothing; either half alone is a
// declaration a generated owner could not honour.
func TestANestedMemberSetIsThreeRowsOrNone(t *testing.T) {
	if !memberSetDefinition().Complete() {
		t.Fatal("a whole member-set declaration was refused")
	}
	for _, test := range []struct {
		name  string
		amend func(*definition.Relation)
	}{
		{name: "no-parent", amend: func(relation *definition.Relation) { relation.MemberParent = member.RelationRef{} }},
		{name: "no-count", amend: func(relation *definition.Relation) { relation.MemberCount = definition.GoSymbol{} }},
		{name: "no-accessor", amend: func(relation *definition.Relation) { relation.MemberAt = definition.GoSymbol{} }},
		{name: "parent-is-itself", amend: func(relation *definition.Relation) {
			relation.MemberParent = relation.CandidateProvider
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := memberSetDefinition()
			test.amend(&source.Relations[len(source.Relations)-1])
			if source.Complete() {
				t.Fatal("a half-declared member set was admitted")
			}
		})
	}
}

// TestAMemberSetRelationIsAddressedByItsOwnDirectory states why a member needs
// no projection language of its own. MemberAt answers a row of the member
// relation, and that relation is self-provided, so the coordinate a member
// projects to is reached through the same Project every other row uses. A
// member relation addressed by its parent's directory would have members that
// could be counted and never read.
func TestAMemberSetRelationIsAddressedByItsOwnDirectory(t *testing.T) {
	source := memberSetDefinition()
	relation := &source.Relations[len(source.Relations)-1]
	relation.CandidateProvider = member.RelationRef{Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "self"}, Member: "self/candidates"}
	relation.CandidateResolver = definition.GoSymbol{}
	relation.CandidateOrdinal = definition.GoSymbol{}
	relation.CandidateAt = definition.GoSymbol{}
	if source.Complete() {
		t.Fatal("a member set addressed by its parent's directory was admitted")
	}
}

// TestAGeneratedOwnerPublishesTheDeclaredMemberSet is the emission half: the
// declared set reaches the bind-time owner as the two accessors the engine
// asks a vector read's denominator through, and an axis that declares no set
// still publishes the pair as empty rather than omitting it.
func TestAGeneratedOwnerPublishesTheDeclaredMemberSet(t *testing.T) {
	rendered, err := Render("self", memberSetDefinition())
	if err != nil {
		t.Fatalf("render member set: %v", err)
	}
	owner := string(rendered.Relations)
	for _, want := range []string{
		"func (owner *RelationOwner) MemberCount(relationOrdinal, parentCandidateOrdinal uint32) (int, bool) {",
		"parent, parentOK := owner.schema.CandidateAt(int(parentCandidateOrdinal))",
		"count := parent.PortCount()",
		"member, memberOK := parent.PortAt(ordinal)",
		"return owner.schema.PortOrdinal(member)",
	} {
		if !strings.Contains(owner, want) {
			t.Fatalf("generated owner does not publish %q", want)
		}
	}

	empty, err := Render("self", selfProviderDefinition())
	if err != nil {
		t.Fatalf("render without member set: %v", err)
	}
	if !strings.Contains(string(empty.Relations), "func (owner *RelationOwner) MemberCount(relationOrdinal, parentCandidateOrdinal uint32) (int, bool) {\n\treturn 0, false\n}") {
		t.Fatal("an axis with no member set does not publish the empty census")
	}
}
