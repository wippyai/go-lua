package wir

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Body holds the lowered instruction stream for one function together with its
// operand intern pools. Instructions are attached per CFG point: points maps a
// cfg.Point to a [start, start+len) window over the flat instrs slice. The CFG
// itself owns topology; a Body never duplicates edges.
//
// Intern pools are 1-based: index 0 of every pool is a reserved sentinel so a
// zero ref (PathRef/ConstRef/TypeRef/CheckRef) is unambiguously "none". Refs are
// dense per Body, which is what a later codegen backend maps to VM slots.
type Body struct {
	Name string

	instrs []Instruction
	calls  []uint32
	points []pointRange // indexed by cfg.Point

	paths  []path.Path
	consts []Const
	types  []Type
	checks []Check
	protos []FuncProto

	declaredReturns []TypeRef
	// declaredReturnSpans anchors each declared return slot to its authored
	// annotation. It is source metadata only and never participates in typing.
	declaredReturnSpans []Span

	operandPool       []Operand
	typeRefPool       []TypeRef
	tableEntries      []TableEntry
	segments          []segment.Segment
	callArgMeta       []CallArgumentMeta
	returnMeta        []ReturnValueMeta
	concatMeta        []ConcatOperandMeta
	rootTypes         []RootType
	impliedChecks     []ImpliedCheck
	armGroups         []ImpliedCheckRange
	branchDiffs       []BranchDiffConstraint
	callTargets       map[callResultTargetKey]CallResultTarget
	evaluations       map[cfg.Point]ExpressionEvaluation
	ifChains          map[uint32]IfChainDescriptor
	structuralRegions map[StructuralExpressionOwner]StructuralExpressionRegion
	symbols           map[SymbolID]SymbolInfo
	globalSymbols     map[string]SymbolID

	// lexicalPath is the source-order path from the containing chunk to this
	// body.  It is structural metadata only: front derives its content-addressed
	// BodyID from this path and the root source identity.
	lexicalPath []uint32
	boundary    BodyBoundary

	pathIndex     map[path.PathKey]PathRef
	constIndex    map[Const]ConstRef
	typeIndex     map[uint64]TypeRef
	rootTypeIndex map[path.PathKey]int
}

// FuncProto is a nested function lowered as its own Body and CFG. A parent
// OpClosure references it by FuncRef; the child owns its own topology exactly
// like a top-level function.
type FunctionSymbolID uint64

type FuncProto struct {
	Name  string
	Body  *Body
	Graph *cfg.CFG
	// LexicalPath is the stable source-order prototype path.  It deliberately
	// does not use Name: sibling names are not a lexical identity.
	LexicalPath []uint32
	// Boundary is the child body's passive interprocedural boundary schema.
	// Stage 2 records it here but does not admit or evaluate child bodies.
	Boundary BodyBoundary
	// Symbol is the binder-owned identity of the function literal this proto
	// came from. It lets transfer publish expression-function facts from WIR
	// without reaching back to the AST.
	Symbol FunctionSymbolID
	// Type is the resolved function type for the literal. Lowering records the
	// annotation/binding identity; transfer decides how that type contributes to
	// value evidence.
	Type typ.Type
}

// BoundaryParameter identifies one formal slot at a lexical-body boundary.
// The declared type is a body-local TypeRef and is deliberately descriptive;
// it has no transfer semantics until the entry wire stage.
type BoundaryParameter struct {
	Symbol       SymbolID
	Name         string
	Type         TypeRef
	Vararg       bool
	ImplicitSelf bool
}

// BoundaryCapture identifies an ordered captured declaration. Mutable means
// the declaration has a lexical write somewhere in the bound unit; later
// stages turn this declarative bit into a capture-cell lens rather than a
// captured value snapshot.
type BoundaryCapture struct {
	Symbol SymbolID
	Name   string
	// Type retains the declared capture root for typed private child entry.
	// It is descriptive WIR metadata, never a runtime value.
	Type    TypeRef
	Mutable bool
}

// BodyBoundary is immutable-by-convention WIR metadata for an independently
// admitted lexical body. It is intentionally transport-free: no entry values,
// effects, summaries, or evaluator hooks live here.
type BodyBoundary struct {
	Parameters      []BoundaryParameter
	Captures        []BoundaryCapture
	DeclaredReturns []TypeRef
}

// CallResultTargetKind classifies the syntactic consumer of a call result.
type CallResultTargetKind uint8

const (
	CallResultTargetLocalAssignment CallResultTargetKind = iota + 1
	CallResultTargetOrdinaryAssignment
	CallResultTargetReturn
	CallResultTargetExpression
)

// CallResultTarget records where a call result is subsequently consumed. It is
// structural WIR metadata, not a semantic conclusion: lowering records the
// syntax-level result flow and transfer decides what facts that flow implies.
type CallResultTarget struct {
	Kind        CallResultTargetKind
	Index       int
	ResultIndex int
	Path        path.Path
}

// TableEntry records one statically-addressable table-constructor entry. Suffix
// is rootless and relative to the constructed object; Value is the lowered
// operand written to that suffix. ValueSpan and ValueLabel are syntax metadata
// for diagnostics; they do not describe or constrain runtime values.
type TableEntry struct {
	Suffix     path.Path
	Value      Operand
	ValueSpan  Span
	ValueLabel string
}

// CallArgumentMeta records source-only metadata for a syntactic call argument.
// It is diagnostic metadata: it never participates in value derivation.
type CallArgumentMeta struct {
	Span  Span
	Label string
}

