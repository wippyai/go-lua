package parserproducts

import "fmt"

// ActionTermID names one node in the one immutable action arena. Zero is
// invalid so an omitted semantic operand cannot look well formed.
type ActionTermID uint32

type ActionSymbolID uint32

type ActionSymbolKind uint8

const (
	ActionSymbolInvalid ActionSymbolKind = iota
	ActionSymbolConstant
	ActionSymbolEnum
	ActionSymbolField
	ActionSymbolType
	ActionSymbolCallable
	ActionSymbolOwner
	ActionSymbolDiagnostic
	ActionSymbolControl
)

type ActionSymbol struct {
	Kind ActionSymbolKind
	Text string
}

type ActionScopeKind uint8

const (
	ActionScopeInvalid ActionScopeKind = iota
	ActionScopeProduction
	ActionScopeHelper
	ActionScopeMapItem
)

// ActionScope is the complete binder geometry for one action relation.
// Production owners are owner symbols; helper and map owners are callable
// symbols, making helper identity shared with every application edge.
type ActionScope struct {
	Kind    ActionScopeKind
	Owner   ActionSymbolID
	Inputs  uint16
	Formals uint16
	Locals  uint16
	Results uint16
}

type ActionScopeID uint16

type ActionTermKind uint8

const (
	ActionTermInvalid ActionTermKind = iota
	ActionTermInput
	ActionTermFormal
	ActionTermLocal
	ActionTermResult
	ActionTermNil
	ActionTermBool
	ActionTermString
	ActionTermInt
	ActionTermEnum
	ActionTermProject
	ActionTermIndex
	ActionTermRecord
	ActionTermSequence
	ActionTermAddress
	ActionTermCall
	ActionTermAppend
	ActionTermControl
)

// ActionTerm uses one contiguous edge arena. Children always precede their
// parent and always live in that parent's scope.
type ActionTerm struct {
	Scope     ActionScopeID
	Kind      ActionTermKind
	Slot      uint16
	Symbol    ActionSymbolID
	EdgeStart uint32
	EdgeCount uint16
}

type ActionEdgeFlags uint8

const (
	ActionEdgeNoFlags ActionEdgeFlags = 0
	ActionEdgeSpread  ActionEdgeFlags = 1 << iota
)

type ActionEdge struct {
	Term  ActionTermID
	Label ActionSymbolID
	Flags ActionEdgeFlags
}

// ChainTail is an exact final-node field assignment in a ChainLaw. It lives
// in the action arena so field/value coordinates remain numeric and shared.
type ChainTail struct {
	Field ActionSymbolID
	Value ActionTermID
}

type PlaceRoot uint8

const (
	PlaceRootInvalid PlaceRoot = iota
	PlaceRootResult
	PlaceRootFormal
	PlaceRootLocal
)

type PlaceStepKind uint8

const (
	PlaceStepInvalid PlaceStepKind = iota
	PlaceStepField
	PlaceStepIndex
)

type PlaceStep struct {
	Kind  PlaceStepKind
	Field ActionSymbolID
	Index ActionTermID
}

// Place is a destination, not an expression. Its path is a span in the same
// numeric arena as terms rather than an independently mutable child slice.
type Place struct {
	Scope     ActionScopeID
	Root      PlaceRoot
	Slot      uint16
	StepStart uint32
	StepCount uint16
}

type EditKind uint8

const (
	EditInvalid EditKind = iota
	EditAssign
	EditAppend
)

type Edit struct {
	Kind  EditKind
	Guard Guard
	Place Place
	Value ActionTermID
}

type ActionTerms struct {
	Symbols      []ActionSymbol
	Scopes       []ActionScope
	Terms        []ActionTerm
	Edges        []ActionEdge
	ChainTails   []ChainTail
	PlaceSteps   []PlaceStep
	GuardSymbols []ActionSymbolID
}

func (table ActionTerms) Term(id ActionTermID) (ActionTerm, bool) {
	if id == 0 || int(id) > len(table.Terms) {
		return ActionTerm{}, false
	}
	return table.Terms[id-1], true
}

func (table ActionTerms) Scope(id ActionScopeID) (ActionScope, bool) {
	if id == 0 || int(id) > len(table.Scopes) {
		return ActionScope{}, false
	}
	return table.Scopes[id-1], true
}

