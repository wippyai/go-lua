package manifest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	internalhash "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type operationalEffectsWire struct {
	SuspensionKnown                 bool                            `json:"suspensionKnown,omitempty"`
	MaySuspend                      bool                            `json:"maySuspend,omitempty"`
	ReturnPresenceRelations         []returnPresenceRelationWire    `json:"returnPresenceRelations,omitempty"`
	NormalReturnPresenceRefinements []pathPresenceRefinementWire    `json:"normalReturnPresenceRefinements,omitempty"`
	NormalReturnTypeRefinements     []pathTypeRefinementWire        `json:"normalReturnTypeRefinements,omitempty"`
	PathPresenceImplications        []pathPresenceImplicationWire   `json:"pathPresenceImplications,omitempty"`
	PathStaticMembers               []pathStaticMemberWire          `json:"pathStaticMembers,omitempty"`
	PathStaticMemberDeltas          []pathStaticMemberDeltaWire     `json:"pathStaticMemberDeltas,omitempty"`
	PathInvalidations               []pathInvalidationWire          `json:"pathInvalidations,omitempty"`
	BranchProofs                    []branchProofWire               `json:"branchProofs,omitempty"`
	DynamicIndexFacts               []dynamicIndexFactWire          `json:"dynamicIndexFacts,omitempty"`
	KeyMemberships                  []keyMembershipWire             `json:"keyMemberships,omitempty"`
	DynamicValueKeys                []dynamicValueKeyMembershipWire `json:"dynamicValueKeys,omitempty"`
	FrozenTables                    []frozenTableWire               `json:"frozenTables,omitempty"`
	EscapeEvents                    []escapeEventWire               `json:"escapeEvents,omitempty"`
	StoreRelations                  []storeRelationWire             `json:"storeRelations,omitempty"`
	ParamRelations                  []paramRelationWire             `json:"paramRelations,omitempty"`
	ReturnFlows                     []returnFlowWire                `json:"returnFlows,omitempty"`
	LifecycleEffects                []lifecycleEffectWire           `json:"lifecycleEffects,omitempty"`
	TypestateRequirements           []typestateRequirementWire      `json:"typestateRequirements,omitempty"`
	ReturnAllocationTemplates       []returnAllocationTemplateWire  `json:"returnAllocationTemplates,omitempty"`
}

type operationalEffectsWireLane struct {
	fieldName    string
	encode       func(context.Context, *signature.OperationalEffects, *operationalEffectsWire) error
	decode       func(*operationalEffectsWire, *signature.OperationalEffects) error
	canonicalize func(context.Context, *operationalEffectsWire) error
}

type returnPresenceRelationWire struct {
	TriggerIndex    *int   `json:"triggerIndex"`
	TriggerPresence string `json:"triggerPresence"`
	TargetIndex     *int   `json:"targetIndex"`
	TargetPresence  string `json:"targetPresence"`
}

type pathPresenceRefinementWire struct {
	Path     *placeholderPathWire `json:"path,omitempty"`
	Presence string               `json:"presence"`
}

type pathTypeRefinementWire struct {
	Path       *placeholderPathWire `json:"path,omitempty"`
	Type       *typeWire            `json:"type,omitempty"`
	Assertions []string             `json:"assertions,omitempty"`
}

type pathPresenceImplicationWire struct {
	Trigger         *boundaryPathWire `json:"trigger,omitempty"`
	TriggerPresence string            `json:"triggerPresence"`
	TriggerType     *typeWire         `json:"triggerType,omitempty"`
	Target          *boundaryPathWire `json:"target,omitempty"`
	TargetPresence  string            `json:"targetPresence"`
}

type pathStaticMemberWire struct {
	Path *placeholderPathWire `json:"path,omitempty"`
	Type *typeWire            `json:"type,omitempty"`
}

type pathStaticMemberDeltaWire struct {
	Path     *placeholderPathWire `json:"path,omitempty"`
	Type     *typeWire            `json:"type,omitempty"`
	Required bool                 `json:"required,omitempty"`
}

type pathInvalidationWire struct {
	Path                      *placeholderPathWire `json:"path,omitempty"`
	PreserveStructuralWitness bool                 `json:"preserveStructuralWitness,omitempty"`
}

type branchProofWire struct {
	Kind     string            `json:"kind"`
	Path     *boundaryPathWire `json:"path,omitempty"`
	Presence string            `json:"presence,omitempty"`
	Other    *boundaryPathWire `json:"other,omitempty"`
}

type dynamicIndexFactWire struct {
	Table       *boundaryPathWire       `json:"table,omitempty"`
	Site        string                  `json:"site"`
	KeyPresence string                  `json:"keyPresence"`
	Key         dynamicIndexOperandWire `json:"key"`
	Value       dynamicIndexOperandWire `json:"value"`
	Admission   string                  `json:"admission"`
}

type dynamicIndexOperandWire struct {
	Path *placeholderPathWire `json:"path,omitempty"`
	Type *typeWire            `json:"type,omitempty"`
}

type keyMembershipWire struct {
	Key   *boundaryPathWire `json:"key,omitempty"`
	Table *boundaryPathWire `json:"table,omitempty"`
}

type dynamicValueKeyMembershipWire struct {
	Container *boundaryPathWire `json:"container,omitempty"`
	Site      string            `json:"site"`
	Table     *boundaryPathWire `json:"table,omitempty"`
}

type frozenTableWire struct {
	Target *placeholderPathWire `json:"target,omitempty"`
}

type escapeEventWire struct {
	Target    *placeholderPathWire `json:"target,omitempty"`
	Kind      string               `json:"kind"`
	Recursive bool                 `json:"recursive,omitempty"`
}

type storeRelationWire struct {
	Source *placeholderPathWire `json:"source,omitempty"`
	Into   *placeholderPathWire `json:"into,omitempty"`
}

type paramRelationWire struct {
	Param                *int   `json:"param"`
	EscapeClass          string `json:"escapeClass"`
	PlacementConsequence string `json:"placementConsequence"`
	ThroughReturn        bool   `json:"throughReturn,omitempty"`
	StoredInto           *int   `json:"storedInto,omitempty"`
}

type returnFlowWire struct {
	ReturnIndex *int   `json:"returnIndex"`
	Kind        string `json:"kind"`
	Param       *int   `json:"param"`
	Path        string `json:"path,omitempty"`
}

type lifecycleEffectWire struct {
	Target   *boundaryPathWire `json:"target,omitempty"`
	Kind     string            `json:"kind"`
	Protocol string            `json:"protocol"`
	From     string            `json:"from,omitempty"`
	To       string            `json:"to,omitempty"`
	Final    string            `json:"final,omitempty"`
	Finals   []string          `json:"finals,omitempty"`
}

type typestateRequirementWire struct {
	Target   *placeholderPathWire `json:"target,omitempty"`
	Protocol string               `json:"protocol"`
	State    string               `json:"state"`
}

type returnAllocationTemplateWire struct {
	ReturnIndex *int                   `json:"returnIndex"`
	Root        string                 `json:"root"`
	Objects     []allocationObjectWire `json:"objects,omitempty"`
}

type allocationObjectWire struct {
	ID             string                       `json:"id"`
	Type           *typeWire                    `json:"type,omitempty"`
	StableShape    bool                         `json:"stableShape,omitempty"`
	PrefixStable   bool                         `json:"prefixStable,omitempty"`
	StaticMembers  []allocationStaticMemberWire `json:"staticMembers,omitempty"`
	DynamicEntries []allocationDynamicEntryWire `json:"dynamicEntries,omitempty"`
}

type allocationStaticMemberWire struct {
	Suffix string `json:"suffix"`
	Value  string `json:"value"`
}

type allocationDynamicEntryWire struct {
	Key     string    `json:"key,omitempty"`
	KeyType *typeWire `json:"keyType,omitempty"`
	Value   string    `json:"value,omitempty"`
}

