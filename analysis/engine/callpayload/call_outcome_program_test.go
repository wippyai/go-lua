package callpayload

import (
	"errors"
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func testPrepareCallOutcome(t *testing.T, program CallOutcomeProgram, ctx transfer.NodeContext, site factflow.CallSiteView) CallOutcomeSiteProgram {
	t.Helper()
	prepared, err := program.PrepareSite(ctx, site)
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func testEvaluateCallOutcome(t *testing.T, program CallOutcomeProgram, ctx transfer.NodeContext, site factflow.CallSiteView, input CallOutcomeInput) CallOutcome {
	t.Helper()
	prepared := testPrepareCallOutcome(t, program, ctx, site)
	outcome, err := prepared.Evaluate(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

func TestCallOutcomeProgramCatalogAcceptsEveryRoleAndRejectsOutsideVocabulary(t *testing.T) {
	evaluator := func(transfer.NodeContext, factflow.CallSiteView, CallOutcomeInput) (CallOutcome, error) {
		return CallOutcome{}, nil
	}
	roles := CallOutcomeFieldRoles()
	fields := make([]string, len(roles))
	for index, role := range roles {
		fields[index] = role.FieldName
	}
	program := SealCallOutcomeProgram("totality test", fields, state.LaneSet{}, state.LaneSet{}, nil, nil, evaluator)
	if got := testPrepareCallOutcome(t, program, transfer.NodeContext{}, factflow.CallSiteView{}).Capability().FieldRoles(); !reflect.DeepEqual(got, roles) {
		t.Fatalf("sealed roles = %#v, want canonical catalog %#v", got, roles)
	}
	assertProgramSealPanics(t, []string{"not-a-call-outcome-field"}, evaluator)
	assertProgramSealPanics(t, []string{fields[0], fields[0]}, evaluator)
}

func TestComposeCallOutcomeProgramsUnionsCapabilitiesInCatalogOrder(t *testing.T) {
	evaluator := func(transfer.NodeContext, factflow.CallSiteView, CallOutcomeInput) (CallOutcome, error) {
		return CallOutcome{}, nil
	}
	first := SealCallOutcomeProgram("first", []string{"ParamObligations", "Results"}, state.NewLaneSet(state.LanePathEvidence), state.LaneSet{}, nil, nil, evaluator)
	second := SealCallOutcomeProgram("second", []string{"Results", "NormalReturnFacts"}, state.NewLaneSet(state.LaneTypestates), state.LaneSet{}, func(transfer.NodeContext, factflow.CallSiteView) (CallOutcomeSiteShape, error) {
		return CallOutcomeSiteShape{FieldNames: []string{"NormalReturnFacts"}, InputLanes: state.NewLaneSet(state.LaneTypestates)}, nil
	}, nil, evaluator)
	composed := ComposeCallOutcomePrograms([]CallOutcomeProgram{first, {}, second}, func(_ transfer.NodeContext, left, _ CallOutcome) CallOutcome {
		return left
	})
	prepared := testPrepareCallOutcome(t, composed, transfer.NodeContext{}, factflow.CallSiteView{})
	if prepared.ComponentCount() != 2 {
		t.Fatalf("composed component count = %d, want 2", prepared.ComponentCount())
	}
	for index := 0; index < prepared.ComponentCount(); index++ {
		component, exact := prepared.Component(index)
		if !exact || component.ComponentCount() != 0 {
			t.Fatalf("component %d = exact:%v children:%d, want leaf", index, exact, component.ComponentCount())
		}
	}
	got := prepared.Capability().FieldRoles()
	var names []string
	for _, role := range got {
		names = append(names, role.FieldName)
	}
	want := canonicalProgramRoleNames("Results", "NormalReturnFacts", "ParamObligations")
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("composed roles = %v, want %v", names, want)
	}
	inputs := prepared.Capability().PrimaryInputLanes()
	if !inputs.Has(state.LanePathEvidence) || !inputs.Has(state.LaneTypestates) || inputs.Len() != 2 {
		t.Fatalf("composed input lanes = %v", inputs.IDs())
	}
}

func TestResultOnlyProgramBindsExactlyConsumedResultIndices(t *testing.T) {
	evaluator := func(transfer.NodeContext, factflow.CallSiteView, CallOutcomeInput) (CallOutcome, error) {
		return CallOutcome{Results: []CallResult{{Index: 1}, {Index: 2}, {Index: 3}}}, nil
	}
	program := SealCallOutcomeProgram("result only", []string{"Results"}, state.LaneSet{}, state.LaneSet{}, nil, nil, evaluator)
	site := factflow.NewCallSite(factflow.CallSiteConfig{ResultTargets: []factflow.CallResultTarget{
		factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 3, 0, pathdom.Path{}),
		factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 1, 1, 0, pathdom.Path{}),
		factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 2, 3, 0, pathdom.Path{}),
	}}).View()
	capability := testPrepareCallOutcome(t, program, transfer.NodeContext{}, site).Capability()
	if got, want := capability.ResultIndices(), []int{1, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("result indices = %v, want %v", got, want)
	}
	if !capability.HasField("Results") || capability.HasField("NormalReturnFacts") {
		t.Fatalf("result-only roles = %#v", capability.FieldRoles())
	}
	if got := testEvaluateCallOutcome(t, program, transfer.NodeContext{}, site, CallOutcomeInput{}).Results; len(got) != 3 {
		t.Fatalf("provider result relation = %#v, want all three mathematical results", got)
	}
	nonResult := SealCallOutcomeProgram("diagnostic only", []string{"ParamObligations"}, state.LaneSet{}, state.LaneSet{}, nil, nil, evaluator)
	if got := testPrepareCallOutcome(t, nonResult, transfer.NodeContext{}, site).Capability().ResultIndices(); len(got) != 0 {
		t.Fatalf("non-result capability exposed indices %v", got)
	}
}

