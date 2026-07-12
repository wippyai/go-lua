// Package concrete executes sealed semantic primitive programs against typed
// backend cells. It owns no primitive meaning: transaction opcodes dispatch to
// one sealed handler, and intrinsic calls delegate to primitive.Registry.
package concrete

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/wippyai/go-lua/analysis/semantic/primitive"
	"github.com/wippyai/go-lua/analysis/semantic/transaction"
)

const Schema = "go-lua.engine.concrete/v1"

var (
	ErrInvalid = errors.New("concrete backend: invalid")
	ErrSealed  = errors.New("concrete backend: builder is sealed")
)

// Cell is one backend-local typed bank cell. clone must deeply detach values
// that carry mutable storage; it is used at both scratch and publication seams.
type Cell[T any] struct {
	value T
	clone func(T) T
}

func NewCell[T any](value T, clone func(T) T) (*Cell[T], error) {
	if clone == nil {
		return nil, invalid("cell", errors.New("clone function must not be nil"))
	}
	return &Cell[T]{value: clone(value), clone: clone}, nil
}

func (c *Cell[T]) Load() T {
	if c == nil || c.clone == nil {
		var zero T
		return zero
	}
	return c.clone(c.value)
}

// Handler is one registered implementation of a transaction opcode.
type Handler[T any] func(context.Context, T, []byte) (T, error)

type slotKey struct {
	kind       transaction.SlotKind
	capability string
	id         string
}

func keyForSlot(slot transaction.Slot) slotKey {
	return slotKey{kind: slot.Kind, capability: slot.Capability, id: slot.ID}
}

func (k slotKey) String() string { return fmt.Sprintf("%d:%s:%s", k.kind, k.capability, k.id) }

type cellBinding struct {
	key              slotKey
	typeToken        reflect.Type
	implementationID string
	snapshot         func() any
	publish          func(any) error
	clone            func(any) (any, error)
}

type opcodeBinding struct {
	id               string
	implementationID string
	typeToken        reflect.Type
	allowed          map[transaction.Capability]struct{}
	apply            func(context.Context, any, []byte) (any, error)
}

type Builder struct {
	primitives primitive.Registry
	cells      map[slotKey]cellBinding
	opcodes    map[string]opcodeBinding
	sealed     bool
}

func NewBuilder(primitives primitive.Registry) *Builder {
	return &Builder{primitives: primitives, cells: make(map[slotKey]cellBinding), opcodes: make(map[string]opcodeBinding)}
}

// BindCell binds an exact public transaction slot to one typed backend cell.
func BindCell[T any](builder *Builder, slot transaction.Slot, implementationID string, cell *Cell[T]) error {
	if err := builder.mutable(); err != nil {
		return err
	}
	if err := validateID("cell implementation", implementationID); err != nil {
		return err
	}
	if cell == nil || cell.clone == nil {
		return invalid("cell binding", errors.New("nil or unverified cell"))
	}
	key := keyForSlot(slot)
	if key.capability == "" || key.id == "" || key.kind == 0 {
		return invalid("cell binding", errors.New("invalid slot descriptor"))
	}
	if _, duplicate := builder.cells[key]; duplicate {
		return invalid("cell binding", fmt.Errorf("duplicate slot %q", key))
	}
	typeToken := reflect.TypeOf((*T)(nil)).Elem()
	builder.cells[key] = cellBinding{
		key: key, typeToken: typeToken, implementationID: implementationID,
		snapshot: func() any { return cell.clone(cell.value) },
		clone: func(value any) (any, error) {
			typed, ok := value.(T)
			if !ok {
				return nil, invalid("cell clone", fmt.Errorf("slot %q received %T, want %s", key, value, typeToken))
			}
			return cell.clone(typed), nil
		},
		publish: func(value any) error {
			typed, ok := value.(T)
			if !ok {
				return invalid("cell publish", fmt.Errorf("slot %q received %T, want %s", key, value, typeToken))
			}
			cell.value = cell.clone(typed)
			return nil
		},
	}
	return nil
}

