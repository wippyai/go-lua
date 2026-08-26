package composite

import (
	analysiscatalog "github.com/wippyai/go-lua/analysis/catalog"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/query"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	callsite "github.com/wippyai/go-lua/domain/effect/callsite"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	contextdomain "github.com/wippyai/go-lua/domain/heap/context"
	contextowner "github.com/wippyai/go-lua/domain/heap/context/owner"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementquery "github.com/wippyai/go-lua/domain/placement/query"
	"github.com/wippyai/go-lua/domain/sendsafety"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	staticowner "github.com/wippyai/go-lua/domain/static/owner"
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
	rules   *RuleBinding
	factors map[schema.Key]engine.FactorSlotCapability

	// allocations is the Link-owned allocation directory admitted with this
	// binding.
	allocations   *allocationcatalog.Catalog
	placement     placementdomain.Schema
	contextSchema contextdomain.Schema
	composition   modulecomposition.Composition

	// The mount phase seals one authority per axis into the Link record this
	// binding is built from. They are retained here so the axis a consumer
	// needs is answered by the record that sealed it, and never sealed twice.
	heapSchema      heapdomain.Schema
	packSchema      *packdomain.Schema
	effectAlgebra   *effectfactor.Algebra
	callAlgebra     *calldomain.Algebra
	staticAuthority *staticdomain.Authority
	topology        *heapindex.Topology

	value      *valueowner.HotOwner
	staticType *staticowner.HotOwner
	context    *contextowner.HotOwner
	call       *callowner.HotOwner
	effect     *effectowner.HotOwner

	// queries holds every declared query family's sealed implementation at its
	// slot, opaque here and recovered at its type by the accessor the family's
	// own consumers read it through.
	queries queryCells
}

