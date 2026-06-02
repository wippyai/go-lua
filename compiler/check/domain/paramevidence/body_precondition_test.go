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

type bodyPreconditionFacts struct {
	condition constraint.Condition
	proves    bool
	observed  map[constraint.PathKey]typ.Type
}

func (f bodyPreconditionFacts) DeclaredAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

func (f bodyPreconditionFacts) RefinedAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: nil, State: flow.StateUnknown}
}

func (f bodyPreconditionFacts) EffectiveTypeAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

func (f bodyPreconditionFacts) IsAnnotated(cfg.SymbolID) bool {
	return false
}

func (f bodyPreconditionFacts) ConditionAt(cfg.Point) constraint.Condition {
	if len(f.condition.Disjuncts) == 0 {
		return constraint.TrueCondition()
	}
	return f.condition
}

func (f bodyPreconditionFacts) ProvesTypeAt(cfg.Point, constraint.Path, typ.Type) bool {
	return f.proves
}

func (f bodyPreconditionFacts) ConditionTypeAt(cfg.Point, constraint.Path) typ.Type {
	return nil
}

func (f bodyPreconditionFacts) ConditionedTypeAt(cfg.Point, constraint.Path, constraint.Condition) typ.Type {
	return nil
}

func (f bodyPreconditionFacts) ConditionedSeedTypeAt(cfg.Point, constraint.Path, typ.Type, constraint.Path, constraint.Condition) typ.Type {
	return nil
}

func (f bodyPreconditionFacts) ObservePath(q flow.PathObservationQuery) flow.PathObservation {
	if f.observed == nil {
		return flow.PathObservation{}
	}
	t := f.observed[q.Path.Key()]
	if t == nil {
		return flow.PathObservation{}
	}
	return flow.PathObservation{
		Type:   t,
		State:  flow.StateResolved,
		Source: flow.PathObservationFactProjection,
	}
}

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

func TestBodyPreconditionContext_MapElementEvidenceUsesPathObservationFactsWithoutFlowSolution(t *testing.T) {
	mapSym := cfg.SymbolID(1)
	keySym := cfg.SymbolID(2)
	keyPath := constraint.NewPath(keySym, "kind")
	ctx := BodyPreconditionContext{
		result: &api.FuncResult{
			Facts: bodyPreconditionFacts{
				observed: map[constraint.PathKey]typ.Type{
					keyPath.Key(): typ.String,
				},
			},
		},
	}

	_, evidence, ok := ctx.sourceEvidenceForDerivedLocal(flow.AssignmentSource{
		Kind:      flow.AssignmentSourceMapElement,
		MapPath:   constraint.NewPath(mapSym, "items"),
		KeyVar:    "kind",
		KeySymbol: keySym,
	}, typ.Integer, 5)
	if !ok {
		t.Fatal("sourceEvidenceForDerivedLocal() rejected map element source")
	}
	want := typ.NewMap(typ.String, typ.Integer)
	if !typ.TypeEquals(evidence, want) {
		t.Fatalf("evidence = %v, want %v", evidence, want)
	}
}

func TestBodyPreconditionContext_LocalProofUsesConditionProofFactsWithoutFlowSolution(t *testing.T) {
	paramSym := cfg.SymbolID(1)
	arg := &ast.IdentExpr{Value: "value"}
	bindings := bind.NewBindingTable()
	bindings.Bind(arg, paramSym)
	ctx := BodyPreconditionContext{
		result: &api.FuncResult{
			Facts: bodyPreconditionFacts{proves: true},
		},
		bindings:        bindings,
		paramIndexBySym: map[cfg.SymbolID]int{paramSym: 0},
	}

	preconditions := ctx.PreconditionsFromCall(4, api.CallEvidence{
		Point:        4,
		Info:         &cfg.CallInfo{CalleeSymbol: 9, Args: []ast.Expr{arg}},
		ExpectedArgs: []typ.Type{typ.String},
	}, nil, nil)

	if len(preconditions.Body) != 1 || !typ.TypeEquals(preconditions.Body[0], typ.String) {
		t.Fatalf("body preconditions = %v, want string", preconditions.Body)
	}
	if len(preconditions.Public) != 0 {
		t.Fatalf("locally proven precondition leaked to public contract: %v", preconditions.Public)
	}
}

