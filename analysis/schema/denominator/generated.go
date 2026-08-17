// Code generated from catalog.schema; DO NOT EDIT.

package denominator

import "github.com/wippyai/go-lua/analysis/schema"

var generatedRelationEntries = []*RelationEntry{
	{key: schema.Key("ProgramSourceProvenance@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceProvenance@-")), owner: RelationOwnerProgramSource, form: RelationFormAuthored},
	{key: schema.Key("ProgramSourceOrder@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceOrder@-")), owner: RelationOwnerProgramSource, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceProvenance@-"))}},
	{key: schema.Key("ProgramSourceKey@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceKey@-")), owner: RelationOwnerProgramSource, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceProvenance@-"))}},
	{key: schema.Key("ProgramSourceExactKey@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceExactKey@-")), owner: RelationOwnerProgramSource, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceKey@-"))}},
	{key: schema.Key("ProgramSourceControlFault@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceControlFault@-")), owner: RelationOwnerProgramSource, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceProvenance@-"))}},
	{key: schema.Key("ProgramFlowLiterals@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowLiterals@-")), owner: RelationOwnerProgramSource, form: RelationFormAuthored},
	{key: schema.Key("ProgramFlowValues@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@-")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence"))}},
	{key: schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence")), owner: RelationOwnerProgramFlow, form: RelationFormVirtualPredicate, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowLiterals@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageRead")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageVararg")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowConstructors@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowFunction@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowCall@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowClaim@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowTypeValue@-"))}},
	{key: schema.Key("ProgramFlowLens@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowLens@-")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceKey@-"))}},
	{key: schema.Key("ProgramFlowStorage@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@-")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored},
	{key: schema.Key("ProgramFlowStorage@ProgramFlowStorageCell"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageCell")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored},
	{key: schema.Key("ProgramFlowStorage@ProgramFlowStorageGlobal"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageGlobal")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageCell")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceKey@-"))}},
	{key: schema.Key("ProgramFlowStorage@ProgramFlowStorageRead"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageRead")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageCell")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowLens@-"))}},
	{key: schema.Key("ProgramFlowStorage@ProgramFlowStorageAssign"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageAssign")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@-"))}},
	{key: schema.Key("ProgramFlowStorage@ProgramFlowStorageWrite"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageWrite")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageAssign")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageCell")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowLens@-"))}},
	{key: schema.Key("ProgramFlowStorage@ProgramFlowStorageVararg"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageVararg")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageCell"))}},
	{key: schema.Key("ProgramFlowStorage@ProgramFlowStorageBind"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageBind")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageCell")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@-"))}},
	{key: schema.Key("ProgramFlowConstructors@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowConstructors@-")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceKey@-"))}},
	{key: schema.Key("ProgramFlowConstructors@ProgramFlowConstructorField"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowConstructors@ProgramFlowConstructorField")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowConstructors@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowLens@-"))}},
	{key: schema.Key("ProgramFlowOperators@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence"))}},
	{key: schema.Key("ProgramFlowOperators@ProgramFlowUnaryNumeric"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowUnaryNumeric")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-"))}},
	{key: schema.Key("ProgramFlowOperators@ProgramFlowLength"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowLength")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-"))}},
	{key: schema.Key("ProgramFlowOperators@ProgramFlowArithmetic"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowArithmetic")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-"))}},
	{key: schema.Key("ProgramFlowOperators@ProgramFlowBitwise"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowBitwise")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-"))}},
	{key: schema.Key("ProgramFlowOperators@ProgramFlowConcat"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowConcat")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-"))}},
	{key: schema.Key("ProgramFlowOperators@ProgramFlowEquality"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowEquality")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-"))}},
	{key: schema.Key("ProgramFlowOperators@ProgramFlowOrder"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowOrder")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-"))}},
	{key: schema.Key("ProgramFlowOperators@ProgramFlowIndexGet"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowIndexGet")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageRead"))}},
	{key: schema.Key("ProgramFlowOperators@ProgramFlowIndexSet"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowIndexSet")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageWrite"))}},
	{key: schema.Key("ProgramFlowFunction@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowFunction@-")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored},
	{key: schema.Key("ProgramFlowFunction@ProgramFlowFunctionCapture"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowFunction@ProgramFlowFunctionCapture")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowFunction@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageCell"))}},
	{key: schema.Key("ProgramFlowCall@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowCall@-")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@-"))}},
	{key: schema.Key("ProgramFlowCall@ProgramFlowDirectCallBinding"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowCall@ProgramFlowDirectCallBinding")), owner: RelationOwnerProgramFlow, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowCall@-"))}},
	{key: schema.Key("ProgramFlowControl@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowControl@-")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored},
	{key: schema.Key("ProgramFlowControl@ProgramFlowGenericFor"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowControl@ProgramFlowGenericFor")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowControl@-"))}},
	{key: schema.Key("ProgramFlowClaim@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowClaim@-")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence"))}},
	{key: schema.Key("ProgramFlowTypeValue@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowTypeValue@-")), owner: RelationOwnerProgramFlow, form: RelationFormAuthored},
	{key: schema.Key("ProgramFlowOutcome@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOutcome@-")), owner: RelationOwnerProgramFlow, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowCall@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowControl@-"))}},
	{key: schema.Key("ProgramFlowTransfer@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowTransfer@-")), owner: RelationOwnerProgramFlow, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowCall@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOutcome@-"))}},
	{key: schema.Key("ProgramFlowBody@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowBody@-")), owner: RelationOwnerProgramSource, form: RelationFormAuthored},
	{key: schema.Key("ProgramFlowBody@ProgramFlowBodyRoots"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowBody@ProgramFlowBodyRoots")), owner: RelationOwnerProgramSource, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowBody@-"))}},
	{key: schema.Key("ProgramStatic@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@-")), owner: RelationOwnerProgramStatic, form: RelationFormAuthored},
	{key: schema.Key("ProgramStatic@ProgramStaticFunctionContract"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticFunctionContract")), owner: RelationOwnerProgramStatic, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowFunction@-"))}},
	{key: schema.Key("ProgramStatic@ProgramStaticCallTypeArguments"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticCallTypeArguments")), owner: RelationOwnerProgramStatic, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowCall@-"))}},
	{key: schema.Key("ProgramStatic@ProgramStaticCellDeclaredType"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticCellDeclaredType")), owner: RelationOwnerProgramStatic, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageCell"))}},
	{key: schema.Key("ProgramStatic@ProgramStaticClaimTarget"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticClaimTarget")), owner: RelationOwnerProgramStatic, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowClaim@-"))}},
	{key: schema.Key("ProgramStatic@ProgramStaticTypeValueTarget"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticTypeValueTarget")), owner: RelationOwnerProgramStatic, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowTypeValue@-"))}},
	{key: schema.Key("ProgramStatic@ProgramStaticTypeof"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticTypeof")), owner: RelationOwnerProgramStatic, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticTypeRef"))}},
	{key: schema.Key("ProgramStatic@ProgramStaticAnnotation"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticAnnotation")), owner: RelationOwnerProgramStatic, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticTypeRef"))}},
	{key: schema.Key("ProgramStatic@ProgramStaticPublication"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticPublication")), owner: RelationOwnerProgramStatic, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageAssign")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticTypeRef"))}},
	{key: schema.Key("ProgramStatic@ProgramStaticTypeRef"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticTypeRef")), owner: RelationOwnerProgramStatic, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@-"))}},
	{key: schema.Key("ProgramModuleImport@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramModuleImport@-")), owner: RelationOwnerProgramModule, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowCall@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageCell"))}},
	{key: schema.Key("ProgramModuleImport@ProgramModuleRequest"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramModuleImport@ProgramModuleRequest")), owner: RelationOwnerProgramModule, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramModuleImport@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowLiterals@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceExactKey@-"))}},
	{key: schema.Key("ProgramModuleEntry@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramModuleEntry@-")), owner: RelationOwnerProgramModule, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowControl@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowBody@ProgramFlowBodyRoots"))}},
	{key: schema.Key("ProgramModuleEntry@ProgramModuleEntryRootCell"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramModuleEntry@ProgramModuleEntryRootCell")), owner: RelationOwnerProgramModule, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramModuleEntry@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageCell"))}},
	{key: schema.Key("ProgramModuleEntry@ProgramModuleEntryMember"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramModuleEntry@ProgramModuleEntryMember")), owner: RelationOwnerProgramModule, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramModuleEntry@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowConstructors@ProgramFlowConstructorField"))}},
	{key: schema.Key("ProgramModuleEntry@ProgramModuleEntryRootFunction"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramModuleEntry@ProgramModuleEntryRootFunction")), owner: RelationOwnerProgramModule, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramModuleEntry@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowFunction@-"))}},
	{key: schema.Key("TargetContract@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetContract@-")), owner: RelationOwnerTarget, form: RelationFormAuthored},
	{key: schema.Key("TargetOperation@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetContract@-"))}},
	{key: schema.Key("TargetOperation@TargetABI"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-"))}},
	{key: schema.Key("TargetOperation@TargetSubedge"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSubedge")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-"))}},
	{key: schema.Key("TargetOperation@TargetCallback"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallback")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-"))}},
	{key: schema.Key("TargetOperation@TargetBinding"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetBinding")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-"))}},
	{key: schema.Key("TargetOperation@TargetResume"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetResume")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-"))}},
	{key: schema.Key("TargetOperation@TargetSpawn"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSpawn")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallback"))}},
	{key: schema.Key("TargetOperation@TargetOpaque"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOpaque")), owner: RelationOwnerTarget, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-"))}},
	{key: schema.Key("TargetOperation@TargetOperationEffect"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOperationEffect")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI"))}},
	{key: schema.Key("TargetOperation@TargetCallbackEffect"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallbackEffect")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallback")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI"))}},
	{key: schema.Key("TargetOperation@TargetCallbackRelease"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallbackRelease")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallback")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome"))}},
	{key: schema.Key("TargetOperation@TargetOutcome"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI"))}},
	{key: schema.Key("TargetOperation@TargetTransfer"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetTransfer")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome"))}},
	{key: schema.Key("TargetOperation@TargetTransferOutcome"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetTransferOutcome")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetTransfer")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome"))}},
	{key: schema.Key("TargetOperation@TargetSuspension"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSuspension")), owner: RelationOwnerTarget, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome"))}},
	{key: schema.Key("TargetOperation@TargetResumeOutcome"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetResumeOutcome")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetResume")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome"))}},
	{key: schema.Key("TargetOperation@TargetSpawnSibling"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSpawnSibling")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSpawn"))}},
	{key: schema.Key("TargetOperation@TargetSubedgeArgumentOrigin"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSubedgeArgumentOrigin")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSubedge")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI"))}},
	{key: schema.Key("TargetOperation@TargetCallbackResult"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallbackResult")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallback"))}},
	{key: schema.Key("TargetOperation@TargetResultAlias"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetResultAlias")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI"))}},
	{key: schema.Key("TargetOperation@TargetProduced"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetProduced")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome"))}},
	{key: schema.Key("TargetOperation@TargetProducedCapture"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetProducedCapture")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetProduced")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallback"))}},
	{key: schema.Key("TargetOperation@TargetFreshResult"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetFreshResult")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome"))}},
	{key: schema.Key("TargetOperation@TargetPublicationEffect"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetPublicationEffect")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallback")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOperationEffect")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallbackEffect"))}},
	{key: schema.Key("TargetOperation@TargetSubedgeRelation"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSubedgeRelation")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSubedge")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOperationEffect")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetBinding")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome"))}},
	{key: schema.Key("TargetProtocol@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@-")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetContract@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI"))}},
	{key: schema.Key("TargetProtocol@TargetProtocolState"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolState")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@-"))}},
	{key: schema.Key("TargetProtocol@TargetProtocolAcquisition"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolAcquisition")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolState")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome"))}},
	{key: schema.Key("TargetProtocol@TargetProtocolTransition"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolTransition")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolState")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI"))}},
	{key: schema.Key("TargetProtocol@TargetProtocolTransitionOutcome"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolTransitionOutcome")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolTransition")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolState")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome"))}},
	{key: schema.Key("TargetProtocol@TargetProtocolEscape"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolEscape")), owner: RelationOwnerTarget, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI"))}},
	{key: schema.Key("TargetProtocol@TargetProtocolCallbackHolder"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolCallbackHolder")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallback")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI"))}},
	{key: schema.Key("TargetBoot@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetBoot@-")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetContract@-"))}},
	{key: schema.Key("TargetBoot@TargetBootEntry"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetBoot@TargetBootEntry")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetBoot@-"))}},
	{key: schema.Key("TargetBoot@TargetBootMetatableAttachment"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetBoot@TargetBootMetatableAttachment")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetBoot@-"))}},
	{key: schema.Key("TargetBoot@TargetBootBinding"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetBoot@TargetBootBinding")), owner: RelationOwnerTarget, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetBoot@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetBoot@TargetBootEntry"))}},
	{key: schema.Key("LinkProjectShardMount@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkProjectShardMount@-")), owner: RelationOwnerLinkProject, form: RelationFormAuthored},
	{key: schema.Key("LinkProjectBaseApplication@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkProjectBaseApplication@-")), owner: RelationOwnerLinkProject, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkProjectShardMount@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowCall@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowUnaryNumeric")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowLength")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowArithmetic")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowBitwise")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowConcat")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowEquality")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowOrder")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowIndexGet")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOperators@ProgramFlowIndexSet")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowControl@ProgramFlowGenericFor"))}},
	{key: schema.Key("LinkBoundary@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkBoundary@-")), owner: RelationOwnerLinkBoundary, form: RelationFormVirtualPredicate, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkProjectBaseApplication@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSubedge")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallback")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetBinding")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetResume")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSpawn")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOpaque")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOperationEffect")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallbackEffect")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallbackRelease")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetOutcome")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetTransfer")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetTransferOutcome")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSuspension")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetResumeOutcome")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSpawnSibling")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSubedgeArgumentOrigin")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetCallbackResult")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetResultAlias")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetProduced")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetProducedCapture")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetFreshResult")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetPublicationEffect")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetSubedgeRelation")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolState")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolAcquisition")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolTransition")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolTransitionOutcome")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolEscape")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetProtocol@TargetProtocolCallbackHolder"))}},
	{key: schema.Key("LinkModule@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@-")), owner: RelationOwnerLinkModule, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkProjectShardMount@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramModuleImport@-"))}},
	{key: schema.Key("LinkModule@LinkModuleCache"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleCache")), owner: RelationOwnerLinkModule, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@-"))}},
	{key: schema.Key("LinkModule@LinkModuleRepresentative"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleRepresentative")), owner: RelationOwnerLinkModule, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleCache"))}},
	{key: schema.Key("LinkModule@LinkModuleTransport"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleTransport")), owner: RelationOwnerLinkModule, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkBoundary@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleCache")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleRepresentative")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleAnalysisRoot"))}},
	{key: schema.Key("LinkModule@LinkModuleAnalysisRoot"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleAnalysisRoot")), owner: RelationOwnerLinkModule, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleCache"))}},
	{key: schema.Key("LinkModule@LinkModuleInitGeneration"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleInitGeneration")), owner: RelationOwnerLinkModule, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleTransport")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowBody@ProgramFlowBodyRoots"))}},
	{key: schema.Key("LinkModule@LinkModuleInitOutcome"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleInitOutcome")), owner: RelationOwnerLinkModule, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleInitGeneration")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowOutcome@-"))}},
	{key: schema.Key("LinkModule@LinkModuleInitTerminal"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleInitTerminal")), owner: RelationOwnerLinkModule, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleInitOutcome"))}},
	{key: schema.Key("LinkStatic@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkStatic@-")), owner: RelationOwnerLinkStatic, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramStatic@ProgramStaticPublication"))}},
	{key: schema.Key("LinkHost@-"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkHost@-")), owner: RelationOwnerLinkHost, form: RelationFormAuthored, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetContract@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetBinding")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetABI"))}},
	{key: schema.Key("LinkHost@LinkHostExposure"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkHost@LinkHostExposure")), owner: RelationOwnerLinkHost, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkHost@-"))}},
	{key: schema.Key("LinkHost@LinkHostBoot"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkHost@LinkHostBoot")), owner: RelationOwnerLinkHost, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleAnalysisRoot")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleCache")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkModule@LinkModuleRepresentative")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetBoot@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowStorage@ProgramFlowStorageGlobal")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceKey@-"))}},
	{key: schema.Key("LinkHost@LinkHostMember"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkHost@LinkHostMember")), owner: RelationOwnerLinkHost, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkHost@LinkHostExposure")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramFlowLens@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("ProgramSourceKey@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetBinding"))}},
	{key: schema.Key("LinkHost@LinkHostEndpointTarget"), id: schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkHost@LinkHostEndpointTarget")), owner: RelationOwnerLinkHost, form: RelationFormSealDerived, parents: []schema.EntryID{schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("LinkHost@-")), schema.NewEntryID(schema.SurfaceKindDenominator, schema.Key("TargetOperation@TargetBinding"))}},
}

