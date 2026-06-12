package summary_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFromResultProjectsReturnSlotsFromExitState(t *testing.T) {
	reg, axisKey := projectTestRegistry(t)
	first := projectValue(reg, axisKey, projectMarkA)
	stmts := projectParseChunk(t, "return 1, nil")

	result := projectCheckChunk(t, stmts, check.Config{
		Registry: reg,
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
			switch source.TargetIndex {
			case 0:
				return first, true
			case 1:
				return product.Absent(reg), true
			default:
				return product.Value{}, false
			}
		},
	})
	got := summary.FromResult(result)

	if len(got.Returns) != 2 {
		t.Fatalf("FromResult returned %d slots, want 2", len(got.Returns))
	}
	projectAssertValue(t, reg, got.Returns[0], first)
	projectAssertValue(t, reg, got.Returns[1], product.Absent(reg))
}

func TestFromResultNoExplicitReturnProjectsEmptySummary(t *testing.T) {
	reg, _ := projectTestRegistry(t)
	result := projectCheckChunk(t, projectParseChunk(t, "local x = 1"), check.Config{Registry: reg})

	if got := summary.FromResult(result); len(got.Returns) != 0 {
		t.Fatalf("FromResult returned %#v, want empty summary", got)
	}
}

func TestFromResultUnresolvedReturnExpressionNormalizesBottomSlot(t *testing.T) {
	reg, _ := projectTestRegistry(t)
	result := projectCheckChunk(t, projectParseChunk(t, "return unknown"), check.Config{Registry: reg})

	if got := summary.FromResult(result); len(got.Returns) != 0 {
		t.Fatalf("FromResult returned %#v, want empty summary", got)
	}
}

func TestFromResultReadsJoinedExitReturnSlot(t *testing.T) {
	reg, axisKey := projectTestRegistry(t)
	branchA := projectValue(reg, axisKey, projectMarkA)
	branchB := projectValue(reg, axisKey, projectMarkB)
	joined := projectValue(reg, axisKey, projectMarkTop)
	values := []product.Value{branchA, branchB}
	byExpr := make(map[factflow.ExprRef]product.Value)
	stmts := projectParseChunk(t, "if c then return 1 else return 2 end")

	result := projectCheckChunk(t, stmts, check.Config{
		Registry: reg,
		Globals:  []string{"c"},
		ExpressionValue: func(_ cfg.Point, expr factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
			if source.TargetIndex != 0 {
				return product.Value{}, false
			}
			if value, ok := byExpr[expr]; ok {
				return value, true
			}
			if len(byExpr) >= len(values) {
				return product.Value{}, false
			}
			value := values[len(byExpr)]
			byExpr[expr] = value
			return value, true
		},
	})
	got := summary.FromResult(result)

	if len(got.Returns) != 1 {
		t.Fatalf("FromResult returned %d slots, want 1", len(got.Returns))
	}
	projectAssertValue(t, reg, got.Returns[0], joined)
	if product.Equal(reg, got.Returns[0], branchA) || product.Equal(reg, got.Returns[0], branchB) {
		t.Fatalf("FromResult returned an individual branch value, want joined exit value")
	}
}

func TestFromResultIgnoresDeadReturnFacts(t *testing.T) {
	reg, axisKey := projectTestRegistry(t)
	live := projectValue(reg, axisKey, projectMarkA)
	dead := projectValue(reg, axisKey, projectMarkB)
	values := []product.Value{live, dead}
	var seen int
	stmts := projectParseChunk(t, "do return 1 end\nreturn 2")

	result := projectCheckChunk(t, stmts, check.Config{
		Registry: reg,
		ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
			if source.TargetIndex != 0 || seen >= len(values) {
				return product.Value{}, false
			}
			value := values[seen]
			seen++
			return value, true
		},
	})
	if got := result.ReturnPoints(); len(got) != 1 {
		t.Fatalf("ReturnPoints returned %d points, want only the reachable return", len(got))
	}
	got := summary.FromResult(result)

	if len(got.Returns) != 1 {
		t.Fatalf("FromResult returned %d slots, want 1", len(got.Returns))
	}
	projectAssertValue(t, reg, got.Returns[0], live)
	if product.Equal(reg, got.Returns[0], dead) {
		t.Fatalf("FromResult used dead return value")
	}
}

func TestFromResultMissingReadModelReturnsEmptySummary(t *testing.T) {
	if got := summary.FromResult(nil); len(got.Returns) != 0 {
		t.Fatalf("FromResult(nil) returned %#v, want empty summary", got)
	}
}

type projectMark uint8

const (
	projectMarkBottom projectMark = iota
	projectMarkA
	projectMarkB
	projectMarkTop
)

func projectTestRegistry(t *testing.T) (*axis.Registry, axis.Key[projectMark]) {
	t.Helper()
	axisKey := axis.NewKey[projectMark]("summary.project.test." + strings.ReplaceAll(t.Name(), "/", "."))
	reg, err := standard.RegistryWithAxes(axis.Spec[projectMark]{
		Key:    axisKey,
		Bottom: func() projectMark { return projectMarkBottom },
		Top:    func() projectMark { return projectMarkTop },
		Equal:  func(a, b projectMark) bool { return a == b },
		LessOrEq: func(a, b projectMark) bool {
			return a == b || a == projectMarkBottom || b == projectMarkTop
		},
		Join: func(a, b projectMark) projectMark {
			if a == b {
				return a
			}
			if a == projectMarkBottom {
				return b
			}
			if b == projectMarkBottom {
				return a
			}
			return projectMarkTop
		},
		Meet: func(a, b projectMark) projectMark {
			if a == b {
				return a
			}
			if a == projectMarkTop {
				return b
			}
			if b == projectMarkTop {
				return a
			}
			return projectMarkBottom
		},
		Widen: func(prev, next projectMark) projectMark {
			if prev == next {
				return prev
			}
			if prev == projectMarkBottom {
				return next
			}
			return projectMarkTop
		},
		Hash: func(v projectMark) uint64 { return uint64(v) },
	}.Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes: %v", err)
	}
	return reg, axisKey
}

func projectValue(reg *axis.Registry, axisKey axis.Key[projectMark], mark projectMark) product.Value {
	return product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), axisKey, mark)
}

func projectCheckChunk(t *testing.T, stmts []ast.Stmt, config check.Config) *check.Result {
	t.Helper()
	result, err := check.CheckChunk(stmts, config)
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	return result
}

func projectParseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "summary_project_test.lua")
	if err != nil {
		t.Fatalf("ParseString(%q): %v", src, err)
	}
	return stmts
}

func projectAssertValue(t *testing.T, reg *axis.Registry, got, want product.Value) {
	t.Helper()
	if !product.Equal(reg, got, want) {
		t.Fatalf("value = %v, want %v", got, want)
	}
}
