package engine

import (
	"go/ast"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

// constructTopologyFile is the one file the pure program-geometry function
// lives in. Every law here reads that function's sealed outputs directly; none
// of them compares against a second construction.
const constructTopologyFile = "construct_topology.go"

// constructionTransactionTypes are the mutable construction surfaces the pure
// function must be structurally unable to reach. Naming one of them - the type
// itself or any method declared on it - would make the function a wrapper
// around the transaction it replaces rather than an independent derivation.
var constructionTransactionTypes = []string{
	"BindingTopologyBuilder",
	"RuleProgramAttach",
	"ProgramConstruction",
}

// constructionTransactionFunctions are the transaction entry points.
var constructionTransactionFunctions = []string{
	"AssembleMountedProgram",
	"BeginProgramConstruction",
	"BeginMountedProgram",
	"assembleSealedProgramMounts",
	"beginMountedProgramMounts",
	"beginMountedProgramAssembly",
	"beginBindingTopologyBuilder",
	"applyMountedProgramAdmission",
	"admitMountedArtifactSites",
	"buildMountedArtifactRows",
}

// constructionCarrierIdentifiers are the sealed committed values the geometry
// publishes into. bindingTopologyCarrier is the sealed value a committed
// BindingTopology owns - the Batch its rows were admitted into, the key that
// Batch sealed to, and the spec the topology was sealed from. Constructing
// that finished value is publication, not delegation, so it is named here
// rather than fenced.
var constructionCarrierIdentifiers = []string{
	"bindingTopologyCarrier",
}

// TestConstructTopologyCannotNameTheConstructionTransaction is the leak fence.
// The pure function is only pure if it cannot delegate: the fenced identifier
// set is every construction-transaction type, every method declared on one,
// and every transaction entry point.
func TestConstructTopologyCannotNameTheConstructionTransaction(t *testing.T) {
	fenced := make(map[string]string, len(constructionTransactionTypes)+len(constructionTransactionFunctions))
	owned := make(map[string]bool, len(constructionTransactionTypes))
	for _, name := range constructionTransactionTypes {
		fenced[name] = name
		owned[name] = true
	}
	for _, name := range constructionTransactionFunctions {
		fenced[name] = name
	}
	for _, file := range engineSourceFiles(t) {
		if file == constructTopologyFile {
			continue
		}
		parsed := parseEngineFile(t, file)
		for _, declaration := range parsed.Decls {
			function, isFunction := declaration.(*ast.FuncDecl)
			if !isFunction || function.Recv == nil || len(function.Recv.List) != 1 {
				continue
			}
			receiver := function.Recv.List[0].Type
			if star, pointer := receiver.(*ast.StarExpr); pointer {
				receiver = star.X
			}
			named, isNamed := receiver.(*ast.Ident)
			if !isNamed || !owned[named.Name] {
				continue
			}
			fenced[function.Name.Name] = named.Name + "." + function.Name.Name
		}
		if !owned["RuleProgramAttach"] {
			continue
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			spec, isSpec := node.(*ast.TypeSpec)
			if !isSpec || spec.Name.Name != "RuleProgramAttach" {
				return true
			}
			surface, isInterface := spec.Type.(*ast.InterfaceType)
			if !isInterface || surface.Methods == nil {
				return true
			}
			for _, method := range surface.Methods.List {
				for _, name := range method.Names {
					fenced[name.Name] = "RuleProgramAttach." + name.Name
				}
			}
			return true
		})
	}
	if len(fenced) < len(constructionTransactionTypes)+len(constructionTransactionFunctions) {
		t.Fatalf("construction transaction surface not discovered: %d names", len(fenced))
	}
	for _, name := range constructionCarrierIdentifiers {
		if _, collides := fenced[name]; collides {
			t.Fatalf("carrier identifier %s is also a fenced construction surface", name)
		}
	}
	ast.Inspect(parseEngineFile(t, constructTopologyFile), func(node ast.Node) bool {
		ident, isIdent := node.(*ast.Ident)
		if !isIdent {
			return true
		}
		if source, leaked := fenced[ident.Name]; leaked {
			t.Errorf("%s names construction transaction surface %s", constructTopologyFile, source)
		}
		return true
	})
}

// constructedProgramFixture is one sealed declaration over a mounted template:
// four points in parent WTO order, one region, and three native Call stage
// rules chained base -> dispatch -> summary -> effect.
type constructedProgramFixture struct {
	owner       nativeCallStageLawFixture
	rules       []rows.ArtifactScalarRule
	declaration topologyDeclaration
	baseSite    equation.Site
	effectSite  equation.Site
}

