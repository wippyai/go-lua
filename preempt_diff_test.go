package lua

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The differential tests check one invariant: a program run as a coroutine
// body under any tick budget, resumed until it finishes, is observably
// identical to the same program run without preemption.

// diffOutcome is everything observable about one run of a program.
type diffOutcome struct {
	rets string
	err  string
	log  string
}

func (o diffOutcome) String() string {
	return fmt.Sprintf("rets: %s\nerr: %s\nlog:\n%s", o.rets, o.err, o.log)
}

var addrRe = regexp.MustCompile(`0x[0-9a-fA-F]+`)

func normalizeAddrs(s string) string { return addrRe.ReplaceAllString(s, "0x?") }

// diffRepr renders a value with its exact type, tables with sorted entries.
func diffRepr(v LValue, depth int) string {
	switch x := v.(type) {
	case *LNilType:
		return "nil"
	case LBool:
		return x.String()
	case LNumber:
		return "float:" + strconv.FormatFloat(float64(x), 'g', -1, 64)
	case LInteger:
		return "int:" + strconv.FormatInt(int64(x), 10)
	case LString:
		return strconv.Quote(string(x))
	case *LTable:
		if depth <= 0 {
			return "{...}"
		}
		var items []string
		x.ForEach(func(k, val LValue) {
			items = append(items, diffRepr(k, depth-1)+"="+diffRepr(val, depth-1))
		})
		sort.Strings(items)
		return "{" + strings.Join(items, ",") + "}"
	case *Error:
		return "error:" + x.String()
	case *LFunction:
		return "function"
	case *LState:
		return "thread"
	}
	return fmt.Sprintf("%T:%s", v, v.String())
}

func diffReprList(vs []LValue) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = diffRepr(v, 4)
	}
	return strings.Join(parts, " | ")
}

// budgetSchedule returns the tick budget for the i-th resume.
type budgetSchedule func(i int) int64

func fixedBudget(n int64) budgetSchedule { return func(int) int64 { return n } }

func randomBudget(seed int64, max int) budgetSchedule {
	r := rand.New(rand.NewSource(seed))
	return func(int) int64 { return int64(r.Intn(max)) }
}

const diffNoLimit = -1

// A program takes part in the sweep when it finishes within screenPreempts
// preemptions of screenTicks ticks.
const (
	screenTicks    = 2000
	screenPreempts = 10
	screenTime     = 100 * time.Millisecond
)

// diffRun runs src as a coroutine body. Yields are forwarded back to the
// coroutine as resume values. It returns the outcome and the number of
// preemptions seen; ok is false when more than maxResumes resumes were needed.
func diffRun(src string, mods map[string]string, sched budgetSchedule, withCtx bool, maxResumes int, deadline time.Time) (out diffOutcome, preempts int, ok bool) {
	L := NewState()
	defer L.Close()
	var buf bytes.Buffer
	capturePrint(L, &buf)
	if mods != nil {
		installRequire(L, mods)
	}
	fn, err := L.LoadString(src)
	if err != nil {
		return diffOutcome{err: "load: " + err.Error()}, 0, true
	}
	var th *LState
	if withCtx {
		var cancel func()
		th, cancel = L.NewThread()
		defer cancel()
	} else {
		th = L.NewThreadWithContext(nil)
	}

	defer func() {
		if r := recover(); r != nil {
			out = diffOutcome{err: normalizeAddrs(fmt.Sprintf("host panic: %v", r)), log: normalizeAddrs(buf.String())}
			ok = true
		}
	}()

	var args []LValue
	for i := 0; maxResumes < 0 || i < maxResumes; i++ {
		if !deadline.IsZero() && time.Now().After(deadline) {
			break
		}
		L.SetTickBudget(sched(i))
		st, res, err := L.Resume(th, fn, args...)
		args = nil
		if err != nil {
			out.err = normalizeAddrs(err.Error())
			out.log = normalizeAddrs(buf.String())
			return out, preempts, true
		}
		switch st {
		case ResumePreempted:
			preempts++
		case ResumeYield:
			fmt.Fprintf(&buf, "yield: %s\n", diffReprList(res))
			args = append([]LValue(nil), res...)
		case ResumeOK:
			out.rets = normalizeAddrs(diffReprList(res))
			out.log = normalizeAddrs(buf.String())
			return out, preempts, true
		}
	}
	return out, preempts, false
}

