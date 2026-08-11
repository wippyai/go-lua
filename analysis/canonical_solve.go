package analysis

import (
	"context"
	"encoding/binary"
	"sort"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	callactivation "github.com/wippyai/go-lua/analysis/domain/call/activation"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapbootstrap "github.com/wippyai/go-lua/analysis/domain/heap/bootstrap"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program"
	programflow "github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

type programObservation struct{ bodies []bodyObservation }

// valueObservation is already detached from the Value domain: it preserves
// only the canonical Boundary row identity and whether that query cell was
// populated. The source assembly owns all Value/domain handles.
type valueObservation struct {
	ids     []keyspace.ContentID
	present []bool
	valid   bool
}

type bodyObservation struct {
	values valueObservation
	effect effectObservation
}

// canonicalItem retains one owner-issued occurrence and the exact Flow finish
// endpoint at which it is evaluated. It has no global phase or role point.
type canonicalItem struct {
	instance engine.SourceInstance
	identity engine.SemanticKey
	ports    []engine.SemanticKey
	arity    int
	input    canonicalInputMode
	authored keyspace.Term
	anchor   *causalSite
	entry    *causalSite
	point    *causalSite
	incoming []engine.SourceBoundary
}

// canonicalInputMode is an endpoint law, not a phase classifier. It records
// the owner-issued boundary shape for one exact occurrence. In particular,
// writes have no Entry port and therefore consume the existing Flow causal
// predecessor route that already terminates at their Finish site.
type canonicalInputMode uint8

const (
	canonicalInputFinish canonicalInputMode = iota + 1
	canonicalInputEntry
	canonicalInputPredecessor
)

type canonicalActivation struct {
	application linkproject.Application
	identity    engine.SemanticKey
	entry       *causalSite
	point       *causalSite
	prepared    engine.SourceInstance
	incoming    []engine.SourceBoundary
}

// causalMount is the one mounted Program/Flow quartet used by the sparse
// source transaction. Its sites are exactly Flow.Causal().Sites().
type causalMount struct {
	shard     linkproject.Shard
	program   *program.Program
	module    string
	moduleID  keyspace.ContentID
	flow      programflow.View
	entry     *causalSite
	sites     map[keyspace.Term]*causalSite
	decisions map[keyspace.Term]engine.SourceDecision
}

type causalSite struct {
	mount     *causalMount
	term      keyspace.Term
	context   keyspace.ContentID
	key       engine.SemanticKey
	site      engine.SourceSite
	scope     engine.SourceScope
	decisions map[keyspace.Term]engine.SourceDecision
	ordered   []keyspace.Term
}

// causalSpan is the immutable owner-issued Flow boundary for one authored
// occurrence. A finish site can be shared by several authored terms, so the
// Entry handle must travel with the occurrence rather than being mutable
// metadata on causalSite.
type causalSpan struct {
	authored keyspace.Term
	entry    *causalSite
	finish   *causalSite
}

type causalRoute struct {
	from      *causalSite
	to        *causalSite
	successor programflow.Successor
	key       engine.SemanticKey
	boundary  engine.SourceBoundary
}

type causalRouteIdentity struct {
	mount    *causalMount
	identity programflow.RouteIdentity
}

type causalTopology struct {
	linkID           keyspace.ContentID
	projectID        keyspace.ContentID
	mounts           []*causalMount
	sites            []*causalSite
	routes           []*causalRoute
	routesByIdentity map[causalRouteIdentity]*causalRoute
}

func (topology *causalTopology) mount(shard linkproject.Shard) *causalMount {
	if topology == nil {
		return nil
	}
	for _, mount := range topology.mounts {
		if mount != nil && mount.shard == shard {
			return mount
		}
	}
	return nil
}

func (topology *causalTopology) span(shard linkproject.Shard, term keyspace.Term) *causalSpan {
	mount := topology.mount(shard)
	if mount == nil {
		return nil
	}
	entry, entryOK := mount.flow.Ports().Entry(term)
	finish, finishOK := mount.flow.Ports().Finish(term)
	if !entryOK || !finishOK || entry == 0 || finish == 0 {
		return nil
	}
	entrySite := mount.sites[entry]
	finishSite := mount.sites[finish]
	if entrySite == nil || finishSite == nil {
		return nil
	}
	return &causalSpan{authored: term, entry: entrySite, finish: finishSite}
}

func (topology *causalTopology) finishSpan(shard linkproject.Shard, term keyspace.Term) *causalSpan {
	mount := topology.mount(shard)
	if mount == nil {
		return nil
	}
	finish, finishOK := mount.flow.Ports().Finish(term)
	if !finishOK || finish == 0 {
		return nil
	}
	finishSite := mount.sites[finish]
	if finishSite == nil {
		return nil
	}
	return &causalSpan{authored: term, finish: finishSite}
}

func (topology *causalTopology) spanWithRoot(shard linkproject.Shard, term keyspace.Term) *causalSpan {
	if span := topology.span(shard, term); span != nil {
		return span
	}
	mount := topology.mount(shard)
	if mount == nil || mount.program == nil {
		return nil
	}
	root, rootOK := mount.program.Source().Index().Root(term)
	entry, entryOK := mount.flow.Ports().Entry(root)
	finish, finishOK := mount.flow.Ports().Finish(root)
	if !rootOK || !entryOK || !finishOK || entry == 0 || finish == 0 {
		return nil
	}
	entrySite := mount.sites[entry]
	finishSite := mount.sites[finish]
	if entrySite == nil || finishSite == nil {
		return nil
	}
	return &causalSpan{authored: root, entry: entrySite, finish: finishSite}
}

func (topology *causalTopology) point(shard linkproject.Shard, term keyspace.Term) *causalSite {
	mount := topology.mount(shard)
	if mount == nil {
		return nil
	}
	return mount.sites[term]
}

// assignmentPredecessor resolves the owner-issued reverse commit successor
// for one Write. The Flow Successors projection owns assignment order and the
// identity-indexed route table supplies the already-issued boundary.
func (topology *causalTopology) assignmentPredecessor(finish *causalSite, authored keyspace.Term) (*causalRoute, bool) {
	if topology == nil || finish == nil || finish.mount == nil || keyspace.TermFamily(authored) != keyspace.FamilyWrite || keyspace.TermOrdinal(authored) == 0 {
		return nil, false
	}
	flowFinish, finishOK := finish.mount.flow.Ports().Finish(authored)
	if !finishOK || flowFinish == 0 || flowFinish != finish.term {
		return nil, false
	}
	successor, successorOK := finish.mount.flow.Causal().Successors().AssignmentPredecessor(authored)
	if !successorOK {
		return nil, false
	}
	identity, identityOK := successor.Identity()
	if !identityOK || topology.routesByIdentity == nil {
		return nil, false
	}
	route := topology.routesByIdentity[causalRouteIdentity{mount: finish.mount, identity: identity}]
	if route == nil || route.from == nil || route.to != finish || route.from.mount != finish.mount || route.successor.From != successor.From || route.successor.To != successor.To {
		return nil, false
	}
	routeIdentity, routeIdentityOK := route.successor.Identity()
	if !routeIdentityOK || !routeIdentity.Equal(identity) {
		return nil, false
	}
	return route, true
}

type bodyQueryPlan struct {
	body        mountedBody
	attachments []bodyQueryAttachment
	coordinates []valuedomain.Coordinate
	ids         []keyspace.ContentID
}

type bodyQueryAttachment struct {
	point  *causalSite
	value  *engine.QueryInstance[valueSummaryObservation]
	effect *engine.QueryInstance[effectObservation]
}

type bodyValuePlan struct {
	coordinates []valuedomain.Coordinate
	ids         []keyspace.ContentID
}

// solveCanonicalBodies is the one production Program transaction.  Every
// mounted Program contributes its sealed causal sites and successors to this
// one SourceAssembly; no global phase point or cached summary relation exists.
func (declared *programAnalysis) solveCanonicalBodies(ctx context.Context, bodies []mountedBody) (programObservation, bool) {
	if ctx == nil || declared == nil || declared.composition == nil || !declared.composition.Sealed() || len(bodies) == 0 || declared.queries.value == nil || declared.queries.effect == nil {
		return programObservation{}, false
	}
	linked := bodies[0].linked
	if linked == nil || linked != declared.heapSchema.Link() || linked != declared.valueSchema.Link() || linked.Project() == nil || linked.Boundary() == nil || linked.Host() == nil {
		return programObservation{}, false
	}
	for _, body := range bodies {
		if !body.valid(linked) {
			return programObservation{}, false
		}
	}

	source := engine.NewSourceAssembly(declared.composition)
	truth, truthOK := source.TrueExpr()
	if !truthOK {
		return programObservation{}, false
	}
	topology, topologyOK := buildCausalTopology(linked, source, truth)
	if !topologyOK {
		return programObservation{}, false
	}
	items := make([]canonicalItem, 0)
	valuePlans := make([]bodyValuePlan, len(bodies))
	bodyIndexFor := func(bodyTerm keyspace.Term, shard linkproject.Shard) (int, bool) {
		for index, body := range bodies {
			if body.term == bodyTerm && body.shard == shard {
				return index, true
			}
		}
		return 0, false
	}

	// Value's detached result plan is still derived from the exact Boundary
	// rows, while each SourceSeed is anchored by its own Origin endpoint.
	values := linked.Boundary().Values()
	for index := 0; index < values.Count(); index++ {
		value, valueOK := values.At(index)
		shard, term, originOK := values.Origin(value)
		p, programOK := linked.Project().Mounts().Program(shard)
		bodyTerm := keyspace.Term(0)
		positioned := false
		if programOK && p != nil {
			bodyTerm, _, _, positioned = p.Source().Index().Position(term)
		}
		coordinate, coordinateOK := declared.valueSchema.CoordinateFor(value)
		id, idOK := values.ID(value)
		if valueOK && originOK && positioned && coordinateOK && idOK {
			if bodyIndex, bodyOK := bodyIndexFor(bodyTerm, shard); bodyOK {
				valuePlans[bodyIndex].coordinates = append(valuePlans[bodyIndex].coordinates, coordinate)
				valuePlans[bodyIndex].ids = append(valuePlans[bodyIndex].ids, id)
			}
		}
		seed, seedOK := declared.valueSchema.SourceSeedAt(index)
		if !seedOK {
			continue
		}
		seedShard, seedTerm, originOK := seed.Origin()
		anchor := topology.spanWithRoot(seedShard, seedTerm)
		instance, instanceOK := declared.valueSource.Instance(seed)
		seedID, seedIDOK := seed.ID()
		if !originOK || !instanceOK || !seedIDOK || !appendCanonicalRule(source, topology, &items, anchor, "value-source", []keyspace.ContentID{seedID}, instance) {
			return programObservation{}, false
		}
	}

	for index := 0; index < declared.packSchema.RootCount(); index++ {
		root, rootOK := declared.packSchema.RootAt(index)
		pack, packOK := declared.packSchema.Source(root)
		if !packOK {
			continue
		}
		shard, term, anchorOK := pack.Anchor()
		anchor := topology.spanWithRoot(shard, term)
		instance, instanceOK := declared.packSource.Instance(pack)
		packID, packIDOK := pack.ContentID()
		if !rootOK || !anchorOK || !instanceOK || !packIDOK || !appendCanonicalRule(source, topology, &items, anchor, "pack-source", []keyspace.ContentID{packID}, instance) {
			return programObservation{}, false
		}
	}

	for index := 0; index < declared.heapSchema.KeyCount(); index++ {
		key, keyOK := declared.heapSchema.KeyAt(index)
		shard, term, allocationKind, programAllocation := key.ProgramAllocation()
		if !programAllocation {
			continue
		}
		anchor := topology.span(shard, term)
		ingress, ingressOK := declared.heapIngress.Instance(key)
		keyID, keyIDOK := key.ContentID()
		if !keyOK || !ingressOK || !keyIDOK || !appendCanonicalRule(source, topology, &items, anchor, "heap-ingress", []keyspace.ContentID{keyID}, ingress) {
			return programObservation{}, false
		}
		allocation, allocationOK := declared.valueAllocation.Instance(key)
		if !allocationOK || !appendCanonicalEntryRule(source, topology, &items, anchor, "value-allocation", []keyspace.ContentID{keyID}, allocation, declared.semantics.valueFactor) {
			return programObservation{}, false
		}
		if allocationKind == heapdomain.AllocationClosure || (allocationKind == heapdomain.AllocationTable && declared.heapSchema.FieldCount(key) == 0) {
			emptyInstance, emptyOK := declared.heapEmpty.Instance(key)
			if !emptyOK || !appendCanonicalRule(source, topology, &items, anchor, "heap-empty", []keyspace.ContentID{keyID}, emptyInstance, declared.semantics.heapFactor) {
				return programObservation{}, false
			}
		}
		if closed, closedOK := declared.heapClosed.Instance(key); closedOK {
			if !appendCanonicalRule(source, topology, &items, anchor, "heap-closed", []keyspace.ContentID{keyID}, closed, declared.semantics.heapFactor) {
				return programObservation{}, false
			}
		}
	}

	globals := linked.Host().Globals()
	for index := 0; index < globals.Count(); index++ {
		global, globalOK := globals.At(index)
		anchor := causalGlobalAnchor(topology, globals, global)
		valueInstance, valueOK := declared.valueBootstrap.Instance(global)
		globalID, globalIDOK := globals.ID(global)
		if !globalOK || !valueOK || !globalIDOK || !appendCanonicalRule(source, topology, &items, anchor, "value-bootstrap", []keyspace.ContentID{globalID}, valueInstance) {
			return programObservation{}, false
		}
	}
	bootRoots := linked.Host().BootRoots()
	for index := 0; index < bootRoots.Count(); index++ {
		boot, bootOK := bootRoots.At(index)
		heapKey, heapKeyOK := declared.heapSchema.KeyForBootRoot(boot)
		heapRoot, heapRootOK := heapbootstrap.NewRoot(declared.heapSchema, heapKey)
		heapRootID, heapRootIDOK := heapRoot.ID()
		anchors := causalBootAnchors(topology, linked, boot)
		if !bootOK || !heapKeyOK || !heapRootOK || !heapRootIDOK || len(anchors) == 0 {
			return programObservation{}, false
		}
		for _, anchor := range anchors {
			heapInstance, heapOK := declared.heapBootstrap.Instance(heapRoot)
			if !heapOK {
				return programObservation{}, false
			}
			if !appendCanonicalRule(source, topology, &items, anchor, "heap-bootstrap", []keyspace.ContentID{heapRootID}, heapInstance) {
				return programObservation{}, false
			}
		}
	}

	for index := 0; index < declared.valueSchema.StorageTransferCount(); index++ {
		transfer, transferOK := declared.valueSchema.StorageTransferAt(index)
		shard, term, occurrenceOK := transfer.Occurrence()
		anchor := topology.span(shard, term)
		if keyspace.TermFamily(term) == keyspace.FamilyWrite {
			anchor = topology.finishSpan(shard, term)
		}
		instance, instanceOK := declared.valueTransfer.Instance(transfer)
		transferID, transferIDOK := transfer.ID()
		if !transferOK || !occurrenceOK || !instanceOK || !transferIDOK {
			return programObservation{}, false
		}
		if keyspace.TermFamily(term) == keyspace.FamilyWrite {
			if !appendCanonicalPredecessorRule(source, topology, &items, anchor, "value-transfer", []keyspace.ContentID{transferID}, instance, declared.semantics.valueFactor) {
				return programObservation{}, false
			}
		} else {
			family := keyspace.TermFamily(term)
			if family != keyspace.FamilyRead && family != keyspace.FamilyBind {
				return programObservation{}, false
			}
			if !appendCanonicalEntryRule(source, topology, &items, anchor, "value-transfer", []keyspace.ContentID{transferID}, instance, declared.semantics.valueFactor) {
				return programObservation{}, false
			}
		}
	}
	for index := 0; index < declared.heapSchema.IndexAccessCount(); index++ {
		access, accessOK := declared.heapSchema.IndexAccessAt(index)
		topologyAccess, topologyOK := declared.topology.Access(access)
		accessID, accessIDOK := topologyAccess.ID()
		geometry, geometryOK := declared.heapSchema.IndexAccessGeometry(access)
		if !accessOK || !topologyOK || !accessIDOK || !geometryOK {
			return programObservation{}, false
		}
		anchorTerm := geometry.ReadTerm
		role := "raw-get"
		if topologyAccess.Write() {
			anchorTerm = geometry.WriteTerm
			role = "raw-set"
		}
		anchor := topology.span(geometry.Shard, anchorTerm)
		if topologyAccess.Write() {
			anchor = topology.finishSpan(geometry.Shard, anchorTerm)
		}
		if topologyAccess.Read() {
			instance, instanceOK := declared.rawGet.Instance(topologyAccess)
			if !instanceOK || !appendCanonicalEntryRule(source, topology, &items, anchor, role, []keyspace.ContentID{accessID}, instance, declared.semantics.valueFactor, declared.semantics.callFactor, declared.semantics.heapFactor, declared.semantics.packFactor) {
				return programObservation{}, false
			}
			continue
		}
		if topologyAccess.Write() {
			instance, instanceOK := declared.rawSet.Instance(topologyAccess)
			if !instanceOK || !appendCanonicalPredecessorRule(source, topology, &items, anchor, role, []keyspace.ContentID{accessID}, instance, declared.semantics.valueFactor, declared.semantics.heapFactor, declared.semantics.packFactor) {
				return programObservation{}, false
			}
			continue
		}
		return programObservation{}, false
	}

	applications := linked.Project().Applications().Calls()
	activations := make([]canonicalActivation, 0)
	var activationSession *callactivation.Session
	bodyTargets := declared.callAlgebra.Bodies()
	for index := 0; index < applications.Count(); index++ {
		application, applicationOK := applications.At(index)
		callShard, callTerm, applicationTermOK := linked.Project().Applications().Call(application)
		anchor := topology.span(callShard, callTerm)
		dispatch, dispatchOK := declared.callDispatch.Instance(application)
		callKey, callKeyOK := declared.calls.Algebra().KeyForApplication(application)
		callID, callIDOK := callKey.ContentID()
		callee, calleeOK := linked.Boundary().Calls().Callee(application)
		calleeID, calleeIDOK := linked.Boundary().Values().ID(callee)
		packRoot, packRootOK := declared.packSchema.CallRoot(application)
		packRootID, packRootIDOK := declared.packSchema.RootID(packRoot)
		dispatchIDs := []keyspace.ContentID{callID, calleeID, packRootID}
		selected, selectedOK := declared.effectSelected.Instance(application)
		effectRoot, effectRootOK := declared.effectAlgebra.RootForCall(application)
		effectRootID, effectRootIDOK := declared.effectAlgebra.RootID(effectRoot)
		opaque, opaqueOK := declared.effectOpaque.Instance(application)
		if !applicationOK || !applicationTermOK || anchor == nil || !dispatchOK || !callKeyOK || !callIDOK || !calleeOK || !calleeIDOK || !packRootOK || !packRootIDOK || !selectedOK || !effectRootOK || !effectRootIDOK || !opaqueOK ||
			!appendCanonicalEntryRule(source, topology, &items, anchor, "call-dispatch", dispatchIDs, dispatch, declared.semantics.valueFactor) ||
			!appendCanonicalRule(source, topology, &items, anchor, "effect-selected", []keyspace.ContentID{callID, effectRootID}, selected, declared.semantics.callFactor) ||
			!appendCanonicalRule(source, topology, &items, anchor, "effect-opaque", []keyspace.ContentID{callID, effectRootID}, opaque, declared.semantics.callFactor) {
			return programObservation{}, false
		}
		if bodyTargets.Count() != 0 {
			bodyCall, bodyCallOK := declared.effectBody.Instance(application)
			if !bodyCallOK || !appendCanonicalRule(source, topology, &items, anchor, "effect-body", []keyspace.ContentID{callID, effectRootID}, bodyCall, declared.semantics.callFactor) {
				return programObservation{}, false
			}
			activationIdentity, activationIdentityOK := causalOccurrenceKey(topology.linkID, topology.projectID, anchor.finish, "call-body-activation", callID)
			activationOccurrence, activationOccurrenceOK := source.Relation(anchor.finish.site, activationIdentity)
			prepared, preparedOK := declared.callActivation.Prepare(source, activationOccurrence, declared.semantics.callActivation)
			if !activationIdentityOK || !activationOccurrenceOK || !preparedOK {
				return programObservation{}, false
			}
			activations = append(activations, canonicalActivation{application: application, identity: activationIdentity, entry: anchor.entry, point: anchor.finish, prepared: prepared})
		}
	}

	for index := range items {
		item := &items[index]
		arity, arityOK := source.InputCount(item.instance)
		if !arityOK || arity < 0 || len(item.ports) != arity {
			return programObservation{}, false
		}
		item.arity = arity
	}
	activationEntries := []callactivation.Entry(nil)
	activationEntriesOK := bodyTargets.Count() == 0
	if bodyTargets.Count() != 0 {
		activationEntries, activationEntriesOK = buildCallActivationEntries(declared, topology, bodyTargets)
	}
	if !activationEntriesOK {
		return programObservation{}, false
	}
	if bodyTargets.Count() != 0 {
		var staged bool
		activationSession, staged = declared.callActivation.Stage(source, activationEntries)
		if !staged || activationSession == nil {
			return programObservation{}, false
		}
	}
	if len(items) == 0 || !prepareCausalTopology(source, topology, items, truth) {
		return programObservation{}, false
	}
	for index := range activations {
		activation := &activations[index]
		arity, arityOK := source.InputCount(activation.prepared)
		if !arityOK || arity != 1 || activation.entry == nil || activation.point == nil || !activation.identity.Available() {
			return programObservation{}, false
		}
		boundaryKey, keyOK := causalFinishBoundaryKey(topology.linkID, topology.projectID, activation.point, activation.identity, 0)
		boundary, boundaryOK := issueFinishBoundary(source, activation.point, boundaryKey, truth)
		if !keyOK || !boundaryOK {
			return programObservation{}, false
		}
		activation.incoming = []engine.SourceBoundary{boundary}
	}
	if !source.Seal() {
		return programObservation{}, false
	}
	if bodyTargets.Count() != 0 {
		if _, finalized := activationSession.Finalize(); !finalized {
			return programObservation{}, false
		}
	}

	plans, plansOK := newBodyQueryPlans(declared, topology, bodies, valuePlans)
	if !plansOK {
		return programObservation{}, false
	}
	solver, assembled := source.Assemble(func(assembly *engine.Assembly) bool {
		points := make(map[engine.SemanticKey]engine.AssemblyPoint, len(topology.sites))
		for _, site := range topology.sites {
			if site == nil {
				return false
			}
			point, pointOK := assembly.Point(site.site)
			if !pointOK {
				return false
			}
			points[site.key] = point
		}
		for _, route := range topology.routes {
			point, pointOK := points[route.to.key]
			if !pointOK {
				return false
			}
			if !assembly.EnvironmentEdge(point, route.boundary) {
				return false
			}
		}
		for _, item := range items {
			if item.point == nil || len(item.incoming) != item.arity {
				return false
			}
			point, pointOK := points[item.point.key]
			if !pointOK {
				return false
			}
			member, memberOK := assembly.Member(point, item.instance)
			group, groupOK := assembly.Group(point, member)
			if !memberOK || !groupOK {
				return false
			}
			for _, boundary := range item.incoming {
				if !assembly.Boundary(group, boundary) {
					return false
				}
			}
		}
		for _, activation := range activations {
			if activation.point == nil || !activation.prepared.Available() {
				return false
			}
			point, pointOK := points[activation.point.key]
			if !pointOK {
				return false
			}
			base, baseOK := assembly.ActivationBase(point)
			if activationSession == nil {
				return false
			}
			trigger, triggerOK := activationSession.Trigger(activation.application, base)
			member, memberOK := assembly.ActivationMember(point, activation.prepared, trigger)
			if !baseOK || !triggerOK || !memberOK {
				return false
			}
			group, groupOK := assembly.Group(point, member)
			if !groupOK || len(activation.incoming) != 1 || !assembly.Boundary(group, activation.incoming[0]) {
				return false
			}
		}
		for _, plan := range plans {
			for _, attachment := range plan.attachments {
				if attachment.point == nil {
					return false
				}
				point, pointOK := points[attachment.point.key]
				if !pointOK {
					return false
				}
				if attachment.value != nil {
					if _, queryOK := assembly.Query(point, attachment.value); !queryOK {
						return false
					}
				}
				if attachment.effect != nil {
					if _, queryOK := assembly.Query(point, attachment.effect); !queryOK {
						return false
					}
				}
			}
		}
		return true
	})
	if !assembled || solver == nil {
		return programObservation{}, false
	}
	state, status := solver.Solve(ctx)
	if status != engine.SolveComplete || state == nil {
		return programObservation{}, false
	}
	result := programObservation{bodies: make([]bodyObservation, len(bodies))}
	for index, plan := range plans {
		valueAttachments := make([]*engine.QueryInstance[valueSummaryObservation], 0, len(plan.attachments))
		effectAttachments := make([]*engine.QueryInstance[effectObservation], 0, len(plan.attachments))
		for _, attachment := range plan.attachments {
			if attachment.value != nil {
				valueAttachments = append(valueAttachments, attachment.value)
			}
			if attachment.effect != nil {
				effectAttachments = append(effectAttachments, attachment.effect)
			}
		}
		if len(valueAttachments) == 0 {
			result.bodies[index].values = valueObservation{ids: append([]keyspace.ContentID(nil), plan.ids...), present: make([]bool, len(plan.ids)), valid: true}
		} else {
			joined := valueSummaryObservation{}
			observedAny := false
			for _, query := range valueAttachments {
				receipt, receiptOK := query.Receipt()
				summary, observed := engine.QueryResult(receipt, state)
				if !receiptOK || !observed {
					continue
				}
				if !summary.valid {
					return programObservation{}, false
				}
				// A zero-row query is a valid unreachable observation.  It does
				// not participate in the body join; a reachable outcome may
				// still provide the one row that determines the result.
				if summary.rows == 0 {
					continue
				}
				if summary.rows != 1 || len(summary.present) != len(plan.ids) {
					return programObservation{}, false
				}
				if !observedAny {
					joined = summary
					joined.values = append([]valuedomain.Value(nil), summary.values...)
					joined.present = append([]bool(nil), summary.present...)
					observedAny = true
					continue
				}
				for cell := range joined.present {
					if !summary.present[cell] {
						continue
					}
					if !joined.present[cell] {
						joined.values[cell], joined.present[cell] = summary.values[cell], true
						continue
					}
					value, joinOK := declared.valueSchema.Join(joined.values[cell], summary.values[cell])
					if !joinOK {
						return programObservation{}, false
					}
					joined.values[cell], joined.present[cell] = value, true
				}
			}
			if !observedAny {
				result.bodies[index].values = valueObservation{ids: append([]keyspace.ContentID(nil), plan.ids...), present: make([]bool, len(plan.ids)), valid: true}
			} else {
				result.bodies[index].values = valueObservation{ids: append([]keyspace.ContentID(nil), plan.ids...), present: append([]bool(nil), joined.present...), valid: true}
			}
		}
		joinedEffect := effectObservation{}
		observedAny := false
		for _, query := range effectAttachments {
			receipt, receiptOK := query.Receipt()
			effect, observed := engine.QueryResult(receipt, state)
			if !receiptOK || !observed {
				continue
			}
			if !effect.valid {
				return programObservation{}, false
			}
			if effect.rows == 0 {
				continue
			}
			if effect.rows != 1 {
				return programObservation{}, false
			}
			if !observedAny {
				joinedEffect = cloneEffectObservation(effect)
				observedAny = true
			} else {
				joinedEffect = joinEffectObservations(joinedEffect, effect)
			}
		}
		if !observedAny {
			// Every available exit was unreachable.  Publish the exact empty
			// Effect identity rather than treating the body as an assembly
			// failure.
			result.bodies[index].effect = effectObservation{valid: true}
		} else {
			result.bodies[index].effect = joinedEffect
		}
	}
	return result, true
}

func appendCanonicalRule[V, O any](source *engine.SourceAssembly, topology *causalTopology, items *[]canonicalItem, span *causalSpan, role string, ids []keyspace.ContentID, instance *engine.RuleInstance[V, O], ports ...engine.SemanticKey) bool {
	return appendCanonicalRuleMode(source, topology, items, span, role, ids, canonicalInputFinish, instance, ports...)
}

func appendCanonicalEntryRule[V, O any](source *engine.SourceAssembly, topology *causalTopology, items *[]canonicalItem, span *causalSpan, role string, ids []keyspace.ContentID, instance *engine.RuleInstance[V, O], ports ...engine.SemanticKey) bool {
	return appendCanonicalRuleMode(source, topology, items, span, role, ids, canonicalInputEntry, instance, ports...)
}

func appendCanonicalPredecessorRule[V, O any](source *engine.SourceAssembly, topology *causalTopology, items *[]canonicalItem, span *causalSpan, role string, ids []keyspace.ContentID, instance *engine.RuleInstance[V, O], ports ...engine.SemanticKey) bool {
	return appendCanonicalRuleMode(source, topology, items, span, role, ids, canonicalInputPredecessor, instance, ports...)
}

func appendCanonicalRuleMode[V, O any](source *engine.SourceAssembly, topology *causalTopology, items *[]canonicalItem, span *causalSpan, role string, ids []keyspace.ContentID, input canonicalInputMode, instance *engine.RuleInstance[V, O], ports ...engine.SemanticKey) bool {
	if source == nil || topology == nil || items == nil || span == nil || span.finish == nil || role == "" || instance == nil {
		return false
	}
	if input != canonicalInputFinish && input != canonicalInputEntry && input != canonicalInputPredecessor {
		return false
	}
	if input != canonicalInputPredecessor && span.entry == nil {
		return false
	}
	for _, port := range ports {
		if !port.Available() {
			return false
		}
	}
	identity, identityOK := causalOccurrenceKey(topology.linkID, topology.projectID, span.finish, role, ids...)
	if !identityOK {
		return false
	}
	occurrence, occurrenceOK := source.Relation(span.finish.site, identity)
	prepared, preparedOK := source.PrepareInstance(occurrence, instance)
	if !occurrenceOK || !preparedOK {
		return false
	}
	*items = append(*items, canonicalItem{instance: prepared, identity: identity, ports: append([]engine.SemanticKey(nil), ports...), arity: 0, input: input, authored: span.authored, anchor: span.finish, entry: span.entry})
	return true
}

func buildCausalTopology(linked *link.Link, source *engine.SourceAssembly, truth engine.SourceExpr) (*causalTopology, bool) {
	if linked == nil || source == nil || linked.Project() == nil || linked.ContentID() == (keyspace.ContentID{}) {
		return nil, false
	}
	project := linked.Project()
	projectID := project.Cold().ContentID()
	if !projectID.Available() {
		return nil, false
	}
	topology := &causalTopology{linkID: linked.ContentID(), projectID: projectID, routesByIdentity: make(map[causalRouteIdentity]*causalRoute)}
	falsity, falsityOK := source.FalseExpr()
	if !falsityOK {
		return nil, false
	}
	mounts := project.Mounts()
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		programValue, programOK := mounts.Program(shard)
		module, moduleOK := mounts.Name(shard)
		moduleID, moduleIDOK := project.ModuleKey(shard)
		if !shardOK || !programOK || !moduleOK || !moduleIDOK || programValue == nil || module == "" {
			return nil, false
		}
		flowView := programValue.Flow()
		sourceEntry, sourceEntryOK := programValue.Source().Index().Entry()
		flowEntry, flowEntryOK := flowView.Ports().Entry(sourceEntry)
		if !sourceEntryOK || !flowEntryOK || flowEntry == 0 {
			return nil, false
		}
		mount := &causalMount{shard: shard, program: programValue, module: module, moduleID: moduleID, flow: flowView, sites: make(map[keyspace.Term]*causalSite), decisions: make(map[keyspace.Term]engine.SourceDecision)}
		sites := flowView.Causal().Sites()
		for siteIndex := 0; siteIndex < sites.Count(); siteIndex++ {
			flowSite, flowSiteOK := sites.At(siteIndex)
			term, termOK := flowSite.Term()
			contextID := flowSite.ContextID()
			if !flowSiteOK || !termOK || term == 0 || !contextID.Available() {
				return nil, false
			}
			guardCount, guardOK := flowView.Continuation().GuardCount(term)
			if !guardOK || guardCount < 0 {
				return nil, false
			}
			ordered := make([]keyspace.Term, 0, guardCount)
			decisions := make(map[keyspace.Term]engine.SourceDecision, guardCount)
			for guardIndex := 0; guardIndex < guardCount; guardIndex++ {
				guard, guardOK := flowView.Continuation().GuardAt(term, guardIndex)
				if !guardOK || guard == 0 {
					return nil, false
				}
				decision, decisionOK := mount.decisions[guard]
				if !decisionOK {
					decisionKey, keyOK := causalDecisionKey(topology.linkID, topology.projectID, moduleID, guard)
					if !keyOK {
						return nil, false
					}
					decision, decisionOK = source.Decision(decisionKey)
					if !decisionOK {
						return nil, false
					}
					mount.decisions[guard] = decision
				}
				if _, duplicate := decisions[guard]; duplicate {
					continue
				}
				decisions[guard] = decision
				ordered = append(ordered, guard)
			}
			scope, scopeOK := source.Scope(decisionSlice(ordered, decisions)...)
			if !scopeOK {
				return nil, false
			}
			siteKey, siteKeyOK := causalSiteKey(topology.linkID, topology.projectID, moduleID, contextID)
			if !siteKeyOK {
				return nil, false
			}
			init := falsity
			if term == flowEntry {
				init = truth
			}
			base, baseOK := source.Site(siteKey, scope, init, term == flowEntry)
			if !baseOK {
				return nil, false
			}
			causal := &causalSite{mount: mount, term: term, context: contextID, key: siteKey, site: base, scope: scope, decisions: decisions, ordered: ordered}
			mount.sites[term] = causal
			topology.sites = append(topology.sites, causal)
			if term == flowEntry {
				mount.entry = causal
			}
		}
		if mount.entry == nil {
			return nil, false
		}
		topology.mounts = append(topology.mounts, mount)
	}
	// Successors() is the only control authority.  No authored/source-order
	// edge is consulted here; every route is identity-fenced by its Flow
	// route digest and endpoint ContextIDs.
	for _, mount := range topology.mounts {
		successors := mount.flow.Causal().Successors()
		for _, from := range topology.sites {
			if from == nil || from.mount != mount {
				continue
			}
			for successorIndex := 0; successorIndex < successors.Count(from.term); successorIndex++ {
				successor, successorOK := successors.At(from.term, successorIndex)
				if !successorOK {
					return nil, false
				}
				to := mount.sites[successor.To]
				if to == nil {
					return nil, false
				}
				identity, identityOK := successor.Identity()
				if !identityOK || !identity.Digest().Available() {
					return nil, false
				}
				routeKey, routeKeyOK := causalRouteKey(topology.linkID, topology.projectID, mount.moduleID, from.context, to.context, identity.Digest())
				if !routeKeyOK {
					return nil, false
				}
				route := &causalRoute{from: from, to: to, successor: successor, key: routeKey}
				identityKey := causalRouteIdentity{mount: mount, identity: identity}
				if _, duplicate := topology.routesByIdentity[identityKey]; duplicate {
					return nil, false
				}
				topology.routesByIdentity[identityKey] = route
				topology.routes = append(topology.routes, route)
			}
		}
	}
	return topology, true
}

