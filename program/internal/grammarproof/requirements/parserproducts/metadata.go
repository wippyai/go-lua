package parserproducts

import (
	"fmt"
	goast "go/ast"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/program/internal/grammarproof"
	"github.com/wippyai/go-lua/program/internal/grammarproof/requirements/grammar"
)

// deriveHelperSummary admits the three finite range relations in parser.go.y.
// Their range-body writes are deliberately absent from HelperLaw.Edits: the
// map row is the one exact authority for each output coordinate.
func deriveHelperSummary(name string, function *goast.FuncDecl, builder *actionTermBuilder, scope *actionTermScope) (HelperSummary, error) {
	switch name {
	case "splitNameList":
		return deriveRangeSummary(name, function, builder, scope, []string{"names", "positions"}, []uint16{0, 1}, false)
	case "splitTypedNames":
		return deriveTypedNameSummary(function, builder, scope)
	case "toFuncParams":
		return deriveRangeSummary(name, function, builder, scope, []string{"params"}, []uint16{0}, false)
	}
	var rangeCount int
	goast.Inspect(function.Body, func(node goast.Node) bool {
		if _, ok := node.(*goast.RangeStmt); ok {
			rangeCount++
		}
		return true
	})
	if rangeCount != 0 {
		return HelperSummary{}, fmt.Errorf("unsupported helper range")
	}
	return HelperSummary{}, nil
}

func deriveRangeSummary(name string, function *goast.FuncDecl, builder *actionTermBuilder, scope *actionTermScope, targets []string, outputs []uint16, typedPresence bool) (HelperSummary, error) {
	if len(targets) != len(outputs) || len(targets) == 0 {
		return HelperSummary{}, fmt.Errorf("invalid map contract")
	}
	rangeStmt, err := oneRange(function.Body)
	if err != nil {
		return HelperSummary{}, err
	}
	entries, ok := rangeStmt.X.(*goast.Ident)
	if !ok || scope.formals[entries.Name] != 0 {
		return HelperSummary{}, fmt.Errorf("map range input is not formal zero")
	}
	value, ok := rangeStmt.Value.(*goast.Ident)
	if !ok || value.Name == "" {
		return HelperSummary{}, fmt.Errorf("map range has no item binding")
	}
	maps, err := deriveRangeMaps(name, rangeStmt, builder, scope, targets, outputs)
	if err != nil {
		return HelperSummary{}, err
	}
	summary := HelperSummary{Maps: maps}
	if typedPresence {
		summary.Presence = []ConditionalPresence{{Scope: scope.id, Output: 2, Predicate: PresencePredicateAnyNonNilField, Input: 0, ItemField: builder.symbol(ActionSymbolField, "Type")}}
	}
	return summary, nil
}

// splitTypedNames has a first always-present name/position map and a second
// type map guarded by hasTypes. Presence is the exact authority for the
// latter; range body assignments never escape as generic writes or products.
func deriveTypedNameSummary(function *goast.FuncDecl, builder *actionTermBuilder, scope *actionTermScope) (HelperSummary, error) {
	if function == nil || function.Body == nil {
		return HelperSummary{}, fmt.Errorf("splitTypedNames has no body")
	}
	var firstRange *goast.RangeStmt
	var presenceBranch *goast.IfStmt
	for _, statement := range function.Body.List {
		switch row := statement.(type) {
		case *goast.RangeStmt:
			if firstRange != nil {
				return HelperSummary{}, fmt.Errorf("splitTypedNames has multiple unconditional ranges")
			}
			firstRange = row
		case *goast.IfStmt:
			if presenceBranch != nil {
				return HelperSummary{}, fmt.Errorf("splitTypedNames has multiple type-presence branches")
			}
			presenceBranch = row
		}
	}
	if firstRange == nil || presenceBranch == nil || presenceBranch.Else != nil {
		return HelperSummary{}, fmt.Errorf("splitTypedNames has incomplete map control")
	}
	condition, ok := presenceBranch.Cond.(*goast.Ident)
	if !ok || condition.Name != "hasTypes" {
		return HelperSummary{}, fmt.Errorf("splitTypedNames does not guard types with hasTypes")
	}
	ranges := topLevelRanges(presenceBranch.Body)
	if len(ranges) != 1 {
		return HelperSummary{}, fmt.Errorf("splitTypedNames has %d conditional ranges, want one", len(ranges))
	}
	first, err := deriveRangeMaps("splitTypedNames", firstRange, builder, scope, []string{"names", "positions"}, []uint16{0, 1})
	if err != nil {
		return HelperSummary{}, err
	}
	second, err := deriveRangeMaps("splitTypedNames", ranges[0], builder, scope, []string{"types"}, []uint16{2})
	if err != nil {
		return HelperSummary{}, err
	}
	return HelperSummary{
		Maps: append(first, second...),
		Presence: []ConditionalPresence{{
			Scope:     scope.id,
			Output:    2,
			Predicate: PresencePredicateAnyNonNilField,
			Input:     0,
			ItemField: builder.symbol(ActionSymbolField, "Type"),
		}},
	}, nil
}

