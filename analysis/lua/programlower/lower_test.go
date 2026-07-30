package programlower_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/programlower"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/program"
)

var loweredSink *program.Program

func lowerSource(source string) (*program.Program, error) {
	stmts, err := parse.ParseString(source, "fixture.lua")
	if err != nil {
		return nil, err
	}
	return programlower.Lower(
		"fixture.lua",
		stmts,
		bind.BindChunk(stmts, bind.Options{}),
	)
}

func parseBindLower(t *testing.T, source string) *program.Program {
	t.Helper()
	lowered, err := lowerSource(source)
	if err != nil {
		t.Fatal(err)
	}
	return lowered
}

func bodyRoots(t *testing.T, p *program.Program, body program.Term) []program.Term {
	t.Helper()
	count, ok := p.BodyLen(body)
	if !ok {
		t.Fatalf("%v is not a Body", body)
	}
	roots := make([]program.Term, count)
	for i := range roots {
		var ok bool
		roots[i], ok = p.Root(body, i)
		if !ok {
			t.Fatalf("Root(%v, %d) failed", body, i)
		}
	}
	return roots
}

func valuesTail(t *testing.T, p *program.Program, values program.Term) program.Term {
	t.Helper()
	_, tail, ok := p.Values(values)
	if !ok {
		t.Fatalf("%v is not Values", values)
	}
	return tail
}

func valueAt(t *testing.T, p *program.Program, values program.Term, index int) program.Term {
	t.Helper()
	value, ok := p.Value(values, index)
	if !ok {
		t.Fatalf("Value(%v, %d) failed", values, index)
	}
	return value
}

func boundCell(t *testing.T, p *program.Program, bind program.Term, index int) program.Term {
	t.Helper()
	cell, ok := p.BoundCell(bind, index)
	if !ok {
		t.Fatalf("BoundCell(%v, %d) failed", bind, index)
	}
	return cell
}

func functionCapture(t *testing.T, p *program.Program, function program.Term, index int) (program.Term, program.Term) {
	t.Helper()
	inner, outer, ok := p.FunctionCapture(function, index)
	if !ok {
		t.Fatalf("FunctionCapture(%v, %d) failed", function, index)
	}
	return inner, outer
}

func mustBranch(t *testing.T, p *program.Program, term program.Term) (program.Term, program.Term, program.Term, program.Term) {
	t.Helper()
	owner, condition, whenTrue, whenFalse, ok := p.Branch(term)
	if !ok {
		t.Fatalf("%v is not a Branch", term)
	}
	return owner, condition, whenTrue, whenFalse
}

func mustLoop(t *testing.T, p *program.Program, term program.Term) (program.Term, program.Term, program.Term, program.LoopKind) {
	t.Helper()
	owner, body, control, kind, ok := p.Loop(term)
	if !ok {
		t.Fatalf("%v is not a Loop", term)
	}
	return owner, body, control, kind
}

func TestEmptyChunkHasOneCanonicalEntryAndStructuralTail(t *testing.T) {
	p := parseBindLower(t, "")
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("empty chunk has no Entry")
	}
	roots := bodyRoots(t, p, entry)
	if len(roots) != 0 {
		t.Fatalf("empty Entry roots = %v; want structural Body tail only", roots)
	}
}

func TestParseBindLowerLiteralLocalsTableAssignmentAndReturn(t *testing.T) {
	p := parseBindLower(t, `
local x = 1
local t = {name = x, [2] = "two", [x] = false}
x = 3
return x, t
`)

	var tableTerm, returnValues program.Term
	rootBody, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	var cellTerms, bindTerms []program.Term
	for i := 0; i < p.CellCount(); i++ {
		cell, ok := p.CellAt(i)
		if !ok {
			t.Fatalf("CellAt(%d) failed", i)
		}
		cellTerms = append(cellTerms, cell)
	}
	for i := 0; i < p.BindCount(); i++ {
		binding, ok := p.BindAt(i)
		if !ok {
			t.Fatalf("BindAt(%d) failed", i)
		}
		bindTerms = append(bindTerms, binding)
	}
	if count := p.IntegerCount(); count != 3 {
		t.Fatalf("IntegerCount = %d, want 3", count)
	}
	for i := 0; i < p.IntegerCount(); i++ {
		term, _ := p.IntegerAt(i)
		_, value, ok := p.Integer(term)
		if ok && value == 1 {
			span, ok := p.Span(term)
			if !ok || span.File != "fixture.lua" || span.StartLine != 2 {
				t.Fatalf("literal span = %#v, %v", span, ok)
			}
		}
	}
	if p.BodyCount() != 1 || p.CellCount() != 2 || p.ReadCount() != 4 || p.BindCount() != 2 || p.AssignCount() != 1 || p.TableCount() != 1 {
		t.Fatalf("typed family counts: bodies=%d cells=%d reads=%d binds=%d assigns=%d tables=%d",
			p.BodyCount(), p.CellCount(), p.ReadCount(), p.BindCount(), p.AssignCount(), p.TableCount())
	}
	if p.LensExactCount()+p.LensKeyCount() != 0 {
		t.Fatalf("constructor minted lenses: exact=%d dynamic=%d", p.LensExactCount(), p.LensKeyCount())
	}
	if p.ReturnCount() != 1 {
		t.Fatalf("ReturnCount = %d", p.ReturnCount())
	}
	tableTerm, _ = p.TableAt(0)
	returnTerm, _ := p.ReturnAt(0)
	_, returnValues, _ = p.Return(returnTerm)
	roots := bodyRoots(t, p, rootBody)
	if len(roots) != 4 {
		t.Fatalf("root Body = %v; want four statement roots", roots)
	}
	if _, ok := p.BindLen(roots[0]); !ok {
		t.Fatalf("root 0 is not Bind: %v", roots[0])
	}
	if _, ok := p.BindLen(roots[1]); !ok {
		t.Fatalf("root 1 is not Bind: %v", roots[1])
	}
	if _, ok := p.AssignLen(roots[2]); !ok {
		t.Fatalf("root 2 is not Assign: %v", roots[2])
	}
	if _, _, ok := p.Return(roots[3]); !ok {
		t.Fatalf("root 3 is not Return: %v", roots[3])
	}
	for i, binding := range bindTerms {
		owner, values, ok := p.Bind(binding)
		if !ok || owner != rootBody || values == 0 {
			t.Fatalf("Bind %d = owner %v values %v ok %v", i, owner, values, ok)
		}
	}
	for i, cell := range cellTerms {
		owner, ok := p.Cell(cell)
		if !ok || owner != rootBody {
			t.Fatalf("Cell %d owner = %v, %v; want root Body %v", i, owner, ok, rootBody)
		}
		span, ok := p.Span(cell)
		if !ok || span.File != "fixture.lua" || span.StartLine != i+2 || span.StartCol != 7 {
			t.Fatalf("Cell %d span = %#v, %v", i, span, ok)
		}
	}
	if count, ok := p.TableLen(tableTerm); !ok || count != 3 {
		t.Fatalf("TableLen = %d, %v; want three direct fields", count, ok)
	}
	for i, wantKind := range []program.FieldKind{program.FieldName, program.FieldExact, program.FieldKey} {
		source, values, kind, _, ok := p.Field(tableTerm, i)
		if !ok || source == 0 || values == 0 || kind != wantKind {
			t.Fatalf("Field(%d) = source %v values %v kind %v ok %v", i, source, values, kind, ok)
		}
		if count, ok := p.ValuesLen(values); !ok || count != 1 {
			t.Fatalf("field %d ValuesLen = %d, %v", i, count, ok)
		}
		if i == 0 {
			_, got, _, ok := p.Name(source)
			if !ok || got != "name" {
				t.Fatalf("field 0 key = %q, %v", got, ok)
			}
		}
		if i == 1 {
			_, got, ok := p.Integer(source)
			if !ok || got != 2 {
				t.Fatalf("field 1 key = %d, %v", got, ok)
			}
			if span, ok := p.Span(source); !ok || span.StartLine != 3 || span.StartCol != 23 {
				t.Fatalf("field 1 key span = %#v, %v; want bracket key token", span, ok)
			}
		}
		if i == 2 {
			if _, _, ok := p.Read(source); !ok {
				t.Fatalf("dynamic field key is not a bound Read: %v", source)
			}
		}
		if source == tableTerm || values == tableTerm {
			t.Fatal("Table constructor contains its own allocation identity")
		}
	}
	if count, ok := p.ValuesLen(returnValues); !ok || count != 2 {
		t.Fatalf("return Values = %d, %v", count, ok)
	}
	for i := 0; i < 2; i++ {
		value := valueAt(t, p, returnValues, i)
		if _, _, ok := p.Read(value); !ok {
			t.Fatalf("return value %d is not a bound Read", i)
		}
	}
}

func TestBinderIdentityControlsShadowedRead(t *testing.T) {
	p := parseBindLower(t, `
local x = 1
do
  local x = 2
  return x
end
`)
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	cells := make([]program.Term, p.CellCount())
	for i := range cells {
		cells[i], _ = p.CellAt(i)
	}
	if len(cells) != 2 {
		t.Fatalf("cells = %d", len(cells))
	}
	outerRoots := bodyRoots(t, p, entry)
	if len(outerRoots) != 2 {
		t.Fatalf("outer Body = %v; want Bind then child Body", outerRoots)
	}
	innerBody := outerRoots[1]
	if _, ok := p.BodyLen(innerBody); !ok || innerBody == entry {
		t.Fatalf("nested Body = %v, %v; Entry = %v", innerBody, ok, entry)
	}
	innerRoots := bodyRoots(t, p, innerBody)
	if len(innerRoots) != 2 {
		t.Fatalf("inner Body = %v; want Bind then Return", innerRoots)
	}
	_, returnValues, ok := p.Return(innerRoots[1])
	if !ok {
		t.Fatalf("nested root is not Return: %v", innerRoots[1])
	}
	if owner, ok := p.Cell(cells[0]); !ok || owner != entry {
		t.Fatalf("outer Cell owner = %v, %v; want %v", owner, ok, entry)
	}
	if owner, ok := p.Cell(cells[1]); !ok || owner != innerBody {
		t.Fatalf("inner Cell owner = %v, %v; want %v", owner, ok, innerBody)
	}
	value := valueAt(t, p, returnValues, 0)
	_, cell, ok := p.Read(value)
	if !ok || cell != cells[1] || cell == cells[0] {
		t.Fatalf("shadowed Read cell = %v, want inner %v", cell, cells[1])
	}
}

func TestLocalInitializerReadsOuterBindingBeforeNewCellExists(t *testing.T) {
	p := parseBindLower(t, `local x = 1
local x = x
return x`)
	roots := entryRoots(t, p)
	if len(roots) != 3 {
		t.Fatalf("Entry roots = %v, want two Bind roots and Return", roots)
	}
	outer := boundCell(t, p, roots[0], 0)
	inner := boundCell(t, p, roots[1], 0)
	if outer == inner {
		t.Fatal("shadowing declaration reused its outer Cell")
	}
	_, values, ok := p.Bind(roots[1])
	if !ok {
		t.Fatal("second root is not Bind")
	}
	read := valueAt(t, p, values, 0)
	_, source, ok := p.Read(read)
	if !ok || source != outer || source == inner {
		t.Fatalf("initializer Read source = %v, want outer Cell %v", source, outer)
	}
}

func TestParallelAttributeAssignmentPreservesBothTargetLenses(t *testing.T) {
	p := parseBindLower(t, `local t = {}
local k = 1
t[k], t[2] = 3, 4`)
	roots := entryRoots(t, p)
	if len(roots) != 3 {
		t.Fatalf("Entry roots = %v, want two Bind roots and Assign", roots)
	}
	assign := roots[2]
	if count, ok := p.AssignLen(assign); !ok || count != 2 {
		t.Fatalf("AssignLen = %d, %v; want 2", count, ok)
	}
	first := mustTarget(t, p, assign, 0)
	second := mustTarget(t, p, assign, 1)
	_, _, firstSource, firstKind, _, ok := p.Lens(first)
	if !ok || firstKind != program.FieldKey || firstSource == 0 {
		t.Fatalf("first target Lens = source %v kind %v ok %v", firstSource, firstKind, ok)
	}
	_, _, secondSource, secondKind, _, ok := p.Lens(second)
	if !ok || secondKind != program.FieldExact || secondSource == 0 {
		t.Fatalf("second target Lens = source %v kind %v ok %v", secondSource, secondKind, ok)
	}
}

func mustTarget(t *testing.T, p *program.Program, assign program.Term, index int) program.Term {
	t.Helper()
	target, ok := p.Target(assign, index)
	if !ok {
		t.Fatalf("Target(%v, %d) failed", assign, index)
	}
	return target
}

func TestBodyTailIsStructuralForTerminalAndNonterminalBodies(t *testing.T) {
	p := parseBindLower(t, `do local x = 1 end`)
	roots := entryRoots(t, p)
	if len(roots) != 1 {
		t.Fatalf("Entry roots = %v, want child Body", roots)
	}
	child := roots[0]
	childRoots := bodyRoots(t, p, child)
	if len(childRoots) != 1 {
		t.Fatalf("do Body roots = %v, want Bind", childRoots)
	}

	p = parseBindLower(t, `do return 1 end`)
	child = entryRoots(t, p)[0]
	childRoots = bodyRoots(t, p, child)
	if len(childRoots) != 1 {
		t.Fatalf("terminal do Body roots = %v, want only Return", childRoots)
	}
	if _, _, ok := p.Return(childRoots[0]); !ok {
		t.Fatal("terminal child Body did not retain Return")
	}
}

func TestIfConditionIsOneParentOccurrenceAndArmsKeepSourceSpans(t *testing.T) {
	p := parseBindLower(t, `local condition
if condition() then
  local x = 1
else
  local x = 2
end
return 3`)
	entry, _ := p.Entry()
	roots := entryRoots(t, p)
	if len(roots) != 3 {
		t.Fatalf("Entry roots = %v; want Bind, Branch, Return", roots)
	}
	owner, condition, whenTrue, whenFalse := mustBranch(t, p, roots[1])
	if owner != entry || whenTrue == whenFalse {
		t.Fatalf(
			"Branch owner/arms = %v %v/%v; want Entry %v and distinct arms",
			owner,
			whenTrue,
			whenFalse,
			entry,
		)
	}
	if p.CallCount() != 1 || condition == 0 {
		t.Fatalf("condition Calls = %d, condition %v; want exactly one", p.CallCount(), condition)
	}
	callOwner, _, _, _, _, ok := p.Call(condition)
	if !ok || callOwner != entry {
		t.Fatalf("Branch condition = Call owner %v ok %v; want Entry %v", callOwner, ok, entry)
	}
	if span, ok := p.Span(roots[1]); !ok ||
		span.StartLine != 2 || span.StartCol != 1 || span.EndLine != 6 {
		t.Fatalf("Branch span = %#v, %v", span, ok)
	}
	if span, ok := p.Span(whenTrue); !ok ||
		span.StartLine != 3 || span.StartCol != 3 || span.EndLine != 3 {
		t.Fatalf("Then Body span = %#v, %v", span, ok)
	}
	if span, ok := p.Span(whenFalse); !ok ||
		span.StartLine != 5 || span.StartCol != 3 || span.EndLine != 5 {
		t.Fatalf("Else Body span = %#v, %v", span, ok)
	}
	for _, arm := range []struct {
		name string
		body program.Term
	}{
		{"Then", whenTrue},
		{"Else", whenFalse},
	} {
		armRoots := bodyRoots(t, p, arm.body)
		if len(armRoots) != 1 {
			t.Fatalf("%s roots = %v; want Bind", arm.name, armRoots)
		}
	}
}

