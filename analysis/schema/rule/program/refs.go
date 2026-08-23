package program

import "github.com/wippyai/go-lua/analysis/schema"

// InputRef names one rule input port. Zero is a valid port; ports are sealed
// as a contiguous prefix from zero by Program.Check.
type InputRef uint64

func (ref InputRef) Uint64() uint64 { return uint64(ref) }

// AxisRef and DenominatorRef keep the cold ABI descriptive without giving the
// program a domain import or a runtime handle. Their target entry is still
// resolved only by schema/seal after every surface is published. Member and
// output references are owned by analysis/schema/axis and axis/member.
type AxisRef schema.EntryReference
type DenominatorRef schema.EntryReference

func (ref AxisRef) Available() bool {
	reference := schema.EntryReference(ref)
	return reference.Surface == schema.SurfaceKindAxis && reference.Key.Available()
}

func (ref AxisRef) Declared() bool { return schema.EntryReference(ref).Declared() }

func (ref AxisRef) EntryReference() schema.EntryReference { return schema.EntryReference(ref) }

func (ref DenominatorRef) Available() bool {
	reference := schema.EntryReference(ref)
	return reference.Surface == schema.SurfaceKindDenominator && reference.Key.Available()
}

func (ref DenominatorRef) Declared() bool { return schema.EntryReference(ref).Declared() }

func (ref DenominatorRef) EntryReference() schema.EntryReference { return schema.EntryReference(ref) }
