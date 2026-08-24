// read_lane_equivalence_law_test.go fences the typed and opaque exact Rule
// read lanes against each other. Both are driven from one sealed declaration
// with one shared compiled read row, so any divergence in admission or refusal
// is a property of the lane code alone.

package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

func readLaneID(value int) identity.ContentID {
	var id identity.ContentID
	id[0], id[1], id[2] = byte(value), byte(value>>8), 0x71
	return id
}

// readLaneSink is the read collector half of a bound member. It records what a
// lane installed without adding any admission of its own.
type readLaneSink struct {
	reads []readRuntime
}

func (sink *readLaneSink) appendReadRuntime(read readRuntime) bool {
	if sink == nil || read == nil {
		return false
	}
	sink.reads = append(sink.reads, read)
	return true
}

// readLaneFixture is one sealed program whose single mounted Rule owns one
// exact read. The opaque binding is the installed one; the typed binding is
// its exact twin over the same row, Factor cell and Read handle.
type readLaneFixture struct {
	binding *SchemaBinding
	program *CommittedProgram
	plane   *programPlane
	member  equation.RuleMember
	readKey composition.Key
	cell    *schemaFactorBindingCell[uint64, uint64]
	row     *schemaRuleReadRow
	opaque  *schemaOpaqueExactRuleReadBinding[uint64]
	typed   *schemaExactRuleReadBinding[uint64, uint64]
	factors map[composition.Key]runtimeFactor
}

func newReadLaneFixture(t testing.TB) readLaneFixture {
	t.Helper()
	builder := NewSchema()
	readFactor, readFactorOK := DeclareFactorSlot[uint64](builder, coldKey(961_000))
	outputFactor, outputFactorOK := DeclareFactorSlot[uint64](builder, coldKey(961_001))
	readForm, readFormOK := readFactor.ExactRead()
	writeForm, writeFormOK := outputFactor.ExactWrite()
	outputReadForm, outputReadFormOK := outputFactor.ExactRead()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(961_002), OperandFamily: unitOperandFamily, Inputs: 1,
		Output: outputFactor.Ref(),
	})
	input, inputOK := rule.Input(0)
	readSlot, readSlotOK := SchemaRead(rule, readForm, input)
	writeSlot, writeSlotOK := SchemaWrite(rule, writeForm)
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(961_003), Freezer: coldKey(961_004), Population: queryschema.PopulationKindSelectedPoint})
	if queryOK {
		queryOK = SchemaQueryRead(query, outputReadForm)
	}
	schema, schemaOK := builder.Seal()
	if !readFactorOK || !outputFactorOK || !readFormOK || !writeFormOK || !outputReadFormOK || !ruleOK || !inputOK || !readSlotOK || !writeSlotOK || !queryOK || !schemaOK || schema == nil {
		t.Fatal("read lane schema")
	}
	binding := NewSchemaBinding(schema)
	spec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent:  ruleUnitContent,
		OperandResolver: func(OperandCoords) (ruleUnit, bool) { return ruleUnitForSemantic(coldKey(961_009)), true },
		Fold: func(frame Frame[uint64, ruleUnit]) RuleResult[uint64] {
			return Staged(frame, uint64(1))
		},
	}
	querySpec := hotExactQuerySpec()
	querySpec.Result.Semantic = coldKey(961_004)
	if !BindFactor(binding, readFactor, hotUintFactorSpec()) || !BindFactor(binding, outputFactor, hotUintFactorSpec()) ||
		!BindExactQuery(binding, query, outputFactor, querySpec) {
		t.Fatal("read lane factor binding")
	}
	if _, bound := BindRuleWithOpaqueExactRead[uint64, uint64, ruleUnit, uint64](binding, rule, readSlot, readFactor.Ref(), writeSlot, outputFactor.Ref(), spec,
		func(ruleUnit) (uint64, bool) { return 1, true },
		func(ruleUnit) (uint64, bool) { return 1, true }); !bound {
		t.Fatalf("read lane rule binding poisoned=%t", binding.Poisoned())
	}
	capability, capabilityOK := IssueMountedRuleCapability(binding, rule)
	if !capabilityOK || !RegisterRuleSlot(binding, rule, capability) || !binding.Seal() {
		t.Fatal("read lane capability")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !implementationOK || implementation == nil || !queryImplementationOK || queryImplementation == nil {
		t.Fatal("read lane rule implementation")
	}
	program := constructReadLaneProgram(t, binding, schema, capability, implementation, queryImplementation)
	plane, _, planeOK := bindProgramPlane(program.state, program.graph)
	if !planeOK || plane == nil {
		t.Fatal("read lane factor plane")
	}
	fixture := readLaneFixture{
		binding: binding, program: program, plane: plane,
		readKey: schema.factorSemanticAt(0), factors: plane.byKey,
	}
	fixture.member = readLaneMember(t, program, schema.ruleSemanticAt(0))
	cell, cellOK := binding.state.rules[0].(*schemaRuleBindingCellImpl[uint64, uint64, ruleUnit])
	if !cellOK || cell == nil || cell.impl == nil || len(cell.impl.reads) != 1 {
		t.Fatal("read lane sealed rule cell")
	}
	opaque, opaqueOK := cell.impl.reads[0].(*schemaOpaqueExactRuleReadBinding[uint64])
	if !opaqueOK || opaque == nil || opaque.row == nil {
		t.Fatal("read lane opaque binding")
	}
	factorCell, factorCellOK := opaque.factor.(*schemaFactorBindingCell[uint64, uint64])
	if !factorCellOK || factorCell == nil {
		t.Fatal("read lane factor cell")
	}
	fixture.cell, fixture.row, fixture.opaque = factorCell, opaque.row, opaque
	// The typed twin is field-for-field what the typed installer builds for
	// this same read: same compiled row, same Factor cell, same Read handle,
	// same local projector.
	fixture.typed = &schemaExactRuleReadBinding[uint64, uint64]{row: opaque.row, factor: factorCell, read: opaque.read, projector: opaque.projector}
	return fixture
}

