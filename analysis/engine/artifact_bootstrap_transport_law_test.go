package engine

import (
	"context"
	"crypto/sha256"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"sync/atomic"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

type bootstrapTransportLawOwner struct {
	schema                         *Schema
	factors                        [3]*FactorSlot[uint64]
	rules                          [3]*RuleSlot[uint64, struct{}]
	writes                         [3]SchemaWriteSlot[uint64]
	admissions                     [3]identity.SemanticKey
	queries                        [4]*QuerySlot[uint64]
	implementations                [3]*RuleImplementation[uint64, uint64, struct{}]
	queryImplementations           [4]*ExactQueryImplementation[uint64, uint64]
	activationFamily               SchemaActivationFamily
	activationRule                 *SchemaActivationRuleSlot
	activationImplementation       *ActivationRuleImplementation
	activationSemantic             identity.SemanticKey
	activationAdmission            identity.SemanticKey
	transfers                      [3]*atomic.Int64
	activationRuns                 *atomic.Int64
	binding                        *SchemaBinding
	value, heap, excluded          RuleSlotCapability
	mounted, activation            RuleSlotCapability
	valueFactor, heapFactor, other composition.Key
}

// bootstrapTransportLawQueryFactor is the factor each declared query reads. The
// fourth query reads the excluded lane's factor at the mounted Point that lane
// writes, so the third query keeps proving the same factor is absent at the
// mounted initial Point the Link bootstrap transports reach.
var bootstrapTransportLawQueryFactor = [4]int{0, 1, 2, 2}

func newBootstrapTransportLawOwner(t testing.TB) bootstrapTransportLawOwner {
	t.Helper()
	builder := NewSchema()
	var owner bootstrapTransportLawOwner
	for index := range owner.transfers {
		owner.transfers[index] = &atomic.Int64{}
	}
	owner.activationRuns = &atomic.Int64{}
	var forms [3]SchemaWriteForm[uint64]
	var reads [3]SchemaReadForm[uint64]
	for index := range owner.factors {
		factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(uint64(947_950+index*4)))
		if !factorOK || factor == nil {
			t.Fatal("bootstrap transport factor rows")
		}
		form, formOK := factor.ExactWrite()
		read, readOK := factor.ExactRead()
		if !formOK || !readOK {
			t.Fatal("bootstrap transport factor rows")
		}
		owner.factors[index], forms[index], reads[index] = factor, form, read
	}
	for index := range owner.rules {
		admission := coldKey(uint64(947_953 + index*4))
		rule, ruleOK := DeclareRuleSlot[uint64, struct{}](builder, SchemaRuleSpec[uint64]{
			Semantic: coldKey(uint64(947_951 + index*4)), OperandFamily: coldKey(uint64(947_952 + index*4)),
			Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: admission}, Output: owner.factors[index].Ref(),
		})
		write, writeOK := SchemaWrite(rule, forms[index])
		if !ruleOK || !writeOK {
			t.Fatal("bootstrap transport schema rows")
		}
		owner.rules[index], owner.writes[index], owner.admissions[index] = rule, write, admission
	}
	for index := range owner.queries {
		query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(uint64(947_980 + index*2)), Freezer: coldKey(uint64(947_981 + index*2))})
		if !queryOK || !SchemaQueryRead(query, reads[bootstrapTransportLawQueryFactor[index]]) {
			t.Fatal("bootstrap transport query rows")
		}
		owner.queries[index] = query
	}
	owner.activationSemantic, owner.activationAdmission = coldKey(947_990), coldKey(947_991)
	activationFamily, familyOK := DeclareSchemaActivationFamily(builder, coldKey(947_989))
	activationRule, activationOK := DeclareSchemaActivationRule(builder, SchemaStructuralRuleSpec{
		Semantic: owner.activationSemantic, Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: owner.activationAdmission}, Activation: activationFamily,
	})
	schema, schemaOK := builder.Seal()
	if !familyOK || !activationOK || !schemaOK || schema == nil {
		t.Fatal("bootstrap transport schema")
	}
	owner.activationFamily, owner.activationRule = activationFamily, activationRule
	owner.schema = schema
	capabilities := bindBootstrapTransportLawOwner(t, owner)
	owner.binding, owner.value, owner.heap = capabilities.binding, capabilities.value, capabilities.heap
	owner.excluded, owner.mounted, owner.activation = capabilities.excluded, capabilities.mounted, capabilities.activation
	for index := range owner.implementations {
		implementation, implementationOK := RuleImplementationAt[uint64, uint64, struct{}](owner.binding, owner.rules[index])
		if !implementationOK || implementation == nil {
			t.Fatal("bootstrap transport rule implementation")
		}
		owner.implementations[index] = implementation
	}
	for index := range owner.queryImplementations {
		implementation, implementationOK := ExactQueryImplementationAt[uint64, uint64](owner.binding, owner.queries[index])
		if !implementationOK || implementation == nil {
			t.Fatal("bootstrap transport query implementation")
		}
		owner.queryImplementations[index] = implementation
	}
	activationImplementation, activationImplementationOK := ActivationRuleImplementationAt(owner.binding, owner.activationRule)
	if !activationImplementationOK || activationImplementation == nil {
		t.Fatal("bootstrap transport activation implementation")
	}
	owner.activationImplementation = activationImplementation
	owner.valueFactor, _ = linkTransportFactorSemantic(owner.binding.state, owner.value)
	owner.heapFactor, _ = linkTransportFactorSemantic(owner.binding.state, owner.heap)
	otherOrdinal, otherOK := owner.factors[2].Ordinal()
	owner.other = owner.schema.factorSemanticAt(otherOrdinal)
	if !owner.valueFactor.Available() || !owner.heapFactor.Available() || !otherOK || !owner.other.Available() || owner.valueFactor == owner.heapFactor || owner.valueFactor == owner.other || owner.heapFactor == owner.other {
		t.Fatal("bootstrap transport factor identities")
	}
	return owner
}

// bootstrapTransportLawCapabilities is the capability set one binding of this
// owner issues: the two authorized Link bootstrap transports, a third Link
// capability outside that pair, the mounted capability the same third rule slot
// also carries, and the mounted activation capability.
type bootstrapTransportLawCapabilities struct {
	binding                                    *SchemaBinding
	value, heap, excluded, mounted, activation RuleSlotCapability
}

