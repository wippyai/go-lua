package effectlowering

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type operationalEffectContext struct {
	effects               *signature.OperationalEffects
	signatureType         *typ.Function
	argSources            signatureArgumentReader
	sources               sourcevalue.SourceValues
	expressionRefinements sourcevalue.ExpressionRefinements
	in                    state.State
	read                  func(cfg.Point) state.State
	keySpace              *keyspace.KeySpace
	typeValues            *typevalue.Cache
}

func applyOperationalEffects(ctx transfer.NodeContext, out callpayload.CallOutcome, op operationalEffectContext) callpayload.CallOutcome {
	effects := op.effects
	if effects == nil {
		return out
	}
	out.ReturnPresenceRelations = operationalReturnPresenceRelations(*effects)
	applyOperationalNormalReturnFacts(ctx, op, *effects, &out.NormalReturnFacts)
	out.HeapTableObjects = operationalHeapTableObjects(ctx, op.typeValues, op.keySpace, op.signatureType, *effects)
	out.Placements = operationalAllocationPlacements(ctx.Point, *effects)
	return out
}

type operationalNormalReturnLaneHandler func(transfer.NodeContext, operationalEffectContext, signature.OperationalEffects, *callboundary.NormalReturnFacts)

var operationalNormalReturnLanes = callboundary.BindNormalReturnFactLanes(
	"operational-effects normal-return",
	map[callboundary.NormalReturnFactLaneID]operationalNormalReturnLaneHandler{
		callboundary.LanePathRefinements: func(ctx transfer.NodeContext, op operationalEffectContext, effects signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.PathRefinements = append(out.PathRefinements, operationalPathPresenceRefinements(ctx, effects)...)
			out.PathRefinements = append(out.PathRefinements, operationalPathTypeRefinements(ctx, op.typeValues, effects)...)
		},
		callboundary.LanePersistentPathWrites: operationalNormalReturnNoop,
		callboundary.LanePathStaticMembers: func(ctx transfer.NodeContext, op operationalEffectContext, effects signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.PathStaticMembers = operationalPathStaticMembers(ctx, op.typeValues, effects)
		},
		callboundary.LanePathPresenceImplications: func(ctx transfer.NodeContext, op operationalEffectContext, effects signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.PathPresenceImplications = operationalPathPresenceImplications(ctx, op.typeValues, effects)
		},
		callboundary.LanePathInvalidations: func(_ transfer.NodeContext, _ operationalEffectContext, effects signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.PathInvalidations = operationalPathInvalidations(effects)
		},
		callboundary.LaneDynamicIndexFacts: func(ctx transfer.NodeContext, op operationalEffectContext, _ signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.DynamicIndexFacts = operationalDynamicIndexFacts(ctx, op)
		},
		callboundary.LaneKeyMemberships: func(_ transfer.NodeContext, _ operationalEffectContext, effects signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.KeyMemberships = operationalKeyMemberships(effects)
		},
		callboundary.LaneDynamicValueKeys: func(_ transfer.NodeContext, _ operationalEffectContext, effects signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.DynamicValueKeys = operationalDynamicValueKeys(effects)
		},
		callboundary.LaneDynamicAllValues: operationalNormalReturnNoop,
		callboundary.LaneBranchProofs: func(_ transfer.NodeContext, _ operationalEffectContext, effects signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.BranchProofs = operationalBranchProofs(effects)
		},
		callboundary.LaneChannelSelects: operationalNormalReturnNoop,
		callboundary.LaneFrozenTables: func(_ transfer.NodeContext, _ operationalEffectContext, effects signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.FrozenTables = operationalFrozenTables(effects)
		},
		callboundary.LaneEffectDeltas: operationalNormalReturnNoop,
		callboundary.LaneEscapeEvents: func(_ transfer.NodeContext, _ operationalEffectContext, effects signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.EscapeEvents = operationalEscapeEvents(effects)
		},
		callboundary.LaneStoreRelations: func(_ transfer.NodeContext, _ operationalEffectContext, effects signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.StoreRelations = operationalStoreRelations(effects)
		},
		callboundary.LaneLifecycleFacts: func(_ transfer.NodeContext, _ operationalEffectContext, effects signature.OperationalEffects, out *callboundary.NormalReturnFacts) {
			out.LifecycleFacts = operationalLifecycleFacts(effects)
		},
		callboundary.LaneNumFloors:      operationalNormalReturnNoop,
		callboundary.LaneRelConstraints: operationalNormalReturnNoop,
	},
	func(handler operationalNormalReturnLaneHandler) bool { return handler != nil },
)

