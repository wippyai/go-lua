package suspension

import (
	"bytes"
	"crypto/sha256"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/materialization"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

type RetainedKind uint8

const (
	RetainedValue RetainedKind = iota + 1
	RetainedCell
)

// RetentionClass classifies whether a suspended subject is private to its
// continuation or shared beyond it. This is not materialization age.
type RetentionClass uint8

const (
	RetentionPrivate RetentionClass = iota + 1
	RetentionShared
)

// RetentionClasses is the finite projection of the classes present in a
// lifecycle relation. It deliberately does not reuse materialization terms.
type RetentionClasses uint8

const (
	RetentionClassesNone RetentionClasses = iota
	RetentionClassesPrivate
	RetentionClassesShared
	RetentionClassesMixed
)

// RetentionSubject is a family-issued typed Boundary Value/Cell subject. It does
// not carry a Key; the same semantic subject is comparable across occurrence
// keys, while Retain checks its target key's exact support.
type RetentionSubject struct {
	owner *schema
	kind  RetainedKind
	value linkboundary.Value
}

func (subject RetentionSubject) Kind() RetainedKind        { return subject.kind }
func (subject RetentionSubject) Value() linkboundary.Value { return subject.value }

type retainedAtom struct {
	kind  RetainedKind
	value linkboundary.Value
}

// Schema is the one immutable Suspension Factor family for a sealed Link.
// Values carry only this family owner; keys select exact occurrence support at
// construction and rank validation.
type Schema struct{ owner *schema }

type schema struct {
	source  *link.Link
	linkID  keyspace.ContentID
	keys    []keySupport
	keyByID map[keyspace.ContentID]uint32
}

func (owner *schema) compareValues(left, right linkboundary.Value) (int, bool) {
	if owner == nil || owner.source == nil {
		return 0, false
	}
	if owner.source.Boundary() == nil {
		return 0, false
	}
	return owner.source.Boundary().Values().Compare(left, right)
}

func (owner *schema) compareSubjects(left, right RetentionSubject) (int, bool) {
	if owner == nil || left.owner != owner || right.owner != owner {
		return 0, false
	}
	if left.kind < right.kind {
		return -1, true
	}
	if left.kind > right.kind {
		return 1, true
	}
	return owner.compareValues(left.value, right.value)
}

// generationSource is Suspension's closed typed source family. It is not a
// generic Link boundary: an operation suspension and a module-cache init are
// the only two structures that create a continuation generation. Their
// lifecycle, Recent/Summary alternatives, liveness, and consumption remain
// one Suspension semantic owner.
type generationSourceKind uint8

const (
	generationSourceOperation generationSourceKind = iota + 1
	generationSourceModuleInit
)

type generationSource struct {
	kind        generationSourceKind
	application linkproject.Application
	operation   target.Operation
	suspension  uint32
	moduleInit  linkmodule.ModuleInitGeneration
}

type keySupport struct {
	generation generationSource
	id         keyspace.ContentID
	retained   []retainedAtom
}

// NewSchema derives the complete finite key→retained-support table directly
// from Link's complete typed generation ranges. Link owns the factored Call ×
// operation × port range; Suspension never recreates an Application×Seed or
// operation universe. Module-init sites share this one lifecycle family while
// retaining their distinct Link source type.
func NewSchema(source *link.Link) (Schema, bool) {
	if source == nil || !source.ContentID().Available() {
		return Schema{}, false
	}
	owner := &schema{source: source, linkID: source.ContentID()}
	seen := make(map[keyspace.ContentID]struct{})
	contract, contractOK := source.Boundary().Target()
	if !contractOK || contract == nil {
		return Schema{}, false
	}
	for applicationIndex := 0; applicationIndex < source.Project().Applications().Count(); applicationIndex++ {
		application, applicationOK := source.Project().Applications().At(applicationIndex)
		if !applicationOK {
			return Schema{}, false
		}
		for operationIndex := 0; operationIndex < contract.OperationCount(); operationIndex++ {
			operation := target.Operation(operationIndex + 1)
			if !source.Boundary().ApplicationOperationAvailable(contract, application, operation) {
				continue
			}
			for suspensionIndex := 0; suspensionIndex < contract.SuspensionCount(operation); suspensionIndex++ {
				if _, _, _, _, ok := contract.SuspensionAt(operation, suspensionIndex); !ok {
					return Schema{}, false
				}
				id, idOK := operationGenerationID(source, application, operation, uint32(suspensionIndex))
				retained, retainedOK := occurrenceSubjects(source, application)
				if !idOK || !retainedOK {
					return Schema{}, false
				}
				if _, duplicate := seen[id]; duplicate {
					return Schema{}, false
				}
				seen[id] = struct{}{}
				owner.keys = append(owner.keys, keySupport{generation: generationSource{kind: generationSourceOperation, application: application, operation: operation, suspension: uint32(suspensionIndex)}, id: id, retained: retained})
			}
		}
	}
	for index := 0; index < source.Module().Generations().Count(); index++ {
		generation, ok := source.Module().Generations().At(index)
		if !ok {
			return Schema{}, false
		}
		id, ok := source.Module().Generations().ID(generation)
		if !ok || !id.Available() {
			return Schema{}, false
		}
		if _, duplicate := seen[id]; duplicate {
			return Schema{}, false
		}
		seen[id] = struct{}{}
		owner.keys = append(owner.keys, keySupport{generation: generationSource{kind: generationSourceModuleInit, moduleInit: generation}, id: id})
	}
	sort.Slice(owner.keys, func(left, right int) bool {
		return bytes.Compare(owner.keys[left].id[:], owner.keys[right].id[:]) < 0
	})
	if !owner.indexKeys() {
		return Schema{}, false
	}
	return Schema{owner: owner}, true
}

// indexKeys derives the one reverse lookup over already-issued lifecycle
// keys. It carries no generation semantics: each row remains the sole owner
// of its source kind and Link handle. Building it only after canonical sorting
// keeps the dense Key ordinal and reverse index identical.
func (owner *schema) indexKeys() bool {
	if owner == nil || len(owner.keys) > int(^uint32(0)) {
		return false
	}
	index := make(map[keyspace.ContentID]uint32, len(owner.keys))
	for ordinal, support := range owner.keys {
		if !support.id.Available() || uint64(ordinal) > uint64(^uint32(0)) {
			return false
		}
		if _, duplicate := index[support.id]; duplicate {
			return false
		}
		index[support.id] = uint32(ordinal)
	}
	owner.keyByID = index
	return true
}

func (schema Schema) Valid() bool {
	return schema.owner != nil && schema.owner.source != nil && schema.owner.linkID.Available() && schema.owner.source.ContentID() == schema.owner.linkID
}

// LinkContentID returns Suspension's sealed Link replay identity.  The live
// Link projection below remains the construction/rule authority fence.
func (schema Schema) LinkContentID() (keyspace.ContentID, bool) {
	if !schema.Valid() {
		return keyspace.ContentID{}, false
	}
	return schema.owner.linkID, true
}

// Link returns Suspension's exact immutable structural authority.  A
// same-content independently sealed Link is not interchangeable at live
// construction boundaries.
func (schema Schema) Link() *link.Link {
	if !schema.Valid() {
		return nil
	}
	return schema.owner.source
}
func (schema Schema) KeyCount() int {
	if !schema.Valid() {
		return 0
	}
	return len(schema.owner.keys)
}
func (schema Schema) KeyAt(index int) (Key, bool) {
	if !schema.Valid() || index < 0 || index >= len(schema.owner.keys) {
		return Key{}, false
	}
	return Key{owner: schema.owner, index: uint32(index)}, true
}

// KeyForModuleInitGeneration returns the one Suspension lifecycle key for an
// exact Link module-init generation site. It does not make module init an
// operation suspension or introduce another continuation owner.
func (schema Schema) KeyForModuleInitGeneration(generation linkmodule.ModuleInitGeneration) (Key, bool) {
	if !schema.Valid() {
		return Key{}, false
	}
	id, ok := schema.owner.source.Module().Generations().ID(generation)
	if !ok {
		return Key{}, false
	}
	index, found := schema.owner.keyByID[id]
	if !found || uint64(index) >= uint64(len(schema.owner.keys)) {
		return Key{}, false
	}
	support := schema.owner.keys[index]
	if support.generation.kind != generationSourceModuleInit || support.generation.moduleInit != generation {
		return Key{}, false
	}
	return Key{owner: schema.owner, index: index}, true
}

func (schema Schema) SubjectCount(key Key) int {
	owner, support, ok := key.support()
	if !ok || owner != schema.owner {
		return 0
	}
	return len(support.retained)
}
func (schema Schema) SubjectAt(key Key, index int) (RetentionSubject, bool) {
	owner, support, ok := key.support()
	if !ok || owner != schema.owner || index < 0 || index >= len(support.retained) {
		return RetentionSubject{}, false
	}
	atom := support.retained[index]
	return RetentionSubject{owner: schema.owner, kind: atom.kind, value: atom.value}, true
}

type Retention struct {
	subject RetentionSubject
	roles   uint8
}

func (retention Retention) Kind() RetainedKind          { return retention.subject.kind }
func (retention Retention) Subject() linkboundary.Value { return retention.subject.value }
func (retention Retention) HasClass(class RetentionClass) bool {
	return retention.roles&classBit(class) != 0
}

// lifecycle keeps materialization age with exactly the lifecycle and
// retention alternatives it qualifies.  A separate role set would correlate
// neither liveness nor retention with age and would silently admit a second
// suspension materialization path.
type lifecycle struct {
	role     materialization.Role
	live     bool
	consumed bool
	retained []Retention
}

// Value is one family relation and never carries a Key. Top is interpreted
// against the exact key support at an observation/rank boundary.
type Value struct {
	owner      *schema
	top        bool
	lifecycles []lifecycle
}

func (schema Schema) Default() (Value, bool) { return schema.Bottom() }
func (schema Schema) Bottom() (Value, bool) {
	if !schema.Valid() {
		return Value{}, false
	}
	return Value{owner: schema.owner}, true
}
func (schema Schema) Top() (Value, bool) {
	if !schema.Valid() {
		return Value{}, false
	}
	return Value{owner: schema.owner, top: true}, true
}
func (schema Schema) Live(key Key, role materialization.Role) (Value, bool) {
	owner, _, ok := key.support()
	if !ok || owner != schema.owner || !role.Valid() {
		return Value{}, false
	}
	return Value{owner: schema.owner, lifecycles: []lifecycle{{role: role, live: true}}}, true
}
func (schema Schema) Consumed(key Key, role materialization.Role) (Value, bool) {
	owner, _, ok := key.support()
	if !ok || owner != schema.owner || !role.Valid() {
		return Value{}, false
	}
	return Value{owner: schema.owner, lifecycles: []lifecycle{{role: role, consumed: true}}}, true
}

func (schema Schema) Retain(key Key, value Value, subject RetentionSubject, class RetentionClass) (Value, bool) {
	owner, support, keyOK := key.support()
	wanted := retainedAtom{kind: subject.kind, value: subject.value}
	if !keyOK || owner != schema.owner || !schema.owns(value) || subject.owner != schema.owner || !validClass(class) || !schema.owner.containsAtom(support.retained, wanted) {
		return Value{}, false
	}
	if value.top {
		return schema.Top()
	}
	if len(value.lifecycles) == 0 {
		return value, true
	}
	lifecycles := cloneLifecycles(value.lifecycles)
	for index := range lifecycles {
		lifecycles[index].retained = append(lifecycles[index].retained, Retention{subject: subject, roles: classBit(class)})
	}
	return schema.normalize(lifecycles), true
}

// Materialize advances exactly the selected continuation site's Recent
// lifecycle to Summary.  RetentionClass remains an independent semantic
// property inside that same lifecycle alternative.
func (schema Schema) Materialize(value Value, key Key) (Value, bool) {
	owner, _, keyOK := key.support()
	if !keyOK || owner != schema.owner || !schema.owns(value) || !schema.Admits(key, value) {
		return Value{}, false
	}
	if value.top || len(value.lifecycles) == 0 {
		return value, true
	}
	lifecycles := cloneLifecycles(value.lifecycles)
	changed := false
	for index := range lifecycles {
		if role, advanced := materialization.RecentToSummary(lifecycles[index].role); advanced {
			lifecycles[index].role = role
			changed = true
		}
	}
	if !changed {
		return value, true
	}
	return schema.normalize(lifecycles), true
}

func (value Value) Valid() bool { return value.owner != nil }
func (value Value) IsBottom() bool {
	return value.Valid() && !value.top && len(value.lifecycles) == 0
}
func (value Value) IsTop() bool { return value.Valid() && value.top }
func (value Value) MayBeLive() bool {
	if !value.Valid() || value.top {
		return value.Valid() && value.top
	}
	for _, lifecycle := range value.lifecycles {
		if lifecycle.live {
			return true
		}
	}
	return false
}
func (value Value) MayBeConsumed() bool {
	if !value.Valid() || value.top {
		return value.Valid() && value.top
	}
	for _, lifecycle := range value.lifecycles {
		if lifecycle.consumed {
			return true
		}
	}
	return false
}
func (value Value) RetentionCount() int {
	if !value.Valid() || value.top {
		return 0
	}
	count := 0
	for _, lifecycle := range value.lifecycles {
		count += len(lifecycle.retained)
	}
	return count
}
func (value Value) RetentionAt(index int) (Retention, bool) {
	if !value.Valid() || value.top || index < 0 {
		return Retention{}, false
	}
	for _, lifecycle := range value.lifecycles {
		if index < len(lifecycle.retained) {
			return lifecycle.retained[index], true
		}
		index -= len(lifecycle.retained)
	}
	return Retention{}, false
}
func (value Value) LifecycleCount() int {
	if !value.Valid() || value.top {
		return 0
	}
	return len(value.lifecycles)
}
func (value Value) LifecycleAt(index int) (materialization.Role, bool, bool, []Retention, bool) {
	if !value.Valid() || value.top || index < 0 || index >= len(value.lifecycles) {
		return materialization.Invalid, false, false, nil, false
	}
	lifecycle := value.lifecycles[index]
	return lifecycle.role, lifecycle.live, lifecycle.consumed, append([]Retention(nil), lifecycle.retained...), true
}
func (value Value) Classes() (RetentionClasses, bool) {
	if !value.Valid() {
		return RetentionClassesNone, false
	}
	if value.top {
		return RetentionClassesMixed, true
	}
	roles := uint8(0)
	for _, lifecycle := range value.lifecycles {
		for _, retention := range lifecycle.retained {
			roles |= retention.roles
		}
	}
	switch roles {
	case 0:
		return RetentionClassesNone, true
	case classPrivate:
		return RetentionClassesPrivate, true
	case classShared:
		return RetentionClassesShared, true
	default:
		return RetentionClassesMixed, true
	}
}

const (
	classPrivate uint8 = 1 << iota
	classShared
)

func classBit(class RetentionClass) uint8 {
	switch class {
	case RetentionPrivate:
		return classPrivate
	case RetentionShared:
		return classShared
	default:
		return 0
	}
}
func validKind(kind RetainedKind) bool { return kind == RetainedValue || kind == RetainedCell }
func validClass(class RetentionClass) bool {
	return class == RetentionPrivate || class == RetentionShared
}
func (schema Schema) owns(value Value) bool { return schema.Valid() && value.owner == schema.owner }

func occurrenceSubjects(source *link.Link, application linkproject.Application) ([]retainedAtom, bool) {
	if source == nil {
		return nil, false
	}
	if _, ok := source.Project().Applications().Index(application); !ok {
		return nil, false
	}
	// Continuation retention is a Program-owned source traversal. The former
	// Link suspension port did not add another retained subject.
	out := make([]retainedAtom, 0)
	if len(out) == 0 {
		return nil, true
	}
	sort.Slice(out, func(left, right int) bool {
		if out[left].kind != out[right].kind {
			return out[left].kind < out[right].kind
		}
		if source.Boundary() == nil {
			return false
		}
		order, ok := source.Boundary().Values().Compare(out[left].value, out[right].value)
		return ok && order < 0
	})
	end := 1
	for index := 1; index < len(out); index++ {
		if out[index] != out[end-1] {
			out[end] = out[index]
			end++
		}
	}
	return out[:end], true
}

func operationGenerationID(source *link.Link, application linkproject.Application, operation target.Operation, suspension uint32) (keyspace.ContentID, bool) {
	if source == nil {
		return keyspace.ContentID{}, false
	}
	contract, contractOK := source.Boundary().Target()
	if !contractOK || contract == nil || !source.Boundary().ApplicationOperationAvailable(contract, application, operation) {
		return keyspace.ContentID{}, false
	}
	project := source.Project()
	if project == nil {
		return keyspace.ContentID{}, false
	}
	applicationID, ok := project.ApplicationID(application)
	if !ok {
		return keyspace.ContentID{}, false
	}
	var image [32 + 32 + 4 + 4]byte
	linkID := source.ContentID()
	copy(image[:], linkID[:])
	copy(image[32:], applicationID[:])
	image[64] = byte(operation >> 24)
	image[65] = byte(operation >> 16)
	image[66] = byte(operation >> 8)
	image[67] = byte(operation)
	image[68] = byte(suspension >> 24)
	image[69] = byte(suspension >> 16)
	image[70] = byte(suspension >> 8)
	image[71] = byte(suspension)
	return keyspace.ContentID(sha256.Sum256(image[:])), true
}

func (owner *schema) containsAtom(atoms []retainedAtom, wanted retainedAtom) bool {
	if !validKind(wanted.kind) {
		return false
	}
	index := sort.Search(len(atoms), func(index int) bool {
		current := atoms[index]
		if current.kind != wanted.kind {
			return current.kind > wanted.kind
		}
		order, ok := owner.compareValues(current.value, wanted.value)
		return ok && order >= 0
	})
	return index < len(atoms) && atoms[index] == wanted
}

func (schema Schema) normalize(lifecycles []lifecycle) Value {
	if len(lifecycles) == 0 {
		return Value{owner: schema.owner}
	}
	items := make([]lifecycle, 0, len(lifecycles))
	for _, state := range lifecycles {
		if !state.role.Valid() {
			continue
		}
		retained := normalizeRetentions(schema, state.retained)
		if retained == nil && len(state.retained) != 0 {
			continue
		}
		if !state.live && !state.consumed && len(retained) == 0 {
			continue
		}
		items = append(items, lifecycle{role: state.role, live: state.live, consumed: state.consumed, retained: retained})
	}
	sort.Slice(items, func(left, right int) bool { return items[left].role < items[right].role })
	out := items[:0]
	for _, item := range items {
		if len(out) != 0 && out[len(out)-1].role == item.role {
			out[len(out)-1].live = out[len(out)-1].live || item.live
			out[len(out)-1].consumed = out[len(out)-1].consumed || item.consumed
			out[len(out)-1].retained = mergeRetentions(schema, out[len(out)-1].retained, item.retained)
			continue
		}
		out = append(out, item)
	}
	return Value{owner: schema.owner, lifecycles: out}
}

func normalizeRetentions(schema Schema, retained []Retention) []Retention {
	if len(retained) == 0 {
		return nil
	}
	items := make([]Retention, 0, len(retained))
	for _, item := range retained {
		if item.subject.owner == schema.owner && validKind(item.subject.kind) && item.roles != 0 && item.roles&^(classPrivate|classShared) == 0 {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(left, right int) bool {
		order, ok := schema.owner.compareSubjects(items[left].subject, items[right].subject)
		return ok && order < 0
	})
	out := items[:0]
	for _, item := range items {
		if len(out) != 0 && out[len(out)-1].subject == item.subject {
			out[len(out)-1].roles |= item.roles
			continue
		}
		out = append(out, item)
	}
	return out
}

func mergeRetentions(schema Schema, left, right []Retention) []Retention {
	return normalizeRetentions(schema, append(append([]Retention(nil), left...), right...))
}

func cloneLifecycles(values []lifecycle) []lifecycle {
	cloned := make([]lifecycle, len(values))
	for index, value := range values {
		cloned[index] = lifecycle{role: value.role, live: value.live, consumed: value.consumed, retained: append([]Retention(nil), value.retained...)}
	}
	return cloned
}