func bindBootstrapTransportLawOwner(t testing.TB, owner bootstrapTransportLawOwner) bootstrapTransportLawCapabilities {
	t.Helper()
	binding := NewSchemaBinding(owner.schema)
	for index := range owner.factors {
		staged := uint64(41 + index*32)
		spec := HotRuleSpec[uint64, struct{}]{
			OperandContent: func(value struct{}) (struct{}, [32]byte, bool) { return value, [32]byte{byte(index + 1)}, true },
			Admission:      AdmitRuleByTrustedTheorem[uint64, struct{}](owner.admissions[index]),
			Transfer: func(access Access[uint64, struct{}]) bool {
				owner.transfers[index].Add(1)
				return Product(access, func(row Row) bool { return StageValue(access, row, staged) })
			},
		}
		if !BindFactor(binding, owner.factors[index], hotUintFactorSpec()) || !BindRule[uint64, uint64, struct{}](binding, owner.rules[index], owner.writes[index], owner.factors[index], spec, testRuleProjector[struct{}]) {
			t.Fatal("bootstrap transport binding rows")
		}
	}
	value, valueOK := IssueLinkRuleCapability(binding, owner.rules[0])
	heap, heapOK := IssueLinkRuleCapability(binding, owner.rules[1])
	excluded, excludedOK := IssueLinkRuleCapability(binding, owner.rules[2])
	mounted, mountedOK := IssueMountedRuleCapability(binding, owner.rules[2])
	activation, activationCapabilityOK := IssueActivationRuleCapability(binding, owner.activationRule)
	queriesOK := true
	for index, query := range owner.queries {
		queriesOK = queriesOK && BindExactQuery(binding, query, owner.factors[bootstrapTransportLawQueryFactor[index]], bootstrapTransportQuerySpec(coldKey(uint64(947_981+index*2))))
	}
	activationOK := BindActivationRule(binding, owner.activationRule, HotActivationSpec{
		Admission: AdmitActivationByTrustedTheorem(owner.activationAdmission),
		Run: func(activation Activation) bool {
			owner.activationRuns.Add(1)
			return Activate(activation, coldKey(947_992), coldKey(947_993), coldKey(947_994))
		},
	})
	if !valueOK || !heapOK || !excludedOK || !mountedOK || !activationCapabilityOK ||
		!RegisterRuleSlot(binding, owner.rules[0], value) || !RegisterRuleSlot(binding, owner.rules[1], heap) ||
		!RegisterRuleSlot(binding, owner.rules[2], excluded) || !RegisterRuleSlot(binding, owner.rules[2], mounted) ||
		!RegisterActivationRuleSlot(binding, owner.activationRule, activation) ||
		!RegisterLinkBootstrapTransportPair(binding, value, heap) || !queriesOK || !activationOK || !binding.Seal() {
		t.Fatal("bootstrap transport capabilities")
	}
	return bootstrapTransportLawCapabilities{binding: binding, value: value, heap: heap, excluded: excluded, mounted: mounted, activation: activation}
}

func bootstrapTransportQuerySpec(freezer identity.SemanticKey) HotExactQuerySpec[uint64, uint64] {
	return HotExactQuerySpec[uint64, uint64]{
		Project: func(cells OrderedCells[uint64]) uint64 {
			value, present, valid := cells.At(0)
			if !valid || !present {
				return ^uint64(0)
			}
			return value
		},
		Result: FrozenResult[uint64]{
			Semantic: freezer,
			Freeze:   func(value uint64) uint64 { return value }, Clone: func(value uint64) uint64 { return value },
			Equal: func(left, right uint64) bool { return left == right }, Fingerprint: func(value uint64) uint64 { return value },
			Present: func(value uint64) bool { return true },
		},
	}
}

type bootstrapTransportLawArtifact struct {
	template          *rows.ArtifactScalarTemplate
	artifact, initial identity.ContentID
	noninitial        identity.ContentID
	// producer and activation are the two parent-issued rule rows the mounted
	// lane occupies. Both are staged at the non-initial Point, so the excluded
	// lane's factor is written where no Link bootstrap transport lands.
	producerRole, activationRole rows.ArtifactScalarRole
	producer, activation         identity.ContentID
}

func newBootstrapTransportLawArtifact(t testing.TB, schema *Schema, salt byte) bootstrapTransportLawArtifact {
	return newBootstrapTransportLawArtifactWithInitials(t, schema, salt, true, false)
}

func newBootstrapTransportLawArtifactWithInitials(t testing.TB, schema *Schema, salt byte, firstInitial, secondInitial bool) bootstrapTransportLawArtifact {
	t.Helper()
	artifactID := bootstrapTransportLawID(salt, 1)
	program := bootstrapTransportLawID(salt, 2)
	initial := bootstrapTransportLawID(salt, 3)
	noninitial := bootstrapTransportLawID(salt, 4)
	regionID := bootstrapTransportLawID(salt, 5)
	bodyID := bootstrapTransportLawID(salt, 6)
	producer := bootstrapTransportLawID(salt, 7)
	activation := bootstrapTransportLawID(salt, 8)
	spec, specOK := rows.NewArtifactScalarSpec(artifactID, program, identity.ContentID(schema.ID().Digest()), rows.ArtifactScalarCapacity{Roles: 2, Points: 2, Regions: 1, Events: 4, Bodies: 1, Rules: 2})
	if !specOK || spec == nil {
		t.Fatal("bootstrap transport artifact spec")
	}
	producerRole, producerRoleOK := spec.DeclareRole(bootstrapTransportLawID(salt, 9))
	activationRole, activationRoleOK := spec.DeclareRole(bootstrapTransportLawID(salt, 10))
	if !producerRoleOK || !activationRoleOK {
		t.Fatal("bootstrap transport artifact roles")
	}
	initialRow, initialOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: initial, Initial: firstInitial})
	_, noninitialOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: noninitial, Initial: secondInitial})
	region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: regionID, Head: initial})
	body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: bodyID})
	if initialRow != 0 || !initialOK || !noninitialOK || !regionOK || !bodyOK ||
		!spec.AddRegionMember(region, initial) || !spec.AddRegionMember(region, noninitial) ||
		!spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: regionID}) ||
		!spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: initial}) ||
		!spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: noninitial}) ||
		!spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: regionID}) ||
		!spec.AddBodyEntry(body, initial) || !spec.AddBodyExit(body, noninitial) ||
		!spec.AddRule(rows.ArtifactScalarRule{Role: producerRole, Stage: rows.ArtifactRuleStageBase, Point: noninitial, ID: producer}) ||
		!spec.AddRule(rows.ArtifactScalarRule{Role: activationRole, Stage: rows.ArtifactRuleStageBase, Point: noninitial, ID: activation}) {
		t.Fatal("bootstrap transport artifact rows")
	}
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	if !templateOK || template == nil {
		t.Fatal("bootstrap transport artifact")
	}
	return bootstrapTransportLawArtifact{
		template: template, artifact: artifactID, initial: initial, noninitial: noninitial,
		producerRole: producerRole, activationRole: activationRole, producer: producer, activation: activation,
	}
}

