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

func TestValueTransferHasOneSealedExecutableProgram(t *testing.T) {
	compilation, ok := Build()
	if !ok {
		t.Fatal("composite compilation unavailable")
	}
	plans, plansOK := compilation.RulePlans()
	table, failure := Table(compilation)
	if !plansOK || failure.Available() || table == nil || plans.Digest() != table.Digest() {
		t.Fatal("rule plans do not carry the composite declaration identity")
	}
	position := -1
	for index := 0; index < RuleCount(compilation); index++ {
		key, keyOK := RuleKeyAt(compilation, index)
		if keyOK && key == "value-transfer" {
			position = index
			break
		}
	}
	if position < 0 {
		t.Fatal("value-transfer rule missing from the sealed inventory")
	}
	compiled, compiledOK := plans.At(position)
	if !compiledOK || !compiled.Present() || compiled.Rule() != uint32(position) ||
		compiled.JoinCount() != 1 || compiled.FoldInputCount() != 1 || compiled.OutputCount() != 1 {
		t.Fatalf("value-transfer compiled plan = %+v/%t", compiled, compiledOK)
	}
}

func TestValueSourceHasOneSealedExecutableProgram(t *testing.T) {
	compilation, ok := Build()
	if !ok {
		t.Fatal("composite compilation unavailable")
	}
	plans, plansOK := compilation.RulePlans()
	table, failure := Table(compilation)
	if !plansOK || failure.Available() || table == nil || plans.Digest() != table.Digest() {
		t.Fatal("rule plans do not carry the composite declaration identity")
	}
	position := -1
	for index := 0; index < RuleCount(compilation); index++ {
		key, keyOK := RuleKeyAt(compilation, index)
		if keyOK && key == "value-source" {
			position = index
			break
		}
	}
	if position < 0 {
		t.Fatal("value-source rule missing from the sealed inventory")
	}
	compiled, compiledOK := plans.At(position)
	if !compiledOK || !compiled.Present() || compiled.Rule() != uint32(position) ||
		compiled.JoinCount() != 0 || compiled.FoldInputCount() != 0 || compiled.OutputCount() != 1 {
		t.Fatalf("value-source compiled plan = %+v/%t", compiled, compiledOK)
	}
}

func TestHeapIngressHasOneSealedExecutableProgram(t *testing.T) {
	if _, failure := newCatalog(); failure.Available() {
		key := ""
		templates, _, templatesOK := RuleTemplates[principals, authorities]()
		if templatesOK {
			for _, template := range templates {
				if template != nil && template.ID() == failure.Entry {
					key = string(template.Key())
					break
				}
			}
		}
		t.Fatalf("composite compilation unavailable: law=%d disposition=%s rule=%q", failure.Law, failure.Disposition, key)
	}
	compilation, ok := Build()
	if !ok {
		t.Fatal("composite compilation unavailable")
	}
	plans, plansOK := compilation.RulePlans()
	table, failure := Table(compilation)
	if !plansOK || failure.Available() || table == nil || plans.Digest() != table.Digest() {
		t.Fatal("rule plans do not carry the composite declaration identity")
	}
	position := -1
	for index := 0; index < RuleCount(compilation); index++ {
		key, keyOK := RuleKeyAt(compilation, index)
		if keyOK && key == "heap-ingress" {
			position = index
			break
		}
	}
	if position < 0 {
		t.Fatal("heap-ingress rule missing from the sealed inventory")
	}
	compiled, compiledOK := plans.At(position)
	if !compiledOK || !compiled.Present() || compiled.Rule() != uint32(position) ||
		compiled.JoinCount() != 0 || compiled.FoldInputCount() != 0 || compiled.OutputCount() != 1 {
		t.Fatalf("heap-ingress compiled plan = %+v/%t", compiled, compiledOK)
	}
}

func TestPackSourceHasOneSealedExecutableProgram(t *testing.T) {
	compilation, ok := Build()
	if !ok {
		t.Fatal("composite compilation unavailable")
	}
	plans, plansOK := compilation.RulePlans()
	table, failure := Table(compilation)
	if !plansOK || failure.Available() || table == nil || plans.Digest() != table.Digest() {
		t.Fatal("rule plans do not carry the composite declaration identity")
	}
	position := -1
	for index := 0; index < RuleCount(compilation); index++ {
		key, keyOK := RuleKeyAt(compilation, index)
		if keyOK && key == "pack-source" {
			position = index
			break
		}
	}
	if position < 0 {
		t.Fatal("pack-source rule missing from the sealed inventory")
	}
	compiled, compiledOK := plans.At(position)
	if !compiledOK || !compiled.Present() || compiled.Rule() != uint32(position) ||
		compiled.JoinCount() != 0 || compiled.FoldInputCount() != 0 || compiled.OutputCount() != 1 {
		t.Fatalf("pack-source compiled plan = %+v/%t", compiled, compiledOK)
	}
}

func TestStaticTransferHasOneSealedExecutableProgram(t *testing.T) {
	compilation, ok := Build()
	if !ok {
		t.Fatal("composite compilation unavailable")
	}
	plans, plansOK := compilation.RulePlans()
	table, failure := Table(compilation)
	if !plansOK || failure.Available() || table == nil || plans.Digest() != table.Digest() {
		t.Fatal("rule plans do not carry the composite declaration identity")
	}
	position := -1
	for index := 0; index < RuleCount(compilation); index++ {
		key, keyOK := RuleKeyAt(compilation, index)
		if keyOK && key == "static-transfer" {
			position = index
			break
		}
	}
	if position < 0 {
		t.Fatal("static-transfer rule missing from the sealed inventory")
	}
	compiled, compiledOK := plans.At(position)
	if !compiledOK || !compiled.Present() || compiled.Rule() != uint32(position) ||
		compiled.JoinCount() != 1 || compiled.FoldInputCount() != 1 || compiled.OutputCount() != 1 {
		t.Fatalf("static-transfer compiled plan = %+v/%t", compiled, compiledOK)
	}
}