// GeneratedRelationEntries returns the generated relation declarations in
// catalog order. The slice is detached; declaration values remain immutable
// outside this package.
func GeneratedRelationEntries() []*RelationEntry {
	return append([]*RelationEntry(nil), generatedRelationEntries...)
}

// GeneratedRelationByKey resolves one generated relation declaration by its
// authored schema key.
func GeneratedRelationByKey(key schema.Key) (*RelationEntry, bool) {
	switch key {
	case schema.Key("ProgramSourceProvenance@-"):
		return generatedRelationEntries[0], true
	case schema.Key("ProgramSourceOrder@-"):
		return generatedRelationEntries[1], true
	case schema.Key("ProgramSourceKey@-"):
		return generatedRelationEntries[2], true
	case schema.Key("ProgramSourceExactKey@-"):
		return generatedRelationEntries[3], true
	case schema.Key("ProgramSourceControlFault@-"):
		return generatedRelationEntries[4], true
	case schema.Key("ProgramFlowLiterals@-"):
		return generatedRelationEntries[5], true
	case schema.Key("ProgramFlowValues@-"):
		return generatedRelationEntries[6], true
	case schema.Key("ProgramFlowValues@ProgramFlowValueOccurrence"):
		return generatedRelationEntries[7], true
	case schema.Key("ProgramFlowLens@-"):
		return generatedRelationEntries[8], true
	case schema.Key("ProgramFlowStorage@-"):
		return generatedRelationEntries[9], true
	case schema.Key("ProgramFlowStorage@ProgramFlowStorageCell"):
		return generatedRelationEntries[10], true
	case schema.Key("ProgramFlowStorage@ProgramFlowStorageGlobal"):
		return generatedRelationEntries[11], true
	case schema.Key("ProgramFlowStorage@ProgramFlowStorageRead"):
		return generatedRelationEntries[12], true
	case schema.Key("ProgramFlowStorage@ProgramFlowStorageAssign"):
		return generatedRelationEntries[13], true
	case schema.Key("ProgramFlowStorage@ProgramFlowStorageWrite"):
		return generatedRelationEntries[14], true
	case schema.Key("ProgramFlowStorage@ProgramFlowStorageVararg"):
		return generatedRelationEntries[15], true
	case schema.Key("ProgramFlowStorage@ProgramFlowStorageBind"):
		return generatedRelationEntries[16], true
	case schema.Key("ProgramFlowConstructors@-"):
		return generatedRelationEntries[17], true
	case schema.Key("ProgramFlowConstructors@ProgramFlowConstructorField"):
		return generatedRelationEntries[18], true
	case schema.Key("ProgramFlowOperators@-"):
		return generatedRelationEntries[19], true
	case schema.Key("ProgramFlowOperators@ProgramFlowUnaryNumeric"):
		return generatedRelationEntries[20], true
	case schema.Key("ProgramFlowOperators@ProgramFlowLength"):
		return generatedRelationEntries[21], true
	case schema.Key("ProgramFlowOperators@ProgramFlowArithmetic"):
		return generatedRelationEntries[22], true
	case schema.Key("ProgramFlowOperators@ProgramFlowBitwise"):
		return generatedRelationEntries[23], true
	case schema.Key("ProgramFlowOperators@ProgramFlowConcat"):
		return generatedRelationEntries[24], true
	case schema.Key("ProgramFlowOperators@ProgramFlowEquality"):
		return generatedRelationEntries[25], true
	case schema.Key("ProgramFlowOperators@ProgramFlowOrder"):
		return generatedRelationEntries[26], true
	case schema.Key("ProgramFlowOperators@ProgramFlowIndexGet"):
		return generatedRelationEntries[27], true
	case schema.Key("ProgramFlowOperators@ProgramFlowIndexSet"):
		return generatedRelationEntries[28], true
	case schema.Key("ProgramFlowFunction@-"):
		return generatedRelationEntries[29], true
	case schema.Key("ProgramFlowFunction@ProgramFlowFunctionCapture"):
		return generatedRelationEntries[30], true
	case schema.Key("ProgramFlowCall@-"):
		return generatedRelationEntries[31], true
	case schema.Key("ProgramFlowCall@ProgramFlowDirectCallBinding"):
		return generatedRelationEntries[32], true
	case schema.Key("ProgramFlowControl@-"):
		return generatedRelationEntries[33], true
	case schema.Key("ProgramFlowControl@ProgramFlowGenericFor"):
		return generatedRelationEntries[34], true
	case schema.Key("ProgramFlowClaim@-"):
		return generatedRelationEntries[35], true
	case schema.Key("ProgramFlowTypeValue@-"):
		return generatedRelationEntries[36], true
	case schema.Key("ProgramFlowOutcome@-"):
		return generatedRelationEntries[37], true
	case schema.Key("ProgramFlowTransfer@-"):
		return generatedRelationEntries[38], true
	case schema.Key("ProgramFlowBody@-"):
		return generatedRelationEntries[39], true
	case schema.Key("ProgramFlowBody@ProgramFlowBodyRoots"):
		return generatedRelationEntries[40], true
	case schema.Key("ProgramStatic@-"):
		return generatedRelationEntries[41], true
	case schema.Key("ProgramStatic@ProgramStaticFunctionContract"):
		return generatedRelationEntries[42], true
	case schema.Key("ProgramStatic@ProgramStaticCallTypeArguments"):
		return generatedRelationEntries[43], true
	case schema.Key("ProgramStatic@ProgramStaticCellDeclaredType"):
		return generatedRelationEntries[44], true
	case schema.Key("ProgramStatic@ProgramStaticClaimTarget"):
		return generatedRelationEntries[45], true
	case schema.Key("ProgramStatic@ProgramStaticTypeValueTarget"):
		return generatedRelationEntries[46], true
	case schema.Key("ProgramStatic@ProgramStaticTypeof"):
		return generatedRelationEntries[47], true
	case schema.Key("ProgramStatic@ProgramStaticAnnotation"):
		return generatedRelationEntries[48], true
	case schema.Key("ProgramStatic@ProgramStaticPublication"):
		return generatedRelationEntries[49], true
	case schema.Key("ProgramStatic@ProgramStaticTypeRef"):
		return generatedRelationEntries[50], true
	case schema.Key("ProgramModuleImport@-"):
		return generatedRelationEntries[51], true
	case schema.Key("ProgramModuleImport@ProgramModuleRequest"):
		return generatedRelationEntries[52], true
	case schema.Key("ProgramModuleEntry@-"):
		return generatedRelationEntries[53], true
	case schema.Key("ProgramModuleEntry@ProgramModuleEntryRootCell"):
		return generatedRelationEntries[54], true
	case schema.Key("ProgramModuleEntry@ProgramModuleEntryMember"):
		return generatedRelationEntries[55], true
	case schema.Key("ProgramModuleEntry@ProgramModuleEntryRootFunction"):
		return generatedRelationEntries[56], true
	case schema.Key("TargetContract@-"):
		return generatedRelationEntries[57], true
	case schema.Key("TargetOperation@-"):
		return generatedRelationEntries[58], true
	case schema.Key("TargetOperation@TargetABI"):
		return generatedRelationEntries[59], true
	case schema.Key("TargetOperation@TargetSubedge"):
		return generatedRelationEntries[60], true
	case schema.Key("TargetOperation@TargetCallback"):
		return generatedRelationEntries[61], true
	case schema.Key("TargetOperation@TargetBinding"):
		return generatedRelationEntries[62], true
	case schema.Key("TargetOperation@TargetResume"):
		return generatedRelationEntries[63], true
	case schema.Key("TargetOperation@TargetSpawn"):
		return generatedRelationEntries[64], true
	case schema.Key("TargetOperation@TargetOpaque"):
		return generatedRelationEntries[65], true
	case schema.Key("TargetOperation@TargetOperationEffect"):
		return generatedRelationEntries[66], true
	case schema.Key("TargetOperation@TargetCallbackEffect"):
		return generatedRelationEntries[67], true
	case schema.Key("TargetOperation@TargetCallbackRelease"):
		return generatedRelationEntries[68], true
	case schema.Key("TargetOperation@TargetOutcome"):
		return generatedRelationEntries[69], true
	case schema.Key("TargetOperation@TargetTransfer"):
		return generatedRelationEntries[70], true
	case schema.Key("TargetOperation@TargetTransferOutcome"):
		return generatedRelationEntries[71], true
	case schema.Key("TargetOperation@TargetSuspension"):
		return generatedRelationEntries[72], true
	case schema.Key("TargetOperation@TargetResumeOutcome"):
		return generatedRelationEntries[73], true
	case schema.Key("TargetOperation@TargetSpawnSibling"):
		return generatedRelationEntries[74], true
	case schema.Key("TargetOperation@TargetSubedgeArgumentOrigin"):
		return generatedRelationEntries[75], true
	case schema.Key("TargetOperation@TargetCallbackResult"):
		return generatedRelationEntries[76], true
	case schema.Key("TargetOperation@TargetResultAlias"):
		return generatedRelationEntries[77], true
	case schema.Key("TargetOperation@TargetProduced"):
		return generatedRelationEntries[78], true
	case schema.Key("TargetOperation@TargetProducedCapture"):
		return generatedRelationEntries[79], true
	case schema.Key("TargetOperation@TargetFreshResult"):
		return generatedRelationEntries[80], true
	case schema.Key("TargetOperation@TargetPublicationEffect"):
		return generatedRelationEntries[81], true
	case schema.Key("TargetOperation@TargetSubedgeRelation"):
		return generatedRelationEntries[82], true
	case schema.Key("TargetProtocol@-"):
		return generatedRelationEntries[83], true
	case schema.Key("TargetProtocol@TargetProtocolState"):
		return generatedRelationEntries[84], true
	case schema.Key("TargetProtocol@TargetProtocolAcquisition"):
		return generatedRelationEntries[85], true
	case schema.Key("TargetProtocol@TargetProtocolTransition"):
		return generatedRelationEntries[86], true
	case schema.Key("TargetProtocol@TargetProtocolTransitionOutcome"):
		return generatedRelationEntries[87], true
	case schema.Key("TargetProtocol@TargetProtocolEscape"):
		return generatedRelationEntries[88], true
	case schema.Key("TargetProtocol@TargetProtocolCallbackHolder"):
		return generatedRelationEntries[89], true
	case schema.Key("TargetBoot@-"):
		return generatedRelationEntries[90], true
	case schema.Key("TargetBoot@TargetBootEntry"):
		return generatedRelationEntries[91], true
	case schema.Key("TargetBoot@TargetBootMetatableAttachment"):
		return generatedRelationEntries[92], true
	case schema.Key("TargetBoot@TargetBootBinding"):
		return generatedRelationEntries[93], true
	case schema.Key("LinkProjectShardMount@-"):
		return generatedRelationEntries[94], true
	case schema.Key("LinkProjectBaseApplication@-"):
		return generatedRelationEntries[95], true
	case schema.Key("LinkBoundary@-"):
		return generatedRelationEntries[96], true
	case schema.Key("LinkModule@-"):
		return generatedRelationEntries[97], true
	case schema.Key("LinkModule@LinkModuleCache"):
		return generatedRelationEntries[98], true
	case schema.Key("LinkModule@LinkModuleRepresentative"):
		return generatedRelationEntries[99], true
	case schema.Key("LinkModule@LinkModuleTransport"):
		return generatedRelationEntries[100], true
	case schema.Key("LinkModule@LinkModuleAnalysisRoot"):
		return generatedRelationEntries[101], true
	case schema.Key("LinkModule@LinkModuleInitGeneration"):
		return generatedRelationEntries[102], true
	case schema.Key("LinkModule@LinkModuleInitOutcome"):
		return generatedRelationEntries[103], true
	case schema.Key("LinkModule@LinkModuleInitTerminal"):
		return generatedRelationEntries[104], true
	case schema.Key("LinkStatic@-"):
		return generatedRelationEntries[105], true
	case schema.Key("LinkHost@-"):
		return generatedRelationEntries[106], true
	case schema.Key("LinkHost@LinkHostExposure"):
		return generatedRelationEntries[107], true
	case schema.Key("LinkHost@LinkHostBoot"):
		return generatedRelationEntries[108], true
	case schema.Key("LinkHost@LinkHostMember"):
		return generatedRelationEntries[109], true
	case schema.Key("LinkHost@LinkHostEndpointTarget"):
		return generatedRelationEntries[110], true
	default:
		return nil, false
	}
}

