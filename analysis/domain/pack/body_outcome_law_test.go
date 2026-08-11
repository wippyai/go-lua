package pack

import (
	"testing"

	program "github.com/wippyai/go-lua/program"
	flow "github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// TestBodyNormalOutcomeRootRoundTripsThroughOneDescriptorCoordinate keeps the
// Body.normal representation honest: it is an outcome-descriptor coordinate,
// while Outcome.Root is the only root projection. First/last bodies exercise
// the dense endpoints; foreign and malformed capabilities must fail closed.
func TestBodyNormalOutcomeRootRoundTripsThroughOneDescriptorCoordinate(t *testing.T) {
	_, linked, statics := sealCallLaw(t, `
local function first() return 1 end
local function last() return 2 end
first()
last()
`)
	schema, ok := Seal(linked, statics)
	if !ok || schema == nil {
		t.Fatal("Pack schema")
	}
	mounts := linked.Project().Mounts()
	type bodyAt struct {
		shard linkproject.Shard
		body  Body
	}
	var descriptors []bodyAt
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		program, programOK := mounts.Program(shard)
		if !shardOK || !programOK || program == nil {
			continue
		}
		identity := program.Source().Identity()
		for ordinal := 1; ordinal <= identity.FamilyCount(keyspace.FamilyBody); ordinal++ {
			term := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
			if !program.Flow().Executable().Contains(term) {
				continue
			}
			body, bodyOK := schema.Body(shard, term)
			if !bodyOK {
				t.Fatalf("Body(%v,%v)", shard, term)
			}
			descriptors = append(descriptors, bodyAt{shard: shard, body: body})
		}
	}
	if len(descriptors) < 2 {
		t.Fatalf("body descriptors = %d, want first and last", len(descriptors))
	}
	for _, candidate := range []bodyAt{descriptors[0], descriptors[len(descriptors)-1]} {
		body := candidate.body
		term, termOK := body.Term()
		normal, normalOK := body.Normal()
		outcomeTerm, outcomeTermOK := normal.Term()
		root, rootOK := normal.Root()
		roundTrip, roundTripOK := schema.OutcomeRoot(candidate.shard, outcomeTerm)
		bodyRoundTrip, bodyRoundTripOK := schema.Body(candidate.shard, term)
		if !termOK || !normalOK || !outcomeTermOK || !rootOK || !roundTripOK || !bodyRoundTripOK || root != roundTrip || !bodyRoundTrip.Same(body) {
			t.Fatalf("Body normal round trip: body=%v term=%v/%v normal=%v/%v outcome=%v/%v root=%v/%v round=%v/%v bodyRound=%v/%v", body, term, termOK, normal, normalOK, outcomeTerm, outcomeTermOK, root, rootOK, roundTrip, roundTripOK, bodyRoundTrip, bodyRoundTripOK)
		}
	}

	foreignSchema, foreignOK := Seal(linked, statics)
	if !foreignOK || foreignSchema == nil {
		t.Fatal("foreign Pack schema")
	}
	foreignShard, _ := mounts.At(0)
	foreignBody, foreignBodyOK := foreignSchema.Body(foreignShard, keyspace.MakeTerm(keyspace.FamilyBody, 1))
	foreignRoot, foreignRootOK := foreignBody.Root()
	if !foreignBodyOK || !foreignRootOK || foreignRoot == (Root{}) {
		t.Fatal("foreign Body root")
	}
	if _, ok := schema.RootID(foreignRoot); ok {
		t.Fatal("foreign Body root crossed schema owner")
	}

	malformedBody := descriptors[0].body
	malformedBody.index = uint32(len(schema.state.bodies))
	if _, ok := malformedBody.Normal(); ok {
		t.Fatal("malformed Body issued normal Outcome")
	}
	malformedOutcome := Outcome{schema: schema.state, index: uint32(len(schema.state.outcomes))}
	if _, ok := malformedOutcome.Root(); ok {
		t.Fatal("malformed Outcome issued root")
	}
}

