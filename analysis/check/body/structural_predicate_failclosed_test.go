package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestStructuralLuaTypePredicateRejectsUnsealedGlobalEnvironment(t *testing.T) {
	for _, test := range []struct{ name, source string }{
		{name: "conditional overwrite", source: `function f(value, flag)
			if flag then type = replacement end
			return type(value) == "string" and value ~= ""
		end`},
		{name: "nested overwrite", source: `function f(value)
			local function poison() type = replacement end
			return type(value) == "string" and value ~= ""
		end`},
		{name: "global table escape", source: `function f(value)
			mutate(_G)
			return type(value) == "string" and value ~= ""
		end`},
	} {
		t.Run(test.name, func(t *testing.T) {
			reg := standard.Registry()
			prepared, err := PrepareFunction(parseFunction(t, test.source), Config{
				Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true},
				Globals: []string{"replacement", "mutate"},
			})
			if err != nil {
				t.Fatal(err)
			}
			sealedTerm := false
			prepared.operationPlan.Facts().ForEachExpressionOperation(func(_ factflow.ExprRef, op factflow.ExpressionOperation) bool {
				if _, exact := op.Intrinsic(); exact {
					sealedTerm = true
				}
				return true
			})
			if sealedTerm {
				t.Fatal("unsealed global environment published lua_type expression authority")
			}
			shape := transformer.Shape{
				Params:   uint32(len(prepared.operationPlan.BoundaryParams())),
				Captures: uint32(len(prepared.operationPlan.BoundaryCaptures())),
				Globals:  uint32(len(prepared.operationPlan.BoundaryGlobals())),
			}
			relation := transformer.NewPlanCompiler().Compile(reg, prepared.cfg.Graph, prepared.operationPlan, shape)
			if reason := relation.ContextualReason(); reason == "" {
				t.Fatal("unsealed structural type predicate compiled non-contextually")
			}
		})
	}
}

func TestStructuralLuaTypePredicateRejectsEffectfulRHS(t *testing.T) {
	reg := standard.Registry()
	prepared, err := PrepareFunction(parseFunction(t, `function f(value)
		return type(value) == "string" and effect(value)
	end`), Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}, Globals: []string{"effect"}})
	if err != nil {
		t.Fatal(err)
	}
	shape := transformer.Shape{Params: uint32(len(prepared.operationPlan.BoundaryParams())), Globals: uint32(len(prepared.operationPlan.BoundaryGlobals()))}
	if relation := transformer.NewPlanCompiler().Compile(reg, prepared.cfg.Graph, prepared.operationPlan, shape); relation.ContextualReason() == "" {
		t.Fatal("effectful structural RHS compiled non-contextually")
	}
}

func TestStructuralLuaTypePredicateCastCertificateFailsClosed(t *testing.T) {
	for _, test := range []struct{ name, source string }{
		{name: "unrelated cast", source: `function f(value)
			local unused = value :: string
			return type(value) == "string" and value ~= ""
		end`},
		{name: "effectful cast source", source: `function f(value)
			return type(value) == "string" and (effect(value) :: string) ~= ""
		end`},
	} {
		t.Run(test.name, func(t *testing.T) {
			reg := standard.Registry()
			prepared, err := PrepareFunction(parseFunction(t, test.source), Config{
				Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}, Globals: []string{"effect"},
			})
			if err != nil {
				t.Fatal(err)
			}
			shape := transformer.Shape{
				Params: uint32(len(prepared.operationPlan.BoundaryParams())), Globals: uint32(len(prepared.operationPlan.BoundaryGlobals())),
			}
			if relation := transformer.NewPlanCompiler().Compile(reg, prepared.cfg.Graph, prepared.operationPlan, shape); relation.ContextualReason() == "" {
				t.Fatal("uncertified cast compiled non-contextually")
			}
		})
	}
}

func TestStructuralLuaTypePredicateRejectsWrongShortCircuitPolarity(t *testing.T) {
	reg := standard.Registry()
	prepared, err := PrepareFunction(parseFunction(t, `function f(value)
		return type(value) == "string" or value ~= ""
	end`), Config{Registry: reg, Signatures: signaturelookup.Source{IncludeStdlib: true}})
	if err != nil {
		t.Fatal(err)
	}
	shape := transformer.Shape{Params: uint32(len(prepared.operationPlan.BoundaryParams())), Globals: uint32(len(prepared.operationPlan.BoundaryGlobals()))}
	if relation := transformer.NewPlanCompiler().Compile(reg, prepared.cfg.Graph, prepared.operationPlan, shape); relation.ContextualReason() == "" {
		t.Fatal("logical or predicate compiled through and-only structural slice")
	}
}
