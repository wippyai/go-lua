package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func controlSourceAt(t *testing.T, p *program.Program, body keyspace.Term, index int) keyspace.Term {
	t.Helper()
	term, ok := p.Source().Order().BodyAt(body, index)
	if !ok {
		t.Fatalf("Source Order BodyAt(%v, %d) is absent", body, index)
	}
	return term
}

func TestSourceControlKeepsBranchBodiesInSourceAndFlowOwners(t *testing.T) {
	p := parseBindLower(t, "\nif first() then\n  return 1\nelseif second() then\n  return 2\nelse\n  return 3\nend\n")
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry")
	}
	outer := controlSourceAt(t, p, entry, 0)
	branches := p.Flow().Authored().Control().Branches()
	owner, condition, whenTrue, whenFalse, branchOK := branches.Get(outer)
	if !branchOK || owner != entry || condition == 0 || whenTrue == 0 || whenFalse == 0 {
		t.Fatalf("Branch = owner %v condition %v true %v false %v ok %v", owner, condition, whenTrue, whenFalse, branchOK)
	}
	if parent, ok := p.Source().Index().BodyParent(whenTrue); !ok || parent != entry {
		t.Fatalf("truthy Body parent = %v/%v, want %v", parent, ok, entry)
	}
	inner := controlSourceAt(t, p, whenFalse, 0)
	innerOwner, innerCondition, innerTrue, innerFalse, innerOK := branches.Get(inner)
	if !innerOK || innerOwner != whenFalse || innerCondition == 0 || innerTrue == 0 || innerFalse == 0 {
		t.Fatalf("elseif Branch = owner %v condition %v true %v false %v ok %v", innerOwner, innerCondition, innerTrue, innerFalse, innerOK)
	}
	if parent, ok := p.Source().Index().BodyParent(innerTrue); !ok || parent != whenFalse {
		t.Fatalf("elseif truthy Body parent = %v/%v, want %v", parent, ok, whenFalse)
	}
}

func TestFlowControlRowsCoverEveryLuaLoopForm(t *testing.T) {
	for _, sample := range []struct {
		name  string
		input string
		kind  kind.LoopKind
		cells int
	}{
		{"while", "while test() do local value = 1 end", kind.LoopWhile, 0},
		{"repeat", "repeat local value = 1 until test()", kind.LoopRepeat, 0},
		{"numeric", "for i = 1, 2, 1 do local value = i end", kind.LoopNumericFor, 1},
		{"generic", "for key, value in iterate() do local seen = key end", kind.LoopGenericFor, 2},
	} {
		t.Run(sample.name, func(t *testing.T) {
			p := parseBindLower(t, sample.input)
			loops := p.Flow().Authored().Control().Loops()
			loop, ok := loops.At(0)
			if !ok {
				t.Fatal("missing Loop")
			}
			owner, body, loopKind, control, rowOK := loops.Get(loop)
			if !rowOK || owner == 0 || body == 0 || control == 0 || loopKind != sample.kind {
				t.Fatalf("Loop = owner %v body %v kind %v control %v ok %v", owner, body, loopKind, control, rowOK)
			}
			if sample.cells != 0 {
				if count, ok := loops.CellCount(loop); !ok || count != sample.cells {
					t.Fatalf("Loop CellCount = %d/%v, want %d", count, ok, sample.cells)
				}
			}
			normal, normalOK := p.Flow().Outcomes().BodyExit(body, kind.OutcomeNormal)
			if !normalOK || normal == 0 {
				t.Fatal("Loop Body has no normal Outcome")
			}
			found := false
			edges := p.Flow().Causal().Edges()
			for index := 0; index < edges.Count(); index++ {
				edge, edgeOK := edges.At(index)
				found = found || edgeOK && edge.Mu == loop
			}
			if !found {
				t.Fatal("Loop has no final recurrence Edge")
			}
		})
	}
}

func TestFlowControlOutcomesKeepBreakAndGotoTyped(t *testing.T) {
	p := parseBindLower(t, "\n::again::\nwhile test() do break end\ngoto again\n")
	control := p.Flow().Authored().Control()
	loop, loopOK := control.Loops().At(0)
	breakTerm, breakOK := control.Breaks().At(0)
	jump, gotoOK := control.Gotos().At(0)
	label, labelOK := control.Labels().At(0)
	if !loopOK || !breakOK || !gotoOK || !labelOK {
		t.Fatalf("control rows loop=%v/%v break=%v/%v goto=%v/%v label=%v/%v", loop, loopOK, breakTerm, breakOK, jump, gotoOK, label, labelOK)
	}
	if target, ok := control.Breaks().Get(breakTerm); !ok || target != loop {
		t.Fatalf("Break target = %v/%v, want Loop %v", target, ok, loop)
	}
	if _, target, ok := control.Gotos().Get(jump); !ok || target != label {
		t.Fatalf("Goto target = %v/%v, want Label %v", target, ok, label)
	}
	breakExit, breakExitOK := p.Flow().Outcomes().BreakExit(breakTerm)
	jumpExit, jumpExitOK := p.Flow().Outcomes().GotoExit(jump)
	if !breakExitOK || !jumpExitOK || breakExit == 0 || jumpExit == 0 {
		t.Fatalf("control exits break=%v/%v goto=%v/%v", breakExit, breakExitOK, jumpExit, jumpExitOK)
	}
}

func TestFlowDirectFunctionsFilterDeadCallsWithoutRootDecisionPlane(t *testing.T) {
	dead := parseBindLower(t, "local function f() goto done; f(); ::done:: end")
	function, functionOK := dead.Flow().Authored().Functions().At(0)
	call, callOK := dead.Flow().Authored().Calls().At(0)
	if !functionOK || !callOK {
		t.Fatalf("dead Function/Call = %v/%v %v/%v", function, functionOK, call, callOK)
	}
	if direct, ok := dead.Flow().DirectFunctions().Call(call); ok || direct != 0 {
		t.Fatalf("dead Call direct Function = %v/%v, want absent", direct, ok)
	}
	if dead.Flow().Executable().Contains(call) {
		t.Fatal("dead Call became executable")
	}

	live := parseBindLower(t, "local function f() return f() end")
	function, functionOK = live.Flow().Authored().Functions().At(0)
	call, callOK = live.Flow().Authored().Calls().At(0)
	if !functionOK || !callOK {
		t.Fatalf("live Function/Call = %v/%v %v/%v", function, functionOK, call, callOK)
	}
	if direct, ok := live.Flow().DirectFunctions().Call(call); !ok || direct != function {
		t.Fatalf("live Call direct Function = %v/%v, want %v", direct, ok, function)
	}
}

func TestSourceControlFaultsStaySourceOwnedStaticEvidence(t *testing.T) {
	p := parseBindLower(t, "type Snapshot = typeof(function() goto missing end)")
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing static function Body")
	}
	fault := controlSourceAt(t, p, entry, 0)
	got, faultOK := p.Source().Faults().At(fault)
	if !faultOK || got.Owner != entry || got.Kind != source.ControlFaultUndefinedGoto {
		t.Fatalf("Source ControlFault = %#v/%v", got, faultOK)
	}
	if !p.Flow().Containment().Static(fault) {
		t.Fatal("static ControlFault escaped static containment")
	}
	if got := p.Flow().Authored().Control().Gotos().Count(); got != 0 {
		t.Fatalf("static invalid Goto became executable: %d", got)
	}
}
