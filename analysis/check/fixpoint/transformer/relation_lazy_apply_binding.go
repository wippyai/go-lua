package transformer

import "fmt"

// relationArenaValueRef is an existing value term together with its lexical
// arena owner.  It is an evaluation reference, not stored term syntax: Apply
// follows the caller's parent binding when owner itself is being specialized.
type relationArenaValueRef struct {
	owner relationVar
	arena *Arena
	term  ValueTerm
}

func (r relationArenaValueRef) valid() bool {
	return r.owner != 0 && r.arena != nil && r.term != 0 && int(r.term) < len(r.arena.values)
}

// relationArenaPathRef is the path counterpart of relationArenaValueRef.
// Call-frame paths are optional; absence is reported separately rather than
// represented by a synthetic target-owned path.
type relationArenaPathRef struct {
	owner relationVar
	arena *Arena
	term  PathTerm
}

func (r relationArenaPathRef) valid() bool {
	return r.owner != 0 && r.arena != nil && r.term != 0 && int(r.term) < len(r.arena.paths)
}

// relationArenaGuardRef keeps Boolean syntax in its sealed target arena.  The
// binding says how that target's IN roots are read; MID roots remain target
// registers and are interpreted by the predecessor formal-region cell.
type relationArenaGuardRef struct {
	owner   relationVar
	arena   *Arena
	guard   Guard
	binding relationLazyApplyBinding
}

func (r relationArenaGuardRef) valid() bool {
	return r.owner != 0 && r.arena != nil && r.guard != 0 && int(r.guard) < len(r.arena.guards) &&
		r.binding.validFor(r.binding.caller, r.owner, r.binding.frame) && r.arena.validGuard(r.guard, r.binding.targetShape)
}

// relationLazyApplyBinding is the borrowed, arena-qualified binding seam for
// one lexical Apply.  It owns no copied ValueTerm, PathTerm, Guard, State, or
// solver cell.  Nested Apply composes these immutable frame views as a runtime
// stack; it never imports a target DAG into its caller.
type relationLazyApplyBinding struct {
	caller      relationVar
	target      relationVar
	frame       callFrameTerm
	callerArena *Arena
	targetArena *Arena
	callerCode  *relationCode
	targetCode  *relationCode
	callerShape Shape
	targetShape Shape
}

func freezeRelationLazyApplyBinding(callerVar relationVar, caller, target *relationCode, apply relationApplyRef) (relationLazyApplyBinding, error) {
	if callerVar == 0 || caller == nil || target == nil || caller.terms == nil || target.terms == nil ||
		apply.variable == 0 || apply.frame == 0 || int(apply.frame) >= len(caller.terms.callFrames) {
		return relationLazyApplyBinding{}, fmt.Errorf("transformer: lazy Apply binding is unowned")
	}
	frame := caller.terms.callFrames[apply.frame]
	if frame.variable != apply.variable || frame.shape != target.shape ||
		len(frame.values) != target.shape.InputCount() || len(frame.paths) != len(frame.values) {
		return relationLazyApplyBinding{}, fmt.Errorf("transformer: lazy Apply frame differs from its sealed target shape")
	}
	for index, value := range frame.values {
		if value == 0 || int(value) >= len(caller.terms.values) ||
			!caller.terms.validValue(value, caller.shape, make(map[ValueTerm]bool)) ||
			frame.paths[index] != 0 && !caller.terms.validPath(frame.paths[index], caller.shape) {
			return relationLazyApplyBinding{}, fmt.Errorf("transformer: lazy Apply frame contains a foreign caller term")
		}
	}
	return relationLazyApplyBinding{
		caller: callerVar, target: apply.variable, frame: apply.frame,
		callerArena: caller.terms, targetArena: target.terms, callerCode: caller, targetCode: target,
		callerShape: caller.shape, targetShape: target.shape,
	}, nil
}

func (b relationLazyApplyBinding) validFor(caller, target relationVar, frame callFrameTerm) bool {
	if b.caller != caller || b.target != target || b.frame != frame || b.callerArena == nil || b.targetArena == nil ||
		b.callerCode == nil || b.targetCode == nil || b.callerCode.terms != b.callerArena || b.targetCode.terms != b.targetArena ||
		frame == 0 || int(frame) >= len(b.callerArena.callFrames) {
		return false
	}
	node := b.callerArena.callFrames[frame]
	return node.variable == target && node.shape == b.targetShape && len(node.values) == b.targetShape.InputCount() && len(node.paths) == len(node.values)
}

// inputValue resolves only one target IN root to its existing caller term.
// In particular, a caller-local MID remains an arena-qualified caller MID;
// this method never asks RebaseTermDAGs to export it.
func (b relationLazyApplyBinding) inputValue(root Root) (relationArenaValueRef, bool) {
	if !b.validFor(b.caller, b.target, b.frame) || !b.targetShape.validateInput(root) {
		return relationArenaValueRef{}, false
	}
	term := b.callerArena.callFrames[b.frame].values[b.targetShape.offset(root.Kind)+int(root.Index)]
	ref := relationArenaValueRef{owner: b.caller, arena: b.callerArena, term: term}
	return ref, ref.valid()
}

// inputPath resolves an optional target IN path without constructing target
// syntax. present=false is the exact pathless binding, not a failure.
func (b relationLazyApplyBinding) inputPath(root Root) (ref relationArenaPathRef, present, ok bool) {
	if !b.validFor(b.caller, b.target, b.frame) || !b.targetShape.validateInput(root) {
		return relationArenaPathRef{}, false, false
	}
	term := b.callerArena.callFrames[b.frame].paths[b.targetShape.offset(root.Kind)+int(root.Index)]
	if term == 0 {
		return relationArenaPathRef{}, false, true
	}
	ref = relationArenaPathRef{owner: b.caller, arena: b.callerArena, term: term}
	return ref, true, ref.valid()
}

// targetGuard borrows one existing target-owned Guard DAG under this frame.
// It performs no atom substitution and grows neither arena.
func (b relationLazyApplyBinding) targetGuard(guard Guard) (relationArenaGuardRef, bool) {
	if !b.validFor(b.caller, b.target, b.frame) || guard == 0 || int(guard) >= len(b.targetArena.guards) {
		return relationArenaGuardRef{}, false
	}
	ref := relationArenaGuardRef{owner: b.target, arena: b.targetArena, guard: guard, binding: b}
	return ref, ref.valid()
}
