package staticcheck

import (
	"errors"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// contextTree is seal-local lexical scratch. A point is one Body gap and
// points form an immutable outer chain; dense introduction points plus Euler
// intervals keep a wide/deep lexical forest linear without a second owner graph.
type contextTree struct {
	points     []contextPoint
	bodies     []contextBody
	cellPoint  []int
	cellScope  []int
	generic    []int
	paramFirst []int
	paramNext  []int
	tin        []int
	tout       []int
}

type contextPoint struct {
	body  keyspace.Term
	gap   int
	outer int
}

type contextBody struct {
	body       keyspace.Term
	gapStart   int
	gapCount   int
	base       int
	anchor     keyspace.Term
	anchorPos  source.Position
	anchorKind contextAnchorKind
}

type contextAnchorKind uint8

const (
	contextAnchorEntry contextAnchorKind = iota + 1
	contextAnchorOrdinary
	contextAnchorFunction
	contextAnchorBranch
	contextAnchorLoop
)

// buildContext builds one lexical point tree and drops all relation inputs on
// return. Typed child Bodies inherit the construct's Position. Function
// headers and loop cells are installed at child gap zero; ordinary Bind cells
// are installed in the gap after Bind.
func buildContext(
	sourceView source.View,
	flowView authored.View,
	staticView staticquery.View,
	bodies *body.Result,
	bindings binding.Result,
	entry keyspace.Term,
) (*contextTree, error) {
	bodyCount := bodies.BodyCount()
	cellCount := flowView.Storage().Cells().Count()
	tree := &contextTree{
		points:    []contextPoint{{}},
		bodies:    make([]contextBody, bodyCount+1),
		cellPoint: make([]int, cellCount+1),
		cellScope: make([]int, cellCount+1),
		generic:   make([]int, staticView.Declarations().TypeParams().Count()+1),
	}
	if keyspace.TermFamily(entry) != keyspace.FamilyBody || keyspace.TermOrdinal(entry) == 0 || int(keyspace.TermOrdinal(entry)) > bodyCount {
		return nil, errors.New("program/flow/staticcheck: invalid context Entry Body")
	}
	bindFirst, bindNext, err := contextBindRanges(flowView, bodyCount)
	if err != nil {
		return nil, err
	}
	paramFirst, paramNext, err := contextParamRanges(staticView, flowView.Functions().Count())
	if err != nil {
		return nil, err
	}
	tree.paramFirst = paramFirst
	tree.paramNext = paramNext
	if err := contextAnchors(sourceView, flowView, bodies, entry, tree); err != nil {
		return nil, err
	}
	firstChild, nextChild, err := contextChildren(bodies, entry)
	if err != nil {
		return nil, err
	}
	stack := make([]int, 0)
	stack = append(stack, int(keyspace.TermOrdinal(entry)))
	built := 0
	for len(stack) != 0 {
		bodyOrdinal := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if tree.bodies[bodyOrdinal].gapCount != 0 {
			return nil, errors.New("program/flow/staticcheck: duplicate context Body")
		}
		if err := buildBodyContext(sourceView, flowView, staticView, bodies, bindings, tree, bodyOrdinal, entry, bindFirst, bindNext, paramFirst, paramNext); err != nil {
			return nil, err
		}
		built++
		for child := firstChild[bodyOrdinal]; child != 0; child = nextChild[child] {
			stack = append(stack, child)
		}
	}
	if built != bodyCount {
		return nil, errors.New("program/flow/staticcheck: Body context tree is disconnected")
	}
	if err := contextCellsComplete(flowView, bindings, tree); err != nil {
		return nil, err
	}
	if err := contextIntervals(tree); err != nil {
		return nil, err
	}
	if err := contextValidateFunctionGenerics(flowView, tree); err != nil {
		return nil, err
	}
	return tree, nil
}

func contextAnchors(
	sourceView source.View,
	flowView authored.View,
	bodies *body.Result,
	entry keyspace.Term,
	tree *contextTree,
) error {
	bodyCount := bodies.BodyCount()
	entryOrdinal := int(keyspace.TermOrdinal(entry))
	tree.bodies[entryOrdinal] = contextBody{body: entry, anchorKind: contextAnchorEntry}
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		if bodyTerm == entry {
			continue
		}
		// Position is a closure projection, so a typed child Body may carry
		// the coordinate of an enclosing direct Body root.  Only an exact
		// Root(body) identity denotes an ordinary Body anchor; construct
		// children receive their Function/Branch/Loop anchor below.
		if root, ok := sourceView.Index().Root(bodyTerm); ok && root == bodyTerm {
			position, ok := contextPosition(sourceView, bodyTerm)
			if !ok || position.Root != bodyTerm {
				return errors.New("program/flow/staticcheck: ordinary Body Position is malformed")
			}
			if err := setContextAnchor(tree, bodyTerm, bodyTerm, contextAnchorOrdinary, position); err != nil {
				return err
			}
		}
	}
	functions := flowView.Functions()
	for ordinal := 1; ordinal <= functions.Count(); ordinal++ {
		function := keyspace.MakeTerm(keyspace.FamilyFunction, uint32(ordinal))
		owner, child, _, ok := functions.Get(function)
		if !ok {
			return errors.New("program/flow/staticcheck: Function row is unavailable")
		}
		position, positionOK := contextPosition(sourceView, function)
		if positionOK {
			if position.Body != owner {
				return errors.New("program/flow/staticcheck: Function Position disagrees with owner")
			}
		} else if _, _, _, present := sourceView.Index().Position(function); present {
			return errors.New("program/flow/staticcheck: Function Position is malformed")
		}
		if err := setContextAnchor(tree, child, function, contextAnchorFunction, position); err != nil {
			return err
		}
	}
	branches := flowView.Control().Branches()
	for ordinal := 1; ordinal <= branches.Count(); ordinal++ {
		branch := keyspace.MakeTerm(keyspace.FamilyBranch, uint32(ordinal))
		owner, _, whenTrue, whenFalse, ok := branches.Get(branch)
		if !ok {
			return errors.New("program/flow/staticcheck: Branch row is unavailable")
		}
		position, positionOK := contextPosition(sourceView, branch)
		if !positionOK || position.Body != owner {
			return errors.New("program/flow/staticcheck: Branch Position disagrees with owner")
		}
		if err := setContextAnchor(tree, whenTrue, branch, contextAnchorBranch, position); err != nil {
			return err
		}
		if err := setContextAnchor(tree, whenFalse, branch, contextAnchorBranch, position); err != nil {
			return err
		}
	}
	loops := flowView.Control().Loops()
	for ordinal := 1; ordinal <= loops.Count(); ordinal++ {
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, uint32(ordinal))
		owner, child, loopKind, _, ok := loops.Get(loop)
		if !ok || loopKind < kind.LoopWhile || loopKind > kind.LoopGenericFor {
			return errors.New("program/flow/staticcheck: Loop row is unavailable")
		}
		position, positionOK := contextPosition(sourceView, loop)
		if !positionOK || position.Body != owner {
			return errors.New("program/flow/staticcheck: Loop Position disagrees with owner")
		}
		if err := setContextAnchor(tree, child, loop, contextAnchorLoop, position); err != nil {
			return err
		}
	}
	return nil
}

