package lower_test

import (
    "testing"
    "github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestZZDebug(t *testing.T) {
    p := parseBindLower(t, `local function candidates(a,b,...)
  local got = a[b]
  a[b] = b
  a:b(b,...)
  return a(b,...)
end`)
    f := p.Flow(); a := f.Authored();
    t.Logf("counts read=%d exact=%d dynamic=%d write=%d calls=%d", a.Storage().Reads().Count(), a.Access().Exact().Count(), a.Access().Dynamic().Count(), a.Storage().Writes().Count(), a.Calls().Count())
    spans:=p.Source().Identity()
    for i:=0;i<a.Storage().Reads().Count();i++ { term,_:=a.Storage().Reads().At(i); o,s,imp,ok:=a.Storage().Reads().Get(term); sp,_:=spans.Span(term); t.Logf("read %d term=%v span=%#v owner=%v source=%v fam=%v imp=%v ok=%v exec=%v",i,term,sp,o,s,keyspace.TermFamily(s),imp,ok,f.Executable().Contains(term)) }
    for i:=0;i<a.Access().Exact().Count();i++ { term,_:=a.Access().Exact().At(i); o,b,s,k,ok:=a.Access().Exact().Get(term); sp,_:=spans.Span(term); t.Logf("exact %d term=%v span=%#v owner=%v base=%v source=%v kind=%v ok=%v",i,term,sp,o,b,s,k,ok) }
    for i:=0;i<a.Access().Dynamic().Count();i++ { term,_:=a.Access().Dynamic().At(i); o,b,k,ok:=a.Access().Dynamic().Get(term); sp,_:=spans.Span(term); t.Logf("dynamic %d term=%v span=%#v owner=%v base=%v key=%v ok=%v",i,term,sp,o,b,k,ok) }
    for i:=0;i<a.Storage().Writes().Count();i++ { term,_:=a.Storage().Writes().At(i); as,tg,ok:=a.Storage().Writes().Get(term); sp,_:=spans.Span(term); t.Logf("write %d term=%v span=%#v assign=%v target=%v fam=%v ok=%v exec=%v",i,term,sp,as,tg,keyspace.TermFamily(tg),ok,f.Executable().Contains(term)) }
    c:=f.Candidates().Access(); t.Logf("candidates get=%d set=%d",c.GetCount(),c.SetCount()); for i:=0;i<c.GetCount();i++ { x,_:=c.GetAt(i);t.Logf("getcand %v",x)};for i:=0;i<c.SetCount();i++ {x,_:=c.SetAt(i);t.Logf("setcand %v",x)}
}

func TestZZDebugMethod(t *testing.T) {
 p:=parseBindLower(t, `local captured=1
local receiver={}
function receiver:method(first,...)
 return captured,self,first,...
end`)
 a:=p.Flow().Authored(); t.Logf("funcs=%d cells=%d varargs=%d",a.Functions().Count(),a.Storage().Cells().Count(),a.Storage().Varargs().Count())
 for i:=0;i<a.Functions().Count();i++ {x,_:=a.Functions().At(i);o,b,v,ok:=a.Functions().Get(x);t.Logf("fn %v(fam %v) owner=%v body=%v vararg=%v(fam %v) ok=%v",x,keyspace.TermFamily(x),o,b,v,keyspace.TermFamily(v),ok); if v!=0 {vo,c,vok:=a.Storage().Varargs().Get(v);t.Logf("vararg owner=%v cell=%v ok=%v",vo,c,vok)} }
 for i:=0;i<a.Storage().Cells().Count();i++ {x,_:=a.Storage().Cells().At(i);k,b,key,ok:=a.Storage().Cells().Get(x);t.Logf("cell %v(fam %v) kind=%v body=%v key=%v ok=%v",x,keyspace.TermFamily(x),k,b,key,ok)}
}

func TestZZDebugSelect(t *testing.T) {
 p:=parseBindLower(t, `local left,right=true,false; return left and right, left or right`)
 f:=p.Flow(); a:=f.Authored(); s:=a.Operators().Selects(); t.Logf("selects=%d edges=%d successors=%d",s.Count(),f.Causal().Edges().Count(),f.Causal().Successors().Count(0))
 for i:=0;i<s.Count();i++ {x,_:=s.At(i);o,op,l,r,ok:=s.Get(x);t.Logf("sel %d=%v owner=%v op=%v left=%v right=%v ok=%v exec=%v",i,x,o,op,l,r,ok,f.Executable().Contains(x));t.Logf(" succ count=%d",f.Causal().Successors().Count(x)); for j:=0;j<f.Causal().Successors().Count(x);j++ {q,qok:=f.Causal().Successors().At(x,j);t.Logf("  succ %d %#v ok=%v",j,q,qok)} }
 for i:=0;i<f.Causal().Edges().Count();i++ {e,ok:=f.Causal().Edges().At(i);t.Logf("edge %d %#v ok=%v",i,e,ok)}
}