func decisionSlice(terms []keyspace.Term, decisions map[keyspace.Term]engine.SourceDecision) []engine.SourceDecision {
	result := make([]engine.SourceDecision, 0, len(terms))
	for _, term := range terms {
		if decision, ok := decisions[term]; ok {
			result = append(result, decision)
		}
	}
	return result
}

func prepareCausalTopology(source *engine.SourceAssembly, topology *causalTopology, items []canonicalItem, truth engine.SourceExpr) bool {
	if source == nil || topology == nil {
		return false
	}
	for _, base := range topology.sites {
		if base == nil {
			return false
		}
	}
	for _, route := range topology.routes {
		if route == nil || route.from == nil || route.to == nil {
			return false
		}
		boundary, boundaryOK := issueCausalRouteBoundary(source, route, route.from.site, route.to.site, truth)
		if !boundaryOK {
			return false
		}
		route.boundary = boundary
	}
	for index := range items {
		item := &items[index]
		if item.anchor == nil || item.anchor.mount == nil || (item.input != canonicalInputPredecessor && (item.entry == nil || item.entry.mount != item.anchor.mount)) {
			return false
		}
		finish := item.anchor
		item.point = finish
		if item.arity == 0 {
			continue
		}
		item.incoming = make([]engine.SourceBoundary, item.arity)
		for slot := 0; slot < item.arity; slot++ {
			var boundary engine.SourceBoundary
			var boundaryOK bool
			switch item.input {
			case canonicalInputEntry:
				boundaryKey, keyOK := causalEntryBoundaryKey(topology.linkID, topology.projectID, item.entry, finish, item.identity, uint64(slot))
				boundary, boundaryOK = issueSpanBoundary(source, item.entry, finish, boundaryKey, truth)
				boundaryOK = keyOK && boundaryOK
			case canonicalInputPredecessor:
				route, routeOK := topology.assignmentPredecessor(finish, item.authored)
				if routeOK {
					boundary, boundaryOK = route.boundary, true
				}
			case canonicalInputFinish:
				boundaryKey, keyOK := causalFinishBoundaryKey(topology.linkID, topology.projectID, item.anchor, item.identity, uint64(slot))
				boundary, boundaryOK = issueFinishBoundary(source, finish, boundaryKey, truth)
				boundaryOK = keyOK && boundaryOK
			default:
				return false
			}
			if !boundaryOK {
				return false
			}
			item.incoming[slot] = boundary
		}
	}
	return true
}