func deriveRangeMaps(owner string, rangeStmt *goast.RangeStmt, builder *actionTermBuilder, scope *actionTermScope, targets []string, outputs []uint16) ([]MapIndex, error) {
	if rangeStmt == nil || len(targets) != len(outputs) || len(targets) == 0 {
		return nil, fmt.Errorf("invalid map range contract")
	}
	entries, ok := rangeStmt.X.(*goast.Ident)
	if !ok || scope.formals[entries.Name] != 0 {
		return nil, fmt.Errorf("map range input is not formal zero")
	}
	value, ok := rangeStmt.Value.(*goast.Ident)
	if !ok || value.Name == "" {
		return nil, fmt.Errorf("map range has no item binding")
	}
	itemScope := builder.mapItemScope(owner, value.Name)
	assignments, err := rangeAssignments(rangeStmt, targets)
	if err != nil {
		return nil, err
	}
	maps := make([]MapIndex, len(targets))
	for index, target := range targets {
		term, termErr := builder.expression(&itemScope, assignments[target])
		if termErr != nil {
			return nil, termErr
		}
		maps[index] = MapIndex{Scope: scope.id, ItemScope: itemScope.id, Input: 0, Output: outputs[index], Element: term}
	}
	builder.closeScope(itemScope)
	return maps, nil
}

func oneRange(block *goast.BlockStmt) (*goast.RangeStmt, error) {
	ranges := topLevelRanges(block)
	if len(ranges) != 1 {
		return nil, fmt.Errorf("range helper has %d ranges, want one", len(ranges))
	}
	return ranges[0], nil
}

func topLevelRanges(block *goast.BlockStmt) []*goast.RangeStmt {
	var ranges []*goast.RangeStmt
	if block == nil {
		return ranges
	}
	for _, statement := range block.List {
		if row, ok := statement.(*goast.RangeStmt); ok {
			ranges = append(ranges, row)
		}
	}
	return ranges
}

