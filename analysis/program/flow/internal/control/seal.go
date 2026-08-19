package control

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal proves the complete authored lexical-control denominator. Source order
// is the sole cursor authority; Body, Binding, and Containment are consumed as
// already-proved structural relations. All cursor, scope, and traversal state
// is discarded before the compact Shape is published.
func Seal(
	preimage source.Preimage,
	view authored.View,
	bodies *body.Result,
	bindings binding.Result,
	forest *containment.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Shape, error) {
	identity := preimage.Identity()
	sourceID := identity.ContentID()
	flowID := view.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() ||
		identity.TermCount() == 0 || bodies == nil || forest == nil {
		return nil, errors.New("program/flow/control: owner view expired")
	}
	if !body.Matches(bodies, sourceID, flowID) {
		return nil, errors.New("program/flow/control: Body provenance disagrees with Source or Flow")
	}
	if !binding.Matches(&bindings, sourceID, flowID) {
		return nil, errors.New("program/flow/control: Binding provenance disagrees with Source or Flow")
	}
	if !containment.Matches(forest, sourceID, flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/control: Containment provenance disagrees with Source, Flow, Static, or Module")
	}
	var counts [keyspace.FamilyCount]int
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return nil, errors.New("program/flow/control: invalid Source family cardinality")
		}
		counts[family] = count
		total += uint64(count)
	}
	if total != uint64(identity.TermCount()) || forest.Count() != int(identity.TermCount()) ||
		bodies.BodyCount() != counts[keyspace.FamilyBody] || bindings.CellCount() != counts[keyspace.FamilyCell] ||
		counts[keyspace.FamilyBody] == 0 || counts[keyspace.FamilyOutcome] != 0 {
		return nil, errors.New("program/flow/control: structural denominator mismatch")
	}

	control := view.Control()
	cells := view.Storage().Cells()
	functions := view.Functions()
	returns, breaks, labels := control.Returns(), control.Breaks(), control.Labels()
	gotos, branches, loops := control.Gotos(), control.Branches(), control.Loops()
	if returns.Count() != counts[keyspace.FamilyReturn] || breaks.Count() != counts[keyspace.FamilyBreak] ||
		labels.Count() != counts[keyspace.FamilyLabel] || gotos.Count() != counts[keyspace.FamilyGoto] ||
		branches.Count() != counts[keyspace.FamilyBranch] || loops.Count() != counts[keyspace.FamilyLoop] ||
		functions.Count() != counts[keyspace.FamilyFunction] || cells.Count() != counts[keyspace.FamilyCell] ||
		preimage.Faults().Count() != counts[keyspace.FamilyControlFault] {
		return nil, errors.New("program/flow/control: authored control cardinality mismatch")
	}

	labelBody := make([]keyspace.Term, labels.Count()+1)
	breakLoop := make([]keyspace.Term, breaks.Count()+1)
	gotoTargetBody := make([]keyspace.Term, gotos.Count()+1)
	if err := validateRows(counts, functions, returns, breaks, labels, gotos, branches, loops, cells, bodies, bindings, forest, labelBody, breakLoop); err != nil {
		return nil, err
	}

	scratch, err := buildScopes(preimage, view, bodies, bindings, counts, labelBody)
	if err != nil {
		return nil, err
	}
	pre, post, err := scopeIntervals(scratch.parents)
	if err != nil {
		return nil, err
	}
	for index := 0; index < gotos.Count(); index++ {
		gotoTerm := keyspace.MakeTerm(keyspace.FamilyGoto, uint32(index+1))
		owner, target, ok := gotos.Get(gotoTerm)
		if !ok || !validTerm(target, counts, keyspace.FamilyLabel) || !scratch.gotoSeen[index+1] || !scratch.labelSeen[keyspace.TermOrdinal(target)] {
			return nil, errors.New("program/flow/control: invalid Goto row")
		}
		targetBody := labelBody[keyspace.TermOrdinal(target)]
		ownerActivation, ownerOK := bodies.Activation(owner)
		targetActivation, targetOK := bodies.Activation(targetBody)
		if !ownerOK || !targetOK || ownerActivation != targetActivation || !bodies.AncestorOrSelf(targetBody, owner) ||
			!scopeAncestor(pre, post, scratch.labelScope[keyspace.TermOrdinal(target)], scratch.gotoScope[index+1]) {
			return nil, errors.New("program/flow/control: Goto enters a lexical scope or activation")
		}
		gotoTargetBody[index+1] = targetBody
	}

	return &Shape{
		sourceID:       sourceID,
		flowID:         flowID,
		staticID:       staticID,
		moduleID:       moduleID,
		labelBody:      labelBody,
		breakLoop:      breakLoop,
		gotoTargetBody: gotoTargetBody,
	}, nil
}