// ReturnValueMeta records source-only metadata for a syntactic return value.
// It never participates in value derivation; transfer owns return facts and
// body/readmodel use this only for labels and spans while the semantic sidecar
// is retired.
type ReturnValueMeta struct {
	Span  Span
	Label string
}

// ConcatOperandMeta records the source anchor for one flattened concat
// operand. It is diagnostic metadata only: expression evaluation still uses
// the ordinary operand list and never consults it for a value.
type ConcatOperandMeta struct {
	Span  Span
	Label string
}

// ExpressionEvaluation records a structural expression-evaluation anchor at a
// CFG point. It carries only neutral source identity and span metadata; body
// readmodels join ExprID back to their source AST when they still need syntax
// during the migration away from cfgbuild sidecars.
type ExpressionEvaluation struct {
	ExprID ExpressionID
	Span   Span
}

// IfChainDescriptor is passive source topology for one authored if/elseif
// chain. It is deliberately independent of the CFG: later consumers can
// associate branch facts with their original chain without reconstructing a
// source tree after solving.
type IfChainDescriptor struct {
	ID       uint32
	HeadSpan Span
	Branches []IfChainBranch
	HasElse  bool
}

// IfChainBranch identifies one condition in source order. Point is the CFG
// point that owns the corresponding OpBranch.
type IfChainBranch struct {
	Point cfg.Point
	Span  Span
}

// StructuralExpressionRegion preserves source-authored short-circuit topology
// under the result temporary which transfer lowering turns into an ExprRef.
// OwnedRHSPoints is complete effect ownership: every point conditionally
// executed as part of the RHS is included, regardless of its current opcode.
//
// BypassValue is the left operand the result temporary carries on the bypass
// edge, where Lua's short-circuit yields the left operand itself. The bypass
// edge is the join, so that edge owns no point of its own and the assignment
// stands before the guard; naming the operand here is what lets a consumer
// attribute the pre-guard row to the one edge it describes. It is set only by
// the branch-topology lowering; the point-local value form carries none.
type StructuralExpressionRegion struct {
	Guard          cfg.Point
	TrueTarget     cfg.Point
	FalseTarget    cfg.Point
	Join           cfg.Point
	RHSOnTrue      bool
	OwnedRHSPoints []cfg.Point
	BypassValue    Operand
}

// StructuralExpressionOwner identifies the exact WIR producer whose ExprRef
// transfer lowering owns. Temps include zero; HasTemp distinguishes them from
// direct point-local OpLogical producers.
type StructuralExpressionOwner struct {
	HasTemp bool
	Temp    uint32
	Point   cfg.Point
}

// TableEntryRange is a [Start, Start+Len) window into Body.tableEntries.
type TableEntryRange struct {
	Start uint32
	Len   uint32
}

// CallArgumentMetaRange is a [Start, Start+Len) window into Body.callArgMeta.
type CallArgumentMetaRange struct {
	Start uint32
	Len   uint32
}

// ReturnValueMetaRange is a [Start, Start+Len) window into Body.returnMeta.
type ReturnValueMetaRange struct {
	Start uint32
	Len   uint32
}

// ConcatOperandMetaRange is a [Start, Start+Len) window into Body.concatMeta.
type ConcatOperandMetaRange struct {
	Start uint32
	Len   uint32
}

// TypeRefRange is a [Start, Start+Len) window into Body.typeRefPool.
type TypeRefRange struct {
	Start uint32
	Len   uint32
}

// ImpliedCheckRange is a [Start, Start+Len) window into Body.impliedChecks.
type ImpliedCheckRange struct {
	Start uint32
	Len   uint32
}

// ArmRange is a [Start, Start+Len) window into Body.armGroups: one
// ImpliedCheckRange per top-level sufficiency arm, in source order. An arm
// that resolved no leaf check is stored as a zero-length ImpliedCheckRange
// rather than omitted, so a sound joined conclusion can require every arm to
// contribute before trusting it.
type ArmRange struct {
	Start uint32
	Len   uint32
}

// BranchDiffConstraintRange is a [Start, Start+Len) window into Body.branchDiffs.
type BranchDiffConstraintRange struct {
	Start uint32
	Len   uint32
}

// SegmentRange is a [Start, Start+Len) window into Body.segments.
type SegmentRange struct {
	Start uint32
	Len   uint32
}

type callResultTargetKey struct {
	point cfg.Point
	index int
}

// pointRange is the instruction window owned by one CFG point.
type pointRange struct {
	valid bool
	start uint32
	len   uint32
}

// ConstKind tags a constant operand.
type ConstKind uint8

const (
	ConstNil ConstKind = iota
	ConstBool
	ConstNumber
	ConstString
)

// Const is an interned literal operand. It is comparable so it can key the
// intern map directly. Number holds the raw source spelling to keep the literal
// codegen-exact (no lossy float round-trip).
type Const struct {
	Kind   ConstKind
	Bool   bool
	Number string
	Str    string
}

// Type is an interned type reference: the resolved type identity together with a
// display spelling derived from it. Lowering resolves type expressions through
// the same resolution path the engine uses (decision D5), so the pool carries
// real typ.Type identity for the transfer interpreter, not a syntactic spelling.
// Display is t.String(); it exists only for printing and never keys the pool.
type Type struct {
	T       typ.Type
	Display string
}

// RootType records a resolved type for a root path that is referenced by this
// body but whose declaration may live outside this body's instruction stream.
// Lowering records the type identity; transfer decides how to use it.
type RootType struct {
	Path path.Path
	Type TypeRef
}

