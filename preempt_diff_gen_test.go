package lua

import (
	"fmt"
	"math/rand"
	"strings"
)

// Variable kinds tracked by the program generator.
const (
	kNum  = 'n'
	kCtr  = 'c'
	kStr  = 's'
	kTbl  = 't'
	kObj  = 'o'
	kFn   = 'f'
	kAny  = 'a'
	kBool = 'b'
)

type genVar struct {
	name string
	kind byte
}

// progGen builds terminating Lua programs from a stream of choice bytes. An
// exhausted stream selects the simplest alternative everywhere, so every
// stream yields a finite program.
type progGen struct {
	in      []byte
	pos     int
	sb      strings.Builder
	scope   []genVar
	cnt     int
	budget  int
	nest    int
	inCo    int
	varargs bool
}

// genProgram returns a program derived from the choice stream.
func genProgram(in []byte) string {
	g := &progGen{in: in, budget: 40, varargs: true}
	g.sb.WriteString("local t, a, b, s = {1, 2, 3}, 7, 2.5, 'ab'\n")
	g.scope = []genVar{{"t", kTbl}, {"a", kNum}, {"b", kNum}, {"s", kStr}}
	g.block(1 + g.n(5))
	g.sb.WriteString("return t, a, b, s\n")
	return g.sb.String()
}

// randomChoices returns n random choice bytes.
func randomChoices(r *rand.Rand, n int) []byte {
	buf := make([]byte, n)
	r.Read(buf)
	return buf
}

func (g *progGen) n(k int) int {
	if g.pos >= len(g.in) {
		return 0
	}
	b := int(g.in[g.pos])
	g.pos++
	return b % k
}

func (g *progGen) fresh() string {
	g.cnt++
	return fmt.Sprintf("v%d", g.cnt)
}

func (g *progGen) w(format string, args ...any) {
	fmt.Fprintf(&g.sb, format, args...)
}

// pick returns a visible variable of the kind; numbers include read-only
// loop variables, which assignments never target.
func (g *progGen) pick(kind byte) (string, bool) {
	var cands []string
	for _, v := range g.scope {
		if v.kind == kind || (kind == kNum && v.kind == kCtr) {
			cands = append(cands, v.name)
		}
	}
	if len(cands) == 0 {
		return "", false
	}
	return cands[g.n(len(cands))], true
}

func (g *progGen) pickAssignable() (string, bool) {
	var cands []string
	for _, v := range g.scope {
		if v.kind == kNum {
			cands = append(cands, v.name)
		}
	}
	if len(cands) == 0 {
		return "", false
	}
	return cands[g.n(len(cands))], true
}

func (g *progGen) declare(name string, kind byte) {
	g.scope = append(g.scope, genVar{name, kind})
}

func (g *progGen) block(count int) {
	mark := len(g.scope)
	for i := 0; i < count && g.budget > 0; i++ {
		g.budget--
		g.stmt()
	}
	g.scope = g.scope[:mark]
}

func (g *progGen) smallInt() string {
	return fmt.Sprint(1 + g.n(6))
}

func (g *progGen) numLit() string {
	switch g.n(10) {
	case 0:
		return "0"
	case 1:
		return "-3"
	case 2:
		return "2.5"
	case 3:
		return "1e3"
	case 4:
		return "0.1"
	case 5:
		return "9007199254740993"
	case 6:
		return "9223372036854775807"
	case 7:
		return "-0.0"
	case 8:
		return "2^53"
	}
	return fmt.Sprint(1 + g.n(20))
}

func (g *progGen) strLit() string {
	return []string{`"x"`, `"ab"`, `"hello world"`, `"10"`, `""`, `"a,b,c"`, `"3.5"`}[g.n(7)]
}

