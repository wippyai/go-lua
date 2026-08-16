package causal

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (s *arcState) markArc(source, target, decision keyspace.Term, truth bool) (int, bool) {
	return s.claimArc(source, target, decision, truth, arcLocal)
}

func (s *arcState) markLivenessArc(source, target, decision keyspace.Term, truth bool) (int, bool) {
	return s.claimArc(source, target, decision, truth, arcLivenessOnly)
}

// causalTarget converts an authored structural destination to its sealed
// runtime Entry port. Outcome coordinates are already terminal. Body is an
// evaluation leaf/activation anchor and therefore deliberately resolves to
// runtimeentry.Entry(Body); emitBodyEntry owns the separate Body-to-first-root route.
func (s *routeState) causalTarget(raw keyspace.Term) (keyspace.Term, bool) {
	if raw == 0 {
		return 0, false
	}
	candidate := raw
	for {
		if keyspace.TermFamily(candidate) == keyspace.FamilyOutcome {
			if keyspace.TermOrdinal(candidate) == 0 || keyspace.TermOrdinal(candidate) > s.counts[keyspace.FamilyOutcome] {
				return 0, false
			}
			return candidate, true
		}
		if !validPreTerm(candidate, s.counts) {
			return 0, false
		}
		if keyspace.TermFamily(candidate) == keyspace.FamilyLoop {
			_, _, loopKind, _, loopOK := s.flow.Control().Loops().Get(candidate)
			if loopOK && loopKind == kind.LoopRepeat && s.live(candidate) && !s.static(candidate) {
				// Repeat has no pre-body control expression. Its authored Loop
				// anchor owns a separate exact Loop -> Body initial route, so a
				// predecessor must stop at that anchor instead of normalizing
				// through it to the child Body and bypassing the structural Arc.
				return candidate, true
			}
		}
		if !s.live(candidate) {
			// Resume proofs may point at a dead/static direct root. Skip that
			// typed root through its Body forest range, retaining no Source
			// traversal or second structural index.
			owner, ownerOK := s.bodyOf(candidate)
			if keyspace.TermFamily(candidate) == keyspace.FamilyBody {
				owner, ownerOK = s.bodies.Parent(candidate)
			}
			if !ownerOK {
				return 0, false
			}
			cursor, cursorOK := s.rootCursor(owner, candidate)
			if !cursorOK {
				return 0, false
			}
			next, nextOK := s.nextLiveRoot(owner, cursor)
			if !nextOK {
				return 0, false
			}
			if next == owner {
				normal, normalOK := s.bodyNormal(owner)
				if !normalOK {
					return 0, false
				}
				candidate = normal
				continue
			}
			candidate = next
			continue
		}
		// runtimeentry owns the exact Ports/Executable normalization and follows
		// typed children past static/dead authored operands. Causal consumes only
		// the sealed O(1) projection here.
		target, ok := s.entries.Entry(candidate)
		if !ok || target == 0 {
			return 0, false
		}
		if isOutcome(target) || s.live(target) {
			return target, true
		}
		return 0, false
	}
}

// resumeTarget interprets the typed sourcecontrol Resume projection. A Body
// result is a terminal activation tail, not an evaluation Entry anchor; all
// other resume coordinates retain the ordinary Entry conversion.
func (s *routeState) resumeTarget(raw keyspace.Term) (keyspace.Term, bool) {
	if keyspace.TermFamily(raw) == keyspace.FamilyBody {
		return s.bodyNormal(raw)
	}
	return s.causalTarget(raw)
}

// loopIterationTarget is the explicit continuation anchor for numeric and
// generic-for headers.  Their initial Body entry is Ports.Entry(loop), which
// evaluates the header once; a subsequent header/control completion and a
// Body Normal outcome resume at the Loop term itself.  Passing that anchor
// through causalTarget would expand it back to the first header operand and
// evaluate a one-shot header on every iteration.
func (s *routeState) loopIterationTarget(loop keyspace.Term) (keyspace.Term, bool) {
	if keyspace.TermFamily(loop) != keyspace.FamilyLoop ||
		!validPreTerm(loop, s.counts) || !s.live(loop) || s.static(loop) {
		return 0, false
	}
	return loop, true
}

