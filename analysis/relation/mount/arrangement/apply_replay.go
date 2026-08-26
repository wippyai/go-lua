package arrangement

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// ApplyReplay is the sealed population driver plus one generic correlated
// subtree per declared Apply child. The historic direct/shared/scalar forms
// are not stored as modes: each child is described only by its exact Input and
// Complete extent sources.
type ApplyReplay struct {
	apply             identity.ContentID
	correlation       algebra.ApplyCorrelation
	population        model.DenominatorRef
	coordinate        model.ColumnID
	coordinateOrdinal uint32
	driver            Layout
	children          []CorrelatedSubtree
	digest            identity.ContentID
	sealed            bool
}

func newApplyReplay(apply identity.ContentID, correlation algebra.ApplyCorrelation, driver Layout, children []CorrelatedSubtree) (ApplyReplay, bool) {
	if !apply.Available() || !correlation.Available() || !driver.Available() || len(children) != correlation.ProjectionCount() || len(children) < 2 {
		return ApplyReplay{}, false
	}
	population, coordinate := correlation.Population(), correlation.Coordinate()
	if !population.Available() || !coordinate.Available() || driver.Access().Relation() != population.Relation() || driver.Access().Key().Available() || driver.CoordinateClass() != CoordinateClassNone || !containsColumn(driver.Columns(), coordinate) || !driver.ValidFor(driver.Handle().Fence()) {
		return ApplyReplay{}, false
	}
	coordinateOrdinal, ordinalOK := columnOrdinal(driver.Columns(), coordinate)
	if !ordinalOK {
		return ApplyReplay{}, false
	}
	value := ApplyReplay{
		apply:             apply,
		correlation:       correlation,
		population:        population,
		coordinate:        coordinate,
		coordinateOrdinal: coordinateOrdinal,
		driver:            cloneLayout(driver),
		children:          append([]CorrelatedSubtree(nil), children...),
	}
	if !validApplyReplayExtents(value) {
		return ApplyReplay{}, false
	}
	parts := applyReplayDigestParts(value)
	if parts == nil {
		return ApplyReplay{}, false
	}
	digest, digestOK := identity.DeriveContentID("analysis/relation/mount/arrangement/apply-replay/v2", parts...)
	if !digestOK {
		return ApplyReplay{}, false
	}
	value.digest, value.sealed = digest, true
	if !validApplyReplay(value) {
		return ApplyReplay{}, false
	}
	return value, true
}

// validApplyReplayExtents checks only the closed extent capabilities issued
// for every child. It deliberately does not classify a child by a structural
// replay shape: driver, partition, and mounted sources are independent facts
// attached to exact occurrences.
func validApplyReplayExtents(value ApplyReplay) bool {
	if !value.apply.Available() || !value.correlation.Available() || !value.population.Available() || !value.coordinate.Available() || value.population != value.correlation.Population() || value.coordinate != value.correlation.Coordinate() || !value.driver.Available() || value.driver.Access().Relation() != value.population.Relation() || value.driver.Access().Key().Available() || value.driver.CoordinateClass() != CoordinateClassNone || !columnOrdinalMatches(value.driver.Columns(), value.coordinate, value.coordinateOrdinal) || len(value.children) != value.correlation.ProjectionCount() || len(value.children) < 2 {
		return false
	}
	driverChildren := 0
	for index, child := range value.children {
		if !child.Available() || child.ordinal != uint32(index) || child.root == nil {
			return false
		}
		projection, projectionOK := value.correlation.ProjectionAt(index)
		if !projectionOK || len(projection) > 1 {
			return false
		}
		carrierDirectory, partitioned := child.carrierDirectory()
		driverSource, driverInput := child.driverSource()
		if driverInput {
			driverChildren++
			layout, slot, sourceOK := driverSource.PopulationDriver()
			if !sourceOK || !layout.Equal(value.driver) || slot.Child() != uint32(index) || len(projection) != 1 {
				return false
			}
			if partitioned || carrierDirectory.Available() {
				return false
			}
			continue
		}
		if len(projection) == 0 {
			if partitioned || carrierDirectory.Available() {
				return false
			}
			continue
		}
		if !partitioned || !carrierDirectory.Available() || carrierDirectory.Population() != value.population {
			return false
		}
	}
	// A scalar population Input is optional, but a duplicate would make one
	// driver row appear as two independent child extents.
	return driverChildren <= 1
}

