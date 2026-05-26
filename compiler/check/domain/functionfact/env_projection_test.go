package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/typ"
)

func TestProjectEnvironmentReturns_SkipsGuardProvenFalse(t *testing.T) {
	badGuard := constraint.FromConstraints(constraint.Truthy{Path: constraint.ParamPath(0).Field("bad")})
	remoteGuard := constraint.FromConstraints(constraint.Truthy{Path: constraint.ParamPath(0).Field("remote_bad")})
	errorUnknown := typ.NewRecord().
		Field("success", typ.False).
		Field("error_message", typ.Unknown).
		Build()
	errorString := typ.NewRecord().
		Field("success", typ.False).
		Field("error_message", typ.String).
		Build()
	success := typ.NewRecord().
		Field("success", typ.True).
		Build()
	current := typ.NewUnion(success, errorUnknown)
	callee := typ.Func().
		Param("args", typ.Any).
		Returns(current).
		Spec(contract.NewSpec().WithEnvReturns(
			contract.EnvReturnSpec{When: badGuard, ReturnIndex: 0, ResultIndex: 0},
			contract.EnvReturnSpec{When: remoteGuard, ReturnIndex: 0, ResultIndex: 0},
		)).
		Build()

	got := ProjectEnvironmentReturns(callee, []typ.Type{current}, []typ.Type{
		typ.NewRecord().Field("bad", typ.True).Build(),
	}, func(spec contract.EnvReturnSpec) []typ.Type {
		if spec.When.Equals(badGuard) {
			return []typ.Type{errorString}
		}
		return []typ.Type{errorUnknown}
	})

	if len(got) != 1 {
		t.Fatalf("projection returned %d slots, want 1", len(got))
	}
	if !unionHasRecordField(got[0], "error_message", typ.String) {
		t.Fatalf("expected reachable error branch to refine error_message to string, got %v", got[0])
	}
	if unionHasRecordField(got[0], "error_message", typ.Unknown) {
		t.Fatalf("unreachable remote_bad branch must not keep unknown error_message, got %v", got[0])
	}
}

func unionHasRecordField(t typ.Type, field string, want typ.Type) bool {
	if rec, ok := t.(*typ.Record); ok {
		f := rec.GetField(field)
		return f != nil && typ.TypeEquals(f.Type, want)
	}
	if union, ok := t.(*typ.Union); ok {
		for _, member := range union.Members {
			if unionHasRecordField(member, field, want) {
				return true
			}
		}
	}
	return false
}