// structuralRoute resolves the next authored structural coordinate without
// consuming its sourcecontrol witness.  The caller chooses the exact witness
// source (the current root, a nested Body tail, or a control term) and then
// marks that one Arc exactly once.  A static/dead authored witness root is still returned
// as arcTarget so its live predecessor can discharge the witness while the
// runtime route skips it.
func (s *structureState) structuralRoute(owner keyspace.Term, cursor int) (target, raw keyspace.Term, ok bool) {
	raw, rawOK := s.nextRoot(owner, cursor)
	if !rawOK {
		return 0, 0, false
	}
	if raw == owner {
		target, targetOK := s.bodyNormal(owner)
		return target, raw, targetOK
	}
	if rootKind(keyspace.TermFamily(raw)) && !s.static(raw) && s.live(raw) {
		return raw, raw, true
	}
	target, targetOK := s.nextLiveRoot(owner, cursor)
	if !targetOK {
		return 0, raw, false
	}
	if target == owner {
		target, targetOK = s.bodyNormal(owner)
	}
	return target, raw, targetOK
}

func (s *structureState) routeTargetFrom(source, owner keyspace.Term, cursor int) (target keyspace.Term, arcTarget keyspace.Term, arcIndex int, ok bool) {
	target, raw, routeOK := s.structuralRoute(owner, cursor)
	if !routeOK {
		return 0, raw, -1, false
	}
	rawLive := raw == owner || (rootKind(keyspace.TermFamily(raw)) && !s.static(raw) && s.live(raw))
	if !rawLive {
		if _, livenessOK := s.markLivenessArc(source, raw, 0, false); !livenessOK {
			return 0, raw, -1, false
		}
		return target, raw, -1, true
	}
	arcIndex, arcOK := s.markArc(source, raw, 0, false)
	if !arcOK {
		return 0, raw, -1, false
	}
	// A route that skips a static/dead authored witness root is liveness-only: the emitted
	// Edge points at the next executable endpoint, not at the Arc's authored
	// witness target,
	// so it must not retain that Arc annotation. The witness remains consumed.
	return target, raw, arcIndex, true
}

func (s *structureState) emitBodyEntry(bodyTerm keyspace.Term) error {
	count, ok := s.bodies.RootCount(bodyTerm)
	if !ok || count < 0 {
		return errors.New("program/flow/causal: Body root range is unavailable")
	}
	entryArc := -1
	ordinal := keyspace.TermOrdinal(bodyTerm)
	if ordinal != 0 && uint64(ordinal) < uint64(len(s.bodyParentRoot)) && s.bodyParentRoot[ordinal] != 0 {
		// A direct nested Body root has the self-witness Body -> Body emitted
		// by sourcecontrol. Branch and Loop children instead use the parent's
		// guarded Arc (Branch/Loop -> Body), which is claimed by emitBranch or
		// emitLoop before this Body-entry pass. The Body anchor's separate
		// Body -> first-root route therefore carries no second Arc annotation.
		parentRoot := s.bodyParentRoot[ordinal]
		if keyspace.TermFamily(parentRoot) == keyspace.FamilyBody && !s.static(bodyTerm) {
			var arcOK bool
			if s.live(bodyTerm) {
				entryArc, arcOK = s.markArc(bodyTerm, bodyTerm, 0, false)
			} else {
				_, arcOK = s.markLivenessArc(bodyTerm, bodyTerm, 0, false)
			}
			if !arcOK {
				return errors.New("program/flow/causal: nested Body self Arc is unavailable")
			}
		}
	}
	for cursor := 0; cursor < count; cursor++ {
		root, rootOK := s.bodies.RootAt(bodyTerm, cursor)
		if !rootOK {
			return errors.New("program/flow/causal: Body root is unavailable")
		}
		if !rootKind(keyspace.TermFamily(root)) || s.static(root) || !s.live(root) {
			continue
		}
		to, toOK := s.causalTarget(root)
		if !toOK {
			return errors.New("program/flow/causal: Body entry root has no causal Entry")
		}
		// The parent structural Body -> Body Arc is a separate final route;
		// it cannot be the child's synthetic BodyEntry route origin.
		return s.appendBodyEntryEdge(bodyTerm, to, entryArc)
	}
	normal, normalOK := s.bodyNormal(bodyTerm)
	if !normalOK {
		return errors.New("program/flow/causal: empty Body has no Normal Outcome")
	}
	return s.appendBodyEntryEdge(bodyTerm, normal, entryArc)
}