func buildCallActivationEntries(declared *programAnalysis, topology *causalTopology, bodies calldomain.Bodies) ([]callactivation.Entry, bool) {
	if declared == nil || topology == nil || declared.callAlgebra == nil || bodies.Count() == 0 {
		return nil, false
	}
	sourceRole, sourceRoleOK := analysisSemanticKeyParts(topology.linkID, "canonical/call-body-source-role")
	if !sourceRoleOK {
		return nil, false
	}
	targetRole, targetRoleOK := analysisSemanticKeyParts(topology.linkID, "canonical/call-body-target-role")
	if !targetRoleOK {
		return nil, false
	}
	entries := make([]callactivation.Entry, bodies.Count())
	carried := []engine.CarryForm{declared.values.Carry(), declared.calls.Carry(), declared.heap.Carry(), declared.packs.Carry()}
	kinds := []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel}
	for index := 0; index < bodies.Count(); index++ {
		body, bodyOK := bodies.At(index)
		if !bodyOK {
			return nil, false
		}
		shard, term, resolved := declared.callAlgebra.ResolveBody(body)
		if !resolved {
			return nil, false
		}
		bodyID, bodyIDOK := body.ContentID()
		if !bodyIDOK {
			return nil, false
		}
		entrySite := causalBodyEntry(topology, shard, term)
		if entrySite == nil {
			return nil, false
		}
		target, targetOK := analysisSemanticKeyParts(topology.linkID, "canonical/call-body-target", topology.projectID[:], bodyID[:])
		endpoint, endpointOK := analysisSemanticKeyParts(topology.linkID, "canonical/call-body-endpoint", topology.projectID[:], bodyID[:])
		if !targetOK {
			return nil, false
		}
		if !endpointOK {
			return nil, false
		}
		if target == endpoint {
			return nil, false
		}
		edges := make([]engine.ActivationFactorEdge, 0, len(carried)+len(kinds))
		for _, factor := range carried {
			provenance, provenanceOK := callActivationEdgeKey(topology, "entry", bodyID, entrySite)
			if !provenanceOK {
				return nil, false
			}
			edges = append(edges, engine.ActivationFactorEdge{SourceRole: sourceRole, TargetSite: entrySite.site, Factor: factor, Provenance: provenance})
		}
		exits := make([]*causalSite, 0, len(kinds))
		seenExits := make(map[keyspace.Term]struct{}, len(kinds))
		mount := topology.mount(shard)
		if mount == nil {
			return nil, false
		}
		for _, kind := range kinds {
			exit, exitOK := mount.flow.Outcomes().BodyExit(term, kind)
			if !exitOK {
				continue
			}
			if _, duplicate := seenExits[exit]; duplicate {
				continue
			}
			seenExits[exit] = struct{}{}
			exitSite := topology.point(shard, exit)
			if exitSite == nil {
				return nil, false
			}
			exits = append(exits, exitSite)
		}
		if len(exits) == 0 {
			return nil, false
		}
		for _, exitSite := range exits {
			effectCarry := declared.effects.Carry()
			provenance, provenanceOK := callActivationEdgeKey(topology, "effect", bodyID, exitSite)
			if !provenanceOK {
				return nil, false
			}
			edges = append(edges, engine.ActivationFactorEdge{SourceSite: exitSite.site, TargetRole: targetRole, Factor: effectCarry, Provenance: provenance})
		}
		entries[index] = callactivation.Entry{Body: body, Target: target, Endpoint: endpoint, FactorEdges: edges}
	}
	return entries, true
}

