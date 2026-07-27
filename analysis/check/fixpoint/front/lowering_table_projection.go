package front

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/wir"
)

// The table publication joins allocation-owned ordered entry/capacity/spread
// and duplicate-child metadata, growth-owned preallocation/recurrence/escape
// closure, and length-owned density/meta invalidation into typed projection
// drafts. The drafts enter factkey.NativeProjection only at the semantic tail.
//
// tableNativeFacts projects the resolved table topology the front already
// publishes. It never reconstructs a table from source text: a missing maker,
// exact member window, loop iterator, or call operand leaves the native row
// absent. nested lexical bodies contribute drafts to the same publication tail.
func tableNativeFacts(root Compilation) []NativeProjection {
	var rows []NativeProjection
	forEachNativeBody(root, func(compilation Compilation) {
		rows = append(rows, tableBodyFacts(compilation)...)
	})
	return rows
}

func tableBodyFacts(compilation Compilation) []NativeProjection {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	row := func(family, occurrence, subject, content, revocation string) NativeProjection {
		key := family + "/" + fmt.Sprintf("%x", compilation.Body) + "/" + occurrence
		if revocation != "" {
			key += "/contract-revocation/" + revocation
		}
		return NativeProjection{Key: key, Value: content, Subject: subject, Occurrence: occurrence}
	}

	var out []NativeProjection
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpMakeTable || instruction.ListSpread {
			continue
		}
		occurrence := nativeOccurrence(index)
		if len(body.TableEntries(instruction.TableEntries)) == 0 && len(body.Operands(instruction.List)) == 0 {
			out = append(out, row("list_construction", occurrence, tableOperandSubject(body, instruction.Dst), "kind=empty_table", ""))
			continue
		}
		array, keys, exact := nativeTableCapacity(body, instruction)
		if !exact || array == 0 {
			continue
		}
		content := "capacity=" + strconv.Itoa(array) + " ordered_occurrences=" + strconv.Itoa(array) + " parent_allocation=published"
		if keys != 0 {
			content = "array_capacity=" + strconv.Itoa(array) + " entry_destinations=committed key_capacity=" + strconv.Itoa(keys)
		} else {
			if duplicates := nativeDuplicateListOperands(body, instruction); duplicates != 0 {
				content += " all_edges_closed=true duplicate_children=" + strconv.Itoa(duplicates) + " edges=" + strconv.Itoa(array)
			}
			if nativeFreshTableList(body, instruction) {
				content += " edges=" + strconv.Itoa(array) + " ownership=move write_barrier=required"
			}
		}
		out = append(out, row("list_construction", occurrence, tableOperandSubject(body, instruction.Dst), content, ""))
	}

	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpUnOp || instruction.Operator != wir.UnLen || instruction.A.Kind != wir.OperandPath {
			continue
		}
		table := body.Path(wir.PathRef(instruction.A.Ref))
		if nativeTableLengthProof(body, table, index) {
			for _, event := range []string{"write.element", "write.length", "meta.set", "call.opaque"} {
				out = append(out, row("table_length", nativeOccurrence(index), table.String(), "border_algorithm=canonical dense_prefix=true disposition=raw", event))
			}
		}
	}

	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpStaticMemberWrite || instruction.Dst.Kind != wir.OperandPath || !nativeNilOperand(body, instruction.A) {
			continue
		}
		member := body.Path(wir.PathRef(instruction.Dst.Ref))
		root, indexed := nativeIndexedParent(member)
		if !indexed || !nativeHasLengthRead(body, root, index+1) {
			continue
		}
		out = append(out, row("table_length", nativeOccurrence(index), root.String(), "disposition=withheld reason=sequence_border_changed", "write.length"))
	}

	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpCall || nativeCallName(body, instruction) != "setmetatable" {
			continue
		}
		arguments := body.Operands(instruction.List)
		if len(arguments) == 0 || arguments[0].Kind != wir.OperandPath {
			continue
		}
		table := body.Path(wir.PathRef(arguments[0].Ref))
		if nativeHasLengthRead(body, table, index+1) {
			out = append(out, row("table_length", nativeOccurrence(index), table.String(), "disposition=withheld reason=metamethod_possible", "meta.set"))
		}
	}

	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpDynamicIndexWrite || instruction.Dst.Kind != wir.OperandPath || !nativeLoopIndex(body, instruction.A) {
			continue
		}
		table := body.Path(wir.PathRef(instruction.Dst.Ref))
		if nativeTableEscapesAt(body, table, index) {
			continue
		}
		if capacity, preallocated := nativePreallocatedCapacity(body, table, index); preallocated {
			out = append(out, row("table_growth", nativeOccurrence(index), table.String(), "capacity="+strconv.Itoa(capacity)+" growth=absent", ""))
			continue
		}
		if !nativeMadeTable(body, table) {
			continue
		}
		for _, event := range []string{"escape", "meta.set", "call.opaque", "load.dynamic"} {
			out = append(out, row("table_growth", nativeOccurrence(index), table.String(), "occurrence_mode=repeatable retirement=array_or_hash rollback=published throw_inventory=complete", event))
		}
	}
	return out
}

