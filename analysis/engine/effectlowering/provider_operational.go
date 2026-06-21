package effectlowering

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/inspect"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type operationalEffectContext struct {
	effects               *signature.OperationalEffects
	argSources            signatureArgumentReader
	sources               sourcevalue.SourceValues
	expressionRefinements map[factflow.ExprRef]factflow.ExpressionRefinement
	in                    state.State
	read                  func(cfg.Point) state.State
	keySpace              *keyspace.KeySpace
}

func applyOperationalEffects(ctx transfer.NodeContext, out callpayload.CallOutcome, op operationalEffectContext) callpayload.CallOutcome {
	effects := op.effects
	if effects == nil {
		return out
	}
	out.ReturnPresenceRelations = operationalReturnPresenceRelations(*effects)
	out.NormalReturnFacts.PathRefinements = operationalPathPresenceRefinements(ctx, *effects)
	out.NormalReturnFacts.PathStaticMembers = operationalPathStaticMembers(ctx, *effects)
	out.NormalReturnFacts.PathInvalidations = operationalPathInvalidations(*effects)
	out.NormalReturnFacts.DynamicIndexFacts = operationalDynamicIndexFacts(ctx, op)
	out.NormalReturnFacts.FrozenTables = operationalFrozenTables(*effects)
	out.NormalReturnFacts.EscapeEvents = operationalEscapeEvents(*effects)
	out.NormalReturnFacts.StoreRelations = operationalStoreRelations(*effects)
	out.NormalReturnFacts.LifecycleFacts = operationalLifecycleFacts(*effects)
	out.HeapTableObjects = operationalHeapTableObjects(ctx, op.keySpace, *effects)
	out.Placements = operationalAllocationPlacements(*effects)
	return out
}

func operationalReturnPresenceRelations(e signature.OperationalEffects) []callpayload.CallReturnPresenceRelation {
	if len(e.ReturnPresenceRelations) == 0 {
		return nil
	}
	out := make([]callpayload.CallReturnPresenceRelation, 0, len(e.ReturnPresenceRelations))
	for _, relation := range e.ReturnPresenceRelations {
		out = append(out, callpayload.CallReturnPresenceRelation{
			TriggerIndex:    relation.TriggerIndex,
			TriggerPresence: relation.TriggerPresence,
			TargetIndex:     relation.TargetIndex,
			TargetPresence:  relation.TargetPresence,
		})
	}
	return out
}

func operationalPathPresenceRefinements(ctx transfer.NodeContext, e signature.OperationalEffects) []callboundary.PathValueFact {
	if len(e.NormalReturnPresenceRefinements) == 0 {
		return nil
	}
	out := make([]callboundary.PathValueFact, 0, len(e.NormalReturnPresenceRefinements))
	for _, refinement := range e.NormalReturnPresenceRefinements {
		value, ok := operationalPresenceValue(ctx, refinement.Presence)
		if !ok {
			continue
		}
		out = append(out, callboundary.PathValueFact{
			Path:  refinement.Path,
			Value: value,
		})
	}
	return out
}

func operationalPathStaticMembers(ctx transfer.NodeContext, e signature.OperationalEffects) []callboundary.PathStaticMemberFact {
	if ctx.Registry == nil || len(e.PathStaticMembers) == 0 {
		return nil
	}
	out := make([]callboundary.PathStaticMemberFact, 0, len(e.PathStaticMembers))
	for _, fact := range e.PathStaticMembers {
		if fact.Type == nil {
			continue
		}
		value := typevalue.WithWitness(ctx.Registry, typevalue.FromType(ctx.Registry, fact.Type), fact.Type)
		out = append(out, callboundary.PathStaticMemberFact{
			Path:  fact.Path,
			Value: value,
		})
	}
	return out
}