// GeneratedRelationID resolves one generated relation's stable denominator
// entry identity by its authored schema key.
func GeneratedRelationID(key schema.Key) (schema.EntryID, bool) {
	entry, ok := GeneratedRelationByKey(key)
	if !ok {
		return schema.EntryID{}, false
	}
	return entry.ID(), true
}

// GeneratedProgramSourceRelationIDs contains the stable denominator identities owned by ProgramSource.
type GeneratedProgramSourceRelationIDs struct {
	ProgramSourceProvenance   schema.EntryID
	ProgramSourceOrder        schema.EntryID
	ProgramSourceKey          schema.EntryID
	ProgramSourceExactKey     schema.EntryID
	ProgramSourceControlFault schema.EntryID
	ProgramFlowLiterals       schema.EntryID
	ProgramFlowBody           schema.EntryID
	ProgramFlowBodyRoots      schema.EntryID
}

func GeneratedProgramSourceIDs() GeneratedProgramSourceRelationIDs {
	return GeneratedProgramSourceRelationIDs{
		ProgramSourceProvenance:   generatedRelationEntries[0].ID(),
		ProgramSourceOrder:        generatedRelationEntries[1].ID(),
		ProgramSourceKey:          generatedRelationEntries[2].ID(),
		ProgramSourceExactKey:     generatedRelationEntries[3].ID(),
		ProgramSourceControlFault: generatedRelationEntries[4].ID(),
		ProgramFlowLiterals:       generatedRelationEntries[5].ID(),
		ProgramFlowBody:           generatedRelationEntries[39].ID(),
		ProgramFlowBodyRoots:      generatedRelationEntries[40].ID(),
	}
}

