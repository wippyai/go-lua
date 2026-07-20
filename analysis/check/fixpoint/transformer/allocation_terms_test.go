package transformer

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestAllocationResultAndEffectShareOneTemplateTerm(t *testing.T) {
	reg := standard.Registry()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("allocation-result-effect")))
	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	template, ok := effectlowering.StaticSignatureAllocationTemplate(sig)
	if !ok {
		t.Fatal("table.create template rejected")
	}
	op, _ := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
		Owner: 41, Template: template.Root, Ordinal: 7,
	}, template)
	arena := NewArena(reg)
	allocation := arena.AllocationTemplate(op)
	result := arena.AllocationResultValue(allocation, 0)
	effects := NewEffectArena(arena)
	effect, err := effects.AllocationTemplate(allocation)
	if err != nil {
		t.Fatal(err)
	}
	if !arena.bindLexicalOwner(owner) {
		t.Fatal("allocation arena owner rejected")
	}
	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	value, exact := arena.evalValue(result, cursor, SpecializationContext{})
	resolved, resolvedExact := effects.resolve(effect, cursor, SpecializationContext{})
	if !exact || !resolvedExact || resolved.Kind != EffectAllocationTemplate || !product.Equal(reg, value, resolved.Allocation.Result) {
		t.Fatalf("shared allocation value/effect exact=%v/%v resolved=%#v", exact, resolvedExact, resolved)
	}
	identityTerm, singleton := product.Get(reg, value, identity.Key).Term()
	templateIdentity, symbolic := identityTerm.Allocation()
	if !singleton || !symbolic || !templateIdentity.Valid() {
		t.Fatalf("allocation result identity = %#v/%v, want one symbolic allocation term", identityTerm, singleton)
	}

	callerTerms := NewArena(reg)
	callerEffects := NewEffectArena(callerTerms)
	bindings, _ := NewTermRootBindings(Shape{}, Shape{}, nil, nil)
	rebased, err := RebaseEffectDAGs(callerEffects, effects, bindings, []EffectTerm{effect})
	if err != nil || len(rebased.Effects) != 1 {
		t.Fatalf("allocation effect rebase = %#v/%v", rebased, err)
	}
	if !callerTerms.bindLexicalOwner(owner) {
		t.Fatal("rebased allocation arena owner rejected")
	}
	again, ok := callerEffects.resolve(rebased.Effects[0], cursor, SpecializationContext{})
	if !ok || again.Allocation.Site != op.Site() || !product.Equal(reg, again.Allocation.Result, value) {
		t.Fatalf("rebased allocation = %#v/%v", again, ok)
	}
}

