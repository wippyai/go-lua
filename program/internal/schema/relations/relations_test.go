package relations

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/wippyai/go-lua/program/semanticsource"
)

func TestCanonicalSchemaCanonicalizesAndDefendsSnapshots(t *testing.T) {
	first, err := CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	reversed := first.Rows()
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	second, err := Seal(semanticsource.CatalogSchema(), reversed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Canonical(), second.Canonical()) || first.Digest() != second.Digest() {
		t.Fatal("input row order changed canonical schema identity")
	}
	operator := definitionFor(t, semanticsource.OriginProgramFlowOperators, 0)
	operatorSource := rowFor(t, first.Rows(), semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric)
	if len(operatorSource.Parents) != 1 || operatorSource.Parents[0] != operator.Token() {
		t.Fatal("typed operator-source parent is not exact")
	}
	rows := first.Rows()
	for index := range rows {
		if rows[index].Definition.Token() == operatorSource.Definition.Token() {
			rows[index].Parents[0] = semanticsource.Token{}
		}
	}
	again := first.Rows()
	if rowFor(t, again, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric).Parents[0] != operator.Token() {
		t.Fatal("returned rows changed sealed schema")
	}
	canonical := first.Canonical()
	canonical[0] ^= 0xff
	if bytes.Equal(canonical, first.Canonical()) {
		t.Fatal("returned canonical bytes changed sealed schema")
	}
}

