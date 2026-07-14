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
	owed     []observationCoverageGuardWorld
	evidence []observationCoverageGuardWorld
	direct   bool
}

type observationCoverageGuardWorld struct{ row, local Guard }

type coverageBDD uint32

const (
	coverageFalse coverageBDD = iota
	coverageTrue
)

type coverageBDDNode struct {
	variable uint32
	low      coverageBDD
	high     coverageBDD
}

type coverageUniqueKey struct {
	variable uint32
	low      coverageBDD
	high     coverageBDD
}

type coverageApplyOp uint8

const (
	coverageAnd coverageApplyOp = iota + 1
	coverageOr
)

type coverageApplyKey struct {
	op          coverageApplyOp
	left, right coverageBDD
}

type observationCoverageScratch struct {
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
	bddWork         []coverageBDD
	lastUnionInputs []coverageBDD
	lastUnionResult coverageBDD
	lastUnionValid  bool

	nodes     []coverageBDDNode
	unique    map[coverageUniqueKey]coverageBDD
	applyMemo map[coverageApplyKey]coverageBDD
	notMemo   map[coverageBDD]coverageBDD
	guardMemo map[Guard]coverageBDD
	applyOps  int
}

func newObservationCoverageScratch() *observationCoverageScratch {
	return &observationCoverageScratch{
		worlds: make(map[observationCoverageKey]int),
		seen:   make(map[ValueTerm]struct{}), ranks: make(map[ValueTerm]uint32), names: make(map[ValueTerm]string), collectSeen: make(map[Guard]struct{}), directSet: make(map[observationCoverageGuardWorld]struct{}),
		unique: make(map[coverageUniqueKey]coverageBDD), applyMemo: make(map[coverageApplyKey]coverageBDD),
		notMemo: make(map[coverageBDD]coverageBDD), guardMemo: make(map[Guard]coverageBDD),
	}
}