type placeholderPathWire struct {
	Param  *int   `json:"param"`
	Suffix string `json:"suffix,omitempty"`
}

type boundaryPathWire struct {
	Param  *int   `json:"param,omitempty"`
	Return *int   `json:"return,omitempty"`
	Suffix string `json:"suffix,omitempty"`
}

func encodeOperationalEffects(e *signature.OperationalEffects) (*operationalEffectsWire, error) {
	if e == nil || e.IsEmpty() {
		return nil, nil
	}
	out := &operationalEffectsWire{}
	for _, lane := range operationalEffectsWireLanes {
		if err := lane.encode(context.Background(), e, out); err != nil {
			return nil, err
		}
	}
	canonicalizeOperationalEffectsWire(out)
	return out, nil
}

func encodeOperationalEffectsContext(ctx context.Context, e *signature.OperationalEffects) (*operationalEffectsWire, error) {
	if e == nil || e.IsEmpty() {
		return nil, nil
	}
	out := &operationalEffectsWire{}
	for _, lane := range operationalEffectsWireLanes {
		if err := operationalEffectsContextErr(ctx); err != nil {
			return nil, err
		}
		if err := lane.encode(ctx, e, out); err != nil {
			return nil, err
		}
	}
	if err := canonicalizeOperationalEffectsWireContext(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// CanonicalOperationalEffectsBytes encodes operational effects through the
// descriptor-driven manifest wire codec. The returned JSON is stable across
// input slice order and is suitable for content digests.
func CanonicalOperationalEffectsBytes(e *signature.OperationalEffects) ([]byte, error) {
	w, err := encodeOperationalEffects(e)
	if err != nil {
		return nil, err
	}
	return json.Marshal(w)
}

// CanonicalOperationalEffectsDigestBytes encodes operational effects through
// the descriptor-driven manifest wire codec after replacing every embedded type
// with its equality identity. Digest clients need stable semantic type identity,
// not a re-decodable type graph; this also keeps recursive in-memory type
// graphs out of the acyclic manifest type codec.
func CanonicalOperationalEffectsDigestBytes(e *signature.OperationalEffects) ([]byte, error) {
	if e == nil {
		return CanonicalOperationalEffectsBytes(nil)
	}
	canonical := e.Clone()
	canonicalizeOperationalEffectDigestTypes(&canonical)
	return CanonicalOperationalEffectsBytes(&canonical)
}

// CanonicalOperationalEffectsDigest streams the canonical operational-effects
// wire value into a content hash. Unlike CanonicalOperationalEffectsBytes, it
// never materializes a JSON representation merely to hash it. The returned
// value is an internal cache-key component, not a manifest wire format.
func CanonicalOperationalEffectsDigest(ctx context.Context, e *signature.OperationalEffects) (uint64, error) {
	canonical, err := canonicalOperationalEffectsDigestCloneContext(ctx, e)
	if err != nil {
		return 0, err
	}
	w, err := encodeOperationalEffectsContext(ctx, canonical)
	if err != nil {
		return 0, err
	}
	h := internalhash.NewWriter()
	writer := operationalEffectsDigestWriter{ctx: ctx, h: &h}
	if err := writer.write(reflect.ValueOf(w)); err != nil {
		return 0, err
	}
	return h.Sum64(), nil
}

func canonicalOperationalEffectsDigestCloneContext(ctx context.Context, e *signature.OperationalEffects) (*signature.OperationalEffects, error) {
	if e == nil {
		return nil, nil
	}
	out := *e
	cloneTypes := func() error { return operationalEffectsContextErr(ctx) }
	if err := cloneTypes(); err != nil {
		return nil, err
	}
	out.NormalReturnTypeRefinements = append([]signature.PathTypeRefinement(nil), e.NormalReturnTypeRefinements...)
	for i := range out.NormalReturnTypeRefinements {
		if i%64 == 0 {
			if err := cloneTypes(); err != nil {
				return nil, err
			}
		}
		value, err := canonicalOperationalEffectDigestTypeContext(ctx, out.NormalReturnTypeRefinements[i].Type)
		if err != nil {
			return nil, err
		}
		out.NormalReturnTypeRefinements[i].Type = value
	}
	out.PathPresenceImplications = append([]signature.PathPresenceImplication(nil), e.PathPresenceImplications...)
	for i := range out.PathPresenceImplications {
		if i%64 == 0 {
			if err := cloneTypes(); err != nil {
				return nil, err
			}
		}
		value, err := canonicalOperationalEffectDigestTypeContext(ctx, out.PathPresenceImplications[i].TriggerType)
		if err != nil {
			return nil, err
		}
		out.PathPresenceImplications[i].TriggerType = value
	}
	out.PathStaticMembers = append([]signature.PathStaticMemberFact(nil), e.PathStaticMembers...)
	for i := range out.PathStaticMembers {
		if i%64 == 0 {
			if err := cloneTypes(); err != nil {
				return nil, err
			}
		}
		value, err := canonicalOperationalEffectDigestTypeContext(ctx, out.PathStaticMembers[i].Type)
		if err != nil {
			return nil, err
		}
		out.PathStaticMembers[i].Type = value
	}
	out.PathStaticMemberDeltas = append([]signature.PathStaticMemberDelta(nil), e.PathStaticMemberDeltas...)
	for i := range out.PathStaticMemberDeltas {
		if i%64 == 0 {
			if err := cloneTypes(); err != nil {
				return nil, err
			}
		}
		value, err := canonicalOperationalEffectDigestTypeContext(ctx, out.PathStaticMemberDeltas[i].Type)
		if err != nil {
			return nil, err
		}
		out.PathStaticMemberDeltas[i].Type = value
	}
	out.DynamicIndexFacts = append([]signature.DynamicIndexFact(nil), e.DynamicIndexFacts...)
	for i := range out.DynamicIndexFacts {
		if i%64 == 0 {
			if err := cloneTypes(); err != nil {
				return nil, err
			}
		}
		keyType, err := canonicalOperationalEffectDigestTypeContext(ctx, out.DynamicIndexFacts[i].Key.Type)
		if err != nil {
			return nil, err
		}
		valueType, err := canonicalOperationalEffectDigestTypeContext(ctx, out.DynamicIndexFacts[i].Value.Type)
		if err != nil {
			return nil, err
		}
		out.DynamicIndexFacts[i].Key.Type = keyType
		out.DynamicIndexFacts[i].Value.Type = valueType
	}
	out.ReturnAllocationTemplates = append([]signature.ReturnAllocationTemplate(nil), e.ReturnAllocationTemplates...)
	for i := range out.ReturnAllocationTemplates {
		if i%64 == 0 {
			if err := cloneTypes(); err != nil {
				return nil, err
			}
		}
		out.ReturnAllocationTemplates[i].Objects = append([]signature.AllocationObjectTemplate(nil), e.ReturnAllocationTemplates[i].Objects...)
		for j := range out.ReturnAllocationTemplates[i].Objects {
			if j%64 == 0 {
				if err := cloneTypes(); err != nil {
					return nil, err
				}
			}
			object := &out.ReturnAllocationTemplates[i].Objects[j]
			value, err := canonicalOperationalEffectDigestTypeContext(ctx, object.Type)
			if err != nil {
				return nil, err
			}
			object.Type = value
			object.DynamicEntries = append([]signature.AllocationDynamicEntryTemplate(nil), e.ReturnAllocationTemplates[i].Objects[j].DynamicEntries...)
			for k := range object.DynamicEntries {
				if k%64 == 0 {
					if err := cloneTypes(); err != nil {
						return nil, err
					}
				}
				value, err := canonicalOperationalEffectDigestTypeContext(ctx, object.DynamicEntries[k].KeyType)
				if err != nil {
					return nil, err
				}
				object.DynamicEntries[k].KeyType = value
			}
		}
	}
	return &out, nil
}

func operationalEffectsContextErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

type operationalEffectsDigestWriter struct {
	ctx   context.Context
	h     *internalhash.Writer
	steps uint64
}

func (w *operationalEffectsDigestWriter) write(v reflect.Value) error {
	if w == nil || w.h == nil {
		return nil
	}
	w.steps++
	if w.steps%64 == 0 {
		if err := operationalEffectsContextErr(w.ctx); err != nil {
			return err
		}
	}
	if !v.IsValid() {
		return w.byte('0')
	}
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return w.byte('p')
		}
		if err := w.byte('P'); err != nil {
			return err
		}
		return w.write(v.Elem())
	case reflect.Struct:
		if err := w.byte('{'); err != nil {
			return err
		}
		for i := 0; i < v.NumField(); i++ {
			if err := w.write(v.Field(i)); err != nil {
				return err
			}
		}
		return w.byte('}')
	case reflect.Slice:
		if err := w.byte('['); err != nil {
			return err
		}
		if err := w.uint64(uint64(v.Len())); err != nil {
			return err
		}
		for i := 0; i < v.Len(); i++ {
			if err := w.write(v.Index(i)); err != nil {
				return err
			}
		}
		return w.byte(']')
	case reflect.String:
		if err := w.byte('s'); err != nil {
			return err
		}
		if err := w.uint64(uint64(v.Len())); err != nil {
			return err
		}
		_, _ = w.h.WriteString(v.String())
		return nil
	case reflect.Bool:
		if err := w.byte('b'); err != nil {
			return err
		}
		if v.Bool() {
			return w.byte(1)
		}
		return w.byte(0)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if err := w.byte('i'); err != nil {
			return err
		}
		return w.uint64(uint64(v.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if err := w.byte('u'); err != nil {
			return err
		}
		return w.uint64(v.Uint())
	case reflect.Float64:
		if err := w.byte('f'); err != nil {
			return err
		}
		return w.uint64(math.Float64bits(v.Float()))
	default:
		return fmt.Errorf("manifest: unsupported operational-effects digest wire kind %s", v.Kind())
	}
}

func (w *operationalEffectsDigestWriter) byte(value byte) error { return w.h.WriteByte(value) }

func (w *operationalEffectsDigestWriter) uint64(value uint64) error {
	for shift := uint(0); shift < 64; shift += 8 {
		if err := w.byte(byte(value >> shift)); err != nil {
			return err
		}
	}
	return nil
}

func canonicalizeOperationalEffectDigestTypes(e *signature.OperationalEffects) {
	if e == nil {
		return
	}
	for i := range e.NormalReturnTypeRefinements {
		e.NormalReturnTypeRefinements[i].Type = canonicalOperationalEffectDigestType(e.NormalReturnTypeRefinements[i].Type)
	}
	for i := range e.PathPresenceImplications {
		e.PathPresenceImplications[i].TriggerType = canonicalOperationalEffectDigestType(e.PathPresenceImplications[i].TriggerType)
	}
	for i := range e.PathStaticMembers {
		e.PathStaticMembers[i].Type = canonicalOperationalEffectDigestType(e.PathStaticMembers[i].Type)
	}
	for i := range e.PathStaticMemberDeltas {
		e.PathStaticMemberDeltas[i].Type = canonicalOperationalEffectDigestType(e.PathStaticMemberDeltas[i].Type)
	}
	for i := range e.DynamicIndexFacts {
		e.DynamicIndexFacts[i].Key.Type = canonicalOperationalEffectDigestType(e.DynamicIndexFacts[i].Key.Type)
		e.DynamicIndexFacts[i].Value.Type = canonicalOperationalEffectDigestType(e.DynamicIndexFacts[i].Value.Type)
	}
	for i := range e.ReturnAllocationTemplates {
		for j := range e.ReturnAllocationTemplates[i].Objects {
			object := &e.ReturnAllocationTemplates[i].Objects[j]
			object.Type = canonicalOperationalEffectDigestType(object.Type)
			for k := range object.DynamicEntries {
				object.DynamicEntries[k].KeyType = canonicalOperationalEffectDigestType(object.DynamicEntries[k].KeyType)
			}
		}
	}
}

func canonicalOperationalEffectDigestType(t typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	return typ.NewRef("manifest-digest-type", strconv.FormatUint(typ.EqualityHash(t), 10)+":"+t.String())
}

func canonicalOperationalEffectDigestTypeContext(ctx context.Context, t typ.Type) (typ.Type, error) {
	if t == nil {
		return nil, nil
	}
	h, err := typ.EqualityHashContext(ctx, t)
	if err != nil {
		return nil, err
	}
	return typ.NewRef("manifest-digest-type", strconv.FormatUint(h, 10)+":"+t.String()), nil
}

func decodeOperationalEffects(w *operationalEffectsWire) (signature.OperationalEffects, error) {
	if w == nil {
		return signature.OperationalEffects{}, nil
	}
	var out signature.OperationalEffects
	for _, lane := range operationalEffectsWireLanes {
		if err := lane.decode(w, &out); err != nil {
			return signature.OperationalEffects{}, err
		}
	}
	return out, nil
}

func canonicalizeOperationalEffectsWire(w *operationalEffectsWire) {
	if w == nil {
		return
	}
	for _, lane := range operationalEffectsWireLanes {
		_ = lane.canonicalize(context.Background(), w)
	}
}

func canonicalizeOperationalEffectsWireContext(ctx context.Context, w *operationalEffectsWire) error {
	if w == nil {
		return nil
	}
	for _, lane := range operationalEffectsWireLanes {
		if err := lane.canonicalize(ctx, w); err != nil {
			return err
		}
	}
	return nil
}

func encodePathPresenceImplication(fact signature.PathPresenceImplication) (pathPresenceImplicationWire, error) {
	return encodePathPresenceImplicationContext(context.Background(), fact)
}

func encodePathPresenceImplicationContext(ctx context.Context, fact signature.PathPresenceImplication) (pathPresenceImplicationWire, error) {
	trigger, err := encodeBoundaryPath(fact.Trigger)
	if err != nil {
		return pathPresenceImplicationWire{}, fmt.Errorf("trigger: %w", err)
	}
	triggerPresence, err := encodePresence(fact.TriggerPresence)
	if err != nil {
		return pathPresenceImplicationWire{}, fmt.Errorf("trigger presence: %w", err)
	}
	target, err := encodeBoundaryPath(fact.Target)
	if err != nil {
		return pathPresenceImplicationWire{}, fmt.Errorf("target: %w", err)
	}
	targetPresence, err := encodePresence(fact.TargetPresence)
	if err != nil {
		return pathPresenceImplicationWire{}, fmt.Errorf("target presence: %w", err)
	}
	out := pathPresenceImplicationWire{
		Trigger:         trigger,
		TriggerPresence: triggerPresence,
		Target:          target,
		TargetPresence:  targetPresence,
	}
	if fact.HasTriggerType {
		if fact.TriggerType == nil {
			return pathPresenceImplicationWire{}, fmt.Errorf("trigger type: missing")
		}
		triggerType, err := encodeOperationalEffectType(ctx, fact.TriggerType)
		if err != nil {
			return pathPresenceImplicationWire{}, fmt.Errorf("trigger type: %w", err)
		}
		out.TriggerType = triggerType
	}
	return out, nil
}

func decodePathPresenceImplication(w pathPresenceImplicationWire) (signature.PathPresenceImplication, error) {
	trigger, err := decodeBoundaryPath(w.Trigger)
	if err != nil {
		return signature.PathPresenceImplication{}, fmt.Errorf("trigger: %w", err)
	}
	triggerPresence, err := decodePresence(w.TriggerPresence)
	if err != nil {
		return signature.PathPresenceImplication{}, fmt.Errorf("trigger presence: %w", err)
	}
	target, err := decodeBoundaryPath(w.Target)
	if err != nil {
		return signature.PathPresenceImplication{}, fmt.Errorf("target: %w", err)
	}
	targetPresence, err := decodePresence(w.TargetPresence)
	if err != nil {
		return signature.PathPresenceImplication{}, fmt.Errorf("target presence: %w", err)
	}
	out := signature.PathPresenceImplication{
		Trigger:         trigger,
		TriggerPresence: triggerPresence,
		Target:          target,
		TargetPresence:  targetPresence,
	}
	if w.TriggerType != nil {
		triggerType, err := decodeType(w.TriggerType)
		if err != nil {
			return signature.PathPresenceImplication{}, fmt.Errorf("trigger type: %w", err)
		}
		if triggerType == nil {
			return signature.PathPresenceImplication{}, fmt.Errorf("trigger type: missing")
		}
		out.TriggerType = triggerType
		out.HasTriggerType = true
	}
	return out, nil
}

func comparePathPresenceImplicationWire(a, b pathPresenceImplicationWire) int {
	if c := compareBoundaryPathWire(a.Trigger, b.Trigger); c != 0 {
		return c
	}
	if a.TriggerPresence != b.TriggerPresence {
		return strings.Compare(a.TriggerPresence, b.TriggerPresence)
	}
	if leftKey, rightKey := typeWireKey(a.TriggerType), typeWireKey(b.TriggerType); leftKey != rightKey {
		return strings.Compare(leftKey, rightKey)
	}
	if c := compareBoundaryPathWire(a.Target, b.Target); c != 0 {
		return c
	}
	return strings.Compare(a.TargetPresence, b.TargetPresence)
}

func encodeBranchProof(proof signature.BranchProof) (branchProofWire, error) {
	kind, err := encodeBranchProofKind(proof.Kind)
	if err != nil {
		return branchProofWire{}, err
	}
	p, err := encodeBoundaryPath(proof.Path)
	if err != nil {
		return branchProofWire{}, fmt.Errorf("path: %w", err)
	}
	out := branchProofWire{Kind: kind, Path: p}
	switch proof.Kind {
	case signature.BranchProofPathPresence:
		pres, err := encodePresence(proof.Presence)
		if err != nil {
			return branchProofWire{}, fmt.Errorf("presence: %w", err)
		}
		out.Presence = pres
	case signature.BranchProofPathEqual, signature.BranchProofPathNotEqual, signature.BranchProofIndexInRange:
		other, err := encodeBoundaryPath(proof.Other)
		if err != nil {
			return branchProofWire{}, fmt.Errorf("other: %w", err)
		}
		out.Other = other
	default:
		return branchProofWire{}, fmt.Errorf("unsupported branch proof kind %d", proof.Kind)
	}
	return out, nil
}

func decodeBranchProof(w branchProofWire) (signature.BranchProof, error) {
	kind, err := decodeBranchProofKind(w.Kind)
	if err != nil {
		return signature.BranchProof{}, err
	}
	p, err := decodeBoundaryPath(w.Path)
	if err != nil {
		return signature.BranchProof{}, fmt.Errorf("path: %w", err)
	}
	out := signature.BranchProof{Kind: kind, Path: p}
	switch kind {
	case signature.BranchProofPathPresence:
		pres, err := decodePresence(w.Presence)
		if err != nil {
			return signature.BranchProof{}, fmt.Errorf("presence: %w", err)
		}
		out.Presence = pres
	case signature.BranchProofPathEqual, signature.BranchProofPathNotEqual, signature.BranchProofIndexInRange:
		other, err := decodeBoundaryPath(w.Other)
		if err != nil {
			return signature.BranchProof{}, fmt.Errorf("other: %w", err)
		}
		out.Other = other
	default:
		return signature.BranchProof{}, fmt.Errorf("unsupported branch proof kind %d", kind)
	}
	return out, nil
}

func compareBranchProofWire(a, b branchProofWire) int {
	if a.Kind != b.Kind {
		return strings.Compare(a.Kind, b.Kind)
	}
	if c := compareBoundaryPathWire(a.Path, b.Path); c != 0 {
		return c
	}
	if c := compareBoundaryPathWire(a.Other, b.Other); c != 0 {
		return c
	}
	return strings.Compare(a.Presence, b.Presence)
}

func encodeDynamicIndexFact(fact signature.DynamicIndexFact) (dynamicIndexFactWire, error) {
	return encodeDynamicIndexFactContext(context.Background(), fact)
}

func encodeDynamicIndexFactContext(ctx context.Context, fact signature.DynamicIndexFact) (dynamicIndexFactWire, error) {
	if fact.Site == "" {
		return dynamicIndexFactWire{}, fmt.Errorf("missing site")
	}
	table, err := encodeBoundaryPath(fact.Table)
	if err != nil {
		return dynamicIndexFactWire{}, fmt.Errorf("table: %w", err)
	}
	keyPresence, err := encodePresence(fact.KeyPresence)
	if err != nil {
		return dynamicIndexFactWire{}, fmt.Errorf("key presence: %w", err)
	}
	key, err := encodeDynamicIndexOperandContext(ctx, fact.Key)
	if err != nil {
		return dynamicIndexFactWire{}, fmt.Errorf("key: %w", err)
	}
	value, err := encodeDynamicIndexOperandContext(ctx, fact.Value)
	if err != nil {
		return dynamicIndexFactWire{}, fmt.Errorf("value: %w", err)
	}
	admission, err := encodeDynamicIndexAdmission(fact.Admission)
	if err != nil {
		return dynamicIndexFactWire{}, fmt.Errorf("admission: %w", err)
	}
	return dynamicIndexFactWire{
		Table:       table,
		Site:        fact.Site,
		KeyPresence: keyPresence,
		Key:         key,
		Value:       value,
		Admission:   admission,
	}, nil
}

func encodeDynamicIndexOperand(operand signature.DynamicIndexOperand) (dynamicIndexOperandWire, error) {
	return encodeDynamicIndexOperandContext(context.Background(), operand)
}

func encodeDynamicIndexOperandContext(ctx context.Context, operand signature.DynamicIndexOperand) (dynamicIndexOperandWire, error) {
	var out dynamicIndexOperandWire
	if !operand.Path.IsEmpty() {
		path, err := encodePlaceholderPath(operand.Path)
		if err != nil {
			return dynamicIndexOperandWire{}, fmt.Errorf("path: %w", err)
		}
		out.Path = path
	}
	if operand.Type != nil {
		typ, err := encodeOperationalEffectType(ctx, operand.Type)
		if err != nil {
			return dynamicIndexOperandWire{}, fmt.Errorf("type: %w", err)
		}
		out.Type = typ
	}
	if out.Path == nil && out.Type == nil {
		return dynamicIndexOperandWire{}, fmt.Errorf("missing path or type")
	}
	return out, nil
}

func decodeDynamicIndexFact(w dynamicIndexFactWire) (signature.DynamicIndexFact, error) {
	if w.Site == "" {
		return signature.DynamicIndexFact{}, fmt.Errorf("missing site")
	}
	table, err := decodeBoundaryPath(w.Table)
	if err != nil {
		return signature.DynamicIndexFact{}, fmt.Errorf("table: %w", err)
	}
	keyPresence, err := decodePresence(w.KeyPresence)
	if err != nil {
		return signature.DynamicIndexFact{}, fmt.Errorf("key presence: %w", err)
	}
	key, err := decodeDynamicIndexOperand(w.Key)
	if err != nil {
		return signature.DynamicIndexFact{}, fmt.Errorf("key: %w", err)
	}
	value, err := decodeDynamicIndexOperand(w.Value)
	if err != nil {
		return signature.DynamicIndexFact{}, fmt.Errorf("value: %w", err)
	}
	admission, err := decodeDynamicIndexAdmission(w.Admission)
	if err != nil {
		return signature.DynamicIndexFact{}, fmt.Errorf("admission: %w", err)
	}
	return signature.DynamicIndexFact{
		Table:       table,
		Site:        w.Site,
		KeyPresence: keyPresence,
		Key:         key,
		Value:       value,
		Admission:   admission,
	}, nil
}

func decodeDynamicIndexOperand(w dynamicIndexOperandWire) (signature.DynamicIndexOperand, error) {
	var out signature.DynamicIndexOperand
	if w.Path != nil {
		path, err := decodePlaceholderPath(w.Path)
		if err != nil {
			return signature.DynamicIndexOperand{}, fmt.Errorf("path: %w", err)
		}
		out.Path = path
	}
	if w.Type != nil {
		typ, err := decodeType(w.Type)
		if err != nil {
			return signature.DynamicIndexOperand{}, fmt.Errorf("type: %w", err)
		}
		out.Type = typ
	}
	if out.Path.IsEmpty() && out.Type == nil {
		return signature.DynamicIndexOperand{}, fmt.Errorf("missing path or type")
	}
	return out, nil
}

func compareDynamicIndexOperandWire(a, b dynamicIndexOperandWire) int {
	if c := comparePlaceholderPathWire(a.Path, b.Path); c != 0 {
		return c
	}
	left, right := typeWireKey(a.Type), typeWireKey(b.Type)
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func encodeKeyMembership(fact signature.KeyMembership) (keyMembershipWire, error) {
	key, err := encodeBoundaryPath(fact.Key)
	if err != nil {
		return keyMembershipWire{}, fmt.Errorf("key: %w", err)
	}
	table, err := encodeBoundaryPath(fact.Table)
	if err != nil {
		return keyMembershipWire{}, fmt.Errorf("table: %w", err)
	}
	return keyMembershipWire{Key: key, Table: table}, nil
}

func decodeKeyMembership(w keyMembershipWire) (signature.KeyMembership, error) {
	key, err := decodeBoundaryPath(w.Key)
	if err != nil {
		return signature.KeyMembership{}, fmt.Errorf("key: %w", err)
	}
	table, err := decodeBoundaryPath(w.Table)
	if err != nil {
		return signature.KeyMembership{}, fmt.Errorf("table: %w", err)
	}
	return signature.KeyMembership{Key: key, Table: table}, nil
}

func encodeDynamicValueKeyMembership(fact signature.DynamicValueKeyMembership) (dynamicValueKeyMembershipWire, error) {
	if fact.Site == "" {
		return dynamicValueKeyMembershipWire{}, fmt.Errorf("missing site")
	}
	container, err := encodeBoundaryPath(fact.Container)
	if err != nil {
		return dynamicValueKeyMembershipWire{}, fmt.Errorf("container: %w", err)
	}
	table, err := encodeBoundaryPath(fact.Table)
	if err != nil {
		return dynamicValueKeyMembershipWire{}, fmt.Errorf("table: %w", err)
	}
	return dynamicValueKeyMembershipWire{
		Container: container,
		Site:      fact.Site,
		Table:     table,
	}, nil
}

func decodeDynamicValueKeyMembership(w dynamicValueKeyMembershipWire) (signature.DynamicValueKeyMembership, error) {
	if w.Site == "" {
		return signature.DynamicValueKeyMembership{}, fmt.Errorf("missing site")
	}
	container, err := decodeBoundaryPath(w.Container)
	if err != nil {
		return signature.DynamicValueKeyMembership{}, fmt.Errorf("container: %w", err)
	}
	table, err := decodeBoundaryPath(w.Table)
	if err != nil {
		return signature.DynamicValueKeyMembership{}, fmt.Errorf("table: %w", err)
	}
	return signature.DynamicValueKeyMembership{
		Container: container,
		Site:      w.Site,
		Table:     table,
	}, nil
}

func encodeLifecycleEffect(effect signature.LifecycleEffect) (lifecycleEffectWire, error) {
	// Lifecycle effects may describe a resource passed into a call or one
	// created in a result slot.  Use the shared boundary-path codec rather than
	// the older parameter-only codec so an acquisition such as connect() ->
	// ret[0]:open survives manifest transport.
	target, err := encodeBoundaryPath(effect.Target)
	if err != nil {
		return lifecycleEffectWire{}, fmt.Errorf("target: %w", err)
	}
	kind, err := encodeLifecycleKind(effect.Kind)
	if err != nil {
		return lifecycleEffectWire{}, err
	}
	protocol, err := encodeLifecycleProtocol(effect.Protocol, "missing protocol")
	if err != nil {
		return lifecycleEffectWire{}, err
	}
	var to string
	if effect.Kind == signature.LifecycleAcquire {
		to, err = encodeRequiredLifecycleState(effect.To, "acquire missing state")
		if err != nil {
			return lifecycleEffectWire{}, err
		}
	}
	if effect.Kind == signature.LifecycleTransition {
		if _, err := encodeRequiredLifecycleState(effect.From, "transition missing source state"); err != nil {
			return lifecycleEffectWire{}, err
		}
		to, err = encodeRequiredLifecycleState(effect.To, "transition missing target state")
		if err != nil {
			return lifecycleEffectWire{}, err
		}
	}
	if to == "" {
		to = encodeOptionalLifecycleState(effect.To)
	}
	return lifecycleEffectWire{
		Target:   target,
		Kind:     kind,
		Protocol: protocol,
		From:     encodeOptionalLifecycleState(effect.From),
		To:       to,
		Final:    encodeOptionalLifecycleState(effect.Obligation.Final),
		Finals:   encodeOptionalLifecycleFinalStates(effect.Obligation.Finals),
	}, nil
}

func decodeLifecycleEffect(w lifecycleEffectWire) (signature.LifecycleEffect, error) {
	target, err := decodeBoundaryPath(w.Target)
	if err != nil {
		return signature.LifecycleEffect{}, fmt.Errorf("target: %w", err)
	}
	kind, err := decodeLifecycleKind(w.Kind)
	if err != nil {
		return signature.LifecycleEffect{}, err
	}
	protocol, err := decodeLifecycleProtocol(w.Protocol, "missing protocol")
	if err != nil {
		return signature.LifecycleEffect{}, err
	}
	var to typestate.State
	if kind == signature.LifecycleAcquire {
		to, err = decodeRequiredLifecycleState(w.To, "acquire missing state")
		if err != nil {
			return signature.LifecycleEffect{}, err
		}
	}
	if kind == signature.LifecycleTransition {
		if _, err = decodeRequiredLifecycleState(w.From, "transition missing source state"); err != nil {
			return signature.LifecycleEffect{}, err
		}
		to, err = decodeRequiredLifecycleState(w.To, "transition missing target state")
		if err != nil {
			return signature.LifecycleEffect{}, err
		}
	}
	if to == "" {
		to = decodeOptionalLifecycleState(w.To)
	}
	finals, err := decodeOptionalLifecycleFinalStates(w.Finals)
	if err != nil {
		return signature.LifecycleEffect{}, err
	}
	return signature.LifecycleEffect{
		Target:   target,
		Kind:     kind,
		Protocol: protocol,
		From:     decodeOptionalLifecycleState(w.From),
		To:       to,
		Obligation: typestate.Obligation{
			Final:  decodeOptionalLifecycleState(w.Final),
			Finals: finals,
		},
	}, nil
}

func compareLifecycleEffectWire(a, b lifecycleEffectWire) int {
	if c := compareBoundaryPathWire(a.Target, b.Target); c != 0 {
		return c
	}
	pairs := [][2]string{
		{a.Protocol, b.Protocol},
		{a.Kind, b.Kind},
		{a.From, b.From},
		{a.To, b.To},
		{a.Final, b.Final},
		{strings.Join(a.Finals, "\x00"), strings.Join(b.Finals, "\x00")},
	}
	for _, pair := range pairs {
		switch {
		case pair[0] < pair[1]:
			return -1
		case pair[0] > pair[1]:
			return 1
		}
	}
	return 0
}

func encodeTypestateRequirement(requirement signature.TypestateRequirement) (typestateRequirementWire, error) {
	target, err := encodePlaceholderPath(requirement.Target)
	if err != nil {
		return typestateRequirementWire{}, fmt.Errorf("target: %w", err)
	}
	return typestateRequirementWire{Target: target, Protocol: string(requirement.Protocol), State: string(requirement.State)}, nil
}

func decodeTypestateRequirement(w typestateRequirementWire) (signature.TypestateRequirement, error) {
	target, err := decodePlaceholderPath(w.Target)
	if err != nil {
		return signature.TypestateRequirement{}, fmt.Errorf("target: %w", err)
	}
	if w.Protocol == "" {
		return signature.TypestateRequirement{}, errors.New("missing protocol")
	}
	if w.State == "" {
		return signature.TypestateRequirement{}, errors.New("missing state")
	}
	return signature.TypestateRequirement{Target: target, Protocol: typestate.Protocol(w.Protocol), State: typestate.State(w.State)}, nil
}

func compareTypestateRequirementWire(a, b typestateRequirementWire) int {
	if c := comparePlaceholderPathWire(a.Target, b.Target); c != 0 {
		return c
	}
	if a.Protocol != b.Protocol {
		return strings.Compare(a.Protocol, b.Protocol)
	}
	return strings.Compare(a.State, b.State)
}

func encodeReturnAllocationTemplate(template signature.ReturnAllocationTemplate) (returnAllocationTemplateWire, error) {
	return encodeReturnAllocationTemplateContext(context.Background(), template)
}

func encodeReturnAllocationTemplateContext(ctx context.Context, template signature.ReturnAllocationTemplate) (returnAllocationTemplateWire, error) {
	if template.ReturnIndex < 0 {
		return returnAllocationTemplateWire{}, fmt.Errorf("negative return index %d", template.ReturnIndex)
	}
	if template.Root == "" {
		return returnAllocationTemplateWire{}, fmt.Errorf("missing root")
	}
	if err := validateReturnAllocationTemplate(template); err != nil {
		return returnAllocationTemplateWire{}, err
	}
	out := returnAllocationTemplateWire{
		ReturnIndex: encodeInt(template.ReturnIndex),
		Root:        string(template.Root),
	}
	for _, object := range template.Objects {
		encoded, err := encodeAllocationObjectTemplateContext(ctx, object)
		if err != nil {
			return returnAllocationTemplateWire{}, err
		}
		out.Objects = append(out.Objects, encoded)
	}
	return out, nil
}

func encodeAllocationObjectTemplate(object signature.AllocationObjectTemplate) (allocationObjectWire, error) {
	return encodeAllocationObjectTemplateContext(context.Background(), object)
}

func encodeAllocationObjectTemplateContext(ctx context.Context, object signature.AllocationObjectTemplate) (allocationObjectWire, error) {
	if object.ID == "" {
		return allocationObjectWire{}, fmt.Errorf("missing object id")
	}
	out := allocationObjectWire{ID: string(object.ID), StableShape: object.StableShape, PrefixStable: object.PrefixStable}
	if object.Type != nil {
		encoded, err := encodeOperationalEffectType(ctx, object.Type)
		if err != nil {
			return allocationObjectWire{}, fmt.Errorf("object %s type: %w", object.ID, err)
		}
		out.Type = encoded
	}
	for _, member := range object.StaticMembers {
		if member.Value == "" {
			return allocationObjectWire{}, fmt.Errorf("static member %s missing value", segment.FormatSegments(member.Suffix))
		}
		out.StaticMembers = append(out.StaticMembers, allocationStaticMemberWire{
			Suffix: segment.FormatSegments(member.Suffix),
			Value:  string(member.Value),
		})
	}
	for _, entry := range object.DynamicEntries {
		if entry.Key == "" && entry.KeyType == nil && entry.Value == "" {
			return allocationObjectWire{}, fmt.Errorf("object %q dynamic entry missing key, key type, or value", object.ID)
		}
		var keyType *typeWire
		if entry.KeyType != nil {
			encoded, err := encodeOperationalEffectType(ctx, entry.KeyType)
			if err != nil {
				return allocationObjectWire{}, fmt.Errorf("dynamic entry key type: %w", err)
			}
			keyType = encoded
		}
		out.DynamicEntries = append(out.DynamicEntries, allocationDynamicEntryWire{
			Key:     string(entry.Key),
			KeyType: keyType,
			Value:   string(entry.Value),
		})
	}
	return out, nil
}

func decodeReturnAllocationTemplate(w returnAllocationTemplateWire) (signature.ReturnAllocationTemplate, error) {
	returnIndex, err := decodeRequiredInt(w.ReturnIndex, "return allocation template return index missing")
	if err != nil {
		return signature.ReturnAllocationTemplate{}, err
	}
	if returnIndex < 0 {
		return signature.ReturnAllocationTemplate{}, fmt.Errorf("negative return index %d", returnIndex)
	}
	if w.Root == "" {
		return signature.ReturnAllocationTemplate{}, fmt.Errorf("missing root")
	}
	out := signature.ReturnAllocationTemplate{
		ReturnIndex: returnIndex,
		Root:        signature.AllocationTemplateID(w.Root),
	}
	for _, object := range w.Objects {
		decoded, err := decodeAllocationObjectTemplate(object)
		if err != nil {
			return signature.ReturnAllocationTemplate{}, err
		}
		out.Objects = append(out.Objects, decoded)
	}
	if err := validateReturnAllocationTemplate(out); err != nil {
		return signature.ReturnAllocationTemplate{}, err
	}
	return out, nil
}

func validateReturnAllocationTemplate(template signature.ReturnAllocationTemplate) error {
	if template.ReturnIndex < 0 {
		return fmt.Errorf("negative return index %d", template.ReturnIndex)
	}
	if template.Root == "" {
		return fmt.Errorf("missing root")
	}
	objects := make(map[signature.AllocationTemplateID]struct{}, len(template.Objects))
	for _, object := range template.Objects {
		if object.ID == "" {
			return fmt.Errorf("missing object id")
		}
		if _, ok := objects[object.ID]; ok {
			return fmt.Errorf("duplicate object id %q", object.ID)
		}
		objects[object.ID] = struct{}{}
	}
	if _, ok := objects[template.Root]; !ok {
		return fmt.Errorf("root %q has no object template", template.Root)
	}
	for _, object := range template.Objects {
		for _, member := range object.StaticMembers {
			if member.Value == "" {
				return fmt.Errorf("object %q static member %s missing value",
					object.ID, segment.FormatSegments(member.Suffix))
			}
			if _, ok := objects[member.Value]; !ok {
				return fmt.Errorf("object %q static member %s references missing object %q",
					object.ID, segment.FormatSegments(member.Suffix), member.Value)
			}
		}
		for _, entry := range object.DynamicEntries {
			if entry.Key == "" && entry.KeyType == nil && entry.Value == "" {
				return fmt.Errorf("object %q dynamic entry missing key, key type, or value", object.ID)
			}
			if entry.Key != "" {
				if _, ok := objects[entry.Key]; !ok {
					return fmt.Errorf("object %q dynamic entry references missing key object %q", object.ID, entry.Key)
				}
			}
			if entry.Value != "" {
				if _, ok := objects[entry.Value]; !ok {
					return fmt.Errorf("object %q dynamic entry references missing value object %q", object.ID, entry.Value)
				}
			}
		}
	}
	return nil
}

func decodeAllocationObjectTemplate(w allocationObjectWire) (signature.AllocationObjectTemplate, error) {
	if w.ID == "" {
		return signature.AllocationObjectTemplate{}, fmt.Errorf("missing object id")
	}
	out := signature.AllocationObjectTemplate{ID: signature.AllocationTemplateID(w.ID), StableShape: w.StableShape, PrefixStable: w.PrefixStable}
	if w.Type != nil {
		t, err := decodeType(w.Type)
		if err != nil {
			return signature.AllocationObjectTemplate{}, fmt.Errorf("object %s type: %w", w.ID, err)
		}
		out.Type = t
	}
	for _, member := range w.StaticMembers {
		if member.Value == "" {
			return signature.AllocationObjectTemplate{}, fmt.Errorf("static member %q missing value", member.Suffix)
		}
		segs, ok := segment.ParseFormattedSegments(member.Suffix)
		if !ok {
			return signature.AllocationObjectTemplate{}, fmt.Errorf("invalid static member suffix %q", member.Suffix)
		}
		out.StaticMembers = append(out.StaticMembers, signature.AllocationStaticMemberTemplate{
			Suffix: segs,
			Value:  signature.AllocationTemplateID(member.Value),
		})
	}
	for _, entry := range w.DynamicEntries {
		var keyType typ.Type
		if entry.KeyType != nil {
			t, err := decodeType(entry.KeyType)
			if err != nil {
				return signature.AllocationObjectTemplate{}, fmt.Errorf("dynamic entry key type: %w", err)
			}
			keyType = t
		}
		if entry.Key == "" && keyType == nil && entry.Value == "" {
			return signature.AllocationObjectTemplate{}, fmt.Errorf("object %q dynamic entry missing key, key type, or value", w.ID)
		}
		out.DynamicEntries = append(out.DynamicEntries, signature.AllocationDynamicEntryTemplate{
			Key:     signature.AllocationTemplateID(entry.Key),
			KeyType: keyType,
			Value:   signature.AllocationTemplateID(entry.Value),
		})
	}
	return out, nil
}

func canonicalizeReturnAllocationTemplateWire(ctx context.Context, w *returnAllocationTemplateWire) error {
	if w == nil {
		return nil
	}
	for i := range w.Objects {
		if i%64 == 0 {
			if err := operationalEffectsContextErr(ctx); err != nil {
				return err
			}
		}
		if err := canonicalizeAllocationObjectWire(ctx, &w.Objects[i]); err != nil {
			return err
		}
	}
	var canceled error
	compares := 0
	sort.Slice(w.Objects, func(i, j int) bool {
		compares++
		if compares%64 == 0 && canceled == nil {
			canceled = operationalEffectsContextErr(ctx)
		}
		if canceled != nil {
			return false
		}
		return w.Objects[i].ID < w.Objects[j].ID
	})
	return canceled
}

func canonicalizeAllocationObjectWire(ctx context.Context, w *allocationObjectWire) error {
	if w == nil {
		return nil
	}
	var canceled error
	compares := 0
	sort.Slice(w.StaticMembers, func(i, j int) bool {
		compares++
		if compares%64 == 0 && canceled == nil {
			canceled = operationalEffectsContextErr(ctx)
		}
		if canceled != nil {
			return false
		}
		left, right := w.StaticMembers[i], w.StaticMembers[j]
		if left.Suffix != right.Suffix {
			return left.Suffix < right.Suffix
		}
		return left.Value < right.Value
	})
	if canceled != nil {
		return canceled
	}
	compares = 0
	sort.Slice(w.DynamicEntries, func(i, j int) bool {
		compares++
		if compares%64 == 0 && canceled == nil {
			canceled = operationalEffectsContextErr(ctx)
		}
		if canceled != nil {
			return false
		}
		left, right := w.DynamicEntries[i], w.DynamicEntries[j]
		if left.Key != right.Key {
			return left.Key < right.Key
		}
		if left.Value != right.Value {
			return left.Value < right.Value
		}
		return typeWireKey(left.KeyType) < typeWireKey(right.KeyType)
	})
	return canceled
}

func typeWireKey(w *typeWire) string {
	if w == nil {
		return ""
	}
	data, err := json.Marshal(w)
	if err != nil {
		return ""
	}
	return string(data)
}

func comparePlaceholderPathWire(a, b *placeholderPathWire) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	default:
		if c := compareOptionalInt(a.Param, b.Param); c != 0 {
			return c
		}
	}
	switch {
	case a.Suffix < b.Suffix:
		return -1
	case a.Suffix > b.Suffix:
		return 1
	default:
		return 0
	}
}

func compareBoundaryPathWire(a, b *boundaryPathWire) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return -1
	case b == nil:
		return 1
	}
	if c := compareBoundaryPathRoot(a, b); c != 0 {
		return c
	}
	switch {
	case a.Suffix < b.Suffix:
		return -1
	case a.Suffix > b.Suffix:
		return 1
	default:
		return 0
	}
}