func causalBodyEntry(topology *causalTopology, shard linkproject.Shard, body keyspace.Term) *causalSite {
	mount := topology.mount(shard)
	if mount == nil || body == 0 {
		return nil
	}
	entry, entryOK := mount.flow.Ports().Entry(body)
	if !entryOK || entry == 0 {
		return nil
	}
	return mount.sites[entry]
}

func callActivationEdgeKey(topology *causalTopology, direction string, bodyID keyspace.ContentID, site *causalSite) (engine.SemanticKey, bool) {
	if topology == nil || direction == "" || !bodyID.Available() || site == nil || !site.context.Available() {
		return engine.SemanticKey{}, false
	}
	return analysisSemanticKeyParts(topology.linkID, "canonical/call-body-edge/"+direction, topology.projectID[:], site.mount.moduleID[:], bodyID[:], site.context[:])
}

func issueCausalRouteBoundary(source *engine.SourceAssembly, route *causalRoute, from, to engine.SourceSite, truth engine.SourceExpr) (engine.SourceBoundary, bool) {
	if source == nil || route == nil || route.from == nil || route.to == nil {
		return engine.SourceBoundary{}, false
	}
	maps := make([]engine.SourceDecisionMap, 0, len(route.from.ordered))
	for _, term := range route.from.ordered {
		decision, decisionOK := route.from.decisions[term]
		if !decisionOK {
			return engine.SourceBoundary{}, false
		}
		var mapping engine.SourceDecisionMap
		var mapOK bool
		if route.successor.ResetContains(term) {
			mapping, mapOK = source.ForgetMap(decision)
		} else if _, retained := route.to.decisions[term]; retained {
			mapping, mapOK = source.IdentityMap(decision)
		} else {
			mapping, mapOK = source.ForgetMap(decision)
		}
		if !mapOK {
			return engine.SourceBoundary{}, false
		}
		maps = append(maps, mapping)
	}
	reindex, reindexOK := source.Reindex(route.from.scope, route.to.scope, maps...)
	if !reindexOK {
		return engine.SourceBoundary{}, false
	}
	pre := truth
	post := truth
	if route.successor.Decision != 0 {
		sourceDecision, sourceOwns := route.from.decisions[route.successor.Decision]
		targetDecision, targetOwns := route.to.decisions[route.successor.Decision]
		if !sourceOwns && !targetOwns {
			return engine.SourceBoundary{}, false
		}
		decision := targetDecision
		if sourceOwns {
			decision = sourceDecision
		}
		literal, literalOK := source.DecisionExpr(decision)
		if !literalOK {
			return engine.SourceBoundary{}, false
		}
		if sourceOwns {
			pre = literal
		}
		if targetOwns {
			post = literal
		}
		if !route.successor.Truth {
			if sourceOwns {
				var negatedOK bool
				pre, negatedOK = source.NotExpr(pre)
				if !negatedOK {
					return engine.SourceBoundary{}, false
				}
			}
			if targetOwns {
				var negatedOK bool
				post, negatedOK = source.NotExpr(post)
				if !negatedOK {
					return engine.SourceBoundary{}, false
				}
			}
		}
	}
	return source.Boundary(from, to, route.key, pre, reindex, post)
}

