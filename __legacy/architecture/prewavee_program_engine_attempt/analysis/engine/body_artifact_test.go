package engine_test

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/program/lower"
	"github.com/wippyai/go-lua/analysis/program/target"
)

func TestEquationCacheConsumesPublicRuleSchema(t *testing.T) {
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	p, err := lower.Lower(lower.Source{Name: "equation-cache.lua", Text: []byte(`
local function child(value)
  return value
end
local payload = 7
child(payload)
`)})
	if err != nil {
		t.Fatal(err)
	}
	project, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	shard, ok := project.ShardAt(0)
	if !ok {
		t.Fatal("main shard")
	}
	function, ok := p.FunctionAt(0)
	if !ok {
		t.Fatal("child function")
	}
	_, body, _, ok := p.Function(function)
	if !ok {
		t.Fatal("child body")
	}
	candidate, ok := project.CandidateAt(0)
	if !ok {
		t.Fatal("child candidate")
	}
	if candidateShard, candidateBody, ok := project.CandidateBody(candidate); !ok || candidateShard != shard || candidateBody != body {
		t.Fatal("candidate body")
	}
	application, ok := project.CandidateApplication(candidate)
	if !ok {
		t.Fatal("candidate application")
	}
	applicationShard, call, ok := project.ApplicationOccurrence(application)
	if !ok || applicationShard != shard {
		t.Fatal("application occurrence")
	}
	_, actual, tail, _, nilFill, ok := project.CallFormalAt(candidate, 0)
	if !ok || actual == 0 || tail != 0 || nilFill {
		t.Fatal("formal actual")
	}
	entry, ok := p.BodyEntry(body)
	if !ok {
		t.Fatal("child entry")
	}
	var edge program.Edge
	for index := 0; ; index++ {
		candidateEdge, present := p.BodyEdgeAt(body, index)
		if !present {
			break
		}
		if candidateEdge.From() == entry && candidateEdge.To() != 0 {
			edge = candidateEdge
			break
		}
	}
	if edge.To() == 0 {
		t.Fatal("child entry edge")
	}
	semantic := func(id byte) engine.SemanticKey {
		return engine.SemanticKey{ID: program.ContentID{id}, Version: 1}
	}
	factorConfig := func(id byte) engine.FactorConfig[uint64, uint8] {
		return engine.FactorConfig[uint64, uint8]{
			Keys:     engine.KeySpace{End: 1},
			Semantic: semantic(id),
			Lattice: lattice.Lattice[uint8]{
				Bottom:   func() uint8 { return 0 },
				Top:      func() uint8 { return ^uint8(0) },
				Equal:    func(left, right uint8) bool { return left == right },
				LessOrEq: func(left, right uint8) bool { return left <= right },
				Join: func(left, right uint8) uint8 {
					if left > right {
						return left
					}
					return right
				},
				Widen: func(left, right uint8) uint8 {
					if left > right {
						return left
					}
					return right
				},
			},
			Fingerprint: func(value uint8) uint64 { return uint64(value) },
			WidenRank: engine.Measure[uint64, uint8]{
				Width: 1,
				At: func(_ uint64, value uint8, _ int) uint64 {
					return uint64(^uint8(0) - value)
				},
			},
		}
	}
	build := func(caches ...artifact.EquationCache) (*engine.Solver, *engine.Query[uint64, uint8]) {
		solver, err := engine.New(project, caches...)
		if err != nil {
			t.Fatal(err)
		}
		input, ok := engine.DeclareFactor(solver, factorConfig(1))
		if !ok {
			t.Fatal("input factor")
		}
		selector, ok := engine.DeclareFactor(solver, factorConfig(2))
		if !ok {
			t.Fatal("selector factor")
		}
		transfer, ok := engine.DeclareFactor(solver, factorConfig(3))
		if !ok {
			t.Fatal("transfer factor")
		}
		result, ok := engine.DeclareFactor(solver, factorConfig(4))
		if !ok {
			t.Fatal("result factor")
		}

		var left, right engine.ReadRef[uint64, uint8]
		relation, ok := engine.DeclareRule(solver, transfer, semantic(13), func(binding *engine.RuleBinding) bool {
			return binding.Relation(application, 2)
		}, func(access engine.Access[uint64, uint8]) bool {
			leftValue, leftPresent, leftOK := engine.ReadAt(access, left, 0)
			rightValue, rightPresent, rightOK := engine.ReadAt(access, right, 0)
			return leftOK && rightOK && leftPresent && rightPresent && access.Set(0, leftValue+rightValue)
		})
		if !ok {
			t.Fatal("relation engine.Rule")
		}
		if !engine.WriteExact(relation, 0) {
			t.Fatal("relation exact write")
		}
		if left, ok = engine.ReadExact(relation, 0, input, 0); !ok {
			t.Fatal("relation left read")
		}
		if right, ok = engine.ReadExact(relation, 1, input, 0); !ok {
			t.Fatal("relation right read")
		}
		if _, ok := engine.DeclareRule(solver, input, semantic(11), func(binding *engine.RuleBinding) bool {
			return binding.At(shard, actual)
		}, func(access engine.Access[uint64, uint8]) bool {
			return access.Set(0, 7)
		}); !ok {
			t.Fatal("At engine.Rule")
		}
		if _, ok := engine.DeclareRule(solver, selector, semantic(12), func(binding *engine.RuleBinding) bool {
			return binding.At(shard, call)
		}, func(access engine.Access[uint64, uint8]) bool {
			return engine.Activate(access, relation, candidate, func(resolver engine.Relation) bool {
				caller, ok := resolver.Caller(actual)
				if !ok {
					return false
				}
				output, ok := resolver.Selected(entry)
				return ok && resolver.Bind(output, caller, caller)
			})
		}); !ok {
			t.Fatal("selector At engine.Rule")
		}
		var carried engine.ReadRef[uint64, uint8]
		fromRule, ok := engine.DeclareRule(solver, result, semantic(14), func(binding *engine.RuleBinding) bool {
			return binding.From(shard, edge)
		}, func(access engine.Access[uint64, uint8]) bool {
			value, present, valid := engine.ReadAt(access, carried, 0)
			return valid && present && access.Set(0, value+1)
		})
		if !ok {
			t.Fatal("From engine.Rule")
		}
		if carried, ok = engine.Read(fromRule, 0, transfer); !ok {
			t.Fatal("From engine.Rule read")
		}
		query, ok := engine.DeclareCandidateQuery(solver, result, candidate, shard, edge.To(), 0)
		if !ok {
			t.Fatal("result query")
		}
		if _, ok := engine.DeclareQuery(solver, selector, shard, call, 0); !ok {
			t.Fatal("selector query")
		}
		if !solver.Seal() {
			t.Fatal("engine.Solver.Seal")
		}
		return solver, query
	}
	assertResult := func(solver *engine.Solver, query *engine.Query[uint64, uint8]) {
		state, ok := solver.Solve(context.Background(), nil)
		if !ok || state == nil {
			t.Fatal("Solve")
		}
		if value, present := query.Read(state); !present || value != 15 {
			t.Fatalf("result = %d/%t, want 15/true", value, present)
		}
	}

	producer, producerQuery := build()
	assertResult(producer, producerQuery)
	cache, ok := producer.EquationCache(shard)
	if !ok {
		t.Fatal("EquationCache")
	}
	relationIndex := -1
	for index, boundary := range cache.Boundary {
		if boundary.Rule == artifact.SemanticKey(semantic(13)) {
			relationIndex = index
			break
		}
	}
	if relationIndex < 0 || cache.Boundary[relationIndex].At != call || len(cache.Boundary[relationIndex].Reads) != 2 ||
		!cache.Boundary[relationIndex].Reads[0].Exact || !cache.Boundary[relationIndex].Reads[1].Exact ||
		cache.Boundary[relationIndex].Reads[0].Key != 0 || cache.Boundary[relationIndex].Reads[1].Key != 0 ||
		len(cache.Boundary[relationIndex].Writes) != 1 || cache.Boundary[relationIndex].Writes[0] != 0 {
		t.Fatal("engine.Relation boundary")
	}
	encoded, err := artifact.Encode(p, contract, artifact.Metadata{Equations: &cache})
	if err != nil {
		t.Fatal("encode exact Rule cache", err)
	}
	_, metadata, err := artifact.Decode(encoded, contract)
	if err != nil || metadata.Equations == nil {
		t.Fatalf("exact Rule cache round trip = %v/%#v", err, metadata.Equations)
	}
	roundTrip := -1
	for index, boundary := range metadata.Equations.Boundary {
		if boundary.Rule == artifact.SemanticKey(semantic(13)) {
			roundTrip = index
			break
		}
	}
	if roundTrip < 0 || len(metadata.Equations.Boundary[roundTrip].Reads) != 2 ||
		!metadata.Equations.Boundary[roundTrip].Reads[0].Exact || !metadata.Equations.Boundary[roundTrip].Reads[1].Exact ||
		metadata.Equations.Boundary[roundTrip].Reads[0].Key != 0 || metadata.Equations.Boundary[roundTrip].Reads[1].Key != 0 ||
		len(metadata.Equations.Boundary[roundTrip].Writes) != 1 || metadata.Equations.Boundary[roundTrip].Writes[0] != 0 {
		t.Fatal("exact Rule cache lost its direct-key signature on artifact round trip")
	}

	consumer, consumerQuery := build(cache)
	assertResult(consumer, consumerQuery)
	copyCache := func(source artifact.EquationCache) artifact.EquationCache {
		copy := source
		copy.Factors = append([]artifact.SemanticKey(nil), source.Factors...)
		copy.Rules = append([]artifact.SemanticKey(nil), source.Rules...)
		copy.Boundary = make([]artifact.EquationBoundary, len(source.Boundary))
		for index, boundary := range source.Boundary {
			copy.Boundary[index] = boundary
			copy.Boundary[index].Reads = append([]artifact.EquationRead(nil), boundary.Reads...)
			copy.Boundary[index].Writes = append([]uint64(nil), boundary.Writes...)
		}
		return copy
	}
	for _, test := range []struct {
		name   string
		mutate func(*artifact.EquationCache)
	}{
		{name: "output", mutate: func(cache *artifact.EquationCache) { cache.Boundary[relationIndex].Output.Version++ }},
		{name: "factor-version", mutate: func(cache *artifact.EquationCache) { cache.Factors[0].Version++ }},
		{name: "application-occurrence", mutate: func(cache *artifact.EquationCache) { cache.Boundary[relationIndex].At = actual }},
		{name: "activation", mutate: func(cache *artifact.EquationCache) { cache.Boundary[relationIndex].Activation = body }},
		{name: "arity", mutate: func(cache *artifact.EquationCache) { cache.Boundary[relationIndex].InputArity++ }},
		{name: "read-position", mutate: func(cache *artifact.EquationCache) { cache.Boundary[relationIndex].Reads[1].Position = 0 }},
		{name: "read-key", mutate: func(cache *artifact.EquationCache) { cache.Boundary[relationIndex].Reads[0].Key = 1 }},
		{name: "write-key", mutate: func(cache *artifact.EquationCache) { cache.Boundary[relationIndex].Writes[0] = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			mutated := copyCache(cache)
			test.mutate(&mutated)
			consumer, query := build(mutated)
			assertResult(consumer, query)
		})
	}
}