// diffProgram is a source with the modules it requires.
type diffProgram struct {
	name string
	src  string
	mods map[string]string
}

// screen reports whether the program is deterministic and finishes quickly
// enough to be run at many budgets.
func (p diffProgram) screen() (diffOutcome, bool) {
	a, preempts, ok := diffRun(p.src, p.mods, fixedBudget(screenTicks), true, 400, time.Now().Add(screenTime))
	if !ok || preempts > screenPreempts {
		return a, false
	}
	b, _, ok := diffRun(p.src, p.mods, fixedBudget(diffNoLimit), true, 400, time.Time{})
	if !ok || a != b {
		return a, false
	}
	return a, true
}

type diffConfig struct {
	budgets []budgetSchedule
	names   []string
}

// budgetConfigs returns the budgets a sweep covers: 1..maxFixed, and nrandom
// random schedules whose budgets are below maxRandom.
func budgetConfigs(fixed []int, nrandom, maxRandom int, seed int64) diffConfig {
	var c diffConfig
	for _, n := range fixed {
		c.budgets = append(c.budgets, fixedBudget(int64(n)))
		c.names = append(c.names, fmt.Sprintf("fixed(%d)", n))
	}
	for i := 0; i < nrandom; i++ {
		s := seed + int64(i)
		m := 2 + (i*maxRandom)/max(nrandom, 1)
		c.budgets = append(c.budgets, randomBudget(s, m))
		c.names = append(c.names, fmt.Sprintf("random(seed=%d,max=%d)", s, m))
	}
	return c
}

// diffCheck runs p at every configured budget against the unpreempted
// baseline. It returns the number of runs and the first divergence text.
func diffCheck(p diffProgram, cfg diffConfig) (runs int, failure string) {
	base, ok := p.screen()
	if !ok {
		return 0, ""
	}
	for i, sched := range cfg.budgets {
		withCtx := i%2 == 0
		got, _, fin := diffRun(p.src, p.mods, sched, withCtx, 20_000_000, time.Time{})
		runs++
		if !fin {
			return runs, fmt.Sprintf("budget %s ctx=%v: no progress\n", cfg.names[i], withCtx)
		}
		if got != base {
			return runs, fmt.Sprintf("budget %s ctx=%v diverges\n--- unpreempted ---\n%s\n--- preempted ---\n%s\n", cfg.names[i], withCtx, base, got)
		}
	}
	return runs, ""
}

func rangeInts(lo, hi int) []int {
	var r []int
	for i := lo; i <= hi; i++ {
		r = append(r, i)
	}
	return r
}

// sweepScale returns the budgets and generated-program count of the sweep.
// PREEMPT_DIFF=full covers budgets 1..64 plus random ones.
func sweepScale() (cfg diffConfig, generated int) {
	if os.Getenv("PREEMPT_DIFF") == "full" {
		n := 2000
		if s := os.Getenv("PREEMPT_DIFF_PROGRAMS"); s != "" {
			n, _ = strconv.Atoi(s)
		}
		return budgetConfigs(rangeInts(1, 64), 24, 5000, 1000), n
	}
	return budgetConfigs([]int{1, 2, 3, 4, 5, 7, 11, 16, 33, 64}, 3, 500, 1000), 150
}