func operationalPresenceValue(ctx transfer.NodeContext, p presence.Value) (product.Value, bool) {
	if ctx.Registry == nil {
		return product.Value{}, false
	}
	switch {
	case presence.Equal(p, presence.Present()):
		return product.NewWithPresence(ctx.Registry, product.ShapeTop, presence.Present()), true
	case presence.Equal(p, presence.Absent()):
		return product.Absent(ctx.Registry), true
	default:
		return product.Value{}, false
	}
}

func operationalPathInvalidations(e signature.OperationalEffects) []callboundary.PathInvalidationFact {
	return projectOperationalFacts(e.PathInvalidations, func(f signature.PathInvalidation) callboundary.PathInvalidationFact {
		return callboundary.PathInvalidationFact{Path: f.Path}
	})
}

func operationalDynamicIndexFacts(ctx transfer.NodeContext, op operationalEffectContext) []callboundary.DynamicIndexFact {
	if ctx.Registry == nil || op.effects == nil || len(op.effects.DynamicIndexFacts) == 0 {
		return nil
	}
	out := make([]callboundary.DynamicIndexFact, 0, len(op.effects.DynamicIndexFacts))
	for _, fact := range op.effects.DynamicIndexFacts {
		if fact.Site == "" || fact.Table.IsEmpty() {
			continue
		}
		key, ok := operationalDynamicIndexOperandValue(ctx, op, fact.Key)
		if !ok {
			continue
		}
		value, ok := operationalDynamicIndexOperandValue(ctx, op, fact.Value)
		if !ok {
			continue
		}
		admission, ok := operationalDynamicIndexAdmission(fact.Admission)
		if !ok {
			continue
		}
		keyPresence := fact.KeyPresence
		if keyPresence.IsBottom() || keyPresence.IsTop() {
			keyPresence = product.PresenceOf(key)
		}
		out = append(out, callboundary.DynamicIndexFact{
			Table: fact.Table,
			Site:  dynamicindex.Site(fact.Site),
			Value: dynamicindex.Fact{
				KeyPresence: keyPresence,
				KeyValue:    key,
				Value:       value,
				Admission:   admission,
			},
		})
	}
	return out
}

func operationalDynamicIndexOperandValue(
	ctx transfer.NodeContext,
	op operationalEffectContext,
	operand signature.DynamicIndexOperand,
) (product.Value, bool) {
	if !operand.Path.IsEmpty() {
		if value, ok := operationalPlaceholderOperandValue(ctx, op, operand.Path); ok {
			return value, true
		}
	}
	if operand.Type != nil {
		return typevalue.WithWitness(ctx.Registry, typevalue.FromType(ctx.Registry, operand.Type), operand.Type), true
	}
	return product.Value{}, false
}

func operationalPlaceholderOperandValue(
	ctx transfer.NodeContext,
	op operationalEffectContext,
	operandPath pathdom.Path,
) (product.Value, bool) {
	if !operandPath.IsPlaceholder() || len(operandPath.Segments) != 0 || op.sources == nil {
		return product.Value{}, false
	}
	index := operandPath.PlaceholderIndex()
	source, ok := op.argSources.ArgumentSourceAt(index)
	if !ok {
		return product.Value{}, false
	}
	resolver := sourcevalue.WithExpressionRefinements(ctx.Registry, op.sources, op.expressionRefinements)
	return resolver.ValueOfSource(ctx.Point, source, op.in, op.read)
}

func operationalDynamicIndexAdmission(admission signature.DynamicIndexAdmission) (dynamicindex.Admission, bool) {
	switch admission {
	case signature.DynamicIndexAdmissionAdmitted:
		return dynamicindex.AdmissionAdmitted, true
	case signature.DynamicIndexAdmissionUnknown:
		return dynamicindex.AdmissionUnknown, true
	default:
		return dynamicindex.AdmissionBottom, false
	}
}

func operationalFrozenTables(e signature.OperationalEffects) []callboundary.FrozenTableFact {
	return projectOperationalFacts(e.FrozenTables, func(f signature.FrozenTable) callboundary.FrozenTableFact {
		return callboundary.FrozenTableFact{Target: f.Target}
	})
}