func (s *structureState) breakTarget(term keyspace.Term) keyspace.Term {
	exit, ok := s.outs.BreakExit(term)
	if !ok {
		return 0
	}
	_, _, target, ok := s.outs.Get(exit)
	if !ok || keyspace.TermFamily(target) != keyspace.FamilyLoop {
		return 0
	}
	return target
}

func (s *structureState) emitGotoRoot(owner, root keyspace.Term) error {
	_, label, ok := s.flow.Control().Gotos().Get(root)
	if !ok || keyspace.TermFamily(label) != keyspace.FamilyLabel {
		return errors.New("program/flow/causal: Goto Label target is unavailable")
	}
	exit, ok := s.outs.GotoExit(root)
	if !ok {
		return errors.New("program/flow/causal: Goto Outcome is unavailable")
	}
	arcIndex, arcOK := s.markArc(root, label, 0, false)
	if !arcOK {
		return fmt.Errorf("program/flow/causal: Goto %v has no structural Arc", root)
	}
	if keyspace.TermFamily(exit) == keyspace.FamilyLabel {
		resume, resumeOK := s.graph.Resume(label)
		if !resumeOK {
			return errors.New("program/flow/causal: same-Body Goto Label resume is unavailable")
		}
		to, toOK := s.resumeTarget(resume)
		if !toOK {
			return errors.New("program/flow/causal: same-Body Goto resume is not executable")
		}
		return s.appendEdge(root, to, owner, 0, false, arcIndex)
	}
	return s.appendEdge(root, exit, owner, 0, false, arcIndex)
}

func (s *structureState) emitBodyNormalRoute(bodyTerm keyspace.Term) error {
	normal, normalOK := s.bodyNormal(bodyTerm)
	if !normalOK {
		return errors.New("program/flow/causal: Body Normal Outcome is unavailable")
	}
	tail, tailOK := s.graph.Tail(bodyTerm)
	if !tailOK {
		return errors.New("program/flow/causal: Body tail phase is unavailable")
	}
	ordinal := keyspace.TermOrdinal(bodyTerm)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(s.bodyParentRoot)) {
		return nil
	}
	parentRoot := s.bodyParentRoot[ordinal]
	if parentRoot == 0 || keyspace.TermFamily(parentRoot) == keyspace.FamilyFunction {
		// Function Bodies stop at their activation boundary. A direct Body with
		// no structural parent is likewise a complete activation root.
		return nil
	}
	parent, parentOK := s.bodies.Parent(bodyTerm)
	if !parentOK {
		return nil
	}
	parentCursor := s.bodyParentCursor[ordinal]
	if parentCursor < 0 {
		return nil
	}
	if !s.graph.Reachable(tail) {
		// A Return-terminated Body has no structural fallthrough. Consume its
		// exact tail witness as liveness-only so arc coverage stays complete,
		// but do not fabricate an OutcomeNormal -> continuation route or a
		// BodyTail WTO endpoint.
		switch keyspace.TermFamily(parentRoot) {
		case keyspace.FamilyBranch, keyspace.FamilyBody:
			raw, rawOK := s.nextRoot(parent, parentCursor)
			if !rawOK {
				return errors.New("program/flow/causal: unreachable Body tail target is unavailable")
			}
			if _, livenessOK := s.markLivenessArc(bodyTerm, raw, 0, false); !livenessOK {
				return errors.New("program/flow/causal: unreachable Body tail Arc is unavailable")
			}
		case keyspace.FamilyLoop:
			if _, livenessOK := s.markLivenessArc(bodyTerm, parentRoot, 0, false); !livenessOK {
				return errors.New("program/flow/causal: unreachable Loop Body tail Arc is unavailable")
			}
		}
		return nil
	}
	switch keyspace.TermFamily(parentRoot) {
	case keyspace.FamilyBranch, keyspace.FamilyBody:
		target, raw, arcIndex, ok := s.routeTargetFrom(bodyTerm, parent, parentCursor)
		if !ok {
			return errors.New("program/flow/causal: Body normal route target is unavailable")
		}
		to, toOK := s.causalTarget(target)
		if !toOK {
			if s.live(target) && !s.static(target) {
				return errors.New("program/flow/causal: live Body normal target has no Entry port")
			}
			return nil
		}
		if arcIndex < 0 && raw != parent {
			return s.appendEdge(normal, to, bodyTerm, 0, false, -1)
		}
		return s.appendEdge(normal, to, bodyTerm, 0, false, arcIndex)
	case keyspace.FamilyLoop:
		_, loopBody, loopKind, control, ok := s.flow.Control().Loops().Get(parentRoot)
		if !ok || loopBody != bodyTerm {
			return errors.New("program/flow/causal: Loop Body normal route is malformed")
		}
		var target keyspace.Term
		switch loopKind {
		case kind.LoopWhile, kind.LoopRepeat:
			if !s.live(control) || s.static(control) {
				if _, livenessOK := s.markLivenessArc(bodyTerm, parentRoot, 0, false); !livenessOK {
					return errors.New("program/flow/causal: static Loop control Body Arc is unavailable")
				}
				return nil
			}
			target, ok = s.causalTarget(control)
		default:
			if !s.live(control) || s.static(control) {
				if _, livenessOK := s.markLivenessArc(bodyTerm, parentRoot, 0, false); !livenessOK {
					return errors.New("program/flow/causal: static Loop control Body Arc is unavailable")
				}
				return nil
			}
			target, ok = s.loopIterationTarget(parentRoot)
		}
		if !ok {
			return errors.New("program/flow/causal: Loop continuation endpoint is unavailable")
		}
		arcIndex, arcOK := s.markArc(bodyTerm, parentRoot, 0, false)
		if !arcOK {
			return errors.New("program/flow/causal: Loop Body normal Arc is unavailable")
		}
		return s.appendEdge(normal, target, bodyTerm, 0, false, arcIndex)
	default:
		return nil
	}
}

