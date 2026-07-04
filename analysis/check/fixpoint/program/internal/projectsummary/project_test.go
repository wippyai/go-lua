package projectsummary_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
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

func TestFromResultPreservesAnnotatedArrayReturnAfterDynamicIPairsInsert(t *testing.T) {
	reg := standard.Registry()
	fn := projectParseFunction(t, `
function group_by_suite(entries)
	local suites: {[string]: any[]} = {}
	local no_suite: any[] = {}
	for _, entry in ipairs(entries) do
		table.insert(no_suite, entry)
	end
	return suites, no_suite
end`)

	result := projectCheckFunction(t, fn, body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})
	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 2 {
		t.Fatalf("returns = %d, want 2", len(got.Returns))
	}
	gotType, ok := typevalue.TypeOf(reg, got.Returns[1])
	if !ok || !typ.TypeEquals(gotType, typ.NewArray(typ.Any)) {
		t.Fatalf("return slot 2 type = %v/%v, want any[]", gotType, ok)
	}
}

func TestFromResultDeclaredReturnDoesNotEraseComputedIdentity(t *testing.T) {
	reg := standard.Registry()
	retID := identity.ID{Kind: "test.return", Site: "declared", Index: 1}
	numberValue := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(runtimekind.Number))
	numberValue = product.Set(reg, numberValue, identity.Key, identity.Singleton(retID))
	fn := projectParseFunction(t, `
function f(): number
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
	id, ok := product.Get(reg, got.Returns[0], identity.Key).ID()
	if !ok || id != retID {
		t.Fatalf("return identity = %v/%v, want %s", id, ok, retID)
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

func TestFromResultDeclaredReturnDoesNotWidenComputedVariantOrigin(t *testing.T) {
	reg := standard.Registry()
	msg := typetable.NewRecord().
		Field("kind", typ.LiteralString("msg")).
		Field("value", typ.String).
		Build()
	timer := typetable.NewRecord().
		Field("kind", typ.LiteralString("timer")).
		Field("value", typ.Number).
		Build()
	bodyValue := typevalue.WithWitness(reg, typevalue.FromType(reg, msg), msg)
	fn := projectParseFunction(t, `
function f(): {kind: "msg", value: string} | {kind: "timer", value: number}
	return 1
end`)

	result := projectCheckFunction(t, fn, body.Config{
		Registry: reg,
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
			if source.TargetIndex != 0 {
				return product.Value{}, false
			}
			return bodyValue, true
		},
	})
	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 1 {
		t.Fatalf("FromResult returned %d slots, want 1", len(got.Returns))
	}
	gotType, ok := typevalue.TypeOf(reg, got.Returns[0])
	if !ok || !typ.TypeEquals(gotType, msg) {
		t.Fatalf("return type = %v/%v, want computed variant %v (not declared %v)", gotType, ok, msg, typeexpr.Union(msg, timer))
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

func TestFromResultProjectsAnyParamPresenceAfterAssert(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(x: any)
	assert(x)
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	if len(got.NormalReturnParams) != 1 {
		t.Fatalf("normal return params = %d, want 1: %#v", len(got.NormalReturnParams), got)
	}
	if gotPresence := product.PresenceOf(got.NormalReturnParams[0]); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("normal return param presence = %s, want present", gotPresence)
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

func TestFromResultProjectsRuntimeTypeGuardParamConstraintAfterErrorGuard(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(value)
	if type(value) ~= "string" then
		error("expected string")
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

func TestFromResultPreservesStructuralWitnessFlagForUnchangedMutation(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function sort_tests(tests: any[])
	table.sort(tests, function(a, b)
		return true
	end)
end`), body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})

	got := summaryprojection.FromResult(result)
	if len(got.NormalReturnFacts.PathInvalidations) != 1 {
		t.Fatalf("path invalidations = %#v, want one $0 invalidation", got.NormalReturnFacts.PathInvalidations)
	}
	fact := got.NormalReturnFacts.PathInvalidations[0]
	if !fact.Path.Equal(pathdom.NewPlaceholder(0)) || !fact.PreserveStructuralWitness {
		t.Fatalf("path invalidation = %#v, want structural-preserving $0 invalidation", fact)
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

func TestFromResultProjectsParamObligationThroughStableLocalConcat(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(http: {get: (url: string, options: table) -> ()}, endpoint_path)
	local full_url = "https://api.example.test" .. endpoint_path
	return http.get(full_url, {})
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	want := runtimekind.Join(runtimekind.Singleton(runtimekind.String), runtimekind.Singleton(runtimekind.Number))
	projectAssertParamObligationKind(t, reg, got, 1, want)
}

func TestFromResultProjectsParamObligationThroughProjectedFieldConcat(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(http: {get: (url: string, options: table) -> ()}, config: {base_url: string, headers: any}, endpoint_path)
	local full_url = config.base_url .. endpoint_path
	return http.get(full_url, {})
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	want := runtimekind.Join(runtimekind.Singleton(runtimekind.String), runtimekind.Singleton(runtimekind.Number))
	projectAssertParamObligationKind(t, reg, got, 2, want)
}

func TestFromResultProjectsNestedReturnParamPathAliases(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(client)
	return {
		registry = {
			primary = client,
			backup = client,
		},
	}
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	projectAssertReturnParamPathAlias(t, got, 0, pathdom.PathKey(".registry.primary"), pathdom.PathKey("$0"))
	projectAssertReturnParamPathAlias(t, got, 0, pathdom.PathKey(".registry.backup"), pathdom.PathKey("$0"))
}

func TestFromResultProjectsSharedNestedReturnParamPathAliasesUnderEachParent(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(client)
	local entry = {
		primary = client,
	}
	return {
		left = entry,
		right = entry,
	}
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	projectAssertReturnParamPathAlias(t, got, 0, pathdom.PathKey(".left.primary"), pathdom.PathKey("$0"))
	projectAssertReturnParamPathAlias(t, got, 0, pathdom.PathKey(".right.primary"), pathdom.PathKey("$0"))
}

func TestFromResultDoesNotProjectSharedLocalReturnAliasAfterFieldMutation(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(client, fallback)
	local entry = {
		primary = client,
	}
	entry.primary = fallback
	return {
		left = entry,
	}
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	projectAssertNoReturnParamPathAlias(t, got, 0, pathdom.PathKey(".left.primary"), pathdom.PathKey("$0"))
}

func TestFromResultProjectsReturnedMemberParamPathAlias(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(layer)
	return {
		api = layer.registry,
	}
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	projectAssertReturnParamPathAlias(t, got, 0, pathdom.PathKey(".api"), pathdom.PathKey("$0.registry"))
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

func TestFromResultProjectsParamObligationFromArithmeticMemberOperand(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(result)
	return result.delay_applied * 2
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	if len(got.ParamObligations) == 0 {
		t.Fatalf("param obligations = %#v, want obligation for result.delay_applied", got.ParamObligations)
	}
	gotType, ok := typevalue.TypeOf(reg, got.ParamObligations[0])
	if !ok {
		t.Fatalf("param obligation type missing: %#v", got.ParamObligations[0])
	}
	want := typetable.NewRecord().Field("delay_applied", typ.Number).Build()
	if !typ.TypeEquals(gotType, want) {
		t.Fatalf("param obligation type = %v, want %v", gotType, want)
	}
}

func TestFromResultProjectsParamMemberObligationFromLengthOperand(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(template)
	return #template.operations
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	if len(got.ParamObligations) == 0 {
		t.Fatalf("param obligations = %#v, want obligation for template.operations", got.ParamObligations)
	}
	gotType, ok := typevalue.TypeOf(reg, got.ParamObligations[0])
	if !ok {
		t.Fatalf("param obligation type missing: %#v", got.ParamObligations[0])
	}
	want := typetable.NewRecord().
		Field("operations", normalize.UnionForEvidence(typ.String, typetable.BuiltinTopMarker())).
		Build()
	if !typ.TypeEquals(gotType, want) {
		t.Fatalf("param obligation type = %v, want %v", gotType, want)
	}
}

func TestFromResultDoesNotProjectGuardedParamObligation(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(x: unknown)
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

func TestFromResultDoesNotProjectGuardedAliasParamObligation(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(x)
	local y = x
	if type(y) == "string" then
		sink(y)
	end
end`), body.Config{
		Registry: reg,
		Globals:  []string{"type", "sink"},
	})

	got := summaryprojection.FromResult(result)

	if len(got.ParamObligations) != 0 {
		t.Fatalf("param obligations = %#v, want none for guarded alias use", got.ParamObligations)
	}
}

func TestFromResultDoesNotProjectGuardedAliasTypedCallObligation(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function f(x)
	local function sink(s: string): ()
	end
	local y = x
	if type(y) == "string" then
		sink(y)
	end
end`), body.Config{
		Registry: reg,
		Globals:  []string{"type"},
	})

	got := summaryprojection.FromResult(result)

	if len(got.ParamObligations) != 0 {
		t.Fatalf("param obligations = %#v, want none for guarded typed call through alias", got.ParamObligations)
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
	projectAssertReturnPresenceRelation(t, got.ReturnPresenceRelations, 0, presence.Absent(), 1, presence.Present())
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

func TestFromResultInfersErrorReturnPresenceRelationsForTableValue(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function raw_get(dsn: string): ({release: any}?, string?)
	if dsn == "" then
		return nil, "missing dsn"
	end
	return {}, nil
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	projectAssertReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Present(), 0, presence.Absent())
	projectAssertReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Absent(), 0, presence.Present())
	projectAssertReturnPresenceRelation(t, got.ReturnPresenceRelations, 0, presence.Absent(), 1, presence.Present())
}

func TestFromResultInfersFalseReturnConditionSlotRefinement(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function wait_for_database(ready: boolean): (boolean, string?)
	if ready then
		return true, nil
	end
	return false, "database unavailable"
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	var found summary.ReturnConditionSlotRefinement
	ok := false
	for _, candidate := range got.ReturnConditionSlotRefinements {
		if candidate.ReturnIndex == 0 && !candidate.ReturnValue && candidate.TargetIndex == 1 {
			found = candidate
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("false return condition slot refinement missing: %#v", got.ReturnConditionSlotRefinements)
	}
	if gotType, typeOK := typevalue.TypeOf(reg, found.Value); !typeOK || !subtype.IsSubtype(gotType, typ.String) {
		t.Fatalf("false return condition target type = %v/%v, want subtype of string", gotType, typeOK)
	}
}

func TestFromResultReturnConditionSlotUsesDeclaredTargetContract(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function raw_get(dsn: string): ({ release: any }?, string?)
	if dsn == "" then
		return nil, "missing dsn"
	end
	return {}, nil
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	var found summary.ReturnConditionSlotRefinement
	ok := false
	for _, candidate := range got.ReturnConditionSlotRefinements {
		if candidate.ReturnIndex == 1 && !candidate.ReturnValue && candidate.TargetIndex == 0 {
			found = candidate
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("err-false return condition slot refinement missing: %#v", got.ReturnConditionSlotRefinements)
	}
	if gotPresence := product.PresenceOf(found.Value); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("err-false target presence = %s, want present", gotPresence)
	}
	gotType, typeOK := typevalue.TypeOf(reg, found.Value)
	want := typetable.NewRecord().Field("release", typ.Any).Build()
	if !typeOK || !subtype.IsSubtype(gotType, want) {
		t.Fatalf("err-false target type = %v/%v, want declared DB contract with release member", gotType, typeOK)
	}
}

func TestFromResultFalseReturnConditionSlotRefinementKeepsMissingArm(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function wait_for_database(ready: boolean, missing: boolean): (boolean, string?)
	if ready then
		return true, nil
	end
	if missing then
		return false, nil
	end
	return false, "database unavailable"
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	var found summary.ReturnConditionSlotRefinement
	ok := false
	for _, candidate := range got.ReturnConditionSlotRefinements {
		if candidate.ReturnIndex == 0 && !candidate.ReturnValue && candidate.TargetIndex == 1 {
			found = candidate
			ok = true
			break
		}
	}
	if !ok {
		t.Fatalf("false return condition slot refinement missing: %#v", got.ReturnConditionSlotRefinements)
	}
	if gotPresence := product.PresenceOf(found.Value); !presence.Equal(gotPresence, presence.Maybe()) {
		t.Fatalf("false return condition target presence = %s, want maybe", gotPresence)
	}
}

func TestFromResultDoesNotInferReturnPresenceRelationThroughUnknownSlot(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function fetch(): (number?, string?)
	return value, "failed"
end`), body.Config{
		Registry: reg,
		Globals:  []string{"value"},
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
			switch source.TargetIndex {
			case 0:
				return product.Top(), true
			case 1:
				return product.NewWithPresence(reg, product.ShapeTop, presence.Present()), true
			default:
				return product.Value{}, false
			}
		},
	})

	got := summaryprojection.FromResult(result)

	if len(got.ReturnPresenceRelations) != 0 {
		t.Fatalf("return presence relations = %#v, want none when a slot has top presence", got.ReturnPresenceRelations)
	}
	if len(got.Returns) != 2 {
		t.Fatalf("returns = %d, want declared arity 2", len(got.Returns))
	}
	if gotPresence := product.PresenceOf(got.Returns[0]); !presence.Equal(gotPresence, presence.Maybe()) {
		t.Fatalf("return 0 presence = %s, want maybe from top source evidence", gotPresence)
	}
}

func TestFromResultDeclaredNonOptionalReturnForcesPresentPresence(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function fetch(): number
	return value
end`), body.Config{
		Registry: reg,
		Globals:  []string{"value"},
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
			if source.TargetIndex == 0 {
				return product.Top(), true
			}
			return product.Value{}, false
		},
	})

	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 1 {
		t.Fatalf("returns = %d, want declared arity 1", len(got.Returns))
	}
	if gotPresence := product.PresenceOf(got.Returns[0]); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("return 0 presence = %s, want present from declared number", gotPresence)
	}
	gotType, ok := typevalue.TypeOf(reg, got.Returns[0])
	if !ok || !typ.TypeEquals(gotType, typ.Number) {
		t.Fatalf("return 0 type = %v/%v, want number", gotType, ok)
	}
}

