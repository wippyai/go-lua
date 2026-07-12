// Package transaction defines immutable, capability-scoped semantic state
// transactions. It contains no concrete engine state and no execution backend.
package transaction

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const Schema = "go-lua.semantic.transaction/v1"

var (
	ErrInvalid = errors.New("semantic transaction: invalid")
	ErrSealed  = errors.New("semantic transaction: builder is sealed")
)

// SlotKind identifies a typed storage bank without exposing its backend-local
// kind/index reference.
type SlotKind uint8

const (
	SlotLane SlotKind = iota + 1
	SlotAxis
	SlotResource
	SlotExtension
)

func (k SlotKind) valid() bool {
	return k >= SlotLane && k <= SlotExtension
}

// Capability is a stable permission to bind slots of one storage kind.
// Reducer closure may add capabilities, but may not change a seed's kind.
type Capability struct {
	ID   string
	Kind SlotKind
}

// ClosureHook computes the complete reducer-expanded capability closure of
// seed. The builder validates seed preservation, conflicting kinds, IDs and
// deterministic normalization of the returned set.
type ClosureHook func(seed []Capability) ([]Capability, error)

// Outcome is a terminal semantic route selected by an executor.
type Outcome uint8

const (
	OutcomeNormal Outcome = iota + 1
	OutcomeRaised
	OutcomeSuspended
	OutcomeNonreturning
)

func (o Outcome) valid() bool {
	return o >= OutcomeNormal && o <= OutcomeNonreturning
}

// OverlayDisposition states whether one outcome-local overlay survives the
// selected terminal route.
type OverlayDisposition uint8

const (
	Rollback OverlayDisposition = iota + 1
	Commit
)

func (d OverlayDisposition) valid() bool { return d == Rollback || d == Commit }

// OutcomePolicy explicitly covers every terminal route. Its zero value is
// invalid so omission can never accidentally mean commit or rollback.
type OutcomePolicy struct {
	normal       OverlayDisposition
	raised       OverlayDisposition
	suspended    OverlayDisposition
	nonreturning OverlayDisposition
	explicit     bool
}

func NewOutcomePolicy(normal, raised, suspended, nonreturning OverlayDisposition) (OutcomePolicy, error) {
	values := []OverlayDisposition{normal, raised, suspended, nonreturning}
	for index, value := range values {
		if !value.valid() {
			return OutcomePolicy{}, invalid("outcome policy", fmt.Errorf("route %d has invalid disposition %d", index, value))
		}
	}
	return OutcomePolicy{
		normal:       normal,
		raised:       raised,
		suspended:    suspended,
		nonreturning: nonreturning,
		explicit:     true,
	}, nil
}

// For returns the explicit disposition for outcome.
func (p OutcomePolicy) For(outcome Outcome) (OverlayDisposition, error) {
	if !p.explicit {
		return 0, invalid("outcome policy", errors.New("all routes must be explicit"))
	}
	switch outcome {
	case OutcomeNormal:
		return p.normal, nil
	case OutcomeRaised:
		return p.raised, nil
	case OutcomeSuspended:
		return p.suspended, nil
	case OutcomeNonreturning:
		return p.nonreturning, nil
	default:
		return 0, invalid("outcome policy", fmt.Errorf("invalid outcome %d", outcome))
	}
}

// Handle is a typed transaction-local slot capability. Its fields and the
// scope token are private: callers can copy a valid handle but cannot forge
// one or transplant one between builders. The zero value is invalid.
type Handle[T any] struct {
	token *scopeToken
	ref   slotRef
}

// The token must have non-zero size: Go may coalesce pointers to distinct
// zero-sized allocations, which would collapse otherwise independent scopes.
type scopeToken struct{ marker byte }

// slotRef is the private backend-neutral kind/index model. Indices assigned
// during construction are rebased to canonical slot order by Freeze.
type slotRef struct {
	kind  SlotKind
	index uint32
}

type mutableSlot struct {
	capability string
	id         string
	ref        slotRef
	typeToken  reflect.Type
}

type mutableOperation struct {
	target  slotRef
	opcode  string
	payload []byte
}

type mutableOverlay struct {
	id         string
	policy     OutcomePolicy
	operations []mutableOperation
}

