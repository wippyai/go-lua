package static

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/semanticsource"
	staticrole "github.com/wippyai/go-lua/program/static/role"
)

var (
	// errSemanticSourceUnavailable means that the supplied owner view no
	// longer resolves to authored Static content. A zero cardinality is a
	// valid publication, so it must not be used as the unavailable signal.
	errSemanticSourceUnavailable = errors.New("program/static: semantic-source fragment unavailable")
	// errSemanticSourceIncomplete means that a typed owner view could not be
	// enumerated to its declared cardinality. The fragment is fail-closed:
	// it never publishes a partial denominator.
	errSemanticSourceIncomplete = errors.New("program/static: incomplete semantic-source fragment")
	// errSemanticSourceOverflow means that a primary or facet cardinality did
	// not fit in the publication count representation.
	errSemanticSourceOverflow = errors.New("program/static: semantic-source cardinality overflow")
)

// SemanticSourceFragment emits Static's complete authored contribution to the
// generated semantic-source catalog. It consumes only the typed owner views;
// no component internals, generic term graph, map, or sorting pass is part of
// this boundary.
//
// The primary ProgramStatic relation is the explicit sum of its authored
// static-type/operator families. TypeField is intentionally excluded: fields
// are owned child rows of records/interfaces, not static-type roots. Facets
// retain their distinct owner relations, including the sparse ClaimTarget
// relation and the sum of authored call type arguments.
func buildSemanticSourceFragment(view View) ([]semanticsource.Publication, error) {
	counts, err := staticSemanticSourceCounts(view)
	if err != nil {
		return nil, err
	}
	definitions, err := staticSemanticSourceDefinitions()
	if err != nil {
		return nil, err
	}
	if len(definitions) != len(counts) {
		return nil, errors.New("program/static: semantic-source definition count mismatch")
	}
	result := make([]semanticsource.Publication, 0, len(definitions))
	for index, definition := range definitions {
		publication, err := semanticsource.SealPublication(definition, counts[index])
		if err != nil {
			return nil, err
		}
		result = append(result, publication)
	}
	return result, nil
}

