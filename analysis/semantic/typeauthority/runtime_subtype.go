package typeauthority

import "github.com/wippyai/go-lua/analysis/type/kind"

// runtimeProofRef is one query-local view of a dense Runtime row. self binds
// a free Self occurrence without rewriting a row; present removes nil from a
// readonly table entry without materializing a projection type.
type runtimeProofRef struct {
	index      uint32
	self       uint32
	selfActive bool
	present    bool
}

type runtimeProofRelation uint8

const (
	runtimeProofSubtype runtimeProofRelation = iota
	runtimeProofWiden
	runtimeProofEqual
)

// runtimeProofPath is the exact active obligation path. It is query-local and
// never retained by Runtime. Re-entering the same relation/ref pair closes the
// coinductive obligation; there is no fuel or depth result.
type runtimeProofPath struct {
	relation    runtimeProofRelation
	left, right runtimeProofRef
	parent      *runtimeProofPath
}

func (r *Runtime) runtimeRowSubtype(left, right uint32) (bool, bool) {
	return r.runtimeSubtypeAt(runtimeProofRef{index: left}, runtimeProofRef{index: right}, nil)
}

func (r *Runtime) runtimeSubtypeAt(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	var ok bool
	if left, ok = r.runtimeNormalizeRef(left); !ok {
		return false, false
	}
	if right, ok = r.runtimeNormalizeRef(right); !ok || right.present {
		return false, false
	}
	if left == right {
		return true, true
	}
	if runtimeProofPathContains(path, runtimeProofSubtype, left, right) {
		return true, true
	}
	frame := runtimeProofPath{relation: runtimeProofSubtype, left: left, right: right, parent: path}
	path = &frame

	if left.present {
		return r.runtimePresentSubtype(left, right, path)
	}
	leftRow := r.rows[left.index-1]
	rightRow := r.rows[right.index-1]

	// Recursive and instantiated equations precede top/bottom and outer-form
	// dispatch, matching the canonical relation.
	if leftRow.form == FormRecursive && rightRow.form == FormRecursive {
		if !leftRow.body.present || !rightRow.body.present {
			return false, false
		}
		return r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.body), r.runtimeChildRef(right, rightRow.body), path)
	}
	if leftRow.form == FormRecursive {
		if !leftRow.body.present {
			return false, false
		}
		return r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.body), right, path)
	}
	if rightRow.form == FormRecursive {
		if !rightRow.body.present {
			return false, false
		}
		return r.runtimeSubtypeAt(left, r.runtimeChildRef(right, rightRow.body), path)
	}
	if leftRow.form == FormInstantiated && rightRow.form == FormInstantiated {
		if !leftRow.base.present || !rightRow.base.present || !r.owns(leftRow.base.inner) || !r.owns(rightRow.base.inner) {
			return false, false
		}
		if r.runtimeSameRow(leftRow.base.inner.index, rightRow.base.inner.index) {
			return r.runtimeInstantiationInvariant(left, right, path)
		}
	}
	if leftRow.form == FormInstantiated {
		if !leftRow.expansion.present {
			return false, false
		}
		expanded := runtimeProofRef{index: leftRow.expansion.inner.index}
		return r.runtimeSubtypeAt(expanded, r.runtimeBindSelf(right, left.index), path)
	}
	if rightRow.form == FormInstantiated {
		if !rightRow.expansion.present {
			return false, false
		}
		expanded := runtimeProofRef{index: rightRow.expansion.inner.index}
		return r.runtimeSubtypeAt(r.runtimeBindSelf(left, right.index), expanded, path)
	}

	if leftRow.form == FormNever {
		return true, true
	}
	if rightRow.form == FormNever {
		return false, true
	}
	if rightRow.form == FormAny || rightRow.form == FormUnknown || r.runtimeOptionalTop(right) {
		return true, true
	}
	if leftRow.form == FormAny {
		if rightRow.form == FormInterface && rightRow.tableTop {
			return true, true
		}
		switch rightRow.form {
		case FormUnion:
			return r.runtimeAnyToVariants(left, right, false, path)
		case FormIntersection:
			return r.runtimeAnyToVariants(left, right, true, path)
		default:
			return false, true
		}
	}
	if leftRow.form == FormUnknown {
		return false, true
	}

	if leftRow.form == FormUnion {
		return r.runtimeVariantsAllLeft(left, right, path)
	}
	if rightRow.form == FormUnion {
		if leftRow.form == FormOptional {
			if !leftRow.inner.present {
				return r.runtimeSubtypeAt(runtimeProofRef{index: r.nilRow}, right, path)
			}
			if answer, decided := r.runtimeSubtypeAt(runtimeProofRef{index: r.nilRow}, right, path); !decided || !answer {
				return answer, decided
			}
			return r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.inner), right, path)
		}
		return r.runtimeVariantsAnyRight(left, right, path)
	}
	if leftRow.form == FormIntersection {
		return r.runtimeVariantsAnyLeft(left, right, path)
	}
	if rightRow.form == FormIntersection {
		return r.runtimeVariantsAllRight(left, right, path)
	}
	if rightRow.form == FormOptional {
		if !rightRow.inner.present {
			return leftRow.form == FormNil, true
		}
		if leftRow.form == FormOptional {
			if !leftRow.inner.present {
				return true, true
			}
			return r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.inner), r.runtimeChildRef(right, rightRow.inner), path)
		}
		if leftRow.form == FormNil {
			return true, true
		}
		return r.runtimeSubtypeAt(left, r.runtimeChildRef(right, rightRow.inner), path)
	}
	if leftRow.form == FormOptional {
		if answer, decided := r.runtimeSubtypeAt(runtimeProofRef{index: r.nilRow}, right, path); !decided || !answer {
			return answer, decided
		}
		if !leftRow.inner.present {
			return true, true
		}
		return r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.inner), right, path)
	}
	if rightRow.tableTop {
		return r.runtimeTableLike(left), true
	}

	if leftRow.form == FormRecord && r.runtimeEmptyRecord(leftRow) {
		switch rightRow.form {
		case FormArray, FormMap, FormReadonlyMap:
			return true, true
		}
	}
	if leftRow.form == FormRecord {
		switch rightRow.form {
		case FormMap:
			return r.runtimeRecordToMap(left, right, false, path)
		case FormReadonlyMap:
			return r.runtimeRecordToMap(left, right, true, path)
		case FormInterface:
			return r.runtimeRecordToInterface(left, right, path)
		}
	}
	if leftRow.form == FormMap {
		switch rightRow.form {
		case FormRecord:
			return r.runtimeMapToRecord(left, right, path)
		case FormReadonlyMap:
			return r.runtimeReadonlyMapping(left, right, path)
		}
	}
	if leftRow.form == FormArray {
		switch rightRow.form {
		case FormMap:
			if answer, decided := r.runtimeSubtypeAt(runtimeProofRef{index: r.integerRow}, r.runtimeChildRef(right, rightRow.key), path); !decided || !answer {
				return answer, decided
			}
			return r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.element), r.runtimeChildRef(right, rightRow.value), path)
		case FormReadonlyMap:
			return r.runtimeReadonlyParts(runtimeProofRef{index: r.integerRow}, r.runtimeChildRef(left, leftRow.element), right, path)
		}
	}
	if leftRow.form == FormTuple {
		switch rightRow.form {
		case FormArray:
			return r.runtimeTupleToElement(left, r.runtimeChildRef(right, rightRow.element), path)
		case FormMap:
			if answer, decided := r.runtimeSubtypeAt(runtimeProofRef{index: r.integerRow}, r.runtimeChildRef(right, rightRow.key), path); !decided || !answer {
				return answer, decided
			}
			return r.runtimeTupleToElement(left, r.runtimeChildRef(right, rightRow.value), path)
		case FormReadonlyMap:
			return r.runtimeTupleToReadonly(left, right, path)
		}
	}

	if leftRow.form == FormTypeParameter {
		if rightRow.form == FormTypeParameter {
			return r.runtimeEqualAt(left, right, path)
		}
		if leftRow.inner.present {
			return r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.inner), right, path)
		}
		return rightRow.form == FormAny, true
	}
	if rightRow.form == FormTypeParameter {
		if rightRow.inner.present {
			return r.runtimeSubtypeAt(left, r.runtimeChildRef(right, rightRow.inner), path)
		}
		return true, true
	}

	if leftRow.form == FormLiteral {
		return r.runtimeLiteralSubtype(leftRow.literal, right, path)
	}
	if rightRow.form == FormLiteral {
		return false, true
	}
	if leftRow.form == FormInteger && rightRow.form == FormNumber {
		return true, true
	}
	if leftRow.form != rightRow.form {
		return false, true
	}

	switch leftRow.form {
	case FormNil, FormBoolean, FormNumber, FormInteger, FormString, FormAny,
		FormUnknown, FormNever, FormSelf:
		return leftRow.form == rightRow.form, true
	case FormFunction:
		return r.runtimeFunctionSubtype(left, right, path)
	case FormRecord:
		return r.runtimeRecordSubtype(left, right, path)
	case FormArray:
		return r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.element), r.runtimeChildRef(right, rightRow.element), path)
	case FormMap:
		return r.runtimeMutableMapping(left, right, path)
	case FormReadonlyMap:
		return r.runtimeReadonlyMapping(left, right, path)
	case FormTuple:
		return r.runtimeTupleSubtype(left, right, path)
	case FormInterface:
		return r.runtimeInterfaceSubtype(left, right, path)
	case FormMeta:
		if !leftRow.inner.present || !rightRow.inner.present {
			return false, false
		}
		return r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.inner), r.runtimeChildRef(right, rightRow.inner), path)
	case FormGeneric:
		return r.runtimeEqualAt(left, right, path)
	default:
		return false, false
	}
}