func sweep(t *testing.T, progs []diffProgram, cfg diffConfig) {
	t.Helper()
	runs, skipped, failures := 0, 0, 0
	for _, p := range progs {
		n, fail := diffCheck(p, cfg)
		if n == 0 && fail == "" {
			skipped++
			continue
		}
		runs += n
		if fail != "" {
			failures++
			if failures <= 8 {
				t.Errorf("%s: %s\nsource:\n%s", p.name, fail, p.src)
			}
		}
	}
	t.Logf("%d programs (%d skipped), %d preempted runs, %d divergent", len(progs), skipped, runs, failures)
}

var handwritten = []diffProgram{
	{name: "meta_index_yield", src: `
local t = setmetatable({}, {__index = function(_, k) local s = 0 for i = 1, k do s = s + i end return coroutine.yield(s) end})
local r = 0
for i = 1, 5 do r = r + t[i] end
return r`},
	{name: "meta_all_yield", src: `
local mt = {}
local function y(v) return coroutine.yield(v) end
mt.__add = function(a, b) return y('add') end
mt.__concat = function(a, b) return y('cat') end
mt.__eq = function(a, b) return y('eq') end
mt.__lt = function(a, b) return y('lt') end
mt.__le = function(a, b) return y('le') end
mt.__len = function(a) return y('len') end
mt.__unm = function(a) return y('unm') end
mt.__call = function(self, x) return y('call') end
mt.__newindex = function(t, k, v) rawset(t, k, y('ni')) end
local a, b = setmetatable({}, mt), setmetatable({}, mt)
local r = {a + 1, a .. 'x', 'x' .. a, a == b, a < b, a <= b, #a, -a, a(1)}
a.z = 1
r[#r + 1] = a.z
return table.unpack(r)`},
	{name: "pcall_yield_loop", src: `
local acc = {}
for i = 1, 6 do
  local ok, e = pcall(function()
    for j = 1, i do acc[#acc + 1] = coroutine.yield(j) end
    if i % 2 == 0 then error({code = i}) end
    return i
  end)
  acc[#acc + 1] = ok and 'ok' or e.code
end
return table.concat(acc, ',')`},
	{name: "nested_coroutines", src: `
local function mk(d)
  return coroutine.wrap(function(x)
    for i = 1, 3 do
      if d > 0 then x = x + mk(d - 1)(i) end
      x = x + coroutine.yield(x)
    end
    return x
  end)
end
local c = mk(2)
local r = {}
for i = 1, 4 do r[#r + 1] = c(i) end
return table.unpack(r)`},
	{name: "resume_chain_status", src: `
local co2 = coroutine.create(function() coroutine.yield(1) return 2 end)
local co1 = coroutine.create(function()
  print(coroutine.status(co2), coroutine.resume(co2))
  coroutine.yield(coroutine.status(co2))
  print(coroutine.resume(co2))
  print(coroutine.resume(co2))
  return coroutine.status(co2)
end)
local out = {}
while true do
  local ok, v = coroutine.resume(co1)
  out[#out + 1] = tostring(ok) .. tostring(v) .. coroutine.status(co1)
  if coroutine.status(co1) == 'dead' then break end
end
return table.concat(out, ';')`},
	{name: "gsub_sort_callbacks", src: `
local n = 0
local s = string.gsub('hello world foo bar', '%w+', function(w) for i = 1, 20 do n = n + i end return w:upper() end)
local t = {}
for i = 1, 20 do t[i] = (i * 7) % 11 end
table.sort(t, function(a, b) for i = 1, 3 do n = n + 1 end return a < b end)
return s, n, table.concat(t, ',')`},
	{name: "gsub_callback_yield", src: `
local ok, e = pcall(string.gsub, 'abc', '.', function(c) coroutine.yield(c) end)
local t = {3, 2, 1}
local ok2, e2 = pcall(table.sort, t, function(a, b) coroutine.yield(1) return a < b end)
return ok, e, ok2, e2`},
	{name: "tailcalls_varargs", src: `
local function f(n, ...)
  if n == 0 then return select('#', ...), ... end
  return f(n - 1, n, ...)
end
return f(50)`},
	{name: "mutual_tail", src: `
local even, odd
function even(n) if n == 0 then return true end return odd(n - 1) end
function odd(n) if n == 0 then return false end return even(n - 1) end
return even(1001), odd(7)`},
	{name: "error_levels", src: `
local function lvl(n) error('msg', n) end
local function wrap(n) lvl(n) end
local r = {}
for n = 0, 3 do r[#r + 1] = select(2, pcall(wrap, n)) end
r[#r + 1] = select(2, pcall(error))
r[#r + 1] = select(2, pcall(error, nil))
r[#r + 1] = select(2, pcall(error, setmetatable({}, {__tostring = function() return 'T' end}))) .. ''
return table.unpack(r)`},
	{name: "xpcall_handler_loop", src: `
local function h(m) local s = 0 for i = 1, 10 do s = s + i end return m .. s end
return xpcall(function() local x = nil; return x.y end, h)`},
	{name: "xpcall_handler_error", src: `
return xpcall(function() error('a') end, function(m) error('b') end)`},
	{name: "numeric_for_floats", src: `
local r = {}
for x = 0, 1, 0.1 do r[#r + 1] = x end
for x = 1, 0, -0.25 do r[#r + 1] = x end

for i = 1, 3 do r[#r + 1] = i + 0.5 end
return #r, r[3], r[#r], r[#r - 4]`},
	{name: "int_float", src: `
local a, b, c = 1, 1.0, 2^53
local r = {a + b, a * 2, b * 2, 7 // 2, 7.0 // 2, 7 % 3, -7 % 3, 7 / 7, c + 1, math.maxinteger + 1, 1e15 + 1}
for i = 1, 5 do a = a + 0.5; b = b * 2 end
r[#r + 1] = a
r[#r + 1] = b
return table.unpack(r)`},
	{name: "concat_chain", src: `
local s = ''
for i = 1, 40 do s = s .. i .. ',' .. i * 0.5 .. ';' end
local t = setmetatable({}, {__concat = function(a, b) return 'T' end})
return s, 'a' .. 1 .. 2 .. 'b', t .. 'x' .. 'y', 'x' .. 'y' .. t`},
	{name: "generic_for_iterators", src: `
local function range(n) local i = 0 return function() i = i + 1 if i <= n then return i end end end
local s = 0
for i in range(30) do for j in range(i) do s = s + j end end
local t = {a = 1, b = 2, c = 3, 10, 20, 30}
local keys = {}
for k, v in pairs(t) do keys[#keys + 1] = tostring(k) .. '=' .. v end
table.sort(keys)
for i, v in ipairs(t) do s = s + v end
local gen = coroutine.wrap(function() for i = 1, 5 do coroutine.yield(i) end end)
for v in gen do s = s + v end
return s, table.concat(keys, ',')`},
	{name: "pcall_pcall_error_object", src: `
local e = {n = 0}
local ok, r = pcall(pcall, function() e.n = e.n + 1 error(e) end)
local ok2, r2 = pcall(function() local ok, r = pcall(error, e) error(r, 2) end)
return ok, r, ok2, r2 == e, e.n`},
	{name: "coroutine_error_dead", src: `
local co = coroutine.create(function() for i = 1, 5 do end error('x') end)
local r = {coroutine.resume(co)}
r[#r + 1] = coroutine.status(co)
local r2 = {coroutine.resume(co)}
local w = coroutine.wrap(function() error({c = 1}) end)
local ok, e = pcall(w)
local ok2, e2 = pcall(w)
return r[1], r[2], r[3], r2[1], r2[2], ok, e.c, ok2, e2`},
	{name: "wrap_yield_across_pcall", src: `
local w = coroutine.wrap(function()
  local ok, e = pcall(function()
    local v = coroutine.yield(1)
    error('after ' .. v)
  end)
  coroutine.yield(e)
  return 'done'
end)
return w(), w('r'), w()`},
	{name: "deep_recursion", src: `
local function d(n) if n == 0 then return 0 end return 1 + d(n - 1) end
local function inf(n) return 1 + inf(n + 1) end
local ok, e = pcall(inf, 1)
return d(150), ok, type(e)`},
	{name: "stack_overflow_message", src: `
local function inf(n) return 1 + inf(n + 1) end
local ok, e = pcall(inf, 1)
return ok, e`},
	{name: "sort_errors", src: `
local t = {}
for i = 1, 30 do t[i] = (i * 13) % 17 end
local ok, e = pcall(table.sort, t, function(a, b) return true end)
local ok2, e2 = pcall(table.sort, t, function(a, b) error('cmp') end)
local ok3, e3 = pcall(table.sort, {1, 'x', 2})
return ok, e, ok2, e2, ok3, e3`},
	{name: "meta_loop_arith", src: `
local V = {}
V.__index = V
V.__add = function(a, b) return setmetatable({x = a.x + b.x}, V) end
V.__eq = function(a, b) return a.x == b.x end
V.__lt = function(a, b) return a.x < b.x end
V.__le = function(a, b) return a.x <= b.x end
V.__tostring = function(a) return 'V' .. a.x end
local acc = setmetatable({x = 0}, V)
for i = 1, 50 do acc = acc + setmetatable({x = i}, V) if acc < acc or acc <= acc then end end
return tostring(acc), acc == setmetatable({x = 1275}, V)`},
	{name: "index_chain", src: `
local base = {v = 1}
local cur = base
for i = 1, 20 do cur = setmetatable({}, {__index = cur}) end
local s = 0
for i = 1, 20 do s = s + cur.v end
local np = setmetatable({}, {__newindex = function(t, k, v) rawset(t, k, v * 2) end})
for i = 1, 10 do np[i] = i end
return s, np[10]`},
	{name: "yield_in_loops_values", src: `
local s = 0
for i = 1, 4 do
  local a, b = coroutine.yield(i, i * 2)
  s = s + (a or 0) + (b or 0)
  for j = 1, 3 do s = s + coroutine.yield() end
end
return s`},
	{name: "isyieldable_running", src: `
local co = coroutine.create(function()
  local c, main = coroutine.running()
  return coroutine.isyieldable(), main, c == co
end)
return coroutine.resume(co)`},
	{name: "string_methods_loop", src: `
local parts = {}
for w in string.gmatch('one two three four', '%a+') do parts[#parts + 1] = w:rep(2) end
local s = ('abc'):gsub('b', '%0%0')
return table.concat(parts, '-'), s, string.format('%5.2f|%d|%s', 3.14159, 42, 'x'), ('x'):byte(), #('x'):rep(100)`},
	{name: "close_and_wrap_misuse", src: `
local co = coroutine.create(function() coroutine.yield(1) end)
coroutine.resume(co)
local ok, e = coroutine.close and coroutine.close(co)
local w = coroutine.wrap(function() return 1 end)
w()
local ok2, e2 = pcall(w)
return ok, e, ok2, e2, coroutine.status(co)`},
}