func validateRows(
	counts [keyspace.FamilyCount]int,
	functions authored.Functions,
	returns authored.Returns,
	breaks authored.Breaks,
	labels authored.Labels,
	gotos authored.Gotos,
	branches authored.Branches,
	loops authored.Loops,
	cells authored.Cells,
	bodies *body.Result,
	bindings binding.Result,
	forest *containment.Result,
	labelBody, breakLoop []keyspace.Term,
) error {
	for index := 0; index < returns.Count(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyReturn, uint32(index+1))
		owner, _, ok := returns.Get(term)
		if !ok || !controlParent(forest, term, owner, counts) {
			return errors.New("program/flow/control: invalid Return owner")
		}
	}
	for index := 0; index < labels.Count(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyLabel, uint32(index+1))
		owner, ok := labels.Get(term)
		if !ok || !controlParent(forest, term, owner, counts) {
			return errors.New("program/flow/control: invalid Label owner")
		}
		labelBody[index+1] = owner
	}
	for index := 0; index < gotos.Count(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyGoto, uint32(index+1))
		owner, target, ok := gotos.Get(term)
		if !ok || !validTerm(target, counts, keyspace.FamilyLabel) || !controlParent(forest, term, owner, counts) {
			return errors.New("program/flow/control: invalid Goto owner or target")
		}
	}
	for index := 0; index < branches.Count(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyBranch, uint32(index+1))
		owner, _, whenTrue, whenFalse, ok := branches.Get(term)
		trueParent, trueOK := bodies.Parent(whenTrue)
		falseParent, falseOK := bodies.Parent(whenFalse)
		if !ok || !controlParent(forest, term, owner, counts) || !trueOK || !falseOK || trueParent != owner || falseParent != owner || whenTrue == whenFalse {
			return errors.New("program/flow/control: invalid Branch shape")
		}
	}
	for index := 0; index < loops.Count(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyLoop, uint32(index+1))
		owner, child, loopKind, _, ok := loops.Get(term)
		parent, parentOK := bodies.Parent(child)
		if !ok || !validLoopKind(loopKind) || !controlParent(forest, term, owner, counts) || !parentOK || parent != owner {
			return errors.New("program/flow/control: invalid Loop shape")
		}
		cellCount, cellsOK := loops.CellCount(term)
		if !cellsOK || !validLoopCells(loopKind, cellCount) {
			return errors.New("program/flow/control: invalid Loop cell cardinality")
		}
		for cellIndex := 0; cellIndex < cellCount; cellIndex++ {
			cell, cellOK := loops.CellAt(term, cellIndex)
			cellKind, cellBody, cellKey, cellRowOK := cells.Get(cell)
			role, roleOK := bindings.Role(cell)
			host, hostOK := bindings.Host(cell)
			if !cellOK || !cellRowOK || cellKind != authored.CellLocal || cellBody != child || cellKey != 0 ||
				!roleOK || !hostOK || role != kind.CellLoop || host != term {
				return errors.New("program/flow/control: invalid Loop Cell binding")
			}
		}
	}
	if err := validateBodyTopology(counts, functions, loops, bodies); err != nil {
		return err
	}
	for index := 0; index < breaks.Count(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyBreak, uint32(index+1))
		owner, _, ok := breaks.Get(term)
		loop, selected := bodies.NearestLoop(owner)
		if !ok || !selected || !validTerm(loop, counts, keyspace.FamilyLoop) ||
			!controlParent(forest, term, owner, counts) {
			return errors.New("program/flow/control: invalid Break selection")
		}
		breakLoop[index+1] = loop
	}
	return nil
}

