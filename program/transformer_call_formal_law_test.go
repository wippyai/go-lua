package program_test

import (
	"testing"

	"github.com/wippyai/go-lua/program"
	programlower "github.com/wippyai/go-lua/program/lower"
)

func TestTransformerCallFormalIgnoresGlobalRenumberingButKeepsLocalGeometry(t *testing.T) {
	base := lowerCallFormal(t, `
local function target()
  sink(1)
end
return target
`)
	rootPrior := lowerCallFormal(t, `
sink(0)
local function target()
  sink(1)
end
return target
`)
	sameBodyPrior := lowerCallFormal(t, `
local function target()
  sink(0)
  sink(1)
end
return target
`)

	baseCall := callOccurrenceAt(t, base, 0)
	renumberedCall := callOccurrenceAt(t, rootPrior, 1)
	shiftedCall := callOccurrenceAt(t, sameBodyPrior, 1)
	baseFormal, baseOK := baseCall.Formal()
	renumberedFormal, renumberedOK := renumberedCall.Formal()
	shiftedFormal, shiftedOK := shiftedCall.Formal()
	if !baseOK || !renumberedOK || !shiftedOK || !baseFormal.Equal(renumberedFormal) || baseFormal.ID() != renumberedFormal.ID() {
		t.Fatal("unrelated root Call renumbered a nested semantic CallFormal")
	}
	if baseFormal.Equal(shiftedFormal) || baseFormal.ID() == shiftedFormal.ID() {
		t.Fatal("same-Body call insertion did not change local CallFormal position")
	}
	baseInput := base.TransformerInput()
	if !baseInput.OwnsCallFormal(renumberedFormal, baseCall) {
		t.Fatal("equivalent owner-neutral CallFormal did not bind through current exact occurrence")
	}
	if baseInput.OwnsCallFormal(baseFormal, renumberedCall) || baseInput.OwnsCallFormal(program.CallFormal{}, baseCall) {
		t.Fatal("foreign occurrence or zero CallFormal crossed the exact Program owner fence")
	}
}

func TestTransformerCallFormalCommitsCallFormAndOperandGeometry(t *testing.T) {
	plain := lowerCallFormal(t, `sink(1)`)
	method := lowerCallFormal(t, `
local receiver = {}
receiver:sink(1)
`)
	closed := lowerCallFormal(t, `
local function forward(...)
  sink(1)
end
return forward
`)
	open := lowerCallFormal(t, `
local function forward(...)
  sink(...)
end
return forward
`)
	plainFormal, plainOK := callOccurrenceAt(t, plain, 0).Formal()
	methodFormal, methodOK := callOccurrenceAt(t, method, 0).Formal()
	closedFormal, closedOK := callOccurrenceAt(t, closed, 0).Formal()
	openFormal, openOK := callOccurrenceAt(t, open, 0).Formal()
	if !plainOK || !methodOK || !closedOK || !openOK || plainFormal.Equal(methodFormal) || closedFormal.Equal(openFormal) {
		t.Fatal("CallFormal omitted call form, operand width, or open-tail geometry")
	}
}

func lowerCallFormal(t testing.TB, text string) *program.Program {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "call-formal.lua", Text: []byte(text)})
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	return p
}

func callOccurrenceAt(t testing.TB, p *program.Program, index int) program.CallOccurrence {
	t.Helper()
	call, ok := p.TransformerInput().CallAt(index)
	if !ok {
		t.Fatalf("CallAt(%d)", index)
	}
	return call
}
