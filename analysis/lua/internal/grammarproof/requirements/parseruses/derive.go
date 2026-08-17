package parseruses

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/astcodec"
	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/requirements/parserproducts"
	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// sequenceIndex validates the one parserproducts-owned sequence authority.
// Consumers retain only coordinates into that authority; no list action or
// parallel composition plane exists here.
func sequenceIndex(products []parserproducts.ProductLaw, laws []parserproducts.SequenceLaw) (map[SequenceCoordinate]parserproducts.SequenceLaw, error) {
	productions := make(map[string]bool, len(products))
	for _, product := range products {
		productions[product.Production] = true
	}
	result := make(map[SequenceCoordinate]parserproducts.SequenceLaw, len(laws))
	for _, law := range laws {
		key := SequenceCoordinate{Production: law.Production, Tag: law.Destination.Tag, Field: law.Destination.Field}
		if !productions[law.Production] || key.Tag == "" || law.Construction == parserproducts.SequenceConstructionInvalid {
			return nil, fmt.Errorf("parser uses: invalid sequence authority coordinate")
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("parser uses: duplicate sequence authority coordinate")
		}
		result[key] = law
	}
	return result, nil
}

// buildValuesTails projects segment positions from SequenceLaw. It does not
// infer a list from a yacc carrier or from expression spelling.
func buildValuesTails(products []parserproducts.ProductLaw, sequences map[SequenceCoordinate]parserproducts.SequenceLaw) ([]ValuesTail, error) {
	byProduction := make(map[string]parserproducts.ProductLaw, len(products))
	for _, law := range products {
		byProduction[law.Production] = law
	}
	var result []ValuesTail
	for coordinate, law := range sequences {
		product := byProduction[coordinate.Production]
		if product.Nonterminal != "exprlist" && product.Nonterminal != "args" {
			continue
		}
		for index, segment := range law.Segments {
			if segment.Kind != parserproducts.SequenceElement {
				continue
			}
			tail := ValuesTail{Sequence: SequenceCoordinate{Production: coordinate.Production, Tag: coordinate.Tag, Field: coordinate.Field, Segment: index + 1}, Position: ValuesPositionFinalOpen}
			for successor := index + 1; successor < len(law.Segments); successor++ {
				if law.Segments[successor].Kind == parserproducts.SequenceElement {
					tail.Position, tail.Successor = ValuesPositionNonFinal, successor+1
					break
				}
			}
			result = append(result, tail)
		}
	}
	sort.Slice(result, func(left, right int) bool { return valuesTailLess(result[left], result[right]) })
	for index, tail := range result {
		if tail.Sequence.Production == "" || tail.Sequence.Tag == "" || tail.Sequence.Segment <= 0 || tail.Position == ValuesPositionInvalid ||
			tail.Position == ValuesPositionNonFinal && tail.Successor <= tail.Sequence.Segment || tail.Position != ValuesPositionNonFinal && tail.Successor != 0 ||
			index != 0 && !valuesTailLess(result[index-1], tail) {
			return nil, fmt.Errorf("parser uses: Values tails are not canonical")
		}
	}
	return result, nil
}

func valuesPosition(target ProgramUseClass, child string) ValuesPosition {
	if child != "Expr" {
		return ValuesPositionNotApplicable
	}
	if target == ProgramUseValues {
		return ValuesPositionFinalOpen
	}
	return ValuesPositionScalar
}

func buildLValuePaths(laws []parserproducts.ProductLaw, paths []UsePath, sequences map[SequenceCoordinate]parserproducts.SequenceLaw, terms parserproducts.ActionTerms) ([]LValuePath, error) {
	byProduction := make(map[string]parserproducts.ProductLaw, len(laws))
	for _, law := range laws {
		byProduction[law.Production] = law
	}
	var result []LValuePath
	for _, seed := range paths {
		if seed.Role != UseRoleLValue {
			continue
		}
		law, exists := byProduction[seed.ParentProduction]
		if !exists {
			return nil, fmt.Errorf("parser uses: lvalue seed has no sealed product law")
		}
		argument, ok := inputOrdinal(terms, seed.Term, law.Scope)
		if !ok || argument > len(law.RHS) {
			return nil, fmt.Errorf("parser uses: lvalue seed lacks exact sealed input")
		}
		list := law.RHS[argument-1]
		for coordinate, sequence := range sequences {
			composition, exists := byProduction[coordinate.Production]
			if !exists || composition.Nonterminal != list || sequence.Scope != composition.Scope {
				continue
			}
			for segmentIndex, segment := range sequence.Segments {
				if segment.Kind != parserproducts.SequenceElement {
					continue
				}
				memberArgument, ok := inputOrdinal(terms, segment.Term, composition.Scope)
				if !ok || memberArgument > len(composition.RHS) {
					return nil, fmt.Errorf("parser uses: invalid lvalue sequence element")
				}
				member := composition.RHS[memberArgument-1]
				for _, terminal := range laws {
					if terminal.Nonterminal != member {
						continue
					}
					for _, product := range terminal.Products {
						if !assignmentAddressForm(product.Constructor) {
							continue
						}
						result = append(result, LValuePath{SeedProduction: seed.ParentProduction, SeedOrdinal: seed.ParentOrdinal, Sequence: SequenceCoordinate{Production: coordinate.Production, Tag: coordinate.Tag, Field: coordinate.Field, Segment: segmentIndex + 1}, TerminalProduction: terminal.Production, TerminalOrdinal: product.Ordinal, TerminalForm: product.Constructor})
					}
				}
			}
		}
	}
	sort.Slice(result, func(left, right int) bool { return lvaluePathLess(result[left], result[right]) })
	for index, path := range result {
		if path.SeedProduction == "" || path.Sequence.Production == "" || path.Sequence.Tag == "" || path.Sequence.Segment <= 0 || path.TerminalProduction == "" || path.TerminalOrdinal <= 0 || path.TerminalForm == "" ||
			index != 0 && !lvaluePathLess(result[index-1], path) {
			return nil, fmt.Errorf("parser uses: lvalue paths are not canonical")
		}
	}
	covered := make(map[lvalueSeed]bool, len(result))
	for _, path := range result {
		covered[lvalueSeed{Production: path.SeedProduction, Ordinal: path.SeedOrdinal}] = true
	}
	for _, seed := range paths {
		if seed.Role == UseRoleLValue && !covered[lvalueSeed{Production: seed.ParentProduction, Ordinal: seed.ParentOrdinal}] {
			return nil, fmt.Errorf("parser uses: lvalue seed has no legal terminal path")
		}
	}
	return result, nil
}

