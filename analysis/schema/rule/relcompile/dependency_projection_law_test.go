package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	valuesource "github.com/wippyai/go-lua/domain/value/source"
)

// TestDependencyProjectionCarriesEverySemanticRead states that a lowered
// dependency's read set is the whole set the expression reads. A semantic
// operation reads the relation each declared input is delivered from, the
// denominator that input is closed against, and the denominator its output
// authority is proven under. None of those is named by a join, so a projection
// built from joins alone is a read set the recurrence checker derives
// differently - and a dependency graph two authorities disagree about is not
// one graph.
func TestDependencyProjectionCarriesEverySemanticRead(t *testing.T) {
	spec := valuesource.RuleEntry()
	surfaces := newOwners(t)
	placement := surfaces.install(spec)
	resolution, err := relcompile.Resolve(surfaces.registry, spec, placement)
	if err != nil {
		t.Fatalf("resolve %s: %v", spec.Key, err)
	}
	rules := resolution.Rules
	compiled := lower(t, surfaces, spec, rules)

	dependencies := compiled.Dependencies()
	if len(dependencies) != 1 {
		t.Fatalf("%s lowered to %d dependencies, want 1", spec.Key, len(dependencies))
	}
	operations := compiled.Signatures()
	if len(operations) != 1 {
		t.Fatalf("%s lowered to %d signatures, want 1", spec.Key, len(operations))
	}
	authority := operations[0].Authority().Denominator.Relation()
	if !authority.Available() {
		t.Fatal("the lowered signature carries no output authority denominator")
	}
	if !readsRelation(dependencies[0], authority) {
		t.Fatalf("the dependency of %s does not read the denominator its output authority is proven under", spec.Key)
	}
	for index, input := range operations[0].Inputs() {
		if !readsRelation(dependencies[0], input.Relation) {
			t.Fatalf("the dependency of %s does not read the relation input[%d] is delivered from", spec.Key, index)
		}
		if !readsRelation(dependencies[0], input.Denominator.Relation()) {
			t.Fatalf("the dependency of %s does not read the denominator input[%d] is closed against", spec.Key, index)
		}
	}
}

func readsRelation(dependency plan.Dependency, relation model.RelationID) bool {
	for _, read := range dependency.Reads() {
		if read.ID() == relation {
			return true
		}
	}
	return false
}
