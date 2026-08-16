package program_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
)

func TestTransformerAllocationRolesAreOrderedOwnerFencedAndReplayStable(t *testing.T) {
	text := `
local table = { first = 1, second = 2 }
local function make() return {} end
return table, make
`
	left, err := lualower.Lower(lualower.Source{Name: "allocation-roles.lua", Text: []byte(text)})
	if err != nil {
		t.Fatalf("Lower(left): %v", err)
	}
	right, err := lualower.Lower(lualower.Source{Name: "allocation-roles.lua", Text: []byte(text)})
	if err != nil {
		t.Fatalf("Lower(right): %v", err)
	}
	leftInput, rightInput := left.TransformerInput(), right.TransformerInput()
	leftAllocations, rightAllocations := leftInput.Allocations(), rightInput.Allocations()
	if leftAllocations.Count() == 0 || leftAllocations.Count() != rightAllocations.Count() {
		t.Fatalf("allocation counts = %d/%d, want equal nonzero", leftAllocations.Count(), rightAllocations.Count())
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		for index := 0; index < leftAllocations.Count(); index++ {
			if allocation, ok := leftAllocations.At(index); !ok || !allocation.Available() {
				t.Fatal("sealed allocation receipt lookup")
			}
		}
	}); allocations != 0 {
		t.Fatalf("sealed allocation receipt lookup allocated %v times", allocations)
	}
	for index := 0; index < leftAllocations.Count(); index++ {
		leftAllocation, leftOK := leftAllocations.At(index)
		rightAllocation, rightOK := rightAllocations.At(index)
		if !leftOK || !rightOK || !leftAllocation.Available() || !rightAllocation.Available() {
			t.Fatalf("At(%d) = %#v/%v, %#v/%v", index, leftAllocation, leftOK, rightAllocation, rightOK)
		}
		if leftAllocation.ID() != rightAllocation.ID() || leftAllocation.Owns(rightInput) {
			t.Fatalf("allocation %d replay identity/owner fence failed", index)
		}
		if leftAllocation.Role() != program.AllocationTable && leftAllocation.Role() != program.AllocationClosure {
			t.Fatalf("allocation %d has invalid role %d", index, leftAllocation.Role())
		}
		leftGeometry := leftAllocation.Geometry()
		rightGeometry := rightAllocation.Geometry()
		if !leftGeometry.Available() || !rightGeometry.Available() || leftGeometry.Role() != leftAllocation.Role() || leftGeometry.Form() != rightGeometry.Form() || leftGeometry.FieldCount() != rightGeometry.FieldCount() {
			t.Fatalf("allocation %d geometry mismatch", index)
		}
		if !leftGeometry.Span().Available() || !rightGeometry.Span().Available() {
			t.Fatalf("allocation %d span receipt unavailable", index)
		}
		switch leftAllocation.Role() {
		case program.AllocationClosure:
			if leftAllocation.Form() != program.AllocationFormEmpty || leftAllocation.FieldCount() != 0 {
				t.Fatalf("closure allocation %d = form %d fields %d, want empty/zero", index, leftAllocation.Form(), leftAllocation.FieldCount())
			}
			leftTarget, leftTargetOK := leftAllocation.ClosureTarget()
			rightTarget, rightTargetOK := rightAllocation.ClosureTarget()
			leftBody, leftBodyOK := leftTarget.Body()
			leftFunction, leftFunctionOK := leftTarget.Function()
			leftCallTarget, leftCallTargetOK := leftTarget.CallTarget()
			leftFunctionBody, leftFunctionBodyOK := leftFunction.Body()
			rightBody, rightBodyOK := rightTarget.Body()
			rightFunction, rightFunctionOK := rightTarget.Function()
			rightCallTarget, rightCallTargetOK := rightTarget.CallTarget()
			rightFunctionBody, rightFunctionBodyOK := rightFunction.Body()
			if !leftTargetOK || !rightTargetOK || !leftBodyOK || !leftFunctionOK || !leftCallTargetOK || !rightBodyOK || !rightFunctionOK || !rightCallTargetOK || !leftFunctionBodyOK || !rightFunctionBodyOK || !leftFunctionBody.Equal(leftBody) || !rightFunctionBody.Equal(rightBody) || leftBody.ContextID() != rightBody.ContextID() || leftCallTarget.ContextID() != leftBody.ContextID() || rightCallTarget.ContextID() != rightBody.ContextID() {
				t.Fatalf("closure %d target proofs were not exact/replay-stable", index)
			}
		case program.AllocationTable:
			if !leftAllocation.Form().Valid() {
				t.Fatalf("table allocation %d has invalid form", index)
			}
			for fieldIndex := 0; fieldIndex < leftAllocation.FieldCount(); fieldIndex++ {
				field, fieldOK := leftAllocation.FieldAt(fieldIndex)
				rightField, rightFieldOK := rightAllocation.FieldAt(fieldIndex)
				if !fieldOK || !rightFieldOK || !field.Available() || !leftAllocations.OwnsField(field) || field.ID() != rightField.ID() || field.Owns(rightInput) {
					t.Fatalf("table allocation %d field %d is not ordered/owner-fenced", index, fieldIndex)
				}
				leftFieldGeometry, leftFieldOK := leftGeometry.FieldAt(fieldIndex)
				rightFieldGeometry, rightFieldOK := rightGeometry.FieldAt(fieldIndex)
				leftKind, leftKindOK := leftFieldGeometry.Kind()
				if !leftFieldOK || !rightFieldOK || !leftFieldGeometry.Available() || !leftFieldGeometry.BelongsTo(leftGeometry) || !leftKindOK || leftKind == 0 {
					t.Fatalf("table allocation %d field %d geometry is unavailable", index, fieldIndex)
				}
				leftValues, leftWidth, leftOpen, leftValuesOK := leftFieldGeometry.Values()
				_, rightWidth, rightOpen, rightValuesOK := rightFieldGeometry.Values()
				if !leftValuesOK || !rightValuesOK || leftValues == 0 || leftWidth != rightWidth || leftOpen != rightOpen {
					t.Fatalf("table allocation %d field %d values geometry mismatch", index, fieldIndex)
				}
			}
			if _, ok := leftAllocation.FieldAt(leftAllocation.FieldCount()); ok {
				t.Fatalf("table allocation %d accepted out-of-range field", index)
			}
		}
	}
	if _, ok := leftAllocations.At(leftAllocations.Count()); ok {
		t.Fatal("allocation view accepted out-of-range index")
	}
	hotAllocation, hotAllocationOK := leftAllocations.At(0)
	if !hotAllocationOK {
		t.Fatal("allocation view lost first hot handle")
	}
	hotField, hotFieldOK := hotAllocation.FieldAt(0)
	if hotFieldOK {
		if allocations := testing.AllocsPerRun(1000, func() {
			_ = hotAllocation.Available()
			_ = hotAllocation.Owns(leftInput)
			_ = leftAllocations.Owns(hotAllocation)
			_ = hotField.Available()
			_ = hotField.Owns(leftInput)
			_ = leftAllocations.OwnsField(hotField)
		}); allocations != 0 {
			t.Fatalf("allocation hot fences allocated %v times", allocations)
		}
	}
	foreign, err := lualower.Lower(lualower.Source{Name: "allocation-roles-foreign.lua", Text: []byte("return {}")})
	if err != nil {
		t.Fatalf("Lower(foreign): %v", err)
	}
	foreignAllocations := foreign.TransformerInput().Allocations()
	first, firstOK := leftAllocations.At(0)
	if !firstOK || foreignAllocations.Owns(first) || first.Owns(foreign.TransformerInput()) {
		t.Fatal("foreign allocation owner fence accepted a foreign handle")
	}
	foreignFirst, foreignFirstOK := foreignAllocations.At(0)
	foreignGeometry := foreignFirst.Geometry()
	if !foreignFirstOK || !foreignGeometry.Available() || first.Geometry().Allocation().Owns(foreign.TransformerInput()) || foreignGeometry.Allocation().Owns(leftInput) {
		t.Fatal("allocation geometry crossed its Program owner fence")
	}
	if foreignField, ok := foreignGeometry.FieldAt(0); ok && first.Geometry().FieldCount() > 0 && foreignField.BelongsTo(first.Geometry()) {
		t.Fatal("foreign allocation geometry issued a local field proof")
	}
	var zero program.TransformerInput
	if zero.Allocations().Count() != 0 {
		t.Fatal("zero TransformerInput exposed allocations")
	}
}

