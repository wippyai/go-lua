package adapter

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/callresult"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestFunctionCanRunWithExactSuppliedKey(t *testing.T) {
	reg, markKey := adapterTestRegistry(t)
	want := adapterTestValue(reg, markKey, adapterMarkA)
	key := summary.SummaryKey{
		Ref:   ref.FuncRef{Kind: ref.KindSymbol, ID: 101},
		Entry: summary.EntryKey{Values: 1, Facts: 2, References: 3},
	}

	snap, err := query.Run(query.Config{
		Registry: reg,
		Functions: []query.Function{
			Function(key, adapterParseFunction(t, "function f() return 1 end"), check.Config{
				Registry: reg,
				ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
					if source.TargetIndex != 0 {
						return product.Value{}, false
					}
					return want, true
				},
			}),
		},
	})
	if err != nil {
		t.Fatalf("query.Run() error = %v", err)
	}

	assertSnapshotValue(t, reg, snap, key, want)
	if got, ok := snap.Read(summary.DefaultSummaryKey(key.Ref)); ok {
		t.Fatalf("Read(default key) = %#v, want missing exact key", got)
	}
}

func TestChunkCanRunWithExactSuppliedKey(t *testing.T) {
	reg, markKey := adapterTestRegistry(t)
	want := adapterTestValue(reg, markKey, adapterMarkA)
	key := summary.SummaryKey{
		Ref:   ref.FuncRef{Kind: ref.KindSymbol, ID: 102},
		Entry: summary.EntryKey{Values: 4, Facts: 5, References: 6},
	}

	snap, err := query.Run(query.Config{
		Registry: reg,
		Functions: []query.Function{
			Chunk(key, adapterParseChunk(t, "return 1"), check.Config{
				Registry: reg,
				ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
					if source.TargetIndex != 0 {
						return product.Value{}, false
					}
					return want, true
				},
			}),
		},
	})
	if err != nil {
		t.Fatalf("query.Run() error = %v", err)
	}

	assertSnapshotValue(t, reg, snap, key, want)
	if got, ok := snap.Read(summary.DefaultSummaryKey(key.Ref)); ok {
		t.Fatalf("Read(default key) = %#v, want missing exact key", got)
	}
}

func TestExpressionValueProviderAppearsInCanonicalSummary(t *testing.T) {
	reg, markKey := adapterTestRegistry(t)
	want := adapterTestValue(reg, markKey, adapterMarkB)
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 103})

	snap, err := query.Run(query.Config{
		Registry: reg,
		Functions: []query.Function{
			Chunk(key, adapterParseChunk(t, "return 1"), check.Config{
				Registry: reg,
				ExpressionValue: func(_ cfg.Point, _ factflow.ExprRef, source factflow.ValueSource, _ state.State) (product.Value, bool) {
					if source.TargetIndex != 0 {
						return product.Value{}, false
					}
					return want, true
				},
			}),
		},
	})
	if err != nil {
		t.Fatalf("query.Run() error = %v", err)
	}

	assertSnapshotValue(t, reg, snap, key, want)
}

func TestChunkWithCallResultsReturnCallSource(t *testing.T) {
	reg, markKey := adapterTestRegistry(t)
	want := adapterTestValue(reg, markKey, adapterMarkA)
	callerKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 201})
	calleeKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 202})

	snap, err := query.Run(query.Config{
		Registry: reg,
		Functions: []query.Function{
			{
				Key: calleeKey,
				Body: func(query.Context) (summary.Summary, error) {
					return summary.Summary{Returns: []product.Value{want}}, nil
				},
			},
			ChunkWithCallResults(callerKey, adapterParseChunk(t, "return callee()"), check.Config{
				Registry: reg,
				Globals:  []string{"callee"},
			}, adapterCalleeKey(calleeKey, factflow.CallProducerContextReturn)),
		},
	})
	if err != nil {
		t.Fatalf("query.Run() error = %v", err)
	}

	assertSnapshotValue(t, reg, snap, callerKey, want)
}

func TestChunkWithCallResultsAssignmentCallSource(t *testing.T) {
	reg, markKey := adapterTestRegistry(t)
	want := adapterTestValue(reg, markKey, adapterMarkB)
	callerKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 203})
	calleeKey := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 204})

	snap, err := query.Run(query.Config{
		Registry: reg,
		Functions: []query.Function{
			{
				Key: calleeKey,
				Body: func(query.Context) (summary.Summary, error) {
					return summary.Summary{Returns: []product.Value{want}}, nil
				},
			},
			ChunkWithCallResults(callerKey, adapterParseChunk(t, "local x = callee(); return x"), check.Config{
				Registry: reg,
				Globals:  []string{"callee"},
			}, adapterCalleeKey(calleeKey, factflow.CallProducerContextAssignment)),
		},
	})
	if err != nil {
		t.Fatalf("query.Run() error = %v", err)
	}

	assertSnapshotValue(t, reg, snap, callerKey, want)
}

func TestAdapterClonesMutableConfig(t *testing.T) {
	reg, markKey := adapterTestRegistry(t)
	want := adapterTestValue(reg, markKey, adapterMarkA)
	after := adapterTestValue(reg, markKey, adapterMarkB)
	expr := factflow.ExprRef(42)
	config := check.Config{
		Registry:         reg,
		Globals:          []string{"before"},
		ExpressionValues: map[factflow.ExprRef]product.Value{expr: want},
	}

	clone := cloneConfig(config)
	config.Globals[0] = "after"
	config.ExpressionValues[expr] = after

	if got := clone.Globals; len(got) != 1 || got[0] != "before" {
		t.Fatalf("clone.Globals = %#v, want [before]", got)
	}

	got, ok := clone.ExpressionValues[expr]
	if !ok {
		t.Fatalf("clone.ExpressionValues[%v] missing", expr)
	}
	if !product.Equal(reg, got, want) {
		t.Fatalf("clone.ExpressionValues[%v] = %v, want %v", expr, got, want)
	}
}

