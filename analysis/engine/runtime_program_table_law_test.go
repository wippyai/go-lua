package engine

import (
	"os"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/identity"
)

// The two published-table laws of the construction plane. bindProgramQueryTable
// and bindProgramObservationTable are the whole of what a sealed program knows
// about its own addressability, so these laws state that knowledge directly:
// every declared query reaches exactly one published row coordinate, and every
// admitted observation identity reaches exactly one dense ordinal.
//
// Both are driven from a real construction. The inputs the mutations start from
// are the ones the seal itself resolved - the published address set the
// committed directory issued, the graph the construction bound, and the rows the
// bind produced - so a mutation names a hostile variant of a real construction
// rather than a table assembled for the test.

// programQueryBindInputs recovers the exact triple bindProgramQueryTable was
// called with during the fixture's own seal: the published address set, the
// graph, and the bound rows keyed by query.
func programQueryBindInputs(t testing.TB, fixture receiptQueryMatrixFixture) ([]composition.Key, map[composition.Key]runtimeQuery) {
	t.Helper()
	program := fixture.solver.runtime.program
	if !program.valid() {
		t.Fatal("sealed program is invalid")
	}
	bound := make(map[composition.Key]runtimeQuery, program.queryCount())
	for index := 0; index < program.queryCount(); index++ {
		row, present := program.queryAt(index)
		if !present || row == nil {
			t.Fatalf("sealed query row %d is absent", index)
		}
		bound[row.query().Key()] = row
	}
	addressed := append([]composition.Key(nil), fixture.addressed...)
	if len(addressed) == 0 || len(bound) != len(addressed) {
		t.Fatalf("construction published %d addresses for %d bound rows", len(addressed), len(bound))
	}
	return addressed, bound
}

