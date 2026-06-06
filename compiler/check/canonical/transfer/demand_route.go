package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func (t *Transfer) demandExprCtx(out *flow.PointState, expr ast.Expr, ctx typ.Type, demand func(int, paramevidence.ParamContract)) {
	if demand == nil || ctx == nil || typ.IsAbsentOrUnknown(ctx) {
		return
	}
	valuePath, ok := t.demandPathForExpr(expr)
	if !ok || valuePath.Symbol == 0 {
		return
	}
	contract := paramevidence.DemandFromType(ctx)
	localContract := t.conditionedLeafContract(out, valuePath, contract)
	t.demandLocalPathContract(out, valuePath, localContract, demand)
}

func (t *Transfer) demandExprCapabilityCtx(
	out *flow.PointState,
	expr ast.Expr,
	cap paramevidence.Capability,
	demand func(int, paramevidence.ParamContract),
) {
	if demand == nil {
		return
	}
	valuePath, ok := t.demandPathForExpr(expr)
	if !ok || valuePath.Symbol == 0 {
		return
	}
	contract := paramevidence.DemandFromCapability(cap)
	localContract := t.conditionedLeafContract(out, valuePath, contract)
	t.demandLocalPathContract(out, valuePath, localContract, demand)
}

func (t *Transfer) demandExprContractCtx(
	out *flow.PointState,
	expr ast.Expr,
	contract paramevidence.ParamContract,
	demand func(int, paramevidence.ParamContract),
) {
	if demand == nil || paramevidence.ParamContractDomain.Equal(contract, paramevidence.ParamContractDomain.Bottom()) {
		return
	}
	valuePath, ok := t.demandPathForExpr(expr)
	if !ok || valuePath.Symbol == 0 {
		return
	}
	localContract := t.conditionedLeafContract(out, valuePath, contract)
	t.demandLocalPathContract(out, valuePath, localContract, demand)
}

func (t *Transfer) demandPathForExpr(expr ast.Expr) (constraint.Path, bool) {
	sym, segments, ok := t.pathSymbol(expr)
	if !ok || sym == 0 {
		return constraint.Path{}, false
	}
	return constraint.Path{
		Symbol:   sym,
		Segments: append([]constraint.Segment(nil), segments...),
	}, true
}

func (t *Transfer) demandParamPathContract(
	out *flow.PointState,
	idx int,
	path constraint.Path,
	contract paramevidence.ParamContract,
	demand func(int, paramevidence.ParamContract),
) {
	if demand == nil || idx < 0 ||
		paramevidence.ParamContractDomain.Equal(contract, paramevidence.ParamContractDomain.Bottom()) {
		return
	}
	if out != nil {
		conditioned, _ := paramevidence.ConditionedPathContract(path, contract, out.Cond)
		demand(idx, conditioned)
		return
	}
	demand(idx, paramevidence.DemandFromPathContract(path.Segments, contract))
}

func (t *Transfer) conditionedLeafContract(
	out *flow.PointState,
	path constraint.Path,
	contract paramevidence.ParamContract,
) paramevidence.ParamContract {
	if out == nil {
		return contract
	}
	conditioned, _ := paramevidence.ConditionedLeafContract(path, contract, out.Cond)
	return conditioned
}

type demandRouteItem struct {
	path     constraint.Path
	contract paramevidence.ParamContract
}

