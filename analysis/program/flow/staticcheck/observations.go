package staticcheck

import (
	"errors"

	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func validateObservations(
	sourceView source.View,
	flowView authored.View,
	staticView staticquery.View,
	bodies *body.Result,
	forest *containment.Result,
	proof *containment.StaticScopeProof,
	bindings binding.Result,
	tree *contextTree,
) error {
	points := newObservationPoints(sourceView, flowView, forest, tree)
	if err := validateObservationRows(sourceView, flowView, staticView, bodies, forest, proof, tree, points, true); err != nil {
		return err
	}
	if err := points.resolveDescriptors(); err != nil {
		return err
	}
	if err := points.reanchorFunctions(); err != nil {
		return err
	}
	if err := validateObservationRows(sourceView, flowView, staticView, bodies, forest, proof, tree, points, false); err != nil {
		return err
	}
	typeFunctions := staticView.Signatures().TypeFunctions()
	for ordinal := 1; ordinal <= typeFunctions.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyTypeFunction, uint32(ordinal))
		at, ok := typeFunctions.At(ordinal - 1)
		if !ok || at != term {
			return errors.New("program/flow/staticcheck: noncanonical TypeFunction ordinal")
		}
		scope, _, _, _, ok := typeFunctions.Get(term)
		if !ok {
			return errors.New("program/flow/staticcheck: TypeFunction row is unavailable")
		}
		if _, _, err := validateScope(sourceView, flowView, staticView, bodies, proof, points, scope); err != nil {
			return err
		}
	}
	if err := validateStaticOccurrences(sourceView, flowView, forest, bindings, points); err != nil {
		return err
	}
	return nil
}

func validateObservationRows(
	sourceView source.View,
	flowView authored.View,
	staticView staticquery.View,
	bodies *body.Result,
	forest *containment.Result,
	proof *containment.StaticScopeProof,
	tree *contextTree,
	points *observationPoints,
	collect bool,
) error {
	typeOfs := staticView.Operators().TypeOfs()
	for ordinal := 1; ordinal <= typeOfs.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyTypeOf, uint32(ordinal))
		at, ok := typeOfs.At(ordinal - 1)
		if !ok || at != term {
			return errors.New("program/flow/staticcheck: noncanonical TypeOf ordinal")
		}
		scope, operand, ok := typeOfs.Get(term)
		if !ok || !forest.Static(term) || !forest.Static(operand) {
			return errors.New("program/flow/staticcheck: TypeOf static membership is invalid")
		}
		bodyTerm, observationKind, observation, err := observationDescriptorFor(sourceView, flowView, bodies, proof, tree, scope)
		if err != nil {
			return err
		}
		if !forest.Contains(bodyTerm, operand) {
			return errors.New("program/flow/staticcheck: TypeOf scope and operand Bodies disagree")
		}
		if collect {
			if err := points.assignDescriptor(operand, bodyTerm, observationKind, observation); err != nil {
				return err
			}
		} else {
			resolvedBody, _, err := validateScope(sourceView, flowView, staticView, bodies, proof, points, scope)
			if err != nil || resolvedBody != bodyTerm {
				if err != nil {
					return err
				}
				return errors.New("program/flow/staticcheck: TypeOf scope descriptor changed")
			}
		}
	}
	annotations := staticView.Operands().Annotations()
	for ordinal := 1; ordinal <= annotations.Count(); ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyAnnotation, uint32(ordinal))
		at, ok := annotations.At(ordinal - 1)
		if !ok || at != term {
			return errors.New("program/flow/staticcheck: noncanonical Annotation ordinal")
		}
		row, ok := annotations.Get(term)
		if !ok || !forest.Static(term) || !annotationTarget(staticView, row.Target) {
			return errors.New("program/flow/staticcheck: Annotation static membership is invalid")
		}
		bodyTerm, observationKind, observation, err := observationDescriptorFor(sourceView, flowView, bodies, proof, tree, row.Scope)
		if err != nil {
			return err
		}
		valuesOwner, _, valuesOK := flowView.Values().Get(row.Values)
		if !valuesOK || valuesOwner != bodyTerm || !forest.Static(row.Values) {
			return errors.New("program/flow/staticcheck: Annotation Values scope disagrees")
		}
		if collect {
			if err := points.assignDescriptor(row.Values, bodyTerm, observationKind, observation); err != nil {
				return err
			}
		} else {
			resolvedBody, _, err := validateScope(sourceView, flowView, staticView, bodies, proof, points, row.Scope)
			if err != nil || resolvedBody != bodyTerm {
				if err != nil {
					return err
				}
				return errors.New("program/flow/staticcheck: Annotation scope descriptor changed")
			}
		}
	}
	return nil
}

