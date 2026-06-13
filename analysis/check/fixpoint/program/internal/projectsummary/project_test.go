package projectsummary_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFromResultProjectsReturnSlotsFromExitState(t *testing.T) {
	reg, axisKey := projectTestRegistry(t)
	first := projectValue(reg, axisKey, projectMarkA)
	stmts := projectParseChunk(t, "return 1, nil")

	result := projectCheckChunk(t, stmts, body.Config{
		Registry: reg,
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
			switch source.TargetIndex {
			case 0:
				return first, true
			case 1:
				return product.Absent(reg), true
			default:
				return product.Value{}, false
			}
		},
	})
	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 2 {
		t.Fatalf("FromResult returned %d slots, want 2", len(got.Returns))
	}
	projectAssertValue(t, reg, got.Returns[0], first)
	projectAssertValue(t, reg, got.Returns[1], product.Absent(reg))
}

func TestFromResultNoExplicitReturnProjectsEmptySummary(t *testing.T) {
	reg, _ := projectTestRegistry(t)
	result := projectCheckChunk(t, projectParseChunk(t, "local x = 1"), body.Config{Registry: reg})

	if got := summaryprojection.FromResult(result); len(got.Returns) != 0 {
		t.Fatalf("FromResult returned %#v, want empty summary", got)
	}
}

func TestFromResultUnresolvedReturnExpressionNormalizesBottomSlot(t *testing.T) {
	reg, _ := projectTestRegistry(t)
	result := projectCheckChunk(t, projectParseChunk(t, "return unknown"), body.Config{Registry: reg})

	if got := summaryprojection.FromResult(result); len(got.Returns) != 0 {
		t.Fatalf("FromResult returned %#v, want empty summary", got)
	}
}

func TestFromResultReadsJoinedExitReturnSlot(t *testing.T) {
	reg, axisKey := projectTestRegistry(t)
	branchA := projectValue(reg, axisKey, projectMarkA)
	branchB := projectValue(reg, axisKey, projectMarkB)
	joined := projectValue(reg, axisKey, projectMarkTop)
	values := []product.Value{branchA, branchB}
	byExpr := make(map[factflow.ExprRef]product.Value)
	stmts := projectParseChunk(t, "if c then return 1 else return 2 end")

	result := projectCheckChunk(t, stmts, body.Config{
		Registry: reg,
		Globals:  []string{"c"},
		ExpressionValue: func(_ cfg.Point, expr factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
			if source.TargetIndex != 0 {
				return product.Value{}, false
			}
			if value, ok := byExpr[expr]; ok {
				return value, true
			}
			if len(byExpr) >= len(values) {
				return product.Value{}, false
			}
			value := values[len(byExpr)]
			byExpr[expr] = value
			return value, true
		},
	})
	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 1 {
		t.Fatalf("FromResult returned %d slots, want 1", len(got.Returns))
	}
	projectAssertValue(t, reg, got.Returns[0], joined)
	if product.Equal(reg, got.Returns[0], branchA) || product.Equal(reg, got.Returns[0], branchB) {
		t.Fatalf("FromResult returned an individual branch value, want joined exit value")
	}
}

func TestFromResultJoinsDeclaredReturnTypeEvidence(t *testing.T) {
	reg := standard.Registry()
	numberValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	fn := projectParseFunction(t, `
function f(): number | string
	return 1
end`)

	result := projectCheckFunction(t, fn, body.Config{
		Registry: reg,
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
			if source.TargetIndex != 0 {
				return product.Value{}, false
			}
			return numberValue, true
		},
	})
	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 1 {
		t.Fatalf("FromResult returned %d slots, want 1", len(got.Returns))
	}
	want := runtimekind.Join(runtimekind.Singleton(runtimekind.Number), runtimekind.Singleton(runtimekind.String))
	if kind := product.Get(reg, got.Returns[0], runtimekind.Key); !runtimekind.Equal(kind, want) {
		t.Fatalf("return runtime kind = %s, want %s", kind, want)
	}
}

func TestFromResultPreservesDeclaredReturnVariantOrigin(t *testing.T) {
	reg := standard.Registry()
	tableValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	fn := projectParseFunction(t, `
function f(): {kind: "a", value: number} | {kind: "b", value: string}
	return {kind = "a", value = 1}
end`)

	result := projectCheckFunction(t, fn, body.Config{
		Registry: reg,
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
			if source.TargetIndex != 0 {
				return product.Value{}, false
			}
			return tableValue, true
		},
	})
	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 1 {
		t.Fatalf("FromResult returned %d slots, want 1", len(got.Returns))
	}
	origin := product.Get(reg, got.Returns[0], variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		t.Fatalf("return variant origin = %v, want declared record-union origin", origin)
	}
}

