package transferfacts

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type wirTableExprRefKey struct {
	id wir.ExpressionID
}

func (l *lowerer) objectLiteralEntriesFromWIR(inst wir.Instruction) []factflow.ObjectEntry {
	wirEntries := l.wir.TableEntries(inst.TableEntries)
	entries := make([]factflow.ObjectEntry, 0, len(wirEntries))
	for _, entry := range wirEntries {
		source, ok := l.objectEntryValueSourceFromWIR(inst.Point, entry.Value)
		if !ok {
			source = factflow.NewUnknownValueSource(factflow.NoValueSourceIndex)
		}
		entries = append(entries, factflow.NewObjectEntryWithMetadata(
			entry.Suffix,
			source,
			sourceSpanFromWIR(entry.ValueSpan),
			entry.ValueLabel,
		))
	}
	return entries
}

func (l *lowerer) objectLiteralListElementSourceFromWIR(inst wir.Instruction) (factflow.ValueSource, bool) {
	if !inst.ListSpread {
		return factflow.ValueSource{}, false
	}
	ops := l.wir.Operands(inst.List)
	if len(ops) == 0 {
		return factflow.ValueSource{}, false
	}
	return l.valueSourceFromWIROperand(
		ops[len(ops)-1],
		len(ops)-1,
		factflow.NoValueSourceIndex,
		true,
		true,
		true,
	)
}

func (l *lowerer) objectEntryValueSourceFromWIR(point cfg.Point, op wir.Operand) (factflow.ValueSource, bool) {
	if p, ok := l.wirPathOperand(op, false, wir.SymbolLocal, wir.SymbolParam, wir.SymbolGlobal, wir.SymbolUpvalue); ok {
		witness, _ := l.aliasPathType(p)
		return l.wirPathExpressionSourceWithShape("object-entry", point, p, witness, -1, -1, false, false, false, false)
	}
	if op.Kind == wir.OperandTemp {
		if inst, ok := l.wirTempDefs()[op.Ref]; ok && inst.Op == wir.OpMakeTable {
			return l.wirTableExpressionValueSourceWithShape(inst, -1, -1, false, false, false, false)
		}
	}
	return l.valueSourceFromWIROperand(op, -1, -1, false, false, false)
}

func (l *lowerer) addObjectLiteralFromWIR(input *factflow.FactsInput, inst wir.Instruction) {
	if input == nil || inst.Op != wir.OpMakeTable {
		return
	}
	exprRef, hasExpr := l.tableConstructorExprRefFromWIR(inst)
	if !hasExpr {
		return
	}
	lowered := factflow.NewObjectLiteral(l.objectLiteralEntriesFromWIR(inst)).
		WithExpressionID(uint64(inst.ExprID)).
		WithIdentity(identity.LuaTableLiteral(l.graphID, uint64(exprRef)))
	if inst.ExprSpan.Valid() {
		lowered = lowered.WithSpan(sourceSpanFromWIR(inst.ExprSpan))
	}
	if inst.StaticStringKeysComplete {
		lowered = lowered.WithStaticStringKeysComplete()
	}
	if source, ok := l.objectLiteralListElementSourceFromWIR(inst); ok {
		lowered = lowered.WithListElementSource(source)
	}
	if expected, ok := l.wirObjectLiteralExpectedType(inst); ok {
		lowered = l.objectLiteralWithExpectedType(lowered, expected)
	}
	if input.ObjectLiterals == nil {
		input.ObjectLiterals = make(map[factflow.ExprRef]factflow.ObjectLiteral)
	}
	input.ObjectLiterals[exprRef] = lowered
	l.addNestedObjectLiteralsFromWIR(input, inst)
	if expected, ok := l.wirObjectLiteralExpectedType(inst); ok {
		l.setObjectLiteralExpectedExpressionValue(exprRef, lowered, expected)
		l.addNestedObjectLiteralExpectedTypes(input, lowered, expected)
	}
}

func (l *lowerer) addObjectLiteralsFromWIR(input *factflow.FactsInput) {
	if input == nil {
		return
	}
	for i := 0; i < l.wir.Len(); i++ {
		inst := l.wir.Instr(i)
		if inst.Op != wir.OpMakeTable {
			continue
		}
		l.addObjectLiteralFromWIR(input, inst)
	}
}