// validateStaticOccurrences is the one dense containment scan for static
// authored occurrences. It checks the exact Source Position of every static
// Read, Vararg, and Function; non-static counterparts are deliberately
// skipped. Static Values membership alone is not a visibility proof.
func validateStaticOccurrences(
	sourceView source.View,
	flowView authored.View,
	forest *containment.Result,
	bindings binding.Result,
	points *observationPoints,
) error {
	for index := 0; index < forest.Count(); index++ {
		term, ok := forest.At(index)
		if !ok || term == 0 {
			return errors.New("program/flow/staticcheck: containment scan is not canonical")
		}
		if !forest.Static(term) {
			continue
		}
		switch keyspace.TermFamily(term) {
		case keyspace.FamilyRead:
			if err := validateStaticReadAt(flowView, forest, bindings, points, term); err != nil {
				return err
			}
		case keyspace.FamilyVararg:
			if err := validateStaticVarargAt(flowView, bindings, points, term); err != nil {
				return err
			}
		case keyspace.FamilyFunction:
			if err := validateFunctionCapturesAt(flowView, bindings, points, term); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateStaticReadAt(
	flowView authored.View,
	forest *containment.Result,
	bindings binding.Result,
	points *observationPoints,
	read keyspace.Term,
) error {
	owner, sourceTerm, implicit, ok := flowView.Storage().Reads().Get(read)
	if !ok || implicit {
		return errors.New("program/flow/staticcheck: static Read is implicit or unavailable")
	}
	point, pointErr := points.point(read)
	bodyTerm, bodyOK := points.tree.pointBody(point)
	if pointErr != nil || !bodyOK || owner != bodyTerm {
		return errors.New("program/flow/staticcheck: static Read owner disagrees with Position")
	}
	if keyspace.TermFamily(sourceTerm) != keyspace.FamilyCell {
		if keyspace.TermFamily(sourceTerm) != keyspace.FamilyLensExact && keyspace.TermFamily(sourceTerm) != keyspace.FamilyLensKey {
			return errors.New("program/flow/staticcheck: static Read source is unavailable")
		}
		if forest == nil || !forest.Static(sourceTerm) || !validateStaticReadLens(flowView, points, sourceTerm, bodyTerm) {
			return errors.New("program/flow/staticcheck: static Read lens is not local")
		}
		return nil
	}
	cells := flowView.Storage().Cells()
	cellKind, cellBody, cellKey, cellOK := cells.Get(sourceTerm)
	role, roleOK := bindings.Role(sourceTerm)
	if !cellOK || !roleOK {
		return errors.New("program/flow/staticcheck: static Read Cell is unavailable")
	}
	if cellKind == authored.CellGlobal {
		if role != kind.CellGlobal || cellBody != 0 || cellKey == 0 {
			return errors.New("program/flow/staticcheck: static Read global Cell is malformed")
		}
		return nil
	}
	if cellKind != authored.CellLocal || role == kind.CellGlobal || cellBody == 0 || cellKey != 0 || !points.tree.cellVisible(point, sourceTerm) {
		return errors.New("program/flow/staticcheck: static Read Cell is not visible")
	}
	return nil
}

func validateStaticReadLens(
	flowView authored.View,
	points *observationPoints,
	lens keyspace.Term,
	wantBody keyspace.Term,
) bool {
	var base keyspace.Term
	var owner keyspace.Term
	var ok bool
	switch keyspace.TermFamily(lens) {
	case keyspace.FamilyLensExact:
		owner, base, _, _, ok = flowView.Access().Exact().Get(lens)
	case keyspace.FamilyLensKey:
		owner, base, _, ok = flowView.Access().Dynamic().Get(lens)
	default:
		return false
	}
	if !ok || owner != wantBody {
		return false
	}
	point, err := points.point(base)
	body, bodyOK := points.tree.pointBody(point)
	return err == nil && bodyOK && body == wantBody
}

func validateStaticVarargAt(
	flowView authored.View,
	bindings binding.Result,
	points *observationPoints,
	vararg keyspace.Term,
) error {
	owner, cell, ok := flowView.Storage().Varargs().Get(vararg)
	if !ok {
		return errors.New("program/flow/staticcheck: static Vararg is unavailable")
	}
	point, pointErr := points.point(vararg)
	bodyTerm, bodyOK := points.tree.pointBody(point)
	if pointErr != nil || !bodyOK || owner != bodyTerm {
		return errors.New("program/flow/staticcheck: static Vararg owner disagrees with Position")
	}
	cellKind, cellBody, cellKey, cellOK := flowView.Storage().Cells().Get(cell)
	role, roleOK := bindings.Role(cell)
	if !cellOK || !roleOK || cellKind != authored.CellLocal || role == kind.CellGlobal || cellBody == 0 || cellKey != 0 || !points.tree.cellVisible(point, cell) {
		return errors.New("program/flow/staticcheck: static Vararg Cell is not visible")
	}
	return nil
}

// validateFunctionCaptures checks visibility at the Function's creation
// point and at its header. A FunctionCell is the only permitted exception to
// creation visibility: the exact Cell returned by FunctionCell(function).
func validateFunctionCapturesAt(
	flowView authored.View,
	bindings binding.Result,
	points *observationPoints,
	function keyspace.Term,
) error {
	functions := flowView.Functions()
	cells := flowView.Storage().Cells()
	owner, functionBody, _, rowOK := functions.Get(function)
	if !rowOK {
		return errors.New("program/flow/staticcheck: Function capture owner is unavailable")
	}
	creationPoint, creationErr := points.point(function)
	creationBody, bodyOK := points.tree.pointBody(creationPoint)
	if creationErr != nil || !bodyOK || creationBody != owner {
		return errors.New("program/flow/staticcheck: Function creation point disagrees")
	}
	headerPoint, headerOK := points.tree.pointAt(functionBody, 0)
	if !headerOK {
		return errors.New("program/flow/staticcheck: Function header point is unavailable")
	}
	self, selfOK := bindings.FunctionCell(function)
	captureCount, countOK := functions.CaptureCount(function)
	if !countOK || captureCount < 0 {
		return errors.New("program/flow/staticcheck: Function capture range is unavailable")
	}
	for index := 0; index < captureCount; index++ {
		inner, outer, captureOK := functions.CaptureAt(function, index)
		if !captureOK {
			return errors.New("program/flow/staticcheck: Function capture range is unavailable")
		}
		innerRole, innerRoleOK := bindings.Role(inner)
		innerHost, innerHostOK := bindings.Host(inner)
		outerRole, outerRoleOK := bindings.Role(outer)
		outerHost, outerHostOK := bindings.Host(outer)
		_, innerBody, innerKey, innerRowOK := cells.Get(inner)
		_, outerBody, outerKey, outerRowOK := cells.Get(outer)
		if !innerRoleOK || !innerHostOK || innerRole != kind.CellCapture || innerHost != function ||
			!outerRoleOK || !outerHostOK || outerRole == kind.CellGlobal || outerHost == 0 ||
			!innerRowOK || innerBody != functionBody || innerKey != 0 || !outerRowOK || outerBody == 0 || outerKey != 0 {
			return errors.New("program/flow/staticcheck: Capture binding role is unavailable")
		}
		if !points.tree.cellVisible(headerPoint, inner) {
			return errors.New("program/flow/staticcheck: Capture Inner is not visible at header")
		}
		if !points.tree.cellVisible(creationPoint, outer) && (!selfOK || outer != self) {
			return errors.New("program/flow/staticcheck: Capture Outer is not visible at creation")
		}
	}
	return nil
}

// observationDescriptorFor validates only the descriptor projections of one
// StaticScopeProof row. It intentionally does not resolve a lexical point:
// all row descriptors must be installed before the seed dependency walk.
func observationDescriptorFor(
	sourceView source.View,
	flowView authored.View,
	bodies *body.Result,
	proof *containment.StaticScopeProof,
	tree *contextTree,
	scope keyspace.Term,
) (keyspace.Term, containment.ScopeObservationKind, keyspace.Term, error) {
	bodyTerm, bodyOK := proof.Body(scope)
	observationKind, observation, observationOK := proof.Observation(scope)
	if !bodyOK || !observationOK || bodyTerm == 0 || observation == 0 {
		return 0, containment.ScopeObservationInvalid, 0, errors.New("program/flow/staticcheck: Static scope proof observation is unavailable")
	}
	if keyspace.TermFamily(bodyTerm) != keyspace.FamilyBody || keyspace.TermOrdinal(bodyTerm) == 0 || int(keyspace.TermOrdinal(bodyTerm)) > bodies.BodyCount() {
		return 0, containment.ScopeObservationInvalid, 0, errors.New("program/flow/staticcheck: Static scope proof Body is invalid")
	}
	switch observationKind {
	case containment.ScopeObservationSourceOccurrence:
		if _, _, ok := observationTerm(observation, sourceView); !ok {
			return 0, containment.ScopeObservationInvalid, 0, errors.New("program/flow/staticcheck: source scope descriptor is unavailable")
		}
	case containment.ScopeObservationCellIntroduction:
		if keyspace.TermFamily(observation) != keyspace.FamilyCell || keyspace.TermOrdinal(observation) == 0 {
			return 0, containment.ScopeObservationInvalid, 0, errors.New("program/flow/staticcheck: Cell scope descriptor is invalid")
		}
		cellKind, cellBody, cellKey, ok := flowView.Storage().Cells().Get(observation)
		if !ok || cellKind != authored.CellLocal || cellBody != bodyTerm || cellKey != 0 || tree == nil || int(keyspace.TermOrdinal(observation)) >= len(tree.cellScope) {
			return 0, containment.ScopeObservationInvalid, 0, errors.New("program/flow/staticcheck: Cell scope descriptor disagrees")
		}
	case containment.ScopeObservationFunctionGeneric, containment.ScopeObservationFunctionHeader:
		if keyspace.TermFamily(observation) != keyspace.FamilyFunction || keyspace.TermOrdinal(observation) == 0 {
			return 0, containment.ScopeObservationInvalid, 0, errors.New("program/flow/staticcheck: Function scope descriptor is invalid")
		}
		owner, functionBody, _, ok := flowView.Functions().Get(observation)
		if !ok {
			return 0, containment.ScopeObservationInvalid, 0, errors.New("program/flow/staticcheck: Function scope descriptor is unavailable")
		}
		if observationKind == containment.ScopeObservationFunctionGeneric && owner != bodyTerm {
			return 0, containment.ScopeObservationInvalid, 0, errors.New("program/flow/staticcheck: Function generic descriptor Body disagrees")
		}
		if observationKind == containment.ScopeObservationFunctionHeader && functionBody != bodyTerm {
			return 0, containment.ScopeObservationInvalid, 0, errors.New("program/flow/staticcheck: Function header descriptor Body disagrees")
		}
		functionOrdinal := int(keyspace.TermOrdinal(observation))
		if tree == nil || functionOrdinal <= 0 || functionOrdinal >= len(tree.paramFirst) || observationKind == containment.ScopeObservationFunctionGeneric && tree.paramFirst[functionOrdinal] == 0 {
			return 0, containment.ScopeObservationInvalid, 0, errors.New("program/flow/staticcheck: Function generic descriptor range is unavailable")
		}
	default:
		return 0, containment.ScopeObservationInvalid, 0, errors.New("program/flow/staticcheck: invalid Static scope descriptor kind")
	}
	return bodyTerm, observationKind, observation, nil
}

// validateScope consumes both projections of one exact StaticScopeProof row.
// It deliberately does not derive scope from an alternate source walk or
// authored owner walk: the proof's Body and terminal observation are the two
// authorities being checked.
func validateScope(
	sourceView source.View,
	flowView authored.View,
	staticView staticquery.View,
	bodies *body.Result,
	proof *containment.StaticScopeProof,
	points *observationPoints,
	scope keyspace.Term,
) (keyspace.Term, int, error) {
	bodyTerm, bodyOK := proof.Body(scope)
	observationKind, observation, observationOK := proof.Observation(scope)
	if !bodyOK || !observationOK || bodyTerm == 0 || observation == 0 {
		return 0, 0, errors.New("program/flow/staticcheck: Static scope proof observation is unavailable")
	}
	if keyspace.TermFamily(bodyTerm) != keyspace.FamilyBody || keyspace.TermOrdinal(bodyTerm) == 0 || int(keyspace.TermOrdinal(bodyTerm)) > bodies.BodyCount() {
		return 0, 0, errors.New("program/flow/staticcheck: Static scope proof Body is invalid")
	}
	validatedPoint := 0
	switch observationKind {
	case containment.ScopeObservationSourceOccurrence:
		point, err := points.point(observation)
		if err != nil {
			return 0, 0, errors.New("program/flow/staticcheck: source scope observation has no Position")
		}
		observedBody, observedBodyOK := points.tree.pointBody(point)
		if !observedBodyOK || observedBody != bodyTerm {
			return 0, 0, errors.New("program/flow/staticcheck: source scope observation Body disagrees")
		}
		validatedPoint = point
	case containment.ScopeObservationCellIntroduction:
		if keyspace.TermFamily(observation) != keyspace.FamilyCell || keyspace.TermOrdinal(observation) == 0 {
			return 0, 0, errors.New("program/flow/staticcheck: Cell scope observation is invalid")
		}
		ordinal := keyspace.TermOrdinal(observation)
		if uint64(ordinal) >= uint64(len(points.tree.cellScope)) {
			return 0, 0, errors.New("program/flow/staticcheck: Cell scope observation is unavailable")
		}
		point := points.tree.cellScope[ordinal]
		if point == 0 {
			return 0, 0, errors.New("program/flow/staticcheck: Cell scope observation has no introduction")
		}
		observedBody, observedBodyOK := points.tree.pointBody(point)
		if !observedBodyOK || observedBody != bodyTerm {
			return 0, 0, errors.New("program/flow/staticcheck: Cell scope observation Body disagrees")
		}
		validatedPoint = point
	case containment.ScopeObservationFunctionGeneric:
		if err := validateFunctionObservationAt(sourceView, flowView, points, observation, true, bodyTerm); err != nil {
			return 0, 0, err
		}
		point, err := points.point(observation)
		if err != nil {
			return 0, 0, errors.New("program/flow/staticcheck: Function generic point is unavailable")
		}
		validatedPoint = point
	case containment.ScopeObservationFunctionHeader:
		if err := validateFunctionObservationAt(sourceView, flowView, points, observation, false, bodyTerm); err != nil {
			return 0, 0, err
		}
		_, functionBody, _, ok := flowView.Functions().Get(observation)
		if !ok {
			return 0, 0, errors.New("program/flow/staticcheck: Function header point is unavailable")
		}
		point, ok := points.tree.pointAt(functionBody, 0)
		if !ok {
			return 0, 0, errors.New("program/flow/staticcheck: Function header point is unavailable")
		}
		validatedPoint = point
	default:
		return 0, 0, errors.New("program/flow/staticcheck: invalid Static scope observation kind")
	}
	if validatedPoint <= 0 {
		return 0, 0, errors.New("program/flow/staticcheck: Static scope point is unavailable")
	}
	return bodyTerm, validatedPoint, nil
}

func validateFunctionObservationAt(
	sourceView source.View,
	flowView authored.View,
	points *observationPoints,
	function keyspace.Term,
	generic bool,
	wantBody keyspace.Term,
) error {
	if keyspace.TermFamily(function) != keyspace.FamilyFunction || keyspace.TermOrdinal(function) == 0 || int(keyspace.TermOrdinal(function)) > flowView.Functions().Count() {
		return errors.New("program/flow/staticcheck: Function scope observation is invalid")
	}
	owner, functionBody, _, ok := flowView.Functions().Get(function)
	if !ok {
		return errors.New("program/flow/staticcheck: Function scope Body disagrees")
	}
	point := 0
	pointOK := false
	if generic {
		if owner != wantBody {
			return errors.New("program/flow/staticcheck: Function generic Body disagrees")
		}
		resolved, err := points.point(function)
		point, pointOK = resolved, err == nil
	} else {
		if functionBody != wantBody {
			return errors.New("program/flow/staticcheck: Function header Body disagrees")
		}
		point, pointOK = points.tree.pointAt(functionBody, 0)
	}
	if !pointOK {
		return errors.New("program/flow/staticcheck: Function scope point is unavailable")
	}
	pointBody, bodyOK := points.tree.pointBody(point)
	wantPointBody := owner
	if !generic {
		wantPointBody = functionBody
	}
	if !bodyOK || pointBody != wantPointBody {
		return errors.New("program/flow/staticcheck: Function scope point Body disagrees")
	}
	functionOrdinal := int(keyspace.TermOrdinal(function))
	if functionOrdinal <= 0 || functionOrdinal >= len(points.tree.paramFirst) {
		return errors.New("program/flow/staticcheck: Function generic range is unavailable")
	}
	if generic && points.tree.paramFirst[functionOrdinal] == 0 {
		return errors.New("program/flow/staticcheck: Function generic observation has no type parameter")
	}
	return nil
}

func annotationTarget(view staticquery.View, target keyspace.Term) bool {
	family := keyspace.TermFamily(target)
	ordinal := keyspace.TermOrdinal(target)
	if ordinal == 0 {
		return false
	}
	if family == keyspace.FamilyTypeField {
		return ordinal <= uint32(view.Types().Fields().Count())
	}
	if !annotationTypeFamily(family) {
		return false
	}
	return ordinal <= staticFamilyCount(view, family)
}

func annotationTypeFamily(family keyspace.Family) bool {
	switch family {
	case keyspace.FamilyTypePrimitive, keyspace.FamilyTypeLiteral,
		keyspace.FamilyTypeOptional, keyspace.FamilyTypeUnion,
		keyspace.FamilyTypeIntersection, keyspace.FamilyTypeRef,
		keyspace.FamilyTypeGeneric, keyspace.FamilyTypeArray,
		keyspace.FamilyTypeMap, keyspace.FamilyTypeRecord,
		keyspace.FamilyTypeFunction, keyspace.FamilyTypeAsserts,
		keyspace.FamilyTypeOf, keyspace.FamilyTypeKeyOf,
		keyspace.FamilyTypeIndexAccess, keyspace.FamilyTypeConditional:
		return true
	default:
		return false
	}
}

func staticFamilyCount(view staticquery.View, family keyspace.Family) uint32 {
	var count int
	switch family {
	case keyspace.FamilyTypeAlias:
		count = view.Declarations().Aliases().Count()
	case keyspace.FamilyTypeInterface:
		count = view.Declarations().Interfaces().Count()
	case keyspace.FamilyTypeParam:
		count = view.Declarations().TypeParams().Count()
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
	}
	if count < 0 {
		return 0
	}
	return uint32(count)
}
