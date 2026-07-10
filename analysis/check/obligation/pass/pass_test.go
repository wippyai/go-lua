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

func TestPassFiltersUnreachableJudgments(t *testing.T) {
	producer := fakeProducer{
		name: "reachability",
		items: []judgment.Judgment{
			callArgJudgment("reachable", 1),
			callArgJudgment("unreachable", 2),
		},
	}

	got := obligationpass.New(producer).Run(obligationpass.Context{
		FunctionKey: "fn",
		PointReachable: func(point cfg.Point) bool {
			return point == 1
		},
	})
	if len(got) != 1 || got[0].Subject.Key != "reachable" {
		t.Fatalf("judgments = %#v, want only reachable point", got)
	}
}

func TestPassSuppressesReturnCascadeFromInvalidAssignment(t *testing.T) {
	producer := fakeProducer{
		name: "cascade",
		items: []judgment.Judgment{
			assignmentJudgment("x", 3),
			returnJudgment("x", 4),
			returnJudgment("y", 5),
		},
	}

	got := obligationpass.New(producer).Run(obligationpass.Context{FunctionKey: "fn"})
	if len(got) != 2 {
		t.Fatalf("judgments = %d, want 2: %#v", len(got), got)
	}
	if got[0].Code != judgment.CodeAssignment || got[1].Code != judgment.CodeReturn || got[1].Actual.Label != "y" {
		t.Fatalf("cascade suppression kept wrong judgments: %#v", got)
	}
}

func TestPassSuppressesConcatCascadeFromInvalidAssignment(t *testing.T) {
	producer := fakeProducer{
		name: "cascade",
		items: []judgment.Judgment{
			concatJudgment("user_id", 4),
			assignmentJudgment("user_id", 3),
			concatJudgment("room_id", 5),
		},
	}

	got := obligationpass.New(producer).Run(obligationpass.Context{FunctionKey: "fn"})
	if len(got) != 2 {
		t.Fatalf("judgments = %d, want 2: %#v", len(got), got)
	}
	if got[0].Code != judgment.CodeAssignment || got[1].Code != judgment.CodeConcatOperand || got[1].Subject.Label != "room_id" {
		t.Fatalf("cascade suppression kept wrong judgments: %#v", got)
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

func assignmentJudgment(label string, point cfg.Point) judgment.Judgment {
	return judgment.Judgment{
		Code:  judgment.CodeAssignment,
		Point: point,
		Subject: judgment.SubjectRef{
			FunctionKey: "fn",
			Kind:        judgment.SubjectPath,
			Key:         "assign:" + label,
			Label:       label,
		},
		Evidence: judgment.EvidenceChain{
			{Kind: judgment.EvidenceAbstractFact},
			{Kind: judgment.EvidenceUserAssertion},
			{Kind: judgment.EvidenceMissingProof},
		},
	}
}

func returnJudgment(sourceLabel string, point cfg.Point) judgment.Judgment {
	return judgment.Judgment{
		Code:  judgment.CodeReturn,
		Point: point,
		Subject: judgment.SubjectRef{
			FunctionKey: "fn",
			Kind:        judgment.SubjectReturnValue,
			Key:         "return:" + sourceLabel,
		},
		Actual: judgment.ValueRef{Label: sourceLabel},
		Evidence: judgment.EvidenceChain{
			{Kind: judgment.EvidenceAbstractFact},
			{Kind: judgment.EvidenceUserAssertion},
			{Kind: judgment.EvidenceMissingProof},
		},
	}
}

func concatJudgment(label string, point cfg.Point) judgment.Judgment {
	return judgment.Judgment{
		Code:  judgment.CodeConcatOperand,
		Point: point,
		Subject: judgment.SubjectRef{
			FunctionKey: "fn",
			Kind:        judgment.SubjectExpression,
			Key:         "concat:" + label,
			Label:       label,
		},
		Actual: judgment.ValueRef{Label: label},
		Evidence: judgment.EvidenceChain{
			{
				Kind: judgment.EvidenceAbstractFact,
				Detail: judgment.EvidenceDetail{
					Kind:  judgment.EvidenceDetailConcatOperand,
					Field: "left",
				},
			},
		},
		Spans: []judgment.SpanRef{{StartLine: int(point), StartCol: 1, EndLine: int(point), EndCol: 2}},
	}
}
