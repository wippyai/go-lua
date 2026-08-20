package composite

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/observation"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// The observation inventory is the declaration half of the two live
// observation producers. Its rows name existing query families and generated
// denominator relations; no runtime attachment or result value crosses this
// boundary.
const (
	ObservationBranchValueSummary      schema.Key = "value-summary/branch-condition"
	ObservationConformanceValueSummary schema.Key = "value-summary/type-conformance"
)

// These semantic roles are owned by the composite declaration. The generic
// observation surface carries only structural references to them; it does not
// know what a branch, direct-allocation, or publication geometry means.
const (
	observationGeometryBranchRole      = "observation/geometry/branch-evidence"
	observationGeometryConformanceRole = "observation/geometry/type-conformance-evidence"
	observationAnchorEvidenceRole      = "observation/anchor/evidence-point"
)

const (
	observationRelationBranchCondition schema.Key = "ProgramFlowControl@-"
	observationRelationTypeConformance schema.Key = "ProgramFlowCall@-"
)

func observationRoleVocabulary() []structure.Spec {
	return vocabulary.RoleSpecs(
		observationGeometryBranchRole,
		observationGeometryConformanceRole,
		observationAnchorEvidenceRole,
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
	for _, relation := range [...]schema.Key{observationRelationBranchCondition, observationRelationTypeConformance} {
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
			// The type-conformance population is measured on the same value
			// summary as a branch condition and at the same kind of address: the
			// rule occurrence that produces the measured value. It quantifies
			// over the call relation rather than the control relation, so it is
			// its own row rather than a second population on the branch row.
			Key:      ObservationConformanceValueSummary,
			Producer: observationReference(schema.SurfaceKindQuery, QueryFamilyValueSummary),
			Population: observation.Population{
				Relation: observationReference(schema.SurfaceKindDenominator, observationRelationTypeConformance),
				Kind:     observationReference(schema.SurfaceKindStructure, structure.DiagnosticObservationTypeConformance.Key()),
			},
			Geometry: observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey(observationGeometryConformanceRole)),
			Anchor:   observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey(observationAnchorEvidenceRole)),
			Codec:    observationReference(schema.SurfaceKindStructure, vocabulary.RoleKey("query-result/value-summary")),
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

// ProducedValueAxes is the declared axis set every produced-value observation
// reads its measured value from: the subjects of the query family each sealed
// observation names as its producer.
//
// A consumer that holds the rule placements of one occurrence separates the
// ones that establish an observed value from the ones that establish another
// domain's result by this set. The chain is declared end to end - population
// names its producing query family, the family names the axes it reads, a rule
// names the axis it writes - so nothing here is a name this package chose.
func ProducedValueAxes() ([]schema.Key, bool) {
	sealRegistry()
	if registry.sealed == nil {
		return nil, false
	}
	subjects := make(map[schema.Key]struct{})
	for _, issued := range ObservationIssuance() {
		if !issued.Producer.Available() {
			continue
		}
		registration, found := queryRegistrationFor(issued.Producer)
		if !found {
			return nil, false
		}
		for _, subject := range registration.Subjects() {
			if !subject.Available() {
				return nil, false
			}
			subjects[subject] = struct{}{}
		}
	}
	if len(subjects) == 0 {
		return nil, false
	}
	axes := make([]schema.Key, 0, len(subjects))
	for subject := range subjects {
		axes = append(axes, subject)
	}
	sort.Slice(axes, func(left, right int) bool { return axes[left] < axes[right] })
	return axes, true
}

// queryRegistrationFor resolves one sealed query family by the key its
// declaration is identified by.
func queryRegistrationFor(family schema.Key) (*query.Registration, bool) {
	for _, registration := range registry.queries {
		if registration != nil && registration.Key() == family {
			return registration, true
		}
	}
	return nil, false
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