func (table ActionTerms) Symbol(id ActionSymbolID) (ActionSymbol, bool) {
	if id == 0 || int(id) > len(table.Symbols) {
		return ActionSymbol{}, false
	}
	return table.Symbols[id-1], true
}

func symbolLess(left, right ActionSymbol) bool {
	return left.Kind < right.Kind || left.Kind == right.Kind && left.Text < right.Text
}

// Validate checks the closed term language, canonical symbol order, exact
// contiguous/non-overlapping edge partition, and binder-role bounds.
func (table ActionTerms) Validate() error {
	for index, symbol := range table.Symbols {
		if symbol.Kind == ActionSymbolInvalid || symbol.Text == "" && symbol.Kind != ActionSymbolConstant {
			return fmt.Errorf("parser products: invalid action symbol %d", index+1)
		}
		if index != 0 && !symbolLess(table.Symbols[index-1], symbol) {
			return fmt.Errorf("parser products: noncanonical action symbols")
		}
	}
	for index, scope := range table.Scopes {
		if err := table.validateScope(ActionScopeID(index+1), scope); err != nil {
			return err
		}
	}
	edgeCursor := uint32(0)
	for index, term := range table.Terms {
		id := ActionTermID(index + 1)
		if term.Kind == ActionTermInvalid {
			return fmt.Errorf("parser products: invalid action term %d", id)
		}
		scope, ok := table.Scope(term.Scope)
		if !ok {
			return fmt.Errorf("parser products: invalid action scope on term %d", id)
		}
		if term.EdgeStart != edgeCursor {
			return fmt.Errorf("parser products: noncontiguous action edge partition at term %d", id)
		}
		end := uint64(term.EdgeStart) + uint64(term.EdgeCount)
		if end > uint64(len(table.Edges)) {
			return fmt.Errorf("parser products: invalid action edge range %d", id)
		}
		for childIndex, edge := range table.Edges[term.EdgeStart:end] {
			child, childOK := table.Term(edge.Term)
			if !childOK || edge.Term >= id || child.Scope != term.Scope {
				return fmt.Errorf("parser products: invalid action child on term %d", id)
			}
			if err := table.validateEdge(term, edge, childIndex); err != nil {
				return fmt.Errorf("parser products: term %d: %w", id, err)
			}
			if term.Kind == ActionTermRecord && childIndex != 0 && table.Edges[term.EdgeStart+uint32(childIndex-1)].Label >= edge.Label {
				return fmt.Errorf("parser products: noncanonical record labels on term %d", id)
			}
		}
		if err := table.validateTerm(id, term, scope); err != nil {
			return err
		}
		edgeCursor = uint32(end)
	}
	if edgeCursor != uint32(len(table.Edges)) {
		return fmt.Errorf("parser products: action edge residue")
	}
	for index, tail := range table.ChainTails {
		symbol, ok := table.Symbol(tail.Field)
		if !ok || symbol.Kind != ActionSymbolField || tail.Value == 0 {
			return fmt.Errorf("parser products: malformed chain tail %d", index)
		}
	}
	for index, step := range table.PlaceSteps {
		switch step.Kind {
		case PlaceStepField:
			symbol, ok := table.Symbol(step.Field)
			if !ok || symbol.Kind != ActionSymbolField || step.Index != 0 {
				return fmt.Errorf("parser products: malformed place field step %d", index)
			}
		case PlaceStepIndex:
			if step.Field != 0 || step.Index == 0 {
				return fmt.Errorf("parser products: malformed place index step %d", index)
			}
		default:
			return fmt.Errorf("parser products: invalid place step %d", index)
		}
	}
	for _, symbol := range table.GuardSymbols {
		item, ok := table.Symbol(symbol)
		if !ok || item.Kind != ActionSymbolType {
			return fmt.Errorf("parser products: malformed guard type set")
		}
	}
	return nil
}

