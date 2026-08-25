package unary

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Mapping is the physical projection of one sealed logical ColumnMapping.
// The schema contract owns the relation identities and output key; the
// physical kernel carries no target catalogue or inferred type mirror.
type Mapping struct {
	source model.ColumnID
	target model.ColumnID
}

func mappingOf(value algebra.ColumnMapping) (Mapping, bool) {
	if !value.Source().Available() || !value.Target().Available() {
		return Mapping{}, false
	}
	return Mapping{source: value.Source(), target: value.Target()}, true
}

func (mapping Mapping) Available() bool {
	return mapping.source.Available() && mapping.target.Available()
}
func (mapping Mapping) Source() model.ColumnID { return mapping.source }
func (mapping Mapping) Target() model.ColumnID { return mapping.target }

// ProjectPlan is a mounted physical redemption of one sealed ProjectContract.
// Its target key is part of the contract and is resolved by the target Reader;
// no correspondence callback or runtime RowID authority is accepted here.
type ProjectPlan struct {
	contract algebra.ProjectContract
	mappings []Mapping
}

func NewProjectPlan(contract algebra.ProjectContract) (ProjectPlan, bool) {
	if !contract.Target().Available() || !contract.Key().Available() || contract.Key().Relation() != contract.Target() {
		return ProjectPlan{}, false
	}
	declared := contract.Mappings()
	if declared == nil || len(declared) == 0 {
		return ProjectPlan{}, false
	}
	mappings := make([]Mapping, len(declared))
	seen := make(map[model.ColumnID]struct{}, len(declared))
	for index, value := range declared {
		mapping, ok := mappingOf(value)
		if !ok || mapping.Target().Relation() != contract.Target() {
			return ProjectPlan{}, false
		}
		if _, duplicate := seen[mapping.Target()]; duplicate {
			return ProjectPlan{}, false
		}
		seen[mapping.Target()] = struct{}{}
		mappings[index] = mapping
	}
	return ProjectPlan{contract: contract, mappings: mappings}, true
}

func (plan ProjectPlan) Available() bool {
	if !plan.contract.Target().Available() || !plan.contract.Key().Available() || plan.contract.Key().Relation() != plan.contract.Target() || plan.mappings == nil {
		return false
	}
	seen := make(map[model.ColumnID]struct{}, len(plan.mappings))
	for _, mapping := range plan.mappings {
		if !mapping.Available() || mapping.Target().Relation() != plan.contract.Target() {
			return false
		}
		if _, duplicate := seen[mapping.Target()]; duplicate {
			return false
		}
		seen[mapping.Target()] = struct{}{}
	}
	return true
}

func (plan ProjectPlan) Contract() algebra.ProjectContract {
	if !plan.Available() {
		return algebra.ProjectContract{}
	}
	return plan.contract
}

// ProjectedCell is a transient, typed output projection. It is not a second
// store: publication remains state-owned. A source cell must be present in
// the reader row; missing source cells refuse the projection rather than
// fabricating UnprovenMissing, Bottom, Unknown, or a lineage default.
type ProjectedCell struct {
	target   model.ColumnID
	typeID   model.TypeID
	value    binding.ValueToken
	presence model.Presence
	region   support.Mask
	lineage  model.LineageRef
}

func (cell ProjectedCell) Available() bool {
	// Refused is an evaluation outcome, not a row-cell state.  Keeping it in
	// this transient projection would turn a hard refusal into an apparently
	// deliverable row because Refused has no value token.  The state column
	// layer already rejects it; keep the unary boundary equally closed so a
	// malformed/future source cannot weaken that contract.
	if !cell.target.Available() || !cell.typeID.Available() || !cell.presence.Available() || cell.presence.Is(model.Refused) || !cell.region.Valid() || !cell.lineage.Available() {
		return false
	}
	if cell.value.Available() {
		return cell.value.Type() == cell.typeID
	}
	return !cell.presence.Is(model.Present) && !cell.presence.Is(model.AuthenticatedOpaque)
}

func (cell ProjectedCell) Target() model.ColumnID    { return cell.target }
func (cell ProjectedCell) Type() model.TypeID        { return cell.typeID }
func (cell ProjectedCell) Value() binding.ValueToken { return cell.value }
func (cell ProjectedCell) Presence() model.Presence  { return cell.presence }
func (cell ProjectedCell) Region() support.Mask      { return cell.region }
func (cell ProjectedCell) Lineage() model.LineageRef { return cell.lineage }

// Projection is one transient target row selected by the target Reader's
// owner-issued key lookup. The destination RowID comes from that reader; the
// unary kernel never issues or derives it.
type Projection struct {
	source      read.Row
	destination read.Row
	relation    model.RelationID
	scope       witness.Scope
	lineage     model.LineageRef
	cells       []ProjectedCell
}

func (projection Projection) Available() bool {
	if projection.source == nil || !projection.source.Available() || projection.destination == nil || !projection.destination.Available() || !projection.relation.Available() || projection.destination.ID().Relation() != projection.relation || !projection.scope.Available() || !projection.lineage.Available() || len(projection.cells) == 0 {
		return false
	}
	seen := make(map[model.ColumnID]struct{}, len(projection.cells))
	for _, cell := range projection.cells {
		if !cell.Available() || cell.Target().Relation() != projection.relation {
			return false
		}
		if _, duplicate := seen[cell.Target()]; duplicate {
			return false
		}
		seen[cell.Target()] = struct{}{}
	}
	return true
}

func (projection Projection) Source() read.Row {
	if !projection.Available() {
		return nil
	}
	return projection.source
}

func (projection Projection) Destination() read.Row {
	if !projection.Available() {
		return nil
	}
	return projection.destination
}