func TestCanonicalSchemaDeclaresTheRetainedDenominator(t *testing.T) {
	schema, err := CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	if schema.Count() != 111 {
		t.Fatalf("relation count = %d, want 111", schema.Count())
	}
	t.Logf("catalog relations=%d digest=%x", schema.Count(), schema.Digest())
	counts := map[Owner]int{}
	for _, row := range schema.Rows() {
		counts[row.Owner]++
	}
	for owner, want := range map[Owner]int{
		OwnerProgramSource: 8,
		OwnerProgramFlow:   33,
		OwnerProgramStatic: 10,
		OwnerProgramModule: 6,
		OwnerTarget:        37,
		OwnerLinkProject:   2,
		OwnerLinkBoundary:  1,
		OwnerLinkModule:    8,
		OwnerLinkStatic:    1,
		OwnerLinkHost:      5,
	} {
		if got := counts[owner]; got != want {
			t.Fatalf("owner %d count = %d, want %d", owner, got, want)
		}
	}
	for _, facet := range []semanticsource.Facet{
		semanticsource.FacetProgramFlowUnaryNumeric,
		semanticsource.FacetProgramFlowLength,
		semanticsource.FacetProgramFlowArithmetic,
		semanticsource.FacetProgramFlowBitwise,
		semanticsource.FacetProgramFlowConcat,
		semanticsource.FacetProgramFlowEquality,
		semanticsource.FacetProgramFlowOrder,
		semanticsource.FacetProgramFlowIndexGet,
		semanticsource.FacetProgramFlowIndexSet,
	} {
		assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowOperators, facet, FormAuthored)
	}
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowOutcome, 0, FormSealDerived)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowTransfer, 0, FormSealDerived)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageVararg, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageBind, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence, FormVirtualPredicate)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowBody, 0, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots, FormSealDerived)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticPublication, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramModuleImport, 0, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramModuleImport, semanticsource.FacetProgramModuleRequest, FormSealDerived)
	assertForm(t, schema.Rows(), semanticsource.OriginProgramFlowCall, semanticsource.FacetProgramFlowDirectCallBinding, FormSealDerived)
	assertForm(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque, FormSealDerived)
	assertForm(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetPublicationEffect, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackRelease, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetSuspension, FormSealDerived)
	assertForm(t, schema.Rows(), semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolEscape, FormSealDerived)
	assertForm(t, schema.Rows(), semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative, FormSealDerived)
	assertForm(t, schema.Rows(), semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginLinkHost, 0, FormAuthored)
	assertForm(t, schema.Rows(), semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure, FormSealDerived)
	assertForm(t, schema.Rows(), semanticsource.OriginLinkHost, semanticsource.FacetLinkHostMember, FormSealDerived)
	assertForm(t, schema.Rows(), semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget, FormSealDerived)
	for _, retired := range []string{
		"ProgramFlowOperators@ProgramFlowOperatorArms",
		"ProgramStatic@ProgramStaticTypeSyntax",
		"TargetOperation@TargetEffects",
		"TargetOperation@TargetLifecycle",
		"ProgramFlowStorage@ProgramFlowStorageBindingSelection",
		"ProgramFlowBody@ProgramFlowBodyActivationDecision",
		"ProgramFlowMu@-",
		"ProgramFlowMu@ProgramFlowMuDecision",
		"ProgramFlowContinuation@-",
		"ProgramFlowContinuation@ProgramFlowContinuationDecision",
		"ProgramFlowContinuation@ProgramFlowContinuationCell",
		"ProgramFlowContinuation@ProgramFlowContinuationValue",
	} {
		if _, ok := CatalogToken(retired); ok {
			t.Fatalf("retired relation revived: %s", retired)
		}
	}

	storageCell := definitionFor(t, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell)
	storageGlobal := definitionFor(t, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal)
	storageRead := definitionFor(t, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead)
	storageAssign := definitionFor(t, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign)
	storageVararg := definitionFor(t, semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageVararg)
	key := definitionFor(t, semanticsource.OriginProgramSourceKey, 0)
	values := definitionFor(t, semanticsource.OriginProgramFlowValues, 0)
	valueOccurrence := definitionFor(t, semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence)
	lens := definitionFor(t, semanticsource.OriginProgramFlowLens, 0)
	constructors := definitionFor(t, semanticsource.OriginProgramFlowConstructors, 0)
	operators := definitionFor(t, semanticsource.OriginProgramFlowOperators, 0)
	call := definitionFor(t, semanticsource.OriginProgramFlowCall, 0)
	claim := definitionFor(t, semanticsource.OriginProgramFlowClaim, 0)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageCell))
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageGlobal), storageCell, key)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageRead), storageCell, lens)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageAssign), values)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageWrite), storageAssign, storageCell, lens)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageVararg), storageCell)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowStorage, semanticsource.FacetProgramFlowStorageBind), storageCell, values)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowValues, 0), valueOccurrence)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence),
		definitionFor(t, semanticsource.OriginProgramFlowLiterals, 0),
		storageRead,
		storageVararg,
		constructors,
		operators,
		definitionFor(t, semanticsource.OriginProgramFlowFunction, 0),
		call,
		claim,
		definitionFor(t, semanticsource.OriginProgramFlowTypeValue, 0),
	)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowLens, 0), valueOccurrence, key)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowConstructors, 0), valueOccurrence, values, key)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowOperators, 0), valueOccurrence)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowConstructors, semanticsource.FacetProgramFlowConstructorField), constructors, valueOccurrence, lens)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowFunction, semanticsource.FacetProgramFlowFunctionCapture),
		definitionFor(t, semanticsource.OriginProgramFlowFunction, 0), storageCell)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowCall, 0), valueOccurrence, values)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowClaim, 0), valueOccurrence)
	moduleImport := definitionFor(t, semanticsource.OriginProgramModuleImport, 0)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramModuleImport, 0), call, storageCell)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramModuleImport, semanticsource.FacetProgramModuleRequest),
		moduleImport,
		valueOccurrence,
		definitionFor(t, semanticsource.OriginProgramFlowLiterals, 0),
		definitionFor(t, semanticsource.OriginProgramSourceExactKey, 0),
	)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowCall, semanticsource.FacetProgramFlowDirectCallBinding), call)
	entry := definitionFor(t, semanticsource.OriginProgramModuleEntry, 0)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramModuleEntry, 0),
		definitionFor(t, semanticsource.OriginProgramFlowControl, 0),
		definitionFor(t, semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots))
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryRootCell), entry, storageCell)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryMember),
		entry, definitionFor(t, semanticsource.OriginProgramFlowConstructors, semanticsource.FacetProgramFlowConstructorField))
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramModuleEntry, semanticsource.FacetProgramModuleEntryRootFunction),
		entry, definitionFor(t, semanticsource.OriginProgramFlowFunction, 0))

	body := definitionFor(t, semanticsource.OriginProgramFlowBody, 0)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowBody, 0))
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramFlowBody, semanticsource.FacetProgramFlowBodyRoots), body)

	typeRef := definitionFor(t, semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeRef)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticAnnotation), values, typeRef)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticTypeof),
		definitionFor(t, semanticsource.OriginProgramFlowValues, semanticsource.FacetProgramFlowValueOccurrence), typeRef)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticCellDeclaredType),
		definitionFor(t, semanticsource.OriginProgramStatic, 0), storageCell)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticPublication), storageAssign, typeRef)

	targetBinding := definitionFor(t, semanticsource.OriginTargetOperation, semanticsource.FacetTargetBinding)
	targetOperation := definitionFor(t, semanticsource.OriginTargetOperation, 0)
	targetCallback := definitionFor(t, semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallback)
	targetABI := definitionFor(t, semanticsource.OriginTargetOperation, semanticsource.FacetTargetABI)
	targetOutcome := definitionFor(t, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOutcome)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetResume), targetOperation)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetSpawn), targetOperation, targetCallback)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetOpaque), targetOperation)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect), targetOperation, targetABI)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect), targetCallback, targetABI)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetPublicationEffect),
		targetOperation,
		targetABI,
		targetCallback,
		definitionFor(t, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect),
		definitionFor(t, semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackEffect),
	)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetCallbackRelease), targetCallback, targetOperation, targetABI, targetOutcome)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginTargetOperation, semanticsource.FacetTargetProduced), targetOperation, targetOutcome)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginTargetGsub, 0),
		definitionFor(t, semanticsource.OriginTargetOperation, 0),
		definitionFor(t, semanticsource.OriginTargetOperation, semanticsource.FacetTargetSubedge),
		targetABI,
		definitionFor(t, semanticsource.OriginTargetOperation, semanticsource.FacetTargetOperationEffect),
		targetBinding,
		targetOutcome,
	)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleTransport),
		definitionFor(t, semanticsource.OriginLinkBoundary, 0),
		definitionFor(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache),
		definitionFor(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative),
		definitionFor(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot),
	)

	linkHost := definitionFor(t, semanticsource.OriginLinkHost, 0)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginLinkHost, semanticsource.FacetLinkHostEndpointTarget), linkHost, targetBinding)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure), linkHost)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginLinkHost, semanticsource.FacetLinkHostMember),
		definitionFor(t, semanticsource.OriginLinkHost, semanticsource.FacetLinkHostExposure), definitionFor(t, semanticsource.OriginProgramFlowLens, 0), key, targetBinding)
	assertParents(t, rowFor(t, schema.Rows(), semanticsource.OriginLinkHost, semanticsource.FacetLinkHostBoot),
		definitionFor(t, semanticsource.OriginLinkModule, 0),
		definitionFor(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleAnalysisRoot),
		definitionFor(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleCache),
		definitionFor(t, semanticsource.OriginLinkModule, semanticsource.FacetLinkModuleRepresentative),
		definitionFor(t, semanticsource.OriginTargetBoot, 0), storageGlobal, key)

	application := rowFor(t, schema.Rows(), semanticsource.OriginLinkProjectBaseApplication, 0)
	assertParents(t, application,
		definitionFor(t, semanticsource.OriginLinkProjectShardMount, 0),
		definitionFor(t, semanticsource.OriginProgramFlowCall, 0),
		definitionFor(t, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowUnaryNumeric),
		definitionFor(t, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowLength),
		definitionFor(t, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowArithmetic),
		definitionFor(t, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowBitwise),
		definitionFor(t, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowConcat),
		definitionFor(t, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowEquality),
		definitionFor(t, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowOrder),
		definitionFor(t, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexGet),
		definitionFor(t, semanticsource.OriginProgramFlowOperators, semanticsource.FacetProgramFlowIndexSet),
		definitionFor(t, semanticsource.OriginProgramFlowControl, semanticsource.FacetProgramFlowGenericFor),
	)
	boundary := rowFor(t, schema.Rows(), semanticsource.OriginLinkBoundary, 0)
	boundaryParents := []semanticsource.RelationDef{
		definitionFor(t, semanticsource.OriginLinkProjectBaseApplication, 0),
		definitionFor(t, semanticsource.OriginTargetOperation, 0),
	}
	for _, facet := range []semanticsource.Facet{
		semanticsource.FacetTargetABI,
		semanticsource.FacetTargetSubedge,
		semanticsource.FacetTargetCallback,
		semanticsource.FacetTargetBinding,
		semanticsource.FacetTargetResume,
		semanticsource.FacetTargetSpawn,
		semanticsource.FacetTargetOpaque,
		semanticsource.FacetTargetOperationEffect,
		semanticsource.FacetTargetCallbackEffect,
		semanticsource.FacetTargetCallbackRelease,
		semanticsource.FacetTargetOutcome,
		semanticsource.FacetTargetTransfer,
		semanticsource.FacetTargetTransferOutcome,
		semanticsource.FacetTargetSuspension,
		semanticsource.FacetTargetResumeOutcome,
		semanticsource.FacetTargetSpawnSibling,
		semanticsource.FacetTargetSubedgeArgumentOrigin,
		semanticsource.FacetTargetCallbackResult,
		semanticsource.FacetTargetResultAlias,
		semanticsource.FacetTargetProduced,
		semanticsource.FacetTargetProducedCapture,
		semanticsource.FacetTargetFreshResult,
		semanticsource.FacetTargetPublicationEffect,
	} {
		boundaryParents = append(boundaryParents, definitionFor(t, semanticsource.OriginTargetOperation, facet))
	}
	boundaryParents = append(boundaryParents,
		definitionFor(t, semanticsource.OriginTargetProtocol, 0),
		definitionFor(t, semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolState),
		definitionFor(t, semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolAcquisition),
		definitionFor(t, semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransition),
		definitionFor(t, semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolTransitionOutcome),
		definitionFor(t, semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolEscape),
		definitionFor(t, semanticsource.OriginTargetProtocol, semanticsource.FacetTargetProtocolCallbackHolder),
		definitionFor(t, semanticsource.OriginTargetGsub, 0),
	)
	assertParents(t, boundary, boundaryParents...)

	var recursive []string
	for _, component := range parentSCCs(schema.Rows()) {
		if len(component) < 2 {
			continue
		}
		names := make([]string, len(component))
		for index, token := range component {
			name, ok := CatalogName(token)
			if !ok {
				t.Fatalf("recursive SCC contains unnamed token %v", token)
			}
			names[index] = name
		}
		recursive = append(recursive, strings.Join(names, ","))
	}
	const wantRecursive = "ProgramFlowValues@-,ProgramFlowValues@ProgramFlowValueOccurrence,ProgramFlowLens@-,ProgramFlowStorage@ProgramFlowStorageRead,ProgramFlowConstructors@-,ProgramFlowOperators@-,ProgramFlowCall@-,ProgramFlowClaim@-"
	t.Logf("derived recursive SCCs: %s", strings.Join(recursive, "; "))
	if len(recursive) != 1 || recursive[0] != wantRecursive {
		t.Fatalf("derived recursive SCCs = %q, want %q", recursive, wantRecursive)
	}
}