func TestBodyPreconditionContext_ConditionedEvidenceUsesProofAndPathObservationFactsWithoutFlowSolution(t *testing.T) {
	paramSym := cfg.SymbolID(1)
	constSym := cfg.SymbolID(2)
	msg := constraint.NewPath(paramSym, "msg")
	roleConst := constraint.NewPath(constSym, "prompt").Field("ROLE").Field("FUNCTION_CALL")
	cond := constraint.FromConstraints(constraint.FieldEqualsPath{
		Target: msg,
		Field:  "role",
		Value:  roleConst,
	})
	ctx := BodyPreconditionContext{
		result: &api.FuncResult{
			Facts: bodyPreconditionFacts{
				condition: cond,
				observed: map[constraint.PathKey]typ.Type{
					roleConst.Key(): typ.LiteralString("function_call"),
				},
			},
		},
		paramIndexBySym: map[cfg.SymbolID]int{paramSym: 0},
	}

	idx, evidence, conditional, ok := ctx.paramEvidenceFromPath(msg.Field("function_call").Field("id"), typ.String, 7, nil)
	if !ok || idx != 0 {
		t.Fatalf("paramEvidenceFromPath() = idx %d ok %v, want idx 0 ok true", idx, ok)
	}
	if !conditional {
		t.Fatal("conditioned evidence was not marked conditional")
	}
	want := typ.NewRecord().
		ReadonlyField("role", typ.LiteralString("function_call")).
		ReadonlyField("function_call", typ.NewRecord().ReadonlyField("id", typ.String).Build()).
		Build()
	if !typ.TypeEquals(evidence, want) {
		t.Fatalf("conditioned evidence = %v, want %v", evidence, want)
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
	}, nil, nil)

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
	}, nil, nil)

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

func TestMergeConditionedRecordEvidence_OrdersByStructuralFieldKey(t *testing.T) {
	evidence := typ.NewRecord().
		Field("b", typ.String).
		Field("a", typ.Number).
		Build()
	condition := typ.NewRecord().
		ReadonlyField("b", typ.String).
		Build()

	got, ok := mergeConditionedRecordEvidence(evidence, condition)
	if !ok {
		t.Fatal("mergeConditionedRecordEvidence rejected record evidence")
	}
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("merged evidence = %T, want record (%v)", got, got)
	}
	if len(rec.Fields) != 2 || rec.Fields[0].Name != "a" || rec.Fields[1].Name != "b" {
		t.Fatalf("merged fields not in structural order: %#v", rec.Fields)
	}
	field := rec.GetField("b")
	if field == nil || !field.Readonly {
		t.Fatalf("merged duplicate field did not retain readonly evidence: %#v", field)
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

func TestConditionedPathEvidenceFromCondition_TruthyGuardAdmitsFalsyLeaf(t *testing.T) {
	pageData := constraint.NewPath(1, "page").Field("data_func")
	evidence := PathEvidence(pageData.Segments, typ.String)
	cond := constraint.FromConstraints(constraint.Truthy{Path: pageData})

	got, conditional := conditionedPathEvidenceFromCondition(pageData, evidence, cond, nil)
	if !conditional {
		t.Fatal("truthy guard did not mark path evidence conditional")
	}
	want := typ.NewRecord().
		ReadonlyField("data_func", typ.NewUnion(typ.String, typ.Nil, typ.False)).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("truthy-conditioned evidence = %v, want %v", got, want)
	}
}

func TestConditionedPathEvidenceFromCondition_NotNilGuardAdmitsNilLeaf(t *testing.T) {
	pageData := constraint.NewPath(1, "page").Field("data_func")
	evidence := PathEvidence(pageData.Segments, typ.String)
	cond := constraint.FromConstraints(constraint.NotNil{Path: pageData})

	got, conditional := conditionedPathEvidenceFromCondition(pageData, evidence, cond, nil)
	if !conditional {
		t.Fatal("not-nil guard did not mark path evidence conditional")
	}
	want := typ.NewRecord().
		ReadonlyField("data_func", typ.NewOptional(typ.String)).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("not-nil-conditioned evidence = %v, want %v", got, want)
	}
}

func TestConditionedPathEvidenceFromCondition_TruthyGuardAdmitsNestedFalsyLeaf(t *testing.T) {
	nested := constraint.NewPath(1, "page").Field("meta").Field("data_func")
	evidence := PathEvidence(nested.Segments, typ.String)
	cond := constraint.FromConstraints(constraint.Truthy{Path: nested})

	got, conditional := conditionedPathEvidenceFromCondition(nested, evidence, cond, nil)
	if !conditional {
		t.Fatal("nested truthy guard did not mark path evidence conditional")
	}
	want := typ.NewRecord().
		ReadonlyField("meta", typ.NewRecord().
			ReadonlyField("data_func", typ.NewUnion(typ.String, typ.Nil, typ.False)).
			Build()).
		Build()
	if !typ.TypeEquals(got, want) {
		t.Fatalf("nested truthy-conditioned evidence = %v, want %v", got, want)
	}
}
