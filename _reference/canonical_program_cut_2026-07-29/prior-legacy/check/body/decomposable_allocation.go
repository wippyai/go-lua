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

const AllocationSiteFactSchemaVersion = 4

// AllocationSiteFact is the solved allocation-site export for one table
// constructor. Decomposable permits scalar replacement when the checker proves
// fixed shape, stack placement, static-only access, no identity demand, no
// capture, and no metatable involvement.
//
// FrameLocalUseProof captures the body-local-use condition. Frame-local
// placement also requires stack placement and DiesBeforeSuspension.
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
	licenses                placement.AllocationSiteLicenses
}

// Licenses returns the canonical allocation-site proof record. The exported
// booleans above are its schema-pinned wire projection.
func (f AllocationSiteFact) Licenses() placement.AllocationSiteLicenses {
	return f.licenses
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

// ForEachDecomposableAllocationSiteFact visits only allocation sites whose
// Decomposable license holds.
func (r *Result) ForEachDecomposableAllocationSiteFact(visit func(AllocationSiteFact) bool) bool {
	if r == nil || visit == nil {
		return false
	}
	visited := false
	r.ForEachAllocationSiteFact(func(fact AllocationSiteFact) bool {
		if !fact.Licenses().State(placement.LicenseDecomposable).Proven() {
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
		if r.tableLiteralSite != "" {
			id = identity.LuaTableLiteralAtSite(r.tableLiteralSite, uint64(exprRef))
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
		useProof
	fact.licenses = placement.NewAllocationSiteLicenses(
		fact.Decomposable, fact.FrameLocalUseProof,
		fact.DiesBeforeSuspension, fact.HasDiesBeforeSuspension,
		fact.Placement, fact.HasPlacement,
	)
	projection := fact.licenses.Projection()
	fact.Decomposable = projection.Decomposable
	fact.FrameLocalUseProof = projection.FrameLocalUseProof
	fact.DiesBeforeSuspension = projection.DiesBeforeSuspension
	fact.HasDiesBeforeSuspension = projection.HasDiesBeforeSuspension
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
	disqualified map[wir.ExpressionID]struct{}
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
		if inst.ExprID == 0 {
			continue
		}
		tracker := newDecomposableUseTracker(r.wir, r.Graph(), inst)
		if !inst.StaticStringKeysComplete || inst.ListSpread || tracker.disqualifiedByUses() {
			analysis.disqualified[inst.ExprID] = struct{}{}
		}
	}
	return analysis
}

type decomposableUseTracker struct {
	body       *wir.Body
	graph      cfg.Graph
	allocation wir.Instruction
	bad        bool
	active     decomposableAliasSet
}

// decomposableAliasSet is a per-program-point may-alias state for one
// allocation site. Joins only add aliases; transfer may kill an alias when its
// destination is overwritten on the current control-flow path.
type decomposableAliasSet struct {
	temps   map[uint32]struct{}
	aliases map[wir.SymbolID]struct{}
}

func newDecomposableAliasSet() decomposableAliasSet {
	return decomposableAliasSet{
		temps:   make(map[uint32]struct{}),
		aliases: make(map[wir.SymbolID]struct{}),
	}
}

func (s decomposableAliasSet) clone() decomposableAliasSet {
	out := newDecomposableAliasSet()
	for temp := range s.temps {
		out.temps[temp] = struct{}{}
	}
	for alias := range s.aliases {
		out.aliases[alias] = struct{}{}
	}
	return out
}

// join adds every alias that may arrive from another predecessor and reports
// whether the input fact grew. It never removes an input fact.
func (s *decomposableAliasSet) join(other decomposableAliasSet) bool {
	changed := false
	if s.temps == nil {
		s.temps = make(map[uint32]struct{})
	}
	for temp := range other.temps {
		if _, ok := s.temps[temp]; !ok {
			s.temps[temp] = struct{}{}
			changed = true
		}
	}
	if s.aliases == nil {
		s.aliases = make(map[wir.SymbolID]struct{})
	}
	for alias := range other.aliases {
		if _, ok := s.aliases[alias]; !ok {
			s.aliases[alias] = struct{}{}
			changed = true
		}
	}
	return changed
}

func newDecomposableUseTracker(body *wir.Body, graph cfg.Graph, allocation wir.Instruction) *decomposableUseTracker {
	return &decomposableUseTracker{
		body:       body,
		graph:      graph,
		allocation: allocation,
	}
}

func (t *decomposableUseTracker) disqualifiedByUses() bool {
	if t == nil || t.body == nil || t.graph == nil || t.bad {
		return true
	}

	// RPO seeds every reachable point once, including points with an empty
	// input fact. A point is re-queued only when a predecessor's monotone
	// may-alias join grows its input.
	queue := append([]cfg.Point(nil), cfg.RPOReadOnly(t.graph)...)
	if len(queue) == 0 {
		return true
	}
	queued := make(map[cfg.Point]bool, len(queue))
	for _, point := range queue {
		queued[point] = true
	}
	inputs := make(map[cfg.Point]decomposableAliasSet, len(queue))

	for len(queue) != 0 && !t.bad {
		point := queue[0]
		queue = queue[1:]
		queued[point] = false

		t.active = inputs[point].clone()
		for _, inst := range t.body.PointInstructions(point) {
			t.classifyInstruction(inst)
			if t.bad {
				return true
			}
		}

		for _, successor := range cfg.SuccessorsReadOnly(t.graph, point) {
			input := inputs[successor]
			if !input.join(t.active) {
				continue
			}
			inputs[successor] = input
			if !queued[successor] {
				queue = append(queue, successor)
				queued[successor] = true
			}
		}
	}
	return t.bad
}

func (t *decomposableUseTracker) classifyInstruction(inst wir.Instruction) bool {
	if inst.Point == t.allocation.Point && inst.ExprID == t.allocation.ExprID && inst.Op == wir.OpMakeTable {
		return t.replaceAliasDestination(inst.Dst)
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
		// The default disqualifies tracked aliases. The exhaustiveness test
		// requires an explicit policy for every WIR opcode.
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
		return t.replaceAliasDestination(dst)
	}
	return t.clearDestinationAlias(dst)
}

func (t *decomposableUseTracker) replaceAliasDestination(dst wir.Operand) bool {
	changed := t.clearDestinationAlias(dst)
	return t.addAliasDestination(dst) || changed
}

func (t *decomposableUseTracker) addAliasDestination(dst wir.Operand) bool {
	switch dst.Kind {
	case wir.OperandTemp:
		if _, ok := t.active.temps[dst.Ref]; ok {
			return false
		}
		t.active.temps[dst.Ref] = struct{}{}
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
		if _, ok := t.active.aliases[p.Symbol]; ok {
			return false
		}
		t.active.aliases[p.Symbol] = struct{}{}
		return true
	default:
		t.bad = true
		return false
	}
}

func (t *decomposableUseTracker) clearDestinationAlias(dst wir.Operand) bool {
	switch dst.Kind {
	case wir.OperandTemp:
		if _, ok := t.active.temps[dst.Ref]; ok {
			delete(t.active.temps, dst.Ref)
			return true
		}
	case wir.OperandPath:
		p := t.body.Path(wir.PathRef(dst.Ref))
		if p.IsEmpty() || len(p.Segments) != 0 {
			return false
		}
		if _, ok := t.active.aliases[p.Symbol]; ok {
			delete(t.active.aliases, p.Symbol)
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
		_, ok := t.active.temps[op.Ref]
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
	_, ok := t.active.aliases[p.Symbol]
	return ok
}

func (t *decomposableUseTracker) localAliasRoot(p path.Path) bool {
	if p.Symbol == 0 {
		return false
	}
	kind, ok := t.body.SymbolKind(p.Symbol)
	return ok && (kind == wir.SymbolLocal || kind == wir.SymbolParam)
}
