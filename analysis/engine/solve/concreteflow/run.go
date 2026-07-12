package concreteflow

import (
	"context"
	"errors"

	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// RunConfig holds solve-local policy which cannot be retained by Plan.
type RunConfig struct {
	Context           context.Context
	IncludeVersions   bool
	Transfer          func(cfg.Point, func(cfg.Point) state.State, func(cfg.Point, state.State))
	TransferVersioned func(cfg.Point, func(cfg.Point) (state.State, uint64), func(cfg.Point, state.State))
	// FuseIdentity certifies that the canonical equation at point is an
	// unobserved identity node+edge transaction. Nil disables fusion.
	FuseIdentity func(cfg.Point) bool
}

type Result struct {
	Points   map[cfg.Point]state.State
	Versions map[cfg.Point]uint64
}

type loopFrame struct {
	begin  int
	before uint64
}

// Run executes sys transactionally. Errors publish no state or revisions.
func Run(config RunConfig, sys solve.EquationSystem[cfg.Point, state.State], plan *Plan) (Result, error) {
	if plan == nil || !plan.wto.Matches(sys.Cells) || len(sys.Cells) != plan.graph.Size() {
		return Result{}, solve.ErrWTOPlanUncovered
	}
	token := cancellation.FromContext(config.Context).Token()
	if err := token.Err(); err != nil {
		return Result{}, errors.Join(solve.ErrCanceled, err)
	}
	domain := sys.Lattice
	n := plan.graph.Size()
	bottom := domain.Bottom()
	cur := make([]state.State, n)
	versions := make([]uint64, n)
	hasInitial := make([]bool, n)
	initialPoints := make([]cfg.Point, 0, 4)
	initialValues := make([]state.State, 0, 4)
	visits := make([]uint32, n)
	widenChanges := make([]uint32, n)
	for i := range cur {
		cur[i] = bottom
	}
	widenAt := sys.WidenAt
	if widenAt == nil {
		widenAt = func(cfg.Point) bool { return false }
	}
	widenDelay := sys.WidenDelay
	if widenDelay == nil {
		widenDelay = func(cfg.Point) int { return 0 }
	}
	abstract := sys.Abstract
	if abstract == nil {
		abstract = func(_ cfg.Point, value state.State) state.State { return value }
	}
	var nextVersion uint64
	bump := func(point cfg.Point) {
		nextVersion++
		versions[point] = nextVersion
	}
	for _, point := range sys.Cells {
		if sys.InitialSparse != nil {
			if value, ok := sys.InitialSparse(point); ok {
				cur[point], hasInitial[point] = value, true
				initialPoints = append(initialPoints, point)
				initialValues = append(initialValues, value)
				bump(point)
			}
			continue
		}
		value := bottom
		if sys.Initial != nil {
			value = sys.Initial(point)
		}
		cur[point], hasInitial[point] = value, true
		initialPoints = append(initialPoints, point)
		initialValues = append(initialValues, value)
		bump(point)
	}
	materialize := func(point cfg.Point) state.State {
		if versions[point] == 0 {
			bump(point)
		}
		return cur[point]
	}
	uncovered := false
	active := cfg.Point(0)
	read := func(point cfg.Point) state.State {
		if uint64(point) >= uint64(n) || !plan.wto.CoversInfluence(point, active) {
			uncovered = true
			return bottom
		}
		return materialize(point)
	}
	readVersioned := func(point cfg.Point) (state.State, uint64) {
		value := read(point)
		if uint64(point) >= uint64(n) {
			return value, 0
		}
		return value, versions[point]
	}
	emit := func(point cfg.Point, value state.State) {
		if uint64(point) >= uint64(n) || !plan.wto.CoversEmission(active, point) {
			uncovered = true
			return
		}
		prev := materialize(point)
		next := domain.Join(prev, value)
		delayConsumed := false
		if widenAt(point) && visits[point] != 0 {
			delay := widenDelay(point)
			if delay < 0 {
				delay = 0
			}
			if int(widenChanges[point]) >= delay {
				next = domain.Widen(prev, next)
			} else {
				delayConsumed = true
			}
		}
		next = abstract(point, next)
		if domain.Equal(next, prev) {
			return
		}
		cur[point] = next
		bump(point)
		if delayConsumed {
			widenChanges[point]++
		}
	}
	var iterations uint64
	var narrowCandidate []state.State
	var narrowCandidateSet []bool
	candidateEmit := func(dst cfg.Point, value state.State) {
		if uint64(dst) >= uint64(n) || !plan.wto.CoversEmission(active, dst) {
			uncovered = true
			return
		}
		prev := bottom
		if narrowCandidateSet[dst] {
			prev = narrowCandidate[dst]
		}
		narrowCandidate[dst] = abstract(dst, domain.Join(prev, value))
		narrowCandidateSet[dst] = true
	}
	runPoint := func(point cfg.Point, narrowing bool, candidate []state.State, candidateSet []bool) error {
		if iterations%uint64(cancellation.EveryCheap) == 0 {
			if err := token.Err(); err != nil {
				return errors.Join(solve.ErrCanceled, err)
			}
		}
		iterations++
		active = point
		if sys.Stats != nil && !narrowing {
			sys.Stats.TransferCalls++
		}
		if !narrowing && config.FuseIdentity != nil && plan.identity[point] && config.FuseIdentity(point) {
			in := read(point)
			succ := cfg.SuccessorsReadOnly(plan.graph, point)[0]
			if !hasInitial[succ] {
				// The destination has one predecessor and no independent seed: its
				// equation is exactly the predecessor snapshot, not a general join.
				// Preserve materialization/version boundaries while reusing the
				// immutable operand directly.
				prev := materialize(succ)
				if !domain.Equal(in, prev) {
					cur[succ] = in
					bump(succ)
				}
			} else {
				emit(succ, in)
			}
		} else if narrowing {
			narrowCandidate, narrowCandidateSet = candidate, candidateSet
			if config.FuseIdentity != nil && plan.identity[point] && config.FuseIdentity(point) {
				succ := cfg.SuccessorsReadOnly(plan.graph, point)[0]
				if !hasInitial[succ] {
					candidate[succ] = read(point)
					candidateSet[succ] = true
				} else {
					candidateEmit(succ, read(point))
				}
			} else {
				transfer := sys.Transfer
				if config.Transfer != nil {
					transfer = config.Transfer
				}
				transfer(point, read, candidateEmit)
			}
		} else if config.TransferVersioned != nil {
			config.TransferVersioned(point, readVersioned, emit)
		} else if sys.TransferVersioned != nil {
			sys.TransferVersioned(point, readVersioned, emit)
		} else if config.Transfer != nil {
			config.Transfer(point, read, emit)
		} else {
			sys.Transfer(point, read, emit)
		}
		if err := token.Err(); err != nil {
			return errors.Join(solve.ErrCanceled, err)
		}
		if uncovered {
			return solve.ErrWTOPlanUncovered
		}
		if !narrowing && widenAt(point) {
			visits[point]++
		}
		return nil
	}
	frames := make([]loopFrame, 0, plan.maxNesting)
	for pc := 0; pc < len(plan.tape); {
		ins := plan.tape[pc]
		switch ins.op {
		case opVertex:
			if err := runPoint(ins.point, false, nil, nil); err != nil {
				return Result{}, err
			}
			pc++
		case opLoopBegin:
			before := versions[ins.point]
			if err := runPoint(ins.point, false, nil, nil); err != nil {
				return Result{}, err
			}
			frames = append(frames, loopFrame{begin: pc, before: before})
			pc++
		case opLoopEnd:
			frame := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			head := plan.tape[frame.begin].point
			if versions[head] != frame.before {
				pc = frame.begin
			} else {
				pc++
			}
		}
	}

	// Match solve's two bounded decreasing passes exactly. Candidate equations
	// read the converged state and accumulate from immutable initial values.
	hasWiden := false
	for _, point := range sys.Cells {
		if widenAt(point) {
			hasWiden = true
			break
		}
	}
	if domain.Narrow != nil && hasWiden {
		candidate := make([]state.State, n)
		candidateSet := make([]bool, n)
		for pass := 0; pass < 2; pass++ {
			for point := 0; point < n; point++ {
				candidate[point] = bottom
				candidateSet[point] = false
			}
			for i, point := range initialPoints {
				candidate[point] = initialValues[i]
				candidateSet[point] = true
			}
			for _, point := range sys.Cells {
				if err := runPoint(point, true, candidate, candidateSet); err != nil {
					return Result{}, err
				}
			}
			changed := false
			for _, point := range sys.Cells {
				prev := materialize(point)
				input := bottom
				if candidateSet[point] {
					input = candidate[point]
				}
				next := abstract(point, domain.Narrow(prev, input))
				if domain.LessOrEq(next, prev) && !domain.Equal(next, prev) {
					cur[point] = next
					bump(point)
					changed = true
				}
			}
			if !changed {
				break
			}
		}
	}
	if err := token.Err(); err != nil {
		return Result{}, errors.Join(solve.ErrCanceled, err)
	}
	points := make(map[cfg.Point]state.State, n)
	var versionMap map[cfg.Point]uint64
	if config.IncludeVersions {
		versionMap = make(map[cfg.Point]uint64, n)
	}
	for _, point := range sys.Cells {
		points[point] = materialize(point)
		if versionMap != nil {
			versionMap[point] = versions[point]
		}
	}
	return Result{Points: points, Versions: versionMap}, nil
}