func setContextAnchor(tree *contextTree, child, anchor keyspace.Term, anchorKind contextAnchorKind, position source.Position) error {
	if tree == nil || keyspace.TermFamily(child) != keyspace.FamilyBody || keyspace.TermOrdinal(child) == 0 || int(keyspace.TermOrdinal(child)) >= len(tree.bodies) {
		return errors.New("program/flow/staticcheck: invalid child Body anchor")
	}
	node := &tree.bodies[keyspace.TermOrdinal(child)]
	if node.anchor != 0 {
		return errors.New("program/flow/staticcheck: duplicate Body lexical anchor")
	}
	node.anchor = anchor
	node.anchorPos = position
	node.anchorKind = anchorKind
	return nil
}

func contextChildren(bodies *body.Result, entry keyspace.Term) ([]int, []int, error) {
	count := bodies.BodyCount()
	first := make([]int, count+1)
	next := make([]int, count+1)
	for ordinal := 1; ordinal <= count; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		parent, hasParent := bodies.Parent(term)
		if term == entry {
			if hasParent || parent != 0 {
				return nil, nil, errors.New("program/flow/staticcheck: Entry Body parent mismatch")
			}
			continue
		}
		if !hasParent || keyspace.TermFamily(parent) != keyspace.FamilyBody || keyspace.TermOrdinal(parent) == 0 || int(keyspace.TermOrdinal(parent)) > count {
			return nil, nil, errors.New("program/flow/staticcheck: Body parent is unavailable")
		}
		parentOrdinal := int(keyspace.TermOrdinal(parent))
		next[ordinal] = first[parentOrdinal]
		first[parentOrdinal] = ordinal
	}
	return first, next, nil
}