// newConstructedProgramFixture seals one declaration.
//
// The source plane - Sites, Occurrences and Operands - is admitted through the
// production source path, because a Batch capability cannot be minted outside
// it at this step of the migration. Only the sealed Batch, the sites it
// admitted, and the sealed equation rule rows its owners issued are read back;
// no geometry is read from the admission transaction, and every law below
// measures the geometry the pure function derives from this declaration.
func newConstructedProgramFixture(t testing.TB) constructedProgramFixture {
	t.Helper()
	owner := newNativeCallStageLawOwner(t)
	rules := owner.rules()
	mount, mountOK := owner.scalarMount(t, rules, owner.order())
	bootstrap, bootstrapOK := NewLinkBootstrapWitness(artifactScalarLawID(0x68), LinkBootstrapPoint{PointID: artifactScalarLawID(0x69), Known: true, Initial: true}, nil)
	sealed, sealedOK := sealMountedProgramArtifacts([]MountedProgramArtifact{mount})
	if !mountOK || !bootstrapOK || !sealedOK || len(sealed) != 1 {
		t.Fatal("constructed program mount seal")
	}
	admission, admissionFailure, admitted := beginMountedProgramMounts(owner.binding, []MountedProgramArtifact{mount}, bootstrap)
	if !admitted || admission == nil || admissionFailure != receiptAssemblyFailureNone {
		t.Fatalf("constructed program source admission failure=%v", admissionFailure)
	}
	if !installConstOperandResolver(owner.implementation, struct{}{}) {
		t.Fatal("constructed program operand resolver")
	}
	for _, rule := range rules {
		if !owner.implementation.AdmitMounted(admission, owner.capability, owner.mount, rule.Point, rule.ID) {
			t.Fatalf("constructed program occurrence %v", rule.ID)
		}
	}
	if !admission.SealSources() {
		failure, available := admission.SealFailure()
		t.Fatalf("constructed program source seal failure=%v available=%t", failure.Failure(), available)
	}
	batch := admission.inner.batch
	issued := admission.inner.spec.Rules
	source := admission.mountedRows
	if batch == nil || !batch.Sealed() || source == nil || len(issued) != len(rules) {
		t.Fatalf("constructed program sealed source rows=%d", len(issued))
	}
	sites := constructedSitePlane{mounted: make(map[artifactMountedPoint]equation.Site, len(source.mounted)), bootstrap: source.bootstrap.site}
	for handle, site := range source.mounted {
		sites.mounted[handle] = site
	}
	members := make([]declaredMemberRow, len(rules))
	for index, rule := range rules {
		members[index] = declaredMemberRow{
			Plane: declaredMemberMount, Role: owner.capability, Mount: owner.mount,
			Point: rule.Point, Occurrence: rule.ID, Row: issued[index],
		}
	}
	fixture := constructedProgramFixture{
		owner: owner, rules: rules,
		declaration: topologyDeclaration{
			binding: owner.binding, batch: batch, mounts: sealed, bootstrap: bootstrap,
			sites: sites, members: members,
		},
	}
	baseSite, baseOK := sites.mounted[artifactMountedPoint{mount: owner.mount, reusable: owner.base}]
	effectSite, effectOK := sites.mounted[artifactMountedPoint{mount: owner.mount, reusable: owner.effect}]
	if !baseOK || !effectOK {
		t.Fatal("constructed program admitted sites")
	}
	fixture.baseSite, fixture.effectSite = baseSite, effectSite
	return fixture
}

// constructedPointOrder is the parent WTO order the fixture declares, with the
// Link bootstrap anchor last in row order and first in schedule order.
func (fixture constructedProgramFixture) constructedPointOrder() []identity.ContentID {
	return fixture.owner.order()
}

// TestConstructedTopologyPublishesInjectiveAddressTables proves the published
// tables are total and injective against the declaration: every declared point
// and member reaches exactly one distinct row, and no two declared identities
// share one address.
func TestConstructedTopologyPublishesInjectiveAddressTables(t *testing.T) {
	fixture := newConstructedProgramFixture(t)
	constructed, refusal := constructTopology(fixture.declaration)
	if refusal.Available() || !constructed.Available() {
		t.Fatalf("construction refused stage=%v step=%v ordinal=%d", refusal.Stage(), refusal.Step(), refusal.Ordinal())
	}
	program := CommittedProgramFrom(constructed.topology, constructed.graph)
	if program == nil {
		t.Fatal("committed program")
	}
	declaredPoints := len(fixture.constructedPointOrder()) + 1
	if got := constructed.graph.PointCount(); got != declaredPoints {
		t.Fatalf("graph points=%d declared=%d", got, declaredPoints)
	}
	if got := constructed.topology.topology.RuleMemberRowCount(); got != len(fixture.declaration.members) {
		t.Fatalf("member rows=%d declared=%d", got, len(fixture.declaration.members))
	}
	if got := constructed.topology.topology.QueryRowCount(); got != 0 {
		t.Fatalf("query rows=%d declared=0", got)
	}
	seenPoint := make(map[equation.PointRowLocator]identity.ContentID, declaredPoints)
	for _, reusable := range fixture.constructedPointOrder() {
		id := mountedArtifactID("analysis/engine/artifact-point/v1", fixture.owner.mount, fixture.declaration.mounts[0].template.ArtifactID(), reusable)
		locator, found := constructed.topology.directory.point(id)
		if !found {
			t.Fatalf("declared point %v publishes no address", reusable)
		}
		if prior, duplicate := seenPoint[locator]; duplicate {
			t.Fatalf("points %v and %v share one address", prior, id)
		}
		seenPoint[locator] = id
	}
	bootstrapID := linkBootstrapPointSemanticID(fixture.declaration.bootstrap.OwnerID(), artifactScalarLawID(0x69))
	locator, bootstrapFound := constructed.topology.directory.point(bootstrapID)
	if _, duplicate := seenPoint[locator]; !bootstrapFound || duplicate {
		t.Fatal("Link bootstrap anchor publishes no distinct address")
	}
	seenPoint[locator] = bootstrapID
	if len(seenPoint) != declaredPoints {
		t.Fatalf("point addresses=%d declared=%d", len(seenPoint), declaredPoints)
	}
	seenMember := make(map[equation.RuleMemberRowLocator]identity.ContentID, len(fixture.rules))
	for _, rule := range fixture.rules {
		id := mountedRuleMemberID(fixture.owner.capability, fixture.owner.mount, rule.Point, rule.ID)
		memberLocator, found := constructed.topology.directory.member(id)
		if !found {
			t.Fatalf("declared member %v publishes no address", rule.ID)
		}
		if prior, duplicate := seenMember[memberLocator]; duplicate {
			t.Fatalf("members %v and %v share one address", prior, id)
		}
		seenMember[memberLocator] = id
	}
	if len(seenMember) != len(fixture.rules) {
		t.Fatalf("member addresses=%d declared=%d", len(seenMember), len(fixture.rules))
	}
}

