package pass_test

import (
	"testing"

	testutil "github.com/wippyai/go-lua/analysis/check/checktest"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	lifecyclefx "github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestLifecycleObligationsRefutesOpenResourceAtExit(t *testing.T) {
	checked := testutil.CheckFile(`local tx = {}
begin(tx)`, "test.lua", testutil.WithManifest("lifecycle", lifecyclePassManifest()), testutil.WithGlobals("begin"))

	got := lifecycleJudgmentsForAllBodies(checked)
	if len(got) != 1 {
		t.Fatalf("lifecycle judgments = %d, want 1: %#v", len(got), got)
	}
	if got[0].Code != judgment.CodeLifecycle {
		t.Fatalf("code = %q, want %q", got[0].Code, judgment.CodeLifecycle)
	}
	if got[0].Verdict != judgment.VerdictRefuted {
		t.Fatalf("verdict = %v, want refuted", got[0].Verdict)
	}
	if got[0].Subject.Label != "tx" {
		t.Fatalf("subject label = %q, want tx", got[0].Subject.Label)
	}
	if !judgmentHasEvidenceDetail(got[0], judgment.EvidenceDetailLifecycleAcquire) ||
		!judgmentHasEvidence(got[0], judgment.EvidenceMissingProof) {
		t.Fatalf("evidence = %#v, want acquire plus missing proof", got[0].Evidence)
	}
}

func lifecycleJudgmentsForAllBodies(checked testutil.Result) []judgment.Judgment {
	var out []judgment.Judgment
	for _, result := range checked.BodyResults() {
		out = append(out, obligationpass.New(obligationpass.LifecycleObligations{}).Run(obligationpass.Context{
			FunctionKey: "fixture:lifecycle",
			SourceFile:  "test.lua",
			Reader:      readmodel.New(result),
		})...)
	}
	return out
}

func lifecyclePassManifest() *manifest.Manifest {
	m := manifest.New("lifecycle")
	if err := m.DefineTypestateProtocol(typestate.Definition{
		Protocol:    typestate.Protocol("transaction"),
		States:      []typestate.State{typestate.State("active"), typestate.State("finished")},
		FinalStates: []typestate.State{typestate.State("finished")},
		Transitions: []typestate.TransitionDecl{{
			From: typestate.State("active"),
			To:   typestate.State("finished"),
		}},
	}); err != nil {
		panic(err)
	}
	m.DefineFunctionSignature("begin", signature.Function{
		Type: typ.Func().
			Param("tx", typ.Any).
			Build(),
		Effect: effect.Row{Labels: []effect.Label{lifecyclefx.Acquire{
			Target:   effect.ParamRef{Index: 0},
			Protocol: typestate.Protocol("transaction"),
			State:    typestate.State("active"),
			Obligation: typestate.Obligation{
				Final: typestate.State("finished"),
			},
		}}},
	})
	return m
}