// sweepPrograms returns the generated programs for seeds 0..n-1.
func generatedPrograms(n int, seed int64) []diffProgram {
	progs := make([]diffProgram, 0, n)
	for i := 0; i < n; i++ {
		r := rand.New(rand.NewSource(seed + int64(i)))
		progs = append(progs, diffProgram{name: fmt.Sprintf("gen/%d", seed+int64(i)), src: genProgram(randomChoices(r, 600))})
	}
	return progs
}

// fixturePrograms returns the runnable Lua of the fixture corpus: each
// directory with Lua files is a program whose last file is the entry point and
// whose other files are modules.
func fixturePrograms(t testing.TB) []diffProgram {
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discover fixtures: %v", err)
	}
	var progs []diffProgram
	for _, s := range suites {
		files := resolveFiles(s)
		if len(files) == 0 {
			continue
		}
		mods := make(map[string]string)
		ok := true
		for _, f := range files {
			if _, err := os.Stat(filepath.Join(s.Dir, f)); err != nil {
				ok = false
			}
		}
		if !ok {
			continue
		}
		for _, f := range files[:len(files)-1] {
			mods[strings.TrimSuffix(f, ".lua")] = readFixtureFile(s.Dir, f)
		}
		progs = append(progs, diffProgram{name: "fixture/" + s.Name, src: readFixtureFile(s.Dir, files[len(files)-1]), mods: mods})
	}
	return progs
}

