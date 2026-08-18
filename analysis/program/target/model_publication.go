package target

import (
	"errors"
)

import (
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// PublicationEffectDescriptor is the immutable Target-owned projection of an
// explicitly authored PublicationEffectSpec. It can only be obtained from a
// sealed Contract query; its fields intentionally remain private so callers
// cannot forge a descriptor to splice into another owner.
type PublicationEffectDescriptor struct {
	kind        vocabulary.PublicationEffectKind
	subject     vocabulary.ValueFormal
	destination vocabulary.PublicationDestinationRole
	context     vocabulary.ValueFormal
	escape      vocabulary.PublicationEscapeDisposition
	mutability  vocabulary.PublicationMutabilityDisposition
	lifetime    vocabulary.PublicationLifetimeDisposition
}

func (d PublicationEffectDescriptor) Kind() vocabulary.PublicationEffectKind { return d.kind }

func (d PublicationEffectDescriptor) Subject() vocabulary.ValueFormal { return d.subject }

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

// freezePublicationEffect freezes one authored publication declaration into the
// sealed projection. It is the only constructor of PublicationEffectDescriptor:
// the draft layer carries the declaration and never builds the projection, so a
// descriptor can only originate here, under this contract's own admission.
func freezePublicationEffect(input vocabulary.PublicationEffectSpec) (PublicationEffectDescriptor, error) {
	descriptor := PublicationEffectDescriptor{
		kind: input.Kind, subject: input.Subject, destination: input.Destination,
		context: input.Context, escape: input.Escape, mutability: input.Mutability, lifetime: input.Lifetime,
	}
	if descriptor.destination != vocabulary.PublicationDestinationNone && descriptor.destination != vocabulary.PublicationDestinationValueFormal {
		return PublicationEffectDescriptor{}, errors.New("invalid destination role")
	}
	if descriptor.destination == vocabulary.PublicationDestinationNone && descriptor.context != 0 {
		return PublicationEffectDescriptor{}, errors.New("destination-free publication carries context formal")
	}
	if !descriptor.validConsequences() {
		return PublicationEffectDescriptor{}, errors.New("kind and typed consequences disagree")
	}
	return descriptor, nil
}