func (g *progGen) num(d int) string {
	if d <= 0 {
		if v, ok := g.pick(kNum); ok && g.n(2) == 0 {
			return v
		}
		return g.numLit()
	}
	switch g.n(16) {
	case 0, 1:
		if v, ok := g.pick(kNum); ok {
			return v
		}
		return g.numLit()
	case 2, 3, 4:
		ops := []string{"+", "-", "*", "/", "//", "%", "^", "+", "-"}
		return fmt.Sprintf("(%s %s %s)", g.num(d-1), ops[g.n(len(ops))], g.num(d-1))
	case 5:
		return fmt.Sprintf("(- %s)", g.num(d-1))
	case 6:
		return fmt.Sprintf("#%s", g.str(d-1, true))
	case 7:
		if v, ok := g.pick(kTbl); ok {
			return "#" + v
		}
	case 8:
		if v, ok := g.pick(kTbl); ok {
			return fmt.Sprintf("(%s[%s] or 0)", v, g.smallInt())
		}
	case 9:
		if v, ok := g.pick(kObj); ok {
			ops := []string{"+", "-", "*", "..", "//"}
			op := ops[g.n(len(ops))]
			if op == ".." {
				return fmt.Sprintf("#(%s .. %s)", v, g.num(d-1))
			}
			return fmt.Sprintf("(tonumber(%s %s %s) or 0)", v, op, g.num(d-1))
		}
	case 10:
		if v, ok := g.pick(kObj); ok {
			return fmt.Sprintf("(tonumber(#%s) or 0)", v)
		}
	case 11:
		if v, ok := g.pick(kFn); ok {
			return fmt.Sprintf("(tonumber((%s(%s, %s))) or 0)", v, g.smallInt(), g.num(d-1))
		}
	case 12:
		if g.varargs {
			return "select('#', ...)"
		}
	case 13:
		return fmt.Sprintf("math.floor(%s)", g.num(d-1))
	case 14:
		return fmt.Sprintf("(tonumber(%s) or 0)", g.str(d-1, true))
	case 15:
		if v, ok := g.pick(kObj); ok {
			return fmt.Sprintf("(tonumber(%s.%s) or 0)", v, []string{"x", "y", "z"}[g.n(3)])
		}
	}
	return g.numLit()
}

func (g *progGen) str(d int, pure bool) string {
	if d <= 0 {
		if v, ok := g.pick(kStr); ok && g.n(2) == 0 {
			return v
		}
		return g.strLit()
	}
	switch g.n(10) {
	case 0:
		if v, ok := g.pick(kStr); ok {
			return v
		}
	case 1, 2, 3:
		parts := []string{g.str(d-1, pure)}
		for i := 0; i < 1+g.n(3); i++ {
			if g.n(2) == 0 {
				parts = append(parts, g.num(d-1))
			} else {
				parts = append(parts, g.str(d-1, pure))
			}
		}
		return "(" + strings.Join(parts, " .. ") + ")"
	case 4:
		return fmt.Sprintf("tostring(%s)", g.num(d-1))
	case 5:
		return fmt.Sprintf("string.rep(%s, %d)", g.str(d-1, pure), g.n(4))
	case 6:
		return fmt.Sprintf("(%s):sub(%s, %s)", g.str(d-1, pure), g.smallInt(), g.smallInt())
	case 7:
		if v, ok := g.pick(kTbl); ok {
			return fmt.Sprintf("table.concat(%s, ',')", v)
		}
	case 8:
		if v, ok := g.pick(kObj); ok {
			return fmt.Sprintf("tostring(%s .. %s)", v, g.str(d-1, pure))
		}
	case 9:
		return fmt.Sprintf("string.format('%%s|%%s', %s, %s)", g.num(d-1), g.str(d-1, pure))
	}
	return g.strLit()
}

func (g *progGen) cond(d int) string {
	switch g.n(8) {
	case 0, 1:
		ops := []string{"<", "<=", ">", ">=", "==", "~="}
		return fmt.Sprintf("%s %s %s", g.num(d), ops[g.n(len(ops))], g.num(d))
	case 2:
		return fmt.Sprintf("%s == %s", g.str(d, true), g.str(d, true))
	case 3:
		return fmt.Sprintf("%s < %s", g.str(d, true), g.str(d, true))
	case 4:
		if v, ok := g.pick(kObj); ok {
			if w, ok := g.pick(kObj); ok {
				ops := []string{"<", "<=", "==", "~=", ">"}
				return fmt.Sprintf("%s %s %s", v, ops[g.n(len(ops))], w)
			}
		}
	case 5:
		return fmt.Sprintf("not (%s)", g.cond(d-1))
	case 6:
		return fmt.Sprintf("(%s) and (%s) or (%s)", g.cond(d-1), g.cond(d-1), g.cond(d-1))
	}
	return "true"
}