// projectOperationalFacts maps each operational fact to a call-boundary fact via
// build, returning nil for an empty input.
func projectOperationalFacts[F any, T any](facts []F, build func(F) T) []T {
	if len(facts) == 0 {
		return nil
	}
	out := make([]T, 0, len(facts))
	for _, fact := range facts {
		out = append(out, build(fact))
	}
	return out
}

func operationalEscapeEvents(e signature.OperationalEffects) []callboundary.EscapeEventFact {
	if len(e.EscapeEvents) == 0 {
		return nil
	}
	out := make([]callboundary.EscapeEventFact, 0, len(e.EscapeEvents))
	for _, event := range e.EscapeEvents {
		kind, ok := operationalEscapeKind(event.Kind)
		if !ok {
			continue
		}
		out = append(out, callboundary.EscapeEventFact{
			Target:    event.Target,
			Kind:      kind,
			Recursive: event.Recursive,
		})
	}
	return out
}

func operationalEscapeKind(kind signature.EscapeKind) (callboundary.EscapeEventKind, bool) {
	switch kind {
	case signature.EscapeBorrow:
		return callboundary.EscapeEventBorrow, true
	case signature.EscapeRetain:
		return callboundary.EscapeEventRetain, true
	case signature.EscapeStore:
		return callboundary.EscapeEventStore, true
	case signature.EscapeSend:
		return callboundary.EscapeEventSend, true
	case signature.EscapeExport:
		return callboundary.EscapeEventExport, true
	case signature.EscapeOpaque:
		return callboundary.EscapeEventOpaque, true
	default:
		return callboundary.EscapeEventNone, false
	}
}

func operationalStoreRelations(e signature.OperationalEffects) []callboundary.StoreRelationFact {
	if len(e.StoreRelations) == 0 {
		return nil
	}
	out := make([]callboundary.StoreRelationFact, 0, len(e.StoreRelations))
	for _, relation := range e.StoreRelations {
		out = append(out, callboundary.StoreRelationFact{
			Source: relation.Source,
			Into:   relation.Into,
		})
	}
	return out
}

func operationalLifecycleFacts(e signature.OperationalEffects) []callboundary.LifecycleFact {
	if len(e.LifecycleEffects) == 0 {
		return nil
	}
	out := make([]callboundary.LifecycleFact, 0, len(e.LifecycleEffects))
	for _, fact := range e.LifecycleEffects {
		kind, ok := operationalLifecycleKind(fact.Kind)
		if !ok {
			continue
		}
		out = append(out, callboundary.LifecycleFact{
			Target:     fact.Target,
			Kind:       kind,
			Protocol:   fact.Protocol,
			From:       fact.From,
			To:         fact.To,
			Obligation: fact.Obligation,
		})
	}
	return out
}

func operationalLifecycleKind(kind signature.LifecycleKind) (callboundary.LifecycleKind, bool) {
	switch kind {
	case signature.LifecycleAcquire:
		return callboundary.LifecycleAcquire, true
	case signature.LifecycleTransition:
		return callboundary.LifecycleTransition, true
	case signature.LifecycleEscape:
		return callboundary.LifecycleEscape, true
	default:
		return callboundary.LifecycleNone, false
	}
}

func operationalReturnAllocationValue(reg *axis.Registry, effects *signature.OperationalEffects, returnIndex int, value product.Value) product.Value {
	if reg == nil || effects == nil || len(effects.ReturnAllocationTemplates) == 0 {
		return value
	}
	for _, template := range effects.ReturnAllocationTemplates {
		if template.ReturnIndex != returnIndex || template.Root == "" {
			continue
		}
		return product.Set(reg, value, identity.Key, identity.Singleton(allocationTemplateIdentity(template.Root)))
	}
	return value
}