// NewBody creates an empty Body with reserved index-0 sentinels in each pool.
func NewBody(name string) *Body {
	b := &Body{
		Name:          name,
		paths:         make([]path.Path, 1),
		consts:        make([]Const, 1),
		types:         make([]Type, 1),
		checks:        make([]Check, 1),
		protos:        make([]FuncProto, 1),
		pathIndex:     make(map[path.PathKey]PathRef),
		constIndex:    make(map[Const]ConstRef),
		typeIndex:     make(map[uint64]TypeRef),
		rootTypeIndex: make(map[path.PathKey]int),
	}
	return b
}

// SetSymbolInfo records stable identity metadata for a value symbol referenced
// by this body. Later calls merge new non-zero metadata into the existing entry.
func (b *Body) SetSymbolInfo(id SymbolID, config SymbolInfoConfig) {
	if b == nil || id == 0 {
		return
	}
	if b.symbols == nil {
		b.symbols = make(map[SymbolID]SymbolInfo)
	}
	info := b.symbols[id]
	if config.Kind != SymbolUnknown {
		info.Kind = config.Kind
	}
	if config.Name != "" {
		info.Name = b.InternConst(Const{Kind: ConstString, Str: config.Name})
	}
	if config.RequireModule != "" {
		info.RequireModule = b.InternConst(Const{Kind: ConstString, Str: config.RequireModule})
	}
	info.HasWrite = info.HasWrite || config.HasWrite
	info.ImplicitGlobal = info.ImplicitGlobal || config.ImplicitGlobal
	b.symbols[id] = info
	if info.Kind == SymbolGlobal {
		name := b.symbolInfoString(info.Name)
		if name != "" {
			if b.globalSymbols == nil {
				b.globalSymbols = make(map[string]SymbolID)
			}
			b.globalSymbols[name] = id
		}
	}
}

// SymbolInfo returns the recorded symbol metadata for id.
func (b *Body) SymbolInfo(id SymbolID) (SymbolInfo, bool) {
	if b == nil || b.symbols == nil || id == 0 {
		return SymbolInfo{}, false
	}
	info, ok := b.symbols[id]
	return info, ok
}

// SymbolKind returns the WIR symbol kind for id.
func (b *Body) SymbolKind(id SymbolID) (SymbolKind, bool) {
	info, ok := b.SymbolInfo(id)
	if !ok || info.Kind == SymbolUnknown {
		return SymbolUnknown, false
	}
	return info.Kind, true
}

// SymbolName returns the declaration name recorded for id.
func (b *Body) SymbolName(id SymbolID) string {
	info, ok := b.SymbolInfo(id)
	if !ok {
		return ""
	}
	return b.symbolInfoString(info.Name)
}

// SymbolHasWrite reports whether ordinary assignment syntax writes id.
func (b *Body) SymbolHasWrite(id SymbolID) bool {
	info, ok := b.SymbolInfo(id)
	return ok && info.HasWrite
}

// IsImplicitGlobalSymbol reports whether id was created by an unresolved global
// read rather than a configured global or explicit write target.
func (b *Body) IsImplicitGlobalSymbol(id SymbolID) bool {
	info, ok := b.SymbolInfo(id)
	return ok && info.ImplicitGlobal
}

// SymbolResolvesToGlobal reports whether id is the global symbol named name.
func (b *Body) SymbolResolvesToGlobal(id SymbolID, name string) bool {
	if id == 0 || name == "" {
		return false
	}
	info, ok := b.SymbolInfo(id)
	return ok && info.Kind == SymbolGlobal && b.symbolInfoString(info.Name) == name
}

// GlobalSymbol returns the symbol recorded for a global name.
func (b *Body) GlobalSymbol(name string) (SymbolID, bool) {
	if b == nil || b.globalSymbols == nil || name == "" {
		return 0, false
	}
	id, ok := b.globalSymbols[name]
	return id, ok && id != 0
}

// SymbolRequireModulePath returns the exact require module identity recorded for
// a local require root.
func (b *Body) SymbolRequireModulePath(id SymbolID) (string, bool) {
	info, ok := b.SymbolInfo(id)
	if !ok || info.RequireModule == 0 {
		return "", false
	}
	modulePath := b.symbolInfoString(info.RequireModule)
	return modulePath, modulePath != ""
}

func (b *Body) symbolInfoString(ref ConstRef) string {
	if b == nil || ref == 0 {
		return ""
	}
	c := b.Const(ref)
	if c.Kind != ConstString {
		return ""
	}
	return c.Str
}

// Instr returns the instruction at flat index i.
func (b *Body) Instr(i int) Instruction { return b.instrs[i] }

// Len returns the number of instructions in the stream.
func (b *Body) Len() int { return len(b.instrs) }

// ForEachCall visits every OpCall in lowering order without rescanning the
// complete instruction stream. Lowering is the independent authority for this
// census: semantic consumers may classify a call, but cannot make one appear
// or disappear by omitting a derived fact.
func (b *Body) ForEachCall(fn func(Instruction) bool) {
	if b == nil || fn == nil {
		return
	}
	for _, raw := range b.calls {
		if uint64(raw) >= uint64(len(b.instrs)) {
			return
		}
		instruction := b.instrs[raw]
		if instruction.Op != OpCall || !fn(instruction) {
			return
		}
	}
}

// PointInstructions returns the instruction window attached to point p.
func (b *Body) PointInstructions(p cfg.Point) []Instruction {
	idx := int(p)
	if idx < 0 || idx >= len(b.points) || !b.points[idx].valid {
		return nil
	}
	r := b.points[idx]
	return b.instrs[r.start : r.start+r.len]
}

