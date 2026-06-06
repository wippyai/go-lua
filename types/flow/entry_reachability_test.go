package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/numeric"
)

func TestLiftEntryReachabilityLiftsBottomAxes(t *testing.T) {
	ps := PointState{
		Num:             numeric.Bottom(),
		CellEffects:     CaptureEffectsDomain.Bottom(),
		ReceiverEffects: ReceiverEffectsDomain.Bottom(),
	}

	if !LiftEntryReachability(&ps) {
		t.Fatal("entry reachability did not report a change")
	}
	if ps.Num == nil || ps.Num.IsUnsat() {
		t.Fatalf("entry numeric state = %v, want reachable empty state", ps.Num)
	}
	if !constraint.Domain.Equal(ps.Cond, constraint.Domain.Top()) {
		t.Fatalf("entry condition = %v, want reachable true condition", ps.Cond)
	}
	if !PointRelationsDomain.Equal(ps.Rel, PointRelationsDomain.Top()) {
		t.Fatalf("entry point relations = %#v, want reachable empty relation set", ps.Rel)
	}
	if !ReturnRelationsDomain.Equal(ps.ReturnRel, ReturnRelationsDomain.Top()) {
		t.Fatalf("entry return relations = %#v, want reachable empty relation set", ps.ReturnRel)
	}
	if !CaptureEffectsDomain.Equal(ps.CellEffects, CaptureEffectsIdentity()) {
		t.Fatalf("entry cell effects = %v, want identity", ps.CellEffects)
	}
	if !ReceiverEffectsDomain.Equal(ps.ReceiverEffects, ReceiverEffectsIdentity()) {
		t.Fatalf("entry receiver effects = %v, want identity", ps.ReceiverEffects)
	}
	if !StaticMemberFactsDomain.Equal(ps.StaticMembers, StaticMemberFactsDomain.Top()) {
		t.Fatalf("entry static member facts = %s, want reachable empty fact set", ps.StaticMembers.Format())
	}
	if !KeyPresenceFactsDomain.Equal(ps.KeyPresence, KeyPresenceFactsDomain.Top()) {
		t.Fatalf("entry key-presence facts = %s, want reachable empty fact set", ps.KeyPresence.Format())
	}
	if !ValueOriginFactsDomain.Equal(ps.ValueOrigins, ValueOriginFactsDomain.Top()) {
		t.Fatalf("entry value-origin facts = %s, want reachable empty fact set", ps.ValueOrigins.Format())
	}
	if !PathAliasFactsDomain.Equal(ps.PathAliases, PathAliasFactsDomain.Top()) {
		t.Fatalf("entry path aliases = %s, want reachable empty fact set", ps.PathAliases.Format())
	}
	if !IndexWriteAdmissionFactsDomain.Equal(ps.IndexWrites, IndexWriteAdmissionFactsDomain.Top()) {
		t.Fatalf("entry index writes = %s, want reachable empty fact set", ps.IndexWrites.Format())
	}
	if LiftEntryReachability(&ps) {
		t.Fatal("entry reachability should be idempotent")
	}
}
