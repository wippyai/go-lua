package composite

import (
	analysiscatalog "github.com/wippyai/go-lua/analysis/catalog"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	callsite "github.com/wippyai/go-lua/domain/effect/callsite"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementquery "github.com/wippyai/go-lua/domain/placement/query"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// ProgramBinding is the one sealed Link-local hot binding for a Compilation's
// reusable Program schema. Compile constructs it once; repeated and concurrent
// Plan solves reuse its immutable typed cells while creating independent
// solve-local topology/runtime transactions.
//
// The rule plane is not enumerated here: the binding transaction drives the
// sealed rule table and this record retains only its neutral hot projection.
type ProgramBinding struct {
	compilation Compilation
	catalog     *catalog
	binding     *engine.SchemaBinding
	publication analysiscatalog.Publication

	// rules is the sealed rule table's Link-local projection. Every rule
	// admission, attachment, and classification goes through it.
	rules *RuleBinding

	// allocations is the Link-owned allocation directory admitted with this
	// binding.
	allocations *allocationcatalog.Catalog
	placement   placementdomain.Schema

	value *valueowner.HotOwner

	// queries holds every declared query family's sealed implementation at its
	// slot, opaque here and recovered at its type by the accessor the family's
	// own consumers read it through.
	queries queryCells
}

// Available states that this binding completed its transaction and sealed.
func (bound *ProgramBinding) Available() bool {
	return bound != nil && bound.binding != nil && bound.binding.Sealed() &&
		bound.compilation.Available() && bound.catalog != nil && bound.rules != nil && bound.allocations != nil &&
		bound.placement.Valid()
}

// SchemaBinding is the one sealed engine binding this transaction produced.
func (bound *ProgramBinding) SchemaBinding() *engine.SchemaBinding {
	if bound == nil {
		return nil
	}
	return bound.binding
}

// Compilation returns the immutable declaration state that admitted this binding.
// Consumers carry it when they need a declaration-derived projection after
// the binding transaction; no unrelated declaration state is consulted.
func (bound *ProgramBinding) Compilation() Compilation {
	if bound == nil {
		return Compilation{}
	}
	return bound.compilation
}

// Publication is the immutable snapshot column plan carried by this binding's
// compilation. Consumers use it to address declared columns; they do not
// rediscover a plan from another composition.
func (bound *ProgramBinding) Publication() (analysiscatalog.Publication, bool) {
	if bound == nil || !bound.publication.Available() {
		return analysiscatalog.Publication{}, false
	}
	return bound.publication, true
}

// Rules is the hot projection of the sealed rule table.
func (bound *ProgramBinding) Rules() *RuleBinding {
	if bound == nil {
		return nil
	}
	return bound.rules
}

// ValueSchema is the sealed value schema this binding's principal carries. The
// principal itself stays inside the binding.
func (bound *ProgramBinding) ValueSchema() *valuedomain.Schema {
	if bound == nil || bound.value == nil {
		return nil
	}
	return bound.value.Schema()
}

// PlacementSchema returns the exact Link-bound Placement authority used by
// this program's rules and query encoder. Detached result consumers must carry
// this authority explicitly; a payload schema ID alone cannot authorize a
// schema-bound decode.
func (bound *ProgramBinding) PlacementSchema() (placementdomain.Schema, bool) {
	if bound == nil || !bound.placement.Valid() {
		return placementdomain.Schema{}, false
	}
	return bound.placement, true
}

// Query recovers the sealed implementation cell of one issued family. The
// cell stays opaque here; the caller recovers it at the type the family's
// own contributor declared.
func (bound *ProgramBinding) Query(family schema.Key) (query.Cell, bool) {
	if bound == nil || !family.Available() {
		return query.Cell{}, false
	}
	position, ok := queryPositionForFamily(bound.catalog, family)
	if !ok || position < 0 || position >= len(bound.queries) {
		return query.Cell{}, false
	}
	cell := bound.queries[position]
	return cell, cell.Available()
}

// ValueQuery is the sealed implementation of the value-summary family. The
// fragment remains the canonical row; the implementation is projected from
// that row against this exact sealed binding.
func (bound *ProgramBinding) ValueQuery() *valueowner.SummaryQueryImplementation {
	cell, ok := bound.Query(QueryFamilyValueSummary)
	if !ok {
		return nil
	}
	fragment, present := query.Payload[*valueowner.SummaryQueryFragment](cell)
	if !present {
		return nil
	}
	implementation, recovered := valueowner.RecoverQuery(bound.binding, query.Sealed[*valueowner.SummaryQueryFragment]{Fragment: fragment})
	if !recovered {
		return nil
	}
	return implementation
}

// EffectQuery is the sealed implementation of the effect-exact family.
func (bound *ProgramBinding) EffectQuery() *effectowner.ExactQueryImplementation {
	cell, ok := bound.Query(QueryFamilyEffectExact)
	if !ok {
		return nil
	}
	fragment, present := query.Payload[*effectowner.ExactQueryFragment](cell)
	if !present {
		return nil
	}
	implementation, recovered := effectowner.RecoverQuery(bound.binding, query.Sealed[*effectowner.ExactQueryFragment]{Fragment: fragment})
	if !recovered {
		return nil
	}
	return implementation
}

// EffectPublicationObservations walks the canonical mounted Program
// occurrences and directly asks Effect's selected callsite rule for each
// declared publication observation. No candidate or post-solve proof view is
// retained in Composite; Effect owns the observation's construction.
func (bound *ProgramBinding) EffectPublicationObservations(committed *engine.CommittedProgram, mounts []programmount.MountedArtifact) ([]engine.ProgramObservationAdmission, bool) {
	if bound == nil || !bound.Available() || committed == nil || len(mounts) == 0 {
		return nil, false
	}
	query := bound.EffectQuery()
	if query == nil {
		return nil, false
	}
	cell, cellOK := bound.rules.cellByKey("effect-selected")
	selected, selectedOK := rule.Payload[*callsite.HotRule](cell)
	if !cellOK || !selectedOK || selected == nil {
		return nil, false
	}
	observations := make([]engine.ProgramObservationAdmission, 0)
	seen := make(map[identity.ContentID]struct{})
	_, walked := WalkSealedPlacements(mounts, func(ruleKey schema.Key, mount, _, occurrence identity.ContentID) bool {
		if ruleKey != "effect-selected" {
			return true
		}
		admission, present, observationOK := selected.MountedPublicationObservation(committed, query, mount, occurrence)
		if !observationOK {
			return false
		}
		if !present {
			return true
		}
		if !admission.Available() {
			return false
		}
		if _, duplicate := seen[admission.ID]; duplicate {
			return false
		}
		seen[admission.ID] = struct{}{}
		observations = append(observations, admission)
		return true
	})
	if !walked {
		return nil, false
	}
	return observations, true
}

// PlacementQuery is the sealed implementation of the placement-summary
// family.
func (bound *ProgramBinding) PlacementQuery() *engine.HeterogeneousQueryImplementation[placementdomain.PlacementSummaryObservation] {
	cell, ok := bound.Query(QueryFamilyPlacementSummary)
	if !ok {
		return nil
	}
	fragment, present := query.Payload[*placementquery.SummaryQueryFragment](cell)
	if !present {
		return nil
	}
	implementation, recovered := placementquery.RecoverQuery(bound.binding, query.Sealed[*placementquery.SummaryQueryFragment]{Fragment: fragment})
	if !recovered {
		return nil
	}
	return implementation
}

// QueryAdmission seals one selected-point query row from the family's own
// implementation. Construction walks sealed issuance for sites; the sealed
// family selects its owner's admission callback. Projection is descriptive
// query geometry and is never used as a concrete implementation type tag.
func (bound *ProgramBinding) QueryAdmission(id, mount, point identity.ContentID, family schema.Key) (engine.ProgramQueryAdmission, bool) {
	if bound == nil {
		return engine.ProgramQueryAdmission{}, false
	}
	position, ok := queryPositionForFamily(bound.catalog, family)
	if !ok || position < 0 || position >= len(bound.catalog.queryContributors) {
		return engine.ProgramQueryAdmission{}, false
	}
	cell, ok := bound.Query(family)
	if !ok {
		return engine.ProgramQueryAdmission{}, false
	}
	return bound.catalog.queryContributors[position].admit(bound.binding, cell, id, mount, point)
}

// BindProgram binds the complete compilation-owned schema in one SchemaBinding. The
// caller supplies the one immutable compilation handle obtained at the
// composition root and the record the mount phase produced; the factor
// principals, allocation catalog, rules, and query families are admitted by the
// grammar's own transaction.
func BindProgram(compilation Compilation, inputs LinkInputs) (*ProgramBinding, BindFailure) {
	state := compilation.catalog
	publication, publicationOK := compilation.Publication()
	if state == nil || !publicationOK {
		return nil, BindFailure{Stage: BindStageCompilation}
	}
	bound, failure := bind(compilation, inputs)
	if failure.Available() {
		return nil, failure
	}
	return &ProgramBinding{
		compilation: compilation,
		catalog:     state,
		binding:     bound.binding,
		publication: publication,
		rules:       bound.rules,
		allocations: bound.allocations,
		placement:   inputs.PlacementSchema,
		value:       bound.value,
		queries:     bound.queries,
	}, BindFailure{}
}