// RegisterOpcode binds an opcode once, including its exact allowed capability
// kinds. The implementation receives only a typed scratch value and payload.
func RegisterOpcode[T any](builder *Builder, opcode, implementationID string, allowed []transaction.Capability, handler Handler[T]) error {
	if err := builder.mutable(); err != nil {
		return err
	}
	if err := validateID("opcode", opcode); err != nil {
		return err
	}
	if err := validateID("opcode implementation", implementationID); err != nil {
		return err
	}
	if handler == nil {
		return invalid("opcode "+opcode, errors.New("handler must not be nil"))
	}
	if _, duplicate := builder.opcodes[opcode]; duplicate {
		return invalid("opcode", fmt.Errorf("duplicate id %q", opcode))
	}
	if len(allowed) == 0 {
		return invalid("opcode "+opcode, errors.New("allowed capabilities must not be empty"))
	}
	allowedSet := make(map[transaction.Capability]struct{}, len(allowed))
	for _, capability := range allowed {
		if capability.ID == "" || capability.Kind == 0 {
			return invalid("opcode "+opcode, errors.New("invalid allowed capability"))
		}
		if _, duplicate := allowedSet[capability]; duplicate {
			return invalid("opcode "+opcode, fmt.Errorf("duplicate allowed capability %q", capability.ID))
		}
		allowedSet[capability] = struct{}{}
	}
	typeToken := reflect.TypeOf((*T)(nil)).Elem()
	builder.opcodes[opcode] = opcodeBinding{
		id: opcode, implementationID: implementationID, typeToken: typeToken, allowed: allowedSet,
		apply: func(ctx context.Context, current any, payload []byte) (any, error) {
			typed, ok := current.(T)
			if !ok {
				return nil, invalid("opcode "+opcode, fmt.Errorf("received %T, want %s", current, typeToken))
			}
			return handler(ctx, typed, append([]byte(nil), payload...))
		},
	}
	return nil
}

// Backend is an exact sealed binding of a primitive registry to typed cells
// and opcode implementations.
type Backend struct {
	primitives primitive.Registry
	cells      map[slotKey]cellBinding
	opcodes    map[string]opcodeBinding
	cellOrder  []slotKey
	canonical  []byte
	digest     [sha256.Size]byte
	mu         sync.Mutex
}

func (b *Builder) Seal() (*Backend, error) {
	if err := b.mutable(); err != nil {
		return nil, err
	}
	b.sealed = true
	requiredCells := make(map[slotKey]transaction.Slot)
	requiredOpcodes := make(map[string]map[slotKey]struct{})
	for _, program := range b.primitives.Programs() {
		for _, step := range program.Steps {
			frozen, ok := step.Transaction()
			if !ok {
				continue
			}
			for _, slot := range frozen.Slots() {
				requiredCells[keyForSlot(slot)] = slot
			}
			for _, overlay := range frozen.Overlays() {
				for _, operation := range overlay.Operations() {
					slot, err := frozen.Target(operation)
					if err != nil {
						return nil, invalid("seal", err)
					}
					if requiredOpcodes[operation.Opcode()] == nil {
						requiredOpcodes[operation.Opcode()] = make(map[slotKey]struct{})
					}
					requiredOpcodes[operation.Opcode()][keyForSlot(slot)] = struct{}{}
				}
			}
		}
	}
	for key, slot := range requiredCells {
		binding, ok := b.cells[key]
		if !ok {
			return nil, invalid("seal", fmt.Errorf("slot %q has no backend cell", key))
		}
		_ = slot
		if binding.snapshot == nil || binding.publish == nil || binding.clone == nil {
			return nil, invalid("seal", fmt.Errorf("slot %q has incomplete backend cell", key))
		}
	}
	for key := range b.cells {
		if _, ok := requiredCells[key]; !ok {
			return nil, invalid("seal", fmt.Errorf("backend cell %q has no primitive slot", key))
		}
	}
	for opcode, targets := range requiredOpcodes {
		handler, ok := b.opcodes[opcode]
		if !ok {
			return nil, invalid("seal", fmt.Errorf("opcode %q has no handler", opcode))
		}
		for target := range targets {
			cell := b.cells[target]
			if cell.typeToken != handler.typeToken {
				return nil, invalid("seal", fmt.Errorf("opcode %q type %s does not match slot %q type %s", opcode, handler.typeToken, target, cell.typeToken))
			}
			capability := transaction.Capability{ID: target.capability, Kind: target.kind}
			if _, ok := handler.allowed[capability]; !ok {
				return nil, invalid("seal", fmt.Errorf("opcode %q is not authorized for capability %q kind %d", opcode, target.capability, target.kind))
			}
		}
	}
	for opcode := range b.opcodes {
		if _, ok := requiredOpcodes[opcode]; !ok {
			return nil, invalid("seal", fmt.Errorf("opcode handler %q has no primitive operation", opcode))
		}
	}
	order := make([]slotKey, 0, len(requiredCells))
	for key := range requiredCells {
		order = append(order, key)
	}
	sort.Slice(order, func(left, right int) bool { return order[left].String() < order[right].String() })
	backend := &Backend{
		primitives: b.primitives, cells: cloneCellBindings(b.cells), opcodes: cloneOpcodeBindings(b.opcodes), cellOrder: order,
	}
	canonical, err := encodeCanonical(backend)
	if err != nil {
		return nil, err
	}
	backend.canonical = canonical
	backend.digest = sha256.Sum256(canonical)
	return backend, nil
}