func (g *progGen) anyExpr(d int) string {
	switch g.n(6) {
	case 0:
		return g.str(d, true)
	case 1:
		return g.cond(d - 1)
	case 2:
		if v, ok := g.pick(kTbl); ok {
			return v
		}
	case 3:
		return "nil"
	}
	return g.num(d)
}

// print emits an observable statement.
func (g *progGen) printStmt() {
	g.w("print(%s, %s, math.type(%s))\n", g.anyExpr(2), g.num(2), g.num(2))
}

func (g *progGen) yieldStmt() {
	g.w("print('y', coroutine.yield(%s, %s))\n", g.num(1), g.str(1, true))
}

func (g *progGen) errorExpr() string {
	switch g.n(6) {
	case 0:
		return fmt.Sprintf("error(%s)", g.str(1, true))
	case 1:
		return fmt.Sprintf("error(%s, 0)", g.str(1, true))
	case 2:
		return fmt.Sprintf("error(%s, 2)", g.str(1, true))
	case 3:
		return fmt.Sprintf("error({code = %s})", g.num(1))
	case 4:
		return "error()"
	}
	return fmt.Sprintf("error(%s, 1)", g.anyExpr(1))
}

func (g *progGen) errVal(name string) string {
	return fmt.Sprintf("(type(%s) == 'table' and %s.code or %s)", name, name, name)
}

// fnBody writes statements and a final return with the given result kind.
func (g *progGen) fnBody(kind byte) {
	g.nest++
	g.block(g.n(4))
	g.nest--
	switch kind {
	case kStr:
		g.w("return %s\n", g.str(2, true))
	case kBool:
		g.w("return %s\n", g.cond(2))
	case kNum:
		g.w("return %s\n", g.num(2))
	default:
		g.w("return %s, %s\n", g.anyExpr(1), g.num(1))
	}
}

func (g *progGen) obj() {
	name := g.fresh()
	g.w("local %s = setmetatable({x = %s, y = %s}, {\n", name, g.numLit(), g.numLit())
	ops := []struct {
		mm   string
		args string
		kind byte
	}{
		{"__index", "o, k", kAny},
		{"__newindex", "o, k, v", 0},
		{"__call", "o, p", kAny},
		{"__concat", "p, q", kStr},
		{"__eq", "p, q", kBool},
		{"__lt", "p, q", kBool},
		{"__le", "p, q", kBool},
		{"__len", "o", kNum},
		{"__unm", "o", kNum},
		{"__add", "p, q", kNum},
		{"__sub", "p, q", kNum},
		{"__mul", "p, q", kNum},
		{"__idiv", "p, q", kNum},
		{"__tostring", "o", kStr},
	}
	used := map[int]bool{}
	for i := 0; i < 1+g.n(4); i++ {
		j := g.n(len(ops))
		if used[j] {
			continue
		}
		used[j] = true
		op := ops[j]
		mark := len(g.scope)
		g.w("  %s = function(%s)\n", op.mm, op.args)
		oldVar := g.varargs
		g.varargs = false
		if g.n(3) == 0 {
			g.yieldStmt()
		}
		if op.mm == "__newindex" {
			g.nest++
			g.block(g.n(3))
			g.nest--
			g.w("  rawset(o, k, %s)\n", g.num(1))
		} else {
			g.fnBody(op.kind)
		}
		g.w("  end,\n")
		g.varargs = oldVar
		g.scope = g.scope[:mark]
	}
	g.w("})\n")
	g.declare(name, kObj)
}