func buildBodyContext(
	sourceView source.View,
	flowView authored.View,
	staticView staticquery.View,
	bodies *body.Result,
	bindings binding.Result,
	tree *contextTree,
	bodyOrdinal int,
	entry keyspace.Term,
	bindFirst []int,
	bindNext []int,
	paramFirst []int,
	paramNext []int,
) error {
	bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, uint32(bodyOrdinal))
	node := &tree.bodies[bodyOrdinal]
	rootCount, ok := bodies.RootCount(bodyTerm)
	if !ok || rootCount < 0 {
		return errors.New("program/flow/staticcheck: Body gaps are unavailable")
	}
	base := 0
	if bodyTerm != entry {
		parent, hasParent := bodies.Parent(bodyTerm)
		if !hasParent || node.anchor == 0 || (node.anchorPos.Root != 0 && node.anchorPos.Body != parent) {
			return errors.New("program/flow/staticcheck: Body anchor disagrees with parent")
		}
		anchorGap := node.anchorPos.Cursor
		if node.anchorPos.Root == 0 {
			if node.anchorKind != contextAnchorFunction {
				return errors.New("program/flow/staticcheck: Body anchor Position is unavailable")
			}
			anchorGap = 0
		}
		parentPoint, pointOK := tree.pointAt(parent, anchorGap)
		if !pointOK {
			return errors.New("program/flow/staticcheck: Body anchor gap is unavailable")
		}
		base = parentPoint
	}
	node.body = bodyTerm
	node.base = base
	node.gapStart = len(tree.points)
	node.gapCount = rootCount + 1
	for gap := 0; gap <= rootCount; gap++ {
		outer := base
		if gap > 0 {
			outer = node.gapStart + gap - 1
		}
		tree.points = append(tree.points, contextPoint{body: bodyTerm, gap: gap, outer: outer})
	}
	if bodyTerm == entry {
		if chunk, chunkOK := bindings.ChunkVararg(); chunkOK {
			if err := tree.addCell(node.gapStart, chunk); err != nil {
				return err
			}
		}
	}
	switch node.anchorKind {
	case contextAnchorFunction:
		if err := addFunctionHeader(sourceView, flowView, bindings, tree, node.anchor, node.base, node.gapStart, paramFirst, paramNext); err != nil {
			return err
		}
	case contextAnchorLoop:
		if err := addLoopCells(flowView, bindings, tree, node.anchor, node.gapStart); err != nil {
			return err
		}
	case contextAnchorEntry, contextAnchorOrdinary, contextAnchorBranch:
	default:
		return errors.New("program/flow/staticcheck: invalid Body anchor kind")
	}
	if err := addBindCells(sourceView, flowView, bindings, tree, bodyTerm, node.gapStart, rootCount, bindFirst, bindNext); err != nil {
		return err
	}
	return nil
}

