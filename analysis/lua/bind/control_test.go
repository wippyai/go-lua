package bind

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func bindControlSource(t *testing.T, source string) ([]ast.Stmt, *Result) {
	t.Helper()
	stmts, err := parse.ParseString(source, "control.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts, BindChunk(stmts, Options{})
}

func requireGotoTarget(
	t *testing.T,
	result *Result,
	jump *ast.GotoStmt,
	want *ast.LabelStmt,
) {
	t.Helper()
	got, ok := result.GotoTarget(jump)
	if !ok || got != want {
		t.Fatalf("GotoTarget(%q) = %p/%v, want %p/true", jump.Label, got, ok, want)
	}
}

func requireControlKinds(t *testing.T, result *Result, want ...ControlIssueKind) []ControlIssue {
	t.Helper()
	got := result.ControlIssues()
	if len(got) != len(want) {
		t.Fatalf("ControlIssues = %#v, want kinds %v", got, want)
	}
	for i, kind := range want {
		if got[i].Kind != kind {
			t.Fatalf("ControlIssues[%d].Kind = %v, want %v", i, got[i].Kind, kind)
		}
	}
	return got
}

func TestGotoTargetsAreExactForForwardBackwardAndAncestorLabels(t *testing.T) {
	stmts, result := bindControlSource(t, `
::back::
goto back
do
    goto forward
end
::forward::
`)
	back := stmts[0].(*ast.LabelStmt)
	backward := stmts[1].(*ast.GotoStmt)
	block := stmts[2].(*ast.DoBlockStmt)
	forward := block.Stmts[0].(*ast.GotoStmt)
	target := stmts[3].(*ast.LabelStmt)

	requireControlKinds(t, result)
	requireGotoTarget(t, result, backward, back)
	requireGotoTarget(t, result, forward, target)
}

func TestLabelVisibilityIsBlockExactAndDeclarationOrderSensitive(t *testing.T) {
	t.Run("outer cannot enter child", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
goto hidden
do
    ::hidden::
end
`)
		jump := stmts[0].(*ast.GotoStmt)
		issues := requireControlKinds(t, result, ControlIssueUndefinedLabel)
		if issues[0].Goto != jump {
			t.Fatalf("undefined issue Goto = %p, want %p", issues[0].Goto, jump)
		}
		if target, ok := result.GotoTarget(jump); ok || target != nil {
			t.Fatalf("invisible child target = %p/%v, want nil/false", target, ok)
		}
	})

	t.Run("siblings cannot target each other", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
do
    goto peer
end
do
    ::peer::
end
`)
		jump := stmts[0].(*ast.DoBlockStmt).Stmts[0].(*ast.GotoStmt)
		requireControlKinds(t, result, ControlIssueUndefinedLabel)
		if _, ok := result.GotoTarget(jump); ok {
			t.Fatal("sibling Goto acquired a target")
		}
	})

	t.Run("same name in siblings is legal", func(t *testing.T) {
		_, result := bindControlSource(t, `
do ::same:: goto same end
do ::same:: goto same end
`)
		requireControlKinds(t, result)
	})

	t.Run("earlier outer declaration forbids nested reuse", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
::same::
do
    ::same::
end
`)
		outer := stmts[0].(*ast.LabelStmt)
		inner := stmts[1].(*ast.DoBlockStmt).Stmts[0].(*ast.LabelStmt)
		issues := requireControlKinds(t, result, ControlIssueDuplicateLabel)
		if issues[0].Label != inner || issues[0].Previous != outer {
			t.Fatalf("duplicate issue = %#v, want inner against outer", issues[0])
		}
	})

	t.Run("completed inner declaration does not poison later outer label", func(t *testing.T) {
		_, result := bindControlSource(t, `
do
    ::same::
end
::same::
goto same
`)
		requireControlKinds(t, result)
	})
}

func TestGotoLocalScopeLawAndFinalLabelException(t *testing.T) {
	t.Run("forward jump cannot enter local", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
goto target
local entered = 1
::target::
entered = entered
`)
		jump := stmts[0].(*ast.GotoStmt)
		local := stmts[1].(*ast.LocalAssignStmt)
		target := stmts[2].(*ast.LabelStmt)
		issues := requireControlKinds(t, result, ControlIssueGotoEntersLocal)
		localID, _ := result.LocalSymbolAt(local, 0)
		if issues[0].Goto != jump || issues[0].Label != target ||
			issues[0].Local != localID || result.Name(issues[0].Local) != "entered" {
			t.Fatalf("scope issue = %#v, want target and entered local %d", issues[0], localID)
		}
		if _, ok := result.GotoTarget(jump); ok {
			t.Fatal("scope-invalid Goto acquired a target")
		}
	})

	t.Run("trailing labels are outside block locals", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
goto target
local skipped = 1
::target::
::also_final::
`)
		jump := stmts[0].(*ast.GotoStmt)
		target := stmts[2].(*ast.LabelStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, jump, target)
	})

	t.Run("backward jump before local re-enters declaration", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
::again::
local fresh = 1
goto again
`)
		target := stmts[0].(*ast.LabelStmt)
		jump := stmts[2].(*ast.GotoStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, jump, target)
	})

	t.Run("backward jump after local keeps it active", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
local retained = 1
::again::
retained = retained + 1
goto again
`)
		target := stmts[1].(*ast.LabelStmt)
		jump := stmts[3].(*ast.GotoStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, jump, target)
	})

	t.Run("leaving child locals is legal", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
::outer::
do
    local child = 1
    goto outer
end
`)
		target := stmts[0].(*ast.LabelStmt)
		jump := stmts[1].(*ast.DoBlockStmt).Stmts[1].(*ast.GotoStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, jump, target)
	})

	t.Run("child local count cannot mask later outer local", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
do
    local child = 1
    goto target
end
local entered = 1
::target::
entered = entered
`)
		block := stmts[0].(*ast.DoBlockStmt)
		jump := block.Stmts[1].(*ast.GotoStmt)
		local := stmts[1].(*ast.LocalAssignStmt)
		issues := requireControlKinds(t, result, ControlIssueGotoEntersLocal)
		localID, _ := result.LocalSymbolAt(local, 0)
		if issues[0].Goto != jump || issues[0].Local != localID {
			t.Fatalf("projected scope issue = %#v, want outer local %d", issues[0], localID)
		}
	})

	t.Run("repeat condition keeps trailing label inside body locals", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
repeat
    goto target
    local entered = true
    ::target::
until entered
`)
		repeat := stmts[0].(*ast.RepeatStmt)
		jump := repeat.Stmts[0].(*ast.GotoStmt)
		local := repeat.Stmts[1].(*ast.LocalAssignStmt)
		target := repeat.Stmts[2].(*ast.LabelStmt)
		localID, _ := result.LocalSymbolAt(local, 0)
		issues := requireControlKinds(t, result, ControlIssueGotoEntersLocal)
		if issues[0].Goto != jump || issues[0].Label != target || issues[0].Local != localID {
			t.Fatalf("repeat scope issue = %#v, want entered local %d", issues[0], localID)
		}
	})

	t.Run("repeat goto after local preserves condition scope", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
repeat
    local retained = true
    goto target
    ::target::
until retained
`)
		repeat := stmts[0].(*ast.RepeatStmt)
		jump := repeat.Stmts[1].(*ast.GotoStmt)
		target := repeat.Stmts[2].(*ast.LabelStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, jump, target)
	})
}

func TestLabelsAndGotosResetAtEveryFunctionBoundary(t *testing.T) {
	t.Run("nested function cannot target outer label", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
::outer::
local box = {
    function()
        goto outer
    end
}
`)
		table := stmts[1].(*ast.LocalAssignStmt).Exprs[0].(*ast.TableExpr)
		fn := table.Fields[0].Value.(*ast.FunctionExpr)
		jump := fn.Stmts[0].(*ast.GotoStmt)
		issues := requireControlKinds(t, result, ControlIssueUndefinedLabel)
		if issues[0].Goto != jump {
			t.Fatalf("function issue Goto = %p, want %p", issues[0].Goto, jump)
		}
	})

	t.Run("same name and target are independent in nested function", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
::same::
local box = {
    function()
        ::same::
        goto same
    end
}
goto same
`)
		outer := stmts[0].(*ast.LabelStmt)
		table := stmts[1].(*ast.LocalAssignStmt).Exprs[0].(*ast.TableExpr)
		fn := table.Fields[0].Value.(*ast.FunctionExpr)
		inner := fn.Stmts[0].(*ast.LabelStmt)
		innerJump := fn.Stmts[1].(*ast.GotoStmt)
		outerJump := stmts[2].(*ast.GotoStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, innerJump, inner)
		requireGotoTarget(t, result, outerJump, outer)
	})
}

func TestControlIssuesAreHonestOrderedAndCallerOwned(t *testing.T) {
	stmts, result := bindControlSource(t, `
goto entered
local x = 1
::entered::
x = x
goto missing
::dup::
::dup::
`)
	issues := requireControlKinds(
		t,
		result,
		ControlIssueGotoEntersLocal,
		ControlIssueUndefinedLabel,
		ControlIssueDuplicateLabel,
	)
	issues[0].Kind = 0
	requireControlKinds(
		t,
		result,
		ControlIssueGotoEntersLocal,
		ControlIssueUndefinedLabel,
		ControlIssueDuplicateLabel,
	)
	if _, ok := result.GotoTarget(stmts[0].(*ast.GotoStmt)); ok {
		t.Fatal("invalid first Goto acquired a target")
	}

	invalid := &ast.LabelStmt{}
	malformed := BindChunk([]ast.Stmt{invalid}, Options{})
	got := requireControlKinds(t, malformed, ControlIssueInvalidLabel)
	if got[0].Label != invalid {
		t.Fatalf("invalid Label issue = %#v", got[0])
	}

	invalidGoto := &ast.GotoStmt{}
	malformed = BindChunk([]ast.Stmt{invalidGoto}, Options{})
	got = requireControlKinds(t, malformed, ControlIssueInvalidGoto)
	if got[0].Goto != invalidGoto {
		t.Fatalf("invalid Goto issue = %#v", got[0])
	}

	jump := &ast.GotoStmt{Label: "target"}
	local := &ast.LocalAssignStmt{Names: []string{"entered"}}
	target := &ast.LabelStmt{Name: "target"}
	var nilLabel *ast.LabelStmt
	malformed = BindChunk([]ast.Stmt{jump, local, target, nilLabel}, Options{})
	got = requireControlKinds(t, malformed, ControlIssueGotoEntersLocal)
	if got[0].Goto != jump || got[0].Label != target {
		t.Fatalf("typed-nil trailing Label changed scope law: %#v", got[0])
	}
}

func TestFunctionParamsAndLoopVariablesBelongToBodyLabelBaseline(t *testing.T) {
	t.Run("function parameter", func(t *testing.T) {
		fn := &ast.FunctionExpr{
			ParList: &ast.ParList{Names: []string{"parameter"}},
			Stmts: []ast.Stmt{
				&ast.GotoStmt{Label: "done"},
				&ast.LabelStmt{Name: "done"},
			},
		}
		result := BindFunction(fn, Options{})
		jump := fn.Stmts[0].(*ast.GotoStmt)
		target := fn.Stmts[1].(*ast.LabelStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, jump, target)
	})

	t.Run("numeric loop variable", func(t *testing.T) {
		loop := &ast.NumberForStmt{
			Name: "iteration", Init: &ast.NumberExpr{Value: "1"},
			Limit: &ast.NumberExpr{Value: "2"},
			Stmts: []ast.Stmt{
				&ast.GotoStmt{Label: "done"},
				&ast.LabelStmt{Name: "done"},
			},
		}
		result := BindChunk([]ast.Stmt{loop}, Options{})
		jump := loop.Stmts[0].(*ast.GotoStmt)
		target := loop.Stmts[1].(*ast.LabelStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, jump, target)
	})

	t.Run("generic loop variables", func(t *testing.T) {
		loop := &ast.GenericForStmt{
			Names: []string{"key", "value"},
			Exprs: []ast.Expr{&ast.NilExpr{}},
			Stmts: []ast.Stmt{
				&ast.GotoStmt{Label: "done"},
				&ast.LabelStmt{Name: "done"},
			},
		}
		result := BindChunk([]ast.Stmt{loop}, Options{})
		jump := loop.Stmts[0].(*ast.GotoStmt)
		target := loop.Stmts[1].(*ast.LabelStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, jump, target)
	})
}

func TestControlBindingIsIterativeAtFourThousandDepthAndWidth(t *testing.T) {
	const size = 4 * 1024

	forward := &ast.GotoStmt{Label: "outer"}
	var nested ast.Stmt = forward
	for range size {
		nested = &ast.DoBlockStmt{Stmts: []ast.Stmt{nested}}
	}
	outer := &ast.LabelStmt{Name: "outer"}
	deep := BindChunk([]ast.Stmt{nested, outer}, Options{})
	requireControlKinds(t, deep)
	requireGotoTarget(t, deep, forward, outer)

	stmts := make([]ast.Stmt, 0, size*2)
	jumps := make([]*ast.GotoStmt, size)
	labels := make([]*ast.LabelStmt, size)
	for i := 0; i < size; i++ {
		name := fmt.Sprintf("label_%d", i)
		jumps[i] = &ast.GotoStmt{Label: name}
		labels[i] = &ast.LabelStmt{Name: name}
		stmts = append(stmts, jumps[i])
	}
	for _, label := range labels {
		stmts = append(stmts, label)
	}
	wide := BindChunk(stmts, Options{})
	requireControlKinds(t, wide)
	for _, index := range []int{0, size / 2, size - 1} {
		requireGotoTarget(t, wide, jumps[index], labels[index])
	}
}

func TestControlBindingAllocationGrowthIsLinear(t *testing.T) {
	build := func(size int) []ast.Stmt {
		stmts := make([]ast.Stmt, 0, size*2)
		for i := 0; i < size; i++ {
			name := fmt.Sprintf("L_%d", i)
			stmts = append(stmts, &ast.GotoStmt{Label: name}, &ast.LabelStmt{Name: name})
		}
		return stmts
	}
	measure := func(stmts []ast.Stmt) int64 {
		result := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = BindChunk(stmts, Options{})
			}
		})
		return result.AllocedBytesPerOp()
	}

	small := measure(build(1024))
	large := measure(build(2048))
	t.Logf("control binding allocations: 1K=%dB 2K=%dB", small, large)
	if large > small*3+64*1024 {
		t.Fatalf("control binding allocation growth is not linear: 1K=%dB 2K=%dB", small, large)
	}
}
