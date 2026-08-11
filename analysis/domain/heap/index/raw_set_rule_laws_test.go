package index

import (
	"testing"

	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	heapowner "github.com/wippyai/go-lua/analysis/domain/heap/owner"
	"github.com/wippyai/go-lua/analysis/domain/pack"
	packowner "github.com/wippyai/go-lua/analysis/domain/pack/owner"
	"github.com/wippyai/go-lua/analysis/domain/static"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	valueowner "github.com/wippyai/go-lua/analysis/domain/value/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestDeclareRawSetBindsFixedRHSAndRejectsReadAccess(t *testing.T) {
	program, err := lower.Lower(lower.Source{
		Name: "raw_set_rule.lua",
		Text: []byte(`local id = {}; local tags = {}; tags["source"] = id; return tags`),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	heapSchema, heapOK := heapdomain.Seal(linked)
	valueSchema, valueOK := valuedomain.Seal(linked, heapSchema)
	callAlgebra, callOK := calldomain.New(linked)
	types, typesOK := typeauthority.Seal(linked)
	statics, _, staticErr := static.Seal(linked, types)
	packSchema, packOK := pack.Seal(linked, statics)
	topology, topologyOK := Seal(heapSchema, valueSchema, callAlgebra)
	if !heapOK || !valueOK || !callOK || !typesOK || staticErr != nil || !packOK || !topologyOK {
		t.Fatal("write rule schemas")
	}
	var writeAccess, readAccess Access
	for index := 0; index < heapSchema.IndexAccessCount(); index++ {
		candidate, candidateOK := heapSchema.IndexAccessAt(index)
		access, accessOK := topology.Access(candidate)
		if !candidateOK || !accessOK {
			t.Fatal("write rule access")
		}
		if access.Write() {
			writeAccess = access
		}
		if access.Read() {
			readAccess = access
		}
	}
	if !writeAccess.Write() {
		t.Fatal("write access")
	}
	composition := engine.NewComposition()
	values, valuesOK := valueowner.Declare(composition, rawSetKey(1), rawSetKey(2), valueSchema)
	heap, heapOK := heapowner.Declare(composition, rawSetKey(4), heapSchema)
	packs, packsOK := packowner.Declare(composition, rawSetKey(5), packSchema)
	rule, ruleOK := DeclareRawSet(composition, rawSetKey(6), rawSetKey(7), rawSetKey(8), topology, values, heap, packs)
	query, queryOK := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: rawSetKey(9), Project: func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{Semantic: rawSetKey(10), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value }, Equal: func(left, right bool) bool { return left == right }, Fingerprint: func(value bool) uint64 {
			if value {
				return 1
			}
			return 0
		}},
	}, func(query *engine.Query[bool]) bool {
		_, ok := engine.QueryReadFrom(query, values.ExactRead())
		return ok
	})
	if !valuesOK || !heapOK || !packsOK || !ruleOK || rule == nil || !queryOK || query == nil || !composition.Seal() {
		t.Fatal("write rule declaration")
	}
	if _, ok := rule.Instance(writeAccess); !ok {
		t.Fatal("RawSet rejected fixed RHS write access")
	}
	if readAccess != (Access{}) {
		if _, ok := rule.Instance(readAccess); ok {
			t.Fatal("RawSet accepted a read access")
		}
	}
}

func rawSetKey(value byte) engine.SemanticKey {
	var digest [32]byte
	digest[30], digest[31] = 0x73, value
	key, _ := engine.NewSemanticKey(digest, 1)
	return key
}
