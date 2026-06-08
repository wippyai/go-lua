package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
)

func TestApplyConditionFactConjoinsAndCollapsesContradictions(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(1), "x")
	first := constraint.FromConstraints(constraint.Truthy{Path: path})
	second := constraint.FromConstraints(constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")})
	out := PointState{}

	if !ApplyConditionFact(&out, first) || !constraint.Domain.Equal(out.Cond, first) {
		t.Fatalf("first condition not installed: %v", out.Cond)
	}
	if !ApplyConditionFact(&out, second) {
		t.Fatal("second condition reported no change")
	}
	if !out.Cond.HasConstraints() {
		t.Fatalf("condition not conjoined: %v", out.Cond)
	}
	if !ApplyConditionFact(&out, constraint.FalseCondition()) {
		t.Fatal("false condition reported no change")
	}
	if !PointStateDomain.Equal(out, PointStateDomain.Bottom()) {
		t.Fatalf("false condition did not collapse point state: %#v", out)
	}
}

func TestForgetConditionAffectedByWriteRemovesDescendantConstraints(t *testing.T) {
	root := cfg.SymbolID(2)
	parent := constraint.NewPath(root, "tbl").Field("cfg")
	child := constraint.NewPath(root, "tbl").Field("cfg").Field("value")
	other := constraint.NewPath(root, "tbl").Field("other")
	out := PointState{Cond: constraint.FromConstraints(
		constraint.Truthy{Path: child},
		constraint.Truthy{Path: other},
	)}

	if !ForgetConditionAffectedByWrite(&out, parent) {
		t.Fatal("ForgetConditionAffectedByWrite reported no change")
	}
	if out.Cond.IsFalse() || out.Cond.IsTrue() {
		t.Fatalf("write invalidation dropped all constraints: %v", out.Cond)
	}
	for _, c := range out.Cond.MustConstraints() {
		if constraint.ConstraintAffectedByWrite(c, parent) {
			t.Fatalf("write invalidation kept affected constraint %#v in %v", c, out.Cond)
		}
	}
	if constraint.PathAffectedByWrite(other, parent) {
		t.Fatal("test setup invalid: unrelated path considered affected")
	}
}
