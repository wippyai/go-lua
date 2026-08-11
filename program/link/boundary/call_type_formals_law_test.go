package boundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestBoundaryCallTypeFormalArgumentsExactZeroAndUnconstrainedBinding(t *testing.T) {
	zeroProgram := typeFormalProgram(t, `local function zero() return nil end
return zero()`)
	zeroComponent, zeroContract, zeroProject := typeFormalBoundary(t, zeroProgram, nil)
	zeroOperation := typeFormalOperation(t, zeroContract)
	zeroCall := typeFormalCall(t, zeroProject, 0)
	zero, ok := zeroComponent.Calls().TypeFormalArguments(zeroContract, zeroCall, zeroOperation)
	if !ok || zero.Count() != 0 {
		t.Fatalf("zero-formal correspondence = %#v/%t", zero, ok)
	}
	if _, ok := zero.At(0); ok {
		t.Fatal("zero-formal correspondence exposed an argument")
	}

	genericProgram := typeFormalProgram(t, `local function generic<T, U>() return nil end
return generic::<string, integer>()`)
	component, contract, project := typeFormalBoundary(t, genericProgram, []*typ.TypeParam{
		typ.NewTypeParam("T", nil), typ.NewTypeParam("U", nil),
	})
	operation := typeFormalOperation(t, contract)
	call := typeFormalCall(t, project, 0)
	arguments, ok := component.Calls().TypeFormalArguments(contract, call, operation)
	if !ok || arguments.Count() != 2 {
		t.Fatalf("unconstrained correspondence = %#v/%t", arguments, ok)
	}
	for index := 0; index < arguments.Count(); index++ {
		got, gotOK := arguments.At(index)
		want, wantOK := genericProgram.Static().Contracts().Calls().TypeArgumentAt(callTerm(t, project, call), index)
		if !gotOK || !wantOK || got != want {
			t.Fatalf("argument %d = %v/%t, want %v/%t", index, got, gotOK, want, wantOK)
		}
	}
}

func TestBoundaryCallTypeFormalArgumentsRejectsMissingExtraConstrainedAndNonCalls(t *testing.T) {
	genericProgram := typeFormalProgram(t, `local function generic<T, U>() return nil end
return generic::<string, integer>()`)
	for _, formals := range [][]*typ.TypeParam{
		{typ.NewTypeParam("T", nil)},
		{typ.NewTypeParam("T", nil), typ.NewTypeParam("U", nil), typ.NewTypeParam("V", nil)},
		{typ.NewTypeParam("T", typ.String), typ.NewTypeParam("U", nil)},
	} {
		component, contract, project := typeFormalBoundary(t, genericProgram, formals)
		if _, ok := component.Calls().TypeFormalArguments(contract, typeFormalCall(t, project, 0), typeFormalOperation(t, contract)); ok {
			t.Fatalf("formals %#v admitted without an exact unconstrained static proof", formals)
		}
	}

	component, contract, project := typeFormalBoundary(t, boundaryProgram(t), nil)
	operation := typeFormalOperation(t, contract)
	applications := project.Applications()
	var generic, imported, operator linkproject.Application
	for index := 0; index < applications.Count(); index++ {
		application, ok := applications.At(index)
		if !ok {
			t.Fatalf("application %d unavailable", index)
		}
		if _, _, ok := applications.Generic(application); ok {
			generic = application
		}
		if _, _, _, ok := applications.Import(application); ok {
			imported = application
		}
		if _, _, ok := applications.Operators().Arithmetic(application); ok {
			operator = application
		}
	}
	for name, application := range map[string]linkproject.Application{
		"generic": generic, "import": imported, "operator": operator,
	} {
		if application == (linkproject.Application{}) {
			t.Fatalf("fixture lacks %s application", name)
		}
		if _, ok := component.Calls().TypeFormalArguments(contract, application, operation); ok {
			t.Fatalf("%s application admitted as an ordinary static Call", name)
		}
	}

	foreignContract := typeFormalContract(t, nil)
	if _, ok := component.Calls().TypeFormalArguments(foreignContract, typeFormalCall(t, project, 0), operation); ok {
		t.Fatal("equivalent foreign Target crossed the Boundary fence")
	}
	foreignProject, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: boundaryProgram(t)}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	foreignComponent, err := foreignProject.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := component.Calls().TypeFormalArguments(contract, typeFormalCall(t, foreignComponent, 0), operation); ok {
		t.Fatal("equivalent foreign Project Call crossed the Boundary fence")
	}
}

