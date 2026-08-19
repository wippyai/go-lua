package target

import (
	"errors"
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

func (c *Contract) effect(op vocabulary.Operation, index int) (effectRow, bool) {
	row, ok := c.operation(op)
	if !ok || index < 0 || index >= row.effects.len() {
		return effectRow{}, false
	}
	effects := row.effects
	return c.effects[effects.start+uint32(index)], true
}

func (c *Contract) validPublicationEffectRow(effect effectRow) bool {
	if c == nil || !effect.hasPublication || !effect.publication.validConsequences() {
		return false
	}
	target, ok := c.Core.Input(effect.target)
	if !ok || uint64(effect.publication.subject) >= uint64(c.Core.ValuesCount(target)) {
		return false
	}
	return effect.publication.destination != vocabulary.PublicationDestinationValueFormal ||
		uint64(effect.publication.context) < uint64(c.Core.ValuesCount(target))
}

// PublicationEffectDescriptor returns the Target-owned publication semantics
// for one exact operation effect occurrence.
func (c *Contract) PublicationEffectDescriptor(op vocabulary.Operation, index int) (PublicationEffectDescriptor, bool) {
	row, ok := c.effect(op, index)
	if !ok || !c.sealed || !c.validPublicationEffectRow(row) {
		return PublicationEffectDescriptor{}, false
	}
	return row.publication, true
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
