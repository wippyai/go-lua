package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

const setmetatableGlobalName = "setmetatable"

func configWithMetatableMethodSignatureArguments(config body.Config, proof metatableMethodProof) body.Config {
	if proof.empty() {
		return config
	}
	baseOutcomeFactory := config.CallOutcomeFactory
	config.CallOutcomeFactory = func(ctx body.CallOutcomeContext) callpayload.CallOutcomeProvider {
		var base callpayload.CallOutcomeProvider
		if baseOutcomeFactory != nil {
			base = baseOutcomeFactory(ctx)
		}
		methodProvider := metatableMethodCallOutcomeProvider(ctx, proof)
		if base == nil {
			return methodProvider
		}
		if methodProvider == nil {
			return base
		}
		return func(node transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
			return calloutcome.MergeSupplemental(node.Registry, base(node, site, in, read), methodProvider(node, site, in, read))
		}
	}
	baseFactory := config.SignatureArgumentTypeFactory
	config.SignatureArgumentTypeFactory = func(ctx body.CallOutcomeContext) body.SignatureArgumentTypeFunc {
		var base body.SignatureArgumentTypeFunc
		if baseFactory != nil {
			base = baseFactory(ctx)
		}
		return func(node transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool) {
			if base != nil {
				if t, ok := base(node, source, in, read); ok {
					if surfaced, surfacedOK := metatableSignatureArgumentType(ctx, proof, node, source, t); surfacedOK {
						return surfaced, true
					}
					return t, true
				}
			}
			if t, ok := metatableSignatureArgumentTypeFromSource(ctx, proof, node, source, in, read); ok {
				return t, true
			}
			return nil, false
		}
	}
	return config
}

func metatableMethodCallOutcomeProvider(ctx body.CallOutcomeContext, proof metatableMethodProof) callpayload.CallOutcomeProvider {
	if proof.empty() || ctx.Sources == nil || ctx.KeySpace == nil {
		return nil
	}
	return func(node transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if node.Registry == nil || !callSiteIsSetmetatable(proof, site) {
			return callpayload.CallOutcome{}
		}
		tableSource, ok := site.ArgumentSourceAt(0)
		if !ok {
			return callpayload.CallOutcome{}
		}
		metatable, ok := setmetatableCallMetatableSymbol(ctx.Facts, proof, node.Point, tableSource)
		if !ok {
			return callpayload.CallOutcome{}
		}
		methods, ok := proof.metaIndexes[metatable]
		if !ok || methods == 0 {
			return callpayload.CallOutcome{}
		}
		tableValue, ok := ctx.Sources.ValueOfSource(node.Point, tableSource, in, read)
		if !ok {
			return callpayload.CallOutcome{}
		}
		if base, ok := typevalue.TypeOf(node.Registry, tableValue); ok {
			if surfaced, ok := proof.receiverWithMethodSurfaceForMetatable(metatable, base); ok {
				tableValue = typevalue.WithWitness(node.Registry, tableValue, surfaced)
			}
		}
		tableID, ok := product.Get(node.Registry, tableValue, identity.Key).ID()
		if !ok {
			return callpayload.CallOutcome{
				Results: []callpayload.CallResult{{Index: 0, Value: tableValue}},
			}
		}
		object := in.ReadHeapTableObject(node.Registry, tableID)
		if heapidentity.ObjectDomain(node.Registry).Equal(object, heapidentity.BottomObject(node.Registry)) {
			object = heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: tableValue})
		}
		receiver := proof.receiverHints[metatable]
		if receiver == nil {
			receiver = typetable.NewRecord().Build()
		}
		for _, member := range proof.methodSurfaceMembers(node.Registry, methods, receiver) {
			if member.name == "" {
				continue
			}
			for _, suffix := range [][]segment.Segment{
				{{Kind: segment.SegmentField, Name: member.name}},
				{{Kind: segment.SegmentIndexString, Name: member.name}},
			} {
				next, ok := object.WithStaticMember(node.Registry, ctx.KeySpace, suffix, member.value)
				if ok {
					object = next
				}
			}
		}
		return callpayload.CallOutcome{
			Results: []callpayload.CallResult{{Index: 0, Value: tableValue}},
			HeapTableObjects: map[identity.ID]heapidentity.TableObject{
				tableID: object,
			},
		}
	}
}

func metatableSignatureArgumentTypeFromSource(
	ctx body.CallOutcomeContext,
	proof metatableMethodProof,
	node transfer.NodeContext,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (typ.Type, bool) {
	if ctx.Sources == nil || node.Registry == nil {
		return nil, false
	}
	value, ok := ctx.Sources.ValueOfSource(node.Point, source, in, read)
	if !ok {
		return nil, false
	}
	t, ok := typevalue.TypeOf(node.Registry, value)
	if !ok {
		return nil, false
	}
	return metatableSignatureArgumentType(ctx, proof, node, source, t)
}

func metatableSignatureArgumentType(
	ctx body.CallOutcomeContext,
	proof metatableMethodProof,
	node transfer.NodeContext,
	source factflow.ValueSource,
	base typ.Type,
) (typ.Type, bool) {
	if base == nil {
		return nil, false
	}
	metatable, ok := setmetatableCallMetatableSymbol(ctx.Facts, proof, node.Point, source)
	if !ok {
		return nil, false
	}
	return proof.receiverWithMethodSurfaceForMetatable(metatable, base)
}

func setmetatableCallMetatableSymbol(facts factflow.Facts, proof metatableMethodProof, point cfg.Point, source factflow.ValueSource) (symbol.ID, bool) {
	site, ok := facts.CallSiteView(point)
	if !ok || !callSiteIsSetmetatable(proof, site) {
		return 0, false
	}
	tableArg, ok := site.ArgumentSourceAt(0)
	if !ok || tableArg != source {
		return 0, false
	}
	metaArg, ok := site.ArgumentSourceAt(1)
	if !ok || metaArg.Kind != factflow.ValueSourceExpression || !metaArg.HasExpr {
		return 0, false
	}
	metaPath, ok := facts.ExpressionPathRef(metaArg.ExprRef)
	if !ok || metaPath.Symbol == 0 || len(metaPath.Segments) != 0 {
		return 0, false
	}
	return metaPath.Symbol, true
}

func callSiteIsSetmetatable(proof metatableMethodProof, site factflow.CallSiteView) bool {
	if site.MethodName() != "" {
		return false
	}
	if global, ok := proof.bindings.GlobalSymbol(setmetatableGlobalName); ok && site.CalleeSymbol() == global {
		return true
	}
	p := site.CalleePathRef()
	if len(p.Segments) != 0 {
		return false
	}
	if p.Root == setmetatableGlobalName && p.Symbol != 0 {
		return true
	}
	if p.Symbol == 0 {
		return false
	}
	kind, ok := proof.bindings.Kind(p.Symbol)
	return ok && kind == symbol.Global && proof.bindings.Name(p.Symbol) == setmetatableGlobalName
}
