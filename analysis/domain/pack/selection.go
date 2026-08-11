package pack

import (
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/static"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
)

type scalarSelectionKind uint8

const (
	scalarSelectionInvalid scalarSelectionKind = iota
	scalarSelectionTableIndex
)

type TableIndex struct {
	offset Offset
	value  uint64 // cached cold-offset value; never an interning key
	sealed bool
}

func (index TableIndex) Value() (uint64, bool) {
	if !index.valid() {
		return 0, false
	}
	return index.value, true
}

func (index TableIndex) valid() bool {
	if !index.sealed || !index.offset.valid() {
		return false
	}
	// value is a hot selection index, but Offset remains its sole authority.
	// Recheck their cold correspondence so even an in-package malformed
	// selector cannot pair one sealed owner handle with another index.
	value, ok := offsetUint64(index.offset)
	return ok && value == index.value
}

// OwnsTableIndex is the Pack schema's exact TableIndex fence. Numeric Value
// is only a derived operation parameter; it is not sufficient to admit a
// selector issued by another sealed algebra with the same offset number.
func (schema *Schema) OwnsTableIndex(index TableIndex) bool {
	return schema != nil && schema.state != nil && index.valid() && index.offset.owner == schema.state.owner
}

type ScalarSelection struct {
	schema     *schema
	values     Values
	kind       scalarSelectionKind
	tableIndex TableIndex
	sealed     bool
}

func (selection ScalarSelection) valid() bool {
	return selection.sealed && selection.schema != nil && selection.values.valid() && selection.values.schema == selection.schema && selection.kind == scalarSelectionTableIndex && selection.tableIndex.valid() && selection.tableIndex.offset.owner == selection.schema.owner
}
func (schema *Schema) TableIndex(value int64) (TableIndex, bool) {
	if schema == nil || schema.state == nil || value < 0 {
		return TableIndex{}, false
	}
	offset, ok := offsetForUint64(schema.state.owner, uint64(value))
	if !ok {
		return TableIndex{}, false
	}
	return tableIndexForOffset(offset)
}

// tableIndexForOffset issues a scalar selector only from a previously sealed
// Pack offset.  The cached value is derived once from the cold offset table;
// selection never creates or interns offsets while solving.
func tableIndexForOffset(offset Offset) (TableIndex, bool) {
	value, ok := offsetUint64(offset)
	if !ok {
		return TableIndex{}, false
	}
	index := TableIndex{offset: offset, value: value, sealed: true}
	return index, index.valid()
}
func (schema *Schema) TableScalarSelection(values Values, index TableIndex) (ScalarSelection, bool) {
	if schema == nil || schema.state == nil || !values.valid() || values.schema != schema.state || !schema.OwnsTableIndex(index) {
		return ScalarSelection{}, false
	}
	selection := ScalarSelection{schema: schema.state, values: values, kind: scalarSelectionTableIndex, tableIndex: index, sealed: true}
	return selection, selection.valid()
}
func (schema *Schema) FirstScalarSelection(values Values) (ScalarSelection, bool) {
	index, ok := schema.TableIndex(0)
	if !ok {
		return ScalarSelection{}, false
	}
	return schema.TableScalarSelection(values, index)
}

type ScalarObservation struct {
	owner               *algebra
	scalars             []Scalar
	bottom, top, sealed bool
}