func (artifact bootstrapTransportLawArtifact) mount(owner bootstrapTransportLawOwner, module identity.ContentID) MountedProgramArtifact {
	return MountedProgramArtifact{
		Template: artifact.template, Module: module,
		Roles: []MountedProgramRole{
			{Scalar: artifact.producerRole, Capability: owner.mounted},
			{Scalar: artifact.activationRole, Capability: owner.activation},
		},
	}
}

func bootstrapTransportLawID(salt, value byte) identity.ContentID {
	return identity.ContentID(sha256.Sum256([]byte{0xB7, salt, value}))
}

func bootstrapTransportLawWitness(t testing.TB, owner bootstrapTransportLawOwner, salt byte) LinkBootstrapWitness {
	t.Helper()
	witness, ok := NewLinkBootstrapWitnessByCapability(
		bootstrapTransportLawID(salt, 20),
		LinkBootstrapPoint{PointID: bootstrapTransportLawID(salt, 21), Known: true, Initial: true},
		LinkBootstrapCatalog{Capability: owner.value}, LinkBootstrapCatalog{Capability: owner.heap},
	)
	if !ok || witness.transportCapabilityCount() != 2 || witness.OccurrenceCount() != 0 {
		t.Fatal("empty-catalog bootstrap transport witness")
	}
	return witness
}

func commitBootstrapTransportLaw(t testing.TB, binding *SchemaBinding, mounts []MountedProgramArtifact, witness LinkBootstrapWitness) (*BindingTopology, *CommittedProgram, *mountedArtifactRows) {
	t.Helper()
	sealedMounts, sealedOK := sealMountedProgramArtifacts(mounts)
	assembly, failure, assemblyOK := beginMountedProgramMounts(binding, mounts, witness)
	if !sealedOK || !assemblyOK || assembly == nil || failure != receiptAssemblyFailureNone {
		t.Fatalf("bootstrap transport assembly failure=%d", failure)
	}
	if !assembly.SealSources() {
		t.Fatalf("bootstrap transport source seal=%+v", assembly.sealFailure)
	}
	rows := assembly.mountedRows
	declaration, declared := declareSealedTopology(assembly, sealedMounts, witness)
	// The owner declares three query families and admits no query row, so the
	// program publishes its Query demand as solve-local observations.
	declaration.observationQueries = true
	constructed, refusal := constructTopology(declaration)
	topology, graph := constructed.topology, constructed.graph
	committed := declared && !refusal.Available() && constructed.Available()
	if !committed || topology == nil || graph == nil {
		t.Fatalf("bootstrap transport construction declared=%t stage=%v step=%v ordinal=%d", declared, refusal.Stage(), refusal.Step(), refusal.Ordinal())
	}
	program := CommittedProgramFrom(topology, graph)
	if program == nil {
		t.Fatal("bootstrap transport committed program")
	}
	return topology, program, rows
}

func TestLinkBootstrapTransportsOnlyValueAndHeapToMountedInitialPoints(t *testing.T) {
	owner := newBootstrapTransportLawOwner(t)
	artifact := newBootstrapTransportLawArtifact(t, owner.schema, 1)
	mountID := bootstrapTransportLawID(1, 30)
	mounted := artifact.mount(owner, mountID)
	mountedOK := mounted.Template != nil && mounted.Module.Available()
	witness := bootstrapTransportLawWitness(t, owner, 1)
	if !mountedOK {
		t.Fatal("bootstrap transport mount")
	}
	substituted, substitutedOK := NewLinkBootstrapWitnessByCapability(bootstrapTransportLawID(1, 31), LinkBootstrapPoint{PointID: bootstrapTransportLawID(1, 32), Known: true, Initial: true}, LinkBootstrapCatalog{Capability: owner.value}, LinkBootstrapCatalog{Capability: owner.excluded})
	foreignAssembly, foreignFailure, foreignAssembled := beginMountedProgramMounts(owner.binding, []MountedProgramArtifact{mounted}, substituted)
	if !substitutedOK || !substituted.Available() || foreignAssembled || foreignAssembly != nil || foreignFailure != receiptAssemblyFailureSnapshotBootstrap {
		t.Fatal("same-binding third Link factor substituted for authorized Heap transport")
	}
	reversed, reversedOK := NewLinkBootstrapWitnessByCapability(bootstrapTransportLawID(1, 33), LinkBootstrapPoint{PointID: bootstrapTransportLawID(1, 34), Known: true, Initial: true}, LinkBootstrapCatalog{Capability: owner.heap}, LinkBootstrapCatalog{Capability: owner.value})
	reversedAssembly, reversedFailure, reversedAssembled := beginMountedProgramMounts(owner.binding, []MountedProgramArtifact{mounted}, reversed)
	if !reversedOK || !reversed.Available() || reversedAssembled || reversedAssembly != nil || reversedFailure != receiptAssemblyFailureSnapshotBootstrap {
		t.Fatal("authorized Link bootstrap transport order was not retained")
	}
	topology, graph, rows := commitBootstrapTransportLaw(t, owner.binding, []MountedProgramArtifact{mounted}, witness)
	initialID := mountedArtifactID("analysis/engine/artifact-point/v1", mountID, artifact.artifact, artifact.initial)
	noninitialID := mountedArtifactID("analysis/engine/artifact-point/v1", mountID, artifact.artifact, artifact.noninitial)
	initialRef, initialRefOK := rows.pointRef[initialID]
	noninitialRef, noninitialRefOK := rows.pointRef[noninitialID]
	if !initialRefOK || !noninitialRefOK || initialRef == noninitialRef || len(topology.carrier.spec.EnvironmentEdges) != 0 || len(topology.carrier.spec.FactorEdges) != 2 || graph.graph.FactorEdgeTotal() != 2 {
		t.Fatal("bootstrap transport exact edge census")
	}
	wantFactors := []composition.Key{owner.valueFactor, owner.heapFactor}
	for index, edge := range topology.carrier.spec.FactorEdges {
		wantProvenance, provenanceOK := linkBootstrapTransportKey(rows.bootstrap.owner, rows.pointMeta[initialID], wantFactors[index])
		if edge.Target != initialRef || edge.Target == noninitialRef || edge.Factor != wantFactors[index] || edge.Factor == owner.other || !edge.Input.Source().Same(rows.bootstrap.site) || !edge.Input.Target().Same(rows.sites[initialID]) || edge.Input.Target().Same(rows.sites[noninitialID]) || !provenanceOK || edge.Input.Provenance() != wantProvenance {
			t.Fatalf("bootstrap transport row %d escaped exact initial Value/Heap plane", index)
		}
	}
	// The initial Point of a mount is read from the sealed template on every
	// construction, so a mount declaring a second one publishes no geometry.
	ambiguous := newBootstrapTransportLawArtifactWithInitials(t, owner.schema, 1, true, true)
	ambiguousMounts, ambiguousSealed := sealMountedProgramArtifacts([]MountedProgramArtifact{ambiguous.mount(owner, mountID)})
	admission, admissionFailure, admitted := beginMountedProgramMounts(owner.binding, []MountedProgramArtifact{mounted}, witness)
	if !ambiguousSealed || !admitted || admission == nil || admissionFailure != receiptAssemblyFailureNone || !admission.SealSources() {
		t.Fatal("second mounted initial point declaration")
	}
	ambiguousDeclaration, ambiguousDeclared := declareSealedTopology(admission, ambiguousMounts, witness)
	ambiguousDeclaration.observationQueries = true
	ambiguousConstructed, ambiguousRefusal := constructTopology(ambiguousDeclaration)
	if !ambiguousDeclared || ambiguousConstructed.Available() || ambiguousRefusal.Stage() != ProgramConstructionStageAdmission || ambiguousRefusal.Step() != topologyConstructionStepMountRow {
		t.Fatalf("second mounted initial point admitted declared=%t stage=%v step=%v published=%t", ambiguousDeclared, ambiguousRefusal.Stage(), ambiguousRefusal.Step(), ambiguousConstructed.Available())
	}
}

