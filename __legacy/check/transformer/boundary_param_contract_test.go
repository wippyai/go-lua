package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	valuerefinement "github.com/wippyai/go-lua/__legacy/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func TestBoundaryUntypedParameterContractPreservesConcreteArgument(t *testing.T) {
	reg := standard.Registry()
	actualType := typetable.NewRecord().OptField("data_func", typ.String).Build()
	actual := typevalue.WithWitness(reg, typevalue.FromType(reg, actualType), actualType)
	gradual := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())

	if got := meetBoundaryParamContract(reg, actual, gradual); !product.Equal(reg, got, actual) {
		t.Fatalf("concrete argument = %#v after untyped contract, want exact %#v", got, actual)
	}
}

func TestBoundaryParamContractModesSeparateDefinitionFromConcreteApply(t *testing.T) {
	reg := standard.Registry()
	param := symbol.ID(91)
	contract := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	plan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryParams([]symbol.ID{param}).
		WithBoundaryParamContracts([]product.Value{contract}).
		WithBoundaryCaptures(nil).
		WithBoundaryGlobals(nil)
	builder := visibility.NewBuilder()
	builder.Define(0, param, "param")
	resolver := visibility.NewResolver(builder.Build())
	roots, err := sealRelationRootCarrier(plan, resolver.KeySpace(), Shape{Params: 1})
	if err != nil {
		t.Fatal(err)
	}
	body := relationProgramBody{plan: plan, roots: roots}

	actualID := identity.LuaFunction(uint64(param))
	actual := product.Set(reg, contract, identity.Key, identity.Singleton(actualID))
	entry := state.Reachable(state.Domain(reg).Bottom()).
		WriteValue(reg, key.SymbolValue(param), actual).
		WriteLocalPathKey(reg, resolver.KeySpace().FromPath(pathdom.NewPath(param, "")), actual)

	definition := applyBoundaryParamContracts(reg, &body, entry, boundaryParamContractDefinition)
	if got := definition.ReadValue(reg, key.SymbolValue(param)); !product.Equal(reg, got, contract) {
		t.Fatalf("definition scalar = %#v, want authoritative contract %#v", got, contract)
	}
	if got := definition.ReadLocalPathKey(reg, roots.roots[0].path); !product.Equal(reg, got, contract) {
		t.Fatalf("definition path = %#v, want authoritative contract %#v", got, contract)
	}

	concrete := applyBoundaryParamContracts(reg, &body, entry, boundaryParamContractConcrete)
	for label, got := range map[string]product.Value{
		"scalar": concrete.ReadValue(reg, key.SymbolValue(param)),
		"path":   concrete.ReadLocalPathKey(reg, roots.roots[0].path),
	} {
		if gotID, ok := product.Get(reg, got, identity.Key).ID(); !ok || gotID != actualID {
			t.Fatalf("concrete %s identity = %#v/%v, want transported %v", label, gotID, ok, actualID)
		}
	}

	literal := typevalue.LiteralString(reg, "exact")
	literalEntry := state.Reachable(state.Domain(reg).Bottom()).
		WriteValue(reg, key.SymbolValue(param), literal).
		WriteLocalPathKey(reg, roots.roots[0].path, literal)
	concreteLiteral := applyBoundaryParamContracts(reg, &body, literalEntry, boundaryParamContractConcrete)
	for label, got := range map[string]product.Value{
		"scalar": concreteLiteral.ReadValue(reg, key.SymbolValue(param)),
		"path":   concreteLiteral.ReadLocalPathKey(reg, roots.roots[0].path),
	} {
		if !product.Equal(reg, got, literal) {
			t.Fatalf("concrete %s literal widened across declared string contract", label)
		}
	}

	bottom := state.Reachable(state.Domain(reg).Bottom())
	concreteBottom := applyBoundaryParamContracts(reg, &body, bottom, boundaryParamContractConcrete)
	if got := concreteBottom.ReadValue(reg, key.SymbolValue(param)); !product.Equal(reg, got, product.Bottom(reg)) {
		t.Fatalf("concrete Bottom scalar = %#v, want Bottom", got)
	}
}

func TestBoundaryReturnContractPlanIsUnaryAndMatchesWholeStateApplication(t *testing.T) {
	reg := standard.Registry()
	literalContract := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	declaredRecordType := typetable.NewRecord().Field("name", typ.String).Build()
	declaredRecord := typevalue.WithWitness(reg, typevalue.FromType(reg, declaredRecordType), declaredRecordType)
	targetPlan := operationplan.New(cfg.New(), factflow.FactsInput{}).
		WithBoundaryReturns([]product.Value{literalContract, declaredRecord, literalContract})
	callerPlan := operationplan.New(cfg.New(), factflow.FactsInput{})
	caller := &relationProgramBody{plan: callerPlan}
	target := &relationProgramBody{plan: targetPlan}
	frame := &linkedRelationFrame{}

	literal := typevalue.LiteralString(reg, "exact")
	computedRecordType := typetable.NewRecord().Field("name", typ.LiteralString("Ada")).Build()
	computedRecord := typevalue.WithWitness(reg, typevalue.FromType(reg, computedRecordType), computedRecordType)
	bottom := product.Bottom(reg)
	callee := state.Reachable(state.Domain(reg).Bottom()).
		WriteReturnSlot(reg, 0, literal).
		WriteReturnSlot(reg, 1, computedRecord)

	plan := prepareBoundaryReturnContractPlan(reg, caller, target, frame, state.State{}, callee)
	whole := applyBoundaryReturnContracts(reg, caller, target, frame, state.State{}, callee)
	for index, input := range []product.Value{literal, computedRecord, bottom} {
		want := plan.NormalizeResult(index, input)
		if got := whole.ReadReturnSlot(reg, index); !product.Equal(reg, got, want) {
			t.Fatalf("whole-state result %d = %#v, want unary normalization %#v", index, got, want)
		}
	}

	if got := plan.NormalizeResult(0, literal); !product.Equal(reg, got, literal) {
		t.Fatalf("literal result widened across string declaration: %#v", got)
	}
	wantRecord := product.WithPresence(reg,
		valuerefinement.MergeDeclaredContract(reg, computedRecord, declaredRecord),
		product.PresenceOf(computedRecord))
	if got := plan.NormalizeResult(1, computedRecord); !product.Equal(reg, got, wantRecord) {
		t.Fatalf("record result witness = %#v, want declaration-normalized witness %#v", got, wantRecord)
	}
	if got := plan.NormalizeResult(2, bottom); !product.Equal(reg, got, bottom) {
		t.Fatalf("Bottom result changed across declaration: %#v", got)
	}
	computedOptional := product.WithPresence(reg, typevalue.FromType(reg, typ.Number), presence.Maybe())
	requiredNumber := typevalue.FromType(reg, typ.Number)
	requiredPlan := boundaryReturnContractPlan{reg: reg, results: []boundaryReturnContract{{contract: requiredNumber, active: true}}}
	if got := requiredPlan.NormalizeResult(0, computedOptional); !presence.Equal(product.PresenceOf(got), presence.Present()) {
		t.Fatalf("declared required result retained body-local optionality: %#v", got)
	}
	if got := plan.NormalizeResult(99, literal); !product.Equal(reg, got, literal) {
		t.Fatalf("out-of-range normalization was not identity: %#v", got)
	}
}
