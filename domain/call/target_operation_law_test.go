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
	algebra.targets[1].kind = targetBody
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
		t.Fatalf("none classification = %d/%d, want %d/0", gotKind, gotOperation, TargetOperationNone)
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

	// Breaking the detached-role inverse cannot invalidate an already-issued,
	// owner-fenced Target. The inverse itself still refuses the detached replay.
	algebra.roleIndex[firstRole] = 2
	if gotOperation, gotKind = algebra.ClassifyTargetOperation(first); gotKind != TargetOperationPresent || gotOperation != operation {
		t.Fatalf("owned target classification after inverse damage = %d/%d, want %d/%d", gotKind, gotOperation, TargetOperationPresent, operation)
	}
	if _, replayed := algebra.TargetForRole(firstRole); replayed {
		t.Fatal("broken detached-role inverse replayed a target")
	}
}

func TestMountedCallKeyForOccurrenceFencesApplicationAndOwner(t *testing.T) {
	_, plain, algebra := targetOperationLawAlgebra(t)
	module := identity.ContentID{51}
	occurrence := identity.ContentID{52}
	application := identity.ContentID{53}
	algebra = targetOperationLawCopy(algebra, plain)
	algebra.mounts = []mountRow{{moduleID: module, programID: identity.ContentID{59}, loaderSeedID: identity.ContentID{55}}}
	algebra.mountModuleIndex = map[identity.ContentID]uint32{module: 1}
	algebra.mountedCalls = []mountedCallRow{{callID: occurrence, calleeValueID: identity.ContentID{54}, mount: 1, applicationKey: 1}}
	algebra.mountedCallIndex = map[identity.ContentID]uint32{application: 1}
	algebra.mountedCallOccurrenceIndex = map[mountedCallOccurrenceRef]uint32{{moduleID: module, callID: occurrence}: 1}
	algebra.keys = []keyRow{{kind: keyApplication, id: application}}
	algebra.keyIndex = map[identity.ContentID]uint32{application: 1}

	mounted, key, ok := algebra.MountedCallKeyForOccurrence(module, occurrence)
	if !ok || !mounted.Valid() || !key.Valid() || !key.IsApplication() {
		t.Fatal("valid occurrence did not produce an owner-fenced Call key")
	}
	if got, gotOK := key.ApplicationID(); !gotOK || got != application {
		t.Fatalf("application key = %v/%t, want %v/true", got, gotOK, application)
	}
	if ordinal, ordinalOK := algebra.MountedCallOrdinal(mounted); !ordinalOK || ordinal != 0 {
		t.Fatalf("mounted ordinal = %d/%t, want 0/true", ordinal, ordinalOK)
	}
	gotApplication, gotCall, gotModule, gotCallee, gotLoader, identityOK := algebra.MountedCallIdentity(mounted)
	if !identityOK || gotApplication != application || gotCall != occurrence || gotModule != module || gotCallee != (identity.ContentID{54}) || gotLoader != (identity.ContentID{55}) {
		t.Fatalf("mounted identity = %v/%v/%v/%v/%v/%t", gotApplication, gotCall, gotModule, gotCallee, gotLoader, identityOK)
	}
	if _, _, ok := algebra.MountedCallKeyForOccurrence(identity.ContentID{56}, occurrence); ok {
		t.Fatal("foreign module crossed mounted occurrence fence")
	}
	if _, _, ok := algebra.MountedCallKeyForOccurrence(module, identity.ContentID{57}); ok {
		t.Fatal("foreign occurrence crossed mounted occurrence fence")
	}
	if _, ok := algebra.MountedCallOrdinal(MountedCall{owner: algebra, slot: 2}); ok {
		t.Fatal("forged mounted coordinate crossed ordinal fence")
	}
	algebra.mountedCalls[0].mount = 2
	if _, _, _, _, _, ok := algebra.MountedCallIdentity(mounted); ok {
		t.Fatal("mounted call with a nonexistent canonical mount authenticated")
	}
	algebra.mountedCalls[0].mount = 1

	algebra.mountedCalls[0].applicationKey = 0
	if _, _, _, _, _, ok := algebra.MountedCallIdentity(mounted); ok {
		t.Fatal("mounted call with a zero application key authenticated")
	}
	algebra.mountedCalls[0].applicationKey = 2
	if _, ok := algebra.KeyForMountedCall(mounted); ok {
		t.Fatal("mounted call with an out-of-range application key authenticated")
	}
	algebra.keys = append(algebra.keys, keyRow{kind: keyCallback, id: identity.ContentID{58}})
	if _, ok := algebra.MountedCallForApplication(application); ok {
		t.Fatal("mounted call pointing at a callback key authenticated as an application")
	}
	algebra.mountedCalls[0].applicationKey = 1
	algebra.mountedCallIndex[application] = 2
	if _, ok := algebra.MountedCallForApplication(application); ok {
		t.Fatal("corrupted mounted-call inverse authenticated")
	}
	algebra.mountedCallIndex[application] = 1

	foreign := *algebra
	if _, _, ok := foreign.MountedCallKeyForOccurrence(module, occurrence); !ok {
		t.Fatal("equivalent copied owner should independently authenticate its own rows")
	}
	if _, _, ok := algebra.MountedCallKeyForOccurrence(module, occurrence); !ok {
		t.Fatal("original owner was affected by foreign copy")
	}
}

func TestBodyProjectionUsesCanonicalMountRow(t *testing.T) {
	_, plain, algebra := targetOperationLawAlgebra(t)
	algebra = targetOperationLawCopy(algebra, plain)
	module := identity.ContentID{61}
	program := identity.ContentID{62}
	bodyPath := identity.ContentID{63}
	role, roleOK := newTargetRoleID(TargetRoleBody, identity.ContentID{64})
	if !roleOK {
		t.Fatal("body role")
	}
	algebra.mounts = []mountRow{{moduleID: module, programID: program, loaderSeedID: identity.ContentID{65}}}
	algebra.targets[1] = targetRow{kind: targetBody, mount: 1, bodyPath: bodyPath, role: role}
	target, targetOK := algebra.targetForSelector(2)
	body, bodyOK := target.Body()
	gotModule, moduleOK := body.ModuleKey()
	gotProgram, programOK := body.ProgramID()
	gotPath, pathOK := body.BodyPath()
	if !targetOK || !bodyOK || !moduleOK || !programOK || !pathOK || gotModule != module || gotProgram != program || gotPath != bodyPath {
		t.Fatalf("body projection = %v/%v/%v (%t/%t/%t/%t/%t)", gotModule, gotProgram, gotPath, targetOK, bodyOK, moduleOK, programOK, pathOK)
	}
	algebra.targets[1].mount = 2
	if body.Valid() {
		t.Fatal("body with nonexistent canonical mount remained valid")
	}
	algebra.targets[1].mount = 1
	algebra.mounts[0].programID = identity.ContentID{}
	if body.Valid() {
		t.Fatal("body with invalid canonical program identity remained valid")
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
	copy.seedIndex = make(map[identity.ContentID]selector, len(source.seedIndex))
	for key, selector := range source.seedIndex {
		copy.seedIndex[key] = selector
	}
	copy.roleIndex = make(map[TargetRoleID]selector, len(source.roleIndex))
	for role, selector := range source.roleIndex {
		copy.roleIndex[role] = selector
	}
	_ = plain
	return &copy
}