func (l *lowerer) addNestedObjectLiteralsFromWIR(input *factflow.FactsInput, inst wir.Instruction) {
	if input == nil {
		return
	}
	tempDefs := l.wirTempDefs()
	for _, entry := range l.wir.TableEntries(inst.TableEntries) {
		if entry.Value.Kind != wir.OperandTemp {
			continue
		}
		nested, ok := tempDefs[entry.Value.Ref]
		if !ok || nested.Op != wir.OpMakeTable {
			continue
		}
		l.addObjectLiteralFromWIR(input, nested)
	}
}

func (l *lowerer) wirObjectLiteralExpectedType(inst wir.Instruction) (typ.Type, bool) {
	if inst.Type == 0 {
		return nil, false
	}
	expected := l.wir.Type(inst.Type)
	if expected == nil || !luatypeprojection.ReachesTableContract(expected) {
		return nil, false
	}
	return expected, true
}

func dynamicIndexMapValueType(container typ.Type, depth int) (typ.Type, bool) {
	if container == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch t := unwrap.Alias(container).(type) {
	case *typ.Optional:
		return dynamicIndexMapValueType(t.Inner, depth+1)
	case *typ.Map:
		return t.Value, t.Value != nil
	case *typ.ReadonlyMap:
		return t.Value, t.Value != nil
	case *typ.Record:
		if t.HasMapComponent() {
			return t.MapValue, t.MapValue != nil
		}
		return nil, false
	case *typ.Union:
		values := make([]typ.Type, 0, len(t.Members))
		for _, member := range t.Members {
			value, ok := dynamicIndexMapValueType(member, depth+1)
			if !ok {
				return nil, false
			}
			values = append(values, value)
		}
		return normalize.UnionForEvidence(values...), len(values) != 0
	default:
		return nil, false
	}
}

func (l *lowerer) addNestedObjectLiteralExpectedTypes(input *factflow.FactsInput, lit factflow.ObjectLiteral, expected typ.Type) {
	if input == nil || input.ObjectLiterals == nil || expected == nil {
		return
	}
	rootType := luatypeprojection.PresentConstructorRoot(expected)
	lit.View().ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		source := entry.Source()
		projected, ok := luatypeprojection.ExpectedConstructorEntryType(rootType, entry.SuffixSegmentsView())
		if !ok || projected == nil || !luatypeprojection.ReachesTableContract(projected) {
			return true
		}
		l.addObjectLiteralExpectedTypeFromExpressionSource(input, source, projected, nil)
		return true
	})
}

func (l *lowerer) addObjectLiteralExpectedTypeFromExpressionSource(input *factflow.FactsInput, source factflow.ValueSource, expected typ.Type, seen map[factflow.ExprRef]bool) {
	if input == nil ||
		input.ObjectLiterals == nil ||
		expected == nil ||
		!source.HasExpr ||
		source.ExprRef == 0 ||
		!luatypeprojection.ReachesTableContract(expected) {
		return
	}
	if seen[source.ExprRef] {
		return
	}
	if seen == nil {
		seen = make(map[factflow.ExprRef]bool)
	}
	seen[source.ExprRef] = true
	if nested, ok := input.ObjectLiterals[source.ExprRef]; ok {
		updated := l.objectLiteralWithExpectedType(nested, expected)
		input.ObjectLiterals[source.ExprRef] = updated
		l.setObjectLiteralExpectedExpressionValue(source.ExprRef, updated, expected)
		l.addNestedObjectLiteralExpectedTypes(input, updated, expected)
		return
	}
	op, ok := l.expressionOperations[source.ExprRef]
	if !ok {
		return
	}
	switch op.Kind() {
	case factflow.ExpressionOperationBinary:
		if op.Op() != "and" && op.Op() != "or" {
			return
		}
		l.addObjectLiteralExpectedTypeFromExpressionSource(input, op.Left(), expected, seen)
		l.addObjectLiteralExpectedTypeFromExpressionSource(input, op.Right(), expected, seen)
		if !l.addWIRLogicalExpressionOperationValue(source.ExprRef, op, op.Left(), op.Right()) {
			l.addWIRExpressionOperationValue(source.ExprRef, op, op.Left(), op.Right())
		}
	}
}

func (l *lowerer) setObjectLiteralExpectedExpressionValue(ref factflow.ExprRef, lit factflow.ObjectLiteral, expected typ.Type) {
	if ref == 0 || expected == nil {
		return
	}
	rootType := luatypeprojection.PresentConstructorRoot(expected)
	value := l.valueFromTypeWithWitness(rootType)
	if id, ok := lit.Identity(); ok {
		value = product.Set(l.registry, value, identity.Key, identity.Singleton(id))
	}
	if l.expressionValues == nil {
		l.expressionValues = make(map[factflow.ExprRef]product.Value)
	}
	l.expressionValues[ref] = value
}