// GeneratedProgramFlowRelationIDs contains the stable denominator identities owned by ProgramFlow.
type GeneratedProgramFlowRelationIDs struct {
	ProgramFlowValues            schema.EntryID
	ProgramFlowValueOccurrence   schema.EntryID
	ProgramFlowLens              schema.EntryID
	ProgramFlowStorage           schema.EntryID
	ProgramFlowStorageCell       schema.EntryID
	ProgramFlowStorageGlobal     schema.EntryID
	ProgramFlowStorageRead       schema.EntryID
	ProgramFlowStorageAssign     schema.EntryID
	ProgramFlowStorageWrite      schema.EntryID
	ProgramFlowStorageVararg     schema.EntryID
	ProgramFlowStorageBind       schema.EntryID
	ProgramFlowConstructors      schema.EntryID
	ProgramFlowConstructorField  schema.EntryID
	ProgramFlowOperators         schema.EntryID
	ProgramFlowUnaryNumeric      schema.EntryID
	ProgramFlowLength            schema.EntryID
	ProgramFlowArithmetic        schema.EntryID
	ProgramFlowBitwise           schema.EntryID
	ProgramFlowConcat            schema.EntryID
	ProgramFlowEquality          schema.EntryID
	ProgramFlowOrder             schema.EntryID
	ProgramFlowIndexGet          schema.EntryID
	ProgramFlowIndexSet          schema.EntryID
	ProgramFlowFunction          schema.EntryID
	ProgramFlowFunctionCapture   schema.EntryID
	ProgramFlowCall              schema.EntryID
	ProgramFlowDirectCallBinding schema.EntryID
	ProgramFlowControl           schema.EntryID
	ProgramFlowGenericFor        schema.EntryID
	ProgramFlowClaim             schema.EntryID
	ProgramFlowTypeValue         schema.EntryID
	ProgramFlowOutcome           schema.EntryID
	ProgramFlowTransfer          schema.EntryID
}

