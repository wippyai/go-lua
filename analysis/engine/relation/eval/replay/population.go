// Package replay contains the small, authenticated runtime pieces used to
// redeem an Apply replay.  Population is deliberately kept separate from
// tuple construction: it records only the coordinate evidence redeemed from
// the already-bound, unkeyed population reader.
package replay

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
)

// CoordinateEvidence is the authenticated part of one redeemed population
// row.  It intentionally does not retain a read.Row (rows are borrowed by a
// Reader callback), a key, or a copied relation.  The coordinate and its type
// are carried once by Population.
type CoordinateEvidence struct {
	row         model.RowID
	ordinal     uint32
	scope       witness.Scope
	lineage     model.LineageRef
	cellLineage model.LineageRef
	value       binding.ValueToken
	presence    model.Presence
	fence       binding.Fence
}

// Available reports whether the evidence has the shape issued by Populate.
// The semantic type check is performed against Population's declared type.
func (value CoordinateEvidence) Available() bool {
	if !value.row.Available() || !value.fence.Available() {
		return false
	}
	if !value.scope.Available() || !value.scope.ValidFor(value.fence) {
		return false
	}
	if !value.lineage.Available() {
		return false
	}
	if !value.cellLineage.Available() {
		return false
	}
	if !value.value.Available() || !value.value.ValidFor(value.fence) {
		return false
	}
	return value.presence.Is(model.Present) || value.presence.Is(model.AuthenticatedOpaque)
}

// RowID returns the owner-issued population RowID.
func (value CoordinateEvidence) RowID() model.RowID { return value.row }

// Ordinal returns the exact position of RowID in the owner witness that drove
// this callback. It is not derived from a physical row/key address.
func (value CoordinateEvidence) Ordinal() (uint32, bool) {
	if !value.Available() {
		return 0, false
	}
	return value.ordinal, true
}

// Scope returns the authenticated scope of the redeemed row.
func (value CoordinateEvidence) Scope() witness.Scope { return value.scope }

// Lineage returns the authenticated lineage of the redeemed row.
func (value CoordinateEvidence) Lineage() model.LineageRef { return value.lineage }

// CellLineage returns the authenticated lineage attached to the coordinate
// cell itself. It is distinct from the row lineage for child reconstruction.
func (value CoordinateEvidence) CellLineage() model.LineageRef { return value.cellLineage }

// Value returns the declared correlation coordinate value.
func (value CoordinateEvidence) Value() binding.ValueToken { return value.value }

// Presence returns the coordinate cell's authenticated presence.
func (value CoordinateEvidence) Presence() model.Presence { return value.presence }

// Populate redeems every owner-issued Q row from an already-bound unkeyed
// It is intentionally fail-closed: no scan, key inversion, cache, fallback,
// scope key, relation copy, or runtime step integration is performed here.
// The visitor runs synchronously in exact witness/callback order; no population
// list is retained between calls. It follows Reader's completion/validity pair:
// (true,true) admits the stream, (false,true) is a visitor stop, and
// (false,false) refuses malformed evidence. A directory-issued RowID with no
// posting is an ordinary empty lookup in the bound reader and contributes no
// population callback.
func Populate(replay arrangement.ApplyReplay, mounted witness.Mounted, reader read.Reader, visit func(CoordinateEvidence) bool) (completed, valid bool) {
	if visit == nil {
		return false, false
	}
	if !replay.Available() || !mounted.Available() || !reader.Available() {
		return false, false
	}

	correlation := replay.Correlation()
	if !correlation.Available() {
		return false, false
	}
	ref := replay.Population()
	coordinate, ok := replay.Coordinate()
	if !ok || !ref.Available() || !coordinate.Available() {
		return false, false
	}
	typeID := correlation.Type()
	if !typeID.Available() || coordinate.Relation() != ref.Relation() {
		return false, false
	}

	addressFence := mounted.Fence()
	fence := mounted.RuntimeFence()
	if !addressFence.Available() || !fence.Available() || !ref.Relation().Available() {
		return false, false
	}
	population, ok := mounted.Denominator(ref)
	if !ok || !population.Available() || !population.ValidFor(fence) || !population.Matches(ref) {
		return false, false
	}

	driver, ok := replay.Driver()
	coordinateOrdinal, ordinalOK := replay.CoordinateOrdinal()
	if !ok || !ordinalOK || !validPopulationDriver(driver, reader, mounted, ref, coordinate, int(coordinateOrdinal), typeID, addressFence, fence) {
		return false, false
	}
	authority, ok := mounted.Lineage()
	if !ok || authority == nil {
		return false, false
	}

	for index := 0; index < population.Len(); index++ {
		rowID, ok := population.At(index)
		if !ok || !rowID.Available() || !population.Contains(rowID) {
			return false, false
		}
		rowIndex, rowOK := mounted.RowIndex(ref.Relation(), rowID)
		if !rowOK {
			return false, false
		}
		inverse, inverseOK := mounted.RowAt(ref.Relation(), rowIndex)
		if !inverseOK || inverse != rowID {
			return false, false
		}

		malformed := false
		stopped := false
		completed, valid := reader.LookupRowID(rowID, func(row read.Row) bool {
			evidence, validRow := redeemCoordinate(row, reader, mounted, authority, rowID, uint32(index), ref.Relation(), coordinate, int(coordinateOrdinal), typeID, fence)
			if !validRow {
				malformed = true
				return false
			}
			if !visit(evidence) {
				stopped = true
				return false
			}
			return true
		})
		if stopped {
			return false, true
		}
		// LookupRowID deliberately reports a directory member with no posting as
		// a completed empty result. The mounted denominator is an owner-issued
		// population witness, so a row may be absent from this committed root
		// (including the empty initial root) without making the replay malformed.
		if malformed || !completed || !valid {
			return false, false
		}
	}
	return true, true
}