func TestCheckErrorsPropagateThroughQueryRun(t *testing.T) {
	reg := product.DefaultRegistry()
	key := summary.DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 105})

	_, err := query.Run(query.Config{
		Registry: reg,
		Functions: []query.Function{
			Chunk(key, adapterParseChunk(t, "return 1"), check.Config{}),
		},
	})
	if !errors.Is(err, check.ErrRegistryRequired) {
		t.Fatalf("query.Run(nil check registry) error = %v, want ErrRegistryRequired", err)
	}

	_, err = query.Run(query.Config{
		Registry: reg,
		Functions: []query.Function{
			Chunk(key, adapterParseChunk(t, "print(value())"), check.Config{Registry: reg}),
		},
	})
	if !errors.Is(err, check.ErrUnsupportedCFG) {
		t.Fatalf("query.Run(unsupported chunk) error = %v, want ErrUnsupportedCFG", err)
	}
}

func TestProductionImportsAreBounded(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{range .Imports}}{{.}}\n{{end}}", ".").Output()
	if err != nil {
		t.Fatalf("go list imports . error = %v", err)
	}
	allowed := map[string]bool{
		"maps":   true,
		"slices": true,
		"github.com/wippyai/go-lua/analysis/check":                     true,
		"github.com/wippyai/go-lua/analysis/check/fixpoint/callresult": true,
		"github.com/wippyai/go-lua/analysis/check/fixpoint/query":      true,
		"github.com/wippyai/go-lua/analysis/check/fixpoint/summary":    true,
		"github.com/wippyai/go-lua/compiler/ast":                       true,
	}
	for _, dep := range strings.Fields(string(out)) {
		if !allowed[dep] {
			t.Fatalf("unexpected production import %q", dep)
		}
	}

	forbidden := []string{
		"/__old",
		"/diagnostic",
		"/diagnostics",
		"/store",
		"/session",
		"/analysis/lua",
		"/cfgbuild",
		"/semantics",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, forbiddenPart := range forbidden {
			if strings.Contains(dep, forbiddenPart) {
				t.Fatalf("forbidden production import %q matched %q", dep, forbiddenPart)
			}
		}
	}
}

func adapterParseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "adapter_test.lua")
	if err != nil {
		t.Fatalf("ParseString(%q): %v", src, err)
	}
	return stmts
}

func adapterParseFunction(t *testing.T, src string) *ast.FunctionExpr {
	t.Helper()
	stmts := adapterParseChunk(t, src)
	if len(stmts) != 1 {
		t.Fatalf("got %d stmts, want 1", len(stmts))
	}
	def, ok := stmts[0].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt = %T, want function definition", stmts[0])
	}
	return def.Func
}

func adapterCalleeKey(key summary.SummaryKey, wantContext factflow.CallProducerContext) callresult.KeyFunc {
	return func(_ transfer.NodeContext, call factflow.CallProducer) (summary.SummaryKey, bool) {
		if call.CalleeSymbol() == 0 || call.Context() != wantContext {
			return summary.SummaryKey{}, false
		}
		return key, true
	}
}

func assertSnapshotValue(t *testing.T, reg *axis.Registry, snap summary.Snapshot, key summary.SummaryKey, want product.Value) {
	t.Helper()
	got, ok := snap.Read(key)
	if !ok {
		t.Fatalf("Read(key) missing")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("Read(key) returned %d slots, want 1", len(got.Returns))
	}
	if !product.Equal(reg, got.Returns[0], want) {
		t.Fatalf("Read(key) return = %v, want %v", got.Returns[0], want)
	}
}

type adapterMark uint8

const (
	adapterMarkBottom adapterMark = iota
	adapterMarkA
	adapterMarkB
	adapterMarkTop
)

func adapterTestRegistry(t *testing.T) (*axis.Registry, axis.Key[adapterMark]) {
	t.Helper()
	markKey := axis.NewKey[adapterMark]("adapter.test.mark." + strings.ReplaceAll(t.Name(), "/", "."))
	reg, err := product.DefaultRegistryWithAxes(axis.Spec[adapterMark]{
		Key:    markKey,
		Bottom: func() adapterMark { return adapterMarkBottom },
		Top:    func() adapterMark { return adapterMarkTop },
		Equal:  func(a, b adapterMark) bool { return a == b },
		LessOrEq: func(a, b adapterMark) bool {
			return a == b || a == adapterMarkBottom || b == adapterMarkTop
		},
		Join: func(a, b adapterMark) adapterMark {
			if a == b {
				return a
			}
			if a == adapterMarkBottom {
				return b
			}
			if b == adapterMarkBottom {
				return a
			}
			return adapterMarkTop
		},
		Meet: func(a, b adapterMark) adapterMark {
			if a == b {
				return a
			}
			if a == adapterMarkTop {
				return b
			}
			if b == adapterMarkTop {
				return a
			}
			return adapterMarkBottom
		},
		Widen: func(prev, next adapterMark) adapterMark {
			if prev == next {
				return prev
			}
			if prev == adapterMarkBottom {
				return next
			}
			return adapterMarkTop
		},
		Hash: func(v adapterMark) uint64 { return uint64(v) },
	}.Erase())
	if err != nil {
		t.Fatalf("DefaultRegistryWithAxes() error = %v", err)
	}
	return reg, markKey
}

func adapterTestValue(reg *axis.Registry, markKey axis.Key[adapterMark], mark adapterMark) product.Value {
	return product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), markKey, mark)
}
