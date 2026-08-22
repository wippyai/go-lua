package resultalias

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestTargetAliasPlanCarriesExactlyTheDeclaredResultZeroAliases states what
// the bind-time plan may contain. The plan is the whole of this rule's Target
// reading: an operation reaches the hot path only through it, so an operation
// the plan admits without a declared result-zero ValueFormal alias would make
// the transfer alias a call result to an input the provider never tied it to,
// and an operation the plan drops silences a declared alias.
func TestTargetAliasPlanCarriesExactlyTheDeclaredResultZeroAliases(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	plan, planOK := newTargetAliasPlan(contract)
	if !planOK {
		t.Fatal("the sealed standard Target produced no alias plan")
	}

	declared := make(map[vocabulary.Operation]map[uint32]bool)
	for index := 0; index < contract.Operations.OperationCount(); index++ {
		operation, operationOK := contract.Operations.OperationAt(index)
		if !operationOK || operation == 0 {
			t.Fatalf("target operation %d", index)
		}
		for outcome := 0; outcome < contract.Operations.OutcomeCount(operation); outcome++ {
			for alias := 0; alias < contract.Operations.ResultAliasCount(operation, outcome); alias++ {
				result, kind, source, aliasOK := contract.Operations.ResultAliasAt(operation, outcome, alias)
				if !aliasOK {
					t.Fatalf("operation %d outcome %d alias %d", operation, outcome, alias)
				}
				if result != 0 {
					continue
				}
				if kind != vocabulary.InputSourceValueFormal {
					t.Fatalf("operation %d declares a result-zero alias from source kind %v; the compiler admits only ValueFormal", operation, kind)
				}
				if declared[operation] == nil {
					declared[operation] = make(map[uint32]bool)
				}
				declared[operation][source] = true
			}
		}
	}
	if len(declared) == 0 {
		t.Fatal("the sealed Target declares no result-zero alias at all")
	}

	setmetatable, setmetatableOK := contract.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingBuiltin, Member: []string{"setmetatable"},
	})
	if !setmetatableOK || !declared[setmetatable][0] {
		t.Fatalf("setmetatable = %d/%t; the subject operation must declare a result-zero alias of its first input", setmetatable, setmetatableOK)
	}

	if len(plan.byOperation) != len(declared) {
		t.Fatalf("plan carries %d operations, the Target declares %d", len(plan.byOperation), len(declared))
	}
	for operation, sources := range declared {
		entry, planned := plan.byOperation[operation]
		if !planned {
			t.Fatalf("operation %d declares a result-zero alias the plan drops", operation)
		}
		if len(entry.sources) != len(sources) {
			t.Fatalf("operation %d plan sources = %v, want the %d declared input formals", operation, entry.sources, len(sources))
		}
		for index, source := range entry.sources {
			if !sources[source] {
				t.Fatalf("operation %d plan names input formal %d, which no declared alias sources", operation, source)
			}
			if index != 0 && entry.sources[index-1] >= source {
				t.Fatalf("operation %d plan sources %v are not the strictly ordered declared set", operation, entry.sources)
			}
			if uint64(source) >= uint64(contract.Operations.InputFormalCount(operation)) {
				t.Fatalf("operation %d plan names input formal %d beyond its %d declared formals", operation, source, contract.Operations.InputFormalCount(operation))
			}
		}
	}

	coroutineCreate, coroutineOK := contract.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule, Owner: []string{"coroutine"}, Member: []string{"create"},
	})
	if !coroutineOK {
		t.Fatal("coroutine.create is not a sealed operation")
	}
	if _, planned := plan.byOperation[coroutineCreate]; planned {
		t.Fatal("an operation that declares no result alias reached the plan; an unaliased member must contribute nothing")
	}
}

// TestTargetAliasPlanRefusesAnAbsentContract keeps the bind-time read
// fail-closed: no Target is not an empty plan.
func TestTargetAliasPlanRefusesAnAbsentContract(t *testing.T) {
	if _, planOK := newTargetAliasPlan(nil); planOK {
		t.Fatal("an absent Target produced a plan")
	}
}
