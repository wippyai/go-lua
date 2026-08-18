package target

import (
	"fmt"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"testing"
)

// The sealed census is a set of measurements of the Contract's own columns. What
// makes a measurement correct is that it agrees with what a consumer reading the
// published surface would count, so the law below states the census against a walk
// of that surface rather than against a recorded number.
//
// Every contract the suite seals is checked, through mustSeal. A census row that
// drifts from the read surface fails at the seal that produced it.

// censusFromReadSurface counts one sealed contract the way a consumer reading it
// would: operation by operation, relation by relation, through public queries only.
func censusFromReadSurface(t *testing.T, contract *Contract) map[schema.EntryID]uint64 {
	t.Helper()
	ids := denominator.GeneratedTargetIDs()
	observed := map[schema.EntryID]uint64{}
	add := func(key schema.EntryID, value int) {
		t.Helper()
		if value < 0 {
			t.Fatalf("negative cardinality %d", value)
		}
		observed[key] += uint64(value)
	}

	observed[ids.TargetContract] = 1
	observed[ids.TargetOpaque] = 1

	for index := 0; index < contract.OperationCount(); index++ {
		op, ok := contract.OperationAt(index)
		if !ok {
			t.Fatalf("operation %d is not readable", index)
		}
		add(ids.TargetOperation, 1)
		add(ids.TargetABI, 1)
		add(ids.TargetOperationEffect, contract.EffectCount(op))
		add(ids.TargetSubedge, contract.SubedgeCount(op))
		add(ids.TargetBinding, contract.BindingCount(op))
		add(ids.TargetResume, contract.ResumeCount(op))
		add(ids.TargetSpawn, contract.spawnCount(op))
		add(ids.TargetSuspension, contract.suspensionCount(op))
		add(ids.TargetTransfer, contract.transferCount(op))

		for effect := 0; effect < contract.EffectCount(op); effect++ {
			if _, found := contract.PublicationEffectDescriptor(op, effect); found {
				add(ids.TargetPublicationEffect, 1)
			}
		}
		for subedge := 0; subedge < contract.SubedgeCount(op); subedge++ {
			edge, found := contract.SubedgeAt(op, subedge)
			if !found {
				t.Fatalf("subedge %d of operation %d is not readable", subedge, op)
			}
			add(ids.TargetSubedgeArgumentOrigin, contract.argumentOriginCount(edge))
		}
		for transfer := 0; transfer < contract.transferCount(op); transfer++ {
			add(ids.TargetTransferOutcome, contract.transferOutcomeCount(op, transfer))
		}
		for index := 0; index < contract.CallbackCount(op); index++ {
			callback, found := contract.CallbackAt(op, index)
			if !found {
				t.Fatalf("callback %d of operation %d is not readable", index, op)
			}
			add(ids.TargetCallback, 1)
			add(ids.TargetCallbackEffect, contract.CallbackEffectCount(callback))
			for effect := 0; effect < contract.CallbackEffectCount(callback); effect++ {
				if _, found := contract.CallbackPublicationEffectDescriptor(callback, effect); found {
					add(ids.TargetPublicationEffect, 1)
				}
			}
			if _, _, _, _, found := contract.callbackRelease(callback); found {
				add(ids.TargetCallbackRelease, 1)
			}
		}
		for index := 0; index < contract.ResumeCount(op); index++ {
			resume, found := contract.ResumeIDAt(op, index)
			if !found {
				t.Fatalf("resume %d of operation %d is not readable", index, op)
			}
			add(ids.TargetResumeOutcome, contract.resumeOutcomeCount(resume))
		}
		for index := 0; index < contract.spawnCount(op); index++ {
			spawn, found := contract.spawnIDAt(op, index)
			if !found {
				t.Fatalf("spawn %d of operation %d is not readable", index, op)
			}
			add(ids.TargetSpawnSibling, contract.spawnSiblingCount(spawn))
		}
		for outcome := 0; outcome < contract.OutcomeCount(op); outcome++ {
			add(ids.TargetOutcome, 1)
			add(ids.TargetCallbackResult, contract.callbackResultCount(op, outcome))
			add(ids.TargetResultAlias, contract.resultAliasCount(op, outcome))
			add(ids.TargetFreshResult, contract.FreshResultCount(op, outcome))
			add(ids.TargetProduced, contract.producedCount(op, outcome))
			for produced := 0; produced < contract.producedCount(op, outcome); produced++ {
				add(ids.TargetProducedCapture, contract.producedCaptureCount(op, outcome, produced))
			}
		}
		if _, _, _, _, _, found := contract.OperationSubedgeRelation(op); found {
			add(ids.TargetSubedgeRelation, 1)
		}
	}

	for index := 0; index < contract.protocolCount(); index++ {
		protocol, ok := contract.protocolAt(index)
		if !ok {
			t.Fatalf("protocol %d is not readable", index)
		}
		add(ids.TargetProtocol, 1)
		add(ids.TargetProtocolState, contract.stateCount(protocol))
		add(ids.TargetProtocolAcquisition, contract.protocolAcquisitionCount(protocol))
		add(ids.TargetProtocolEscape, contract.escapeCount(protocol))
		add(ids.TargetProtocolCallbackHolder, contract.protocolCallbackHolderCount(protocol))
		for transition := 0; transition < contract.transitionCount(protocol); transition++ {
			add(ids.TargetProtocolTransition, 1)
			add(ids.TargetProtocolTransitionOutcome, contract.transitionOutcomeCount(protocol, transition))
		}
	}

	add(ids.TargetBoot, contract.InitialRootCount())
	add(ids.TargetBootEntry, contract.InitialEntryCount())
	add(ids.TargetBootMetatableAttachment, contract.InitialMetatableAttachmentCount())
	add(ids.TargetBootBinding, contract.InitialBindingCount())
	return observed
}