func readLaneMember(t testing.TB, program *CommittedProgram, ruleKey composition.Key) equation.RuleMember {
	t.Helper()
	for group := 0; group < program.graph.GroupCount(); group++ {
		node, nodeOK := program.graph.HyperedgeAt(group)
		if !nodeOK {
			continue
		}
		for index := 0; index < node.MemberCount(); index++ {
			member, memberOK := node.MemberAt(index)
			if memberOK && member.Rule() == ruleKey && member.ReadCount() == 1 {
				return member
			}
		}
	}
	t.Fatal("read lane graph member")
	return equation.RuleMember{}
}

func constructReadLaneProgram(t testing.TB, binding *SchemaBinding, schema *Schema, capability RuleSlotCapability, implementation *RuleImplementation[uint64, uint64, ruleUnit], queryImplementation *ExactQueryImplementation[uint64, uint64]) *CommittedProgram {
	t.Helper()
	spec, specOK := rows.NewArtifactScalarSpec(readLaneID(2), readLaneID(3), identity.ContentID(schema.ID().Digest()), rows.ArtifactScalarCapacity{
		Roles: 1, Points: 2, Regions: 1, Events: 4, Rules: 1, Bodies: 1,
	})
	role, roleOK := spec.DeclareRole(readLaneID(4))
	if !specOK || !roleOK {
		t.Fatal("read lane artifact header")
	}
	entry, member := readLaneID(10), readLaneID(11)
	_, entryOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: entry, Initial: true})
	_, memberOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: member})
	region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: readLaneID(40), Head: entry})
	regionOK = regionOK && spec.AddRegionMember(region, entry) && spec.AddRegionMember(region, member)
	if !entryOK || !memberOK || !regionOK {
		t.Fatal("read lane artifact region")
	}
	events := spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: readLaneID(40)}) &&
		spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: entry}) &&
		spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: member}) &&
		spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: readLaneID(40)})
	body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: readLaneID(41)})
	if !events || !bodyOK || !spec.AddBodyEntry(body, entry) || !spec.AddBodyExit(body, member) {
		t.Fatal("read lane artifact body")
	}
	if !spec.AddRule(rows.ArtifactScalarRule{Role: role, Stage: programissuance.StageCallDispatch, Point: member, Inputs: [6]identity.ContentID{entry}, InputCount: 1, ID: readLaneID(60), Native: true}) {
		t.Fatal("read lane artifact rule")
	}
	installArtifactStageTable(t, spec)
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	bootstrap, bootstrapOK := NewProgramBootstrap(readLaneID(70), readLaneID(71))
	contexts := explicitTestContextDirectory(t, readLaneID(70), []identity.ContentID{readLaneID(1)}, readLaneID(72), readLaneID(73))
	cell, cellOK := implementation.sealedRuleCell()
	if !templateOK || !bootstrapOK || !cellOK || cell == nil {
		t.Fatal("read lane artifact seal")
	}
	queryAdmission, queryAdmissionOK := NewExactQueryAdmission(queryImplementation, readLaneID(110), readLaneID(1), member, explicitTestContext(t, contexts, readLaneID(1)))
	if !queryAdmissionOK {
		t.Fatal("read lane query admission")
	}
	mount := MountedProgramArtifact{Template: template, Roles: []MountedProgramRole{{Scalar: role, Capability: capability}}, Module: readLaneID(1)}
	admission := MountedProgramAdmission{Mounted: []MountedRuleAdmission{{
		Capability: capability, Mount: readLaneID(1),
		Point: member, Occurrence: readLaneID(60),
	}}, Queries: []ProgramQueryAdmission{queryAdmission}}
	program, refusal, constructed := ConstructProgram(ProgramDeclaration{Binding: binding, Mounts: []MountedProgramArtifact{mount}, Bootstrap: bootstrap, Contexts: contexts, Admission: admission})
	if !constructed || program == nil {
		t.Fatalf("read lane ConstructProgram stage=%v seal=%v commit=%v", refusal.Stage(), refusal.Seal(), refusal.Commit())
	}
	return program
}

