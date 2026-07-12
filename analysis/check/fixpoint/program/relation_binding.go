package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/relationcall"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// relationResolverFactory is inactive run-local infrastructure. It owns a
// generation-fenced route catalog, but does not change the production call
// provider or cache policy until the whole-function differential gate passes.
type relationResolverFactory func(body.CallOutcomeContext) relationcall.Resolver

// inactiveRelationResolverFactory binds one exact query owner to the frozen
// relations minted by the same run. Independently prepared bodies, generation
// tokens, point tables, shapes, and producer identities all fail closed.
func (s relationRunSnapshot) inactiveRelationResolverFactory(owner relationConsumerIdentity) (relationResolverFactory, bool) {
	direct, ok := s.DirectCalls(owner)
	if !ok || owner.Prepared == nil || owner.Prepared.IdentityDigest() != owner.BodyDigest {
		return nil, false
	}
	plan := owner.Prepared.OperationPlan()
	if plan == nil || direct.PointCount() != plan.PointCount() {
		return nil, false
	}
	routes := make([]relationcall.Route, 0, len(direct.Cells()))
	for raw := 0; raw < direct.PointCount(); raw++ {
		point := cfg.Point(raw)
		target, found := direct.Lookup(point)
		if !found {
			continue
		}
		identity, found := s.Identity(target.Cell)
		if !found || identity.Generation != owner.Generation {
			return nil, false
		}
		relation, found := s.Lookup(identity)
		if !found || relation.Shape() != target.Shape || !paramsOnlyShape(relation.Shape()) || relation.ContextualReason() != "" || relation.Widened() {
			return nil, false
		}
		routes = append(routes, relationcall.Route{Point: point, Target: relationcall.Target{Cell: identity.Cell, SummaryKey: identity.Summary}})
	}
	catalog, err := relationcall.NewCatalog(plan.PointCount(), routes)
	if err != nil {
		return nil, false
	}

	// Catalog and RelationSnapshot are immutable publication products. Capture
	// them by value so the factory contains no mutable run-catalog dependency.
	relations := s.relations
	return func(callCtx body.CallOutcomeContext) relationcall.Resolver {
		adapter := callresult.ProviderConfig{
			ProtectedCall:           callCtx.ProtectedCall,
			CalleeValue:             callresult.CalleeValueFunc(callCtx.CalleeValue),
			ReceiverCallable:        callresult.ReceiverCallableFunc(callCtx.ReceiverCallable),
			Facts:                   callCtx.Facts,
			Sources:                 callCtx.Sources,
			ReturnPresenceRelations: callresult.ReturnPresenceRelationsForPathFunc(callCtx.ReturnPresenceRelationsPath),
			KeySpace:                callCtx.KeySpace,
			TypeValues:              callCtx.TypeValues,
		}
		return relationcall.NewResolver(relationcall.Config{
			Relations:      relations,
			Catalog:        &catalog,
			Bindings:       relationBindings(callCtx),
			Specialization: relationSpecialization(callCtx),
			// Effects are deliberately unavailable in this first production-shaped
			// slice. Any effect term therefore rejects the complete application.
			EffectResolver: nil,
			Adapter:        adapter,
		})
	}, true
}

func paramsOnlyShape(shape transformer.Shape) bool {
	return shape.Captures == 0 && shape.Globals == 0 && shape.Results == 0 && shape.HeapTemplates == 0
}

func relationBindings(callCtx body.CallOutcomeContext) relationcall.Bindings {
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State, shape transformer.Shape) (transformer.BindingCursor, bool) {
		if !paramsOnlyShape(shape) || !exactScalarCallSite(site, shape.Params) || (shape.Params != 0 && callCtx.Sources == nil) {
			return transformer.BindingCursor{}, false
		}
		values := make([]product.Value, int(shape.Params))
		paths := make([]pathdom.Path, int(shape.Params))
		point, _ := site.Point()
		for i := range values {
			source, ok := site.ArgumentSourceAt(i)
			if !ok || !exactScalarSource(source) {
				return transformer.BindingCursor{}, false
			}
			value, ok := callCtx.Sources.ValueOfSource(point, source, in, read)
			if !ok || product.ShapeOf(value).IsBottom() || product.PresenceOf(value).IsBottom() {
				return transformer.BindingCursor{}, false
			}
			values[i] = value
			paths[i] = canonicalSourcePath(callCtx.Facts, source)
		}
		cursor, err := transformer.NewBindingCursor(shape, values, paths)
		return cursor, err == nil
	}
}

func exactScalarCallSite(site factflow.CallSiteView, params uint32) bool {
	if _, ok := site.Point(); !ok || site.ArgumentSourceCount() != int(params) {
		return false
	}
	if site.CalleeSymbol() == 0 || site.CalleeMemberAccess() || site.MethodName() != "" || site.TypeArgCount() != 0 {
		return false
	}
	if _, ok := site.ReceiverPath(); ok {
		return false
	}
	if _, ok := site.ReceiverSource(); ok {
		return false
	}
	if _, ok := site.MethodPath(); ok {
		return false
	}
	callee := site.CalleePathRef()
	return callee.Symbol == site.CalleeSymbol() && len(callee.Segments) == 0
}

// exactScalarSource accepts one closed value-list slot. Adjusted expressions
// are scalar by definition; expanded/open sources are not safe dense bindings.
func exactScalarSource(source factflow.ValueSource) bool {
	return source.Valid() && !source.Expanded && !source.OpenTail && source.Kind != factflow.ValueSourceUnknown && source.Kind != factflow.ValueSourceVararg
}

// canonicalSourcePath retains the caller's exact local version and segment
// spelling. Missing path identity is represented by an empty dense slot; a
// relation that demands that path then fails specialization transactionally.
func canonicalSourcePath(facts factflow.Facts, source factflow.ValueSource) pathdom.Path {
	switch source.Kind {
	case factflow.ValueSourceExpression:
		if source.HasExpr {
			if p, ok := facts.ExpressionPath(source.ExprRef); ok {
				return p
			}
		}
	case factflow.ValueSourcePath:
		if p, ok := pathaddr.LocalPathFromKey(source.PathKey); ok {
			return p
		}
	}
	return pathdom.Path{}
}

func relationSpecialization(callCtx body.CallOutcomeContext) relationcall.Specialization {
	return func(ctx transfer.NodeContext, _ factflow.CallSiteView, in state.State, _ func(cfg.Point) state.State) (transformer.SpecializationContext, bool) {
		var out transformer.SpecializationContext
		if callCtx.DynamicRead != nil {
			out.DynamicRead = func(path pathdom.Path, owner, key product.Value) (product.Value, bool) {
				return callCtx.DynamicRead(ctx, path, owner, key, in)
			}
		}
		if callCtx.DynamicTableRead != nil {
			out.DynamicTableRead = func(path pathdom.Path, table, key product.Value) (product.Value, bool) {
				return callCtx.DynamicTableRead(ctx, path, table, key, in)
			}
		}
		return out, true
	}
}
