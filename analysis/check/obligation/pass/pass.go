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

// WithRegistry returns p configured with registry. A zero registry is not
// special; callers should pass judgment.DefaultRegistry when they want defaults.
func (p Pass) WithRegistry(registry judgment.Registry) Pass {
	p.registry = registry
	return p
}

// Run emits all valid judgments. Invalid records are dropped at the boundary so
// producer mistakes do not enter shadow-diff baselines as if they were policy.
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
	return out
}