func GeneratedProgramFlowIDs() GeneratedProgramFlowRelationIDs {
	return GeneratedProgramFlowRelationIDs{
		ProgramFlowValues:            generatedRelationEntries[6].ID(),
		ProgramFlowValueOccurrence:   generatedRelationEntries[7].ID(),
		ProgramFlowLens:              generatedRelationEntries[8].ID(),
		ProgramFlowStorage:           generatedRelationEntries[9].ID(),
		ProgramFlowStorageCell:       generatedRelationEntries[10].ID(),
		ProgramFlowStorageGlobal:     generatedRelationEntries[11].ID(),
		ProgramFlowStorageRead:       generatedRelationEntries[12].ID(),
		ProgramFlowStorageAssign:     generatedRelationEntries[13].ID(),
		ProgramFlowStorageWrite:      generatedRelationEntries[14].ID(),
		ProgramFlowStorageVararg:     generatedRelationEntries[15].ID(),
		ProgramFlowStorageBind:       generatedRelationEntries[16].ID(),
		ProgramFlowConstructors:      generatedRelationEntries[17].ID(),
		ProgramFlowConstructorField:  generatedRelationEntries[18].ID(),
		ProgramFlowOperators:         generatedRelationEntries[19].ID(),
		ProgramFlowUnaryNumeric:      generatedRelationEntries[20].ID(),
		ProgramFlowLength:            generatedRelationEntries[21].ID(),
		ProgramFlowArithmetic:        generatedRelationEntries[22].ID(),
		ProgramFlowBitwise:           generatedRelationEntries[23].ID(),
		ProgramFlowConcat:            generatedRelationEntries[24].ID(),
		ProgramFlowEquality:          generatedRelationEntries[25].ID(),
		ProgramFlowOrder:             generatedRelationEntries[26].ID(),
		ProgramFlowIndexGet:          generatedRelationEntries[27].ID(),
		ProgramFlowIndexSet:          generatedRelationEntries[28].ID(),
		ProgramFlowFunction:          generatedRelationEntries[29].ID(),
		ProgramFlowFunctionCapture:   generatedRelationEntries[30].ID(),
		ProgramFlowCall:              generatedRelationEntries[31].ID(),
		ProgramFlowDirectCallBinding: generatedRelationEntries[32].ID(),
		ProgramFlowControl:           generatedRelationEntries[33].ID(),
		ProgramFlowGenericFor:        generatedRelationEntries[34].ID(),
		ProgramFlowClaim:             generatedRelationEntries[35].ID(),
		ProgramFlowTypeValue:         generatedRelationEntries[36].ID(),
		ProgramFlowOutcome:           generatedRelationEntries[37].ID(),
		ProgramFlowTransfer:          generatedRelationEntries[38].ID(),
	}
}