func TestFromResultIgnoresDeadReturnFacts(t *testing.T) {
	reg, axisKey := projectTestRegistry(t)
	live := projectValue(reg, axisKey, projectMarkA)
	dead := projectValue(reg, axisKey, projectMarkB)
	values := []product.Value{live, dead}
	var seen int
	stmts := projectParseChunk(t, "do return 1 end\nreturn 2")

	result := projectCheckChunk(t, stmts, body.Config{
		Registry: reg,
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
			if source.TargetIndex != 0 || seen >= len(values) {
				return product.Value{}, false
			}
			value := values[seen]
			seen++
			return value, true
		},
	})
	if got := result.ReturnPoints(); len(got) != 1 {
		t.Fatalf("ReturnPoints returned %d points, want only the reachable return", len(got))
	}
	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 1 {
		t.Fatalf("FromResult returned %d slots, want 1", len(got.Returns))
	}
	projectAssertValue(t, reg, got.Returns[0], live)
	if product.Equal(reg, got.Returns[0], dead) {
		t.Fatalf("FromResult used dead return value")
	}
}

func TestFromResultProjectsStrictNormalReturnParamConstraint(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(x: string?)
	assert(x)
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	if len(got.NormalReturnParams) != 1 {
		t.Fatalf("normal return params = %d, want 1: %#v", len(got.NormalReturnParams), got)
	}
	if gotPresence := product.PresenceOf(got.NormalReturnParams[0]); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("normal return param presence = %s, want present", gotPresence)
	}
	if gotKind := product.Get(reg, got.NormalReturnParams[0], runtimekind.Key); !runtimekind.Equal(gotKind, runtimekind.Singleton(runtimekind.String)) {
		t.Fatalf("normal return param runtime kind = %s, want string", gotKind)
	}
}

func TestFromResultProjectsParamConstraintAfterErrorGuard(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(x: string?)
	if x == nil then
		error("missing")
	end
end`), body.Config{
		Registry: reg,
		Signatures: signaturelookup.Source{
			IncludeStdlib: true,
		},
	})

	got := summaryprojection.FromResult(result)

	if len(got.NormalReturnParams) != 1 {
		t.Fatalf("normal return params = %d, want 1: %#v", len(got.NormalReturnParams), got)
	}
	if gotPresence := product.PresenceOf(got.NormalReturnParams[0]); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("normal return param presence = %s, want present", gotPresence)
	}
	if gotKind := product.Get(reg, got.NormalReturnParams[0], runtimekind.Key); !runtimekind.Equal(gotKind, runtimekind.Singleton(runtimekind.String)) {
		t.Fatalf("normal return param runtime kind = %s, want string", gotKind)
	}
}

func TestFromResultDoesNotProjectUnchangedParamEntryAssumption(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(x: string?)
	local y = x
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)
	if len(got.NormalReturnParams) != 1 {
		t.Fatalf("normal return params = %#v, want one explicit top", got.NormalReturnParams)
	}
	if !product.Equal(reg, got.NormalReturnParams[0], product.Top()) {
		t.Fatalf("normal return param = %#v, want top/no constraint", got.NormalReturnParams[0])
	}
}

func TestFromResultDoesNotProjectConditionalParamAssert(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(x: string?, c: boolean)
	if c then
		assert(x)
	end
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)
	if len(got.NormalReturnParams) != 2 {
		t.Fatalf("normal return params = %#v, want explicit top slots", got.NormalReturnParams)
	}
	for i, value := range got.NormalReturnParams {
		if !product.Equal(reg, value, product.Top()) {
			t.Fatalf("normal return param %d = %#v, want top/no constraint", i, value)
		}
	}
}

func TestFromResultDoesNotProjectReassignedParamAssert(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(x: string?)
	x = "fallback"
	assert(x)
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)
	if len(got.NormalReturnParams) != 1 {
		t.Fatalf("normal return params = %#v, want one explicit top", got.NormalReturnParams)
	}
	if !product.Equal(reg, got.NormalReturnParams[0], product.Top()) {
		t.Fatalf("normal return param = %#v, want top/no constraint", got.NormalReturnParams[0])
	}
}

func TestFromResultProjectsParamObligationFromTypedMemberArgument(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(client: {invoke: (model_id: string, payload: any, options: any) -> ()}, model_id)
	return client.invoke(model_id, {}, {})
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	projectAssertParamObligationKind(t, reg, got, 1, runtimekind.Singleton(runtimekind.String))
	if len(got.NormalReturnParams) > 1 && !product.Equal(reg, got.NormalReturnParams[1], product.Top()) {
		t.Fatalf("normal return param 1 = %#v, want no post-return refinement", got.NormalReturnParams[1])
	}
}

func TestFromResultProjectsParamObligationFromArithmeticOperand(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(tokens)
	return tokens * 2
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	projectAssertParamObligationKind(t, reg, got, 0, runtimekind.Singleton(runtimekind.Number))
}

func TestFromResultDoesNotProjectGuardedParamObligation(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(x)
	if type(x) == "number" then
		return x * 2
	end
end`), body.Config{
		Registry: reg,
		Globals:  []string{"type"},
	})

	got := summaryprojection.FromResult(result)

	if len(got.ParamObligations) != 0 {
		t.Fatalf("param obligations = %#v, want none for guarded use", got.ParamObligations)
	}
}

