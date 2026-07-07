// Package pass runs post-solve obligation producers over the canonical read
// model. Producers in this package tree must not reach back into body.Result or
// syntax; missing data belongs in the read model or in lowering-time facts.
package pass

import (
	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Context is the only semantic input an obligation producer receives.
type Context struct {
	FunctionKey                   string
	SourceFile                    string
	Reader                        readmodel.Reader
	SuppressCallerOwnedParameters bool
	PointReachable                func(cfg.Point) bool
}

// Producer emits one family of semantic judgments from a solved body view.
type Producer interface {
	Name() string
	Produce(Context) []judgment.Judgment
}

// Pass assembles post-solve obligation producers in deterministic order.
type Pass struct {
	producers []Producer
	registry  judgment.Registry
}

// New builds a pass from producers. The producer order is the output order.
func New(producers ...Producer) Pass {
	return Pass{
		producers: append([]Producer(nil), producers...),
		registry:  judgment.DefaultRegistry(),
	}
}

// Run emits all valid judgments. Invalid records are dropped at the boundary so
// producer mistakes do not enter rendering, deduplication, or policy as if they
// were valid facts.
func (p Pass) Run(ctx Context) []judgment.Judgment {
	var out []judgment.Judgment
	for _, producer := range p.producers {
		if producer == nil {
			continue
		}
		for _, item := range producer.Produce(ctx) {
			if item.Code == "" || !p.registry.Validate(item) {
				continue
			}
			if ctx.PointReachable != nil && !ctx.PointReachable(item.Point) {
				continue
			}
			out = append(out, item)
		}
	}
	return suppressJudgmentCascades(out)
}

func suppressJudgmentCascades(in []judgment.Judgment) []judgment.Judgment {
	if len(in) < 2 {
		return in
	}
	invalidAssignments := make(map[string]cfg.Point)
	for _, item := range in {
		if item.Code == judgment.CodeAssignment && item.Subject.Label != "" {
			invalidAssignments[item.Subject.Label] = item.Point
		}
	}
	out := in[:0]
	for _, item := range in {
		if item.Code == judgment.CodeReturn && item.Actual.Label != "" {
			if causePoint, ok := invalidAssignments[item.Actual.Label]; ok && causePoint < item.Point {
				continue
			}
		}
		if item.Code == judgment.CodeConcatOperand && item.Subject.Label != "" {
			if causePoint, ok := invalidAssignments[item.Subject.Label]; ok && causePoint < item.Point {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}