// HasInstruction reports whether point p carries at least one instruction with
// opcode op. It is the canonical zero-allocation query for consumers that need
// point-level topology without inspecting the raw instruction stream.
func (b *Body) HasInstruction(p cfg.Point, op Op) bool {
	for _, inst := range b.PointInstructions(p) {
		if inst.Op == op {
			return true
		}
	}
	return false
}

// Path returns the interned path for ref, or the zero Path for a none ref.
func (b *Body) Path(ref PathRef) path.Path {
	if ref == 0 || int(ref) >= len(b.paths) {
		return path.Path{}
	}
	return b.paths[ref]
}

// Const returns the interned constant for ref.
func (b *Body) Const(ref ConstRef) Const {
	if ref == 0 || int(ref) >= len(b.consts) {
		return Const{}
	}
	return b.consts[ref]
}

// ForEachConst visits every interned constant in body order, excluding the
// sentinel none entry.
func (b *Body) ForEachConst(fn func(Const) bool) {
	if b == nil || fn == nil {
		return
	}
	for i := 1; i < len(b.consts); i++ {
		if !fn(b.consts[i]) {
			return
		}
	}
}

// Type returns the resolved type identity for ref, or nil for a none ref.
func (b *Body) Type(ref TypeRef) typ.Type {
	if ref == 0 || int(ref) >= len(b.types) {
		return nil
	}
	return b.types[ref].T
}

// ForEachType visits each resolved type identity interned by this body. The
// callback receives only fully resolved entries; callers must not infer a
// type from a display spelling when an exact provider identity is required.
func (b *Body) ForEachType(fn func(typ.Type) bool) {
	if b == nil || fn == nil {
		return
	}
	for i := 1; i < len(b.types); i++ {
		if b.types[i].T != nil && !fn(b.types[i].T) {
			return
		}
	}
}

// TypeDisplay returns the interned type's display spelling for ref.
func (b *Body) TypeDisplay(ref TypeRef) string {
	if ref == 0 || int(ref) >= len(b.types) {
		return ""
	}
	return b.types[ref].Display
}

// SetRootType records a resolved type for a root path used by this body.
func (b *Body) SetRootType(p path.Path, t typ.Type) {
	if b == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 || t == nil {
		return
	}
	ref := b.InternType(t)
	entry := RootType{Path: p, Type: ref}
	key := p.Key()
	if idx, ok := b.rootTypeIndex[key]; ok {
		b.rootTypes[idx] = entry
		return
	}
	b.rootTypeIndex[key] = len(b.rootTypes)
	b.rootTypes = append(b.rootTypes, entry)
}

// RootTypes returns resolved root-path types recorded for external declarations
// referenced by this body.
func (b *Body) RootTypes() []RootType {
	if b == nil || len(b.rootTypes) == 0 {
		return nil
	}
	out := make([]RootType, len(b.rootTypes))
	copy(out, b.rootTypes)
	return out
}

// DeclaredReturnArity returns the syntactic declared return arity for this
// function body. It is zero for chunks and unannotated functions.
func (b *Body) DeclaredReturnArity() int {
	if b == nil {
		return 0
	}
	return len(b.declaredReturns)
}

// DeclaredReturnTypes returns the resolved declared return types when lowering
// could resolve them. Unresolved slots are nil but still count toward arity.
func (b *Body) DeclaredReturnTypes() []typ.Type {
	if b == nil || len(b.declaredReturns) == 0 {
		return nil
	}
	out := make([]typ.Type, len(b.declaredReturns))
	for i, ref := range b.declaredReturns {
		out[i] = b.Type(ref)
	}
	return out
}

// Check returns the interned branch check for ref.
func (b *Body) Check(ref CheckRef) Check {
	if ref == 0 || int(ref) >= len(b.checks) {
		return Check{}
	}
	return b.checks[ref]
}

// ForEachBranchCheck visits the branch checks attached to point p, in
// instruction order. Consumers that need branch topology should use this view
// instead of rescanning raw instructions for OpBranch.
func (b *Body) ForEachBranchCheck(p cfg.Point, fn func(Check) bool) {
	for _, inst := range b.PointInstructions(p) {
		if inst.Op != OpBranch {
			continue
		}
		if !fn(b.Check(inst.Check)) {
			return
		}
	}
}

// BranchChecks returns the branch checks attached to point p, in instruction
// order. It is a convenience wrapper for tests and one-shot consumers; hot paths
// should prefer ForEachBranchCheck.
func (b *Body) BranchChecks(p cfg.Point) []Check {
	var out []Check
	b.ForEachBranchCheck(p, func(check Check) bool {
		out = append(out, check)
		return true
	})
	return out
}

// TableConstructorByExpressionID returns the table-constructor instruction with
// source expression identity id. It is a migration bridge for facts that remain
// keyed by expression identity while constructor structure lives in WIR.
func (b *Body) TableConstructorByExpressionID(id ExpressionID) (Instruction, bool) {
	if b == nil || id == 0 {
		return Instruction{}, false
	}
	for _, inst := range b.instrs {
		if inst.Op == OpMakeTable && inst.ExprID == id {
			return inst, true
		}
	}
	return Instruction{}, false
}

// Operands returns the operand slice for a variadic range.
func (b *Body) Operands(r OperandRange) []Operand {
	if r.Len == 0 {
		return nil
	}
	return b.operandPool[r.Start : r.Start+r.Len]
}