func (projection Projection) Relation() model.RelationID {
	if !projection.Available() {
		return model.RelationID{}
	}
	return projection.relation
}

func (projection Projection) Scope() witness.Scope {
	if !projection.Available() {
		return witness.Scope{}
	}
	return projection.scope
}

func (projection Projection) Lineage() model.LineageRef {
	if !projection.Available() {
		return model.LineageRef{}
	}
	return projection.lineage
}

func (projection Projection) Cells() []ProjectedCell {
	if !projection.Available() {
		return nil
	}
	return append([]ProjectedCell(nil), projection.cells...)
}

// Project maps source values into the sealed target key tuple and lets the
// target Reader resolve owner-issued destination rows. A source with no target
// match is simply absent from the output; it is not converted into an identity
// or default cell. Target scope and lineage are joined through the mounted
// authorities before the projection is delivered.
// The third return value distinguishes an ordinary visitor stop from refusal:
// (false, true, true) is a valid early stop, while (false, false, false) is a
// malformed/foreign authority. A source with no target match still completes
// as (true, true, false) and emits no synthetic projection.
func Project(input, target read.Reader, mounted witness.Mounted, plan ProjectPlan, visit func(Projection) bool) (completed, valid, stopped bool) {
	if input == nil || target == nil || !mounted.Available() || !plan.Available() || visit == nil || !input.Layout().ValidFor(mounted.Fence()) || !target.Layout().ValidFor(mounted.Fence()) || input.Layout().Access().Relation() == plan.contract.Target() || target.Layout().Access().Relation() != plan.contract.Target() || target.Layout().Access().Key() != plan.contract.Key() {
		return false, false, false
	}
	lineage, lineageOK := mounted.Lineage()
	if !lineageOK || lineage == nil {
		return false, false, false
	}
	declaredSource := input.Layout().Access().Relation()
	for _, mapping := range plan.mappings {
		if mapping.Source().Relation() != declaredSource {
			return false, false, false
		}
	}

	refused := false
	completed, valid = Scan(input, func(source read.Row) bool {
		targetColumns := target.Layout().KeyColumns()
		keyValues := make([]binding.ValueToken, len(targetColumns))
		for index, targetColumn := range targetColumns {
			mapping, found := mappingForTarget(plan.mappings, targetColumn)
			if !found {
				refused = true
				return false
			}
			part, found := cellFor(source.Cells(), mapping.Source())
			if !found || !cellAvailable(part) || !part.Value().Available() || (!part.Presence().Is(model.Present) && !part.Presence().Is(model.AuthenticatedOpaque)) {
				refused = true
				return false
			}
			keyValues[index] = part.Value()
		}
		tuple, tupleOK := target.TupleFrom(keyValues)
		if !tupleOK {
			refused = true
			return false
		}
		matchedCompleted, matchedValid := target.Lookup(tuple, func(destination read.Row) bool {
			if destination.ID().Relation() != plan.contract.Target() || !destination.Scope().ValidFor(mounted.RuntimeFence()) || !lineage.Validate(destination.Lineage()) {
				refused = true
				return false
			}
			joinedScope, scopeOK := mounted.ConjoinScopes(source.Scope(), destination.Scope())
			if !scopeOK {
				refused = true
				return false
			}
			joinedLineage, lineageOK := lineage.Join(source.Lineage(), destination.Lineage())
			if !lineageOK {
				refused = true
				return false
			}
			cells := make([]ProjectedCell, 0, len(plan.mappings))
			for _, mapping := range plan.mappings {
				part, found := cellFor(source.Cells(), mapping.Source())
				if !found || !cellAvailable(part) {
					refused = true
					return false
				}
				targetType, typeOK := target.Type(mapping.Target())
				if !typeOK || !targetType.Available() || (part.Value().Available() && part.Value().Type() != targetType) {
					refused = true
					return false
				}
				cells = append(cells, ProjectedCell{target: mapping.Target(), typeID: targetType, value: part.Value(), presence: part.Presence(), region: part.Region(), lineage: part.Lineage()})
			}
			projection := Projection{source: source, destination: destination, relation: plan.contract.Target(), scope: joinedScope, lineage: joinedLineage, cells: cells}
			if !projection.Available() {
				refused = true
				return false
			}
			if !visit(projection) {
				stopped = true
				return false
			}
			return true
		})
		if !matchedValid {
			refused = true
			return false
		}
		if !matchedCompleted {
			if !stopped && !refused {
				stopped = true
			}
			return false
		}
		return !refused && !stopped
	})
	if refused {
		return false, false, false
	}
	return completed, valid, stopped
}

type cellView interface {
	Column() model.ColumnID
	Type() model.TypeID
	Value() binding.ValueToken
	Presence() model.Presence
	Region() support.Mask
	Lineage() model.LineageRef
}

func cellFor[T cellView](cells []T, column model.ColumnID) (T, bool) {
	var zero T
	var result T
	found := false
	for _, cell := range cells {
		if cell.Column() != column {
			continue
		}
		if found {
			return zero, false
		}
		result = cell
		found = true
	}
	return result, found
}

func cellAvailable[T cellView](cell T) bool {
	if !cell.Column().Available() || !cell.Type().Available() || !cell.Presence().Available() || cell.Presence().Is(model.Refused) || !cell.Region().Valid() || !cell.Lineage().Available() {
		return false
	}
	return !cell.Value().Available() || cell.Value().Type() == cell.Type()
}

func mappingForTarget(mappings []Mapping, target model.ColumnID) (Mapping, bool) {
	for _, mapping := range mappings {
		if mapping.Target() == target {
			return mapping, true
		}
	}
	return Mapping{}, false
}
