package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

const AllocationSiteFactSchemaVersion = 1

// AllocationSiteFact is the solved allocation-site export for one table
// constructor. Decomposable is an optimization license: when true, the table
// allocation may be scalar-replaced because the checker proved fixed shape,
// stack placement, static-only access, no identity demand, no capture, and no
// metatable involvement for this phase.
type AllocationSiteFact struct {
	SchemaVersion int
	Point         cfg.Point
	ExpressionID  wir.ExpressionID
	ExprRef       factflow.ExprRef
	Identity      identity.ID

	Placement    placement.Value
	HasPlacement bool

	Shape       typ.Type
	Fields      []StableShapeField
	StableShape bool

	Decomposable bool
}

// AllocationSiteFacts returns table-allocation facts attached to OpMakeTable
// instructions at point, in WIR instruction order.
func (r *Result) AllocationSiteFacts(point cfg.Point) []AllocationSiteFact {
	if r == nil || r.wir == nil || r.registry == nil {
		return nil
	}
	useAnalysis := r.decomposableUseAnalysis()
	var out []AllocationSiteFact
	for _, inst := range r.wir.PointInstructions(point) {
		if inst.Op != wir.OpMakeTable {
			continue
		}
		fact, ok := r.allocationSiteFact(inst, useAnalysis)
		if ok {
			out = append(out, fact)
		}
	}
	return out
}

// ForEachAllocationSiteFact visits all table-allocation facts in deterministic
// RPO order. Returning false stops iteration.
func (r *Result) ForEachAllocationSiteFact(visit func(AllocationSiteFact) bool) bool {
	if r == nil || visit == nil || r.Graph() == nil {
		return false
	}
	visited := false
	for _, point := range r.Graph().RPO() {
		for _, fact := range r.AllocationSiteFacts(point) {
			visited = true
			if !visit(fact) {
				return true
			}
		}
	}
	return visited
}

// ForEachDecomposableAllocationFact visits only allocation sites whose
// Decomposable license holds.
func (r *Result) ForEachDecomposableAllocationFact(visit func(AllocationSiteFact) bool) bool {
	if r == nil || visit == nil {
		return false
	}
	visited := false
	r.ForEachAllocationSiteFact(func(fact AllocationSiteFact) bool {
		if !fact.Decomposable {
			return true
		}
		visited = true
		return visit(fact)
	})
	return visited
}

func (r *Result) allocationSiteFact(inst wir.Instruction, uses decomposableUseAnalysis) (AllocationSiteFact, bool) {
	exprRef, ok := r.tableConstructorExprRef(inst)
	if !ok {
		return AllocationSiteFact{}, false
	}
	value, ok := r.ExpressionValueRef(exprRef)
	if !ok {
		return AllocationSiteFact{}, false
	}
	id, ok := identityvalue.ExactID(r.registry, value)
	if !ok {
		if graph := r.Graph(); graph != nil {
			id = identity.LuaTableLiteral(graph.ID(), uint64(exprRef))
			ok = id != (identity.ID{})
		}
	}
	if !ok {
		return AllocationSiteFact{}, false
	}

	fact := AllocationSiteFact{
		SchemaVersion: AllocationSiteFactSchemaVersion,
		Point:         inst.Point,
		ExpressionID:  inst.ExprID,
		ExprRef:       exprRef,
		Identity:      id,
	}
	if exit, ok := r.ExitState(); ok {
		fact.Placement = exit.ReadPlacement(id)
		fact.HasPlacement = !fact.Placement.IsBottom()
	}
	if shape, ok := r.StableShapeForValueAtBoundary(inst.Point, value); ok {
		fact.Shape = shape.Shape
		fact.Fields = append([]StableShapeField(nil), shape.Fields...)
		fact.StableShape = true
	}
	fact.Decomposable = fact.StableShape &&
		fact.HasPlacement &&
		fact.Placement == placement.Stack &&
		inst.StaticStringKeysComplete &&
		!inst.ListSpread &&
		!uses.bodyHasDynamicConstructorKey &&
		!uses.allocationDisqualified(inst)
	return fact, true
}

func (r *Result) tableConstructorExprRef(inst wir.Instruction) (factflow.ExprRef, bool) {
	if inst.ExprID == 0 {
		return 0, false
	}
	var out factflow.ExprRef
	r.facts.ForEachObjectLiteral(func(ref factflow.ExprRef, literal factflow.ObjectLiteralView) bool {
		id, ok := literal.ExpressionID()
		if ok && id == uint64(inst.ExprID) {
			out = ref
			return false
		}
		return true
	})
	return out, out != 0
}

