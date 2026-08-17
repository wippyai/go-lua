package engine

import (
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// The I3 construction laws are measured against these constants. R is the
// number of distinct root bindings the sealed topology admits, counted from
// the topology alone; a directory that indexed paths, pages, generations or
// products of the root bindings would exceed the linear ceilings immediately.
const (
	semanticDirectoryConstructionBase  = 8
	semanticDirectoryOperationsPerRoot = 2
	semanticDirectoryCapacityPerRoot   = 3
	semanticDirectoryPublicationBytes  = 4096
)

const (
	semanticDirectoryPointRole = iota + 1
	semanticDirectoryMemberRole
	semanticDirectoryQueryRole
)

func semanticDirectoryScaleID(role byte, index int) identity.ContentID {
	var id identity.ContentID
	id[0] = role
	id[1] = byte(index + 1)
	id[2] = byte((index + 1) >> 8)
	id[3] = byte((index + 1) >> 16)
	return id
}

type semanticDirectoryScaleFixture struct {
	topology *BindingTopology
	graph    *ReceiptGraph
	roots    int
}

// newSemanticDirectoryScaleFixture commits one topology carrying count Points,
// count Rule members and count Queries, so the admitted root-binding count
// scales with count and nothing else.
func newSemanticDirectoryScaleFixture(t testing.TB, count int) semanticDirectoryScaleFixture {
	t.Helper()
	schema, factor, rule, write, query := receiptExactQuerySchemaFixture(t)
	binding := NewSchemaBinding(schema)
	if !BindFactor(binding, factor, hotUintFactorSpec()) || !BindRule[uint64, uint64, ruleUnit](binding, rule, write, factor, receiptExactQueryRuleSpec()) || !BindExactQuery(binding, query, factor, hotExactQuerySpec()) || !binding.Seal() {
		t.Fatal("directory scale binding")
	}
	ruleImplementation, ruleOK := RuleImplementationAt[uint64, uint64, ruleUnit](binding, rule)
	queryImplementation, queryOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	assembly, assemblyOK := beginReceiptAssembly(binding)
	if !ruleOK || !queryOK || !assemblyOK {
		t.Fatal("directory scale implementations")
	}
	sites := make([]equation.Site, count)
	occurrences := make([]equation.Occurrence, count)
	operands := make([]equation.Operand, count)
	for index := 0; index < count; index++ {
		site, siteOK := assembly.builder.admitSite(compositionKeyOf(coldKey(960_000+index)), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
		occurrence, occurrenceOK := assembly.builder.admitAt(site)
		entity, entityOK := operandEntityForContent(ruleUnitForSemantic(coldKey(970_000 + index)).content)
		operand, operandOK := assembly.builder.admitOperand(occurrence, entity)
		if !siteOK || !occurrenceOK || !entityOK || !operandOK {
			t.Fatal("directory scale source rows")
		}
		sites[index], occurrences[index], operands[index] = site, occurrence, operand
	}
	if !assembly.SealSources() {
		t.Fatal("directory scale source seal")
	}
	proof := ruleImplementation.receipt.proof
	for index := 0; index < count; index++ {
		pointReceipt, pointReceiptOK := assembly.builder.issuePointRow(equation.PointSpec{Site: sites[index]})
		point, pointOK := assembly.builder.addSemanticPoint(semanticDirectoryScaleID(semanticDirectoryPointRole, index), pointReceipt)
		source, sourceOK := assembly.builder.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{
			Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrences[index], Operand: operands[index],
			Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}},
		})
		draft, draftOK := ruleImplementation.BeginBindingRuleRow(source)
		writePart, writePartOK := ruleImplementation.WritePart(source, 0)
		if !pointReceiptOK || !pointOK || !sourceOK || !draftOK || !writePartOK || !draft.AddWrite(writePart) {
			t.Fatal("directory scale Rule source")
		}
		ruleReceipt, ruleReceiptOK := assembly.builder.issueRuleRow(draft)
		_, memberOK := assembly.builder.addSemanticRule(semanticDirectoryScaleID(semanticDirectoryMemberRole, index), ruleReceipt)
		queryReceipt, queryReceiptOK := assembly.builder.issueQueryRow(queryImplementation, equation.QueryInstance{
			Family: schema.querySemanticAt(0), Point: point.ref,
			Surfaces: []equation.Surface{{Factor: proof.output, Form: equation.SurfaceReadExact, Local: 1}},
		})
		if !ruleReceiptOK || !memberOK || !queryReceiptOK {
			t.Fatal("directory scale topology rows")
		}
		if _, ok := assembly.builder.addSemanticQuery(semanticDirectoryScaleID(semanticDirectoryQueryRole, index), queryReceipt); !ok {
			t.Fatal("directory scale semantic Query")
		}
	}
	topology, graph, committed := assembly.Commit()
	if !committed || topology == nil || graph == nil {
		t.Fatalf("directory scale commit: %+v", assembly.commitFailure)
	}
	return semanticDirectoryScaleFixture{topology: topology, graph: graph, roots: admittedRootBindings(topology.topology)}
}