func operationalNormalReturnNoop(transfer.NodeContext, operationalEffectContext, signature.OperationalEffects, *callboundary.NormalReturnFacts) {
}

func applyOperationalNormalReturnFacts(
	ctx transfer.NodeContext,
	op operationalEffectContext,
	effects signature.OperationalEffects,
	out *callboundary.NormalReturnFacts,
) {
	for _, lane := range operationalNormalReturnLanes {
		lane.Value(ctx, op, effects, out)
	}
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

func operationalPathTypeRefinements(ctx transfer.NodeContext, typeValues *typevalue.Cache, e signature.OperationalEffects) []callboundary.PathValueFact {
	if ctx.Registry == nil || len(e.NormalReturnTypeRefinements) == 0 {
		return nil
	}
	out := make([]callboundary.PathValueFact, 0, len(e.NormalReturnTypeRefinements))
	for _, refinement := range e.NormalReturnTypeRefinements {
		if refinement.Type == nil {
			continue
		}
		value := returnValueFromTypeCached(ctx.Registry, typeValues, refinement.Type)
		claim := refinement.Assertion
		if claim.IsBottom() {
			claim = assertion.Top()
		}
		if !claim.IsTop() {
			value = product.Set(ctx.Registry, value, assertion.Key, claim)
		}
		out = append(out, callboundary.PathValueFact{
			Path:  refinement.Path,
			Value: value,
		})
	}
	return out
}

func operationalPathStaticMembers(ctx transfer.NodeContext, typeValues *typevalue.Cache, e signature.OperationalEffects) []callboundary.PathStaticMemberFact {
	if ctx.Registry == nil || len(e.PathStaticMembers) == 0 {
		return nil
	}
	out := make([]callboundary.PathStaticMemberFact, 0, len(e.PathStaticMembers))
	for _, fact := range e.PathStaticMembers {
		if fact.Type == nil {
			continue
		}
		value := returnValueFromTypeCached(ctx.Registry, typeValues, fact.Type)
		out = append(out, callboundary.PathStaticMemberFact{
			Path:  fact.Path,
			Value: value,
		})
	}
	return out
}

func operationalPathPresenceImplications(ctx transfer.NodeContext, typeValues *typevalue.Cache, e signature.OperationalEffects) []callboundary.PathPresenceImplicationFact {
	if ctx.Registry == nil || len(e.PathPresenceImplications) == 0 {
		return nil
	}
	out := make([]callboundary.PathPresenceImplicationFact, 0, len(e.PathPresenceImplications))
	for _, fact := range e.PathPresenceImplications {
		implication := callboundary.PathPresenceImplicationFact{
			Trigger:         fact.Trigger,
			TriggerPresence: fact.TriggerPresence,
			HasTriggerValue: fact.HasTriggerType,
			Target:          fact.Target,
			TargetPresence:  fact.TargetPresence,
		}
		if fact.HasTriggerType {
			if fact.TriggerType == nil {
				continue
			}
			implication.TriggerValue = returnValueFromTypeCached(ctx.Registry, typeValues, fact.TriggerType)
		}
		out = append(out, implication)
	}
	return out
}

func operationalBranchProofs(e signature.OperationalEffects) []callboundary.BranchProof {
	if len(e.BranchProofs) == 0 {
		return nil
	}
	out := make([]callboundary.BranchProof, 0, len(e.BranchProofs))
	for _, proof := range e.BranchProofs {
		kind, ok := callBoundaryBranchProofKind(proof.Kind)
		if !ok {
			continue
		}
		out = append(out, callboundary.BranchProof{
			Kind:     kind,
			Path:     proof.Path,
			Presence: proof.Presence,
			Other:    proof.Other,
		})
	}
	return out
}

func callBoundaryBranchProofKind(kind signature.BranchProofKind) (pathevidence.BranchProofKind, bool) {
	switch kind {
	case signature.BranchProofPathPresence:
		return pathevidence.BranchProofPathPresence, true
	case signature.BranchProofPathEqual:
		return pathevidence.BranchProofPathEqual, true
	case signature.BranchProofPathNotEqual:
		return pathevidence.BranchProofPathNotEqual, true
	case signature.BranchProofIndexInRange:
		return pathevidence.BranchProofIndexInRange, true
	default:
		return 0, false
	}
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
		return returnValueFromTypeCached(ctx.Registry, op.typeValues, operand.Type), true
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
	resolver := op.expressionRefinements.Bind(ctx.Registry, op.sources)
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

func operationalKeyMemberships(e signature.OperationalEffects) []callboundary.KeyMembershipFact {
	return projectOperationalFacts(e.KeyMemberships, func(f signature.KeyMembership) callboundary.KeyMembershipFact {
		return callboundary.KeyMembershipFact{Key: f.Key, Table: f.Table}
	})
}

func operationalDynamicValueKeys(e signature.OperationalEffects) []callboundary.DynamicValueKeyMembershipFact {
	return projectOperationalFacts(e.DynamicValueKeys, func(f signature.DynamicValueKeyMembership) callboundary.DynamicValueKeyMembershipFact {
		return callboundary.DynamicValueKeyMembershipFact{
			Container: f.Container,
			Site:      dynamicindex.Site(f.Site),
			Table:     f.Table,
		}
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

func operationalReturnAllocationValue(reg *axis.Registry, typeValues *typevalue.Cache, effects *signature.OperationalEffects, signatureType *typ.Function, point cfg.Point, returnIndex int, value product.Value) product.Value {
	if reg == nil || effects == nil || len(effects.ReturnAllocationTemplates) == 0 {
		return value
	}
	for _, template := range effects.ReturnAllocationTemplates {
		if template.ReturnIndex != returnIndex || template.Root == "" {
			continue
		}
		if rootValue, ok := returnAllocationTemplateRootValue(reg, typeValues, template, signatureType, point, value); ok {
			value = refineReturnAllocationValue(reg, value, rootValue)
		}
		return product.Set(reg, value, identity.Key, identity.Singleton(allocationTemplateIdentityAt(point, template.Root)))
	}
	return value
}

func refineReturnAllocationValue(reg *axis.Registry, current, allocated product.Value) product.Value {
	currentType, currentOK := typevalue.TypeOf(reg, current)
	allocatedType, allocatedOK := typevalue.TypeOf(reg, allocated)
	if currentOK && allocatedOK {
		switch {
		case subtype.IsSubtype(currentType, allocatedType):
			return current
		case subtype.IsSubtype(allocatedType, currentType):
			return allocated
		}
	}
	return refinement.MeetConstraint(reg, current, allocated)
}

func returnAllocationTemplateRootValue(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	template signature.ReturnAllocationTemplate,
	signatureType *typ.Function,
	point cfg.Point,
	current product.Value,
) (product.Value, bool) {
	for _, object := range template.Objects {
		if object.ID != template.Root {
			continue
		}
		rootType := closeUninferredSignatureTypeParams(signatureType, object.Type)
		if rootType == nil {
			return product.Value{}, false
		}
		value := returnValueFromTypeCached(reg, typeValues, rootType)
		return product.WithPresence(reg, value, product.PresenceOf(current)), true
	}
	return product.Value{}, false
}

func operationalHeapTableObjects(ctx transfer.NodeContext, typeValues *typevalue.Cache, ks *keyspace.KeySpace, signatureType *typ.Function, e signature.OperationalEffects) map[identity.ID]heapidentity.TableObject {
	if ctx.Registry == nil || ks == nil || len(e.ReturnAllocationTemplates) == 0 {
		return nil
	}
	out := make(map[identity.ID]heapidentity.TableObject)
	for _, template := range e.ReturnAllocationTemplates {
		objectTypes := allocationObjectTypes(template.Objects, signatureType)
		for _, object := range template.Objects {
			if object.ID == "" {
				continue
			}
			id := allocationTemplateIdentityAt(ctx.Point, object.ID)
			root := allocationTemplateValue(ctx.Registry, typeValues, ctx.Point, object.ID, object.Type, signatureType)
			staticMembers := make(map[keyspace.Key]product.Value, len(object.StaticMembers))
			for _, member := range object.StaticMembers {
				if member.Value == "" {
					continue
				}
				key, ok := heapidentity.StaticMemberSuffixKey(ks, member.Suffix)
				if !ok {
					continue
				}
				staticMembers[key] = allocationTemplateValue(ctx.Registry, typeValues, ctx.Point, member.Value, objectTypes[member.Value], nil)
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
					KeyValue:    allocationTemplateKeyValue(ctx.Registry, typeValues, ctx.Point, entry, signatureType),
					Value:       allocationTemplateValue(ctx.Registry, typeValues, ctx.Point, entry.Value, objectTypes[entry.Value], nil),
					Admission:   dynamicindex.AdmissionAdmitted,
				}
			}
			out[id] = heapidentity.NewTableObject(heapidentity.TableObjectConfig{
				Root:              root,
				StaticMembers:     staticMembers,
				DynamicIndexFacts: dynamicEntries,
				StableShape:       object.StableShape,
			})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func allocationObjectTypes(objects []signature.AllocationObjectTemplate, signatureType *typ.Function) map[signature.AllocationTemplateID]typ.Type {
	if len(objects) == 0 {
		return nil
	}
	out := make(map[signature.AllocationTemplateID]typ.Type, len(objects))
	for _, object := range objects {
		t := closeUninferredSignatureTypeParams(signatureType, object.Type)
		if object.ID != "" && t != nil && !typ.ContainsTypeParam(t) {
			out[object.ID] = t
		}
	}
	return out
}

func operationalAllocationPlacements(point cfg.Point, e signature.OperationalEffects) map[identity.ID]placement.Value {
	if len(e.ReturnAllocationTemplates) == 0 {
		return nil
	}
	out := make(map[identity.ID]placement.Value)
	for _, template := range e.ReturnAllocationTemplates {
		for _, object := range template.Objects {
			if object.ID == "" {
				continue
			}
			id := allocationTemplateIdentityAt(point, object.ID)
			out[id] = placement.Join(out[id], placement.OwnedHeap)
		}
	}
	return out
}

func allocationTemplateKeyValue(reg *axis.Registry, typeValues *typevalue.Cache, point cfg.Point, entry signature.AllocationDynamicEntryTemplate, signatureType *typ.Function) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	if keyType := closeUninferredSignatureTypeParams(signatureType, entry.KeyType); keyType != nil {
		value = returnValueFromTypeCached(reg, typeValues, keyType)
	}
	if entry.Key != "" {
		value = product.Set(reg, value, identity.Key, identity.Singleton(allocationTemplateIdentityAt(point, entry.Key)))
	}
	return value
}

func allocationTemplateValue(reg *axis.Registry, typeValues *typevalue.Cache, point cfg.Point, id signature.AllocationTemplateID, t typ.Type, signatureType *typ.Function) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	if t = closeUninferredSignatureTypeParams(signatureType, t); t != nil {
		value = returnValueFromTypeCached(reg, typeValues, t)
	}
	if id == "" {
		return value
	}
	return product.Set(reg, value, identity.Key, identity.Singleton(allocationTemplateIdentityAt(point, id)))
}

func allocationTemplateIdentity(id signature.AllocationTemplateID) identity.ID {
	return allocationTemplateIdentityAt(0, id)
}

func allocationTemplateIdentityAt(point cfg.Point, id signature.AllocationTemplateID) identity.ID {
	return identity.ID{Kind: "manifest.allocation", Site: string(id), Index: uint64(point)}
}