func validPopulationDriver(
	driver arrangement.Layout,
	reader read.Reader,
	mounted witness.Mounted,
	ref model.DenominatorRef,
	coordinate model.ColumnID,
	coordinateOrdinal int,
	typeID model.TypeID,
	addressFence address.Fence,
	fence binding.Fence,
) bool {
	if !driver.Available() || !driver.ValidFor(addressFence) {
		return false
	}
	access := driver.Access()
	if access.Relation() != ref.Relation() {
		return false
	}
	driverColumns := driver.Columns()
	if access.Key().Available() || driver.KeyWidth() != 0 || len(driver.KeyColumns()) != 0 ||
		driver.CoordinateClass() != arrangement.CoordinateClassNone {
		return false
	}
	if coordinateOrdinal < 0 || coordinateOrdinal >= len(driverColumns) || driverColumns[coordinateOrdinal] != coordinate {
		return false
	}
	if !reader.Available() || !reader.Layout().Equal(driver) || !reader.Layout().ValidFor(addressFence) {
		return false
	}
	readerType, typeOK := reader.Type(coordinate)
	if !typeOK || readerType != typeID {
		return false
	}
	return len(driverColumns) == 1 && driverColumns[0] == coordinate
}

func redeemCoordinate(
	row read.Row,
	reader read.Reader,
	mounted witness.Mounted,
	authority lineage.Authority,
	rowID model.RowID,
	ordinal uint32,
	relation model.RelationID,
	coordinate model.ColumnID,
	coordinateOrdinal int,
	typeID model.TypeID,
	fence binding.Fence,
) (CoordinateEvidence, bool) {
	if row == nil || !reader.Owns(row) || row.ID() != rowID || row.ID().Relation() != relation {
		return CoordinateEvidence{}, false
	}
	scope := row.Scope()
	if !scope.Available() || !scope.ValidFor(fence) {
		return CoordinateEvidence{}, false
	}
	if _, ok := mounted.ScopeToken(scope); !ok {
		return CoordinateEvidence{}, false
	}
	lineage := row.Lineage()
	if !lineage.Available() || !authority.Validate(lineage) {
		return CoordinateEvidence{}, false
	}

	coordinateCell, cellOK := row.CellAt(coordinateOrdinal)
	if !cellOK || !coordinateCell.Available() || coordinateCell.Column() != coordinate || coordinateCell.Column().Relation() != relation {
		return CoordinateEvidence{}, false
	}
	if !coordinateCell.Scope().Same(scope) || !coordinateCell.Scope().ValidFor(fence) {
		return CoordinateEvidence{}, false
	}
	if !authority.Validate(coordinateCell.Lineage()) || coordinateCell.Type() != typeID {
		return CoordinateEvidence{}, false
	}
	presence := coordinateCell.Presence()
	if !presence.Is(model.Present) && !presence.Is(model.AuthenticatedOpaque) {
		return CoordinateEvidence{}, false
	}
	value := coordinateCell.Value()
	if !value.Available() || !value.ValidFor(fence) || value.Type() != typeID {
		return CoordinateEvidence{}, false
	}
	return CoordinateEvidence{
		row:         rowID,
		ordinal:     ordinal,
		scope:       scope,
		lineage:     lineage,
		cellLineage: coordinateCell.Lineage(),
		value:       value,
		presence:    presence,
		fence:       fence,
	}, true
}