func nativeOccurrence(index int) string { return fmt.Sprintf("op-%08d", index) }

func tableOperandSubject(body *wir.Body, operand wir.Operand) string {
	if operand.Kind != wir.OperandPath {
		return ""
	}
	return body.Path(wir.PathRef(operand.Ref)).String()
}

func nativeTableCapacity(body *wir.Body, instruction wir.Instruction) (array, keys int, exact bool) {
	positions := make(map[int]bool)
	for _, entry := range body.TableEntries(instruction.TableEntries) {
		if len(entry.Suffix.Segments) != 1 {
			continue
		}
		item := entry.Suffix.Segments[0]
		switch item.Kind {
		case segment.SegmentIndexInt:
			if item.Index <= 0 || positions[item.Index] {
				return 0, 0, false
			}
			positions[item.Index] = true
		case segment.SegmentField, segment.SegmentIndexString:
			keys++
		}
	}
	for index := 1; index <= len(positions); index++ {
		if !positions[index] {
			return 0, 0, false
		}
	}
	return len(positions), keys, true
}

func nativeDuplicateListOperands(body *wir.Body, instruction wir.Instruction) int {
	seen := make(map[wir.Operand]int)
	duplicates := 0
	for _, operand := range body.Operands(instruction.List) {
		seen[operand]++
	}
	for _, count := range seen {
		if count > 1 {
			duplicates += count - 1
		}
	}
	return duplicates
}

func nativeFreshTableList(body *wir.Body, instruction wir.Instruction) bool {
	makers := make(map[wir.Operand]bool)
	for index := 0; index < body.Len(); index++ {
		candidate := body.Instr(index)
		if candidate.Op == wir.OpMakeTable {
			makers[candidate.Dst] = true
		}
	}
	values := body.Operands(instruction.List)
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if !makers[value] {
			return false
		}
	}
	return true
}

func nativeTableLengthProof(body *wir.Body, table path.Path, before int) bool {
	if !nativeMadeTable(body, table) || nativeTableEscapesAt(body, table, before) {
		return false
	}
	for index := 0; index < before; index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpStaticMemberWrite && instruction.Dst.Kind == wir.OperandPath {
			member := body.Path(wir.PathRef(instruction.Dst.Ref))
			if root, indexed := nativeIndexedParent(member); indexed && root.EqualIgnoringVersion(table) {
				return false
			}
		}
	}
	return nativeDenseLoopWrite(body, table, before)
}

func nativeDenseLoopWrite(body *wir.Body, table path.Path, before int) bool {
	for index := 0; index < before; index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpDynamicIndexWrite || instruction.Dst.Kind != wir.OperandPath || !body.Path(wir.PathRef(instruction.Dst.Ref)).EqualIgnoringVersion(table) || !nativeLoopIndex(body, instruction.A) {
			continue
		}
		return true
	}
	return false
}

func nativeLoopIndex(body *wir.Body, operand wir.Operand) bool {
	if operand.Kind != wir.OperandPath {
		return false
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpIterate || instruction.Iter != wir.IterNumeric || !nativeStartsAtOne(body, instruction) {
			continue
		}
		for _, result := range body.Operands(instruction.Results) {
			if result == operand {
				return true
			}
		}
	}
	return false
}

