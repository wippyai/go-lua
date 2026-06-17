package signature

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// OperationalEffects carries analyzed, call-boundary facts across module
// manifests. Unlike Effect rows, this is not handwritten contract vocabulary:
// it is a stable, param-relative serialization of facts the analyzer proved.
type OperationalEffects struct {
	ReturnPresenceRelations         []ReturnPresenceRelation
	NormalReturnPresenceRefinements []PathPresenceRefinement
	PathStaticMembers               []PathStaticMemberFact
	PathInvalidations               []PathInvalidation
	FrozenTables                    []FrozenTable
	EscapeEvents                    []EscapeEvent
	StoreRelations                  []StoreRelation
	ReturnAllocationTemplates       []ReturnAllocationTemplate
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

type PathStaticMemberFact struct {
	Path pathdom.Path
	Type typ.Type
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

type AllocationTemplateID string

type ReturnAllocationTemplate struct {
	ReturnIndex int
	Root        AllocationTemplateID
	Objects     []AllocationObjectTemplate
}

type AllocationObjectTemplate struct {
	ID             AllocationTemplateID
	Type           typ.Type
	StaticMembers  []AllocationStaticMemberTemplate
	DynamicEntries []AllocationDynamicEntryTemplate
}

type AllocationStaticMemberTemplate struct {
	Suffix []segment.Segment
	Value  AllocationTemplateID
}

type AllocationDynamicEntryTemplate struct {
	Key     AllocationTemplateID
	KeyType typ.Type
	Value   AllocationTemplateID
}

func (e OperationalEffects) IsEmpty() bool {
	return len(e.ReturnPresenceRelations) == 0 &&
		len(e.NormalReturnPresenceRefinements) == 0 &&
		len(e.PathStaticMembers) == 0 &&
		len(e.PathInvalidations) == 0 &&
		len(e.FrozenTables) == 0 &&
		len(e.EscapeEvents) == 0 &&
		len(e.StoreRelations) == 0 &&
		len(e.ReturnAllocationTemplates) == 0
}

func (e OperationalEffects) Clone() OperationalEffects {
	return OperationalEffects{
		ReturnPresenceRelations:         cloneReturnPresenceRelations(e.ReturnPresenceRelations),
		NormalReturnPresenceRefinements: clonePathPresenceRefinements(e.NormalReturnPresenceRefinements),
		PathStaticMembers:               clonePathStaticMemberFacts(e.PathStaticMembers),
		PathInvalidations:               clonePathInvalidations(e.PathInvalidations),
		FrozenTables:                    cloneFrozenTables(e.FrozenTables),
		EscapeEvents:                    cloneEscapeEvents(e.EscapeEvents),
		StoreRelations:                  cloneStoreRelations(e.StoreRelations),
		ReturnAllocationTemplates:       cloneReturnAllocationTemplates(e.ReturnAllocationTemplates),
	}
}

func (e OperationalEffects) Equals(other OperationalEffects) bool {
	return equalReturnPresenceRelations(e.ReturnPresenceRelations, other.ReturnPresenceRelations) &&
		equalPathPresenceRefinements(e.NormalReturnPresenceRefinements, other.NormalReturnPresenceRefinements) &&
		equalPathStaticMemberFacts(e.PathStaticMembers, other.PathStaticMembers) &&
		equalPathInvalidations(e.PathInvalidations, other.PathInvalidations) &&
		equalFrozenTables(e.FrozenTables, other.FrozenTables) &&
		equalEscapeEvents(e.EscapeEvents, other.EscapeEvents) &&
		equalStoreRelations(e.StoreRelations, other.StoreRelations) &&
		equalReturnAllocationTemplates(e.ReturnAllocationTemplates, other.ReturnAllocationTemplates)
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
		out[i] = PathPresenceRefinement{Path: fact.Path.Clone(), Presence: fact.Presence}
	}
	return out
}

func clonePathStaticMemberFacts(in []PathStaticMemberFact) []PathStaticMemberFact {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathStaticMemberFact, len(in))
	for i, fact := range in {
		out[i] = PathStaticMemberFact{Path: fact.Path.Clone(), Type: fact.Type}
	}
	return out
}

func clonePathInvalidations(in []PathInvalidation) []PathInvalidation {
	if len(in) == 0 {
		return nil
	}
	out := make([]PathInvalidation, len(in))
	for i, fact := range in {
		out[i] = PathInvalidation{Path: fact.Path.Clone()}
	}
	return out
}