func (tree *contextTree) reanchorFunction(functionBody, function keyspace.Term, point int) error {
	if tree == nil || point <= 0 || point >= len(tree.points) || keyspace.TermFamily(function) != keyspace.FamilyFunction || keyspace.TermFamily(functionBody) != keyspace.FamilyBody {
		return errors.New("program/flow/staticcheck: invalid Function seed anchor")
	}
	bodyOrdinal := int(keyspace.TermOrdinal(functionBody))
	if bodyOrdinal <= 0 || bodyOrdinal >= len(tree.bodies) {
		return errors.New("program/flow/staticcheck: Function seed Body is unavailable")
	}
	node := &tree.bodies[bodyOrdinal]
	if node.anchor != function || node.anchorKind != contextAnchorFunction || node.gapStart <= 0 || node.gapStart >= len(tree.points) {
		return errors.New("program/flow/staticcheck: Function seed anchor is malformed")
	}
	node.base = point
	tree.points[node.gapStart].outer = point
	functionOrdinal := int(keyspace.TermOrdinal(function))
	if functionOrdinal <= 0 || functionOrdinal >= len(tree.paramFirst) {
		return errors.New("program/flow/staticcheck: Function seed generic range is unavailable")
	}
	for paramOrdinal := tree.paramFirst[functionOrdinal]; paramOrdinal != 0; paramOrdinal = tree.paramNext[paramOrdinal] {
		if paramOrdinal >= len(tree.generic) {
			return errors.New("program/flow/staticcheck: Function seed generic parameter is unavailable")
		}
		tree.generic[paramOrdinal] = point
	}
	return nil
}

func addBindCells(
	sourceView source.View,
	flowView authored.View,
	bindings binding.Result,
	tree *contextTree,
	bodyTerm keyspace.Term,
	gapStart int,
	rootCount int,
	bindFirst []int,
	bindNext []int,
) error {
	binds := flowView.Storage().Binds()
	order := sourceView.Binds()
	bodyOrdinal := int(keyspace.TermOrdinal(bodyTerm))
	if bodyOrdinal <= 0 || bodyOrdinal >= len(bindFirst) {
		return errors.New("program/flow/staticcheck: Bind owner range is unavailable")
	}
	for ordinal := bindFirst[bodyOrdinal]; ordinal != 0; ordinal = bindNext[ordinal] {
		bind := keyspace.MakeTerm(keyspace.FamilyBind, uint32(ordinal))
		owner, _, ok := binds.Get(bind)
		if !ok {
			return errors.New("program/flow/staticcheck: Bind row is unavailable")
		}
		if owner != bodyTerm {
			continue
		}
		position, positionOK := contextPosition(sourceView, bind)
		if !positionOK || position.Body != bodyTerm || int(position.Cursor) >= rootCount {
			return errors.New("program/flow/staticcheck: Bind Position is unavailable")
		}
		gap := int(position.Cursor) + 1
		if gap > rootCount {
			return errors.New("program/flow/staticcheck: Bind introduction gap is unavailable")
		}
		length, lengthOK := order.Len(bind)
		if !lengthOK || length <= 0 {
			return errors.New("program/flow/staticcheck: Bind Cell order is unavailable")
		}
		for index := 0; index < length; index++ {
			cell, cellOK := order.At(bind, index)
			if !cellOK {
				return errors.New("program/flow/staticcheck: Bind Cell order is unavailable")
			}
			role, roleOK := bindings.Role(cell)
			host, hostOK := bindings.Host(cell)
			if !roleOK || !hostOK || role != kind.CellLocal || host != bind {
				return errors.New("program/flow/staticcheck: Bind Cell role is unavailable")
			}
			cellOrdinal := int(keyspace.TermOrdinal(cell))
			if cellOrdinal <= 0 || cellOrdinal >= len(tree.cellScope) || tree.cellScope[cellOrdinal] != 0 {
				return errors.New("program/flow/staticcheck: duplicate Bind Cell scope point")
			}
			tree.cellScope[cellOrdinal] = gapStart + int(position.Cursor)
			if err := tree.addCell(gapStart+gap, cell); err != nil {
				return err
			}
		}
	}
	return nil
}

