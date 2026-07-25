package engine

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// metatableNativeFacts publishes the metatable seal of every table binding the
// module builds. A binding is a lexical symbol, so the analysis spans every
// lexical body of the module at once: a literal built in the enclosing body and
// read through a capture is the same allocation, and a metatable installed
// anywhere reaches every read of it.
//
// Two seals exist and neither is a snapshot. A binding no `setmetatable` call
// ever names has a proven-absent metatable, so each read of one of its members
// elides the `__index` walk until a metatable is installed. A binding a
// `setmetatable` call does name is sealed to the exact index table it was given,
// and that seal ends when the metatable is replaced or the index table itself is
// mutated.
func metatableNativeFacts(root front.Compilation) []NativeFact {
	state := newMetatableState()
	forEachNativeBody(root, state.observe)
	var rows []NativeFact
	forEachNativeBody(root, func(compilation front.Compilation) {
		rows = append(rows, state.bodyFacts(compilation)...)
	})
	return rows
}

func forEachNativeBody(root front.Compilation, visit func(front.Compilation)) {
	var walk func(front.Compilation)
	walk = func(compilation front.Compilation) {
		if compilation.WIR != nil {
			visit(compilation)
		}
		for _, child := range compilation.Nested {
			walk(child)
		}
	}
	walk(root)
}

// metatableState is the module-wide binding evidence the seals rest on. Every
// entry is keyed by lexical symbol, which is the only identity shared across
// the bodies that capture it.
type metatableState struct {
	constructed  map[wir.SymbolID]bool
	memberWrite  map[wir.SymbolID]bool
	metatabled   map[wir.SymbolID]bool
	indexTable   map[wir.SymbolID]string
	entryIndexes map[string]string
}

func newMetatableState() *metatableState {
	return &metatableState{
		constructed:  make(map[wir.SymbolID]bool),
		memberWrite:  make(map[wir.SymbolID]bool),
		metatabled:   make(map[wir.SymbolID]bool),
		indexTable:   make(map[wir.SymbolID]string),
		entryIndexes: make(map[string]string),
	}
}

func (s *metatableState) observe(compilation front.Compilation) {
	body := compilation.WIR
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpMakeTable:
			if item, root := nativeRootBinding(body, instruction.Dst); root {
				s.constructed[item.Symbol] = true
			}
			if name, found := nativeConstructorIndexTable(body, instruction); found {
				if item, root := nativeRootBinding(body, instruction.Dst); root {
					s.indexTable[item.Symbol] = name
				}
				if instruction.Dst.Kind == wir.OperandTemp {
					s.entryIndexes[nativeTempKey(compilation, instruction.Dst)] = name
				}
			}
		case wir.OpStaticMemberWrite:
			if instruction.Dst.Kind != wir.OperandPath {
				continue
			}
			target := body.Path(wir.PathRef(instruction.Dst.Ref))
			if len(target.Segments) == 0 {
				continue
			}
			s.memberWrite[target.Symbol] = true
			if field, named := target.LastSegment(); named && field.Kind == segment.SegmentField && field.Name == "__index" {
				if source, root := nativeRootBinding(body, instruction.A); root {
					s.indexTable[target.Symbol] = source.String()
				}
			}
		case wir.OpCall:
			if nativeCallName(body, instruction) != "setmetatable" {
				continue
			}
			arguments := body.Operands(instruction.List)
			if len(arguments) < 1 {
				continue
			}
			if receiver, root := nativeRootBinding(body, arguments[0]); root {
				s.metatabled[receiver.Symbol] = true
			}
		}
	}
}

