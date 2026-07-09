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

const AllocationSiteFactSchemaVersion = 3

// AllocationSiteFact is the solved allocation-site export for one table
// constructor. Decomposable is an optimization license: when true, the table
// allocation may be scalar-replaced because the checker proved fixed shape,
// stack placement, static-only access, no identity demand, no capture, and no
// metatable involvement for this phase.
//
// FrameLocalUseProof is a stricter body-local use proof shared with the
// decomposable scan. It is not a placement license by itself: placement-plan
// projection also requires stack placement and dies-before-suspension. Keeping
// the suspension conjunct makes phase 1 proof-only; relaxing suspension-crossing
// frame locals later should require only dropping that conjunct after the
// runtime confirms stable thread-block safety.
type AllocationSiteFact struct {
	SchemaVersion int
	Point         cfg.Point
	ExpressionID  wir.ExpressionID
	ExprRef       factflow.ExprRef
	Identity      identity.ID
	BirthPoint    cfg.Point
	BirthSpan     SourceSpan
	HasBirthSpan  bool

	Placement    placement.Value
	HasPlacement bool

	Shape       typ.Type
	Fields      []StableShapeField
	StableShape bool

	Decomposable bool

	FrameLocalUseProof      bool
	DiesBeforeSuspension    bool
	HasDiesBeforeSuspension bool
}

// AllocationSiteFacts returns table-allocation facts attached to OpMakeTable
// instructions at point, in WIR instruction order.
func (r *Result) AllocationSiteFacts(point cfg.Point) []AllocationSiteFact {
	if r == nil || r.wir == nil || r.registry == nil {
		return nil
	}
	return r.allocationSiteFacts(point, r.decomposableUseAnalysis(), r.allocationLifetimes())
}

func (r *Result) allocationSiteFacts(point cfg.Point, uses decomposableUseAnalysis, lifetimes map[identity.ID]allocationLifetime) []AllocationSiteFact {
	var out []AllocationSiteFact
	for _, inst := range r.wir.PointInstructions(point) {
		if inst.Op != wir.OpMakeTable {
			continue
		}
		fact, ok := r.allocationSiteFact(inst, uses, lifetimes)
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
	uses := r.decomposableUseAnalysis()
	lifetimes := r.allocationLifetimes()
	for _, point := range r.Graph().RPO() {
		for _, fact := range r.allocationSiteFacts(point, uses, lifetimes) {
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

func (r *Result) allocationSiteFact(inst wir.Instruction, uses decomposableUseAnalysis, lifetimes map[identity.ID]allocationLifetime) (AllocationSiteFact, bool) {
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
	if lifetime, ok := lifetimes[id]; ok {
		fact.BirthPoint = lifetime.BirthPoint
		fact.BirthSpan = lifetime.BirthSpan
		fact.HasBirthSpan = lifetime.HasBirthSpan
		fact.DiesBeforeSuspension = lifetime.DiesBeforeSuspension
		fact.HasDiesBeforeSuspension = true
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
	useProof := !uses.allocationDisqualified(inst)
	fact.FrameLocalUseProof = useProof
	fact.Decomposable = fact.StableShape &&
		fact.HasPlacement &&
		fact.Placement == placement.Stack &&
		inst.StaticStringKeysComplete &&
		!inst.ListSpread &&
		!uses.bodyHasDynamicConstructorKey &&
		useProof
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
		if inst.Op != wir.OpMakeTable {
			continue
		}
		if !inst.StaticStringKeysComplete {
			analysis.bodyHasDynamicConstructorKey = true
		}
		if inst.ExprID == 0 {
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
	case wir.OpNoop, wir.OpEntry, wir.OpExit:
		return false
	case wir.OpAssign, wir.OpClaim:
		return t.classifyTransparentAssign(inst.Dst, inst.A)
	case wir.OpStaticMemberWrite:
		return t.disqualifyIf(t.operandIsRootAlias(inst.A) || t.pathOperandIsExactRootAlias(inst.Dst))
	case wir.OpDynamicIndexWrite:
		return t.disqualifyIf(t.pathOperandIsExactRootAlias(inst.Dst) ||
			t.operandIsRootAlias(inst.A) ||
			t.operandIsRootAlias(inst.B))
	case wir.OpDynamicIndexRead:
		return t.classifyResult(inst.Dst, t.operandIsRootAlias(inst.A) || t.operandIsRootAlias(inst.B))
	case wir.OpMakeTable:
		return t.classifyResult(inst.Dst, t.operandRangeHasRootAlias(inst.List) || t.tableEntriesHaveRootAlias(inst.TableEntries))
	case wir.OpBinOp:
		return t.classifyResult(inst.Dst, t.operandIsRootAlias(inst.A) || t.operandIsRootAlias(inst.B))
	case wir.OpUnOp, wir.OpLogical:
		return t.classifyResult(inst.Dst, t.operandIsRootAlias(inst.A) || t.operandIsRootAlias(inst.B))
	case wir.OpConcat, wir.OpSelect:
		return t.classifyResult(inst.Dst, t.operandRangeHasRootAlias(inst.List))
	case wir.OpCall:
		return t.disqualifyIf(t.operandIsRootAlias(inst.Call.Callee) ||
			t.operandIsRootAlias(inst.Call.Receiver) ||
			t.operandRangeHasRootAlias(inst.List))
	case wir.OpReturn, wir.OpIterate:
		return t.disqualifyIf(t.operandRangeHasRootAlias(inst.List))
	case wir.OpBranch:
		return t.disqualifyIf(t.operandIsRootAlias(inst.A) || t.checkDemandsRootIdentity(t.body.Check(inst.Check)))
	case wir.OpClosure:
		return t.classifyResult(inst.Dst, t.operandRangeHasRootAlias(inst.List))
	default:
		// An instruction kind with no explicit policy above is unclassified.
		// Disqualify rather than assume safety whenever it touches a tracked
		// alias; the exhaustiveness test keeps every wir opcode covered by an
		// explicit case, so this branch only fires for a future opcode added
		// without an accompanying decision here.
		return t.disqualifyIf(t.instructionTouchesTrackedValue(inst))
	}
}

// instructionTouchesTrackedValue reports whether any operand-bearing field of
// inst references a tracked temp or root alias. It is the conservative
// fallback used for instruction kinds without an explicit classification.
func (t *decomposableUseTracker) instructionTouchesTrackedValue(inst wir.Instruction) bool {
	if t.operandIsRootAlias(inst.Dst) || t.operandIsRootAlias(inst.A) || t.operandIsRootAlias(inst.B) {
		return true
	}
	if t.operandRangeHasRootAlias(inst.List) || t.operandRangeHasRootAlias(inst.Results) {
		return true
	}
	if t.tableEntriesHaveRootAlias(inst.TableEntries) {
		return true
	}
	if t.operandIsRootAlias(inst.Call.Callee) || t.operandIsRootAlias(inst.Call.Receiver) {
		return true
	}
	if inst.Check != 0 {
		check := t.body.Check(inst.Check)
		if t.pathIsExactRootAlias(check.Path) || t.pathIsExactRootAlias(check.OtherPath) {
			return true
		}
	}
	return false
}

func (t *decomposableUseTracker) disqualifyIf(disqualified bool) bool {
	if disqualified {
		t.bad = true
	}
	return false
}

func (t *decomposableUseTracker) classifyResult(dst wir.Operand, disqualified bool) bool {
	if disqualified {
		t.bad = true
		return false
	}
	return t.clearDestinationAlias(dst)
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
		if alias.EqualIgnoringVersion(p) {
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
		if alias.EqualIgnoringVersion(p) {
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
