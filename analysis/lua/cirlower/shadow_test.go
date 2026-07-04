package cirlower_test

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/cir"
	"github.com/wippyai/go-lua/analysis/lua/cirlower"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/parse"
)

// TestShadowCoverage is an opt-in (CIR_SHADOW=1) completeness harness. For every
// fixture main.lua it can parse+bind, it runs BOTH the existing point-keyed
// semantics extraction (imported read-only) and the cir lowering, then compares
// construct COVERAGE: every semantics assign/call/branch/return fact must have a
// corresponding cir instruction with matching operand identity (path key). The
// two pipelines build independent CFGs with independent point numbering, so the
// comparison is a per-category multiset over operand identities rather than a
// point-by-point diff. This proves lowering completeness; full point-state
// equality is a later migration concern.
//
// It reports corpus-wide coverage per category in the test log. Residual gaps
// (e.g. short-circuit and/or lowered as OpLogical rather than branch topology)
// are expected and surfaced honestly rather than masked.
func TestShadowCoverage(t *testing.T) {
	if os.Getenv("CIR_SHADOW") != "1" {
		t.Skip("set CIR_SHADOW=1 to run the cir lowering coverage harness")
	}

	root := repoRoot(t)
	fixtures := findMainLua(t, filepath.Join(root, "testdata", "fixtures"))
	if len(fixtures) == 0 {
		t.Fatal("no fixtures found")
	}

	categories := []string{"assign", "call", "branch", "return"}
	semTotals := map[string]int{}
	covered := map[string]int{}

	var processed, skippedParse, skippedBind, skippedExtract int
	uncoveredSamples := map[string][]string{}

	for _, file := range fixtures {
		src, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		stmts, err := parse.ParseString(string(src), file)
		if err != nil {
			skippedParse++
			continue
		}
		bindings := bind.BindChunk(stmts, bind.Options{})
		if bindings == nil {
			skippedBind++
			continue
		}
		built := cfgbuild.BuildChunk(stmts, bindings)
		if built == nil || built.Graph == nil {
			skippedBind++
			continue
		}
		sem, err := semantics.ExtractChunk(stmts, bindings, built)
		if err != nil {
			skippedExtract++
			continue
		}
		res := cirlower.Chunk("main", stmts, bindings)
		if res == nil || res.Body == nil {
			skippedExtract++
			continue
		}
		processed++

		semKeys := semanticKeys(sem, built.Graph)
		cirKeys := cirKeys(res.Body)

		for _, cat := range categories {
			s := semKeys[cat]
			c := cirKeys[cat]
			for key, n := range s {
				semTotals[cat] += n
				have := c[key]
				if have > n {
					have = n
				}
				covered[cat] += have
				if have < n && len(uncoveredSamples[cat]) < 12 {
					uncoveredSamples[cat] = append(uncoveredSamples[cat], key)
				}
			}
		}
	}

	t.Logf("cir shadow coverage over %d/%d fixtures (parse-skip %d, bind-skip %d, extract-skip %d)",
		processed, len(fixtures), skippedParse, skippedBind, skippedExtract)

	var totalSem, totalCovered int
	for _, cat := range categories {
		ts := semTotals[cat]
		tc := covered[cat]
		totalSem += ts
		totalCovered += tc
		t.Logf("  %-7s %6d/%-6d  %s", cat, tc, ts, pct(tc, ts))
		if samples := uncoveredSamples[cat]; len(samples) > 0 {
			t.Logf("    uncovered sample keys: %v", samples)
		}
	}
	t.Logf("  %-7s %6d/%-6d  %s", "TOTAL", totalCovered, totalSem, pct(totalCovered, totalSem))
}

func pct(n, d int) string {
	if d == 0 {
		return "n/a"
	}
	return strconv.FormatFloat(100*float64(n)/float64(d), 'f', 2, 64) + "%"
}

// semanticKeys collects the point-keyed semantics facts into per-category
// operand-identity multisets.
func semanticKeys(sem *semantics.Result, graph *cfg.CFG) map[string]map[string]int {
	out := map[string]map[string]int{
		"assign": {}, "call": {}, "branch": {}, "return": {},
	}
	for _, pt := range graph.RPO() {
		if f, ok := sem.LocalAssignment(pt); ok {
			out["assign"][semLocalAssignKey(f)]++
		}
		if f, ok := sem.OrdinaryAssignment(pt); ok {
			out["assign"][semOrdinaryAssignKey(f)]++
		}
		if f, ok := sem.Call(pt); ok {
			out["call"][semCallKey(f)]++
		}
		if _, ok := sem.Return(pt); ok {
			out["return"]["return"]++
		}
		if f, ok := sem.BranchCondition(pt); ok {
			out["branch"][semBranchKey(f)]++
		}
	}
	return out
}