func cloneFrozenTables(in []FrozenTable) []FrozenTable {
	if len(in) == 0 {
		return nil
	}
	out := make([]FrozenTable, len(in))
	for i, fact := range in {
		out[i] = FrozenTable{Target: fact.Target.Clone()}
	}
	return out
}

func cloneEscapeEvents(in []EscapeEvent) []EscapeEvent {
	if len(in) == 0 {
		return nil
	}
	out := make([]EscapeEvent, len(in))
	for i, fact := range in {
		out[i] = EscapeEvent{Target: fact.Target.Clone(), Kind: fact.Kind, Recursive: fact.Recursive}
	}
	return out
}

func cloneStoreRelations(in []StoreRelation) []StoreRelation {
	if len(in) == 0 {
		return nil
	}
	out := make([]StoreRelation, len(in))
	for i, fact := range in {
		out[i] = StoreRelation{Source: fact.Source.Clone(), Into: fact.Into.Clone()}
	}
	return out
}

func cloneReturnAllocationTemplates(in []ReturnAllocationTemplate) []ReturnAllocationTemplate {
	if len(in) == 0 {
		return nil
	}
	out := make([]ReturnAllocationTemplate, len(in))
	for i, template := range in {
		out[i] = ReturnAllocationTemplate{
			ReturnIndex: template.ReturnIndex,
			Root:        template.Root,
			Objects:     cloneAllocationObjectTemplates(template.Objects),
		}
	}
	return out
}

func cloneAllocationObjectTemplates(in []AllocationObjectTemplate) []AllocationObjectTemplate {
	if len(in) == 0 {
		return nil
	}
	out := make([]AllocationObjectTemplate, len(in))
	for i, object := range in {
		out[i] = AllocationObjectTemplate{
			ID:             object.ID,
			Type:           object.Type,
			StaticMembers:  cloneAllocationStaticMemberTemplates(object.StaticMembers),
			DynamicEntries: cloneAllocationDynamicEntryTemplates(object.DynamicEntries),
		}
	}
	return out
}

func cloneAllocationStaticMemberTemplates(in []AllocationStaticMemberTemplate) []AllocationStaticMemberTemplate {
	if len(in) == 0 {
		return nil
	}
	out := make([]AllocationStaticMemberTemplate, len(in))
	for i, member := range in {
		out[i] = AllocationStaticMemberTemplate{
			Suffix: append([]segment.Segment(nil), member.Suffix...),
			Value:  member.Value,
		}
	}
	return out
}

func cloneAllocationDynamicEntryTemplates(in []AllocationDynamicEntryTemplate) []AllocationDynamicEntryTemplate {
	if len(in) == 0 {
		return nil
	}
	out := make([]AllocationDynamicEntryTemplate, len(in))
	copy(out, in)
	return out
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

func equalPathStaticMemberFacts(a, b []PathStaticMemberFact) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Path.Equal(b[i].Path) || !typ.TypeEquals(a[i].Type, b[i].Type) {
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

func equalReturnAllocationTemplates(a, b []ReturnAllocationTemplate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ReturnIndex != b[i].ReturnIndex || a[i].Root != b[i].Root ||
			!equalAllocationObjectTemplates(a[i].Objects, b[i].Objects) {
			return false
		}
	}
	return true
}

func equalAllocationObjectTemplates(a, b []AllocationObjectTemplate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID ||
			!typ.TypeEquals(a[i].Type, b[i].Type) ||
			!equalAllocationStaticMemberTemplates(a[i].StaticMembers, b[i].StaticMembers) ||
			!equalAllocationDynamicEntryTemplates(a[i].DynamicEntries, b[i].DynamicEntries) {
			return false
		}
	}
	return true
}

func equalAllocationStaticMemberTemplates(a, b []AllocationStaticMemberTemplate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Value != b[i].Value || segment.FormatSegments(a[i].Suffix) != segment.FormatSegments(b[i].Suffix) {
			return false
		}
	}
	return true
}

func equalAllocationDynamicEntryTemplates(a, b []AllocationDynamicEntryTemplate) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Value != b[i].Value || !typ.TypeEquals(a[i].KeyType, b[i].KeyType) {
			return false
		}
	}
	return true
}
