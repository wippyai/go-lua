package lower_test

import "testing"

func TestFlowActivationSeparatesNestedFunctionBodies(t *testing.T) {
	p := parseBindLower(t, `
local function outer()
  local function inner()
    return 1
  end
  return inner()
end
return outer()
`)
	functions := p.Flow().Authored().Functions()
	if functions.Count() != 2 {
		t.Fatalf("FunctionCount = %d, want 2", functions.Count())
	}
	outer, _ := functions.At(0)
	inner, _ := functions.At(1)
	_, outerBody, _, outerOK := functions.Get(outer)
	_, innerBody, _, innerOK := functions.Get(inner)
	if !outerOK || !innerOK || outerBody == 0 || innerBody == 0 {
		t.Fatal("nested Functions have malformed Bodies")
	}
	outerActivation, outerActivationOK := p.Flow().Activation().For(outerBody)
	innerActivation, innerActivationOK := p.Flow().Activation().For(innerBody)
	if !outerActivationOK || !innerActivationOK || outerActivation != outer || innerActivation != inner || outerActivation == innerActivation {
		t.Fatalf("nested activation owners = %v/%v and %v/%v", outerActivation, outerActivationOK, innerActivation, innerActivationOK)
	}
}

func TestFlowActivationDoesNotMakeDeadFunctionExecutable(t *testing.T) {
	p := parseBindLower(t, `
do return end
local function dead()
  return deadCall()
end
`)
	function, _ := p.Flow().Authored().Functions().At(0)
	_, body, _, functionOK := p.Flow().Authored().Functions().Get(function)
	call, callOK := p.Flow().Authored().Calls().At(0)
	if !functionOK || !callOK {
		t.Fatal("dead function shape is absent")
	}
	executable := p.Flow().Executable()
	if executable.Contains(function) || executable.Contains(body) || executable.Contains(call) {
		t.Fatal("dead source retained executable function state")
	}
}

func TestFlowActivationIsAlphaStable(t *testing.T) {
	left := lowerNamed(t, "activation_owner.lua", `local function visit(flag) if flag then return flag end end`)
	right := lowerNamed(t, "activation_owner.lua", `local function inspect(enabled) if enabled then return enabled end end`)
	leftFunction, leftOK := left.Flow().Authored().Functions().At(0)
	rightFunction, rightOK := right.Flow().Authored().Functions().At(0)
	if !leftOK || !rightOK || leftFunction != rightFunction {
		t.Fatalf("alpha Function identities = %v/%v and %v/%v", leftFunction, leftOK, rightFunction, rightOK)
	}
	_, leftBody, _, _ := left.Flow().Authored().Functions().Get(leftFunction)
	_, rightBody, _, _ := right.Flow().Authored().Functions().Get(rightFunction)
	leftActivation, leftActivationOK := left.Flow().Activation().For(leftBody)
	rightActivation, rightActivationOK := right.Flow().Activation().For(rightBody)
	if !leftActivationOK || !rightActivationOK || leftActivation != rightActivation || leftActivation != leftFunction {
		t.Fatalf("alpha Activation = %v/%v and %v/%v", leftActivation, leftActivationOK, rightActivation, rightActivationOK)
	}
}

func TestFlowActivationQueryDoesNotAllocate(t *testing.T) {
	p := parseBindLower(t, `local function child() return 1 end; return child()`)
	function, _ := p.Flow().Authored().Functions().At(0)
	_, body, _, _ := p.Flow().Authored().Functions().Get(function)
	allocations := testing.AllocsPerRun(1000, func() {
		_, _ = p.Flow().Activation().For(body)
	})
	if allocations != 0 {
		t.Fatalf("Activation.For allocates %f times", allocations)
	}
}