type decomposableUseAnalysis struct {
	bodyHasDynamicConstructorKey bool
	disqualified                 map[wir.ExpressionID]struct{}
}

func (a decomposableUseAnalysis) allocationDisqualified(inst wir.Instruction) bool {
	if inst.ExprID == 0 {
		return true
	}
	_, bad := a.disqualified[inst.ExprID]
	return bad
}

func (r *Result) decomposableUseAnalysis() decomposableUseAnalysis {
	analysis := decomposableUseAnalysis{disqualified: make(map[wir.ExpressionID]struct{})}
	if r == nil || r.wir == nil || r.Graph() == nil {
		return analysis
	}
	for i := 0; i < r.wir.Len(); i++ {
		inst := r.wir.Instr(i)
		if inst.Op == wir.OpMakeTable && !inst.StaticStringKeysComplete {
			analysis.bodyHasDynamicConstructorKey = true
		}
	}
	for i := 0; i < r.wir.Len(); i++ {
		inst := r.wir.Instr(i)
		if inst.Op != wir.OpMakeTable || inst.ExprID == 0 {
			continue
		}
		tracker := newDecomposableUseTracker(r.wir, inst)
		if !inst.StaticStringKeysComplete || inst.ListSpread || tracker.disqualifiedByUses() {
			analysis.disqualified[inst.ExprID] = struct{}{}
		}
	}
	return analysis
}

type decomposableUseTracker struct {
	body    *wir.Body
	alloc   wir.Instruction
	bad     bool
	temps   map[uint32]struct{}
	aliases []path.Path
}

func newDecomposableUseTracker(body *wir.Body, alloc wir.Instruction) *decomposableUseTracker {
	t := &decomposableUseTracker{
		body:  body,
		alloc: alloc,
		temps: make(map[uint32]struct{}),
	}
	t.addAliasDestination(alloc.Dst)
	return t
}

func (t *decomposableUseTracker) disqualifiedByUses() bool {
	if t == nil || t.body == nil || t.bad {
		return true
	}
	changed := true
	for changed && !t.bad {
		changed = false
		for i := 0; i < t.body.Len(); i++ {
			if t.classifyInstruction(t.body.Instr(i)) {
				changed = true
			}
			if t.bad {
				return true
			}
		}
	}
	return t.bad
}

func (t *decomposableUseTracker) classifyInstruction(inst wir.Instruction) bool {
	if inst.Point == t.alloc.Point && inst.ExprID == t.alloc.ExprID && inst.Op == wir.OpMakeTable {
		return false
	}
	switch inst.Op {
	case wir.OpAssign, wir.OpClaim:
		return t.classifyTransparentAssign(inst.Dst, inst.A)
	case wir.OpStaticMemberWrite:
		if t.operandIsRootAlias(inst.A) {
			t.bad = true
			return false
		}
		if t.pathOperandIsExactRootAlias(inst.Dst) {
			t.bad = true
			return false
		}
		return false
	case wir.OpDynamicIndexWrite:
		if t.pathOperandIsExactRootAlias(inst.Dst) ||
			t.operandIsRootAlias(inst.A) ||
			t.operandIsRootAlias(inst.B) {
			t.bad = true
		}
		return false
	case wir.OpDynamicIndexRead:
		if t.operandIsRootAlias(inst.A) || t.operandIsRootAlias(inst.B) {
			t.bad = true
			return false
		}
		return t.clearDestinationAlias(inst.Dst)
	case wir.OpMakeTable:
		if t.operandRangeHasRootAlias(inst.List) || t.tableEntriesHaveRootAlias(inst.TableEntries) {
			t.bad = true
			return false
		}
		return t.clearDestinationAlias(inst.Dst)
	case wir.OpBinOp:
		if t.operandIsRootAlias(inst.A) || t.operandIsRootAlias(inst.B) {
			t.bad = true
			return false
		}
		return t.clearDestinationAlias(inst.Dst)
	case wir.OpUnOp, wir.OpLogical:
		if t.operandIsRootAlias(inst.A) || t.operandIsRootAlias(inst.B) {
			t.bad = true
			return false
		}
		return t.clearDestinationAlias(inst.Dst)
	case wir.OpConcat, wir.OpSelect:
		if t.operandRangeHasRootAlias(inst.List) {
			t.bad = true
			return false
		}
		return t.clearDestinationAlias(inst.Dst)
	case wir.OpCall:
		if t.operandIsRootAlias(inst.Call.Callee) ||
			t.operandIsRootAlias(inst.Call.Receiver) ||
			t.operandRangeHasRootAlias(inst.List) {
			t.bad = true
		}
		return false
	case wir.OpReturn, wir.OpIterate:
		if t.operandRangeHasRootAlias(inst.List) {
			t.bad = true
		}
		return false
	case wir.OpBranch:
		if t.operandIsRootAlias(inst.A) || t.checkDemandsRootIdentity(t.body.Check(inst.Check)) {
			t.bad = true
		}
		return false
	case wir.OpClosure:
		if t.operandRangeHasRootAlias(inst.List) {
			t.bad = true
			return false
		}
		return t.clearDestinationAlias(inst.Dst)
	default:
		return false
	}
}