func (r *Runtime) runtimeNormalizeRef(ref runtimeProofRef) (runtimeProofRef, bool) {
	if r == nil || ref.index == 0 || uint64(ref.index) > uint64(len(r.rows)) || ref.selfActive != (ref.self != 0) {
		return runtimeProofRef{}, false
	}
	row := r.rows[ref.index-1]
	if row.form == FormSelf && ref.selfActive {
		ref.index, ref.self, ref.selfActive = ref.self, 0, false
		if ref.index == 0 || uint64(ref.index) > uint64(len(r.rows)) {
			return runtimeProofRef{}, false
		}
		row = r.rows[ref.index-1]
	}
	// Self substitution does not cross declaration/binding boundaries.
	if row.form == FormInterface {
		ref.self, ref.selfActive = 0, false
	}
	return ref, true
}

func (r *Runtime) runtimeChildRef(parent runtimeProofRef, child runtimeChild) runtimeProofRef {
	if !child.present {
		return runtimeProofRef{}
	}
	return runtimeProofRef{index: child.inner.index, self: parent.self, selfActive: parent.selfActive}
}

func (r *Runtime) runtimeBindSelf(ref runtimeProofRef, self uint32) runtimeProofRef {
	ref.self, ref.selfActive = 0, false
	if self != 0 && ref.index != 0 && uint64(ref.index) <= uint64(len(r.rows)) && r.rows[ref.index-1].selfRewrite {
		ref.self, ref.selfActive = self, true
	}
	return ref
}

func (r *Runtime) runtimeSameRow(left, right uint32) bool {
	return r != nil && left != 0 && left == right && uint64(left) <= uint64(len(r.rows))
}

func runtimeProofPathContains(path *runtimeProofPath, relation runtimeProofRelation, left, right runtimeProofRef) bool {
	for current := path; current != nil; current = current.parent {
		if current.relation == relation && current.left == left && current.right == right {
			return true
		}
	}
	return false
}

func (r *Runtime) runtimeOptionalTop(ref runtimeProofRef) bool {
	row := r.rows[ref.index-1]
	if row.form != FormOptional || !row.inner.present {
		return false
	}
	inner, ok := r.runtimeNormalizeRef(r.runtimeChildRef(ref, row.inner))
	if !ok {
		return false
	}
	form := r.rows[inner.index-1].form
	return form == FormAny || form == FormUnknown
}

func (r *Runtime) runtimeTableLike(ref runtimeProofRef) bool {
	row := r.rows[ref.index-1]
	switch row.form {
	case FormRecord, FormArray, FormMap, FormReadonlyMap, FormTuple, FormInterface:
		return true
	default:
		return false
	}
}

func (r *Runtime) runtimeEmptyRecord(row runtimeRow) bool {
	return row.form == FormRecord && row.fields.start == row.fields.end && row.staticMembers.start == row.staticMembers.end
}