func compareBoundaryPathRoot(a, b *boundaryPathWire) int {
	if c := boundaryPathRootKind(a) - boundaryPathRootKind(b); c != 0 {
		return c
	}
	switch boundaryPathRootKind(a) {
	case 0:
		return compareOptionalInt(a.Param, b.Param)
	case 1:
		return compareOptionalInt(a.Return, b.Return)
	default:
		return 0
	}
}

func boundaryPathRootKind(w *boundaryPathWire) int {
	switch {
	case w != nil && w.Param != nil:
		return 0
	case w != nil && w.Return != nil:
		return 1
	default:
		return 2
	}
}

func encodePlaceholderPath(p pathdom.Path) (*placeholderPathWire, error) {
	if !p.IsPlaceholder() {
		return nil, fmt.Errorf("path %q is not a placeholder path", p.String())
	}
	return &placeholderPathWire{
		Param:  encodeInt(p.PlaceholderIndex()),
		Suffix: segment.FormatSegments(p.Segments),
	}, nil
}

func decodePlaceholderPath(w *placeholderPathWire) (pathdom.Path, error) {
	if w == nil {
		return pathdom.Path{}, fmt.Errorf("missing placeholder path")
	}
	param, err := decodeRequiredInt(w.Param, "placeholder path param missing")
	if err != nil {
		return pathdom.Path{}, err
	}
	if param < 0 {
		return pathdom.Path{}, fmt.Errorf("negative placeholder index %d", param)
	}
	segs, ok := segment.ParseFormattedSegments(w.Suffix)
	if !ok {
		return pathdom.Path{}, fmt.Errorf("invalid placeholder path suffix %q", w.Suffix)
	}
	p := pathdom.NewPlaceholder(param)
	p.Segments = segs
	return p, nil
}

