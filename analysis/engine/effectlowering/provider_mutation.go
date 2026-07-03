package effectlowering

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/literal"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type signatureParamMutationEffects struct {
	Writes            []callpayload.CallParamPathWrite
	Obligations       []callpayload.CallParamObligation
	DynamicIndexFacts []callboundary.DynamicIndexFact
}

func signatureParamMutationEffectsForReader(
	ctx transfer.NodeContext,
	sig signature.Function,
	args signatureArgumentReader,
	sources sourcevalue.SourceValues,
	argumentType SignatureArgumentTypeFunc,
	in state.State,
	read func(cfg.Point) state.State,
	typeValues *typevalue.Cache,
) signatureParamMutationEffects {
	if ctx.Registry == nil || len(sig.Effect.Labels) == 0 || sources == nil {
		return signatureParamMutationEffects{}
	}
	var out signatureParamMutationEffects
	for _, label := range sig.Effect.Labels {
		mutator, ok := effect.NormalizeLabel(label).(mutation.TableMutator)
		if !ok {
			continue
		}
		evidence, ok := readTableMutatorEvidence(ctx, mutator, args, sources, argumentType, in, read, typeValues)
		if !ok {
			continue
		}
		if write, ok := tableMutatorParamWrite(ctx, typeValues, evidence); ok {
			out.Writes = append(out.Writes, write)
		}
		if obligation, ok := tableMutatorParamObligation(ctx, typeValues, evidence); ok {
			out.Obligations = append(out.Obligations, obligation)
		}
		if fact, ok := tableMutatorDynamicIndexFact(mutator, evidence); ok {
			out.DynamicIndexFacts = append(out.DynamicIndexFacts, fact)
		}
	}
	return out
}

func tableMutatorDynamicIndexSite(mutator mutation.TableMutator) dynamicindex.Site {
	return dynamicindex.Site("signature.table_mutator:" +
		strconv.Itoa(mutator.Target.Index) + ":" +
		strconv.Itoa(mutator.Value.Index))
}

func tableMutatorParamWrite(ctx transfer.NodeContext, typeValues *typevalue.Cache, evidence tableMutatorEvidence) (callpayload.CallParamPathWrite, bool) {
	if evidence.valueType == nil {
		return callpayload.CallParamPathWrite{}, false
	}
	refined, ok := tableMutatorTargetType(evidence.targetType, evidence.valueType)
	if !ok {
		return callpayload.CallParamPathWrite{}, false
	}
	return callpayload.CallParamPathWrite{
		Path:  path.NewPlaceholder(evidence.targetIndex),
		Value: tableMutatorWriteValue(ctx.Registry, typeValues, evidence.target, refined),
	}, true
}

func tableMutatorWriteValue(reg *axis.Registry, typeValues *typevalue.Cache, target product.Value, refined typ.Type) product.Value {
	value := returnValueFromTypeCached(reg, typeValues, refined)
	if id, ok := identityvalue.ExactID(reg, target); ok {
		value = identityvalue.WithExact(reg, value, id)
	}
	if claim := product.Get(reg, target, assertion.Key); !claim.IsTop() && !claim.IsBottom() {
		value = product.Set(reg, value, assertion.Key, claim)
	}
	return value
}

func tableMutatorParamObligation(ctx transfer.NodeContext, typeValues *typevalue.Cache, evidence tableMutatorEvidence) (callpayload.CallParamObligation, bool) {
	element, ok := tableMutatorValueObligationType(ctx.Registry, evidence.target, evidence.targetType)
	if !ok || element == nil || typ.IsAny(element) || typ.IsUnknown(element) {
		return callpayload.CallParamObligation{}, false
	}
	return callpayload.CallParamObligation{
		ParamIndex: evidence.valueIndex,
		Value:      returnValueFromTypeCached(ctx.Registry, typeValues, element),
	}, true
}

func tableMutatorDynamicIndexFact(mutator mutation.TableMutator, evidence tableMutatorEvidence) (callboundary.DynamicIndexFact, bool) {
	out := callboundary.DynamicIndexFact{
		Table: path.NewPlaceholder(evidence.targetIndex),
		Site:  tableMutatorDynamicIndexSite(mutator),
		Value: dynamicindex.Fact{
			KeyPresence: product.PresenceOf(evidence.indexKey),
			KeyValue:    evidence.indexKey,
			Value:       evidence.value,
			Admission:   dynamicindex.AdmissionAdmitted,
		},
	}
	if evidence.valueCanBindPath {
		out.ValuePath = path.NewPlaceholder(evidence.valueIndex)
	}
	return out, true
}

