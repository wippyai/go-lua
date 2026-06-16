package signature

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// OperationalEffects carries analyzed, call-boundary facts across module
// manifests. Unlike Effect rows, this is not handwritten contract vocabulary:
// it is a stable, param-relative serialization of facts the analyzer proved.
type OperationalEffects struct {
	ReturnPresenceRelations         []ReturnPresenceRelation
	NormalReturnPresenceRefinements []PathPresenceRefinement
	PathInvalidations               []PathInvalidation
	FrozenTables                    []FrozenTable
	EscapeEvents                    []EscapeEvent
	StoreRelations                  []StoreRelation
}

type ReturnPresenceRelation struct {
	TriggerIndex    int
	TriggerPresence presence.Value
	TargetIndex     int
	TargetPresence  presence.Value
}

type PathPresenceRefinement struct {
	Path     pathdom.Path
	Presence presence.Value
}

type PathInvalidation struct {
	Path pathdom.Path
}

type FrozenTable struct {
	Target pathdom.Path
}

type EscapeKind uint8

const (
	EscapeNone EscapeKind = iota
	EscapeBorrow
	EscapeRetain
	EscapeStore
	EscapeSend
	EscapeExport
	EscapeOpaque
)

type EscapeEvent struct {
	Target    pathdom.Path
	Kind      EscapeKind
	Recursive bool
}

type StoreRelation struct {
	Source pathdom.Path
	Into   pathdom.Path
}

func (e OperationalEffects) IsEmpty() bool {
	return len(e.ReturnPresenceRelations) == 0 &&
		len(e.NormalReturnPresenceRefinements) == 0 &&
		len(e.PathInvalidations) == 0 &&
		len(e.FrozenTables) == 0 &&
		len(e.EscapeEvents) == 0 &&
		len(e.StoreRelations) == 0
}

func (e OperationalEffects) Clone() OperationalEffects {
	return OperationalEffects{
		ReturnPresenceRelations:         cloneReturnPresenceRelations(e.ReturnPresenceRelations),
		NormalReturnPresenceRefinements: clonePathPresenceRefinements(e.NormalReturnPresenceRefinements),
		PathInvalidations:               clonePathInvalidations(e.PathInvalidations),
		FrozenTables:                    cloneFrozenTables(e.FrozenTables),
		EscapeEvents:                    cloneEscapeEvents(e.EscapeEvents),
		StoreRelations:                  cloneStoreRelations(e.StoreRelations),
	}
}

func (e OperationalEffects) Equals(other OperationalEffects) bool {
	return equalReturnPresenceRelations(e.ReturnPresenceRelations, other.ReturnPresenceRelations) &&
		equalPathPresenceRefinements(e.NormalReturnPresenceRefinements, other.NormalReturnPresenceRefinements) &&
		equalPathInvalidations(e.PathInvalidations, other.PathInvalidations) &&
		equalFrozenTables(e.FrozenTables, other.FrozenTables) &&
		equalEscapeEvents(e.EscapeEvents, other.EscapeEvents) &&
		equalStoreRelations(e.StoreRelations, other.StoreRelations)
}

func cloneReturnPresenceRelations(in []ReturnPresenceRelation) []ReturnPresenceRelation {
	if len(in) == 0 {
		return nil
	}
	return append([]ReturnPresenceRelation(nil), in...)
}

func clonePathPresenceRefinements(in []PathPresenceRefinement) []PathPresenceRefinement {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathPresenceRefinement, len(in))
	for i, fact := range in {
		out[i] = PathPresenceRefinement{Path: clonePath(fact.Path), Presence: fact.Presence}
	}
	return out
}

func clonePathInvalidations(in []PathInvalidation) []PathInvalidation {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathInvalidation, len(in))
	for i, fact := range in {
		out[i] = PathInvalidation{Path: clonePath(fact.Path)}
	}
	return out
}

func cloneFrozenTables(in []FrozenTable) []FrozenTable {
	if len(in) == 0 {
		return nil
	}
	out := make([]FrozenTable, len(in))
	for i, fact := range in {
		out[i] = FrozenTable{Target: clonePath(fact.Target)}
	}
	return out
}

func cloneEscapeEvents(in []EscapeEvent) []EscapeEvent {
	if len(in) == 0 {
		return nil
	}
	out := make([]EscapeEvent, len(in))
	for i, fact := range in {
		out[i] = EscapeEvent{Target: clonePath(fact.Target), Kind: fact.Kind, Recursive: fact.Recursive}
	}
	return out
}

func cloneStoreRelations(in []StoreRelation) []StoreRelation {
	if len(in) == 0 {
		return nil
	}
	out := make([]StoreRelation, len(in))
	for i, fact := range in {
		out[i] = StoreRelation{Source: clonePath(fact.Source), Into: clonePath(fact.Into)}
	}
	return out
}

func clonePath(p pathdom.Path) pathdom.Path {
	if len(p.Segments) == 0 {
		return p
	}
	p.Segments = append([]segment.Segment(nil), p.Segments...)
	return p
}

func equalReturnPresenceRelations(a, b []ReturnPresenceRelation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalPathPresenceRefinements(a, b []PathPresenceRefinement) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Path.Equal(b[i].Path) || !presence.Equal(a[i].Presence, b[i].Presence) {
			return false
		}
	}
	return true
}

func equalPathInvalidations(a, b []PathInvalidation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Path.Equal(b[i].Path) {
			return false
		}
	}
	return true
}

func equalFrozenTables(a, b []FrozenTable) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Target.Equal(b[i].Target) {
			return false
		}
	}
	return true
}

func equalEscapeEvents(a, b []EscapeEvent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Kind != b[i].Kind || a[i].Recursive != b[i].Recursive || !a[i].Target.Equal(b[i].Target) {
			return false
		}
	}
	return true
}

func equalStoreRelations(a, b []StoreRelation) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Source.Equal(b[i].Source) || !a[i].Into.Equal(b[i].Into) {
			return false
		}
	}
	return true
}