// TestProgramQueryTableResolvesEveryDeclaredQueryToOnePublishedRow is the query
// half: the sealed table is a bijection between the graph's declared queries and
// the directory's published addresses, laid out in graph order, and every
// hostile variant of that correspondence is refused whole.
func TestProgramQueryTableResolvesEveryDeclaredQueryToOnePublishedRow(t *testing.T) {
	const count = 4
	order := benchIdentityOrder(count)
	fixture := newReceiptQueryMatrixFixture(t, count, order, order)
	runtime := fixture.solver.runtime
	program := runtime.program
	graph := runtime.graph

	// Total: one row per declared query, and the runtime reads the same table.
	if program.queryCount() != graph.QueryCount() || len(runtime.queries) != graph.QueryCount() {
		t.Fatalf("program holds %d query rows and the runtime %d for %d declared queries", program.queryCount(), len(runtime.queries), graph.QueryCount())
	}
	// Injective and in canonical graph order: row i answers declared query i,
	// and no key repeats.
	seen := make(map[composition.Key]int, program.queryCount())
	for index := 0; index < program.queryCount(); index++ {
		row, present := program.queryAt(index)
		declared, indexed := graph.QueryAt(index)
		if !present || row == nil || !indexed {
			t.Fatalf("query row %d present=%t declared=%t", index, present && row != nil, indexed)
		}
		if row.query().Key() != declared.Key() {
			t.Fatalf("query row %d answers %v, the graph declares %v at that ordinal", index, row.query().Key(), declared.Key())
		}
		if previous, duplicate := seen[declared.Key()]; duplicate {
			t.Fatalf("query rows %d and %d both answer %v", previous, index, declared.Key())
		}
		seen[declared.Key()] = index
	}
	// Every published address names a distinct declared query, so the address
	// set and the table are one correspondence rather than two orderings.
	addressed, bound := programQueryBindInputs(t, fixture)
	published := make(map[composition.Key]struct{}, len(addressed))
	for ordinal, key := range addressed {
		if _, duplicate := published[key]; duplicate {
			t.Fatalf("published address ordinal %d repeats %v", ordinal, key)
		}
		if _, declared := seen[key]; !declared {
			t.Fatalf("published address ordinal %d names %v, which the graph does not declare", ordinal, key)
		}
		published[key] = struct{}{}
	}
	if len(published) != graph.QueryCount() {
		t.Fatalf("directory published %d addresses for %d declared queries", len(published), graph.QueryCount())
	}

	// The real inputs rebuild the real table.
	rebuilt, rebuiltOK := bindProgramQueryTable(addressed, graph, bound)
	if !rebuiltOK || len(rebuilt) != program.queryCount() {
		t.Fatalf("the construction's own inputs rebound %d rows, ok=%t", len(rebuilt), rebuiltOK)
	}
	for index, row := range rebuilt {
		sealed, present := program.queryAt(index)
		if !present || row != sealed {
			t.Fatalf("rebound row %d is not the sealed row", index)
		}
	}

	// Duplicate address: two ordinals publishing one query. The whole declared
	// set stays addressed, so only the injectivity of the address set is at
	// stake.
	duplicated := append(append([]composition.Key(nil), addressed...), addressed[0])
	if _, ok := bindProgramQueryTable(duplicated, graph, bound); ok {
		t.Fatal("two published ordinals addressed one query")
	}
	// Dropped address: a declared query with no column to answer on.
	dropped := append([]composition.Key(nil), addressed[:len(addressed)-1]...)
	if _, ok := bindProgramQueryTable(dropped, graph, bound); ok {
		t.Fatal("a declared query was sealed with no published address")
	}
	// Foreign address: an ordinal publishing a query this graph does not own,
	// added alongside the complete declared set so only the surplus is at stake.
	foreign := append(append([]composition.Key(nil), addressed...), compositionKeyOf(coldKey(956_401)))
	if _, ok := bindProgramQueryTable(foreign, graph, bound); ok {
		t.Fatal("a published ordinal addressed a query outside the graph")
	}
	// Substituted address: an ordinal naming a foreign query in place of a
	// declared one, so the declared query loses its column.
	substituted := append([]composition.Key(nil), addressed...)
	substituted[0] = compositionKeyOf(coldKey(956_402))
	if _, ok := bindProgramQueryTable(substituted, graph, bound); ok {
		t.Fatal("a declared query lost its column to a foreign address")
	}
	// Unavailable address alongside the complete declared set.
	unavailable := append(append([]composition.Key(nil), addressed...), composition.Key{})
	if _, ok := bindProgramQueryTable(unavailable, graph, bound); ok {
		t.Fatal("an unavailable published address entered the table")
	}
	// Dropped row: an addressed query with no bound implementation.
	missing := make(map[composition.Key]runtimeQuery, len(bound))
	for key, row := range bound {
		missing[key] = row
	}
	delete(missing, addressed[0])
	if _, ok := bindProgramQueryTable(addressed, graph, missing); ok {
		t.Fatal("an addressed query was sealed with no bound row")
	}
	// Displaced row: a bound row filed under a key it does not answer.
	displaced := make(map[composition.Key]runtimeQuery, len(bound))
	for key, row := range bound {
		displaced[key] = row
	}
	displaced[addressed[0]] = displaced[addressed[1]]
	if _, ok := bindProgramQueryTable(addressed, graph, displaced); ok {
		t.Fatal("a bound row answered a key other than its own")
	}
	// Nil row under a real address.
	nilled := make(map[composition.Key]runtimeQuery, len(bound))
	for key, row := range bound {
		nilled[key] = row
	}
	nilled[addressed[0]] = nil
	if _, ok := bindProgramQueryTable(addressed, graph, nilled); ok {
		t.Fatal("a nil row was sealed under a published address")
	}
	if _, ok := bindProgramQueryTable(addressed, nil, bound); ok {
		t.Fatal("the query table bound against no graph")
	}
	if _, ok := bindProgramQueryTable(addressed, graph, nil); ok {
		t.Fatal("the query table bound with no rows")
	}
}

