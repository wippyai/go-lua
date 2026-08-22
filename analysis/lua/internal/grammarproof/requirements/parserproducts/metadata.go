package parserproducts

import (
	"fmt"
	goast "go/ast"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/astcodec"
	"github.com/wippyai/go-lua/analysis/lua/parsersource"
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

func buildCarriers(schema parsersource.Schema) ([]Carrier, error) {
	var rows []Carrier
	for _, constructor := range schema.Constructors {
		for _, field := range constructor.Fields {
			child, cardinality, ok := carrier(field.Type)
			if !ok && (field.Name != "AdjustRet" || field.Form != parsersource.FieldFormBool) {
				continue
			}
			if field.Name == "AdjustRet" && field.Form == parsersource.FieldFormBool {
				child, cardinality = "ValuesAdjustment", astcodec.FieldStateTrue
			}
			rows = append(rows, Carrier{Form: constructor.Name, Field: field.Name, Class: constructor.Class, ChildType: child, Cardinality: cardinality})
		}
	}
	sort.Slice(rows, func(left, right int) bool {
		return rows[left].Form < rows[right].Form || rows[left].Form == rows[right].Form && rows[left].Field < rows[right].Field
	})
	return rows, nil
}

func carrier(typ string) (string, astcodec.FieldState, bool) {
	sequence := strings.HasPrefix(typ, "[]")
	typ = strings.TrimPrefix(strings.TrimPrefix(typ, "[]"), "*")
	typ = strings.TrimPrefix(typ, "ast.")
	switch typ {
	case "Expr", "Stmt", "TypeExpr", "FunctionParamExpr", "InterfaceMember", "AnnotationExpr", "RecordFieldExpr", "TypeParamExpr":
		if sequence {
			return typ, astcodec.FieldStateNonEmpty, true
		}
		return typ, astcodec.FieldStatePresent, true
	}
	return "", 0, false
}