func (r *Runtime) runtimeVariantSlice(ref runtimeProofRef) ([]runtimeChild, bool) {
	row := r.rows[ref.index-1]
	if row.variants.start > row.variants.end || uint64(row.variants.end) > uint64(len(r.variants)) {
		return nil, false
	}
	return r.variants[row.variants.start:row.variants.end], true
}

func (r *Runtime) runtimeVariantsAllLeft(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	children, ok := r.runtimeVariantSlice(left)
	if !ok {
		return false, false
	}
	for _, child := range children {
		if !child.present {
			return false, false
		}
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, child), right, path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeVariantsAnyLeft(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	children, ok := r.runtimeVariantSlice(left)
	if !ok {
		return false, false
	}
	for _, child := range children {
		if !child.present {
			return false, false
		}
		answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, child), right, path)
		if !decided {
			return false, false
		}
		if answer {
			return true, true
		}
	}
	return false, true
}

func (r *Runtime) runtimeVariantsAnyRight(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	children, ok := r.runtimeVariantSlice(right)
	if !ok {
		return false, false
	}
	for _, child := range children {
		if !child.present {
			return false, false
		}
		answer, decided := r.runtimeSubtypeAt(left, r.runtimeChildRef(right, child), path)
		if !decided {
			return false, false
		}
		if answer {
			return true, true
		}
	}
	return false, true
}