func finishScalarObservation(v ScalarObservation) (ScalarObservation, bool) {
	if v.owner == nil || !v.owner.valid() || (v.bottom && v.top) {
		return ScalarObservation{}, false
	}
	if v.bottom || v.top {
		v.sealed = true
		return v, true
	}
	if len(v.scalars) == 0 {
		return ScalarObservation{}, false
	}
	v.sealed = true
	return v, true
}
func (v ScalarObservation) valid() bool    { return v.sealed && v.owner != nil && v.owner.valid() }
func (v ScalarObservation) IsBottom() bool { return v.valid() && v.bottom }
func (v ScalarObservation) IsTop() bool    { return v.valid() && v.top }
func (v ScalarObservation) Count() int {
	if !v.valid() || v.bottom || v.top {
		return 0
	}
	return len(v.scalars)
}
func (v ScalarObservation) At(i int) (Scalar, bool) {
	if i < 0 || i >= v.Count() {
		return Scalar{}, false
	}
	return v.scalars[i], true
}
func (schema *Schema) ObserveScalar(root Root, value Value, values Values, selection ScalarSelection) (ScalarObservation, bool) {
	_, ok := schema.relation(root)
	if !ok || !schema.Admit(root, value) || !values.valid() || values.schema != schema.state || values.schema.values[values.index].root != root.index || !selection.valid() || selection.values != values {
		return ScalarObservation{}, false
	}
	if value.bottom {
		return finishScalarObservation(ScalarObservation{owner: schema.state.owner, bottom: true})
	}
	if value.top {
		return finishScalarObservation(ScalarObservation{owner: schema.state.owner, top: true})
	}
	port := schema.state.values[values.index].port
	scalars := make([]Scalar, 0, len(value.cases))
	for _, c := range value.cases {
		term, ok := casePortTerm(c, port)
		if !ok {
			return ScalarObservation{}, false
		}
		alternatives, ok := projectTermTableIndexAlternatives(term, selection.tableIndex)
		if !ok {
			return ScalarObservation{}, false
		}
		scalars = append(scalars, alternatives...)
	}
	return finishScalarObservation(ScalarObservation{owner: schema.state.owner, scalars: scalars})
}

func casePortTerm(current Case, port Port) (Term, bool) {
	if !current.valid() || !port.valid() || port.owner != current.owner {
		return Term{}, false
	}
	for _, equation := range current.equations {
		if equation.kind == EquationPack && equation.port == port {
			return equation.term, equation.term.valid()
		}
	}
	return Term{}, false
}

func projectTermTableIndex(term Term, index TableIndex) (Scalar, bool) {
	alternatives, ok := projectTermTableIndexAlternatives(term, index)
	if !ok || len(alternatives) != 1 {
		return Scalar{}, false
	}
	return alternatives[0], true
}

// projectTermTableIndexAlternatives preserves the finite suffix branches of
// an open tail.  The scalar projection is a marginal, so callers that need a
// single Scalar must reject ambiguity; Pack-to-Pack transforms can instead
// publish each exact alternative into their output Value without inventing a
// class-only overapproximation.
func projectTermTableIndexAlternatives(term Term, index TableIndex) ([]Scalar, bool) {
	if !term.valid() || !index.valid() || term.owner != index.offset.owner || index.value > uint64(^uint(0)>>1) {
		return nil, false
	}
	at := int(index.value)
	if at < len(term.prefix) {
		return []Scalar{term.prefix[at]}, term.prefix[at].valid()
	}
	switch term.kind {
	case TermClosed:
		scalar, scalarOK := anyScalar(term.owner, term.owner.classes.Nil())
		return []Scalar{scalar}, scalarOK
	case TermOpen:
		delta := at - len(term.prefix)
		var head Scalar
		headOK := false
		if term.rest.kind == RestTail {
			deltaOffset, deltaOK := offsetForUint64(term.owner, uint64(delta))
			offset, offsetOK := addOffsets(term.rest.offset, deltaOffset)
			if !deltaOK || !offsetOK {
				return nil, false
			}
			head, headOK = headScalar(term.rest.tail, offset)
		} else if term.rest.kind == RestAny {
			unknownClass, classOK := term.owner.joinClass(term.rest.class, term.owner.classes.Nil())
			if !classOK {
				return nil, false
			}
			var anyOK bool
			head, anyOK = anyScalar(term.owner, unknownClass)
			headOK = anyOK
		}
		if !headOK {
			return nil, false
		}
		if len(term.suffix) == 0 {
			return []Scalar{head}, true
		}
		if term.rest.kind == RestAny {
			// RestAny is intentionally class-unknown. Its head already joins
			// Nil for the unknown-length case; suffix classes are a finite
			// marginal and therefore join into the one admitted Any scalar.
			classes := make([]static.Class, 0, len(term.suffix)+1)
			headClass, classOK := head.Class()
			if !classOK {
				return nil, false
			}
			classes = append(classes, headClass)
			for suffixIndex := 0; suffixIndex <= delta && suffixIndex < len(term.suffix); suffixIndex++ {
				suffixClass, suffixOK := term.suffix[suffixIndex].Class()
				if !suffixOK {
					return nil, false
				}
				classes = append(classes, suffixClass)
			}
			joined, joinedOK := joinClasses(term.owner, classes)
			if !joinedOK {
				return nil, false
			}
			scalar, scalarOK := anyScalar(term.owner, joined)
			return []Scalar{scalar}, scalarOK
		}
		alternatives := []Scalar{head}
		appendUnique := func(value Scalar) {
			for _, existing := range alternatives {
				if equalScalar(existing, value) {
					return
				}
			}
			alternatives = append(alternatives, value)
		}
		for suffixIndex := 0; suffixIndex < len(term.suffix) && suffixIndex <= delta; suffixIndex++ {
			value := term.suffix[suffixIndex]
			if !value.valid() {
				return nil, false
			}
			appendUnique(value)
		}
		if delta >= len(term.suffix) {
			nilScalar, nilOK := anyScalar(term.owner, term.owner.classes.Nil())
			if !nilOK {
				return nil, false
			}
			appendUnique(nilScalar)
		}
		return alternatives, true
	case TermAny:
		scalar, scalarOK := anyScalar(term.owner, term.owner.classes.AnyValue())
		return []Scalar{scalar}, scalarOK
	default:
		return nil, false
	}
}