// validateBodyTopology cross-checks the current authored Function→Body and
// Loop→Body relations against the immutable Body topology. The local recurrence
// is construction scratch only: an entry starts at zero; a Function child
// starts a new activation and clears the enclosing loop; every other child
// inherits activation and takes its authored Loop at that Body or its parent's
// nearest loop. Body remains the sole owner of the published projections.
func validateBodyTopology(
	counts [keyspace.FamilyCount]int,
	functions authored.Functions,
	loops authored.Loops,
	bodies *body.Result,
) error {
	bodyCount := counts[keyspace.FamilyBody]
	functionAt := make([]keyspace.Term, bodyCount+1)
	loopAt := make([]keyspace.Term, bodyCount+1)
	for index := 0; index < functions.Count(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1))
		owner, child, _, ok := functions.Get(term)
		if !ok || !validTerm(owner, counts, keyspace.FamilyBody) || !validTerm(child, counts, keyspace.FamilyBody) || owner == child {
			return errors.New("program/flow/control: invalid Function Body shape")
		}
		childOrdinal := keyspace.TermOrdinal(child)
		if functionAt[childOrdinal] != 0 || loopAt[childOrdinal] != 0 {
			return errors.New("program/flow/control: duplicate Function Body")
		}
		parent, parentOK := bodies.Parent(child)
		activation, activationOK := bodies.Activation(child)
		nearest, nearestOK := bodies.NearestLoop(child)
		if !parentOK || parent != owner || !activationOK || activation != term || !nearestOK || nearest != 0 {
			return errors.New("program/flow/control: Function Body disagrees with Body topology")
		}
		functionAt[childOrdinal] = term
	}
	for index := 0; index < loops.Count(); index++ {
		term := keyspace.MakeTerm(keyspace.FamilyLoop, uint32(index+1))
		owner, child, _, _, ok := loops.Get(term)
		if !ok || !validTerm(owner, counts, keyspace.FamilyBody) || !validTerm(child, counts, keyspace.FamilyBody) || owner == child {
			return errors.New("program/flow/control: invalid Loop Body shape")
		}
		childOrdinal := keyspace.TermOrdinal(child)
		if loopAt[childOrdinal] != 0 || functionAt[childOrdinal] != 0 {
			return errors.New("program/flow/control: duplicate Loop Body")
		}
		parent, parentOK := bodies.Parent(child)
		if !parentOK || parent != owner {
			return errors.New("program/flow/control: Loop Body disagrees with Body topology")
		}
		loopAt[childOrdinal] = term
	}

	expectedActivation := make([]keyspace.Term, bodyCount+1)
	expectedNearestLoop := make([]keyspace.Term, bodyCount+1)
	state := make([]uint8, bodyCount+1)
	path := make([]uint32, 0)
	for index := 0; index < bodyCount; index++ {
		bodyTerm, ok := bodies.BodyAt(index)
		if !ok {
			return errors.New("program/flow/control: Body view expired")
		}
		bodyOrdinal := keyspace.TermOrdinal(bodyTerm)
		if state[bodyOrdinal] == 2 {
			continue
		}
		path = path[:0]
		current := bodyTerm
		for {
			currentOrdinal := keyspace.TermOrdinal(current)
			if currentOrdinal == 0 || int(currentOrdinal) > bodyCount {
				return errors.New("program/flow/control: invalid Body topology index")
			}
			switch state[currentOrdinal] {
			case 2:
				current = 0
			case 1:
				return errors.New("program/flow/control: cyclic Body topology")
			default:
				state[currentOrdinal] = 1
				path = append(path, currentOrdinal)
				parent, hasParent := bodies.Parent(current)
				if !hasParent {
					current = 0
				} else {
					current = parent
				}
			}
			if current == 0 {
				break
			}
		}
		for pathIndex := len(path) - 1; pathIndex >= 0; pathIndex-- {
			ordinal := path[pathIndex]
			bodyTerm, _ := bodies.BodyAt(int(ordinal - 1))
			parent, hasParent := bodies.Parent(bodyTerm)
			active, nearest := keyspace.Term(0), keyspace.Term(0)
			if hasParent {
				parentOrdinal := keyspace.TermOrdinal(parent)
				if parentOrdinal == 0 || int(parentOrdinal) > bodyCount || state[parentOrdinal] != 2 {
					return errors.New("program/flow/control: unresolved Body topology parent")
				}
				active = expectedActivation[parentOrdinal]
				nearest = expectedNearestLoop[parentOrdinal]
			}
			if functionAt[ordinal] != 0 {
				active, nearest = functionAt[ordinal], 0
			} else if loopAt[ordinal] != 0 {
				nearest = loopAt[ordinal]
			}
			expectedActivation[ordinal], expectedNearestLoop[ordinal] = active, nearest
			state[ordinal] = 2
			actualActivation, activationOK := bodies.Activation(bodyTerm)
			actualNearest, nearestOK := bodies.NearestLoop(bodyTerm)
			if !activationOK || !nearestOK || actualActivation != active || actualNearest != nearest {
				return errors.New("program/flow/control: Body topology recurrence mismatch")
			}
		}
	}
	return nil
}