// admittedRootBindings counts the distinct root bindings the sealed topology
// admits. It reads only the sealed topology, never the directory that indexes
// it, so it is an independent denominator for the entry law.
func admittedRootBindings(topology *equation.Topology) int {
	roots := topology.PointRowCount() + topology.RuleMemberRowCount() + topology.QueryRowCount()
	for index := 0; index < topology.RuleMemberRowCount(); index++ {
		if _, ok := topology.ActivationMemberRow(equation.RuleAt(index)); ok {
			roots++
		}
	}
	return roots
}

// semanticDirectoryRows rebuilds the exact admitted row set of a scale fixture
// so a construction law can hand the constructor a hostile variant of it.
func semanticDirectoryRows(count int) *bindingSemanticRows {
	rows := &bindingSemanticRows{
		points:      make(map[identity.ContentID]equation.PointRef, count),
		members:     make(map[identity.ContentID]equation.RuleRef, count),
		queries:     make(map[identity.ContentID]uint64, count),
		activations: map[identity.ContentID]equation.RuleRef{},
	}
	for index := 0; index < count; index++ {
		rows.points[semanticDirectoryScaleID(semanticDirectoryPointRole, index)] = equation.PointAt(index)
		rows.members[semanticDirectoryScaleID(semanticDirectoryMemberRole, index)] = equation.RuleAt(index)
		rows.queries[semanticDirectoryScaleID(semanticDirectoryQueryRole, index)] = uint64(index)
	}
	return rows
}

// TestSemanticDirectoryResolvesAtMostOneLocatorPerContentID proves the
// published directory is a function from ContentID to one role locator, and
// that a construction aliasing one identity across two roles, or two
// identities onto one row slot, rejects the seal instead of publishing an
// ambiguous entry.
func TestSemanticDirectoryResolvesAtMostOneLocatorPerContentID(t *testing.T) {
	const count = 4
	fixture := newSemanticDirectoryScaleFixture(t, count)
	directory := fixture.topology.directory
	for id, entry := range directory.entries {
		_, isPoint := directory.point(id)
		_, isMember := directory.member(id)
		_, isQuery := directory.query(id)
		_, isActivation := directory.activation(id)
		roles := 0
		for _, resolved := range []bool{isPoint, isMember, isQuery, isActivation} {
			if resolved {
				roles++
			}
		}
		if roles != 1 || entry.kind < bindingSemanticPoint || entry.kind > bindingSemanticActivation {
			t.Fatalf("ContentID %v resolved %d role locators", id[0], roles)
		}
	}
	if _, resolved := directory.resolve(identity.ContentID{}); resolved {
		t.Fatal("unavailable ContentID resolved")
	}
	if _, resolved := directory.point(semanticDirectoryScaleID(semanticDirectoryMemberRole, 0)); resolved {
		t.Fatal("member identity resolved a Point locator")
	}

	topology, state, authority := fixture.topology.topology, fixture.topology.state, fixture.topology.authority
	if _, ok := sealSemanticDirectory(topology, state, authority, semanticDirectoryRows(count)); !ok {
		t.Fatal("admitted row set rejected")
	}
	aliased := semanticDirectoryRows(count)
	aliased.members[semanticDirectoryScaleID(semanticDirectoryPointRole, 0)] = equation.RuleAt(0)
	if _, ok := sealSemanticDirectory(topology, state, authority, aliased); ok {
		t.Fatal("one ContentID reached two role locators")
	}
	shared := semanticDirectoryRows(count)
	shared.points[semanticDirectoryScaleID(semanticDirectoryPointRole, count)] = equation.PointAt(0)
	if _, ok := sealSemanticDirectory(topology, state, authority, shared); ok {
		t.Fatal("two ContentIDs reached one row slot")
	}
	unavailable := semanticDirectoryRows(count)
	unavailable.points[identity.ContentID{}] = equation.PointAt(0)
	if _, ok := sealSemanticDirectory(topology, state, authority, unavailable); ok {
		t.Fatal("unavailable ContentID entered the directory")
	}
}

