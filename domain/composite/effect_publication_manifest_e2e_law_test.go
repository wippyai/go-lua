package composite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/snapshot"
	calldomain "github.com/wippyai/go-lua/domain/call"
	manifesttarget "github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// TestManifestPublicationEffectCompletesMountedTransition proves the positive
// composition seam. A provider-owned global callable carries an explicit
// publication occurrence in its manifest; Lua calls that global through the
// real Link/Program mount; and Effect issues the complete pre-solve call-site
// observation denominator. The solved selected call then drives Placement's
// publication escape rule, proving the payload allocation is SharedHeap.
func TestManifestPublicationEffectCompletesMountedTransition(t *testing.T) {
	target := sealManifestPublicationTarget(t)
	record, failure, mounted := mountFormalTargetRecord(t, target, "publication-manifest-e2e", `
local api = require("publication-manifest-host")
local payload = { value = 1 }
local destination = { name = "receiver" }
return api.publish(payload, destination)
	`)
	if !mounted {
		t.Fatalf("compile and mount manifest publication fixture: %s", failure.Error())
	}
	if record.Source == nil || len(record.Artifacts) == 0 || record.targetContract != target {
		t.Fatal("mounted publication fixture did not retain the exact manifest Target contract")
	}

	operation, operationOK := target.Operations.Lookup(vocabulary.BindingSpec{
		Namespace: vocabulary.BindingModule,
		Owner:     []string{"publication-manifest-host"},
		Member:    []string{"publish"},
	})
	if !operationOK {
		t.Fatal("manifest Target omitted the global publish operation")
	}
	descriptor, descriptorOK := target.Operations.EffectPublication(operation, 0)
	if !descriptorOK || !descriptor.Valid() {
		t.Fatal("manifest Target did not issue the authored publication descriptor and occurrence")
	}

	bound := materializerBinding(t, record)
	committed, table := queryCanonicalProgram(t, record, bound)
	observations, observationsOK := bound.EffectPublicationObservations(committed, record.Artifacts, record.Source.ContextDirectory())
	// Observation admission precedes Call solving. Boundary deliberately owns a
	// factorized Application x Operation may-envelope, so both executable calls
	// (require and publish) need exact observation coordinates. Only the solved
	// publish call may contribute a publication effect or Placement widening.
	if !observationsOK || len(observations) != 2 {
		t.Fatalf("manifest publication observations = %d/%t, want the two-call may-envelope", len(observations), observationsOK)
	}
	for index, observation := range observations {
		if !observation.Available() {
			t.Fatalf("manifest publication observation %d is unavailable", index)
		}
	}
	sealed, sealFailure, sealedOK := committed.Seal(observations)
	if !sealedOK || sealed == nil {
		t.Fatalf("seal manifest publication observation: %v", sealFailure)
	}
	state, solveStatus, solveReport := sealed.SolveWithReport(context.Background())
	if solveStatus != engine.SolveComplete || state == nil {
		t.Fatalf("solve manifest publication observation: status=%v state=%v reason=%v failure=%v point=%v group=%v member=%v rule=%v", solveStatus, state, solveReport.Reason(), solveReport.Failure(), solveReport.Point(), solveReport.Group(), solveReport.Member(), solveReport.Rule())
	}
	assertManifestPublicationPayloadShared(t, record, bound, committed, sealed, state, table)
}