type tableMutatorEvidence struct {
	targetIndex      int
	valueIndex       int
	valueCanBindPath bool
	indexKey         product.Value
	target           product.Value
	value            product.Value
	targetType       typ.Type
	valueType        typ.Type
}

func readTableMutatorEvidence(
	ctx transfer.NodeContext,
	mutator mutation.TableMutator,
	args signatureArgumentReader,
	sources sourcevalue.SourceValues,
	argumentType SignatureArgumentTypeFunc,
	in state.State,
	read func(cfg.Point) state.State,
	typeValues *typevalue.Cache,
) (tableMutatorEvidence, bool) {
	targetIndex, ok := effect.ResolveParamIndex(mutator.Target, args.ArgumentSourceCount())
	if !ok {
		return tableMutatorEvidence{}, false
	}
	targetSource, ok := args.ArgumentSourceAt(targetIndex)
	if !ok || !callArgumentSourceCanBindPath(targetSource) {
		return tableMutatorEvidence{}, false
	}
	valueIndex, ok := effect.ResolveParamIndex(mutator.Value, args.ArgumentSourceCount())
	if !ok {
		return tableMutatorEvidence{}, false
	}
	valueSource, ok := args.ArgumentSourceAt(valueIndex)
	if !ok {
		return tableMutatorEvidence{}, false
	}
	target, ok := sources.ValueOfSource(ctx.Point, targetSource, in, read)
	if !ok {
		return tableMutatorEvidence{}, false
	}
	targetType, ok := typevalue.TypeOf(ctx.Registry, target)
	if !tableMutatorConcreteType(targetType, ok) {
		if t, tok := tableMutatorArgumentType(ctx, argumentType, targetSource, in, read); tok {
			targetType = t
			target = tableMutatorValueWithType(ctx.Registry, typeValues, target, true, targetType)
		}
	}
	if !tableMutatorConcreteType(targetType, true) {
		return tableMutatorEvidence{}, false
	}
	value, valueOK := sources.ValueOfSource(ctx.Point, valueSource, in, read)
	valueType, valueTypeOK := typevalue.TypeOf(ctx.Registry, value)
	if !valueOK || !tableMutatorConcreteType(valueType, valueTypeOK) {
		if t, tok := tableMutatorArgumentType(ctx, argumentType, valueSource, in, read); tok {
			valueType = t
			valueTypeOK = true
			value = tableMutatorValueWithType(ctx.Registry, typeValues, value, valueOK, valueType)
			valueOK = true
		}
	}
	if !valueOK || !tableMutatorConcreteType(valueType, valueTypeOK) {
		fallbackType, fallbackOK := tableMutatorTopValueFallback(targetType)
		switch {
		case fallbackOK:
			valueType = fallbackType
			if !valueOK {
				value = returnValueFromTypeCached(ctx.Registry, typeValues, valueType)
			}
		case valueOK && product.DefinitelyPresent(value) && tableMutatorFreshEmptyRecordTarget(targetType):
			valueType = typ.Unknown
		default:
			value = product.Bottom(ctx.Registry)
			valueType = nil
		}
	}
	return tableMutatorEvidence{
		targetIndex:      targetIndex,
		valueIndex:       valueIndex,
		valueCanBindPath: callArgumentSourceCanBindPath(valueSource),
		indexKey:         returnValueFromTypeCached(ctx.Registry, typeValues, typ.Integer),
		target:           target,
		value:            value,
		targetType:       targetType,
		valueType:        valueType,
	}, true
}

func tableMutatorArgumentType(
	ctx transfer.NodeContext,
	argumentType SignatureArgumentTypeFunc,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
) (typ.Type, bool) {
	if argumentType == nil {
		return nil, false
	}
	t, ok := argumentType(ctx, source, in, read)
	if !tableMutatorConcreteType(t, ok) {
		return nil, false
	}
	return t, true
}