// TestConstructedMountedMemberFoldsIntoItsMountedPoint proves the member table
// is addressed by the coordinates it was declared under: the member published
// for role+mount+point+occurrence folds into exactly the Point that mount and
// point name, and its native Call stage answers under the same coordinates.
func TestConstructedMountedMemberFoldsIntoItsMountedPoint(t *testing.T) {
	fixture := newConstructedProgramFixture(t)
	constructed, refusal := constructTopology(fixture.declaration)
	if refusal.Available() || !constructed.Available() {
		t.Fatalf("construction refused stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
	program := CommittedProgramFrom(constructed.topology, constructed.graph)
	if program == nil {
		t.Fatal("committed program")
	}
	artifactID := fixture.declaration.mounts[0].template.ArtifactID()
	for _, rule := range fixture.rules {
		member, memberOK := program.MountedRuleMember(fixture.owner.capability, fixture.owner.mount, rule.Point, rule.ID)
		if !memberOK {
			t.Fatalf("mounted member %v unaddressed", rule.ID)
		}
		site, sited := fixture.declaration.sites.mounted[artifactMountedPoint{mount: fixture.owner.mount, reusable: rule.Point}]
		if _, pointOK := program.lookupPoint(mountedArtifactID("analysis/engine/artifact-point/v1", fixture.owner.mount, artifactID, rule.Point)); !pointOK || !sited || !member.member.Occurrence().Site().Same(site) {
			t.Fatalf("member %v folds into a foreign Point", rule.ID)
		}
		stage, stageOK := program.MountedNativeCallStage(fixture.owner.capability, fixture.owner.mount, rule.ID)
		if !stageOK || stage.Kind() != rule.Stage || stage.PointID() != rule.Point {
			t.Fatalf("native Call stage %v unaddressed stage=%v", rule.ID, rule.Stage)
		}
	}
}

// TestConstructedScheduleCoversEveryDeclaredPointExactlyOnce proves the
// schedule well-formedness the geometry publishes: every declared Point is
// scheduled exactly once, the Link bootstrap anchor holds semantic rank zero,
// and every graph region is a non-empty set of declared Points.
func TestConstructedScheduleCoversEveryDeclaredPointExactlyOnce(t *testing.T) {
	fixture := newConstructedProgramFixture(t)
	constructed, refusal := constructTopology(fixture.declaration)
	if refusal.Available() || !constructed.Available() {
		t.Fatalf("construction refused stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
	graph := constructed.graph
	compiled := graph.Schedule()
	if compiled == nil {
		t.Fatal("composed schedule")
	}
	program := CommittedProgramFrom(constructed.topology, graph)
	artifactID := fixture.declaration.mounts[0].template.ArtifactID()
	expected := make(map[equation.Point]identity.ContentID, len(fixture.constructedPointOrder())+1)
	for _, reusable := range fixture.constructedPointOrder() {
		id := mountedArtifactID("analysis/engine/artifact-point/v1", fixture.owner.mount, artifactID, reusable)
		point, pointOK := program.lookupPoint(id)
		if !pointOK {
			t.Fatalf("declared point %v is not in the composed graph", reusable)
		}
		expected[point] = id
	}
	bootstrapID := linkBootstrapPointSemanticID(fixture.declaration.bootstrap.OwnerID(), artifactScalarLawID(0x69))
	bootstrapPoint, bootstrapOK := program.lookupPoint(bootstrapID)
	if !bootstrapOK {
		t.Fatal("Link bootstrap anchor is not in the composed graph")
	}
	expected[bootstrapPoint] = bootstrapID
	scheduled := make(map[identity.ContentID]int, len(expected))
	for index := 0; index < compiled.EventCount(); index++ {
		event, eventOK := compiled.EventAt(index)
		if !eventOK {
			t.Fatalf("schedule event %d", index)
		}
		if event.Kind != schedule.EventNode {
			continue
		}
		point, pointOK := graph.PointAt(event.Node)
		id, declared := expected[point]
		if !pointOK || !declared {
			t.Fatalf("schedule event %d names an undeclared Point", index)
		}
		if _, duplicate := scheduled[id]; duplicate {
			t.Fatalf("Point %v is scheduled twice", id)
		}
		scheduled[id] = len(scheduled)
	}
	if len(scheduled) != len(expected) {
		t.Fatalf("scheduled=%d declared=%d", len(scheduled), len(expected))
	}
	if scheduled[bootstrapID] != 0 {
		t.Fatalf("Link bootstrap anchor scheduled at %d, not zero", scheduled[bootstrapID])
	}
	// Every native Call stage follows the Point its input is staged from.
	for _, rule := range fixture.rules {
		if !rule.Stage.NativeCall() || !rule.Input.Available() {
			continue
		}
		staged := scheduled[mountedArtifactID("analysis/engine/artifact-point/v1", fixture.owner.mount, artifactID, rule.Point)]
		base := scheduled[mountedArtifactID("analysis/engine/artifact-point/v1", fixture.owner.mount, artifactID, rule.Input)]
		if base >= staged {
			t.Fatalf("native Call stage %v scheduled at %d before its input at %d", rule.ID, staged, base)
		}
	}
	for index := 0; index < compiled.RegionCount(); index++ {
		view, viewOK := graph.RegionAt(index)
		if !viewOK || view.PointCount() == 0 {
			t.Fatalf("graph region %d is empty", index)
		}
		for member := 0; member < view.PointCount(); member++ {
			point, pointOK := view.PointAt(member)
			if _, declared := expected[point]; !pointOK || !declared {
				t.Fatalf("graph region %d holds an undeclared Point", index)
			}
		}
	}
}

// TestConstructedTopologyRefusesDuplicateIdentity proves one published
// ContentID addresses exactly one row: a declaration that issues the same
// mounted coordinates twice is refused at the declaration boundary and
// publishes nothing.
func TestConstructedTopologyRefusesDuplicateIdentity(t *testing.T) {
	fixture := newConstructedProgramFixture(t)
	declaration := fixture.declaration
	declaration.members = append(append([]declaredMemberRow(nil), fixture.declaration.members...), fixture.declaration.members[0])
	constructed, refusal := constructTopology(declaration)
	if constructed.Available() || refusal.Stage() != ProgramConstructionStageAdmission || refusal.Step() != topologyConstructionStepDuplicateIdentity {
		t.Fatalf("duplicate member identity admitted stage=%v step=%v published=%t", refusal.Stage(), refusal.Step(), constructed.Available())
	}
	if !refusal.Failure().Available() {
		t.Fatal("duplicate identity refusal carries no construction failure")
	}
}

// TestConstructedTopologyRefusesInadmissibleIssuance proves a member is
// admissible only where the sealed template carries its exact
// role+mount+point+occurrence rule row.
func TestConstructedTopologyRefusesInadmissibleIssuance(t *testing.T) {
	fixture := newConstructedProgramFixture(t)
	for name, mutate := range map[string]func(declaredMemberRow) declaredMemberRow{
		"occurrence": func(member declaredMemberRow) declaredMemberRow {
			member.Occurrence = artifactScalarLawID(0x5F)
			return member
		},
		"point": func(member declaredMemberRow) declaredMemberRow {
			member.Point = artifactScalarLawID(0x5E)
			return member
		},
		"mount": func(member declaredMemberRow) declaredMemberRow {
			member.Mount = artifactScalarLawID(0x5D)
			return member
		},
		"role": func(member declaredMemberRow) declaredMemberRow {
			member.Role = fixture.owner.foreignRole
			return member
		},
	} {
		declaration := fixture.declaration
		declaration.members = append([]declaredMemberRow(nil), fixture.declaration.members...)
		declaration.members[0] = mutate(declaration.members[0])
		constructed, refusal := constructTopology(declaration)
		if constructed.Available() || refusal.Stage() != ProgramConstructionStageAdmission || refusal.Step() != topologyConstructionStepMemberIssuance {
			t.Fatalf("inadmissible %s issuance admitted stage=%v step=%v published=%t", name, refusal.Stage(), refusal.Step(), constructed.Available())
		}
	}
}

// TestConstructedTopologyRefusesScheduleViolation proves the composition
// schedule gate: an owner-declared environment edge that forces a native Call
// stage ahead of the Point its input is staged from is refused at the geometry
// commit, and publishes no topology.
func TestConstructedTopologyRefusesScheduleViolation(t *testing.T) {
	fixture := newConstructedProgramFixture(t)
	reindex, reindexOK := ruleInputReindex(fixture.effectSite.Scope(), fixture.baseSite.Scope())
	provenance := compositionKeyOf(coldKey(947_311))
	input := equation.BoundaryInput(fixture.effectSite, fixture.baseSite, provenance, equation.TrueExpr(), reindex, equation.TrueExpr())
	if !reindexOK || !input.Available() {
		t.Fatal("schedule violation edge")
	}
	declaration := fixture.declaration
	// The base Point is the first row of the only mount, so it is the first
	// dense Point of the composed graph.
	declaration.environmentEdges = []equation.EnvironmentEdge{{Target: equation.PointAt(0), Input: input}}
	constructed, refusal := constructTopology(declaration)
	if constructed.Available() || refusal.Stage() != ProgramConstructionStageTopologySeal || refusal.Step() != topologyConstructionStepSchedule {
		t.Fatalf("schedule violation admitted stage=%v step=%v published=%t", refusal.Stage(), refusal.Step(), constructed.Available())
	}
	if !refusal.Failure().Available() {
		t.Fatal("schedule refusal carries no construction failure")
	}
}

// TestConstructedTopologyRefusesUnsitedDeclaration proves the geometry is
// derived only where the source plane admitted a Site for every declared
// Point: a template point with no admitted Site publishes nothing.
func TestConstructedTopologyRefusesUnsitedDeclaration(t *testing.T) {
	fixture := newConstructedProgramFixture(t)
	declaration := fixture.declaration
	sites := constructedSitePlane{mounted: make(map[artifactMountedPoint]equation.Site, len(fixture.declaration.sites.mounted)), bootstrap: fixture.declaration.sites.bootstrap}
	for handle, site := range fixture.declaration.sites.mounted {
		if handle.reusable == fixture.owner.summary {
			continue
		}
		sites.mounted[handle] = site
	}
	declaration.sites = sites
	constructed, refusal := constructTopology(declaration)
	if constructed.Available() || refusal.Stage() != ProgramConstructionStageAdmission || refusal.Step() != topologyConstructionStepPointRow {
		t.Fatalf("unsited Point admitted stage=%v step=%v published=%t", refusal.Stage(), refusal.Step(), constructed.Available())
	}
}

// constructedOwnerFixture is one sealed declaration with no mounted template:
// a single owner-declared Point, the rule row folded into it, and one query
// row published against it.
type constructedOwnerFixture struct {
	declaration topologyDeclaration
	pointID     identity.ContentID
	memberID    identity.ContentID
	queryID     identity.ContentID
}

// newConstructedOwnerFixture seals the owner-declared declaration. Its source
// plane is admitted through the production source path for the same reason as
// the mounted fixture; only the sealed Batch and the sealed equation rows are
// read back.
func newConstructedOwnerFixture(t testing.TB) constructedOwnerFixture {
	t.Helper()
	schemaBuilder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](schemaBuilder, coldKey(957_000))
	write, writeOK := factor.ExactWrite()
	read, readOK := factor.ExactRead()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](schemaBuilder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(957_001), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(957_002)}, Output: factor.Ref(),
	})
	writeSlot, writeSlotOK := SchemaWrite(rule, write)
	query, queryOK := DeclareQuerySlot[uint64](schemaBuilder, SchemaQuerySpec{Semantic: coldKey(957_003), Freezer: coldKey(957_004)})
	if !factorOK || !writeOK || !readOK || !ruleOK || !writeSlotOK || !queryOK || !SchemaQueryRead(query, read) {
		t.Fatal("owner-declared schema")
	}
	schema, schemaOK := schemaBuilder.Seal()
	if !schemaOK || schema == nil {
		t.Fatal("owner-declared schema seal")
	}
	binding := NewSchemaBinding(schema)
	ruleSpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(957_002)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}
	querySpec := hotExactQuerySpec()
	querySpec.Result.Semantic = coldKey(957_004)
	if !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, rule, writeSlot, factor, ruleSpec, testRuleProjector[ruleUnit]) ||
		!BindExactQuery(binding, query, factor, querySpec) || !binding.Seal() {
		t.Fatal("owner-declared binding")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	admission, admissionOK := binding.beginBindingTopologyBuilder()
	if !implementationOK || implementation == nil || !admissionOK || admission == nil {
		t.Fatal("owner-declared source admission")
	}
	site, siteOK := admission.admitSite(compositionKeyOf(coldKey(957_010)), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
	occurrence, occurrenceOK := admission.admitAt(site)
	operandValue := ruleUnitForSemantic(coldKey(957_011))
	entity, entityOK := operandEntityForContent(operandValue.content)
	operand, operandOK := admission.admitOperand(occurrence, entity)
	if !siteOK || !occurrenceOK || !entityOK || !operandOK || !admission.SealSources() {
		t.Fatal("owner-declared source seal")
	}
	proof := implementation.binding.proof
	source, sourceOK := admission.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
		Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrence, Operand: operand,
		Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
	})
	draft, draftOK := implementation.beginBindingRuleRow(source)
	part, partOK := implementation.WritePart(source, 0)
	if !sourceOK || !draftOK || !partOK || !draft.AddWrite(part) {
		t.Fatal("owner-declared rule draft")
	}
	issued, issuedOK := admission.issueRuleRow(draft)
	queryOrdinal, queryOrdinalOK := query.Ordinal()
	factorOrdinal, factorOrdinalOK := factor.Ordinal()
	if !issuedOK || !queryOrdinalOK || !factorOrdinalOK {
		t.Fatal("owner-declared rule row")
	}
	fixture := constructedOwnerFixture{
		pointID:  artifactScalarLawID(0x21),
		memberID: artifactScalarLawID(0x22),
		queryID:  artifactScalarLawID(0x23),
	}
	fixture.declaration = topologyDeclaration{
		binding: binding, batch: admission.inner.batch,
		points:  []declaredPointRow{{ID: fixture.pointID, Site: site}},
		members: []declaredMemberRow{{Plane: declaredMemberOwner, ID: fixture.memberID, Row: issued.row}},
		queries: []declaredQueryRow{{ID: fixture.queryID, Row: equation.QueryInstance{
			Family: schema.querySemanticAt(queryOrdinal), Point: equation.PointAt(0),
			Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(factorOrdinal), Form: equation.SurfaceReadExact, Local: 1}},
		}}},
	}
	return fixture
}