type scopeScratch struct {
	parents     []int
	bodyBase    []int
	labelScope  []int
	gotoScope   []int
	labelSeen   []bool
	gotoSeen    []bool
	controlSeen [keyspace.FamilyCount][]bool
}

func buildScopes(
	preimage source.Preimage,
	view authored.View,
	bodies *body.Result,
	bindings binding.Result,
	counts [keyspace.FamilyCount]int,
	labelBody []keyspace.Term,
) (scopeScratch, error) {
	scratch := scopeScratch{
		parents:    []int{0},
		bodyBase:   make([]int, counts[keyspace.FamilyBody]+1),
		labelScope: make([]int, counts[keyspace.FamilyLabel]+1),
		gotoScope:  make([]int, counts[keyspace.FamilyGoto]+1),
		labelSeen:  make([]bool, counts[keyspace.FamilyLabel]+1),
		gotoSeen:   make([]bool, counts[keyspace.FamilyGoto]+1),
	}
	for _, family := range [...]keyspace.Family{
		keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyGoto,
		keyspace.FamilyBranch, keyspace.FamilyLoop,
	} {
		scratch.controlSeen[family] = make([]bool, counts[family]+1)
	}
	seeded := make([]bool, counts[keyspace.FamilyBody]+1)
	queued := make([]keyspace.Term, 0, counts[keyspace.FamilyBody])
	seed := func(child keyspace.Term, scope int) error {
		if !validTerm(child, counts, keyspace.FamilyBody) || scope < 0 || scope >= len(scratch.parents) {
			return errors.New("program/flow/control: invalid Body scope seed")
		}
		ordinal := keyspace.TermOrdinal(child)
		if seeded[ordinal] {
			return errors.New("program/flow/control: duplicate Body scope seed")
		}
		seeded[ordinal] = true
		scratch.bodyBase[ordinal] = scope
		queued = append(queued, child)
		return nil
	}
	for index := 0; index < bodies.BodyCount(); index++ {
		bodyTerm, ok := bodies.BodyAt(index)
		if !ok {
			return scopeScratch{}, errors.New("program/flow/control: Body view expired")
		}
		parent, hasParent := bodies.Parent(bodyTerm)
		activation, activationOK := bodies.Activation(bodyTerm)
		if !activationOK {
			return scopeScratch{}, errors.New("program/flow/control: Body activation unavailable")
		}
		if !hasParent {
			if activation != 0 {
				return scopeScratch{}, errors.New("program/flow/control: invalid Entry activation")
			}
			if err := seed(bodyTerm, 0); err != nil {
				return scopeScratch{}, err
			}
			continue
		}
		parentActivation, parentOK := bodies.Activation(parent)
		if !parentOK {
			return scopeScratch{}, errors.New("program/flow/control: parent activation unavailable")
		}
		if activation != parentActivation {
			if !validTerm(activation, counts, keyspace.FamilyFunction) {
				return scopeScratch{}, errors.New("program/flow/control: invalid Function activation")
			}
			if err := seed(bodyTerm, 0); err != nil {
				return scopeScratch{}, err
			}
		}
	}

	order, bindOrder := preimage.Order(), preimage.Binds()
	control, storage := view.Control(), view.Storage()
	branches, loops := control.Branches(), control.Loops()
	binds := storage.Binds()
	cells := storage.Cells()
	repeatBody := make([]bool, counts[keyspace.FamilyBody]+1)
	for index := 0; index < loops.Count(); index++ {
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, uint32(index+1))
		_, child, loopKind, _, ok := loops.Get(loop)
		if !ok {
			return scopeScratch{}, errors.New("program/flow/control: Loop view expired")
		}
		if loopKind == kind.LoopRepeat {
			repeatBody[keyspace.TermOrdinal(child)] = true
		}
	}

	push := func(parent int) int {
		scratch.parents = append(scratch.parents, parent)
		return len(scratch.parents) - 1
	}
	processed := 0
	for at := 0; at < len(queued); at++ {
		bodyTerm := queued[at]
		processed++
		bodyOrdinal := keyspace.TermOrdinal(bodyTerm)
		env := scratch.bodyBase[bodyOrdinal]
		rootCount, ok := bodies.RootCount(bodyTerm)
		if !ok {
			return scopeScratch{}, errors.New("program/flow/control: Body roots unavailable")
		}
		length, ok := order.BodyLen(bodyTerm)
		if !ok {
			return scopeScratch{}, errors.New("program/flow/control: Source order expired")
		}
		rootCursor := 0
		for index := 0; index < length; index++ {
			term, ok := order.BodyAt(bodyTerm, index)
			if !ok {
				return scopeScratch{}, errors.New("program/flow/control: Source order expired")
			}
			if keyspace.TermFamily(term) == keyspace.FamilyLabel {
				ordinal := keyspace.TermOrdinal(term)
				if ordinal == 0 || uint64(ordinal) >= uint64(len(scratch.labelSeen)) || scratch.labelSeen[ordinal] || labelBody[ordinal] != bodyTerm {
					return scopeScratch{}, errors.New("program/flow/control: ambiguous Label source position")
				}
				labelScope := env
				if rootCursor == rootCount && !repeatBody[bodyOrdinal] {
					labelScope = scratch.bodyBase[bodyOrdinal]
				}
				scratch.labelSeen[ordinal] = true
				scratch.labelScope[ordinal] = labelScope
			}

			if rootCursor >= rootCount {
				if body.RootFamily(keyspace.TermFamily(term)) {
					return scopeScratch{}, errors.New("program/flow/control: unexpected source root")
				}
				continue
			}
			root, ok := bodies.RootAt(bodyTerm, rootCursor)
			if !ok || root != term {
				if body.RootFamily(keyspace.TermFamily(term)) {
					return scopeScratch{}, errors.New("program/flow/control: control row is not a Body root")
				}
				continue
			}
			rootCursor++
			family, ordinal := keyspace.TermFamily(root), keyspace.TermOrdinal(root)
			if seen := scratch.controlSeen[family]; seen != nil {
				if ordinal == 0 || uint64(ordinal) >= uint64(len(seen)) || seen[ordinal] {
					return scopeScratch{}, errors.New("program/flow/control: duplicate control Body root")
				}
				seen[ordinal] = true
			}
			switch keyspace.TermFamily(root) {
			case keyspace.FamilyGoto:
				ordinal := keyspace.TermOrdinal(root)
				if ordinal == 0 || uint64(ordinal) >= uint64(len(scratch.gotoSeen)) || scratch.gotoSeen[ordinal] {
					return scopeScratch{}, errors.New("program/flow/control: ambiguous Goto source position")
				}
				scratch.gotoSeen[ordinal] = true
				scratch.gotoScope[ordinal] = env
			case keyspace.FamilyBind:
				count, ok := bindOrder.Len(root)
				owner, _, rowOK := binds.Get(root)
				if !ok || !rowOK || owner != bodyTerm {
					return scopeScratch{}, errors.New("program/flow/control: invalid Bind scope")
				}
				for cellIndex := 0; cellIndex < count; cellIndex++ {
					cell, cellOK := bindOrder.At(root, cellIndex)
					cellKind, cellBody, cellKey, cellRowOK := cells.Get(cell)
					role, roleOK := bindings.Role(cell)
					host, hostOK := bindings.Host(cell)
					if !cellOK || !cellRowOK || cellKind != authored.CellLocal || cellBody != bodyTerm || cellKey != 0 ||
						!roleOK || !hostOK || role != kind.CellLocal || host != root {
						return scopeScratch{}, errors.New("program/flow/control: invalid Bind Cell scope")
					}
				}
				if count != 0 {
					env = push(env)
				}
			case keyspace.FamilyBody:
				if err := seed(root, env); err != nil {
					return scopeScratch{}, err
				}
			case keyspace.FamilyBranch:
				_, _, whenTrue, whenFalse, ok := branches.Get(root)
				if !ok {
					return scopeScratch{}, errors.New("program/flow/control: Branch view expired")
				}
				if err := seed(whenTrue, env); err != nil {
					return scopeScratch{}, err
				}
				if err := seed(whenFalse, env); err != nil {
					return scopeScratch{}, err
				}
			case keyspace.FamilyLoop:
				_, child, _, _, ok := loops.Get(root)
				if !ok {
					return scopeScratch{}, errors.New("program/flow/control: Loop view expired")
				}
				cellCount, ok := loops.CellCount(root)
				if !ok {
					return scopeScratch{}, errors.New("program/flow/control: Loop Cell view expired")
				}
				loopScope := env
				if cellCount != 0 {
					loopScope = push(env)
				}
				if err := seed(child, loopScope); err != nil {
					return scopeScratch{}, err
				}
			}
		}
		if rootCursor != rootCount {
			return scopeScratch{}, errors.New("program/flow/control: Body root order mismatch")
		}
	}
	if processed != counts[keyspace.FamilyBody] {
		return scopeScratch{}, errors.New("program/flow/control: unseeded Body scope")
	}
	for ordinal := 1; ordinal < len(scratch.labelSeen); ordinal++ {
		if !scratch.labelSeen[ordinal] {
			return scopeScratch{}, errors.New("program/flow/control: Label lacks Source position")
		}
	}
	for ordinal := 1; ordinal < len(scratch.gotoSeen); ordinal++ {
		if !scratch.gotoSeen[ordinal] {
			return scopeScratch{}, errors.New("program/flow/control: Goto lacks Source position")
		}
	}
	for _, family := range [...]keyspace.Family{
		keyspace.FamilyReturn, keyspace.FamilyBreak, keyspace.FamilyGoto,
		keyspace.FamilyBranch, keyspace.FamilyLoop,
	} {
		for ordinal := 1; ordinal < len(scratch.controlSeen[family]); ordinal++ {
			if !scratch.controlSeen[family][ordinal] {
				return scopeScratch{}, errors.New("program/flow/control: authored control lacks Body root")
			}
		}
	}
	return scratch, nil
}

