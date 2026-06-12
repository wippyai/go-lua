package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
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

// SignatureNoNormalReturnConfig carries signature lookup plus the generic fact
// model needed to mark calls that cannot complete normally.
type SignatureNoNormalReturnConfig struct {
	Graph      cfg.Graph
	Registry   *axis.Registry
	Signatures SignatureLookup
	NameFor    NameFunc
	Facts      factflow.Facts
}

// SummaryPostconditionConfig carries summary lookup plus the generic fact model
// needed to lower function-body-derived normal-return postconditions.
type SummaryPostconditionConfig struct {
	Graph     cfg.Graph
	Registry  *axis.Registry
	Summaries summary.Reader
	KeyFor    KeyFunc
	Facts     factflow.Facts
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

// WithSignatureNoNormalReturns returns Facts extended with call points whose
// declared signature has no normal return value.
func WithSignatureNoNormalReturns(config SignatureNoNormalReturnConfig) factflow.Facts {
	if config.Graph == nil || config.Registry == nil || config.Signatures == nil || config.NameFor == nil {
		return config.Facts
	}
	points := signatureNoNormalReturnFacts(config)
	return config.Facts.WithNoNormalReturns(points)
}

// WithSummaryPostconditions returns Facts extended with generic postcondition
// facts lowered from solved function-body summaries.
func WithSummaryPostconditions(config SummaryPostconditionConfig) factflow.Facts {
	if config.Graph == nil || config.Registry == nil || config.Summaries == nil || config.KeyFor == nil {
		return config.Facts
	}
	facts := summaryPostconditionFacts(config)
	out := config.Facts.WithPostconditionRefinements(facts.refinements)
	out = out.WithPostconditionPathRelations(facts.pathRelations)
	return out.WithBranchRefinementSets(facts.branchRefinements)
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

func signatureNoNormalReturnFacts(config SignatureNoNormalReturnConfig) map[cfg.Point]struct{} {
	out := make(map[cfg.Point]struct{})
	for _, point := range config.Graph.RPO() {
		site, ok := config.Facts.CallSite(point)
		if !ok {
			continue
		}
		name, ok := config.NameFor(transfer.NodeContext{
			Graph:    config.Graph,
			Registry: config.Registry,
			Point:    point,
			Node:     config.Graph.Node(point),
		}, callProducerForSite(site))
		if !ok {
			continue
		}
		sig, ok := config.Signatures.Lookup(name)
		if !ok || !signatureHasNoNormalReturn(sig) {
			continue
		}
		out[point] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type summaryPostconditionFactsResult struct {
	refinements       map[cfg.Point]factflow.PostconditionRefinementSet
	pathRelations     map[cfg.Point]factflow.PostconditionPathRelationSet
	branchRefinements map[cfg.Point]factflow.BranchRefinementSet
}

func summaryPostconditionFacts(config SummaryPostconditionConfig) summaryPostconditionFactsResult {
	out := summaryPostconditionFactsResult{
		refinements:       make(map[cfg.Point]factflow.PostconditionRefinementSet),
		pathRelations:     make(map[cfg.Point]factflow.PostconditionPathRelationSet),
		branchRefinements: make(map[cfg.Point]factflow.BranchRefinementSet),
	}
	for _, point := range config.Graph.RPO() {
		site, ok := config.Facts.CallSite(point)
		if !ok {
			continue
		}
		key, ok := config.KeyFor(transfer.NodeContext{
			Graph:    config.Graph,
			Registry: config.Registry,
			Point:    point,
			Node:     config.Graph.Node(point),
		}, callProducerForSite(site))
		if !ok {
			continue
		}
		got, ok := config.Summaries.Read(key)
		if !ok {
			continue
		}
		for i, value := range got.NormalReturnParams {
			refinement, ok := normalReturnSummaryPostcondition(config, site, i, value)
			if ok {
				appendPostconditionRefinements(out.refinements, point, refinement)
			}
		}
		for i, condition := range got.NormalReturnParamConditions {
			if !condition.IsUseful() {
				continue
			}
			refinements, relations, ok := normalReturnSummaryConditionPostconditions(config, site, i, condition)
			if !ok {
				continue
			}
			appendPostconditionRefinements(out.refinements, point, refinements...)
			appendPostconditionPathRelations(out.pathRelations, point, relations...)
		}
		for _, equality := range got.NormalReturnParamEqualities {
			relation, ok := normalReturnSummaryParamEquality(config, site, equality)
			if ok {
				appendPostconditionPathRelations(out.pathRelations, point, relation)
			}
		}
		for _, refinement := range got.ReturnConditionParamRefinements {
			branch, lowered, ok := returnConditionSummaryBranchRefinement(config, point, site, refinement)
			if ok {
				appendBranchRefinements(out.branchRefinements, branch, lowered)
			}
		}
	}
	if len(out.refinements) == 0 {
		out.refinements = nil
	}
	if len(out.pathRelations) == 0 {
		out.pathRelations = nil
	}
	if len(out.branchRefinements) == 0 {
		out.branchRefinements = nil
	}
	return out
}

func signatureHasNoNormalReturn(sig signature.Function) bool {
	return sig.Type != nil && len(sig.Type.Returns) == 1 && typ.IsNever(sig.Type.Returns[0])
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

func normalReturnSummaryPostcondition(
	config SummaryPostconditionConfig,
	site factflow.CallSite,
	paramIndex int,
	value product.Value,
) (factflow.PostconditionRefinement, bool) {
	if !usefulPostconditionConstraint(config.Registry, value) {
		return factflow.PostconditionRefinement{}, false
	}
	args := site.ArgumentSources()
	if paramIndex < 0 || paramIndex >= len(args) {
		return factflow.PostconditionRefinement{}, false
	}
	arg := args[paramIndex]
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		return factflow.PostconditionRefinement{}, false
	}
	targetPath, ok := config.Facts.ExpressionPath(arg.ExprRef)
	if !ok || targetPath.IsEmpty() {
		return factflow.PostconditionRefinement{}, false
	}
	return factflow.NewPostconditionRefinement(targetPath, factflow.NewValueConstraint(value)), true
}

func normalReturnSummaryConditionPostconditions(
	config SummaryPostconditionConfig,
	site factflow.CallSite,
	paramIndex int,
	condition summary.ParamCondition,
) ([]factflow.PostconditionRefinement, []factflow.PostconditionPathRelation, bool) {
	args := site.ArgumentSources()
	if paramIndex < 0 || paramIndex >= len(args) {
		return nil, nil, false
	}
	arg := args[paramIndex]
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		return nil, nil, false
	}
	expressionCondition, ok := config.Facts.ExpressionCondition(arg.ExprRef)
	if !ok {
		return nil, nil, false
	}
	value := condition == summary.ParamConditionTruthy
	refinements := expressionCondition.RefinementsForValue(value)
	relations := expressionCondition.PathRelationsForValue(value)
	if len(refinements) == 0 && len(relations) == 0 {
		return nil, nil, false
	}
	return refinements, relations, true
}

func normalReturnSummaryParamEquality(
	config SummaryPostconditionConfig,
	site factflow.CallSite,
	equality summary.ParamEquality,
) (factflow.PostconditionPathRelation, bool) {
	left, ok := normalReturnSummaryParamPath(config, site, equality.Left)
	if !ok {
		return factflow.PostconditionPathRelation{}, false
	}
	right, ok := normalReturnSummaryParamPath(config, site, equality.Right)
	if !ok {
		return factflow.PostconditionPathRelation{}, false
	}
	if left.Equal(right) {
		return factflow.PostconditionPathRelation{}, false
	}
	return factflow.NewPostconditionPathEquality(left, right), true
}

func normalReturnSummaryParamPath(
	config SummaryPostconditionConfig,
	site factflow.CallSite,
	paramIndex int,
) (pathdom.Path, bool) {
	args := site.ArgumentSources()
	if paramIndex < 0 || paramIndex >= len(args) {
		return pathdom.Path{}, false
	}
	arg := args[paramIndex]
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		return pathdom.Path{}, false
	}
	targetPath, ok := config.Facts.ExpressionPath(arg.ExprRef)
	if !ok || targetPath.IsEmpty() {
		return pathdom.Path{}, false
	}
	return targetPath, true
}

func returnConditionSummaryBranchRefinement(
	config SummaryPostconditionConfig,
	point cfg.Point,
	site factflow.CallSite,
	refinement summary.ReturnConditionParamRefinement,
) (cfg.Point, factflow.BranchRefinement, bool) {
	if site.Context() != factflow.CallSiteContextCondition || refinement.ReturnIndex != 0 {
		return 0, factflow.BranchRefinement{}, false
	}
	branch, ok := conditionCallBranchPoint(config.Graph, point)
	if !ok {
		return 0, factflow.BranchRefinement{}, false
	}
	targetPath, ok := substituteSummaryParamPath(config, site, refinement.Target)
	if !ok {
		return 0, factflow.BranchRefinement{}, false
	}
	value := factflow.NewValueConstraint(refinement.Value)
	if refinement.ReturnValue {
		return branch, factflow.NewBranchRefinement(targetPath, value, true, factflow.ValueRefinement{}, false), true
	}
	return branch, factflow.NewBranchRefinement(targetPath, factflow.ValueRefinement{}, false, value, true), true
}

func substituteSummaryParamPath(
	config SummaryPostconditionConfig,
	site factflow.CallSite,
	target pathdom.Path,
) (pathdom.Path, bool) {
	args := site.ArgumentSources()
	paths := make([]pathdom.Path, len(args))
	for i, arg := range args {
		if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
			continue
		}
		p, ok := config.Facts.ExpressionPath(arg.ExprRef)
		if !ok || p.IsEmpty() {
			continue
		}
		paths[i] = p
	}
	return target.Substitute(paths)
}

func conditionCallBranchPoint(graph cfg.Graph, point cfg.Point) (cfg.Point, bool) {
	if graph == nil {
		return 0, false
	}
	if graph.IsBranch(point) {
		return point, true
	}
	successors := graph.Successors(point)
	if len(successors) != 1 {
		return 0, false
	}
	branch := successors[0]
	if !graph.IsBranch(branch) {
		return 0, false
	}
	return branch, true
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

func usefulPostconditionConstraint(reg *axis.Registry, value product.Value) bool {
	return !product.Equal(reg, value, product.Bottom(reg)) && !product.Equal(reg, value, product.Top())
}

func appendPostconditionRefinements(
	out map[cfg.Point]factflow.PostconditionRefinementSet,
	point cfg.Point,
	refinements ...factflow.PostconditionRefinement,
) {
	if len(refinements) == 0 {
		return
	}
	existing := out[point].Refinements()
	existing = append(existing, refinements...)
	out[point] = factflow.NewPostconditionRefinementSet(existing...)
}

func appendPostconditionPathRelations(
	out map[cfg.Point]factflow.PostconditionPathRelationSet,
	point cfg.Point,
	relations ...factflow.PostconditionPathRelation,
) {
	if len(relations) == 0 {
		return
	}
	existing := out[point].Relations()
	existing = append(existing, relations...)
	out[point] = factflow.NewPostconditionPathRelationSet(existing...)
}

func appendBranchRefinements(
	out map[cfg.Point]factflow.BranchRefinementSet,
	point cfg.Point,
	refinements ...factflow.BranchRefinement,
) {
	if len(refinements) == 0 {
		return
	}
	existing := out[point].Refinements()
	existing = append(existing, refinements...)
	out[point] = factflow.NewBranchRefinementSet(existing...)
}