// TestConstructedOwnerDeclaredTablesArePublished proves the point, member and
// query tables of an unmounted program are total against the declaration: each
// declared identity publishes one row of its own kind, and the query table
// holds exactly the declared query rows in declared order.
func TestConstructedOwnerDeclaredTablesArePublished(t *testing.T) {
	fixture := newConstructedOwnerFixture(t)
	constructed, refusal := constructTopology(fixture.declaration)
	if refusal.Available() || !constructed.Available() {
		t.Fatalf("owner-declared construction refused stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
	if constructed.topology.artifactBacked {
		t.Fatal("unmounted program published a mounted bootstrap anchor")
	}
	program := CommittedProgramFrom(constructed.topology, constructed.graph)
	if program == nil {
		t.Fatal("owner-declared committed program")
	}
	if got := constructed.graph.PointCount(); got != 1 {
		t.Fatalf("points=%d declared=1", got)
	}
	if got := constructed.topology.topology.QueryRowCount(); got != len(fixture.declaration.queries) {
		t.Fatalf("query rows=%d declared=%d", got, len(fixture.declaration.queries))
	}
	if _, published := constructed.topology.directory.point(fixture.pointID); !published {
		t.Fatal("declared point publishes no address")
	}
	if _, published := program.RuleMember(fixture.memberID); !published {
		t.Fatal("declared member publishes no address")
	}
	published, publishedOK := program.Query(fixture.queryID)
	addressed, addressedOK := program.publishedQueryKeys()
	if !publishedOK || !addressedOK || len(addressed) != 1 || addressed[0] != published.identity.Key() {
		t.Fatalf("declared query addresses=%d published=%t", len(addressed), publishedOK)
	}
}

// TestConstructedOwnerDeclaredRefusesDuplicateQueryIdentity proves the query
// table publishes one row per identity: a second row under a published
// identity is refused at the declaration boundary.
func TestConstructedOwnerDeclaredRefusesDuplicateQueryIdentity(t *testing.T) {
	fixture := newConstructedOwnerFixture(t)
	declaration := fixture.declaration
	second := fixture.declaration.queries[0].Row
	second.Surfaces = []equation.Surface{{Factor: second.Surfaces[0].Factor, Form: equation.SurfaceReadExact, Local: second.Surfaces[0].Local + 1}}
	declaration.queries = append(append([]declaredQueryRow(nil), fixture.declaration.queries...), declaredQueryRow{ID: fixture.queryID, Row: second})
	constructed, refusal := constructTopology(declaration)
	if constructed.Available() || refusal.Stage() != ProgramConstructionStageAdmission || refusal.Step() != topologyConstructionStepDuplicateIdentity {
		t.Fatalf("duplicate query identity admitted stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
}

// TestConstructedOwnerDeclaredRefusesPointCollision proves the point table is
// injective over admitted Sites: two identities over one Site publish nothing.
func TestConstructedOwnerDeclaredRefusesPointCollision(t *testing.T) {
	fixture := newConstructedOwnerFixture(t)
	declaration := fixture.declaration
	declaration.points = append(append([]declaredPointRow(nil), fixture.declaration.points...), declaredPointRow{ID: artifactScalarLawID(0x24), Site: fixture.declaration.points[0].Site})
	constructed, refusal := constructTopology(declaration)
	if constructed.Available() || refusal.Stage() != ProgramConstructionStageAdmission || refusal.Step() != topologyConstructionStepPointRow {
		t.Fatalf("Site collision admitted stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
}

// constructedActivationFixture is one sealed owner-declared declaration whose
// member plane registers an activation trigger: three Points, the trigger row
// folded into the first, two ordinary rows folded into the others, and one
// direct activation candidate whose transport selects the third from the
// second.
type constructedActivationFixture struct {
	declaration  topologyDeclaration
	triggerID    identity.ContentID
	activationID identity.ContentID
	targetID     identity.ContentID
	bodyID       identity.ContentID
	candidate    equation.DirectActivationCandidate
	second       equation.DirectActivationCandidate
	origin       equation.MaterializationOrigin
	transport    equation.DirectActivationTransportSet
	schema       *Schema
	batch        *equation.Batch
}

// newConstructedActivationFixture seals the activation declaration. Its source
// plane is admitted through the production source path for the same reason as
// the other fixtures here; only the sealed Batch and the sealed equation rows
// are read back.
func newConstructedActivationFixture(t testing.TB) constructedActivationFixture {
	t.Helper()
	schemaBuilder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](schemaBuilder, coldKey(958_000))
	transport, transportOK := DeclareFactorSlot[uint64](schemaBuilder, coldKey(958_001))
	write, writeOK := factor.ExactWrite()
	ordinary, ordinaryOK := DeclareRuleSlot[uint64, ruleUnit](schemaBuilder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(958_002), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(958_003)}, Output: factor.Ref(),
	})
	ordinaryWrite, ordinaryWriteOK := SchemaWrite(ordinary, write)
	family, familyOK := DeclareSchemaActivationFamily(schemaBuilder, coldKey(958_004))
	trigger, triggerOK := DeclareSchemaActivationRule(schemaBuilder, SchemaStructuralRuleSpec{
		Semantic: coldKey(958_005), Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(958_006)}, Activation: family,
	})
	if !factorOK || !transportOK || !writeOK || !ordinaryOK || !ordinaryWriteOK || !familyOK || !triggerOK {
		t.Fatal("activation schema")
	}
	schema, schemaOK := schemaBuilder.Seal()
	if !schemaOK || schema == nil {
		t.Fatal("activation schema seal")
	}
	binding := NewSchemaBinding(schema)
	ordinarySpec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent: ruleUnitContent,
		Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(958_003)),
		Transfer: func(access Access[uint64, ruleUnit]) bool {
			return Product(access, func(row Row) bool { return StageValue(access, row, uint64(1)) })
		},
	}
	application, target, endpoint := coldKey(958_007), coldKey(958_008), coldKey(958_009)
	activationSpec := HotActivationSpec{
		Admission: AdmitActivationByTrustedTheorem(coldKey(958_006)),
		Run: func(value Activation) bool {
			return Activate(value, application, target, endpoint)
		},
	}
	if !BindFactor(binding, factor, hotExactObservationFactorSpec()) || !BindFactor(binding, transport, hotUintFactorSpec()) ||
		!BindRule[uint64, uint64, ruleUnit](binding, ordinary, ordinaryWrite, factor, ordinarySpec, testRuleProjector[ruleUnit]) ||
		!BindActivationRule(binding, trigger, activationSpec) || !binding.Seal() {
		t.Fatal("activation binding")
	}
	activationImplementation, activationImplementationOK := ActivationRuleImplementationAt(binding, trigger)
	ordinaryImplementation, ordinaryImplementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, ordinary)
	admission, admissionOK := binding.beginBindingTopologyBuilder()
	if !activationImplementationOK || !ordinaryImplementationOK || !admissionOK || admission == nil {
		t.Fatal("activation source admission")
	}
	scope := equation.EmptyScope()
	sites := make([]equation.Site, 3)
	occurrences := make([]equation.Occurrence, 3)
	operands := make([]equation.Operand, 3)
	for index := range sites {
		site, siteOK := admission.admitSite(compositionKeyOf(coldKey(958_010+index)), scope, equation.TrueExpr(), equation.InitPresent)
		occurrence, occurrenceOK := admission.admitAt(site)
		entity, entityOK := operandEntityForContent(ruleUnitForSemantic(coldKey(958_020 + index)).content)
		operand, operandOK := admission.admitOperand(occurrence, entity)
		if !siteOK || !occurrenceOK || !entityOK || !operandOK {
			t.Fatalf("activation source row %d", index)
		}
		sites[index], occurrences[index], operands[index] = site, occurrence, operand
	}
	if !admission.SealSources() {
		t.Fatal("activation source seal")
	}
	triggerProof := activationImplementation.binding.proof
	triggerSource, triggerSourceOK := admission.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
		Schema: triggerProof.semantic, OperandFamily: triggerProof.operandFamily, Occurrence: occurrences[0], Operand: operands[0],
	})
	triggerDraft, triggerDraftOK := activationImplementation.beginBindingRuleRow(triggerSource)
	triggerRow, triggerRowOK := admission.issueRuleRow(triggerDraft)
	if !triggerSourceOK || !triggerDraftOK || !triggerRowOK {
		t.Fatal("activation trigger row")
	}
	ordinaryProof := ordinaryImplementation.binding.proof
	ordinaryRows := make([]equation.RuleInstance, 2)
	for index := range ordinaryRows {
		source, sourceOK := admission.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
			Schema: ordinaryProof.semantic, OperandFamily: ordinaryProof.operandFamily, Occurrence: occurrences[index+1], Operand: operands[index+1],
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: ordinaryProof.output, Form: equation.SurfaceWriteExact, Local: 7, Mode: equation.TargetModeStrong}}},
		})
		draft, draftOK := ordinaryImplementation.beginBindingRuleRow(source)
		part, partOK := ordinaryImplementation.WritePart(source, 0)
		if !sourceOK || !draftOK || !partOK || !draft.AddWrite(part) {
			t.Fatalf("activation ordinary draft %d", index)
		}
		row, rowOK := admission.issueRuleRow(draft)
		if !rowOK {
			t.Fatalf("activation ordinary row %d", index)
		}
		ordinaryRows[index] = row.row
	}
	shape, shapeOK := schema.cold.RuleShapeAt(triggerProof.ordinal)
	transportSet, transportSetOK := equation.NewDirectActivationTransportSet(schema.cold, admission.inner.batch,
		[]equation.PointRef{equation.PointAt(1)}, []equation.PointRef{equation.PointAt(2)},
		[]composition.Key{compositionKeyOf(coldKey(958_001))}, compositionKeyOf(coldKey(958_000)))
	origin := equation.MaterializationOrigin{
		Family: shape.ActivationFamily, Application: compositionKeyOf(application),
		Target: compositionKeyOf(target), Endpoint: compositionKeyOf(endpoint), TriggerOrdinal: 0,
	}
	candidate, candidateOK := equation.NewDirectActivationCandidate(schema.cold, admission.inner.batch, origin, equation.PointAt(0), transportSet)
	second, secondOK := equation.NewDirectActivationCandidate(schema.cold, admission.inner.batch, origin, equation.PointAt(1), transportSet)
	if !shapeOK || !transportSetOK || !candidateOK || !secondOK {
		t.Fatal("activation candidate")
	}
	fixture := constructedActivationFixture{
		triggerID: artifactScalarLawID(0x31), activationID: artifactScalarLawID(0x32),
		targetID: artifactScalarLawID(0x33), bodyID: artifactScalarLawID(0x34),
		candidate: candidate, second: second, origin: origin, transport: transportSet, schema: schema, batch: admission.inner.batch,
	}
	fixture.declaration = topologyDeclaration{
		binding: binding, batch: admission.inner.batch,
		points: []declaredPointRow{
			{ID: artifactScalarLawID(0x21), Site: sites[0]},
			{ID: artifactScalarLawID(0x22), Site: sites[1]},
			{ID: artifactScalarLawID(0x23), Site: sites[2]},
		},
		members: []declaredMemberRow{
			{Plane: declaredMemberOwner, ID: fixture.triggerID, ActivationID: fixture.activationID, Activation: true, Row: triggerRow.row},
			{Plane: declaredMemberOwner, ID: fixture.targetID, Row: ordinaryRows[0]},
			{Plane: declaredMemberOwner, ID: fixture.bodyID, Row: ordinaryRows[1]},
		},
		directCandidates: []equation.DirectActivationCandidate{candidate},
	}
	return fixture
}