func tableMutatorConcreteType(t typ.Type, ok bool) bool {
	return ok &&
		t != nil &&
		!typ.IsAny(t) &&
		!typ.IsUnknown(t) &&
		!typ.IsNever(t) &&
		!refinement.ContainsFreeTypeParam(t)
}

func tableMutatorValueWithType(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	value product.Value,
	valueOK bool,
	t typ.Type,
) product.Value {
	if !valueOK {
		return returnValueFromTypeCached(reg, typeValues, t)
	}
	return typevalue.WithWitness(reg, value, t)
}

func tableMutatorFreshEmptyRecordTarget(targetType typ.Type) bool {
	target, ok := unwrap.Alias(targetType).(*typ.Record)
	return ok &&
		len(target.Fields) == 0 &&
		len(target.StaticMembers) == 0 &&
		target.Metatable == nil &&
		!target.Open &&
		!target.HasMapComponent()
}

func tableMutatorValueObligationType(reg *axis.Registry, target product.Value, targetType typ.Type) (typ.Type, bool) {
	element, ok := projection.ElementOf(targetType)
	if !ok || element == nil {
		return nil, false
	}
	if identityvalue.HasExact(reg, target) {
		claim := product.Get(reg, target, assertion.Key)
		if !claim.Has(assertion.TypeClaim) {
			return nil, false
		}
		if base, ok := literal.FamilyBase(element); ok {
			return base, true
		}
	}
	return element, true
}

func tableMutatorTopValueFallback(targetType typ.Type) (typ.Type, bool) {
	element, ok := projection.ElementOf(targetType)
	if !ok || element == nil {
		return nil, false
	}
	if typ.IsAny(element) || typ.IsUnknown(element) {
		return element, true
	}
	return nil, false
}

func tableMutatorTargetType(targetType, valueType typ.Type) (typ.Type, bool) {
	if targetType == nil || valueType == nil {
		return nil, false
	}
	insertedElement := valueType
	if existingElement, ok := projection.ElementOf(targetType); ok {
		insertedElement = tableMutatorInsertedElementType(existingElement, valueType)
	}
	switch target := unwrap.Alias(targetType).(type) {
	case *typ.Record:
		return tableMutatorRecordType(target, insertedElement)
	case *typ.Array, *typ.Tuple:
		return typ.NewArray(insertedElement), true
	case *typ.Map:
		key := normalize.UnionForEvidence(target.Key, typ.Integer)
		return typetable.NewRecord().MapComponent(key, insertedElement).Build(), true
	default:
		return nil, false
	}
}

func tableMutatorRecordType(target *typ.Record, insertedElement typ.Type) (typ.Type, bool) {
	if target == nil || insertedElement == nil {
		return nil, false
	}
	if arrayType, ok := tableMutatorArrayRecordType(target, insertedElement); ok {
		return arrayType, true
	}
	key := typ.Integer
	value := insertedElement
	if target.HasMapComponent() {
		if target.MapKey != nil {
			key = normalize.UnionForEvidence(target.MapKey, typ.Integer)
		}
		if target.MapValue != nil {
			value = tableMutatorInsertedElementType(target.MapValue, insertedElement)
		}
	}
	overlay := typetable.NewRecord().MapComponent(key, value).Build()
	return typetable.OverlayRecordMembers(target, overlay)
}

func tableMutatorArrayRecordType(target *typ.Record, insertedElement typ.Type) (typ.Type, bool) {
	if target == nil ||
		len(target.Fields) != 0 ||
		len(target.StaticMembers) != 0 ||
		target.Metatable != nil ||
		target.Open {
		return nil, false
	}
	if !target.HasMapComponent() {
		return typ.NewArray(insertedElement), true
	}
	if target.MapKey == nil || !typ.TypeEquals(unwrap.Alias(target.MapKey), typ.Integer) {
		return nil, false
	}
	value := insertedElement
	if target.MapValue != nil {
		value = tableMutatorInsertedElementType(target.MapValue, insertedElement)
	}
	return typ.NewArray(value), true
}

func tableMutatorInsertedElementType(existingElement, insertedElement typ.Type) typ.Type {
	if existingElement == nil {
		return insertedElement
	}
	existing := unwrap.Annotated(existingElement)
	if typ.IsAny(existing) || typ.IsUnknown(existing) {
		return existingElement
	}
	return normalize.UnionForEvidence(existingElement, insertedElement)
}
