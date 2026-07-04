package wir

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
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
	points []pointRange // indexed by cfg.Point

	paths  []path.Path
	consts []Const
	types  []Type
	checks []Check
	protos []FuncProto

	operandPool []Operand

	pathIndex  map[path.PathKey]PathRef
	constIndex map[Const]ConstRef
	typeIndex  map[uint64]TypeRef
}

// FuncProto is a nested function lowered as its own Body and CFG. A parent
// OpClosure references it by FuncRef; the child owns its own topology exactly
// like a top-level function.
type FuncProto struct {
	Name  string
	Body  *Body
	Graph *cfg.CFG
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

// NewBody creates an empty Body with reserved index-0 sentinels in each pool.
func NewBody(name string) *Body {
	b := &Body{
		Name:       name,
		paths:      make([]path.Path, 1),
		consts:     make([]Const, 1),
		types:      make([]Type, 1),
		checks:     make([]Check, 1),
		protos:     make([]FuncProto, 1),
		pathIndex:  make(map[path.PathKey]PathRef),
		constIndex: make(map[Const]ConstRef),
		typeIndex:  make(map[uint64]TypeRef),
	}
	return b
}

// Instr returns the instruction at flat index i.
func (b *Body) Instr(i int) Instruction { return b.instrs[i] }

// Len returns the number of instructions in the stream.
func (b *Body) Len() int { return len(b.instrs) }

// PointInstructions returns the instruction window attached to point p.
func (b *Body) PointInstructions(p cfg.Point) []Instruction {
	idx := int(p)
	if idx < 0 || idx >= len(b.points) || !b.points[idx].valid {
		return nil
	}
	r := b.points[idx]
	return b.instrs[r.start : r.start+r.len]
}

// HasPoint reports whether point p carries any instruction window.
func (b *Body) HasPoint(p cfg.Point) bool {
	idx := int(p)
	return idx >= 0 && idx < len(b.points) && b.points[idx].valid
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

// Type returns the resolved type identity for ref, or nil for a none ref.
func (b *Body) Type(ref TypeRef) typ.Type {
	if ref == 0 || int(ref) >= len(b.types) {
		return nil
	}
	return b.types[ref].T
}

// TypeDisplay returns the interned type's display spelling for ref.
func (b *Body) TypeDisplay(ref TypeRef) string {
	if ref == 0 || int(ref) >= len(b.types) {
		return ""
	}
	return b.types[ref].Display
}

// Check returns the interned branch check for ref.
func (b *Body) Check(ref CheckRef) Check {
	if ref == 0 || int(ref) >= len(b.checks) {
		return Check{}
	}
	return b.checks[ref]
}

// Operands returns the operand slice for a variadic range.
func (b *Body) Operands(r OperandRange) []Operand {
	if r.Len == 0 {
		return nil
	}
	return b.operandPool[r.Start : r.Start+r.Len]
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

// InternCheck appends a branch check and returns its 1-based ref. Checks are not
// deduplicated: each branch owns exactly one.
func (b *Body) InternCheck(c Check) CheckRef {
	ref := CheckRef(len(b.checks))
	b.checks = append(b.checks, c)
	return ref
}

// AddProto appends a nested function proto and returns its 1-based ref.
func (b *Body) AddProto(p FuncProto) FuncRef {
	ref := FuncRef(len(b.protos))
	b.protos = append(b.protos, p)
	return ref
}

// Proto returns the nested proto for ref, or the zero FuncProto for a none ref.
func (b *Body) Proto(ref FuncRef) FuncProto {
	if ref == 0 || int(ref) >= len(b.protos) {
		return FuncProto{}
	}
	return b.protos[ref]
}

// Protos returns the nested function protos (excluding the index-0 sentinel).
func (b *Body) Protos() []FuncProto {
	if len(b.protos) <= 1 {
		return nil
	}
	return b.protos[1:]
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

// Emit appends an instruction and returns its flat index.
func (b *Body) Emit(inst Instruction) int {
	i := len(b.instrs)
	b.instrs = append(b.instrs, inst)
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
