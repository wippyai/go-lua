package link

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// TestExecutableProjectionLaw keeps source identity universal while making
// every execution-shaped Link projection a direct image of the Flow executable view.
// The fixture contains both goto-dead and return-dead subtrees; the latter is
// nested in a live closure so it cannot be hidden by excluding that closure.
func TestExecutableProjectionLaw(t *testing.T) {
	p := source(t, `
type Token = number
local function uncalled() return Token({}) end
local function recursive(flag)
  if flag then return recursive(false) end
  return 0
end
local function invoke(selected) return selected() end
if unknown then
  local guarded = function() return {} end
  invoke(guarded)
end
local function return_dead()
  do return end
  local hidden = function()
    local value = 1
    value = value + 1
    local table = {}
    invoke(value)
    for i = 1, 2 do value = value + i end
    return Token(table)
  end
  hidden()
end
goto after
do
  local value = 1
  value = value + 1
  local table = {}
  local function hidden()
    invoke(value)
    for i = 1, 2 do value = value + i end
    return Token(table)
  end
  hidden()
end
::after::
return uncalled
`)
	contract := contract(t)
	l := linked(t, contract, linkproject.Module{Name: "main", Program: p})
	shard := onlyShard(t, l, p)
	projectShard := shard
	_, shardOK := l.Project().Mounts().Index(projectShard)
	authored := p.Flow().Authored()

	assertDeadFamily(t, authored.Values().Count(), authored.Values().At, p)
	assertDeadFamily(t, authored.Storage().Binds().Count(), authored.Storage().Binds().At, p)
	assertDeadFamily(t, authored.Storage().Assigns().Count(), authored.Storage().Assigns().At, p)
	assertDeadFamily(t, authored.Calls().Count(), authored.Calls().At, p)
	assertDeadFamily(t, authored.Tables().Count(), authored.Tables().At, p)
	assertDeadFamily(t, authored.Functions().Count(), authored.Functions().At, p)
	assertDeadFamily(t, authored.Control().Loops().Count(), authored.Control().Loops().At, p)
	assertDeadFamily(t, authored.TypeValues().Count(), authored.TypeValues().At, p)

	live, dead := 0, 0
	functions := authored.Functions()
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			t.Fatalf("FunctionAt(%d)", index)
		}
		_, present := l.Boundary().Values().Of(projectShard, function)
		if !shardOK || !present {
			t.Fatalf("Function %v lost universal Value identity", function)
		}
		if p.Flow().Executable().Contains(function) {
			live++
			continue
		}
		dead++
	}
	if live != 5 || dead < 2 {
		t.Fatalf("executable/dead Functions=%d/%d, want guarded + uncalled + recursive + return-dead live and nested closures dead", live, dead)
	}
	artifactAssertProjectionRoundTrip(t, l, contract, p)
}

func assertDeadFamily(t testing.TB, count int, at func(int) (keyspace.Term, bool), p *program.Program) {
	t.Helper()
	for index := 0; index < count; index++ {
		term, ok := at(index)
		if ok && !p.Flow().Executable().Contains(term) {
			return
		}
	}
	t.Fatalf("fixture has no source-dead occurrence in family")
}