func (l *lowerer) addReturnObjectLiteralExpectedTypesFromWIR(input *factflow.FactsInput, sources []factflow.ValueSource) {
	if input == nil {
		return
	}
	declared := l.wir.DeclaredReturnTypes()
	if len(declared) == 0 {
		return
	}
	for i, source := range sources {
		if i >= len(declared) ||
			declared[i] == nil ||
			!source.HasExpr ||
			source.ExprRef == 0 ||
			!luatypeprojection.ReachesTableContract(declared[i]) {
			continue
		}
		lit, ok := input.ObjectLiterals[source.ExprRef]
		if !ok {
			continue
		}
		updated := l.objectLiteralWithExpectedType(lit, declared[i])
		input.ObjectLiterals[source.ExprRef] = updated
		l.setObjectLiteralExpectedExpressionValue(source.ExprRef, updated, declared[i])
		l.addNestedObjectLiteralExpectedTypes(input, updated, declared[i])
	}
}

func (l *lowerer) addObjectLiteralExpectedTypeFromValueSource(input *factflow.FactsInput, source factflow.ValueSource, expected typ.Type) {
	if input == nil ||
		expected == nil ||
		!source.HasExpr ||
		source.ExprRef == 0 ||
		!luatypeprojection.ReachesTableContract(expected) {
		return
	}
	l.addObjectLiteralExpectedTypeFromExpressionSource(input, source, expected, nil)
}

func (l *lowerer) addObjectLiteralFieldExposuresFromWIR(input *factflow.FactsInput, point cfg.Point, inst wir.Instruction, declared typ.Type) {
	if input == nil || inst.Op != wir.OpMakeTable || declared == nil {
		return
	}
	if typ.IsAny(declared) || typ.IsUnknown(declared) {
		return
	}
	exprRef, ok := l.tableConstructorExprRefFromWIR(inst)
	if !ok {
		return
	}
	lit, ok := input.ObjectLiterals[exprRef]
	if !ok {
		return
	}
	lit.View().ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		slotType, ok := luatypeprojection.ApplySegments(declared, entry.SuffixSegmentsView())
		if !ok || slotType == nil {
			return true
		}
		l.addAliasExposureValueSourceToContractType(input, point, entry.Source(), slotType)
		return true
	})
}

func (l *lowerer) tableConstructorExprRefFromWIR(inst wir.Instruction) (factflow.ExprRef, bool) {
	if inst.ExprID == 0 {
		return 0, false
	}
	return l.exprRef(wirTableExprRefKey{id: inst.ExprID})
}

func sourceSpanFromWIR(span wir.Span) factflow.SourceSpan {
	return factflow.SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func (l *lowerer) objectLiteralWithExpectedType(lit factflow.ObjectLiteral, declared typ.Type) factflow.ObjectLiteral {
	rootType := luatypeprojection.PresentConstructorRoot(declared)
	root := l.valueFromTypeWithWitness(rootType)
	entries := make([]factflow.ObjectEntry, 0, lit.View().EntryCount())
	lit.View().ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		out := factflow.NewObjectEntryWithMetadata(entry.Suffix(), entry.Source(), entry.ValueSpan(), entry.ValueLabel())
		if expected, ok := entry.Expected(); ok {
			out = out.WithExpected(expected)
		}
		if projected, ok := luatypeprojection.ExpectedConstructorEntryType(rootType, entry.SuffixSegmentsView()); ok && projected != nil {
			out = out.WithExpected(l.valueFromTypeWithWitness(projected))
		}
		entries = append(entries, out)
		return true
	})
	out := factflow.NewObjectLiteral(entries).WithExpected(root)
	if id, ok := lit.Identity(); ok {
		out = out.WithIdentity(id)
	}
	if id, ok := lit.ExpressionID(); ok {
		out = out.WithExpressionID(id)
	}
	if span, ok := lit.Span(); ok {
		out = out.WithSpan(span)
	}
	if source, ok := lit.ListElementSource(); ok {
		out = out.WithListElementSource(source)
	}
	if lit.StaticStringKeysComplete() {
		out = out.WithStaticStringKeysComplete()
	}
	return out
}