func (r *Runtime) runtimeVariantsAllRight(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	children, ok := r.runtimeVariantSlice(right)
	if !ok {
		return false, false
	}
	for _, child := range children {
		if !child.present {
			return false, false
		}
		if answer, decided := r.runtimeSubtypeAt(left, r.runtimeChildRef(right, child), path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeAnyToVariants(left, right runtimeProofRef, all bool, path *runtimeProofPath) (bool, bool) {
	if all {
		return r.runtimeVariantsAllRight(left, right, path)
	}
	return r.runtimeVariantsAnyRight(left, right, path)
}

func (r *Runtime) runtimePresentSubtype(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	row := r.rows[left.index-1]
	switch row.form {
	case FormNil, FormNever:
		return true, true
	case FormOptional:
		if !row.inner.present {
			return true, true
		}
		child := r.runtimeChildRef(left, row.inner)
		child.present = true
		return r.runtimeSubtypeAt(child, right, path)
	case FormUnion:
		children, ok := r.runtimeVariantSlice(left)
		if !ok {
			return false, false
		}
		for _, child := range children {
			if !child.present {
				return false, false
			}
			projected := r.runtimeChildRef(left, child)
			projected.present = true
			if answer, decided := r.runtimeSubtypeAt(projected, right, path); !decided || !answer {
				return answer, decided
			}
		}
		return true, true
	default:
		left.present = false
		return r.runtimeSubtypeAt(left, right, path)
	}
}

func (r *Runtime) runtimeInstantiationInvariant(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow, rightRow := r.rows[left.index-1], r.rows[right.index-1]
	if leftRow.form != FormInstantiated || rightRow.form != FormInstantiated ||
		!leftRow.base.present || !rightRow.base.present ||
		!r.runtimeSameRow(leftRow.base.inner.index, rightRow.base.inner.index) ||
		leftRow.arguments.start > leftRow.arguments.end || rightRow.arguments.start > rightRow.arguments.end ||
		uint64(leftRow.arguments.end) > uint64(len(r.arguments)) || uint64(rightRow.arguments.end) > uint64(len(r.arguments)) {
		return false, false
	}
	leftCount := int(leftRow.arguments.end - leftRow.arguments.start)
	rightCount := int(rightRow.arguments.end - rightRow.arguments.start)
	if leftCount != rightCount {
		return false, true
	}
	for index := 0; index < leftCount; index++ {
		leftArgument := runtimeProofRef{index: r.arguments[int(leftRow.arguments.start)+index].index, self: left.self, selfActive: left.selfActive}
		rightArgument := runtimeProofRef{index: r.arguments[int(rightRow.arguments.start)+index].index, self: right.self, selfActive: right.selfActive}
		if answer, decided := r.runtimeSubtypeAt(leftArgument, rightArgument, path); !decided || !answer {
			return answer, decided
		}
		if answer, decided := r.runtimeSubtypeAt(rightArgument, leftArgument, path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeLiteralSubtype(left runtimeLiteral, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	row := r.rows[right.index-1]
	if row.form == FormLiteral {
		return left.base == row.literal.base && left.bits == row.literal.bits && left.text == row.literal.text, true
	}
	switch left.base {
	case kind.Boolean:
		return row.form == FormBoolean, true
	case kind.Integer:
		return row.form == FormInteger || row.form == FormNumber, true
	case kind.Number:
		return row.form == FormNumber, true
	case kind.String:
		return row.form == FormString, true
	default:
		return false, false
	}
}

func (r *Runtime) runtimeMutableMapping(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow, rightRow := r.rows[left.index-1], r.rows[right.index-1]
	if !leftRow.key.present || !leftRow.value.present || !rightRow.key.present || !rightRow.value.present {
		return false, false
	}
	leftKey, rightKey := r.runtimeChildRef(left, leftRow.key), r.runtimeChildRef(right, rightRow.key)
	leftValue, rightValue := r.runtimeChildRef(left, leftRow.value), r.runtimeChildRef(right, rightRow.value)
	for _, pair := range [][2]runtimeProofRef{{leftKey, rightKey}, {rightKey, leftKey}, {leftValue, rightValue}, {rightValue, leftValue}} {
		if answer, decided := r.runtimeSubtypeAt(pair[0], pair[1], path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeReadonlyParts(leftKey, leftValue, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	rightRow := r.rows[right.index-1]
	if !rightRow.key.present || !rightRow.value.present {
		return false, false
	}
	if answer, decided := r.runtimeSubtypeAt(leftKey, r.runtimeChildRef(right, rightRow.key), path); !decided || !answer {
		return answer, decided
	}
	leftValue.present = true
	return r.runtimeSubtypeAt(leftValue, r.runtimeChildRef(right, rightRow.value), path)
}

func (r *Runtime) runtimeReadonlyMapping(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow := r.rows[left.index-1]
	if !leftRow.key.present || !leftRow.value.present {
		return false, false
	}
	return r.runtimeReadonlyParts(r.runtimeChildRef(left, leftRow.key), r.runtimeChildRef(left, leftRow.value), right, path)
}

func (r *Runtime) runtimeTupleSlice(ref runtimeProofRef) ([]runtimeTupleElement, bool) {
	row := r.rows[ref.index-1]
	if row.elements.start > row.elements.end || uint64(row.elements.end) > uint64(len(r.elements)) {
		return nil, false
	}
	return r.elements[row.elements.start:row.elements.end], true
}

func (r *Runtime) runtimeTupleToElement(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	elements, ok := r.runtimeTupleSlice(left)
	if !ok {
		return false, false
	}
	for _, element := range elements {
		if !element.child.present {
			return false, false
		}
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, element.child), right, path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeTupleToReadonly(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	elements, ok := r.runtimeTupleSlice(left)
	if !ok {
		return false, false
	}
	rightRow := r.rows[right.index-1]
	if !rightRow.key.present || !rightRow.value.present {
		return false, false
	}
	for _, element := range elements {
		if !element.key.present || !element.child.present {
			return false, false
		}
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, element.key), r.runtimeChildRef(right, rightRow.key), path); !decided || !answer {
			return answer, decided
		}
		value := r.runtimeChildRef(left, element.child)
		value.present = true
		if answer, decided := r.runtimeSubtypeAt(value, r.runtimeChildRef(right, rightRow.value), path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeTupleSubtype(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftElements, leftOK := r.runtimeTupleSlice(left)
	rightElements, rightOK := r.runtimeTupleSlice(right)
	if !leftOK || !rightOK {
		return false, false
	}
	if len(leftElements) != len(rightElements) {
		return false, true
	}
	for index := range leftElements {
		if !leftElements[index].child.present || !rightElements[index].child.present {
			return false, false
		}
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, leftElements[index].child), r.runtimeChildRef(right, rightElements[index].child), path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeParameterSlice(ref runtimeProofRef) ([]runtimeParameter, bool) {
	row := r.rows[ref.index-1]
	if row.parameters.start > row.parameters.end || uint64(row.parameters.end) > uint64(len(r.parameters)) {
		return nil, false
	}
	return r.parameters[row.parameters.start:row.parameters.end], true
}

func (r *Runtime) runtimeResultSlice(ref runtimeProofRef) ([]runtimeChild, bool) {
	row := r.rows[ref.index-1]
	if row.results.start > row.results.end || uint64(row.results.end) > uint64(len(r.results)) {
		return nil, false
	}
	return r.results[row.results.start:row.results.end], true
}

func runtimeRequiredParameters(parameters []runtimeParameter) int {
	required := 0
	for index, parameter := range parameters {
		if !parameter.optional {
			required = index + 1
		}
	}
	return required
}

func (r *Runtime) runtimeFunctionSubtype(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftParameters, leftOK := r.runtimeParameterSlice(left)
	rightParameters, rightOK := r.runtimeParameterSlice(right)
	leftResults, leftResultsOK := r.runtimeResultSlice(left)
	rightResults, rightResultsOK := r.runtimeResultSlice(right)
	if !leftOK || !rightOK || !leftResultsOK || !rightResultsOK {
		return false, false
	}
	leftRow, rightRow := r.rows[left.index-1], r.rows[right.index-1]
	leftRequired, rightRequired := runtimeRequiredParameters(leftParameters), runtimeRequiredParameters(rightParameters)
	if leftRequired > rightRequired || (!rightRow.variadic.present && leftRequired > len(rightParameters)) ||
		(!leftRow.variadic.present && len(rightParameters) > len(leftParameters)) {
		return false, true
	}
	count := len(leftParameters)
	if len(rightParameters) > count {
		count = len(rightParameters)
	}
	for index := 0; index < count; index++ {
		var leftType, rightType runtimeProofRef
		var leftPresent, rightPresent, leftReceiver, rightReceiver bool
		if index < len(leftParameters) {
			leftPresent = leftParameters[index].child.present
			leftReceiver = leftParameters[index].receiver
			leftType = r.runtimeChildRef(left, leftParameters[index].child)
		} else if leftRow.variadic.present {
			leftPresent = true
			leftType = r.runtimeChildRef(left, leftRow.variadic)
		}
		if index < len(rightParameters) {
			rightPresent = rightParameters[index].child.present
			rightReceiver = rightParameters[index].receiver
			rightType = r.runtimeChildRef(right, rightParameters[index].child)
		} else if rightRow.variadic.present {
			rightPresent = true
			rightType = r.runtimeChildRef(right, rightRow.variadic)
		}
		if leftReceiver != rightReceiver && (leftPresent || rightPresent) {
			return false, true
		}
		if leftPresent && rightPresent {
			if answer, decided := r.runtimeSubtypeAt(rightType, leftType, path); !decided || !answer {
				return answer, decided
			}
		}
	}
	if leftRow.variadic.present && rightRow.variadic.present {
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(right, rightRow.variadic), r.runtimeChildRef(left, leftRow.variadic), path); !decided || !answer {
			return answer, decided
		}
	}
	for index, expected := range rightResults {
		if !expected.present {
			return false, false
		}
		actual := runtimeProofRef{index: r.nilRow}
		if index < len(leftResults) {
			if !leftResults[index].present {
				return false, false
			}
			actual = r.runtimeChildRef(left, leftResults[index])
		}
		if answer, decided := r.runtimeSubtypeAt(actual, r.runtimeChildRef(right, expected), path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

type runtimeRecordMember struct {
	key       runtimeChild
	child     runtimeChild
	effective runtimeChild
	optional  bool
	readonly  bool
}

func (r *Runtime) runtimeFieldSlice(ref runtimeProofRef) ([]runtimeNamedChild, bool) {
	row := r.rows[ref.index-1]
	if row.fields.start > row.fields.end || uint64(row.fields.end) > uint64(len(r.fields)) {
		return nil, false
	}
	return r.fields[row.fields.start:row.fields.end], true
}

func (r *Runtime) runtimeStaticSlice(ref runtimeProofRef) ([]runtimeStaticChild, bool) {
	row := r.rows[ref.index-1]
	if row.staticMembers.start > row.staticMembers.end || uint64(row.staticMembers.end) > uint64(len(r.staticMembers)) {
		return nil, false
	}
	return r.staticMembers[row.staticMembers.start:row.staticMembers.end], true
}

func runtimeNamedMember(value runtimeNamedChild) runtimeRecordMember {
	return runtimeRecordMember{key: value.key, child: value.child, effective: value.effective, optional: value.optional, readonly: value.readonly}
}

func runtimeStaticMember(value runtimeStaticChild) runtimeRecordMember {
	return runtimeRecordMember{key: value.key, child: value.child, effective: value.effective, optional: value.optional, readonly: value.readonly}
}

func (r *Runtime) runtimeReadableField(ref runtimeProofRef, name string) (runtimeRecordMember, bool, bool) {
	fields, ok := r.runtimeFieldSlice(ref)
	if !ok {
		return runtimeRecordMember{}, false, false
	}
	low, high := 0, len(fields)
	for low < high {
		middle := low + (high-low)/2
		if fields[middle].name < name {
			low = middle + 1
		} else {
			high = middle
		}
	}
	if low < len(fields) && fields[low].name == name {
		return runtimeNamedMember(fields[low]), true, true
	}
	static, valid := r.runtimeStaticSlice(ref)
	if !valid {
		return runtimeRecordMember{}, false, false
	}
	for _, member := range static {
		if member.stringKey && member.name == name {
			return runtimeStaticMember(member), true, true
		}
	}
	return runtimeRecordMember{}, false, true
}

func (r *Runtime) runtimeReadableStatic(ref runtimeProofRef, expected runtimeStaticChild) (runtimeRecordMember, bool, bool) {
	static, ok := r.runtimeStaticSlice(ref)
	if !ok {
		return runtimeRecordMember{}, false, false
	}
	for _, member := range static {
		if member.kind == expected.kind && member.name == expected.name && member.integer == expected.integer {
			return runtimeStaticMember(member), true, true
		}
	}
	if expected.stringKey {
		return r.runtimeReadableField(ref, expected.name)
	}
	return runtimeRecordMember{}, false, true
}

func (r *Runtime) runtimeRecordSubtype(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	rightFields, rightFieldsOK := r.runtimeFieldSlice(right)
	rightStatic, rightStaticOK := r.runtimeStaticSlice(right)
	if !rightFieldsOK || !rightStaticOK {
		return false, false
	}
	for _, expected := range rightFields {
		actual, found, valid := r.runtimeReadableField(left, expected.name)
		if !valid {
			return false, false
		}
		if !found {
			if expected.optional || r.runtimeOptionalLike(r.runtimeChildRef(right, expected.child), nil) {
				continue
			}
			return false, true
		}
		if answer, decided := r.runtimeRecordMemberSubtype(left, actual, right, runtimeNamedMember(expected), path); !decided || !answer {
			return answer, decided
		}
	}
	for _, expected := range rightStatic {
		actual, found, valid := r.runtimeReadableStatic(left, expected)
		if !valid {
			return false, false
		}
		if !found {
			if expected.optional || r.runtimeOptionalLike(r.runtimeChildRef(right, expected.child), nil) {
				continue
			}
			return false, true
		}
		if answer, decided := r.runtimeRecordMemberSubtype(left, actual, right, runtimeStaticMember(expected), path); !decided || !answer {
			return answer, decided
		}
	}
	leftRow, rightRow := r.rows[left.index-1], r.rows[right.index-1]
	if rightRow.key.present || rightRow.value.present {
		if !rightRow.key.present || !rightRow.value.present || !leftRow.key.present || !leftRow.value.present {
			return false, rightRow.key.present && rightRow.value.present
		}
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.key), r.runtimeChildRef(right, rightRow.key), path); !decided || !answer {
			return answer, decided
		}
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.value), r.runtimeChildRef(right, rightRow.value), path); !decided || !answer {
			return answer, decided
		}
	}
	return r.runtimeMetatableSubtype(left, right, path)
}

func (r *Runtime) runtimeRecordMemberSubtype(leftOwner runtimeProofRef, actual runtimeRecordMember, rightOwner runtimeProofRef, expected runtimeRecordMember, path *runtimeProofPath) (bool, bool) {
	if !actual.child.present || !expected.child.present || !expected.effective.present {
		return false, false
	}
	actualType := r.runtimeChildRef(leftOwner, actual.child)
	expectedType := r.runtimeChildRef(rightOwner, expected.effective)
	if expected.optional && r.rows[actualType.index-1].form == FormNil {
		return true, true
	}
	if answer, decided := r.runtimeSubtypeAt(actualType, expectedType, path); !decided || !answer {
		return answer, decided
	}
	if !expected.readonly {
		if actual.readonly {
			return false, true
		}
		answer, decided := r.runtimeSubtypeAt(expectedType, actualType, path)
		if !decided {
			return false, false
		}
		if !answer {
			answer, decided = r.runtimeWidenAt(actualType, expectedType, path)
			if !decided || !answer {
				return answer, decided
			}
		}
	}
	if !expected.optional && !r.runtimeOptionalLike(r.runtimeChildRef(rightOwner, expected.child), nil) && actual.optional {
		return false, true
	}
	return true, true
}

type runtimeIndexPath struct {
	index  uint32
	parent *runtimeIndexPath
}

func (r *Runtime) runtimeOptionalLike(ref runtimeProofRef, path *runtimeIndexPath) bool {
	var ok bool
	ref, ok = r.runtimeNormalizeRef(ref)
	if !ok {
		return false
	}
	for current := path; current != nil; current = current.parent {
		if current.index == ref.index {
			return false
		}
	}
	frame := runtimeIndexPath{index: ref.index, parent: path}
	row := r.rows[ref.index-1]
	switch row.form {
	case FormNil, FormAny, FormUnknown, FormOptional:
		return true
	case FormUnion:
		children, valid := r.runtimeVariantSlice(ref)
		if !valid {
			return false
		}
		for _, child := range children {
			if child.present && r.runtimeOptionalLike(r.runtimeChildRef(ref, child), &frame) {
				return true
			}
		}
	}
	return false
}

func (r *Runtime) runtimeMetatableSubtype(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow, rightRow := r.rows[left.index-1], r.rows[right.index-1]
	if !leftRow.metatable.present && !rightRow.metatable.present {
		return true, true
	}
	if leftRow.metatableAny && (!rightRow.metatable.present || rightRow.metatableAny) {
		return true, true
	}
	if rightRow.metatableAny {
		return true, true
	}
	if rightRow.metatable.present && r.rows[rightRow.metatable.inner.index-1].form == FormUnknown {
		return true, true
	}
	if leftRow.metatable.present && r.rows[leftRow.metatable.inner.index-1].form == FormUnknown {
		return false, true
	}
	if leftRow.metatableAny || !leftRow.metatable.present || !rightRow.metatable.present {
		return false, true
	}
	return r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.metatable), r.runtimeChildRef(right, rightRow.metatable), path)
}

func (r *Runtime) runtimeRecordToInterface(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	rightRow := r.rows[right.index-1]
	if rightRow.methods.start > rightRow.methods.end || uint64(rightRow.methods.end) > uint64(len(r.methods)) {
		return false, false
	}
	for _, method := range r.methods[rightRow.methods.start:rightRow.methods.end] {
		field, found, valid := r.runtimeReadableField(left, method.name)
		if !valid {
			return false, false
		}
		if !found || !field.child.present || !method.child.present {
			return false, true
		}
		expected := r.runtimeBindSelf(r.runtimeChildRef(right, method.child), left.index)
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, field.child), expected, path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeRecordToMap(left, right runtimeProofRef, readonly bool, path *runtimeProofPath) (bool, bool) {
	fields, fieldsOK := r.runtimeFieldSlice(left)
	static, staticOK := r.runtimeStaticSlice(left)
	rightRow := r.rows[right.index-1]
	if !fieldsOK || !staticOK || !rightRow.key.present || !rightRow.value.present {
		return false, false
	}
	rightKey, rightValue := r.runtimeChildRef(right, rightRow.key), r.runtimeChildRef(right, rightRow.value)
	for _, field := range fields {
		if !field.key.present || !field.child.present {
			return false, false
		}
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, field.key), rightKey, path); !decided || !answer {
			return answer, decided
		}
		value := r.runtimeChildRef(left, field.child)
		if readonly {
			value.present = true
		}
		if answer, decided := r.runtimeSubtypeAt(value, rightValue, path); !decided || !answer {
			return answer, decided
		}
	}
	if readonly {
		for _, member := range static {
			if !member.key.present || !member.child.present {
				return false, false
			}
			if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, member.key), rightKey, path); !decided || !answer {
				return answer, decided
			}
			value := r.runtimeChildRef(left, member.child)
			value.present = true
			if answer, decided := r.runtimeSubtypeAt(value, rightValue, path); !decided || !answer {
				return answer, decided
			}
		}
		leftRow := r.rows[left.index-1]
		if leftRow.open || leftRow.metatable.present {
			if answer, decided := r.runtimeSubtypeAt(runtimeProofRef{index: r.stringRow}, rightKey, path); !decided || !answer {
				return answer, decided
			}
			if answer, decided := r.runtimeSubtypeAt(runtimeProofRef{index: r.unknownRow}, rightValue, path); !decided || !answer {
				return answer, decided
			}
		}
	}
	leftRow := r.rows[left.index-1]
	if leftRow.key.present || leftRow.value.present {
		if !leftRow.key.present || !leftRow.value.present {
			return false, false
		}
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.key), rightKey, path); !decided || !answer {
			return answer, decided
		}
		value := r.runtimeChildRef(left, leftRow.value)
		if readonly {
			value.present = true
		}
		if answer, decided := r.runtimeSubtypeAt(value, rightValue, path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeMapToRecord(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow, rightRow := r.rows[left.index-1], r.rows[right.index-1]
	if !leftRow.key.present || !leftRow.value.present || !rightRow.key.present || !rightRow.value.present {
		return false, true
	}
	if answer, decided := r.runtimeMutableMapping(left, right, path); !decided || !answer {
		return answer, decided
	}
	fields, ok := r.runtimeFieldSlice(right)
	if !ok {
		return false, false
	}
	leftKey, leftValue := r.runtimeChildRef(left, leftRow.key), r.runtimeChildRef(left, leftRow.value)
	for _, field := range fields {
		if !field.optional && !r.runtimeOptionalLike(r.runtimeChildRef(right, field.child), nil) {
			return false, true
		}
		if !field.key.present || !field.effective.present {
			return false, false
		}
		included, decided := r.runtimeSubtypeAt(r.runtimeChildRef(right, field.key), leftKey, path)
		if !decided {
			return false, false
		}
		if !included {
			continue
		}
		expected := r.runtimeChildRef(right, field.effective)
		if answer, decided := r.runtimeSubtypeAt(leftValue, expected, path); !decided || !answer {
			return answer, decided
		}
		answer, decided := r.runtimeSubtypeAt(expected, leftValue, path)
		if !decided {
			return false, false
		}
		if !answer {
			answer, decided = r.runtimeWidenAt(leftValue, expected, path)
			if !decided || !answer {
				return answer, decided
			}
		}
	}
	return true, true
}

func (r *Runtime) runtimeInterfaceSubtype(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow, rightRow := r.rows[left.index-1], r.rows[right.index-1]
	if leftRow.methods.start > leftRow.methods.end || rightRow.methods.start > rightRow.methods.end ||
		uint64(leftRow.methods.end) > uint64(len(r.methods)) || uint64(rightRow.methods.end) > uint64(len(r.methods)) {
		return false, false
	}
	leftMethods := r.methods[leftRow.methods.start:leftRow.methods.end]
	for _, expected := range r.methods[rightRow.methods.start:rightRow.methods.end] {
		found := false
		for _, actual := range leftMethods {
			if actual.name != expected.name {
				continue
			}
			found = true
			if !actual.child.present || !expected.child.present {
				return false, false
			}
			if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, actual.child), r.runtimeChildRef(right, expected.child), path); !decided || !answer {
				return answer, decided
			}
			break
		}
		if !found {
			return false, true
		}
	}
	return true, true
}

func (r *Runtime) runtimeWidenEither(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	answer, decided := r.runtimeSubtypeAt(left, right, path)
	if !decided || answer {
		return answer, decided
	}
	return r.runtimeWidenAt(left, right, path)
}

func (r *Runtime) runtimeWidenAt(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	var ok bool
	if left, ok = r.runtimeNormalizeRef(left); !ok {
		return false, false
	}
	if right, ok = r.runtimeNormalizeRef(right); !ok || right.present {
		return false, false
	}
	if left == right {
		return true, true
	}
	if runtimeProofPathContains(path, runtimeProofWiden, left, right) {
		return true, true
	}
	frame := runtimeProofPath{relation: runtimeProofWiden, left: left, right: right, parent: path}
	path = &frame
	if left.present {
		return r.runtimePresentWiden(left, right, path)
	}
	leftRow, rightRow := r.rows[left.index-1], r.rows[right.index-1]
	if leftRow.form == FormInstantiated {
		if !leftRow.expansion.present {
			return false, false
		}
		return r.runtimeWidenEither(runtimeProofRef{index: leftRow.expansion.inner.index}, r.runtimeBindSelf(right, left.index), path)
	}
	if rightRow.form == FormInstantiated {
		if !rightRow.expansion.present {
			return false, false
		}
		return r.runtimeWidenEither(r.runtimeBindSelf(left, right.index), runtimeProofRef{index: rightRow.expansion.inner.index}, path)
	}
	if leftRow.form == FormRecursive {
		if !leftRow.body.present {
			return false, false
		}
		if rightRow.form == FormRecursive {
			if !rightRow.body.present {
				return false, false
			}
			return r.runtimeWidenEither(r.runtimeChildRef(left, leftRow.body), r.runtimeChildRef(right, rightRow.body), path)
		}
		return r.runtimeWidenEither(r.runtimeChildRef(left, leftRow.body), right, path)
	}
	if rightRow.form == FormRecursive {
		if !rightRow.body.present {
			return false, false
		}
		return r.runtimeWidenEither(left, r.runtimeChildRef(right, rightRow.body), path)
	}
	if rightRow.form == FormAny {
		return true, true
	}
	if rightRow.tableTop {
		return r.runtimeTableLike(left), true
	}
	if rightRow.form == FormOptional {
		if !rightRow.inner.present {
			return leftRow.form == FormNil, true
		}
		if leftRow.form == FormOptional {
			if !leftRow.inner.present {
				return true, true
			}
			return r.runtimeWidenEither(r.runtimeChildRef(left, leftRow.inner), r.runtimeChildRef(right, rightRow.inner), path)
		}
		return r.runtimeWidenEither(left, r.runtimeChildRef(right, rightRow.inner), path)
	}
	if leftRow.form == FormUnion {
		children, valid := r.runtimeVariantSlice(left)
		if !valid || len(children) == 0 {
			return false, valid
		}
		for _, child := range children {
			if !child.present {
				return false, false
			}
			if answer, decided := r.runtimeWidenEither(r.runtimeChildRef(left, child), right, path); !decided || !answer {
				return answer, decided
			}
		}
		return true, true
	}
	if rightRow.form == FormUnion {
		children, valid := r.runtimeVariantSlice(right)
		if !valid {
			return false, false
		}
		for _, child := range children {
			if !child.present {
				return false, false
			}
			candidate := r.runtimeChildRef(right, child)
			if r.rows[candidate.index-1].form == FormLiteral {
				continue
			}
			answer, decided := r.runtimeWidenEither(left, candidate, path)
			if !decided {
				return false, false
			}
			if answer {
				return true, true
			}
		}
		return false, true
	}
	if leftRow.form == FormInteger && rightRow.form == FormNumber {
		return true, true
	}
	if leftRow.form == FormLiteral {
		return r.runtimeLiteralSubtype(leftRow.literal, right, path)
	}
	if leftRow.form == FormRecord {
		if r.runtimeEmptyRecord(leftRow) {
			switch rightRow.form {
			case FormRecord, FormArray, FormMap, FormReadonlyMap:
				return true, true
			}
		}
		switch rightRow.form {
		case FormRecord:
			return r.runtimeWidenRecord(left, right, path)
		case FormArray:
			return r.runtimeWidenRecordToArray(left, right, path)
		case FormMap:
			return r.runtimeWidenRecordToMap(left, right, path)
		}
	}
	if leftRow.form == FormMap && rightRow.form == FormMap {
		return r.runtimeWidenMap(left, right, path)
	}
	if leftRow.form == FormArray {
		if rightRow.form == FormArray {
			return r.runtimeWidenEither(r.runtimeChildRef(left, leftRow.element), r.runtimeChildRef(right, rightRow.element), path)
		}
		if rightRow.form == FormMap {
			if answer, decided := r.runtimeSubtypeAt(runtimeProofRef{index: r.integerRow}, r.runtimeChildRef(right, rightRow.key), path); !decided || !answer {
				return answer, decided
			}
			return r.runtimeWidenEither(r.runtimeChildRef(left, leftRow.element), r.runtimeChildRef(right, rightRow.value), path)
		}
	}
	if leftRow.form == FormTuple && rightRow.form == FormTuple {
		leftElements, leftOK := r.runtimeTupleSlice(left)
		rightElements, rightOK := r.runtimeTupleSlice(right)
		if !leftOK || !rightOK {
			return false, false
		}
		if len(leftElements) != len(rightElements) {
			return false, true
		}
		for index := range leftElements {
			if answer, decided := r.runtimeWidenEither(r.runtimeChildRef(left, leftElements[index].child), r.runtimeChildRef(right, rightElements[index].child), path); !decided || !answer {
				return answer, decided
			}
		}
		return true, true
	}
	if leftRow.form == FormFunction && rightRow.form == FormFunction {
		return r.runtimeWidenFunction(left, right, path)
	}
	return false, true
}

func (r *Runtime) runtimePresentWiden(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	row := r.rows[left.index-1]
	switch row.form {
	case FormNil, FormNever:
		return true, true
	case FormOptional:
		if !row.inner.present {
			return true, true
		}
		child := r.runtimeChildRef(left, row.inner)
		child.present = true
		return r.runtimeWidenEither(child, right, path)
	case FormUnion:
		children, valid := r.runtimeVariantSlice(left)
		if !valid {
			return false, false
		}
		for _, child := range children {
			projected := r.runtimeChildRef(left, child)
			projected.present = true
			if answer, decided := r.runtimeWidenEither(projected, right, path); !decided || !answer {
				return answer, decided
			}
		}
		return true, true
	default:
		left.present = false
		return r.runtimeWidenEither(left, right, path)
	}
}

func (r *Runtime) runtimeWidenMap(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow, rightRow := r.rows[left.index-1], r.rows[right.index-1]
	leftKey, rightKey := r.runtimeChildRef(left, leftRow.key), r.runtimeChildRef(right, rightRow.key)
	leftValue, rightValue := r.runtimeChildRef(left, leftRow.value), r.runtimeChildRef(right, rightRow.value)
	if answer, decided := r.runtimeSubtypeAt(leftKey, rightKey, path); !decided || !answer {
		return answer, decided
	}
	if answer, decided := r.runtimeSubtypeAt(rightKey, leftKey, path); !decided || !answer {
		return answer, decided
	}
	if r.rows[leftValue.index-1].form == FormNever {
		return r.runtimeWidenEither(leftValue, rightValue, path)
	}
	if answer, decided := r.runtimeSubtypeAt(leftValue, rightValue, path); !decided || !answer {
		return answer, decided
	}
	return r.runtimeSubtypeAt(rightValue, leftValue, path)
}

func (r *Runtime) runtimeWidenRecordToMap(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	fields, ok := r.runtimeFieldSlice(left)
	if !ok {
		return false, false
	}
	rightRow := r.rows[right.index-1]
	rightKey, rightValue := r.runtimeChildRef(right, rightRow.key), r.runtimeChildRef(right, rightRow.value)
	for _, field := range fields {
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, field.key), rightKey, path); !decided || !answer {
			return answer, decided
		}
		if answer, decided := r.runtimeWidenEither(r.runtimeChildRef(left, field.child), rightValue, path); !decided || !answer {
			return answer, decided
		}
	}
	leftRow := r.rows[left.index-1]
	if leftRow.key.present || leftRow.value.present {
		if !leftRow.key.present || !leftRow.value.present {
			return false, false
		}
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.key), rightKey, path); !decided || !answer {
			return answer, decided
		}
		return r.runtimeWidenEither(r.runtimeChildRef(left, leftRow.value), rightValue, path)
	}
	return true, true
}