func TestCanonicalSchemaCachedConcurrentReplayAndZeroAlloc(t *testing.T) {
	first, err := CanonicalSchema()
	if err != nil || first == nil {
		t.Fatalf("initial CanonicalSchema() = %p/%v", first, err)
	}
	wantDigest := first.Digest()
	wantCount := first.Count()
	const readers = 32
	results := make(chan *Schema, readers)
	errorsSeen := make(chan error, readers)
	var wait sync.WaitGroup
	wait.Add(readers)
	for index := 0; index < readers; index++ {
		go func() {
			defer wait.Done()
			schema, schemaErr := CanonicalSchema()
			if schemaErr != nil {
				errorsSeen <- schemaErr
				return
			}
			results <- schema
		}()
	}
	wait.Wait()
	close(results)
	close(errorsSeen)
	for schemaErr := range errorsSeen {
		t.Fatalf("concurrent CanonicalSchema() error = %v", schemaErr)
	}
	for schema := range results {
		if schema != first || schema.Digest() != wantDigest || schema.Count() != wantCount {
			t.Fatal("concurrent CanonicalSchema() did not replay the cached immutable authority")
		}
	}
	allocations := testing.AllocsPerRun(1000, func() {
		schema, schemaErr := CanonicalSchema()
		if schemaErr != nil || schema != first {
			panic("CanonicalSchema cache replay failed")
		}
	})
	if allocations != 0 {
		t.Fatalf("cached CanonicalSchema() allocations = %f, want zero", allocations)
	}
}