// TestConstructedActivationPlaneIsTotalOverRegisteredTriggers proves the
// candidate geometry a program publishes is total: the trigger publishes one
// activation address, its candidate set is exactly the declared candidates,
// and every candidate names that trigger under one application.
func TestConstructedActivationPlaneIsTotalOverRegisteredTriggers(t *testing.T) {
	fixture := newConstructedActivationFixture(t)
	constructed, refusal := constructTopology(fixture.declaration)
	if refusal.Available() || !constructed.Available() {
		t.Fatalf("activation construction refused stage=%v step=%v ordinal=%d", refusal.Stage(), refusal.Step(), refusal.Ordinal())
	}
	program := CommittedProgramFrom(constructed.topology, constructed.graph)
	if program == nil {
		t.Fatal("activation committed program")
	}
	if _, published := program.ActivationMember(fixture.activationID); !published {
		t.Fatal("registered trigger publishes no activation address")
	}
	if _, published := program.RuleMember(fixture.triggerID); !published {
		t.Fatal("registered trigger publishes no member address")
	}
	if _, aliased := program.ActivationMember(fixture.triggerID); aliased {
		t.Fatal("member identity also addresses an activation row")
	}
	activations, activationsOK := constructActivationPlaneOf(t, fixture.declaration)
	if !activationsOK {
		t.Fatal("activation plane")
	}
	ref := equation.RuleAt(0)
	if len(activations.directCandidates) != len(fixture.declaration.directCandidates) || len(activations.materializations) != 0 {
		t.Fatalf("candidate rows direct=%d materializations=%d declared=%d", len(activations.directCandidates), len(activations.materializations), len(fixture.declaration.directCandidates))
	}
	if activations.candidates[ref] != uint64(len(fixture.declaration.directCandidates)) {
		t.Fatalf("trigger denominator=%d declared=%d", activations.candidates[ref], len(fixture.declaration.directCandidates))
	}
	if activations.application[ref] != fixture.origin.Application {
		t.Fatal("trigger application diverged from the candidate origin")
	}
	if owner, owned := activations.directCandidateAt[fixture.candidate]; !owned || owner != ref {
		t.Fatal("candidate is owned by no registered trigger")
	}
	if owner, owned := activations.directCandidateKey[fixture.candidate.Key()]; !owned || owner != ref {
		t.Fatal("candidate key addresses no registered trigger")
	}
	// A second candidate of the same trigger raises that trigger's
	// denominator: the set is complete at two, not at one.
	widened := fixture.declaration
	widened.directCandidates = []equation.DirectActivationCandidate{fixture.candidate, fixture.second}
	widenedPlane, widenedOK := constructActivationPlaneOf(t, widened)
	if !widenedOK || widenedPlane.candidates[ref] != 2 || len(widenedPlane.directCandidates) != 2 {
		t.Fatalf("second candidate denominator=%d rows=%d", widenedPlane.candidates[ref], len(widenedPlane.directCandidates))
	}
	if _, refused := constructTopology(widened); refused.Available() {
		t.Fatalf("two candidates on one trigger refused stage=%v step=%v", refused.Stage(), refused.Step())
	}
}

