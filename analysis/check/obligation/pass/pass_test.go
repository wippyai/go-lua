package pass_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	obligationpass "github.com/wippyai/go-lua/analysis/check/obligation/pass"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestPassRunsProducersInOrderAndFiltersInvalidJudgments(t *testing.T) {
	first := fakeProducer{
		name: "first",
		items: []judgment.Judgment{
			callArgJudgment("first", 1),
			{Code: judgment.CodeCallArgType},
		},
	}
	second := fakeProducer{
		name:  "second",
		items: []judgment.Judgment{callArgJudgment("second", 2)},
	}

	got := obligationpass.New(first, nil, second).Run(obligationpass.Context{FunctionKey: "fn"})
	if len(got) != 2 {
		t.Fatalf("judgments = %d, want 2: %#v", len(got), got)
	}
	if got[0].Subject.Key != "first" || got[1].Subject.Key != "second" {
		t.Fatalf("producer order = %q, %q; want first, second", got[0].Subject.Key, got[1].Subject.Key)
	}
}

type fakeProducer struct {
	name  string
	items []judgment.Judgment
}

func (p fakeProducer) Name() string {
	return p.name
}

func (p fakeProducer) Produce(obligationpass.Context) []judgment.Judgment {
	return append([]judgment.Judgment(nil), p.items...)
}

func callArgJudgment(key string, point cfg.Point) judgment.Judgment {
	return judgment.Judgment{
		Code:  judgment.CodeCallArgType,
		Point: point,
		Subject: judgment.SubjectRef{
			FunctionKey: "fn",
			Kind:        judgment.SubjectCallArgument,
			Key:         key,
		},
		Evidence: judgment.EvidenceChain{
			{Kind: judgment.EvidenceAbstractFact},
			{Kind: judgment.EvidenceUserAssertion},
			{Kind: judgment.EvidenceMissingProof},
		},
	}
}
