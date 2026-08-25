package engine

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// mountedPointLawID supplies stable, disjoint identities for the small
// capability and source-key laws below. The tests deliberately stop at the
// engine admission boundary; a full artifact fixture is not needed to prove
// the lane and identity fences.
func mountedPointLawID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = 0xe7
	id[1] = value
	return id
}

type mountedPointCapabilityLawFixture struct {
	binding *SchemaBinding
	rule    *RuleSlot[uint64, struct{}]
	cap     RuleSlotCapability
}

func newMountedPointCapabilityLawFixture(t testing.TB, register bool) mountedPointCapabilityLawFixture {
	t.Helper()
	schema, factor, rule, write := zeroWriteRuleLawSchema(t, 0)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, struct{}](binding, rule, write, factor, lawHotRuleSpec(), testRuleProjector[struct{}]) {
		t.Fatal("mounted-point Rule binding")
	}
	var capability RuleSlotCapability
	var capabilityOK bool
	if register {
		capability, capabilityOK = RegisterMountedPointSlot(binding, rule)
	} else {
		capability, capabilityOK = IssueMountedPointRuleCapability(binding, rule)
	}
	if !capabilityOK {
		t.Fatal("mounted-point capability")
	}
	return mountedPointCapabilityLawFixture{binding: binding, rule: rule, cap: capability}
}

func sealMountedPointCapabilityLawFixture(t testing.TB, fixture mountedPointCapabilityLawFixture) mountedPointCapabilityLawFixture {
	t.Helper()
	if !fixture.binding.Seal() {
		t.Fatal("mounted-point capability seal")
	}
	sealed, sealedOK := MountedPointCapabilityForSlot(fixture.binding, fixture.rule)
	if !sealedOK || !sealed.MountedPoint() {
		t.Fatal("sealed mounted-point capability")
	}
	fixture.cap = sealed
	return fixture
}

func TestMountedPointCapabilityRejectsWrongLaneAndForeignCapability(t *testing.T) {
	local := newMountedPointCapabilityLawFixture(t, false)
	ordinary, ordinaryOK := IssueMountedRuleCapability(local.binding, local.rule)
	if !ordinaryOK || !ordinary.Mounted() || ordinary.MountedPoint() {
		t.Fatal("ordinary mounted capability")
	}
	if _, accepted := admitMountedPointRuleIssuances(nil, nil, nil, executioncontext.Directory{}, nil, MountedPointRuleAdmission{
		Capability: ordinary,
		Occurrence: mountedPointLawID(1),
	}); accepted {
		t.Fatal("mounted-point admission accepted an ordinary mounted capability")
	}

	foreign := sealMountedPointCapabilityLawFixture(t, newMountedPointCapabilityLawFixture(t, true))
	if RegisterRuleSlot(local.binding, local.rule, foreign.cap) {
		t.Fatal("foreign mounted-point capability crossed the local binding")
	}
	if _, ok := RegisterMountedPointSlot(local.binding, local.rule); !ok || !local.binding.Seal() {
		t.Fatal("local mounted-point capability registration")
	}
	sealed, sealedOK := MountedPointCapabilityForSlot(local.binding, local.rule)
	if !sealedOK || !sealed.MountedPoint() {
		t.Fatal("local mounted-point capability was not sealed")
	}
	if sealed == foreign.cap {
		t.Fatal("local and foreign mounted-point capabilities aliased")
	}
}

func TestMountedPointAdmissionHasOneOccurrenceAndNoQuerySurface(t *testing.T) {
	typeOfAdmission := reflect.TypeOf(MountedPointRuleAdmission{})
	if typeOfAdmission.NumField() != 2 || typeOfAdmission.Field(0).Name != "Capability" || typeOfAdmission.Field(1).Name != "Occurrence" {
		t.Fatalf("mounted-point admission fields = %v, want Capability/Occurrence only", typeOfAdmission)
	}

	fixture := sealMountedPointCapabilityLawFixture(t, newMountedPointCapabilityLawFixture(t, true))
	row := MountedPointRuleAdmission{Capability: fixture.cap, Occurrence: mountedPointLawID(2)}
	if !row.Capability.MountedPoint() || !row.Occurrence.Available() {
		t.Fatal("mounted-point admission row")
	}
}

func TestMountedPointMemberIdentityIsOnePerPointAndStableAtSamePoint(t *testing.T) {
	fixture := sealMountedPointCapabilityLawFixture(t, newMountedPointCapabilityLawFixture(t, true))
	mount, occurrence := mountedPointLawID(3), mountedPointLawID(4)
	points := []identity.ContentID{mountedPointLawID(5), mountedPointLawID(6), mountedPointLawID(7), mountedPointLawID(8)}
	members := make(map[identity.ContentID]struct{}, len(points))
	occurrences := make(map[composition.Key]struct{}, len(points))
	for _, point := range points {
		member := mountedPointRuleMemberID(fixture.cap, mount, point, occurrence)
		if !member.Available() {
			t.Fatal("mounted-point member identity")
		}
		members[member] = struct{}{}
		key, keyOK := mountedPointRuleOccurrenceKey(fixture.cap, mount, point, occurrence)
		if !keyOK || !key.Available() {
			t.Fatal("mounted-point occurrence identity")
		}
		occurrences[key] = struct{}{}
	}
	if len(members) != len(points) || len(occurrences) != len(points) {
		t.Fatalf("mounted-point identity count members=%d occurrences=%d points=%d", len(members), len(occurrences), len(points))
	}
	sameMember := mountedPointRuleMemberID(fixture.cap, mount, points[0], occurrence)
	sameKey, sameKeyOK := mountedPointRuleOccurrenceKey(fixture.cap, mount, points[0], occurrence)
	if sameMember == (identity.ContentID{}) || !sameKeyOK {
		t.Fatal("same-point mounted-point lookup identity")
	}
	otherMember := mountedPointRuleMemberID(fixture.cap, mount, points[1], occurrence)
	otherKey, otherKeyOK := mountedPointRuleOccurrenceKey(fixture.cap, mount, points[1], occurrence)
	if sameMember != mountedPointRuleMemberID(fixture.cap, mount, points[0], occurrence) || sameMember == otherMember || sameKey == otherKey || !otherKeyOK {
		t.Fatal("mounted-point same-point lookup was not stable and injective")
	}
}