// TypeRefs returns the type-ref slice for a variadic range.
func (b *Body) TypeRefs(r TypeRefRange) []TypeRef {
	if r.Len == 0 {
		return nil
	}
	return b.typeRefPool[r.Start : r.Start+r.Len]
}

// TableEntries returns the table-entry slice for a variadic range.
func (b *Body) TableEntries(r TableEntryRange) []TableEntry {
	if r.Len == 0 {
		return nil
	}
	return b.tableEntries[r.Start : r.Start+r.Len]
}

// CallArgumentMeta returns the call-argument metadata slice for a variadic
// range.
func (b *Body) CallArgumentMeta(r CallArgumentMetaRange) []CallArgumentMeta {
	if r.Len == 0 {
		return nil
	}
	return b.callArgMeta[r.Start : r.Start+r.Len]
}

// ReturnValueMeta returns the return-value metadata slice for a variadic range.
func (b *Body) ReturnValueMeta(r ReturnValueMetaRange) []ReturnValueMeta {
	if r.Len == 0 {
		return nil
	}
	return b.returnMeta[r.Start : r.Start+r.Len]
}

// ConcatOperandMeta returns metadata for one flattened concat expression.
func (b *Body) ConcatOperandMeta(r ConcatOperandMetaRange) []ConcatOperandMeta {
	if r.Len == 0 {
		return nil
	}
	return b.concatMeta[r.Start : r.Start+r.Len]
}

// ImpliedChecks returns the branch-implied check slice for a variadic range.
func (b *Body) ImpliedChecks(r ImpliedCheckRange) []ImpliedCheck {
	if r.Len == 0 {
		return nil
	}
	return b.impliedChecks[r.Start : r.Start+r.Len]
}

// SufficientChecks returns the branch-sufficient check slice for a variadic
// range. It shares the branch-check pool with ImpliedChecks; the instruction
// field determines which relation the window represents.
func (b *Body) SufficientChecks(r ImpliedCheckRange) []ImpliedCheck {
	return b.ImpliedChecks(r)
}

// SufficientCheckArms resolves an arm range into its per-arm leaf-check
// groups, in source order. An element is nil when that arm resolved no leaf
// check; unlike SufficientChecks, arm boundaries are preserved.
func (b *Body) SufficientCheckArms(r ArmRange) [][]ImpliedCheck {
	if r.Len == 0 {
		return nil
	}
	groups := b.armGroups[r.Start : r.Start+r.Len]
	out := make([][]ImpliedCheck, len(groups))
	for i, group := range groups {
		out[i] = b.ImpliedChecks(group)
	}
	return out
}

// BranchDiffConstraints returns the branch difference-constraint descriptors
// for a variadic range.
func (b *Body) BranchDiffConstraints(r BranchDiffConstraintRange) []BranchDiffConstraint {
	if r.Len == 0 {
		return nil
	}
	return b.branchDiffs[r.Start : r.Start+r.Len]
}

// Segments returns the static suffix slice for a variadic segment range.
func (b *Body) Segments(r SegmentRange) []segment.Segment {
	if r.Len == 0 {
		return nil
	}
	return b.segments[r.Start : r.Start+r.Len]
}

// AppendSegments stores a static suffix segment window in the body.
func (b *Body) AppendSegments(segments []segment.Segment) SegmentRange {
	if len(segments) == 0 {
		return SegmentRange{}
	}
	start := uint32(len(b.segments))
	b.segments = append(b.segments, segments...)
	return SegmentRange{Start: start, Len: uint32(len(segments))}
}

// InternPath interns a path and returns its 1-based ref. The empty path is none.
func (b *Body) InternPath(p path.Path) PathRef {
	if p.IsEmpty() {
		return 0
	}
	key := p.Key()
	if ref, ok := b.pathIndex[key]; ok {
		return ref
	}
	ref := PathRef(len(b.paths))
	b.paths = append(b.paths, p)
	b.pathIndex[key] = ref
	return ref
}

// InternConst interns a constant and returns its 1-based ref.
func (b *Body) InternConst(c Const) ConstRef {
	if ref, ok := b.constIndex[c]; ok {
		return ref
	}
	ref := ConstRef(len(b.consts))
	b.consts = append(b.consts, c)
	b.constIndex[c] = ref
	return ref
}

// InternType interns a resolved type by its stable identity and returns its
// 1-based ref. A nil type (an unresolved type expression) is none. Interning
// keys on typ.EqualityHash, the canonical structural dedup hash, so distinct
// spellings of the same resolved type share one ref.
func (b *Body) InternType(t typ.Type) TypeRef {
	if t == nil {
		return 0
	}
	h := typ.EqualityHash(t)
	if ref, ok := b.typeIndex[h]; ok {
		return ref
	}
	ref := TypeRef(len(b.types))
	b.types = append(b.types, Type{T: t, Display: t.String()})
	b.typeIndex[h] = ref
	return ref
}

// SetDeclaredReturnTypes records resolved declared returns for this function
// body. Nil entries are preserved as unresolved slots so arity remains
// authoritative even when a type cannot be resolved.
func (b *Body) SetDeclaredReturnTypes(types []typ.Type) {
	if b == nil || len(types) == 0 {
		return
	}
	b.declaredReturns = make([]TypeRef, len(types))
	for i, t := range types {
		b.declaredReturns[i] = b.InternType(t)
	}
}

// SetDeclaredReturnSpans records the authored anchor of each declared return
// slot. Slots without a source anchor keep an invalid span so arity stays
// aligned with the declared contract.
func (b *Body) SetDeclaredReturnSpans(spans []Span) {
	if b == nil || len(spans) == 0 {
		return
	}
	b.declaredReturnSpans = append(b.declaredReturnSpans[:0], spans...)
}