func addFunctionHeader(
	sourceView source.View,
	flowView authored.View,
	bindings binding.Result,
	tree *contextTree,
	function keyspace.Term,
	genericPoint int,
	headerPoint int,
	paramFirst []int,
	paramNext []int,
) error {
	functions := flowView.Functions()
	_, _, vararg, ok := functions.Get(function)
	if !ok {
		return errors.New("program/flow/staticcheck: Function header is unavailable")
	}
	formalOrder := sourceView.Formals()
	formalCount, countOK := formalOrder.Len(function)
	if !countOK || formalCount < 0 {
		return errors.New("program/flow/staticcheck: Function formal order is unavailable")
	}
	for index := 0; index < formalCount; index++ {
		cell, cellOK := formalOrder.At(function, index)
		if !cellOK {
			return errors.New("program/flow/staticcheck: Function formal order is unavailable")
		}
		role, roleOK := bindings.Role(cell)
		host, hostOK := bindings.Host(cell)
		if !roleOK || !hostOK || role != kind.CellFormal || host != function {
			return errors.New("program/flow/staticcheck: Function formal role is unavailable")
		}
		if err := tree.addCell(headerPoint, cell); err != nil {
			return err
		}
	}
	if vararg != 0 {
		role, roleOK := bindings.Role(vararg)
		host, hostOK := bindings.Host(vararg)
		if !roleOK || !hostOK || role != kind.CellFunctionVararg || host != function {
			return errors.New("program/flow/staticcheck: Function vararg role is unavailable")
		}
		if err := tree.addCell(headerPoint, vararg); err != nil {
			return err
		}
	}
	captureCount, captureOK := functions.CaptureCount(function)
	if !captureOK || captureCount < 0 {
		return errors.New("program/flow/staticcheck: Function capture range is unavailable")
	}
	for index := 0; index < captureCount; index++ {
		inner, _, ok := functions.CaptureAt(function, index)
		if !ok {
			return errors.New("program/flow/staticcheck: Function capture range is unavailable")
		}
		role, roleOK := bindings.Role(inner)
		host, hostOK := bindings.Host(inner)
		if !roleOK || !hostOK || role != kind.CellCapture || host != function {
			return errors.New("program/flow/staticcheck: Function capture role is unavailable")
		}
		if err := tree.addCell(headerPoint, inner); err != nil {
			return err
		}
	}
	functionOrdinal := int(keyspace.TermOrdinal(function))
	if functionOrdinal <= 0 || functionOrdinal >= len(paramFirst) {
		return errors.New("program/flow/staticcheck: Function generic range is unavailable")
	}
	for ordinal := paramFirst[functionOrdinal]; ordinal != 0; ordinal = paramNext[ordinal] {
		param := keyspace.MakeTerm(keyspace.FamilyTypeParam, uint32(ordinal))
		if err := tree.addParam(genericPoint, param); err != nil {
			return err
		}
	}
	return nil
}

func addLoopCells(
	flowView authored.View,
	bindings binding.Result,
	tree *contextTree,
	loop keyspace.Term,
	point int,
) error {
	loops := flowView.Control().Loops()
	count, ok := loops.CellCount(loop)
	if !ok || count < 0 {
		return errors.New("program/flow/staticcheck: Loop Cell range is unavailable")
	}
	for index := 0; index < count; index++ {
		cell, cellOK := loops.CellAt(loop, index)
		if !cellOK {
			return errors.New("program/flow/staticcheck: Loop Cell range is unavailable")
		}
		role, roleOK := bindings.Role(cell)
		host, hostOK := bindings.Host(cell)
		if !roleOK || !hostOK || role != kind.CellLoop || host != loop {
			return errors.New("program/flow/staticcheck: Loop Cell role is unavailable")
		}
		if err := tree.addCell(point, cell); err != nil {
			return err
		}
	}
	return nil
}

func contextCellsComplete(flowView authored.View, bindings binding.Result, tree *contextTree) error {
	cells := flowView.Storage().Cells()
	for ordinal := 1; ordinal <= cells.Count(); ordinal++ {
		cell := keyspace.MakeTerm(keyspace.FamilyCell, uint32(ordinal))
		role, ok := bindings.Role(cell)
		if !ok {
			return errors.New("program/flow/staticcheck: Cell role is unavailable")
		}
		if role == kind.CellGlobal {
			continue
		}
		if tree.cellPoint[ordinal] == 0 {
			return errors.New("program/flow/staticcheck: lexical Cell has no introduction point")
		}
		if tree.cellScope[ordinal] == 0 {
			tree.cellScope[ordinal] = tree.cellPoint[ordinal]
		}
	}
	return nil
}
