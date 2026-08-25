package composite

import (
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/analysis/schema/modulecomposition"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/domain/effect/owner"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	allocationcatalog "github.com/wippyai/go-lua/domain/heap/allocation/catalog"
	contextdomain "github.com/wippyai/go-lua/domain/heap/context"
	contextowner "github.com/wippyai/go-lua/domain/heap/context/owner"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	"github.com/wippyai/go-lua/domain/heap/keymatch"
	heapowner "github.com/wippyai/go-lua/domain/heap/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	packowner "github.com/wippyai/go-lua/domain/pack/owner"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	placementsuspension "github.com/wippyai/go-lua/domain/placement/suspension"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	staticowner "github.com/wippyai/go-lua/domain/static/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// principals is the composition's cold factor principal record. It is the
// declaration surface's P parameter, so a rule's Declare hook receives its
// owners already typed and never asserts.
type principals struct {
	value      *valueowner.SchemaFragment
	staticType *staticowner.SchemaFragment
	call       *callowner.SchemaFragment
	heap       *heapowner.SchemaFragment
	placement  *placementowner.SchemaFragment
	context    *contextowner.SchemaFragment
	evidence   *placementsuspension.EvidenceFactorFragment
	pack       *packowner.SchemaFragment
	effect     *effectowner.SchemaFragment
}

func (set principals) available() bool {
	return set.value != nil && set.staticType != nil && set.call != nil && set.heap != nil && set.placement != nil && set.context != nil && set.evidence != nil && set.pack != nil && set.effect != nil
}

// The principal getters are the record's read surface. An owning domain names
// exactly the ones its own Declare hook consumes in its own need interface, so
// a rule reaches its cold owners without this record's shape reaching the
// domain.
func (set principals) ValuePrincipal() *valueowner.SchemaFragment { return set.value }

func (set principals) StaticTypePrincipal() *staticowner.SchemaFragment { return set.staticType }

func (set principals) CallPrincipal() *callowner.SchemaFragment { return set.call }

func (set principals) HeapPrincipal() *heapowner.SchemaFragment { return set.heap }

func (set principals) PlacementPrincipal() *placementowner.SchemaFragment { return set.placement }

func (set principals) ContextPrincipal() *contextowner.SchemaFragment { return set.context }

func (set principals) EvidencePrincipal() *placementsuspension.EvidenceFactorFragment {
	return set.evidence
}

func (set principals) PackPrincipal() *packowner.SchemaFragment { return set.pack }

func (set principals) EffectPrincipal() *effectowner.SchemaFragment { return set.effect }

// writes reports whether the axis a rule writes has a declared cold owner. The
// axis is named by the key its own domain declared it under, so this record
// answers for exactly the principals it carries and for no other spelling.
func (set principals) writes(key schema.Key) bool {
	switch key {
	case axisKeyValue:
		return set.value != nil
	case axisKeyStaticType:
		return set.staticType != nil
	case axisKeyCall:
		return set.call != nil
	case axisKeyHeap:
		return set.heap != nil
	case axisKeyPlacement:
		return set.placement != nil
	case axisKeyContext:
		return set.context != nil
	case axisKeyPlacementEvidence:
		return set.evidence != nil
	case axisKeyPack:
		return set.pack != nil
	case axisKeyEffect:
		return set.effect != nil
	default:
		return false
	}
}

// authorities is the composition's Link authority record. It is the surface's
// A parameter and carries every already-sealed authority a hot rule binds
// against; no runtime policy or live capability enters through it.
type authorities struct {
	value      *valueowner.HotOwner
	staticType *staticowner.HotOwner
	call       *callowner.HotOwner
	heap       *heapowner.HotOwner
	placement  *placementowner.HotOwner
	context    *contextowner.HotOwner
	evidence   *placementsuspension.EvidenceOwner
	pack       *packowner.HotOwner
	effect     *effectowner.HotOwner

	valueSchema     *valuedomain.Schema
	heapSchema      heapdomain.Schema
	placementSchema placementdomain.Schema
	packSchema      *packdomain.Schema
	// contextSchema is the one Link-local contextual Heap authority. It is
	// derived by the mount phase from the exact Link directory and mounted Heap;
	// callers never supply a contextual directory or schema alongside inputs.
	contextSchema contextdomain.Schema
	composition   modulecomposition.Composition
	topology      *heapindex.Topology
	// keySelection is the mount phase's sealed Heap key/class projection over
	// the Heap and Value pair. It is derived once and read, never rebuilt per
	// binding.
	keySelection *keymatch.SelectorProjection
	allocations  *allocationcatalog.Catalog
	// targetContract is the exact immutable Target authority retained by the
	// Link Boundary. Mounted actual geometry remains owned by Pack and is
	// authenticated directly against the exact Call rows when consumed.
	targetContract *contract.Contract
}

func (set authorities) available() bool {
	return set.value != nil && set.staticType != nil && set.call != nil && set.heap != nil && set.placement != nil && set.context != nil && set.evidence != nil && set.pack != nil && set.effect != nil &&
		set.valueSchema != nil && set.heapSchema.Valid() && set.placementSchema.Valid() && set.packSchema != nil &&
		set.contextSchema.Valid() && set.contextSchema.Heap() == set.heapSchema && set.composition.Available() && set.composition.LinkID() == set.contextSchema.Directory().LinkID() &&
		set.topology != nil && set.allocations != nil && set.targetContract != nil &&
		set.keySelection.FencedTo(set.heapSchema, set.valueSchema)
}

// writes is the sealed half of the same question: whether the axis a rule
// writes has a bound authority in this record.
func (set authorities) writes(key schema.Key) bool {
	switch key {
	case axisKeyValue:
		return set.value != nil
	case axisKeyStaticType:
		return set.staticType != nil
	case axisKeyCall:
		return set.call != nil
	case axisKeyHeap:
		return set.heap != nil
	case axisKeyPlacement:
		return set.placement != nil
	case axisKeyContext:
		return set.context != nil
	case axisKeyPlacementEvidence:
		return set.evidence != nil
	case axisKeyPack:
		return set.pack != nil
	case axisKeyEffect:
		return set.effect != nil
	default:
		return false
	}
}

// The authority getters are the record's read surface. An owning domain names
// exactly the ones its own Bind and Finalize hooks consume in its own need
// interface, so a rule reaches its sealed authorities without this record's
// shape reaching the domain.
func (set authorities) ValueAuthority() *valueowner.HotOwner { return set.value }

func (set authorities) StaticTypeAuthority() *staticowner.HotOwner { return set.staticType }

func (set authorities) CallAuthority() *callowner.HotOwner { return set.call }

func (set authorities) HeapAuthority() *heapowner.HotOwner { return set.heap }

func (set authorities) PlacementAuthority() *placementowner.HotOwner { return set.placement }

func (set authorities) ContextAuthority() *contextowner.HotOwner { return set.context }

func (set authorities) EvidenceAuthority() *placementsuspension.EvidenceOwner { return set.evidence }

func (set authorities) PackAuthority() *packowner.HotOwner { return set.pack }

func (set authorities) EffectAuthority() *effectowner.HotOwner { return set.effect }

func (set authorities) ValueSchema() *valuedomain.Schema { return set.valueSchema }

func (set authorities) HeapSchema() heapdomain.Schema { return set.heapSchema }

func (set authorities) PlacementSchema() placementdomain.Schema { return set.placementSchema }

func (set authorities) PackSchema() *packdomain.Schema { return set.packSchema }

// ContextSchema returns the exact contextual Heap authority derived for this
// Link. The zero value is returned if the authority join did not seal.
func (set authorities) ContextSchema() contextdomain.Schema {
	if !set.contextSchema.Valid() {
		return contextdomain.Schema{}
	}
	return set.contextSchema
}

func (set authorities) ModuleComposition() modulecomposition.Composition {
	if !set.composition.Available() {
		return modulecomposition.Composition{}
	}
	return set.composition
}

func (set authorities) Topology() *heapindex.Topology { return set.topology }

func (set authorities) Allocations() *allocationcatalog.Catalog { return set.allocations }

// KeySelection returns the one sealed Heap key/class projection this Link
// derived. A rule that quotients Value alternatives by what Heap observes
// reads this projection; it does not seal a second one beside it.
func (set authorities) KeySelection() *keymatch.SelectorProjection { return set.keySelection }

// TargetContract returns the exact Target contract issued by the Link
// Boundary. Consumers receive this sealed authority directly; they never
// reopen Link or substitute an equivalent reseal.
func (set authorities) TargetContract() *contract.Contract { return set.targetContract }

// LinkInputs is the neutral set of Link inputs one mount and binding
// transaction consumes: the Link itself, the neutral view of its mounted
// artifacts, the static authority no factor axis owns, the sealed factor
// authorities, and the authorities the mount phase derives once every mount has
// sealed. An authority an axis mounts for itself is written here by the mount
// phase; one whose owner has not moved is supplied by the caller. No runtime
// policy or live capability enters here, and no principal or catalog is handed
// back out.
type LinkInputs struct {
	// Source is the sealed Link every mounted authority is derived from.
	Source *link.Link
	// Artifacts is the neutral sealed artifact view, one row per Link mount, in
	// the Link's own mount order.
	Artifacts []programmount.MountedArtifact
	// StaticAuthority is the Link's sealed static inventory. It is a factor of
	// no axis, so it enters the phase as a neutral input rather than as a
	// mounted authority, and a mounting axis that seals over it names it in its
	// own need interface.
	StaticAuthority *staticdomain.Authority
	ValueSchema     *valuedomain.Schema
	CallAlgebra     *calldomain.Algebra
	HeapSchema      heapdomain.Schema
	PlacementSchema placementdomain.Schema
	PackSchema      *packdomain.Schema
	EffectAlgebra   *effectfactor.Algebra

	// vocabulary is the one sealed structural table every domain mount reads
	// through its own need interface. It is composition input, not a published
	// domain declaration state and not a second runtime-kind list.
	vocabulary structure.Table

	// topology is the mount phase's post-mount derivation over several sealed
	// factors. Activation owns and seals its own private Call projection during
	// binding, so no activation state is retained in this root record.
	topology *heapindex.Topology
	// keySelection is the mount phase's sealed Heap key/class projection. Like
	// topology it is derived, never a caller input.
	keySelection *keymatch.SelectorProjection
	// contextSchema is the mount phase's private contextual authority. It is
	// never a caller input: derive seals it from Source.ContextDirectory and
	// the mounted Heap schema after all factor axes have sealed.
	contextSchema contextdomain.Schema
	// composition is derived once after all mounted authorities seal. Domain
	// rules consume it during binding and publication later uses this exact
	// immutable owner; callers cannot inject or rebuild it.
	composition modulecomposition.Composition
	// These are mount-phase derivations. They remain private to the neutral
	// LinkInputs record and are projected into typed authorities only after all
	// factor axes have sealed.
	targetContract *contract.Contract
}

// mountable is the mount phase's admission: the Link, its artifact view, and
// the static inventory are the phase's whole input, and every mounting axis
// derives its authority from them and from its declared dependencies alone.
func (inputs LinkInputs) mountable() bool {
	if inputs.Source == nil || inputs.StaticAuthority == nil || len(inputs.Artifacts) == 0 {
		return false
	}
	for _, row := range inputs.Artifacts {
		if !row.Available() {
			return false
		}
	}
	return true
}

// neutral is the phase's own input half: what the caller supplied, with every
// mounted authority cleared. Each axis's mount receives this plus exactly the
// authorities its declared dependencies sealed, so an axis that reads a peer it
// did not declare an edge to reads nothing and rejects with its own evidence.
func (inputs LinkInputs) neutral() LinkInputs {
	return LinkInputs{Source: inputs.Source, Artifacts: inputs.Artifacts, StaticAuthority: inputs.StaticAuthority, vocabulary: inputs.vocabulary}
}

func (inputs LinkInputs) available() bool {
	return inputs.mountable() && inputs.ValueSchema != nil && inputs.CallAlgebra != nil && inputs.CallAlgebra.Valid() &&
		inputs.HeapSchema.Valid() && inputs.PlacementSchema.Valid() && inputs.PackSchema != nil &&
		inputs.EffectAlgebra != nil && inputs.EffectAlgebra.Valid() && inputs.topology != nil &&
		inputs.contextAuthorityAvailable() && inputs.composition.Available() && inputs.composition.LinkID() == inputs.Source.ContentID() &&
		inputs.targetContract != nil && mountedActualsComplete(inputs.CallAlgebra, inputs.PackSchema)
}

// contextAuthorityAvailable is the LinkInputs identity fence. The contextual
// schema must retain the exact mounted Heap issuer and the exact Link-scoped
// directory rows from Source; a content digest or caller-rebuilt directory is
// not enough to authorize this record.
func (inputs LinkInputs) contextAuthorityAvailable() bool {
	if inputs.Source == nil || !inputs.HeapSchema.Valid() || !inputs.contextSchema.Valid() ||
		inputs.contextSchema.Heap() != inputs.HeapSchema {
		return false
	}
	directory := inputs.Source.ContextDirectory()
	return inputs.Source.ContentID().Available() && directory.Available() && directory.LinkID() == inputs.Source.ContentID() &&
		sameContextDirectory(inputs.contextSchema.Directory(), directory)
}

func sameContextDirectory(left, right executioncontext.Directory) bool {
	if !left.Available() || !right.Available() || left.LinkID() != right.LinkID() ||
		left.ContextCount() != right.ContextCount() || left.RootCount() != right.RootCount() ||
		left.TransitionCount() != right.TransitionCount() {
		return false
	}
	for index := 0; index < left.ContextCount(); index++ {
		leftRow, leftOK := left.ContextAt(index)
		rightRow, rightOK := right.ContextAt(index)
		if !leftOK || !rightOK || leftRow.ID() != rightRow.ID() || leftRow.LinkID() != rightRow.LinkID() ||
			leftRow.ModuleKey() != rightRow.ModuleKey() || leftRow.ActorID() != rightRow.ActorID() ||
			leftRow.RepresentativeCacheInstanceID() != rightRow.RepresentativeCacheInstanceID() {
			return false
		}
	}
	for index := 0; index < left.RootCount(); index++ {
		leftRow, leftOK := left.RootAt(index)
		rightRow, rightOK := right.RootAt(index)
		if !leftOK || !rightOK || leftRow.ID() != rightRow.ID() || leftRow.LinkID() != rightRow.LinkID() ||
			leftRow.AnalysisRootID() != rightRow.AnalysisRootID() || leftRow.ContextID() != rightRow.ContextID() {
			return false
		}
	}
	for index := 0; index < left.TransitionCount(); index++ {
		leftRow, leftOK := left.TransitionAt(index)
		rightRow, rightOK := right.TransitionAt(index)
		if !leftOK || !rightOK || leftRow.ID() != rightRow.ID() || leftRow.LinkID() != rightRow.LinkID() ||
			leftRow.FromContextID() != rightRow.FromContextID() || leftRow.ToContextID() != rightRow.ToContextID() {
			return false
		}
	}
	return true
}

// The mount input getters are the record's read surface for the mount pass. A
// mounting axis names exactly the terms it seals from in its own need
// interface, so this record's shape never reaches the domain.
func (inputs LinkInputs) LinkSource() *link.Link { return inputs.Source }

func (inputs LinkInputs) MountedArtifactCount() int { return len(inputs.Artifacts) }

func (inputs LinkInputs) MountedArtifactAt(index int) (programmount.MountedArtifact, bool) {
	if index < 0 || index >= len(inputs.Artifacts) {
		return programmount.MountedArtifact{}, false
	}
	row := inputs.Artifacts[index]
	return row, row.Available()
}

func (inputs LinkInputs) StaticInput() *staticdomain.Authority { return inputs.StaticAuthority }

// The axis input getters are the record's read surface for the axis pass. An
// axis's owning domain names exactly the one it binds against in its own need
// interface, so this record's shape never reaches the domain.
func (inputs LinkInputs) ValueInput() *valuedomain.Schema { return inputs.ValueSchema }

func (inputs LinkInputs) CallInput() *calldomain.Algebra { return inputs.CallAlgebra }

func (inputs LinkInputs) HeapInput() heapdomain.Schema { return inputs.HeapSchema }

func (inputs LinkInputs) PlacementInput() placementdomain.Schema { return inputs.PlacementSchema }

// ContextInput returns the mount-derived contextual Heap authority. It is
// private in LinkInputs and therefore cannot be caller-supplied; Context's
// axis binds only against this exact issuer.
func (inputs LinkInputs) ContextInput() contextdomain.Schema { return inputs.contextSchema }

func (inputs LinkInputs) StructureInput() structure.Table { return inputs.vocabulary }

func (inputs LinkInputs) PackInput() *packdomain.Schema { return inputs.PackSchema }

func (inputs LinkInputs) EffectInput() *effectfactor.Algebra { return inputs.EffectAlgebra }

// IndexTopology is the mount phase's sealed heap index topology. It is a
// derivation over several sealed factors, so it is read from the record the
// phase produced and never sealed a second time by a consumer.
func (inputs LinkInputs) IndexTopology() *heapindex.Topology { return inputs.topology }

// KeySelection is the mount phase's sealed heap key and class projection, read
// from the record the phase produced for the same reason.
func (inputs LinkInputs) KeySelection() *keymatch.SelectorProjection { return inputs.keySelection }