func TestLinkBootstrapTransportRejectsForeignCapabilityAtSnapshot(t *testing.T) {
	owner := newBootstrapTransportLawOwner(t)
	foreign := bindBootstrapTransportLawOwner(t, owner)
	foreignBinding, foreignValue, foreignHeap := foreign.binding, foreign.value, foreign.heap
	artifact := newBootstrapTransportLawArtifact(t, owner.schema, 2)
	mounted := artifact.mount(owner, bootstrapTransportLawID(2, 30))
	mountedOK := mounted.Template != nil && mounted.Module.Available()
	witness, witnessOK := NewLinkBootstrapWitnessByCapability(bootstrapTransportLawID(2, 31), LinkBootstrapPoint{PointID: bootstrapTransportLawID(2, 32), Known: true, Initial: true}, LinkBootstrapCatalog{Capability: owner.value}, LinkBootstrapCatalog{Capability: foreignHeap})
	assembly, failure, assembled := beginMountedProgramMounts(owner.binding, []MountedProgramArtifact{mounted}, witness)
	if foreignBinding == nil || !foreignValue.Link() || !mountedOK || !witnessOK || assembled || assembly != nil || failure != receiptAssemblyFailureSnapshotBootstrap {
		t.Fatal("foreign equal-schema bootstrap capability crossed owner fence")
	}
}

func TestLinkBootstrapTransportRejectsMissingOrAmbiguousMountedInitialPoint(t *testing.T) {
	owner := newBootstrapTransportLawOwner(t)
	witness := bootstrapTransportLawWitness(t, owner, 4)
	for index, initials := range [][2]bool{{false, false}, {true, true}} {
		artifact := newBootstrapTransportLawArtifactWithInitials(t, owner.schema, byte(4+index), initials[0], initials[1])
		mounted := artifact.mount(owner, bootstrapTransportLawID(byte(4+index), 30))
		mountedOK := mounted.Template != nil && mounted.Module.Available()
		assembly, failure, assembled := beginMountedProgramMounts(owner.binding, []MountedProgramArtifact{mounted}, witness)
		if !mountedOK || assembled || assembly != nil || failure != receiptAssemblyFailureSnapshotTopologyPoint {
			t.Fatalf("mounted initial cardinality %v did not fail closed", initials)
		}
	}
}

func TestLinkBootstrapTransportTwoMountOrderAndResealAreCanonical(t *testing.T) {
	owner := newBootstrapTransportLawOwner(t)
	artifact := newBootstrapTransportLawArtifact(t, owner.schema, 3)
	firstMount, secondMount := bootstrapTransportLawID(3, 30), bootstrapTransportLawID(3, 31)
	first := artifact.mount(owner, firstMount)
	firstOK := first.Template != nil && first.Module.Available()
	second := artifact.mount(owner, secondMount)
	secondOK := second.Template != nil && second.Module.Available()
	if !firstOK || !secondOK {
		t.Fatal("two-mount bootstrap transport receipts")
	}
	witness := bootstrapTransportLawWitness(t, owner, 3)
	firstTopology, firstGraph, firstRows := commitBootstrapTransportLaw(t, owner.binding, []MountedProgramArtifact{first, second}, witness)
	secondTopology, secondGraph, secondRows := commitBootstrapTransportLaw(t, owner.binding, []MountedProgramArtifact{first, second}, witness)
	_ = secondRows
	if len(firstTopology.carrier.spec.FactorEdges) != 4 || len(secondTopology.carrier.spec.FactorEdges) != 4 || firstGraph.graph.FactorEdgeTotal() != 4 || secondGraph.graph.FactorEdgeTotal() != 4 {
		t.Fatal("two-mount bootstrap transport census")
	}
	wantMounts := []identity.ContentID{firstMount, secondMount, firstMount, secondMount}
	wantFactors := []composition.Key{owner.valueFactor, owner.valueFactor, owner.heapFactor, owner.heapFactor}
	seenProvenance := make(map[composition.Key]struct{}, 4)
	for index, edge := range firstTopology.carrier.spec.FactorEdges {
		mountedInitial := mountedArtifactID("analysis/engine/artifact-point/v1", wantMounts[index], artifact.artifact, artifact.initial)
		metadata := firstRows.pointMeta[mountedInitial]
		wantProvenance, provenanceOK := linkBootstrapTransportKey(firstRows.bootstrap.owner, metadata, wantFactors[index])
		secondEdge := secondTopology.carrier.spec.FactorEdges[index]
		if !provenanceOK || edge.Factor != wantFactors[index] || edge.Input.Provenance() != wantProvenance || secondEdge.Factor != edge.Factor || secondEdge.Input.Provenance() != edge.Input.Provenance() {
			t.Fatalf("bootstrap transport order/reseal row %d", index)
		}
		if _, duplicate := seenProvenance[wantProvenance]; duplicate {
			t.Fatal("cross-mount bootstrap transport provenance alias")
		}
		seenProvenance[wantProvenance] = struct{}{}
	}
	if len(firstTopology.carrier.spec.PointRanks) != 5 || firstTopology.carrier.spec.PointRanks[0] != 1 || firstTopology.carrier.spec.PointRanks[1] != 2 || firstTopology.carrier.spec.PointRanks[2] != 3 || firstTopology.carrier.spec.PointRanks[3] != 4 || firstTopology.carrier.spec.PointRanks[4] != 0 {
		t.Fatal("bootstrap rank did not precede authenticated parent mount order")
	}
	for index := 0; index < firstGraph.graph.FactorEdgeTotal(); index++ {
		left, leftOK := firstGraph.graph.FactorEdgeAtIndex(index)
		right, rightOK := secondGraph.graph.FactorEdgeAtIndex(index)
		if !leftOK || !rightOK || left.Key() != right.Key() {
			t.Fatal("bootstrap transport graph identity changed across reseal")
		}
	}
	reversedTopology, _, reversedRows := commitBootstrapTransportLaw(t, owner.binding, []MountedProgramArtifact{second, first}, witness)
	for index, edge := range reversedTopology.carrier.spec.FactorEdges {
		wantMount := wantMounts[index^1]
		mountedInitial := mountedArtifactID("analysis/engine/artifact-point/v1", wantMount, artifact.artifact, artifact.initial)
		if edge.Target != reversedRows.pointRef[mountedInitial] {
			t.Fatal("bootstrap transport ignored parent-authenticated mount order")
		}
	}
}