// DeclaredReturnSpans returns the authored anchors of the declared returns.
func (b *Body) DeclaredReturnSpans() []Span {
	if b == nil {
		return nil
	}
	return append([]Span(nil), b.declaredReturnSpans...)
}

// DeclaredReturnRefs returns the body-local declared contract slots.
func (b *Body) DeclaredReturnRefs() []TypeRef {
	if b == nil {
		return nil
	}
	return append([]TypeRef(nil), b.declaredReturns...)
}

// SetLexicalPath records this body's source-order path below the chunk. The
// input is copied so later lowering of siblings cannot mutate an identity that
// has already been handed to the front.
func (b *Body) SetLexicalPath(lexicalPath []uint32) {
	if b == nil {
		return
	}
	b.lexicalPath = append(b.lexicalPath[:0], lexicalPath...)
	// A child finishes lowering before its parent attaches the parent path.
	// Repair its already-created descendants here, preserving source order at
	// every depth without needing a second AST traversal.
	for index := 1; index < len(b.protos); index++ {
		childPath := append(append([]uint32(nil), b.lexicalPath...), uint32(index-1))
		b.protos[index].LexicalPath = childPath
		if b.protos[index].Body != nil {
			b.protos[index].Body.SetLexicalPath(childPath)
		}
	}
}

// LexicalPath returns an owned copy of this body's source-order identity.
func (b *Body) LexicalPath() []uint32 {
	if b == nil {
		return nil
	}
	return append([]uint32(nil), b.lexicalPath...)
}

// SetBoundary records passive lexical boundary metadata.  Stage 2 must retain
// this information without changing how the current single-body evaluator
// executes, so every slice is copied on both sides of the API.
func (b *Body) SetBoundary(boundary BodyBoundary) {
	if b == nil {
		return
	}
	b.boundary = cloneBodyBoundary(boundary)
}

// Boundary returns a copy of this body's passive lexical boundary metadata.
func (b *Body) Boundary() BodyBoundary {
	if b == nil {
		return BodyBoundary{}
	}
	return cloneBodyBoundary(b.boundary)
}

func cloneBodyBoundary(boundary BodyBoundary) BodyBoundary {
	boundary.Parameters = append([]BoundaryParameter(nil), boundary.Parameters...)
	boundary.Captures = append([]BoundaryCapture(nil), boundary.Captures...)
	boundary.DeclaredReturns = append([]TypeRef(nil), boundary.DeclaredReturns...)
	return boundary
}

// InternCheck appends a branch check and returns its 1-based ref. Checks are not
// deduplicated: each branch owns exactly one.
func (b *Body) InternCheck(c Check) CheckRef {
	ref := CheckRef(len(b.checks))
	b.checks = append(b.checks, c)
	return ref
}

// AddProto appends a nested function proto and returns its 1-based ref.
func (b *Body) AddProto(p FuncProto) FuncRef {
	p.LexicalPath = append([]uint32(nil), p.LexicalPath...)
	p.Boundary = cloneBodyBoundary(p.Boundary)
	ref := FuncRef(len(b.protos))
	b.protos = append(b.protos, p)
	return ref
}

// Proto returns the nested proto for ref, or the zero FuncProto for a none ref.
func (b *Body) Proto(ref FuncRef) FuncProto {
	if ref == 0 || int(ref) >= len(b.protos) {
		return FuncProto{}
	}
	proto := b.protos[ref]
	proto.LexicalPath = append([]uint32(nil), proto.LexicalPath...)
	proto.Boundary = cloneBodyBoundary(proto.Boundary)
	return proto
}

// Protos returns the nested function protos (excluding the index-0 sentinel).
func (b *Body) Protos() []FuncProto {
	if len(b.protos) <= 1 {
		return nil
	}
	out := make([]FuncProto, len(b.protos)-1)
	for index := range out {
		out[index] = b.Proto(FuncRef(index + 1))
	}
	return out
}

// SetCallResultTarget records the syntactic consumer of one result from the
// call/select instruction at point.
func (b *Body) SetCallResultTarget(point cfg.Point, target CallResultTarget) {
	if b == nil || target.ResultIndex < 0 {
		return
	}
	if b.callTargets == nil {
		b.callTargets = make(map[callResultTargetKey]CallResultTarget)
	}
	target.Path = target.Path.Clone()
	b.callTargets[callResultTargetKey{point: point, index: target.ResultIndex}] = target
}

// CallResultTarget returns the statically known consumer of a call/select
// result, if lowering could determine one.
func (b *Body) CallResultTarget(point cfg.Point, index int) (CallResultTarget, bool) {
	if b == nil || b.callTargets == nil || index < 0 {
		return CallResultTarget{}, false
	}
	target, ok := b.callTargets[callResultTargetKey{point: point, index: index}]
	target.Path = target.Path.Clone()
	return target, ok
}

// CallResultTargets returns every call/select result target recorded at point,
// ordered by result index.
func (b *Body) CallResultTargets(point cfg.Point) []CallResultTarget {
	if b == nil || b.callTargets == nil {
		return nil
	}
	max := -1
	for key := range b.callTargets {
		if key.point == point && key.index > max {
			max = key.index
		}
	}
	if max < 0 {
		return nil
	}
	out := make([]CallResultTarget, 0, max+1)
	for i := 0; i <= max; i++ {
		target, ok := b.callTargets[callResultTargetKey{point: point, index: i}]
		if !ok {
			continue
		}
		target.Path = target.Path.Clone()
		out = append(out, target)
	}
	return out
}