func (g *progGen) fnDef() {
	name := g.fresh()
	g.w("local function %s(n, ...)\n", name)
	g.w("  if n <= 0 then return %s end\n", g.anyExpr(1))
	oldVar := g.varargs
	g.varargs = true
	mark := len(g.scope)
	g.declare("n", kCtr)
	g.nest++
	g.block(g.n(3))
	g.nest--
	switch g.n(6) {
	case 0:
		g.w("  return %s(n - 1, ...)\n", name)
	case 1:
		g.w("  local r = %s(n - 1, %s)\n  return r, ...\n", name, g.num(1))
	case 2:
		g.w("  return n, select('#', ...), ...\n")
	case 3:
		g.w("  return %s(n - 1, select(2, ...))\n", name)
	case 4:
		g.w("  return (%s(n - 1, ...))\n", name)
	default:
		g.w("  local r = %s(n - 1, ...)\n  return n + (tonumber(r) or 0)\n", name)
	}
	g.scope = g.scope[:mark]
	g.varargs = oldVar
	g.w("end\n")
	g.declare(name, kFn)
}

func (g *progGen) stmt() {
	if g.nest > 3 {
		g.simpleStmt()
		return
	}
	switch g.n(26) {
	case 0:
		g.simpleStmt()
	case 1, 2:
		name := g.fresh()
		switch g.n(3) {
		case 0:
			g.w("local %s = %s\n", name, g.str(2, true))
			g.declare(name, kStr)
		case 1:
			g.w("local %s = {%s, %s, %s}\n", name, g.num(1), g.num(1), g.num(1))
			g.declare(name, kTbl)
		default:
			g.w("local %s = %s\n", name, g.num(2))
			g.declare(name, kNum)
		}
	case 3:
		if v, ok := g.pickAssignable(); ok {
			g.w("%s = %s\n", v, g.num(3))
		}
	case 4:
		g.w("if %s then\n", g.cond(2))
		g.nest++
		g.block(1 + g.n(3))
		if g.n(2) == 0 {
			g.w("else\n")
			g.block(1 + g.n(3))
		}
		g.nest--
		g.w("end\n")
	case 5, 6:
		iv := g.fresh()
		var hdr string
		switch g.n(4) {
		case 0:
			hdr = fmt.Sprintf("%s = 1, %s", iv, g.smallInt())
		case 1:
			hdr = fmt.Sprintf("%s = %s, 1, -1", iv, g.smallInt())
		case 2:
			hdr = fmt.Sprintf("%s = 0, 1, %s", iv, []string{"0.25", "0.3", "0.5", "1/3"}[g.n(4)])
		default:
			hdr = fmt.Sprintf("%s = 1.5, %s", iv, g.smallInt())
		}
		g.w("for %s do\n", hdr)
		mark := len(g.scope)
		g.declare(iv, kCtr)
		g.nest++
		g.block(1 + g.n(3))
		g.nest--
		g.scope = g.scope[:mark]
		g.w("end\n")
	case 7:
		c := g.fresh()
		g.w("local %s = 0\nwhile %s < %s do\n  %s = %s + 1\n", c, c, g.smallInt(), c, c)
		mark := len(g.scope)
		g.declare(c, kCtr)
		g.nest++
		g.block(1 + g.n(3))
		g.nest--
		g.scope = g.scope[:mark]
		g.w("end\n")
	case 8:
		c := g.fresh()
		g.w("local %s = 0\nrepeat\n  %s = %s + 1\n", c, c, c)
		mark := len(g.scope)
		g.declare(c, kCtr)
		g.nest++
		g.block(1 + g.n(3))
		g.nest--
		g.w("until %s >= %s\n", c, g.smallInt())
		g.scope = g.scope[:mark]
	case 9:
		g.iterLoop()
	case 10, 11:
		g.fnDef()
	case 12:
		if v, ok := g.pick(kFn); ok {
			g.w("print(%s(%s, %s))\n", v, g.smallInt(), g.num(2))
		}
	case 13, 14:
		g.obj()
	case 15:
		g.objUse()
	case 16, 17:
		g.pcallStmt()
	case 18, 19:
		g.coStmt()
	case 20:
		g.callbackStmt()
	case 21:
		if v, ok := g.pick(kTbl); ok {
			switch g.n(4) {
			case 0:
				g.w("%s[#%s + 1] = %s\n", v, v, g.num(2))
			case 1:
				g.w("table.insert(%s, %s)\n", v, g.num(1))
			case 2:
				g.w("print(table.remove(%s))\n", v)
			default:
				g.w("%s.k%s = %s\n", v, g.smallInt(), g.num(2))
			}
		}
	case 22:
		g.yieldStmt()
	case 23:
		g.varargStmt()
	case 24:
		g.w("print(%s)\n", g.str(3, true))
	default:
		g.printStmt()
	}
}