func TestLinkBootstrapValueAndHeapProducersReachMountedInitialAfterReleaseAndRevision(t *testing.T) {
	owner := newBootstrapTransportLawOwner(t)
	foreignValue := bindBootstrapTransportLawOwner(t, owner).value
	artifact := newBootstrapTransportLawArtifact(t, owner.schema, 8)
	mountID := bootstrapTransportLawID(8, 30)
	mounted := artifact.mount(owner, mountID)
	mountedOK := mounted.Template != nil && mounted.Module.Available()
	valueOccurrence, heapOccurrence := bootstrapTransportLawID(8, 40), bootstrapTransportLawID(8, 41)
	witness, witnessOK := NewLinkBootstrapWitnessByCapability(
		bootstrapTransportLawID(8, 20),
		LinkBootstrapPoint{PointID: bootstrapTransportLawID(8, 21), Known: true, Initial: true},
		LinkBootstrapCatalog{Capability: owner.value, Occurrences: []identity.ContentID{valueOccurrence}},
		LinkBootstrapCatalog{Capability: owner.heap, Occurrences: []identity.ContentID{heapOccurrence}},
	)
	sealedMounts, sealedOK := sealMountedProgramArtifacts([]MountedProgramArtifact{mounted})
	assembly, failure, assemblyOK := beginMountedProgramMounts(owner.binding, []MountedProgramArtifact{mounted}, witness)
	if !mountedOK || !witnessOK || !sealedOK || !assemblyOK || assembly == nil || failure != receiptAssemblyFailureNone {
		t.Fatalf("bootstrap producer assembly failure=%d", failure)
	}
	occurrences := []identity.ContentID{valueOccurrence, heapOccurrence}
	capabilities := []RuleSlotCapability{owner.value, owner.heap}
	for index := range occurrences {
		queueBootstrapTransportLawProducer(t, assembly, capabilities[index], occurrences[index], owner.implementations[index])
	}
	unrelatedMemberID := mountedRuleMemberID(owner.mounted, mountID, artifact.noninitial, artifact.producer)
	queueBootstrapTransportLawMountedProducer(t, assembly, owner.mounted, mountID, artifact.noninitial, artifact.producer, owner.implementations[2])
	activationID := mountedRuleActivationID(owner.activation, mountID, artifact.noninitial, artifact.activation)
	queueBootstrapTransportLawMountedActivation(t, assembly, owner.activation, mountID, artifact.noninitial, artifact.activation, owner.activationImplementation)
	queryIDs := []identity.ContentID{bootstrapTransportLawID(8, 50), bootstrapTransportLawID(8, 51), bootstrapTransportLawID(8, 52), bootstrapTransportLawID(8, 53)}
	// The first three queries read at the mounted initial Point the Link
	// bootstrap transports land on; the fourth reads the excluded lane's factor
	// at the non-initial Point that lane writes.
	queryPoints := []identity.ContentID{artifact.initial, artifact.initial, artifact.initial, artifact.noninitial}
	queried := 0
	if !assembly.QueueMountedQueryBatch(func(batch *MountedQueryBatch) bool {
		for index := range queryIDs {
			if !AddMountedExactQuery(batch, owner.queryImplementations[index], queryIDs[index], mountID, queryPoints[index]) {
				return false
			}
			queried++
		}
		return true
	}) {
		t.Fatal("bootstrap producer query batch")
	}
	if !assembly.SealSources() {
		t.Fatalf("bootstrap producer source seal=%+v", assembly.sealFailure)
	}
	rows := assembly.mountedRows
	if queried != len(queryIDs) {
		t.Fatalf("bootstrap producer mounted query %d", queried)
	}
	initialID := mountedArtifactID("analysis/engine/artifact-point/v1", mountID, artifact.artifact, artifact.initial)
	noninitialID := mountedArtifactID("analysis/engine/artifact-point/v1", mountID, artifact.artifact, artifact.noninitial)
	application, target, endpoint := coldKey(947_992), coldKey(947_993), coldKey(947_994)
	if !addBootstrapTransportLawActivationCandidate(t, assembly, owner, activationID, initialID, application, target, endpoint) {
		t.Fatal("bootstrap producer activation candidate")
	}
	declaration, declared := declareSealedTopology(assembly, sealedMounts, witness)
	constructed, refusal := constructTopology(declaration)
	topology, issued := constructed.topology, constructed.graph
	committed := declared && !refusal.Available() && constructed.Available()
	if !committed || topology == nil || issued == nil {
		t.Fatalf("bootstrap producer construction declared=%t stage=%v step=%v ordinal=%d", declared, refusal.Stage(), refusal.Step(), refusal.Ordinal())
	}
	graph := CommittedProgramFrom(topology, issued)
	if graph == nil {
		t.Fatal("bootstrap producer committed program")
	}
	baseActivation, baseActivationOK := graph.ActivationMember(activationID)
	if !baseActivationOK || !baseActivation.member.Key().Available() {
		t.Fatal("bootstrap producer base activation receipt")
	}
	// The published activation identity is the mount coordinate, so the
	// coordinate accessor and the identity accessor address one row.
	coordinateActivation, coordinateActivationOK := graph.MountedActivationMember(owner.activation, mountID, artifact.noninitial, artifact.activation)
	if !coordinateActivationOK || coordinateActivation.member.Key() != baseActivation.member.Key() || coordinateActivation.locator != baseActivation.locator {
		t.Fatal("bootstrap producer activation coordinate")
	}
	bootstrapPoint, bootstrapPointOK := graph.lookupPoint(rows.bootstrap.semantic)
	initialPoint, initialPointOK := graph.lookupPoint(initialID)
	noninitialPoint, noninitialPointOK := graph.lookupPoint(noninitialID)
	demand, demandOK := graph.graph.Demand()
	if !bootstrapPointOK || !initialPointOK || !noninitialPointOK || !demandOK || demand == nil || demand.PointCount() != 3 || graph.graph.GroupCount() != 4 {
		t.Fatal("bootstrap producer demand census")
	}
	wantPoints := map[composition.Key]bool{bootstrapPoint.Key(): false, initialPoint.Key(): false, noninitialPoint.Key(): false}
	for index := 0; index < demand.PointCount(); index++ {
		point, pointOK := demand.PointAt(index)
		if !pointOK {
			t.Fatal("bootstrap producer demanded point")
		}
		if _, expected := wantPoints[point.Key()]; !expected {
			t.Fatal("bootstrap producer demand escaped exact source/target cone")
		}
		wantPoints[point.Key()] = true
	}
	for _, seen := range wantPoints {
		if !seen {
			t.Fatal("bootstrap producer demand omitted source or target")
		}
	}
	// The two Link producers are rooted at the Link bootstrap Point; the mounted
	// producer and the activation trigger are rooted at the mounted Point their
	// parent-issued rule rows are placed on.
	wantGroupRoots := map[composition.Key]int{bootstrapPoint.Key(): 2, noninitialPoint.Key(): 2}
	seenGroupRoots := make(map[composition.Key]int, len(wantGroupRoots))
	for index := 0; index < graph.graph.GroupCount(); index++ {
		group, groupOK := graph.graph.HyperedgeAt(index)
		if !groupOK || group.MemberCount() != 1 {
			t.Fatalf("bootstrap producer group %d", index)
		}
		if _, expected := wantGroupRoots[group.Output().Key()]; !expected {
			t.Fatalf("bootstrap producer group %d not rooted at a declared member Point", index)
		}
		seenGroupRoots[group.Output().Key()]++
	}
	for key, want := range wantGroupRoots {
		if seenGroupRoots[key] != want {
			t.Fatalf("bootstrap producer group root census %d/%d", seenGroupRoots[key], want)
		}
	}
	initialRelation, relationOK := topology.topology.InitialRelation()
	selected, selectedOK := topology.topology.SelectReceiptMember(baseActivation.member.Key(), equation.PairLocator{Application: compositionKeyOf(application), Target: compositionKeyOf(target), Endpoint: compositionKeyOf(endpoint)})
	accepted, acceptedOK := topology.topology.Accept(selected, equation.TrueExpr())
	acceptedRows := []equation.AcceptedMember{accepted}
	acceptedRelation, acceptedRelationOK := topology.topology.Publish(initialRelation, acceptedRows)
	if !relationOK || !initialRelation.Available() || !selectedOK || !acceptedOK || !accepted.Available() || !acceptedRelationOK || !acceptedRelation.Available() || acceptedRelation.Digest() == initialRelation.Digest() {
		t.Fatal("bootstrap producer accepted structural revision")
	}
	if !initialRelation.Precedes(acceptedRelation) || acceptedRelation.Generation() != initialRelation.Generation().Next() {
		t.Fatal("bootstrap producer publication did not advance exactly one generation")
	}
	unknownOccurrence := bootstrapTransportLawID(8, 60)
	if _, ok := graph.LinkRuleMember(owner.excluded, valueOccurrence); ok {
		t.Fatal("bootstrap producer admitted wrong same-binding Link role")
	}
	if _, ok := graph.LinkRuleMember(owner.value, unknownOccurrence); ok {
		t.Fatal("bootstrap producer admitted unknown Link occurrence")
	}
	if _, ok := graph.LinkRuleMember(foreignValue, valueOccurrence); ok {
		t.Fatal("bootstrap producer admitted foreign-binding Link capability")
	}
	baseMembers := make([]ProgramMember, 0, len(occurrences))
	for index := range occurrences {
		member, memberOK := graph.LinkRuleMember(capabilities[index], occurrences[index])
		if !memberOK {
			t.Fatalf("bootstrap producer pre-release Link member %d", index)
		}
		baseMembers = append(baseMembers, member)
	}
	siblingGraph, siblingGraphOK := initialCommittedProgram(topology)
	siblingMembers := make([]ProgramMember, 0, len(occurrences))
	if !siblingGraphOK || siblingGraph == nil || siblingGraph == graph || siblingGraph.graph != graph.graph {
		t.Fatal("bootstrap producer sibling graph receipt")
	}
	for index := range occurrences {
		member, memberOK := siblingGraph.LinkRuleMember(capabilities[index], occurrences[index])
		if !memberOK || member.member.Key() != baseMembers[index].member.Key() || member.locator != baseMembers[index].locator {
			t.Fatalf("bootstrap producer sibling graph Link locator %d", index)
		}
		siblingMembers = append(siblingMembers, member)
	}
	unrelatedBaseMember, unrelatedBaseMemberOK := graph.RuleMember(unrelatedMemberID)
	coordinateProducer, coordinateProducerOK := graph.MountedRuleMember(owner.mounted, mountID, artifact.noninitial, artifact.producer)
	if !unrelatedBaseMemberOK || !coordinateProducerOK || coordinateProducer.member.Key() != unrelatedBaseMember.member.Key() || coordinateProducer.locator != unrelatedBaseMember.locator {
		t.Fatal("bootstrap producer mounted member coordinate")
	}
	baseQueries := make([]ProgramQuery, len(queryIDs))
	for index := range queryIDs {
		query, queryOK := graph.Query(queryIDs[index])
		if !queryOK {
			t.Fatalf("bootstrap producer base query %d", index)
		}
		baseQueries[index] = query
	}
	baseFactorKeys := make([]composition.Key, graph.graph.FactorEdgeTotal())
	for index := range baseFactorKeys {
		edge, edgeOK := graph.graph.FactorEdgeAtIndex(index)
		if !edgeOK {
			t.Fatal("bootstrap producer base factor edge")
		}
		baseFactorKeys[index] = edge.Key()
	}
	bootstrapOwner := topology.bootstrapOwner
	topology.bootstrapOwner = unknownOccurrence
	if graph.ReleaseArtifact() {
		t.Fatal("bootstrap producer released mismatched retained owner")
	}
	topology.bootstrapOwner = bootstrapOwner
	if !unrelatedBaseMemberOK || !graph.ReleaseArtifact() {
		t.Fatal("bootstrap producer release receipts")
	}
	retainedOwner, retainedPoint, retainedSemantic := topology.bootstrapOwner, topology.bootstrapPoint, topology.bootstrapSemantic
	topology.bootstrapOwner, topology.bootstrapPoint, topology.bootstrapSemantic = identity.ContentID{}, identity.ContentID{}, identity.ContentID{}
	if graph.valid() {
		t.Fatal("bootstrap producer accepted zeroed released bootstrap identity")
	}
	substitutedPoint := bootstrapTransportLawID(8, 61)
	topology.bootstrapOwner, topology.bootstrapPoint, topology.bootstrapSemantic = unknownOccurrence, substitutedPoint, linkBootstrapPointSemanticID(unknownOccurrence, substitutedPoint)
	if graph.valid() {
		t.Fatal("bootstrap producer accepted substituted released bootstrap coordinates")
	}
	topology.bootstrapOwner, topology.bootstrapPoint, topology.bootstrapSemantic = retainedOwner, retainedPoint, retainedSemantic
	for index := range occurrences {
		member, memberOK := graph.LinkRuleMember(capabilities[index], occurrences[index])
		if !memberOK || member.member.Key() != baseMembers[index].member.Key() || member.locator != baseMembers[index].locator {
			t.Fatalf("bootstrap producer post-release Link member %d", index)
		}
	}
	issued, issuedOK := topology.Graph(acceptedRelation)
	revisionGraph := CommittedProgramFrom(topology, issued)
	if !issuedOK || revisionGraph == nil || revisionGraph == graph || revisionGraph.graph == graph.graph || revisionGraph.graph.FactorEdgeTotal() != len(baseFactorKeys) {
		t.Fatal("bootstrap producer accepted initial revision")
	}
	for index, key := range baseFactorKeys {
		edge, edgeOK := revisionGraph.graph.FactorEdgeAtIndex(index)
		if !edgeOK || edge.Key() != key {
			t.Fatalf("bootstrap producer revision factor edge %d", index)
		}
	}
	for index := range occurrences {
		member, memberOK := revisionGraph.LinkRuleMember(capabilities[index], occurrences[index])
		if !memberOK || member.member.Key() != baseMembers[index].member.Key() || member.locator != baseMembers[index].locator {
			t.Fatalf("bootstrap producer revision Link member locator %d", index)
		}
	}
	unrelatedRevisionMember, unrelatedRevisionMemberOK := revisionGraph.RuleMember(unrelatedMemberID)
	revisionActivation, revisionActivationOK := revisionGraph.ActivationMember(activationID)
	if !unrelatedRevisionMemberOK || unrelatedRevisionMember.member.Key() != unrelatedBaseMember.member.Key() || unrelatedRevisionMember.locator != unrelatedBaseMember.locator || !revisionActivationOK || revisionActivation.member.Key() != baseActivation.member.Key() || revisionActivation.locator != baseActivation.locator {
		t.Fatal("bootstrap producer revision ordinary/activation member locators")
	}
	baseCompilation, baseCompilationOK := BeginProgramConstruction(owner.binding, graph)
	if !baseCompilationOK || baseCompilation == nil {
		t.Fatal("bootstrap producer base receipt compilation")
	}
	for index := range occurrences {
		if !owner.implementations[index].HasOperandResolver() && !installConstOperandResolver(owner.implementations[index], struct{}{}) {
			t.Fatalf("bootstrap producer base resolver %d", index)
		}
		if attached := AttachLinkRuleMember(baseCompilation, owner.implementations[index], capabilities[index], occurrences[index]); !attached {
			t.Fatalf("bootstrap producer base member attachment %d", index)
		}
	}
	if !baseCompilation.Close() {
		t.Fatal("bootstrap producer base compilation close")
	}
	compilation, compilationOK := BeginProgramConstruction(owner.binding, revisionGraph)
	if !compilationOK || compilation == nil {
		t.Fatal("bootstrap producer receipt compilation")
	}
	for index := range occurrences {
		if _, memberOK := revisionGraph.LinkRuleMember(capabilities[index], occurrences[index]); !memberOK {
			t.Fatalf("bootstrap producer released member %d", index)
		}
		if !owner.implementations[index].HasOperandResolver() && !installConstOperandResolver(owner.implementations[index], struct{}{}) {
			t.Fatalf("bootstrap producer resolver %d", index)
		}
		if attached := AttachLinkRuleMember(compilation, owner.implementations[index], capabilities[index], occurrences[index]); !attached {
			t.Fatalf("bootstrap producer member attachment %d", index)
		}
	}
	if !owner.implementations[2].HasOperandResolver() && !installConstOperandResolver(owner.implementations[2], struct{}{}) {
		t.Fatal("bootstrap producer unrelated resolver")
	}
	if attached := AttachRuleMember(compilation, owner.implementations[2], unrelatedMemberID); !attached {
		t.Fatal("bootstrap producer unrelated member attachment")
	}
	if attached := AttachActivationMember(compilation, owner.activationImplementation, activationID); !attached {
		t.Fatal("bootstrap producer activation member attachment")
	}
	queries := make([]ProgramQuery, len(queryIDs))
	for index := range queryIDs {
		query, queryOK := revisionGraph.Query(queryIDs[index])
		if !queryOK || query.identity.Key() != baseQueries[index].identity.Key() || query.locator != baseQueries[index].locator || !AttachExactQuery(compilation, owner.queryImplementations[index], queryIDs[index]) {
			t.Fatalf("bootstrap producer released query %d", index)
		}
		queries[index] = query
	}
	solver, _, solverOK := compilation.Seal()
	if !solverOK || solver == nil {
		t.Fatal("bootstrap producer solver")
	}
	state, status, report := solver.SolveWithReport(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("bootstrap producer solve status=%v report=%t reason=%v phase=%v", status, report.Available(), report.Reason(), report.Failure())
	}
	if !initialRelation.Precedes(solver.relation) || solver.runtime == nil || solver.runtime.graph == nil || solver.runtime.graph == graph.graph || owner.activationRuns.Load() == 0 {
		t.Fatalf("bootstrap producer runtime revision=%d runtime=%t distinct=%t activationRuns=%d", solver.relation.Generation(), solver.runtime != nil, solver.runtime != nil && solver.runtime.graph != graph.graph, owner.activationRuns.Load())
	}
	for index, counter := range owner.transfers {
		if counter.Load() == 0 {
			t.Fatalf("bootstrap producer transfer %d did not execute", index)
		}
	}
	wantValues := []uint64{41, 73, ^uint64(0), 105}
	for index, query := range queries {
		key, keyed := query.PublicationKey()
		if !keyed {
			t.Fatalf("bootstrap producer query %d has no snapshot key", index)
		}
		value, readable := testSnapshotQueryValue[uint64](solver, state, key)
		if !readable || value != wantValues[index] {
			t.Fatalf("bootstrap producer result %d=%d/%t", index, value, readable)
		}
	}
}