func encodeBoundaryPath(p pathdom.Path) (*boundaryPathWire, error) {
	switch {
	case p.IsPlaceholder():
		if index := p.PlaceholderIndex(); index < 0 {
			return nil, fmt.Errorf("negative placeholder index %d", index)
		}
		return &boundaryPathWire{
			Param:  encodeInt(p.PlaceholderIndex()),
			Suffix: segment.FormatSegments(p.Segments),
		}, nil
	default:
		if index, ok := returnSlotPathIndex(p); ok {
			return &boundaryPathWire{
				Return: encodeInt(index),
				Suffix: segment.FormatSegments(p.Segments),
			}, nil
		}
		return nil, fmt.Errorf("path %q is not a boundary path", p.String())
	}
}

func decodeBoundaryPath(w *boundaryPathWire) (pathdom.Path, error) {
	if w == nil {
		return pathdom.Path{}, fmt.Errorf("missing boundary path")
	}
	if (w.Param == nil) == (w.Return == nil) {
		return pathdom.Path{}, fmt.Errorf("boundary path must set exactly one of param or return")
	}
	segs, ok := segment.ParseFormattedSegments(w.Suffix)
	if !ok {
		return pathdom.Path{}, fmt.Errorf("invalid boundary path suffix %q", w.Suffix)
	}
	if w.Param != nil {
		param, err := decodeRequiredInt(w.Param, "boundary path param missing")
		if err != nil {
			return pathdom.Path{}, err
		}
		if param < 0 {
			return pathdom.Path{}, fmt.Errorf("negative placeholder index %d", param)
		}
		p := pathdom.NewPlaceholder(param)
		p.Segments = segs
		return p, nil
	}
	index, err := decodeRequiredInt(w.Return, "boundary path return missing")
	if err != nil {
		return pathdom.Path{}, err
	}
	if index < 0 {
		return pathdom.Path{}, fmt.Errorf("negative return index %d", index)
	}
	return pathdom.Path{Root: returnSlotRoot(index), Segments: segs}, nil
}

