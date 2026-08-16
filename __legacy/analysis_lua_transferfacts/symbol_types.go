package transferfacts

import (
	"github.com/wippyai/go-lua/__legacy/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typecall"
)

func lowerSymbolTypesFromWIR(body *wir.Body, moduleExports importlookup.Source) map[symbol.ID]typ.Type {
	if body == nil {
		return nil
	}
	out := make(map[symbol.ID]typ.Type)
	addWIRRequireExportSymbolTypes(out, body, moduleExports)
	addWIRRootTypes(out, body)
	tempDefs := wirTempDefinitions(body)
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		switch inst.Op {
		case wir.OpClaim:
			if inst.Type == 0 || inst.Claim != wir.ClaimAnnotation {
				continue
			}
		case wir.OpMakeTable:
			if inst.Assign != wir.AssignLocalDeclaration {
				continue
			}
		case wir.OpAssign:
			if inst.Assign != wir.AssignLocalDeclaration {
				continue
			}
		case wir.OpCall:
			addWIRCallResultSymbolTypes(out, body, inst)
			continue
		case wir.OpClosure:
			addWIRClosureSymbolType(out, body, inst)
			continue
		case wir.OpIterate:
			addWIRNumericForSymbolType(out, body, tempDefs, inst)
			continue
		default:
			continue
		}
		if inst.Dst.Kind != wir.OperandPath {
			continue
		}
		p := body.Path(wir.PathRef(inst.Dst.Ref))
		if p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 {
			continue
		}
		t := body.Type(inst.Type)
		if t == nil && inst.Op == wir.OpAssign && inst.Assign == wir.AssignLocalDeclaration {
			if source, ok := inst.AssignmentSourceOperand(); ok && source.Kind == wir.OperandPath {
				if inferred, ok := wirPathTypeFromSymbols(out, body.Path(wir.PathRef(source.Ref))); ok &&
					inferred != nil &&
					!typ.IsAny(inferred) &&
					!typ.IsUnknown(inferred) {
					t = inferred
				}
			}
		}
		if t == nil && inst.Op == wir.OpMakeTable {
			t, _ = wirObjectLiteralTypeFromSymbols(out, body, tempDefs, inst, nil)
		}
		if t == nil {
			continue
		}
		out[p.Symbol] = t
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addWIRRootTypes(out map[symbol.ID]typ.Type, body *wir.Body) {
	if out == nil || body == nil {
		return
	}
	for _, root := range body.RootTypes() {
		if root.Path.IsEmpty() || root.Path.Symbol == 0 || len(root.Path.Segments) != 0 {
			continue
		}
		if _, exists := out[root.Path.Symbol]; exists {
			continue
		}
		if t := body.Type(root.Type); t != nil {
			out[root.Path.Symbol] = t
		}
	}
}

func addWIRClosureSymbolType(out map[symbol.ID]typ.Type, body *wir.Body, inst wir.Instruction) {
	if out == nil || body == nil || inst.Op != wir.OpClosure || inst.Dst.Kind != wir.OperandPath {
		return
	}
	p := body.Path(wir.PathRef(inst.Dst.Ref))
	if p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 {
		return
	}
	if _, exists := out[p.Symbol]; exists {
		return
	}
	proto := body.Proto(inst.Func)
	if proto.Type == nil {
		return
	}
	out[p.Symbol] = proto.Type
}

func addWIRNumericForSymbolType(out map[symbol.ID]typ.Type, body *wir.Body, tempDefs map[uint32]wir.Instruction, inst wir.Instruction) {
	if out == nil || body == nil || inst.Op != wir.OpIterate || inst.Iter != wir.IterNumeric {
		return
	}
	results := body.Operands(inst.Results)
	if len(results) == 0 || results[0].Kind != wir.OperandPath {
		return
	}
	p := body.Path(wir.PathRef(results[0].Ref))
	if p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 {
		return
	}
	if _, exists := out[p.Symbol]; exists {
		return
	}
	if t := numericForLoopVariableTypeFromWIR(body, out, tempDefs, inst); t != nil {
		out[p.Symbol] = t
	}
}

func addWIRRequireExportSymbolTypes(out map[symbol.ID]typ.Type, body *wir.Body, moduleExports importlookup.Source) {
	if out == nil || body == nil {
		return
	}
	written := wirRootAssignmentSymbols(body)
	for id := range wirReferencedRootSymbols(body) {
		if _, exists := out[id]; exists || written[id] {
			continue
		}
		modulePath, ok := body.SymbolRequireModulePath(id)
		if !ok {
			continue
		}
		if t, ok := moduleExports.LookupExport(modulePath); ok {
			out[id] = t
		}
	}
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Op != wir.OpCall {
			continue
		}
		for _, target := range body.CallResultTargets(inst.Point) {
			if target.Kind != wir.CallResultTargetLocalAssignment ||
				target.Path.Symbol == 0 ||
				len(target.Path.Segments) != 0 ||
				target.ResultIndex != 0 {
				continue
			}
			if _, exists := out[target.Path.Symbol]; exists {
				continue
			}
			modulePath, ok := body.SymbolRequireModulePath(target.Path.Symbol)
			if !ok {
				continue
			}
			if t, ok := moduleExports.LookupExport(modulePath); ok {
				out[target.Path.Symbol] = t
			}
		}
	}
}

