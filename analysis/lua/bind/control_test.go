package bind

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/typeindex"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func bindControlSource(t *testing.T, source string) ([]ast.Stmt, *Result) {
	t.Helper()
	stmts, err := parse.ParseString(source, "control.lua")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return stmts, BindChunk(stmts, typeindex.Table{})
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

}

func TestBreakRequiresEnclosingLoopInCurrentFunction(t *testing.T) {
	t.Run("all loop forms are legal", func(t *testing.T) {
		_, result := bindControlSource(t, `
while true do break end
repeat break until true
for i = 1, 1 do break end
for _, value in pairs({}) do break end
`)
		requireControlKinds(t, result)
	})

	t.Run("top-level", func(t *testing.T) {
		stmts, result := bindControlSource(t, `break`)
		issue := requireControlKinds(t, result, ControlIssueBreakOutsideLoop)[0]
		if issue.Break != stmts[0].(*ast.BreakStmt) {
			t.Fatalf("Break issue = %#v, want top-level Break", issue)
		}
	})

	t.Run("nested function cannot use outer loop", func(t *testing.T) {
		stmts, result := bindControlSource(t, `
while true do
    local f = function() break end
end
`)
		outer := stmts[0].(*ast.WhileStmt)
		fn := outer.Stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
		issue := requireControlKinds(t, result, ControlIssueBreakOutsideLoop)[0]
		if issue.Break != fn.Stmts[0].(*ast.BreakStmt) {
			t.Fatalf("Break issue = %#v, want nested-function Break", issue)
		}
	})

}

func TestFunctionParamsAndLoopVariablesBelongToBodyLabelBaseline(t *testing.T) {
	t.Run("function parameter", func(t *testing.T) {
		stmts, result := bindControlSource(t, `local f = function(parameter) goto done ::done:: end`)
		fn := stmts[0].(*ast.LocalAssignStmt).Exprs[0].(*ast.FunctionExpr)
		jump := fn.Stmts[0].(*ast.GotoStmt)
		target := fn.Stmts[1].(*ast.LabelStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, jump, target)
	})

	t.Run("numeric loop variable", func(t *testing.T) {
		stmts, result := bindControlSource(t, `for iteration = 1, 2 do goto done ::done:: end`)
		loop := stmts[0].(*ast.NumberForStmt)
		jump := loop.Stmts[0].(*ast.GotoStmt)
		target := loop.Stmts[1].(*ast.LabelStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, jump, target)
	})

	t.Run("generic loop variables", func(t *testing.T) {
		stmts, result := bindControlSource(t, `for key, value in nil do goto done ::done:: end`)
		loop := stmts[0].(*ast.GenericForStmt)
		jump := loop.Stmts[0].(*ast.GotoStmt)
		target := loop.Stmts[1].(*ast.LabelStmt)
		requireControlKinds(t, result)
		requireGotoTarget(t, result, jump, target)
	})
}