func (s *structureState) emitBranch(owner keyspace.Term, cursor int, branch, _ keyspace.Term) error {
	branchOwner, condition, whenTrue, whenFalse, ok := s.flow.Control().Branches().Get(branch)
	if !ok || branchOwner != owner {
		return errors.New("program/flow/causal: Branch owner is unavailable")
	}
	left, leftOK := s.finishEndpoint(condition)
	if !leftOK {
		if s.live(condition) && !s.static(condition) {
			return errors.New("program/flow/causal: live Branch condition has no Finish port")
		}
	}
	if leftOK && keyspace.TermFamily(left) == keyspace.FamilyCall {
		// A condition Call resumes at the owning Branch control anchor. The
		// Branch's guarded alternatives remain public local rows; using its
		// Entry here would re-enter the condition operand before the boundary
		// had completed.
		if err := s.planCallFinish(left, branch); err != nil {
			return err
		}
	}
	for _, arm := range []struct {
		body  keyspace.Term
		truth bool
	}{
		{whenTrue, true},
		{whenFalse, false},
	} {
		if !leftOK {
			if _, livenessOK := s.markLivenessArc(branch, arm.body, branch, arm.truth); !livenessOK {
				return fmt.Errorf("program/flow/causal: Branch %v static condition Arc is unavailable", branch)
			}
			continue
		}
		to, toOK := s.causalTarget(arm.body)
		if !toOK {
			targetLive := rootKind(keyspace.TermFamily(arm.body)) && !s.static(arm.body) && s.live(arm.body)
			if targetLive {
				return fmt.Errorf("program/flow/causal: Branch %v live arm has no Entry port", branch)
			}
			if _, livenessOK := s.markLivenessArc(branch, arm.body, branch, arm.truth); !livenessOK {
				return fmt.Errorf("program/flow/causal: Branch %v arm Arc is unavailable", branch)
			}
			continue
		}
		arcIndex, arcOK := s.markArc(branch, arm.body, branch, arm.truth)
		if !arcOK {
			return fmt.Errorf("program/flow/causal: Branch %v arm Arc is unavailable", branch)
		}
		from := left
		if keyspace.TermFamily(left) == keyspace.FamilyCall {
			from = branch
		}
		if err := s.appendEdge(from, to, owner, branch, arm.truth, arcIndex); err != nil {
			return err
		}
	}
	return nil
}