func rangeAssignments(loop *goast.RangeStmt, targets []string) (map[string]goast.Expr, error) {
	result := make(map[string]goast.Expr, len(targets))
	for _, statement := range loop.Body.List {
		assignment, ok := statement.(*goast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		indexed, ok := assignment.Lhs[0].(*goast.IndexExpr)
		if !ok {
			continue
		}
		name, ok := indexed.X.(*goast.Ident)
		if !ok {
			continue
		}
		for _, target := range targets {
			if name.Name != target {
				continue
			}
			if result[target] != nil {
				return nil, fmt.Errorf("duplicate map output %s", target)
			}
			result[target] = assignment.Rhs[0]
		}
	}
	for _, target := range targets {
		if result[target] == nil {
			return nil, fmt.Errorf("missing map output %s", target)
		}
	}
	return result, nil
}

// deriveActionSequences is the one sequence denominator. Its source inputs
// are the current action AST and (only for wrapper forwarding) a helper AST;
// it never serializes or reparses an expression.
func deriveActionSequences(template grammarproof.ActionTemplate, block *goast.BlockStmt, builder *actionTermBuilder, scope *actionTermScope, carriers []grammarproof.SequenceCarrier, helpers map[string]*goast.FuncDecl) ([]SequenceLaw, error) {
	if len(carriers) == 0 {
		return nil, nil
	}
	result, fields, err := resultAssignments(block)
	if err != nil {
		return nil, err
	}
	rows := make([]SequenceLaw, 0, len(carriers))
	for _, carrier := range carriers {
		var expression goast.Expr
		if carrier.Field == "" {
			expression = result
		} else {
			expression = fields[carrier.Field]
			if expression == nil {
				if literal := rootCompositeLiteral(result); literal != nil {
					expression = goast.NewIdent("nil")
				} else if call, ok := result.(*goast.CallExpr); ok {
					law, found, helperErr := helperSequenceTyped(call, carrier.Field, helpers, builder, scope)
					if helperErr != nil {
						return nil, helperErr
					}
					if found {
						law.Production, law.Scope, law.Destination = template.Key, scope.id, SequenceDestination{Tag: carrier.Tag, Field: carrier.Field}
						rows = append(rows, law)
						continue
					}
				}
			}
		}
		if expression == nil {
			return nil, fmt.Errorf("no sequence disposition for %s.%s", carrier.Tag, carrier.Field)
		}
		construction, segments, ok, expressionErr := sequenceExpressionTyped(expression, builder, scope)
		if expressionErr != nil {
			return nil, expressionErr
		}
		if !ok {
			return nil, fmt.Errorf("nonsequence expression for %s.%s", carrier.Tag, carrier.Field)
		}
		rows = append(rows, SequenceLaw{Production: template.Key, Scope: scope.id, Destination: SequenceDestination{Tag: carrier.Tag, Field: carrier.Field}, Construction: construction, Segments: segments})
	}
	sort.Slice(rows, func(left, right int) bool { return rows[left].Destination.Field < rows[right].Destination.Field })
	return rows, nil
}

func resultAssignments(block *goast.BlockStmt) (goast.Expr, map[string]goast.Expr, error) {
	var result goast.Expr
	fields := make(map[string]goast.Expr)
	for _, statement := range block.List {
		assignment, ok := statement.(*goast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		if name, ok := assignment.Lhs[0].(*goast.Ident); ok && name.Name == "Result" {
			if result != nil {
				return nil, nil, fmt.Errorf("multiple top-level Result assignments")
			}
			result = assignment.Rhs[0]
			continue
		}
		selector, ok := assignment.Lhs[0].(*goast.SelectorExpr)
		if !ok {
			continue
		}
		base, ok := selector.X.(*goast.Ident)
		if !ok || base.Name != "Result" {
			continue
		}
		if fields[selector.Sel.Name] != nil {
			return nil, nil, fmt.Errorf("multiple top-level writes to sequence destination %s", selector.Sel.Name)
		}
		fields[selector.Sel.Name] = assignment.Rhs[0]
	}
	if literal := rootCompositeLiteral(result); literal != nil {
		for _, element := range literal.Elts {
			pair, ok := element.(*goast.KeyValueExpr)
			if !ok {
				continue
			}
			name, ok := pair.Key.(*goast.Ident)
			if !ok {
				continue
			}
			if fields[name.Name] != nil {
				return nil, nil, fmt.Errorf("multiple result field writes %s", name.Name)
			}
			fields[name.Name] = pair.Value
		}
	}
	return result, fields, nil
}

func sequenceExpressionTyped(expression goast.Expr, builder *actionTermBuilder, scope *actionTermScope) (SequenceConstruction, []SequenceSegment, bool, error) {
	if ident, ok := expression.(*goast.Ident); ok && ident.Name == "nil" {
		return SequenceConstructionNil, nil, true, nil
	}
	if literal, ok := expression.(*goast.CompositeLit); ok {
		if _, slice := literal.Type.(*goast.ArrayType); !slice {
			return SequenceConstructionInvalid, nil, false, nil
		}
		segments := make([]SequenceSegment, 0, len(literal.Elts))
		for _, element := range literal.Elts {
			term, err := builder.expression(scope, element)
			if err != nil {
				return SequenceConstructionInvalid, nil, false, err
			}
			segments = append(segments, SequenceSegment{Kind: SequenceElement, Term: term})
		}
		return SequenceConstructionLiteral, segments, true, nil
	}
	call, ok := expression.(*goast.CallExpr)
	if !ok {
		term, err := builder.expression(scope, expression)
		if err != nil {
			return SequenceConstructionInvalid, nil, false, err
		}
		return SequenceConstructionForward, []SequenceSegment{{Kind: SequenceSpread, Term: term}}, true, nil
	}
	name, ok := call.Fun.(*goast.Ident)
	if !ok || name.Name != "append" || len(call.Args) < 2 {
		return SequenceConstructionInvalid, nil, false, nil
	}
	segments := make([]SequenceSegment, 0, len(call.Args))
	for index, argument := range call.Args {
		if index == 0 {
			if literal, ok := argument.(*goast.CompositeLit); ok {
				if _, slice := literal.Type.(*goast.ArrayType); slice {
					for _, element := range literal.Elts {
						term, err := builder.expression(scope, element)
						if err != nil {
							return SequenceConstructionInvalid, nil, false, err
						}
						segments = append(segments, SequenceSegment{Kind: SequenceElement, Term: term})
					}
					continue
				}
			}
		}
		term, err := builder.expression(scope, argument)
		if err != nil {
			return SequenceConstructionInvalid, nil, false, err
		}
		kind := SequenceElement
		if index == 0 || call.Ellipsis.IsValid() && index == len(call.Args)-1 {
			kind = SequenceSpread
		}
		segments = append(segments, SequenceSegment{Kind: kind, Term: term})
	}
	return SequenceConstructionAppend, segments, true, nil
}

func helperSequenceTyped(call *goast.CallExpr, field string, helpers map[string]*goast.FuncDecl, builder *actionTermBuilder, scope *actionTermScope) (SequenceLaw, bool, error) {
	name, ok := call.Fun.(*goast.Ident)
	if !ok || helpers[name.Name] == nil {
		return SequenceLaw{}, false, nil
	}
	function := helpers[name.Name]
	formals, err := helperFormalNames(function)
	if err != nil {
		return SequenceLaw{}, false, err
	}
	if len(formals) != len(call.Args) {
		return SequenceLaw{}, false, fmt.Errorf("helper %s call arity differs from declaration", name.Name)
	}
	actual := make(map[string]goast.Expr, len(formals))
	for index, formal := range formals {
		actual[formal] = call.Args[index]
	}
	returns := allReturns(function.Body)
	if len(returns) != 1 || len(returns[0].Results) != 1 {
		return SequenceLaw{}, false, nil
	}
	returned, ok := returns[0].Results[0].(*goast.Ident)
	if !ok || actual[returned.Name] == nil {
		return SequenceLaw{}, false, nil
	}
	writes := helperFieldWrites(function.Body, returned.Name, field)
	if len(writes) > 1 {
		return SequenceLaw{}, false, fmt.Errorf("helper %s writes %s more than once", name.Name, field)
	}
	if len(writes) == 0 {
		expression, found := wrapperField(actual[returned.Name], field)
		if !found {
			return SequenceLaw{}, false, fmt.Errorf("helper %s has no wrapper field %s", name.Name, field)
		}
		construction, segments, sequence, sequenceErr := sequenceExpressionTyped(expression, builder, scope)
		if sequenceErr != nil || !sequence {
			return SequenceLaw{}, false, sequenceErr
		}
		return SequenceLaw{Construction: construction, Segments: segments}, true, nil
	}
	expression, substituteErr := substituteActionExpr(writes[0], actual)
	if substituteErr != nil {
		return SequenceLaw{}, false, substituteErr
	}
	construction, segments, sequence, sequenceErr := sequenceExpressionTyped(expression, builder, scope)
	if sequenceErr != nil || !sequence {
		return SequenceLaw{}, false, sequenceErr
	}
	return SequenceLaw{Construction: construction, Segments: segments}, true, nil
}

func helperFieldWrites(block *goast.BlockStmt, receiver, field string) []goast.Expr {
	var result []goast.Expr
	goast.Inspect(block, func(node goast.Node) bool {
		assignment, ok := node.(*goast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return true
		}
		selector, ok := assignment.Lhs[0].(*goast.SelectorExpr)
		if !ok || selector.Sel.Name != field {
			return true
		}
		base, ok := selector.X.(*goast.Ident)
		if ok && base.Name == receiver {
			result = append(result, assignment.Rhs[0])
		}
		return true
	})
	return result
}

func wrapperField(expression goast.Expr, field string) (goast.Expr, bool) {
	if literal := rootCompositeLiteral(expression); literal != nil {
		for _, element := range literal.Elts {
			pair, ok := element.(*goast.KeyValueExpr)
			if !ok {
				continue
			}
			name, ok := pair.Key.(*goast.Ident)
			if ok && name.Name == field {
				return pair.Value, true
			}
		}
		return goast.NewIdent("nil"), true
	}
	return &goast.SelectorExpr{X: expression, Sel: goast.NewIdent(field)}, true
}

// substituteActionExpr is a closed AST copier for helper wrapper values. It
// substitutes formals with caller expressions without rendering either one.
func substituteActionExpr(expression goast.Expr, actual map[string]goast.Expr) (goast.Expr, error) {
	switch node := expression.(type) {
	case *goast.Ident:
		if replacement := actual[node.Name]; replacement != nil {
			return replacement, nil
		}
		return &goast.Ident{Name: node.Name}, nil
	case *goast.BasicLit:
		return &goast.BasicLit{Kind: node.Kind, Value: node.Value}, nil
	case *goast.SelectorExpr:
		base, err := substituteActionExpr(node.X, actual)
		if err != nil {
			return nil, err
		}
		return &goast.SelectorExpr{X: base, Sel: goast.NewIdent(node.Sel.Name)}, nil
	case *goast.IndexExpr:
		base, err := substituteActionExpr(node.X, actual)
		if err != nil {
			return nil, err
		}
		index, err := substituteActionExpr(node.Index, actual)
		if err != nil {
			return nil, err
		}
		return &goast.IndexExpr{X: base, Index: index}, nil
	case *goast.ParenExpr:
		child, err := substituteActionExpr(node.X, actual)
		if err != nil {
			return nil, err
		}
		return &goast.ParenExpr{X: child}, nil
	case *goast.UnaryExpr:
		child, err := substituteActionExpr(node.X, actual)
		if err != nil {
			return nil, err
		}
		return &goast.UnaryExpr{Op: node.Op, X: child}, nil
	case *goast.CallExpr:
		function, err := substituteActionExpr(node.Fun, actual)
		if err != nil {
			return nil, err
		}
		arguments := make([]goast.Expr, len(node.Args))
		for index, argument := range node.Args {
			arguments[index], err = substituteActionExpr(argument, actual)
			if err != nil {
				return nil, err
			}
		}
		return &goast.CallExpr{Fun: function, Args: arguments, Ellipsis: node.Ellipsis}, nil
	case *goast.CompositeLit:
		elements := make([]goast.Expr, len(node.Elts))
		for index, element := range node.Elts {
			if pair, ok := element.(*goast.KeyValueExpr); ok {
				value, err := substituteActionExpr(pair.Value, actual)
				if err != nil {
					return nil, err
				}
				elements[index] = &goast.KeyValueExpr{Key: pair.Key, Value: value}
				continue
			}
			value, err := substituteActionExpr(element, actual)
			if err != nil {
				return nil, err
			}
			elements[index] = value
		}
		return &goast.CompositeLit{Type: node.Type, Elts: elements}, nil
	default:
		return nil, fmt.Errorf("unsupported helper substitution %T", expression)
	}
}

func sequenceLess(left, right SequenceLaw) bool {
	if left.Production != right.Production {
		return left.Production < right.Production
	}
	if left.Destination.Tag != right.Destination.Tag {
		return left.Destination.Tag < right.Destination.Tag
	}
	return left.Destination.Field < right.Destination.Field
}

func buildCarriers(schema grammar.Schema) ([]Carrier, error) {
	var rows []Carrier
	for _, constructor := range schema.Constructors {
		for _, field := range constructor.Fields {
			child, cardinality, ok := carrier(field.Type)
			if !ok && !(field.Name == "AdjustRet" && field.Form == grammar.FieldFormBool) {
				continue
			}
			if field.Name == "AdjustRet" && field.Form == grammar.FieldFormBool {
				child, cardinality = "ValuesAdjustment", grammarproof.FieldStateTrue
			}
			rows = append(rows, Carrier{Form: constructor.Name, Field: field.Name, Class: constructor.Class, ChildType: child, Cardinality: cardinality})
		}
	}
	sort.Slice(rows, func(left, right int) bool {
		return rows[left].Form < rows[right].Form || rows[left].Form == rows[right].Form && rows[left].Field < rows[right].Field
	})
	return rows, nil
}

func carrier(typ string) (string, grammarproof.FieldState, bool) {
	sequence := strings.HasPrefix(typ, "[]")
	typ = strings.TrimPrefix(strings.TrimPrefix(typ, "[]"), "*")
	typ = strings.TrimPrefix(typ, "ast.")
	switch typ {
	case "Expr", "Stmt", "TypeExpr", "FunctionParamExpr", "InterfaceMember", "AnnotationExpr", "RecordFieldExpr", "TypeParamExpr":
		if sequence {
			return typ, grammarproof.FieldStateNonEmpty, true
		}
		return typ, grammarproof.FieldStatePresent, true
	}
	return "", 0, false
}