func TestEmptyIfArmsRemainDistinctOwnedBodies(t *testing.T) {
	p := parseBindLower(t, `if true then
else
end`)
	entry, _ := p.Entry()
	roots := entryRoots(t, p)
	if len(roots) != 1 {
		t.Fatalf("Entry roots = %v; want Branch", roots)
	}
	owner, _, whenTrue, whenFalse := mustBranch(t, p, roots[0])
	if owner != entry || whenTrue == whenFalse || whenTrue == entry || whenFalse == entry {
		t.Fatalf(
			"Branch owner/arms = %v %v/%v; want Entry %v and two child Bodies",
			owner,
			whenTrue,
			whenFalse,
			entry,
		)
	}
	for _, root := range roots {
		if root == whenTrue || root == whenFalse {
			t.Fatalf("Branch arm Body %v is also an Entry root", root)
		}
	}
	trueRoots := bodyRoots(t, p, whenTrue)
	falseRoots := bodyRoots(t, p, whenFalse)
	if len(trueRoots) != 0 || len(falseRoots) != 0 {
		t.Fatalf("empty arm roots = %v / %v; want structural tails only", trueRoots, falseRoots)
	}
}

func TestIfArmScopesAreIsolatedAndRestoreTheOuterBinding(t *testing.T) {
	p := parseBindLower(t, `local x = 0
if true then
  local x = 1
  x = x
else
  local x = 2
  x = x
end
x = x`)
	entry, _ := p.Entry()
	roots := entryRoots(t, p)
	if len(roots) != 3 {
		t.Fatalf("Entry roots = %v; want Bind, Branch, Assign", roots)
	}
	outerCell := boundCell(t, p, roots[0], 0)
	_, _, whenTrue, whenFalse := mustBranch(t, p, roots[1])
	armCells := make([]program.Term, 0, 2)
	for _, arm := range []struct {
		name string
		body program.Term
	}{
		{"Then", whenTrue},
		{"Else", whenFalse},
	} {
		armRoots := bodyRoots(t, p, arm.body)
		if len(armRoots) != 2 {
			t.Fatalf("%s roots = %v; want Bind and Assign", arm.name, armRoots)
		}
		cell := boundCell(t, p, armRoots[0], 0)
		armCells = append(armCells, cell)
		if owner, ok := p.Cell(cell); !ok || owner != arm.body {
			t.Fatalf("%s Cell owner = %v, %v; want %v", arm.name, owner, ok, arm.body)
		}
		assignOwner, assigned, ok := p.Assign(armRoots[1])
		if !ok || assignOwner != arm.body || mustTarget(t, p, armRoots[1], 0) != cell {
			t.Fatalf("%s Assign owner/target = %v/%v, %v", arm.name, assignOwner, cell, ok)
		}
		read := valueAt(t, p, assigned, 0)
		if readOwner, source, ok := p.Read(read); !ok || readOwner != arm.body || source != cell {
			t.Fatalf("%s Read owner/source = %v/%v, %v; want %v/%v", arm.name, readOwner, source, ok, arm.body, cell)
		}
	}
	if armCells[0] == armCells[1] || armCells[0] == outerCell || armCells[1] == outerCell {
		t.Fatalf("shadow Cells = outer %v arms %v; want three identities", outerCell, armCells)
	}
	postOwner, postValues, ok := p.Assign(roots[2])
	if !ok || postOwner != entry || mustTarget(t, p, roots[2], 0) != outerCell {
		t.Fatalf("post-If Assign owner/target = %v/%v, %v; want %v/%v", postOwner, mustTarget(t, p, roots[2], 0), ok, entry, outerCell)
	}
	postRead := valueAt(t, p, postValues, 0)
	if readOwner, source, ok := p.Read(postRead); !ok || readOwner != entry || source != outerCell {
		t.Fatalf("post-If Read owner/source = %v/%v, %v; want %v/%v", readOwner, source, ok, entry, outerCell)
	}
}

func elseIfSource(depth int) string {
	var source strings.Builder
	source.WriteString("if false then\n")
	for level := 1; level < depth; level++ {
		source.WriteString("elseif false then\n")
	}
	source.WriteString("else\nend")
	return source.String()
}

func TestDeepElseIfConditionsStayInLazyFalseBodiesAndSourceOrder(t *testing.T) {
	const depth = 512
	p := parseBindLower(t, elseIfSource(depth))
	if p.BranchCount() != depth || p.BoolCount() != depth {
		t.Fatalf("families = Branch %d Bool %d; want %d each", p.BranchCount(), p.BoolCount(), depth)
	}
	for level := 0; level < depth; level++ {
		condition, ok := p.BoolAt(level)
		if !ok {
			t.Fatalf("BoolAt(%d) failed", level)
		}
		span, ok := p.Span(condition)
		if !ok || span.StartLine != level+1 {
			t.Fatalf("condition %d span = %#v, %v; want line %d", level, span, ok, level+1)
		}
	}

	entry, _ := p.Entry()
	roots := entryRoots(t, p)
	branch := roots[0]
	owner := entry
	for level := 0; level < depth; level++ {
		branchOwner, condition, whenTrue, whenFalse := mustBranch(t, p, branch)
		if branchOwner != owner {
			t.Fatalf("level %d Branch owner = %v; want %v", level, branchOwner, owner)
		}
		conditionOwner, value, ok := p.Bool(condition)
		if !ok || value || conditionOwner != owner {
			t.Fatalf(
				"level %d condition = owner %v value %v ok %v; want %v false",
				level,
				conditionOwner,
				value,
				ok,
				owner,
			)
		}
		trueRoots := bodyRoots(t, p, whenTrue)
		if len(trueRoots) != 0 {
			t.Fatalf("level %d Then roots = %v; want structural tail only", level, trueRoots)
		}
		falseRoots := bodyRoots(t, p, whenFalse)
		if level == depth-1 {
			if len(falseRoots) != 0 {
				t.Fatalf("terminal Else roots = %v; want structural tail only", falseRoots)
			}
			break
		}
		if len(falseRoots) != 1 {
			t.Fatalf("level %d Else roots = %v; want nested Branch", level, falseRoots)
		}
		branch = falseRoots[0]
		owner = whenFalse
	}
}

func TestIfTerminationKeepsOnlyReachableParentContinuation(t *testing.T) {
	for _, source := range []string{
		`if true then return 1 end
return 2`,
		`local x = 0
if true then
  return 1
else
  x = 2
end
x = 3
return x`,
	} {
		p := parseBindLower(t, source)
		roots := entryRoots(t, p)
		var branch program.Term
		for _, root := range roots {
			if _, _, _, _, ok := p.Branch(root); ok {
				branch = root
				break
			}
		}
		if branch == 0 {
			t.Fatalf("source has no parent Branch:\n%s", source)
		}
		_, _, whenTrue, whenFalse := mustBranch(t, p, branch)
		trueRoots := bodyRoots(t, p, whenTrue)
		if len(trueRoots) != 1 {
			t.Fatalf("Then roots = %v; want one Return for source:\n%s", trueRoots, source)
		}
		if _, _, ok := p.Return(trueRoots[0]); !ok {
			t.Fatalf("Then root is not Return for source:\n%s", source)
		}
		_ = bodyRoots(t, p, whenFalse)
		if _, _, ok := p.Return(roots[len(roots)-1]); !ok {
			t.Fatalf("source has no trailing Return root:\n%s\nroots %v", source, roots)
		}
	}
}

func TestExhaustiveTerminalIfRetainsTrailingAuthoredStatement(t *testing.T) {
	for _, source := range []string{
		`if true then
  return 1
else
  return 2
end
local unreachable = 3`,
		`if true then
  return 1
elseif false then
  return 2
else
  return 3
end
local unreachable = 4`,
	} {
		p := parseBindLower(t, source)
		entry, _ := p.Entry()
		roots := bodyRoots(t, p, entry)
		if len(roots) != 2 {
			t.Fatalf("terminal Entry roots = %v; want Branch and authored Bind", roots)
		}
		if _, _, _, _, ok := p.Branch(roots[0]); !ok {
			t.Fatalf("first root is not Branch: %v", roots[0])
		}
		if _, ok := p.BindLen(roots[1]); !ok {
			t.Fatalf("trailing authored root is not Bind: %v", roots[1])
		}
	}
}

func TestExhaustiveTerminalIfRetainsOnlyAuthoredReturns(t *testing.T) {
	p := parseBindLower(t, `if true then
  return 1
elseif false then
  return 2
else
  return 3
end`)
	roots := entryRoots(t, p)
	if len(roots) != 1 {
		t.Fatalf("Entry roots = %v; want only the exhaustive Branch", roots)
	}
	branch := roots[0]
	for level := 0; level < 2; level++ {
		_, _, whenTrue, whenFalse := mustBranch(t, p, branch)
		trueRoots := bodyRoots(t, p, whenTrue)
		if len(trueRoots) != 1 {
			t.Fatalf("level %d Then roots = %v; want one Return", level, trueRoots)
		}
		if _, _, ok := p.Return(trueRoots[0]); !ok {
			t.Fatalf("level %d Then root is not Return", level)
		}
		falseRoots := bodyRoots(t, p, whenFalse)
		if level == 0 {
			if len(falseRoots) != 1 {
				t.Fatalf("outer Else roots = %v; want only nested Branch", falseRoots)
			}
			branch = falseRoots[0]
			continue
		}
		if len(falseRoots) != 1 {
			t.Fatalf("terminal Else roots = %v; want one Return", falseRoots)
		}
		if _, _, ok := p.Return(falseRoots[0]); !ok {
			t.Fatal("terminal Else root is not Return")
		}
	}
}

func TestTerminalIfRetainsTrailingAuthoredAssignment(t *testing.T) {
	p := parseBindLower(t, `
local x = 0
if true then
	return 1
else
	return 2
end
x = 3
`)
	entry, _ := p.Entry()
	roots := bodyRoots(t, p, entry)
	if len(roots) != 3 {
		t.Fatalf("Entry roots = %v; want Bind, Branch, authored Assign", roots)
	}
	if _, ok := p.AssignLen(roots[2]); !ok {
		t.Fatalf("trailing root is not Assign: %v", roots[2])
	}
}

func TestFourAndEightThousandElseIfArmsHaveExactLinearShape(t *testing.T) {
	for _, depth := range []int{4 * 1024, 8 * 1024} {
		source := elseIfSource(depth)
		stmts, err := parse.ParseString(source, "deep-elseif.lua")
		if err != nil {
			t.Fatal(err)
		}
		binding := bind.BindChunk(stmts, bind.Options{})
		first, err := programlower.Lower("deep-elseif.lua", stmts, binding)
		if err != nil {
			t.Fatal(err)
		}
		second, err := programlower.Lower("deep-elseif.lua", stmts, binding)
		if err != nil {
			t.Fatal(err)
		}
		if first.BranchCount() != depth ||
			first.BodyCount() != 2*depth+1 ||
			first.BoolCount() != depth ||
			first.ValuesCount() != 0 ||
			first.TermCount() != 4*depth+1 {
			t.Fatalf(
				"%d-chain families: branches=%d bodies=%d bools=%d values=%d terms=%d",
				depth,
				first.BranchCount(),
				first.BodyCount(),
				first.BoolCount(),
				first.ValuesCount(),
				first.TermCount(),
			)
		}
		if second.TermCount() != first.TermCount() ||
			second.BranchCount() != first.BranchCount() ||
			second.BodyCount() != first.BodyCount() {
			t.Fatalf("%d-chain fresh lowering changed family counts", depth)
		}
		firstEntry, _ := first.Entry()
		secondEntry, _ := second.Entry()
		firstRoots := bodyRoots(t, first, firstEntry)
		secondRoots := bodyRoots(t, second, secondEntry)
		if firstEntry != secondEntry ||
			len(firstRoots) != len(secondRoots) ||
			firstRoots[0] != secondRoots[0] {
			t.Fatalf("%d-chain fresh lowering changed Entry identity or roots", depth)
		}
		for i := 0; i < depth; i++ {
			firstBranch, _ := first.BranchAt(i)
			secondBranch, _ := second.BranchAt(i)
			firstOwner, firstCondition, firstTrue, firstFalse := mustBranch(t, first, firstBranch)
			secondOwner, secondCondition, secondTrue, secondFalse := mustBranch(t, second, secondBranch)
			if firstBranch != secondBranch ||
				firstOwner != secondOwner ||
				firstCondition != secondCondition ||
				firstTrue != secondTrue ||
				firstFalse != secondFalse {
				t.Fatalf("%d-chain Branch %d changed across fresh lowerings", depth, i)
			}
		}
	}
}

func TestSyntheticListKeyHasGeneratedCoordinates(t *testing.T) {
	p := parseBindLower(t, `return {"first"}`)
	if p.TableCount() != 1 {
		t.Fatalf("TableCount = %d, want 1", p.TableCount())
	}
	table, _ := p.TableAt(0)
	key, _, kind, _, ok := p.Field(table, 0)
	if !ok {
		t.Fatal("missing table field")
	}
	if kind != program.FieldList {
		t.Fatalf("synthetic list kind = %v, want FieldList", kind)
	}
	span, ok := p.Span(key)
	if !ok || span.File != "fixture.lua" ||
		span.StartLine != 0 || span.StartCol != 0 || span.EndLine != 0 || span.EndCol != 0 {
		t.Fatalf("synthetic key span = %#v, %v", span, ok)
	}
}

func TestInterleavedListKeysKeepOrdinalsAndOnlyFinalListStaysOpen(t *testing.T) {
	p := parseBindLower(t, `local f
return {1, named = 2, 3, f()}`)
	roots := entryRoots(t, p)
	_, returned, ok := p.Return(roots[1])
	if !ok {
		t.Fatal("Entry return missing")
	}
	table := valueAt(t, p, returned, 0)
	if count, ok := p.TableLen(table); !ok || count != 4 {
		t.Fatalf("TableLen = %d, %v; want 4", count, ok)
	}
	for i, want := range []struct {
		kind    program.FieldKind
		ordinal int64
	}{
		{program.FieldList, 1},
		{program.FieldName, 0},
		{program.FieldList, 2},
		{program.FieldList, 3},
	} {
		source, values, kind, _, ok := p.Field(table, i)
		if !ok || kind != want.kind {
			t.Fatalf("Field(%d) kind = %v, ok %v; want %v", i, kind, ok, want.kind)
		}
		if want.kind == program.FieldList {
			_, ordinal, _, ok := p.List(source)
			if !ok || ordinal != want.ordinal {
				t.Fatalf("Field(%d) list ordinal = %d, %v; want %d", i, ordinal, ok, want.ordinal)
			}
		}
		if i != 3 && valuesTail(t, p, values) != 0 {
			t.Fatalf("non-final field %d retained an open tail", i)
		}
		if i == 3 && valuesTail(t, p, values) == 0 {
			t.Fatal("final list field did not retain its open call tail")
		}
	}
}