// TestConstructedActivationPlaneRefusesIncompleteCandidateSets proves the
// candidate admission boundary: every refusal below publishes no geometry.
func TestConstructedActivationPlaneRefusesIncompleteCandidateSets(t *testing.T) {
	fixture := newConstructedActivationFixture(t)
	foreignOrigin := fixture.origin
	foreignOrigin.Application = compositionKeyOf(coldKey(958_030))
	foreignApplication, foreignApplicationOK := equation.NewDirectActivationCandidate(fixture.schema.cold, fixture.batch, foreignOrigin, equation.PointAt(1), fixture.transport)
	unregisteredOrigin := fixture.origin
	unregisteredOrigin.TriggerOrdinal = 1
	unregistered, unregisteredOK := equation.NewDirectActivationCandidate(fixture.schema.cold, fixture.batch, unregisteredOrigin, equation.PointAt(0), fixture.transport)
	if !foreignApplicationOK || !unregisteredOK {
		t.Fatal("activation refusal candidates")
	}
	for name, mutate := range map[string]func(topologyDeclaration) topologyDeclaration{
		"registered trigger with no candidate": func(declaration topologyDeclaration) topologyDeclaration {
			declaration.directCandidates = nil
			return declaration
		},
		"candidate on an unregistered trigger": func(declaration topologyDeclaration) topologyDeclaration {
			declaration.directCandidates = []equation.DirectActivationCandidate{fixture.candidate, unregistered}
			return declaration
		},
		"two applications on one trigger": func(declaration topologyDeclaration) topologyDeclaration {
			declaration.directCandidates = []equation.DirectActivationCandidate{fixture.candidate, foreignApplication}
			return declaration
		},
		"one candidate declared twice": func(declaration topologyDeclaration) topologyDeclaration {
			declaration.directCandidates = []equation.DirectActivationCandidate{fixture.candidate, fixture.candidate}
			return declaration
		},
	} {
		declaration := mutate(fixture.declaration)
		constructed, refusal := constructTopology(declaration)
		if constructed.Available() || refusal.Stage() != ProgramConstructionStageAdmission || refusal.Step() != topologyConstructionStepCandidateRow {
			t.Fatalf("%s admitted stage=%v step=%v published=%t", name, refusal.Stage(), refusal.Step(), constructed.Available())
		}
	}
}