func (r *Runtime) runtimeWidenRecordToArray(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	fields, fieldsOK := r.runtimeFieldSlice(left)
	static, staticOK := r.runtimeStaticSlice(left)
	if !fieldsOK || !staticOK {
		return false, false
	}
	if len(fields) != 0 {
		return false, true
	}
	rightElement := r.runtimeChildRef(right, r.rows[right.index-1].element)
	for _, member := range static {
		if member.stringKey {
			return false, true
		}
		if answer, decided := r.runtimeWidenEither(r.runtimeChildRef(left, member.child), rightElement, path); !decided || !answer {
			return answer, decided
		}
	}
	leftRow := r.rows[left.index-1]
	if leftRow.key.present || leftRow.value.present {
		if !leftRow.key.present || !leftRow.value.present {
			return false, false
		}
		if answer, decided := r.runtimeSubtypeAt(r.runtimeChildRef(left, leftRow.key), runtimeProofRef{index: r.integerRow}, path); !decided || !answer {
			return answer, decided
		}
		return r.runtimeWidenEither(r.runtimeChildRef(left, leftRow.value), rightElement, path)
	}
	return true, true
}

func (r *Runtime) runtimeWidenRecord(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	rightFields, fieldsOK := r.runtimeFieldSlice(right)
	rightStatic, staticOK := r.runtimeStaticSlice(right)
	if !fieldsOK || !staticOK {
		return false, false
	}
	for _, wanted := range rightFields {
		actual, found, valid := r.runtimeReadableField(left, wanted.name)
		if !valid {
			return false, false
		}
		if !found {
			if wanted.optional || r.runtimeOptionalLike(r.runtimeChildRef(right, wanted.child), nil) {
				continue
			}
			return false, true
		}
		if answer, decided := r.runtimeWidenEither(r.runtimeChildRef(left, actual.child), r.runtimeChildRef(right, wanted.child), path); !decided || !answer {
			return answer, decided
		}
	}
	leftStatic, leftOK := r.runtimeStaticSlice(left)
	if !leftOK {
		return false, false
	}
	for _, wanted := range rightStatic {
		var actual runtimeStaticChild
		found := false
		for _, candidate := range leftStatic {
			if candidate.kind == wanted.kind && candidate.name == wanted.name && candidate.integer == wanted.integer {
				actual, found = candidate, true
				break
			}
		}
		if !found {
			if wanted.optional || r.runtimeOptionalLike(r.runtimeChildRef(right, wanted.child), nil) {
				continue
			}
			return false, true
		}
		if answer, decided := r.runtimeWidenEither(r.runtimeChildRef(left, actual.child), r.runtimeChildRef(right, wanted.child), path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeWidenFunction(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftParameters, leftOK := r.runtimeParameterSlice(left)
	rightParameters, rightOK := r.runtimeParameterSlice(right)
	if !leftOK || !rightOK {
		return false, false
	}
	if len(leftParameters) != len(rightParameters) {
		return false, true
	}
	for index := range leftParameters {
		leftParam, rightParam := leftParameters[index], rightParameters[index]
		if leftParam.optional != rightParam.optional || leftParam.receiver != rightParam.receiver || !leftParam.child.present || !rightParam.child.present {
			return false, true
		}
		leftType, rightType := r.runtimeChildRef(left, leftParam.child), r.runtimeChildRef(right, rightParam.child)
		if answer, decided := r.runtimeSubtypeAt(leftType, rightType, path); !decided || !answer {
			return answer, decided
		}
		if answer, decided := r.runtimeSubtypeAt(rightType, leftType, path); !decided || !answer {
			return answer, decided
		}
	}
	leftRow, rightRow := r.rows[left.index-1], r.rows[right.index-1]
	if leftRow.variadic.present != rightRow.variadic.present {
		return false, true
	}
	if leftRow.variadic.present {
		leftType, rightType := r.runtimeChildRef(left, leftRow.variadic), r.runtimeChildRef(right, rightRow.variadic)
		if answer, decided := r.runtimeSubtypeAt(leftType, rightType, path); !decided || !answer {
			return answer, decided
		}
		if answer, decided := r.runtimeSubtypeAt(rightType, leftType, path); !decided || !answer {
			return answer, decided
		}
	}
	leftResults, leftResultsOK := r.runtimeResultSlice(left)
	rightResults, rightResultsOK := r.runtimeResultSlice(right)
	if !leftResultsOK || !rightResultsOK {
		return false, false
	}
	for index, expected := range rightResults {
		actual := runtimeProofRef{index: r.nilRow}
		if index < len(leftResults) {
			actual = r.runtimeChildRef(left, leftResults[index])
		}
		if answer, decided := r.runtimeWidenEither(actual, r.runtimeChildRef(right, expected), path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}
