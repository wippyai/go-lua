package call

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
)

func TestTargetOperationClassificationIsCanonicalAndClosed(t *testing.T) {
	operation, plain, algebra := targetOperationLawAlgebra(t)
	firstRole, firstRoleOK := newTargetRoleID(TargetRoleSeed, identity.ContentID{41})
	secondRole, secondRoleOK := newTargetRoleID(TargetRoleBody, identity.ContentID{42})
	if !firstRoleOK || !secondRoleOK {
		t.Fatal("target role identities")
	}
	algebra.targets[0].role = firstRole
	algebra.targets[1].key.kind = targetBody
	algebra.targets[1].role = secondRole
	algebra.roleIndex = map[TargetRoleID]selector{firstRole: 1, secondRole: 2}

	first, firstOK := algebra.targetForSelector(1)
	second, secondOK := algebra.targetForSelector(2)
	if !firstOK || !secondOK {
		t.Fatal("target capabilities")
	}
	gotOperation, gotKind := algebra.ClassifyTargetOperation(first)
	if gotKind != TargetOperationPresent || gotOperation != operation {
		t.Fatalf("present classification = %d/%d, want %d/%d", gotKind, gotOperation, TargetOperationPresent, operation)
	}
	gotOperation, gotKind = algebra.ClassifyTargetOperation(second)
	if gotKind != TargetOperationNone || gotOperation != 0 {
		t.Fatalf("none classification = %d/%d, want %d/0", gotKind, gotOperation)
	}
	if _, gotKind = algebra.ClassifyTargetOperation(Target{}); gotKind != TargetOperationInvalid {
		t.Fatalf("zero target classification = %d, want invalid", gotKind)
	}

	foreign := targetOperationLawCopy(algebra, plain)
	foreignTarget, foreignOK := foreign.targetForSelector(1)
	if !foreignOK {
		t.Fatal("foreign target capability")
	}
	if _, gotKind = algebra.ClassifyTargetOperation(foreignTarget); gotKind != TargetOperationInvalid {
		t.Fatalf("foreign target classification = %d, want invalid", gotKind)
	}

	// Break only the canonical role inverse. The target remains locally valid,
	// but replay must refuse to treat a mismatched role row as an operation.
	algebra.roleIndex[firstRole] = 2
	if _, gotKind = algebra.ClassifyTargetOperation(first); gotKind != TargetOperationInvalid {
		t.Fatalf("broken role round-trip classification = %d, want invalid", gotKind)
	}
}

func TestMountedCallKeyForOccurrenceFencesApplicationAndOwner(t *testing.T) {
	operation, plain, algebra := targetOperationLawAlgebra(t)
	module := identity.ContentID{51}
	occurrence := identity.ContentID{52}
	application := identity.ContentID{53}
	algebra = targetOperationLawCopy(algebra, plain)
	algebra.mountModules = []identity.ContentID{module}
	algebra.mountModuleIndex = map[identity.ContentID]uint32{module: 1}
	algebra.mountedCalls = []mountedCallRow{{applicationID: application, callID: occurrence, moduleID: module, calleeValueID: identity.ContentID{54}, loaderSeedID: identity.ContentID{55}}}
	algebra.mountedCallIndex = map[identity.ContentID]uint32{application: 1}
	algebra.mountedCallOccurrenceIndex = map[mountedCallOccurrenceRef]uint32{{moduleID: module, callID: occurrence}: 1}
	algebra.keys = []keyRow{{kind: keyApplication, applicationID: application, id: application, operation: operation}}
	algebra.keyIndex = map[identity.ContentID]uint32{application: 1}

	mounted, key, ok := algebra.MountedCallKeyForOccurrence(module, occurrence)
	if !ok || !mounted.Valid() || !key.Valid() || !key.IsApplication() {
		t.Fatal("valid occurrence did not produce an owner-fenced Call key")
	}
	if got, gotOK := key.ApplicationID(); !gotOK || got != application {
		t.Fatalf("application key = %v/%t, want %v/true", got, gotOK, application)
	}
	if _, _, ok := algebra.MountedCallKeyForOccurrence(identity.ContentID{56}, occurrence); ok {
		t.Fatal("foreign module crossed mounted occurrence fence")
	}
	if _, _, ok := algebra.MountedCallKeyForOccurrence(module, identity.ContentID{57}); ok {
		t.Fatal("foreign occurrence crossed mounted occurrence fence")
	}

	foreign := *algebra
	if _, _, ok := foreign.MountedCallKeyForOccurrence(module, occurrence); !ok {
		t.Fatal("equivalent copied owner should independently authenticate its own rows")
	}
	if _, _, ok := algebra.MountedCallKeyForOccurrence(module, occurrence); !ok {
		t.Fatal("original owner was affected by foreign copy")
	}
}

func targetOperationLawAlgebra(t *testing.T) (vocabulary.Operation, vocabulary.Operation, *Algebra) {
	t.Helper()
	anyType, anyOK := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !anyOK {
		t.Fatal("any type declaration")
	}
	contract, err := compiler.Seal(&declaration.Spec{
		Semantics: domaincontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{
			{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"classify-op"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{anyType}, Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
			{Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"classify-plain"}}}, Input: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}, Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}}, Effects: vocabulary.RowSpec{Tail: vocabulary.RowClosed}},
		},
	})
	if err != nil || contract == nil {
		t.Fatalf("seal classification contract: %v", err)
	}
	program, lowerErr := lower.Lower(lower.Source{Name: "target_operation_law.lua", Text: []byte("return 1")})
	if lowerErr != nil || program == nil {
		t.Fatalf("lower classification module: %v", lowerErr)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "target-operation-law", Program: program}}})
	if err != nil || linked == nil {
		t.Fatalf("link classification module: %v", err)
	}
	operation, operationOK := contract.Operations.OperationAt(0)
	plain, plainOK := contract.Operations.OperationAt(1)
	if !operationOK || !plainOK {
		t.Fatal("classification operation handles")
	}
	return operation, plain, behaviorTestAlgebra(contract, linked.OwnerCapability(), operation, plain)
}

func targetOperationLawCopy(source *Algebra, plain vocabulary.Operation) *Algebra {
	copy := *source
	copy.targets = append([]targetRow(nil), source.targets...)
	copy.targetIndex = make(map[targetKey]selector, len(source.targetIndex))
	for key, selector := range source.targetIndex {
		copy.targetIndex[key] = selector
	}
	copy.roleIndex = make(map[TargetRoleID]selector, len(source.roleIndex))
	for role, selector := range source.roleIndex {
		copy.roleIndex[role] = selector
	}
	_ = plain
	return &copy
}