func TestTypedLocalFailsUntilAuthoredTypesLand(t *testing.T) {
	stmts, err := parse.ParseString(`local x: number = 1`, "typed.lua")
	if err != nil {
		t.Fatal(err)
	}
	_, err = programlower.Lower("typed.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), "unsupported declared type for local slot 0") {
		t.Fatalf("typed local error = %v", err)
	}
}

func TestUnknownTableKeySyntaxFailsExplicitly(t *testing.T) {
	key := &ast.StringExpr{Value: "key"}
	value := &ast.NumberExpr{Value: "1"}
	table := &ast.TableExpr{Fields: []*ast.Field{{
		Key:       key,
		KeySyntax: ast.AttrKeyUnknown,
		Value:     value,
	}}}
	stmts := []ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{table}}}
	_, err := programlower.Lower("manual.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), "unknown key syntax") {
		t.Fatalf("unknown table key syntax error = %v", err)
	}
}

func TestNilKeysRemainExactAuthoredOccurrences(t *testing.T) {
	p := parseBindLower(t, `return {[nil] = 1}`)
	table, _ := p.TableAt(0)
	key, _, kind, _, ok := p.Field(table, 0)
	if _, nilKey := p.Nil(key); !ok || kind != program.FieldExact || !nilKey {
		t.Fatalf("nil table field = key %v kind %v ok %v", key, kind, ok)
	}

	p = parseBindLower(t, `
local t = {}
t[nil] = 1
return t
`)
	if p.LensExactCount() != 1 || p.LensKeyCount() != 0 {
		t.Fatalf("nil target lenses exact=%d dynamic=%d", p.LensExactCount(), p.LensKeyCount())
	}
	lens, _ := p.LensExactAt(0)
	_, _, key, kind, _, ok = p.Lens(lens)
	if _, nilKey := p.Nil(key); !ok || kind != program.FieldExact || !nilKey {
		t.Fatalf("nil assignment Lens = key %v kind %v ok %v", key, kind, ok)
	}
}

func loweredReturnValue(t *testing.T, source string) (*program.Program, program.Term) {
	t.Helper()
	p := parseBindLower(t, source)
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	bodies := []program.Term{entry}
	for len(bodies) != 0 {
		body := bodies[len(bodies)-1]
		bodies = bodies[:len(bodies)-1]
		roots := bodyRoots(t, p, body)
		for _, root := range roots {
			if _, ok := p.BodyLen(root); ok {
				bodies = append(bodies, root)
				continue
			}
			_, values, ok := p.Return(root)
			if !ok {
				continue
			}
			value := valueAt(t, p, values, 0)
			return p, value
		}
	}
	t.Fatal("Entry-reachable Bodies have no Return")
	return nil, 0
}

func TestEveryClosedUnaryOperatorLowers(t *testing.T) {
	tests := []struct {
		source string
		want   program.UnaryOp
	}{
		{`return -1`, program.UnaryNeg},
		{`return not true`, program.UnaryNot},
		{`return #"x"`, program.UnaryLen},
		{`return ~1`, program.UnaryBitNot},
	}
	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			p, term := loweredReturnValue(t, test.source)
			_, op, operand, ok := p.Unary(term)
			if !ok || op != test.want || operand == 0 {
				t.Fatalf("Unary = op %v operand %v ok %v", op, operand, ok)
			}
		})
	}
}

func TestEveryClosedBinaryOperatorLowers(t *testing.T) {
	tests := []struct {
		source string
		want   program.BinaryOp
	}{
		{`return 1 + 2`, program.BinaryAdd},
		{`return 1 - 2`, program.BinarySub},
		{`return 1 * 2`, program.BinaryMul},
		{`return 1 / 2`, program.BinaryDiv},
		{`return 1 // 2`, program.BinaryIDiv},
		{`return 1 % 2`, program.BinaryMod},
		{`return 1 ^ 2`, program.BinaryPow},
		{`return "a" .. "b"`, program.BinaryConcat},
		{`return 1 & 2`, program.BinaryBitAnd},
		{`return 1 | 2`, program.BinaryBitOr},
		{`return 1 ~ 2`, program.BinaryBitXor},
		{`return 1 << 2`, program.BinaryShiftLeft},
		{`return 1 >> 2`, program.BinaryShiftRight},
		{`return 1 == 2`, program.BinaryEqual},
		{`return 1 ~= 2`, program.BinaryNotEqual},
		{`return 1 < 2`, program.BinaryLess},
		{`return 1 <= 2`, program.BinaryLessEqual},
		{`return 1 > 2`, program.BinaryGreater},
		{`return 1 >= 2`, program.BinaryGreaterEqual},
	}
	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			p, term := loweredReturnValue(t, test.source)
			_, op, left, right, ok := p.Binary(term)
			if !ok || op != test.want || left == 0 || right == 0 {
				t.Fatalf("Binary = op %v left %v right %v ok %v", op, left, right, ok)
			}
			span, ok := p.Span(term)
			if !ok || span.File != "fixture.lua" || span.StartLine != 1 ||
				span.StartCol != 8 || span.EndLine != 1 || span.EndCol == 0 {
				t.Fatalf("Binary span = %#v, %v", span, ok)
			}
		})
	}
}

func TestLogicalSelectionKeepsRightSideConditional(t *testing.T) {
	for _, test := range []struct {
		source string
		want   program.SelectOp
	}{
		{`return false and (1 + 2)`, program.SelectAnd},
		{`return true or (1 + 2)`, program.SelectOr},
	} {
		t.Run(test.source, func(t *testing.T) {
			p, term := loweredReturnValue(t, test.source)
			_, op, left, right, ok := p.Select(term)
			if !ok || op != test.want || left == 0 || right == 0 {
				t.Fatalf("Select = op %v left %v right %v ok %v", op, left, right, ok)
			}
			if _, _, _, _, ok := p.Binary(right); !ok {
				t.Fatalf("conditional right operand is not retained Binary: %v", right)
			}
		})
	}
}

func TestAttributeReadsPreserveExactAndDynamicLenses(t *testing.T) {
	p := parseBindLower(t, "local t = {}\nlocal k = \"name\"\nreturn t.name, t[nil], t[k]")
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	roots := bodyRoots(t, p, entry)
	if len(roots) != 3 {
		t.Fatalf("Entry roots = %v", roots)
	}
	_, returned, ok := p.Return(roots[2])
	if !ok {
		t.Fatalf("Entry root 2 is not Return: %v", roots[2])
	}
	for i, wantKind := range []program.FieldKind{program.FieldName, program.FieldExact, program.FieldKey} {
		read := valueAt(t, p, returned, i)
		_, lens, ok := p.Read(read)
		if !ok {
			t.Fatalf("return value %d is not Read", i)
		}
		_, base, key, kind, _, ok := p.Lens(lens)
		if !ok || base == 0 || key == 0 || kind != wantKind {
			t.Fatalf("Lens %d = base %v key %v kind %v ok %v", i, base, key, kind, ok)
		}
		if i == 0 {
			_, value, _, ok := p.Name(key)
			if !ok || value != "name" {
				t.Fatalf("dot key = %q, %v", value, ok)
			}
		}
		if i == 1 {
			if _, ok := p.Nil(key); !ok {
				t.Fatalf("nil index key = %v", key)
			}
		}
		span, ok := p.Span(read)
		if !ok || span.StartLine != 3 || span.StartCol == 0 || span.EndCol == 0 {
			t.Fatalf("Read %d span = %#v, %v", i, span, ok)
		}
	}
}

func TestInvalidOperatorAndUnknownAttributeSyntaxFail(t *testing.T) {
	badOp := &ast.ArithmeticOpExpr{
		Operator: "??",
		Lhs:      &ast.NumberExpr{Value: "1"},
		Rhs:      &ast.NumberExpr{Value: "2"},
	}
	stmts := []ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{badOp}}}
	_, err := programlower.Lower("manual.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), `unsupported arithmetic operator "??"`) {
		t.Fatalf("invalid operator error = %v", err)
	}

	attr := &ast.AttrGetExpr{
		Object:    &ast.StringExpr{Value: "base"},
		Key:       &ast.StringExpr{Value: "key"},
		KeySyntax: ast.AttrKeyUnknown,
	}
	stmts = []ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{attr}}}
	_, err = programlower.Lower("manual.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), "unknown key syntax") {
		t.Fatalf("unknown attribute syntax error = %v", err)
	}
}

func TestWhileConditionAndBreakHaveExactLoopOwnership(t *testing.T) {
	p := parseBindLower(t, `
local x = 1
while x do
	break
end
return x
`)
	entry, _ := p.Entry()
	roots := bodyRoots(t, p, entry)
	if len(roots) != 3 {
		t.Fatalf("Entry roots = %v; want Bind, Loop, Return", roots)
	}
	loop := roots[1]
	owner, body, condition, kind := mustLoop(t, p, loop)
	if owner != entry || kind != program.LoopWhile {
		t.Fatalf("While = owner %v kind %v; want %v/%v", owner, kind, entry, program.LoopWhile)
	}
	if count, ok := p.LoopCellCount(loop); !ok || count != 0 {
		t.Fatalf("While LoopCellCount = %d, %v; want 0", count, ok)
	}
	bindCell := boundCell(t, p, roots[0], 0)
	readOwner, source, ok := p.Read(condition)
	if !ok || readOwner != entry || source != bindCell {
		t.Fatalf("While condition = owner %v source %v ok %v", readOwner, source, ok)
	}
	bodyTerms := bodyRoots(t, p, body)
	if len(bodyTerms) != 1 {
		t.Fatalf("While Body roots = %v; want one Break", bodyTerms)
	}
	breakOwner, breakLoop, ok := p.Break(bodyTerms[0])
	if !ok || breakOwner != body || breakLoop != loop {
		t.Fatalf(
			"Break = owner %v loop %v ok %v; want %v/%v",
			breakOwner,
			breakLoop,
			ok,
			body,
			loop,
		)
	}
	if mu, ok := p.Mu(loop); ok || mu != 0 {
		t.Fatalf("terminal-only While Mu = %v, %v; want none", mu, ok)
	}
}

func TestRepeatConditionSharesBodyLocalsAndStructuralBackedge(t *testing.T) {
	p := parseBindLower(t, `
repeat
	local x = 1
until x
return 2
`)
	loop, _ := p.LoopAt(0)
	entry, body, condition, kind := mustLoop(t, p, loop)
	programEntry, _ := p.Entry()
	if entry != programEntry || kind != program.LoopRepeat {
		t.Fatalf("Repeat = owner %v kind %v; want %v/%v", entry, kind, programEntry, program.LoopRepeat)
	}
	bodyTerms := bodyRoots(t, p, body)
	if len(bodyTerms) != 1 {
		t.Fatalf("Repeat Body roots = %v; want Bind", bodyTerms)
	}
	cell := boundCell(t, p, bodyTerms[0], 0)
	readOwner, source, ok := p.Read(condition)
	if !ok || readOwner != body || source != cell {
		t.Fatalf("Repeat condition = owner %v source %v ok %v; want %v/%v", readOwner, source, ok, body, cell)
	}
	if mu, ok := p.Mu(loop); !ok || mu != loop {
		t.Fatalf("Repeat Mu = %v, %v; want self", mu, ok)
	}
}

func TestNumericForControlsAreFixedOnceAndDefaultStepIsSemantic(t *testing.T) {
	p := parseBindLower(t, `
for i = 1, 10 do
	local x = i
end
for j = 1, 10, 2 do
	local y = j
end
`)
	entry, _ := p.Entry()
	if p.LoopCount() != 2 || p.IntegerCount() != 5 {
		t.Fatalf("numeric families: loops=%d integers=%d; want 2/5", p.LoopCount(), p.IntegerCount())
	}
	for index, wantCount := range []int{2, 3} {
		loop, _ := p.LoopAt(index)
		owner, body, values, kind := mustLoop(t, p, loop)
		if owner != entry || kind != program.LoopNumericFor {
			t.Fatalf("NumericFor %d = owner %v kind %v", index, owner, kind)
		}
		if count, ok := p.ValuesLen(values); !ok || count != wantCount {
			t.Fatalf("NumericFor %d control count = %d, %v; want %d", index, count, ok, wantCount)
		}
		valuesOwner, tail, ok := p.Values(values)
		if !ok || valuesOwner != entry || tail != 0 {
			t.Fatalf("NumericFor %d control = owner %v tail %v ok %v", index, valuesOwner, tail, ok)
		}
		if count, ok := p.LoopCellCount(loop); !ok || count != 1 {
			t.Fatalf("NumericFor %d Cell count = %d, %v", index, count, ok)
		}
		cell, _ := p.LoopCell(loop, 0)
		if cellOwner, ok := p.Cell(cell); !ok || cellOwner != body {
			t.Fatalf("NumericFor %d Cell owner = %v, %v; want %v", index, cellOwner, ok, body)
		}
		foundRead := false
		for readIndex := 0; readIndex < p.ReadCount(); readIndex++ {
			read, _ := p.ReadAt(readIndex)
			readOwner, source, ok := p.Read(read)
			if ok && readOwner == body && source == cell {
				foundRead = true
				break
			}
		}
		if !foundRead {
			t.Fatalf("NumericFor %d iteration Cell is not visible in its Body", index)
		}
	}
}

func TestNumericForCallControlsStayOrderedFixedAndClosed(t *testing.T) {
	p := parseBindLower(t, `
return function(init, limit, step)
	for i = init(), limit(), step() do
	end
end
`)
	loop, _ := p.LoopAt(0)
	_, _, values, kind := mustLoop(t, p, loop)
	if kind != program.LoopNumericFor {
		t.Fatalf("loop kind = %v; want NumericFor", kind)
	}
	if count, ok := p.ValuesLen(values); !ok || count != 3 {
		t.Fatalf("numeric call controls = %d, %v; want 3", count, ok)
	}
	if tail := valuesTail(t, p, values); tail != 0 {
		t.Fatalf("numeric call control tail = %v; want closed", tail)
	}
	if p.CallCount() != 3 {
		t.Fatalf("CallCount = %d; want three once-evaluated controls", p.CallCount())
	}
	for i := 0; i < 3; i++ {
		call, _ := p.CallAt(i)
		if got := valueAt(t, p, values, i); got != call {
			t.Fatalf("numeric control %d = %v; want source-order Call %v", i, got, call)
		}
	}
}

func TestGenericForControlKeepsOpenTailAndAllIteratorCells(t *testing.T) {
	p := parseBindLower(t, `
return function(iter)
	for k, v, w in 1, iter() do
		local a, b, c = k, v, w
	end
end
`)
	loop, _ := p.LoopAt(0)
	owner, body, values, kind := mustLoop(t, p, loop)
	if kind != program.LoopGenericFor {
		t.Fatalf("GenericFor kind = %v", kind)
	}
	if count, ok := p.ValuesLen(values); !ok || count != 1 {
		t.Fatalf("GenericFor fixed controls = %d, %v; want 1", count, ok)
	}
	valuesOwner, tail, ok := p.Values(values)
	if !ok || valuesOwner != owner || tail == 0 {
		t.Fatalf("GenericFor control = owner %v tail %v ok %v", valuesOwner, tail, ok)
	}
	callOwner, _, _, _, _, ok := p.Call(tail)
	if !ok || callOwner != owner || p.CallCount() != 1 {
		t.Fatalf("GenericFor open tail = owner %v ok %v calls %d", callOwner, ok, p.CallCount())
	}
	if count, ok := p.LoopCellCount(loop); !ok || count != 3 {
		t.Fatalf("GenericFor Cell count = %d, %v; want 3", count, ok)
	}
	for i := 0; i < 3; i++ {
		cell, ok := p.LoopCell(loop, i)
		if !ok {
			t.Fatalf("GenericFor Cell %d missing", i)
		}
		if cellOwner, ok := p.Cell(cell); !ok || cellOwner != body {
			t.Fatalf("GenericFor Cell %d owner = %v, %v; want %v", i, cellOwner, ok, body)
		}
		foundRead := false
		for readIndex := 0; readIndex < p.ReadCount(); readIndex++ {
			read, _ := p.ReadAt(readIndex)
			readOwner, source, ok := p.Read(read)
			if ok && readOwner == body && source == cell {
				foundRead = true
				break
			}
		}
		if !foundRead {
			t.Fatalf("GenericFor Cell %d is not visible in its Body", i)
		}
	}
}

