package paramevidence

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBodyPreconditionContext_DerivedLocalPathReachesParameter(t *testing.T) {
	paramSym := cfg.SymbolID(1)
	localSym := cfg.SymbolID(2)
	ctx := BodyPreconditionContext{
		result: &api.FuncResult{
			FlowInputs: &flow.Inputs{
				Assignments: []flow.UnifiedAssignment{{
					Point:      1,
					TargetPath: constraint.NewPath(localSym, "local_user"),
					Source: flow.AssignmentSource{
						Kind: flow.AssignmentSourcePath,
						Path: constraint.NewPath(paramSym, "user"),
					},
				}},
			},
		},
		paramIndexBySym: map[cfg.SymbolID]int{paramSym: 0},
		dominates: func(a, b cfg.Point) bool {
			return a == b || (a == 1 && b == 2)
		},
	}

	idx, evidence, conditional, ok := ctx.paramEvidenceFromPath(
		constraint.NewPath(localSym, "local_user").Field("name"),
		typ.String,
		2,
		nil,
	)
	if !ok || idx != 0 {
		t.Fatalf("paramEvidenceFromPath() = idx %d ok %v, want idx 0 ok true", idx, ok)
	}
	want := typ.NewRecord().ReadonlyField("name", typ.String).Build()
	if !typ.TypeEquals(evidence, want) {
		t.Fatalf("evidence = %v, want %v", evidence, want)
	}
	if conditional {
		t.Fatal("unconditional derived local evidence reported conditional")
	}
}

func TestBodyPreconditionContext_FieldWriteDoesNotBackPropagateWholeRootEvidence(t *testing.T) {
	paramSym := cfg.SymbolID(1)
	localSym := cfg.SymbolID(2)
	localRoot := constraint.NewPath(localSym, "instance")
	paramRoot := constraint.NewPath(paramSym, "dataflow_id")
	ctx := BodyPreconditionContext{
		result: &api.FuncResult{
			FlowInputs: &flow.Inputs{
				Assignments: []flow.UnifiedAssignment{
					{
						Point:      1,
						TargetPath: localRoot,
					},
					{
						Point:      1,
						TargetPath: localRoot.Field("dataflow_id"),
						Source: flow.AssignmentSource{
							Kind: flow.AssignmentSourcePath,
							Path: paramRoot,
						},
					},
				},
			},
		},
		paramIndexBySym: map[cfg.SymbolID]int{paramSym: 0},
		dominates: func(a, b cfg.Point) bool {
			return a == b || (a == 1 && b == 2)
		},
	}
	expectedRoot := typ.NewRecord().ReadonlyField("dataflow_id", typ.String).Build()

	if _, evidence, _, ok := ctx.paramEvidenceFromPath(localRoot, expectedRoot, 2, nil); ok {
		t.Fatalf("whole-root evidence leaked through descendant field write: %v", evidence)
	}
}

func TestBodyPreconditionContext_FieldWriteBackPropagatesOnlyLeafEvidence(t *testing.T) {
	paramSym := cfg.SymbolID(1)
	localSym := cfg.SymbolID(2)
	localRoot := constraint.NewPath(localSym, "instance")
	paramRoot := constraint.NewPath(paramSym, "dataflow_id")
	ctx := BodyPreconditionContext{
		result: &api.FuncResult{
			FlowInputs: &flow.Inputs{
				Assignments: []flow.UnifiedAssignment{
					{
						Point:      1,
						TargetPath: localRoot,
					},
					{
						Point:      1,
						TargetPath: localRoot.Field("dataflow_id"),
						Source: flow.AssignmentSource{
							Kind: flow.AssignmentSourcePath,
							Path: paramRoot,
						},
					},
				},
			},
		},
		paramIndexBySym: map[cfg.SymbolID]int{paramSym: 0},
		dominates: func(a, b cfg.Point) bool {
			return a == b || (a == 1 && b == 2)
		},
	}

	idx, evidence, _, ok := ctx.paramEvidenceFromPath(localRoot.Field("dataflow_id"), typ.String, 2, nil)
	if !ok || idx != 0 {
		t.Fatalf("field evidence = idx %d ok %v, want idx 0 ok true", idx, ok)
	}
	if !typ.TypeEquals(evidence, typ.String) {
		t.Fatalf("field write propagated %v, want leaf string evidence", evidence)
	}
}