type lvalueSeed struct {
	Production string
	Ordinal    int
}

// assignmentAddressForm is the only grammar-level write target boundary.
// `var` actions may construct StringExpr keys as implementation details of a
// dotted AttrGetExpr; those keys are read-only address components, never
// independently assignable values.
func assignmentAddressForm(form string) bool {
	return form == "IdentExpr" || form == "AttrGetExpr"
}

// buildUsePaths derives only direct, action-backed child edges. It retains an
// arena term ID rather than serializing a parser action expression.
func buildUsePaths(laws []parserproducts.ProductLaw, carriers map[string]parserproducts.Carrier, terms parserproducts.ActionTerms) ([]UsePath, error) {
	var paths []UsePath
	for _, law := range laws {
		for _, product := range law.Products {
			for _, vector := range product.Fields {
				if vector.Kind != parserproducts.ActionValueTerm {
					continue
				}
				carrier, exists := carriers[carrierKey(product.Constructor, vector.Field)]
				if !exists {
					continue
				}
				if !termInScope(terms, vector.Term, law.Scope) {
					return nil, fmt.Errorf("parser uses: direct term crosses product scope")
				}
				child := carrier.ChildType
				role := useRole(product.Constructor, carrier.Class, vector.Field, child)
				target, targetErr := programUse(terms, product, vector.Field, role, child)
				if targetErr != nil {
					return nil, fmt.Errorf("parser uses: direct use %s.%s: %w", product.Constructor, vector.Field, targetErr)
				}
				path := UsePath{ParentProduction: law.Production, ParentOrdinal: product.Ordinal, ParentForm: product.Constructor, ParentField: vector.Field, Term: vector.Term, Role: role, Child: childClass(child), Target: target, Open: openAxis(terms, law, product.Constructor, vector.Field, vector.Term, child), Table: tableAxis(terms, product, vector.Field), LValue: lvalueAxis(product.Constructor, vector.Field), Values: valuesPosition(target, child)}
				paths = append(paths, path)
			}
		}
		for _, chain := range law.Chains {
			field, err := actionFieldName(terms, chain.LinkField)
			if err != nil {
				return nil, err
			}
			if !termInScope(terms, chain.Input, law.Scope) {
				return nil, fmt.Errorf("parser uses: chain input crosses product scope")
			}
			for _, product := range law.Products {
				vector, found := productField(product, field)
				if !found || vector.Kind == parserproducts.ActionValueTerm {
					continue
				}
				carrier, exists := carriers[carrierKey(product.Constructor, field)]
				if !exists {
					continue
				}
				child := carrier.ChildType
				role := useRole(product.Constructor, carrier.Class, field, child)
				target, targetErr := programUse(terms, product, field, role, child)
				if targetErr != nil {
					return nil, fmt.Errorf("parser uses: chain use %s.%s: %w", product.Constructor, field, targetErr)
				}
				paths = append(paths, UsePath{ParentProduction: law.Production, ParentOrdinal: product.Ordinal, ParentForm: product.Constructor, ParentField: field, Term: chain.Input, Role: role, Child: childClass(child), Target: target, Open: OpenAxisClosed, Table: tableAxis(terms, product, field), LValue: lvalueAxis(product.Constructor, field), Values: valuesPosition(target, child)})
			}
		}
	}
	sort.Slice(paths, func(left, right int) bool { return usePathLess(paths[left], paths[right]) })
	for index, path := range paths {
		if index != 0 && !usePathLess(paths[index-1], path) {
			return nil, fmt.Errorf("parser uses: duplicate direct use path")
		}
	}
	return paths, nil
}

func buildHelperUsePaths(callers []parserproducts.ProductLaw, helpers []parserproducts.HelperLaw, carriers map[string]parserproducts.Carrier, terms parserproducts.ActionTerms) ([]HelperUsePath, error) {
	byHelper, err := helperIndex(helpers, terms)
	if err != nil {
		return nil, err
	}
	var result []HelperUsePath
	for _, caller := range callers {
		for index, application := range caller.Helpers {
			paths, expandErr := expandHelperUsePaths(caller, []uint16{uint16(index + 1)}, caller.Scope, application, application, nil, byHelper, carriers, terms)
			if expandErr != nil {
				return nil, expandErr
			}
			result = append(result, paths...)
		}
	}
	sort.Slice(result, func(left, right int) bool { return helperUsePathLess(result[left], result[right]) })
	for index, path := range result {
		if index != 0 && !helperUsePathLess(result[index-1], path) {
			return nil, fmt.Errorf("parser uses: duplicate helper use path")
		}
	}
	return result, nil
}

