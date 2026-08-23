package composite

import "testing"

// TestBuildSealsIndependentCompilationsWithOneSemanticIdentity states the
// Workspace environment boundary: equal declarations seal equal identities,
// but no mutable engine schema instance is shared between environments.
func TestBuildSealsIndependentCompilationsWithOneSemanticIdentity(t *testing.T) {
	first, firstOK := Build()
	second, secondOK := Build()
	if !firstOK || !secondOK || !first.Available() || !second.Available() {
		t.Fatal("independent compilation unavailable")
	}
	if first.Schema() == second.Schema() {
		t.Fatal("independent compilations shared one engine schema instance")
	}
	if first.Digest() != second.Digest() || first.ExecutionSchemaID() != second.ExecutionSchemaID() {
		t.Fatal("equal declarations produced different semantic identities")
	}
	firstPublication, firstPublicationOK := first.Publication()
	secondPublication, secondPublicationOK := second.Publication()
	firstSchemaID, firstSchemaIDOK := firstPublication.SchemaID()
	secondSchemaID, secondSchemaIDOK := secondPublication.SchemaID()
	if !firstPublicationOK || !secondPublicationOK || !firstSchemaIDOK || !secondSchemaIDOK || firstSchemaID != secondSchemaID {
		t.Fatal("equal declarations produced different publication layouts")
	}
}

// TestEveryGeneratedRuleDispatchesThroughItsOwnPlanRow is the one dispatch law
// over the whole table, in place of one copy per family. A generated member
// dispatches by the Rule ordinal its descriptor carries, so the pairing between
// a declared Program and the plan row that executes it must be a bijection: a
// Program-carrying rule has exactly one present plan row, that row sits at the
// rule's own canonical ordinal and restates the Program's declared shape, and
// no plan row exists for a rule that declares no Program. Two rules sharing a
// row, or a row without a declaration, would both be a member executing a
// program its own declaration never named.
func TestEveryGeneratedRuleDispatchesThroughItsOwnPlanRow(t *testing.T) {
	compilation, ok := Build()
	if !ok {
		t.Fatal("composite compilation unavailable")
	}
	plans, plansOK := compilation.RulePlans()
	table, failure := Table(compilation)
	if !plansOK || failure.Available() || table == nil || plans.Digest() != table.Digest() {
		t.Fatal("rule plans do not carry the composite declaration identity")
	}
	templates, contributors, templatesOK := RuleTemplates[principals, authorities]()
	if !templatesOK || len(templates) != len(contributors) || len(templates) != RuleCount(compilation) {
		t.Fatalf("rule table = %d templates / %d contributors / %d sealed rows", len(templates), len(contributors), RuleCount(compilation))
	}
	generated := 0
	for position, template := range templates {
		key, keyOK := RuleKeyAt(compilation, position)
		if template == nil || !keyOK || key != template.Key() {
			t.Fatalf("sealed row %d = %q, table row = %v", position, key, template)
		}
		compiled, compiledOK := plans.At(position)
		declaration := template.Program()
		if !declaration.Available() {
			// A hand-wired rule is executed by its own domain. A plan row for it
			// would be a second executable body for one declaration.
			if compiledOK && compiled.Present() {
				t.Fatalf("%s declares no Program yet carries plan row %d", key, position)
			}
			if contributors[position].generated.Available() {
				t.Fatalf("%s is wired generated without a Program", key)
			}
			continue
		}
		generated++
		if !compiledOK || !compiled.Present() || compiled.Rule() != uint32(position) {
			t.Fatalf("%s plan row = %+v/%t at ordinal %d", key, compiled, compiledOK, position)
		}
		semantic, semanticOK := RuleSemantic(compilation, key)
		if !semanticOK || compiled.Semantic() != semantic {
			t.Fatalf("%s plan row semantic = %v, resolved %v/%t", key, compiled.Semantic(), semantic, semanticOK)
		}
		if compiled.JoinCount() != declaration.JoinCount() || compiled.FoldInputCount() != len(declaration.Fold.Inputs) ||
			compiled.OutputCount() != len(declaration.Fold.Outputs) {
			t.Fatalf("%s plan row shape = joins:%d inputs:%d outputs:%d, declared joins:%d inputs:%d outputs:%d",
				key, compiled.JoinCount(), compiled.FoldInputCount(), compiled.OutputCount(),
				declaration.JoinCount(), len(declaration.Fold.Inputs), len(declaration.Fold.Outputs))
		}
		if _, carried := compiled.Carry(); carried != (declaration.Carry != nil) {
			t.Fatalf("%s plan row carry = %t, declared %t", key, carried, declaration.Carry != nil)
		}
		// The lane is the contributor's, and the composition registers the slot
		// through the handoff that lane names. A generated row wired on a lane
		// with no handoff would declare a program nothing can register.
		if contributors[position].generated != template.Lane() {
			t.Fatalf("%s wired lane = %v, declared %v", key, contributors[position].generated, template.Lane())
		}
		if _, laneOK := generatedLaneHandoff(template.Lane()); !laneOK {
			t.Fatalf("%s declares a Program on lane %v, which has no generated handoff", key, template.Lane())
		}
	}
	present := 0
	for index := 0; index < plans.Count(); index++ {
		compiled, compiledOK := plans.At(index)
		if compiledOK && compiled.Present() {
			present++
		}
	}
	if generated == 0 || present != generated {
		t.Fatalf("plan rows = %d, generated declarations = %d", present, generated)
	}
}