func TestTransformerAllocationSemanticOccurrencesAreLocalAndGeometrySensitive(t *testing.T) {
	left, err := lualower.Lower(lualower.Source{Name: "allocation-semantic.lua", Text: []byte(`
local first = {value = 1}
local second = {value = 1}
return first, second
`)})
	if err != nil {
		t.Fatalf("Lower(left): %v", err)
	}
	replay, err := lualower.Lower(lualower.Source{Name: "allocation-semantic.lua", Text: []byte(`
local first = {value = 1}
local second = {value = 1}
return first, second
`)})
	if err != nil {
		t.Fatalf("Lower(replay): %v", err)
	}
	leftAllocations := left.TransformerInput().Allocations()
	replayAllocations := replay.TransformerInput().Allocations()
	if leftAllocations.Count() < 2 || replayAllocations.Count() != leftAllocations.Count() {
		t.Fatalf("allocation counts = %d/%d, want two replay-stable allocations", leftAllocations.Count(), replayAllocations.Count())
	}
	first, firstOK := leftAllocations.At(0)
	second, secondOK := leftAllocations.At(1)
	replayFirst, replayFirstOK := replayAllocations.At(0)
	replaySecond, replaySecondOK := replayAllocations.At(1)
	if !firstOK || !secondOK || !replayFirstOK || !replaySecondOK {
		t.Fatal("semantic occurrence fixture omitted allocations")
	}
	firstOccurrence := first.SemanticOccurrence()
	secondOccurrence := second.SemanticOccurrence()
	if !firstOccurrence.Available() || !secondOccurrence.Available() || firstOccurrence.ID() == secondOccurrence.ID() || first.Template().ID() == second.Template().ID() {
		t.Fatal("same-body sibling allocations were not locally distinct")
	}
	if firstOccurrence.ID() != replayFirst.SemanticOccurrence().ID() || secondOccurrence.ID() != replaySecond.SemanticOccurrence().ID() || first.Template().ID() != replayFirst.Template().ID() || second.Template().ID() != replaySecond.Template().ID() {
		t.Fatal("semantic occurrence/template changed across equivalent replay")
	}

	changed, err := lualower.Lower(lualower.Source{Name: "allocation-semantic-changed.lua", Text: []byte(`
	local first = {value = 2, other = 3}
return first
`)})
	if err != nil {
		t.Fatalf("Lower(changed): %v", err)
	}
	changedAllocation, changedOK := changed.TransformerInput().Allocations().At(0)
	if !changedOK || changedAllocation.Template().ID() == first.Template().ID() {
		t.Fatal("field selector/value geometry mutation did not change template identity")
	}
}