func TestGeneratedCatalogNameTokenRoundTrip(t *testing.T) {
	schema, err := CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]struct{}{}
	for _, row := range schema.Rows() {
		name, ok := CatalogName(row.Definition.Token())
		if !ok || name == "" {
			t.Fatal("issued token has no generated name")
		}
		if _, duplicate := seen[name]; duplicate {
			t.Fatalf("duplicate generated name %q", name)
		}
		seen[name] = struct{}{}
		token, ok := CatalogToken(name)
		if !ok || token != row.Definition.Token() {
			t.Fatalf("name/token round-trip failed for %q", name)
		}
	}
	if len(seen) != schema.Count() {
		t.Fatalf("generated names = %d, want %d", len(seen), schema.Count())
	}
	if _, ok := CatalogToken("not-a-catalog-name"); ok {
		t.Fatal("foreign generated name accepted")
	}
}

func TestSealRejectsInvalidRows(t *testing.T) {
	canonical, err := CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	primary := definitionFor(t, semanticsource.OriginProgramSourceProvenance, 0)
	valid := canonical.Rows()
	tests := []struct {
		name string
		rows []Row
		want error
	}{
		{name: "zero definition", rows: []Row{{Definition: semanticsource.RelationDef{}, Owner: OwnerProgramFlow, Form: FormAuthored}}, want: ErrInvalidDefinition},
		{name: "duplicate definition", rows: append(append([]Row(nil), valid...), valid[0]), want: ErrDuplicateDefinition},
		{name: "missing definition", rows: valid[:len(valid)-1], want: ErrMissingDefinition},
		{name: "invalid owner", rows: replace(valid, primary.Token(), Row{Definition: primary, Form: FormAuthored}), want: ErrInvalidOwner},
		{name: "invalid form", rows: replace(valid, primary.Token(), Row{Definition: primary, Owner: OwnerProgramSource}), want: ErrInvalidForm},
		{name: "unknown parent", rows: replace(valid, primary.Token(), Row{Definition: primary, Owner: OwnerProgramSource, Form: FormAuthored, Parents: []semanticsource.Token{{}}}), want: ErrUnknownParent},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Seal(semanticsource.CatalogSchema(), test.rows)
			if !errors.Is(err, test.want) {
				t.Fatalf("Seal() error = %v, want %v", err, test.want)
			}
		})
	}
	if _, err := Seal(semanticsource.Schema{}, valid); !errors.Is(err, ErrInvalidDefinitions) {
		t.Fatalf("invalid source definitions error = %v, want %v", err, ErrInvalidDefinitions)
	}
}

