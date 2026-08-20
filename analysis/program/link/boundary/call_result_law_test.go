package boundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func callResultLawContract(t testing.TB, outcomes, results int) *contract.Contract {
	t.Helper()
	fixed := make([]schematype.Type, results)
	for index := range fixed {
		fixed[index] = neutralTypes(t, typ.Any)[0]
	}
	outcomeRows := make([]vocabulary.OutcomeSpec, outcomes)
	for index := range outcomeRows {
		kind := flowkind.OutcomeNormal
		if index%2 == 1 {
			kind = flowkind.OutcomeThrow
		}
		outcomeRows[index] = vocabulary.OutcomeSpec{Kind: kind, Values: vocabulary.ValuesSpec{Fixed: fixed, Tail: vocabulary.ValuesClosed}}
	}
	sealed, err := compiler.Seal(&declaration.Spec{
		Semantics: domaincontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"op"}}},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: outcomeRows,
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func callResultLawMounted(t testing.TB, source string, outcomes, results int) (*Component, *contract.Contract, identity.ContentID, identity.ContentID, vocabulary.Operation) {
	t.Helper()
	p := typeFormalProgram(t, source)
	target := callResultLawContract(t, outcomes, results)
	component, project := typeFormalBoundaryForContract(t, p, target)
	application, applicationOK := project.Applications().Calls().At(0)
	if !applicationOK {
		t.Fatal("call-result law has no ordinary Call application")
	}
	_, module, callID, mountedOK := project.Applications().Calls().MountedIdentity(application)
	if !mountedOK {
		t.Fatal("call-result law mounted Call identity")
	}
	operation, operationOK := target.Operations.Lookup(vocabulary.BindingSpec{Namespace: vocabulary.BindingBuiltin, Member: []string{"op"}})
	if !operationOK {
		t.Fatal("call-result law operation")
	}
	return component, target, module, callID, operation
}

func TestBoundaryCallResultMapsEverySelectedOpenResultWithoutOutcomeProduct(t *testing.T) {
	component, target, module, callID, operation := callResultLawMounted(t, `local first, second = op()`, 2, 2)
	if _, ok := component.CallResultRelationID(); !ok {
		t.Fatal("call-result relation identity unavailable")
	}
	seen := make(map[identity.ContentID]struct{})
	for outcome := 0; outcome < 2; outcome++ {
		for resultIndex := 0; resultIndex < 2; resultIndex++ {
			outcomeResult, outcomeResultOK := target.OutcomeResultID(operation, outcome, resultIndex)
			if !outcomeResultOK {
				t.Fatalf("outcome/result identity %d/%d", outcome, resultIndex)
			}
			result, resultOK := component.Calls().CallResult(module, callID, outcomeResult)
			if !resultOK || !result.Available() {
				t.Fatalf("open Call result %d/%d unavailable", outcome, resultIndex)
			}
			values, ordinal, valuesOK := result.Values()
			valuesID, valuesIDOK := result.ValuesID()
			if !valuesOK || ordinal != uint32(resultIndex) || !valuesIDOK {
				t.Fatalf("open Call result %d/%d lost Values coordinate", outcome, resultIndex)
			}
			expected, expectedOK := component.Values().ForMountedSemantic(module, valuesID)
			if order, orderOK := component.Values().Compare(values, expected); !expectedOK || !orderOK || order != 0 {
				t.Fatalf("open Call result %d/%d Values semantic identity did not rebind", outcome, resultIndex)
			}
			if _, duplicate := seen[outcomeResult]; duplicate {
				t.Fatalf("outcome/result identities conflated at %d/%d", outcome, resultIndex)
			}
			seen[outcomeResult] = struct{}{}
		}
	}
}

func TestBoundaryCallResultRejectsUnselectedMultiResultFixedCall(t *testing.T) {
	component, target, module, callID, operation := callResultLawMounted(t, `local first = op(), 1`, 1, 2)
	first, firstOK := target.OutcomeResultID(operation, 0, 0)
	second, secondOK := target.OutcomeResultID(operation, 0, 1)
	if !firstOK || !secondOK {
		t.Fatal("fixed Call result identities")
	}
	result, resultOK := component.Calls().CallResult(module, callID, first)
	if !resultOK || result.Form() != programschema.CallResultValue {
		t.Fatal("fixed Call result did not retain its exact Value geometry")
	}
	if _, ok := result.Value(); !ok {
		t.Fatal("fixed Call result did not resolve its Boundary Value")
	}
	if _, ok := component.Calls().CallResult(module, callID, second); ok {
		t.Fatal("fixed Call admitted an unselected second Target result")
	}
}

func TestBoundaryCallResultTruncatesBoundedTailAfterFixedPrefix(t *testing.T) {
	component, target, module, callID, operation := callResultLawMounted(t, `local first, second, third = 1, op()`, 1, 3)
	for resultIndex := 0; resultIndex < 2; resultIndex++ {
		outcomeResult, outcomeResultOK := target.OutcomeResultID(operation, 0, resultIndex)
		if !outcomeResultOK {
			t.Fatalf("bounded tail result identity %d", resultIndex)
		}
		result, resultOK := component.Calls().CallResult(module, callID, outcomeResult)
		if !resultOK || result.Form() != programschema.CallResultValues {
			t.Fatalf("bounded tail result %d admitted=%v/%v", resultIndex, resultOK, result.Form())
		}
	}
	third, thirdOK := target.OutcomeResultID(operation, 0, 2)
	if !thirdOK {
		t.Fatal("bounded tail third result identity")
	}
	if _, resultOK := component.Calls().CallResult(module, callID, third); resultOK {
		t.Fatal("bounded tail admitted a result beyond remaining destination slots")
	}
}

func TestBoundaryCallResultBoundsLoopControlAdjustment(t *testing.T) {
	t.Run("numeric-control-is-scalar", func(t *testing.T) {
		component, target, module, callID, operation := callResultLawMounted(t, `for index = op(), 1 do end`, 1, 2)
		first, firstOK := target.OutcomeResultID(operation, 0, 0)
		second, secondOK := target.OutcomeResultID(operation, 0, 1)
		if !firstOK || !secondOK {
			t.Fatal("numeric loop result identities")
		}
		result, resultOK := component.Calls().CallResult(module, callID, first)
		if !resultOK || result.Form() != programschema.CallResultValue {
			t.Fatalf("numeric loop scalar result unavailable: %v/%v", resultOK, result.Form())
		}
		if _, resultOK := component.Calls().CallResult(module, callID, second); resultOK {
			t.Fatal("numeric loop scalar call admitted result one")
		}
	})

	t.Run("generic-control-is-three-slots", func(t *testing.T) {
		component, target, module, callID, operation := callResultLawMounted(t, `for first, second in op() do end`, 1, 4)
		for resultIndex := 0; resultIndex < 3; resultIndex++ {
			outcomeResult, outcomeResultOK := target.OutcomeResultID(operation, 0, resultIndex)
			if !outcomeResultOK {
				t.Fatalf("generic loop result identity %d", resultIndex)
			}
			result, resultOK := component.Calls().CallResult(module, callID, outcomeResult)
			if !resultOK || result.Form() != programschema.CallResultValues {
				t.Fatalf("generic loop result %d unavailable: %v/%v", resultIndex, resultOK, result.Form())
			}
		}
		fourth, fourthOK := target.OutcomeResultID(operation, 0, 3)
		if !fourthOK {
			t.Fatal("generic loop fourth result identity")
		}
		if _, resultOK := component.Calls().CallResult(module, callID, fourth); resultOK {
			t.Fatal("generic loop admitted a result beyond its three iterator slots")
		}
	})
}
