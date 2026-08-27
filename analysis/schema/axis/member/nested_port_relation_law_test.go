package member_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

func portAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "call"}
}

func portProvider() member.RelationRef {
	return member.RelationRef{Axis: portAxis(), Member: "call/activation/candidates"}
}

func candidateRelation() member.Relation {
	return member.Relation{
		Key:               "call/activation/candidates",
		Subject:           "CallActivationCandidateCarrier",
		CandidateProvider: member.AxisRelationCandidate(portProvider()),
	}
}

func memberTestAuthorities(keys ...carrier.Key) []carrier.Authority {
	authorities := make([]carrier.Authority, 0, len(keys))
	seen := make(map[carrier.Key]struct{}, len(keys))
	for _, key := range keys {
		if !key.Available() {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		authorities = append(authorities, carrier.Authority{Carrier: key, Capability: carrier.DecodeOnly})
	}
	return authorities
}

// TestANestedRelationDeclaresBothItsParentAndItsOrdinalCarrier is the
// declaration law of an ordinal-addressed member set. A bounded ordered set of
// ports is a relation whose rows hang off one parent candidate and are
// addressed by an ordinal carrier: half of that statement is not a nested set
// at all, so a relation that names a parent without the carrier that keys it,
// or a carrier with no parent to key it against, is refused.
func TestANestedRelationDeclaresBothItsParentAndItsOrdinalCarrier(t *testing.T) {
	nested := member.Relation{
		Key:               "call/activation/ports",
		Subject:           "CallActivationPortCarrier",
		CandidateProvider: member.AxisRelationCandidate(portProvider()),
		Parent:            portProvider(),
		Ordinal:           "CallActivationPortOrdinalCarrier",
	}
	if !nested.Available() {
		t.Fatal("a relation naming both its parent and its ordinal carrier is a declarable nested member set")
	}
	orphanCarrier := nested
	orphanCarrier.Parent = member.RelationRef{}
	if orphanCarrier.Available() {
		t.Fatal("an ordinal carrier with no parent keys nothing")
	}
	unkeyed := nested
	unkeyed.Ordinal = ""
	if unkeyed.Available() {
		t.Fatal("a parented relation with no ordinal carrier has no address for its members")
	}
}

// TestANestedRelationNamesAParentTheCatalogHolds closes the nested set inside
// its own catalog: the parent it hangs off must be a relation the same axis
// declares, and a relation cannot be its own parent - a set addressed by its
// own rows has no base row to address from.
func TestANestedRelationNamesAParentTheCatalogHolds(t *testing.T) {
	parent := candidateRelation()
	nested := member.Relation{
		Key:               "call/activation/ports",
		Subject:           "CallActivationPortCarrier",
		CandidateProvider: member.AxisRelationCandidate(portProvider()),
		Parent:            member.RelationRef{Axis: portAxis(), Member: parent.Key},
		Ordinal:           "CallActivationPortOrdinalCarrier",
	}
	authorities := memberTestAuthorities("CallActivationCandidateCarrier", "CallActivationPortCarrier", "CallActivationPortOrdinalCarrier")
	if _, ok := member.NewCatalog(authorities, []carrier.Binding{}, []member.Relation{parent, nested}, nil, nil, nil); !ok {
		t.Fatal("a nested set whose parent the catalog declares is admissible")
	}
	foreign := nested
	foreign.Parent = member.RelationRef{Axis: portAxis(), Member: "call/activation/absent"}
	if _, ok := member.NewCatalog(authorities, []carrier.Binding{}, []member.Relation{parent, foreign}, nil, nil, nil); ok {
		t.Fatal("a nested set may not hang off a relation the catalog never declared")
	}
	reflexive := nested
	reflexive.Parent = member.RelationRef{Axis: portAxis(), Member: nested.Key}
	if _, ok := member.NewCatalog(authorities, []carrier.Binding{}, []member.Relation{parent, reflexive}, nil, nil, nil); ok {
		t.Fatal("a relation addressed by its own rows has no base row to address from")
	}
}

// TestTheAttributeRoleIsDeclarable states the fourth projection role. A
// candidate row carries columns that are neither the join key, the selection
// predicate, nor the write destination - a trigger context, a body context, a
// transition, a settled outcome - and a Program that cannot name them cannot
// read the row it joins on.
func TestTheAttributeRoleIsDeclarable(t *testing.T) {
	if !member.Attribute.Available() {
		t.Fatal("Attribute is a declared projection role")
	}
	projection := member.Projection{
		Key:               "call/activation/candidate-outcome",
		Relation:          "call/activation/candidates",
		Role:              member.Attribute,
		Result:            "CallActivationOutcomeCarrier",
		CandidateProvider: member.AxisRelationCandidate(portProvider()),
	}
	if !projection.Available() {
		t.Fatal("an attribute projection is a declarable member")
	}
	// Attribute is the last role whose projected value is a LOCAL: the address
	// of a row this analyzer minted, which the owner answers from Project. The
	// ordinal after it is Identity, which is read from a different surface
	// entirely, so this law's closure statement is about the local roles and
	// the vocabulary's own end is stated where Identity is declared.
	if beyond := member.Attribute + 1; beyond != member.Identity {
		t.Fatal("the local projection roles are closed at Attribute")
	}
}
