package axis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
	"github.com/wippyai/go-lua/analysis/schema/carrier"
)

// TestAxisNewStoresOwnerIssuedMemberAndOutputIdentities states the root
// issuance law: construction rows are raw, while the immutable catalog and
// frame retained by an admitted axis carry exact identities for their owner.
func TestAxisNewStoresOwnerIssuedMemberAndOutputIdentities(t *testing.T) {
	spec := scratchSpec("value", valueRole)
	spec.Catalog = testMemberCatalog()
	spec.Signature = Signature{Key: "axis/key", Fact: "axis/fact"}
	spec.Frame = Frame{Outputs: []Output{{Key: "value/facts", Writer: "value"}}}
	template := mustTemplate(t, spec)
	owner := schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: spec.Key}
	catalog := template.Catalog()
	relation, relationOK := catalog.RelationAt(0)
	projection, projectionOK := catalog.ProjectionAt(0)
	reducer, reducerOK := catalog.ReducerAt(0)
	if !relationOK || !projectionOK || !reducerOK {
		t.Fatal("issued catalog lost a declared row")
	}
	if relation.ID() != member.IssueID(owner, relation.Key) ||
		projection.ID() != member.IssueID(owner, projection.Key) ||
		reducer.ID() != member.IssueID(owner, reducer.Key) {
		t.Fatal("stored member row identity is not owner-qualified")
	}
	for _, key := range []carrier.Key{"relation/input", "relation/subject", "projection/result", "input/carrier", "output/carrier", "axis/key", "axis/fact"} {
		authority, authorityOK := catalog.Authority(key)
		expected, expectedOK := carrier.Issue(owner, carrier.Authority{Carrier: key, Capability: authority.Capability})
		if !authorityOK || !expectedOK || authority.ID() != expected.ID() {
			t.Fatalf("stored carrier authority %q identity is not owner-qualified: %+v/%t", key, authority, authorityOK)
		}
		if authority.ID() == relation.ID() {
			t.Fatalf("carrier authority %q reused the member issuance domain", key)
		}
	}
	output, outputOK := template.OutputAt(0)
	if !outputOK || output.ID() != member.IssueID(owner, output.Key) {
		t.Fatal("stored frame output identity is not owner-qualified")
	}
}

// TestAxisNewRejectsReissueAndOutputMemberCollision states the two hostile
// paths that must fail closed: an issued catalog cannot be supplied as raw
// input for another axis, and a frame output cannot reuse a member identity.
func TestAxisNewRejectsReissueAndOutputMemberCollision(t *testing.T) {
	base := scratchSpec("value", valueRole)
	base.Catalog = testMemberCatalog()
	base.Signature = Signature{Key: "axis/key", Fact: "axis/fact"}
	template := mustTemplate(t, base)
	reissued := base
	reissued.Catalog = template.Catalog()
	if candidate, ok := New(reissued); ok || candidate != nil {
		t.Fatal("axis admitted an already-issued catalog")
	}

	collisionCatalog, ok := member.NewCatalog(axisTestAuthorities("subject", "axis/key", "axis/fact"), []carrier.Binding{}, []member.Relation{{
		Key:               "value/facts",
		Subject:           "subject",
		CandidateProvider: member.AxisRelationCandidate(member.RelationRef{Axis: schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "value"}, Member: "value/facts"}),
	}}, nil, nil, nil)
	if !ok {
		t.Fatal("build collision catalog")
	}
	collision := scratchSpec("value", valueRole)
	collision.Catalog = collisionCatalog
	collision.Signature = Signature{Key: "axis/key", Fact: "axis/fact"}
	collision.Frame = Frame{Outputs: []Output{{Key: "value/facts", Writer: "value"}}}
	if candidate, ok := New(collision); ok || candidate != nil {
		t.Fatal("axis admitted an output/member identity collision")
	}
}
