package provenance

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/flow"
)

func TestNormalizeSelectResultVariantOrigins(t *testing.T) {
	in := provenanceInputs(t, `
local result = select({
	events:receive(),
	timeout:receive(),
	default = timeout,
})
return result
`, "events", "timeout")

	facts := Normalize(Input{
		Graph:       in.Graph,
		ConstValues: in.ConstValues,
		ReturnTransform: func(call *cfg.CallInfo, retIndex int) (effect.ReturnType, bool) {
			if call == nil || call.CalleeName != "select" || retIndex != 0 {
				return nil, false
			}
			return effect.SelectResultOfCases{Cases: effect.ParamRef{Index: 0}}, true
		},
	})
	if len(facts.VariantFieldOrigins) != 2 {
		t.Fatalf("VariantFieldOrigins = %#v, want two non-default select cases", facts.VariantFieldOrigins)
	}
	if len(facts.VariantCaseFieldProjections) != 2 {
		t.Fatalf("VariantCaseFieldProjections = %#v, want two non-default select payload projections", facts.VariantCaseFieldProjections)
	}

	resultSym := oneSymbolNamed(t, in.Graph, "result")
	eventsSym := oneSymbolNamed(t, in.Graph, "events")
	timeoutSym := oneSymbolNamed(t, in.Graph, "timeout")

	assertSelectOrigin(t, facts.VariantFieldOrigins, resultSym, eventsSym, 0)
	assertSelectOrigin(t, facts.VariantFieldOrigins, resultSym, timeoutSym, 1)
	assertSelectProjection(t, facts.VariantCaseFieldProjections, resultSym, eventsSym, 0)
	assertSelectProjection(t, facts.VariantCaseFieldProjections, resultSym, timeoutSym, 1)
}

func provenanceInputs(t *testing.T, body string, params ...string) input.Inputs {
	t.Helper()
	stmts, err := parse.ParseString(body, "provenance.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{Names: params},
		Stmts:   stmts,
	}
	return input.BuildFromFunction(fn, nil, nil, "select")
}

func oneSymbolNamed(t *testing.T, g *cfg.Graph, name string) cfg.SymbolID {
	t.Helper()
	if g == nil || g.Bindings() == nil {
		t.Fatalf("missing graph bindings while looking up %s", name)
	}
	symbols := g.Bindings().SymbolsByName(name)
	if len(symbols) != 1 {
		t.Fatalf("symbols named %s = %v, want one", name, symbols)
	}
	return symbols[0]
}

func assertSelectOrigin(t *testing.T, origins []flow.VariantFieldOrigin, targetSym, sourceSym cfg.SymbolID, caseID int) {
	t.Helper()
	for _, origin := range origins {
		if origin.Target.Symbol != targetSym ||
			origin.Field != effect.SelectResultChannelField ||
			origin.Source.Symbol != sourceSym ||
			origin.OriginFamily == 0 ||
			origin.CaseIndex != caseID {
			continue
		}
		return
	}
	t.Fatalf("missing select origin target=%d source=%d case=%d in %#v", targetSym, sourceSym, caseID, origins)
}

func assertSelectProjection(t *testing.T, projections []flow.VariantCaseFieldProjection, targetSym, sourceSym cfg.SymbolID, caseID int) {
	t.Helper()
	for _, projection := range projections {
		if projection.Target.Symbol != targetSym ||
			projection.Field != effect.SelectResultValueField ||
			projection.Source.Symbol != sourceSym ||
			projection.OriginFamily == 0 ||
			projection.CaseIndex != caseID ||
			len(projection.SourceSteps) != 1 ||
			projection.SourceSteps[0].Kind != effect.TypeProjectionGenericArg ||
			projection.SourceSteps[0].Index != 0 {
			continue
		}
		return
	}
	t.Fatalf("missing select projection target=%d source=%d case=%d in %#v", targetSym, sourceSym, caseID, projections)
}
