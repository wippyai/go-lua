package program

import "github.com/wippyai/go-lua/analysis/schema"

// InputRef names one rule input port, and a port IS an observation point: it
// selects the predecessor state this read is taken against. The engine opens
// an invocation with one carrier state per port and a read resolves its own
// through that index alone, so what a read observes is decided here and
// nowhere else.
//
// Two reads naming one port therefore observe ONE point. That is legal and
// sometimes intended - a carry and the read it carries share theirs - but it
// is a statement, not a default: a declaration copied from another rule that
// leaves a read on the port it was copied with observes that rule's point
// rather than its own, and every other clause of the read can be right while
// it does. The emitted law suite renders the ports as their own rows so the
// count of distinct points a rule observes is reviewable next to the count of
// reads that observe them.
//
// Zero is a valid port; ports are sealed as a contiguous prefix from zero by
// Program.Check, which counts the set the reads and the carry name together.
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