// expandHelperUsePaths records each immediate helper binding as a typed term
// instance. Nested applications remain nested in their own helper scope;
// parser uses never renders, reparses, or materializes a substitution.
func expandHelperUsePaths(root parserproducts.ProductLaw, applications []uint16, callerScope parserproducts.ActionScopeID, rootApplication, application parserproducts.HelperApplication, active map[parserproducts.ActionSymbolID]bool, helpers map[parserproducts.ActionSymbolID]parserproducts.HelperLaw, carriers map[string]parserproducts.Carrier, terms parserproducts.ActionTerms) ([]HelperUsePath, error) {
	if application.Scope != callerScope {
		return nil, fmt.Errorf("parser uses: helper application crosses caller scope")
	}
	helper, exists := helpers[application.Helper]
	if !exists {
		return nil, fmt.Errorf("parser uses: unknown sealed helper application")
	}
	helperScope, scopeOK := terms.Scope(helper.Scope)
	if !scopeOK || helperScope.Kind != parserproducts.ActionScopeHelper || len(application.Actuals) != int(helperScope.Formals) {
		return nil, fmt.Errorf("parser uses: malformed sealed helper application")
	}
	if active == nil {
		active = make(map[parserproducts.ActionSymbolID]bool)
	}
	if active[application.Helper] {
		return nil, fmt.Errorf("parser uses: recursive helper composition")
	}
	active[application.Helper] = true
	defer delete(active, application.Helper)
	instanceActuals := append([]parserproducts.ActionTermID(nil), application.Actuals...)
	var result []HelperUsePath
	for _, product := range helper.Products {
		for _, vector := range product.Fields {
			if vector.Kind != parserproducts.ActionValueTerm {
				continue
			}
			carrier, exists := carriers[carrierKey(product.Constructor, vector.Field)]
			if !exists {
				continue
			}
			instance := parserproducts.TermInstance{CallerScope: callerScope, HelperScope: helper.Scope, Root: vector.Term, Actuals: append([]parserproducts.ActionTermID(nil), instanceActuals...)}
			if err := terms.ValidateInstance(instance); err != nil {
				return nil, fmt.Errorf("parser uses: helper field binding: %w", err)
			}
			child := carrier.ChildType
			role := useRole(product.Constructor, carrier.Class, vector.Field, child)
			target, targetErr := programUse(terms, product, vector.Field, role, child)
			if targetErr != nil {
				return nil, fmt.Errorf("parser uses: helper use %s.%s: %w", product.Constructor, vector.Field, targetErr)
			}
			result = append(result, HelperUsePath{Production: root.Production, Applications: append([]uint16(nil), applications...), Helper: application.Helper, Ordinal: product.Ordinal, ParentForm: product.Constructor, ParentField: vector.Field, Instance: instance, Role: role, Child: childClass(child), Target: target, Open: helperOpenAxis(terms, root, rootApplication, product.Constructor, vector.Field, child), Table: tableAxis(terms, product, vector.Field), LValue: lvalueAxis(product.Constructor, vector.Field), Values: valuesPosition(target, child)})
		}
	}
	for index, nested := range helper.Helpers {
		paths, err := expandHelperUsePaths(root, appendApplication(applications, uint16(index+1)), helper.Scope, rootApplication, nested, active, helpers, carriers, terms)
		if err != nil {
			return nil, err
		}
		result = append(result, paths...)
	}
	return result, nil
}

func appendApplication(path []uint16, next uint16) []uint16 {
	result := make([]uint16, len(path)+1)
	copy(result, path)
	result[len(path)] = next
	return result
}

func helperIndex(rows []parserproducts.HelperLaw, terms parserproducts.ActionTerms) (map[parserproducts.ActionSymbolID]parserproducts.HelperLaw, error) {
	result := make(map[parserproducts.ActionSymbolID]parserproducts.HelperLaw, len(rows))
	for _, row := range rows {
		scope, ok := terms.Scope(row.Scope)
		if !ok || scope.Kind != parserproducts.ActionScopeHelper || scope.Owner == 0 {
			return nil, fmt.Errorf("parser uses: invalid sealed helper law")
		}
		if _, exists := result[scope.Owner]; exists {
			return nil, fmt.Errorf("parser uses: duplicate sealed helper law")
		}
		result[scope.Owner] = row
	}
	return result, nil
}

func buildMutationUsePaths(laws []parserproducts.ProductLaw, helpers []parserproducts.HelperLaw, mutations []parserproducts.FieldMutation, carriers map[string]parserproducts.Carrier, terms parserproducts.ActionTerms) ([]MutationUsePath, error) {
	var result []MutationUsePath
	for index, mutation := range mutations {
		carrier, err := mutationCarrier(laws, helpers, mutation, carriers, terms)
		if err != nil {
			return nil, fmt.Errorf("parser uses: mutation %s: %w", mutation.Production, err)
		}
		if carrier.Form == "" {
			continue
		}
		child := carrier.ChildType
		role := useRole(carrier.Form, carrier.Class, carrier.Field, child)
		target, targetErr := programUse(terms, parserproducts.ConstructorProduct{Constructor: carrier.Form}, carrier.Field, role, child)
		if targetErr != nil {
			return nil, fmt.Errorf("parser uses: mutation %s: %w", mutation.Production, targetErr)
		}
		result = append(result, MutationUsePath{Production: mutation.Production, Ordinal: index + 1, Edit: cloneEdit(mutation.Edit), Role: role, Child: childClass(child), Target: target, Open: OpenAxisClosed, Table: TableAxisNone, LValue: LValueAxisNo})
	}
	sort.Slice(result, func(left, right int) bool { return mutationUsePathLess(result[left], result[right]) })
	for index, path := range result {
		if index != 0 && !mutationUsePathLess(result[index-1], path) {
			return nil, fmt.Errorf("parser uses: duplicate mutation use path")
		}
	}
	return result, nil
}