func issueFinishBoundary(source *engine.SourceAssembly, finish *causalSite, key engine.SemanticKey, truth engine.SourceExpr) (engine.SourceBoundary, bool) {
	return issueSpanBoundary(source, finish, finish, key, truth)
}

func issueSpanBoundary(source *engine.SourceAssembly, entry, finish *causalSite, key engine.SemanticKey, truth engine.SourceExpr) (engine.SourceBoundary, bool) {
	if source == nil || entry == nil || finish == nil || !key.Available() {
		return engine.SourceBoundary{}, false
	}
	maps := make([]engine.SourceDecisionMap, 0, len(entry.ordered))
	for _, term := range entry.ordered {
		decision, decisionOK := entry.decisions[term]
		if !decisionOK {
			return engine.SourceBoundary{}, false
		}
		var mapping engine.SourceDecisionMap
		var mapOK bool
		if _, retained := finish.decisions[term]; retained {
			mapping, mapOK = source.IdentityMap(decision)
		} else {
			mapping, mapOK = source.ForgetMap(decision)
		}
		if !mapOK {
			return engine.SourceBoundary{}, false
		}
		maps = append(maps, mapping)
	}
	reindex, reindexOK := source.Reindex(entry.scope, finish.scope, maps...)
	if !reindexOK {
		return engine.SourceBoundary{}, false
	}
	return source.Boundary(entry.site, finish.site, key, truth, reindex, truth)
}

