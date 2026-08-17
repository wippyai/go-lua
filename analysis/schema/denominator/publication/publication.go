// Package publication declares and materializes the neutral denominator
// cardinality column. The column is an engine-published Snapshot value: its
// key and value are schema identities and counts, so no domain type enters the
// engine or Snapshot.
package publication

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

const (
	// AxisKey is the declaration identity of the neutral denominator count
	// coordinate space. The axis is a writer principal for its own column.
	AxisKey schema.Key = "denominator-count"
	// OutputKey is the one Snapshot column published by this axis.
	OutputKey schema.Key = "denominator/counts"
	// AxisRole is the structural role under which the coordinate space is
	// identified in the sealed schema vocabulary.
	AxisRole = "axis/denominator-count"

	contentDomain = "wippy.analysis/schema/denominator/publication/relation-counts/v1"
)

// SchemaFragment and HotAxis are deliberately empty: this is an
// engine-published axis, not a factor binding. The types make that absence
// explicit in the generic declaration rather than inventing a binding hook.
type (
	SchemaFragment struct{}
	HotAxis        struct{}
)

// AxisEntry declares the neutral relation-count column. The axis is Link
// lifetime because one mounted Snapshot owns one publication, frozen because
// the column is written once, and shared because readers borrow immutable
// Snapshot state concurrently. Its key space is sparse: EntryID values are
// content identities, not dense ordinals.
func AxisEntry[A any]() axis.Spec[A, *SchemaFragment, *HotAxis, uint64] {
	return axis.Spec[A, *SchemaFragment, *HotAxis, uint64]{
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

// StructureSpecs contributes the axis's one semantic role to the structural
// vocabulary. The role is declared beside the axis that consumes it, so the
// composition does not maintain a second role inventory.
func StructureSpecs() []structure.Spec { return vocabulary.RoleSpecs(AxisRole) }

// UniverseID identifies the exact relation-count universe under one sealed
// schema. Binding it to the schema digest prevents a column compiled against
// one declaration catalog from proving totals for another catalog.
func UniverseID(schemaID identity.ContentID) (identity.ContentID, bool) {
	if !schemaID.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID(contentDomain, schemaID[:])
}

// BuildContent sums all supplied owner-local count sets and materializes the
// complete generated relation universe. Owners must provide explicit zero rows
// for declared relations with no facts; a missing row is incomplete authority,
// not an implicit zero. An input row naming no generated relation is rejected:
// accepting it would create a second denominator vocabulary outside the sealed
// schema catalog.
func BuildContent(schemaID identity.ContentID, parts ...denominator.CountRows) (snapshot.Content[schema.EntryID, uint64], bool) {
	var empty snapshot.Content[schema.EntryID, uint64]
	universe, universeOK := UniverseID(schemaID)
	if !universeOK {
		return empty, false
	}
	summed, summedOK := denominator.SumCountRows(parts...)
	if !summedOK {
		return empty, false
	}
	// Every owner publishes an explicit zero row when it has no facts for a
	// declared relation. A missing row is incomplete authority, not an implicit
	// zero, because silently filling it would turn an absent owner fact into a
	// proved negative fact.
	if !denominator.GeneratedCountRowsComplete(summed) {
		return empty, false
	}
	rows := make(map[schema.EntryID]uint64, summed.Count())
	members := make([]schema.EntryID, 0, summed.Count())
	for index := 0; index < summed.Count(); index++ {
		row, rowOK := summed.At(index)
		if !rowOK {
			return empty, false
		}
		rows[row.ID()] = row.Count()
		members = append(members, row.ID())
	}
	return snapshot.Content[schema.EntryID, uint64]{
		Rows:        rows,
		Denominator: universe,
		Members:     members,
	}, true
}

// Publish builds and writes the complete neutral column through Snapshot's
// one public publication primitive. No receipt or adapter is introduced: the
// caller supplies the schema-projected axis, and PutColumn performs the
// schema, slot, denominator, and immutable-column checks.
func Publish(builder *snapshot.Builder, ax snapshot.Axis[schema.EntryID, uint64], schemaID identity.ContentID, parts ...denominator.CountRows) error {
	if !schemaID.Available() || !ax.Available() || ax.SchemaID != schemaID {
		return errors.New("denominator publication: schema axis mismatch")
	}
	content, ok := BuildContent(schemaID, parts...)
	if !ok {
		return errors.New("denominator publication: invalid relation counts")
	}
	return snapshot.PutColumn(builder, ax, content)
}