func TestSealPermitsSameOwnerParentCycles(t *testing.T) {
	canonical, err := CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	primary := definitionFor(t, semanticsource.OriginProgramSourceProvenance, 0)
	other := definitionFor(t, semanticsource.OriginProgramSourceOrder, 0)
	rows := replace(canonical.Rows(), primary.Token(), Row{Definition: primary, Owner: OwnerProgramSource, Form: FormAuthored, Parents: []semanticsource.Token{other.Token()}})
	rows = replace(rows, other.Token(), Row{Definition: other, Owner: OwnerProgramSource, Form: FormAuthored, Parents: []semanticsource.Token{primary.Token()}})
	if _, err = Seal(semanticsource.CatalogSchema(), rows); err != nil {
		t.Fatalf("Seal() same-owner cycle error = %v", err)
	}
}

func TestSealRejectsCrossOwnerParentCycles(t *testing.T) {
	canonical, err := CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	primary := definitionFor(t, semanticsource.OriginProgramSourceProvenance, 0)
	other := definitionFor(t, semanticsource.OriginProgramSourceOrder, 0)
	rows := replace(canonical.Rows(), primary.Token(), Row{Definition: primary, Owner: OwnerProgramSource, Form: FormAuthored, Parents: []semanticsource.Token{other.Token()}})
	rows = replace(rows, other.Token(), Row{Definition: other, Owner: OwnerProgramFlow, Form: FormAuthored, Parents: []semanticsource.Token{primary.Token()}})
	_, err = Seal(semanticsource.CatalogSchema(), rows)
	if !errors.Is(err, ErrCyclicParents) {
		t.Fatalf("Seal() error = %v, want %v", err, ErrCyclicParents)
	}
}

