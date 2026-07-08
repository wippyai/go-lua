package wirlower_test

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/transferfacts"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

type shadowCoverageExpectation struct {
	covered int
	total   int
}

var expectedShadowCoverage = map[string]shadowCoverageExpectation{
	"assign": {covered: 3071, total: 3071},
	"call":   {covered: 1729, total: 1729},
	"branch": {covered: 424, total: 424},
	"return": {covered: 212, total: 212},
}

// TestShadowCoverage is an opt-in (WIR_SHADOW=1) per-point completeness oracle.
// Because D1a lowers onto the SAME cfgbuild graph that semantics extracts from,
// the comparison is now a true per-point diff: for every point that carries a
// semantics fact (assign / call / branch / return, imported read-only), the wir
// Body must carry an instruction AT THAT POINT whose operand identity (path key)
// matches. It pins corpus-wide coverage per category and lists the residual gaps
// honestly rather than masking them. Better coverage should update the expected
// frontier intentionally; worse coverage fails immediately.
//
// Computed assignment targets with no static path/container identity match the
// semantics "target" sentinel only when wir still records a write at the same
// point; all structural path identities remain compared exactly.
func TestShadowCoverage(t *testing.T) {
	if os.Getenv("WIR_SHADOW") != "1" {
		t.Skip("set WIR_SHADOW=1 to run the wir lowering coverage harness")
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
		body := wirlower.Lower("main", stmts, bindings, built)
		if body == nil {
			skippedExtract++
			continue
		}
		facts := transferfacts.Lower(built.Graph, transferfacts.Config{
			Registry: standard.Registry(),
			WIR:      body,
		})
		processed++

		branchKeys := sourceBranchKeys(stmts, bindings, built)
		for _, pt := range built.Graph.RPO() {
			keys := wirPointKeys(body, pt)
			if key, ok := sourceAssignmentKey(facts, pt); ok {
				scorePoint(total, covered, gapSamples, "assign", keys, key)
			}
			if f, ok := facts.CallSiteView(pt); ok {
				scorePoint(total, covered, gapSamples, "call", keys, sourceCallKey(f))
			}
			if built.Graph.Node(pt).Kind == cfg.NodeReturn {
				scorePoint(total, covered, gapSamples, "return", keys, "return")
			}
			if branchKey := branchKeys[pt]; branchKey != "" {
				scorePoint(total, covered, gapSamples, "branch", keys, branchKey)
			}
		}
	}

	t.Logf("wir per-point coverage over %d/%d fixtures (parse-skip %d, bind-skip %d, extract-skip %d)",
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
		want := expectedShadowCoverage[cat]
		if cc != want.covered || tc != want.total {
			t.Fatalf("%s coverage drifted: got %d/%d, want %d/%d; gaps=%v", cat, cc, tc, want.covered, want.total, gapSamples[cat])
		}
	}
	t.Logf("  %-7s %6d/%-6d  %s", "TOTAL", coveredAll, totalAll, pct(coveredAll, totalAll))
}

// scorePoint records one semantics fact at a point and whether wir carries a
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

// wirPointKeys collects the destination and control identities wir attaches to a
// single point, per category, so a semantics fact at that point can be matched
// against them.
func wirPointKeys(b *wir.Body, pt cfg.Point) map[string]map[string]bool {
	out := map[string]map[string]bool{
		"assign": {}, "call": {}, "branch": {}, "return": {},
	}
	for _, inst := range b.PointInstructions(pt) {
		switch inst.Op {
		case wir.OpAssign, wir.OpBinOp, wir.OpUnOp, wir.OpConcat, wir.OpMakeTable,
			wir.OpDynamicIndexRead, wir.OpClaim, wir.OpLogical, wir.OpClosure, wir.OpSelect,
			wir.OpStaticMemberWrite, wir.OpDynamicIndexWrite:
			if k, ok := wirDstKey(b, inst.Dst); ok {
				out["assign"][k] = true
			} else {
				// Semantics uses the same sentinel when an assignment target has
				// no static path or container identity. This keeps the shadow
				// oracle honest: the point must still carry an assignment/write
				// instruction, but there is no structural key to compare.
				out["assign"]["target"] = true
			}
		case wir.OpCall:
			out["call"][wirCallKey(b, inst)] = true
			for _, r := range b.Operands(inst.Results) {
				if k, ok := wirDstKey(b, r); ok {
					out["assign"][k] = true
				}
			}
		case wir.OpIterate:
			for _, r := range b.Operands(inst.Results) {
				if k, ok := wirDstKey(b, r); ok {
					out["assign"][k] = true
				}
			}
		case wir.OpReturn:
			out["return"]["return"] = true
		case wir.OpBranch:
			out["branch"][wirBranchKey(b, inst)] = true
		}
	}
	return out
}