func operationalHeapTableObjects(ctx transfer.NodeContext, ks *keyspace.KeySpace, e signature.OperationalEffects) map[identity.ID]heapidentity.TableObject {
	if ctx.Registry == nil || ks == nil || len(e.ReturnAllocationTemplates) == 0 {
		return nil
	}
	out := make(map[identity.ID]heapidentity.TableObject)
	for _, template := range e.ReturnAllocationTemplates {
		objectTypes := allocationObjectTypes(template.Objects)
		for _, object := range template.Objects {
			if object.ID == "" {
				continue
			}
			id := allocationTemplateIdentity(object.ID)
			root := allocationTemplateValue(ctx.Registry, object.ID, object.Type)
			staticMembers := make(map[keyspace.Key]product.Value, len(object.StaticMembers))
			for _, member := range object.StaticMembers {
				if member.Value == "" {
					continue
				}
				key, ok := heapidentity.StaticMemberSuffixKey(ks, member.Suffix)
				if !ok {
					continue
				}
				staticMembers[key] = allocationTemplateValue(ctx.Registry, member.Value, objectTypes[member.Value])
			}
			tableKey, tableKeyOK := ks.FromStateKey(pathdom.PathKey(object.ID))
			dynamicEntries := make(map[dynamicindex.Key]dynamicindex.Fact, len(object.DynamicEntries))
			for i, entry := range object.DynamicEntries {
				if entry.Key == "" && entry.KeyType == nil && entry.Value == "" {
					continue
				}
				if !tableKeyOK {
					continue
				}
				dynamicEntries[dynamicindex.Key{
					Table: tableKey,
					Site:  dynamicindex.Site(fmt.Sprintf("manifest:%d", i)),
				}] = dynamicindex.Fact{
					KeyPresence: presence.Present(),
					KeyValue:    allocationTemplateKeyValue(ctx.Registry, entry),
					Value:       allocationTemplateValue(ctx.Registry, entry.Value, objectTypes[entry.Value]),
					Admission:   dynamicindex.AdmissionAdmitted,
				}
			}
			out[id] = heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root:              root,
				StaticMembers:     staticMembers,
				DynamicIndexFacts: dynamicEntries,
			})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func allocationObjectTypes(objects []signature.AllocationObjectTemplate) map[signature.AllocationTemplateID]typ.Type {
	if len(objects) == 0 {
		return nil
	}
	out := make(map[signature.AllocationTemplateID]typ.Type, len(objects))
	for _, object := range objects {
		if object.ID != "" && object.Type != nil && !inspect.ContainsTypeParam(object.Type) {
			out[object.ID] = object.Type
		}
	}
	return out
}

func operationalAllocationPlacements(e signature.OperationalEffects) map[identity.ID]placement.Value {
	if len(e.ReturnAllocationTemplates) == 0 {
		return nil
	}
	out := make(map[identity.ID]placement.Value)
	for _, template := range e.ReturnAllocationTemplates {
		for _, object := range template.Objects {
			if object.ID == "" {
				continue
			}
			id := allocationTemplateIdentity(object.ID)
			out[id] = placement.Join(out[id], placement.Stack)
		}
	}
	return out
}

func allocationTemplateKeyValue(reg *axis.Registry, entry signature.AllocationDynamicEntryTemplate) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	if entry.KeyType != nil {
		value = typevalue.WithWitness(reg, typevalue.FromType(reg, entry.KeyType), entry.KeyType)
	}
	if entry.Key != "" {
		value = product.Set(reg, value, identity.Key, identity.Singleton(allocationTemplateIdentity(entry.Key)))
	}
	return value
}

func allocationTemplateValue(reg *axis.Registry, id signature.AllocationTemplateID, t typ.Type) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	if t != nil {
		value = typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
	}
	if id == "" {
		return value
	}
	return product.Set(reg, value, identity.Key, identity.Singleton(allocationTemplateIdentity(id)))
}

func allocationTemplateIdentity(id signature.AllocationTemplateID) identity.ID {
	return identity.ID{Kind: "manifest.allocation", Site: string(id)}
}