func returnSlotPathIndex(p pathdom.Path) (int, bool) {
	if p.Symbol != 0 || !strings.HasPrefix(p.Root, "ret[") || !strings.HasSuffix(p.Root, "]") {
		return 0, false
	}
	body := strings.TrimSuffix(strings.TrimPrefix(p.Root, "ret["), "]")
	index, err := strconv.Atoi(body)
	if err != nil || index < 0 || p.Root != returnSlotRoot(index) {
		return 0, false
	}
	return index, true
}

func returnSlotRoot(index int) string {
	return "ret[" + strconv.Itoa(index) + "]"
}

func encodePresence(p presence.Value) (string, error) {
	switch {
	case presence.Equal(p, presence.Present()):
		return "present", nil
	case presence.Equal(p, presence.Absent()):
		return "absent", nil
	case presence.Equal(p, presence.Maybe()):
		return "maybe", nil
	default:
		return "", fmt.Errorf("unsupported presence %s", p.String())
	}
}

func decodePresence(s string) (presence.Value, error) {
	switch s {
	case "present":
		return presence.Present(), nil
	case "absent":
		return presence.Absent(), nil
	case "maybe":
		return presence.Maybe(), nil
	default:
		return presence.Bottom(), fmt.Errorf("unknown presence %q", s)
	}
}

