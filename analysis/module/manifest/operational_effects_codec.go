package manifest

import (
	"encoding/json"
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

type operationalEffectsWire struct {
	ReturnPresenceRelations         []returnPresenceRelationWire   `json:"returnPresenceRelations,omitempty"`
	NormalReturnPresenceRefinements []pathPresenceRefinementWire   `json:"normalReturnPresenceRefinements,omitempty"`
	PathStaticMembers               []pathStaticMemberWire         `json:"pathStaticMembers,omitempty"`
	PathInvalidations               []pathInvalidationWire         `json:"pathInvalidations,omitempty"`
	DynamicIndexFacts               []dynamicIndexFactWire         `json:"dynamicIndexFacts,omitempty"`
	FrozenTables                    []frozenTableWire              `json:"frozenTables,omitempty"`
	EscapeEvents                    []escapeEventWire              `json:"escapeEvents,omitempty"`
	StoreRelations                  []storeRelationWire            `json:"storeRelations,omitempty"`
	LifecycleEffects                []lifecycleEffectWire          `json:"lifecycleEffects,omitempty"`
	ReturnAllocationTemplates       []returnAllocationTemplateWire `json:"returnAllocationTemplates,omitempty"`
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

type pathStaticMemberWire struct {
	Path *placeholderPathWire `json:"path,omitempty"`
	Type *typeWire            `json:"type,omitempty"`
}

type pathInvalidationWire struct {
	Path *placeholderPathWire `json:"path,omitempty"`
}

type dynamicIndexFactWire struct {
	Table       *placeholderPathWire    `json:"table,omitempty"`
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

type lifecycleEffectWire struct {
	Target   *placeholderPathWire `json:"target,omitempty"`
	Kind     string               `json:"kind"`
	Protocol string               `json:"protocol"`
	From     string               `json:"from,omitempty"`
	To       string               `json:"to,omitempty"`
	Final    string               `json:"final,omitempty"`
}

type returnAllocationTemplateWire struct {
	ReturnIndex *int                   `json:"returnIndex"`
	Root        string                 `json:"root"`
	Objects     []allocationObjectWire `json:"objects,omitempty"`
}

type allocationObjectWire struct {
	ID             string                       `json:"id"`
	Type           *typeWire                    `json:"type,omitempty"`
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

func encodeOperationalEffects(e *signature.OperationalEffects) (*operationalEffectsWire, error) {
	if e == nil || e.IsEmpty() {
		return nil, nil
	}
	out := &operationalEffectsWire{}
	for _, relation := range e.ReturnPresenceRelations {
		trigger, err := encodePresence(relation.TriggerPresence)
		if err != nil {
			return nil, fmt.Errorf("return relation trigger presence: %w", err)
		}
		target, err := encodePresence(relation.TargetPresence)
		if err != nil {
			return nil, fmt.Errorf("return relation target presence: %w", err)
		}
		out.ReturnPresenceRelations = append(out.ReturnPresenceRelations, returnPresenceRelationWire{
			TriggerIndex:    encodeInt(relation.TriggerIndex),
			TriggerPresence: trigger,
			TargetIndex:     encodeInt(relation.TargetIndex),
			TargetPresence:  target,
		})
	}
	for _, refinement := range e.NormalReturnPresenceRefinements {
		p, err := encodePlaceholderPath(refinement.Path)
		if err != nil {
			return nil, fmt.Errorf("normal return presence refinement path: %w", err)
		}
		pr, err := encodePresence(refinement.Presence)
		if err != nil {
			return nil, fmt.Errorf("normal return presence refinement: %w", err)
		}
		out.NormalReturnPresenceRefinements = append(out.NormalReturnPresenceRefinements, pathPresenceRefinementWire{
			Path:     p,
			Presence: pr,
		})
	}
	for _, member := range e.PathStaticMembers {
		p, err := encodePlaceholderPath(member.Path)
		if err != nil {
			return nil, fmt.Errorf("path static member path: %w", err)
		}
		if member.Type == nil {
			return nil, fmt.Errorf("path static member type: missing")
		}
		t, err := encodeType(member.Type)
		if err != nil {
			return nil, fmt.Errorf("path static member type: %w", err)
		}
		out.PathStaticMembers = append(out.PathStaticMembers, pathStaticMemberWire{
			Path: p,
			Type: t,
		})
	}
	for _, invalidation := range e.PathInvalidations {
		p, err := encodePlaceholderPath(invalidation.Path)
		if err != nil {
			return nil, fmt.Errorf("path invalidation: %w", err)
		}
		out.PathInvalidations = append(out.PathInvalidations, pathInvalidationWire{Path: p})
	}
	for _, fact := range e.DynamicIndexFacts {
		encoded, err := encodeDynamicIndexFact(fact)
		if err != nil {
			return nil, fmt.Errorf("dynamic index fact: %w", err)
		}
		out.DynamicIndexFacts = append(out.DynamicIndexFacts, encoded)
	}
	for _, frozen := range e.FrozenTables {
		p, err := encodePlaceholderPath(frozen.Target)
		if err != nil {
			return nil, fmt.Errorf("frozen table: %w", err)
		}
		out.FrozenTables = append(out.FrozenTables, frozenTableWire{Target: p})
	}
	for _, event := range e.EscapeEvents {
		p, err := encodePlaceholderPath(event.Target)
		if err != nil {
			return nil, fmt.Errorf("escape event target: %w", err)
		}
		kind, err := encodeEscapeKind(event.Kind)
		if err != nil {
			return nil, err
		}
		out.EscapeEvents = append(out.EscapeEvents, escapeEventWire{
			Target:    p,
			Kind:      kind,
			Recursive: event.Recursive,
		})
	}
	for _, relation := range e.StoreRelations {
		source, err := encodePlaceholderPath(relation.Source)
		if err != nil {
			return nil, fmt.Errorf("store relation source: %w", err)
		}
		into, err := encodePlaceholderPath(relation.Into)
		if err != nil {
			return nil, fmt.Errorf("store relation target: %w", err)
		}
		out.StoreRelations = append(out.StoreRelations, storeRelationWire{Source: source, Into: into})
	}
	for _, effect := range e.LifecycleEffects {
		encoded, err := encodeLifecycleEffect(effect)
		if err != nil {
			return nil, fmt.Errorf("lifecycle effect: %w", err)
		}
		out.LifecycleEffects = append(out.LifecycleEffects, encoded)
	}
	for _, template := range e.ReturnAllocationTemplates {
		encoded, err := encodeReturnAllocationTemplate(template)
		if err != nil {
			return nil, fmt.Errorf("return allocation template: %w", err)
		}
		out.ReturnAllocationTemplates = append(out.ReturnAllocationTemplates, encoded)
	}
	canonicalizeOperationalEffectsWire(out)
	return out, nil
}

func decodeOperationalEffects(w *operationalEffectsWire) (signature.OperationalEffects, error) {
	if w == nil {
		return signature.OperationalEffects{}, nil
	}
	var out signature.OperationalEffects
	for _, relation := range w.ReturnPresenceRelations {
		trigger, err := decodePresence(relation.TriggerPresence)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("return relation trigger presence: %w", err)
		}
		target, err := decodePresence(relation.TargetPresence)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("return relation target presence: %w", err)
		}
		triggerIndex, err := decodeRequiredInt(relation.TriggerIndex, "return relation trigger index missing")
		if err != nil {
			return signature.OperationalEffects{}, err
		}
		targetIndex, err := decodeRequiredInt(relation.TargetIndex, "return relation target index missing")
		if err != nil {
			return signature.OperationalEffects{}, err
		}
		out.ReturnPresenceRelations = append(out.ReturnPresenceRelations, signature.ReturnPresenceRelation{
			TriggerIndex:    triggerIndex,
			TriggerPresence: trigger,
			TargetIndex:     targetIndex,
			TargetPresence:  target,
		})
	}
	for _, refinement := range w.NormalReturnPresenceRefinements {
		p, err := decodePlaceholderPath(refinement.Path)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("normal return presence refinement path: %w", err)
		}
		pr, err := decodePresence(refinement.Presence)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("normal return presence refinement: %w", err)
		}
		out.NormalReturnPresenceRefinements = append(out.NormalReturnPresenceRefinements, signature.PathPresenceRefinement{
			Path:     p,
			Presence: pr,
		})
	}
	for _, member := range w.PathStaticMembers {
		p, err := decodePlaceholderPath(member.Path)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("path static member path: %w", err)
		}
		t, err := decodeType(member.Type)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("path static member type: %w", err)
		}
		if t == nil {
			return signature.OperationalEffects{}, fmt.Errorf("path static member type: missing")
		}
		out.PathStaticMembers = append(out.PathStaticMembers, signature.PathStaticMemberFact{
			Path: p,
			Type: t,
		})
	}
	for _, invalidation := range w.PathInvalidations {
		p, err := decodePlaceholderPath(invalidation.Path)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("path invalidation: %w", err)
		}
		out.PathInvalidations = append(out.PathInvalidations, signature.PathInvalidation{Path: p})
	}
	for _, fact := range w.DynamicIndexFacts {
		decoded, err := decodeDynamicIndexFact(fact)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("dynamic index fact: %w", err)
		}
		out.DynamicIndexFacts = append(out.DynamicIndexFacts, decoded)
	}
	for _, frozen := range w.FrozenTables {
		p, err := decodePlaceholderPath(frozen.Target)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("frozen table: %w", err)
		}
		out.FrozenTables = append(out.FrozenTables, signature.FrozenTable{Target: p})
	}
	for _, event := range w.EscapeEvents {
		p, err := decodePlaceholderPath(event.Target)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("escape event target: %w", err)
		}
		kind, err := decodeEscapeKind(event.Kind)
		if err != nil {
			return signature.OperationalEffects{}, err
		}
		out.EscapeEvents = append(out.EscapeEvents, signature.EscapeEvent{
			Target:    p,
			Kind:      kind,
			Recursive: event.Recursive,
		})
	}
	for _, relation := range w.StoreRelations {
		source, err := decodePlaceholderPath(relation.Source)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("store relation source: %w", err)
		}
		into, err := decodePlaceholderPath(relation.Into)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("store relation target: %w", err)
		}
		out.StoreRelations = append(out.StoreRelations, signature.StoreRelation{Source: source, Into: into})
	}
	for _, effect := range w.LifecycleEffects {
		decoded, err := decodeLifecycleEffect(effect)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("lifecycle effect: %w", err)
		}
		out.LifecycleEffects = append(out.LifecycleEffects, decoded)
	}
	for _, template := range w.ReturnAllocationTemplates {
		decoded, err := decodeReturnAllocationTemplate(template)
		if err != nil {
			return signature.OperationalEffects{}, fmt.Errorf("return allocation template: %w", err)
		}
		out.ReturnAllocationTemplates = append(out.ReturnAllocationTemplates, decoded)
	}
	return out, nil
}