func (g *progGen) simpleStmt() {
	switch g.n(3) {
	case 0:
		g.printStmt()
	case 1:
		if v, ok := g.pickAssignable(); ok {
			g.w("%s = %s\n", v, g.num(2))
			return
		}
		g.printStmt()
	default:
		g.w("print(%s)\n", g.str(2, true))
	}
}

func (g *progGen) objUse() {
	v, ok := g.pick(kObj)
	if !ok {
		g.printStmt()
		return
	}
	w, _ := g.pick(kObj)
	switch g.n(10) {
	case 0:
		g.w("print(%s.%s)\n", v, []string{"x", "y", "zz"}[g.n(3)])
	case 1:
		g.w("%s.%s = %s\n", v, []string{"x", "q", "r"}[g.n(3)], g.num(2))
	case 2:
		g.w("print(%s(%s))\n", v, g.num(1))
	case 3:
		g.w("print(%s .. %s)\n", v, g.str(1, true))
	case 4:
		g.w("print(%s == %s, %s ~= %s)\n", v, w, v, w)
	case 5:
		g.w("print(%s < %s, %s <= %s)\n", v, w, v, w)
	case 6:
		g.w("print(#%s, - %s)\n", v, v)
	case 7:
		g.w("print(%s + %s, %s * 2, %s - %s)\n", v, g.num(1), v, w, v)
	case 8:
		g.w("print(tostring(%s))\n", v)
	default:
		g.w("print(rawget(%s, 'x'), rawequal(%s, %s), %s[%s])\n", v, v, w, v, g.smallInt())
	}
}

func (g *progGen) iterLoop() {
	it := g.fresh()
	iv, vv := g.fresh(), g.fresh()
	switch g.n(5) {
	case 0:
		g.w("local function %s(n, i) if i < n then return i + 1, i * %s end end\n", it, g.smallInt())
		g.w("for %s, %s in %s, %s, 0 do\n", iv, vv, it, g.smallInt())
	case 1:
		g.w("local function %s()\n  local i = 0\n  return function() i = i + 1 if i <= %s then return i, i * 2 end end\nend\n", it, g.smallInt())
		g.w("for %s, %s in %s() do\n", iv, vv, it)
	case 2:
		if v, ok := g.pick(kTbl); ok {
			g.w("for %s, %s in ipairs({table.unpack(%s, 1, 5)}) do\n", iv, vv, v)
		} else {
			g.w("for %s, %s in ipairs({1, 2, 3}) do\n", iv, vv)
		}
	case 3:
		g.w("local %s = setmetatable({n = 0}, {__call = function(self, st, i) self.n = self.n + 1 if self.n <= %s then return self.n, self.n end end})\n", it, g.smallInt())
		g.w("for %s, %s in %s, nil, nil do\n", iv, vv, it)
	default:
		g.w("local %s = coroutine.wrap(function() for i = 1, %s do coroutine.yield(i, -i) end end)\n", it, g.smallInt())
		g.w("for %s, %s in %s do\n", iv, vv, it)
	}
	mark := len(g.scope)
	g.declare(iv, kCtr)
	g.declare(vv, kCtr)
	g.nest++
	g.block(1 + g.n(3))
	g.nest--
	g.scope = g.scope[:mark]
	g.w("end\n")
}

func (g *progGen) pcallStmt() {
	ok, e := g.fresh(), g.fresh()
	mark := len(g.scope)
	protected := "pcall"
	x := g.n(2) == 1
	if x {
		protected = "xpcall"
	}
	g.w("local %s, %s = %s(function(...)\n", ok, e, protected)
	g.nest++
	g.block(1 + g.n(3))
	g.nest--
	if g.n(2) == 0 {
		g.w("  %s\n", g.errorExpr())
	}
	g.w("  return %s\nend", g.anyExpr(2))
	if x {
		g.w(", function(m) %s return %s end", g.handlerPrefix(), g.errVal("m"))
	}
	g.w(", %s)\n", g.anyExpr(1))
	g.scope = g.scope[:mark]
	g.w("print(%s, %s)\n", ok, g.errVal(e))
}