func encodeAssertion(value assertion.Value) []string {
	flags := value.Flags()
	if len(flags) == 0 {
		return nil
	}
	out := make([]string, 0, len(flags))
	for _, flag := range flags {
		out = append(out, flag.String())
	}
	return out
}

func decodeAssertion(items []string) (assertion.Value, error) {
	if len(items) == 0 {
		return assertion.Top(), nil
	}
	flags := make([]assertion.Flag, 0, len(items))
	for _, item := range items {
		flag, ok := decodeAssertionFlag(item)
		if !ok {
			return assertion.Bottom(), fmt.Errorf("unknown assertion flag %q", item)
		}
		flags = append(flags, flag)
	}
	return assertion.Of(flags...), nil
}

func decodeAssertionFlag(s string) (assertion.Flag, bool) {
	switch s {
	case "type":
		return assertion.TypeClaim, true
	case "any":
		return assertion.AnyClaim, true
	case "non-nil":
		return assertion.NonNilClaim, true
	case "runtime":
		return assertion.RuntimeClaim, true
	default:
		return 0, false
	}
}

func encodeDynamicIndexAdmission(admission signature.DynamicIndexAdmission) (string, error) {
	switch admission {
	case signature.DynamicIndexAdmissionAdmitted:
		return "admitted", nil
	case signature.DynamicIndexAdmissionRejected:
		return "rejected", nil
	case signature.DynamicIndexAdmissionUnknown:
		return "unknown", nil
	default:
		return "", fmt.Errorf("unknown dynamic-index admission %q", admission)
	}
}

