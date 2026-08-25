package member

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

func axisEntry(key schema.Key) schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: key}
}

// TestOneAssignerIssuesEveryMemberIdentity states that relations,
// projections, reducers and carry transforms are named by one assigner. A
// member key is unique within its axis catalog whatever kind it is, so two
// references to one member resolve to one identity and no kind carries an
// assigner of its own.
func TestOneAssignerIssuesEveryMemberIdentity(t *testing.T) {
	heap := axisEntry("heap")
	direct := IssueID(heap, "heap/routes")
	if !direct.Available() {
		t.Fatal("the assigner issued no identity for a declared member")
	}
	relation := RelationRef{Axis: heap, Member: "heap/routes"}
	projection := ProjectionRef{Axis: heap, Member: "heap/routes"}
	reducer := ReducerRef{Axis: heap, Member: "heap/routes"}
	transform := CarryTransformRef{Axis: heap, Member: "heap/routes"}
	for label, issued := range map[string]schema.EntryID{
		"relation":   relation.ID(),
		"projection": projection.ID(),
		"reducer":    reducer.ID(),
		"transform":  transform.ID(),
	} {
		if issued != direct {
			t.Fatalf("%s reference resolves to a second identity for one member", label)
		}
	}
}

// TestMemberIdentityIsFencedByItsAxis states that a member identity carries
// its owner: one member key declared on two axes names two members, and a
// member never collides with the axis entry that issued it.
func TestMemberIdentityIsFencedByItsAxis(t *testing.T) {
	heap := IssueID(axisEntry("heap"), "facts")
	value := IssueID(axisEntry("value"), "facts")
	if heap == value {
		t.Fatal("one member key on two axes resolves to one identity")
	}
	if heap == schema.NewEntryID(schema.SurfaceKindAxis, "heap") {
		t.Fatal("a member carries the identity of the axis that issued it")
	}
	other := IssueID(axisEntry("heap"), "routes")
	if heap == other {
		t.Fatal("two members of one axis resolve to one identity")
	}
}

// TestMemberIdentityIsStableAndFailsClosed states that issuance depends on the
// authored names alone, and that an incomplete reference issues no identity
// rather than a usable one.
func TestMemberIdentityIsStableAndFailsClosed(t *testing.T) {
	first := IssueID(axisEntry("heap"), "heap/routes")
	second := IssueID(axisEntry("heap"), "heap/routes")
	if first != second {
		t.Fatal("issuance is not a function of the authored names")
	}
	if IssueID(schema.EntryReference{}, "heap/routes").Available() {
		t.Fatal("an unavailable axis issued a member identity")
	}
	if IssueID(axisEntry("heap"), "").Available() {
		t.Fatal("an unavailable member key issued a member identity")
	}
	if (RelationRef{}).ID().Available() {
		t.Fatal("an undeclared relation reference issued a member identity")
	}
}
