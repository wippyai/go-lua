package publication

import (
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	programdiagnostic "github.com/wippyai/go-lua/analysis/schema/program/programdiagnostic"
	staticnode "github.com/wippyai/go-lua/analysis/schema/program/staticnode"
)

// C7DebugValidateStages is temporary audit instrumentation; remove after the
// C7 artifact publication owner is identified.
func C7DebugValidateStages(program programschema.Program) [6]bool {
	var out [6]bool
	if !program.Available() {
		return out
	}
	state, ok := program.ColdState()
	if !ok {
		return out
	}
	lifecycleView, lifecycleOK := lifecycle.NewView(state)
	diagnosticView, diagnosticOK := programdiagnostic.NewView(state)
	staticView, staticOK := staticnode.NewView(state)
	if !lifecycleOK || !diagnosticOK || !staticOK {
		return out
	}
	v := validator{program: program, state: state, frozen: state.Frozen(), catalog: state.CatalogID(), lifecycle: lifecycleView, diagnostic: diagnosticView, static: staticView}
	if _, ok := v.program.EntryBody(); !ok {
		return out
	}
	vs := validationState{}
	out[0] = v.validateSealFoundation(&vs)
	if !out[0] {
		return out
	}
	out[1] = v.validateSealIndexes(&vs)
	if !out[1] {
		return out
	}
	out[2] = v.validateSealRows(&vs)
	if !out[2] {
		return out
	}
	out[3] = v.validateSealModule(&vs)
	if !out[3] {
		return out
	}
	out[4] = v.validateSealFreeze(&vs)
	out[5] = true
	return out
}