func decodeDynamicIndexAdmission(s string) (signature.DynamicIndexAdmission, error) {
	switch s {
	case "admitted":
		return signature.DynamicIndexAdmissionAdmitted, nil
	case "rejected":
		return signature.DynamicIndexAdmissionRejected, nil
	case "unknown":
		return signature.DynamicIndexAdmissionUnknown, nil
	default:
		return "", fmt.Errorf("unknown dynamic-index admission %q", s)
	}
}

func encodeBranchProofKind(kind signature.BranchProofKind) (string, error) {
	switch kind {
	case signature.BranchProofPathPresence:
		return "presence", nil
	case signature.BranchProofPathEqual:
		return "equal", nil
	case signature.BranchProofPathNotEqual:
		return "not_equal", nil
	case signature.BranchProofIndexInRange:
		return "index_in_range", nil
	default:
		return "", fmt.Errorf("unsupported branch proof kind %d", kind)
	}
}

func decodeBranchProofKind(s string) (signature.BranchProofKind, error) {
	switch s {
	case "presence":
		return signature.BranchProofPathPresence, nil
	case "equal":
		return signature.BranchProofPathEqual, nil
	case "not_equal":
		return signature.BranchProofPathNotEqual, nil
	case "index_in_range":
		return signature.BranchProofIndexInRange, nil
	default:
		return 0, fmt.Errorf("unknown branch proof kind %q", s)
	}
}

