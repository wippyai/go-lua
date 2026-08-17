package contract

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/internal/framing"
)

// The environment-slot payload format. Binding a slot is the one place in the
// whole surface where a NAME meets a VALUE, and this format is what keeps that
// meeting from becoming name addressing: the slot's name is the member address
// the binding is published at, and the payload carries the value that initially
// occupies it.
//
// A slot holds one of two things, and the two are written as the different things
// they are. Most slots hold a value the contract addresses: `_G` holds the
// environment root itself, `table` holds the aggregate one step from that root,
// `print` holds the callable published at its own address - each written as the
// path it is, so a consumer that walks the binding reaches a value rather than
// resolving a second name through the mutable environment. What that value IS is
// stated by the form that owns it: the boot-root form for an aggregate, the
// callable-signature form for a callable, the denied-entry form for one the host
// refused. The remaining slots hold a constant, which terminates the path and
// therefore has no address; the constant rides the binding, because the binding is
// the only member the environment has at that slot.
//
// The published mutability is the SLOT's policy: whether the initial environment
// lets the slot be rebound. It is deliberately not the bound object's seal - a
// frozen aggregate at a rebindable slot is an ordinary shape, and the boot-root
// form states the object's half.
const (
	environmentSlotDomain  = "analysis/library/contract/environment-slot"
	environmentSlotVersion = 1
)

// SlotBinding is which of the two things a slot binds. A value the contract can
// keep addressing through is not a constant that terminates the path, so a reader
// never has to infer which it holds from an empty field.
type SlotBinding uint8

const (
	SlotBindingInvalid SlotBinding = iota
	// SlotBindingValue binds the slot to the value at one export path. The root
	// path is admissible and meaningful: a slot may bind the environment itself.
	SlotBindingValue
	// SlotBindingConstant binds the slot to one value of the closed literal
	// domain of the language.
	SlotBindingConstant
	slotBindingLimit
)

func (binding SlotBinding) Available() bool {
	return binding > SlotBindingInvalid && binding < slotBindingLimit
}

// EnvironmentSlot is one slot binding: what the slot initially holds, and the
// mutability the slot is published with. Exactly the field the binding selects is
// meaningful, and the other is zero: two spellings of one binding would be two
// contract identities for one environment.
type EnvironmentSlot struct {
	Binding SlotBinding
	// Value is the address of the bound value, for a value binding.
	Value Path
	// Constant is the bound literal, for a constant binding.
	Constant Constant
	// Mutability is the policy the slot is published under.
	Mutability Mutability
}

func (slot EnvironmentSlot) Available() bool {
	if !slot.Binding.Available() || !slot.Mutability.Available() || !slot.Value.Available() {
		return false
	}
	switch slot.Binding {
	case SlotBindingValue:
		return slot.Constant == Constant{}
	case SlotBindingConstant:
		return slot.Value.Len() == 0 && slot.Constant.Available()
	default:
		return false
	}
}

// BindValue is the common slot binding: the slot holds the value at one address.
func BindValue(value Path, mutability Mutability) EnvironmentSlot {
	return EnvironmentSlot{Binding: SlotBindingValue, Value: value, Mutability: mutability}
}

// BindConstant is the slot binding of a slot that holds a literal.
func BindConstant(constant Constant, mutability Mutability) EnvironmentSlot {
	return EnvironmentSlot{Binding: SlotBindingConstant, Constant: constant, Mutability: mutability}
}

// EncodeEnvironmentSlot writes one slot binding as a member payload body. A bound
// address is written in the export-path format this package already owns, so an
// address has one encoding wherever it appears.
func EncodeEnvironmentSlot(slot EnvironmentSlot) ([]byte, error) {
	if !slot.Available() {
		return nil, ErrMalformed
	}
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, environmentSlotDomain, environmentSlotVersion); err != nil {
		return nil, err
	}
	if err := writer.Uint(uint64(slot.Binding)); err != nil {
		return nil, err
	}
	if err := writer.Uint(uint64(slot.Mutability)); err != nil {
		return nil, err
	}
	switch slot.Binding {
	case SlotBindingValue:
		value, err := EncodePath(slot.Value)
		if err != nil {
			return nil, err
		}
		if err := writer.Bytes(value); err != nil {
			return nil, err
		}
	case SlotBindingConstant:
		if err := writeConstant(&writer, slot.Constant); err != nil {
			return nil, err
		}
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// DecodeEnvironmentSlot reads one environment-slot payload body.
func DecodeEnvironmentSlot(data []byte) (EnvironmentSlot, error) {
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		return EnvironmentSlot{}, ErrMalformed
	}
	if err := reader.Header(environmentSlotDomain, environmentSlotVersion); err != nil {
		return EnvironmentSlot{}, ErrMalformed
	}
	binding, err := reader.Uint()
	if err != nil || binding > uint64(^uint8(0)) {
		return EnvironmentSlot{}, ErrMalformed
	}
	mutability, err := reader.Uint()
	if err != nil || mutability > uint64(^uint8(0)) {
		return EnvironmentSlot{}, ErrMalformed
	}
	slot := EnvironmentSlot{Binding: SlotBinding(binding), Mutability: Mutability(mutability)}
	switch slot.Binding {
	case SlotBindingValue:
		body, err := reader.Bytes(maxBody)
		if err != nil {
			return EnvironmentSlot{}, ErrMalformed
		}
		value, err := DecodePath(body)
		if err != nil {
			return EnvironmentSlot{}, ErrMalformed
		}
		slot.Value = value
	case SlotBindingConstant:
		constant, err := readConstant(reader)
		if err != nil {
			return EnvironmentSlot{}, ErrMalformed
		}
		slot.Constant = constant
	default:
		return EnvironmentSlot{}, ErrMalformed
	}
	if err := reader.Finish(); err != nil {
		return EnvironmentSlot{}, ErrMalformed
	}
	if !slot.Available() {
		return EnvironmentSlot{}, ErrMalformed
	}
	return slot, nil
}
