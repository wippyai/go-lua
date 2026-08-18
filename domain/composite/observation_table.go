package composite

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/observation"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// The observation inventory is the declaration half of the three live
// observation producers. Its rows name existing query families and generated
// denominator relations; no runtime attachment or result value crosses this
// boundary.
const (
	ObservationBranchValueSummary         schema.Key = "value-summary/branch-condition"
	ObservationDirectAllocationMembership schema.Key = "value-summary/direct-allocation-membership"
	ObservationPublicationTransition      schema.Key = "effect-exact/publication-transition"
)

// These semantic roles are owned by the composite declaration. The generic
// observation surface carries only structural references to them; it does not
// know what a branch, direct-allocation, or publication geometry means.
const (
	observationGeometryBranchRole      = "observation/geometry/branch-evidence"
	observationGeometryDirectRole      = "observation/geometry/direct-allocation-membership"
	observationGeometryPublicationRole = "observation/geometry/publication-transition"
	observationAnchorEvidenceRole      = "observation/anchor/evidence-point"
	observationAnchorCallEffectRole    = "observation/anchor/selected-call-effect"
	observationAnchorPublicationRole   = "observation/anchor/publication-effect"
)

const (
	observationRelationBranchCondition       schema.Key = "ProgramFlowControl@-"
	observationRelationDirectAllocation      schema.Key = "TargetOperation@TargetOperationEffect"
	observationRelationPublicationTransition schema.Key = "TargetOperation@TargetPublicationEffect"
)

func observationRoleVocabulary() []structure.Spec {
	return vocabulary.RoleSpecs(
		observationGeometryBranchRole,
		observationGeometryDirectRole,
		observationGeometryPublicationRole,
		observationAnchorEvidenceRole,
		observationAnchorCallEffectRole,
		observationAnchorPublicationRole,
	)
}

func observationReference(kind schema.SurfaceKind, key schema.Key) observation.Reference {
	return observation.Reference{Surface: kind, Key: key}
}

// observationSpecs derives the producer inventory from the admitted query
// rows. A missing producer is a composition failure; omitting its observation
// would leave the runtime producer without a declared result family.
func observationSpecs(queries []*query.Registration) ([]observation.Spec, bool) {
	seen := make(map[schema.Key]bool, len(queries))
	for _, registration := range queries {
		if registration == nil || !registration.Key().Available() || seen[registration.Key()] {
			return nil, false
		}
		seen[registration.Key()] = true
	}
	if !seen[QueryFamilyValueSummary] || !seen[QueryFamilyEffectExact] {
		return nil, false
	}
	for _, relation := range [...]schema.Key{
		observationRelationBranchCondition,
		observationRelationDirectAllocation,
		observationRelationPublicationTransition,
	} {
		if _, declared := denominator.GeneratedRelationByKey(relation); !declared {
			return nil, false
		}
	}
	return []observation.Spec{
		{
			Key:      ObservationBranchValueSummary,
			Producer: observationReference(schema.SurfaceKindQuery, QueryFamilyValueSummary),
			Population: observation.Population{
				Relation: observationReference(schema.SurfaceKindDenominator, observationRelationBranchCondition),
				Kind:     observationReference(schema.SurfaceKindStructure, structure.DiagnosticObservationBranchCondition.Key()),
			},
			Geometry: observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey(observationGeometryBranchRole)),
			Anchor:   observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey(observationAnchorEvidenceRole)),
			Codec:    observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey("query-result/value-summary")),
		},
		{
			Key:      ObservationDirectAllocationMembership,
			Producer: observationReference(schema.SurfaceKindQuery, QueryFamilyValueSummary),
			Population: observation.Population{
				Relation: observationReference(schema.SurfaceKindDenominator, observationRelationDirectAllocation),
			},
			Geometry: observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey(observationGeometryDirectRole)),
			Anchor:   observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey(observationAnchorCallEffectRole)),
			Codec:    observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey("query-result/value-summary")),
		},
		{
			Key:      ObservationPublicationTransition,
			Producer: observationReference(schema.SurfaceKindQuery, QueryFamilyEffectExact),
			Population: observation.Population{
				Relation: observationReference(schema.SurfaceKindDenominator, observationRelationPublicationTransition),
			},
			Geometry: observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey(observationGeometryPublicationRole)),
			Anchor:   observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey(observationAnchorPublicationRole)),
			Codec:    observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey("query-result/effect-exact")),
		},
	}, true
}

// observationEntries admits the derived observation inventory. Relations are
// resolved by the observation surface against the generated denominator
// catalog; this helper only turns declaration data into rows.
func observationEntries(queries []*query.Registration) ([]*observation.Entry, bool) {
	specs, specsOK := observationSpecs(queries)
	if !specsOK {
		return nil, false
	}
	entries := make([]*observation.Entry, 0, len(specs))
	for _, spec := range specs {
		entry, ok := observation.New(spec)
		if !ok {
			return nil, false
		}
		entries = append(entries, entry)
	}
	return entries, true
}

// IssuedObservation is one sealed observation's construction handle: the
// authored identity, the query family that produces it, and the population,
// geometry, anchor, and codec it was sealed under.
type IssuedObservation struct {
	Key        schema.Key
	Producer   schema.Key
	Population observation.Population
	Geometry   schema.Key
	Anchor     schema.Key
	Codec      schema.Key
}

// ObservationIssuance returns the sealed observation inventory in catalog
// order.
func ObservationIssuance() []IssuedObservation {
	sealRegistry()
	if !registry.observations.Available() {
		return nil
	}
	issued := make([]IssuedObservation, 0, registry.observations.Count())
	for position := 0; position < registry.observations.Count(); position++ {
		entry, ok := registry.observations.At(position)
		if !ok || entry == nil {
			continue
		}
		issued = append(issued, IssuedObservation{
			Key:        entry.Key(),
			Producer:   entry.Producer().Key,
			Population: entry.Population(),
			Geometry:   entry.Geometry().Key,
			Anchor:     entry.Anchor().Key,
			Codec:      entry.Codec().Key,
		})
	}
	return issued
}

// ObservationProducerForPopulationKind returns the query family that produces
// the sealed observation whose population kind is kind.
func ObservationProducerForPopulationKind(kind schema.Key) (schema.Key, bool) {
	if !kind.Available() {
		return "", false
	}
	for _, issued := range ObservationIssuance() {
		if issued.Population.Kind.Key == kind && issued.Producer.Available() {
			return issued.Producer, true
		}
	}
	return "", false
}