func TestGenericForClosedControlPreservesFixedSourceOrder(t *testing.T) {
	p := parseBindLower(t, `for k, v in 1, 2 do end`)
	loop, _ := p.LoopAt(0)
	entry, _, values, kind := mustLoop(t, p, loop)
	programEntry, _ := p.Entry()
	if kind != program.LoopGenericFor || entry != programEntry {
		t.Fatalf("GenericFor = owner %v kind %v", entry, kind)
	}
	if count, ok := p.ValuesLen(values); !ok || count != 2 {
		t.Fatalf("closed GenericFor values = %d, %v; want 2", count, ok)
	}
	if tail := valuesTail(t, p, values); tail != 0 {
		t.Fatalf("closed GenericFor tail = %v", tail)
	}
	for i, want := range []int64{1, 2} {
		_, got, ok := p.Integer(valueAt(t, p, values, i))
		if !ok || got != want {
			t.Fatalf("closed GenericFor control %d = %d, %v; want %d", i, got, ok, want)
		}
	}
}

func TestRepeatBreakAndReturnFlowsStayDistinctAcrossNestedControl(t *testing.T) {
	p := parseBindLower(t, `
return function(flag)
	repeat
		do
			if flag then
				break
			else
				return 1
			end
		end
	until true
	return 2
end
`)
	loop, _ := p.LoopAt(0)
	_, body, _, kind := mustLoop(t, p, loop)
	if kind != program.LoopRepeat {
		t.Fatalf("loop kind = %v; want Repeat", kind)
	}
	if _, ok := p.Mu(loop); ok {
		t.Fatal("fully terminal Repeat Body unexpectedly has Mu")
	}
	function, _ := p.FunctionAt(0)
	_, functionBody, _, ok := p.Function(function)
	if !ok {
		t.Fatal("missing outer Function")
	}
	functionRoots := bodyRoots(t, p, functionBody)
	if len(functionRoots) != 2 || functionRoots[0] != loop {
		t.Fatalf("Function roots = %v; want Repeat Loop then Return", functionRoots)
	}
	var foundBreak bool
	for i := 0; i < p.BreakCount(); i++ {
		term, _ := p.BreakAt(i)
		breakOwner, breakLoop, ok := p.Break(term)
		if ok && breakLoop == loop && breakOwner != body {
			foundBreak = true
		}
	}
	if !foundBreak {
		t.Fatal("nested Branch/Do Break did not resolve to Repeat")
	}

	nested, err := lowerSource(`
return function()
	repeat
		while true do
			break
		end
		return 1
	until true
	return 2
end
`)
	if err != nil {
		t.Fatalf("nested loop lowering failed: %v", err)
	}
	nestedFunction, _ := nested.FunctionAt(0)
	_, nestedBody, _, ok := nested.Function(nestedFunction)
	if !ok {
		t.Fatal("nested source has no Function")
	}
	nestedRoots := bodyRoots(t, nested, nestedBody)
	if len(nestedRoots) != 2 {
		t.Fatalf("nested Function roots = %v; want Repeat and unreachable Return", nestedRoots)
	}
	if _, _, _, kind := mustLoop(t, nested, nestedRoots[0]); kind != program.LoopRepeat {
		t.Fatalf("nested root 0 is not Repeat: %v", nestedRoots[0])
	}
	if _, _, ok := nested.Return(nestedRoots[1]); !ok {
		t.Fatalf("nested trailing root is not Return: %v", nestedRoots[1])
	}
}

func TestUnreachableBreakInsideRepeatCannotReviveReturnFlow(t *testing.T) {
	p := parseBindLower(t, `
return function()
	repeat
		do
			return 1
		end
		break
	until false
	return 2
end
`)
	loop, _ := p.LoopAt(0)
	_, repeatBody, condition, kind := mustLoop(t, p, loop)
	if kind != program.LoopRepeat {
		t.Fatalf("loop kind = %v; want Repeat", kind)
	}
	if conditionOwner, value, ok := p.Bool(condition); !ok || conditionOwner != repeatBody || value {
		t.Fatalf(
			"unreachable Repeat condition = owner %v value %v ok %v; want body-owned false",
			conditionOwner,
			value,
			ok,
		)
	}
	repeatRoots := bodyRoots(t, p, repeatBody)
	if len(repeatRoots) != 2 {
		t.Fatalf("Repeat roots = %v; want terminal Do and unreachable Break", repeatRoots)
	}
	breakOwner, breakLoop, ok := p.Break(repeatRoots[1])
	if !ok || breakOwner != repeatBody || breakLoop != loop {
		t.Fatalf("unreachable Break = owner %v loop %v ok %v", breakOwner, breakLoop, ok)
	}
	if _, ok := p.Mu(loop); ok {
		t.Fatal("return-terminal Repeat unexpectedly has Mu")
	}
	function, _ := p.FunctionAt(0)
	_, functionBody, _, ok := p.Function(function)
	if !ok {
		t.Fatal("missing Function")
	}
	functionRoots := bodyRoots(t, p, functionBody)
	if len(functionRoots) != 2 {
		t.Fatalf("Function roots = %v; want Loop and unreachable Return", functionRoots)
	}
}

func TestReturnOnlyRepeatRetainsConditionWithoutMu(t *testing.T) {
	p := parseBindLower(t, `repeat return 1 until true`)
	loop, _ := p.LoopAt(0)
	_, body, condition, kind := mustLoop(t, p, loop)
	if kind != program.LoopRepeat {
		t.Fatalf("loop kind = %v; want Repeat", kind)
	}
	conditionOwner, value, ok := p.Bool(condition)
	if !ok || conditionOwner != body || !value {
		t.Fatalf("Repeat condition = owner %v value %v ok %v", conditionOwner, value, ok)
	}
	if _, ok := p.Mu(loop); ok {
		t.Fatal("return-only Repeat unexpectedly has Mu")
	}
}

func TestBreakOutsideLoopAndAcrossFunctionBoundaryFailSeal(t *testing.T) {
	for _, source := range []string{
		`break`,
		`while true do return function() break end end`,
	} {
		_, err := lowerSource(source)
		if err == nil || !strings.Contains(err.Error(), "seal") {
			t.Fatalf("invalid break lowered for %q: %v", source, err)
		}
	}
}

func TestLoopMuRequiresReachableBodyTail(t *testing.T) {
	p := parseBindLower(t, `
while false do end
repeat break until true
`)
	whileLoop, _ := p.LoopAt(0)
	repeatLoop, _ := p.LoopAt(1)
	if mu, ok := p.Mu(whileLoop); !ok || mu != whileLoop {
		t.Fatalf("fallthrough While Mu = %v, %v; want self", mu, ok)
	}
	if mu, ok := p.Mu(repeatLoop); ok || mu != 0 {
		t.Fatalf("terminal Repeat Mu = %v, %v; want none", mu, ok)
	}
}

func TestGotoTargetsExactPredeclaredLabelAtFinalVoidCursor(t *testing.T) {
	p := parseBindLower(t, `
goto done
local dead = 1
::done::
`)
	entry, _ := p.Entry()
	if p.LabelCount() != 1 || p.GotoCount() != 1 {
		t.Fatalf("control families: labels=%d gotos=%d; want 1/1", p.LabelCount(), p.GotoCount())
	}
	label, _ := p.LabelAt(0)
	labelOwner, cursor, ok := p.Label(label)
	if !ok || labelOwner != entry || cursor != 2 {
		t.Fatalf("Label = owner %v cursor %d ok %v; want %v/2/true", labelOwner, cursor, ok, entry)
	}
	jump, _ := p.GotoAt(0)
	jumpOwner, target, ok := p.Goto(jump)
	if !ok || jumpOwner != entry || target != label {
		t.Fatalf("Goto = owner %v target %v ok %v; want %v/%v/true", jumpOwner, target, ok, entry, label)
	}
	roots := bodyRoots(t, p, entry)
	if len(roots) != 2 || roots[0] != jump {
		t.Fatalf("Entry roots = %v; want Goto and Bind with Label kept out of roots", roots)
	}
	if _, ok := p.BindLen(roots[1]); !ok {
		t.Fatalf("Entry root 1 is not Bind: %v", roots[1])
	}
}

func TestConsecutiveLabelsShareOneStructuralCursor(t *testing.T) {
	p := parseBindLower(t, `
local x = 1
::first::
::second::
return x
`)
	entry, _ := p.Entry()
	if p.LabelCount() != 2 {
		t.Fatalf("LabelCount = %d; want 2", p.LabelCount())
	}
	for index := 0; index < 2; index++ {
		label, _ := p.LabelAt(index)
		owner, cursor, ok := p.Label(label)
		if !ok || owner != entry || cursor != 1 {
			t.Fatalf("LabelAt(%d) = owner %v cursor %d ok %v; want %v/1/true", index, owner, cursor, ok, entry)
		}
	}
	roots := bodyRoots(t, p, entry)
	if len(roots) != 2 {
		t.Fatalf("Entry roots = %v; want Bind and Return only", roots)
	}
	if _, ok := p.BindLen(roots[0]); !ok {
		t.Fatalf("Entry root 0 is not Bind: %v", roots[0])
	}
	if _, _, ok := p.Return(roots[1]); !ok {
		t.Fatalf("Entry root 1 is not Return: %v", roots[1])
	}
}

func TestNestedGotoTargetsPredeclaredAncestorLabel(t *testing.T) {
	p := parseBindLower(t, `
do
	goto done
end
::done::
return 1
`)
	entry, _ := p.Entry()
	label, _ := p.LabelAt(0)
	labelOwner, cursor, ok := p.Label(label)
	if !ok || labelOwner != entry || cursor != 1 {
		t.Fatalf("ancestor Label = owner %v cursor %d ok %v; want %v/1/true", labelOwner, cursor, ok, entry)
	}
	roots := bodyRoots(t, p, entry)
	if len(roots) != 2 {
		t.Fatalf("Entry roots = %v; want Body and Return", roots)
	}
	child := roots[0]
	childRoots := bodyRoots(t, p, child)
	if len(childRoots) != 1 {
		t.Fatalf("nested Body roots = %v; want one Goto", childRoots)
	}
	jumpOwner, target, ok := p.Goto(childRoots[0])
	if !ok || jumpOwner != child || target != label {
		t.Fatalf("nested Goto = owner %v target %v ok %v; want %v/%v/true", jumpOwner, target, ok, child, label)
	}
}

func TestBackwardGotoCycleUsesLabelAsCanonicalMu(t *testing.T) {
	p := parseBindLower(t, `
::again::
goto again
`)
	entry, _ := p.Entry()
	label, _ := p.LabelAt(0)
	if owner, cursor, ok := p.Label(label); !ok || owner != entry || cursor != 0 {
		t.Fatalf("Label = owner %v cursor %d ok %v; want %v/0/true", owner, cursor, ok, entry)
	}
	jump, _ := p.GotoAt(0)
	if owner, target, ok := p.Goto(jump); !ok || owner != entry || target != label {
		t.Fatalf("Goto = owner %v target %v ok %v; want %v/%v/true", owner, target, ok, entry, label)
	}
	for _, term := range []program.Term{label, jump} {
		if head, ok := p.Mu(term); !ok || head != label {
			t.Fatalf("Mu(%v) = %v, %v; want Label %v", term, head, ok, label)
		}
	}
}

func TestInvalidGotoAndLabelControlFailsBeforeProgramAssembly(t *testing.T) {
	for _, source := range []string{
		`goto missing`,
		"::same::\n::same::",
		"goto done\nlocal x = 1\n::done::\nreturn x",
		"::outside::\nreturn function() goto outside end",
	} {
		_, err := lowerSource(source)
		if err == nil || !strings.Contains(err.Error(), "invalid control") {
			t.Fatalf("invalid source lowered for %q: %v", source, err)
		}
	}
}