// mutationCarrier maps an edit's final typed field step to its unique semantic
// carrier. Where a production builds the receiver in a fallback branch, that
// product law disambiguates the form without reopening parser source.
func mutationCarrier(laws []parserproducts.ProductLaw, helpers []parserproducts.HelperLaw, mutation parserproducts.FieldMutation, carriers map[string]parserproducts.Carrier, terms parserproducts.ActionTerms) (parserproducts.Carrier, error) {
	field, err := editField(terms, mutation.Edit)
	if err != nil {
		return parserproducts.Carrier{}, err
	}
	var candidates []parserproducts.Carrier
	for _, carrier := range carriers {
		if carrier.Field == field {
			candidates = append(candidates, carrier)
		}
	}
	if len(candidates) == 0 {
		return parserproducts.Carrier{}, nil
	}
	if carrier, found := guardedMutationCarrier(terms, mutation.Edit, candidates); found {
		return carrier, nil
	}
	if carrier, found := rhsMutationCarrier(laws, helpers, mutation.Production, candidates, terms); found {
		return carrier, nil
	}
	for _, law := range laws {
		if law.Production != mutation.Production {
			continue
		}
		for _, product := range law.Products {
			for _, candidate := range candidates {
				if product.Constructor == candidate.Form {
					return candidate, nil
				}
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return parserproducts.Carrier{}, fmt.Errorf("parser uses: mutation field %s has no unique sealed carrier", field)
}

// guardedMutationCarrier uses the sealed type guard emitted alongside a
// controlled edit. The arena type symbol is the discriminant; no action text
// is reconstructed or parsed here.
func guardedMutationCarrier(terms parserproducts.ActionTerms, edit parserproducts.Edit, candidates []parserproducts.Carrier) (parserproducts.Carrier, bool) {
	var found parserproducts.Carrier
	for _, atom := range edit.Guard.Atoms {
		if atom.Kind != parserproducts.GuardAtomTypeIn || atom.Negated {
			continue
		}
		for index := atom.SetStart; index < atom.SetStart+uint32(atom.SetCount); index++ {
			symbol, ok := terms.Symbol(terms.GuardSymbols[index])
			if !ok || symbol.Kind != parserproducts.ActionSymbolType {
				continue
			}
			for _, candidate := range candidates {
				if symbol.Text != "*ast."+candidate.Form {
					continue
				}
				if found.Form != "" && found != candidate {
					return parserproducts.Carrier{}, false
				}
				found = candidate
			}
		}
	}
	return found, found.Form != ""
}

// rhsMutationCarrier follows the parser-product nonterminal relation when a
// mutation targets a forwarded RHS result rather than a guarded local.
func rhsMutationCarrier(laws []parserproducts.ProductLaw, helpers []parserproducts.HelperLaw, production string, candidates []parserproducts.Carrier, terms parserproducts.ActionTerms) (parserproducts.Carrier, bool) {
	forms := make(map[string]parserproducts.Carrier)
	byHelper, err := helperIndex(helpers, terms)
	if err != nil {
		return parserproducts.Carrier{}, false
	}
	for _, owner := range laws {
		if owner.Production != production {
			continue
		}
		for _, rhs := range owner.RHS {
			for _, source := range laws {
				if source.Nonterminal != rhs {
					continue
				}
				for _, product := range source.Products {
					for _, candidate := range candidates {
						if product.Constructor == candidate.Form {
							forms[candidate.Form] = candidate
						}
					}
				}
				for _, application := range source.Helpers {
					for _, candidate := range candidates {
						if helperConstructs(application.Helper, candidate.Form, byHelper, nil) {
							forms[candidate.Form] = candidate
						}
					}
				}
			}
		}
	}
	if len(forms) != 1 {
		return parserproducts.Carrier{}, false
	}
	for _, carrier := range forms {
		return carrier, true
	}
	return parserproducts.Carrier{}, false
}

func helperConstructs(helper parserproducts.ActionSymbolID, form string, laws map[parserproducts.ActionSymbolID]parserproducts.HelperLaw, active map[parserproducts.ActionSymbolID]bool) bool {
	law, ok := laws[helper]
	if !ok {
		return false
	}
	if active == nil {
		active = make(map[parserproducts.ActionSymbolID]bool)
	}
	if active[helper] {
		return false
	}
	active[helper] = true
	defer delete(active, helper)
	for _, product := range law.Products {
		if product.Constructor == form {
			return true
		}
	}
	for _, application := range law.Helpers {
		if helperConstructs(application.Helper, form, laws, active) {
			return true
		}
	}
	return false
}

func editField(terms parserproducts.ActionTerms, edit parserproducts.Edit) (string, error) {
	if err := terms.ValidateEdit(edit, edit.Place.Scope); err != nil {
		return "", fmt.Errorf("parser uses: invalid mutation edit: %w", err)
	}
	if edit.Place.StepCount == 0 {
		return "", fmt.Errorf("parser uses: mutation has no field destination")
	}
	last := edit.Place.StepStart + uint32(edit.Place.StepCount) - 1
	step := terms.PlaceSteps[last]
	if step.Kind != parserproducts.PlaceStepField {
		return "", fmt.Errorf("parser uses: mutation destination does not end in a field")
	}
	symbol, ok := terms.Symbol(step.Field)
	if !ok || symbol.Kind != parserproducts.ActionSymbolField {
		return "", fmt.Errorf("parser uses: mutation destination has invalid field")
	}
	return symbol.Text, nil
}

// verifyRoutes checks both denominators, not merely aggregate slot coverage.
func verifyRoutes(laws []parserproducts.ProductLaw, helperLaws []parserproducts.HelperLaw, mutationsSource []parserproducts.FieldMutation, carriers map[string]parserproducts.Carrier, slots []UseSlot, direct []UsePath, helpers []HelperUsePath, mutations []MutationUsePath, terms parserproducts.ActionTerms) error {
	if err := verifyDirectRoutes(laws, carriers, direct, terms); err != nil {
		return err
	}
	if err := verifyHelperRoutes(laws, helperLaws, carriers, helpers, terms); err != nil {
		return err
	}
	if err := verifyMutationRoutes(laws, helperLaws, mutationsSource, carriers, mutations, terms); err != nil {
		return err
	}
	for _, path := range direct {
		if err := validateUseAxes(path.Role, path.Child, path.Target, path.Open, path.Table, path.LValue, path.Values); err != nil {
			return fmt.Errorf("parser uses: invalid direct route axes: %w", err)
		}
	}
	for _, path := range helpers {
		if path.LValue == LValueAxisYes {
			return fmt.Errorf("parser uses: helper-built lvalue has no direct terminal proof")
		}
		if err := terms.ValidateInstance(path.Instance); err != nil {
			return fmt.Errorf("parser uses: invalid helper instance: %w", err)
		}
		if err := validateUseAxes(path.Role, path.Child, path.Target, path.Open, path.Table, path.LValue, path.Values); err != nil {
			return fmt.Errorf("parser uses: invalid helper route axes: %w", err)
		}
	}
	for _, path := range mutations {
		if path.LValue == LValueAxisYes {
			return fmt.Errorf("parser uses: mutation-built lvalue has no direct terminal proof")
		}
		if err := validateUseAxes(path.Role, path.Child, path.Target, path.Open, path.Table, path.LValue, ValuesPositionNotApplicable); err != nil {
			return fmt.Errorf("parser uses: invalid mutation route axes: %w", err)
		}
	}
	routes := make(map[carrierTarget]bool)
	for _, path := range direct {
		routes[carrierTarget{Form: path.ParentForm, Field: path.ParentField, Target: path.Target}] = true
	}
	for _, path := range helpers {
		routes[carrierTarget{Form: path.ParentForm, Field: path.ParentField, Target: path.Target}] = true
	}
	for _, path := range mutations {
		if path.Ordinal <= 0 || path.Ordinal > len(mutationsSource) {
			return fmt.Errorf("parser uses: mutation route has invalid source ordinal")
		}
		carrier, err := mutationCarrier(laws, helperLaws, mutationsSource[path.Ordinal-1], carriers, terms)
		if err != nil {
			return err
		}
		if carrier.Form != "" {
			routes[carrierTarget{Form: carrier.Form, Field: carrier.Field, Target: path.Target}] = true
		}
	}
	for _, slot := range slots {
		if slot.Disposition != parserproducts.DispositionImpossible && !routes[carrierTarget{Form: slot.ParentForm, Field: slot.ParentField, Target: slot.Target}] {
			return fmt.Errorf("parser uses: semantic carrier %s.%s target=%d has no returned construction/helper/mutation route", slot.ParentForm, slot.ParentField, slot.Target)
		}
	}
	return nil
}

type carrierTarget struct {
	Form   string
	Field  string
	Target ProgramUseClass
}

func validateUseAxes(role UseRole, child ChildClass, target ProgramUseClass, open OpenAxis, table TableAxis, lvalue LValueAxis, values ValuesPosition) error {
	if role == UseRoleInvalid || child == ChildClassInvalid || target == ProgramUseInvalid || open == OpenAxisInvalid || table == TableAxisInvalid || lvalue == LValueAxisInvalid || values == ValuesPositionInvalid {
		return fmt.Errorf("invalid axis")
	}
	return nil
}

func verifyDirectRoutes(laws []parserproducts.ProductLaw, carriers map[string]parserproducts.Carrier, paths []UsePath, terms parserproducts.ActionTerms) error {
	expected, err := buildUsePaths(laws, carriers, terms)
	if err != nil {
		return err
	}
	if len(expected) != len(paths) {
		return fmt.Errorf("parser uses: direct producing-coordinate coverage is incomplete")
	}
	for index := range expected {
		if !sameUsePath(expected[index], paths[index]) {
			return fmt.Errorf("parser uses: direct route is not an exact sealed coordinate")
		}
	}
	return nil
}

func verifyHelperRoutes(callers []parserproducts.ProductLaw, helperLaws []parserproducts.HelperLaw, carriers map[string]parserproducts.Carrier, paths []HelperUsePath, terms parserproducts.ActionTerms) error {
	expected, err := buildHelperUsePaths(callers, helperLaws, carriers, terms)
	if err != nil {
		return err
	}
	if len(expected) != len(paths) {
		return fmt.Errorf("parser uses: helper producing-coordinate coverage is incomplete")
	}
	for index := range expected {
		if !sameHelperUsePath(expected[index], paths[index]) {
			return fmt.Errorf("parser uses: helper route is not an exact sealed coordinate")
		}
	}
	return nil
}

func verifyMutationRoutes(laws []parserproducts.ProductLaw, helpers []parserproducts.HelperLaw, source []parserproducts.FieldMutation, carriers map[string]parserproducts.Carrier, paths []MutationUsePath, terms parserproducts.ActionTerms) error {
	expected, err := buildMutationUsePaths(laws, helpers, source, carriers, terms)
	if err != nil {
		return err
	}
	if len(expected) != len(paths) {
		return fmt.Errorf("parser uses: mutation producing-coordinate coverage is incomplete")
	}
	for index := range expected {
		if !sameMutationUsePath(expected[index], paths[index]) {
			return fmt.Errorf("parser uses: mutation route is not an exact sealed coordinate")
		}
	}
	return nil
}

func helperOpenAxis(terms parserproducts.ActionTerms, caller parserproducts.ProductLaw, application parserproducts.HelperApplication, form, field, child string) OpenAxis {
	if child == "ValuesAdjustment" {
		return OpenAxisFinalOpen
	}
	if form != "FuncCallExpr" || field != "Args" {
		return OpenAxisClosed
	}
	for _, actual := range application.Actuals {
		if index, ok := inputOrdinal(terms, actual, caller.Scope); ok && index <= len(caller.RHS) && caller.RHS[index-1] == "args" {
			return OpenAxisFinalOpen
		}
	}
	return OpenAxisInvalid
}

func childClass(child string) ChildClass {
	switch child {
	case "Expr":
		return ChildClassValue
	case "Stmt":
		return ChildClassStatement
	case "TypeExpr", "FunctionParamExpr", "InterfaceMember":
		return ChildClassStatic
	case "ValuesAdjustment":
		return ChildClassAdjustment
	default:
		return ChildClassStructural
	}
}

func programUse(terms parserproducts.ActionTerms, product parserproducts.ConstructorProduct, field string, role UseRole, child string) (ProgramUseClass, error) {
	if role == UseRoleAdjustment {
		return ProgramUseAdjustment, nil
	}
	if valuesListField(product.Constructor, field) {
		return ProgramUseValues, nil
	}
	if role == UseRoleLValue {
		return ProgramUseLValue, nil
	}
	if product.Constructor == "FuncDefStmt" && field == "Func" {
		return ProgramUseFunctionDefinition, nil
	}
	if product.Constructor == "AnnotatedTypeExpr" && field == "Annotations" && child == "AnnotationExpr" {
		return ProgramUseAnnotations, nil
	}
	if product.Constructor == "RecordTypeExpr" && field == "Fields" && child == "RecordFieldExpr" {
		return ProgramUseRecordFields, nil
	}
	if field == "TypeParams" && child == "TypeParamExpr" {
		return ProgramUseTypeParameters, nil
	}
	if role == UseRoleStatic {
		return ProgramUseStatic, nil
	}
	if product.Constructor == "Field" {
		switch field {
		case "Key":
			if productFieldEnum(terms, product, "KeySyntax", "ast.AttrKeyIndex") {
				return ProgramUseTableBracketKey, nil
			}
			return ProgramUseTableKey, nil
		case "Value":
			return ProgramUseTableValue, nil
		}
	}
	if product.Constructor == "TableExpr" && field == "Fields" {
		return ProgramUseTableFields, nil
	}
	if product.Constructor == "FunctionExpr" && field == "Stmts" {
		return ProgramUseFunctionBody, nil
	}
	if product.Constructor == "FunctionExpr" && field == "ParList" {
		return ProgramUseFunctionParameters, nil
	}
	if product.Constructor == "FuncDefStmt" && field == "Name" {
		return ProgramUseFunctionName, nil
	}
	if role == UseRoleControl {
		return ProgramUseControl, nil
	}
	switch child {
	case "Expr":
		return ProgramUseExpression, nil
	case "Stmt":
		return ProgramUseStatements, nil
	default:
		return ProgramUseInvalid, fmt.Errorf("unclassified target ingress %s.%s child=%s role=%d", product.Constructor, field, child, role)
	}
}

func openAxis(terms parserproducts.ActionTerms, law parserproducts.ProductLaw, form, field string, term parserproducts.ActionTermID, child string) OpenAxis {
	if child == "ValuesAdjustment" {
		return OpenAxisFinalOpen
	}
	if !valuesListField(form, field) {
		return OpenAxisClosed
	}
	if termHasKind(terms, term, parserproducts.ActionTermNil) || sequenceLiteral(terms, term, "[]ast.Expr", true) {
		return OpenAxisEmpty
	}
	if sequenceLiteral(terms, term, "[]ast.Expr", false) {
		return OpenAxisFinalOpen
	}
	if index, ok := inputOrdinal(terms, term, law.Scope); ok && index <= len(law.RHS) && law.RHS[index-1] == "exprlist" {
		return OpenAxisFinalOpen
	}
	return OpenAxisInvalid
}

func valuesListField(form, field string) bool {
	switch form + "." + field {
	case "FuncCallExpr.Args", "ReturnStmt.Exprs", "AssignStmt.Rhs", "LocalAssignStmt.Exprs", "GenericForStmt.Exprs":
		return true
	default:
		return false
	}
}

func tableAxis(terms parserproducts.ActionTerms, product parserproducts.ConstructorProduct, field string) TableAxis {
	if product.Constructor == "AttrGetExpr" && field == "Key" {
		if productFieldEnum(terms, product, "KeySyntax", "ast.AttrKeyIndex") {
			return TableAxisDynamic
		}
		return TableAxisNone
	}
	if product.Constructor != "Field" {
		return TableAxisNone
	}
	if field == "Value" {
		return TableAxisValue
	}
	if field != "Key" {
		return TableAxisNone
	}
	if productFieldEnum(terms, product, "KeySyntax", "ast.AttrKeyDot") {
		return TableAxisNamed
	}
	if productFieldEnum(terms, product, "KeySyntax", "ast.AttrKeyIndex") {
		return TableAxisBracket
	}
	return TableAxisArray
}

func lvalueAxis(form, field string) LValueAxis {
	if form == "AssignStmt" && field == "Lhs" {
		return LValueAxisYes
	}
	return LValueAxisNo
}

func buildUseSlots(fields []parserproducts.FieldState, carriers map[string]parserproducts.Carrier) ([]UseSlot, error) {
	byField := make(map[fieldStateCoordinate]parserproducts.FieldState, len(fields))
	for _, field := range fields {
		key := fieldStateCoordinate{Form: field.Form, Field: field.Field, State: field.State}
		if _, exists := byField[key]; exists {
			return nil, fmt.Errorf("parser uses: duplicate sealed field state")
		}
		byField[key] = field
	}
	result := make([]UseSlot, 0, len(carriers))
	for _, carrier := range carriers {
		field, exists := byField[fieldStateCoordinate{Form: carrier.Form, Field: carrier.Field, State: carrier.Cardinality}]
		if !exists {
			continue
		}
		role := useRole(carrier.Form, carrier.Class, carrier.Field, carrier.ChildType)
		target, err := programUse(parserproducts.ActionTerms{}, parserproducts.ConstructorProduct{Constructor: carrier.Form}, carrier.Field, role, carrier.ChildType)
		if err != nil {
			return nil, err
		}
		disposition := field.Disposition
		if disposition == parserproducts.DispositionSemanticWitness {
			disposition = parserproducts.DispositionObserved
		}
		result = append(result, UseSlot{ParentForm: carrier.Form, ParentField: carrier.Field, ParentContext: field.Context, Role: role, ChildType: carrier.ChildType, Cardinality: carrier.Cardinality, Target: target, Disposition: disposition, Source: field.Source, ParserLaw: field.ParserLaw})
	}
	sort.Slice(result, func(left, right int) bool { return useLess(result[left], result[right]) })
	for index, row := range result {
		if row.Role == UseRoleInvalid || row.Target == ProgramUseInvalid || row.ChildType == "" || row.Disposition == parserproducts.DispositionInvalid || index != 0 && !useLess(result[index-1], row) {
			return nil, fmt.Errorf("parser uses: noncanonical sealed use-slot denominator")
		}
	}
	return result, nil
}

type fieldStateCoordinate struct {
	Form  string
	Field string
	State astcodec.FieldState
}

func carrierIndex(rows []parserproducts.Carrier) (map[string]parserproducts.Carrier, error) {
	result := make(map[string]parserproducts.Carrier, len(rows))
	for _, row := range rows {
		key := carrierKey(row.Form, row.Field)
		if row.Form == "" || row.Field == "" || row.ChildType == "" || row.Cardinality == 0 {
			return nil, fmt.Errorf("parser uses: invalid sealed carrier")
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("parser uses: duplicate sealed carrier %s.%s", row.Form, row.Field)
		}
		result[key] = row
	}
	return result, nil
}

func carrierKey(form, field string) string { return form + "\x00" + field }

func useRole(form string, class parsersource.ConstructorClass, field, child string) UseRole {
	if child == "ValuesAdjustment" {
		return UseRoleAdjustment
	}
	if form == "AssignStmt" && field == "Lhs" {
		return UseRoleLValue
	}
	if class == parsersource.ConstructorTypeExpression || child == "TypeExpr" || child == "FunctionParamExpr" || child == "InterfaceMember" {
		return UseRoleStatic
	}
	switch form {
	case "IfStmt", "WhileStmt", "RepeatStmt", "NumberForStmt", "GenericForStmt", "DoBlockStmt":
		return UseRoleControl
	default:
		return UseRoleChild
	}
}

func termInScope(terms parserproducts.ActionTerms, id parserproducts.ActionTermID, scope parserproducts.ActionScopeID) bool {
	term, ok := terms.Term(id)
	return ok && term.Scope == scope
}

func inputOrdinal(terms parserproducts.ActionTerms, id parserproducts.ActionTermID, scope parserproducts.ActionScopeID) (int, bool) {
	term, ok := terms.Term(id)
	if !ok || term.Scope != scope || term.Kind != parserproducts.ActionTermInput {
		return 0, false
	}
	return int(term.Slot) + 1, true
}

func termHasKind(terms parserproducts.ActionTerms, id parserproducts.ActionTermID, kind parserproducts.ActionTermKind) bool {
	term, ok := terms.Term(id)
	return ok && term.Kind == kind
}

func sequenceLiteral(terms parserproducts.ActionTerms, id parserproducts.ActionTermID, typeName string, empty bool) bool {
	term, ok := terms.Term(id)
	if !ok || term.Kind != parserproducts.ActionTermSequence || (empty && term.EdgeCount != 0) || (!empty && term.EdgeCount == 0) {
		return false
	}
	symbol, ok := terms.Symbol(term.Symbol)
	return ok && symbol.Kind == parserproducts.ActionSymbolType && symbol.Text == typeName
}

func productFieldEnum(terms parserproducts.ActionTerms, product parserproducts.ConstructorProduct, field, value string) bool {
	for _, candidate := range product.Fields {
		if candidate.Field != field || candidate.Kind != parserproducts.ActionValueTerm {
			continue
		}
		term, termOK := terms.Term(candidate.Term)
		if !termOK || term.Kind != parserproducts.ActionTermEnum {
			return false
		}
		symbol, symbolOK := terms.Symbol(term.Symbol)
		return symbolOK && symbol.Kind == parserproducts.ActionSymbolEnum && symbol.Text == value
	}
	return false
}

func productField(product parserproducts.ConstructorProduct, field string) (parserproducts.ProductField, bool) {
	for _, candidate := range product.Fields {
		if candidate.Field == field {
			return candidate, true
		}
	}
	return parserproducts.ProductField{}, false
}

func actionFieldName(terms parserproducts.ActionTerms, id parserproducts.ActionSymbolID) (string, error) {
	symbol, ok := terms.Symbol(id)
	if !ok || symbol.Kind != parserproducts.ActionSymbolField {
		return "", fmt.Errorf("parser uses: invalid action field symbol")
	}
	return symbol.Text, nil
}

func cloneEdit(source parserproducts.Edit) parserproducts.Edit {
	result := source
	if source.Guard.Atoms != nil {
		result.Guard.Atoms = make([]parserproducts.GuardAtom, len(source.Guard.Atoms))
		copy(result.Guard.Atoms, source.Guard.Atoms)
	}
	return result
}

func sameUsePath(left, right UsePath) bool {
	return left.ParentProduction == right.ParentProduction && left.ParentOrdinal == right.ParentOrdinal && left.ParentForm == right.ParentForm && left.ParentField == right.ParentField && left.Term == right.Term && left.Role == right.Role && left.Child == right.Child && left.Target == right.Target && left.Open == right.Open && left.Table == right.Table && left.LValue == right.LValue && left.Values == right.Values
}

func sameHelperUsePath(left, right HelperUsePath) bool {
	return left.Production == right.Production && sameApplications(left.Applications, right.Applications) && left.Helper == right.Helper && left.Ordinal == right.Ordinal && left.ParentForm == right.ParentForm && left.ParentField == right.ParentField && sameInstance(left.Instance, right.Instance) && left.Role == right.Role && left.Child == right.Child && left.Target == right.Target && left.Open == right.Open && left.Table == right.Table && left.LValue == right.LValue && left.Values == right.Values
}

func sameMutationUsePath(left, right MutationUsePath) bool {
	return left.Production == right.Production && left.Ordinal == right.Ordinal && sameEdit(left.Edit, right.Edit) && left.Role == right.Role && left.Child == right.Child && left.Target == right.Target && left.Open == right.Open && left.Table == right.Table && left.LValue == right.LValue
}

func sameApplications(left, right []uint16) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameInstance(left, right parserproducts.TermInstance) bool {
	return left.CallerScope == right.CallerScope && left.HelperScope == right.HelperScope && left.Root == right.Root && sameTerms(left.Actuals, right.Actuals)
}

func sameTerms(left, right []parserproducts.ActionTermID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameEdit(left, right parserproducts.Edit) bool {
	return left.Kind == right.Kind && sameGuard(left.Guard, right.Guard) && left.Place == right.Place && left.Value == right.Value
}

func sameGuard(left, right parserproducts.Guard) bool {
	if len(left.Atoms) != len(right.Atoms) {
		return false
	}
	for index := range left.Atoms {
		if left.Atoms[index] != right.Atoms[index] {
			return false
		}
	}
	return true
}

func useLess(left, right UseSlot) bool {
	if left.ParentForm != right.ParentForm {
		return left.ParentForm < right.ParentForm
	}
	if left.ParentField != right.ParentField {
		return left.ParentField < right.ParentField
	}
	if left.ParentContext != right.ParentContext {
		return left.ParentContext < right.ParentContext
	}
	if left.Role != right.Role {
		return left.Role < right.Role
	}
	if left.ChildType != right.ChildType {
		return left.ChildType < right.ChildType
	}
	if left.Target != right.Target {
		return left.Target < right.Target
	}
	return left.Cardinality < right.Cardinality
}

func usePathLess(left, right UsePath) bool {
	if left.ParentProduction != right.ParentProduction {
		return left.ParentProduction < right.ParentProduction
	}
	if left.ParentOrdinal != right.ParentOrdinal {
		return left.ParentOrdinal < right.ParentOrdinal
	}
	if left.ParentForm != right.ParentForm {
		return left.ParentForm < right.ParentForm
	}
	if left.ParentField != right.ParentField {
		return left.ParentField < right.ParentField
	}
	if left.Term != right.Term {
		return left.Term < right.Term
	}
	if left.Role != right.Role {
		return left.Role < right.Role
	}
	if left.Child != right.Child {
		return left.Child < right.Child
	}
	if left.Target != right.Target {
		return left.Target < right.Target
	}
	if left.Open != right.Open {
		return left.Open < right.Open
	}
	if left.Table != right.Table {
		return left.Table < right.Table
	}
	if left.LValue != right.LValue {
		return left.LValue < right.LValue
	}
	return left.Values < right.Values
}

func helperUsePathLess(left, right HelperUsePath) bool {
	if left.Production != right.Production {
		return left.Production < right.Production
	}
	if lessApplications(left.Applications, right.Applications) {
		return true
	}
	if lessApplications(right.Applications, left.Applications) {
		return false
	}
	if left.Helper != right.Helper {
		return left.Helper < right.Helper
	}
	if left.Ordinal != right.Ordinal {
		return left.Ordinal < right.Ordinal
	}
	if left.ParentForm != right.ParentForm {
		return left.ParentForm < right.ParentForm
	}
	if left.ParentField != right.ParentField {
		return left.ParentField < right.ParentField
	}
	if lessInstance(left.Instance, right.Instance) {
		return true
	}
	if lessInstance(right.Instance, left.Instance) {
		return false
	}
	if left.Role != right.Role {
		return left.Role < right.Role
	}
	if left.Child != right.Child {
		return left.Child < right.Child
	}
	if left.Target != right.Target {
		return left.Target < right.Target
	}
	if left.Open != right.Open {
		return left.Open < right.Open
	}
	if left.Table != right.Table {
		return left.Table < right.Table
	}
	if left.LValue != right.LValue {
		return left.LValue < right.LValue
	}
	return left.Values < right.Values
}

func lessApplications(left, right []uint16) bool {
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return len(left) < len(right)
}

func lessInstance(left, right parserproducts.TermInstance) bool {
	if left.CallerScope != right.CallerScope {
		return left.CallerScope < right.CallerScope
	}
	if left.HelperScope != right.HelperScope {
		return left.HelperScope < right.HelperScope
	}
	if left.Root != right.Root {
		return left.Root < right.Root
	}
	for index := 0; index < len(left.Actuals) && index < len(right.Actuals); index++ {
		if left.Actuals[index] != right.Actuals[index] {
			return left.Actuals[index] < right.Actuals[index]
		}
	}
	return len(left.Actuals) < len(right.Actuals)
}

func mutationUsePathLess(left, right MutationUsePath) bool {
	if left.Production != right.Production {
		return left.Production < right.Production
	}
	if left.Ordinal != right.Ordinal {
		return left.Ordinal < right.Ordinal
	}
	if lessEdit(left.Edit, right.Edit) {
		return true
	}
	if lessEdit(right.Edit, left.Edit) {
		return false
	}
	if left.Role != right.Role {
		return left.Role < right.Role
	}
	if left.Child != right.Child {
		return left.Child < right.Child
	}
	return left.Target < right.Target
}

func lessEdit(left, right parserproducts.Edit) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Place != right.Place {
		return lessPlace(left.Place, right.Place)
	}
	if left.Value != right.Value {
		return left.Value < right.Value
	}
	return lessGuard(left.Guard, right.Guard)
}

func lessPlace(left, right parserproducts.Place) bool {
	if left.Scope != right.Scope {
		return left.Scope < right.Scope
	}
	if left.Root != right.Root {
		return left.Root < right.Root
	}
	if left.Slot != right.Slot {
		return left.Slot < right.Slot
	}
	if left.StepStart != right.StepStart {
		return left.StepStart < right.StepStart
	}
	return left.StepCount < right.StepCount
}

func lessGuard(left, right parserproducts.Guard) bool {
	for index := 0; index < len(left.Atoms) && index < len(right.Atoms); index++ {
		if left.Atoms[index] != right.Atoms[index] {
			return lessGuardAtom(left.Atoms[index], right.Atoms[index])
		}
	}
	return len(left.Atoms) < len(right.Atoms)
}

func lessGuardAtom(left, right parserproducts.GuardAtom) bool {
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

func valuesTailLess(left, right ValuesTail) bool {
	if left.Sequence.Production != right.Sequence.Production {
		return left.Sequence.Production < right.Sequence.Production
	}
	if left.Sequence.Tag != right.Sequence.Tag {
		return left.Sequence.Tag < right.Sequence.Tag
	}
	if left.Sequence.Field != right.Sequence.Field {
		return left.Sequence.Field < right.Sequence.Field
	}
	if left.Sequence.Segment != right.Sequence.Segment {
		return left.Sequence.Segment < right.Sequence.Segment
	}
	if left.Position != right.Position {
		return left.Position < right.Position
	}
	return left.Successor < right.Successor
}

func lvaluePathLess(left, right LValuePath) bool {
	if left.SeedProduction != right.SeedProduction {
		return left.SeedProduction < right.SeedProduction
	}
	if left.SeedOrdinal != right.SeedOrdinal {
		return left.SeedOrdinal < right.SeedOrdinal
	}
	if left.Sequence.Production != right.Sequence.Production {
		return left.Sequence.Production < right.Sequence.Production
	}
	if left.Sequence.Tag != right.Sequence.Tag {
		return left.Sequence.Tag < right.Sequence.Tag
	}
	if left.Sequence.Field != right.Sequence.Field {
		return left.Sequence.Field < right.Sequence.Field
	}
	if left.Sequence.Segment != right.Sequence.Segment {
		return left.Sequence.Segment < right.Sequence.Segment
	}
	if left.TerminalProduction != right.TerminalProduction {
		return left.TerminalProduction < right.TerminalProduction
	}
	if left.TerminalOrdinal != right.TerminalOrdinal {
		return left.TerminalOrdinal < right.TerminalOrdinal
	}
	return left.TerminalForm < right.TerminalForm
}