// GeneratedProgramStaticRelationIDs contains the stable denominator identities owned by ProgramStatic.
type GeneratedProgramStaticRelationIDs struct {
	ProgramStatic                  schema.EntryID
	ProgramStaticFunctionContract  schema.EntryID
	ProgramStaticCallTypeArguments schema.EntryID
	ProgramStaticCellDeclaredType  schema.EntryID
	ProgramStaticClaimTarget       schema.EntryID
	ProgramStaticTypeValueTarget   schema.EntryID
	ProgramStaticTypeof            schema.EntryID
	ProgramStaticAnnotation        schema.EntryID
	ProgramStaticPublication       schema.EntryID
	ProgramStaticTypeRef           schema.EntryID
}

func GeneratedProgramStaticIDs() GeneratedProgramStaticRelationIDs {
	return GeneratedProgramStaticRelationIDs{
		ProgramStatic:                  generatedRelationEntries[41].ID(),
		ProgramStaticFunctionContract:  generatedRelationEntries[42].ID(),
		ProgramStaticCallTypeArguments: generatedRelationEntries[43].ID(),
		ProgramStaticCellDeclaredType:  generatedRelationEntries[44].ID(),
		ProgramStaticClaimTarget:       generatedRelationEntries[45].ID(),
		ProgramStaticTypeValueTarget:   generatedRelationEntries[46].ID(),
		ProgramStaticTypeof:            generatedRelationEntries[47].ID(),
		ProgramStaticAnnotation:        generatedRelationEntries[48].ID(),
		ProgramStaticPublication:       generatedRelationEntries[49].ID(),
		ProgramStaticTypeRef:           generatedRelationEntries[50].ID(),
	}
}