func TestUnsupportedSyntaxIsHonest(t *testing.T) {
	stmts, err := parse.ParseString(`return 1 :: number`, "unsupported.lua")
	if err != nil {
		t.Fatal(err)
	}
	_, err = programlower.Lower("unsupported.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil {
		t.Fatal("cast expression lowered without a typed Program relation")
	}
	if !strings.Contains(err.Error(), "unsupported expression *ast.CastExpr") {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := programlower.Lower("unsupported.lua", stmts, nil); err == nil {
		t.Fatal("nil binding result was accepted")
	}
	if _, err := programlower.Lower("malformed.lua", []ast.Stmt{nil}, bind.BindChunk(nil, bind.Options{})); err == nil {
		t.Fatal("nil statement was accepted")
	}
}

func TestTypedNilASTNodesFailClosed(t *testing.T) {
	emptyBinding := bind.BindChunk(nil, bind.Options{})
	for _, stmts := range [][]ast.Stmt{
		{(*ast.LocalAssignStmt)(nil)}, // supported statement
		{(*ast.IfStmt)(nil)},          // supported statement
		{(*ast.WhileStmt)(nil)},       // unsupported statement
		{&ast.ReturnStmt{Exprs: []ast.Expr{(*ast.NumberExpr)(nil)}}}, // supported expression
		{&ast.ReturnStmt{Exprs: []ast.Expr{(*ast.CastExpr)(nil)}}},   // unsupported expression
	} {
		assertLowerFailsClosed(t, stmts, emptyBinding)
	}
}

func TestMalformedNestedASTNodesFailClosed(t *testing.T) {
	number := func() ast.Expr { return &ast.NumberExpr{Value: "1"} }
	var absentExpr ast.Expr = (*ast.NumberExpr)(nil)
	var absentStmt ast.Stmt = (*ast.ReturnStmt)(nil)
	cases := [][]ast.Stmt{
		{&ast.ReturnStmt{Exprs: []ast.Expr{&ast.ArithmeticOpExpr{Operator: "+", Lhs: absentExpr, Rhs: number()}}}},
		{&ast.ReturnStmt{Exprs: []ast.Expr{&ast.UnaryNotOpExpr{Expr: absentExpr}}}},
		{&ast.ReturnStmt{Exprs: []ast.Expr{&ast.AttrGetExpr{Object: absentExpr, Key: number(), KeySyntax: ast.AttrKeyIndex}}}},
		{&ast.ReturnStmt{Exprs: []ast.Expr{&ast.AttrGetExpr{Object: number(), Key: absentExpr, KeySyntax: ast.AttrKeyIndex}}}},
		{&ast.ReturnStmt{Exprs: []ast.Expr{&ast.TableExpr{Fields: []*ast.Field{{Value: absentExpr}}}}}},
		{&ast.ReturnStmt{Exprs: []ast.Expr{&ast.TableExpr{Fields: []*ast.Field{{Key: absentExpr, KeySyntax: ast.AttrKeyIndex, Value: number()}}}}}},
		{&ast.ReturnStmt{Exprs: []ast.Expr{&ast.FuncCallExpr{Func: absentExpr}}}},
		{&ast.ReturnStmt{Exprs: []ast.Expr{&ast.FuncCallExpr{Func: number(), Args: []ast.Expr{absentExpr}}}}},
		{&ast.FuncCallStmt{Expr: (*ast.FuncCallExpr)(nil)}},
		{&ast.DoBlockStmt{Stmts: []ast.Stmt{absentStmt}}},
		{&ast.IfStmt{Condition: absentExpr}},
		{&ast.IfStmt{Condition: &ast.TrueExpr{}, Then: []ast.Stmt{absentStmt}}},
		{&ast.IfStmt{Condition: &ast.TrueExpr{}, Else: []ast.Stmt{absentStmt}, HasElse: true}},
		{&ast.WhileStmt{Condition: absentExpr}},
		{&ast.WhileStmt{Condition: &ast.TrueExpr{}, Stmts: []ast.Stmt{absentStmt}}},
		{&ast.RepeatStmt{Condition: absentExpr}},
		{&ast.RepeatStmt{Condition: &ast.TrueExpr{}, Stmts: []ast.Stmt{absentStmt}}},
		{&ast.NumberForStmt{Name: "i", Init: absentExpr, Limit: number()}},
		{&ast.NumberForStmt{Name: "i", Init: number(), Limit: absentExpr}},
		{&ast.NumberForStmt{Name: "i", Init: number(), Limit: number(), Step: absentExpr}},
		{&ast.GenericForStmt{Names: []string{"k"}, Exprs: []ast.Expr{absentExpr}}},
		{&ast.GenericForStmt{Names: []string{"k"}, Exprs: []ast.Expr{number()}, Stmts: []ast.Stmt{absentStmt}}},
	}
	for _, stmts := range cases {
		assertLowerFailsClosed(t, stmts, bind.BindChunk(nil, bind.Options{}))
	}

	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{&ast.ReturnStmt{}}}
	stmts := []ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{fn}}}
	binding := bind.BindChunk(stmts, bind.Options{})
	fn.Stmts = []ast.Stmt{absentStmt}
	assertLowerFailsClosed(t, stmts, binding)

	for _, target := range []ast.Expr{
		&ast.AttrGetExpr{
			Object:    &ast.IdentExpr{Value: "t"},
			Key:       number(),
			KeySyntax: ast.AttrKeyIndex,
		},
		&ast.AttrGetExpr{
			Object:    number(),
			Key:       &ast.StringExpr{Value: "f"},
			KeySyntax: ast.AttrKeyDot,
		},
		&ast.AttrGetExpr{
			Object: &ast.FuncCallExpr{
				Func: &ast.IdentExpr{Value: "factory"},
			},
			Key:       &ast.StringExpr{Value: "f"},
			KeySyntax: ast.AttrKeyDot,
		},
	} {
		declaration := &ast.FuncDefStmt{
			Name: &ast.FuncName{Func: &ast.IdentExpr{Value: "f"}},
			Func: &ast.FunctionExpr{},
		}
		stmts := []ast.Stmt{declaration}
		binding := bind.BindChunk(stmts, bind.Options{})
		declaration.Name.Func = target
		assertLowerFailsClosed(t, stmts, binding)
	}
}

func assertLowerFailsClosed(t *testing.T, stmts []ast.Stmt, binding *bind.Result) {
	t.Helper()
	var previous string
	for range 2 {
		_, err := programlower.Lower("malformed.lua", stmts, binding)
		if err == nil {
			t.Fatalf("malformed AST lowered: %#v", stmts)
		}
		if previous != "" && err.Error() != previous {
			t.Fatalf("nondeterministic malformed-AST errors: %q then %q", previous, err)
		}
		previous = err.Error()
	}
}

func entryRoots(t *testing.T, p *program.Program) []program.Term {
	t.Helper()
	entry, ok := p.Entry()
	if !ok {
		t.Fatal("Program has no Entry")
	}
	return bodyRoots(t, p, entry)
}

func TestAnonymousFunctionFormalsVarargAndOpenValues(t *testing.T) {
	p, function := loweredReturnValue(t, `return function(a, ...) return a, ... end`)
	entry, _ := p.Entry()
	owner, body, varargCell, ok := p.Function(function)
	if !ok || owner != entry || body == 0 || varargCell == 0 {
		t.Fatalf("Function = owner %v body %v vararg %v ok %v", owner, body, varargCell, ok)
	}
	if count, ok := p.FormalLen(function); !ok || count != 1 {
		t.Fatalf("FormalLen = %d, %v", count, ok)
	}
	formal, ok := p.FormalAt(function, 0)
	if !ok {
		t.Fatal("missing formal Cell")
	}
	if formalOwner, ok := p.Cell(formal); !ok || formalOwner != body {
		t.Fatalf("formal owner = %v, %v; want %v", formalOwner, ok, body)
	}
	if varargOwner, ok := p.Cell(varargCell); !ok || varargOwner != body {
		t.Fatalf("vararg owner = %v, %v; want %v", varargOwner, ok, body)
	}
	if count, ok := p.FunctionCaptureCount(function); !ok || count != 0 {
		t.Fatalf("FunctionCaptureCount = %d, %v", count, ok)
	}
	bodyRoots := bodyRoots(t, p, body)
	if len(bodyRoots) != 1 {
		t.Fatalf("Function Body roots = %v", bodyRoots)
	}
	_, returned, ok := p.Return(bodyRoots[0])
	if !ok {
		t.Fatalf("Function root is not Return: %v", bodyRoots[0])
	}
	if count, ok := p.ValuesLen(returned); !ok || count != 1 {
		t.Fatalf("function return fixed prefix = %d, %v", count, ok)
	}
	tail := valuesTail(t, p, returned)
	if tail == 0 {
		t.Fatalf("function return tail = %v", tail)
	}
	if _, cell, ok := p.Vararg(tail); !ok || cell != varargCell {
		t.Fatalf("vararg occurrence = Cell %v, %v; want %v", cell, ok, varargCell)
	}
}

func TestDirectCaptureFinalizationAndLexicalIsolation(t *testing.T) {
	p := parseBindLower(t, `local x = 1
return function(a) return x, a end, function(a) return x, a end`)
	roots := entryRoots(t, p)
	if len(roots) != 2 {
		t.Fatalf("Entry roots = %v", roots)
	}
	boundOuter := boundCell(t, p, roots[0], 0)
	_, returned, ok := p.Return(roots[1])
	if !ok {
		t.Fatal("Entry root 1 is not Return")
	}
	functions := make([]program.Term, 2)
	for i := range functions {
		functions[i] = valueAt(t, p, returned, i)
	}
	var priorFormal, priorInner program.Term
	for i, function := range functions {
		_, body, _, ok := p.Function(function)
		if !ok {
			t.Fatalf("value %d is not Function", i)
		}
		formal, ok := p.FormalAt(function, 0)
		if !ok {
			t.Fatalf("Function %d has no formal", i)
		}
		if count, ok := p.FunctionCaptureCount(function); !ok || count != 1 {
			t.Fatalf("Function %d capture count = %d, %v", i, count, ok)
		}
		inner, outer := functionCapture(t, p, function, 0)
		if outer != boundOuter {
			t.Fatalf("Capture %d = inner %v outer %v; want outer %v", i, inner, outer, boundOuter)
		}
		if innerOwner, ok := p.Cell(inner); !ok || innerOwner != body {
			t.Fatalf("Capture %d inner owner = %v, %v; want %v", i, innerOwner, ok, body)
		}
		bodyRoots := bodyRoots(t, p, body)
		_, functionValues, ok := p.Return(bodyRoots[0])
		if !ok {
			t.Fatalf("Function %d Body root is not Return", i)
		}
		capturedRead := valueAt(t, p, functionValues, 0)
		if _, source, ok := p.Read(capturedRead); !ok || source != inner {
			t.Fatalf("Function %d captured Read source = %v, %v; want inner %v", i, source, ok, inner)
		}
		if i != 0 && (formal == priorFormal || inner == priorInner) {
			t.Fatalf("sibling Functions share lexical Cells: formal %v/%v inner %v/%v", formal, priorFormal, inner, priorInner)
		}
		priorFormal, priorInner = formal, inner
	}
}

func TestNestedFunctionCapturesCurrentLexicalAliases(t *testing.T) {
	p := parseBindLower(t, `local x = 1
return function(a)
  return function(b) return x, a, b end
end`)
	roots := entryRoots(t, p)
	chunkX := boundCell(t, p, roots[0], 0)
	_, outerValues, ok := p.Return(roots[1])
	if !ok {
		t.Fatal("outer Return missing")
	}
	outerFunction := valueAt(t, p, outerValues, 0)
	_, outerBody, _, ok := p.Function(outerFunction)
	if !ok {
		t.Fatal("outer value is not Function")
	}
	outerFormal, _ := p.FormalAt(outerFunction, 0)
	if count, ok := p.FunctionCaptureCount(outerFunction); !ok || count != 1 {
		t.Fatalf("outer capture count = %d, %v; want propagated chunk x", count, ok)
	}
	outerX, outerSource := functionCapture(t, p, outerFunction, 0)
	if outerSource != chunkX {
		t.Fatalf("outer x Capture = inner %v outer %v; want chunk Cell %v", outerX, outerSource, chunkX)
	}
	outerBodyRoots := bodyRoots(t, p, outerBody)
	_, innerValues, ok := p.Return(outerBodyRoots[0])
	if !ok {
		t.Fatal("outer Function does not return inner Function")
	}
	innerFunction := valueAt(t, p, innerValues, 0)
	if count, ok := p.FunctionCaptureCount(innerFunction); !ok || count != 2 {
		t.Fatalf("inner capture count = %d, %v", count, ok)
	}
	seenOuter := map[program.Term]bool{}
	for i := 0; i < 2; i++ {
		_, outer := functionCapture(t, p, innerFunction, i)
		seenOuter[outer] = true
	}
	if !seenOuter[outerX] || !seenOuter[outerFormal] || seenOuter[chunkX] {
		t.Fatalf("inner captures = %v; want parent aliases %v and %v, not chunk Cell %v",
			seenOuter, outerX, outerFormal, chunkX)
	}
}