// assertCensusMatchesReadSurface states the census law over one sealed contract.
func assertCensusMatchesReadSurface(t *testing.T, contract *Contract) {
	t.Helper()
	rows := contract.CountRows()
	if !denominator.GeneratedCountRowsCompleteForOwners(rows, denominator.RelationOwnerTarget) {
		t.Fatal("sealed census does not cover the generated Target owner catalog")
	}
	observed := censusFromReadSurface(t, contract)
	for index := 0; index < rows.Count(); index++ {
		row, ok := rows.At(index)
		if !ok {
			t.Fatalf("census row %d is not readable", index)
		}
		if row.Count() != observed[row.ID()] {
			t.Errorf("census row %s = %d, read surface reports %d", censusRelationName(row.ID()), row.Count(), observed[row.ID()])
		}
		delete(observed, row.ID())
	}
	for id, value := range observed {
		if value != 0 {
			t.Errorf("read surface reports %d of %s, which the census does not publish", value, censusRelationName(id))
		}
	}
}

// censusRelationName resolves a denominator entry identity to its declared key.
func censusRelationName(id schema.EntryID) string {
	for _, entry := range denominator.GeneratedRelationEntries() {
		if entry != nil && entry.ID() == id {
			return fmt.Sprintf("%+v", entry.Key())
		}
	}
	return "unknown"
}

func TestCountRowsPublishesExactlyTargetOwnerRelations(t *testing.T) {
	contract := mustSeal(t, Spec{Operations: []vocabulary.OperationSpec{builtin("counted", testString, vocabulary.RowSpec{Tail: vocabulary.RowClosed})}})
	rows := contract.CountRows()
	if !denominator.GeneratedCountRowsCompleteForOwners(rows, denominator.RelationOwnerTarget) {
		t.Fatal("target CountRows did not cover its generated owner catalog")
	}
	ids := denominator.GeneratedTargetIDs()
	if got, ok := rows.Value(ids.TargetOperation); !ok || got == 0 {
		t.Fatalf("target operation count = %d/%v, want a nonzero row", got, ok)
	}
}