// GeneratedProgramModuleRelationIDs contains the stable denominator identities owned by ProgramModule.
type GeneratedProgramModuleRelationIDs struct {
	ProgramModuleImport            schema.EntryID
	ProgramModuleRequest           schema.EntryID
	ProgramModuleEntry             schema.EntryID
	ProgramModuleEntryRootCell     schema.EntryID
	ProgramModuleEntryMember       schema.EntryID
	ProgramModuleEntryRootFunction schema.EntryID
}

func GeneratedProgramModuleIDs() GeneratedProgramModuleRelationIDs {
	return GeneratedProgramModuleRelationIDs{
		ProgramModuleImport:            generatedRelationEntries[51].ID(),
		ProgramModuleRequest:           generatedRelationEntries[52].ID(),
		ProgramModuleEntry:             generatedRelationEntries[53].ID(),
		ProgramModuleEntryRootCell:     generatedRelationEntries[54].ID(),
		ProgramModuleEntryMember:       generatedRelationEntries[55].ID(),
		ProgramModuleEntryRootFunction: generatedRelationEntries[56].ID(),
	}
}

// GeneratedTargetRelationIDs contains the stable denominator identities owned by Target.
type GeneratedTargetRelationIDs struct {
	TargetContract                  schema.EntryID
	TargetOperation                 schema.EntryID
	TargetABI                       schema.EntryID
	TargetSubedge                   schema.EntryID
	TargetCallback                  schema.EntryID
	TargetBinding                   schema.EntryID
	TargetResume                    schema.EntryID
	TargetSpawn                     schema.EntryID
	TargetOpaque                    schema.EntryID
	TargetOperationEffect           schema.EntryID
	TargetCallbackEffect            schema.EntryID
	TargetCallbackRelease           schema.EntryID
	TargetOutcome                   schema.EntryID
	TargetTransfer                  schema.EntryID
	TargetTransferOutcome           schema.EntryID
	TargetSuspension                schema.EntryID
	TargetResumeOutcome             schema.EntryID
	TargetSpawnSibling              schema.EntryID
	TargetSubedgeArgumentOrigin     schema.EntryID
	TargetCallbackResult            schema.EntryID
	TargetResultAlias               schema.EntryID
	TargetProduced                  schema.EntryID
	TargetProducedCapture           schema.EntryID
	TargetFreshResult               schema.EntryID
	TargetPublicationEffect         schema.EntryID
	TargetSubedgeRelation           schema.EntryID
	TargetProtocol                  schema.EntryID
	TargetProtocolState             schema.EntryID
	TargetProtocolAcquisition       schema.EntryID
	TargetProtocolTransition        schema.EntryID
	TargetProtocolTransitionOutcome schema.EntryID
	TargetProtocolEscape            schema.EntryID
	TargetProtocolCallbackHolder    schema.EntryID
	TargetBoot                      schema.EntryID
	TargetBootEntry                 schema.EntryID
	TargetBootMetatableAttachment   schema.EntryID
	TargetBootBinding               schema.EntryID
}