func TestFromResultReturnSourceDoesNotDiscardAccumulatorShape(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function run_migrations(failed: boolean): any
	local results = {
		status = "running",
		migrations_failed = 0,
	}
	if failed then
		results.migrations_failed = results.migrations_failed + 1
		results.status = "error"
		results.error = "failed"
	else
		results.status = "complete"
	end
	return results
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 1 {
		t.Fatalf("returns = %d, want one", len(got.Returns))
	}
	gotType, ok := typevalue.TypeOf(reg, got.Returns[0])
	if !ok {
		t.Fatalf("return 0 has no type witness")
	}
	rec, ok := gotType.(*typ.Record)
	if !ok {
		t.Fatalf("return 0 type = %v, want accumulator record shape", gotType)
	}
	if field := rec.GetField("migrations_failed"); field == nil || !typ.TypeEquals(field.Type, typ.Integer) {
		t.Fatalf("migrations_failed field = %#v, want integer", field)
	}
	status := rec.GetField("status")
	if status == nil {
		t.Fatalf("return 0 type = %v, want status field", gotType)
	}
	if !subtype.IsSubtype(typ.LiteralString("error"), status.Type) ||
		!subtype.IsSubtype(typ.LiteralString("complete"), status.Type) {
		t.Fatalf("status field = %v, want at least error|complete", status.Type)
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

func TestFromResultTreatsUnannotatedOmittedReturnSlotsAsAbsent(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function fetch(ok: boolean)
	if not ok then
		return nil, "failed"
	end
	return 1
end`), body.Config{Registry: reg})

	got := summaryprojection.FromResult(result)

	projectAssertReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Present(), 0, presence.Absent())
	projectAssertReturnPresenceRelation(t, got.ReturnPresenceRelations, 1, presence.Absent(), 0, presence.Present())
	if len(got.Returns) != 2 {
		t.Fatalf("returns = %d, want max observed arity 2", len(got.Returns))
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
	if !ok || !typ.TypeEquals(gotType, typeexpr.Optional(typ.Number)) {
		t.Fatalf("return 0 witness = %v/%v, want number?", gotType, ok)
	}
}

func TestFromResultPreservesDeclaredAliasReturnWithoutExplicitReturn(t *testing.T) {
	reg := standard.Registry()
	stmts := projectParseChunk(t, `
type Message = {
	from: fun(self: Message): string,
}
type Channel = {
	receive: fun(self: Channel): (Message, boolean),
}
function listen(): Channel
	error("stub")
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	def, ok := stmts[2].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want function definition", stmts[2])
	}
	result, err := body.CheckBoundFunction(def.Func, bindings, body.Config{Registry: reg})
	if err != nil {
		t.Fatalf("CheckBoundFunction: %v", err)
	}

	got := summaryprojection.FromResult(result)

	if len(got.Returns) != 1 {
		t.Fatalf("returns = %d, want 1: %#v", len(got.Returns), got)
	}
	witness := product.Get(reg, got.Returns[0], typewitness.Key)
	gotType, ok := witness.Type()
	if !ok || typ.IsAny(gotType) || typ.IsUnknown(gotType) || typ.IsNever(gotType) {
		t.Fatalf("return witness = %v/%v, want concrete Channel", gotType, ok)
	}
}

func TestFromResultProjectsReturnedKeyedArrayProvenanceFromRealBody(t *testing.T) {
	reg := standard.Registry()
	result := projectCheckFunction(t, projectParseFunction(t, `
function sorted_keys(t)
	local keys: string[] = {}
	for k in pairs(t) do
		table.insert(keys, k)
	end
	table.sort(keys)
	return keys
end`), body.Config{
		Registry:   reg,
		Signatures: signaturelookup.Source{IncludeStdlib: true},
	})

	got := summaryprojection.FromResult(result)

	if len(got.NormalReturnFacts.DynamicIndexFacts) == 0 {
		t.Fatalf("dynamic-index facts missing from summary: %#v", got.NormalReturnFacts)
	}
	var dynamicFactFound bool
	for _, fact := range got.NormalReturnFacts.DynamicIndexFacts {
		if fact.Table.Equal(pathdom.Path{Root: "ret[0]"}) && fact.Site != "" && fact.Value.Admission == dynamicindex.AdmissionAdmitted {
			dynamicFactFound = true
			break
		}
	}
	if !dynamicFactFound {
		t.Fatalf("dynamic-index facts = %#v, want admitted ret[0] array write", got.NormalReturnFacts.DynamicIndexFacts)
	}
	if len(got.NormalReturnFacts.DynamicValueKeys) != 1 ||
		!got.NormalReturnFacts.DynamicValueKeys[0].Container.Equal(pathdom.Path{Root: "ret[0]"}) ||
		!got.NormalReturnFacts.DynamicValueKeys[0].Table.Equal(pathdom.NewPlaceholder(0)) {
		t.Fatalf("dynamic value key facts = %#v, want ret[0] values proven as keys of $0", got.NormalReturnFacts.DynamicValueKeys)
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

func projectAssertReturnParamPathAlias(
	t *testing.T,
	got summary.Summary,
	returnIndex int,
	member pathdom.PathKey,
	source pathdom.PathKey,
) {
	t.Helper()
	for _, alias := range got.ReturnParamPathAliases {
		if alias.ReturnIndex == returnIndex && alias.Member.PathKey() == member && alias.Source.PathKey() == source {
			return
		}
	}
	t.Fatalf(
		"return param path aliases = %#v, want return %d %s -> %s",
		got.ReturnParamPathAliases,
		returnIndex,
		member,
		source,
	)
}

func projectAssertNoReturnParamPathAlias(
	t *testing.T,
	got summary.Summary,
	returnIndex int,
	member pathdom.PathKey,
	source pathdom.PathKey,
) {
	t.Helper()
	for _, alias := range got.ReturnParamPathAliases {
		if alias.ReturnIndex == returnIndex && alias.Member.PathKey() == member && alias.Source.PathKey() == source {
			t.Fatalf(
				"return param path aliases = %#v, did not want return %d %s -> %s",
				got.ReturnParamPathAliases,
				returnIndex,
				member,
				source,
			)
		}
	}
}