// driveReadLanes runs both lanes over identical inputs and reports the two
// verdicts plus whatever each installed.
func driveReadLanes(fixture readLaneFixture, member equation.RuleMember, factors map[composition.Key]runtimeFactor) (bool, bool, readRuntime, readRuntime) {
	typedSink, opaqueSink := &readLaneSink{}, &readLaneSink{}
	typedOK := fixture.typed.bind(typedSink, member, factors)
	opaqueOK := fixture.opaque.bind(opaqueSink, member, factors)
	var typedRead, opaqueRead readRuntime
	if len(typedSink.reads) == 1 {
		typedRead = typedSink.reads[0]
	}
	if len(opaqueSink.reads) == 1 {
		opaqueRead = opaqueSink.reads[0]
	}
	return typedOK, opaqueOK, typedRead, opaqueRead
}

func assertReadLanesAgree(t *testing.T, name string, fixture readLaneFixture, member equation.RuleMember, factors map[composition.Key]runtimeFactor, want bool) (readRuntime, readRuntime) {
	t.Helper()
	typedOK, opaqueOK, typedRead, opaqueRead := driveReadLanes(fixture, member, factors)
	if typedOK != want || opaqueOK != want {
		t.Fatalf("%s: typed=%t opaque=%t, want both %t", name, typedOK, opaqueOK, want)
	}
	if want && (typedRead == nil || opaqueRead == nil) {
		t.Fatalf("%s: admitted lanes installed typed=%t opaque=%t", name, typedRead != nil, opaqueRead != nil)
	}
	if !want && (typedRead != nil || opaqueRead != nil) {
		t.Fatalf("%s: refused lanes still installed typed=%t opaque=%t", name, typedRead != nil, opaqueRead != nil)
	}
	return typedRead, opaqueRead
}

// TestExactReadLanesAdmitTheSameBinding proves the two lanes install the same
// runtime read from the same declaration: same input port, same Factor
// binding, same carrier unit and same exact address.
func TestExactReadLanesAdmitTheSameBinding(t *testing.T) {
	fixture := newReadLaneFixture(t)
	typedRead, opaqueRead := assertReadLanesAgree(t, "canonical", fixture, fixture.member, fixture.factors, true)
	typedRow, typedTyped := typedRead.(*typedReadRuntime[uint64, uint64, OrderedCells[uint64]])
	opaqueRow, opaqueTyped := opaqueRead.(*typedReadRuntime[uint64, uint64, OrderedCells[uint64]])
	if !typedTyped || !opaqueTyped {
		t.Fatalf("installed read runtimes typed=%T opaque=%T", typedRead, opaqueRead)
	}
	if typedRow.input != opaqueRow.input || typedRow.binding != opaqueRow.binding || typedRow.unit != opaqueRow.unit ||
		typedRow.exactFactor != opaqueRow.exactFactor || typedRow.exactRaw != opaqueRow.exactRaw ||
		typedRow.exact != opaqueRow.exact || typedRow.summary != opaqueRow.summary {
		t.Fatalf("installed reads diverged typed=%+v opaque=%+v", typedRow, opaqueRow)
	}
	if typedRow.exactFactor != schemaFactorBinding(fixture.cell) || typedRow.exactRaw != 1 || !typedRow.exact {
		t.Fatalf("installed read address = factor:%v raw:%d exact:%t", typedRow.exactFactor, typedRow.exactRaw, typedRow.exact)
	}
	// The normalizer, equality and fingerprint closures are the Factor
	// algebra's in both lanes; compare their behaviour, not their identity.
	left, right := OrderedCells[uint64]{}, OrderedCells[uint64]{}
	if typedRow.equal(left, right) != opaqueRow.equal(left, right) || typedRow.fingerprint(left) != opaqueRow.fingerprint(left) {
		t.Fatal("installed read algebra closures diverged")
	}
	typedAddress, typedRaw, typedAddressOK := typedRead.exactAddress()
	opaqueAddress, opaqueRaw, opaqueAddressOK := opaqueRead.exactAddress()
	if typedAddress != opaqueAddress || typedRaw != opaqueRaw || typedAddressOK != opaqueAddressOK || !typedAddressOK {
		t.Fatal("installed read exact address diverged")
	}
}