// Builder verifies and freezes exactly one transaction.
type Builder struct {
	token        *scopeToken
	capabilities []Capability
	capability   map[string]Capability
	slots        []mutableSlot
	slotByKey    map[string]slotRef
	overlays     []mutableOverlay
	overlayIDs   map[string]struct{}
	sealed       bool
}

// NewBuilder seals the reducer-expanded capability set. The closure hook is
// invoked exactly once; nil means that the seed is already closed.
func NewBuilder(seed []Capability, closure ClosureHook) (*Builder, error) {
	normalizedSeed, err := normalizeCapabilities(seed)
	if err != nil {
		return nil, err
	}
	expanded := append([]Capability(nil), normalizedSeed...)
	if closure != nil {
		expanded, err = closure(append([]Capability(nil), normalizedSeed...))
		if err != nil {
			return nil, invalid("capability closure", err)
		}
	}
	expanded, err = normalizeCapabilities(expanded)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Capability, len(expanded))
	for _, capability := range expanded {
		byID[capability.ID] = capability
	}
	for _, required := range normalizedSeed {
		got, ok := byID[required.ID]
		if !ok {
			return nil, invalid("capability closure", fmt.Errorf("dropped seed capability %q", required.ID))
		}
		if got.Kind != required.Kind {
			return nil, invalid("capability closure", fmt.Errorf("changed seed capability %q kind from %d to %d", required.ID, required.Kind, got.Kind))
		}
	}
	return &Builder{
		token:        &scopeToken{},
		capabilities: expanded,
		capability:   byID,
		slotByKey:    make(map[string]slotRef),
		overlayIDs:   make(map[string]struct{}),
	}, nil
}

// Bind creates or returns a typed handle for a stable slot ID under one
// expanded capability. The same capability/slot pair may be rebound only with
// the same T by caller convention; execution backends bind concrete slot types.
func Bind[T any](builder *Builder, capabilityID, slotID string) (Handle[T], error) {
	if err := builder.mutable(); err != nil {
		return Handle[T]{}, err
	}
	capability, ok := builder.capability[capabilityID]
	if !ok {
		return Handle[T]{}, invalid("bind", fmt.Errorf("capability %q is outside the verified scope", capabilityID))
	}
	if err := validateID("slot", slotID); err != nil {
		return Handle[T]{}, err
	}
	key := capabilityID + "\x00" + slotID
	typeToken := reflect.TypeOf((*T)(nil)).Elem()
	if ref, exists := builder.slotByKey[key]; exists {
		if builder.slots[ref.index].typeToken != typeToken {
			return Handle[T]{}, invalid("bind", fmt.Errorf("slot %q under capability %q was already bound as %s, cannot rebind as %s", slotID, capabilityID, builder.slots[ref.index].typeToken, typeToken))
		}
		return Handle[T]{token: builder.token, ref: ref}, nil
	}
	ref := slotRef{kind: capability.Kind, index: uint32(len(builder.slots))}
	builder.slotByKey[key] = ref
	builder.slots = append(builder.slots, mutableSlot{capability: capabilityID, id: slotID, ref: ref, typeToken: typeToken})
	return Handle[T]{token: builder.token, ref: ref}, nil
}

// OverlayBuilder is scoped to one ordered overlay in one Builder. Its fields
// cannot be constructed by another package.
type OverlayBuilder struct {
	builder *Builder
	index   int
}

// BeginOverlay appends an outcome-local overlay. Overlay order is semantic.
func (b *Builder) BeginOverlay(id string, policy OutcomePolicy) (*OverlayBuilder, error) {
	if err := b.mutable(); err != nil {
		return nil, err
	}
	if err := validateID("overlay", id); err != nil {
		return nil, err
	}
	if !policy.explicit {
		return nil, invalid("overlay "+id, errors.New("all outcome routes require an explicit policy"))
	}
	if _, duplicate := b.overlayIDs[id]; duplicate {
		return nil, invalid("overlay", fmt.Errorf("duplicate id %q", id))
	}
	b.overlayIDs[id] = struct{}{}
	b.overlays = append(b.overlays, mutableOverlay{id: id, policy: policy})
	return &OverlayBuilder{builder: b, index: len(b.overlays) - 1}, nil
}

