package lower_test

import (
    "fmt"
    "testing"
    "github.com/wippyai/go-lua/analysis/lua/bind"
    "github.com/wippyai/go-lua/compiler/parse"
    "github.com/wippyai/go-lua/compiler/ast"
    "github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestDebugBindTyped(t *testing.T) {
    stmts, err := parse.ParseString("type User = number\nlocal function id<T>(value: T): T return value end\nreturn id::<User>(User)", "x.lua")
    if err != nil { t.Fatal(err) }
    ret := stmts[2].(*ast.ReturnStmt)
    _ = ret
    b := bind.BindChunk(stmts)
    call := ret.Exprs[0].(*ast.FuncCallExpr)
    fmt.Printf("call typeargs=%d func=%T args=%d\n", len(call.TypeArgs), call.Func, len(call.Args))
    for _, e := range []ast.Expr{call.Func, call.Args[0]} {
      if id, ok := e.(*ast.IdentExpr); ok { v, vok:=b.RuntimeTypeValue(id); tv, tvok:=b.TypeValueRef(id); sym, sok := b.SymbolOf(id); fmt.Printf("ident %s runtime=%#v/%v typeRef=%#v/%v symbol=%v/%v\n",id.Value,v,vok,tv,tvok, sym,sok) }
    }
}

func TestDebugStatic(t *testing.T) {
    p := parseBindLower(t, `
type Snapshot = typeof(function()
::again::
goto again
end)
`)
    v := p.Flow()
    tof, _ := p.Static().Operators().TypeOfs().At(0)
    scope, operand, _ := p.Static().Operators().TypeOfs().Get(tof)
    fmt.Printf("TypeOf=%v scope=%v operand=%v fam=%v\n", tof, scope, operand, keyspace.TermFamily(operand))
    fn, _ := v.Authored().Functions().At(0)
    own, body, _, _ := v.Authored().Functions().Get(fn)
    fmt.Printf("Function=%v owner=%v body=%v\n", fn, own, body)
    fmt.Printf("counts func=%d body=%d label=%d goto=%d typeOf=%d alias=%d\n", v.Authored().Functions().Count(), p.Source().Identity().FamilyCount(keyspace.FamilyBody), v.Authored().Control().Labels().Count(), v.Authored().Control().Gotos().Count(), p.Static().Operators().TypeOfs().Count(), p.Static().Declarations().Aliases().Count())
    for _, f := range []keyspace.Family{keyspace.FamilyFunction,keyspace.FamilyBody,keyspace.FamilyLabel,keyspace.FamilyGoto,keyspace.FamilyTypeOf,keyspace.FamilyTypeAlias} {
      for i:=0;i<int(p.Source().Identity().FamilyCount(f));i++ { term:=keyspace.MakeTerm(f,uint32(i+1)); parent,has:=v.Containment().Parent(term); fmt.Printf("term %v/%d=%v static=%v exec=%v parent=%v/%v\n", f,i+1,term,v.Containment().Static(term),v.Executable().Contains(term),parent,has) }
    }
}

func TestDebugOpenCall(t *testing.T) {
    p := parseBindLower(t, "local function f() return 1, 2 end; return 1, f()")
    entry, _ := p.Source().Index().Entry()
    bodyLen,_ := p.Source().Order().BodyLen(entry)
    fmt.Printf("entry=%v bodyroots=%d returns=%d calls=%d values=%d funcs=%d\\n", entry, bodyLen, p.Flow().Authored().Control().Returns().Count(), p.Flow().Authored().Calls().Count(), p.Flow().Authored().Values().Count(), p.Flow().Authored().Functions().Count())
    for i:=0; i<bodyLen; i++ { term,ok:=p.Source().Order().BodyAt(entry,i); fmt.Printf("root[%d]=%v/%v family=%v\\n", i,term,ok,keyspace.TermFamily(term)); if owner, vals, rok:=p.Flow().Authored().Control().Returns().Get(term); rok { fmt.Printf(" return owner=%v values=%v\\n",owner,vals) } }
    for i:=0;i<p.Flow().Authored().Control().Returns().Count();i++ { ret,_:=p.Flow().Authored().Control().Returns().At(i); own,vals,ok:=p.Flow().Authored().Control().Returns().Get(ret); fmt.Printf("ret[%d]=%v own=%v vals=%v/%v\\n",i,ret,own,vals,ok) }
}
