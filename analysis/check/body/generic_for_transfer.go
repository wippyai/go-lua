package body

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type genericForMembershipAuthority struct {
	facts      factflow.Facts
	typeValues *typevalue.Cache
	resolver   *visibility.Resolver
}

func (s *genericForMembershipAuthority) PrepareGenericForFactorTransaction(ctx transfer.NodeContext, op factapply.GenericForOperation, domain state.ProductDomain) (state.GenericForFactorTransaction, error) {
	if s == nil || s.resolver == nil || ctx.Registry == nil || !domain.Valid() || op.Target() == 0 {
		return state.GenericForFactorTransaction{}, fmt.Errorf("body: generic-for factor authority is incomplete")
	}
	keys := s.resolver.KeySpace()
	target, ok := visibility.AddressAt(s.resolver, ctx.Point, pathdom.Path{Symbol: op.Target()}).VisibleLocalKeyspaceKey()
	if !ok {
		return state.GenericForFactorTransaction{}, fmt.Errorf("body: generic-for target has no structural key")
	}
	config := state.GenericForFactorConfig{
		Keys: keys, VariableIndex: op.VariableIndex(), Target: target, TypeValues: s.typeValues,
	}
	if first := op.FirstTarget(); first != 0 {
		config.FirstTarget, _ = visibility.AddressAt(s.resolver, ctx.Point, pathdom.Path{Symbol: first}).VisibleLocalKeyspaceKey()
	}
	iterator, hasIterator := op.Iterator()
	config.HasIterator = hasIterator
	if hasIterator {
		config.Iterator = iterator.Kind
	}
	if source, present := op.ProtocolSource(0); present && source.Kind == factapply.GenericForSourceCall && source.HasCallPoint && hasIterator {
		if site, siteOK := s.facts.CallSiteView(source.CallPoint); siteOK {
			if sourceIndex, indexOK := effect.ResolveParamIndex(iterator.Source, site.ArgumentSourceCount()); indexOK {
				if arg, argOK := site.ArgumentSourceAt(sourceIndex); argOK {
					if sourcePath, pathOK := valueSourcePath(s.facts, s.resolver, arg); pathOK {
						config.SourceContainer, _ = valueSourcePathKeyspaceKey(s.resolver, ctx.Point, sourcePath)
						config.SourceTable = config.SourceContainer
					}
				}
			}
		}
	}
	return domain.PrepareGenericForFactorTransaction(config)
}

func genericForIteratorSourceTypeProjects(iter iteration.Iterator, variableIndex int, sourceType typ.Type) bool {
	if sourceType == nil {
		return false
	}
	switch variableIndex {
	case 0:
		_, ok := projection.KeyOf(sourceType)
		return ok
	case 1:
		_, ok := projection.ElementOf(sourceType)
		return ok
	default:
		return false
	}
}

func genericForDeclaredPathIteratorSourceType(argSource factflow.ValueSource, facts factflow.Facts, resolver *visibility.Resolver, symbolTypes map[symbol.ID]typ.Type) (typ.Type, bool) {
	p, ok := valueSourcePath(facts, resolver, argSource)
	if !ok || p.Symbol == 0 {
		return nil, false
	}
	rootType, ok := symbolTypes[p.Symbol]
	if !ok || rootType == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return rootType, true
	}
	return luatypeprojection.ApplySegments(rootType, p.Segments)
}