func (s *observationCoverageScratch) reset(arena *Arena) {
	for index := range s.groups {
		s.groups[index].owed = s.groups[index].owed[:0]
		s.groups[index].evidence = s.groups[index].evidence[:0]
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
	s.nodes = s.nodes[:0]
	s.nodes = append(s.nodes, coverageBDDNode{}, coverageBDDNode{})
	for key := range s.unique {
		delete(s.unique, key)
	}
	for key := range s.applyMemo {
		delete(s.applyMemo, key)
	}
	for key := range s.notMemo {
		delete(s.notMemo, key)
	}
	for key := range s.guardMemo {
		delete(s.guardMemo, key)
	}
	s.applyOps = 0
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
	scratch.reset(arena)
	requirements, sealed := plan.ObservationRequirements()
	if !sealed {
		return false, nil
	}
	owner := plan.ObservationBody()
	cursor := requirements.Cursor(false)
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
		scratch.world(observationCoverageKey{owner: owner, anchor: anchor})
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
		if len(world.owed) == 0 {
			return false, nil
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
		gap, ok := scratch.apply(ctx, coverageAnd, owed, notEvidence)
		if !ok {
			return false, scratch.coverageError(ctx)
		}
		if gap != coverageFalse {
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
	for _, atom := range s.atoms {
		if _, ok := s.names[atom]; !ok {
			s.names[atom] = s.arena.canonicalValue(atom)
		}
	}
	for index := 1; index < len(s.atoms); index++ {
		atom := s.atoms[index]
		name := s.names[atom]
		position := index
		for position > 0 {
			prior := s.atoms[position-1]
			priorName := s.names[prior]
			if name > priorName || (name == priorName && atom >= prior) {
				break
			}
			s.atoms[position] = s.atoms[position-1]
			position--
		}
		s.atoms[position] = atom
	}
	for index, atom := range s.atoms {
		s.ranks[atom] = uint32(index)
	}
	return nil
}

func (s *observationCoverageScratch) guardUnion(ctx context.Context, guards []observationCoverageGuardWorld) (coverageBDD, bool) {
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
		next, ok := s.apply(ctx, coverageAnd, row, local)
		if !ok {
			return 0, false
		}
		s.bddWork = append(s.bddWork, next)
	}
	if len(s.bddWork) == 0 {
		return coverageFalse, true
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
		s.lastUnionInputs = make([]coverageBDD, len(s.bddWork))
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
				joined, ok := s.apply(ctx, coverageOr, s.bddWork[read], s.bddWork[read+1])
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

func (s *observationCoverageScratch) guard(ctx context.Context, guard Guard) (coverageBDD, bool) {
	if cached, ok := s.guardMemo[guard]; ok {
		return cached, true
	}
	if guard == 0 || int(guard) >= len(s.arena.guards) {
		return 0, false
	}
	node := s.arena.guards[guard]
	var out coverageBDD
	var ok bool = true
	switch node.op {
	case guardTrue:
		out = coverageTrue
	case guardFalse:
		out = coverageFalse
	case guardTruthy, guardFalsy:
		rank, found := s.ranks[node.value]
		if !found {
			return 0, false
		}
		out, ok = s.makeNode(rank, coverageFalse, coverageTrue)
		if ok && node.op == guardFalsy {
			out, ok = s.negate(ctx, out)
		}
	case guardAnd, guardOr:
		op, identity := coverageAnd, coverageTrue
		if node.op == guardOr {
			op, identity = coverageOr, coverageFalse
		}
		out = identity
		for _, child := range node.args {
			var next coverageBDD
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

func (s *observationCoverageScratch) makeNode(variable uint32, low, high coverageBDD) (coverageBDD, bool) {
	if low == high {
		return low, true
	}
	key := coverageUniqueKey{variable: variable, low: low, high: high}
	if prior, ok := s.unique[key]; ok {
		return prior, true
	}
	id := coverageBDD(len(s.nodes))
	s.nodes = append(s.nodes, coverageBDDNode{variable: variable, low: low, high: high})
	s.unique[key] = id
	return id, true
}

func (s *observationCoverageScratch) apply(ctx context.Context, op coverageApplyOp, left, right coverageBDD) (coverageBDD, bool) {
	s.applyOps++
	if s.applyOps&255 == 0 && ctx.Err() != nil {
		return 0, false
	}
	if op == coverageAnd {
		if left == coverageFalse || right == coverageFalse {
			return coverageFalse, true
		}
		if left == coverageTrue {
			return right, true
		}
		if right == coverageTrue || left == right {
			return left, true
		}
	} else {
		if left == coverageTrue || right == coverageTrue {
			return coverageTrue, true
		}
		if left == coverageFalse {
			return right, true
		}
		if right == coverageFalse || left == right {
			return left, true
		}
	}
	if right < left {
		left, right = right, left
	}
	key := coverageApplyKey{op: op, left: left, right: right}
	if prior, ok := s.applyMemo[key]; ok {
		return prior, true
	}
	leftNode, rightNode := s.nodes[left], s.nodes[right]
	variable := leftNode.variable
	if rightNode.variable < variable {
		variable = rightNode.variable
	}
	leftLow, leftHigh := left, left
	if leftNode.variable == variable {
		leftLow, leftHigh = leftNode.low, leftNode.high
	}
	rightLow, rightHigh := right, right
	if rightNode.variable == variable {
		rightLow, rightHigh = rightNode.low, rightNode.high
	}
	low, ok := s.apply(ctx, op, leftLow, rightLow)
	if !ok {
		return 0, false
	}
	high, ok := s.apply(ctx, op, leftHigh, rightHigh)
	if !ok {
		return 0, false
	}
	out, ok := s.makeNode(variable, low, high)
	if ok {
		s.applyMemo[key] = out
	}
	return out, ok
}

func (s *observationCoverageScratch) negate(ctx context.Context, value coverageBDD) (coverageBDD, bool) {
	if value == coverageFalse {
		return coverageTrue, true
	}
	if value == coverageTrue {
		return coverageFalse, true
	}
	if prior, ok := s.notMemo[value]; ok {
		return prior, true
	}
	if ctx.Err() != nil {
		return 0, false
	}
	node := s.nodes[value]
	low, ok := s.negate(ctx, node.low)
	if !ok {
		return 0, false
	}
	high, ok := s.negate(ctx, node.high)
	if !ok {
		return 0, false
	}
	out, ok := s.makeNode(node.variable, low, high)
	if ok {
		s.notMemo[value] = out
	}
	return out, ok
}

func (s *observationCoverageScratch) coverageError(ctx context.Context) error {
	if ctx.Err() != nil {
		return errObservationCoverageCanceled
	}
	return errObservationCoverageMalformed
}
