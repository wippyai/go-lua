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
	source.Carriers = append(source.Carriers,
		generatorCarrier("Port", "carrier/self/port", port),
		generatorCarrier("PortOrdinalCarrier", "carrier/self/port-ordinal", definition.GoType{Name: "uint32"}))
	source.Relations = append(source.Relations, definition.Relation{
		Name: "Ports", Key: "self/ports", Subject: "Port",
		CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axis, Member: "self/ports"}),
		CandidateResolver: method("PortForOccurrence", owner),
		CandidateOrdinal:  method("PortOrdinal", owner),
		CandidateAt:       method("PortAt", owner),
		MemberParent:      member.RelationRef{Axis: axis, Member: "self/candidates"},
		MemberOrdinal:     "PortOrdinalCarrier",
		MemberCount:       method("PortCount", candidate),
		MemberAt:          method("PortAt", candidate),
	})
	source.Projections = append(source.Projections, definition.Projection{
		Name: "PortKey", Key: "self/port/key", Relation: "Ports", Role: member.Key, Result: "Key",
		CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: axis, Member: "self/ports"}),
		Accessor:          method("Key", port),
	})
	return source
}

// TestANestedMemberSetIsWholeOrAbsent states the declaration is one
// statement. A parent with no accessors names a set nothing can read,
// accessors with no parent name members of nothing, and a set with no ordinal
// carrier gives its members no address a consumer could reach them by. Any
// part alone is a declaration a generated owner could not honour.
func TestANestedMemberSetIsWholeOrAbsent(t *testing.T) {
	if !memberSetDefinition().Complete() {
		t.Fatal("a whole member-set declaration was refused")
	}
	for _, test := range []struct {
		name  string
		amend func(*definition.Relation)
	}{
		{name: "no-parent", amend: func(relation *definition.Relation) { relation.MemberParent = member.RelationRef{} }},
		{name: "no-ordinal", amend: func(relation *definition.Relation) { relation.MemberOrdinal = "" }},
		{name: "undeclared-ordinal", amend: func(relation *definition.Relation) { relation.MemberOrdinal = "Absent" }},
		{name: "no-count", amend: func(relation *definition.Relation) { relation.MemberCount = definition.GoSymbol{} }},
		{name: "no-accessor", amend: func(relation *definition.Relation) { relation.MemberAt = definition.GoSymbol{} }},
		{name: "parent-is-itself", amend: func(relation *definition.Relation) {
			relation.MemberParent = relation.CandidateProvider.AxisRelation
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
	relation.CandidateProvider = member.AxisRelationCandidate(member.RelationRef{Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "self"}, Member: "self/candidates"})
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

// TestANestedMemberSetSurvivesIntoTheColdCatalog is the clause a CHILD Program
// depends on, and the one that was missing.
//
// A member set declared on an owner was reaching only that owner's own
// bind-time accessors - Go symbols nobody outside the axis can call. A child
// Program consuming another axis's members reads the COLD catalog, so the
// parent it hangs off and the carrier its ordinal is keyed by have to be rows
// there. Without them the set exists and is unaddressable, which is
// indistinguishable from an axis that declares no members at all.
func TestANestedMemberSetSurvivesIntoTheColdCatalog(t *testing.T) {
	source := memberSetDefinition()
	catalog, catalogOK := source.Catalog()
	if !catalogOK {
		t.Fatal("a declared member set did not compose into a catalog")
	}
	nested, nestedOK := catalog.Relation("self/ports")
	if !nestedOK {
		t.Fatal("the member relation is not a catalog row")
	}
	if !nested.Parent.Available() || nested.Parent.Member != "self/candidates" {
		t.Fatalf("cold row parent = %+v, want the relation its members hang off", nested.Parent)
	}
	if !nested.Ordinal.Available() || nested.Ordinal != "carrier/self/port-ordinal" {
		t.Fatalf("cold row ordinal = %q, want the carrier its members are addressed by", nested.Ordinal)
	}
	plain, plainOK := catalog.Relation("self/candidates")
	if !plainOK {
		t.Fatal("the parent relation is not a catalog row")
	}
	if plain.Parent.Declared() || plain.Ordinal.Available() {
		t.Fatalf("a relation with no member set carries parent %+v ordinal %q", plain.Parent, plain.Ordinal)
	}

	rendered, err := Render("self", source)
	if err != nil {
		t.Fatalf("render member set: %v", err)
	}
	if !strings.Contains(string(rendered.Cold), "Parent: ") || !strings.Contains(string(rendered.Cold), "Ordinal: ") {
		t.Fatal("the generated cold catalog drops the nested relation metadata")
	}
}

// TestResolvedMetadataCarriesTheMemberSet states the typed half. An emitter
// building a child's direct calls needs the accessor pair and the ordinal's
// type, and Present is what separates an axis with no member set from one
// whose rows happen to be zero.
func TestResolvedMetadataCarriesTheMemberSet(t *testing.T) {
	metadata, err := Resolve(memberSetDefinition())
	if err != nil {
		t.Fatalf("resolve member set: %v", err)
	}
	var nested MemberSetBinding
	var plainPresent bool
	for _, relation := range metadata.Relations {
		switch relation.Key {
		case "self/ports":
			nested = relation.MemberSet
		case "self/candidates":
			plainPresent = relation.MemberSet.Present
		}
	}
	if !nested.Present {
		t.Fatal("the resolved member relation carries no member set")
	}
	if nested.Parent.Member != "self/candidates" || nested.Ordinal.Name != "uint32" {
		t.Fatalf("resolved member set = %+v", nested)
	}
	if nested.Count.Name != "PortCount" || nested.At.Name != "PortAt" {
		t.Fatalf("resolved accessors = %s/%s", nested.Count.Name, nested.At.Name)
	}
	if plainPresent {
		t.Fatal("a relation with no member set resolved one")
	}
}