func (s *metatableState) bodyFacts(compilation front.Compilation) []NativeFact {
	body := compilation.WIR
	row := func(key, occurrence, subject, content, revocation string) NativeFact {
		return NativeFact{
			Lane: NativeLaneValues, Family: "metatable_seal",
			Key:   "metatable_seal/" + fmt.Sprintf("%x", compilation.Body) + "/" + key + "/contract-revocation/" + revocation,
			Value: content, Subject: subject, Occurrence: occurrence, Trust: NativeTrustProven,
		}
	}

	var out []NativeFact
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		occurrence := fmt.Sprintf("op-%08d", index)
		if instruction.Op == wir.OpCall && nativeCallName(body, instruction) == "setmetatable" {
			arguments := body.Operands(instruction.List)
			if len(arguments) < 2 {
				continue
			}
			receiver, root := nativeRootBinding(body, arguments[0])
			if !root {
				continue
			}
			name, named := s.installedIndexTable(compilation, body, arguments[1])
			if !named {
				continue
			}
			out = append(out, row(nativeSymbolKey(receiver.Symbol), occurrence, receiver.String(),
				"index_table="+name+" sealed=true", "meta.set,meta.mutate"))
			continue
		}
		for _, operand := range nativeInstructionOperands(body, instruction) {
			if operand.Kind != wir.OperandPath {
				continue
			}
			member := body.Path(wir.PathRef(operand.Ref))
			if len(member.Segments) == 0 || !s.absentProved(member.Symbol) {
				continue
			}
			out = append(out, row(string(member.Key()), occurrence, member.RootOnly().String(),
				"index_chain=elided metatable=absent", "meta.set"))
		}
	}
	return out
}

// absentProved holds only for a binding this module builds as a table, never
// mutates through a member write, and never passes to setmetatable. Any one of
// those makes the metatable-absent proof unavailable for the whole module
// generation.
func (s *metatableState) absentProved(symbol wir.SymbolID) bool {
	if symbol == 0 {
		return false
	}
	return s.constructed[symbol] && !s.memberWrite[symbol] && !s.metatabled[symbol]
}

// installedIndexTable names the table an installed metatable delegates to,
// whether the metatable is a named binding carrying an `__index` write or the
// constructor expression passed directly to the call.
func (s *metatableState) installedIndexTable(compilation front.Compilation, body *wir.Body, operand wir.Operand) (string, bool) {
	if operand.Kind == wir.OperandTemp {
		name, found := s.entryIndexes[nativeTempKey(compilation, operand)]
		return name, found
	}
	item, root := nativeRootBinding(body, operand)
	if !root {
		return "", false
	}
	if name, found := s.indexTable[item.Symbol]; found {
		return name, true
	}
	return "", false
}

// nativeConstructorIndexTable reads the `__index` entry of a table constructor
// from the resolved constructor window, never from source text.
func nativeConstructorIndexTable(body *wir.Body, instruction wir.Instruction) (string, bool) {
	for _, entry := range body.TableEntries(instruction.TableEntries) {
		field, named := entry.Suffix.LastSegment()
		if !named || field.Kind != segment.SegmentField || field.Name != "__index" {
			continue
		}
		if source, root := nativeRootBinding(body, entry.Value); root {
			return source.String(), true
		}
	}
	return "", false
}

// nativeRootBinding is the operand's whole-binding path: a path operand with no
// member suffix, carrying a lexical symbol.
func nativeRootBinding(body *wir.Body, operand wir.Operand) (path.Path, bool) {
	if operand.Kind != wir.OperandPath {
		return path.Path{}, false
	}
	item := body.Path(wir.PathRef(operand.Ref))
	if len(item.Segments) != 0 || item.Symbol == 0 {
		return path.Path{}, false
	}
	return item, true
}

func nativeSymbolKey(symbol wir.SymbolID) string {
	return fmt.Sprintf("sym%d", symbol)
}

func nativeTempKey(compilation front.Compilation, operand wir.Operand) string {
	return fmt.Sprintf("%x/temp/%d", compilation.Body, operand.Ref)
}

// nativeInstructionOperands yields every value operand an instruction reads.
func nativeInstructionOperands(body *wir.Body, instruction wir.Instruction) []wir.Operand {
	operands := make([]wir.Operand, 0, 4)
	if instruction.Op != wir.OpStaticMemberWrite && instruction.Op != wir.OpDynamicIndexWrite {
		operands = append(operands, instruction.Dst)
	}
	operands = append(operands, instruction.A, instruction.B)
	operands = append(operands, body.Operands(instruction.List)...)
	if instruction.Call.Callee.Kind != wir.OperandNone {
		operands = append(operands, instruction.Call.Callee)
	}
	if instruction.Call.Receiver.Kind != wir.OperandNone {
		operands = append(operands, instruction.Call.Receiver)
	}
	return operands
}