func TestCallOutcomeProgramRejectsEvaluatorFieldOutsideSiteCapability(t *testing.T) {
	program := SealCallOutcomeProgram(
		"validation test", []string{"Results"}, state.LaneSet{}, state.LaneSet{},
		func(transfer.NodeContext, factflow.CallSiteView) (CallOutcomeSiteShape, error) {
			return CallOutcomeSiteShape{}, nil
		}, nil,
		func(transfer.NodeContext, factflow.CallSiteView, CallOutcomeInput) (CallOutcome, error) {
			return CallOutcome{Results: []CallResult{{Index: 0}}}, nil
		},
	)
	prepared := testPrepareCallOutcome(t, program, transfer.NodeContext{}, factflow.CallSiteView{})
	if _, err := prepared.Evaluate(transfer.NodeContext{}, CallOutcomeInput{}); err == nil {
		t.Fatal("Evaluate accepted an emitted field outside the site capability")
	}
}

func TestCallOutcomeSiteProgramPreparesOnceAndPropagatesTypedErrors(t *testing.T) {
	shapeCalls := 0
	evaluateCalls := 0
	program := SealCallOutcomeProgram(
		"prepared site test", []string{"Results"}, state.LaneSet{}, state.NewLaneSet(state.LanePathEvidence),
		func(transfer.NodeContext, factflow.CallSiteView) (CallOutcomeSiteShape, error) {
			shapeCalls++
			return CallOutcomeSiteShape{FieldNames: []string{"Results"}}, nil
		},
		func(transfer.NodeContext, factflow.CallSiteView, cfg.Point) (state.LaneSet, error) {
			return state.LaneSet{}, errors.New("read shape failed")
		},
		func(transfer.NodeContext, factflow.CallSiteView, CallOutcomeInput) (CallOutcome, error) {
			evaluateCalls++
			return CallOutcome{}, nil
		},
	)
	prepared, err := program.PrepareSite(transfer.NodeContext{}, factflow.CallSiteView{})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := prepared.Evaluate(transfer.NodeContext{}, CallOutcomeInput{}); err != nil {
			t.Fatal(err)
		}
	}
	if shapeCalls != 1 || evaluateCalls != 2 {
		t.Fatalf("shape/evaluate calls = %d/%d, want 1/2", shapeCalls, evaluateCalls)
	}
	if _, err := prepared.Capability().ReadInputLanes(1); err == nil {
		t.Fatal("ReadInputLanes silently dropped its site-dependent error")
	}

	shapeErr := errors.New("shape failed")
	invalid := SealCallOutcomeProgram(
		"shape error test", nil, state.LaneSet{}, state.LaneSet{},
		func(transfer.NodeContext, factflow.CallSiteView) (CallOutcomeSiteShape, error) {
			return CallOutcomeSiteShape{}, shapeErr
		}, nil,
		func(transfer.NodeContext, factflow.CallSiteView, CallOutcomeInput) (CallOutcome, error) {
			return CallOutcome{}, nil
		},
	)
	if _, err := invalid.PrepareSite(transfer.NodeContext{}, factflow.CallSiteView{}); !errors.Is(err, shapeErr) {
		t.Fatalf("PrepareSite error = %v, want %v", err, shapeErr)
	}
}

func TestCallOutcomeProgramRejectsUnknownInputLaneAtSeal(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("SealCallOutcomeProgram accepted an unregistered input lane")
		}
	}()
	_ = SealCallOutcomeProgram(
		"invalid lane", nil, state.NewLaneSet(state.LaneID("not-registered")), state.LaneSet{}, nil, nil,
		func(transfer.NodeContext, factflow.CallSiteView, CallOutcomeInput) (CallOutcome, error) {
			return CallOutcome{}, nil
		},
	)
}

func assertProgramSealPanics(t *testing.T, fields []string, evaluator func(transfer.NodeContext, factflow.CallSiteView, CallOutcomeInput) (CallOutcome, error)) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("SealCallOutcomeProgram(%v) did not panic", fields)
		}
	}()
	_ = SealCallOutcomeProgram("invalid test", fields, state.LaneSet{}, state.LaneSet{}, nil, nil, evaluator)
}

func canonicalProgramRoleNames(wanted ...string) []string {
	set := make(map[string]struct{}, len(wanted))
	for _, name := range wanted {
		set[name] = struct{}{}
	}
	out := make([]string, 0, len(wanted))
	for _, role := range CallOutcomeFieldRoles() {
		if _, ok := set[role.FieldName]; ok {
			out = append(out, role.FieldName)
		}
	}
	return out
}
