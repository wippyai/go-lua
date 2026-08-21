package composite

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	manifesttarget "github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// TestManifestPublicationEffectCompletesMountedTransition proves the positive
// composition seam. A provider-owned global callable carries an explicit
// publication occurrence in its manifest; Lua calls that global through the
// real Link/Program mount; and Effect issues one exact selected-call
// observation. No runtime delivery or placement conclusion is inferred here.
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
	committed, _ := queryCanonicalProgram(t, record, bound)
	observations, observationsOK := bound.EffectPublicationObservations(committed, record.Artifacts)
	if !observationsOK || len(observations) != 1 {
		t.Fatalf("manifest publication observations = %d/%t, want one exact admission", len(observations), observationsOK)
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
