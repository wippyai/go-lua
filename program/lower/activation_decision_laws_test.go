package lower_test

import "testing"

// Activation is a final Flow projection from a Body to its executable
// activation owner. Guard inventories are deliberately not a second global
// query plane; Causal Edges retain their own decisions instead.
func TestActivationProjectionBindsFunctionBody(t *testing.T) {
	p := parseBindLower(t, `
local function child(flag)
  if flag then return end
end
`)
	functions := p.Flow().Authored().Functions()
	function, ok := functions.At(0)
	if !ok {
		t.Fatal("Function is absent")
	}
	_, body, _, ok := functions.Get(function)
	if !ok || body == 0 {
		t.Fatal("Function has no Body")
	}
	if activation, ok := p.Flow().Activation().For(body); !ok || activation != function {
		t.Fatalf("Activation(%v) = %v/%v, want Function %v", body, activation, ok, function)
	}
	if count, ok := p.Flow().Causal().Edges().ActivationCount(body); !ok || count == 0 {
		t.Fatalf("final activation Edge count = %d/%v, want live body edges", count, ok)
	}
}

func TestDeadClosureBodyIsNotExecutable(t *testing.T) {
	p := parseBindLower(t, `
do return end
local function child()
  if liveWhenCalled then end
end
`)
	function, _ := p.Flow().Authored().Functions().At(0)
	_, body, _, functionOK := p.Flow().Authored().Functions().Get(function)
	branch, branchOK := p.Source().Order().BodyAt(body, 0)
	if !functionOK || !branchOK {
		t.Fatal("dead closure shape is absent")
	}
	executable := p.Flow().Executable()
	if executable.Contains(function) || executable.Contains(body) || executable.Contains(branch) {
		t.Fatal("closure behind terminal source control became executable")
	}
	if count, ok := p.Flow().Causal().Edges().ActivationCount(body); ok && count != 0 {
		t.Fatalf("dead Function Body retained activation edges = %d/%v", count, ok)
	}
}

func TestActivationProjectionIsAlphaStable(t *testing.T) {
	left := lowerNamed(t, "activation_alpha.lua", `local function visit(flag) if flag then return end end`)
	right := lowerNamed(t, "activation_alpha.lua", `local function inspect(enabled) if enabled then return end end`)
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
		t.Fatalf("alpha activation owners = %v/%v and %v/%v", leftActivation, leftActivationOK, rightActivation, rightActivationOK)
	}
}

func TestActivationQueryDoesNotAllocate(t *testing.T) {
	p := parseBindLower(t, `local function f() return 1 end`)
	function, _ := p.Flow().Authored().Functions().At(0)
	_, body, _, _ := p.Flow().Authored().Functions().Get(function)
	allocations := testing.AllocsPerRun(1000, func() {
		_, _ = p.Flow().Activation().For(body)
	})
	if allocations != 0 {
		t.Fatalf("Activation.For allocates %f times", allocations)
	}
}