func joinClasses(owner *algebra, classes []static.Class) (static.Class, bool) {
	if owner == nil || len(classes) == 0 || !owner.admits(classes[0]) {
		return static.Class{}, false
	}
	joined := classes[0]
	for _, class := range classes[1:] {
		if !owner.admits(class) {
			return static.Class{}, false
		}
		var ok bool
		joined, ok = owner.joinClass(joined, class)
		if !ok {
			return static.Class{}, false
		}
	}
	return joined, true
}

// InputObservation preserves a scalar selection as scalars and Tail/Whole
// selections as complete Pack terms.  The latter must not be prematurely
// converted into independent scalar form because their open-tail correlation
// is still relevant to Pack's exact-endpoint walk.
type InputObservation struct {
	owner   *algebra
	kind    inputSelectionKind
	scalars []Scalar
	terms   []Term
	start   int
	bottom  bool
	top     bool
	sealed  bool
}

func finishInputObservation(value InputObservation) (InputObservation, bool) {
	if value.owner == nil || !value.owner.valid() || (value.bottom && value.top) {
		return InputObservation{}, false
	}
	if value.bottom || value.top {
		value.sealed = true
		return value, true
	}
	switch value.kind {
	case inputSelectionScalar:
		if len(value.scalars) == 0 || len(value.terms) != 0 {
			return InputObservation{}, false
		}
	case inputSelectionTail, inputSelectionWhole:
		if len(value.terms) == 0 || len(value.scalars) != 0 || value.start < 0 {
			return InputObservation{}, false
		}
	default:
		return InputObservation{}, false
	}
	value.sealed = true
	return value, true
}
func (value InputObservation) valid() bool {
	return value.sealed && value.owner != nil && value.owner.valid()
}
func (value InputObservation) IsBottom() bool { return value.valid() && value.bottom }
func (value InputObservation) IsTop() bool    { return value.valid() && value.top }
func (value InputObservation) ScalarCount() int {
	if !value.valid() || value.bottom || value.top || value.kind != inputSelectionScalar {
		return 0
	}
	return len(value.scalars)
}
func (value InputObservation) ScalarAt(index int) (Scalar, bool) {
	if index < 0 || index >= value.ScalarCount() {
		return Scalar{}, false
	}
	return value.scalars[index], true
}
func (value InputObservation) TermCount() int {
	if !value.valid() || value.bottom || value.top || value.kind == inputSelectionScalar {
		return 0
	}
	return len(value.terms)
}
func (value InputObservation) TermAt(index int) (Term, bool) {
	if index < 0 || index >= value.TermCount() {
		return Term{}, false
	}
	return value.terms[index], true
}
func (value InputObservation) Start() (int, bool) {
	if !value.valid() || value.bottom || value.top || value.kind == inputSelectionScalar {
		return 0, false
	}
	return value.start, true
}

