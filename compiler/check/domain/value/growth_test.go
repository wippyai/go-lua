package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestHigherOrderGrowthRisk_DetectsFunctionReturningFunction(t *testing.T) {
	tp := typ.Func().
		Returns(typ.Func().Returns(typ.String).Build()).
		Build()
	if !HasHigherOrderGrowthRisk(tp) {
		t.Fatalf("expected higher-order growth risk to be detected")
	}
}

func TestContainsFunction_IgnoresInterfaceMethodSignatures(t *testing.T) {
	iface := typ.NewInterface("Reader", []typ.Method{
		{
			Name: "next",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Func().Returns(typ.String).Build()).
				Build(),
		},
	})
	if newGrowthScanState().containsFunction(iface, typ.NewGuard()) {
		t.Fatalf("expected interface method signatures to be ignored, got true")
	}
}

func TestMethodTypeHasSelfRecursiveReturn_IgnoresInterfaceMethods(t *testing.T) {
	owner := typ.NewRecord().Field("id", typ.String).Build()
	methodType := typ.NewInterface("HasBuild", []typ.Method{
		{
			Name: "build",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(owner).
				Build(),
		},
	})
	if newGrowthScanState().methodTypeHasSelfRecursiveReturn(methodType, owner, typ.NewGuard()) {
		t.Fatalf("expected interface method signatures to be ignored for self-recursive detection")
	}
}