func TestSealRejectsCyclicOwnerDependencies(t *testing.T) {
	canonical, err := CanonicalSchema()
	if err != nil {
		t.Fatal(err)
	}
	literals := definitionFor(t, semanticsource.OriginProgramFlowLiterals, 0)
	static := definitionFor(t, semanticsource.OriginProgramStatic, 0)
	annotation := definitionFor(t, semanticsource.OriginProgramStatic, semanticsource.FacetProgramStaticAnnotation)
	rows := replace(canonical.Rows(), literals.Token(), Row{Definition: literals, Owner: OwnerProgramFlow, Form: FormAuthored, Parents: []semanticsource.Token{static.Token()}})
	rows = replace(rows, annotation.Token(), Row{Definition: annotation, Owner: OwnerProgramStatic, Form: FormAuthored, Parents: []semanticsource.Token{literals.Token()}})
	_, err = Seal(semanticsource.CatalogSchema(), rows)
	if !errors.Is(err, ErrCyclicOwners) {
		t.Fatalf("Seal() error = %v, want %v", err, ErrCyclicOwners)
	}
}

func definitionFor(t *testing.T, origin semanticsource.Origin, facet semanticsource.Facet) semanticsource.RelationDef {
	t.Helper()
	for _, definition := range semanticsource.CatalogSchema().Definitions() {
		token := definition.Token()
		if token.Origin() == origin && token.Facet() == facet {
			return definition
		}
	}
	t.Fatalf("missing generated definition origin=%d facet=%d", origin, facet)
	return semanticsource.RelationDef{}
}

func rowFor(t *testing.T, rows []Row, origin semanticsource.Origin, facet semanticsource.Facet) Row {
	t.Helper()
	definition := definitionFor(t, origin, facet)
	for _, row := range rows {
		if row.Definition.Token() == definition.Token() {
			return row
		}
	}
	t.Fatalf("missing row origin=%d facet=%d", origin, facet)
	return Row{}
}

func replace(rows []Row, token semanticsource.Token, replacement Row) []Row {
	owned := append([]Row(nil), rows...)
	for index := range owned {
		if owned[index].Definition.Token() == token {
			owned[index] = replacement
			return owned
		}
	}
	return owned
}

func assertForm(t *testing.T, rows []Row, origin semanticsource.Origin, facet semanticsource.Facet, want Form) {
	t.Helper()
	if got := rowFor(t, rows, origin, facet).Form; got != want {
		t.Fatalf("origin=%d facet=%d form=%d, want %d", origin, facet, got, want)
	}
}

func assertParents(t *testing.T, row Row, definitions ...semanticsource.RelationDef) {
	t.Helper()
	want := make(map[semanticsource.Token]struct{}, len(definitions))
	for _, definition := range definitions {
		want[definition.Token()] = struct{}{}
	}
	if len(row.Parents) != len(want) {
		t.Fatalf("parent count = %d, want %d", len(row.Parents), len(want))
	}
	for _, parent := range row.Parents {
		if _, exists := want[parent]; !exists {
			t.Fatalf("unexpected parent origin=%d facet=%d", parent.Origin(), parent.Facet())
		}
	}
}