// TestProgramObservationTableResolvesEveryIdentityToOneDenseOrdinal is the
// observation half: the attach ordinal an issued handle carries is its position
// in the sealed table, the ordinals are dense over the admitted identities, and
// a table that does not hold one row per admitted identity is refused.
func TestProgramObservationTableResolvesEveryIdentityToOneDenseOrdinal(t *testing.T) {
	const count = 4
	order := benchIdentityOrder(count)
	fixture := newObservedReceiptQueryMatrixFixture(t, count, order, order)
	runtime := fixture.solver.runtime
	program := runtime.program

	// Total: one row per admitted identity, and the runtime reads the same table.
	if program.observationCount() != count || len(runtime.observations) != count {
		t.Fatalf("program holds %d observation rows and the runtime %d for %d attached identities", program.observationCount(), len(runtime.observations), count)
	}
	// Dense and injective: attach ordinal i addresses row i, which carries
	// identity i, and no identity reaches two ordinals.
	byID := make(map[identity.ContentID]uint64, count)
	for attach, observation := range fixture.observations {
		id := fixture.observationIDs[attach]
		if !observation.Available() || observation.id != id {
			t.Fatalf("issued observation %d carries %v for identity %v", attach, observation.id, id)
		}
		if observation.ordinal != uint64(attach) {
			t.Fatalf("observation %v was issued ordinal %d at attach position %d", id, observation.ordinal, attach)
		}
		if previous, duplicate := byID[id]; duplicate {
			t.Fatalf("identity %v reached ordinals %d and %d", id, previous, observation.ordinal)
		}
		byID[id] = observation.ordinal
		row, present := program.observationAt(int(observation.ordinal))
		if !present || row == nil || row.observationID() != id {
			t.Fatalf("ordinal %d addresses row present=%t identity=%v, want %v", observation.ordinal, present && row != nil, rowObservationID(row), id)
		}
	}
	if len(byID) != program.observationCount() {
		t.Fatalf("%d identities address %d sealed rows", len(byID), program.observationCount())
	}
	// Every sealed row is addressed by exactly one issued ordinal, so no row
	// exists past the end of the issued sequence.
	for index := 0; index < program.observationCount(); index++ {
		row, present := program.observationAt(index)
		if !present || row == nil {
			t.Fatalf("sealed observation row %d is absent", index)
		}
		ordinal, addressed := byID[row.observationID()]
		if !addressed || ordinal != uint64(index) {
			t.Fatalf("sealed row %d carries identity %v addressed at ordinal %d/%t", index, row.observationID(), ordinal, addressed)
		}
	}

	// The real rows rebuild the real table.
	bound := make([]runtimeObservation, 0, program.observationCount())
	for index := 0; index < program.observationCount(); index++ {
		row, _ := program.observationAt(index)
		bound = append(bound, row)
	}
	rebuilt, rebuiltOK := bindProgramObservationTable(bound, len(bound))
	if !rebuiltOK || len(rebuilt) != len(bound) {
		t.Fatalf("the construction's own rows rebound %d observations, ok=%t", len(rebuilt), rebuiltOK)
	}
	for index, row := range rebuilt {
		if row != bound[index] {
			t.Fatalf("rebound observation %d is not the sealed row", index)
		}
	}
	// A dropped row leaves an issued ordinal addressing a position past the end
	// of the table.
	if _, ok := bindProgramObservationTable(bound[:len(bound)-1], len(bound)); ok {
		t.Fatal("an admitted identity was sealed with no row")
	}
	// A row with no admitted identity behind it.
	surplus := append(append([]runtimeObservation(nil), bound...), bound[0])
	if _, ok := bindProgramObservationTable(surplus, len(bound)); ok {
		t.Fatal("a row was sealed for no admitted identity")
	}
	// A hole in the table.
	holed := append([]runtimeObservation(nil), bound...)
	holed[len(holed)-1] = nil
	if _, ok := bindProgramObservationTable(holed, len(holed)); ok {
		t.Fatal("a hole was sealed into the observation table")
	}
	if _, ok := bindProgramObservationTable(bound, -1); ok {
		t.Fatal("a negative admitted count sealed a table")
	}
	// The empty table is the one legal degenerate case: no identity, no row.
	if rows, ok := bindProgramObservationTable(nil, 0); !ok || len(rows) != 0 {
		t.Fatal("an unobserved construction was refused an empty observation table")
	}
}

// rowObservationID reads an identity from a possibly absent row so a failure
// message can report what was found.
func rowObservationID(row runtimeObservation) identity.ContentID {
	if row == nil {
		return identity.ContentID{}
	}
	return row.observationID()
}

