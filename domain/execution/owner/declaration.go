// Package owner declares the analyzer's execution-reachability axis: the
// coordinate space of mounted execution points, the column the reachable ones
// are published in, and the semantic role the space is identified by.
//
// The axis carries no factor binding. Which execution points a Link reaches is
// derived by the engine's own demand pass rather than by a rule lane, so the
// declaration states the coordinate space, the writer principal and the
// published column and stops there: there is no cold fragment to record, no
// factor cell to bind, and no algebra of one to publish. The surface admits
// exactly that shape, and it admits it only whole - an axis that declared a
// column here and a factor binding beside it would say two different things
// about who fills the column.
//
// The column's coordinate is one mounted execution point: the mount the point
// was reached in, and the point itself. The mount qualifies the point because a
// program mounted twice is two mounts and a point reachable in one of them is
// not thereby reachable in the other. The pair's own spelling is the Link
// surface's, and a consumer names it at the read site, where the published
// value's checked recovery holds the claim to the column the publisher filled.
package owner

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// The identities this package declares. Each is authored here and named from
// here, so the rows and the references that resolve them are one package's
// statement.
const (
	// AxisKey is this coordinate space's authored identity. An axis is a writer
	// principal, so it is also the identity of the principal admitted to write
	// the column below: the demand pass that derives reachability publishes it
	// as this axis.
	AxisKey schema.Key = "execution-reachability"
	// OutputKey is the one column this axis publishes: the mounted execution
	// points the engine proved reachable.
	OutputKey schema.Key = "execution-reachability/facts"
	// AxisRole is the semantic role this axis is identified by. The space is not
	// a factor, so it is declared under the axis namespace of the role
	// vocabulary rather than the factor one.
	AxisRole = "axis/execution-reachability"
)

// Reachable is the published fact of one mounted execution point: the engine
// reached it. It carries nothing else, because the column carries presence
// alone - the key universe is published with the column, so a point inside that
// universe with no row is absent as a fact and an unreachable point costs no
// row to state.
type Reachable struct{}

// AxisEntry is this package's axis declaration. A is the composition's own Link
// input record: this axis names nothing in it, because it mounts no authority of
// its own and binds no factor against one.
func AxisEntry[A any]() axis.Spec[A] {
	return axis.Spec[A]{
		Key:     AxisKey,
		Storage: axis.StorageEngine,
		// The mounted execution points of a Link are the key universe this
		// column is total over, so the column is published together with that
		// denominator and a point inside it with no row is a published absence
		// rather than ignorance.
		Cardinality: axis.CardinalityDense,
		// Reachability is derived from the mounts of one Link and dies with the
		// binding that mounted them, and the pass publishes the column once: no
		// rule writes it afterwards, and the published column is read by
		// whoever holds the value.
		Lifetime:    axis.LifetimeLink,
		Mutability:  axis.MutabilityFrozen,
		Concurrency: axis.ConcurrencyShared,
		Frame:       axis.Frame{Outputs: []axis.Output{{Key: OutputKey, Writer: AxisKey}}},
		Semantic:    vocabulary.RoleKey(AxisRole),
	}
}

// StructureSpecs is this package's contribution to the analyzer's semantic role
// vocabulary: the one role its axis is identified by. A role is declared where
// it is used, so the row and the reference that names it are one package's
// statement.
func StructureSpecs() []structure.Spec { return vocabulary.RoleSpecs(AxisRole) }