func TestBodyPreconditionContext_AmbiguousDominatingAssignmentsRejected(t *testing.T) {
	paramSym := cfg.SymbolID(1)
	localSym := cfg.SymbolID(2)
	otherSym := cfg.SymbolID(3)
	ctx := BodyPreconditionContext{
		result: &api.FuncResult{
			FlowInputs: &flow.Inputs{
				Assignments: []flow.UnifiedAssignment{
					{
						Point:      2,
						TargetPath: constraint.NewPath(localSym, "x"),
						Source: flow.AssignmentSource{
							Kind: flow.AssignmentSourcePath,
							Path: constraint.NewPath(paramSym, "a"),
						},
					},
					{
						Point:      3,
						TargetPath: constraint.NewPath(localSym, "x"),
						Source: flow.AssignmentSource{
							Kind: flow.AssignmentSourcePath,
							Path: constraint.NewPath(otherSym, "b"),
						},
					},
				},
			},
		},
		paramIndexBySym: map[cfg.SymbolID]int{paramSym: 0, otherSym: 1},
		dominates: func(a, b cfg.Point) bool {
			if a == b {
				return true
			}
			return b == 4 && (a == 2 || a == 3)
		},
	}

	if _, _, _, ok := ctx.paramEvidenceFromPath(constraint.NewPath(localSym, "x"), typ.String, 4, nil); ok {
		t.Fatal("paramEvidenceFromPath() accepted ambiguous dominating assignments")
	}
}

func TestBodyPreconditionContext_MapElementEvidenceUsesAnyKeyWithoutSolvedKeyType(t *testing.T) {
	mapSym := cfg.SymbolID(1)
	ctx := BodyPreconditionContext{result: &api.FuncResult{}}

	sourcePath, evidence, ok := ctx.sourceEvidenceForDerivedLocal(flow.AssignmentSource{
		Kind:    flow.AssignmentSourceMapElement,
		MapPath: constraint.NewPath(mapSym, "items"),
	}, typ.String, 5)
	if !ok {
		t.Fatal("sourceEvidenceForDerivedLocal() rejected map element source")
	}
	if sourcePath.Symbol != mapSym {
		t.Fatalf("source path symbol = %d, want %d", sourcePath.Symbol, mapSym)
	}
	want := typ.NewMap(typ.Any, typ.String)
	if !typ.TypeEquals(evidence, want) {
		t.Fatalf("evidence = %v, want %v", evidence, want)
	}
}

func TestBodyPreconditionContext_RecursiveSelfCallEvidenceStaysBodyLocal(t *testing.T) {
	paramSym := cfg.SymbolID(1)
	currentSym := cfg.SymbolID(2)
	arg := &ast.IdentExpr{Value: "original"}
	bindings := bind.NewBindingTable()
	bindings.Bind(arg, paramSym)
	ctx := BodyPreconditionContext{
		result:          &api.FuncResult{},
		bindings:        bindings,
		currentSym:      currentSym,
		paramIndexBySym: map[cfg.SymbolID]int{paramSym: 0},
	}
	expected := typ.NewMap(typ.Any, typ.Any)

	preconditions := ctx.PreconditionsFromCall(4, api.CallEvidence{
		Point:        4,
		Info:         &cfg.CallInfo{CalleeSymbol: currentSym, Args: []ast.Expr{arg}},
		ExpectedArgs: []typ.Type{expected},
	}, nil)

	if len(preconditions.Body) != 1 || !typ.TypeEquals(preconditions.Body[0], expected) {
		t.Fatalf("body preconditions = %v, want %v", preconditions.Body, expected)
	}
	if len(preconditions.Public) != 0 {
		t.Fatalf("recursive self-call feedback leaked to public preconditions: %v", preconditions.Public)
	}
}