// staticSemanticSourceCounts validates the live typed Static projections and
// returns only the ten canonical owner-local cardinalities. It retains no
// publication claims and is reused by the cached view's scalar integrity
// check.
func staticSemanticSourceCounts(view View) ([semanticSourceFragmentPublicationCount]int, error) {
	var counts [semanticSourceFragmentPublicationCount]int
	if !view.Available() {
		return counts, errSemanticSourceUnavailable
	}

	types := view.Types()
	declarations := view.Declarations()
	signatures := view.Signatures()
	contracts := view.Contracts()
	operators := view.Operators()
	operands := view.Operands()
	validNode := func(term keyspace.Term) bool { return staticNodeTerm(view, term) }
	declaredTypes := declarations.DeclaredTypes()
	if err := validateDeclaredTypes(declaredTypes, validNode); err != nil {
		return counts, err
	}

	// Validate every typed dense enumeration before deriving any publication.
	// A valid immutable Component makes these checks allocation-free and
	// deterministic; they also prevent a malformed/incomplete owner view from
	// producing a seemingly complete but truncated catalog fragment.
	for _, family := range []struct {
		name    string
		count   func() int
		at      func(int) (keyspace.Term, bool)
		kind    keyspace.Family
		payload func(keyspace.Term) bool
	}{
		{"Alias", declarations.Aliases().Count, declarations.Aliases().At, keyspace.FamilyTypeAlias, func(term keyspace.Term) bool { _, _, _, _, ok := declarations.Aliases().Get(term); return ok }},
		{"Interface", declarations.Interfaces().Count, declarations.Interfaces().At, keyspace.FamilyTypeInterface, func(term keyspace.Term) bool { _, _, _, ok := declarations.Interfaces().Get(term); return ok }},
		{"TypeParam", declarations.TypeParams().Count, declarations.TypeParams().At, keyspace.FamilyTypeParam, func(term keyspace.Term) bool { _, _, _, ok := declarations.TypeParams().Get(term); return ok }},
		{"Primitive", types.Primitives().Count, types.Primitives().At, keyspace.FamilyTypePrimitive, func(term keyspace.Term) bool { _, ok := types.Primitives().Get(term); return ok }},
		{"Literal", types.Literals().Count, types.Literals().At, keyspace.FamilyTypeLiteral, func(term keyspace.Term) bool { _, _, _, ok := types.Literals().Get(term); return ok }},
		{"Optional", types.Optionals().Count, types.Optionals().At, keyspace.FamilyTypeOptional, func(term keyspace.Term) bool { _, ok := types.Optionals().Get(term); return ok }},
		{"Union", types.Unions().Count, types.Unions().At, keyspace.FamilyTypeUnion, func(term keyspace.Term) bool { _, ok := types.Unions().MemberCount(term); return ok }},
		{"Intersection", types.Intersections().Count, types.Intersections().At, keyspace.FamilyTypeIntersection, func(term keyspace.Term) bool { _, ok := types.Intersections().MemberCount(term); return ok }},
		{"Generic", types.Generics().Count, types.Generics().At, keyspace.FamilyTypeGeneric, func(term keyspace.Term) bool { _, _, ok := types.Generics().Get(term); return ok }},
		{"Array", types.Arrays().Count, types.Arrays().At, keyspace.FamilyTypeArray, func(term keyspace.Term) bool { _, _, ok := types.Arrays().Get(term); return ok }},
		{"Map", types.Maps().Count, types.Maps().At, keyspace.FamilyTypeMap, func(term keyspace.Term) bool { _, _, _, ok := types.Maps().Get(term); return ok }},
		{"Record", types.Records().Count, types.Records().At, keyspace.FamilyTypeRecord, func(term keyspace.Term) bool { _, _, ok := types.Records().Get(term); return ok }},
		{"Reference", view.References().Count, view.References().At, keyspace.FamilyTypeRef, func(term keyspace.Term) bool { _, _, _, ok := view.References().Get(term); return ok }},
		{"TypeFunction", signatures.TypeFunctions().Count, signatures.TypeFunctions().At, keyspace.FamilyTypeFunction, func(term keyspace.Term) bool { _, _, _, _, ok := signatures.TypeFunctions().Get(term); return ok }},
		{"Assertion", signatures.Assertions().Count, signatures.Assertions().At, keyspace.FamilyTypeAsserts, func(term keyspace.Term) bool { _, _, _, _, _, ok := signatures.Assertions().Get(term); return ok }},
		{"TypeOf", operators.TypeOfs().Count, operators.TypeOfs().At, keyspace.FamilyTypeOf, func(term keyspace.Term) bool { _, _, ok := operators.TypeOfs().Get(term); return ok }},
		{"KeyOf", operators.KeyOfs().Count, operators.KeyOfs().At, keyspace.FamilyTypeKeyOf, func(term keyspace.Term) bool { _, ok := operators.KeyOfs().Get(term); return ok }},
		{"IndexAccess", operators.IndexAccesses().Count, operators.IndexAccesses().At, keyspace.FamilyTypeIndexAccess, func(term keyspace.Term) bool { _, _, ok := operators.IndexAccesses().Get(term); return ok }},
		{"Conditional", operators.Conditionals().Count, operators.Conditionals().At, keyspace.FamilyTypeConditional, func(term keyspace.Term) bool { _, _, _, _, ok := operators.Conditionals().Get(term); return ok }},
	} {
		if err := validateTypedEnumeration(family.name, family.count, family.at, family.kind, family.payload); err != nil {
			return counts, err
		}
	}

	// Sidecar parents are separately enumerated because their facets have
	// different cardinality laws than the static-type primary relation.
	if err := validateTypedEnumeration("Function", contracts.Functions().Count, contracts.Functions().At, keyspace.FamilyFunction, func(term keyspace.Term) bool { _, ok := contracts.Functions().Get(term); return ok }); err != nil {
		return counts, err
	}
	if err := validateTypedEnumeration("Call", contracts.Calls().Count, contracts.Calls().At, keyspace.FamilyCall, func(term keyspace.Term) bool { _, ok := contracts.Calls().TypeArgumentCount(term); return ok }); err != nil {
		return counts, err
	}
	if err := validateTypedEnumeration("TypeValue", operands.TypeValues().Count, operands.TypeValues().At, keyspace.FamilyTypeValue, func(term keyspace.Term) bool {
		target, ok := operands.TypeValues().Target(term)
		return ok && validNode(target)
	}); err != nil {
		return counts, err
	}
	if err := validateSparseClaimTargets(operands.Claims(), validNode); err != nil {
		return counts, err
	}
	if err := validateTypedEnumeration("Annotation", operands.Annotations().Count, operands.Annotations().At, keyspace.FamilyAnnotation, func(term keyspace.Term) bool { _, ok := operands.Annotations().Get(term); return ok }); err != nil {
		return counts, err
	}
	if err := validateTypedEnumeration("Publication", view.Publications().Count, view.Publications().At, keyspace.FamilyTypePublication, func(term keyspace.Term) bool { _, _, _, ok := view.Publications().Get(term); return ok }); err != nil {
		return counts, err
	}

	primary, ok := sumStaticPrimary(
		declarations.Aliases().Count(),
		declarations.Interfaces().Count(),
		declarations.TypeParams().Count(),
		types.Primitives().Count(),
		types.Literals().Count(),
		types.Optionals().Count(),
		types.Unions().Count(),
		types.Intersections().Count(),
		types.Generics().Count(),
		types.Arrays().Count(),
		types.Maps().Count(),
		types.Records().Count(),
		view.References().Count(),
		signatures.TypeFunctions().Count(),
		signatures.Assertions().Count(),
		operators.TypeOfs().Count(),
		operators.KeyOfs().Count(),
		operators.IndexAccesses().Count(),
		operators.Conditionals().Count(),
	)
	if !ok {
		return counts, errSemanticSourceOverflow
	}

	callTypeArguments, ok := sumCallTypeArguments(contracts.Calls(), validNode)
	if !ok {
		return counts, errSemanticSourceIncomplete
	}

	claimTargets := operands.Claims().Count()
	typeValueTargets := operands.TypeValues().Count()
	typeOfs := operators.TypeOfs().Count()
	annotations := operands.Annotations().Count()
	publications := view.Publications().Count()
	typeRefs := view.References().Count()

	counts = [...]int{primary, contracts.Functions().Count(), callTypeArguments, declaredTypes.Count(), claimTargets, typeValueTargets, typeOfs, annotations, publications, typeRefs}
	return counts, nil
}