func (s *structureState) loopExit(owner keyspace.Term, cursor int, loop keyspace.Term, loopKind kind.LoopKind) (keyspace.Term, keyspace.Term, int, bool) {
	target, raw, ok := s.structuralRoute(owner, cursor)
	if !ok {
		return 0, raw, -1, false
	}
	rawLive := raw == owner || (rootKind(keyspace.TermFamily(raw)) && !s.static(raw) && s.live(raw))
	if !rawLive {
		// -2 records that the authored exit witness is not a live runtime
		// endpoint. The caller claims it liveness-only only after it knows
		// whether the loop control can materialize a causal route.
		return target, raw, -2, true
	}
	// A live exit Arc must remain unclaimed until the control endpoint has
	// been validated and the corresponding local route is actually emitted.
	return target, raw, -1, true
}

func (s *structureState) emitLoop(owner keyspace.Term, cursor int, loop, _ keyspace.Term) error {
	loopOwner, body, loopKind, control, ok := s.flow.Control().Loops().Get(loop)
	if !ok || loopOwner != owner {
		return errors.New("program/flow/causal: Loop owner is unavailable")
	}
	exit, exitRaw, exitArc, exitOK := s.loopExit(owner, cursor, loop, loopKind)
	if !exitOK {
		return errors.New("program/flow/causal: Loop exit is unavailable")
	}
	appendArm := func(from, to keyspace.Term, truth bool, target keyspace.Term, preclaimed int) error {
		toEndpoint, toOK := s.causalTarget(to)
		if !toOK {
			targetLive := target == owner || (rootKind(keyspace.TermFamily(target)) && !s.static(target) && s.live(target))
			if targetLive {
				return fmt.Errorf("program/flow/causal: Loop %v live arm has no Entry port", loop)
			}
			if preclaimed == -2 {
				return nil
			}
			if _, livenessOK := s.markLivenessArc(loop, target, loop, truth); !livenessOK {
				return fmt.Errorf("program/flow/causal: Loop %v arm Arc is unavailable", loop)
			}
			return nil
		}
		targetLive := rootKind(keyspace.TermFamily(target)) && !s.static(target) && s.live(target)
		if target == owner {
			targetLive = true
		}
		var arcIndex int
		var arcOK bool
		if preclaimed == -2 {
			arcIndex, arcOK = -1, true
		} else if preclaimed >= 0 {
			arcIndex, arcOK = preclaimed, true
		} else if targetLive {
			arcIndex, arcOK = s.markArc(loop, target, loop, truth)
		} else {
			_, arcOK = s.markLivenessArc(loop, target, loop, truth)
			arcIndex = -1
		}
		if !arcOK {
			return fmt.Errorf("program/flow/causal: Loop %v arm Arc is unavailable", loop)
		}
		return s.appendEdge(from, toEndpoint, owner, loop, truth, arcIndex)
	}
	controlEndpoint, controlOK := s.finishEndpoint(control)
	if !controlOK {
		if s.live(control) && !s.static(control) {
			return errors.New("program/flow/causal: live Loop control has no Finish port")
		}
		if _, livenessOK := s.markLivenessArc(loop, exitRaw, loop, loopKind == kind.LoopRepeat); !livenessOK {
			return errors.New("program/flow/causal: static Loop exit Arc is unavailable")
		}
		if err := s.dischargeLoopControlArcs(loop, body, loopKind); err != nil {
			return err
		}
		return nil
	}
	if exitArc == -2 {
		if _, livenessOK := s.markLivenessArc(loop, exitRaw, loop, loopKind == kind.LoopRepeat); !livenessOK {
			return errors.New("program/flow/causal: Loop static exit Arc is unavailable")
		}
	}
	if keyspace.TermFamily(controlEndpoint) == keyspace.FamilyCall {
		// As with a Branch condition, a direct control Call resumes at the
		// Loop anchor before the public guarded Loop arms.
		if err := s.planCallFinish(controlEndpoint, loop); err != nil {
			return err
		}
	}
	if loopKind == kind.LoopNumericFor || loopKind == kind.LoopGenericFor {
		// Header Values are evaluated once, then the existing Loop term owns
		// iterator/range alternatives. A direct Call header uses its boundary.
		if keyspace.TermFamily(controlEndpoint) == keyspace.FamilyCall {
			// Boundary emission supplies Call -> Loop. The source-control
			// header witness is owned by Loop (not by the nested Call), so
			// claim that exact Arc for the boundary normal disposition here.
			arcIndex, arcOK := s.claimArc(loop, loop, 0, false, arcBoundaryNormal)
			if !arcOK {
				return errors.New("program/flow/causal: Loop Call header Arc is unavailable")
			}
			callOrdinal := keyspace.TermOrdinal(controlEndpoint)
			if callOrdinal == 0 || uint64(callOrdinal) >= uint64(len(s.normalArc)) || s.normalArc[callOrdinal] >= 0 {
				return errors.New("program/flow/causal: Loop Call header normal Arc is malformed")
			}
			s.normalArc[callOrdinal] = arcIndex
		} else {
			seedArc, seedOK := s.markArc(loop, loop, 0, false)
			if !seedOK {
				return errors.New("program/flow/causal: Loop header seed Arc is unavailable")
			}
			seedTarget, targetOK := s.loopIterationTarget(loop)
			if !targetOK {
				return errors.New("program/flow/causal: Loop iteration anchor is unavailable")
			}
			if err := s.appendEdge(controlEndpoint, seedTarget, owner, 0, false, seedArc); err != nil {
				return err
			}
		}
		if err := appendArm(loop, body, true, body, -1); err != nil {
			return err
		}
		if err := appendArm(loop, exit, false, exitRaw, exitArc); err != nil {
			return err
		}
	} else if loopKind == kind.LoopRepeat {
		initialArc, initialOK := s.markArc(loop, body, 0, false)
		if !initialOK {
			return errors.New("program/flow/causal: Repeat initial Arc is unavailable")
		}
		if err := s.appendEdge(loop, body, owner, 0, false, initialArc); err != nil {
			return err
		}
		if keyspace.TermFamily(controlEndpoint) == keyspace.FamilyCall {
			// Boundary emission supplies Call -> Loop after the Body normal Arc.
			controlEndpoint = loop
		}
		if err := appendArm(controlEndpoint, body, false, body, -1); err != nil {
			return err
		}
		if err := appendArm(controlEndpoint, exit, true, exitRaw, exitArc); err != nil {
			return err
		}
	} else {
		if keyspace.TermFamily(controlEndpoint) == keyspace.FamilyCall {
			controlEndpoint = loop
		}
		if err := appendArm(controlEndpoint, body, true, body, -1); err != nil {
			return err
		}
		if err := appendArm(controlEndpoint, exit, false, exitRaw, exitArc); err != nil {
			return err
		}
	}
	if (loopKind == kind.LoopNumericFor || loopKind == kind.LoopGenericFor) && s.live(loop) {
		throwExit, throwOK := s.outs.BodyExit(owner, kind.OutcomeThrow)
		if !throwOK {
			return errors.New("program/flow/causal: Loop Throw Outcome is unavailable")
		}
		if err := s.appendEdge(loop, throwExit, owner, 0, false, -1); err != nil {
			return err
		}
		if loopKind == kind.LoopGenericFor {
			for _, outcomeKind := range [...]kind.OutcomeKind{kind.OutcomeYield, kind.OutcomeCancel} {
				exitOutcome, outcomeOK := s.outs.BodyExit(owner, outcomeKind)
				if !outcomeOK {
					return errors.New("program/flow/causal: generic Loop Outcome is unavailable")
				}
				if err := s.appendEdge(loop, exitOutcome, owner, 0, false, -1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *structureState) dischargeLoopControlArcs(loop, body keyspace.Term, loopKind kind.LoopKind) error {
	if loopKind == kind.LoopNumericFor || loopKind == kind.LoopGenericFor {
		if _, ok := s.markLivenessArc(loop, loop, 0, false); !ok {
			return errors.New("program/flow/causal: static Loop header Arc is unavailable")
		}
	}
	if loopKind == kind.LoopRepeat {
		if _, ok := s.markLivenessArc(loop, body, 0, false); !ok {
			return errors.New("program/flow/causal: static Repeat initial Arc is unavailable")
		}
	}
	truth := loopKind != kind.LoopRepeat
	if _, ok := s.markLivenessArc(loop, body, loop, truth); !ok {
		return errors.New("program/flow/causal: static Loop decision Arc is unavailable")
	}
	return nil
}

func (s *structureState) emitStructure() error {
	for ordinal := uint32(1); ordinal <= s.counts[keyspace.FamilyBody]; ordinal++ {
		bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		if s.live(bodyTerm) {
			if err := s.emitBodyEntry(bodyTerm); err != nil {
				return err
			}
		}
		count, ok := s.bodies.RootCount(bodyTerm)
		if !ok || count < 0 {
			return errors.New("program/flow/causal: Body root range is unavailable")
		}
		for cursor := 0; cursor < count; cursor++ {
			root, rootOK := s.bodies.RootAt(bodyTerm, cursor)
			if !rootOK {
				return errors.New("program/flow/causal: Body root is unavailable")
			}
			if s.static(root) || !s.live(root) || !rootKind(keyspace.TermFamily(root)) {
				continue
			}
			if err := s.emitRoot(bodyTerm, cursor, root); err != nil {
				return err
			}
		}
		if s.live(bodyTerm) && !s.static(bodyTerm) && ordinal != 0 &&
			uint64(ordinal) < uint64(len(s.bodyParentRoot)) && s.bodyParentRoot[ordinal] != 0 {
			if err := s.emitBodyNormalRoute(bodyTerm); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *structureState) emitRoot(owner keyspace.Term, cursor int, root keyspace.Term) error {
	nextRaw, nextOK := s.nextRoot(owner, cursor)
	if !nextOK {
		return errors.New("program/flow/causal: Body successor root is unavailable")
	}
	switch keyspace.TermFamily(root) {
	case keyspace.FamilyBind, keyspace.FamilyAssign:
		return s.emitRootNormal(owner, root, nextRaw)
	case keyspace.FamilyCall:
		// The structural root pass is the sole direct-Call discovery pass. It
		// records the exact normal Entry and Arc witness in dense seal scratch;
		// emitBoundaries later publishes the typed boundary row without a
		// per-Call root scan.
		return s.prepareDirectCall(owner, root, cursor)
	case keyspace.FamilyReturn:
		exit, ok := s.outs.ReturnExit(root)
		if !ok {
			return errors.New("program/flow/causal: Return has no Return Outcome")
		}
		return s.appendEdge(root, exit, owner, 0, false, -1)
	case keyspace.FamilyBreak:
		exit, ok := s.outs.BreakExit(root)
		if !ok {
			return errors.New("program/flow/causal: Break has no Break Outcome")
		}
		arcIndex, arcOK := s.markArc(root, s.breakTarget(root), 0, false)
		if !arcOK {
			if s.static(root) {
				return nil
			}
			return fmt.Errorf("program/flow/causal: Break %v has no structural Arc", root)
		}
		return s.appendEdge(root, exit, owner, 0, false, arcIndex)
	case keyspace.FamilyGoto:
		return s.emitGotoRoot(owner, root)
	case keyspace.FamilyBody:
		// A direct nested Body's self Arc is claimed by emitBodyEntry alongside
		// its Body -> first-root/Normal route. Branch/Loop child Arcs are owned
		// by their guarded parent route; no self Edge is emitted here.
		return nil
	case keyspace.FamilyBranch:
		return s.emitBranch(owner, cursor, root, nextRaw)
	case keyspace.FamilyLoop:
		return s.emitLoop(owner, cursor, root, nextRaw)
	default:
		return errors.New("program/flow/causal: unsupported live Body root")
	}
}

func (s *structureState) emitRootNormal(owner, root, nextRaw keyspace.Term) error {
	cursor, cursorOK := s.rootCursor(owner, root)
	if !cursorOK {
		return errors.New("program/flow/causal: root cursor is unavailable")
	}
	to, _, arcIndex, routeOK := s.routeTargetFrom(root, owner, cursor)
	if !routeOK {
		return fmt.Errorf("program/flow/causal: root %v has no structural route", root)
	}
	_ = nextRaw
	from, fromOK := s.finishEndpoint(root)
	if !fromOK {
		if s.live(root) && !s.static(root) {
			return fmt.Errorf("program/flow/causal: live root %v has no Finish port", root)
		}
		return nil
	}
	toEndpoint, toOK := s.causalTarget(to)
	if !toOK {
		if !s.live(to) || s.static(to) {
			return nil
		}
		return fmt.Errorf("program/flow/causal: root %v structural target is not executable", root)
	}
	return s.appendEdge(from, toEndpoint, owner, 0, false, arcIndex)
}