func causalFinishBoundaryKey(linkID, projectID keyspace.ContentID, anchor *causalSite, instance engine.SemanticKey, slot uint64) (engine.SemanticKey, bool) {
	if anchor == nil || !anchor.key.Available() || !instance.Available() {
		return engine.SemanticKey{}, false
	}
	anchorDigest := anchor.key.Digest()
	instanceDigest := instance.Digest()
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], slot)
	return analysisSemanticKeyParts(linkID, "canonical/causal-finish-self", projectID[:], anchor.mount.moduleID[:], anchor.context[:], anchorDigest[:], instanceDigest[:], encoded[:])
}

func causalEntryBoundaryKey(linkID, projectID keyspace.ContentID, entry, finish *causalSite, instance engine.SemanticKey, slot uint64) (engine.SemanticKey, bool) {
	if entry == nil || finish == nil || entry.mount == nil || finish.mount == nil || entry.mount != finish.mount || !linkID.Available() || !projectID.Available() || !entry.context.Available() || !finish.context.Available() || !instance.Available() {
		return engine.SemanticKey{}, false
	}
	instanceDigest := instance.Digest()
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], slot)
	return analysisSemanticKeyParts(linkID, "canonical/causal-entry-finish", projectID[:], entry.mount.moduleID[:], entry.context[:], finish.context[:], instanceDigest[:], encoded[:])
}