func sumStaticPrimary(counts ...int) (int, bool) {
	total := 0
	for _, count := range counts {
		var ok bool
		total, ok = addCount(total, count)
		if !ok {
			return 0, false
		}
	}
	return total, true
}

func sumCallTypeArguments(calls Calls, validNode func(keyspace.Term) bool) (int, bool) {
	total := 0
	count := calls.Count()
	if !keyspace.TermOrdinalFits(count) {
		return 0, false
	}
	for index := 0; index < count; index++ {
		term, ok := calls.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyCall, uint32(index+1)) {
			return 0, false
		}
		arguments, ok := calls.TypeArgumentCount(term)
		if !ok || !keyspace.TermOrdinalFits(arguments) {
			return 0, false
		}
		total, ok = addCount(total, arguments)
		if !ok {
			return 0, false
		}
		for argumentIndex := 0; argumentIndex < arguments; argumentIndex++ {
			argument, ok := calls.TypeArgumentAt(term, argumentIndex)
			if !ok || argument == 0 || validNode == nil || !validNode(argument) {
				return 0, false
			}
		}
	}
	return total, true
}

func staticNodeTerm(view View, term keyspace.Term) bool {
	family := keyspace.TermFamily(term)
	if !staticrole.NodeFamily(family) {
		return false
	}
	var count int
	switch family {
	case keyspace.FamilyTypePrimitive:
		count = view.Types().Primitives().Count()
	case keyspace.FamilyTypeLiteral:
		count = view.Types().Literals().Count()
	case keyspace.FamilyTypeOptional:
		count = view.Types().Optionals().Count()
	case keyspace.FamilyTypeUnion:
		count = view.Types().Unions().Count()
	case keyspace.FamilyTypeIntersection:
		count = view.Types().Intersections().Count()
	case keyspace.FamilyTypeRef:
		count = view.References().Count()
	case keyspace.FamilyTypeGeneric:
		count = view.Types().Generics().Count()
	case keyspace.FamilyTypeArray:
		count = view.Types().Arrays().Count()
	case keyspace.FamilyTypeMap:
		count = view.Types().Maps().Count()
	case keyspace.FamilyTypeRecord:
		count = view.Types().Records().Count()
	case keyspace.FamilyTypeFunction:
		count = view.Signatures().TypeFunctions().Count()
	case keyspace.FamilyTypeAsserts:
		count = view.Signatures().Assertions().Count()
	case keyspace.FamilyTypeOf:
		count = view.Operators().TypeOfs().Count()
	case keyspace.FamilyTypeKeyOf:
		count = view.Operators().KeyOfs().Count()
	case keyspace.FamilyTypeIndexAccess:
		count = view.Operators().IndexAccesses().Count()
	case keyspace.FamilyTypeConditional:
		count = view.Operators().Conditionals().Count()
	default:
		return false
	}
	return keyspace.ValidTerm(term, family, count)
}

