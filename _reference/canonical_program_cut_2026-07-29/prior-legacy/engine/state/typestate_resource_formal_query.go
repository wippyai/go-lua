package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// TypestateResourceFormalQueryPlan is the formal compilation of one sealed
// TypestateResourceQuery. It selects the least registered path-evidence
// component incident to the receiver and retains the typestate lane only long
// enough to project the query result. Providers never receive either lane.
type TypestateResourceFormalQueryPlan struct {
	query      TypestateResourceQuery
	projection CoordinateFormalPublicationProjection
	typestate  ProductLane
	path       CoordinateFormalBoundaryFactorPlan
}

// SealTypestateResourceFormalQueryPlan compiles the same query program used by
// concrete execution against one point-owned formal publication boundary.
func (d ProductDomain) SealTypestateResourceFormalQueryPlan(
	query TypestateResourceQuery,
	projection CoordinateFormalPublicationProjection,
) (TypestateResourceFormalQueryPlan, error) {
	if !query.ValidFor(d) || !d.OwnsCoordinateFormalPublicationProjection(projection) ||
		query.capability.keys != projection.inverse.to {
		return TypestateResourceFormalQueryPlan{}, fmt.Errorf("%w: foreign formal typestate resource query", ErrInvalidLaneFactor)
	}
	pathFamily, ok := d.PathEvidenceCoordinateFamily()
	if !ok || pathFamily.Lane() != query.capability.path {
		return TypestateResourceFormalQueryPlan{}, fmt.Errorf("%w: formal typestate query has no path-evidence family", ErrInvalidLaneFactor)
	}
	pathRuntime, err := d.validateLane(query.capability.path)
	if err != nil || len(pathRuntime.coordinates) == 0 {
		return TypestateResourceFormalQueryPlan{}, fmt.Errorf("%w: formal typestate query path lane is not coordinate-factored", ErrInvalidLaneFactor)
	}
	var pathCoordinate *coordinateFamilyRuntime
	for index := range pathRuntime.coordinates {
		if pathRuntime.coordinates[index].family == pathFamily {
			pathCoordinate = &pathRuntime.coordinates[index]
			break
		}
	}
	if pathCoordinate == nil {
		return TypestateResourceFormalQueryPlan{}, fmt.Errorf("%w: formal typestate query family is absent from path lane", ErrInvalidLaneFactor)
	}
	sourceSlots, err := projection.selection.coordinates.familySlots(pathFamily)
	if err != nil {
		return TypestateResourceFormalQueryPlan{}, err
	}
	mappedSlots := make([]CoordinateSlot, len(sourceSlots))
	for index, slot := range sourceSlots {
		mapped, mappedOK := pathCoordinate.ops.formalRekey.key(slot.key, projection.inverse)
		if !mappedOK || mapped == nil || !pathCoordinate.ops.keyValid(mapped, projection.inverse.to) {
			return TypestateResourceFormalQueryPlan{}, fmt.Errorf("%w: formal typestate query coordinate is not publishable", ErrInvalidLaneFactor)
		}
		mappedSlots[index] = CoordinateSlot{family: pathFamily, keys: projection.inverse.to, key: mapped}
	}
	target, ok := query.capability.keys.FromStateKey(query.target.PathKey())
	if !ok {
		return TypestateResourceFormalQueryPlan{}, fmt.Errorf("%w: formal typestate query target is unresolved", ErrInvalidLaneFactor)
	}
	const queryDependency CoordinateDependencyID = 1
	dependencies, err := d.PlanPathCoordinateDependencies(
		query.capability.keys,
		mappedSlots,
		[]CoordinateDependencySeed{{ID: queryDependency, ResolvePaths: []keyspace.Key{target}}},
	)
	if err != nil {
		return TypestateResourceFormalQueryPlan{}, fmt.Errorf("%w: formal typestate query cone: %v", ErrInvalidLaneFactor, err)
	}
	dependency, ok := dependencies.Dependency(queryDependency)
	if !ok {
		return TypestateResourceFormalQueryPlan{}, fmt.Errorf("%w: formal typestate query dependency is absent", ErrInvalidLaneFactor)
	}
	reads := dependency.CoordinateReads()
	selected := make([]bool, len(sourceSlots))
	for index, mapped := range mappedSlots {
		for _, read := range reads {
			if read.family == mapped.family && read.keys == mapped.keys && pathCoordinate.ops.keyEqual(read.key, mapped.key) {
				selected[index] = true
				break
			}
		}
	}
	pathPlan := CoordinateFormalBoundaryFactorPlan{
		seal: d.seal, projection: projection, lane: query.capability.path,
		families: make([]coordinateFormalBoundaryFamilyPlan, len(pathRuntime.coordinates)),
	}
	for familyIndex, coordinate := range pathRuntime.coordinates {
		pathPlan.families[familyIndex].family = coordinate.family
		if coordinate.family != pathFamily {
			continue
		}
		for index, slot := range sourceSlots {
			if selected[index] {
				pathPlan.families[familyIndex].slots = append(pathPlan.families[familyIndex].slots, slot)
			}
		}
	}
	if !pathPlan.validFor(d) {
		return TypestateResourceFormalQueryPlan{}, fmt.Errorf("%w: formal typestate query path plan did not seal", ErrInvalidLaneFactor)
	}
	return TypestateResourceFormalQueryPlan{
		query: query, projection: projection, typestate: query.capability.typestate, path: pathPlan,
	}, nil
}

func (p TypestateResourceFormalQueryPlan) ValidFor(d ProductDomain) bool {
	return p.query.ValidFor(d) && p.typestate == p.query.capability.typestate && p.path.validFor(d) &&
		d.OwnsCoordinateFormalPublicationProjection(p.projection)
}

func (p TypestateResourceFormalQueryPlan) TypestateLane() ProductLane { return p.typestate }

func (p TypestateResourceFormalQueryPlan) PathFamilyLayouts() []CoordinateFormalBoundaryFamilyLayout {
	return p.path.FamilyLayouts()
}

// Observe executes the canonical query after state-owned formal publication.
func (d ProductDomain) ObserveFormalTypestateResourceQuery(
	plan TypestateResourceFormalQueryPlan,
	typestateFactor LaneFactor,
	pathFamilies []CoordinateFormalBoundaryFamilyOperands,
) (TypestateResourceObservation, error) {
	if !plan.ValidFor(d) {
		return TypestateResourceObservation{}, fmt.Errorf("%w: foreign formal typestate query plan", ErrInvalidLaneFactor)
	}
	typestateFactor, err := d.RekeyOrdinaryLaneFactorFormalPublication(plan.projection, typestateFactor)
	if err != nil {
		return TypestateResourceObservation{}, err
	}
	pathFactor, err := d.ApplyCoordinateFormalBoundaryFactorPlan(plan.path, pathFamilies)
	if err != nil {
		return TypestateResourceObservation{}, err
	}
	return plan.query.ObserveFactors(d, typestateFactor, pathFactor)
}