// TestExactReadLanesRefuseTheSameInputs drives every refusal the exact read
// lanes own: an absent sink, an absent Factor catalog, a missing or foreign
// Factor row, a wrong key or value instantiation, a read the Factor never
// bound, and a member that declares no read at this position. Each drive is
// pinned to its own guard: the state it perturbs is asserted first, and the
// admission control below proves nothing else in the input was disturbed.
func TestExactReadLanesRefuseTheSameInputs(t *testing.T) {
	fixture := newReadLaneFixture(t)
	foreign := newReadLaneFixture(t)
	sealedImplementation, implementationOK := fixture.cell.sealedImplementation(fixture.binding.state, fixture.binding.state.authority)
	outputCell, outputCellOK := fixture.binding.state.factors[1].(*schemaFactorBindingCell[uint64, uint64])
	canonical, canonicalOK := fixture.factors[fixture.readKey].(*boundFactor[uint64, uint64])
	if !implementationOK || !outputCellOK || !canonicalOK || canonical == nil {
		t.Fatal("read lane sealed factor implementation")
	}
	outputImplementation, outputImplementationOK := outputCell.sealedImplementation(fixture.binding.state, fixture.binding.state.authority)
	surface, surfaceOK := fixture.member.ReadAt(0)
	if !outputImplementationOK || !surfaceOK {
		t.Fatal("read lane output factor implementation")
	}

	// The sink guard: neither lane may install into an absent member.
	if fixture.typed.bind(nil, fixture.member, fixture.factors) || fixture.opaque.bind(nil, fixture.member, fixture.factors) {
		t.Fatal("an absent read sink accepted an installed read")
	}

	assertReadLanesAgree(t, "nil factor catalog", fixture, fixture.member, nil, false)
	assertReadLanesAgree(t, "empty factor catalog", fixture, fixture.member, map[composition.Key]runtimeFactor{}, false)

	// A Factor row bound for another equally shaped sealed program: present,
	// correctly instantiated, and refused only on Factor row identity.
	foreignFactor, foreignTyped := foreign.factors[foreign.readKey].(*boundFactor[uint64, uint64])
	if !foreignTyped || foreignFactor == nil || foreign.readKey != fixture.readKey ||
		!factorRowAvailable(foreignFactor.implementation.row) || foreignFactor.implementation.row == schemaFactorBinding(fixture.cell) {
		t.Fatal("foreign program factor drive")
	}
	assertReadLanesAgree(t, "foreign program factor", fixture, fixture.member,
		map[composition.Key]runtimeFactor{fixture.readKey: foreignFactor}, false)

	// The read key carrying this program's other Factor: the ordinal and the
	// algebra of the sealed read cell are both contradicted.
	if !factorRowAvailable(outputImplementation.row) || outputImplementation.ordinal == fixture.row.factorOrdinal ||
		outputImplementation.algebra == fixture.cell.impl.algebra {
		t.Fatal("wrong factor row drive")
	}
	assertReadLanesAgree(t, "wrong factor row", fixture, fixture.member,
		map[composition.Key]runtimeFactor{fixture.readKey: &boundFactor[uint64, uint64]{implementation: outputImplementation}}, false)

	// A Factor bound at another key instantiation than its sealed cell.
	keyMismatch := &boundFactor[uint32, uint64]{}
	if _, matches := runtimeFactor(keyMismatch).(*boundFactor[uint64, uint64]); matches {
		t.Fatal("wrong key instantiation drive")
	}
	assertReadLanesAgree(t, "wrong key instantiation", fixture, fixture.member,
		map[composition.Key]runtimeFactor{fixture.readKey: keyMismatch}, false)

	// A Factor bound over another value type.
	valueMismatch := &boundFactor[uint64, uint32]{}
	if _, matches := runtimeFactor(valueMismatch).(*boundFactor[uint64, uint64]); matches {
		t.Fatal("wrong value instantiation drive")
	}
	assertReadLanesAgree(t, "wrong value instantiation", fixture, fixture.member,
		map[composition.Key]runtimeFactor{fixture.readKey: valueMismatch}, false)

	// The read-unit guard, isolated. The control carries the canonical sealed
	// implementation, the canonical Factor binding and the canonical unit
	// table, so both lanes admit it; the drive differs in the unit table
	// alone, so the refusal can only be the Factor's own read-unit check.
	control := &boundFactor[uint64, uint64]{implementation: sealedImplementation, binding: canonical.binding, reads: canonical.reads}
	starved := &boundFactor[uint64, uint64]{implementation: sealedImplementation, binding: canonical.binding}
	if _, present := control.reads[surface]; !present {
		t.Fatal("read unit control")
	}
	if _, present := starved.reads[surface]; present {
		t.Fatal("read unit drive")
	}
	assertReadLanesAgree(t, "rebound canonical factor", fixture, fixture.member,
		map[composition.Key]runtimeFactor{fixture.readKey: control}, true)
	assertReadLanesAgree(t, "unavailable read unit", fixture, fixture.member,
		map[composition.Key]runtimeFactor{fixture.readKey: starved}, false)

	// A member that declares no read at this position.
	empty := equation.RuleMember{}
	if empty.ReadCount() != 0 {
		t.Fatal("member without the read drive")
	}
	assertReadLanesAgree(t, "member without the read", fixture, empty, fixture.factors, false)

	if typedOK, opaqueOK, _, _ := driveReadLanes(fixture, fixture.member, fixture.factors); !typedOK || !opaqueOK {
		t.Fatal("refusal drives disturbed the canonical admission")
	}
}