// TestConstructedActivationPlaneRefusesUnshapedTrigger proves a member is
// registered as an activation only where its rule shape declares exactly one
// activation family.
func TestConstructedActivationPlaneRefusesUnshapedTrigger(t *testing.T) {
	fixture := newConstructedActivationFixture(t)
	declaration := fixture.declaration
	declaration.members = append([]declaredMemberRow(nil), fixture.declaration.members...)
	declaration.members[1].Activation = true
	declaration.members[1].ActivationID = artifactScalarLawID(0x35)
	constructed, refusal := constructTopology(declaration)
	if constructed.Available() || refusal.Stage() != ProgramConstructionStageAdmission || refusal.Step() != topologyConstructionStepActivationRow {
		t.Fatalf("unshaped trigger admitted stage=%v step=%v published=%t", refusal.Stage(), refusal.Step(), constructed.Available())
	}
}

// constructActivationPlaneOf folds one declaration to its candidate geometry.
func constructActivationPlaneOf(t *testing.T, declaration topologyDeclaration) (constructedActivationPlane, bool) {
	t.Helper()
	source, refusal := constructSourcePlane(declaration)
	if refusal.Available() {
		return constructedActivationPlane{}, false
	}
	mounts, refusal := constructMountPlane(declaration, source)
	if refusal.Available() {
		return constructedActivationPlane{}, false
	}
	points, refusal := constructPointPlane(declaration, source, mounts)
	if refusal.Available() {
		return constructedActivationPlane{}, false
	}
	members, refusal := constructMemberPlane(declaration, source, mounts, points)
	if refusal.Available() {
		return constructedActivationPlane{}, false
	}
	activations, refusal := constructActivationPlane(declaration, source, members)
	return activations, !refusal.Available()
}