// testCorpusPrograms returns the Lua snippets embedded as string literals in
// the package tests. Each is a standalone chunk; snippets that depend on host
// globals fail identically in every run and so still compare.
func testCorpusPrograms(t testing.TB) []diffProgram {
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	seen := make(map[string]bool)
	L := NewState()
	defer L.Close()
	var progs []diffProgram
	for _, f := range files {
		if strings.HasPrefix(f, "preempt_diff") {
			continue
		}
		fset := token.NewFileSet()
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		ast.Inspect(af, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || !strings.HasPrefix(lit.Value, "`") {
				return true
			}
			src, err := strconv.Unquote(lit.Value)
			if err != nil || len(src) < 24 || seen[src] {
				return true
			}
			seen[src] = true
			if _, err := L.LoadString(src); err != nil {
				return true
			}
			progs = append(progs, diffProgram{name: fmt.Sprintf("%s:%d", f, fset.Position(lit.Pos()).Line), src: src})
			return true
		})
	}
	return progs
}

func TestPreemptDifferentialSweep(t *testing.T) {
	cfg, generated := sweepScale()
	t.Run("handwritten", func(t *testing.T) { sweep(t, handwritten, cfg) })
	t.Run("generated", func(t *testing.T) { sweep(t, generatedPrograms(generated, 1), cfg) })
	t.Run("fixtures", func(t *testing.T) { sweep(t, fixturePrograms(t), cfg) })
	t.Run("test_corpus", func(t *testing.T) { sweep(t, testCorpusPrograms(t), cfg) })
}

