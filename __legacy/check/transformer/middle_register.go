package transformer

import (
	"fmt"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// relationMiddleRegisterKind is the closed lexical-register vocabulary.
// Storage identity is stable across writes; formalRelationCell supplies the
// version dimension. Path-addressable locals are Symbol registers, not a
// parallel path namespace.
type relationMiddleRegisterKind uint8

const (
	relationMiddleRegisterInvalid relationMiddleRegisterKind = iota
	relationMiddleRegisterSymbol
	relationMiddleRegisterCallResult
	relationMiddleRegisterExpression
)

type relationMiddleRegister struct {
	kind       relationMiddleRegisterKind
	slot       statekey.Value
	symbol     symbol.ID
	point      cfg.Point
	ordinal    uint32
	expression factflow.ExprRef
	// formalOrdinal is the uncapped durable ordinal. Root.Index is only the
	// arena-local machine acceleration coordinate and is checked before use.
	formalOrdinal uint64
}

func middleRegisterForSlot(slot statekey.Value) (relationMiddleRegister, bool) {
	if id, ok := statekey.ParseSymbolValue(slot); ok {
		return relationMiddleRegister{kind: relationMiddleRegisterSymbol, slot: slot, symbol: id}, true
	}
	if point, ordinal, ok := statekey.ParseCallResult(slot); ok {
		return relationMiddleRegister{kind: relationMiddleRegisterCallResult, slot: slot, point: cfg.Point(point), ordinal: ordinal}, true
	}
	if expression, ok := statekey.ParseExpressionValue(slot); ok {
		return relationMiddleRegister{kind: relationMiddleRegisterExpression, slot: slot, expression: factflow.ExprRef(expression)}, true
	}
	return relationMiddleRegister{}, false
}

func relationMiddleRegisterLess(left, right relationMiddleRegister) bool {
	if left.kind != right.kind {
		return left.kind < right.kind
	}
	switch left.kind {
	case relationMiddleRegisterSymbol:
		return left.symbol < right.symbol
	case relationMiddleRegisterCallResult:
		if left.point != right.point {
			return left.point < right.point
		}
		return left.ordinal < right.ordinal
	case relationMiddleRegisterExpression:
		return left.expression < right.expression
	default:
		return false
	}
}

type relationMiddleRegisterSchema struct {
	registers []relationMiddleRegister
	bySlot    map[statekey.Value]int
	entries   []relationMiddleEntry
	sealed    bool
}

// relationMiddleEntry is the only IN -> MID seed vocabulary.  A boundary
// symbol is copied into its stable lexical register at the root cell; every
// later cell versions that same register identity instead of renaming it.
type relationMiddleEntry struct {
	middle Root
	input  Root
}

// includeMiddleRegisterInventory admits body-local storage identities that are
// semantically owned even when no executable term happens to read or write
// them. It adds no selector term and no IN binding: callers still cannot supply
// these coordinates. The closed slot decoder remains the sole authority for
// which concrete storage identities can become MID roots.
func (a *Arena) includeMiddleRegisterInventory(slots []statekey.Value) error {
	if a == nil || a.sealed || a.middle.sealed {
		return fmt.Errorf("transformer: Middle register inventory has no open arena")
	}
	for _, slot := range slots {
		if _, valid := middleRegisterForSlot(slot); !valid {
			return fmt.Errorf("transformer: slot %d has no Middle register identity", slot)
		}
		a.environment[slot] = struct{}{}
	}
	return nil
}

func (s *relationMiddleRegisterSchema) seal(environment map[statekey.Value]struct{}) error {
	if s == nil || s.sealed {
		return fmt.Errorf("transformer: Middle register schema is already sealed")
	}
	registers := make([]relationMiddleRegister, 0, len(environment))
	for slot := range environment {
		register, ok := middleRegisterForSlot(slot)
		if !ok {
			// Return slots are publication OUT coordinates, never invocation-local
			// registers. Their presence in the environment is malformed rather than
			// an ignorable spelling: silently dropping it would leave a raw selector.
			return fmt.Errorf("transformer: environment slot %d has no Middle register kind", slot)
		}
		registers = append(registers, register)
	}
	sort.Slice(registers, func(i, j int) bool { return relationMiddleRegisterLess(registers[i], registers[j]) })
	bySlot := make(map[statekey.Value]int, len(registers))
	for index, register := range registers {
		if register.slot == 0 || index != 0 && !relationMiddleRegisterLess(registers[index-1], register) {
			return fmt.Errorf("transformer: Middle register inventory is not canonical")
		}
		registers[index].formalOrdinal = uint64(index) + 1
		bySlot[register.slot] = index
	}
	s.registers, s.bySlot, s.sealed = registers, bySlot, true
	return nil
}

func (s relationMiddleRegisterSchema) validRoot(root Root) bool {
	return s.sealed && root.Kind == RootMiddle && uint64(root.Index) < uint64(len(s.registers))
}

func (s relationMiddleRegisterSchema) root(slot statekey.Value) (Root, bool) {
	index, ok := s.bySlot[slot]
	if !s.sealed || !ok || index < 0 || uint64(index) > math.MaxUint32 {
		return Root{}, false
	}
	return Root{Kind: RootMiddle, Index: uint32(index)}, true
}

func (s relationMiddleRegisterSchema) register(root Root) (relationMiddleRegister, bool) {
	if !s.validRoot(root) {
		return relationMiddleRegister{}, false
	}
	return s.registers[root.Index], true
}

// inputRoot returns the unique invocation-boundary root which seeds middle.
// Unseeded local registers deliberately have no boundary provenance.
func (s relationMiddleRegisterSchema) inputRoot(middle Root) (Root, bool) {
	if !s.validRoot(middle) {
		return Root{}, false
	}
	index := sort.Search(len(s.entries), func(index int) bool {
		return s.entries[index].middle.Index >= middle.Index
	})
	if index >= len(s.entries) || s.entries[index].middle != middle {
		return Root{}, false
	}
	return s.entries[index].input, true
}

// middleInputPath returns the concrete lexical path which seeded a Middle
// register. This is the sole MID -> IN provenance inverse used at boundary
// specialization; local registers intentionally have no result.
func (b *relationProgramBody) middleInputPath(middle Root) (pathdom.Path, bool) {
	if b == nil || b.relation.arena == nil {
		return pathdom.Path{}, false
	}
	input, exact := b.relation.arena.middle.inputRoot(middle)
	if !exact {
		return pathdom.Path{}, false
	}
	for _, carrier := range b.roots.roots {
		if carrier.root != input {
			continue
		}
		symbol := rootSymbol(carrier.slot)
		if symbol == 0 {
			return pathdom.Path{}, false
		}
		return pathdom.NewPath(symbol, ""), true
	}
	return pathdom.Path{}, false
}

func (s relationMiddleRegisterSchema) count() uint64 { return uint64(len(s.registers)) }

func (s *relationMiddleRegisterSchema) bindInputs(shape Shape, entries []relationMiddleEntry) error {
	if s == nil || !s.sealed || s.entries != nil {
		return fmt.Errorf("transformer: Middle input bindings require one sealed register schema")
	}
	bound := append([]relationMiddleEntry(nil), entries...)
	sort.Slice(bound, func(i, j int) bool { return bound[i].middle.Index < bound[j].middle.Index })
	for index, entry := range bound {
		register, valid := s.register(entry.middle)
		if !valid || register.kind != relationMiddleRegisterSymbol || !shape.validateInput(entry.input) {
			return fmt.Errorf("transformer: Middle input binding is outside its typed vocabulary")
		}
		if index != 0 && bound[index-1].middle == entry.middle {
			return fmt.Errorf("transformer: Middle register has two input bindings")
		}
	}
	s.entries = bound
	return nil
}

func (s relationMiddleRegisterSchema) formalRoot(owner lexicalidentity.StableLexicalBodyID, root Root) (formal.Root, bool) {
	// StableLexicalBodyID is currently a full-width 32-byte value. Keeping the
	// conversion here avoids making the term node carry a second root identity.
	if !s.validRoot(root) {
		return formal.Root{}, false
	}
	return formal.NewRoot(owner, s.registers[root.Index].formalOrdinal, formal.Middle), true
}

func (a *Arena) sealMiddleRegisterSchema() error {
	if a == nil || a.sealed {
		return fmt.Errorf("transformer: Middle register schema has no open arena")
	}
	return a.middle.seal(a.environment)
}

func (a *Arena) middleRoot(slot statekey.Value) (Root, bool) {
	if a == nil {
		return Root{}, false
	}
	return a.middle.root(slot)
}

func (a *Arena) middleValue(slot statekey.Value) (ValueTerm, bool) {
	root, ok := a.middleRoot(slot)
	if !ok {
		return 0, false
	}
	term := a.Root(root)
	return term, term != 0
}

func (a *Arena) middleSymbolPath(id symbol.ID, suffix ...segment.Segment) PathTerm {
	if a == nil || id == 0 {
		return 0
	}
	root, ok := a.middleRoot(statekey.SymbolValue(id))
	if !ok {
		return 0
	}
	register, ok := a.middle.register(root)
	if !ok || register.kind != relationMiddleRegisterSymbol || register.symbol != id {
		return 0
	}
	return a.Path(root, suffix...)
}