func (t *decomposableUseTracker) classifyTransparentAssign(dst, src wir.Operand) bool {
	if t.operandIsRootAlias(src) {
		return t.addAliasDestination(dst)
	}
	return t.clearDestinationAlias(dst)
}

func (t *decomposableUseTracker) addAliasDestination(dst wir.Operand) bool {
	switch dst.Kind {
	case wir.OperandTemp:
		if _, ok := t.temps[dst.Ref]; ok {
			return false
		}
		t.temps[dst.Ref] = struct{}{}
		return true
	case wir.OperandPath:
		p := t.body.Path(wir.PathRef(dst.Ref))
		if p.IsEmpty() {
			t.bad = true
			return false
		}
		if len(p.Segments) != 0 || !t.localAliasRoot(p) {
			t.bad = true
			return false
		}
		if t.hasAliasPath(p) {
			return false
		}
		t.aliases = append(t.aliases, p.Clone())
		return true
	default:
		t.bad = true
		return false
	}
}

func (t *decomposableUseTracker) clearDestinationAlias(dst wir.Operand) bool {
	if dst.Kind != wir.OperandPath {
		return false
	}
	p := t.body.Path(wir.PathRef(dst.Ref))
	if p.IsEmpty() || len(p.Segments) != 0 {
		return false
	}
	for i, alias := range t.aliases {
		if pathEqualIgnoringVersion(alias, p) {
			t.aliases = append(t.aliases[:i], t.aliases[i+1:]...)
			return true
		}
	}
	return false
}

func (t *decomposableUseTracker) operandRangeHasRootAlias(r wir.OperandRange) bool {
	for _, op := range t.body.Operands(r) {
		if t.operandIsRootAlias(op) {
			return true
		}
	}
	return false
}

func (t *decomposableUseTracker) tableEntriesHaveRootAlias(r wir.TableEntryRange) bool {
	for _, entry := range t.body.TableEntries(r) {
		if t.operandIsRootAlias(entry.Value) {
			return true
		}
	}
	return false
}

func (t *decomposableUseTracker) operandIsRootAlias(op wir.Operand) bool {
	switch op.Kind {
	case wir.OperandTemp:
		_, ok := t.temps[op.Ref]
		return ok
	case wir.OperandPath:
		return t.pathOperandIsExactRootAlias(op)
	default:
		return false
	}
}

func (t *decomposableUseTracker) pathOperandIsExactRootAlias(op wir.Operand) bool {
	if op.Kind != wir.OperandPath {
		return false
	}
	p := t.body.Path(wir.PathRef(op.Ref))
	if p.IsEmpty() || len(p.Segments) != 0 {
		return false
	}
	return t.hasAliasPath(p)
}

func (t *decomposableUseTracker) checkDemandsRootIdentity(check wir.Check) bool {
	switch check.Kind {
	case wir.CheckPathEqual, wir.CheckPathNot:
		return t.pathIsExactRootAlias(check.Path) || t.pathIsExactRootAlias(check.OtherPath)
	case wir.CheckLenGe, wir.CheckIndexInRange:
		return t.pathIsExactRootAlias(check.Path)
	default:
		return false
	}
}

func (t *decomposableUseTracker) pathIsExactRootAlias(p path.Path) bool {
	if p.IsEmpty() || len(p.Segments) != 0 {
		return false
	}
	return t.hasAliasPath(p)
}

func (t *decomposableUseTracker) hasAliasPath(p path.Path) bool {
	for _, alias := range t.aliases {
		if pathEqualIgnoringVersion(alias, p) {
			return true
		}
	}
	return false
}

func (t *decomposableUseTracker) localAliasRoot(p path.Path) bool {
	if p.Symbol == 0 {
		return false
	}
	kind, ok := t.body.SymbolKind(p.Symbol)
	return ok && (kind == wir.SymbolLocal || kind == wir.SymbolParam)
}

func pathEqualIgnoringVersion(a, b path.Path) bool {
	if !a.SameRootIgnoringVersion(b) || len(a.Segments) != len(b.Segments) {
		return false
	}
	for i := range a.Segments {
		if a.Segments[i] != b.Segments[i] {
			return false
		}
	}
	return true
}