func TestFromResultDoesNotProjectReassignedParamObligation(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(x)
	x = 1
	return x * 2
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	if len(got.ParamObligations) != 0 {
		t.Fatalf("param obligations = %#v, want none for reassigned param", got.ParamObligations)
	}
}

func TestFromResultProjectsReturnTruthyParamRefinement(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(value: any)
	return type(value) == "number" and value > 0
end`), body.Config{
		Registry: reg,
		Globals:  []string{"type"},
	})

	got := summaryprojection.FromResult(result)

	if len(got.ReturnConditionParamRefinements) == 0 {
		t.Fatalf("return condition param refinements missing: %#v", got)
	}
	var refinement summary.ReturnConditionParamRefinement
	found := false
	for _, candidate := range got.ReturnConditionParamRefinements {
		if candidate.ReturnIndex == 0 && candidate.ReturnValue && candidate.Target.PlaceholderIndex() == 0 {
			refinement = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("truthy return condition refinement missing: %#v", got.ReturnConditionParamRefinements)
	}
	if refinement.ReturnIndex != 0 || !refinement.ReturnValue || refinement.Target.PlaceholderIndex() != 0 {
		t.Fatalf("return condition refinement = %#v, want truthy ret[0] -> $0", refinement)
	}
	if kind := product.Get(reg, refinement.Value, runtimekind.Key); !runtimekind.Equal(kind, runtimekind.Singleton(runtimekind.Number)) {
		t.Fatalf("return condition runtime kind = %s, want number", kind)
	}
}

func TestFromResultInfersErrorReturnPresenceRelations(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function process(x: number): (number?, string?)
	if x < 0 then
		return nil, "negative"
	end
	return x * 2, nil
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	projectAssertReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Present(), 0, presence.Absent())
	projectAssertReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Absent(), 0, presence.Present())
	if len(got.Returns) != 2 {
		t.Fatalf("returns = %d, want 2", len(got.Returns))
	}
	if gotPresence := product.PresenceOf(got.Returns[0]); !presence.Equal(gotPresence, presence.Maybe()) {
		t.Fatalf("return 0 presence = %s, want maybe", gotPresence)
	}
	if gotPresence := product.PresenceOf(got.Returns[1]); !presence.Equal(gotPresence, presence.Maybe()) {
		t.Fatalf("return 1 presence = %s, want maybe", gotPresence)
	}
}

func TestFromResultTreatsOmittedEstablishedReturnSlotsAsAbsent(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function fetch(ok: boolean): (number?, string?)
	if not ok then
		return nil, "failed"
	end
	return 1
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	projectAssertReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Present(), 0, presence.Absent())
	projectAssertReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Absent(), 0, presence.Present())
	if len(got.Returns) != 2 {
		t.Fatalf("returns = %d, want 2", len(got.Returns))
	}
	if gotPresence := product.PresenceOf(got.Returns[1]); !presence.Equal(gotPresence, presence.Maybe()) {
		t.Fatalf("return 1 presence = %s, want maybe from explicit error or omitted nil", gotPresence)
	}
}

func TestFromResultUsesDeclaredArityForOpenTailReturn(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function fetch(ok: boolean): (number?, string?)
	return open_db(ok)
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 2 {
		t.Fatalf("returns = %d, want declared arity 2 for open-tail return", len(got.Returns))
	}
	if gotPresence := product.PresenceOf(got.Returns[1]); !presence.Equal(gotPresence, presence.Maybe()) {
		t.Fatalf("return 1 presence = %s, want maybe from declared return slot", gotPresence)
	}
}

func TestFromResultPreservesDeclaredReturnTypeWitness(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function fetch(ok: boolean): (number?, string?)
	if ok then
		return 1, nil
	end
	return nil, "failed"
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 2 {
		t.Fatalf("returns = %d, want 2", len(got.Returns))
	}
	witness := product.Get(reg, got.Returns[0], typewitness.Key)
	gotType, ok := witness.Type()
	if !ok || !typ.TypeEquals(gotType, typ.NewOptional(typ.Number)) {
		t.Fatalf("return 0 witness = %v/%v, want number?", gotType, ok)
	}
}

func TestFromResultMissingReadModelReturnsEmptySummary(t *testing.T) {
	if got := summaryprojection.FromResult(nil); len(got.Returns) != 0 {
		t.Fatalf("FromResult(nil) returned %#v, want empty summary", got)
	}
}

type projectMark uint8

const (
	projectMarkBottom projectMark = iota
	projectMarkA
	projectMarkB
	projectMarkTop
)

func projectTestRegistry(t *testing.T) (*axis.Registry, axis.Key[projectMark]) {
	t.Helper()
	axisKey := axis.NewKey[projectMark]("summary.project.test." + strings.ReplaceAll(t.Name(), "/", "."))
	reg, err := standard.RegistryWithAxes(axis.Spec[projectMark]{
		Key:    axisKey,
		Bottom: func() projectMark { return projectMarkBottom },
		Top:    func() projectMark { return projectMarkTop },
		Equal:  func(a, b projectMark) bool { return a == b },
		LessOrEq: func(a, b projectMark) bool {
			return a == b || a == projectMarkBottom || b == projectMarkTop
		},
		Join: func(a, b projectMark) projectMark {
			if a == b {
				return a
			}
			if a == projectMarkBottom {
				return b
			}
			if b == projectMarkBottom {
				return a
			}
			return projectMarkTop
		},
		Meet: func(a, b projectMark) projectMark {
			if a == b {
				return a
			}
			if a == projectMarkTop {
				return b
			}
			if b == projectMarkTop {
				return a
			}
			return projectMarkBottom
		},
		Widen: func(prev, next projectMark) projectMark {
			if prev == next {
				return prev
			}
			if prev == projectMarkBottom {
				return next
			}
			return projectMarkTop
		},
		Hash: func(v projectMark) uint64 { return uint64(v) },
	}.Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes: %v", err)
	}
	return reg, axisKey
}

func projectValue(reg *axis.Registry, axisKey axis.Key[projectMark], mark projectMark) product.Value {
	return product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), axisKey, mark)
}

func projectCheckChunk(t *testing.T, stmts []ast.Stmt, config body.Config) *body.Result {
	t.Helper()
	result, err := body.CheckChunk(stmts, config)
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	return result
}

func projectCheckFunction(t *testing.T, fn *ast.FunctionExpr, config body.Config) *body.Result {
	t.Helper()
	result, err := body.CheckFunction(fn, config)
	if err != nil {
		t.Fatalf("CheckFunction: %v", err)
	}
	return result
}

func projectParseFunction(t *testing.T, src string) *ast.FunctionExpr {
	t.Helper()
	stmts := projectParseChunk(t, src)
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want function definition", stmts[0])
	}
	return def.Func
}

func projectParseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "summary_project_test.lua")
	if err != nil {
		t.Fatalf("ParseString(%q): %v", src, err)
	}
	return stmts
}

func projectAssertValue(t *testing.T, reg *axis.Registry, got, want product.Value) {
	t.Helper()
	if !product.Equal(reg, got, want) {
		t.Fatalf("value = %v, want %v", got, want)
	}
}

func projectAssertParamObligationKind(
	t *testing.T,
	reg *axis.Registry,
	got summary.Summary,
	param int,
	want runtimekind.Value,
) {
	t.Helper()
	if len(got.ParamObligations) <= param {
		t.Fatalf("param obligations = %#v, want obligation at %d", got.ParamObligations, param)
	}
	if kind := product.Get(reg, got.ParamObligations[param], runtimekind.Key); !runtimekind.Equal(kind, want) {
		t.Fatalf("param obligation %d runtime kind = %s, want %s", param, kind, want)
	}
}

func projectAssertReturnPresenceRelation(
	t *testing.T,
	relations []summary.ReturnPresenceRelation,
	trigger int,
	triggerPresence presence.Value,
	target int,
	targetPresence presence.Value,
) {
	t.Helper()
	for _, relation := range relations {
		if relation.TriggerIndex == trigger &&
			presence.Equal(relation.TriggerPresence, triggerPresence) &&
			relation.TargetIndex == target &&
			presence.Equal(relation.TargetPresence, targetPresence) {
			return
		}
	}
	t.Fatalf(
		"return presence relations = %#v, want %d/%s -> %d/%s",
		relations,
		trigger,
		triggerPresence,
		target,
		targetPresence,
	)
}