// TestProgramConstructionRefusesADuplicateObservationIdentity states the
// injectivity of the identity -> ordinal map at its source: the construction
// admits one ordinal per identity and refuses the second attach, so the sealed
// table can never hold two rows for one identity.
func TestProgramConstructionRefusesADuplicateObservationIdentity(t *testing.T) {
	const count = 2
	order := benchIdentityOrder(count)
	fixture := newObservedReceiptQueryMatrixFixture(t, count, order, order)
	// The sealed construction is terminal, so a duplicate attach is proven on a
	// fresh construction over the same committed graph.
	construction, constructed := BeginProgramConstruction(fixture.binding, fixture.graph)
	if !constructed || construction == nil {
		t.Fatal("second construction over the committed graph")
	}
	defer construction.Close()
	member, memberOK := fixture.graph.RuleMember(receiptAssemblySemanticID(byte(60)))
	if !memberOK || len(fixture.queryImplementations) == 0 {
		t.Fatal("duplicate observation inputs")
	}
	implementation := fixture.queryImplementations[0]
	id := fixture.observationIDs[0]
	first, firstFailure := AttachRuleExactObservationWithFailure(construction, implementation, id, member)
	if firstFailure != receiptObservationAttachFailureNone || first.ordinal != 0 {
		t.Fatalf("first attach failure=%v ordinal=%d", firstFailure, first.ordinal)
	}
	second, secondFailure := AttachRuleExactObservationWithFailure(construction, implementation, id, member)
	if secondFailure != receiptObservationAttachFailureDuplicate || second.Available() {
		t.Fatalf("one identity reached a second ordinal: failure=%v available=%t", secondFailure, second.Available())
	}
}

// TestAssembleMountedProgramConstructsFromSealedTemplates is the production
// assemble floor: sealed templates and role capabilities enter directly.
// CommittedProgram holds the equation graph and binding topology, not a
// receipt wrapper.
func TestAssembleMountedProgramConstructsFromSealedTemplates(t *testing.T) {
	for _, name := range []string{"runtime_program_assemble.go", "runtime_program_construction.go"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "graph *ReceiptGraph") {
			t.Fatalf("%s stores *ReceiptGraph", name)
		}
	}
	src, err := os.ReadFile("runtime_program_assemble.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, name := range []string{
		"NewArtifactScalarReceipt",
		"NewMountedArtifactReceipt",
		"AssembleMountedArtifactReceipt",
		"NewArtifactScalarBinding",
		"ReceiptGraph",
		"*ReceiptAssembly",
	} {
		if strings.Contains(body, name) {
			t.Errorf("assemble still names %s", name)
		}
	}
	for _, check := range []struct {
		path      string
		signature string
		forbidden []string
	}{
		{
			path:      "runtime_program_lower.go",
			signature: "func assembleSealedProgramMounts",
			forbidden: []string{"*ReceiptAssembly", "ReceiptGraph", "beginReceiptAssembly"},
		},
		{
			path:      "runtime_program_lower.go",
			signature: "func beginMountedProgramAssembly",
			forbidden: []string{"*ReceiptAssembly", "beginReceiptAssembly"},
		},
		{
			path:      "runtime_program_lower.go",
			signature: "func admitMountedArtifactSites",
			forbidden: []string{"*ReceiptAssembly"},
		},
		{
			path:      "binding_topology_builder.go",
			signature: "func (binding *BindingTopology) Graph(",
			forbidden: []string{"&ReceiptGraph{"},
		},
		{
			path:      "schema_factor_binding.go",
			signature: "type RuleImplementation[K ~uint32 | ~uint64, V, O any] struct",
			forbidden: []string{"receipt "},
		},
		{
			path:      "schema_factor_binding.go",
			signature: "type FactorImplementation[K ~uint32 | ~uint64, V any] struct",
			forbidden: []string{"receipt "},
		},
		{
			path:      "schema_activation_binding.go",
			signature: "type ActivationRuleImplementation struct",
			forbidden: []string{"receipt "},
		},
		{
			path:      "schema_query_binding.go",
			signature: "type ExactQueryImplementation[V, R any] struct",
			forbidden: []string{"receipt "},
		},
		{
			path:      "schema_query_binding.go",
			signature: "type SummaryQueryImplementation[V, R any] struct",
			forbidden: []string{"receipt "},
		},
		{
			path:      "runtime_binding.go",
			signature: "func (binding *runtimeBinding) pinBinding",
			forbidden: []string{"pinReceipt"},
		},
		{
			path:      "runtime_program_admit.go",
			signature: "func applyMountedProgramAdmission",
			forbidden: []string{"*ReceiptAssembly", "&ReceiptAssembly{"},
		},
		{
			path:      "runtime_program_admit.go",
			signature: "func (implementation *RuleImplementation[K, V, O]) AdmitMounted",
			forbidden: []string{"*ReceiptAssembly"},
		},
		{
			path:      "runtime_program_admit.go",
			signature: "func (implementation *RuleImplementation[K, V, O]) AdmitLink",
			forbidden: []string{"*ReceiptAssembly"},
		},
		{
			path:      "runtime_activation_admit.go",
			signature: "func AdmitMountedActivationOccurrence",
			forbidden: []string{"*ReceiptAssembly"},
		},
		{
			path:      "operand_resolver.go",
			signature: "type RuleProgramAttach interface",
			forbidden: []string{"*ReceiptAssembly"},
		},
		{
			path:      "runtime_binding.go",
			signature: "type runtimeBinding struct",
			forbidden: []string{"runtimeBindingMode", "runtimeBindingReceipt"},
		},
	} {
		source, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatal(err)
		}
		body, ok := functionBody(string(source), check.signature)
		if !ok {
			t.Errorf("%s missing %s", check.path, check.signature)
			continue
		}
		for _, name := range check.forbidden {
			if strings.Contains(body, name) {
				t.Errorf("%s still names %s", check.signature, name)
			}
		}
	}
}