func assertManifestPublicationPayloadShared(t testing.TB, record LinkInputs, bound *ProgramBinding, committed *engine.CommittedProgram, sealed *engine.Solver, state *engine.State, table SelectedQueryTable) {
	t.Helper()
	var publicationCall calldomain.MountedCall
	for index := 0; index < record.CallAlgebra.MountedCallCount(); index++ {
		mounted, mountedOK := record.CallAlgebra.MountedCallAtHandle(index)
		actuals, actualsOK := mountedActualProjectionFor(record.CallAlgebra, record.PackSchema, mounted)
		if !mountedOK || !actualsOK {
			t.Fatalf("mounted publication call %d is unavailable", index)
		}
		if actuals.ActualCount() != 2 {
			continue
		}
		if publicationCall.Valid() {
			t.Fatal("manifest fixture has more than one two-actual publication call")
		}
		publicationCall = mounted
	}
	if !publicationCall.Valid() {
		t.Fatal("manifest fixture has no two-actual publication call")
	}
	_, occurrence, module, _, _, identityOK := record.CallAlgebra.MountedCallIdentity(publicationCall)
	capability, capabilityOK := bound.Rules().CapabilityByKey("placement-publication-escape")
	stage, stageOK := committed.MountedNativeCallStage(capability, module, occurrence)
	if !identityOK || !capabilityOK || !stageOK || !stage.PointID().Available() {
		t.Fatal("publication call has no exact Placement publication-escape stage")
	}

	published, publishedOK := sealed.PublishedSnapshot(state)
	if !publishedOK {
		t.Fatal("manifest publication solve published no snapshot")
	}
	view := published.Snapshot()
	queryPlan, queryPlanOK := snapshot.OpenQuery[identity.ContentID, engine.Answer](&view, published.QueryFamily())
	publications, publicationsOK := bound.QueryPublications(committed, table)
	if !queryPlanOK || !publicationsOK {
		t.Fatal("open manifest publication Placement query surface")
	}
	allocationRoots := formalAllocationRoots(t, record)
	if len(allocationRoots) != 2 {
		t.Fatalf("manifest publication authored allocation roots = %d, want payload and destination", len(allocationRoots))
	}
	payloadID := allocationRoots[0].id
	for _, publication := range publications {
		if publication.Site.Family != QueryFamilyPlacementSummary || publication.Site.Point != stage.PointID() {
			continue
		}
		answer, status := snapshot.Query(&view, queryPlan, publication.Key)
		if status != snapshot.ReadHit || !answer.Available() {
			t.Fatalf("publication Placement answer = %s, want hit", status)
		}
		cell, cellOK := publication.CanonicalCell(answer)
		result, resultOK := placementdomain.DecodeSummaryResult(record.PlacementSchema, cell.Present(), cell.RowCount(), cell.Payload())
		if !cellOK || !resultOK {
			t.Fatal("decode publication Placement result")
		}
		rows := decodeFormalPlacementRows(t, result)
		if got := rows[payloadID]; got != placementdomain.SharedHeap {
			t.Fatalf("published payload placement = %s, want SharedHeap", got)
		}
		return
	}
	t.Fatal("publication call stage has no typed Placement summary")
}

func sealManifestPublicationTarget(t testing.TB) *contract.Contract {
	t.Helper()
	provider := manifest.Provider{
		Identity: "publication-manifest-host",
		Mount:    manifest.MountModule,
		Declaration: func() *manifestwire.Manifest {
			declaration := manifestwire.New("publication-manifest-host")
			functionType := typ.Func().Param("value", typ.Any).Param("destination", typ.Any).Returns(typ.Any).Build()
			declaration.DefineFunctionSignature("publish", signature.Function{Type: functionType})
			declaration.DefineFunctionSignature("publish-target", signature.Function{Type: functionType})
			declaration.DefineFunctionOperation("publish", manifestwire.Operation{
				Effects: manifestwire.RowSpec{
					Occurrences: []manifestwire.EffectSpec{{
						Target:    "publication-manifest-host.publish-target",
						ValueArgs: []manifestwire.ValueFormal{0, 1},
						Publication: &manifestwire.PublicationEffectSpec{
							Kind:        manifestwire.PublicationEffectSendTransfer,
							Subject:     manifestwire.InputSource{Kind: manifestwire.InputSourceValue, Ordinal: 0},
							Destination: manifestwire.PublicationDestinationValueFormal,
							Context:     1,
							Escape:      manifestwire.PublicationEscapeSendTransfer,
							Mutability:  manifestwire.PublicationMutabilityCopyOnWrite,
							Lifetime:    manifestwire.PublicationLifetimePreserve,
						},
					}},
					Tail: manifestwire.RowClosed,
				},
			})
			return declaration
		},
	}
	// The module table is reached through the canonical host require seed. Keep
	// that ingress explicit in the same manifest-backed Target rather than
	// relying on a test-only Link or a handwritten operation.
	requireProvider := manifest.Provider{
		Identity: "publication-manifest-require",
		Mount:    manifest.MountGlobals,
		Declaration: func() *manifestwire.Manifest {
			declaration := manifestwire.New("publication-manifest-require")
			requireType := typ.Func().Param("module", typ.String).Returns(typ.Any).Build()
			declaration.DefineFunctionSignature("require", signature.Function{
				Type: requireType, Effect: effect.Empty.With(dispatch.ModuleLoad{}),
			})
			declaration.DefineGlobalType("require", requireType)
			return declaration
		},
	}
	catalogue, err := manifest.Seal(append(stdlib.Providers(), provider, requireProvider)...)
	if err != nil {
		t.Fatal(err)
	}
	target, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}
	return target
}