// Append adds one operation to an overlay in semantic order.
func Append[T any](overlay *OverlayBuilder, target Handle[T], opcode string, payload []byte) error {
	if overlay == nil || overlay.builder == nil {
		return invalid("append", errors.New("nil overlay"))
	}
	builder := overlay.builder
	if err := builder.mutable(); err != nil {
		return err
	}
	if overlay.index < 0 || overlay.index >= len(builder.overlays) {
		return invalid("append", errors.New("overlay is outside the verified scope"))
	}
	if target.token == nil || target.token != builder.token {
		return invalid("append", errors.New("handle is forged or outside the verified scope"))
	}
	if int(target.ref.index) >= len(builder.slots) || builder.slots[target.ref.index].ref != target.ref {
		return invalid("append", errors.New("handle has an invalid private slot reference"))
	}
	if err := validateID("opcode", opcode); err != nil {
		return err
	}
	builder.overlays[overlay.index].operations = append(builder.overlays[overlay.index].operations, mutableOperation{
		target:  target.ref,
		opcode:  opcode,
		payload: append([]byte(nil), payload...),
	})
	return nil
}

// Slot describes a canonical transaction slot without exposing slotRef.
type Slot struct {
	Capability string
	ID         string
	Kind       SlotKind
}

// Operation is an immutable operation projection. Payload returns detached
// bytes so callers cannot mutate a frozen artifact.
type Operation struct {
	token   *artifactToken
	target  uint32
	opcode  string
	payload []byte
}

func (o Operation) Opcode() string  { return o.opcode }
func (o Operation) Payload() []byte { return append([]byte(nil), o.payload...) }

// Overlay is one immutable outcome-local operation sequence.
type Overlay struct {
	id         string
	policy     OutcomePolicy
	operations []Operation
}

func (o Overlay) ID() string            { return o.id }
func (o Overlay) Policy() OutcomePolicy { return o.policy }
func (o Overlay) Operations() []Operation {
	out := make([]Operation, len(o.operations))
	for index, operation := range o.operations {
		out[index] = operation
		out[index].payload = append([]byte(nil), operation.payload...)
	}
	return out
}

// OverlayDecision preserves overlay definition order for one outcome.
type OverlayDecision struct {
	OverlayID   string
	Disposition OverlayDisposition
}

// FrozenTransaction is a verified immutable transaction artifact.
type FrozenTransaction struct {
	token        *artifactToken
	capabilities []Capability
	slots        []Slot
	overlays     []Overlay
	canonical    []byte
	digest       [sha256.Size]byte
}

// artifactToken prevents an Operation projection from one frozen transaction
// being resolved against another transaction's canonical slot table.
type artifactToken struct{ marker byte }

// Freeze verifies references, rebases private refs into canonical slot order,
// and seals deterministic bytes. A builder may be frozen exactly once.
func (b *Builder) Freeze() (FrozenTransaction, error) {
	if err := b.mutable(); err != nil {
		return FrozenTransaction{}, err
	}
	// Seal before verification so an error cannot leave a partially trusted,
	// mutable builder available for reuse.
	b.sealed = true

	order := make([]int, len(b.slots))
	for index := range order {
		order[index] = index
	}
	sort.Slice(order, func(left, right int) bool {
		a, z := b.slots[order[left]], b.slots[order[right]]
		if a.ref.kind != z.ref.kind {
			return a.ref.kind < z.ref.kind
		}
		if a.capability != z.capability {
			return a.capability < z.capability
		}
		return a.id < z.id
	})
	canonicalIndex := make(map[slotRef]uint32, len(order))
	slots := make([]Slot, len(order))
	for index, oldIndex := range order {
		mutable := b.slots[oldIndex]
		if int(mutable.ref.index) >= len(b.slots) || mutable.ref.kind == 0 {
			return FrozenTransaction{}, invalid("freeze", errors.New("invalid private slot reference"))
		}
		canonicalIndex[mutable.ref] = uint32(index)
		slots[index] = Slot{Capability: mutable.capability, ID: mutable.id, Kind: mutable.ref.kind}
	}

	token := &artifactToken{}
	overlays := make([]Overlay, len(b.overlays))
	for overlayIndex, mutable := range b.overlays {
		overlay := Overlay{id: mutable.id, policy: mutable.policy, operations: make([]Operation, len(mutable.operations))}
		for operationIndex, operation := range mutable.operations {
			target, ok := canonicalIndex[operation.target]
			if !ok {
				return FrozenTransaction{}, invalid("freeze", fmt.Errorf("overlay %q operation %d has unknown slot", mutable.id, operationIndex))
			}
			overlay.operations[operationIndex] = Operation{token: token, target: target, opcode: operation.opcode, payload: append([]byte(nil), operation.payload...)}
		}
		overlays[overlayIndex] = overlay
	}

	frozen := FrozenTransaction{
		token:        token,
		capabilities: append([]Capability(nil), b.capabilities...),
		slots:        slots,
		overlays:     overlays,
	}
	canonical, err := encodeCanonical(frozen)
	if err != nil {
		return FrozenTransaction{}, err
	}
	frozen.canonical = canonical
	frozen.digest = sha256.Sum256(canonical)
	return frozen, nil
}