func scopeIntervals(parents []int) ([]uint64, []uint64, error) {
	if len(parents) == 0 || parents[0] != 0 {
		return nil, nil, errors.New("program/flow/control: invalid scope root")
	}
	starts := make([]int, len(parents)+1)
	for child := 1; child < len(parents); child++ {
		parent := parents[child]
		if parent < 0 || parent >= child {
			return nil, nil, errors.New("program/flow/control: invalid scope parent")
		}
		starts[parent+1]++
	}
	for index := 1; index < len(starts); index++ {
		starts[index] += starts[index-1]
	}
	next := append([]int(nil), starts[:len(parents)]...)
	children := make([]int, len(parents)-1)
	for child := 1; child < len(parents); child++ {
		parent := parents[child]
		children[next[parent]] = child
		next[parent]++
	}
	type frame struct{ node, next int }
	stack := []frame{{node: 0, next: starts[0]}}
	pre, post := make([]uint64, len(parents)), make([]uint64, len(parents))
	var timestamp uint64 = 1
	pre[0] = timestamp
	visited := 1
	for len(stack) != 0 {
		top := &stack[len(stack)-1]
		end := starts[top.node+1]
		if top.next >= end {
			timestamp++
			post[top.node] = timestamp
			stack = stack[:len(stack)-1]
			continue
		}
		child := children[top.next]
		top.next++
		visited++
		timestamp++
		pre[child] = timestamp
		stack = append(stack, frame{node: child, next: starts[child]})
	}
	if visited != len(parents) || timestamp != uint64(len(parents))*2 {
		return nil, nil, errors.New("program/flow/control: malformed scope tree")
	}
	return pre, post, nil
}

func scopeAncestor(pre, post []uint64, ancestor, descendant int) bool {
	return ancestor >= 0 && descendant >= 0 && ancestor < len(pre) && descendant < len(pre) &&
		pre[ancestor] != 0 && post[ancestor] != 0 && pre[ancestor] <= pre[descendant] && post[descendant] <= post[ancestor]
}

func controlParent(forest *containment.Result, term, owner keyspace.Term, counts [keyspace.FamilyCount]int) bool {
	if !validTerm(owner, counts, keyspace.FamilyBody) {
		return false
	}
	parent, ok := forest.Parent(term)
	return ok && parent == owner
}

func validTerm(term keyspace.Term, counts [keyspace.FamilyCount]int, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 &&
		uint64(keyspace.TermOrdinal(term)) <= uint64(counts[family])
}

func validLoopKind(loopKind kind.LoopKind) bool {
	return loopKind >= kind.LoopWhile && loopKind <= kind.LoopGenericFor
}

func validLoopCells(loopKind kind.LoopKind, count int) bool {
	switch loopKind {
	case kind.LoopWhile, kind.LoopRepeat:
		return count == 0
	case kind.LoopNumericFor:
		return count == 1
	case kind.LoopGenericFor:
		return count > 0
	default:
		return false
	}
}