func TestBodyReturnRetainsOrderedBranchAlternativesBesideNormalFallthrough(t *testing.T) {
	_, linked, statics := sealCallLaw(t, `
local function branch(flag)
  if flag then
    return 1
  elseif not flag then
    return 2
  end
end
branch(true)
`)
	schema, ok := Seal(linked, statics)
	if !ok || schema == nil {
		t.Fatal("Pack schema")
	}
	mounts := linked.Project().Mounts()
	var branchFlow flow.View
	var branchShard linkproject.Shard
	var branchProgram *program.Program
	for mountIndex := 0; mountIndex < mounts.Count(); mountIndex++ {
		shard, shardOK := mounts.At(mountIndex)
		candidate, programOK := mounts.Program(shard)
		if shardOK && programOK && candidate != nil {
			branchShard, branchProgram, branchFlow = shard, candidate, candidate.Flow()
			break
		}
	}
	if branchProgram == nil {
		t.Fatal("branch Program")
	}
	returns := branchFlow.Authored().Control().Returns()
	if returns.Count() != 2 {
		t.Fatalf("branch Return rows = %d, want 2", returns.Count())
	}
	var aggregate keyspace.Term
	for index := 0; index < returns.Count(); index++ {
		returnTerm, returnOK := returns.At(index)
		_, _, rowOK := returns.Get(returnTerm)
		direct, directOK := branchFlow.Outcomes().ReturnExit(returnTerm)
		directInfo, directInfoOK := branchFlow.Outcomes().Get(direct)
		directOutcome, directOutcomeOK := schema.Outcome(branchShard, direct)
		if !returnOK || !rowOK || !directOK || !directInfoOK || !directOutcomeOK || directInfo.Kind != flowkind.OutcomeReturn || directOutcome.ValuesCount() != 1 {
			t.Fatalf("branch direct Return[%d] = %v/%v outcome=%v/%v values=%d", index, returnTerm, returnOK, direct, directOK, directOutcome.ValuesCount())
		}
		terminal := direct
		seen := make(map[keyspace.Term]struct{})
		for steps := 0; ; steps++ {
			if steps >= branchFlow.Outcomes().Count() {
				t.Fatalf("branch direct Return[%d] propagation exceeds Outcome bound", index)
			}
			if _, duplicate := seen[terminal]; duplicate {
				t.Fatalf("branch direct Return[%d] propagation cycle at %v", index, terminal)
			}
			seen[terminal] = struct{}{}
			next, propagated := branchFlow.Outcomes().Propagation(terminal)
			if !propagated {
				break
			}
			terminal = next
		}
		terminalInfo, terminalInfoOK := branchFlow.Outcomes().Get(terminal)
		directActivation, directActivationOK := branchFlow.Activation().For(directInfo.Body)
		terminalActivation, terminalActivationOK := branchFlow.Activation().For(terminalInfo.Body)
		nextTerminal, terminalPropagated := branchFlow.Outcomes().Propagation(terminal)
		if !terminalInfoOK || terminalInfo.Kind != flowkind.OutcomeReturn || !directActivationOK || !terminalActivationOK || directActivation != terminalActivation || terminalPropagated {
			t.Fatalf("branch direct Return[%d] terminal = %v info=%v/%v activation=%v/%v terminalNext=%v/%v", index, terminal, terminalInfo, terminalInfoOK, directActivation, terminalActivation, nextTerminal, terminalPropagated)
		}
		if aggregate == 0 {
			aggregate = terminal
		} else if aggregate != terminal {
			t.Fatalf("branch Return[%d] aggregate = %v, want %v", index, terminal, aggregate)
		}
	}
	aggregateOutcome, aggregateOK := schema.Outcome(branchShard, aggregate)
	aggregateInfo, aggregateInfoOK := branchFlow.Outcomes().Get(aggregate)
	aggregateBody, aggregateBodyOK := schema.Body(branchShard, aggregateInfo.Body)
	normal, normalOK := aggregateBody.Normal()
	returned, returnedOK := aggregateBody.Return()
	if !aggregateOK || !aggregateInfoOK || !aggregateBodyOK || !normalOK || !returnedOK || aggregateOutcome.Kind() != flowkind.OutcomeReturn || aggregateOutcome.ValuesCount() != 2 || !returned.Same(aggregateOutcome) || normal.Kind() != flowkind.OutcomeNormal {
		t.Fatalf("branch aggregate Return = %v/%v values=%d, want 2", aggregate, aggregateOK, aggregateOutcome.ValuesCount())
	}
	first, firstOK := returned.ValuesTermAt(0)
	last, lastOK := returned.ValuesTermAt(returned.ValuesCount() - 1)
	if !firstOK || !lastOK || first == last {
		t.Fatal("ordered explicit return Values alternatives collapsed")
	}
}
