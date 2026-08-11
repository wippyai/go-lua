package engine_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
)

// TestRelationRepeatedSelectionHasOneExactContribution proves the public
// consequence of discovery canonicalization.  A selector may encounter the
// same Link relation more than once during an epoch; the next epoch has one
// exact relation and reaches its ordinary semantic result.
func TestRelationRepeatedSelectionHasOneExactContribution(t *testing.T) {
	fixture := directCallFixtureFor(t)
	solver, err := engine.New(fixture.link)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	result := relationFactor(t, solver, "repeated-selection-result")
	relation, ok := engine.DeclareRule(solver, result, relationSemantic("rule/repeated-selection-relation"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(fixture.app, 1)
	}, func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 1)
	})
	if !ok {
		t.Fatal("DeclareRule(repeated selected Relation)")
	}
	declareAt(t, solver, result, "rule/repeated-selection-selector", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		bind := func(resolver engine.Relation) bool {
			caller, valid := resolver.Caller(fixture.call)
			body, bodyOK := resolver.Selected(fixture.entry)
			return valid && bodyOK && resolver.Bind(body, caller)
		}
		return engine.Activate(access, relation, fixture.candidate, bind) && engine.Activate(access, relation, fixture.candidate, bind)
	})
	if _, ok := engine.DeclareQuery(solver, result, fixture.shard, fixture.call, 0); !ok {
		t.Fatal("DeclareQuery(repeated selection trigger)")
	}
	query, ok := engine.DeclareCandidateQuery(solver, result, fixture.candidate, fixture.shard, fixture.entry, 0)
	if !ok {
		t.Fatal("DeclareQuery(repeated selected Relation)")
	}
	if !solver.Seal() {
		t.Fatal("Solver.Seal rejected repeated selection")
	}
	state, solved := solver.Solve(context.Background(), nil)
	if !solved || state == nil {
		t.Fatal("repeated selection did not converge")
	}
	if value, present := query.Read(state); !present || value != 1 {
		t.Fatalf("repeated selected relation = %d/%t, want 1/true", value, present)
	}
}

// TestRelationCapabilityCannotRevive proves the callback cut even when a
// Rule retains the first resolver value and a later activation reuses the
// transaction's private frame. The old generation must have no authority to
// observe or manufacture a term in the later activation.
func TestRelationCapabilityCannotRevive(t *testing.T) {
	fixture := directCallFixtureFor(t)
	solver, err := engine.New(fixture.link)
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}
	result := relationFactor(t, solver, "expired-relation-result")
	relation, ok := engine.DeclareRule(solver, result, relationSemantic("rule/expired-relation"), func(binding *engine.RuleBinding) bool {
		return binding.Relation(fixture.app, 1)
	}, func(access engine.Access[uint64, uint8]) bool {
		return access.Set(0, 1)
	})
	if !ok {
		t.Fatal("DeclareRule(expired Relation)")
	}
	var retained engine.Relation
	var retainedRef engine.RelationRef
	declareAt(t, solver, result, "rule/expired-relation-selector", fixture.shard, fixture.call, func(access engine.Access[uint64, uint8]) bool {
		bind := func(resolver engine.Relation) bool {
			caller, valid := resolver.Caller(fixture.call)
			body, bodyOK := resolver.Selected(fixture.entry)
			return valid && bodyOK && resolver.Bind(body, caller)
		}
		prior, priorRef := retained, retainedRef
		if _, live := prior.Candidate(); live {
			return false
		}
		if _, live := prior.Within(priorRef, fixture.call); live {
			return false
		}
		return engine.Activate(access, relation, fixture.candidate, func(resolver engine.Relation) bool {
			caller, valid := resolver.Caller(fixture.call)
			body, bodyOK := resolver.Selected(fixture.entry)
			if !valid {
				return false
			}
			retained, retainedRef = resolver, caller
			return bodyOK && resolver.Bind(body, caller)
		}) && engine.Activate(access, relation, fixture.candidate, func(resolver engine.Relation) bool {
			if _, live := retained.Candidate(); live {
				return false
			}
			if _, live := retained.Application(); live {
				return false
			}
			if _, live := retained.Within(retainedRef, fixture.call); live {
				return false
			}
			return bind(resolver)
		})
	})
	if _, ok := engine.DeclareQuery(solver, result, fixture.shard, fixture.call, 0); !ok {
		t.Fatal("DeclareQuery(expired Relation trigger)")
	}
	query, ok := engine.DeclareCandidateQuery(solver, result, fixture.candidate, fixture.shard, fixture.entry, 0)
	if !ok {
		t.Fatal("DeclareQuery(expired Relation)")
	}
	if !solver.Seal() {
		t.Fatal("Solver.Seal rejected expired Relation law")
	}
	state, solved := solver.Solve(context.Background(), nil)
	if !solved || state == nil {
		t.Fatal("expired Relation law did not converge")
	}
	if value, present := query.Read(state); !present || value != 1 {
		t.Fatalf("expired Relation result = %d/%t, want 1/true", value, present)
	}
	if _, live := retained.Candidate(); live {
		t.Fatal("retained Relation revived after Solve")
	}
}
