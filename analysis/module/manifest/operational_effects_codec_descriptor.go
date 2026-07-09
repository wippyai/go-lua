package manifest

import (
	"fmt"
	"sort"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

// wireLane builds one operationalEffectsWireLane from per-element behavior. It
// is the descriptor constructor for the OperationalEffects wire codec: a lane
// is fully described by the source fact slice, the wire slice, a per-element
// encode/decode pair, and a canonical comparison. The generic driver walks the
// registered lanes for whole-OperationalEffects encode, decode, and canonical
// ordering, so adding a fact kind is a single descriptor entry rather than a
// new plural encoder/decoder plus an inline sort closure.
//
// Growth sketch. The same descriptor shape extends to the producer side: a
// summary-slot generator can attach, keyed by lane, a projector from
// callboundary.NormalReturnFacts into the source fact slice this lane already
// serializes. Because storage lanes (callboundary.NormalReturnFactLanes) and
// wire lanes share field names and per-element vocabulary, a future
// CallOutcome/NormalReturnFacts generation pass registers one handler per lane
// against these descriptors instead of hand-threading each field, mirroring the
// callboundary lane binding. That generation is intentionally not implemented
// here; this file owns only the wire layer.
func wireLane[Fact any, Wire any](
	fieldName string,
	facts func(*signature.OperationalEffects) *[]Fact,
	wires func(*operationalEffectsWire) *[]Wire,
	encodeElem func(Fact) (Wire, error),
	decodeElem func(Wire) (Fact, error),
	compare func(Wire, Wire) int,
	canonElem func(*Wire),
) operationalEffectsWireLane {
	return operationalEffectsWireLane{
		fieldName: fieldName,
		encode: func(e *signature.OperationalEffects, out *operationalEffectsWire) error {
			src := *facts(e)
			if len(src) == 0 {
				return nil
			}
			dst := wires(out)
			for i := range src {
				encoded, err := encodeElem(src[i])
				if err != nil {
					return err
				}
				*dst = append(*dst, encoded)
			}
			return nil
		},
		decode: func(w *operationalEffectsWire, out *signature.OperationalEffects) error {
			src := *wires(w)
			if len(src) == 0 {
				return nil
			}
			dst := facts(out)
			for i := range src {
				decoded, err := decodeElem(src[i])
				if err != nil {
					return err
				}
				*dst = append(*dst, decoded)
			}
			return nil
		},
		canonicalize: func(w *operationalEffectsWire) {
			dst := wires(w)
			if canonElem != nil {
				for i := range *dst {
					canonElem(&(*dst)[i])
				}
			}
			sort.Slice(*dst, func(i, j int) bool {
				return compare((*dst)[i], (*dst)[j]) < 0
			})
		},
	}
}

func boolWireLane(
	fieldName string,
	get func(*signature.OperationalEffects) *bool,
	wire func(*operationalEffectsWire) *bool,
) operationalEffectsWireLane {
	return operationalEffectsWireLane{
		fieldName: fieldName,
		encode: func(e *signature.OperationalEffects, out *operationalEffectsWire) error {
			if *get(e) {
				*wire(out) = true
			}
			return nil
		},
		decode: func(w *operationalEffectsWire, out *signature.OperationalEffects) error {
			*get(out) = *wire(w)
			return nil
		},
		canonicalize: func(*operationalEffectsWire) {},
	}
}

// operationalEffectsWireLanes is the descriptor-driven wire lane table. It
// registers one entry per OperationalEffects fact kind in canonical field order;
// encodeOperationalEffects, decodeOperationalEffects, and
// canonicalizeOperationalEffectsWire drive it. Adding a fact kind is a single
// wireLane entry.
var operationalEffectsWireLanes = []operationalEffectsWireLane{
	boolWireLane("MaySuspend",
		func(e *signature.OperationalEffects) *bool { return &e.MaySuspend },
		func(w *operationalEffectsWire) *bool { return &w.MaySuspend }),
	wireLane("ReturnPresenceRelations",
		func(e *signature.OperationalEffects) *[]signature.ReturnPresenceRelation {
			return &e.ReturnPresenceRelations
		},
		func(w *operationalEffectsWire) *[]returnPresenceRelationWire { return &w.ReturnPresenceRelations },
		encodeReturnPresenceRelation, decodeReturnPresenceRelation, compareReturnPresenceRelationWire, nil),
	wireLane("NormalReturnPresenceRefinements",
		func(e *signature.OperationalEffects) *[]signature.PathPresenceRefinement {
			return &e.NormalReturnPresenceRefinements
		},
		func(w *operationalEffectsWire) *[]pathPresenceRefinementWire {
			return &w.NormalReturnPresenceRefinements
		},
		encodeNormalReturnPresenceRefinement, decodeNormalReturnPresenceRefinement, comparePathPresenceRefinementWire, nil),
	wireLane("NormalReturnTypeRefinements",
		func(e *signature.OperationalEffects) *[]signature.PathTypeRefinement {
			return &e.NormalReturnTypeRefinements
		},
		func(w *operationalEffectsWire) *[]pathTypeRefinementWire { return &w.NormalReturnTypeRefinements },
		encodeNormalReturnTypeRefinement, decodeNormalReturnTypeRefinement, comparePathTypeRefinementWire, nil),
	wireLane("PathPresenceImplications",
		func(e *signature.OperationalEffects) *[]signature.PathPresenceImplication {
			return &e.PathPresenceImplications
		},
		func(w *operationalEffectsWire) *[]pathPresenceImplicationWire { return &w.PathPresenceImplications },
		encodePathPresenceImplication, decodePathPresenceImplication, comparePathPresenceImplicationWire, nil),
	wireLane("PathStaticMembers",
		func(e *signature.OperationalEffects) *[]signature.PathStaticMemberFact { return &e.PathStaticMembers },
		func(w *operationalEffectsWire) *[]pathStaticMemberWire { return &w.PathStaticMembers },
		encodePathStaticMember, decodePathStaticMember, comparePathStaticMemberWire, nil),
	wireLane("PathInvalidations",
		func(e *signature.OperationalEffects) *[]signature.PathInvalidation { return &e.PathInvalidations },
		func(w *operationalEffectsWire) *[]pathInvalidationWire { return &w.PathInvalidations },
		encodePathInvalidation, decodePathInvalidation, comparePathInvalidationWire, nil),
	wireLane("BranchProofs",
		func(e *signature.OperationalEffects) *[]signature.BranchProof { return &e.BranchProofs },
		func(w *operationalEffectsWire) *[]branchProofWire { return &w.BranchProofs },
		encodeBranchProof, decodeBranchProof, compareBranchProofWire, nil),
	wireLane("DynamicIndexFacts",
		func(e *signature.OperationalEffects) *[]signature.DynamicIndexFact { return &e.DynamicIndexFacts },
		func(w *operationalEffectsWire) *[]dynamicIndexFactWire { return &w.DynamicIndexFacts },
		encodeDynamicIndexFact, decodeDynamicIndexFact, compareDynamicIndexFactWire, nil),
	wireLane("KeyMemberships",
		func(e *signature.OperationalEffects) *[]signature.KeyMembership { return &e.KeyMemberships },
		func(w *operationalEffectsWire) *[]keyMembershipWire { return &w.KeyMemberships },
		encodeKeyMembership, decodeKeyMembership, compareKeyMembershipWire, nil),
	wireLane("DynamicValueKeys",
		func(e *signature.OperationalEffects) *[]signature.DynamicValueKeyMembership {
			return &e.DynamicValueKeys
		},
		func(w *operationalEffectsWire) *[]dynamicValueKeyMembershipWire { return &w.DynamicValueKeys },
		encodeDynamicValueKeyMembership, decodeDynamicValueKeyMembership, compareDynamicValueKeyMembershipWire, nil),
	wireLane("FrozenTables",
		func(e *signature.OperationalEffects) *[]signature.FrozenTable { return &e.FrozenTables },
		func(w *operationalEffectsWire) *[]frozenTableWire { return &w.FrozenTables },
		encodeFrozenTable, decodeFrozenTable, compareFrozenTableWire, nil),
	wireLane("EscapeEvents",
		func(e *signature.OperationalEffects) *[]signature.EscapeEvent { return &e.EscapeEvents },
		func(w *operationalEffectsWire) *[]escapeEventWire { return &w.EscapeEvents },
		encodeEscapeEvent, decodeEscapeEvent, compareEscapeEventWire, nil),
	wireLane("StoreRelations",
		func(e *signature.OperationalEffects) *[]signature.StoreRelation { return &e.StoreRelations },
		func(w *operationalEffectsWire) *[]storeRelationWire { return &w.StoreRelations },
		encodeStoreRelation, decodeStoreRelation, compareStoreRelationWire, nil),
	wireLane("ParamRelations",
		func(e *signature.OperationalEffects) *[]signature.ParamRelation { return &e.ParamRelations },
		func(w *operationalEffectsWire) *[]paramRelationWire { return &w.ParamRelations },
		encodeParamRelation, decodeParamRelation, compareParamRelationWire, nil),
	wireLane("ReturnFlows",
		func(e *signature.OperationalEffects) *[]signature.ReturnFlow { return &e.ReturnFlows },
		func(w *operationalEffectsWire) *[]returnFlowWire { return &w.ReturnFlows },
		encodeReturnFlow, decodeReturnFlow, compareReturnFlowWire, nil),
	wireLane("LifecycleEffects",
		func(e *signature.OperationalEffects) *[]signature.LifecycleEffect { return &e.LifecycleEffects },
		func(w *operationalEffectsWire) *[]lifecycleEffectWire { return &w.LifecycleEffects },
		encodeLifecycleEffect, decodeLifecycleEffect, compareLifecycleEffectWire, nil),
	wireLane("ReturnAllocationTemplates",
		func(e *signature.OperationalEffects) *[]signature.ReturnAllocationTemplate {
			return &e.ReturnAllocationTemplates
		},
		func(w *operationalEffectsWire) *[]returnAllocationTemplateWire { return &w.ReturnAllocationTemplates },
		encodeReturnAllocationTemplate, decodeReturnAllocationTemplate, compareReturnAllocationTemplateWire,
		canonicalizeReturnAllocationTemplateWire),
}

func encodeReturnPresenceRelation(relation signature.ReturnPresenceRelation) (returnPresenceRelationWire, error) {
	trigger, err := encodePresence(relation.TriggerPresence)
	if err != nil {
		return returnPresenceRelationWire{}, fmt.Errorf("return relation trigger presence: %w", err)
	}
	target, err := encodePresence(relation.TargetPresence)
	if err != nil {
		return returnPresenceRelationWire{}, fmt.Errorf("return relation target presence: %w", err)
	}
	return returnPresenceRelationWire{
		TriggerIndex:    encodeInt(relation.TriggerIndex),
		TriggerPresence: trigger,
		TargetIndex:     encodeInt(relation.TargetIndex),
		TargetPresence:  target,
	}, nil
}

func decodeReturnPresenceRelation(w returnPresenceRelationWire) (signature.ReturnPresenceRelation, error) {
	trigger, err := decodePresence(w.TriggerPresence)
	if err != nil {
		return signature.ReturnPresenceRelation{}, fmt.Errorf("return relation trigger presence: %w", err)
	}
	target, err := decodePresence(w.TargetPresence)
	if err != nil {
		return signature.ReturnPresenceRelation{}, fmt.Errorf("return relation target presence: %w", err)
	}
	triggerIndex, err := decodeRequiredInt(w.TriggerIndex, "return relation trigger index missing")
	if err != nil {
		return signature.ReturnPresenceRelation{}, err
	}
	targetIndex, err := decodeRequiredInt(w.TargetIndex, "return relation target index missing")
	if err != nil {
		return signature.ReturnPresenceRelation{}, err
	}
	return signature.ReturnPresenceRelation{
		TriggerIndex:    triggerIndex,
		TriggerPresence: trigger,
		TargetIndex:     targetIndex,
		TargetPresence:  target,
	}, nil
}

func encodeNormalReturnPresenceRefinement(refinement signature.PathPresenceRefinement) (pathPresenceRefinementWire, error) {
	p, err := encodePlaceholderPath(refinement.Path)
	if err != nil {
		return pathPresenceRefinementWire{}, fmt.Errorf("normal return presence refinement path: %w", err)
	}
	pr, err := encodePresence(refinement.Presence)
	if err != nil {
		return pathPresenceRefinementWire{}, fmt.Errorf("normal return presence refinement: %w", err)
	}
	return pathPresenceRefinementWire{Path: p, Presence: pr}, nil
}

func decodeNormalReturnPresenceRefinement(w pathPresenceRefinementWire) (signature.PathPresenceRefinement, error) {
	p, err := decodePlaceholderPath(w.Path)
	if err != nil {
		return signature.PathPresenceRefinement{}, fmt.Errorf("normal return presence refinement path: %w", err)
	}
	pr, err := decodePresence(w.Presence)
	if err != nil {
		return signature.PathPresenceRefinement{}, fmt.Errorf("normal return presence refinement: %w", err)
	}
	return signature.PathPresenceRefinement{Path: p, Presence: pr}, nil
}

func encodeNormalReturnTypeRefinement(refinement signature.PathTypeRefinement) (pathTypeRefinementWire, error) {
	p, err := encodePlaceholderPath(refinement.Path)
	if err != nil {
		return pathTypeRefinementWire{}, fmt.Errorf("normal return type refinement path: %w", err)
	}
	if refinement.Type == nil {
		return pathTypeRefinementWire{}, fmt.Errorf("normal return type refinement type: missing")
	}
	t, err := encodeType(refinement.Type)
	if err != nil {
		return pathTypeRefinementWire{}, fmt.Errorf("normal return type refinement type: %w", err)
	}
	return pathTypeRefinementWire{
		Path:       p,
		Type:       t,
		Assertions: encodeAssertion(refinement.Assertion),
	}, nil
}

func decodeNormalReturnTypeRefinement(w pathTypeRefinementWire) (signature.PathTypeRefinement, error) {
	p, err := decodePlaceholderPath(w.Path)
	if err != nil {
		return signature.PathTypeRefinement{}, fmt.Errorf("normal return type refinement path: %w", err)
	}
	t, err := decodeType(w.Type)
	if err != nil {
		return signature.PathTypeRefinement{}, fmt.Errorf("normal return type refinement type: %w", err)
	}
	if t == nil {
		return signature.PathTypeRefinement{}, fmt.Errorf("normal return type refinement type: missing")
	}
	assertionClaim, err := decodeAssertion(w.Assertions)
	if err != nil {
		return signature.PathTypeRefinement{}, fmt.Errorf("normal return type refinement assertions: %w", err)
	}
	return signature.PathTypeRefinement{Path: p, Type: t, Assertion: assertionClaim}, nil
}

func encodePathStaticMember(member signature.PathStaticMemberFact) (pathStaticMemberWire, error) {
	p, err := encodePlaceholderPath(member.Path)
	if err != nil {
		return pathStaticMemberWire{}, fmt.Errorf("path static member path: %w", err)
	}
	if member.Type == nil {
		return pathStaticMemberWire{}, fmt.Errorf("path static member type: missing")
	}
	t, err := encodeType(member.Type)
	if err != nil {
		return pathStaticMemberWire{}, fmt.Errorf("path static member type: %w", err)
	}
	return pathStaticMemberWire{Path: p, Type: t}, nil
}

func decodePathStaticMember(w pathStaticMemberWire) (signature.PathStaticMemberFact, error) {
	p, err := decodePlaceholderPath(w.Path)
	if err != nil {
		return signature.PathStaticMemberFact{}, fmt.Errorf("path static member path: %w", err)
	}
	t, err := decodeType(w.Type)
	if err != nil {
		return signature.PathStaticMemberFact{}, fmt.Errorf("path static member type: %w", err)
	}
	if t == nil {
		return signature.PathStaticMemberFact{}, fmt.Errorf("path static member type: missing")
	}
	return signature.PathStaticMemberFact{Path: p, Type: t}, nil
}

func encodePathInvalidation(invalidation signature.PathInvalidation) (pathInvalidationWire, error) {
	p, err := encodePlaceholderPath(invalidation.Path)
	if err != nil {
		return pathInvalidationWire{}, fmt.Errorf("path invalidation: %w", err)
	}
	return pathInvalidationWire{Path: p}, nil
}

func decodePathInvalidation(w pathInvalidationWire) (signature.PathInvalidation, error) {
	p, err := decodePlaceholderPath(w.Path)
	if err != nil {
		return signature.PathInvalidation{}, fmt.Errorf("path invalidation: %w", err)
	}
	return signature.PathInvalidation{Path: p}, nil
}

func encodeFrozenTable(frozen signature.FrozenTable) (frozenTableWire, error) {
	p, err := encodePlaceholderPath(frozen.Target)
	if err != nil {
		return frozenTableWire{}, fmt.Errorf("frozen table: %w", err)
	}
	return frozenTableWire{Target: p}, nil
}

func decodeFrozenTable(w frozenTableWire) (signature.FrozenTable, error) {
	p, err := decodePlaceholderPath(w.Target)
	if err != nil {
		return signature.FrozenTable{}, fmt.Errorf("frozen table: %w", err)
	}
	return signature.FrozenTable{Target: p}, nil
}

func encodeEscapeEvent(event signature.EscapeEvent) (escapeEventWire, error) {
	p, err := encodePlaceholderPath(event.Target)
	if err != nil {
		return escapeEventWire{}, fmt.Errorf("escape event target: %w", err)
	}
	kind, err := encodeEscapeKind(event.Kind)
	if err != nil {
		return escapeEventWire{}, err
	}
	return escapeEventWire{Target: p, Kind: kind, Recursive: event.Recursive}, nil
}

func decodeEscapeEvent(w escapeEventWire) (signature.EscapeEvent, error) {
	p, err := decodePlaceholderPath(w.Target)
	if err != nil {
		return signature.EscapeEvent{}, fmt.Errorf("escape event target: %w", err)
	}
	kind, err := decodeEscapeKind(w.Kind)
	if err != nil {
		return signature.EscapeEvent{}, err
	}
	return signature.EscapeEvent{Target: p, Kind: kind, Recursive: w.Recursive}, nil
}

func encodeStoreRelation(relation signature.StoreRelation) (storeRelationWire, error) {
	source, err := encodePlaceholderPath(relation.Source)
	if err != nil {
		return storeRelationWire{}, fmt.Errorf("store relation source: %w", err)
	}
	into, err := encodePlaceholderPath(relation.Into)
	if err != nil {
		return storeRelationWire{}, fmt.Errorf("store relation target: %w", err)
	}
	return storeRelationWire{Source: source, Into: into}, nil
}

func decodeStoreRelation(w storeRelationWire) (signature.StoreRelation, error) {
	source, err := decodePlaceholderPath(w.Source)
	if err != nil {
		return signature.StoreRelation{}, fmt.Errorf("store relation source: %w", err)
	}
	into, err := decodePlaceholderPath(w.Into)
	if err != nil {
		return signature.StoreRelation{}, fmt.Errorf("store relation target: %w", err)
	}
	return signature.StoreRelation{Source: source, Into: into}, nil
}

func encodeParamRelation(relation signature.ParamRelation) (paramRelationWire, error) {
	if relation.Param < 0 {
		return paramRelationWire{}, fmt.Errorf("param relation index %d out of range", relation.Param)
	}
	escapeClass, err := encodeEscapeKind(relation.EscapeClass)
	if err != nil {
		return paramRelationWire{}, fmt.Errorf("param relation escape class: %w", err)
	}
	placementConsequence, err := encodePlacementConsequence(relation.PlacementConsequence)
	if err != nil {
		return paramRelationWire{}, fmt.Errorf("param relation placement consequence: %w", err)
	}
	out := paramRelationWire{
		Param:                encodeInt(relation.Param),
		EscapeClass:          escapeClass,
		PlacementConsequence: placementConsequence,
		ThroughReturn:        relation.ThroughReturn,
	}
	if relation.HasStoredInto {
		if relation.StoredInto < 0 {
			return paramRelationWire{}, fmt.Errorf("param relation storedInto %d out of range", relation.StoredInto)
		}
		out.StoredInto = encodeInt(relation.StoredInto)
	}
	return out, nil
}

func decodeParamRelation(w paramRelationWire) (signature.ParamRelation, error) {
	param, err := decodeRequiredInt(w.Param, "param relation index missing")
	if err != nil {
		return signature.ParamRelation{}, err
	}
	if param < 0 {
		return signature.ParamRelation{}, fmt.Errorf("param relation index %d out of range", param)
	}
	escapeClass, err := decodeEscapeKind(w.EscapeClass)
	if err != nil {
		return signature.ParamRelation{}, fmt.Errorf("param relation escape class: %w", err)
	}
	placementConsequence, err := decodePlacementConsequence(w.PlacementConsequence)
	if err != nil {
		return signature.ParamRelation{}, fmt.Errorf("param relation placement consequence: %w", err)
	}
	out := signature.ParamRelation{
		Param:                param,
		EscapeClass:          escapeClass,
		PlacementConsequence: placementConsequence,
		ThroughReturn:        w.ThroughReturn,
	}
	if w.StoredInto != nil {
		storedInto, err := decodeRequiredInt(w.StoredInto, "param relation storedInto missing")
		if err != nil {
			return signature.ParamRelation{}, err
		}
		if storedInto < 0 {
			return signature.ParamRelation{}, fmt.Errorf("param relation storedInto %d out of range", storedInto)
		}
		out.StoredInto = storedInto
		out.HasStoredInto = true
	}
	return out, nil
}

func encodeReturnFlow(flow signature.ReturnFlow) (returnFlowWire, error) {
	if flow.ReturnIndex < 0 {
		return returnFlowWire{}, fmt.Errorf("return flow index %d out of range", flow.ReturnIndex)
	}
	if flow.Param < 0 {
		return returnFlowWire{}, fmt.Errorf("return flow param %d out of range", flow.Param)
	}
	kind, err := encodeReturnFlowKind(flow.Kind)
	if err != nil {
		return returnFlowWire{}, fmt.Errorf("return flow kind: %w", err)
	}
	out := returnFlowWire{
		ReturnIndex: encodeInt(flow.ReturnIndex),
		Kind:        kind,
		Param:       encodeInt(flow.Param),
	}
	switch flow.Kind {
	case signature.ReturnFlowParam:
		if len(flow.Path) != 0 {
			return returnFlowWire{}, fmt.Errorf("return flow ReturnsParam must not carry path")
		}
	case signature.ReturnFlowParamMember:
		key, ok := pathaddr.RelativeStaticMemberSuffixKey(flow.Path)
		if !ok {
			return returnFlowWire{}, fmt.Errorf("return flow member path %q is not a static member suffix", segment.FormatSegments(flow.Path))
		}
		out.Path = string(key.PathKey())
	}
	return out, nil
}

func decodeReturnFlow(w returnFlowWire) (signature.ReturnFlow, error) {
	returnIndex, err := decodeRequiredInt(w.ReturnIndex, "return flow index missing")
	if err != nil {
		return signature.ReturnFlow{}, err
	}
	if returnIndex < 0 {
		return signature.ReturnFlow{}, fmt.Errorf("return flow index %d out of range", returnIndex)
	}
	param, err := decodeRequiredInt(w.Param, "return flow param missing")
	if err != nil {
		return signature.ReturnFlow{}, err
	}
	if param < 0 {
		return signature.ReturnFlow{}, fmt.Errorf("return flow param %d out of range", param)
	}
	kind, err := decodeReturnFlowKind(w.Kind)
	if err != nil {
		return signature.ReturnFlow{}, fmt.Errorf("return flow kind: %w", err)
	}
	out := signature.ReturnFlow{
		ReturnIndex: returnIndex,
		Kind:        kind,
		Param:       param,
	}
	switch kind {
	case signature.ReturnFlowParam:
		if w.Path != "" {
			return signature.ReturnFlow{}, fmt.Errorf("return flow ReturnsParam must not carry path")
		}
	case signature.ReturnFlowParamMember:
		key, ok := pathaddr.SuffixKeyFromPathKey(pathdom.PathKey(w.Path))
		if !ok {
			return signature.ReturnFlow{}, fmt.Errorf("return flow member path %q is not a static member suffix", w.Path)
		}
		segs, ok := pathaddr.RelativeStaticMemberSuffixSegments(key)
		if !ok {
			return signature.ReturnFlow{}, fmt.Errorf("return flow member path %q is not parseable", w.Path)
		}
		out.Path = segs
	}
	return out, nil
}

func encodeReturnFlowKind(kind signature.ReturnFlowKind) (string, error) {
	switch kind {
	case signature.ReturnFlowParam:
		return "ReturnsParam", nil
	case signature.ReturnFlowParamMember:
		return "ReturnsParamMember", nil
	default:
		return "", fmt.Errorf("unknown return flow kind %d", kind)
	}
}

func decodeReturnFlowKind(kind string) (signature.ReturnFlowKind, error) {
	switch kind {
	case "ReturnsParam":
		return signature.ReturnFlowParam, nil
	case "ReturnsParamMember":
		return signature.ReturnFlowParamMember, nil
	default:
		return signature.ReturnFlowInvalid, fmt.Errorf("unknown return flow kind %q", kind)
	}
}

func compareReturnPresenceRelationWire(a, b returnPresenceRelationWire) int {
	if c := compareOptionalInt(a.TriggerIndex, b.TriggerIndex); c != 0 {
		return c
	}
	if c := strings.Compare(a.TriggerPresence, b.TriggerPresence); c != 0 {
		return c
	}
	if c := compareOptionalInt(a.TargetIndex, b.TargetIndex); c != 0 {
		return c
	}
	return strings.Compare(a.TargetPresence, b.TargetPresence)
}

func comparePathPresenceRefinementWire(a, b pathPresenceRefinementWire) int {
	if c := comparePlaceholderPathWire(a.Path, b.Path); c != 0 {
		return c
	}
	return strings.Compare(a.Presence, b.Presence)
}

func comparePathTypeRefinementWire(a, b pathTypeRefinementWire) int {
	if c := comparePlaceholderPathWire(a.Path, b.Path); c != 0 {
		return c
	}
	if c := strings.Compare(typeWireKey(a.Type), typeWireKey(b.Type)); c != 0 {
		return c
	}
	return strings.Compare(strings.Join(a.Assertions, ","), strings.Join(b.Assertions, ","))
}

func comparePathStaticMemberWire(a, b pathStaticMemberWire) int {
	if c := comparePlaceholderPathWire(a.Path, b.Path); c != 0 {
		return c
	}
	return strings.Compare(typeWireKey(a.Type), typeWireKey(b.Type))
}

func comparePathInvalidationWire(a, b pathInvalidationWire) int {
	return comparePlaceholderPathWire(a.Path, b.Path)
}

func compareDynamicIndexFactWire(a, b dynamicIndexFactWire) int {
	if c := compareBoundaryPathWire(a.Table, b.Table); c != 0 {
		return c
	}
	if c := strings.Compare(a.Site, b.Site); c != 0 {
		return c
	}
	if c := strings.Compare(a.KeyPresence, b.KeyPresence); c != 0 {
		return c
	}
	if c := compareDynamicIndexOperandWire(a.Key, b.Key); c != 0 {
		return c
	}
	if c := compareDynamicIndexOperandWire(a.Value, b.Value); c != 0 {
		return c
	}
	return strings.Compare(a.Admission, b.Admission)
}

func compareKeyMembershipWire(a, b keyMembershipWire) int {
	if c := compareBoundaryPathWire(a.Key, b.Key); c != 0 {
		return c
	}
	return compareBoundaryPathWire(a.Table, b.Table)
}

func compareDynamicValueKeyMembershipWire(a, b dynamicValueKeyMembershipWire) int {
	if c := compareBoundaryPathWire(a.Container, b.Container); c != 0 {
		return c
	}
	if c := strings.Compare(a.Site, b.Site); c != 0 {
		return c
	}
	return compareBoundaryPathWire(a.Table, b.Table)
}

func compareFrozenTableWire(a, b frozenTableWire) int {
	return comparePlaceholderPathWire(a.Target, b.Target)
}

func compareEscapeEventWire(a, b escapeEventWire) int {
	if c := comparePlaceholderPathWire(a.Target, b.Target); c != 0 {
		return c
	}
	if c := strings.Compare(a.Kind, b.Kind); c != 0 {
		return c
	}
	return compareBoolWire(a.Recursive, b.Recursive)
}

func compareStoreRelationWire(a, b storeRelationWire) int {
	if c := comparePlaceholderPathWire(a.Source, b.Source); c != 0 {
		return c
	}
	return comparePlaceholderPathWire(a.Into, b.Into)
}

func compareParamRelationWire(a, b paramRelationWire) int {
	if c := compareOptionalInt(a.Param, b.Param); c != 0 {
		return c
	}
	if c := strings.Compare(a.EscapeClass, b.EscapeClass); c != 0 {
		return c
	}
	if c := strings.Compare(a.PlacementConsequence, b.PlacementConsequence); c != 0 {
		return c
	}
	if c := compareBoolWire(a.ThroughReturn, b.ThroughReturn); c != 0 {
		return c
	}
	return compareOptionalInt(a.StoredInto, b.StoredInto)
}

func compareReturnFlowWire(a, b returnFlowWire) int {
	if c := compareOptionalInt(a.ReturnIndex, b.ReturnIndex); c != 0 {
		return c
	}
	if c := strings.Compare(a.Kind, b.Kind); c != 0 {
		return c
	}
	if c := compareOptionalInt(a.Param, b.Param); c != 0 {
		return c
	}
	return strings.Compare(a.Path, b.Path)
}

func compareReturnAllocationTemplateWire(a, b returnAllocationTemplateWire) int {
	if c := compareOptionalInt(a.ReturnIndex, b.ReturnIndex); c != 0 {
		return c
	}
	return strings.Compare(a.Root, b.Root)
}

// compareBoolWire orders false before true, matching the legacy escape-event
// canonical ordering.
func compareBoolWire(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	default:
		return 1
	}
}
