package parsersource

import (
	"fmt"
	goast "go/ast"
	"os"
	"path/filepath"
	"sort"
)

// SequenceCarrier is one yacc result location that holds a list. A blank Field
// names the semantic value of the nonterminal itself; a named Field is a list
// owned by a parser-private result wrapper. The denominator is derived from the
// %union declaration and the parser's own record declarations together, so a
// wrapper is a sequence owner because it declares a slice member and not
// because it is named as one.
type SequenceCarrier struct {
	Tag   string
	Field string
}

// SequenceConstruction is the way one reduction obtains the list its result
// carries. The vocabulary is closed and is the whole list-building law: a
// reduction either states no list, states one whose members it names, hands one
// through unchanged, or extends one it received.
type SequenceConstruction uint8

const (
	SequenceConstructionInvalid SequenceConstruction = iota
	// SequenceConstructionNil: the reduction leaves the carrier at its zero
	// state. It is a disposition, not an omission: the carrier exists and this
	// alternative puts nothing in it.
	SequenceConstructionNil
	// SequenceConstructionLiteral: the reduction states the whole list, so its
	// length is exactly the number of members it names.
	SequenceConstructionLiteral
	// SequenceConstructionForward: the reduction hands a list it received
	// through unchanged, so its length is the length of that input.
	SequenceConstructionForward
	// SequenceConstructionAppend: the reduction extends a list it received, so
	// its length is that input's length plus the members it names.
	SequenceConstructionAppend
)

// String is the stable spelling a row key uses.
func (c SequenceConstruction) String() string {
	switch c {
	case SequenceConstructionNil:
		return "nil"
	case SequenceConstructionLiteral:
		return "literal"
	case SequenceConstructionForward:
		return "forward"
	case SequenceConstructionAppend:
		return "append"
	default:
		return "invalid"
	}
}

// SequenceSegmentKind separates the two things a list-building operand can be.
// The distinction is the grain the list law is stated at: a member adds exactly
// one position, while a spread adds every position of another list, so a law
// that could not tell them apart could not state where a list's final position
// comes from.
type SequenceSegmentKind uint8

const (
	SequenceSegmentInvalid SequenceSegmentKind = iota
	// SequenceElement: one member of the list.
	SequenceElement
	// SequenceSpread: an entire input list, spliced in place.
	SequenceSpread
)

// String is the stable spelling a row key uses.
func (k SequenceSegmentKind) String() string {
	switch k {
	case SequenceElement:
		return "element"
	case SequenceSpread:
		return "spread"
	default:
		return "invalid"
	}
}

// SequenceSegment is one operand of a list construction, stated in the same
// provenance vocabulary a consumption edge uses. Ordinal is the operand's
// position in the construction, which is what makes a final member final.
type SequenceSegment struct {
	Ordinal int
	Kind    SequenceSegmentKind
	// Input is the exact positional parser operand when this segment is that
	// operand itself. Projections, indexes, assertions, locals, constructions,
	// and other expressions remain zero even when their provenance reaches an
	// operand. This is the canonical directness fact consumed by grammar laws.
	Input   int
	Origins []UseOrigin
	Sources []int
	Symbols []int
}

// ActionSequence is one reduction's disposition of one list-valued result
// carrier. It is the list-building half of the same relation Products and Uses
// read: a product row states a constructed value's field vector and a use row
// states where a value lands, while a sequence row states how the reduction's
// own result list is assembled out of its operands.
//
// Only a reduction owns a row. A parser helper that assembles the list on a
// reduction's behalf is read through the call, so the row states the law the
// alternative holds rather than a law about a callable several alternatives
// share.
type ActionSequence struct {
	Production   string
	Tag          string
	Field        string
	Construction SequenceConstruction
	Segments     []SequenceSegment
}

// SequenceCarriers derives the complete list-valued result denominator from
// parser.go.y alone. Every declared result tag must resolve to a %union arm: a
// slice arm is a list the nonterminal's own value carries, and a parser-private
// wrapper arm contributes one carrier per slice member it declares. An arm this
// cannot read is refused rather than becoming an implicit non-sequence.
func SequenceCarriers(root string) ([]SequenceCarrier, error) {
	records, err := parserRecordTypes(root)
	if err != nil {
		return nil, err
	}
	return sequenceCarriers(root, records)
}

