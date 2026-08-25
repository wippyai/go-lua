package axis

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// OutputRef is the owner-qualified reference to one nested output column of an
// axis. Axis is the root declaration resolved by schema/seal; Key remains a
// local frame name and is never flattened into a second schema entry.
type OutputRef struct {
	Axis schema.EntryReference
	Key  schema.Key
}

func (reference OutputRef) Available() bool {
	return reference.Axis.Surface == schema.SurfaceKindAxis && reference.Axis.Key.Available() && reference.Key.Available()
}

func (reference OutputRef) Declared() bool {
	return reference.Axis.Declared() || reference.Key.Available()
}

func (reference OutputRef) AxisReference() schema.EntryReference { return reference.Axis }

// ID returns the identity the owning axis issued for this published column.
// An output column is a member of its axis like any other, so it is named by
// the one member assigner rather than by a second derivation here.
func (reference OutputRef) ID() schema.EntryID {
	return member.IssueID(reference.Axis, reference.Key)
}

// Frame is one axis's published half: the columns this axis's facts are read
// out of once the engine publishes them, and the principal admitted to write
// each of them. Declaring a coordinate space and publishing it are two acts. A
// space the solver holds facts over is not by itself readable by a consumer,
// and a consumer that reads one must know which writer's rows it is reading,
// so the frame is what turns a declared axis into a published one and states
// the writer in the same breath.
//
// The frame declares identity and capability, and nothing about storage. Where
// an axis's facts live, how its key space is shaped, and the lifetime,
// mutability, and concurrency disciplines it is written under are already
// declared by the axis's own metadata; a publication derives its structural
// choices from those rather than restating them, so the analyzer spells its
// storage classes exactly once and this surface is where.
//
// An axis that declares no output publishes nothing: its facts stay inside the
// solver, and no column names it.
type Frame struct {
	// Outputs are the published columns this axis declares, in declaration
	// order. The order is the order the composition assigns dense column slots
	// in, so a published column's address is a function of the sealed table
	// rather than of the order a publisher happens to write in.
	Outputs []Output
}

// Available reports whether this frame publishes anything.
func (frame Frame) Available() bool { return len(frame.Outputs) > 0 }

// Output is one published column: the name a consumer reads it under, and the
// principal admitted to write it.
type Output struct {
	// Key names one published column across the whole axis surface. Two
	// declarations under one name would leave a consumer reading one name
	// without knowing whose rows it holds, so the surface seals a name to one
	// output and one output to one writer.
	Key schema.Key
	// Writer is the principal admitted to write this column. An axis is a
	// writer principal, so a writer is named by the axis key that declares it:
	// an axis that writes its own facts names its own key, and a column an
	// engine pass fills names the non-factor axis that pass is declared as. The
	// writer is authored rather than derived from the declaring entry, because
	// the pair the seal admits is exactly the pair the engine mints a write
	// capability for, and a derived writer would state nothing for the engine
	// to be held to.
	Writer schema.Key
}

// Available reports whether this output declares both halves. An output with a
// name and no writer is a column nothing may fill; an output with a writer and
// no name is a capability over nothing.
func (output Output) Available() bool {
	return output.Key.Available() && output.Writer.Available()
}

// Coverage is what a consumer may conclude from a key a published column of
// this axis holds no row for. It is derived from the axis's declared
// cardinality rather than declared a second time: a dense axis numbers its
// coordinates over a contiguous range, so that range is the key universe the
// column is total over and a missing row is a published absence; a sparse axis
// materializes only the coordinates that occur, so a missing row is ignorance.
type Coverage uint8

const (
	CoverageInvalid Coverage = iota
	// CoverageTotal is a column published together with the key universe it is
	// total over, so a key inside that universe with no row is absent as a
	// fact.
	CoverageTotal
	// CoveragePartial is a column published without a key universe, so a key
	// with no row is unknown to this column and nothing more.
	CoveragePartial
)

func (coverage Coverage) Available() bool {
	return coverage == CoverageTotal || coverage == CoveragePartial
}

// CoverageFor projects one declared cardinality onto the coverage a published
// column of that axis reports. It is the single derivation between the
// declared key-space shape and the published column's ability to prove an
// absence, so a publisher reads it instead of deciding the same question again
// from its own reading of the metadata.
func CoverageFor(cardinality Cardinality) Coverage {
	switch cardinality {
	case CardinalityDense:
		return CoverageTotal
	case CardinalitySparse:
		return CoveragePartial
	default:
		return CoverageInvalid
	}
}