func TestBoundaryCallTypeFormalArgumentsPreserveAtOrderAndDoNotAllocate(t *testing.T) {
	p := typeFormalProgram(t, `local function generic<T, U>() return nil end
return generic::<string, integer>()`)
	component, contract, project := typeFormalBoundary(t, p, []*typ.TypeParam{
		typ.NewTypeParam("T", nil), typ.NewTypeParam("U", nil),
	})
	operation, call := typeFormalOperation(t, contract), typeFormalCall(t, project, 0)
	arguments, ok := component.Calls().TypeFormalArguments(contract, call, operation)
	if !ok {
		t.Fatal("unconstrained correspondence unavailable")
	}
	callTerm := callTerm(t, project, call)
	for index := 0; index < arguments.Count(); index++ {
		got, gotOK := arguments.At(index)
		want, wantOK := p.Static().Contracts().Calls().TypeArgumentAt(callTerm, index)
		if !gotOK || !wantOK || got != want {
			t.Fatalf("At(%d) = %v/%t, want %v/%t", index, got, gotOK, want, wantOK)
		}
	}
	id, idOK := arguments.CorrespondenceID()
	if !idOK || !id.Available() {
		t.Fatal("correspondence identity unavailable")
	}
	resealProgram := typeFormalProgram(t, `local function generic<T, U>() return nil end
return generic::<string, integer>()`)
	reseal, resealContract, resealProject := typeFormalBoundary(t, resealProgram, []*typ.TypeParam{
		typ.NewTypeParam("X", nil), typ.NewTypeParam("Y", nil),
	})
	resealArguments, resealOK := reseal.Calls().TypeFormalArguments(resealContract, typeFormalCall(t, resealProject, 0), typeFormalOperation(t, resealContract))
	resealID, resealIDOK := resealArguments.CorrespondenceID()
	if !resealOK || !resealIDOK || resealID != id {
		t.Fatal("equivalent reseal changed the correspondence identity")
	}

	changedArgumentsProgram := typeFormalProgram(t, `local function generic<T, U>() return nil end
return generic::<integer, string>()`)
	changedArguments, changedArgumentsContract, changedArgumentsProject := typeFormalBoundary(t, changedArgumentsProgram, []*typ.TypeParam{
		typ.NewTypeParam("T", nil), typ.NewTypeParam("U", nil),
	})
	changedArgumentsView, changedArgumentsOK := changedArguments.Calls().TypeFormalArguments(changedArgumentsContract, typeFormalCall(t, changedArgumentsProject, 0), typeFormalOperation(t, changedArgumentsContract))
	changedArgumentsID, changedArgumentsIDOK := changedArgumentsView.CorrespondenceID()
	if !changedArgumentsOK || !changedArgumentsIDOK || changedArgumentsID == id {
		t.Fatal("static type-argument mutation did not change correspondence identity")
	}

	changedABIContract := typeFormalContractWithInput(t, []*typ.TypeParam{
		typ.NewTypeParam("T", nil), typ.NewTypeParam("U", nil),
	}, target.ValuesSpec{Fixed: []typ.Type{typ.Any}, Tail: target.ValuesClosed})
	changedABI, changedABIProject := typeFormalBoundaryForContract(t, p, changedABIContract)
	changedABIView, changedABIOK := changedABI.Calls().TypeFormalArguments(changedABIContract, typeFormalCall(t, changedABIProject, 0), typeFormalOperation(t, changedABIContract))
	changedABIID, changedABIIDOK := changedABIView.CorrespondenceID()
	if !changedABIOK || !changedABIIDOK || changedABIID == id {
		t.Fatal("owner effect ABI mutation did not change correspondence identity")
	}
	countAtAllocs := testing.AllocsPerRun(100, func() {
		if arguments.Count() != 2 {
			panic("unavailable exact type-formal correspondence")
		}
		if _, available := arguments.At(0); !available {
			panic("missing static type argument")
		}
	})
	idAllocs := testing.AllocsPerRun(100, func() {
		if _, available := arguments.CorrespondenceID(); !available {
			panic("missing correspondence identity")
		}
	})
	if countAtAllocs != 0 || idAllocs != 0 {
		t.Fatalf("type-formal correspondence allocations Count/At=%.2f ID=%.2f", countAtAllocs, idAllocs)
	}
}

func typeFormalProgram(t testing.TB, text string) *program.Program {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "boundary-type-formals.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func typeFormalContract(t testing.TB, formals []*typ.TypeParam) *target.Contract {
	return typeFormalContractWithInput(t, formals, target.ValuesSpec{Tail: target.ValuesClosed})
}

func typeFormalContractWithInput(t testing.TB, formals []*typ.TypeParam, input target.ValuesSpec) *target.Contract {
	t.Helper()
	contract, err := target.Seal(&target.Spec{Operations: []target.OperationSpec{{
		Bindings:    []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"op"}}},
		TypeFormals: formals,
		Input:       input,
		Outcomes:    []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: target.ValuesSpec{Tail: target.ValuesClosed}}},
		Effects:     target.RowSpec{Tail: target.RowClosed},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func typeFormalBoundary(t testing.TB, p *program.Program, formals []*typ.TypeParam) (*Component, *target.Contract, *linkproject.Component) {
	t.Helper()
	contract := typeFormalContract(t, formals)
	component, project := typeFormalBoundaryForContract(t, p, contract)
	return component, contract, project
}

func typeFormalBoundaryForContract(t testing.TB, p *program.Program, contract *target.Contract) (*Component, *linkproject.Component) {
	t.Helper()
	draft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: p}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	boundaryDraft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	component, err := boundaryDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	return component, project
}

func typeFormalOperation(t testing.TB, contract *target.Contract) target.Operation {
	t.Helper()
	operation, ok := contract.Lookup(target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"op"}})
	if !ok {
		t.Fatal("operation unavailable")
	}
	return operation
}

func typeFormalCall(t testing.TB, project *linkproject.Component, index int) linkproject.Application {
	t.Helper()
	call, ok := project.Applications().Calls().At(index)
	if !ok {
		t.Fatalf("Call application %d unavailable", index)
	}
	return call
}

func callTerm(t testing.TB, project *linkproject.Component, application linkproject.Application) keyspace.Term {
	t.Helper()
	_, term, ok := project.Applications().Call(application)
	if !ok {
		t.Fatal("Call term unavailable")
	}
	return term
}