func GeneratedTargetIDs() GeneratedTargetRelationIDs {
	return GeneratedTargetRelationIDs{
		TargetContract:                  generatedRelationEntries[57].ID(),
		TargetOperation:                 generatedRelationEntries[58].ID(),
		TargetABI:                       generatedRelationEntries[59].ID(),
		TargetSubedge:                   generatedRelationEntries[60].ID(),
		TargetCallback:                  generatedRelationEntries[61].ID(),
		TargetBinding:                   generatedRelationEntries[62].ID(),
		TargetResume:                    generatedRelationEntries[63].ID(),
		TargetSpawn:                     generatedRelationEntries[64].ID(),
		TargetOpaque:                    generatedRelationEntries[65].ID(),
		TargetOperationEffect:           generatedRelationEntries[66].ID(),
		TargetCallbackEffect:            generatedRelationEntries[67].ID(),
		TargetCallbackRelease:           generatedRelationEntries[68].ID(),
		TargetOutcome:                   generatedRelationEntries[69].ID(),
		TargetTransfer:                  generatedRelationEntries[70].ID(),
		TargetTransferOutcome:           generatedRelationEntries[71].ID(),
		TargetSuspension:                generatedRelationEntries[72].ID(),
		TargetResumeOutcome:             generatedRelationEntries[73].ID(),
		TargetSpawnSibling:              generatedRelationEntries[74].ID(),
		TargetSubedgeArgumentOrigin:     generatedRelationEntries[75].ID(),
		TargetCallbackResult:            generatedRelationEntries[76].ID(),
		TargetResultAlias:               generatedRelationEntries[77].ID(),
		TargetProduced:                  generatedRelationEntries[78].ID(),
		TargetProducedCapture:           generatedRelationEntries[79].ID(),
		TargetFreshResult:               generatedRelationEntries[80].ID(),
		TargetPublicationEffect:         generatedRelationEntries[81].ID(),
		TargetSubedgeRelation:           generatedRelationEntries[82].ID(),
		TargetProtocol:                  generatedRelationEntries[83].ID(),
		TargetProtocolState:             generatedRelationEntries[84].ID(),
		TargetProtocolAcquisition:       generatedRelationEntries[85].ID(),
		TargetProtocolTransition:        generatedRelationEntries[86].ID(),
		TargetProtocolTransitionOutcome: generatedRelationEntries[87].ID(),
		TargetProtocolEscape:            generatedRelationEntries[88].ID(),
		TargetProtocolCallbackHolder:    generatedRelationEntries[89].ID(),
		TargetBoot:                      generatedRelationEntries[90].ID(),
		TargetBootEntry:                 generatedRelationEntries[91].ID(),
		TargetBootMetatableAttachment:   generatedRelationEntries[92].ID(),
		TargetBootBinding:               generatedRelationEntries[93].ID(),
	}
}

// GeneratedLinkProjectRelationIDs contains the stable denominator identities owned by LinkProject.
type GeneratedLinkProjectRelationIDs struct {
	LinkProjectShardMount      schema.EntryID
	LinkProjectBaseApplication schema.EntryID
}

func GeneratedLinkProjectIDs() GeneratedLinkProjectRelationIDs {
	return GeneratedLinkProjectRelationIDs{
		LinkProjectShardMount:      generatedRelationEntries[94].ID(),
		LinkProjectBaseApplication: generatedRelationEntries[95].ID(),
	}
}

// GeneratedLinkBoundaryRelationIDs contains the stable denominator identities owned by LinkBoundary.
type GeneratedLinkBoundaryRelationIDs struct {
	LinkBoundary schema.EntryID
}

func GeneratedLinkBoundaryIDs() GeneratedLinkBoundaryRelationIDs {
	return GeneratedLinkBoundaryRelationIDs{
		LinkBoundary: generatedRelationEntries[96].ID(),
	}
}

// GeneratedLinkModuleRelationIDs contains the stable denominator identities owned by LinkModule.
type GeneratedLinkModuleRelationIDs struct {
	LinkModule               schema.EntryID
	LinkModuleCache          schema.EntryID
	LinkModuleRepresentative schema.EntryID
	LinkModuleTransport      schema.EntryID
	LinkModuleAnalysisRoot   schema.EntryID
	LinkModuleInitGeneration schema.EntryID
	LinkModuleInitOutcome    schema.EntryID
	LinkModuleInitTerminal   schema.EntryID
}

func GeneratedLinkModuleIDs() GeneratedLinkModuleRelationIDs {
	return GeneratedLinkModuleRelationIDs{
		LinkModule:               generatedRelationEntries[97].ID(),
		LinkModuleCache:          generatedRelationEntries[98].ID(),
		LinkModuleRepresentative: generatedRelationEntries[99].ID(),
		LinkModuleTransport:      generatedRelationEntries[100].ID(),
		LinkModuleAnalysisRoot:   generatedRelationEntries[101].ID(),
		LinkModuleInitGeneration: generatedRelationEntries[102].ID(),
		LinkModuleInitOutcome:    generatedRelationEntries[103].ID(),
		LinkModuleInitTerminal:   generatedRelationEntries[104].ID(),
	}
}

// GeneratedLinkStaticRelationIDs contains the stable denominator identities owned by LinkStatic.
type GeneratedLinkStaticRelationIDs struct {
	LinkStatic schema.EntryID
}

func GeneratedLinkStaticIDs() GeneratedLinkStaticRelationIDs {
	return GeneratedLinkStaticRelationIDs{
		LinkStatic: generatedRelationEntries[105].ID(),
	}
}

// GeneratedLinkHostRelationIDs contains the stable denominator identities owned by LinkHost.
type GeneratedLinkHostRelationIDs struct {
	LinkHost               schema.EntryID
	LinkHostExposure       schema.EntryID
	LinkHostBoot           schema.EntryID
	LinkHostMember         schema.EntryID
	LinkHostEndpointTarget schema.EntryID
}

func GeneratedLinkHostIDs() GeneratedLinkHostRelationIDs {
	return GeneratedLinkHostRelationIDs{
		LinkHost:               generatedRelationEntries[106].ID(),
		LinkHostExposure:       generatedRelationEntries[107].ID(),
		LinkHostBoot:           generatedRelationEntries[108].ID(),
		LinkHostMember:         generatedRelationEntries[109].ID(),
		LinkHostEndpointTarget: generatedRelationEntries[110].ID(),
	}
}