// Available states that this binding completed its transaction and sealed.
func (bound *ProgramBinding) Available() bool {
	return bound != nil && bound.binding != nil && bound.binding.Sealed() &&
		bound.compilation.Available() && bound.catalog != nil && bound.rules != nil && bound.factors != nil && bound.allocations != nil &&
		bound.placement.Valid() && bound.context != nil && bound.call != nil && bound.effect != nil && bound.contextSchema.Valid() && bound.contextSchema.Heap() == bound.placement.Heap() &&
		bound.composition.Available() && bound.composition.LinkID() == bound.contextSchema.Directory().LinkID()
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

// FactorCapability resolves an axis directly to the exact sealed Factor slot
// it owns. It is the mount substitution for local transfers; Rule identities
// never stand in for Factor authority.
func (bound *ProgramBinding) FactorCapability(axis schema.Key) (engine.FactorSlotCapability, bool) {
	if bound == nil || !axis.Available() {
		return engine.FactorSlotCapability{}, false
	}
	capability, ok := bound.factors[axis]
	return capability, ok && capability.Available()
}

// ValueSchema is the sealed value schema this binding's principal carries. The
// principal itself stays inside the binding.
func (bound *ProgramBinding) ValueSchema() *valuedomain.Schema {
	if bound == nil || bound.value == nil {
		return nil
	}
	return bound.value.Schema()
}

// HeapSchema returns the sealed Heap authority the mount phase placed in this
// binding's Link record. The second result separates a record whose Heap mount
// refused from a valid schema.
func (bound *ProgramBinding) HeapSchema() (heapdomain.Schema, bool) {
	if bound == nil || !bound.heapSchema.Valid() {
		return heapdomain.Schema{}, false
	}
	return bound.heapSchema, true
}

// PackSchema returns the sealed Pack authority carried by the same record.
func (bound *ProgramBinding) PackSchema() *packdomain.Schema {
	if bound == nil {
		return nil
	}
	return bound.packSchema
}

// EffectAlgebra returns the sealed Effect factor algebra carried by the same
// record. It is the ascent authority of the Effect axis; EffectAuthority
// remains the owner-fenced hot surface over it.
func (bound *ProgramBinding) EffectAlgebra() *effectfactor.Algebra {
	if bound == nil || bound.effectAlgebra == nil || !bound.effectAlgebra.Valid() {
		return nil
	}
	return bound.effectAlgebra
}

// CallAlgebra returns the sealed Call algebra carried by the same record. It
// is the ascent authority of the Call axis; CallAuthority remains the
// owner-fenced hot surface over it.
func (bound *ProgramBinding) CallAlgebra() *calldomain.Algebra {
	if bound == nil || bound.callAlgebra == nil || !bound.callAlgebra.Valid() {
		return nil
	}
	return bound.callAlgebra
}

// StaticClasses returns the class set of the sealed static inventory the mount
// phase admitted. The inventory itself stays inside the binding.
func (bound *ProgramBinding) StaticClasses() *staticdomain.ClassSet {
	if bound == nil || bound.staticAuthority == nil {
		return nil
	}
	return bound.staticAuthority.Classes()
}

// IndexTopology returns the Heap index topology the mount phase derived once
// every axis had sealed.
func (bound *ProgramBinding) IndexTopology() *heapindex.Topology {
	if bound == nil {
		return nil
	}
	return bound.topology
}

// StaticTypeAuthority returns the one Static-owned typed factor bound over the
// same Value coordinate denominator. Consumers use it to issue typed reads;
// they do not reconstruct Runtime rows from Program or diagnostic metadata.
func (bound *ProgramBinding) StaticTypeAuthority() *staticowner.HotOwner {
	if bound == nil {
		return nil
	}
	return bound.staticType
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

// ContextSchema returns the exact Link-bound contextual Heap authority used by
// this program binding. Contextual consumers must carry this value; a Heap
// schema or Context ID alone cannot authorize a contextual reference.
func (bound *ProgramBinding) ContextSchema() (contextdomain.Schema, bool) {
	if bound == nil || !bound.Available() {
		return contextdomain.Schema{}, false
	}
	return bound.contextSchema, true
}

// ContextAuthority returns the exact owner-fenced Context Factor authority.
// Callers that need factor coordinates use this owner surface rather than
// rebuilding a Factor binding from ContextSchema.
func (bound *ProgramBinding) ContextAuthority() *contextowner.HotOwner {
	if bound == nil || bound.context == nil {
		return nil
	}
	return bound.context
}

// CallAuthority returns the exact owner-fenced Call Factor authority. It is
// the channel a caller uses to reach a Call/Effect two-owner fold's free
// accessors alongside a capability resolved through Rules().CapabilityByKey -
// never through a cast to a retained hot rule payload.
func (bound *ProgramBinding) CallAuthority() *callowner.HotOwner {
	if bound == nil {
		return nil
	}
	return bound.call
}

// EffectAuthority returns the exact owner-fenced Effect Factor authority. See
// CallAuthority.
func (bound *ProgramBinding) EffectAuthority() *effectowner.HotOwner {
	if bound == nil {
		return nil
	}
	return bound.effect
}

// ModuleComposition returns the one immutable catalog derived before this
// binding sealed. Publication and domain consumers share this exact owner.
func (bound *ProgramBinding) ModuleComposition() (modulecomposition.Composition, bool) {
	if bound == nil || !bound.Available() || !bound.composition.Available() {
		return modulecomposition.Composition{}, false
	}
	return bound.composition, true
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
// retained in Composite; Effect owns the observation's construction. The
// supplied directory must carry Context rows from this binding's exact
// Link-owned contextual authority.
func (bound *ProgramBinding) EffectPublicationObservations(committed *engine.CommittedProgram, mounts []programmount.MountedArtifact, contexts executioncontext.Directory) ([]engine.ProgramObservationAdmission, bool) {
	if bound == nil || !bound.Available() || committed == nil || len(mounts) == 0 || !contexts.Available() {
		return nil, false
	}
	boundContexts := bound.contextSchema.Directory()
	if !boundContexts.Available() || contexts.LinkID() != boundContexts.LinkID() {
		return nil, false
	}
	for index := 0; index < contexts.ContextCount(); index++ {
		context, contextOK := contexts.ContextAt(index)
		if !contextOK || !bound.contextSchema.OwnsContext(context) {
			return nil, false
		}
	}
	query := bound.EffectQuery()
	if query == nil {
		return nil, false
	}
	capability, capabilityOK := bound.rules.CapabilityByKey("effect-selected")
	if !capabilityOK {
		return nil, false
	}
	observations := make([]engine.ProgramObservationAdmission, 0)
	seen := make(map[identity.ContentID]struct{})
	_, walked := WalkSealedPlacements(mounts, func(ruleKey schema.Key, mount, _, occurrence identity.ContentID) bool {
		if ruleKey != "effect-selected" {
			return true
		}
		mountedContexts, contextsOK := contexts.ContextsForModule(mount)
		if !contextsOK {
			return false
		}
		for _, context := range mountedContexts {
			if !bound.contextSchema.OwnsContext(context) {
				return false
			}
			admission, present, observationOK := callsite.MountedPublicationObservation(bound.binding, bound.call, bound.effect, capability, committed, query, mount, occurrence, context)
			if !observationOK {
				return false
			}
			if !present {
				continue
			}
			if !admission.Available() {
				return false
			}
			if _, duplicate := seen[admission.ID]; duplicate {
				return false
			}
			seen[admission.ID] = struct{}{}
			observations = append(observations, admission)
		}
		return true
	})
	if !walked {
		return nil, false
	}
	return observations, true
}

// SendSafetyObservations admits the Value and Placement summaries needed to
// decide typed send publications. Both rows read the authenticated input of
// the Call-effect stage, so the current send's own escape transition cannot
// alter the decision about that send.
func (bound *ProgramBinding) SendSafetyObservations(committed *engine.CommittedProgram, mounts []programmount.MountedArtifact, contexts executioncontext.Directory) ([]SendSafetyObservation, bool) {
	if bound == nil || !bound.Available() || committed == nil || len(mounts) == 0 || !contexts.Available() {
		return nil, false
	}
	boundContexts := bound.contextSchema.Directory()
	placementQuery := bound.PlacementQuery()
	valueQuery := bound.ValueQuery()
	capability, capabilityOK := bound.rules.CapabilityByKey("effect-selected")
	if !boundContexts.Available() || contexts.LinkID() != boundContexts.LinkID() || placementQuery == nil || valueQuery == nil || !capabilityOK {
		return nil, false
	}
	observations := make([]SendSafetyObservation, 0)
	seen := make(map[identity.ContentID]struct{})
	_, walked := WalkSealedPlacements(mounts, func(ruleKey schema.Key, mount, _, occurrence identity.ContentID) bool {
		if ruleKey != "effect-selected" {
			return true
		}
		mountedContexts, contextsOK := contexts.ContextsForModule(mount)
		if !contextsOK {
			return false
		}
		stage, publications, publicationsOK := callsite.MountedPublicationBatchStage(bound.binding, bound.call, bound.effect, capability, committed, mount, occurrence)
		if !publicationsOK || !stage.Available() || !stage.InputPointID().Available() {
			return false
		}
		for _, context := range mountedContexts {
			if !bound.contextSchema.OwnsContext(context) {
				return false
			}
			placementID, present, placementIDOK := sendsafety.PlacementObservationID(publications, context)
			valueID, valuePresent, valueIDOK := sendsafety.ValueObservationID(publications, context)
			if !placementIDOK || !valueIDOK || present != valuePresent {
				return false
			}
			if !present {
				continue
			}
			placementAdmission, placementAdmitted := engine.NewHeterogeneousCallInputObservationAdmission(placementQuery, placementID, stage, context)
			valueAdmission, valueAdmitted := engine.NewSummaryCallInputObservationAdmission(valueQuery, valueID, stage, context)
			if !placementAdmitted || !valueAdmitted || !placementAdmission.Available() || !valueAdmission.Available() {
				return false
			}
			if _, duplicate := seen[placementID]; duplicate {
				return false
			}
			if _, duplicate := seen[valueID]; duplicate {
				return false
			}
			seen[placementID] = struct{}{}
			seen[valueID] = struct{}{}
			observation := SendSafetyObservation{batch: publications, context: context, point: stage.InputPointID(), placement: placementAdmission, value: valueAdmission}
			if !observation.Available() {
				return false
			}
			observations = append(observations, observation)
		}
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
func (bound *ProgramBinding) QueryAdmission(id, mount, point identity.ContentID, family schema.Key, context executioncontext.Context) (engine.ProgramQueryAdmission, bool) {
	if bound == nil || !context.Available() || context.ModuleKey() != mount {
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
	return bound.catalog.queryContributors[position].admit(bound.binding, cell, id, mount, point, context)
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
	factors := make(map[schema.Key]engine.FactorSlotCapability)
	for _, entry := range state.axes {
		if entry == nil || !entry.Storage().Bound() {
			continue
		}
		semantic, semanticOK := state.roles.Key(entry.Semantic())
		capability, capabilityOK := engine.FactorCapabilityForSemantic(bound.binding, semantic)
		if !semanticOK || !capabilityOK {
			return nil, BindFailure{Stage: BindStagePrincipal}
		}
		factors[entry.Key()] = capability
	}
	return &ProgramBinding{
		compilation:   compilation,
		catalog:       state,
		binding:       bound.binding,
		publication:   publication,
		rules:         bound.rules,
		factors:       factors,
		allocations:   bound.allocations,
		placement:     inputs.PlacementSchema,
		contextSchema: inputs.contextSchema,
		composition:   inputs.composition,

		heapSchema:      inputs.HeapSchema,
		packSchema:      inputs.PackSchema,
		effectAlgebra:   inputs.EffectAlgebra,
		callAlgebra:     inputs.CallAlgebra,
		staticAuthority: inputs.StaticAuthority,
		topology:        inputs.topology,
		value:           bound.value,
		staticType:      bound.staticType,
		context:         bound.context,
		call:            bound.call,
		effect:          bound.effect,
		queries:         bound.queries,
	}, BindFailure{}
}