func TestTransformerAllocationOccurrenceUsesOwnerLocalPaths(t *testing.T) {
	base := lowerAllocationPathFixture(t, `
local function target()
  return {}
end
return target
`)
	rootPrior := lowerAllocationPathFixture(t, `
local unrelated = {}
local function target()
  return {}
end
return target
`)
	sameBodyPrior := lowerAllocationPathFixture(t, `
local function target()
  return {}, {}
end
return target
`)
	siblingPrior := lowerAllocationPathFixture(t, `
local function sibling() return {} end
local function target()
  return {}
end
return target
`)
	baseClosure := closureAllocation(t, base)
	baseTarget, baseTargetOK := baseClosure.ClosureTarget()
	baseBody, baseBodyOK := baseTarget.Body()
	if !baseTargetOK || !baseBodyOK {
		t.Fatal("base nested target unavailable")
	}
	baseInner := allocationForBody(t, base, baseBody)
	rootInner := allocationForBody(t, rootPrior, bodyByClosure(t, rootPrior, "target"))
	if baseInner.Template().ID() != rootInner.Template().ID() || baseInner.SemanticOccurrence().ID() != rootInner.SemanticOccurrence().ID() {
		t.Fatal("unrelated root allocation renamed nested allocation")
	}
	sameBodyInner := allocationForBody(t, sameBodyPrior, bodyByClosure(t, sameBodyPrior, "target"))
	if sameBodyInner.SemanticOccurrence().ID() == baseInner.SemanticOccurrence().ID() {
		t.Fatal("same-body insertion failed to change nested occurrence")
	}
	siblingInner := allocationForBody(t, siblingPrior, bodyByClosureAt(t, siblingPrior, 1))
	if siblingInner.SemanticOccurrence().ID() == baseInner.SemanticOccurrence().ID() {
		t.Fatal("sibling-function insertion failed to change closure path")
	}
}

