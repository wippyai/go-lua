package publication

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	"github.com/wippyai/go-lua/analysis/schema/program/heapindex"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programdiagnostic "github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/schema/program/staticnode"
)

// WriteArtifactIdentityFields is the complete ordered Program-publication
// portion of an Artifact identity. The manifest constructs each semantic
// reader once, then delegates each segment to the package that owns its rows,
// validation, and version framing.
func WriteArtifactIdentityFields(state programstate.State, writer identity.StringIdentityWriter) bool {
	if writer == nil || !state.Available() {
		return false
	}
	frozen := state.Frozen()
	program := programschema.Program{Frozen: frozen}
	lifecycleView, lifecycleOK := lifecycle.NewView(state)
	diagnosticView, diagnosticOK := programdiagnostic.NewView(state)
	staticView, staticOK := staticnode.NewView(state)
	if !lifecycleOK || !diagnosticOK || !staticOK {
		return false
	}
	return program.WritePointIdentityFields(writer) &&
		program.WriteValuesIdentityFields(writer) &&
		lifecycleView.WriteArtifactIdentityFields(writer) &&
		program.WriteCallIdentityFields(writer) &&
		program.WriteBodyIdentityFields(writer) &&
		program.WriteModuleIdentityFields(writer) &&
		program.WriteOccurrenceIdentityFields(writer) &&
		program.WriteSummaryIdentityFields(writer) &&
		heapallocation.WriteArtifactIdentityFields(frozen, writer) &&
		heapindex.WriteArtifactIdentityFields(frozen, writer) &&
		diagnosticView.WriteArtifactIdentityFields(writer) &&
		program.WriteStaticTypeValueIdentityFields(writer) &&
		staticView.WriteArtifactIdentityFields(writer) &&
		program.WriteStaticExpressionInputIdentityFields(writer) &&
		program.WriteEnvironmentLocalTransferIdentityFields(writer) &&
		program.WriteRuleOccurrenceIdentityFields(writer) &&
		program.WriteRegionWTOIdentityFields(writer)
}
