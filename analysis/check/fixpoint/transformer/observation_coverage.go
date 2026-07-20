package transformer

import (
	"context"
	"errors"
	"sync"

	engineobservation "github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

var errObservationCoverageCanceled = errors.New("observation coverage canceled")
var errObservationCoverageMalformed = errors.New("observation coverage malformed")

var observationCoverageScratchPool = sync.Pool{New: func() any { return newObservationCoverageScratch() }}

type observationCoverageKey struct {
	owner  lexicalidentity.StableLexicalBodyID
	route  engineobservation.InvocationID
	anchor engineobservation.Occurrence
}

type observationCoverageWorlds struct {
	owed          []observationCoverageGuardWorld
	evidence      []observationCoverageGuardWorld
	requiredGuard Guard
	required      bool
	exactRequired bool
	direct        bool
}

type observationCoverageGuardWorld struct{ row, local Guard }

type observationCoverageRequirement struct {
	key   observationCoverageKey
	guard Guard
	exact bool
}

type observationCoverageScratch struct {
	decisionKernel
	arena *Arena

	worlds          map[observationCoverageKey]int
	groups          []observationCoverageWorlds
	atoms           []ValueTerm
	seen            map[ValueTerm]struct{}
	ranks           map[ValueTerm]uint32
	names           map[ValueTerm]string
	collectSeen     map[Guard]struct{}
	collectOps      uint64
	directSet       map[observationCoverageGuardWorld]struct{}
	bddWork         []decisionRef
	lastUnionInputs []decisionRef
	lastUnionResult decisionRef
	lastUnionValid  bool

	guardMemo map[Guard]decisionRef
}

func newObservationCoverageScratch() *observationCoverageScratch {
	return &observationCoverageScratch{
		worlds: make(map[observationCoverageKey]int),
		seen:   make(map[ValueTerm]struct{}), ranks: make(map[ValueTerm]uint32), names: make(map[ValueTerm]string), collectSeen: make(map[Guard]struct{}), directSet: make(map[observationCoverageGuardWorld]struct{}),
		decisionKernel: newDecisionKernel(), guardMemo: make(map[Guard]decisionRef),
	}
}

func (s *observationCoverageScratch) reset(arena *Arena) {
	for index := range s.groups {
		s.groups[index].owed = s.groups[index].owed[:0]
		s.groups[index].evidence = s.groups[index].evidence[:0]
		s.groups[index].requiredGuard = 0
		s.groups[index].required = false
		s.groups[index].exactRequired = false
		s.groups[index].direct = false
	}
	s.groups = s.groups[:0]
	for key := range s.worlds {
		delete(s.worlds, key)
	}
	s.atoms = s.atoms[:0]
	for key := range s.seen {
		delete(s.seen, key)
	}
	for key := range s.ranks {
		delete(s.ranks, key)
	}
	for key := range s.collectSeen {
		delete(s.collectSeen, key)
	}
	for key := range s.directSet {
		delete(s.directSet, key)
	}
	s.collectOps = 0
	if s.arena != arena {
		for key := range s.names {
			delete(s.names, key)
		}
	}
	s.arena = arena
	s.decisionKernel.resetBoolean()
	for key := range s.guardMemo {
		delete(s.guardMemo, key)
	}
	s.bddWork = s.bddWork[:0]
	s.lastUnionInputs = s.lastUnionInputs[:0]
	s.lastUnionValid = false
}

func relationRowsCoverObservations(ctx context.Context, arena *Arena, plan *operationplan.Plan, rows []SymbolicCFGRow, scratch *observationCoverageScratch) (bool, error) {
	if arena == nil || plan == nil || scratch == nil {
		return false, nil
	}
	if ctx.Err() != nil {
		return false, errObservationCoverageCanceled
	}
	requirements, sealed := plan.ObservationRequirements()
	if !sealed {
		return false, nil
	}
	owner := plan.ObservationBody()
	cursor := requirements.Cursor(false)
	observationRequirements := make([]observationCoverageRequirement, 0)
	requirementCount := 0
	for requirement, ok := cursor.Next(); ok; requirement, ok = cursor.Next() {
		requirementCount++
		if requirementCount&63 == 0 && ctx.Err() != nil {
			return false, errObservationCoverageCanceled
		}
		if requirement.Stage() != operationplan.RequirementObservation {
			continue
		}
		anchor, ok := requirement.Anchor()
		if !ok {
			return false, nil
		}
		observationRequirements = append(observationRequirements, observationCoverageRequirement{key: observationCoverageKey{owner: owner, anchor: anchor}})
	}
	return observationRequirementsCover(ctx, arena, observationRequirements, rows, scratch)
}

func sparseProjectionTraceCoversObservations(ctx context.Context, arena *Arena, trace *sparseProjectionTrace, annotations relationAnnotations) (bool, error) {
	if trace == nil || trace.owner == (lexicalidentity.StableLexicalBodyID{}) {
		return false, nil
	}
	requirements := make([]observationCoverageRequirement, 0)
	row := SymbolicCFGRow{Guard: arena.True()}
	for _, slot := range trace.slots {
		if slot.requirement.Stage() != operationplan.RequirementObservation {
			continue
		}
		anchor, ok := slot.requirement.Anchor()
		if !ok {
			return false, nil
		}
		requirements = append(requirements, observationCoverageRequirement{
			key: observationCoverageKey{owner: trace.owner, anchor: anchor}, guard: slot.guard, exact: true,
		})
		row.Observations = append(row.Observations, slot.observed...)
		row.observationObligations = append(row.observationObligations, slot.owed...)
	}
	return observationRequirementsCover(ctx, arena, requirements, []SymbolicCFGRow{row}, newObservationCoverageScratch())
}

func observationRequirementsCover(ctx context.Context, arena *Arena, requirements []observationCoverageRequirement, rows []SymbolicCFGRow, scratch *observationCoverageScratch) (bool, error) {
	if arena == nil || scratch == nil || ctx == nil {
		return false, nil
	}
	if ctx.Err() != nil {
		return false, errObservationCoverageCanceled
	}
	scratch.reset(arena)
	for _, requirement := range requirements {
		world := scratch.world(requirement.key)
		if (world.required && world.exactRequired != requirement.exact) ||
			(world.exactRequired && world.requiredGuard != requirement.guard) {
			return false, nil
		}
		world.required = true
		world.exactRequired = requirement.exact
		world.requiredGuard = requirement.guard
	}
	for rowIndex := range rows {
		if rowIndex&31 == 0 {
			if err := ctx.Err(); err != nil {
				return false, errObservationCoverageCanceled
			}
		}
		row := &rows[rowIndex]
		for _, obligation := range row.observationObligations {
			key := observationCoverageKey{owner: obligation.BodyOwner, route: obligation.Route, anchor: obligation.Anchor}
			world := scratch.world(key)
			world.owed = append(world.owed, observationCoverageGuardWorld{row: row.Guard, local: obligation.Guard})
		}
		for _, evidence := range row.Observations {
			key := observationCoverageKey{owner: evidence.BodyOwner, route: evidence.Route, anchor: evidence.Anchor}
			world := scratch.world(key)
			world.evidence = append(world.evidence, observationCoverageGuardWorld{row: row.Guard, local: evidence.Guard})
		}
	}
	if len(rows) == 0 {
		return false, nil
	}
	for index := range scratch.groups {
		world := &scratch.groups[index]
		if world.exactRequired {
			if err := scratch.collectGuardAtoms(ctx, world.requiredGuard); err != nil {
				return false, err
			}
		}
		if len(world.owed) == 0 {
			if !world.exactRequired {
				return false, nil
			}
			continue
		}
		var err error
		world.direct, err = scratch.observationWorldsDirectlyCovered(ctx, world.owed, world.evidence)
		if err != nil {
			return false, err
		}
		for _, guard := range world.owed {
			if err := scratch.collectGuardAtoms(ctx, guard.row); err != nil {
				return false, err
			}
			if err := scratch.collectGuardAtoms(ctx, guard.local); err != nil {
				return false, err
			}
		}
		for _, guard := range world.evidence {
			if err := scratch.collectGuardAtoms(ctx, guard.row); err != nil {
				return false, err
			}
			if err := scratch.collectGuardAtoms(ctx, guard.local); err != nil {
				return false, err
			}
		}
	}
	if err := scratch.rankAtoms(); err != nil {
		return false, err
	}
	for index := range scratch.groups {
		world := &scratch.groups[index]
		if world.exactRequired {
			required, ok := scratch.guard(ctx, world.requiredGuard)
			if !ok {
				return false, scratch.coverageError(ctx)
			}
			owed, ok := scratch.guardUnion(ctx, world.owed)
			if !ok {
				return false, scratch.coverageError(ctx)
			}
			notOwed, ok := scratch.negate(ctx, owed)
			if !ok {
				return false, scratch.coverageError(ctx)
			}
			gap, ok := scratch.apply(ctx, decisionAnd, required, notOwed)
			if !ok {
				return false, scratch.coverageError(ctx)
			}
			if gap != decisionFalse {
				return false, nil
			}
			if len(world.owed) == 0 {
				continue
			}
		}
		if world.direct {
			continue
		}
		owed, ok := scratch.guardUnion(ctx, world.owed)
		if !ok {
			return false, scratch.coverageError(ctx)
		}
		evidence, ok := scratch.guardUnion(ctx, world.evidence)
		if !ok {
			return false, scratch.coverageError(ctx)
		}
		notEvidence, ok := scratch.negate(ctx, evidence)
		if !ok {
			return false, scratch.coverageError(ctx)
		}
		gap, ok := scratch.apply(ctx, decisionAnd, owed, notEvidence)
		if !ok {
			return false, scratch.coverageError(ctx)
		}
		if gap != decisionFalse {
			return false, nil
		}
	}
	return true, nil
}

func (s *observationCoverageScratch) observationWorldsDirectlyCovered(ctx context.Context, owed, evidence []observationCoverageGuardWorld) (bool, error) {
	for key := range s.directSet {
		delete(s.directSet, key)
	}
	for _, emitted := range evidence {
		s.directSet[emitted] = struct{}{}
	}
	for index, requirement := range owed {
		if index&63 == 0 && ctx.Err() != nil {
			return false, errObservationCoverageCanceled
		}
		if _, covered := s.directSet[requirement]; !covered {
			return false, nil
		}
	}
	return true, nil
}

func (s *observationCoverageScratch) world(key observationCoverageKey) *observationCoverageWorlds {
	if index, ok := s.worlds[key]; ok {
		return &s.groups[index]
	}
	index := len(s.groups)
	if index < cap(s.groups) {
		s.groups = s.groups[:index+1]
	} else {
		s.groups = append(s.groups, observationCoverageWorlds{})
	}
	s.worlds[key] = index
	return &s.groups[index]
}

func (s *observationCoverageScratch) collectGuardAtoms(ctx context.Context, guard Guard) error {
	if _, ok := s.collectSeen[guard]; ok {
		return nil
	}
	s.collectSeen[guard] = struct{}{}
	s.collectOps++
	if s.collectOps&255 == 0 && ctx.Err() != nil {
		return errObservationCoverageCanceled
	}
	if guard == 0 || int(guard) >= len(s.arena.guards) {
		return errObservationCoverageMalformed
	}
	node := s.arena.guards[guard]
	switch node.op {
	case guardTrue, guardFalse:
	case guardTruthy, guardFalsy:
		if _, ok := s.seen[node.value]; !ok {
			s.seen[node.value] = struct{}{}
			s.atoms = append(s.atoms, node.value)
		}
	case guardAnd, guardOr:
		for _, child := range node.args {
			if err := s.collectGuardAtoms(ctx, child); err != nil {
				return err
			}
		}
	default:
		return errObservationCoverageMalformed
	}
	return nil
}

func (s *observationCoverageScratch) rankAtoms() error {
	return rankStructuralGuardAtoms(s.arena, s.atoms, s.names, s.ranks)
}

func (s *observationCoverageScratch) guardUnion(ctx context.Context, guards []observationCoverageGuardWorld) (decisionRef, bool) {
	s.bddWork = s.bddWork[:0]
	for _, world := range guards {
		row, ok := s.guard(ctx, world.row)
		if !ok {
			return 0, false
		}
		local, ok := s.guard(ctx, world.local)
		if !ok {
			return 0, false
		}
		next, ok := s.apply(ctx, decisionAnd, row, local)
		if !ok {
			return 0, false
		}
		s.bddWork = append(s.bddWork, next)
	}
	if len(s.bddWork) == 0 {
		return decisionFalse, true
	}
	if s.lastUnionValid && len(s.lastUnionInputs) == len(s.bddWork) {
		equal := true
		for index := range s.bddWork {
			if s.lastUnionInputs[index] != s.bddWork[index] {
				equal = false
				break
			}
		}
		if equal {
			return s.lastUnionResult, true
		}
	}
	if cap(s.lastUnionInputs) < len(s.bddWork) {
		s.lastUnionInputs = make([]decisionRef, len(s.bddWork))
	} else {
		s.lastUnionInputs = s.lastUnionInputs[:len(s.bddWork)]
	}
	copy(s.lastUnionInputs, s.bddWork)
	for width := len(s.bddWork); width > 1; {
		write := 0
		for read := 0; read < width; read += 2 {
			if read+1 == width {
				s.bddWork[write] = s.bddWork[read]
			} else {
				joined, ok := s.apply(ctx, decisionOr, s.bddWork[read], s.bddWork[read+1])
				if !ok {
					return 0, false
				}
				s.bddWork[write] = joined
			}
			write++
		}
		width = write
	}
	s.lastUnionResult, s.lastUnionValid = s.bddWork[0], true
	return s.lastUnionResult, true
}

func (s *observationCoverageScratch) guard(ctx context.Context, guard Guard) (decisionRef, bool) {
	if cached, ok := s.guardMemo[guard]; ok {
		return cached, true
	}
	if guard == 0 || int(guard) >= len(s.arena.guards) {
		return 0, false
	}
	node := s.arena.guards[guard]
	var out decisionRef
	var ok bool = true
	switch node.op {
	case guardTrue:
		out = decisionTrue
	case guardFalse:
		out = decisionFalse
	case guardTruthy, guardFalsy:
		rank, found := s.ranks[node.value]
		if !found {
			return 0, false
		}
		out, ok = s.makeNode(rank, decisionFalse, decisionTrue)
		if ok && node.op == guardFalsy {
			out, ok = s.negate(ctx, out)
		}
	case guardAnd, guardOr:
		op, identity := decisionAnd, decisionTrue
		if node.op == guardOr {
			op, identity = decisionOr, decisionFalse
		}
		out = identity
		for _, child := range node.args {
			var next decisionRef
			next, ok = s.guard(ctx, child)
			if !ok {
				break
			}
			out, ok = s.apply(ctx, op, out, next)
			if !ok {
				break
			}
		}
	default:
		ok = false
	}
	if ok {
		s.guardMemo[guard] = out
	}
	return out, ok
}

func (s *observationCoverageScratch) makeNode(variable uint32, low, high decisionRef) (decisionRef, bool) {
	if int(low) >= len(s.nodes) || int(high) >= len(s.nodes) {
		return 0, false
	}
	return s.branch(variable, low, high), true
}

func (s *observationCoverageScratch) apply(ctx context.Context, op decisionBooleanOp, left, right decisionRef) (decisionRef, bool) {
	out, err := s.decisionKernel.apply(ctx, uint8(op), true, left, right, func(left, right decisionLeaf) (decisionLeaf, error) {
		if left > 1 || right > 1 {
			return 0, errDecisionMalformed
		}
		if op == decisionAnd {
			if left == 1 && right == 1 {
				return 1, nil
			}
			return 0, nil
		}
		if op == decisionOr {
			if left == 1 || right == 1 {
				return 1, nil
			}
			return 0, nil
		}
		return 0, errDecisionMalformed
	})
	return out, err == nil
}

func (s *observationCoverageScratch) negate(ctx context.Context, value decisionRef) (decisionRef, bool) {
	out, err := s.decisionKernel.mapLeaves(ctx, uint8(decisionNot), value, func(leaf decisionLeaf) (decisionLeaf, error) {
		if leaf > 1 {
			return 0, errDecisionMalformed
		}
		return 1 - leaf, nil
	})
	return out, err == nil
}

func (s *observationCoverageScratch) coverageError(ctx context.Context) error {
	if ctx.Err() != nil {
		return errObservationCoverageCanceled
	}
	return errObservationCoverageMalformed
}
