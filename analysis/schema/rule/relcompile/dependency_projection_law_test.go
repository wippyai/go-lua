package relcompile_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/schema/rule/relcompile"
	valuesource "github.com/wippyai/go-lua/domain/value/source"
)

// TestDependencyProjectionCarriesOnlyCommittedExpressionReads proves that a
// lowered dependency projection is exactly the relational expression's state
// effect. Apply's signature is a transient typed computation over its child;
// its input/authority relations are not extra database edges. Publish is the
// sole write boundary, so adding those signature relations would fabricate a
// recurrence edge the certificate correctly does not derive.
func TestDependencyProjectionCarriesOnlyCommittedExpressionReads(t *testing.T) {
	spec := valuesource.RuleEntry()
	surfaces := newOwners(t)
	placement := surfaces.install(spec)
	rules, err := relcompile.Resolve(surfaces.registry, spec, placement)
	if err != nil {
		t.Fatalf("resolve %s: %v", spec.Key, err)
	}
	compiled := lower(t, surfaces, spec, rules)

	dependencies := compiled.Dependencies()
	if len(dependencies) != 1 {
		t.Fatalf("%s lowered to %d dependencies, want 1", spec.Key, len(dependencies))
	}
	reads := dependencies[0].Reads()
	if len(reads) != 1 || reads[0].ID() != rules[0].Candidate {
		t.Fatalf("%s dependency reads = %d, want exactly its one expression input", spec.Key, len(reads))
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