type ExecutionInput struct {
	Outcome          transaction.Outcome
	IntrinsicPayload []byte
}

type ExecutionResult struct {
	IntrinsicOutputs [][]byte
}

// Execute runs an entire primitive program transactionally. Scratch overlay
// commits are only logical until every step succeeds and ctx remains live;
// publication then updates every changed cell while holding the backend lock.
func (b *Backend) Execute(ctx context.Context, programID string, input ExecutionInput) (ExecutionResult, error) {
	if b == nil {
		return ExecutionResult{}, invalid("execute", errors.New("nil backend"))
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return ExecutionResult{}, err
	}
	program, ok := b.primitives.Program(programID)
	if !ok {
		return ExecutionResult{}, invalid("execute", fmt.Errorf("unknown program %q", programID))
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	working := make(map[slotKey]any, len(b.cells))
	for _, key := range b.cellOrder {
		working[key] = b.cells[key].snapshot()
	}
	result := ExecutionResult{}
	for _, step := range program.Steps {
		if err := ctx.Err(); err != nil {
			return ExecutionResult{}, err
		}
		if frozen, ok := step.Transaction(); ok {
			if err := b.executeTransaction(ctx, frozen, input.Outcome, working); err != nil {
				return ExecutionResult{}, err
			}
			continue
		}
		call, ok := step.IntrinsicCall()
		if !ok {
			return ExecutionResult{}, invalid("execute", errors.New("unknown primitive step"))
		}
		output, err := b.primitives.InvokeIntrinsic(call, primitive.NativeInput{Payload: append([]byte(nil), input.IntrinsicPayload...)})
		if err != nil {
			return ExecutionResult{}, err
		}
		if err := ctx.Err(); err != nil {
			return ExecutionResult{}, err
		}
		result.IntrinsicOutputs = append(result.IntrinsicOutputs, append([]byte(nil), output.Payload...))
	}
	if err := ctx.Err(); err != nil {
		return ExecutionResult{}, err
	}
	// All conversions are verified before the first publication, making a
	// type error unable to produce a partial commit.
	publish := make(map[slotKey]any, len(working))
	for _, key := range b.cellOrder {
		value, err := b.cells[key].clone(working[key])
		if err != nil {
			return ExecutionResult{}, err
		}
		publish[key] = value
	}
	for _, key := range b.cellOrder {
		if err := b.cells[key].publish(publish[key]); err != nil {
			// Seal and preflight make this unreachable without memory corruption.
			return ExecutionResult{}, err
		}
	}
	return cloneExecutionResult(result), nil
}

func (b *Backend) executeTransaction(ctx context.Context, frozen transaction.FrozenTransaction, outcome transaction.Outcome, working map[slotKey]any) error {
	decisions, err := frozen.Decisions(outcome)
	if err != nil {
		return invalid("execute transaction", err)
	}
	overlays := frozen.Overlays()
	if len(decisions) != len(overlays) {
		return invalid("execute transaction", errors.New("overlay decision cardinality mismatch"))
	}
	for index, overlay := range overlays {
		if err := ctx.Err(); err != nil {
			return err
		}
		local, err := b.cloneScratch(working)
		if err != nil {
			return err
		}
		for _, operation := range overlay.Operations() {
			if err := ctx.Err(); err != nil {
				return err
			}
			slot, err := frozen.Target(operation)
			if err != nil {
				return invalid("execute operation", err)
			}
			key := keyForSlot(slot)
			handler, ok := b.opcodes[operation.Opcode()]
			if !ok {
				return invalid("execute operation", fmt.Errorf("opcode %q is not sealed", operation.Opcode()))
			}
			next, err := handler.apply(ctx, local[key], operation.Payload())
			if err != nil {
				return err
			}
			local[key] = next
		}
		if decisions[index].OverlayID != overlay.ID() {
			return invalid("execute transaction", errors.New("overlay decision order mismatch"))
		}
		if decisions[index].Disposition == transaction.Commit {
			working = replaceScratch(working, local)
		}
	}
	return nil
}

func (b *Backend) cloneScratch(input map[slotKey]any) (map[slotKey]any, error) {
	out := make(map[slotKey]any, len(input))
	for _, key := range b.cellOrder {
		value, err := b.cells[key].clone(input[key])
		if err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, nil
}

func replaceScratch(dst, src map[slotKey]any) map[slotKey]any {
	for key := range dst {
		dst[key] = src[key]
	}
	return dst
}

func cloneExecutionResult(input ExecutionResult) ExecutionResult {
	out := ExecutionResult{IntrinsicOutputs: make([][]byte, len(input.IntrinsicOutputs))}
	for index, value := range input.IntrinsicOutputs {
		out.IntrinsicOutputs[index] = append([]byte(nil), value...)
	}
	return out
}

func cloneCellBindings(input map[slotKey]cellBinding) map[slotKey]cellBinding {
	out := make(map[slotKey]cellBinding, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneOpcodeBindings(input map[string]opcodeBinding) map[string]opcodeBinding {
	out := make(map[string]opcodeBinding, len(input))
	for key, value := range input {
		value.allowed = cloneCapabilities(value.allowed)
		out[key] = value
	}
	return out
}

func cloneCapabilities(input map[transaction.Capability]struct{}) map[transaction.Capability]struct{} {
	out := make(map[transaction.Capability]struct{}, len(input))
	for capability := range input {
		out[capability] = struct{}{}
	}
	return out
}

func (b *Builder) mutable() error {
	if b == nil || b.cells == nil || b.opcodes == nil {
		return invalid("builder", errors.New("nil or zero builder"))
	}
	if b.sealed {
		return ErrSealed
	}
	return nil
}

func (b *Backend) CanonicalBytes() []byte    { return append([]byte(nil), b.canonical...) }
func (b *Backend) Digest() [sha256.Size]byte { return b.digest }
func (b *Backend) Equal(other *Backend) bool {
	return b != nil && other != nil && bytes.Equal(b.canonical, other.canonical)
}

func validateID(kind, id string) error {
	if id == "" || strings.TrimSpace(id) != id {
		return invalid(kind, fmt.Errorf("invalid id %q", id))
	}
	for index, character := range []byte(id) {
		if character >= 'a' && character <= 'z' || index > 0 && character >= '0' && character <= '9' || index > 0 && index < len(id)-1 && (character == '-' || character == '.') {
			continue
		}
		return invalid(kind, fmt.Errorf("invalid id %q", id))
	}
	return nil
}

func invalid(part string, err error) error {
	return fmt.Errorf("%w: %s: %v", ErrInvalid, part, err)
}