func newBodyQueryPlans(declared *programAnalysis, topology *causalTopology, bodies []mountedBody, valuePlans []bodyValuePlan) ([]bodyQueryPlan, bool) {
	if declared == nil || topology == nil || len(bodies) != len(valuePlans) {
		return nil, false
	}
	plans := make([]bodyQueryPlan, len(bodies))
	kinds := []flowkind.OutcomeKind{flowkind.OutcomeNormal, flowkind.OutcomeReturn, flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel}
	for index, body := range bodies {
		plan := bodyQueryPlan{body: body, coordinates: append([]valuedomain.Coordinate(nil), valuePlans[index].coordinates...), ids: append([]keyspace.ContentID(nil), valuePlans[index].ids...)}
		root, rootOK := declared.effectAlgebra.RootForBody(body.shard, body.term)
		ref, refOK := declared.effects.Locate(root)
		if !rootOK || !refOK {
			return nil, false
		}
		seen := make(map[keyspace.Term]struct{})
		for _, outcomeKind := range kinds {
			exit, exitOK := body.program.Flow().Outcomes().BodyExit(body.term, outcomeKind)
			if !exitOK {
				continue
			}
			if _, duplicate := seen[exit]; duplicate {
				continue
			}
			seen[exit] = struct{}{}
			point := topology.point(body.shard, exit)
			if point == nil {
				return nil, false
			}
			attachment := bodyQueryAttachment{point: point}
			if len(plan.coordinates) != 0 {
				coordinates := append([]valuedomain.Coordinate(nil), plan.coordinates...)
				query, queryOK := engine.NewQueryInstance(declared.queries.value, func(binding *engine.QueryBinding[valueSummaryObservation]) bool {
					refs := declared.values.NewSummaryRefs()
					if refs == nil {
						return false
					}
					for _, coordinate := range coordinates {
						if !declared.values.AppendSummaryCoordinate(refs, coordinate) {
							return false
						}
					}
					return declared.values.CloseSummaryRefs(refs) && engine.InstanceQuerySummaryRead(binding, declared.queries.valueRead, declared.values.SummaryRead(), refs)
				})
				if !queryOK {
					return nil, false
				}
				attachment.value = query
			}
			query, queryOK := engine.NewQueryInstance(declared.queries.effect, func(binding *engine.QueryBinding[effectObservation]) bool {
				return engine.InstanceQueryRead(binding, declared.queries.effectRead, ref)
			})
			if !queryOK {
				return nil, false
			}
			attachment.effect = query
			plan.attachments = append(plan.attachments, attachment)
		}
		if len(plan.attachments) == 0 {
			return nil, false
		}
		plans[index] = plan
	}
	return plans, true
}