func wirDstKey(b *wir.Body, op wir.Operand) (string, bool) {
	if op.Kind != wir.OperandPath {
		return "", false
	}
	p := b.Path(wir.PathRef(op.Ref))
	if p.IsEmpty() {
		return "", false
	}
	return string(p.Key()), true
}

func wirCallKey(b *wir.Body, inst wir.Instruction) string {
	if inst.Call.Method != 0 {
		method := b.Const(inst.Call.Method).Str
		// Match semantics' callee identity, which folds a method call into the
		// receiver.method member path (dot form), not a distinct receiver:method key.
		if inst.Call.Receiver.Kind == wir.OperandPath {
			p := b.Path(wir.PathRef(inst.Call.Receiver.Ref))
			if !p.IsEmpty() {
				return string(p.Field(method).Key())
			}
		}
		return "recv:" + method
	}
	if k, ok := wirDstKey(b, inst.Call.Callee); ok {
		return k
	}
	return "callexpr"
}

func wirBranchKey(b *wir.Body, inst wir.Instruction) string {
	c := b.Check(inst.Check)
	return string(c.Path.Key()) + "|" + strconv.Itoa(int(c.Kind))
}

func pct(n, d int) string {
	if d == 0 {
		return "n/a"
	}
	return strconv.FormatFloat(100*float64(n)/float64(d), 'f', 2, 64) + "%"
}

func sourceAssignmentKey(facts factflow.Facts, point cfg.Point) (string, bool) {
	if f, ok := facts.PathAssignment(point); ok {
		return string(f.TargetPathRef().Key()), true
	}
	if f, ok := facts.DynamicIndexWrite(point); ok {
		return string(f.TablePathRef().Key()), true
	}
	if f, ok := facts.RootAssignment(point); ok {
		target := f.TargetPathRef()
		if target.IsEmpty() {
			return "target", true
		}
		return string(target.Key()), true
	}
	return "", false
}

func sourceCallKey(f factflow.CallSiteView) string {
	if callee := f.CalleePathRef(); !callee.IsEmpty() {
		return string(callee.Key())
	}
	if method := f.MethodName(); method != "" {
		if receiver, ok := f.ReceiverPath(); ok {
			return string(receiver.Key()) + ":" + method
		}
		return "recv:" + method
	}
	return "callexpr"
}

func sourceBranchKeys(stmts []ast.Stmt, bindings *bind.Result, built *cfgbuild.Result) map[cfg.Point]string {
	if built == nil || built.Graph == nil {
		return nil
	}
	out := map[cfg.Point]string{}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.IfStmt:
				addSourceBranchKey(out, bindings, built, stmt, stmt.Condition)
				walk(stmt.Then)
				walk(stmt.Else)
			case *ast.WhileStmt:
				addSourceBranchKey(out, bindings, built, stmt, stmt.Condition)
				walk(stmt.Stmts)
			case *ast.RepeatStmt:
				walk(stmt.Stmts)
				addSourceBranchKey(out, bindings, built, stmt, stmt.Condition)
			case *ast.DoBlockStmt:
				walk(stmt.Stmts)
			case *ast.NumberForStmt:
				walk(stmt.Stmts)
			case *ast.GenericForStmt:
				walk(stmt.Stmts)
			}
		}
	}
	walk(stmts)
	for _, point := range built.ShortCircuits.GuardPoints() {
		guard, ok := built.ShortCircuits.Guard(point)
		if !ok || guard.Condition == nil || !built.Graph.IsBranch(point) {
			continue
		}
		out[point] = branchCheckKey(branchcond.Normalize(guard.Condition, bindings))
	}
	return out
}

func addSourceBranchKey(out map[cfg.Point]string, bindings *bind.Result, built *cfgbuild.Result, stmt ast.Stmt, condition ast.Expr) {
	if condition == nil {
		return
	}
	points := built.StmtPoints.PointsFor(stmt)
	for i := len(points) - 1; i >= 0; i-- {
		if built.Graph.IsBranch(points[i]) {
			out[points[i]] = branchCheckKey(branchcond.Normalize(condition, bindings))
			return
		}
	}
}

func branchCheckKey(check branchcond.Check) string {
	return string(check.Path.Key()) + "|" + strconv.Itoa(int(check.Kind))
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