func (t *Transfer) demandLocalPathContract(
	out *flow.PointState,
	path constraint.Path,
	contract paramevidence.ParamContract,
	demand func(int, paramevidence.ParamContract),
) {
	if demand == nil || path.Symbol == 0 ||
		paramevidence.ParamContractDomain.Equal(contract, paramevidence.ParamContractDomain.Bottom()) {
		return
	}
	queue := []demandRouteItem{{path: path, contract: contract}}
	seen := map[constraint.PathKey]struct{}{}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		key := flow.StablePathKey(cur.path)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if idx, isParam := t.paramBySym[cur.path.Symbol]; isParam {
			t.demandParamPathContract(out, idx, cur.path, cur.contract, demand)
		}
		if out == nil {
			continue
		}
		facts := flow.PointFactsOf(*out)
		for _, source := range facts.IdentityAliasSourcePaths(cur.path, flow.IdentityAliasReadPolicy) {
			if source.Symbol != 0 {
				queue = append(queue, demandRouteItem{path: source, contract: cur.contract})
			}
		}
		for _, use := range facts.ValueOriginUsesCoveringPath(cur.path) {
			switch use.Origin.Kind {
			case flow.ValueOriginIndexedIterator:
				t.enqueueAppendFieldOriginDemands(out, use.Origin.Source, use.Remainder, cur.contract, &queue)
				localEvidence := paramevidence.DemandFromPathContract(use.Remainder, cur.contract)
				evidence := iteratorOriginContract(use.Origin, localEvidence)
				if paramevidence.ParamContractDomain.Equal(evidence, paramevidence.ParamContractDomain.Bottom()) {
					continue
				}
				if source, ok := use.Origin.SourcePath(); ok && source.Symbol != 0 {
					queue = append(queue, demandRouteItem{path: source, contract: evidence})
				}
			case flow.ValueOriginKeyedIterator:
				localEvidence := paramevidence.DemandFromPathContract(use.Remainder, cur.contract)
				evidence := iteratorOriginContract(use.Origin, localEvidence)
				if paramevidence.ParamContractDomain.Equal(evidence, paramevidence.ParamContractDomain.Bottom()) {
					continue
				}
				if source, ok := use.Origin.SourcePath(); ok && source.Symbol != 0 {
					queue = append(queue, demandRouteItem{path: source, contract: evidence})
				}
			}
		}
	}
}

func (t *Transfer) enqueueAppendFieldOriginDemands(
	out *flow.PointState,
	array constraint.PathKey,
	field []constraint.Segment,
	contract paramevidence.ParamContract,
	queue *[]demandRouteItem,
) {
	if out == nil || array == "" || len(field) == 0 || queue == nil ||
		paramevidence.ParamContractDomain.Equal(contract, paramevidence.ParamContractDomain.Bottom()) {
		return
	}
	for _, use := range flow.AppendElementFieldSources(*out, array, field) {
		source, ok := use.SourcePath()
		if !ok || source.Symbol == 0 {
			continue
		}
		if len(use.SourceField) > 0 {
			sourceField := append([]constraint.Segment(nil), use.SourceField...)
			sourceField = append(sourceField, use.FieldRemainder...)
			evidence := paramevidence.DemandFromSequenceElement(paramevidence.DemandFromPathContract(sourceField, contract))
			*queue = append(*queue, demandRouteItem{path: source, contract: evidence})
			continue
		}
		for _, seg := range use.FieldRemainder {
			source = source.Append(seg)
		}
		*queue = append(*queue, demandRouteItem{path: source, contract: contract})
	}
}

func iteratorOriginEvidence(origin flow.ValueOriginFact, local typ.Type) typ.Type {
	switch origin.Kind {
	case flow.ValueOriginIndexedIterator:
		return paramevidence.IndexedIteratorEvidence(origin.VarIndex, local)
	case flow.ValueOriginKeyedIterator:
		return paramevidence.KeyedIteratorEvidence(origin.VarIndex, local)
	default:
		return nil
	}
}

func iteratorOriginContract(origin flow.ValueOriginFact, local paramevidence.ParamContract) paramevidence.ParamContract {
	switch origin.Kind {
	case flow.ValueOriginIndexedIterator:
		return paramevidence.IndexedIteratorContract(origin.VarIndex, local)
	case flow.ValueOriginKeyedIterator:
		return paramevidence.KeyedIteratorContract(origin.VarIndex, local)
	default:
		return paramevidence.ParamContractDomain.Bottom()
	}
}
