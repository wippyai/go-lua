package body

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestCallArgumentFunctionTypeProofPredicatesOwnSubtypeChecks(t *testing.T) {
	result := &Result{}
	fn := typ.Func().Returns(typ.String).Build()

	if !result.CallArgumentFunctionTypeAdmissible(fn, fn) {
		t.Fatal("matching contextual function type should be admissible")
	}
	if result.CallArgumentFunctionTypeAdmissible(fn, typ.String) {
		t.Fatal("unrelated expected type should not admit contextual function type")
	}
	if !result.CallArgumentFunctionTypeProvenMismatch(fn, typ.String) {
		t.Fatal("contextual function type should prove mismatch against unrelated expected type")
	}
	if result.CallArgumentFunctionTypeProvenMismatch(fn, fn) {
		t.Fatal("matching contextual function type should not prove mismatch")
	}
	if result.CallArgumentFunctionTypeProvenMismatch(fn, typ.Any) {
		t.Fatal("gradual expected type should not produce contextual function mismatch")
	}
}

func TestCallArgumentSolvedTypeProvenMismatchKeepsGradualBoundariesUnknown(t *testing.T) {
	result := &Result{}

	if !result.CallArgumentSolvedTypeProvenMismatch(typ.String, typ.Number, false) {
		t.Fatal("trusted concrete solved type should prove mismatch")
	}
	if result.CallArgumentSolvedTypeProvenMismatch(typ.String, typ.String, false) {
		t.Fatal("matching solved type should not prove mismatch")
	}
	if result.CallArgumentSolvedTypeProvenMismatch(typ.String, typ.Number, true) {
		t.Fatal("untrusted top-origin solved type should remain an unknown proof boundary")
	}
	if result.CallArgumentSolvedTypeProvenMismatch(typ.Any, typ.Number, false) {
		t.Fatal("gradual actual type should not prove mismatch")
	}
	if result.CallArgumentSolvedTypeProvenMismatch(typ.String, typ.Unknown, false) {
		t.Fatal("unknown expected type should not prove mismatch")
	}
}

func TestRootPathTrustedAssignmentSourceUsesLoweredRootAssignment(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function f(root: any): ()
	root = "ready"
	local use = root
end`)
	bindings := bind.BindFunction(fn, bind.Options{})
	result, err := CheckBoundFunction(fn, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}

	root := mustParamSlot(t, bindings, fn, 0).Symbol
	use := fn.Stmts[1].(*ast.LocalAssignStmt)
	usePoint := requireLocalAssignmentPoint(t, result, use, 0)
	if !result.RootPathHasTrustedDominatingAssignmentSource(usePoint, pathdom.NewPath(root, "root")) {
		t.Fatal("root reassignment should provide trusted dominating assignment source")
	}
}

func TestRootPathTrustedAssignmentSourceIgnoresMemberAssignment(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function f(root: any): ()
	root.field = "ready"
	local use = root
end`)
	bindings := bind.BindFunction(fn, bind.Options{})
	result, err := CheckBoundFunction(fn, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}

	root := mustParamSlot(t, bindings, fn, 0).Symbol
	use := fn.Stmts[1].(*ast.LocalAssignStmt)
	usePoint := requireLocalAssignmentPoint(t, result, use, 0)
	if result.RootPathHasTrustedDominatingAssignmentSource(usePoint, pathdom.NewPath(root, "root")) {
		t.Fatal("member assignment must not masquerade as a trusted root assignment source")
	}
}

func TestRootPathTrustedAssignmentSourceRejectsNilableSource(t *testing.T) {
	reg := standard.Registry()
	fn := parseFunction(t, `
function f(maybe: string?): ()
	local root = maybe
	local use = root
end`)
	bindings := bind.BindFunction(fn, bind.Options{})
	result, err := CheckBoundFunction(fn, bindings, Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}

	rootLocal := fn.Stmts[0].(*ast.LocalAssignStmt)
	root := mustLocalAt(t, result, rootLocal, 0)
	use := fn.Stmts[1].(*ast.LocalAssignStmt)
	usePoint := requireLocalAssignmentPoint(t, result, use, 0)
	if result.RootPathHasTrustedDominatingAssignmentSource(usePoint, pathdom.NewPath(root, "root")) {
		t.Fatal("nilable assignment source must not authorize trusted root boundary refinement")
	}
}