func TestTransformerAllocationRootOccurrencesKeepControlArmsOwnerLocal(t *testing.T) {
	branch := lowerAllocationPathFixture(t, `
local function target()
  if cond then return {} else return {} end
end
return target
`)
	branchPrior := lowerAllocationPathFixture(t, `
local unrelated = {}
local function target()
  if cond then return {} else return {} end
end
return target
`)
	branchTables := tableAllocations(t, branch)
	branchPriorTables := tableAllocations(t, branchPrior)
	if len(branchTables) != 2 || len(branchPriorTables) != 3 {
		t.Fatalf("branch table counts = %d/%d, want 2/3", len(branchTables), len(branchPriorTables))
	}
	if branchTables[0].SemanticOccurrence().ID() == branchTables[1].SemanticOccurrence().ID() {
		t.Fatal("true/false branch arms shared an allocation occurrence")
	}
	for _, want := range branchTables {
		found := false
		for _, got := range branchPriorTables {
			if want.SemanticOccurrence().ID() == got.SemanticOccurrence().ID() && want.Template().ID() == got.Template().ID() {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("unrelated preceding root changed a nested branch allocation")
		}
	}

	loop := lowerAllocationPathFixture(t, `
local function target()
  while cond do return {} end
end
return target
`)
	loopPrior := lowerAllocationPathFixture(t, `
local unrelated = {}
local function target()
  while cond do return {} end
end
return target
`)
	loopTables := tableAllocations(t, loop)
	loopPriorTables := tableAllocations(t, loopPrior)
	if len(loopTables) != 1 || len(loopPriorTables) != 2 || loopTables[0].SemanticOccurrence().ID() != loopPriorTables[1].SemanticOccurrence().ID() {
		t.Fatal("unrelated preceding root changed a nested loop allocation")
	}

	direct := lowerAllocationPathFixture(t, `
local function target()
  return {}
end
return target
`)
	directPrior := lowerAllocationPathFixture(t, `
local unrelated = {}
local function target()
  return {}
end
return target
`)
	directTables := tableAllocations(t, direct)
	directPriorTables := tableAllocations(t, directPrior)
	if len(directTables) != 1 || len(directPriorTables) != 2 || directTables[0].SemanticOccurrence().ID() != directPriorTables[1].SemanticOccurrence().ID() {
		t.Fatal("unrelated preceding root changed a direct child Body allocation")
	}
}

func tableAllocations(t *testing.T, p *program.Program) []program.Allocation {
	t.Helper()
	allocations := p.TransformerInput().Allocations()
	result := make([]program.Allocation, 0, allocations.Count())
	for index := 0; index < allocations.Count(); index++ {
		allocation, ok := allocations.At(index)
		if ok && allocation.Role() == program.AllocationTable {
			result = append(result, allocation)
		}
	}
	return result
}

func lowerAllocationPathFixture(t *testing.T, text string) *program.Program {
	t.Helper()
	p, err := lualower.Lower(lualower.Source{Name: "allocation-path.lua", Text: []byte(text)})
	if err != nil {
		t.Fatalf("Lower(path): %v", err)
	}
	return p
}

func closureAllocation(t *testing.T, p *program.Program) program.Allocation {
	t.Helper()
	allocations := p.TransformerInput().Allocations()
	for index := 0; index < allocations.Count(); index++ {
		allocation, ok := allocations.At(index)
		if ok && allocation.Role() == program.AllocationClosure {
			return allocation
		}
	}
	t.Fatal("closure allocation unavailable")
	return program.Allocation{}
}

func bodyByClosure(t *testing.T, p *program.Program, _ string) program.Body {
	return bodyByClosureAt(t, p, 0)
}

func bodyByClosureAt(t *testing.T, p *program.Program, want int) program.Body {
	t.Helper()
	allocations := p.TransformerInput().Allocations()
	seen := 0
	for index := 0; index < allocations.Count(); index++ {
		allocation, allocationOK := allocations.At(index)
		if !allocationOK || allocation.Role() != program.AllocationClosure {
			continue
		}
		if seen != want {
			seen++
			continue
		}
		target, ok := allocation.ClosureTarget()
		if !ok {
			t.Fatal("closure target unavailable")
		}
		body, bodyOK := target.Body()
		if !bodyOK {
			t.Fatal("closure body unavailable")
		}
		return body
	}
	t.Fatal("requested closure unavailable")
	return program.Body{}
}

func allocationForBody(t *testing.T, p *program.Program, body program.Body) program.Allocation {
	t.Helper()
	allocations := p.TransformerInput().Allocations()
	var found program.Allocation
	foundOK := false
	for index := 0; index < allocations.Count(); index++ {
		allocation, ok := allocations.At(index)
		if !ok {
			continue
		}
		term, termOK := allocation.Geometry().RootTerm()
		contained, containedOK := p.TransformerInput().ContainingBody(term)
		if termOK && containedOK && contained.Equal(body) && allocation.Role() == program.AllocationTable {
			found, foundOK = allocation, true
		}
	}
	if foundOK {
		return found
	}
	t.Fatal("nested table allocation unavailable")
	return program.Allocation{}
}