func nativeStartsAtOne(body *wir.Body, instruction wir.Instruction) bool {
	values := body.Operands(instruction.List)
	return len(values) >= 1 && nativeProjectionIntegerConstant(body, values[0]) == 1
}

func nativeMadeTable(body *wir.Body, table path.Path) bool {
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpMakeTable && instruction.Dst.Kind == wir.OperandPath && body.Path(wir.PathRef(instruction.Dst.Ref)).EqualIgnoringVersion(table) {
			return true
		}
	}
	return false
}

func nativeTableEscapesAt(body *wir.Body, table path.Path, before int) bool {
	for index := 0; index < before; index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpCall {
			continue
		}
		for _, argument := range body.Operands(instruction.List) {
			if argument.Kind == wir.OperandPath && body.Path(wir.PathRef(argument.Ref)).EqualIgnoringVersion(table) {
				return true
			}
		}
	}
	return false
}

func nativePreallocatedCapacity(body *wir.Body, table path.Path, before int) (int, bool) {
	for index := 0; index < before; index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpAssign || instruction.Dst.Kind != wir.OperandPath || !body.Path(wir.PathRef(instruction.Dst.Ref)).EqualIgnoringVersion(table) || instruction.A.Kind != wir.OperandTemp {
			continue
		}
		for producerIndex := 0; producerIndex < index; producerIndex++ {
			producer := body.Instr(producerIndex)
			if producer.Op != wir.OpCall || len(body.Operands(producer.Results)) != 1 || body.Operands(producer.Results)[0] != instruction.A || nativeCallName(body, producer) != "table.create" {
				continue
			}
			arguments := body.Operands(producer.List)
			if len(arguments) == 0 {
				continue
			}
			capacity := nativeProjectionIntegerConstant(body, arguments[0])
			if capacity <= 0 || !nativeLoopLimit(body, capacity) {
				continue
			}
			return capacity, true
		}
	}
	return 0, false
}

func nativeLoopLimit(body *wir.Body, limit int) bool {
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpIterate || instruction.Iter != wir.IterNumeric || !nativeStartsAtOne(body, instruction) {
			continue
		}
		values := body.Operands(instruction.List)
		if len(values) >= 2 && nativeProjectionIntegerConstant(body, values[1]) == limit {
			return true
		}
	}
	return false
}

func nativeProjectionIntegerConstant(body *wir.Body, operand wir.Operand) int {
	if operand.Kind != wir.OperandConst {
		return 0
	}
	constant := body.Const(wir.ConstRef(operand.Ref))
	if constant.Kind != wir.ConstNumber {
		return 0
	}
	value, err := strconv.Atoi(constant.Number)
	if err != nil {
		return 0
	}
	return value
}

func nativeNilOperand(body *wir.Body, operand wir.Operand) bool {
	return operand.Kind == wir.OperandConst && body.Const(wir.ConstRef(operand.Ref)).Kind == wir.ConstNil
}

func nativeIndexedParent(member path.Path) (path.Path, bool) {
	last, found := member.LastSegment()
	if !found || last.Kind != segment.SegmentIndexInt {
		return path.Path{}, false
	}
	return member.Parent(), true
}

func nativeHasLengthRead(body *wir.Body, table path.Path, from int) bool {
	for index := from; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpUnOp && instruction.Operator == wir.UnLen && instruction.A.Kind == wir.OperandPath && body.Path(wir.PathRef(instruction.A.Ref)).EqualIgnoringVersion(table) {
			return true
		}
	}
	return false
}

// nativeCallName names the base-library binding a call targets. A source
// spelling does not name it on its own: a local of the same spelling renders
// identically, so the callee's root must carry the global binding the front
// recorded for that root's name. A call through any other binding names nothing
// here, which leaves a shadowed helper outside every contract the base library's
// own semantics license.
func nativeCallName(body *wir.Body, instruction wir.Instruction) string {
	if instruction.Call.Method != 0 || instruction.Call.Callee.Kind != wir.OperandPath {
		return ""
	}
	callee := body.Path(wir.PathRef(instruction.Call.Callee.Ref))
	spelling := callee.String()
	root, _, _ := strings.Cut(spelling, ".")
	if !body.SymbolResolvesToGlobal(callee.Symbol, root) {
		return ""
	}
	return strings.TrimPrefix(spelling, "_G.")
}