func canonicalizeOperationalEffectsWire(w *operationalEffectsWire) {
	if w == nil {
		return
	}
	sort.Slice(w.ReturnPresenceRelations, func(i, j int) bool {
		left, right := w.ReturnPresenceRelations[i], w.ReturnPresenceRelations[j]
		if c := compareOptionalInt(left.TriggerIndex, right.TriggerIndex); c != 0 {
			return c < 0
		}
		if left.TriggerPresence != right.TriggerPresence {
			return left.TriggerPresence < right.TriggerPresence
		}
		if c := compareOptionalInt(left.TargetIndex, right.TargetIndex); c != 0 {
			return c < 0
		}
		return left.TargetPresence < right.TargetPresence
	})
	sort.Slice(w.NormalReturnPresenceRefinements, func(i, j int) bool {
		left, right := w.NormalReturnPresenceRefinements[i], w.NormalReturnPresenceRefinements[j]
		if c := comparePlaceholderPathWire(left.Path, right.Path); c != 0 {
			return c < 0
		}
		return left.Presence < right.Presence
	})
	sort.Slice(w.PathStaticMembers, func(i, j int) bool {
		left, right := w.PathStaticMembers[i], w.PathStaticMembers[j]
		if c := comparePlaceholderPathWire(left.Path, right.Path); c != 0 {
			return c < 0
		}
		return typeWireKey(left.Type) < typeWireKey(right.Type)
	})
	sort.Slice(w.PathInvalidations, func(i, j int) bool {
		return comparePlaceholderPathWire(w.PathInvalidations[i].Path, w.PathInvalidations[j].Path) < 0
	})
	sort.Slice(w.DynamicIndexFacts, func(i, j int) bool {
		left, right := w.DynamicIndexFacts[i], w.DynamicIndexFacts[j]
		if c := comparePlaceholderPathWire(left.Table, right.Table); c != 0 {
			return c < 0
		}
		if left.Site != right.Site {
			return left.Site < right.Site
		}
		if left.KeyPresence != right.KeyPresence {
			return left.KeyPresence < right.KeyPresence
		}
		if c := compareDynamicIndexOperandWire(left.Key, right.Key); c != 0 {
			return c < 0
		}
		if c := compareDynamicIndexOperandWire(left.Value, right.Value); c != 0 {
			return c < 0
		}
		return left.Admission < right.Admission
	})
	sort.Slice(w.FrozenTables, func(i, j int) bool {
		return comparePlaceholderPathWire(w.FrozenTables[i].Target, w.FrozenTables[j].Target) < 0
	})
	sort.Slice(w.EscapeEvents, func(i, j int) bool {
		left, right := w.EscapeEvents[i], w.EscapeEvents[j]
		if c := comparePlaceholderPathWire(left.Target, right.Target); c != 0 {
			return c < 0
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return !left.Recursive && right.Recursive
	})
	sort.Slice(w.StoreRelations, func(i, j int) bool {
		left, right := w.StoreRelations[i], w.StoreRelations[j]
		if c := comparePlaceholderPathWire(left.Source, right.Source); c != 0 {
			return c < 0
		}
		return comparePlaceholderPathWire(left.Into, right.Into) < 0
	})
	sort.Slice(w.LifecycleEffects, func(i, j int) bool {
		return compareLifecycleEffectWire(w.LifecycleEffects[i], w.LifecycleEffects[j]) < 0
	})
	for i := range w.ReturnAllocationTemplates {
		canonicalizeReturnAllocationTemplateWire(&w.ReturnAllocationTemplates[i])
	}
	sort.Slice(w.ReturnAllocationTemplates, func(i, j int) bool {
		left, right := w.ReturnAllocationTemplates[i], w.ReturnAllocationTemplates[j]
		if c := compareOptionalInt(left.ReturnIndex, right.ReturnIndex); c != 0 {
			return c < 0
		}
		return left.Root < right.Root
	})
}

func encodeDynamicIndexFact(fact signature.DynamicIndexFact) (dynamicIndexFactWire, error) {
	if fact.Site == "" {
		return dynamicIndexFactWire{}, fmt.Errorf("missing site")
	}
	table, err := encodePlaceholderPath(fact.Table)
	if err != nil {
		return dynamicIndexFactWire{}, fmt.Errorf("table: %w", err)
	}
	keyPresence, err := encodePresence(fact.KeyPresence)
	if err != nil {
		return dynamicIndexFactWire{}, fmt.Errorf("key presence: %w", err)
	}
	key, err := encodeDynamicIndexOperand(fact.Key)
	if err != nil {
		return dynamicIndexFactWire{}, fmt.Errorf("key: %w", err)
	}
	value, err := encodeDynamicIndexOperand(fact.Value)
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
	var out dynamicIndexOperandWire
	if !operand.Path.IsEmpty() {
		path, err := encodePlaceholderPath(operand.Path)
		if err != nil {
			return dynamicIndexOperandWire{}, fmt.Errorf("path: %w", err)
		}
		out.Path = path
	}
	if operand.Type != nil {
		typ, err := encodeType(operand.Type)
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
	table, err := decodePlaceholderPath(w.Table)
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

func encodeLifecycleEffect(effect signature.LifecycleEffect) (lifecycleEffectWire, error) {
	target, err := encodePlaceholderPath(effect.Target)
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
	}, nil
}

func decodeLifecycleEffect(w lifecycleEffectWire) (signature.LifecycleEffect, error) {
	target, err := decodePlaceholderPath(w.Target)
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
	return signature.LifecycleEffect{
		Target:   target,
		Kind:     kind,
		Protocol: protocol,
		From:     decodeOptionalLifecycleState(w.From),
		To:       to,
		Obligation: typestate.Obligation{
			Final: decodeOptionalLifecycleState(w.Final),
		},
	}, nil
}

func compareLifecycleEffectWire(a, b lifecycleEffectWire) int {
	if c := comparePlaceholderPathWire(a.Target, b.Target); c != 0 {
		return c
	}
	pairs := [][2]string{
		{a.Protocol, b.Protocol},
		{a.Kind, b.Kind},
		{a.From, b.From},
		{a.To, b.To},
		{a.Final, b.Final},
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

func encodeReturnAllocationTemplate(template signature.ReturnAllocationTemplate) (returnAllocationTemplateWire, error) {
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
		encoded, err := encodeAllocationObjectTemplate(object)
		if err != nil {
			return returnAllocationTemplateWire{}, err
		}
		out.Objects = append(out.Objects, encoded)
	}
	return out, nil
}

func encodeAllocationObjectTemplate(object signature.AllocationObjectTemplate) (allocationObjectWire, error) {
	if object.ID == "" {
		return allocationObjectWire{}, fmt.Errorf("missing object id")
	}
	out := allocationObjectWire{ID: string(object.ID)}
	if object.Type != nil {
		encoded, err := encodeType(object.Type)
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
			encoded, err := encodeType(entry.KeyType)
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
	out := signature.AllocationObjectTemplate{ID: signature.AllocationTemplateID(w.ID)}
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

func canonicalizeReturnAllocationTemplateWire(w *returnAllocationTemplateWire) {
	if w == nil {
		return
	}
	for i := range w.Objects {
		canonicalizeAllocationObjectWire(&w.Objects[i])
	}
	sort.Slice(w.Objects, func(i, j int) bool {
		return w.Objects[i].ID < w.Objects[j].ID
	})
}

func canonicalizeAllocationObjectWire(w *allocationObjectWire) {
	if w == nil {
		return
	}
	sort.Slice(w.StaticMembers, func(i, j int) bool {
		left, right := w.StaticMembers[i], w.StaticMembers[j]
		if left.Suffix != right.Suffix {
			return left.Suffix < right.Suffix
		}
		return left.Value < right.Value
	})
	sort.Slice(w.DynamicEntries, func(i, j int) bool {
		left, right := w.DynamicEntries[i], w.DynamicEntries[j]
		if left.Key != right.Key {
			return left.Key < right.Key
		}
		if left.Value != right.Value {
			return left.Value < right.Value
		}
		return typeWireKey(left.KeyType) < typeWireKey(right.KeyType)
	})
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
