package manifest

import (
	"encoding/json"
	"fmt"
	"sort"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

type operationalEffectsWire struct {
	ReturnPresenceRelations         []returnPresenceRelationWire `json:"returnPresenceRelations,omitempty"`
	NormalReturnPresenceRefinements []pathPresenceRefinementWire `json:"normalReturnPresenceRefinements,omitempty"`
	PathStaticMembers               []pathStaticMemberWire       `json:"pathStaticMembers,omitempty"`
	PathInvalidations               []pathInvalidationWire       `json:"pathInvalidations,omitempty"`
	FrozenTables                    []frozenTableWire            `json:"frozenTables,omitempty"`
	EscapeEvents                    []escapeEventWire            `json:"escapeEvents,omitempty"`
	StoreRelations                  []storeRelationWire          `json:"storeRelations,omitempty"`
}

type returnPresenceRelationWire struct {
	TriggerIndex    int    `json:"triggerIndex"`
	TriggerPresence string `json:"triggerPresence"`
	TargetIndex     int    `json:"targetIndex"`
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

type placeholderPathWire struct {
	Param  int    `json:"param"`
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
			TriggerIndex:    relation.TriggerIndex,
			TriggerPresence: trigger,
			TargetIndex:     relation.TargetIndex,
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
		out.ReturnPresenceRelations = append(out.ReturnPresenceRelations, signature.ReturnPresenceRelation{
			TriggerIndex:    relation.TriggerIndex,
			TriggerPresence: trigger,
			TargetIndex:     relation.TargetIndex,
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
	return out, nil
}

func canonicalizeOperationalEffectsWire(w *operationalEffectsWire) {
	if w == nil {
		return
	}
	sort.Slice(w.ReturnPresenceRelations, func(i, j int) bool {
		left, right := w.ReturnPresenceRelations[i], w.ReturnPresenceRelations[j]
		if left.TriggerIndex != right.TriggerIndex {
			return left.TriggerIndex < right.TriggerIndex
		}
		if left.TriggerPresence != right.TriggerPresence {
			return left.TriggerPresence < right.TriggerPresence
		}
		if left.TargetIndex != right.TargetIndex {
			return left.TargetIndex < right.TargetIndex
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
	case a.Param != b.Param:
		if a.Param < b.Param {
			return -1
		}
		return 1
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
		Param:  p.PlaceholderIndex(),
		Suffix: segment.FormatSegments(p.Segments),
	}, nil
}

func decodePlaceholderPath(w *placeholderPathWire) (pathdom.Path, error) {
	if w == nil {
		return pathdom.Path{}, fmt.Errorf("missing placeholder path")
	}
	if w.Param < 0 {
		return pathdom.Path{}, fmt.Errorf("negative placeholder index %d", w.Param)
	}
	segs, ok := segment.ParseFormattedSegments(w.Suffix)
	if !ok {
		return pathdom.Path{}, fmt.Errorf("invalid placeholder path suffix %q", w.Suffix)
	}
	p := pathdom.NewPlaceholder(w.Param)
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