func (table ActionTerms) validateScope(id ActionScopeID, scope ActionScope) error {
	owner, ok := table.Symbol(scope.Owner)
	if !ok {
		return fmt.Errorf("parser products: invalid action scope owner %d", id)
	}
	switch scope.Kind {
	case ActionScopeProduction:
		if owner.Kind != ActionSymbolOwner || scope.Formals != 0 || scope.Results != 1 {
			return fmt.Errorf("parser products: malformed production scope %d", id)
		}
	case ActionScopeHelper:
		if owner.Kind != ActionSymbolCallable || scope.Inputs != 0 {
			return fmt.Errorf("parser products: malformed helper scope %d", id)
		}
	case ActionScopeMapItem:
		if owner.Kind != ActionSymbolCallable || scope.Inputs != 1 || scope.Formals != 0 || scope.Results != 0 || scope.Locals != 0 {
			return fmt.Errorf("parser products: malformed map-item scope %d", id)
		}
	default:
		return fmt.Errorf("parser products: invalid action scope %d", id)
	}
	return nil
}

func (table ActionTerms) validateEdge(term ActionTerm, edge ActionEdge, index int) error {
	if edge.Flags&^ActionEdgeSpread != 0 {
		return fmt.Errorf("unknown edge flags")
	}
	if term.Kind == ActionTermRecord {
		symbol, ok := table.Symbol(edge.Label)
		if !ok || symbol.Kind != ActionSymbolField || edge.Flags != ActionEdgeNoFlags {
			return fmt.Errorf("malformed record edge")
		}
		return nil
	}
	if edge.Label != 0 {
		return fmt.Errorf("label outside record")
	}
	if edge.Flags == ActionEdgeSpread && (term.Kind != ActionTermCall && term.Kind != ActionTermAppend || index != int(term.EdgeCount)-1) {
		return fmt.Errorf("spread is not final call operand")
	}
	return nil
}

func (table ActionTerms) validateTerm(id ActionTermID, term ActionTerm, scope ActionScope) error {
	symbol, hasSymbol := table.Symbol(term.Symbol)
	noEdges := term.EdgeCount == 0
	switch term.Kind {
	case ActionTermInput:
		if (scope.Kind != ActionScopeProduction && scope.Kind != ActionScopeMapItem) || term.Symbol != 0 || !noEdges || term.Slot >= scope.Inputs {
			return fmt.Errorf("parser products: malformed input term %d", id)
		}
	case ActionTermFormal:
		if scope.Kind != ActionScopeHelper || term.Symbol != 0 || !noEdges || term.Slot >= scope.Formals {
			return fmt.Errorf("parser products: malformed formal term %d", id)
		}
	case ActionTermLocal:
		if term.Symbol != 0 || !noEdges || term.Slot >= scope.Locals {
			return fmt.Errorf("parser products: malformed local term %d", id)
		}
	case ActionTermResult:
		if scope.Kind != ActionScopeProduction || term.Symbol != 0 || !noEdges || term.Slot >= scope.Results {
			return fmt.Errorf("parser products: malformed result term %d", id)
		}
	case ActionTermNil:
		if term.Symbol != 0 || !noEdges || term.Slot != 0 {
			return fmt.Errorf("parser products: malformed nil term %d", id)
		}
	case ActionTermBool, ActionTermString, ActionTermInt:
		if !hasSymbol || symbol.Kind != ActionSymbolConstant || !noEdges || term.Slot != 0 {
			return fmt.Errorf("parser products: malformed constant term %d", id)
		}
	case ActionTermEnum:
		if !hasSymbol || symbol.Kind != ActionSymbolEnum || !noEdges || term.Slot != 0 {
			return fmt.Errorf("parser products: malformed enum term %d", id)
		}
	case ActionTermProject:
		if !hasSymbol || symbol.Kind != ActionSymbolField || term.EdgeCount != 1 || term.Slot != 0 {
			return fmt.Errorf("parser products: malformed projection term %d", id)
		}
	case ActionTermIndex:
		if term.Symbol != 0 || term.EdgeCount != 2 || term.Slot != 0 {
			return fmt.Errorf("parser products: malformed index term %d", id)
		}
	case ActionTermRecord:
		if term.Slot != 0 || term.Symbol != 0 && (!hasSymbol || symbol.Kind != ActionSymbolType || symbol.Text == "struct{}") {
			return fmt.Errorf("parser products: malformed record term %d", id)
		}
	case ActionTermSequence:
		if !hasSymbol || symbol.Kind != ActionSymbolType || term.Slot != 0 {
			return fmt.Errorf("parser products: malformed sequence term %d", id)
		}
	case ActionTermAddress:
		if term.Symbol != 0 || term.EdgeCount != 1 || term.Slot != 0 {
			return fmt.Errorf("parser products: malformed address term %d", id)
		}
	case ActionTermCall:
		if !hasSymbol || symbol.Kind != ActionSymbolCallable || term.Slot != 0 {
			return fmt.Errorf("parser products: malformed call term %d", id)
		}
	case ActionTermAppend:
		if !hasSymbol || symbol.Kind != ActionSymbolCallable || symbol.Text != "append" || term.EdgeCount < 2 || term.Slot != 0 {
			return fmt.Errorf("parser products: malformed append term %d", id)
		}
	case ActionTermControl:
		if !hasSymbol || symbol.Kind != ActionSymbolControl || !noEdges || term.Slot != 0 {
			return fmt.Errorf("parser products: malformed control term %d", id)
		}
	default:
		return fmt.Errorf("parser products: unknown action term %d", id)
	}
	return nil
}