// TestExactReadLanesRefuseAnUnsealedDeclaration proves both lanes fence on the
// sealed row, not on the caller: an open binding's read row admits nothing
// even when the Factor catalog and member are the sealed program's.
func TestExactReadLanesRefuseAnUnsealedDeclaration(t *testing.T) {
	sealed := newReadLaneFixture(t)
	open := newOpenReadLaneBindings(t)
	typedSink, opaqueSink := &readLaneSink{}, &readLaneSink{}
	typedOK := open.typed.bind(typedSink, sealed.member, sealed.factors)
	opaqueOK := open.opaque.bind(opaqueSink, sealed.member, sealed.factors)
	if typedOK || opaqueOK || len(typedSink.reads) != 0 || len(opaqueSink.reads) != 0 {
		t.Fatalf("unsealed declaration bound typed=%t opaque=%t", typedOK, opaqueOK)
	}
}

type openReadLaneBindings struct {
	typed  *schemaExactRuleReadBinding[uint64, uint64]
	opaque *schemaOpaqueExactRuleReadBinding[uint64]
}

func newOpenReadLaneBindings(t testing.TB) openReadLaneBindings {
	t.Helper()
	builder := NewSchema()
	readFactor, readFactorOK := DeclareFactorSlot[uint64](builder, coldKey(961_000))
	outputFactor, outputFactorOK := DeclareFactorSlot[uint64](builder, coldKey(961_001))
	readForm, readFormOK := readFactor.ExactRead()
	writeForm, writeFormOK := outputFactor.ExactWrite()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(961_002), OperandFamily: unitOperandFamily, Inputs: 1,
		Output: outputFactor.Ref(),
	})
	input, inputOK := rule.Input(0)
	readSlot, readSlotOK := SchemaRead(rule, readForm, input)
	writeSlot, writeSlotOK := SchemaWrite(rule, writeForm)
	schema, schemaOK := builder.Seal()
	if !readFactorOK || !outputFactorOK || !readFormOK || !writeFormOK || !ruleOK || !inputOK || !readSlotOK || !writeSlotOK || !schemaOK {
		t.Fatal("open read lane schema")
	}
	binding := NewSchemaBinding(schema)
	spec := HotRuleSpec[uint64, ruleUnit]{
		OperandContent:  ruleUnitContent,
		OperandResolver: func(OperandCoords) (ruleUnit, bool) { return ruleUnitForSemantic(coldKey(961_009)), true },
		Fold: func(frame Frame[uint64, ruleUnit]) RuleResult[uint64] {
			return Staged(frame, uint64(1))
		},
	}
	if !BindFactor(binding, readFactor, hotUintFactorSpec()) || !BindFactor(binding, outputFactor, hotUintFactorSpec()) {
		t.Fatal("open read lane factor binding")
	}
	if _, bound := BindRuleWithOpaqueExactRead[uint64, uint64, ruleUnit, uint64](binding, rule, readSlot, readFactor.Ref(), writeSlot, outputFactor.Ref(), spec,
		func(ruleUnit) (uint64, bool) { return 1, true },
		func(ruleUnit) (uint64, bool) { return 1, true }); !bound {
		t.Fatal("open read lane rule binding")
	}
	cell, cellOK := binding.state.rules[0].(*schemaRuleBindingCellImpl[uint64, uint64, ruleUnit])
	if !cellOK || cell == nil || cell.impl == nil || len(cell.impl.reads) != 1 {
		t.Fatal("open read lane rule cell")
	}
	opaque, opaqueOK := cell.impl.reads[0].(*schemaOpaqueExactRuleReadBinding[uint64])
	factorCell, factorCellOK := binding.state.factors[0].(*schemaFactorBindingCell[uint64, uint64])
	if !opaqueOK || opaque == nil || !factorCellOK {
		t.Fatal("open read lane bindings")
	}
	return openReadLaneBindings{
		typed:  &schemaExactRuleReadBinding[uint64, uint64]{row: opaque.row, factor: factorCell, read: opaque.read, projector: opaque.projector},
		opaque: opaque,
	}
}

