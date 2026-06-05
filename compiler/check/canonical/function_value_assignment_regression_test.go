package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/domain/observation"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

func TestCanonicalFunctionValueAssignmentUsesSolvedPathCallable(t *testing.T) {
	src := `
type Res = { answer: string }

local M = {
	dep = {
		get = function()
			return nil
		end,
	},
}

function M.run()
	return M.dep.get()
end

M.dep = {
	get = function()
		return { answer = "ok" }
	end,
}

local f: fun(): Res = M.run
local res = f()
local answer: string = res.answer
return answer
`
	res := testutil.Check(src, testutil.WithStdlib())
	root := res.Session.RootResult
	if root == nil || root.Graph == nil {
		t.Fatal("missing canonical root result")
	}
	mSym := singleSymbolNamed(t, root.Graph, "M")
	assignPoint, targetSym, source := assignmentSourceForTarget(t, root.Graph, "f")
	path := constraint.NewPath(mSym, "M").Field("run")

	pathFacts, ok := root.Facts.(flow.PathFacts)
	if !ok {
		t.Fatal("canonical facts do not expose path facts")
	}
	tv := pathFacts.RefinedPathAt(assignPoint, path)
	assertFunctionReturnsAnswerRecord(t, tv.Type, "RefinedPathAt(M.run)")

	got := observation.FromFuncResult(root, nil).WithProofValues().AssignmentSourceType(source, assignPoint, tv.Type, targetSym)
	assertFunctionReturnsAnswerRecord(t, got, "AssignmentSourceType(M.run)")

	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("expected clean canonical check after solved callable assignment source, got diagnostics: %v", msgs)
	}
}

func assertFunctionReturnsAnswerRecord(t *testing.T, got typ.Type, label string) {
	t.Helper()
	fn := unwrap.Function(got)
	if fn == nil {
		t.Fatalf("%s = %v, want function", label, typ.FormatShort(got))
	}
	if len(fn.Returns) != 1 {
		t.Fatalf("%s returns = %#v, want one return", label, fn.Returns)
	}
	rec, ok := unwrap.Alias(fn.Returns[0]).(*typ.Record)
	if !ok {
		t.Fatalf("%s return = %v, want Res record", label, typ.FormatShort(fn.Returns[0]))
	}
	answer := rec.GetField("answer")
	if answer == nil || (!typ.TypeEquals(answer.Type, typ.String) && !typ.MorePrecise(answer.Type, typ.String)) {
		t.Fatalf("%s return.answer = %#v, want string", label, answer)
	}
}