func (table ActionTerms) ValidatePlace(place Place) error {
	scope, ok := table.Scope(place.Scope)
	if !ok {
		return fmt.Errorf("parser products: place has invalid scope")
	}
	switch place.Root {
	case PlaceRootResult:
		if scope.Kind != ActionScopeProduction || place.Slot >= scope.Results {
			return fmt.Errorf("parser products: invalid result place")
		}
	case PlaceRootFormal:
		if scope.Kind != ActionScopeHelper || place.Slot >= scope.Formals {
			return fmt.Errorf("parser products: invalid formal place")
		}
	case PlaceRootLocal:
		if place.Slot >= scope.Locals {
			return fmt.Errorf("parser products: invalid local place")
		}
	default:
		return fmt.Errorf("parser products: invalid place root")
	}
	end := uint64(place.StepStart) + uint64(place.StepCount)
	if end > uint64(len(table.PlaceSteps)) {
		return fmt.Errorf("parser products: invalid place path")
	}
	for _, step := range table.PlaceSteps[place.StepStart:end] {
		if step.Kind == PlaceStepIndex {
			term, valid := table.Term(step.Index)
			if !valid || term.Scope != place.Scope {
				return fmt.Errorf("parser products: cross-scope place index")
			}
		}
	}
	return nil
}

func (table ActionTerms) ValidateEdit(edit Edit, scope ActionScopeID) error {
	if edit.Kind != EditAssign && edit.Kind != EditAppend {
		return fmt.Errorf("parser products: invalid edit kind")
	}
	if err := table.ValidateGuard(edit.Guard, scope); err != nil {
		return err
	}
	if edit.Place.Scope != scope || table.ValidatePlace(edit.Place) != nil {
		return fmt.Errorf("parser products: invalid edit place")
	}
	value, ok := table.Term(edit.Value)
	if !ok || value.Scope != scope {
		return fmt.Errorf("parser products: invalid edit value")
	}
	if edit.Kind == EditAppend && edit.Place.StepCount == 0 {
		return fmt.Errorf("parser products: append edit requires a field or index destination")
	}
	return nil
}

func (table ActionTerms) ValidateGuard(guard Guard, scope ActionScopeID) error {
	for index, atom := range guard.Atoms {
		if index != 0 && !guardAtomLess(guard.Atoms[index-1], atom) {
			return fmt.Errorf("parser products: noncanonical guard conjunction")
		}
		term, ok := table.Term(atom.Term)
		if !ok || term.Scope != scope {
			return fmt.Errorf("parser products: invalid guard term")
		}
		switch atom.Kind {
		case GuardAtomNil:
			if atom.Constant != 0 || atom.SetStart != 0 || atom.SetCount != 0 || atom.ParseClass != NumberParseClassUnknown {
				return fmt.Errorf("parser products: malformed nil guard")
			}
		case GuardAtomLenEq:
			constant, valid := table.Symbol(atom.Constant)
			if !valid || constant.Kind != ActionSymbolConstant || constant.Text != "0" && constant.Text != "1" || atom.SetStart != 0 || atom.SetCount != 0 || atom.ParseClass != NumberParseClassUnknown {
				return fmt.Errorf("parser products: malformed length guard")
			}
		case GuardAtomEqConst:
			constant, valid := table.Symbol(atom.Constant)
			if !valid || constant.Kind != ActionSymbolConstant || atom.SetStart != 0 || atom.SetCount != 0 || atom.ParseClass != NumberParseClassUnknown {
				return fmt.Errorf("parser products: malformed equality guard")
			}
		case GuardAtomTypeIn:
			if atom.Constant != 0 || atom.ParseClass != NumberParseClassUnknown || atom.SetCount == 0 || uint64(atom.SetStart)+uint64(atom.SetCount) > uint64(len(table.GuardSymbols)) {
				return fmt.Errorf("parser products: malformed type guard")
			}
			set := table.GuardSymbols[atom.SetStart : atom.SetStart+uint32(atom.SetCount)]
			for item := range set {
				if item != 0 && set[item-1] >= set[item] {
					return fmt.Errorf("parser products: noncanonical type guard")
				}
			}
		case GuardAtomNumberParseClass:
			if atom.Constant != 0 || atom.SetStart != 0 || atom.SetCount != 0 || atom.ParseClass != NumberParseClassInteger && atom.ParseClass != NumberParseClassFloat && atom.ParseClass != NumberParseClassInvalid {
				return fmt.Errorf("parser products: malformed number parse guard")
			}
		default:
			return fmt.Errorf("parser products: invalid guard atom")
		}
	}
	return nil
}