func (g *progGen) handlerPrefix() string {
	switch g.n(3) {
	case 0:
		return "local x = 0 for i = 1, 3 do x = x + i end"
	case 1:
		return "local ok2 = pcall(error, 'inner')"
	}
	return ""
}

func (g *progGen) coStmt() {
	co := g.fresh()
	wrapped := g.n(2) == 0
	if wrapped {
		g.w("local %s = coroutine.wrap(function(...)\n", co)
	} else {
		g.w("local %s = coroutine.create(function(...)\n", co)
	}
	g.inCo++
	g.nest++
	oldVar := g.varargs
	g.varargs = true
	mark := len(g.scope)
	if g.n(2) == 0 {
		g.yieldStmt()
	}
	g.block(1 + g.n(3))
	if g.n(2) == 0 {
		g.yieldStmt()
	}
	if g.n(3) == 0 {
		g.w("  %s\n", g.errorExpr())
	}
	g.scope = g.scope[:mark]
	g.varargs = oldVar
	g.nest--
	g.inCo--
	g.w("  return %s\nend)\n", g.anyExpr(1))
	r := g.fresh()
	if wrapped {
		g.w("for i = 1, %d do\n  local %s, %s = pcall(%s, %s)\n  print(%s, %s)\nend\n", 1+g.n(3), r, "m", co, g.num(1), r, g.errVal("m"))
	} else {
		g.w("for i = 1, %d do\n  local %s, m = coroutine.resume(%s, %s)\n  print(%s, %s, coroutine.status(%s))\nend\n", 1+g.n(3), r, co, g.num(1), r, g.errVal("m"), co)
	}
}

func (g *progGen) callbackStmt() {
	switch g.n(4) {
	case 0:
		g.w("print((string.gsub(%s, '%%w', function(c)\n", g.str(1, true))
		g.callbackBody(kStr)
		g.w("end)))\n")
	case 1:
		g.w("print(pcall(string.gsub, %s, '(%%w)', function(c)\n", g.str(1, true))
		g.callbackBody(kStr)
		g.w("end))\n")
	case 2:
		tv := g.fresh()
		g.w("local %s = {%s, %s, %s, %s}\n", tv, g.num(1), g.num(1), g.num(1), g.num(1))
		g.w("print(pcall(table.sort, %s, function(p, q)\n", tv)
		g.callbackBody(kBool)
		g.w("end))\n")
		g.w("print(table.concat(%s, ','))\n", tv)
	default:
		tv := g.fresh()
		g.w("local %s = {3, 1, 2}\n", tv)
		g.w("table.sort(%s, function(p, q) return p > q end)\nprint(table.concat(%s, ','))\n", tv, tv)
	}
}

func (g *progGen) callbackBody(kind byte) {
	oldVar := g.varargs
	g.varargs = false
	defer func() { g.varargs = oldVar }()
	mark := len(g.scope)
	g.declare("p", kCtr)
	g.declare("q", kCtr)
	g.declare("c", kStr)
	g.nest++
	g.block(g.n(3))
	g.nest--
	switch kind {
	case kStr:
		g.w("  return %s\n", g.str(1, true))
	default:
		g.w("  return %s\n", g.cond(1))
	}
	g.scope = g.scope[:mark]
}

func (g *progGen) varargStmt() {
	switch g.n(4) {
	case 0:
		g.w("print(select('#', %s, %s))\n", g.num(1), g.str(1, true))
	case 1:
		g.w("print(select(%s, %s, %s, %s))\n", "2", g.num(1), g.str(1, true), g.num(1))
	case 2:
		g.w("print(table.unpack({%s, %s, %s}))\n", g.num(1), g.num(1), g.num(1))
	default:
		g.w("do local function va(...) local x, y = ... return select('#', ...), y, ... end print(va(%s, %s, nil)) end\n", g.num(1), g.str(1, true))
	}
}