// queueBootstrapTransportLawMountedProducer admits one ordinary mounted member
// under the parent-issued rule row the template declares for it. The member and
// activation identities it publishes are the mount coordinates, never a
// caller-chosen identity.
func queueBootstrapTransportLawMountedProducer(t testing.TB, assembly *BindingTopologyBuilder, capability RuleSlotCapability, mount, point, occurrenceID identity.ContentID, implementation *RuleImplementation[uint64, uint64, struct{}]) {
	t.Helper()
	if assembly == nil || implementation == nil {
		t.Fatal("bootstrap mounted producer arguments")
	}
	occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(capability, mount, point, occurrenceID)
	operand, operandOK := BeginMountedRuleOccurrence(assembly, implementation, occurrence, struct{}{})
	proof := implementation.binding.proof
	if !occurrenceOK || !operandOK || proof == nil {
		t.Fatal("bootstrap mounted producer occurrence")
	}
	if !assembly.QueueMountedRuleFinalizer(capability, func() bool {
		source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
			Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence.value, Operand: operand.value,
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
		})
		draft, draftOK := implementation.beginBindingRuleRow(source)
		write, writeOK := implementation.WritePart(source, 0)
		if !sourceOK || !draftOK || !writeOK || !draft.AddWrite(write) {
			return false
		}
		row, rowOK := assembly.issueRuleRow(draft)
		_, added := assembly.AddRule(occurrence, row)
		return rowOK && added
	}) {
		t.Fatal("bootstrap mounted producer finalizer")
	}
}