func (table ActionTerms) ValidateReject(reject Reject, scope ActionScopeID) error {
	if reject.Ordinal <= 0 || reject.Condition != RejectWhenAll && reject.Condition != RejectUnlessAll {
		return fmt.Errorf("parser products: invalid reject condition")
	}
	if err := table.ValidateGuard(reject.Guard, scope); err != nil {
		return err
	}
	diagnostic, ok := table.Symbol(reject.Diagnostic)
	if !ok || diagnostic.Kind != ActionSymbolDiagnostic {
		return fmt.Errorf("parser products: invalid reject diagnostic")
	}
	return nil
}

func guardAtomLess(left, right GuardAtom) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Negated != right.Negated {
		return !left.Negated && right.Negated
	}
	if left.Term != right.Term {
		return left.Term < right.Term
	}
	if left.Constant != right.Constant {
		return left.Constant < right.Constant
	}
	if left.SetStart != right.SetStart {
		return left.SetStart < right.SetStart
	}
	if left.SetCount != right.SetCount {
		return left.SetCount < right.SetCount
	}
	return left.ParseClass < right.ParseClass
}

// TermInstance is a non-materializing helper substitution. Root belongs to
// HelperScope; every actual belongs to CallerScope.
type TermInstance struct {
	CallerScope ActionScopeID
	HelperScope ActionScopeID
	Root        ActionTermID
	Actuals     []ActionTermID
}

func (table ActionTerms) ValidateInstance(instance TermInstance) error {
	if err := table.Validate(); err != nil {
		return err
	}
	helper, ok := table.Scope(instance.HelperScope)
	if !ok || helper.Kind != ActionScopeHelper {
		return fmt.Errorf("parser products: instance has invalid helper scope")
	}
	if _, ok := table.Scope(instance.CallerScope); !ok {
		return fmt.Errorf("parser products: instance has invalid caller scope")
	}
	root, ok := table.Term(instance.Root)
	if !ok || root.Scope != instance.HelperScope {
		return fmt.Errorf("parser products: instance has invalid root")
	}
	if len(instance.Actuals) != int(helper.Formals) {
		return fmt.Errorf("parser products: instance actual arity differs from helper formals")
	}
	for _, actual := range instance.Actuals {
		term, valid := table.Term(actual)
		if !valid || term.Scope != instance.CallerScope {
			return fmt.Errorf("parser products: instance actual crosses caller scope")
		}
	}
	seen := make(map[ActionTermID]bool)
	var check func(ActionTermID) error
	check = func(id ActionTermID) error {
		if seen[id] {
			return nil
		}
		seen[id] = true
		term, _ := table.Term(id)
		if term.Scope != instance.HelperScope {
			return fmt.Errorf("parser products: instance root captures foreign scope")
		}
		if term.Kind == ActionTermInput || term.Kind == ActionTermLocal || term.Kind == ActionTermResult {
			return fmt.Errorf("parser products: instance root captures non-formal slot")
		}
		for _, edge := range table.Edges[term.EdgeStart : term.EdgeStart+uint32(term.EdgeCount)] {
			if err := check(edge.Term); err != nil {
				return err
			}
		}
		return nil
	}
	return check(instance.Root)
}