// TestConstructedQueryTableIsAddressedInDeclaredOrder proves the query table
// of a program with several rows is total, injective and ordered: each
// declared identity addresses its own row, the published ordinal is the
// declared position, and the published key order is the declared order.
func TestConstructedQueryTableIsAddressedInDeclaredOrder(t *testing.T) {
	fixture := newConstructedOwnerFixture(t)
	declaration := fixture.declaration
	base := fixture.declaration.queries[0].Row
	ids := []identity.ContentID{fixture.queryID, artifactScalarLawID(0x25), artifactScalarLawID(0x26)}
	declaration.queries = make([]declaredQueryRow, len(ids))
	for index, id := range ids {
		row := base
		row.Surfaces = []equation.Surface{{Factor: base.Surfaces[0].Factor, Form: equation.SurfaceReadExact, Local: base.Surfaces[0].Local + uint64(index)}}
		declaration.queries[index] = declaredQueryRow{ID: id, Row: row}
	}
	constructed, refusal := constructTopology(declaration)
	if refusal.Available() || !constructed.Available() {
		t.Fatalf("multi-query construction refused stage=%v step=%v", refusal.Stage(), refusal.Step())
	}
	program := CommittedProgramFrom(constructed.topology, constructed.graph)
	if program == nil {
		t.Fatal("multi-query committed program")
	}
	if got := constructed.topology.topology.QueryRowCount(); got != len(ids) {
		t.Fatalf("query rows=%d declared=%d", got, len(ids))
	}
	addressed, addressedOK := program.publishedQueryKeys()
	if !addressedOK || len(addressed) != len(ids) {
		t.Fatalf("published query addresses=%d declared=%d", len(addressed), len(ids))
	}
	for index, id := range ids {
		published, publishedOK := program.Query(id)
		if !publishedOK {
			t.Fatalf("declared query %v publishes no address", id)
		}
		if published.identity.Key() != addressed[index] {
			t.Fatalf("declared query %d publishes out of declared order", index)
		}
	}
}