// ObserveInput projects one selected Target input from the one Pack fact
// imported for its exact Call root. The root identity is checked here, so a
// Pack fact for a different application in the same body cannot be reused.
func (schema *Schema) ObserveInput(root Root, value Value, selector InputSelector) (InputObservation, bool) {
	if schema == nil || schema.state == nil || !selector.valid() || selector.schema != schema.state || !schema.Admit(root, value) || root.schema != schema.state || schema.state.roots[root.index].kind != rootCall {
		return InputObservation{}, false
	}
	if value.bottom {
		return finishInputObservation(InputObservation{owner: schema.state.owner, kind: selector.kind, bottom: true})
	}
	if value.top {
		return finishInputObservation(InputObservation{owner: schema.state.owner, kind: selector.kind, top: true})
	}
	port := schema.state.roots[root.index].port
	if !port.valid() {
		return InputObservation{}, false
	}
	switch selector.kind {
	case inputSelectionScalar:
		scalars := make([]Scalar, 0, len(value.cases))
		for _, current := range value.cases {
			term, termOK := casePortTerm(current, port)
			scalar, scalarOK := projectTermTableIndex(term, selector.table)
			if !termOK || !scalarOK {
				return InputObservation{}, false
			}
			scalars = append(scalars, scalar)
		}
		return finishInputObservation(InputObservation{owner: schema.state.owner, kind: selector.kind, scalars: scalars})
	case inputSelectionTail, inputSelectionWhole:
		terms := make([]Term, 0, len(value.cases))
		for _, current := range value.cases {
			term, termOK := casePortTerm(current, port)
			if !termOK {
				return InputObservation{}, false
			}
			terms = append(terms, term)
		}
		return finishInputObservation(InputObservation{owner: schema.state.owner, kind: selector.kind, terms: terms, start: selector.start})
	default:
		return InputObservation{}, false
	}
}

// VisitInputSources exposes only exact Boundary values carried by an input
// observation.  complete=false means an unresolved Head/Any/open remainder
// may carry an allocation; callers must conservatively produce Escape Top,
// never silently treat the selection as empty.
func (schema *Schema) VisitInputSources(observation InputObservation, visit func(linkboundary.Value) bool) (complete, ok bool) {
	if schema == nil || schema.state == nil || !observation.valid() || observation.owner != schema.state.owner || visit == nil {
		return false, false
	}
	if observation.bottom {
		return true, true
	}
	if observation.top {
		return false, true
	}
	complete = true
	visitScalar := func(scalar Scalar) bool {
		if source, sourceOK := schema.ScalarSource(scalar); sourceOK {
			return visit(source)
		}
		mayReference, kindsOK := schema.scalarMayReference(scalar)
		if !kindsOK {
			return false
		}
		if mayReference {
			complete = false
		}
		return true
	}
	if observation.kind == inputSelectionScalar {
		for _, scalar := range observation.scalars {
			if !visitScalar(scalar) {
				return false, false
			}
		}
		return complete, true
	}
	for _, term := range observation.terms {
		termComplete, termOK := schema.visitTermSources(term, observation.start, visitScalar)
		if !termOK {
			return false, false
		}
		complete = complete && termComplete
	}
	return complete, true
}

func (schema *Schema) visitTermSources(term Term, start int, visit func(Scalar) bool) (bool, bool) {
	if !term.valid() || term.owner != schema.state.owner || start < 0 || visit == nil {
		return false, false
	}
	for index := start; index < len(term.prefix); index++ {
		if !visit(term.prefix[index]) {
			return false, false
		}
	}
	switch term.kind {
	case TermClosed:
		return true, true
	case TermOpen:
		// The open remainder denotes at least one unknown position in every
		// suffix whose start is not past the known prefix.  Suffix scalars are
		// still exact Pack members and are retained before the caller widens.
		for _, scalar := range term.suffix {
			if !visit(scalar) {
				return false, false
			}
		}
		return false, true
	case TermAny:
		return false, true
	default:
		return false, false
	}
}

func (schema *Schema) scalarMayReference(scalar Scalar) (bool, bool) {
	if schema == nil || schema.state == nil || !scalar.valid() || scalar.owner != schema.state.owner {
		return false, false
	}
	class, classOK := scalar.Class()
	kinds, kindsOK := schema.state.owner.classes.MayRuntimeKinds(class)
	if !classOK || !kindsOK {
		return false, false
	}
	return kinds.Contains(runtimekind.Table) || kinds.Contains(runtimekind.Function) || kinds.Contains(runtimekind.Thread) || kinds.Contains(runtimekind.Userdata), true
}
func (payload Payload) ScalarMayRuntimeKinds(s Scalar) (runtimekind.Set, bool) {
	if payload.schema == nil || !s.valid() {
		return 0, false
	}
	class, ok := s.Class()
	if !ok {
		return 0, false
	}
	return payload.schema.owner.classes.MayRuntimeKinds(class)
}