func joinEffectObservations(left, right effectObservation) effectObservation {
	if !left.valid {
		return cloneEffectObservation(right)
	}
	if !right.valid {
		return left
	}
	result := cloneEffectObservation(left)
	result.present = left.present || right.present
	result.top = left.top || right.top
	if result.top {
		result.atoms = nil
		return result
	}
	seen := make(map[keyspace.ContentID]struct{}, len(left.atoms)+len(right.atoms))
	for _, atom := range left.atoms {
		seen[atom] = struct{}{}
	}
	for _, atom := range right.atoms {
		if _, ok := seen[atom]; !ok {
			result.atoms = append(result.atoms, atom)
			seen[atom] = struct{}{}
		}
	}
	sort.Slice(result.atoms, func(left, right int) bool {
		return string(result.atoms[left][:]) < string(result.atoms[right][:])
	})
	return result
}

func causalGlobalAnchor(topology *causalTopology, globals linkhost.Globals, global linkhost.GlobalBinding) *causalSpan {
	if topology == nil {
		return nil
	}
	_, _, cell, _, _, _, mapped := globals.Mapping(global)
	if !mapped {
		return nil
	}
	for _, mount := range topology.mounts {
		if mount == nil || mount.entry == nil {
			continue
		}
		if _, ok := globals.ForProgramCell(mount.shard, mount.program, cell); ok {
			return &causalSpan{entry: mount.entry, finish: mount.entry}
		}
	}
	return nil
}

func causalBootAnchors(topology *causalTopology, linked *link.Link, boot linkhost.BootRoot) []*causalSpan {
	if topology == nil || linked == nil || linked.Module() == nil {
		return nil
	}
	actor, _, mapped := linked.Host().BootRoots().Mapping(boot)
	if !mapped {
		return nil
	}
	result := make([]*causalSpan, 0)
	roots := linked.Module().Roots()
	for _, mount := range topology.mounts {
		if mount == nil || mount.entry == nil {
			continue
		}
		for index := 0; index < roots.ForShardCount(mount.shard); index++ {
			root, rootOK := roots.ForShardAt(mount.shard, index)
			_, rootActor, _, mappingOK := roots.Mapping(root)
			if rootOK && mappingOK && rootActor == actor {
				result = append(result, &causalSpan{entry: mount.entry, finish: mount.entry})
				break
			}
		}
	}
	return result
}

func causalSiteKey(linkID, projectID, moduleID, contextID keyspace.ContentID) (engine.SemanticKey, bool) {
	if !linkID.Available() || !projectID.Available() || !moduleID.Available() || !contextID.Available() {
		return engine.SemanticKey{}, false
	}
	return analysisSemanticKeyParts(linkID, "canonical/causal-site", projectID[:], moduleID[:], contextID[:])
}

func causalDecisionKey(linkID, projectID, moduleID keyspace.ContentID, term keyspace.Term) (engine.SemanticKey, bool) {
	if !linkID.Available() || !projectID.Available() || !moduleID.Available() || term == 0 {
		return engine.SemanticKey{}, false
	}
	var termBytes [8]byte
	binary.BigEndian.PutUint64(termBytes[:], uint64(term))
	return analysisSemanticKeyParts(linkID, "canonical/causal-decision", projectID[:], moduleID[:], termBytes[:])
}

func causalRouteKey(linkID, projectID, moduleID, from, to, routeID keyspace.ContentID) (engine.SemanticKey, bool) {
	if !linkID.Available() || !projectID.Available() || !moduleID.Available() || !from.Available() || !to.Available() || !routeID.Available() {
		return engine.SemanticKey{}, false
	}
	return analysisSemanticKeyParts(linkID, "canonical/causal-route", projectID[:], moduleID[:], from[:], to[:], routeID[:])
}

func causalOccurrenceKey(linkID, projectID keyspace.ContentID, anchor *causalSite, role string, ids ...keyspace.ContentID) (engine.SemanticKey, bool) {
	if anchor == nil || !linkID.Available() || !projectID.Available() || !anchor.context.Available() || role == "" {
		return engine.SemanticKey{}, false
	}
	parts := make([][]byte, 0, 4+len(ids))
	parts = append(parts, projectID[:], anchor.mount.moduleID[:], anchor.context[:])
	for _, id := range ids {
		if !id.Available() {
			return engine.SemanticKey{}, false
		}
		parts = append(parts, id[:])
	}
	return analysisSemanticKeyParts(linkID, "canonical/causal-occurrence/"+role, parts...)
}

func canonicalScheduleKey(linkID keyspace.ContentID, role string, ids ...keyspace.ContentID) (engine.SemanticKey, bool) {
	if !linkID.Available() || role == "" {
		return engine.SemanticKey{}, false
	}
	parts := make([][]byte, len(ids))
	for index, id := range ids {
		if !id.Available() {
			return engine.SemanticKey{}, false
		}
		parts[index] = id[:]
	}
	return analysisSemanticKeyParts(linkID, "canonical/"+role, parts...)
}

func canonicalBoundaryKey(linkID keyspace.ContentID, instance, port engine.SemanticKey) (engine.SemanticKey, bool) {
	if !linkID.Available() || !instance.Available() || !port.Available() {
		return engine.SemanticKey{}, false
	}
	instanceDigest := instance.Digest()
	portDigest := port.Digest()
	var instanceVersion, portVersion [8]byte
	binary.BigEndian.PutUint64(instanceVersion[:], instance.Version())
	binary.BigEndian.PutUint64(portVersion[:], port.Version())
	return analysisSemanticKeyParts(linkID, "canonical/boundary", instanceDigest[:], instanceVersion[:], portDigest[:], portVersion[:])
}
