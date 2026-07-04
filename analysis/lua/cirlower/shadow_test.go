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
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/cirlower"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/parse"
)

// TestShadowCoverage is an opt-in (CIR_SHADOW=1) per-point completeness oracle.
// Because D1a lowers onto the SAME cfgbuild graph that semantics extracts from,
// the comparison is now a true per-point diff: for every point that carries a
// semantics fact (assign / call / branch / return, imported read-only), the cir
// Body must carry an instruction AT THAT POINT whose operand identity (path key)
// matches. It reports corpus-wide coverage per category and lists the residual
// gaps honestly rather than masking them.
//
// Known residual: a conservatively pure short-circuit `and`/`or` right operand
// keeps the OpLogical value form on the enclosing statement point, so the guard
// point cfgbuild materializes (which semantics reconstructs a branch fact for)
// carries no cir branch. These points surface as the branch-category gap.
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
	total := map[string]int{}
	covered := map[string]int{}
	gapSamples := map[string][]string{}

	var processed, skippedParse, skippedBind, skippedExtract int

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
		body := cirlower.Lower("main", stmts, bindings, built)
		if body == nil {
			skippedExtract++
			continue
		}
		processed++

		for _, pt := range built.Graph.RPO() {
			keys := cirPointKeys(body, pt)
			if f, ok := sem.LocalAssignment(pt); ok {
				scorePoint(total, covered, gapSamples, "assign", keys, semLocalAssignKey(f))
			}
			if f, ok := sem.OrdinaryAssignment(pt); ok {
				scorePoint(total, covered, gapSamples, "assign", keys, semOrdinaryAssignKey(f))
			}
			if f, ok := sem.Call(pt); ok {
				scorePoint(total, covered, gapSamples, "call", keys, semCallKey(f))
			}
			if _, ok := sem.Return(pt); ok {
				scorePoint(total, covered, gapSamples, "return", keys, "return")
			}
			if f, ok := sem.BranchCondition(pt); ok {
				scorePoint(total, covered, gapSamples, "branch", keys, semBranchKey(f))
			}
		}
	}

	t.Logf("cir per-point coverage over %d/%d fixtures (parse-skip %d, bind-skip %d, extract-skip %d)",
		processed, len(fixtures), skippedParse, skippedBind, skippedExtract)

	var totalAll, coveredAll int
	for _, cat := range categories {
		tc, cc := total[cat], covered[cat]
		totalAll += tc
		coveredAll += cc
		t.Logf("  %-7s %6d/%-6d  %s", cat, cc, tc, pct(cc, tc))
		if samples := gapSamples[cat]; len(samples) > 0 {
			t.Logf("    uncovered sample keys: %v", samples)
		}
	}
	t.Logf("  %-7s %6d/%-6d  %s", "TOTAL", coveredAll, totalAll, pct(coveredAll, totalAll))
}

// scorePoint records one semantics fact at a point and whether cir carries a
// matching instruction key in the same category at that same point.
func scorePoint(total, covered map[string]int, gaps map[string][]string, cat string, keys map[string]map[string]bool, key string) {
	total[cat]++
	if keys[cat][key] {
		covered[cat]++
		return
	}
	if len(gaps[cat]) < 12 {
		gaps[cat] = append(gaps[cat], key)
	}
}

// cirPointKeys collects the destination and control identities cir attaches to a
// single point, per category, so a semantics fact at that point can be matched
// against them.
func cirPointKeys(b *cir.Body, pt cfg.Point) map[string]map[string]bool {
	out := map[string]map[string]bool{
		"assign": {}, "call": {}, "branch": {}, "return": {},
	}
	for _, inst := range b.PointInstructions(pt) {
		switch inst.Op {
		case cir.OpAssign, cir.OpBinOp, cir.OpUnOp, cir.OpConcat, cir.OpMakeTable,
			cir.OpClaim, cir.OpLogical, cir.OpClosure, cir.OpSelect,
			cir.OpStaticMemberWrite, cir.OpDynamicIndexWrite:
			if k, ok := cirDstKey(b, inst.Dst); ok {
				out["assign"][k] = true
			}
		case cir.OpCall:
			out["call"][cirCallKey(b, inst)] = true
			for _, r := range b.Operands(inst.Results) {
				if k, ok := cirDstKey(b, r); ok {
					out["assign"][k] = true
				}
			}
		case cir.OpIterate:
			for _, r := range b.Operands(inst.Results) {
				if k, ok := cirDstKey(b, r); ok {
					out["assign"][k] = true
				}
			}
		case cir.OpReturn:
			out["return"]["return"] = true
		case cir.OpBranch:
			out["branch"][cirBranchKey(b, inst)] = true
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

func pct(n, d int) string {
	if d == 0 {
		return "n/a"
	}
	return strconv.FormatFloat(100*float64(n)/float64(d), 'f', 2, 64) + "%"
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
