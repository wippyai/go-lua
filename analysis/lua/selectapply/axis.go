package selectapply

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

const (
	// AxisKey is the channel-select case coordinate space. The axis is the
	// writer principal for its own column.
	AxisKey schema.Key = "channel-select-case"
	// OutputKey is the Snapshot column of accepted select-case facts.
	OutputKey schema.Key = "channel-select-case/facts"
	// AxisRole is the semantic role this coordinate space is identified by.
	AxisRole = "axis/channel-select-case"
)

// AxisEntry declares the accepted channel-select case column. Keys are
// CaseFactID values. The column is sparse: a missing identity is not an
// accepted arm.
func AxisEntry[A any]() axis.Spec[A] {
	return axis.Spec[A]{
		Key:         AxisKey,
		Storage:     axis.StorageEngine,
		Cardinality: axis.CardinalitySparse,
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Frame:       axis.Frame{Outputs: []axis.Output{{Key: OutputKey, Writer: AxisKey}}},
		Semantic:    vocabulary.RoleKey(AxisRole),
	}
}

// StructureSpecs is this axis's semantic role row.
func StructureSpecs() []structure.Spec { return vocabulary.RoleSpecs(AxisRole) }
