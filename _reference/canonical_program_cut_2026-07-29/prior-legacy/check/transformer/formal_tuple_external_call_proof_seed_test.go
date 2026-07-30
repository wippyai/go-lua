package transformer

import (
	"reflect"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
)

func TestFormalExternalCallProofSeedsKeepProvidersSeparate(t *testing.T) {
	a := callpayload.NormalReturnPathPresenceProofSeed(pathdom.NewPlaceholder(0), presence.Absent())
	b := callpayload.NormalReturnPathPresenceProofSeed(pathdom.NewPlaceholder(1), presence.Present())
	provider := callpayload.ComposeCallOutcomePrograms([]callpayload.CallOutcomeProgram{proofSeedProgram(a), proofSeedProgram(b)}, proofSeedMerge)
	prepared := prepareProofSeedProvider(t, provider)
	bindings := callboundary.NewPathBindings([]pathdom.Path{
		pathdom.NewPath(101, "p"), pathdom.NewPath(103, "q"),
	}, nil)
	got, err := freezeFormalExternalCallProofSeeds(7, 11, prepared, bindings, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !reflect.DeepEqual(got[0].path, []uint32{0}) || !reflect.DeepEqual(got[1].path, []uint32{1}) {
		t.Fatalf("provider paths = %#v", got)
	}
	if !got[0].refinement.Equal(pathdom.NewPath(101, "p")) {
		t.Fatalf("provider A binding = %#v", got[0])
	}
	if !got[1].refinement.Equal(pathdom.NewPath(103, "q")) {
		t.Fatalf("provider B binding = %#v", got[1])
	}
}

func TestFormalExternalCallProofSeedsKeepOccurrencesSeparate(t *testing.T) {
	seed := callpayload.NormalReturnPathPresenceProofSeed(pathdom.NewPlaceholder(0), presence.Present())
	prepared := prepareProofSeedProvider(t, proofSeedProgram(seed))
	p, err := freezeFormalExternalCallProofSeeds(3, 17, prepared, callboundary.NewPathBindings([]pathdom.Path{pathdom.NewPath(201, "p")}, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	q, err := freezeFormalExternalCallProofSeeds(3, 19, prepared, callboundary.NewPathBindings([]pathdom.Path{pathdom.NewPath(203, "q")}, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != 1 || len(q) != 1 || p[0].point == q[0].point || p[0].refinement.Equal(q[0].refinement) {
		t.Fatalf("occurrence bindings leaked: p=%#v q=%#v", p, q)
	}
}

func TestFormalExternalCallProofSeedsDoNotInventSiblingCoordinates(t *testing.T) {
	prepared := prepareProofSeedProvider(t, proofSeedProgramWithoutSeed())
	got, err := freezeFormalExternalCallProofSeeds(3, 17, prepared, callboundary.NewPathBindings([]pathdom.Path{pathdom.NewPath(201, "p")}, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("absent proof declared sibling bindings: %#v", got)
	}
}

func TestFormalExternalCallProofSeedsPreserveNestedMergeOrder(t *testing.T) {
	a := callpayload.NormalReturnPathPresenceProofSeed(pathdom.NewPlaceholder(0), presence.Absent())
	b := callpayload.NormalReturnPathPresenceProofSeed(pathdom.NewPlaceholder(1), presence.Present())
	provider := callpayload.ComposeCallOutcomePrograms([]callpayload.CallOutcomeProgram{
		callpayload.ComposeCallOutcomePrograms([]callpayload.CallOutcomeProgram{proofSeedProgram(a), proofSeedProgram(b)}, proofSeedMerge),
		proofSeedProgram(a),
	}, proofSeedMerge)
	got, err := freezeFormalExternalCallProofSeeds(3, 17, prepareProofSeedProvider(t, provider), callboundary.NewPathBindings([]pathdom.Path{pathdom.NewPath(201, "p"), pathdom.NewPath(202, "value_p")}, nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([][]uint32, len(got))
	for index := range got {
		paths[index] = got[index].path
	}
	if want := [][]uint32{{0, 0}, {0, 1}, {1}}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("nested provider seed paths = %#v, want %#v", paths, want)
	}
}

func prepareProofSeedProvider(t *testing.T, program callpayload.CallOutcomeProgram) callpayload.CallOutcomeSiteProgram {
	t.Helper()
	provider, err := program.PrepareSite(transfer.NodeContext{}, proofSeedCallSite())
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func proofSeedProgram(seed callpayload.CallOutcomeProofSeed) callpayload.CallOutcomeProgram {
	return callpayload.SealCallOutcomeProgram("formal proof seed", []string{"NormalReturnFacts"}, state.LaneSet{}, state.LaneSet{},
		func(transfer.NodeContext, factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
			return callpayload.CallOutcomeSiteShape{FieldNames: []string{"NormalReturnFacts"}, ProofSeeds: []callpayload.CallOutcomeProofSeed{seed}}, nil
		}, nil,
		func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
			return callpayload.CallOutcome{}, nil
		},
	)
}

func proofSeedProgramWithoutSeed() callpayload.CallOutcomeProgram {
	return callpayload.SealCallOutcomeProgram("formal proof seed absent", []string{"NormalReturnFacts"}, state.LaneSet{}, state.LaneSet{},
		func(transfer.NodeContext, factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
			return callpayload.CallOutcomeSiteShape{FieldNames: []string{"NormalReturnFacts"}}, nil
		}, nil,
		func(transfer.NodeContext, factflow.CallSiteView, callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
			return callpayload.CallOutcome{}, nil
		},
	)
}

func proofSeedMerge(_ transfer.NodeContext, left, _ callpayload.CallOutcome) callpayload.CallOutcome {
	return left
}

func proofSeedCallSite() factflow.CallSiteView {
	return factflow.NewCallSite(factflow.CallSiteConfig{ResultTargets: []factflow.CallResultTarget{
		factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 0, 0, 0, pathdom.Path{}),
		factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 1, 1, 0, pathdom.Path{}),
		factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 2, 2, 0, pathdom.Path{}),
		factflow.NewCallResultTarget(factflow.CallResultTargetExpression, 3, 3, 0, pathdom.Path{}),
	}}).View()
}