// sequenceCarriers derives the list-carrier denominator from parser source
// using an already-issued parser-private record table. DiscoverProducts has
// to retain that table for product construction, so reparsing parser.go.y here
// would make one source census pay for the same record authority twice.
func sequenceCarriers(root string, records map[string][]Field) ([]SequenceCarrier, error) {
	path := filepath.Join(root, "compiler", "parse", "parser.go.y")
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tags, err := DeclaredResultTags(path)
	if err != nil {
		return nil, err
	}
	union, err := declaredUnion(path, string(contents))
	if err != nil {
		return nil, err
	}
	rows := make([]SequenceCarrier, 0, len(union))
	seen := make(map[string]bool, len(tags))
	for nonterminal, tag := range tags {
		arm, declared := union[tag]
		if !declared {
			return nil, fmt.Errorf("parser sequences %s: nonterminal %s uses %%union member %s which is not declared", path, nonterminal, tag)
		}
		if seen[tag] {
			continue
		}
		seen[tag] = true
		form, formErr := fieldForm(arm)
		if formErr != nil {
			return nil, fmt.Errorf("parser sequences %s: %%union member %s: %w", path, tag, formErr)
		}
		if form == FieldFormSequence {
			rows = append(rows, SequenceCarrier{Tag: tag})
			continue
		}
		name, ok := arm.(*goast.Ident)
		if !ok {
			// A scalar arm holds no list. Its declared form is read above, so
			// the disposition is decided rather than defaulted.
			continue
		}
		for _, field := range records[name.Name] {
			if field.Form != FieldFormSequence {
				continue
			}
			rows = append(rows, SequenceCarrier{Tag: tag, Field: field.Name})
		}
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Tag != rows[right].Tag {
			return rows[left].Tag < rows[right].Tag
		}
		return rows[left].Field < rows[right].Field
	})
	for index, row := range rows {
		if row.Tag == "" || index > 0 && rows[index-1] == row {
			return nil, fmt.Errorf("parser sequences %s: carriers are not a canonical relation", path)
		}
	}
	return rows, nil
}

