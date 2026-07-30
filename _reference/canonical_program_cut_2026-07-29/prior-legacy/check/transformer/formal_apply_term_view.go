package transformer

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// formalApplyTermView is a borrowed evaluation of one sealed target term under
// one immutable call-frame environment.  It is deliberately not expression
// syntax: the only nodes remain in Arena and their only executable semantics
// remains evalValueCanonical/resolveValueNodeProduct.
//
// The value and optional path are stored by formalQualifiedBinding so they
// remain one correlated alternative.  This small value stores only the exact
// environment needed to interpret their target IN roots.
type formalApplyTermView struct {
	binding     relationLazyApplyBinding
	callerScope loopMuTerm
}

// formalApplyFrameEnvironment separates leaf observations by lexical owner.
// A target FrameResult/Middle term must never be interpreted by the caller's
// equally-numbered frame/register resolver, and vice versa.
type formalApplyFrameEnvironment struct {
	caller SpecializationContext
	target SpecializationContext
}

func newFormalApplyTermView(binding relationLazyApplyBinding, value ValueTerm, path PathTerm, callerScope loopMuTerm) (formalApplyTermView, error) {
	view := formalApplyTermView{binding: binding, callerScope: callerScope}
	if !view.valid(value, path) || binding.callerCode == nil || int(binding.frame) >= len(binding.callerCode.applicationGuards) ||
		!binding.callerCode.applicationGuards[binding.frame].validFor(binding.frame, binding.target) ||
		binding.callerCode.applicationGuards[binding.frame].callerScope != callerScope {
		return formalApplyTermView{}, fmt.Errorf("transformer: formal Apply term view is malformed")
	}
	return view, nil
}

func (v formalApplyTermView) present() bool { return v.binding.caller != 0 }

func (v formalApplyTermView) valid(value ValueTerm, path PathTerm) bool {
	b := v.binding
	// Both handles come from a relationCode which passed closure validation
	// before its Arena was sealed.  Re-walking that immutable DAG at every call
	// would add an allocation and linear work without strengthening ownership.
	if !v.present() || !b.validFor(b.caller, b.target, b.frame) || !b.targetArena.Sealed() ||
		value == 0 || int(value) >= len(b.targetArena.values) {
		return false
	}
	return path == 0 || int(path) < len(b.targetArena.paths)
}

// bind qualifies exact target handles after newFormalApplyTermView has
// validated them.  Splitting validation from construction keeps the retained
// view a compact comparable value and avoids any run-owned side table.
func (v formalApplyTermView) bind(value ValueTerm, path PathTerm, scope loopMuTerm) formalQualifiedBinding {
	b := v.binding
	out := formalQualifiedBinding{
		value: relationArenaValueRef{owner: b.target, arena: b.targetArena, term: value},
		apply: v, scope: scope,
	}
	if path != 0 {
		out.path = relationArenaPathRef{owner: b.target, arena: b.targetArena, term: path}
		out.pathPresent = true
	}
	return out
}

func (v formalApplyTermView) validFor(caller relationVar, callerArena *Arena, value relationArenaValueRef, path relationArenaPathRef, pathPresent bool) bool {
	b := v.binding
	pathTerm := PathTerm(0)
	if pathPresent {
		pathTerm = path.term
	}
	if !v.valid(value.term, pathTerm) || b.caller != caller || b.callerArena != callerArena ||
		value.owner != b.target || value.arena != b.targetArena ||
		pathPresent && (path.owner != b.target || path.arena != b.targetArena) ||
		!pathPresent && path != (relationArenaPathRef{}) {
		return false
	}
	return true
}

func (b formalQualifiedBinding) validForAuthority(authority *formalComponentTerminalAuthority) bool {
	if authority == nil || authority.terms == nil || authority.code == nil || !b.value.valid() ||
		b.pathPresent && !b.path.valid() || !b.pathPresent && b.path != (relationArenaPathRef{}) {
		return false
	}
	if b.apply.present() {
		return b.apply.validFor(authority.variable, authority.terms, b.value, b.path, b.pathPresent)
	}
	return b.value.owner == authority.variable && b.value.arena == authority.terms &&
		authority.validFormalValue(b.value.term) &&
		(!b.pathPresent || b.path.owner == authority.variable && b.path.arena == authority.terms &&
			authority.terms.validPath(b.path.term, authority.code.shape))
}

// evalValue specializes the borrowed target term without importing it.  The
// frame operands are evaluated in their original caller arena, then the target
// node algebra runs once over a dense BindingCursor.  Thus adding a ValueOp has
// exactly one executable implementation.
func (v formalApplyTermView) evalValue(value ValueTerm, caller BindingCursor, environment formalApplyFrameEnvironment) (product.Value, bool) {
	if !v.valid(value, 0) {
		return product.Value{}, false
	}
	target, ok := v.targetCursor(caller, environment.caller)
	if !ok {
		return product.Value{}, false
	}
	return v.binding.targetArena.evalValue(value, target, environment.target)
}

func (v formalApplyTermView) evalPath(path PathTerm, caller BindingCursor, environment formalApplyFrameEnvironment) (pathdom.Path, bool) {
	b := v.binding
	if !v.present() || !b.validFor(b.caller, b.target, b.frame) || path == 0 ||
		int(path) >= len(b.targetArena.paths) || !b.targetArena.Sealed() {
		return pathdom.Path{}, false
	}
	target, ok := v.targetCursor(caller, environment.caller)
	if !ok {
		return pathdom.Path{}, false
	}
	return v.binding.targetArena.evalPathWithContext(path, target, environment.target)
}

func (v formalApplyTermView) targetCursor(caller BindingCursor, context SpecializationContext) (BindingCursor, bool) {
	b := v.binding
	if !b.validFor(b.caller, b.target, b.frame) || caller.shape != b.callerShape {
		return BindingCursor{}, false
	}
	frame := b.callerArena.callFrames[b.frame]
	values := make([]product.Value, len(frame.values))
	paths := make([]pathdom.Path, len(frame.paths))
	for index, term := range frame.values {
		value, exact := b.callerArena.evalValue(term, caller, context)
		if !exact {
			return BindingCursor{}, false
		}
		values[index] = value
		if frame.paths[index] == 0 {
			continue
		}
		path, exact := b.callerArena.evalPathWithContext(frame.paths[index], caller, context)
		if !exact {
			return BindingCursor{}, false
		}
		paths[index] = path
	}
	target, err := NewBindingCursor(b.targetShape, values, paths)
	return target, err == nil
}