func validApplyReplay(value ApplyReplay) bool {
	if !value.sealed || !value.digest.Available() || !validApplyReplayExtents(value) {
		return false
	}
	parts := applyReplayDigestParts(value)
	digest, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/apply-replay/v2", parts...)
	return ok && digest == value.digest
}

func applyReplayDigestParts(value ApplyReplay) [][]byte {
	if !value.correlation.Available() || !value.driver.Available() {
		return nil
	}
	parts := [][]byte{
		contentBytes(value.correlation.Digest()),
		contentBytes(value.apply),
		denominatorBytes(value.population),
		nominalBytes(value.coordinate.Relation().Owner().Content(), value.coordinate.Content()),
		correlatedUint32Bytes(value.coordinateOrdinal),
		contentBytes(value.driver.Digest()),
	}
	for _, child := range value.children {
		digest := child.Digest()
		if !digest.Available() {
			return nil
		}
		parts = append(parts, contentBytes(digest))
	}
	return parts
}

// Available redeems the constructor-issued replay seal in O(1). The complete
// subtree/extent walk occurs only while newApplyReplay is sealing the mount.
func (value ApplyReplay) Available() bool {
	return value.sealed && value.digest.Available()
}

func (value ApplyReplay) Correlation() algebra.ApplyCorrelation {
	if !value.Available() {
		return algebra.ApplyCorrelation{}
	}
	return value.correlation
}

func (value ApplyReplay) Population() model.DenominatorRef {
	if !value.Available() {
		return model.DenominatorRef{}
	}
	return value.population
}

func (value ApplyReplay) Coordinate() (model.ColumnID, bool) {
	if !value.Available() {
		return model.ColumnID{}, false
	}
	return value.coordinate, true
}

func (value ApplyReplay) CoordinateOrdinal() (uint32, bool) {
	if !value.Available() {
		return 0, false
	}
	return value.coordinateOrdinal, true
}

func (value ApplyReplay) Driver() (Layout, bool) {
	if !value.Available() {
		return Layout{}, false
	}
	return cloneLayout(value.driver), true
}

func (value ApplyReplay) ChildCount() int {
	if !value.Available() {
		return 0
	}
	return len(value.children)
}

// ChildAt returns the exact sealed child subtree in Apply declaration order.
func (value ApplyReplay) ChildAt(index int) (CorrelatedSubtree, bool) {
	if !value.Available() || index < 0 || index >= len(value.children) || !value.children[index].Available() {
		return CorrelatedSubtree{}, false
	}
	return value.children[index], true
}

func (value ApplyReplay) Digest() identity.ContentID {
	if !value.Available() {
		return identity.ContentID{}
	}
	return value.digest
}

func (value CorrelatedSubtree) digestValue() (identity.ContentID, bool) {
	if !value.Available() {
		return identity.ContentID{}, false
	}
	return value.digest, true
}

func (value CorrelatedSubtree) driverSource() (CorrelationExtentSource, bool) {
	var result CorrelationExtentSource
	found := false
	for _, input := range value.inputs {
		source := input.Source()
		if _, _, driver := source.PopulationDriver(); !driver {
			continue
		}
		if found {
			return CorrelationExtentSource{}, false
		}
		result, found = source, true
	}
	return result, found
}

func columnOrdinal(columns []model.ColumnID, target model.ColumnID) (uint32, bool) {
	var found uint32
	foundValue := false
	for index, column := range columns {
		if column != target {
			continue
		}
		if foundValue {
			return 0, false
		}
		found, foundValue = uint32(index), true
	}
	return found, foundValue
}

func columnOrdinalMatches(columns []model.ColumnID, target model.ColumnID, ordinal uint32) bool {
	if int(ordinal) >= len(columns) || columns[ordinal] != target {
		return false
	}
	for index, column := range columns {
		if uint32(index) != ordinal && column == target {
			return false
		}
	}
	return true
}