func TestPlainStatementImmediateAndMethodCalls(t *testing.T) {
	t.Run("plain and Values adjustment", func(t *testing.T) {
		p := parseBindLower(t, `local f
f(1, 2)
return f(3), 4, f(5)`)
		roots := entryRoots(t, p)
		if len(roots) != 3 {
			t.Fatalf("Entry roots = %v", roots)
		}
		_, callee, receiver, actuals, direct, ok := p.Call(roots[1])
		if !ok || callee == 0 || receiver != 0 || actuals == 0 || direct != 0 {
			t.Fatalf("statement Call = callee %v receiver %v actuals %v direct %v ok %v", callee, receiver, actuals, direct, ok)
		}
		if count, ok := p.ValuesLen(actuals); !ok || count != 2 {
			t.Fatalf("statement actuals = %d, %v", count, ok)
		}
		_, returned, _ := p.Return(roots[2])
		if count, ok := p.ValuesLen(returned); !ok || count != 2 {
			t.Fatalf("return fixed prefix = %d, %v", count, ok)
		}
		tail := valuesTail(t, p, returned)
		if tail == 0 {
			t.Fatalf("return open tail = %v", tail)
		}
		if _, _, _, _, _, ok := p.Call(tail); !ok {
			t.Fatalf("return tail is not Call: %v", tail)
		}
	})

	t.Run("call arguments and table list tail stay open", func(t *testing.T) {
		p := parseBindLower(t, `local f
return f(1, f(2)), {f(3), f(4)}`)
		roots := entryRoots(t, p)
		_, returned, _ := p.Return(roots[1])
		outerCall := valueAt(t, p, returned, 0)
		_, _, _, actuals, _, ok := p.Call(outerCall)
		if !ok {
			t.Fatalf("first return value is not Call: %v", outerCall)
		}
		if count, ok := p.ValuesLen(actuals); !ok || count != 1 {
			t.Fatalf("outer actual fixed prefix = %d, %v", count, ok)
		}
		if tail := valuesTail(t, p, actuals); tail == 0 {
			t.Fatalf("outer actual tail = %v", tail)
		}
		table := valueAt(t, p, returned, 1)
		if count, ok := p.TableLen(table); !ok || count != 2 {
			t.Fatalf("TableLen = %d, %v", count, ok)
		}
		_, firstValues, firstKind, _, _ := p.Field(table, 0)
		_, lastValues, lastKind, _, _ := p.Field(table, 1)
		if firstKind != program.FieldList || lastKind != program.FieldList {
			t.Fatalf("table field kinds = %v, %v", firstKind, lastKind)
		}
		if count, ok := p.ValuesLen(firstValues); !ok || count != 1 {
			t.Fatalf("non-final list field fixed prefix = %d, %v", count, ok)
		}
		if tail := valuesTail(t, p, firstValues); tail != 0 {
			t.Fatalf("non-final list field tail = %v", tail)
		}
		if count, ok := p.ValuesLen(lastValues); !ok || count != 0 {
			t.Fatalf("final list field fixed prefix = %d, %v", count, ok)
		}
		if tail := valuesTail(t, p, lastValues); tail == 0 {
			t.Fatalf("final list field tail = %v", tail)
		}
	})

	t.Run("parenthesized call adjusts to scalar", func(t *testing.T) {
		p := parseBindLower(t, `local f
return (f())`)
		roots := entryRoots(t, p)
		_, returned, _ := p.Return(roots[1])
		if count, ok := p.ValuesLen(returned); !ok || count != 1 {
			t.Fatalf("adjusted return fixed prefix = %d, %v", count, ok)
		}
		if tail := valuesTail(t, p, returned); tail != 0 {
			t.Fatalf("adjusted return tail = %v", tail)
		}
	})

	t.Run("immediate", func(t *testing.T) {
		p := parseBindLower(t, `return (function(a) return a end)(1)`)
		roots := entryRoots(t, p)
		_, returned, _ := p.Return(roots[0])
		call := valuesTail(t, p, returned)
		_, callee, receiver, actuals, direct, ok := p.Call(call)
		if !ok || receiver != 0 || direct != callee {
			t.Fatalf("immediate Call = callee %v receiver %v direct %v ok %v", callee, receiver, direct, ok)
		}
		if _, _, _, ok := p.Function(callee); !ok {
			t.Fatalf("immediate callee is not Function: %v", callee)
		}
		if count, ok := p.ValuesLen(actuals); !ok || count != 1 {
			t.Fatalf("immediate actuals = %d, %v", count, ok)
		}
	})

	t.Run("method needs exact upstream token evidence", func(t *testing.T) {
		stmts, err := parse.ParseString(`local t = {}
return t:m(1)`, "method.lua")
		if err != nil {
			t.Fatal(err)
		}
		_, err = programlower.Lower("method.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
		if err == nil || !strings.Contains(err.Error(), "AST has no MethodPosition evidence") {
			t.Fatalf("method call error = %v", err)
		}
	})
}

func TestFunctionAndCallEvidenceLossFailsClosed(t *testing.T) {
	for _, source := range []string{
		`local function f() end`,
		`local f = function() end`,
	} {
		stmts, err := parse.ParseString(source, "ambiguous.lua")
		if err != nil {
			t.Fatal(err)
		}
		_, err = programlower.Lower("ambiguous.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
		if err == nil || !strings.Contains(err.Error(), "recursive-local syntax was erased") {
			t.Fatalf("%q error = %v", source, err)
		}
	}
	for _, source := range []string{
		`function f() end`,
		`function t.f() end`,
	} {
		stmts, err := parse.ParseString(source, "definition.lua")
		if err != nil {
			t.Fatal(err)
		}
		_, err = programlower.Lower("definition.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
		if err == nil || !strings.Contains(err.Error(), "non-local") {
			t.Fatalf("%q error = %v", source, err)
		}
	}
	stmts, err := parse.ParseString(`local t = {}
function t:f() end`, "method-definition.lua")
	if err != nil {
		t.Fatal(err)
	}
	_, err = programlower.Lower(
		"method-definition.lua",
		stmts,
		bind.BindChunk(stmts, bind.Options{}),
	)
	if err == nil || !strings.Contains(err.Error(), "MethodPosition evidence") {
		t.Fatalf("method definition error = %v", err)
	}

	stmts, err = parse.ParseString(`local f
function f(a: number) return a end`, "typed-definition.lua")
	if err != nil {
		t.Fatal(err)
	}
	_, err = programlower.Lower(
		"typed-definition.lua",
		stmts,
		bind.BindChunk(stmts, bind.Options{}),
	)
	if err == nil || !strings.Contains(err.Error(), "typed function parameter") {
		t.Fatalf("typed function definition error = %v", err)
	}

	stmts, err = parse.ParseString(`return function(a: number) return a end`, "typed.lua")
	if err != nil {
		t.Fatal(err)
	}
	_, err = programlower.Lower("typed.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), "typed function parameter") {
		t.Fatalf("typed Function error = %v", err)
	}

	typedCall := &ast.FuncCallExpr{
		Func:     &ast.NilExpr{},
		TypeArgs: []ast.TypeExpr{&ast.PrimitiveTypeExpr{Name: "number"}},
	}
	stmts = []ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{typedCall}}}
	_, err = programlower.Lower("typed-call.lua", stmts, bind.BindChunk(stmts, bind.Options{}))
	if err == nil || !strings.Contains(err.Error(), "unsupported typed call") {
		t.Fatalf("typed Call error = %v", err)
	}
}

func functionAssignedBy(
	t *testing.T,
	p *program.Program,
	assign program.Term,
) program.Term {
	t.Helper()
	if count, ok := p.AssignLen(assign); !ok || count != 1 {
		t.Fatalf("AssignLen(%v) = %d, %v; want one target", assign, count, ok)
	}
	_, values, ok := p.Assign(assign)
	if !ok {
		t.Fatalf("%v is not Assign", assign)
	}
	if count, ok := p.ValuesLen(values); !ok || count != 1 {
		t.Fatalf("assigned Values = %d, %v; want one fixed Function", count, ok)
	}
	if tail := valuesTail(t, p, values); tail != 0 {
		t.Fatalf("assigned Values tail = %v; want closed", tail)
	}
	function := valueAt(t, p, values, 0)
	if _, _, _, ok := p.Function(function); !ok {
		t.Fatalf("assigned value %v is not Function", function)
	}
	return function
}

func TestLocalCellFunctionDefinitionIsOneDynamicAssignment(t *testing.T) {
	p := parseBindLower(t, `local f
function f(a, ...)
  return f(a), ...
end
return f`)
	roots := entryRoots(t, p)
	if len(roots) != 3 {
		t.Fatalf("Entry roots = %v; want Bind, Assign, Return", roots)
	}
	cell := boundCell(t, p, roots[0], 0)
	assign := roots[1]
	if target := mustTarget(t, p, assign, 0); target != cell {
		t.Fatalf("function-definition target = %v; want predeclared Cell %v", target, cell)
	}
	function := functionAssignedBy(t, p, assign)
	entry, _ := p.Entry()
	owner, body, varargCell, ok := p.Function(function)
	if !ok || owner != entry || body == 0 || varargCell == 0 {
		t.Fatalf(
			"Function = owner %v body %v vararg %v ok %v; want Entry-owned vararg Function",
			owner,
			body,
			varargCell,
			ok,
		)
	}
	formal, ok := p.FormalAt(function, 0)
	if !ok {
		t.Fatal("Function has no formal Cell")
	}
	if owner, ok := p.Cell(formal); !ok || owner != body {
		t.Fatalf("formal owner = %v, %v; want Function Body %v", owner, ok, body)
	}
	if owner, ok := p.Cell(varargCell); !ok || owner != body {
		t.Fatalf("vararg owner = %v, %v; want Function Body %v", owner, ok, body)
	}
	if count, ok := p.FunctionCaptureCount(function); !ok || count != 1 {
		t.Fatalf("Function captures = %d, %v; want target Cell", count, ok)
	}
	inner, outer := functionCapture(t, p, function, 0)
	if outer != cell {
		t.Fatalf("Function capture = inner %v outer %v; want outer Cell %v", inner, outer, cell)
	}
	functionRoots := bodyRoots(t, p, body)
	if len(functionRoots) != 1 {
		t.Fatalf("Function Body roots = %v; want Return", functionRoots)
	}
	_, returned, ok := p.Return(functionRoots[0])
	if !ok {
		t.Fatalf("Function Body root is not Return: %v", functionRoots[0])
	}
	if count, ok := p.ValuesLen(returned); !ok || count != 1 {
		t.Fatalf("Function return fixed prefix = %d, %v; want recursive Call", count, ok)
	}
	call := valueAt(t, p, returned, 0)
	_, callee, receiver, actuals, direct, ok := p.Call(call)
	if !ok || receiver != 0 || direct != 0 {
		t.Fatalf(
			"recursive dynamic Call = callee %v receiver %v direct %v ok %v",
			callee,
			receiver,
			direct,
			ok,
		)
	}
	if _, source, ok := p.Read(callee); !ok || source != inner {
		t.Fatalf("recursive callee Read = source %v, %v; want capture Cell %v", source, ok, inner)
	}
	if count, ok := p.ValuesLen(actuals); !ok || count != 1 {
		t.Fatalf("recursive actuals = %d, %v; want formal argument", count, ok)
	}
	tail := valuesTail(t, p, returned)
	if _, source, ok := p.Vararg(tail); !ok || source != varargCell {
		t.Fatalf("return tail Vararg = Cell %v, %v; want %v", source, ok, varargCell)
	}
	if mu, ok := p.Mu(function); ok || mu != 0 {
		t.Fatalf("assigned mutable Function Mu = %v, %v; want no sealed direct identity", mu, ok)
	}
	_, returnedAtEntry, ok := p.Return(roots[2])
	if !ok {
		t.Fatalf("Entry root 2 is not Return: %v", roots[2])
	}
	if _, source, ok := p.Read(valueAt(t, p, returnedAtEntry, 0)); !ok || source != cell {
		t.Fatalf("Entry returned Read = source %v, %v; want target Cell %v", source, ok, cell)
	}
}

func TestFunctionDefinitionTargetSurvivesNestedBodyAssignmentScratch(t *testing.T) {
	p := parseBindLower(t, `local f, x
function f()
  x = x
end`)
	roots := entryRoots(t, p)
	if len(roots) != 2 {
		t.Fatalf("Entry roots = %v; want Bind, function-definition Assign", roots)
	}
	fCell := boundCell(t, p, roots[0], 0)
	xCell := boundCell(t, p, roots[0], 1)
	outerAssign := roots[1]
	if target := mustTarget(t, p, outerAssign, 0); target != fCell {
		t.Fatalf("outer retained target = %v; want f Cell %v", target, fCell)
	}
	function := functionAssignedBy(t, p, outerAssign)
	if count, ok := p.FunctionCaptureCount(function); !ok || count != 1 {
		t.Fatalf("Function captures = %d, %v; want x Cell only", count, ok)
	}
	innerX, outerX := functionCapture(t, p, function, 0)
	if outerX != xCell {
		t.Fatalf("x Capture = inner %v outer %v; want outer Cell %v", innerX, outerX, xCell)
	}
	_, body, _, _ := p.Function(function)
	functionRoots := bodyRoots(t, p, body)
	if len(functionRoots) != 1 {
		t.Fatalf("Function Body roots = %v; want inner Assign", functionRoots)
	}
	innerAssign := functionRoots[0]
	if target := mustTarget(t, p, innerAssign, 0); target != innerX {
		t.Fatalf("inner Assign target = %v; want captured x Cell %v", target, innerX)
	}
	_, values, ok := p.Assign(innerAssign)
	if !ok {
		t.Fatalf("Function Body root %v is not Assign", innerAssign)
	}
	if _, source, ok := p.Read(valueAt(t, p, values, 0)); !ok || source != innerX {
		t.Fatalf("inner Assign Read = source %v, %v; want capture Cell %v", source, ok, innerX)
	}
}

func TestDeepLocalLensFunctionDefinitionPreservesExactPath(t *testing.T) {
	p := parseBindLower(t, `local t = {a = {}}
function t.a.f(a)
  return t.a.f(a)
end
return t.a.f`)
	roots := entryRoots(t, p)
	if len(roots) != 3 {
		t.Fatalf("Entry roots = %v; want Bind, Assign, Return", roots)
	}
	tableCell := boundCell(t, p, roots[0], 0)
	assign := roots[1]
	target := mustTarget(t, p, assign, 0)
	_, targetBase, targetKey, targetKind, _, ok := p.Lens(target)
	if !ok || targetKind != program.FieldName {
		t.Fatalf("target Lens = base %v key %v kind %v ok %v", targetBase, targetKey, targetKind, ok)
	}
	if _, name, _, ok := p.Name(targetKey); !ok || name != "f" {
		t.Fatalf("target key = %q, %v; want f", name, ok)
	}
	_, innerLens, ok := p.Read(targetBase)
	if !ok {
		t.Fatalf("target base %v is not the once-evaluated t.a Read", targetBase)
	}
	_, innerBase, innerKey, innerKind, _, ok := p.Lens(innerLens)
	if !ok || innerKind != program.FieldName {
		t.Fatalf("inner Lens = base %v key %v kind %v ok %v", innerBase, innerKey, innerKind, ok)
	}
	if _, name, _, ok := p.Name(innerKey); !ok || name != "a" {
		t.Fatalf("inner key = %q, %v; want a", name, ok)
	}
	if _, source, ok := p.Read(innerBase); !ok || source != tableCell {
		t.Fatalf("deep target root Read = source %v, %v; want local Cell %v", source, ok, tableCell)
	}
	function := functionAssignedBy(t, p, assign)
	if count, ok := p.FunctionCaptureCount(function); !ok || count != 1 {
		t.Fatalf("deep Function captures = %d, %v; want table Cell", count, ok)
	}
	_, outer := functionCapture(t, p, function, 0)
	if outer != tableCell {
		t.Fatalf("deep Function capture outer = %v; want table Cell %v", outer, tableCell)
	}
	if mu, ok := p.Mu(function); ok || mu != 0 {
		t.Fatalf("Lens-assigned Function Mu = %v, %v; want no sealed direct identity", mu, ok)
	}
	_, body, _, _ := p.Function(function)
	bodyRoots := bodyRoots(t, p, body)
	_, returned, ok := p.Return(bodyRoots[0])
	if !ok {
		t.Fatal("deep Function Body does not return recursive Call")
	}
	call := valuesTail(t, p, returned)
	if call == 0 {
		call = valueAt(t, p, returned, 0)
	}
	if _, _, _, _, direct, ok := p.Call(call); !ok || direct != 0 {
		t.Fatalf("deep recursive Call direct = %v, %v; want dynamic", direct, ok)
	}
}

func TestFunctionDefinitionShadowingAndReplacementKeepCellIdentity(t *testing.T) {
	p := parseBindLower(t, `local f
do
  local f
  function f() return f end
end
function f() return f end
function f() return 2 end
return f`)
	roots := entryRoots(t, p)
	if len(roots) != 5 {
		t.Fatalf("Entry roots = %v; want Bind, Body, Assign, Assign, Return", roots)
	}
	outerCell := boundCell(t, p, roots[0], 0)
	innerRoots := bodyRoots(t, p, roots[1])
	if len(innerRoots) != 2 {
		t.Fatalf("shadow Body roots = %v; want Bind, Assign", innerRoots)
	}
	innerCell := boundCell(t, p, innerRoots[0], 0)
	innerAssign := innerRoots[1]
	if target := mustTarget(t, p, innerAssign, 0); target != innerCell {
		t.Fatalf("inner definition target = %v; want inner Cell %v", target, innerCell)
	}
	innerFunction := functionAssignedBy(t, p, innerAssign)
	_, innerOuter := functionCapture(t, p, innerFunction, 0)
	if innerOuter != innerCell || innerOuter == outerCell {
		t.Fatalf("inner capture outer = %v; want inner Cell %v", innerOuter, innerCell)
	}

	firstOuterAssign, secondOuterAssign := roots[2], roots[3]
	if firstTarget := mustTarget(t, p, firstOuterAssign, 0); firstTarget != outerCell {
		t.Fatalf("first outer definition target = %v; want %v", firstTarget, outerCell)
	}
	if secondTarget := mustTarget(t, p, secondOuterAssign, 0); secondTarget != outerCell {
		t.Fatalf("replacement target = %v; want %v", secondTarget, outerCell)
	}
	firstOuterFunction := functionAssignedBy(t, p, firstOuterAssign)
	secondOuterFunction := functionAssignedBy(t, p, secondOuterAssign)
	if firstOuterFunction == secondOuterFunction {
		t.Fatalf("replacement reused Function identity %v", firstOuterFunction)
	}
	_, capturedOuter := functionCapture(t, p, firstOuterFunction, 0)
	if capturedOuter != outerCell {
		t.Fatalf("outer capture = %v; want outer Cell %v", capturedOuter, outerCell)
	}
	for _, function := range []program.Term{innerFunction, firstOuterFunction, secondOuterFunction} {
		if mu, ok := p.Mu(function); ok || mu != 0 {
			t.Fatalf("mutable definition Function %v Mu = %v, %v", function, mu, ok)
		}
	}
}

func localFunctionDefinitionsSource(count int) string {
	var source strings.Builder
	source.Grow(8 + count*36)
	source.WriteString("local f\n")
	for i := 0; i < count; i++ {
		source.WriteString("function f(a) return a end\n")
	}
	source.WriteString("return f")
	return source.String()
}

func TestFourThousandLocalFunctionDefinitionsLowerIteratively(t *testing.T) {
	const count = 4 * 1024
	p := parseBindLower(t, localFunctionDefinitionsSource(count))
	if p.FunctionCount() != count || p.AssignCount() != count ||
		p.BodyCount() != count+1 || p.BindCount() != 1 {
		t.Fatalf(
			"4K definition families: functions=%d assigns=%d bodies=%d binds=%d",
			p.FunctionCount(),
			p.AssignCount(),
			p.BodyCount(),
			p.BindCount(),
		)
	}
	roots := entryRoots(t, p)
	if len(roots) != count+2 {
		t.Fatalf("4K Entry roots = %d; want %d", len(roots), count+2)
	}
	cell := boundCell(t, p, roots[0], 0)
	for i := 0; i < count; i++ {
		assign := roots[i+1]
		if target := mustTarget(t, p, assign, 0); target != cell {
			t.Fatalf("definition %d target = %v; want Cell %v", i, target, cell)
		}
		function := functionAssignedBy(t, p, assign)
		if mu, ok := p.Mu(function); ok || mu != 0 {
			t.Fatalf("definition %d Function Mu = %v, %v; want none", i, mu, ok)
		}
	}
}

func TestLocalFunctionDefinitionTermAndAllocationGrowthIsLinear(t *testing.T) {
	type measurement struct {
		bytes int64
		terms int
	}
	measure := func(count int) measurement {
		t.Helper()
		stmts, err := parse.ParseString(localFunctionDefinitionsSource(count), "funcdefs.lua")
		if err != nil {
			t.Fatal(err)
		}
		binding := bind.BindChunk(stmts, bind.Options{})
		p, err := programlower.Lower("funcdefs.lua", stmts, binding)
		if err != nil {
			t.Fatal(err)
		}
		result := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				loweredSink, err = programlower.Lower("funcdefs.lua", stmts, binding)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		return measurement{bytes: result.AllocedBytesPerOp(), terms: p.TermCount()}
	}

	small := measure(1024)
	large := measure(2048)
	t.Logf(
		"function-definition scaling: 1K terms=%d bytes=%d; 2K terms=%d bytes=%d",
		small.terms,
		small.bytes,
		large.terms,
		large.bytes,
	)
	if large.terms > small.terms*26/10+16 {
		t.Fatalf(
			"function-definition Term growth exceeds law: 1K=%d 2K=%d",
			small.terms,
			large.terms,
		)
	}
	if large.bytes > small.bytes*26/10+64*1024 {
		t.Fatalf(
			"function-definition allocation growth exceeds law: 1K=%dB 2K=%dB",
			small.bytes,
			large.bytes,
		)
	}
}

func wideClosureSource(bindings, siblings int) string {
	var source strings.Builder
	if bindings > 0 {
		source.WriteString("local ")
		for i := 0; i < bindings; i++ {
			if i != 0 {
				source.WriteByte(',')
			}
			source.WriteByte('x')
			source.WriteString(strconv.Itoa(i))
		}
		source.WriteString(" = ")
		for i := 0; i < bindings; i++ {
			if i != 0 {
				source.WriteByte(',')
			}
			source.WriteString(strconv.Itoa(i))
		}
		source.WriteByte('\n')
	}
	source.WriteString("return ")
	if siblings == 0 {
		source.WriteByte('0')
		return source.String()
	}
	for i := 0; i < siblings; i++ {
		if i != 0 {
			source.WriteByte(',')
		}
		source.WriteString("function() return x0 end")
	}
	return source.String()
}

func TestWideSiblingCaptureAliasesStayIsolated(t *testing.T) {
	const siblings = 96
	p := parseBindLower(t, wideClosureSource(32, siblings))
	roots := entryRoots(t, p)
	outer := boundCell(t, p, roots[0], 0)
	_, returned, ok := p.Return(roots[1])
	if !ok {
		t.Fatal("wide Return missing")
	}
	seenInner := make(map[program.Term]struct{}, siblings)
	for i := 0; i < siblings; i++ {
		function := valueAt(t, p, returned, i)
		if count, ok := p.FunctionCaptureCount(function); !ok || count != 1 {
			t.Fatalf("Function %d capture count = %d, %v", i, count, ok)
		}
		inner, gotOuter := functionCapture(t, p, function, 0)
		if gotOuter != outer {
			t.Fatalf("Function %d Capture = inner %v outer %v; want outer %v", i, inner, gotOuter, outer)
		}
		if _, duplicate := seenInner[inner]; duplicate {
			t.Fatalf("Function %d reused capture-inner Cell %v", i, inner)
		}
		seenInner[inner] = struct{}{}
	}
}

func TestGrandchildOnlyCaptureCrossesEveryFunctionBoundary(t *testing.T) {
	p := parseBindLower(t, `local x = 1
return function()
  return function() return x end
end`)
	roots := entryRoots(t, p)
	chunkX := boundCell(t, p, roots[0], 0)
	_, outerValues, _ := p.Return(roots[1])
	outerFunction := valueAt(t, p, outerValues, 0)
	if count, ok := p.FunctionCaptureCount(outerFunction); !ok || count != 1 {
		t.Fatalf("outer Function captures = %d, %v; want propagated x", count, ok)
	}
	outerInner, gotOuter := functionCapture(t, p, outerFunction, 0)
	if gotOuter != chunkX {
		t.Fatalf("outer Capture = inner %v outer %v; want chunk declaration %v", outerInner, gotOuter, chunkX)
	}
	_, outerBody, _, _ := p.Function(outerFunction)
	outerRoots := bodyRoots(t, p, outerBody)
	_, innerValues, _ := p.Return(outerRoots[0])
	innerFunction := valueAt(t, p, innerValues, 0)
	_, gotOuter = functionCapture(t, p, innerFunction, 0)
	if gotOuter != outerInner || gotOuter == chunkX {
		t.Fatalf("grandchild Capture outer = %v; want parent capture Cell %v", gotOuter, outerInner)
	}
}

func TestEntryCaptureOrderAndDedupAcrossDescendants(t *testing.T) {
	p := parseBindLower(t, `local x, y, z = 1, 2, 3
return function()
  return x, function() return y, x end, function() return z, y end
end`)
	roots := entryRoots(t, p)
	chunk := make([]program.Term, 3)
	for i := range chunk {
		chunk[i] = boundCell(t, p, roots[0], i)
	}
	_, values, _ := p.Return(roots[1])
	outerFunction := valueAt(t, p, values, 0)
	if count, ok := p.FunctionCaptureCount(outerFunction); !ok || count != 3 {
		t.Fatalf("outer capture count = %d, %v; want x,y,z once", count, ok)
	}
	outerInner := make([]program.Term, 3)
	for i := range outerInner {
		inner, outer := functionCapture(t, p, outerFunction, i)
		if outer != chunk[i] {
			t.Fatalf("outer Capture %d = inner %v outer %v; want chunk Cell %v",
				i, inner, outer, chunk[i])
		}
		outerInner[i] = inner
	}

	_, body, _, _ := p.Function(outerFunction)
	bodyRoots := bodyRoots(t, p, body)
	_, returned, _ := p.Return(bodyRoots[0])
	for childIndex, want := range [][]program.Term{
		{outerInner[1], outerInner[0]},
		{outerInner[2], outerInner[1]},
	} {
		child := valueAt(t, p, returned, childIndex+1)
		if count, ok := p.FunctionCaptureCount(child); !ok || count != len(want) {
			t.Fatalf("child %d capture count = %d, %v; want %d", childIndex, count, ok, len(want))
		}
		for i, wantOuter := range want {
			_, outer := functionCapture(t, p, child, i)
			if outer != wantOuter {
				t.Fatalf("child %d Capture %d outer = %v; want parent Cell %v",
					childIndex, i, outer, wantOuter)
			}
		}
	}
}

func deepClosureSource(depth int) string {
	var source strings.Builder
	source.WriteString("local x = 1\nreturn ")
	for i := 0; i < depth; i++ {
		source.WriteString("function() return ")
	}
	source.WriteByte('x')
	for i := 0; i < depth; i++ {
		source.WriteString(" end")
	}
	return source.String()
}

func TestDeepDescendantCaptureLoweringScalesWithBoundaryEdges(t *testing.T) {
	type measurement struct {
		terms  int
		allocs float64
	}
	measure := func(depth int) measurement {
		t.Helper()
		stmts, err := parse.ParseString(deepClosureSource(depth), "deep.lua")
		if err != nil {
			t.Fatal(err)
		}
		binding := bind.BindChunk(stmts, bind.Options{})
		p, err := programlower.Lower("deep.lua", stmts, binding)
		if err != nil {
			t.Fatal(err)
		}
		roots := entryRoots(t, p)
		declaration := boundCell(t, p, roots[0], 0)
		_, values, ok := p.Return(roots[1])
		if !ok {
			t.Fatalf("depth %d Entry return missing", depth)
		}
		function := valueAt(t, p, values, 0)
		wantOuter := declaration
		for level := 0; level < depth; level++ {
			_, body, _, ok := p.Function(function)
			if !ok {
				t.Fatalf("depth %d level %d is not a Function", depth, level)
			}
			if count, ok := p.FunctionCaptureCount(function); !ok || count != 1 {
				t.Fatalf("depth %d level %d capture count = %d, %v; want one boundary edge",
					depth, level, count, ok)
			}
			inner, outer := functionCapture(t, p, function, 0)
			if outer != wantOuter {
				t.Fatalf("depth %d level %d Capture = inner %v outer %v; want outer %v",
					depth, level, inner, outer, wantOuter)
			}
			bodyRoots := bodyRoots(t, p, body)
			if len(bodyRoots) != 1 {
				t.Fatalf("depth %d level %d Body roots = %v", depth, level, bodyRoots)
			}
			_, returned, ok := p.Return(bodyRoots[0])
			if !ok {
				t.Fatalf("depth %d level %d Body return missing", depth, level)
			}
			value := valueAt(t, p, returned, 0)
			if level == depth-1 {
				if _, read, ok := p.Read(value); !ok || read != inner {
					t.Fatalf("depth %d terminal Read = %v, %v; want terminal capture Cell %v",
						depth, read, ok, inner)
				}
				continue
			}
			function = value
			wantOuter = inner
		}
		allocs := testing.AllocsPerRun(5, func() {
			loweredSink, err = programlower.Lower("deep.lua", stmts, binding)
			if err != nil {
				t.Fatal(err)
			}
		})
		return measurement{terms: p.TermCount(), allocs: allocs}
	}

	small := measure(40)
	large := measure(80)
	if large.terms > small.terms*3 {
		t.Fatalf("deep term growth is not linear: depth40=%d depth80=%d", small.terms, large.terms)
	}
	if large.allocs > small.allocs*3 {
		t.Fatalf("deep allocation growth is not linear: depth40=%.0f depth80=%.0f", small.allocs, large.allocs)
	}
}

func TestFunctionLoweringAllocationGrowthHasNoBindingTimesClosureTerm(t *testing.T) {
	type measurement struct {
		bytes int64
		terms int
	}
	measure := func(bindings, siblings int) measurement {
		t.Helper()
		source := wideClosureSource(bindings, siblings)
		stmts, err := parse.ParseString(source, "growth.lua")
		if err != nil {
			t.Fatal(err)
		}
		binding := bind.BindChunk(stmts, bind.Options{})
		p, err := programlower.Lower("growth.lua", stmts, binding)
		if err != nil {
			t.Fatal(err)
		}
		result := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				loweredSink, err = programlower.Lower("growth.lua", stmts, binding)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		return measurement{bytes: result.AllocedBytesPerOp(), terms: p.TermCount()}
	}

	bindingsOnly := measure(384, 0)
	closuresOnly := measure(1, 48)
	combined := measure(384, 48)
	if combined.terms > bindingsOnly.terms+closuresOnly.terms+16 {
		t.Fatalf("Term growth is non-additive: bindings=%d closures=%d combined=%d",
			bindingsOnly.terms, closuresOnly.terms, combined.terms)
	}
	additiveBytes := bindingsOnly.bytes + closuresOnly.bytes
	if combined.bytes > additiveBytes*2 {
		t.Fatalf("allocation bytes expose binding×closure growth: bindings=%d closures=%d combined=%d",
			bindingsOnly.bytes, closuresOnly.bytes, combined.bytes)
	}
}

func TestCompletedBodyRestoresShadowedCaptureSource(t *testing.T) {
	p := parseBindLower(t, `local x = 1
local holder = {}
do
  local x = 2
  holder[1] = function() return x end
end
holder[2] = function() return x end
return holder`)

	entryRoots := entryRoots(t, p)
	if len(entryRoots) != 5 {
		t.Fatalf("Entry roots = %v; want two Bind, Body, Assign, Return", entryRoots)
	}
	outerCell := boundCell(t, p, entryRoots[0], 0)
	innerBody := entryRoots[2]
	innerRoots := bodyRoots(t, p, innerBody)
	if len(innerRoots) != 2 {
		t.Fatalf("inner Body roots = %v; want Bind and Assign", innerRoots)
	}
	innerCell := boundCell(t, p, innerRoots[0], 0)
	if outerCell == innerCell {
		t.Fatal("shadowed declaration reused its outer Cell")
	}
	if p.FunctionCount() != 2 {
		t.Fatalf("FunctionCount = %d, want 2", p.FunctionCount())
	}
	inside, _ := p.FunctionAt(0)
	after, _ := p.FunctionAt(1)
	_, insideOuter := functionCapture(t, p, inside, 0)
	_, afterOuter := functionCapture(t, p, after, 0)
	if insideOuter != innerCell {
		t.Fatalf("in-scope capture outer = %v; want inner Cell %v", insideOuter, innerCell)
	}
	if afterOuter != outerCell || afterOuter == innerCell {
		t.Fatalf("post-scope capture outer = %v; want restored outer Cell %v", afterOuter, outerCell)
	}
}

func TestFreshLoweringIsDeterministicThroughPublicProgramRelations(t *testing.T) {
	source := `local x = 1
local t = {x, name = x}
do
  local x = 2
  t[x] = function(a, ...) return x, a, ... end
end
return x, t`
	stmts, err := parse.ParseString(source, "deterministic.lua")
	if err != nil {
		t.Fatal(err)
	}
	binding := bind.BindChunk(stmts, bind.Options{})
	want, err := programlower.Lower("deterministic.lua", stmts, binding)
	if err != nil {
		t.Fatal(err)
	}
	wantEntry, _ := want.Entry()
	wantRoots := bodyRoots(t, want, wantEntry)
	wantFunction, _ := want.FunctionAt(0)
	wantFunctionOwner, wantFunctionBody, wantVararg, _ := want.Function(wantFunction)
	wantInner, wantOuter := functionCapture(t, want, wantFunction, 0)
	wantTable, _ := want.TableAt(0)

	for run := 0; run < 3; run++ {
		got, err := programlower.Lower("deterministic.lua", stmts, binding)
		if err != nil {
			t.Fatal(err)
		}
		gotEntry, ok := got.Entry()
		if !ok || gotEntry != wantEntry || got.TermCount() != want.TermCount() ||
			got.BodyCount() != want.BodyCount() || got.ValuesCount() != want.ValuesCount() ||
			got.FunctionCount() != want.FunctionCount() || got.TableCount() != want.TableCount() {
			t.Fatalf(
				"run %d public counts/Entry differ: entry=%v/%v terms=%d/%d bodies=%d/%d values=%d/%d functions=%d/%d tables=%d/%d",
				run,
				gotEntry,
				wantEntry,
				got.TermCount(),
				want.TermCount(),
				got.BodyCount(),
				want.BodyCount(),
				got.ValuesCount(),
				want.ValuesCount(),
				got.FunctionCount(),
				want.FunctionCount(),
				got.TableCount(),
				want.TableCount(),
			)
		}
		gotRoots := bodyRoots(t, got, gotEntry)
		if len(gotRoots) != len(wantRoots) {
			t.Fatalf("run %d Entry root count = %d, want %d", run, len(gotRoots), len(wantRoots))
		}
		for i := range wantRoots {
			if gotRoots[i] != wantRoots[i] {
				t.Fatalf("run %d Entry root %d = %v, want %v", run, i, gotRoots[i], wantRoots[i])
			}
		}
		gotFunction, _ := got.FunctionAt(0)
		gotOwner, gotBody, gotVararg, ok := got.Function(gotFunction)
		gotInner, gotOuter := functionCapture(t, got, gotFunction, 0)
		if !ok || gotFunction != wantFunction || gotOwner != wantFunctionOwner ||
			gotBody != wantFunctionBody || gotVararg != wantVararg ||
			gotInner != wantInner || gotOuter != wantOuter {
			t.Fatalf(
				"run %d Function relation differs: term=%v owner=%v body=%v vararg=%v capture=%v/%v",
				run,
				gotFunction,
				gotOwner,
				gotBody,
				gotVararg,
				gotInner,
				gotOuter,
			)
		}
		gotTable, _ := got.TableAt(0)
		if gotTable != wantTable {
			t.Fatalf("run %d Table identity = %v, want %v", run, gotTable, wantTable)
		}
		for i := 0; i < 2; i++ {
			gotKey, gotValues, gotKind, gotNormalized, gotOK := got.Field(gotTable, i)
			wantKey, wantValues, wantKind, wantNormalized, wantOK := want.Field(wantTable, i)
			if !gotOK || !wantOK || gotKey != wantKey || gotValues != wantValues ||
				gotKind != wantKind || gotNormalized != wantNormalized {
				t.Fatalf("run %d Table field %d differs", run, i)
			}
		}
	}
}

func deepUnarySource(depth int) string {
	var source strings.Builder
	source.Grow(len("return true") + depth*len("not "))
	source.WriteString("return ")
	for i := 0; i < depth; i++ {
		source.WriteString("not ")
	}
	source.WriteString("true")
	return source.String()
}

func wideValuesSource(width int) string {
	var source strings.Builder
	source.WriteString("return ")
	for i := 0; i < width; i++ {
		if i != 0 {
			source.WriteByte(',')
		}
		source.WriteString(strconv.Itoa(i))
	}
	return source.String()
}

func deepWhileSource(depth int) string {
	var source strings.Builder
	source.Grow(depth*len("while true do\nend\n") + len("break\n"))
	for i := 0; i < depth; i++ {
		source.WriteString("while true do\n")
	}
	source.WriteString("break\n")
	for i := 0; i < depth; i++ {
		source.WriteString("end\n")
	}
	return source.String()
}

func wideWhileSource(width int) string {
	var source strings.Builder
	source.Grow(width * len("while false do end\n"))
	for i := 0; i < width; i++ {
		source.WriteString("while false do end\n")
	}
	return source.String()
}

func TestFourThousandDeepAndWideLoopsLowerIterativelyFromSource(t *testing.T) {
	const size = 4 * 1024
	deep := parseBindLower(t, deepWhileSource(size))
	if deep.LoopCount() != size || deep.BodyCount() != size+1 || deep.BreakCount() != 1 {
		t.Fatalf(
			"deep loop families: loops=%d bodies=%d breaks=%d",
			deep.LoopCount(),
			deep.BodyCount(),
			deep.BreakCount(),
		)
	}
	inner, _ := deep.LoopAt(0)
	broken, _ := deep.BreakAt(0)
	_, breakLoop, ok := deep.Break(broken)
	if !ok || breakLoop != inner {
		t.Fatalf("deep Break Loop = %v, %v; want innermost %v", breakLoop, ok, inner)
	}
	outer, _ := deep.LoopAt(size - 1)
	innerHead, innerOK := deep.Mu(inner)
	outerHead, outerOK := deep.Mu(outer)
	if !innerOK || !outerOK || innerHead != outerHead {
		t.Fatalf(
			"nested loop Mu = inner %v/%v outer %v/%v; want one shared recursive component",
			innerHead,
			innerOK,
			outerHead,
			outerOK,
		)
	}

	wide := parseBindLower(t, wideWhileSource(size))
	if wide.LoopCount() != size ||
		wide.BodyCount() != size+1 ||
		wide.BoolCount() != size {
		t.Fatalf(
			"wide loop families: loops=%d bodies=%d bools=%d",
			wide.LoopCount(),
			wide.BodyCount(),
			wide.BoolCount(),
		)
	}
}

func TestThirtyTwoThousandNestedExpressionsLowerFromSource(t *testing.T) {
	const depth = 32 * 1024
	stmts, err := parse.ParseString(deepUnarySource(depth), "deep-source.lua")
	if err != nil {
		t.Fatal(err)
	}
	p, err := programlower.Lower(
		"deep-source.lua",
		stmts,
		bind.BindChunk(stmts, bind.Options{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if p.UnaryCount() != depth {
		t.Fatalf("UnaryCount = %d, want %d", p.UnaryCount(), depth)
	}
	roots := entryRoots(t, p)
	_, returned, ok := p.Return(roots[0])
	if !ok {
		t.Fatal("deep Entry root is not Return")
	}
	term := valueAt(t, p, returned, 0)
	entry, _ := p.Entry()
	for level := 0; level < depth; level++ {
		owner, op, operand, ok := p.Unary(term)
		if !ok || owner != entry || op != program.UnaryNot || operand == 0 {
			t.Fatalf(
				"level %d Unary = owner %v op %v operand %v ok %v",
				level,
				owner,
				op,
				operand,
				ok,
			)
		}
		term = operand
	}
	if _, value, ok := p.Bool(term); !ok || !value {
		t.Fatalf("deep terminal operand = %v, %v; want true Bool", value, ok)
	}
}

func TestDeepAndWideLoweringObeyLinearScalingLaws(t *testing.T) {
	type measurement struct {
		bytes int64
		ns    int64
		terms int
	}
	measure := func(source, sourceName string) measurement {
		t.Helper()
		stmts, err := parse.ParseString(source, sourceName)
		if err != nil {
			t.Fatal(err)
		}
		binding := bind.BindChunk(stmts, bind.Options{})
		p, err := programlower.Lower(sourceName, stmts, binding)
		if err != nil {
			t.Fatal(err)
		}
		result := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				loweredSink, err = programlower.Lower(sourceName, stmts, binding)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		return measurement{
			bytes: result.AllocedBytesPerOp(),
			ns:    result.NsPerOp(),
			terms: p.TermCount(),
		}
	}
	assertLinear := func(name string, small, large measurement) {
		t.Helper()
		t.Logf(
			"%s scaling: 4K terms=%d bytes=%d ns=%d; 8K terms=%d bytes=%d ns=%d",
			name,
			small.terms,
			small.bytes,
			small.ns,
			large.terms,
			large.bytes,
			large.ns,
		)
		if large.terms > small.terms*26/10+16 {
			t.Fatalf(
				"%s Term growth exceeds law: 4K=%d 8K=%d",
				name,
				small.terms,
				large.terms,
			)
		}
		if large.bytes > small.bytes*26/10+64*1024 {
			t.Fatalf(
				"%s allocation growth exceeds law: 4K=%dB 8K=%dB",
				name,
				small.bytes,
				large.bytes,
			)
		}
	}

	assertLinear(
		"deep",
		measure(deepUnarySource(4*1024), "deep-scaling.lua"),
		measure(deepUnarySource(8*1024), "deep-scaling.lua"),
	)
	assertLinear(
		"wide",
		measure(wideValuesSource(4*1024), "wide-scaling.lua"),
		measure(wideValuesSource(8*1024), "wide-scaling.lua"),
	)
}

func deepDoShadowSource(depth int) string {
	var source strings.Builder
	source.WriteString("local x = 0\n")
	for level := 1; level <= depth; level++ {
		source.WriteString("do\nlocal x = ")
		source.WriteString(strconv.Itoa(level))
		source.WriteByte('\n')
	}
	for level := depth; level > 0; level-- {
		source.WriteString("x = x\nend\n")
	}
	source.WriteString("x = x")
	return source.String()
}

func TestNestedLexicalBodiesObeyLinearScalingAndExactOwnership(t *testing.T) {
	type measurement struct {
		bytes int64
		ns    int64
		terms int
	}
	measure := func(depth int) (*program.Program, measurement) {
		t.Helper()
		stmts, err := parse.ParseString(deepDoShadowSource(depth), "deep-do.lua")
		if err != nil {
			t.Fatal(err)
		}
		binding := bind.BindChunk(stmts, bind.Options{})
		p, err := programlower.Lower("deep-do.lua", stmts, binding)
		if err != nil {
			t.Fatal(err)
		}
		result := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				loweredSink, err = programlower.Lower("deep-do.lua", stmts, binding)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		return p, measurement{
			bytes: result.AllocedBytesPerOp(),
			ns:    result.NsPerOp(),
			terms: p.TermCount(),
		}
	}

	const smallDepth = 4 * 1024
	const largeDepth = 8 * 1024
	_, small := measure(smallDepth)
	largeProgram, large := measure(largeDepth)
	t.Logf(
		"lexical Body scaling: 4K terms=%d bytes=%d ns=%d; 8K terms=%d bytes=%d ns=%d",
		small.terms,
		small.bytes,
		small.ns,
		large.terms,
		large.bytes,
		large.ns,
	)
	if large.terms > small.terms*26/10+16 {
		t.Fatalf("lexical Body Term growth exceeds law: 4K=%d 8K=%d", small.terms, large.terms)
	}
	if large.bytes > small.bytes*26/10+64*1024 {
		t.Fatalf(
			"lexical Body allocation growth exceeds law: 4K=%dB 8K=%dB",
			small.bytes,
			large.bytes,
		)
	}
	if largeProgram.BodyCount() != largeDepth+1 ||
		largeProgram.CellCount() != largeDepth+1 ||
		largeProgram.BindCount() != largeDepth+1 ||
		largeProgram.AssignCount() != largeDepth+1 {
		t.Fatalf(
			"8K lexical families: bodies=%d cells=%d binds=%d assigns=%d",
			largeProgram.BodyCount(),
			largeProgram.CellCount(),
			largeProgram.BindCount(),
			largeProgram.AssignCount(),
		)
	}

	body, _ := largeProgram.Entry()
	for level := 0; level <= largeDepth; level++ {
		roots := bodyRoots(t, largeProgram, body)
		wantRoots := 3
		if level == largeDepth {
			wantRoots = 2
		}
		if len(roots) != wantRoots {
			t.Fatalf("level %d Body roots = %v; want %d", level, roots, wantRoots)
		}
		cell := boundCell(t, largeProgram, roots[0], 0)
		if owner, ok := largeProgram.Cell(cell); !ok || owner != body {
			t.Fatalf("level %d Cell owner = %v, %v; want Body %v", level, owner, ok, body)
		}
		assignIndex := 2
		if level == largeDepth {
			assignIndex = 1
		}
		assign := roots[assignIndex]
		target := mustTarget(t, largeProgram, assign, 0)
		owner, assigned, ok := largeProgram.Assign(assign)
		if !ok || owner != body || target != cell {
			t.Fatalf(
				"level %d Assign = owner %v target %v ok %v; want Body %v Cell %v",
				level,
				owner,
				target,
				ok,
				body,
				cell,
			)
		}
		read := valueAt(t, largeProgram, assigned, 0)
		if readOwner, source, ok := largeProgram.Read(read); !ok ||
			readOwner != body || source != cell {
			t.Fatalf(
				"level %d post-child Read = owner %v source %v ok %v; want Body %v Cell %v",
				level,
				readOwner,
				source,
				ok,
				body,
				cell,
			)
		}
		if level != largeDepth {
			body = roots[1]
		}
	}
}

func deepFunctionCaptureSource(depth int) string {
	var source strings.Builder
	source.WriteString("local x = 1\nreturn ")
	for level := 0; level < depth; level++ {
		source.WriteString("function() return ")
	}
	source.WriteByte('x')
	if depth != 0 {
		source.WriteString(" end")
	}
	for level := 1; level < depth; level++ {
		source.WriteString(", x end")
	}
	source.WriteString(", x")
	return source.String()
}

func TestFourThousandFunctionScopesRestoreImmediateParentCapture(t *testing.T) {
	const depth = 4 * 1024
	stmts, err := parse.ParseString(deepFunctionCaptureSource(depth), "deep-function.lua")
	if err != nil {
		t.Fatal(err)
	}
	p, err := programlower.Lower(
		"deep-function.lua",
		stmts,
		bind.BindChunk(stmts, bind.Options{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if p.FunctionCount() != depth || p.BodyCount() != depth+1 ||
		p.CellCount() != depth+1 || p.ReadCount() != depth+1 {
		t.Fatalf(
			"deep Function families: functions=%d bodies=%d cells=%d reads=%d",
			p.FunctionCount(),
			p.BodyCount(),
			p.CellCount(),
			p.ReadCount(),
		)
	}

	roots := entryRoots(t, p)
	rootCell := boundCell(t, p, roots[0], 0)
	_, rootValues, ok := p.Return(roots[1])
	if !ok {
		t.Fatal("deep Function Entry return missing")
	}
	function := valueAt(t, p, rootValues, 0)
	rootRead := valueAt(t, p, rootValues, 1)
	if owner, source, ok := p.Read(rootRead); !ok || source != rootCell {
		t.Fatalf(
			"post-Function root Read = owner %v source %v ok %v; want root Cell %v",
			owner,
			source,
			ok,
			rootCell,
		)
	}

	wantOuter := rootCell
	for level := 0; level < depth; level++ {
		_, body, _, ok := p.Function(function)
		if !ok {
			t.Fatalf("level %d value is not Function: %v", level, function)
		}
		if count, ok := p.FunctionCaptureCount(function); !ok || count != 1 {
			t.Fatalf("level %d capture count = %d, %v; want 1", level, count, ok)
		}
		inner, outer := functionCapture(t, p, function, 0)
		if outer != wantOuter {
			t.Fatalf(
				"level %d capture Outer = %v; want immediate parent Cell %v",
				level,
				outer,
				wantOuter,
			)
		}
		if innerOwner, ok := p.Cell(inner); !ok || innerOwner != body {
			t.Fatalf(
				"level %d capture Inner owner = %v, %v; want Body %v",
				level,
				innerOwner,
				ok,
				body,
			)
		}
		bodyRoots := bodyRoots(t, p, body)
		if len(bodyRoots) != 1 {
			t.Fatalf("level %d Function Body roots = %v; want Return", level, bodyRoots)
		}
		_, returned, ok := p.Return(bodyRoots[0])
		if !ok {
			t.Fatalf("level %d Function Body root is not Return", level)
		}
		if level == depth-1 {
			read := valueAt(t, p, returned, 0)
			if readOwner, source, ok := p.Read(read); !ok ||
				readOwner != body || source != inner {
				t.Fatalf(
					"level %d terminal Read = owner %v source %v ok %v; want Body %v Cell %v",
					level,
					readOwner,
					source,
					ok,
					body,
					inner,
				)
			}
			continue
		}
		function = valueAt(t, p, returned, 0)
		read := valueAt(t, p, returned, 1)
		if readOwner, source, ok := p.Read(read); !ok ||
			readOwner != body || source != inner {
			t.Fatalf(
				"level %d post-child Read = owner %v source %v ok %v; want Body %v restored Cell %v",
				level,
				readOwner,
				source,
				ok,
				body,
				inner,
			)
		}
		wantOuter = inner
	}
}