func validateDeclaredTypes(view DeclaredTypes, validNode func(keyspace.Term) bool) error {
	count := view.Count()
	if !keyspace.TermOrdinalFits(count) {
		return errSemanticSourceIncomplete
	}
	for index := 0; index < count; index++ {
		term, ok := view.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyDeclaredType, uint32(index+1)) {
			return errors.Join(errSemanticSourceIncomplete, errors.New("DeclaredType enumeration"))
		}
		cell, target, ok := view.Get(term)
		if !ok || keyspace.TermFamily(cell) != keyspace.FamilyCell || keyspace.TermOrdinal(cell) == 0 ||
			target == 0 || validNode == nil || !validNode(target) {
			return errors.Join(errSemanticSourceIncomplete, errors.New("DeclaredType row"))
		}
	}
	if _, ok := view.At(count); ok {
		return errors.Join(errSemanticSourceIncomplete, errors.New("DeclaredType has trailing row"))
	}
	return nil
}

func validateSparseClaimTargets(view ClaimTargets, validNode func(keyspace.Term) bool) error {
	component := view.componentOf()
	count := view.Count()
	if component == nil || !keyspace.TermOrdinalFits(count) {
		return errSemanticSourceIncomplete
	}
	previous := keyspace.Term(0)
	for index := 0; index < count; index++ {
		claim, ok := view.At(index)
		if !ok || !keyspace.ValidTerm(claim, keyspace.FamilyValueClaim, len(component.operands.claimTargets)) ||
			(index > 0 && claim <= previous) {
			return errors.Join(errSemanticSourceIncomplete, errors.New("ClaimTarget enumeration"))
		}
		target, ok := view.Target(claim)
		if !ok || target == 0 || validNode == nil || !validNode(target) {
			return errors.Join(errSemanticSourceIncomplete, errors.New("ClaimTarget row"))
		}
		previous = claim
	}
	if _, ok := view.At(count); ok {
		return errors.Join(errSemanticSourceIncomplete, errors.New("ClaimTarget has trailing row"))
	}
	return nil
}

func addCount(left, right int) (int, bool) {
	if left < 0 || right < 0 || right > math.MaxInt-left {
		return 0, false
	}
	return left + right, true
}

func validateTypedEnumeration(name string, count func() int, at func(int) (keyspace.Term, bool), family keyspace.Family, payload func(keyspace.Term) bool) error {
	length := count()
	if !keyspace.TermOrdinalFits(length) {
		return errSemanticSourceIncomplete
	}
	for index := 0; index < length; index++ {
		term, ok := at(index)
		if !ok || term == 0 || keyspace.TermFamily(term) != family || keyspace.TermOrdinal(term) != uint32(index+1) {
			return errors.Join(errSemanticSourceIncomplete, errors.New(name+" enumeration"))
		}
		if payload == nil || !payload(term) {
			return errors.Join(errSemanticSourceIncomplete, errors.New(name+" row"))
		}
	}
	if _, ok := at(length); ok {
		return errors.Join(errSemanticSourceIncomplete, errors.New(name+" has trailing row"))
	}
	return nil
}
