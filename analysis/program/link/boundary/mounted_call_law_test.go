package boundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target"
)

func TestMountedCallProjectionOwnsMethodArgumentsOpenTailAndResult(t *testing.T) {
	p, err := lower.Lower(lower.Source{Name: "boundary-mounted-call.lua", Text: []byte(`
local function many(...) return ... end
local receiver = {}
receiver:invoke(1, 2, many(3))
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	component, project := mountedCallBoundary(t, p, contract)

	mounted, occurrence := mountedMethodCall(t, project)
	shard, shardOK := mounted.Mount()
	values, ok := occurrence.Values()
	if !shardOK || !ok || values.Count() != 2 {
		t.Fatalf("method fixed actuals=%d/%t, want 2", values.Count(), ok)
	}
	calleeProof, calleeProofOK := occurrence.Callee()
	calleeSpan, calleeSpanOK := calleeProof.Span()
	callee, calleeOK := component.Calls().MountedCallCallee(mounted, occurrence)
	calleeForSpan, calleeForSpanOK := component.Values().ForProgramSpan(shard, calleeSpan)
	module, moduleOK := project.ModuleKey(shard)
	calleeForSemantic, calleeForSemanticOK := component.Values().ForMountedSemantic(module, calleeProof.ContextID())
	if !calleeProofOK || !calleeSpanOK || !calleeOK || !calleeForSpanOK || !moduleOK || !calleeForSemanticOK || calleeForSpan != callee || calleeForSemantic != callee {
		t.Fatal("callee Span did not bind to the sole Boundary Value projection")
	}
	form, receiver, actuals, ok := component.Calls().MountedCallOperands(mounted, occurrence)
	if !ok || form != flow.CallFormMethod || receiver == (Value{}) || actuals == (Value{}) || receiver == actuals {
		t.Fatal("sealed method operand projection")
	}
	arguments := make([]Value, values.Count())
	for index := range arguments {
		argument, argumentOK := values.At(index)
		arguments[index], argumentOK = component.Calls().MountedCallArgument(mounted, argument)
		argumentSpan, spanOK := argument.Span()
		forSpan, forSpanOK := component.Values().ForProgramSpan(shard, argumentSpan)
		forSemantic, forSemanticOK := component.Values().ForMountedSemantic(module, argument.ContextID())
		if !argumentOK || !spanOK || !forSpanOK || !forSemanticOK || forSpan != arguments[index] || forSemantic != arguments[index] || arguments[index] == (Value{}) || arguments[index] == receiver || arguments[index] == actuals {
			t.Fatalf("ordered argument %d projection", index)
		}
	}
	if arguments[0] == arguments[1] {
		t.Fatal("ordered method arguments collapsed")
	}
	tailValue, tailSpan, open := component.Calls().MountedCallActualTail(mounted, values)
	tailForSpan, tailForSpanOK := component.Values().ForProgramSpan(shard, tailSpan)
	if !open || tailValue == (Value{}) || !tailSpan.Available() || !tailForSpanOK || tailForSpan != tailValue {
		t.Fatal("open actual tail projection")
	}
	mountedProgram, mountedProgramOK := project.Mounts().Program(shard)
	if !mountedProgramOK || mountedProgram == nil || !mountedProgram.TransformerInput().OwnsSpan(tailSpan) {
		t.Fatal("open actual tail lost its exact Program owner")
	}
	result, resultOK := component.Calls().MountedCallResult(mounted, occurrence)
	resultForSemantic, resultForSemanticOK := component.Values().ForMountedSemantic(module, occurrence.ContextID())
	if !resultOK || !resultForSemanticOK || resultForSemantic != result || result == (Value{}) || result == receiver || result == actuals || result == tailValue {
		t.Fatal("mounted call result projection")
	}

	plain, plainOccurrence := mountedPlainCall(t, project)
	plainValues, plainValuesOK := plainOccurrence.Values()
	plainForm, plainReceiver, _, plainOK := component.Calls().MountedCallOperands(plain, plainOccurrence)
	if !plainValuesOK || !plainOK || plainForm != flow.CallFormPlain || plainReceiver != (Value{}) {
		t.Fatal("plain mounted call projection")
	}
	if _, _, open := component.Calls().MountedCallActualTail(plain, plainValues); open {
		t.Fatal("closed plain call acquired an open-tail proof")
	}
	if _, _, _, ok := component.Calls().MountedCallOperands(mounted, plainOccurrence); ok {
		t.Fatal("sibling occurrence spliced into mounted call projection")
	}
	plainArgument, plainArgumentOK := plainValues.At(0)
	if plainArgumentOK {
		if _, ok := component.Calls().MountedCallArgument(mounted, plainArgument); ok {
			t.Fatal("sibling argument spliced into mounted method projection")
		}
	}

	foreign, foreignProject := mountedCallBoundary(t, p, contract)
	foreignMounted, foreignOccurrence := mountedMethodCall(t, foreignProject)
	if foreignMounted.ContextID() != mounted.ContextID() {
		t.Fatal("equivalent replay changed formal mounted-call identity")
	}
	if _, _, _, ok := component.Calls().MountedCallOperands(foreignMounted, foreignOccurrence); ok {
		t.Fatal("equivalent foreign mounted proof crossed Boundary owner")
	}
	if _, _, _, ok := foreign.Calls().MountedCallOperands(mounted, occurrence); ok {
		t.Fatal("local mounted proof crossed foreign Boundary owner")
	}
	if _, _, _, ok := component.Calls().MountedCallOperands(linkproject.CallApplication{}, program.CallOccurrence{}); ok {
		t.Fatal("zero mounted proof acquired operands")
	}
	if _, ok := component.Values().ForProgramSpan(linkproject.Shard{}, program.Span{}); ok {
		t.Fatal("zero Program Span acquired a Boundary Value")
	}
	if allocations := testing.AllocsPerRun(10_000, func() {
		_, _, _, operandsOK := component.Calls().MountedCallOperands(mounted, occurrence)
		_, calleeOK := component.Calls().MountedCallCallee(mounted, occurrence)
		_, spanOK := component.Values().ForProgramSpan(shard, calleeSpan)
		_, _, tailOK := component.Calls().MountedCallActualTail(mounted, values)
		_, resultOK := component.Calls().MountedCallResult(mounted, occurrence)
		if !operandsOK || !calleeOK || !spanOK || !tailOK || !resultOK {
			panic("sealed mounted call projection became unavailable")
		}
	}); allocations != 0 {
		t.Fatalf("mounted call projection allocations = %g, want 0", allocations)
	}
}

func mountedCallBoundary(t testing.TB, p *program.Program, contract *target.Contract) (*Component, *linkproject.Component) {
	t.Helper()
	draft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: p}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	boundaryDraft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	component, err := boundaryDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	return component, project
}

func mountedMethodCall(t testing.TB, project *linkproject.Component) (linkproject.CallApplication, program.CallOccurrence) {
	return mountedCallWithForm(t, project, flow.CallFormMethod)
}

func mountedPlainCall(t testing.TB, project *linkproject.Component) (linkproject.CallApplication, program.CallOccurrence) {
	return mountedCallWithForm(t, project, flow.CallFormPlain)
}

func mountedCallWithForm(t testing.TB, project *linkproject.Component, want flow.CallForm) (linkproject.CallApplication, program.CallOccurrence) {
	t.Helper()
	calls := project.Applications().Calls()
	for index := 0; index < calls.Count(); index++ {
		mounted, mountedOK := calls.MountedAt(index)
		occurrence, occurrenceOK := mounted.Occurrence()
		form, formOK := occurrence.Form()
		if mountedOK && occurrenceOK && formOK && form == want {
			return mounted, occurrence
		}
	}
	t.Fatalf("mounted Call form %v unavailable", want)
	return linkproject.CallApplication{}, program.CallOccurrence{}
}