// FuzzPreemptGenerated derives a program from the choice bytes and checks it
// under a fixed and a random budget schedule.
func FuzzPreemptGenerated(f *testing.F) {
	for i := 0; i < 8; i++ {
		f.Add(randomChoices(rand.New(rand.NewSource(int64(i))), 400), uint16(i+1))
	}
	f.Fuzz(func(t *testing.T, choices []byte, budget uint16) {
		p := diffProgram{name: "fuzz", src: genProgram(choices)}
		cfg := budgetConfigs([]int{1 + int(budget)%67}, 1, 300, int64(budget))
		if _, fail := diffCheck(p, cfg); fail != "" {
			t.Fatalf("%s\nsource:\n%s", fail, p.src)
		}
	})
}

var lengthIndexRe = regexp.MustCompile(`\[[^\]]*#[^\]]*[-+*][^\]]*\]`)

// FuzzPreemptSource checks arbitrary Lua source; sources that do not compile
// are skipped.
func FuzzPreemptSource(f *testing.F) {
	for _, h := range handwritten {
		f.Add(h.src, uint16(3))
	}
	f.Fuzz(func(t *testing.T, src string, budget uint16) {
		if lengthIndexRe.MatchString(src) {
			t.Skip("an index computed from a length grows a table exponentially")
		}
		L := NewState()
		_, err := L.LoadString(src)
		L.Close()
		if err != nil {
			t.Skip()
		}
		p := diffProgram{name: "fuzz", src: src}
		cfg := budgetConfigs([]int{1 + int(budget)%67}, 1, 300, int64(budget))
		if _, fail := diffCheck(p, cfg); fail != "" {
			t.Fatal(fail)
		}
	})
}