// TestExactReadLanesShareOneAdmissionSurface proves the two lanes publish the
// same admission-time capability: the same exact Factor authority and the same
// operand-local projection.
func TestExactReadLanesShareOneAdmissionSurface(t *testing.T) {
	fixture := newReadLaneFixture(t)
	typedFactor, opaqueFactor := fixture.typed.exactAdmitFactor(), fixture.opaque.exactAdmitFactor()
	if typedFactor != opaqueFactor || typedFactor != schemaFactorBinding(fixture.cell) {
		t.Fatalf("admission factor typed=%v opaque=%v", typedFactor, opaqueFactor)
	}
	operand := ruleUnitForSemantic(coldKey(961_009))
	typedLocal, typedOK := fixture.typed.projectLocal(operand)
	opaqueLocal, opaqueOK := fixture.opaque.projectLocal(operand)
	if typedLocal != opaqueLocal || typedOK != opaqueOK || !typedOK {
		t.Fatalf("operand projection typed=%d/%t opaque=%d/%t", typedLocal, typedOK, opaqueLocal, opaqueOK)
	}
	if typedLocal, typedOK = fixture.typed.projectLocal(struct{}{}); typedOK {
		t.Fatalf("typed lane projected a foreign operand = %d", typedLocal)
	}
	if opaqueLocal, opaqueOK = fixture.opaque.projectLocal(struct{}{}); opaqueOK {
		t.Fatalf("opaque lane projected a foreign operand = %d", opaqueLocal)
	}
	// Both lanes report the same completion verdict against the sealed cell.
	cell, cellOK := fixture.binding.state.rules[0].(*schemaRuleBindingCellImpl[uint64, uint64, ruleUnit])
	if !cellOK {
		t.Fatal("read lane sealed cell")
	}
	typedComplete := fixture.typed.complete(fixture.binding.state, cell, 0)
	opaqueComplete := fixture.opaque.complete(fixture.binding.state, cell, 0)
	if !typedComplete || !opaqueComplete {
		t.Fatalf("sealed completion typed=%t opaque=%t", typedComplete, opaqueComplete)
	}
	foreign := newReadLaneFixture(t)
	foreignCell, foreignCellOK := foreign.binding.state.rules[0].(*schemaRuleBindingCellImpl[uint64, uint64, ruleUnit])
	if !foreignCellOK {
		t.Fatal("foreign read lane sealed cell")
	}
	if fixture.typed.complete(foreign.binding.state, foreignCell, 0) || fixture.opaque.complete(foreign.binding.state, foreignCell, 0) {
		t.Fatal("a foreign owner completed an exact read binding")
	}
}