func TestEffectTargetTermAllocationCanonicalRebaseAndResolution(t *testing.T) {
	reg := standard.Registry()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte("allocation-effect-target")))
	sig, _ := (signaturelookup.Source{IncludeStdlib: true}).Lookup("table.create")
	template, ok := effectlowering.StaticSignatureAllocationTemplate(sig)
	if !ok {
		t.Fatal("table.create template rejected")
	}
	op, _ := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
		Owner: 71, Template: template.Root, Ordinal: 3,
	}, template)
	calleeTerms := NewArena(reg)
	allocation := calleeTerms.AllocationTemplate(op)
	otherOp, _ := operationplan.NewSignatureAllocationOperation(operationplan.SignatureAllocationSite{
		Owner: 71, Template: template.Root, Ordinal: 5,
	}, template)
	otherAllocation := calleeTerms.AllocationTemplate(otherOp)
	allocationTarget := AllocationEffectTarget(allocation)
	key := calleeTerms.Constant(typevalue.LiteralString(reg, "key"))
	value := calleeTerms.Constant(typevalue.LiteralString(reg, "value"))
	callee := NewEffectArena(calleeTerms)
	config := IndexMutationConfig{
		Invalidation: InvalidatePathConfig{Target: allocationTarget, Scope: InvalidationScopeDescendants},
		Table:        allocationTarget, Key: key, Value: value,
		Admission: dynamicindex.AdmissionAdmitted, Readback: factflow.DynamicIndexReadbackKeyAndValue,
		Site: EffectSite{Owner: 71, Ordinal: 4},
	}
	first, err := callee.IndexMutation(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := callee.IndexMutation(config)
	if err != nil || second != first || !callee.Valid(first, Shape{}) {
		t.Fatalf("allocation target was not canonical/valid: first=%d second=%d err=%v", first, second, err)
	}
	if !calleeTerms.bindLexicalOwner(owner) {
		t.Fatal("callee allocation arena owner rejected")
	}

	cursor, _ := NewBindingCursor(Shape{}, nil, nil)
	resolved, exact := callee.resolve(first, cursor, SpecializationContext{})
	invalidated, invalidationExact := resolved.Mutation.Invalidation.TargetRef.Allocation()
	mutated, mutationExact := resolved.Mutation.TableTarget.Allocation()
	if !exact || !invalidationExact || !mutationExact || invalidated.Site != op.Site() || mutated.Site != op.Site() ||
		!resolved.Mutation.Invalidation.Target.IsEmpty() || !resolved.Mutation.Table.IsEmpty() {
		t.Fatalf("allocation target resolution = %#v exact=%v/%v/%v", resolved, exact, invalidationExact, mutationExact)
	}

	callerTerms := NewArena(reg)
	caller := NewEffectArena(callerTerms)
	bindings, _ := NewTermRootBindings(Shape{}, Shape{}, nil, nil)
	rebased, err := RebaseEffectDAGs(caller, callee, bindings, []EffectTerm{first})
	if err != nil || len(rebased.Effects) != 1 {
		t.Fatalf("allocation target rebase = %#v/%v", rebased, err)
	}
	if !callerTerms.bindLexicalOwner(owner) {
		t.Fatal("caller allocation arena owner rejected")
	}
	again, exact := caller.resolve(rebased.Effects[0], cursor, SpecializationContext{})
	rebasedTarget, targetExact := again.Mutation.TableTarget.Allocation()
	if !exact || !targetExact || rebasedTarget.Site != op.Site() {
		t.Fatalf("rebased allocation target = %#v exact=%v/%v", again, exact, targetExact)
	}

	malformed := EffectTargetTerm{kind: effectTargetPath, path: PathTerm(1), allocation: allocation}
	config.Table = malformed
	if _, err := callee.IndexMutation(config); err == nil {
		t.Fatal("target containing both path and allocation was admitted")
	}

	otherTarget := AllocationEffectTarget(otherAllocation)
	config.Table = otherTarget
	if _, err := callee.IndexMutation(config); err == nil {
		t.Fatal("invalidation of one allocation paired with mutation of another")
	}
	pathTarget := PathEffectTarget(calleeTerms.Path(Root{Kind: RootGlobal, Index: 0}))
	config.Table = pathTarget
	if _, err := callee.IndexMutation(config); err == nil {
		t.Fatal("allocation invalidation paired with boundary-path mutation")
	}
}

func TestEffectTargetTermPreservesBoundaryPathResolution(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 1}
	terms := NewArena(reg)
	path := terms.Path(Root{Kind: RootParam, Index: 0})
	target := PathEffectTarget(path)
	value := terms.Constant(typevalue.LiteralString(reg, "value"))
	effects := NewEffectArena(terms)
	term, err := effects.IndexMutation(IndexMutationConfig{
		Invalidation: InvalidatePathConfig{Target: target, Scope: InvalidationScopeDescendants},
		Table:        target, Key: value, Value: value,
		Admission: dynamicindex.AdmissionAdmitted, Readback: factflow.DynamicIndexReadbackKeyAndValue,
		Site: EffectSite{Owner: 72, Ordinal: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := pathdom.NewPlaceholder(0)
	cursor, _ := NewBindingCursor(shape, []product.Value{product.Top()}, []pathdom.Path{want})
	resolved, exact := effects.resolve(term, cursor, SpecializationContext{})
	table, tableExact := resolved.Mutation.TableTarget.Path()
	invalidation, invalidationExact := resolved.Mutation.Invalidation.TargetRef.Path()
	if !exact || !tableExact || !invalidationExact || !table.Equal(want) || !invalidation.Equal(want) ||
		!resolved.Mutation.Table.Equal(want) || !resolved.Mutation.Invalidation.Target.Equal(want) {
		t.Fatalf("boundary path target changed: %#v exact=%v/%v/%v", resolved, exact, tableExact, invalidationExact)
	}
}
