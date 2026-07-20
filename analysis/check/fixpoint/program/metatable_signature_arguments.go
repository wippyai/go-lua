package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func configWithMetatableMethodSignatureArguments(config body.Config, proof metatableMethodProof) body.Config {
	if proof.empty() {
		return config
	}
	baseOutcomeFactory := config.CallOutcomeFactory
	config.CallOutcomeFactory = func(ctx body.CallOutcomeContext) callpayload.CallOutcomeProgram {
		var base callpayload.CallOutcomeProgram
		if baseOutcomeFactory != nil {
			base = baseOutcomeFactory(ctx)
		}
		methodProvider := metatableMethodCallOutcomeProgram(ctx, proof)
		return calloutcome.ComposeSupplemental(base, methodProvider)
	}
	baseFactory := config.SignatureArgumentTypeFactory
	config.SignatureArgumentTypeFactory = func(ctx body.CallOutcomeContext, input effectlowering.SignatureOutcomeInputProgram) effectlowering.SignatureArgumentTypeProgram {
		var base effectlowering.SignatureArgumentTypeProgram
		if baseFactory != nil {
			base = baseFactory(ctx, input)
		}
		metatable, err := effectlowering.SealSignatureArgumentTypeProgram(input, func(argument effectlowering.SignatureArgumentTypeContext) (typ.Type, bool) {
			baseType, ok := typevalue.TypeOf(argument.Node.Registry, argument.Value)
			if !ok {
				return nil, false
			}
			return metatableSignatureArgumentType(ctx, proof, argument.Node, argument.Source, baseType)
		})
		if err != nil {
			panic(err)
		}
		return effectlowering.ComposeSignatureArgumentTypePrograms(metatable, base)
	}
	return config
}

func metatableMethodCallOutcomeProgram(ctx body.CallOutcomeContext, proof metatableMethodProof) callpayload.CallOutcomeProgram {
	if proof.empty() || ctx.Sources == nil || ctx.KeySpace == nil {
		return callpayload.CallOutcomeProgram{}
	}
	shape := func(node transfer.NodeContext, _ factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
		if node.Registry == nil || ctx.OperationPlan == nil {
			return callpayload.CallOutcomeSiteShape{}, nil
		}
		operation, ok := ctx.OperationPlan.AttachMetatableOperation(node.Point)
		if !ok {
			return callpayload.CallOutcomeSiteShape{}, nil
		}
		metatable, ok := attachedMetatableSymbol(ctx, node.Point, operation.Table())
		if !ok || proof.metaIndexes[metatable] == 0 {
			return callpayload.CallOutcomeSiteShape{}, nil
		}
		return callpayload.CallOutcomeSiteShape{FieldNames: []string{"Results", "HeapTableObjects"}, InputLanes: state.NewLaneSet(state.LaneHeapTableIdentity)}, nil
	}
	evaluate := func(node transfer.NodeContext, site factflow.CallSiteView, input callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		if node.Registry == nil || ctx.OperationPlan == nil {
			return callpayload.CallOutcome{}, nil
		}
		operation, ok := ctx.OperationPlan.AttachMetatableOperation(node.Point)
		if !ok {
			return callpayload.CallOutcome{}, nil
		}
		tableSource := operation.Table()
		metatable, ok := attachedMetatableSymbol(ctx, node.Point, tableSource)
		if !ok {
			return callpayload.CallOutcome{}, nil
		}
		methods, ok := proof.metaIndexes[metatable]
		if !ok || methods == 0 {
			return callpayload.CallOutcome{}, nil
		}
		tableIndex := -1
		site.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
			if factflow.ValueSourceEqual(source, tableSource) {
				tableIndex = index
				return false
			}
			return true
		})
		tableValue, ok := input.Argument(tableIndex)
		if !ok {
			return callpayload.CallOutcome{}, nil
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
			}, nil
		}
		factor, ok := input.Primary().Factor(state.LaneHeapTableIdentity)
		if !ok {
			return callpayload.CallOutcome{}, nil
		}
		object, err := input.Domain().ReadHeapTableObjectTermFactor(factor, identity.ConcreteTerm(tableID))
		if err != nil {
			return callpayload.CallOutcome{}, err
		}
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
		}, nil
	}
	return callpayload.SealCallOutcomeProgram(
		"metatable-method outcome", []string{"Results", "HeapTableObjects"},
		state.NewLaneSet(state.LaneHeapTableIdentity), state.LaneSet{}, shape, nil, evaluate,
	)
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
	metatable, ok := attachedMetatableSymbol(ctx, node.Point, source)
	if !ok {
		return nil, false
	}
	return proof.receiverWithMethodSurfaceForMetatable(metatable, base)
}

func attachedMetatableSymbol(ctx body.CallOutcomeContext, point cfg.Point, source factflow.ValueSource) (symbol.ID, bool) {
	if ctx.OperationPlan == nil {
		return 0, false
	}
	operation, ok := ctx.OperationPlan.AttachMetatableOperation(point)
	if !ok || !factflow.ValueSourceEqual(operation.Table(), source) {
		return 0, false
	}
	metaArg := operation.Metatable()
	if metaArg.Kind != factflow.ValueSourceExpression || !metaArg.HasExpr {
		return 0, false
	}
	metaPath, ok := ctx.Facts.ExpressionPathRef(metaArg.ExprRef)
	if !ok || metaPath.Symbol == 0 || len(metaPath.Segments) != 0 {
		return 0, false
	}
	return metaPath.Symbol, true
}
