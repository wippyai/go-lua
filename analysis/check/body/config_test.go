package body

import (
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestConfigSolveConfigOwnsPerSolveAxes(t *testing.T) {
	typeValues := typevalue.NewCache()
	stats := &Stats{}
	entryTable := identity.ID{Kind: "table", Site: "config-entry", Index: 1}
	initialTable := identity.ID{Kind: "table", Site: "config-initial", Index: 1}
	entryState := state.State{}.FreezeTable(entryTable)
	initialState := state.State{}.FreezeTable(initialTable)
	invariant := factapply.ClosedDynamicAllValueInvariant{
		Container: pathdom.NewPath(1, "container"),
		Table:     pathdom.NewPath(2, "table"),
	}
	callOutcome := func(transfer.NodeContext, factflow.CallSiteView, state.State, func(cfg.Point) state.State) callpayload.CallOutcome {
		return callpayload.CallOutcome{PostReturnAuthority: true}
	}
	callOutcomeFactory := func(CallOutcomeContext) callpayload.CallOutcomeProvider {
		return callOutcome
	}
	signatureArgumentType := func(transfer.NodeContext, factflow.ValueSource, state.State, func(cfg.Point) state.State) (typ.Type, bool) {
		return typ.String, true
	}
	signatureArgumentTypeFactory := func(CallOutcomeContext) SignatureArgumentTypeFunc {
		return signatureArgumentType
	}
	wirAssignmentTarget := func(factapply.WIRAssignmentTargetIssue) {}

	config := Config{
		EntryState: entryState,
		Initial: func(cfg.Point) (state.State, bool) {
			return initialState, true
		},
		TypeValues:                   typeValues,
		ClosedDynamicAllValues:       []factapply.ClosedDynamicAllValueInvariant{invariant},
		StateLanes:                   []state.LaneID{state.LaneValues, state.LaneFrozenTables},
		CallOutcome:                  callOutcome,
		CallOutcomeFactory:           callOutcomeFactory,
		SignatureArgumentType:        signatureArgumentType,
		SignatureArgumentTypeFactory: signatureArgumentTypeFactory,
		WIRAssignmentTarget:          wirAssignmentTarget,
		WidenAt: func(cfg.Point) bool {
			return true
		},
		WidenDelay: func(cfg.Point) int {
			return 3
		},
		Stats: stats,
	}

	solve := config.SolveConfig()
	if !solve.EntryState.IsTableFrozen(entryTable) {
		t.Fatal("SolveConfig did not carry EntryState")
	}
	if solve.Initial == nil {
		t.Fatal("SolveConfig did not carry Initial")
	}
	if got, ok := solve.Initial(0); !ok || !got.IsTableFrozen(initialTable) {
		t.Fatalf("SolveConfig Initial returned frozen=%v ok=%v, want frozen initial state", got.IsTableFrozen(initialTable), ok)
	}
	if solve.TypeValues != typeValues {
		t.Fatal("SolveConfig did not carry TypeValues")
	}
	if solve.CallOutcome == nil || solve.CallOutcomeFactory == nil || solve.SignatureArgumentType == nil || solve.SignatureArgumentTypeFactory == nil {
		t.Fatal("SolveConfig dropped a provider axis")
	}
	if solve.WIRAssignmentTarget == nil {
		t.Fatal("SolveConfig did not carry WIRAssignmentTarget")
	}
	if solve.WidenAt == nil || !solve.WidenAt(0) {
		t.Fatal("SolveConfig did not carry WidenAt")
	}
	if solve.WidenDelay == nil || solve.WidenDelay(0) != 3 {
		t.Fatal("SolveConfig did not carry WidenDelay")
	}
	if solve.Stats != stats {
		t.Fatal("SolveConfig did not carry Stats")
	}

	config.StateLanes[0] = state.LaneFrozenTables
	if got := solve.StateLanes[0]; got != state.LaneValues {
		t.Fatalf("SolveConfig StateLanes aliased caller slice: got %q", got)
	}
	config.ClosedDynamicAllValues[0] = factapply.ClosedDynamicAllValueInvariant{}
	if got := solve.ClosedDynamicAllValues[0].Container.String(); got != invariant.Container.String() {
		t.Fatalf("SolveConfig ClosedDynamicAllValues aliased caller slice: got %q", got)
	}
}

func TestSolveConfigAxesAreConfigBacked(t *testing.T) {
	configType := reflect.TypeOf(Config{})
	solveType := reflect.TypeOf(SolveConfig{})
	for i := 0; i < solveType.NumField(); i++ {
		field := solveType.Field(i)
		if _, ok := configType.FieldByName(field.Name); !ok {
			t.Fatalf("SolveConfig field %s has no Config owner; add it to Config or document why it is solve-only", field.Name)
		}
	}
}
