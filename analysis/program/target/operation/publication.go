package operation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// PublicationEffectDescriptor is the sealed, owner-issued projection of an
// explicitly authored publication consequence. It contains no Target facade
// state and can only be obtained from Core's effect rows.
type PublicationEffectDescriptor struct {
	kind        vocabulary.PublicationEffectKind
	subject     vocabulary.InputSource
	destination vocabulary.PublicationDestinationRole
	context     vocabulary.ValueFormal
	escape      vocabulary.PublicationEscapeDisposition
	mutability  vocabulary.PublicationMutabilityDisposition
	lifetime    vocabulary.PublicationLifetimeDisposition
}

// Valid reports whether the descriptor carries one admitted typed
// consequence. Core only publishes descriptors that pass this fence.
func (d PublicationEffectDescriptor) Valid() bool { return d.validConsequences() }

func (d PublicationEffectDescriptor) Kind() vocabulary.PublicationEffectKind { return d.kind }

func (d PublicationEffectDescriptor) Subject() vocabulary.InputSource { return d.subject }

func (d PublicationEffectDescriptor) DestinationRole() vocabulary.PublicationDestinationRole {
	return d.destination
}

func (d PublicationEffectDescriptor) Context() vocabulary.ValueFormal { return d.context }

func (d PublicationEffectDescriptor) Escape() vocabulary.PublicationEscapeDisposition {
	return d.escape
}

func (d PublicationEffectDescriptor) Mutability() vocabulary.PublicationMutabilityDisposition {
	return d.mutability
}

func (d PublicationEffectDescriptor) Lifetime() vocabulary.PublicationLifetimeDisposition {
	return d.lifetime
}

func (d PublicationEffectDescriptor) validConsequences() bool {
	if d.subject.Kind != vocabulary.InputSourceValueFormal && d.subject.Kind != vocabulary.InputSourceValuesVar {
		return false
	}
	switch d.kind {
	case vocabulary.PublicationEffectSendTransfer:
		return d.destination == vocabulary.PublicationDestinationValueFormal &&
			d.escape == vocabulary.PublicationEscapeSendTransfer &&
			(d.mutability == vocabulary.PublicationMutabilityPreserve || d.mutability == vocabulary.PublicationMutabilityCopyOnWrite) &&
			d.lifetime == vocabulary.PublicationLifetimePreserve
	case vocabulary.PublicationEffectReturnEscape:
		return d.destination == vocabulary.PublicationDestinationNone && d.escape == vocabulary.PublicationEscapeReturn &&
			d.mutability == vocabulary.PublicationMutabilityPreserve && d.lifetime == vocabulary.PublicationLifetimePreserve
	case vocabulary.PublicationEffectCallbackEscape:
		return d.destination == vocabulary.PublicationDestinationNone && d.escape == vocabulary.PublicationEscapeCallback &&
			d.mutability == vocabulary.PublicationMutabilityPreserve && d.lifetime == vocabulary.PublicationLifetimePreserve
	case vocabulary.PublicationEffectFreezeSeal:
		return d.destination == vocabulary.PublicationDestinationNone && d.escape == vocabulary.PublicationEscapeNone &&
			d.mutability == vocabulary.PublicationMutabilitySeal && d.lifetime == vocabulary.PublicationLifetimePreserve
	case vocabulary.PublicationEffectWriteMutation:
		return d.destination == vocabulary.PublicationDestinationNone && d.escape == vocabulary.PublicationEscapeNone &&
			(d.mutability == vocabulary.PublicationMutabilityWrite || d.mutability == vocabulary.PublicationMutabilityCopyOnWrite) &&
			d.lifetime == vocabulary.PublicationLifetimePreserve
	case vocabulary.PublicationEffectCloseRelease:
		return d.destination == vocabulary.PublicationDestinationNone && d.escape == vocabulary.PublicationEscapeNone &&
			d.mutability == vocabulary.PublicationMutabilityPreserve && d.lifetime == vocabulary.PublicationLifetimeRelease
	default:
		return false
	}
}

func freezePublicationEffect(input vocabulary.PublicationEffectSpec, present bool, formalCount int, valuesVarCount uint32) (PublicationEffectDescriptor, bool, error) {
	if !present {
		return PublicationEffectDescriptor{}, false, nil
	}
	descriptor := PublicationEffectDescriptor{
		kind: input.Kind, subject: input.Subject, destination: input.Destination,
		context: input.Context, escape: input.Escape, mutability: input.Mutability, lifetime: input.Lifetime,
	}
	if descriptor.destination != vocabulary.PublicationDestinationNone && descriptor.destination != vocabulary.PublicationDestinationValueFormal {
		return PublicationEffectDescriptor{}, false, errors.New("target/operation: invalid publication destination role")
	}
	if descriptor.destination == vocabulary.PublicationDestinationNone && descriptor.context != 0 {
		return PublicationEffectDescriptor{}, false, errors.New("target/operation: destination-free publication carries context formal")
	}
	switch descriptor.subject.Kind {
	case vocabulary.InputSourceValueFormal:
		if uint64(descriptor.subject.Ordinal) >= uint64(formalCount) {
			return PublicationEffectDescriptor{}, false, errors.New("target/operation: publication subject is outside target ABI")
		}
	case vocabulary.InputSourceValuesVar:
		if uint64(descriptor.subject.Ordinal) >= uint64(valuesVarCount) {
			return PublicationEffectDescriptor{}, false, errors.New("target/operation: publication subject is outside target ABI")
		}
	default:
		return PublicationEffectDescriptor{}, false, errors.New("target/operation: publication subject has invalid input source")
	}
	if descriptor.destination == vocabulary.PublicationDestinationValueFormal && int(descriptor.context) >= formalCount {
		return PublicationEffectDescriptor{}, false, errors.New("target/operation: publication context is outside target ABI")
	}
	if !descriptor.validConsequences() {
		return PublicationEffectDescriptor{}, false, errors.New("target/operation: kind and typed consequences disagree")
	}
	return descriptor, true, nil
}