// queueBootstrapTransportLawMountedActivation admits the activation trigger
// under the parent-issued activation rule row. The trigger publishes the mount
// activation coordinate, which is the identity the committed program answers
// ActivationMember from.
func queueBootstrapTransportLawMountedActivation(t testing.TB, assembly *BindingTopologyBuilder, capability RuleSlotCapability, mount, point, occurrenceID identity.ContentID, implementation *ActivationRuleImplementation) {
	t.Helper()
	if assembly == nil || implementation == nil {
		t.Fatal("bootstrap mounted activation arguments")
	}
	occurrence, occurrenceOK := assembly.AdmitMountedRuleOccurrence(capability, mount, point, occurrenceID)
	operand, operandOK := assembly.AdmitMountedRuleOperand(occurrence, [32]byte(occurrenceID))
	proof := implementation.binding.proof
	if !occurrenceOK || !operandOK || proof == nil {
		t.Fatal("bootstrap mounted activation occurrence")
	}
	if !assembly.QueueMountedRuleFinalizer(capability, func() bool {
		source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence.value, Operand: operand.value})
		draft, draftOK := implementation.beginBindingRuleRow(source)
		if !sourceOK || !draftOK {
			return false
		}
		row, rowOK := assembly.issueRuleRow(draft)
		return rowOK && assembly.AddActivationRule(occurrence, row)
	}) {
		t.Fatal("bootstrap mounted activation finalizer")
	}
}