func (b *Builder) mutable() error {
	if b == nil || b.token == nil {
		return invalid("builder", errors.New("nil or zero builder"))
	}
	if b.sealed {
		return ErrSealed
	}
	return nil
}

func (f FrozenTransaction) Capabilities() []Capability {
	return append([]Capability(nil), f.capabilities...)
}

func (f FrozenTransaction) Slots() []Slot { return append([]Slot(nil), f.slots...) }

func (f FrozenTransaction) Overlays() []Overlay {
	out := make([]Overlay, len(f.overlays))
	for index, overlay := range f.overlays {
		out[index] = overlay
		out[index].operations = overlay.Operations()
	}
	return out
}

func (f FrozenTransaction) CanonicalBytes() []byte    { return append([]byte(nil), f.canonical...) }
func (f FrozenTransaction) Digest() [sha256.Size]byte { return f.digest }

// Decisions returns commit/rollback decisions in overlay order for outcome.
func (f FrozenTransaction) Decisions(outcome Outcome) ([]OverlayDecision, error) {
	if !outcome.valid() {
		return nil, invalid("decisions", fmt.Errorf("invalid outcome %d", outcome))
	}
	out := make([]OverlayDecision, len(f.overlays))
	for index, overlay := range f.overlays {
		disposition, err := overlay.policy.For(outcome)
		if err != nil {
			return nil, err
		}
		out[index] = OverlayDecision{OverlayID: overlay.id, Disposition: disposition}
	}
	return out, nil
}

// Target returns the canonical public slot descriptor for operation.
func (f FrozenTransaction) Target(operation Operation) (Slot, error) {
	if f.token == nil || operation.token == nil || operation.token != f.token {
		return Slot{}, invalid("operation target", errors.New("operation is forged or belongs to another frozen transaction"))
	}
	if int(operation.target) >= len(f.slots) {
		return Slot{}, invalid("operation target", fmt.Errorf("slot index %d is out of range", operation.target))
	}
	return f.slots[operation.target], nil
}

func normalizeCapabilities(input []Capability) ([]Capability, error) {
	out := append([]Capability(nil), input...)
	for _, capability := range out {
		if err := validateID("capability", capability.ID); err != nil {
			return nil, err
		}
		if !capability.Kind.valid() {
			return nil, invalid("capability "+capability.ID, fmt.Errorf("invalid slot kind %d", capability.Kind))
		}
	}
	sort.Slice(out, func(left, right int) bool { return out[left].ID < out[right].ID })
	for index := 1; index < len(out); index++ {
		if out[index-1].ID == out[index].ID {
			if out[index-1].Kind != out[index].Kind {
				return nil, invalid("capability", fmt.Errorf("id %q has conflicting kinds %d and %d", out[index].ID, out[index-1].Kind, out[index].Kind))
			}
			return nil, invalid("capability", fmt.Errorf("duplicate id %q", out[index].ID))
		}
	}
	return out, nil
}

func validateID(kind, id string) error {
	if id == "" || strings.TrimSpace(id) != id {
		return invalid(kind, fmt.Errorf("invalid id %q", id))
	}
	separator := false
	for index := 0; index < len(id); index++ {
		character := id[index]
		if character >= 'a' && character <= 'z' || index > 0 && character >= '0' && character <= '9' {
			separator = false
			continue
		}
		if index > 0 && index < len(id)-1 && !separator && (character == '-' || character == '.') {
			separator = true
			continue
		}
		return invalid(kind, fmt.Errorf("invalid id %q", id))
	}
	return nil
}

func invalid(part string, err error) error {
	return fmt.Errorf("%w: %s: %v", ErrInvalid, part, err)
}