// scopeSequences states one reduction's list dispositions. A reduction owns one
// row per list carrier its result tag declares, so an alternative that never
// mentions a carrier still states what that carrier holds after it runs.
func (b *productBuilder) scopeSequences(scope *actionScope, ordered []siteID) ([]ActionSequence, error) {
	carriers := b.carriers[scope.resultTag]
	if scope.kind != ProductScopeProduction || len(carriers) == 0 {
		return nil, nil
	}
	if scope.body == nil {
		return nil, fmt.Errorf("parser sequences: action %s has no body", scope.owner)
	}
	result, fields, err := resultDispositions(scope.body)
	if err != nil {
		return nil, fmt.Errorf("parser sequences: action %s: %w", scope.owner, err)
	}
	ordinals := siteOrdinals(ordered)
	rows := make([]ActionSequence, 0, len(carriers))
	for _, carrier := range carriers {
		row, rowErr := b.sequenceRow(scope, ordinals, carrier, result, fields)
		if rowErr != nil {
			return nil, fmt.Errorf("parser sequences: action %s: %w", scope.owner, rowErr)
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(left, right int) bool {
		if rows[left].Tag != rows[right].Tag {
			return rows[left].Tag < rows[right].Tag
		}
		return rows[left].Field < rows[right].Field
	})
	return rows, nil
}

func (b *productBuilder) sequenceRow(scope *actionScope, ordinals map[siteID]int, carrier SequenceCarrier, result goast.Expr, fields map[string]goast.Expr) (ActionSequence, error) {
	row := ActionSequence{Production: scope.owner, Tag: carrier.Tag, Field: carrier.Field}
	expression := result
	if carrier.Field != "" {
		expression = fields[carrier.Field]
		if expression == nil {
			// The wrapper is either built here without naming the member, which
			// leaves it at its zero state, or handed back by a helper that owns
			// the disposition on this alternative's behalf.
			if literal := rootComposite(result); literal != nil {
				row.Construction = SequenceConstructionNil
				return row, nil
			}
			call, isCall := result.(*goast.CallExpr)
			if !isCall {
				return ActionSequence{}, fmt.Errorf("no disposition for %s.%s", carrier.Tag, carrier.Field)
			}
			return b.helperSequenceRow(scope, ordinals, row, call, carrier.Field)
		}
	}
	if expression == nil {
		return ActionSequence{}, fmt.Errorf("no disposition for %s", carrier.Tag)
	}
	construction, segments, err := b.sequenceExpression(scope, ordinals, expression, nil)
	if err != nil {
		return ActionSequence{}, err
	}
	row.Construction, row.Segments = construction, segments
	return row, nil
}

// helperFrame carries a helper's formals bound to the expressions one call site
// passed. The helper's own body states the list law; the frame is what lets the
// operands of that law be read at the reduction's coordinates instead of being
// reported as an anonymous parameter.
type helperFrame struct {
	actuals map[string]goast.Expr
	caller  *actionScope
}

// helperSequenceRow reads a list disposition a reduction delegates to a parser
// helper. The helper hands back one of its own formals, so the reduction's list
// is either the list that formal already carried or the one the helper wrote
// onto it, and both are read here at the call's operands.
func (b *productBuilder) helperSequenceRow(scope *actionScope, ordinals map[siteID]int, row ActionSequence, call *goast.CallExpr, field string) (ActionSequence, error) {
	callee, ok := call.Fun.(*goast.Ident)
	if !ok {
		return ActionSequence{}, fmt.Errorf("result of %s.%s is not a named call", row.Tag, field)
	}
	index, known := b.helperScopes[callee.Name]
	if !known {
		return ActionSequence{}, fmt.Errorf("result of %s.%s calls %s, which is no parser helper", row.Tag, field, callee.Name)
	}
	helper := b.scopes[index]
	if len(helper.formals) != len(call.Args) {
		return ActionSequence{}, fmt.Errorf("helper %s is called with %d operands and declares %d formals", callee.Name, len(call.Args), len(helper.formals))
	}
	frame := &helperFrame{actuals: make(map[string]goast.Expr, len(helper.formals)), caller: scope}
	for position, formal := range helper.formals {
		frame.actuals[formal] = call.Args[position]
	}
	returned := helperForwardedFormal(helper)
	if returned == "" {
		return ActionSequence{}, fmt.Errorf("helper %s does not hand back one of its own formals", callee.Name)
	}
	writes := b.helperFieldWrites(helper, returned, field)
	if len(writes) > 1 {
		return ActionSequence{}, fmt.Errorf("helper %s writes %s more than once", callee.Name, field)
	}
	if len(writes) == 1 {
		construction, segments, err := b.sequenceExpression(helper, ordinals, writes[0], frame)
		if err != nil {
			return ActionSequence{}, err
		}
		row.Construction, row.Segments = construction, segments
		return row, nil
	}
	// The helper leaves the member alone, so the reduction's list is the one the
	// operand it handed in already carried.
	operand := frame.actuals[returned]
	if literal := rootComposite(operand); literal != nil {
		member := compositeMember(literal, field)
		if member == nil {
			row.Construction = SequenceConstructionNil
			return row, nil
		}
		construction, segments, err := b.sequenceExpression(scope, ordinals, member, nil)
		if err != nil {
			return ActionSequence{}, err
		}
		row.Construction, row.Segments = construction, segments
		return row, nil
	}
	row.Construction = SequenceConstructionForward
	row.Segments = []SequenceSegment{b.sequenceSegment(scope, ordinals, 1, SequenceSpread, operand, nil)}
	return row, nil
}

// sequenceExpression reads one list-valued expression as a construction and its
// operands. The four shapes are the whole vocabulary a yacc action uses to
// build a list, so an expression outside them is refused rather than summarized.
func (b *productBuilder) sequenceExpression(scope *actionScope, ordinals map[siteID]int, expression goast.Expr, frame *helperFrame) (SequenceConstruction, []SequenceSegment, error) {
	if identifier, ok := expression.(*goast.Ident); ok && identifier.Name == "nil" {
		return SequenceConstructionNil, nil, nil
	}
	if literal, ok := expression.(*goast.CompositeLit); ok {
		if _, slice := literal.Type.(*goast.ArrayType); !slice {
			return SequenceConstructionInvalid, nil, fmt.Errorf("list expression builds %s", sourceExpr(literal.Type))
		}
		segments := make([]SequenceSegment, 0, len(literal.Elts))
		for _, element := range literal.Elts {
			segments = append(segments, b.sequenceSegment(scope, ordinals, len(segments)+1, SequenceElement, element, frame))
		}
		return SequenceConstructionLiteral, segments, nil
	}
	call, ok := expression.(*goast.CallExpr)
	if !ok {
		return SequenceConstructionForward, []SequenceSegment{b.sequenceSegment(scope, ordinals, 1, SequenceSpread, expression, frame)}, nil
	}
	callee, named := call.Fun.(*goast.Ident)
	if !named || callee.Name != "append" || len(call.Args) < 2 {
		return SequenceConstructionInvalid, nil, fmt.Errorf("list expression is a call this analysis does not model")
	}
	segments := make([]SequenceSegment, 0, len(call.Args))
	for position, argument := range call.Args {
		if position == 0 {
			if literal, composite := argument.(*goast.CompositeLit); composite {
				if _, slice := literal.Type.(*goast.ArrayType); slice {
					for _, element := range literal.Elts {
						segments = append(segments, b.sequenceSegment(scope, ordinals, len(segments)+1, SequenceElement, element, frame))
					}
					continue
				}
			}
		}
		kind := SequenceElement
		if position == 0 || call.Ellipsis.IsValid() && position == len(call.Args)-1 {
			kind = SequenceSpread
		}
		segments = append(segments, b.sequenceSegment(scope, ordinals, len(segments)+1, kind, argument, frame))
	}
	return SequenceConstructionAppend, segments, nil
}

func (b *productBuilder) sequenceSegment(scope *actionScope, ordinals map[siteID]int, ordinal int, kind SequenceSegmentKind, expression goast.Expr, frame *helperFrame) SequenceSegment {
	origins, sources, symbols := b.originsIn(scope, expression, ordinals, frame)
	return SequenceSegment{Ordinal: ordinal, Kind: kind, Input: directSequenceInput(scope, expression, frame), Origins: origins, Sources: sources, Symbols: symbols}
}

func directSequenceInput(scope *actionScope, expression goast.Expr, frame *helperFrame) int {
	for {
		switch current := expression.(type) {
		case *goast.ParenExpr:
			expression = current.X
			continue
		case *goast.Ident:
			if frame != nil {
				if actual, ok := frame.actuals[current.Name]; ok {
					return directSequenceInput(frame.caller, actual, nil)
				}
			}
			if scope.kind != ProductScopeProduction {
				return 0
			}
			return operandSlot(current.Name)
		default:
			return 0
		}
	}
}

// resultDispositions reads the top-level dispositions one action states for its
// result. Only the top level is read: a list law is a statement about the value
// the reduction hands back, and an alternative that stated two of them for one
// carrier would be stating two laws at once.
func resultDispositions(block *goast.BlockStmt) (goast.Expr, map[string]goast.Expr, error) {
	var result goast.Expr
	fields := make(map[string]goast.Expr)
	for _, statement := range block.List {
		assignment, ok := statement.(*goast.AssignStmt)
		if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		if name, isName := assignment.Lhs[0].(*goast.Ident); isName {
			if name.Name != resultOperand {
				continue
			}
			if result != nil {
				return nil, nil, fmt.Errorf("states its result twice")
			}
			result = assignment.Rhs[0]
			continue
		}
		selector, isSelector := assignment.Lhs[0].(*goast.SelectorExpr)
		if !isSelector {
			continue
		}
		if base, isName := selector.X.(*goast.Ident); !isName || base.Name != resultOperand {
			continue
		}
		if fields[selector.Sel.Name] != nil {
			return nil, nil, fmt.Errorf("writes result member %s twice", selector.Sel.Name)
		}
		fields[selector.Sel.Name] = assignment.Rhs[0]
	}
	if literal := rootComposite(result); literal != nil {
		for _, element := range literal.Elts {
			pair, ok := element.(*goast.KeyValueExpr)
			if !ok {
				continue
			}
			name, isName := pair.Key.(*goast.Ident)
			if !isName {
				continue
			}
			if fields[name.Name] != nil {
				return nil, nil, fmt.Errorf("states result member %s twice", name.Name)
			}
			fields[name.Name] = pair.Value
		}
	}
	return result, fields, nil
}

// helperForwardedFormal names the formal a helper hands back. A helper that
// owns a list disposition on a reduction's behalf works by receiving the value,
// editing it, and returning it, so the formal it returns is the coordinate the
// reduction's own list lives at.
func helperForwardedFormal(scope *actionScope) string {
	if len(scope.returns) != 1 || len(scope.returns[0]) != 1 {
		return ""
	}
	returned, ok := scope.returns[0][0].(*goast.Ident)
	if !ok {
		return ""
	}
	for _, formal := range scope.formals {
		if formal == returned.Name {
			return formal
		}
	}
	return ""
}

// helperFieldWrites are the assignments a helper makes to one member of the
// value it hands back. They are read out of the edits the same walk already
// recorded, so a helper's list edit is not read a second way here.
func (b *productBuilder) helperFieldWrites(scope *actionScope, receiver, field string) []goast.Expr {
	var result []goast.Expr
	for _, mutation := range b.mutations {
		if mutation.scope != scope.index || mutation.field != field {
			continue
		}
		if rootName(mutation.target) != receiver {
			continue
		}
		result = append(result, mutation.value)
	}
	return result
}

// rootComposite reaches the composite literal an expression is, through the
// address-of and parentheses an action writes around one.
func rootComposite(expression goast.Expr) *goast.CompositeLit {
	switch current := expression.(type) {
	case *goast.CompositeLit:
		return current
	case *goast.UnaryExpr:
		return rootComposite(current.X)
	case *goast.ParenExpr:
		return rootComposite(current.X)
	}
	return nil
}

func compositeMember(literal *goast.CompositeLit, field string) goast.Expr {
	for _, element := range literal.Elts {
		pair, ok := element.(*goast.KeyValueExpr)
		if !ok {
			continue
		}
		if name, isName := pair.Key.(*goast.Ident); isName && name.Name == field {
			return pair.Value
		}
	}
	return nil
}