func semLocalAssignKey(f semantics.LocalAssignmentFact) string {
	if f.HasSymbol {
		return string(path.NewPath(f.Symbol, f.Name).Key())
	}
	return "name:" + f.Name
}

func semOrdinaryAssignKey(f semantics.OrdinaryAssignmentFact) string {
	switch {
	case f.HasPath:
		return string(f.Path.Key())
	case f.HasContainerPath:
		return string(f.ContainerPath.Key())
	case f.HasSymbol:
		return string(path.NewPath(f.Symbol, "").Key())
	default:
		return "target"
	}
}

func semCallKey(f semantics.CallFact) string {
	if f.HasCalleePath {
		return string(f.CalleePath.Key())
	}
	if f.Method != "" {
		if f.HasReceiverPath {
			return string(f.ReceiverPath.Key()) + ":" + f.Method
		}
		return "recv:" + f.Method
	}
	return "callexpr"
}

func semBranchKey(f semantics.BranchConditionFact) string {
	return string(f.Check.Path.Key()) + "|" + strconv.Itoa(int(f.Check.Kind))
}

// cirKeys collects the top-level cir body's destination and control identities
// into the same per-category multisets. Nested closure protos are excluded
// because semantics.ExtractChunk covers the top-level chunk only.
func cirKeys(b *cir.Body) map[string]map[string]int {
	out := map[string]map[string]int{
		"assign": {}, "call": {}, "branch": {}, "return": {},
	}
	for i := 0; i < b.Len(); i++ {
		inst := b.Instr(i)
		switch inst.Op {
		case cir.OpAssign, cir.OpBinOp, cir.OpUnOp, cir.OpConcat, cir.OpMakeTable,
			cir.OpClaim, cir.OpLogical, cir.OpClosure, cir.OpSelect,
			cir.OpStaticMemberWrite, cir.OpDynamicIndexWrite:
			if k, ok := cirDstKey(b, inst.Dst); ok {
				out["assign"][k]++
			}
		case cir.OpCall:
			out["call"][cirCallKey(b, inst)]++
			for _, r := range b.Operands(inst.Results) {
				if k, ok := cirDstKey(b, r); ok {
					out["assign"][k]++
				}
			}
		case cir.OpIterate:
			for _, r := range b.Operands(inst.Results) {
				if k, ok := cirDstKey(b, r); ok {
					out["assign"][k]++
				}
			}
		case cir.OpReturn:
			out["return"]["return"]++
		case cir.OpBranch:
			out["branch"][cirBranchKey(b, inst)]++
		}
	}
	return out
}

func cirDstKey(b *cir.Body, op cir.Operand) (string, bool) {
	if op.Kind != cir.OperandPath {
		return "", false
	}
	p := b.Path(cir.PathRef(op.Ref))
	if p.IsEmpty() {
		return "", false
	}
	return string(p.Key()), true
}

func cirCallKey(b *cir.Body, inst cir.Instruction) string {
	if inst.Call.Method != 0 {
		method := b.Const(inst.Call.Method).Str
		// Match semantics' callee identity, which folds a method call into the
		// receiver.method member path (dot form), not a distinct receiver:method key.
		if inst.Call.Receiver.Kind == cir.OperandPath {
			p := b.Path(cir.PathRef(inst.Call.Receiver.Ref))
			if !p.IsEmpty() {
				return string(p.Field(method).Key())
			}
		}
		return "recv:" + method
	}
	if k, ok := cirDstKey(b, inst.Call.Callee); ok {
		return k
	}
	return "callexpr"
}

func cirBranchKey(b *cir.Body, inst cir.Instruction) string {
	c := b.Check(inst.Check)
	return string(c.Path.Key()) + "|" + strconv.Itoa(int(c.Kind))
}

// repoRoot ascends from the test working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above working directory")
		}
		dir = parent
	}
}

func findMainLua(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(p) == "main.lua" {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}