// SetExpressionEvaluation records that point is a structural evaluation anchor
// for source expression eval.ExprID.
func (b *Body) SetExpressionEvaluation(point cfg.Point, eval ExpressionEvaluation) {
	if b == nil || eval.ExprID == 0 {
		return
	}
	if b.evaluations == nil {
		b.evaluations = make(map[cfg.Point]ExpressionEvaluation)
	}
	b.evaluations[point] = eval
}

// ExpressionEvaluation returns the structural expression-evaluation anchor at
// point, if WIR lowering recorded one.
func (b *Body) ExpressionEvaluation(point cfg.Point) (ExpressionEvaluation, bool) {
	if b == nil || b.evaluations == nil {
		return ExpressionEvaluation{}, false
	}
	eval, ok := b.evaluations[point]
	return eval, ok
}

// SetIfChainDescriptor records complete source topology for an if/elseif
// chain. Malformed descriptors are rejected so downstream consumers fail
// closed rather than infer a chain from solved control flow.
func (b *Body) SetIfChainDescriptor(descriptor IfChainDescriptor) {
	if b == nil || descriptor.ID == 0 || !descriptor.HeadSpan.Valid() || len(descriptor.Branches) == 0 {
		return
	}
	branches := append([]IfChainBranch(nil), descriptor.Branches...)
	seen := make(map[cfg.Point]bool, len(branches))
	for _, branch := range branches {
		if branch.Point == 0 || !branch.Span.Valid() || seen[branch.Point] {
			return
		}
		seen[branch.Point] = true
	}
	if b.ifChains == nil {
		b.ifChains = make(map[uint32]IfChainDescriptor)
	}
	b.ifChains[descriptor.ID] = IfChainDescriptor{ID: descriptor.ID, HeadSpan: descriptor.HeadSpan, Branches: branches, HasElse: descriptor.HasElse}
}

// IfChainDescriptor returns a defensive copy of the retained descriptor.
func (b *Body) IfChainDescriptor(id uint32) (IfChainDescriptor, bool) {
	if b == nil || id == 0 {
		return IfChainDescriptor{}, false
	}
	descriptor, ok := b.ifChains[id]
	if !ok {
		return IfChainDescriptor{}, false
	}
	descriptor.Branches = append([]IfChainBranch(nil), descriptor.Branches...)
	return descriptor, true
}

// ForEachIfChainDescriptor visits descriptors in stable chain-ID order.
func (b *Body) ForEachIfChainDescriptor(fn func(IfChainDescriptor) bool) {
	if b == nil || fn == nil || len(b.ifChains) == 0 {
		return
	}
	ids := make([]uint32, 0, len(b.ifChains))
	for id := range b.ifChains {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		descriptor := b.ifChains[id]
		descriptor.Branches = append([]IfChainBranch(nil), descriptor.Branches...)
		if !fn(descriptor) {
			return
		}
	}
}

// SetStructuralExpressionRegion records an exact region for a result temp.
// Malformed or incomplete records are rejected fail-closed.
func (b *Body) SetStructuralExpressionRegion(owner StructuralExpressionOwner, region StructuralExpressionRegion) {
	if b == nil || region.TrueTarget == region.FalseTarget || len(region.OwnedRHSPoints) == 0 {
		return
	}
	// Irrelevant owner fields must be zero. Otherwise two distinct Go keys can
	// spell the same semantic producer and make insertion order choose the
	// retained region nondeterministically.
	if (owner.HasTemp && owner.Point != 0) || (!owner.HasTemp && owner.Temp != 0) {
		return
	}
	rhsTarget, bypassTarget := region.FalseTarget, region.TrueTarget
	if region.RHSOnTrue {
		rhsTarget, bypassTarget = region.TrueTarget, region.FalseTarget
	}
	if bypassTarget != region.Join {
		return
	}
	points := append([]cfg.Point(nil), region.OwnedRHSPoints...)
	sort.Slice(points, func(i, j int) bool { return points[i] < points[j] })
	foundRHS := false
	for i, point := range points {
		if point == region.Guard || point == region.Join || (i != 0 && points[i-1] == point) {
			return
		}
		foundRHS = foundRHS || point == rhsTarget
	}
	if !foundRHS {
		return
	}
	if b.structuralRegions == nil {
		b.structuralRegions = make(map[StructuralExpressionOwner]StructuralExpressionRegion)
	}
	region.OwnedRHSPoints = points
	b.structuralRegions[owner] = region
}

// StructuralExpressionRegion returns a defensive copy of the region owned by owner.
func (b *Body) StructuralExpressionRegion(owner StructuralExpressionOwner) (StructuralExpressionRegion, bool) {
	if b == nil {
		return StructuralExpressionRegion{}, false
	}
	region, ok := b.structuralRegions[owner]
	if !ok {
		return StructuralExpressionRegion{}, false
	}
	region.OwnedRHSPoints = append([]cfg.Point(nil), region.OwnedRHSPoints...)
	return region, true
}

