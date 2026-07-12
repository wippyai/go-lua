// Package primitive defines the canonical declarative microprogram registry.
// It is backend-neutral: transactions describe state effects, while a circuit
// may only retain an IntrinsicCall reference to the one native authority.
package primitive

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/semantic/transaction"
)

const Schema = "go-lua.semantic.primitive/v1"

var (
	ErrInvalid = errors.New("semantic primitive: invalid")
	ErrSealed  = errors.New("semantic primitive: registry builder is sealed")
)

// IntrinsicCall is the complete circuit form of a native intrinsic: a stable
// descriptor reference and canonical payload. It deliberately contains no
// executable callback.
type IntrinsicCall struct {
	ID            string
	SchemaVersion uint16
	Payload       []byte
}

func NewIntrinsicCall(id string, schemaVersion uint16, payload []byte) (IntrinsicCall, error) {
	if err := validateID("intrinsic call", id); err != nil {
		return IntrinsicCall{}, err
	}
	if schemaVersion == 0 {
		return IntrinsicCall{}, invalid("intrinsic call "+id, errors.New("schema version must be non-zero"))
	}
	return IntrinsicCall{ID: id, SchemaVersion: schemaVersion, Payload: append([]byte(nil), payload...)}, nil
}

func (c IntrinsicCall) copy() IntrinsicCall {
	c.Payload = append([]byte(nil), c.Payload...)
	return c
}

// StepKind is a closed microprogram instruction family.
type StepKind uint8

const (
	StepTransaction StepKind = iota + 1
	StepIntrinsicCall
)

// Step is either one immutable transaction or one intrinsic reference.
type Step struct {
	kind        StepKind
	transaction transaction.FrozenTransaction
	intrinsic   IntrinsicCall
}

func TransactionStep(value transaction.FrozenTransaction) Step {
	return Step{kind: StepTransaction, transaction: value}
}

func IntrinsicStep(call IntrinsicCall) Step {
	return Step{kind: StepIntrinsicCall, intrinsic: call.copy()}
}

func (s Step) Kind() StepKind { return s.kind }

func (s Step) Transaction() (transaction.FrozenTransaction, bool) {
	return s.transaction, s.kind == StepTransaction
}

func (s Step) IntrinsicCall() (IntrinsicCall, bool) {
	return s.intrinsic.copy(), s.kind == StepIntrinsicCall
}

func (s Step) copy() Step {
	s.intrinsic = s.intrinsic.copy()
	return s
}

// ProgramDescriptor is one ordered declarative microprogram.
type ProgramDescriptor struct {
	ID            string
	SchemaVersion uint16
	Steps         []Step
}

func (p ProgramDescriptor) copy() ProgramDescriptor {
	steps := p.Steps
	p.Steps = make([]Step, len(steps))
	for index, step := range steps {
		p.Steps[index] = step.copy()
	}
	return p
}

// IntrinsicDescriptor declares one exceptional native operation.
type IntrinsicDescriptor struct {
	ID            string
	SchemaVersion uint16
}

// NativeInput and NativeOutput are detached canonical payloads. CallPayload is
// supplied by Registry from the sealed IntrinsicCall, never reconstructed by a
// backend. They contain no engine State, Universe, arena, provider, or circuit
// callback.
type NativeInput struct {
	CallPayload []byte
	Payload     []byte
}
type NativeOutput struct{ Payload []byte }

// NativeFunc is the one executable authority for an intrinsic. Concrete
// execution invokes it directly; circuit specialization resolves the same
// IntrinsicCall through the same sealed Registry.
type NativeFunc func(NativeInput) (NativeOutput, error)

// NativeBinding binds one descriptor to one stable implementation identity.
// Private fields require construction through NewNativeBinding.
type NativeBinding struct {
	id               string
	schemaVersion    uint16
	implementationID string
	invoke           NativeFunc
}

func NewNativeBinding(id string, schemaVersion uint16, implementationID string, invoke NativeFunc) (NativeBinding, error) {
	if err := validateID("native binding", id); err != nil {
		return NativeBinding{}, err
	}
	if schemaVersion == 0 {
		return NativeBinding{}, invalid("native binding "+id, errors.New("schema version must be non-zero"))
	}
	if err := validateID("implementation", implementationID); err != nil {
		return NativeBinding{}, err
	}
	if invoke == nil {
		return NativeBinding{}, invalid("native binding "+id, errors.New("implementation must not be nil"))
	}
	return NativeBinding{id: id, schemaVersion: schemaVersion, implementationID: implementationID, invoke: invoke}, nil
}

// CoverageRole is one required leaf-coverage dimension.
type CoverageRole uint8

const (
	CoveragePrimitive CoverageRole = iota + 1
	CoverageEffect
	CoverageOutput
	CoverageObserver
)

func (r CoverageRole) valid() bool { return r >= CoveragePrimitive && r <= CoverageObserver }

var requiredCoverageRoles = [...]CoverageRole{
	CoveragePrimitive, CoverageEffect, CoverageOutput, CoverageObserver,
}

// Leaf is one exact verifier-derived transaction slot. Capabilities remain the
// authorization surface; coverage is finer-grained because one capability may
// span several independently meaningful leaves.
type Leaf struct {
	ID           string
	CapabilityID string
	SlotID       string
	Kind         transaction.SlotKind
}

// Coverage binds one program and one exact semantic leaf to one coverage
// dimension. Seal rejects missing, duplicate and extra rows.
type Coverage struct {
	ProgramID string
	LeafID    string
	Role      CoverageRole
}

func leafForSlot(slot transaction.Slot) Leaf {
	return Leaf{
		ID:           fmt.Sprintf("%d:%s:%s", slot.Kind, slot.Capability, slot.ID),
		CapabilityID: slot.Capability,
		SlotID:       slot.ID,
		Kind:         slot.Kind,
	}
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