func functionBody(src, signature string) (string, bool) {
	start := strings.Index(src, signature)
	if start < 0 {
		return "", false
	}
	open := strings.IndexByte(src[start:], '{')
	if open < 0 {
		return "", false
	}
	depth := 0
	for index := start + open; index < len(src); index++ {
		switch src[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : index+1], true
			}
		}
	}
	return "", false
}

// TestAssembleMountedProgramPublishesConstructedGeometry is the production
// execution floor of the constructor switch: the one mounted-program entry
// admits sealed templates plus owner admission rows and publishes the geometry
// the pure constructor derives - the mounted member table, the native Call
// stage inverse and the Link bootstrap anchor.
func TestAssembleMountedProgramPublishesConstructedGeometry(t *testing.T) {
	owner := newNativeCallStageQueryLawOwner(t)
	rules := owner.rules()
	mount, mountOK := owner.scalarMount(t, rules, owner.order())
	bootstrap, bootstrapOK := NewProgramBootstrap(artifactScalarLawID(0x68), artifactScalarLawID(0x69))
	queryID := artifactScalarLawID(0x7C)
	queryAdmission, queryAdmissionOK := NewExactQueryAdmission(owner.query, queryID, owner.mount, owner.base)
	if !mountOK || !bootstrapOK || !queryAdmissionOK {
		t.Fatal("mounted program inputs")
	}
	if !installConstOperandResolver(owner.implementation, struct{}{}) {
		t.Fatal("mounted program operand resolver")
	}
	admission := MountedProgramAdmission{Mounted: make([]MountedRuleAdmission, 0, len(rules)), Queries: []ProgramQueryAdmission{queryAdmission}}
	for _, rule := range rules {
		admission.Mounted = append(admission.Mounted, MountedRuleAdmission{
			Attach: owner.implementation, Capability: owner.capability,
			Mount: owner.mount, Point: rule.Point, Occurrence: rule.ID,
		})
	}
	program, refusal, committed := AssembleMountedProgram(owner.binding, []MountedProgramArtifact{mount}, admission, bootstrap)
	if !committed || program == nil {
		t.Fatalf("mounted program assemble stage=%v lowered=%t commit=%v schedule=%d", refusal.Stage(), refusal.Lowered(), refusal.Commit(), refusal.ScheduleRow())
	}
	artifactID := mount.Template.ArtifactID()
	for _, reusable := range owner.order() {
		if _, published := program.lookupPoint(mountedArtifactID("analysis/engine/artifact-point/v1", owner.mount, artifactID, reusable)); !published {
			t.Fatalf("template point %v publishes no address", reusable)
		}
	}
	if _, published := program.lookupPoint(linkBootstrapPointSemanticID(artifactScalarLawID(0x68), artifactScalarLawID(0x69))); !published {
		t.Fatal("Link bootstrap anchor publishes no address")
	}
	for _, rule := range rules {
		if _, published := program.MountedRuleMember(owner.capability, owner.mount, rule.Point, rule.ID); !published {
			t.Fatalf("admitted member %v publishes no address", rule.ID)
		}
		stage, staged := program.MountedNativeCallStage(owner.capability, owner.mount, rule.ID)
		if !staged || stage.Kind() != rule.Stage || stage.PointID() != rule.Point {
			t.Fatalf("native Call stage %v unaddressed", rule.ID)
		}
	}
	if _, published := program.Query(queryID); !published {
		t.Fatal("admitted query publishes no address")
	}
}
