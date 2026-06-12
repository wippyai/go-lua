package callresult

import (
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// SignaturePostconditionConfig carries signature lookup plus the generic fact
// model needed to lower declared normal-return postconditions.
type SignaturePostconditionConfig struct {
	Graph      cfg.Graph
	Registry   *axis.Registry
	Signatures SignatureLookup
	NameFor    NameFunc
	Facts      factflow.Facts
}

// WithSignaturePostconditions returns Facts extended with generic
// postcondition facts lowered from declared signature effects.
func WithSignaturePostconditions(config SignaturePostconditionConfig) factflow.Facts {
	if config.Graph == nil || config.Registry == nil || config.Signatures == nil || config.NameFor == nil {
		return config.Facts
	}
	refinements := signaturePostconditionFacts(config)
	return config.Facts.WithPostconditionRefinements(refinements)
}

func signaturePostconditionFacts(config SignaturePostconditionConfig) map[cfg.Point]factflow.PostconditionRefinementSet {
	out := make(map[cfg.Point]factflow.PostconditionRefinementSet)
	for _, point := range config.Graph.RPO() {
		site, ok := config.Facts.CallSite(point)
		if !ok {
			continue
		}
		sig, ok := signatureForSite(config, point, site)
		if !ok {
			continue
		}
		for _, label := range normalReturnRefinementLabels(sig) {
			refinement, ok := normalReturnPostcondition(config, site, label)
			if ok {
				appendPostconditionRefinements(out, point, refinement)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func signatureForSite(config SignaturePostconditionConfig, point cfg.Point, site factflow.CallSite) (signature.Function, bool) {
	name, ok := config.NameFor(transfer.NodeContext{
		Graph: config.Graph,
		Point: point,
		Node:  config.Graph.Node(point),
	}, callProducerForSite(site))
	if !ok {
		return signature.Function{}, false
	}
	return config.Signatures.Lookup(name)
}

func callProducerForSite(site factflow.CallSite) factflow.CallProducer {
	expr, hasExpr := site.Expr()
	return factflow.NewCallProducer(factflow.CallProducerConfig{
		CalleeSymbol:  site.CalleeSymbol(),
		CalleePath:    site.CalleePath(),
		ExprRef:       expr,
		HasExpr:       hasExpr,
		ExprIndex:     site.ExprIndex(),
		ResultTargets: site.ResultTargets(),
		Final:         site.Final(),
		Expanded:      site.Expanded(),
		Adjusted:      site.Adjusted(),
		OpenTail:      site.OpenTail(),
	})
}

func normalReturnRefinementLabels(sig signature.Function) []postcondition.NormalReturnRefinement {
	if len(sig.Effect.Labels) == 0 {
		return nil
	}
	out := make([]postcondition.NormalReturnRefinement, 0, len(sig.Effect.Labels))
	for _, label := range sig.Effect.Labels {
		switch normalized := effect.NormalizeLabel(label).(type) {
		case postcondition.NormalReturnRefinement:
			out = append(out, normalized)
		case *postcondition.NormalReturnRefinement:
			if normalized != nil {
				out = append(out, *normalized)
			}
		}
	}
	return out
}

func normalReturnPostcondition(
	config SignaturePostconditionConfig,
	site factflow.CallSite,
	label postcondition.NormalReturnRefinement,
) (factflow.PostconditionRefinement, bool) {
	args := site.ArgumentSources()
	argIndex, ok := effect.ResolveParamIndex(label.Target, len(args))
	if !ok {
		return factflow.PostconditionRefinement{}, false
	}
	arg := args[argIndex]
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		return factflow.PostconditionRefinement{}, false
	}
	targetPath, ok := config.Facts.ExpressionPath(arg.ExprRef)
	if !ok || targetPath.IsEmpty() {
		return factflow.PostconditionRefinement{}, false
	}
	value, ok := postconditionRefinementValue(config.Registry, label.Refinement)
	if !ok {
		return factflow.PostconditionRefinement{}, false
	}
	return factflow.NewPostconditionRefinement(targetPath, value), true
}

func postconditionRefinementValue(reg *axis.Registry, refinement postcondition.Refinement) (factflow.ValueRefinement, bool) {
	switch r := refinement.(type) {
	case postcondition.Present:
		return presentPostconditionRefinement(reg), true
	case *postcondition.Present:
		if r != nil {
			return presentPostconditionRefinement(reg), true
		}
	}
	return factflow.ValueRefinement{}, false
}

func presentPostconditionRefinement(reg *axis.Registry) factflow.ValueRefinement {
	return factflow.NewValueConstraint(product.NewWithPresence(reg, product.ShapeTop, presence.Present()))
}

func appendPostconditionRefinements(
	out map[cfg.Point]factflow.PostconditionRefinementSet,
	point cfg.Point,
	refinements ...factflow.PostconditionRefinement,
) {
	existing := out[point].Refinements()
	existing = append(existing, refinements...)
	out[point] = factflow.NewPostconditionRefinementSet(existing...)
}