func TestBodyPreconditionContext_NonRecursiveHardUseCanPublishPublic(t *testing.T) {
	paramSym := cfg.SymbolID(1)
	currentSym := cfg.SymbolID(2)
	calleeSym := cfg.SymbolID(3)
	arg := &ast.IdentExpr{Value: "original"}
	bindings := bind.NewBindingTable()
	bindings.Bind(arg, paramSym)
	ctx := BodyPreconditionContext{
		result:          &api.FuncResult{},
		bindings:        bindings,
		currentSym:      currentSym,
		paramIndexBySym: map[cfg.SymbolID]int{paramSym: 0},
	}
	expected := typ.NewMap(typ.Any, typ.Any)

	preconditions := ctx.PreconditionsFromCall(4, api.CallEvidence{
		Point:        4,
		Info:         &cfg.CallInfo{CalleeSymbol: calleeSym, Args: []ast.Expr{arg}},
		ExpectedArgs: []typ.Type{expected},
	}, nil)

	if len(preconditions.Public) != 1 || !typ.TypeEquals(preconditions.Public[0], expected) {
		t.Fatalf("public preconditions = %v, want %v", preconditions.Public, expected)
	}
}

func TestConditionedPathEvidenceFromCondition_AddsDiscriminant(t *testing.T) {
	msg := constraint.NewPath(1, "msg")
	evidence := typ.NewRecord().
		ReadonlyField("function_call", typ.NewRecord().ReadonlyField("id", typ.String).Build()).
		Build()
	cond := constraint.FromConstraints(constraint.FieldEquals{
		Target: msg,
		Field:  "role",
		Value:  typ.LiteralString("function_call"),
	})

	got, conditional := conditionedPathEvidenceFromCondition(msg.Field("function_call").Field("id"), evidence, cond, nil)
	if !conditional {
		t.Fatal("conditionedPathEvidenceFromCondition() did not mark discriminant evidence conditional")
	}
	want := typ.NewRecord().
		ReadonlyField("role", typ.LiteralString("function_call")).
		ReadonlyField("function_call", typ.NewRecord().ReadonlyField("id", typ.String).Build()).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("conditioned evidence = %v, want %v", got, want)
	}
}

func TestConditionedPathEvidenceFromCondition_ResolvesDiscriminantPath(t *testing.T) {
	msg := constraint.NewPath(1, "msg")
	roleConst := constraint.NewPath(2, "prompt").Field("ROLE").Field("FUNCTION_CALL")
	evidence := typ.NewRecord().
		ReadonlyField("function_call", typ.NewRecord().ReadonlyField("id", typ.String).Build()).
		Build()
	cond := constraint.FromConstraints(constraint.FieldEqualsPath{
		Target: msg,
		Field:  "role",
		Value:  roleConst,
	})

	got, conditional := conditionedPathEvidenceFromCondition(msg.Field("function_call").Field("id"), evidence, cond, func(path constraint.Path) *typ.Literal {
		if path.Equal(roleConst) {
			return typ.LiteralString("function_call")
		}
		return nil
	})
	if !conditional {
		t.Fatal("conditionedPathEvidenceFromCondition() did not mark path discriminant evidence conditional")
	}
	want := typ.NewRecord().
		ReadonlyField("role", typ.LiteralString("function_call")).
		ReadonlyField("function_call", typ.NewRecord().ReadonlyField("id", typ.String).Build()).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("conditioned path evidence = %v, want %v", got, want)
	}
}

func TestConditionedPathEvidenceFromCondition_UnresolvedDiscriminantPathStillGuarded(t *testing.T) {
	msg := constraint.NewPath(1, "msg")
	roleConst := constraint.NewPath(2, "prompt").Field("ROLE").Field("FUNCTION_CALL")
	evidence := typ.NewRecord().
		ReadonlyField("function_call", typ.NewRecord().ReadonlyField("id", typ.String).Build()).
		Build()
	cond := constraint.FromConstraints(constraint.FieldEqualsPath{
		Target: msg,
		Field:  "role",
		Value:  roleConst,
	})

	got, conditional := conditionedPathEvidenceFromCondition(msg.Field("function_call").Field("id"), evidence, cond, nil)
	if !conditional {
		t.Fatal("unresolved path discriminant did not mark evidence branch-conditional")
	}
	if !typ.TypeEquals(got, evidence) {
		t.Fatalf("unresolved discriminant evidence = %v, want original %v", got, evidence)
	}
}
