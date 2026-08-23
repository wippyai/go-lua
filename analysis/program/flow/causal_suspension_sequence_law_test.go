package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestCausalSequenceConnectsPriorBindToExactYieldCall(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{Name: "causal-suspension-sequence.lua", Text: []byte(`
local function run()
    local captured = { value = 1 }
    coroutine.yield()
    return captured.value
end
local wrapped = coroutine.wrap(run)
wrapped()
return wrapped
`)})
	if err != nil {
		t.Fatal(err)
	}
	authored := program.Flow().Authored()
	var tableBind keyspace.Term
	for index := 0; index < authored.Storage().Binds().Count(); index++ {
		bind, bindOK := authored.Storage().Binds().At(index)
		_, values, rowOK := authored.Storage().Binds().Get(bind)
		length, lengthOK := authored.Values().Len(values)
		if !bindOK || !rowOK || !lengthOK {
			t.Fatalf("Bind %d is unavailable", index)
		}
		for memberIndex := 0; memberIndex < length; memberIndex++ {
			member, memberOK := authored.Values().Member(values, memberIndex)
			if !memberOK {
				t.Fatalf("Bind %v member %d is unavailable", bind, memberIndex)
			}
			if keyspace.TermFamily(member) == keyspace.FamilyTable {
				tableBind = bind
			}
		}
	}
	if tableBind == 0 {
		t.Fatal("fixture has no table-producing Bind")
	}
	bindOwner, _, _, bindOwnerOK := program.Source().Index().Position(tableBind)
	projection := program.Flow().SubjectFlow()
	var yieldCall keyspace.Term
	for index := 0; index < projection.BoundaryCount(); index++ {
		boundary, boundaryOK := projection.BoundaryAt(index)
		callOwner, _, _, callOwnerOK := program.Source().Index().Position(boundary.Call)
		if boundaryOK && bindOwnerOK && callOwnerOK && callOwner == bindOwner {
			if yieldCall != 0 {
				t.Fatal("fixture has more than one same-body suspension Call")
			}
			yieldCall = boundary.Call
		}
	}
	if yieldCall == 0 {
		t.Fatal("fixture has no same-body suspension Call")
	}

	causal := program.Flow().Causal()
	successors := make(map[keyspace.Term][]keyspace.Term)
	for index := 0; index < causal.Successors().TotalCount(); index++ {
		row, rowOK := causal.Successors().TotalAt(index)
		if !rowOK {
			t.Fatalf("Causal successor %d is unavailable", index)
		}
		successors[row.From] = append(successors[row.From], row.To)
	}
	seen := map[keyspace.Term]struct{}{tableBind: {}}
	queue := []keyspace.Term{tableBind}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range successors[current] {
			if _, visited := seen[next]; visited {
				continue
			}
			seen[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	if _, reachable := seen[yieldCall]; !reachable {
		t.Fatalf("table Bind %v has no canonical Causal path to yield Call %v", tableBind, yieldCall)
	}
}
