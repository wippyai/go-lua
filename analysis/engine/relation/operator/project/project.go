package project

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Execute projects one source cofiber Batch through a sealed target/key
// arrangement. target is a Reader bound to ProjectBinding.Target(), the
// complete destination row vector. ProjectBinding.Key remains the separate
// equality/index authority. Destination rows are redeemed by the target
// reader's sealed scan and matched through the mounted key-equality
// authorities before tuple.ProjectInto.
//
// One invocation owns one source cofiber and one sealed range partition. The
// output preserves that partition and is split only when exact destination
// scopes differ. A missing destination is an authenticated empty Batch
// carrying the source cofiber. The returned slice is ordered partition
// transport; it is not a second relation representation.
func Execute(bound arrangement.ProjectBinding, mounted witness.Mounted, source tuple.Batch, target read.Reader) ([]tuple.Batch, bool) {
	if !bound.Available() || !mounted.Available() || !source.ValidFor(mounted) || !target.Available() {
		return nil, false
	}
	targetLayout := bound.Target()
	keyLayout := bound.Key()
	keyColumns := keyLayout.KeyColumns()
	if !targetLayout.ValidFor(mounted.Fence()) || !keyLayout.ValidFor(mounted.Fence()) || !target.Layout().Equal(targetLayout) || target.Layout().Access().Relation() != targetLayout.Access().Relation() || target.Layout().Access().Key().Available() || len(target.Layout().Columns()) == 0 || len(keyColumns) == 0 {
		return nil, false
	}

	if bound.MappingCount() == 0 || bound.KeyMappingCount() != len(keyColumns) {
		return nil, false
	}

	type outputPartition struct {
		scope  witness.Scope
		values []tuple.Tuple
	}
	outputs := make([]outputPartition, 0)
	refused := false
	for sourceIndex := 0; sourceIndex < source.Len(); sourceIndex++ {
		value, valueOK := source.At(sourceIndex)
		if !valueOK || !value.ValidFor(mounted) {
			return nil, false
		}
		// The sealed mapping is a source-column contract, not a candidate
		// selector. A missing source cell is malformed input and must refuse;
		// it must never be interpreted as an ordinary no-match result.
		for index := 0; index < bound.MappingCount(); index++ {
			mapping, mappingOK := bound.MappingAt(index)
			if !mappingOK {
				return nil, false
			}
			if _, cellOK := value.CellFor(mapping.Source()); !cellOK {
				return nil, false
			}
		}
		for index, targetColumn := range keyColumns {
			mapping, mappingOK := bound.KeyMappingAt(index)
			if !mappingOK {
				return nil, false
			}
			cell, cellOK := value.CellFor(mapping.Source())
			if !cellOK || !validKeyCell(cell, mounted) {
				return nil, false
			}
			targetType, typeOK := target.Type(targetColumn)
			if !typeOK || targetType != cell.Type() {
				return nil, false
			}
		}
		// Reader.Lookup is an exact token-index operation. It cannot redeem
		// two independently issued handles for one owner value, so Project's
		// single match path scans the sealed target rows and applies the
		// mounted equality authority to the declared key. This is not a
		// fallback: no token lookup is attempted before or after it.
		matchedCompleted, matchedValid := target.Scan(func(destination read.Row) bool {
			if destination == nil || !destination.Available() || destination.ID().Relation() != targetLayout.Access().Relation() || !destination.Scope().ValidFor(mounted.RuntimeFence()) {
				refused = true
				return false
			}
			destinationTuple, destinationOK := tuple.Input(mounted, target, destination)
			if !destinationOK {
				refused = true
				return false
			}
			equal, equalityValid := destinationKeyEqual(mounted, value, destinationTuple, bound, keyColumns)
			if !equalityValid {
				refused = true
				return false
			}
			if !equal {
				return true
			}
			// A non-conjoining scope is a valid no-selection. Once that
			// closed-world case is separated, any remaining projection failure is
			// an authority/refusal result rather than a silent empty match.
			if _, scopeOK := target.Conjoin(value.Scope(), destinationTuple.Scope()); !scopeOK {
				return true
			}
			output, outputOK := tuple.ProjectInto(mounted, target, value, destinationTuple, bound)
			if !outputOK {
				refused = true
				return false
			}
			partitionIndex := -1
			for index := range outputs {
				if outputs[index].scope.Same(output.Scope()) {
					partitionIndex = index
					break
				}
			}
			if partitionIndex < 0 {
				outputs = append(outputs, outputPartition{scope: output.Scope(), values: []tuple.Tuple{output}})
			} else {
				outputs[partitionIndex].values = append(outputs[partitionIndex].values, output)
			}
			return true
		})
		if refused || !matchedValid || !matchedCompleted {
			return nil, false
		}
	}
	if len(outputs) == 0 {
		empty, ok := tuple.PreserveRange(mounted, source, source.Scope(), []tuple.Tuple{})
		if !ok {
			return nil, false
		}
		return []tuple.Batch{empty}, true
	}
	result := make([]tuple.Batch, len(outputs))
	for index, output := range outputs {
		batch, ok := tuple.PreserveRange(mounted, source, output.scope, output.values)
		if !ok {
			return nil, false
		}
		result[index] = batch
	}
	return result, true
}

func validKeyCell(cell tuple.Cell, mounted witness.Mounted) bool {
	return cell.Column().Available() && cell.Type().Available() && cell.Presence().Available() && !cell.Presence().Is(model.Refused) && cell.Value().Available() && cell.Value().ValidFor(mounted.RuntimeFence()) && cell.Value().Type() == cell.Type() && (cell.Presence().Is(model.Present) || cell.Presence().Is(model.AuthenticatedOpaque))
}

// destinationKeyEqual compares the source's mapped key with one destination
// row through the single mounted equality authority. It returns a separate
// validity bit so a malformed destination row refuses the operation instead
// of becoming a silent no-match.
func destinationKeyEqual(mounted witness.Mounted, source, destination tuple.Tuple, bound arrangement.ProjectBinding, keyColumns []model.ColumnID) (equal, valid bool) {
	if !mounted.Available() || !source.ValidFor(mounted) || !destination.ValidFor(mounted) || len(keyColumns) == 0 {
		return false, false
	}
	for index, targetColumn := range keyColumns {
		mapping, mappingOK := bound.KeyMappingAt(index)
		if !mappingOK {
			return false, false
		}
		sourceCell, sourceOK := source.CellFor(mapping.Source())
		destinationCell, destinationOK := destination.CellFor(targetColumn)
		if !sourceOK || !destinationOK || !validKeyCell(sourceCell, mounted) || !validKeyCell(destinationCell, mounted) || sourceCell.Type() != destinationCell.Type() {
			return false, false
		}
		if !tuple.SemanticEqual(mounted, sourceCell.Type(), sourceCell.Value(), destinationCell.Value()) {
			return false, true
		}
	}
	return true, true
}