func encodeLifecycleKind(kind signature.LifecycleKind) (string, error) {
	switch kind {
	case signature.LifecycleAcquire:
		return "acquire", nil
	case signature.LifecycleTransition:
		return "transition", nil
	case signature.LifecycleEscape:
		return "escape", nil
	default:
		return "", fmt.Errorf("unsupported lifecycle kind %d", kind)
	}
}

func decodeLifecycleKind(s string) (signature.LifecycleKind, error) {
	switch s {
	case "acquire":
		return signature.LifecycleAcquire, nil
	case "transition":
		return signature.LifecycleTransition, nil
	case "escape":
		return signature.LifecycleEscape, nil
	default:
		return signature.LifecycleNone, fmt.Errorf("unknown lifecycle kind %q", s)
	}
}

func encodeEscapeKind(kind signature.EscapeKind) (string, error) {
	switch kind {
	case signature.EscapeNone:
		return "none", nil
	case signature.EscapeBorrow:
		return "borrow", nil
	case signature.EscapeRetain:
		return "retain", nil
	case signature.EscapeStore:
		return "store", nil
	case signature.EscapeSend:
		return "send", nil
	case signature.EscapeExport:
		return "export", nil
	case signature.EscapeOpaque:
		return "opaque", nil
	default:
		return "", fmt.Errorf("unsupported escape kind %d", kind)
	}
}

func decodeEscapeKind(s string) (signature.EscapeKind, error) {
	switch s {
	case "none":
		return signature.EscapeNone, nil
	case "borrow":
		return signature.EscapeBorrow, nil
	case "retain":
		return signature.EscapeRetain, nil
	case "store":
		return signature.EscapeStore, nil
	case "send":
		return signature.EscapeSend, nil
	case "export":
		return signature.EscapeExport, nil
	case "opaque":
		return signature.EscapeOpaque, nil
	default:
		return signature.EscapeNone, fmt.Errorf("unknown escape kind %q", s)
	}
}

func encodePlacementConsequence(consequence signature.PlacementConsequence) (string, error) {
	switch consequence {
	case signature.PlacementConsequenceKeep,
		signature.PlacementConsequenceOwnedHeap,
		signature.PlacementConsequenceSharedHeap:
		return string(consequence), nil
	default:
		return "", fmt.Errorf("unsupported placement consequence %q", consequence)
	}
}

func decodePlacementConsequence(s string) (signature.PlacementConsequence, error) {
	switch signature.PlacementConsequence(s) {
	case signature.PlacementConsequenceKeep:
		return signature.PlacementConsequenceKeep, nil
	case signature.PlacementConsequenceOwnedHeap:
		return signature.PlacementConsequenceOwnedHeap, nil
	case signature.PlacementConsequenceSharedHeap:
		return signature.PlacementConsequenceSharedHeap, nil
	default:
		return "", fmt.Errorf("unknown placement consequence %q", s)
	}
}