// semanticDirectoryCost reads the I3 construction cost directly off the sealed
// directory. Construction claims one entry per installed row and validates one
// query-order slot per query, so the installed row planes are the construction
// operation count; the reserved slots are the row planes themselves.
func semanticDirectoryCost(directory *semanticDirectory) (operations, capacity int) {
	if directory == nil {
		return 0, 0
	}
	operations = len(directory.entries) + len(directory.queryOrder)
	capacity = len(directory.entries) + len(directory.points) + len(directory.members) +
		len(directory.activations) + len(directory.queries) + len(directory.queryOrder)
	return operations, capacity
}

// TestSemanticDirectoryEntriesAreBoundedByAdmittedRootBindings is I3 law (a):
// entries never exceed R, the root bindings the sealed topology admits.
func TestSemanticDirectoryEntriesAreBoundedByAdmittedRootBindings(t *testing.T) {
	for _, count := range []int{1, 4, 16, 64} {
		fixture := newSemanticDirectoryScaleFixture(t, count)
		directory := fixture.topology.directory
		roots := fixture.roots
		if roots != 3*count {
			t.Fatalf("%d admitted root bindings for %d declared rows, want %d", roots, count, 3*count)
		}
		if len(directory.entries) > roots {
			t.Fatalf("directory holds %d entries for %d root bindings", len(directory.entries), roots)
		}
		for index := 0; index < count; index++ {
			_, pointOK := directory.point(semanticDirectoryScaleID(semanticDirectoryPointRole, index))
			_, memberOK := directory.member(semanticDirectoryScaleID(semanticDirectoryMemberRole, index))
			_, queryOK := directory.query(semanticDirectoryScaleID(semanticDirectoryQueryRole, index))
			if !pointOK || !memberOK || !queryOK {
				t.Fatalf("root binding %d is unresolvable", index)
			}
		}
	}
}

// TestSemanticDirectoryConstructionIsLinearInAdmittedRootBindings is I3 law
// (b): construction operations and reserved capacity stay under a+bR, and
// quadrupling R at most quadruples both counters.
func TestSemanticDirectoryConstructionIsLinearInAdmittedRootBindings(t *testing.T) {
	small := newSemanticDirectoryScaleFixture(t, 16)
	large := newSemanticDirectoryScaleFixture(t, 64)
	for _, fixture := range []semanticDirectoryScaleFixture{small, large} {
		operations, capacity := semanticDirectoryCost(fixture.topology.directory)
		if ceiling := semanticDirectoryConstructionBase + semanticDirectoryOperationsPerRoot*fixture.roots; operations > ceiling {
			t.Fatalf("construction ran %d operations for %d root bindings, want at most %d", operations, fixture.roots, ceiling)
		}
		if ceiling := semanticDirectoryConstructionBase + semanticDirectoryCapacityPerRoot*fixture.roots; capacity > ceiling {
			t.Fatalf("construction reserved %d slots for %d root bindings, want at most %d", capacity, fixture.roots, ceiling)
		}
	}
	growth := large.roots / small.roots
	smallOperations, smallCapacity := semanticDirectoryCost(small.topology.directory)
	largeOperations, largeCapacity := semanticDirectoryCost(large.topology.directory)
	if largeOperations > growth*smallOperations || largeCapacity > growth*smallCapacity {
		t.Fatalf("%dx root bindings cost %dx operations and %dx capacity", growth, largeOperations/smallOperations, largeCapacity/smallCapacity)
	}
}