// ForEachStructuralExpressionRegion visits regions in canonical result-temp order.
func (b *Body) ForEachStructuralExpressionRegion(fn func(StructuralExpressionOwner, StructuralExpressionRegion) bool) {
	if b == nil || fn == nil || len(b.structuralRegions) == 0 {
		return
	}
	owners := make([]StructuralExpressionOwner, 0, len(b.structuralRegions))
	for owner := range b.structuralRegions {
		owners = append(owners, owner)
	}
	sort.Slice(owners, func(i, j int) bool {
		if owners[i].HasTemp != owners[j].HasTemp {
			return owners[i].HasTemp
		}
		if owners[i].HasTemp {
			return owners[i].Temp < owners[j].Temp
		}
		return owners[i].Point < owners[j].Point
	})
	for _, owner := range owners {
		region := b.structuralRegions[owner]
		region.OwnedRHSPoints = append([]cfg.Point(nil), region.OwnedRHSPoints...)
		if !fn(owner, region) {
			return
		}
	}
}

// AppendOperands copies ops into the shared pool and returns their range.
func (b *Body) AppendOperands(ops []Operand) OperandRange {
	if len(ops) == 0 {
		return OperandRange{}
	}
	start := uint32(len(b.operandPool))
	b.operandPool = append(b.operandPool, ops...)
	return OperandRange{Start: start, Len: uint32(len(ops))}
}

// AppendTypeRefs copies refs into the shared type-ref pool and returns their
// range.
func (b *Body) AppendTypeRefs(refs []TypeRef) TypeRefRange {
	if len(refs) == 0 {
		return TypeRefRange{}
	}
	start := uint32(len(b.typeRefPool))
	b.typeRefPool = append(b.typeRefPool, refs...)
	return TypeRefRange{Start: start, Len: uint32(len(refs))}
}

// AppendTableEntries copies entries into the shared table-entry pool and returns
// their range.
func (b *Body) AppendTableEntries(entries []TableEntry) TableEntryRange {
	if len(entries) == 0 {
		return TableEntryRange{}
	}
	start := uint32(len(b.tableEntries))
	b.tableEntries = append(b.tableEntries, entries...)
	return TableEntryRange{Start: start, Len: uint32(len(entries))}
}

// AppendCallArgumentMeta copies metadata into the shared call-argument metadata
// pool and returns its range.
func (b *Body) AppendCallArgumentMeta(meta []CallArgumentMeta) CallArgumentMetaRange {
	if len(meta) == 0 {
		return CallArgumentMetaRange{}
	}
	start := uint32(len(b.callArgMeta))
	b.callArgMeta = append(b.callArgMeta, meta...)
	return CallArgumentMetaRange{Start: start, Len: uint32(len(meta))}
}

// AppendReturnValueMeta copies metadata into the shared return-value metadata
// pool and returns its range.
func (b *Body) AppendReturnValueMeta(meta []ReturnValueMeta) ReturnValueMetaRange {
	if len(meta) == 0 {
		return ReturnValueMetaRange{}
	}
	start := uint32(len(b.returnMeta))
	b.returnMeta = append(b.returnMeta, meta...)
	return ReturnValueMetaRange{Start: start, Len: uint32(len(meta))}
}

// AppendConcatOperandMeta copies concat operand anchors into the shared pool.
func (b *Body) AppendConcatOperandMeta(meta []ConcatOperandMeta) ConcatOperandMetaRange {
	if len(meta) == 0 {
		return ConcatOperandMetaRange{}
	}
	start := uint32(len(b.concatMeta))
	b.concatMeta = append(b.concatMeta, meta...)
	return ConcatOperandMetaRange{Start: start, Len: uint32(len(meta))}
}

// AppendImpliedChecks copies checks into the shared branch-implied-check pool
// and returns their range.
func (b *Body) AppendImpliedChecks(checks []ImpliedCheck) ImpliedCheckRange {
	if len(checks) == 0 {
		return ImpliedCheckRange{}
	}
	start := uint32(len(b.impliedChecks))
	b.impliedChecks = append(b.impliedChecks, checks...)
	return ImpliedCheckRange{Start: start, Len: uint32(len(checks))}
}

// AppendSufficientCheckArms copies each arm's leaf checks into the shared
// branch-implied-check pool, then windows them into the arm-group pool,
// returning the outer range. An arm with no checks still occupies its own
// zero-length window, so a caller reading the range back can require every
// arm to contribute before trusting a joined conclusion.
func (b *Body) AppendSufficientCheckArms(arms [][]ImpliedCheck) ArmRange {
	if len(arms) == 0 {
		return ArmRange{}
	}
	start := uint32(len(b.armGroups))
	for _, arm := range arms {
		b.armGroups = append(b.armGroups, b.AppendImpliedChecks(arm))
	}
	return ArmRange{Start: start, Len: uint32(len(arms))}
}

// AppendBranchDiffConstraints copies branch difference-constraint descriptors
// into the shared branch-diff pool and returns their range.
func (b *Body) AppendBranchDiffConstraints(diffs []BranchDiffConstraint) BranchDiffConstraintRange {
	if len(diffs) == 0 {
		return BranchDiffConstraintRange{}
	}
	start := uint32(len(b.branchDiffs))
	b.branchDiffs = append(b.branchDiffs, diffs...)
	return BranchDiffConstraintRange{Start: start, Len: uint32(len(diffs))}
}

// Emit appends an instruction and returns its flat index.
func (b *Body) Emit(inst Instruction) int {
	i := len(b.instrs)
	b.instrs = append(b.instrs, inst)
	if inst.Op == OpCall {
		b.calls = append(b.calls, uint32(i))
	}
	return i
}

// SetPointRange records the instruction window [start, end) for point p.
func (b *Body) SetPointRange(p cfg.Point, start, end int) {
	idx := int(p)
	for len(b.points) <= idx {
		b.points = append(b.points, pointRange{})
	}
	b.points[idx] = pointRange{valid: true, start: uint32(start), len: uint32(end - start)}
}