func addBootstrapTransportLawActivationCandidate(t testing.TB, assembly *BindingTopologyBuilder, owner bootstrapTransportLawOwner, activationID, initialID identity.ContentID, application, target, endpoint identity.SemanticKey) bool {
	t.Helper()
	if assembly == nil || assembly.inner == nil || assembly.inner.semantic == nil || owner.activationImplementation == nil {
		return false
	}
	trigger, registered := assembly.inner.semantic.activations[activationID]
	triggerOrdinal := int(uint64(trigger)) - 1
	if !registered || triggerOrdinal < 0 || triggerOrdinal >= len(assembly.inner.spec.Rules) {
		return false
	}
	bootstrapSite := assembly.mountedRows.bootstrap.site
	initialSite, initialOK := assembly.mountedRows.sites[initialID]
	formals := equation.NewBatch()
	input, inputOK := formals.AdmitFormalPort(compositionKeyOf(coldKey(947_995)), equation.PortImport, nil)
	output, outputOK := formals.AdmitFormalPort(compositionKeyOf(coldKey(947_996)), equation.PortExport, nil)
	if !initialOK || !inputOK || !outputOK || !formals.Seal() {
		return false
	}
	binding, bindingOK := equation.SealTemplateBinding(formals, assembly.inner.batch, []equation.FormalPortActual{{Role: input, Site: bootstrapSite}, {Role: output, Site: initialSite}})
	materialization, materializationOK := equation.MaterializeTemplateBoundary(owner.schema.cold, binding, []equation.Site{input.Site(), output.Site()}, nil)
	proof := owner.activationImplementation.binding.proof
	if proof == nil {
		return false
	}
	shape, shapeOK := owner.schema.cold.RuleShapeAt(proof.ordinal)
	materialization, originOK := materialization.WithOrigin(equation.MaterializationOrigin{
		Family: shape.ActivationFamily, Application: compositionKeyOf(application), Target: compositionKeyOf(target), Endpoint: compositionKeyOf(endpoint), TriggerOrdinal: triggerOrdinal,
	})
	receipt, receiptOK := assembly.issueMaterialization(materialization)
	return bindingOK && materializationOK && shapeOK && originOK && receiptOK && assembly.addActivationCandidate(receipt)
}

func queueBootstrapTransportLawProducer(t testing.TB, assembly *BindingTopologyBuilder, capability RuleSlotCapability, occurrenceID identity.ContentID, implementation *RuleImplementation[uint64, uint64, struct{}]) {
	t.Helper()
	occurrence, occurrenceOK := assembly.AdmitLinkRuleOccurrence(capability, occurrenceID)
	operand, operandOK := BeginMountedRuleOccurrence(assembly, implementation, occurrence, struct{}{})
	if !occurrenceOK || !operandOK {
		t.Fatal("bootstrap producer occurrence")
	}
	proof := implementation.binding.proof
	if proof == nil {
		t.Fatal("bootstrap producer proof")
	}
	if !assembly.QueueLinkRuleFinalizer(capability, func() bool {
		source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
			Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence.value, Operand: operand.value,
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
		})
		draft, draftOK := implementation.beginBindingRuleRow(source)
		write, writeOK := implementation.WritePart(source, 0)
		if !sourceOK || !draftOK || !writeOK || !draft.AddWrite(write) {
			return false
		}
		row, rowOK := assembly.issueRuleRow(draft)
		_, added := assembly.AddRule(occurrence, row)
		return rowOK && added
	}) {
		t.Fatal("bootstrap producer finalizer")
	}
}