// TestSemanticDirectoryGenerationUpdateReplacesRootBindings is I3 law (c): a
// generation update republishes the root bindings a locator resolves against
// and appends nothing to the directory, so retained generations cannot grow it.
func TestSemanticDirectoryGenerationUpdateReplacesRootBindings(t *testing.T) {
	const generations = 32
	fixture := newSemanticDirectoryScaleFixture(t, 16)
	directory := fixture.topology.directory
	beforeOperations, beforeCapacity := semanticDirectoryCost(directory)
	entries := len(directory.entries)
	relation, relationOK := fixture.topology.topology.InitialRelation()
	if !relationOK {
		t.Fatal("initial relation")
	}
	retained := make([]*ReceiptGraph, 0, generations)
	for generation := 0; generation < generations; generation++ {
		next, published := fixture.topology.topology.Publish(relation, nil)
		graph, graphOK := fixture.topology.Graph(next)
		if !published || !graphOK || graph == nil {
			t.Fatalf("generation %d publication", generation)
		}
		for index := 0; index < 16; index++ {
			point, pointOK := graph.lookupPoint(semanticDirectoryScaleID(semanticDirectoryPointRole, index))
			member, memberOK := graph.lookupRuleMember(semanticDirectoryScaleID(semanticDirectoryMemberRole, index))
			if !pointOK || !memberOK || point.graph != graph || member.graph != graph {
				t.Fatalf("generation %d lost root binding %d", generation, index)
			}
		}
		retained = append(retained, graph)
		relation = next
	}
	operations, capacity := semanticDirectoryCost(directory)
	if fixture.topology.directory != directory || len(directory.entries) != entries || operations != beforeOperations || capacity != beforeCapacity {
		t.Fatalf("%d retained generations grew the directory from %d to %d entries", len(retained), entries, len(directory.entries))
	}
}

// TestSemanticDirectoryPublicationAllocationIsIndependentOfRetainedGenerations
// is I3 law (d): publishing a generation that admits no new root binding costs
// a constant allocation, whatever R is and however many generations are held.
func TestSemanticDirectoryPublicationAllocationIsIndependentOfRetainedGenerations(t *testing.T) {
	const generations = 64
	counts := []int{16, 64}
	allocated := make([]uint64, len(counts))
	roots := make([]int, len(counts))
	for step, count := range counts {
		fixture := newSemanticDirectoryScaleFixture(t, count)
		relation, relationOK := fixture.topology.topology.InitialRelation()
		if !relationOK {
			t.Fatal("initial relation")
		}
		retained := make([]*ReceiptGraph, 0, generations)
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for generation := 0; generation < generations; generation++ {
			next, published := fixture.topology.topology.Publish(relation, nil)
			graph, graphOK := fixture.topology.Graph(next)
			if !published || !graphOK {
				t.Fatalf("generation %d publication", generation)
			}
			retained = append(retained, graph)
			relation = next
		}
		runtime.ReadMemStats(&after)
		if len(retained) != generations {
			t.Fatal("retained generations")
		}
		allocated[step], roots[step] = after.TotalAlloc-before.TotalAlloc, fixture.roots
		if ceiling := uint64(generations * semanticDirectoryPublicationBytes); allocated[step] > ceiling {
			t.Fatalf("%d generations over %d root bindings allocated %d bytes, want at most %d", generations, roots[step], allocated[step], ceiling)
		}
	}
	// The delta between the two runs is the root-binding count alone, so an
	// allocation that grew with it would price retention by R instead of by the
	// changed root bindings.
	if roots[1] <= roots[0] || allocated[1] > allocated[0]+generations*semanticDirectoryConstructionBase {
		t.Fatalf("%d root bindings allocated %d bytes against %d bytes for %d", roots[1], allocated[1], allocated[0], roots[0])
	}
}
