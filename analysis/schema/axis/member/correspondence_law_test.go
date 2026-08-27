package member_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

func correspondenceOwnAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}
}

func correspondenceForeignAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "call"}
}

func correspondenceProvider() member.RelationRef {
	return member.RelationRef{Axis: correspondenceOwnAxis(), Member: "value/formal-freeze/call-actuals"}
}

func correspondenceForeign() member.RelationRef {
	return member.RelationRef{Axis: correspondenceForeignAxis(), Member: "call/mounted-call/candidates"}
}

// correspondingRelation is one self-provided relation stating that its own
// sealed order corresponds to Call's.
func correspondingRelation() member.Relation {
	return member.Relation{
		Key:               "value/formal-freeze/call-actuals",
		Subject:           "MountedCallActualsCarrier",
		CandidateProvider: member.AxisRelationCandidate(correspondenceProvider()),
		Correspondences:   []member.RelationRef{correspondenceForeign()},
	}
}

func correspondenceKeyProjection() member.Projection {
	return member.Projection{
		Key:               "value/formal-freeze/call-coordinate",
		Relation:          "value/formal-freeze/call-actuals",
		Role:              member.Key,
		Result:            "CallCoordinateCarrier",
		CandidateProvider: member.AxisRelationCandidate(correspondenceProvider()),
	}
}

func correspondenceCatalog(t *testing.T, relation member.Relation, projections []member.Projection) (member.Catalog, bool) {
	t.Helper()
	return member.NewCatalog(
		memberTestAuthorities("MountedCallActualsCarrier", "CallCoordinateCarrier"),
		[]carrier.Binding{}, []member.Relation{relation}, projections, nil, nil,
	)
}

// TestACorrespondenceCarriesTheForeignAxisAsItsUpwardReference holds the
// statement to the same seal machinery every other member reference answers
// to: the axis whose order is named must be one the composition proves exists,
// so the catalog publishes it as a reference rather than resolving it
// privately.
func TestACorrespondenceCarriesTheForeignAxisAsItsUpwardReference(t *testing.T) {
	catalog, ok := correspondenceCatalog(t, correspondingRelation(), []member.Projection{correspondenceKeyProjection()})
	if !ok {
		t.Fatal("a self-provided relation with one correspondence is a declarable catalog")
	}
	var named int
	for _, reference := range catalog.References() {
		if reference.Surface == schema.SurfaceKindAxis && reference.Key == "call" {
			named++
		}
	}
	if named != 1 {
		t.Fatalf("the catalog references the corresponded axis exactly once, got %d", named)
	}
}

// TestOnlyARelationWithAnOrderOfItsOwnCorresponds is the ownership law. A
// correspondence pairs two sealed orders, so the declaring relation must own
// one: a relation addressed through another authority's candidate has no
// enumeration of its own to pair, and admitting one would let an axis claim a
// correlation between two directories it does not own either side of.
func TestOnlyARelationWithAnOrderOfItsOwnCorresponds(t *testing.T) {
	if _, ok := correspondenceCatalog(t, correspondingRelation(), []member.Projection{correspondenceKeyProjection()}); !ok {
		t.Fatal("a self-provided relation with a complete correspondence is a declarable catalog")
	}
	provided := correspondingRelation()
	provided.CandidateProvider = member.AxisRelationCandidate(member.RelationRef{
		Axis: correspondenceOwnAxis(), Member: "value/mounted-call/argument-candidates",
	})
	projection := correspondenceKeyProjection()
	projection.CandidateProvider = provided.CandidateProvider
	if _, ok := correspondenceCatalog(t, provided, []member.Projection{projection}); ok {
		t.Fatal("a relation addressed through another authority's candidate has no order of its own to correspond")
	}
}

// TestACorrespondenceNamesAForeignOrder refuses the identity. A relation's own
// candidate directory is already how every rule addresses it, so a
// correspondence to that same axis states nothing the candidate provider does
// not already say, and admitting it would give one order two authorities.
func TestACorrespondenceNamesAForeignOrder(t *testing.T) {
	local := correspondingRelation()
	local.Correspondences = []member.RelationRef{{Axis: correspondenceOwnAxis(), Member: "value/mounted-call/argument-candidates"}}
	if _, ok := correspondenceCatalog(t, local, []member.Projection{correspondenceKeyProjection()}); ok {
		t.Fatal("a same-axis correspondence is the identity the candidate provider already spells")
	}
}

// TestOneCorrespondencePerForeignAxis keeps the correlation single-authored.
// Two statements about how one axis's order pairs with a foreign one are two
// answers to one question, and a reader would have no declared way to choose.
func TestOneCorrespondencePerForeignAxis(t *testing.T) {
	doubled := correspondingRelation()
	doubled.Correspondences = append(doubled.Correspondences, member.RelationRef{Axis: correspondenceForeignAxis(), Member: "call/mounted-call/facts"})
	if _, ok := correspondenceCatalog(t, doubled, []member.Projection{correspondenceKeyProjection()}); ok {
		t.Fatal("two correspondences to one foreign axis are two authorities over one correlation")
	}
}

// TestANestedMemberSetHangsOffItsOwnAxisParent is the refusal a correspondence
// exists to make unnecessary. The generated owner resolves a parent ordinal
// through the parent relation's directory on the LOCAL owner, so a parent on
// another axis names an ordinal this owner cannot address; the cold catalog
// refuses it here rather than letting it die in the emitter. A rule that must
// address a nested set from a foreign candidate declares a correspondence and
// keeps its member set at home.
func TestANestedMemberSetHangsOffItsOwnAxisParent(t *testing.T) {
	parent := member.Relation{
		Key:               "value/formal-freeze/call-actuals",
		Subject:           "MountedCallActualsCarrier",
		CandidateProvider: member.AxisRelationCandidate(correspondenceProvider()),
	}
	nested := member.Relation{
		Key:               "value/formal-freeze/actual-members",
		Subject:           "MountedCallArgumentCarrier",
		CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: correspondenceOwnAxis(), Member: "value/formal-freeze/actual-members"}),
		Parent:            correspondenceProvider(),
		Ordinal:           "MountedCallActualTagCarrier",
	}
	authorities := memberTestAuthorities("MountedCallActualsCarrier", "MountedCallArgumentCarrier", "MountedCallActualTagCarrier")
	if _, ok := member.NewCatalog(authorities, []carrier.Binding{}, []member.Relation{parent, nested}, nil, nil, nil); !ok {
		t.Fatal("a nested member set parented on a relation of its own axis is declarable")
	}
	foreign := nested
	foreign.Parent = member.RelationRef{Axis: correspondenceForeignAxis(), Member: parent.Key}
	if _, ok := member.NewCatalog(authorities, []carrier.Binding{}, []member.Relation{parent, foreign}, nil, nil, nil); ok {
		t.Fatal("a nested member set cannot hang off a parent this owner has no directory for")
	}
	absent := nested
	absent.Parent = correspondenceForeign()
	if _, ok := member.NewCatalog(authorities, []carrier.Binding{}, []member.Relation{parent, absent}, nil, nil, nil); ok {
		t.Fatal("a nested member set cannot hang off a relation the catalog does not declare")
	}
}