func wirReferencedRootSymbols(body *wir.Body) map[symbol.ID]bool {
	if body == nil {
		return nil
	}
	out := make(map[symbol.ID]bool)
	addOperandRootSymbol := func(op wir.Operand) {
		if op.Kind != wir.OperandPath {
			return
		}
		p := body.Path(wir.PathRef(op.Ref))
		if p.Symbol != 0 {
			out[p.Symbol] = true
		}
	}
	addOperandRangeRootSymbols := func(r wir.OperandRange) {
		for _, op := range body.Operands(r) {
			addOperandRootSymbol(op)
		}
	}
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		addOperandRootSymbol(inst.Dst)
		addOperandRootSymbol(inst.A)
		addOperandRootSymbol(inst.B)
		addOperandRootSymbol(inst.Call.Callee)
		addOperandRootSymbol(inst.Call.Receiver)
		addOperandRangeRootSymbols(inst.List)
		addOperandRangeRootSymbols(inst.Results)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func wirRootAssignmentSymbols(body *wir.Body) map[symbol.ID]bool {
	if body == nil {
		return nil
	}
	out := make(map[symbol.ID]bool)
	for i := 0; i < body.Len(); i++ {
		inst := body.Instr(i)
		if inst.Assign == wir.AssignNone || inst.Dst.Kind != wir.OperandPath {
			continue
		}
		p := body.Path(wir.PathRef(inst.Dst.Ref))
		if p.Symbol != 0 && len(p.Segments) == 0 {
			out[p.Symbol] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func wirObjectLiteralTypeFromSymbols(
	symbolTypes map[symbol.ID]typ.Type,
	body *wir.Body,
	tempDefs map[uint32]wir.Instruction,
	inst wir.Instruction,
	seen map[uint32]bool,
) (typ.Type, bool) {
	if body == nil || inst.Op != wir.OpMakeTable {
		return nil, false
	}
	if inst.Type != 0 {
		t := body.Type(inst.Type)
		return t, t != nil
	}
	builder := typetable.NewConstructorBuilder()
	anySeen := false
	for _, entry := range body.TableEntries(inst.TableEntries) {
		keys, ok := wirConstructorKeysFromSuffix(entry.Suffix)
		if !ok {
			continue
		}
		valueType, ok := wirConstructorValueTypeFromSymbols(symbolTypes, body, tempDefs, entry.Value, seen)
		if !ok || valueType == nil {
			continue
		}
		if !builder.Add(keys, valueType) {
			return nil, false
		}
		anySeen = true
	}
	if !anySeen {
		return nil, false
	}
	return builder.Build()
}

func wirConstructorValueTypeFromSymbols(
	symbolTypes map[symbol.ID]typ.Type,
	body *wir.Body,
	tempDefs map[uint32]wir.Instruction,
	op wir.Operand,
	seen map[uint32]bool,
) (typ.Type, bool) {
	if body == nil {
		return nil, false
	}
	switch op.Kind {
	case wir.OperandPath:
		return wirPathTypeFromSymbols(symbolTypes, body.Path(wir.PathRef(op.Ref)))
	case wir.OperandTemp:
		if seen == nil {
			seen = make(map[uint32]bool)
		}
		if seen[op.Ref] {
			return nil, false
		}
		def, ok := tempDefs[op.Ref]
		if !ok {
			return nil, false
		}
		seen[op.Ref] = true
		defer delete(seen, op.Ref)
		if def.Type != 0 {
			if t := body.Type(def.Type); t != nil {
				return t, true
			}
		}
		switch def.Op {
		case wir.OpAssign:
			return wirConstructorValueTypeFromSymbols(symbolTypes, body, tempDefs, def.A, seen)
		case wir.OpMakeTable:
			return wirObjectLiteralTypeFromSymbols(symbolTypes, body, tempDefs, def, seen)
		}
	}
	return nil, false
}

func wirPathTypeFromSymbols(symbolTypes map[symbol.ID]typ.Type, p path.Path) (typ.Type, bool) {
	if p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	rootType, ok := symbolTypes[p.Symbol]
	if !ok || rootType == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return rootType, true
	}
	return typeprojection.ApplySegments(rootType, p.Segments)
}

func wirConstructorKeysFromSuffix(suffix path.Path) ([]typetable.ConstructorKey, bool) {
	if len(suffix.Segments) == 0 {
		return nil, false
	}
	keys := make([]typetable.ConstructorKey, 0, len(suffix.Segments))
	for _, seg := range suffix.Segments {
		switch seg.Kind {
		case segment.SegmentField:
			keys = append(keys, typetable.ConstructorKey{Kind: typetable.ConstructorField, Name: seg.Name})
		case segment.SegmentIndexString:
			keys = append(keys, typetable.ConstructorKey{Kind: typetable.ConstructorStringIndex, Name: seg.Name})
		case segment.SegmentIndexInt:
			keys = append(keys, typetable.ConstructorKey{Kind: typetable.ConstructorIntIndex, Index: int64(seg.Index)})
		default:
			return nil, false
		}
	}
	return keys, true
}

func addWIRCallResultSymbolTypes(out map[symbol.ID]typ.Type, body *wir.Body, inst wir.Instruction) {
	if out == nil || body == nil || inst.Op != wir.OpCall {
		return
	}
	fn, ok := callableFromWIRCallType(body, inst)
	if !ok && inst.Type == 0 {
		fn, ok = callableFromWIRCallPathType(out, body, inst)
	}
	if !ok || fn == nil || len(fn.TypeParams) != 0 {
		return
	}
	for _, target := range body.CallResultTargets(inst.Point) {
		if target.Kind != wir.CallResultTargetLocalAssignment ||
			target.Path.Symbol == 0 ||
			len(target.Path.Segments) != 0 ||
			target.ResultIndex < 0 ||
			target.ResultIndex >= len(fn.Returns) ||
			fn.Returns[target.ResultIndex] == nil {
			continue
		}
		out[target.Path.Symbol] = fn.Returns[target.ResultIndex]
	}
}

func callableFromWIRCallPathType(symbolTypes map[symbol.ID]typ.Type, body *wir.Body, inst wir.Instruction) (*typ.Function, bool) {
	if body == nil || inst.Op != wir.OpCall || inst.Call.Callee.Kind != wir.OperandPath {
		return nil, false
	}
	p := body.Path(wir.PathRef(inst.Call.Callee.Ref))
	t, ok := wirPathTypeFromSymbols(symbolTypes, p)
	if !ok {
		return nil, false
	}
	fn, ok := t.(*typ.Function)
	return fn, ok && fn != nil
}

func callableFromWIRCallType(body *wir.Body, inst wir.Instruction) (*typ.Function, bool) {
	t := body.Type(inst.Type)
	if t == nil {
		return nil, false
	}
	if inst.Call.Method != 0 {
		method := body.Const(inst.Call.Method)
		if method.Kind != wir.ConstString || method.Str == "" {
			return nil, false
		}
		fn, _, ok := typecall.MemberCallable(t, method.Str)
		return fn, ok
	}
	return typecall.Callable(t)
}
