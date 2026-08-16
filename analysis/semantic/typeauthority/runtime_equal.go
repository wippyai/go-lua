package typeauthority

// runtimeEqualAt is exact structural equality after applying the query-local
// Self substitution carried by each ref. Unsubstituted rows are already
// structurally interned, so the recursive proof is needed only while at least
// one side carries an active binding. Repeated obligations close
// coinductively, exactly as typ.TypeEquals does for recursive products.
func (r *Runtime) runtimeEqualAt(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	var ok bool
	if left, ok = r.runtimeNormalizeRef(left); !ok {
		return false, false
	}
	if right, ok = r.runtimeNormalizeRef(right); !ok {
		return false, false
	}
	if left.present || right.present {
		return false, false
	}
	if left == right {
		return true, true
	}
	if !left.selfActive && !right.selfActive {
		return false, true
	}
	leftRow, rightRow := &r.rows[left.index-1], &r.rows[right.index-1]
	if leftRow.form != rightRow.form {
		return false, true
	}
	if runtimeProofPathContains(path, runtimeProofEqual, left, right) {
		return true, true
	}
	frame := runtimeProofPath{relation: runtimeProofEqual, left: left, right: right, parent: path}
	path = &frame

	switch leftRow.form {
	case FormNil, FormBoolean, FormNumber, FormInteger, FormString,
		FormAny, FormUnknown, FormNever, FormSelf:
		return true, true
	case FormLiteral:
		return leftRow.literal == rightRow.literal, true
	case FormOptional, FormArray, FormMeta:
		leftChild, rightChild := leftRow.inner, rightRow.inner
		if leftRow.form == FormArray {
			leftChild, rightChild = leftRow.element, rightRow.element
		}
		return r.runtimeEqualChild(left, leftChild, right, rightChild, path)
	case FormMap, FormReadonlyMap:
		if answer, decided := r.runtimeEqualChild(left, leftRow.key, right, rightRow.key, path); !decided || !answer {
			return answer, decided
		}
		return r.runtimeEqualChild(left, leftRow.value, right, rightRow.value, path)
	case FormUnion, FormIntersection:
		leftChildren, leftOK := r.runtimeVariantSlice(left)
		rightChildren, rightOK := r.runtimeVariantSlice(right)
		if !leftOK || !rightOK {
			return false, false
		}
		if len(leftChildren) != len(rightChildren) {
			return false, true
		}
		for index := range leftChildren {
			if answer, decided := r.runtimeEqualChild(left, leftChildren[index], right, rightChildren[index], path); !decided || !answer {
				return answer, decided
			}
		}
		return true, true
	case FormTuple:
		leftElements, leftOK := r.runtimeTupleSlice(left)
		rightElements, rightOK := r.runtimeTupleSlice(right)
		if !leftOK || !rightOK {
			return false, false
		}
		if len(leftElements) != len(rightElements) {
			return false, true
		}
		for index := range leftElements {
			if answer, decided := r.runtimeEqualChild(left, leftElements[index].child, right, rightElements[index].child, path); !decided || !answer {
				return answer, decided
			}
		}
		return true, true
	case FormFunction:
		return r.runtimeFunctionEqual(left, right, path)
	case FormRecord:
		return r.runtimeRecordEqual(left, right, path)
	case FormInterface:
		return r.runtimeInterfaceEqual(left, right, path)
	case FormGeneric:
		return r.runtimeGenericEqual(left, right, path)
	case FormInstantiated:
		return r.runtimeInstantiatedEqual(left, right, path)
	case FormRecursive:
		if leftRow.name != rightRow.name {
			return false, true
		}
		return r.runtimeEqualChild(left, leftRow.body, right, rightRow.body, path)
	case FormTypeParameter:
		if leftRow.name != rightRow.name {
			return false, true
		}
		return r.runtimeEqualChild(left, leftRow.inner, right, rightRow.inner, path)
	default:
		return false, false
	}
}

func (r *Runtime) runtimeEqualChild(leftParent runtimeProofRef, left runtimeChild, rightParent runtimeProofRef, right runtimeChild, path *runtimeProofPath) (bool, bool) {
	if left.present != right.present {
		return false, true
	}
	if !left.present {
		return true, true
	}
	return r.runtimeEqualAt(r.runtimeChildRef(leftParent, left), r.runtimeChildRef(rightParent, right), path)
}

func (r *Runtime) runtimeFunctionEqual(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow, rightRow := &r.rows[left.index-1], &r.rows[right.index-1]
	leftTypeParameters, leftOK := r.runtimeTypeParameterSlice(leftRow)
	rightTypeParameters, rightOK := r.runtimeTypeParameterSlice(rightRow)
	if !leftOK || !rightOK {
		return false, false
	}
	if len(leftTypeParameters) != len(rightTypeParameters) {
		return false, true
	}
	for index := range leftTypeParameters {
		leftParameter, rightParameter := leftTypeParameters[index], rightTypeParameters[index]
		if leftParameter.name != rightParameter.name {
			return false, true
		}
		if answer, decided := r.runtimeEqualChild(left, leftParameter.child, right, rightParameter.child, path); !decided || !answer {
			return answer, decided
		}
	}
	leftParameters, leftOK := r.runtimeParameterSlice(left)
	rightParameters, rightOK := r.runtimeParameterSlice(right)
	if !leftOK || !rightOK {
		return false, false
	}
	if len(leftParameters) != len(rightParameters) {
		return false, true
	}
	for index := range leftParameters {
		leftParameter, rightParameter := leftParameters[index], rightParameters[index]
		if leftParameter.optional != rightParameter.optional || leftParameter.receiver != rightParameter.receiver {
			return false, true
		}
		if answer, decided := r.runtimeEqualChild(left, leftParameter.child, right, rightParameter.child, path); !decided || !answer {
			return answer, decided
		}
	}
	if answer, decided := r.runtimeEqualChild(left, leftRow.variadic, right, rightRow.variadic, path); !decided || !answer {
		return answer, decided
	}
	leftResults, leftOK := r.runtimeResultSlice(left)
	rightResults, rightOK := r.runtimeResultSlice(right)
	if !leftOK || !rightOK {
		return false, false
	}
	if len(leftResults) != len(rightResults) {
		return false, true
	}
	for index := range leftResults {
		if answer, decided := r.runtimeEqualChild(left, leftResults[index], right, rightResults[index], path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeRecordEqual(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow, rightRow := &r.rows[left.index-1], &r.rows[right.index-1]
	if leftRow.open != rightRow.open {
		return false, true
	}
	leftFields, leftOK := r.runtimeFieldSlice(left)
	rightFields, rightOK := r.runtimeFieldSlice(right)
	if !leftOK || !rightOK {
		return false, false
	}
	if len(leftFields) != len(rightFields) {
		return false, true
	}
	for index := range leftFields {
		leftField, rightField := leftFields[index], rightFields[index]
		if leftField.name != rightField.name || leftField.optional != rightField.optional || leftField.readonly != rightField.readonly {
			return false, true
		}
		if answer, decided := r.runtimeEqualChild(left, leftField.child, right, rightField.child, path); !decided || !answer {
			return answer, decided
		}
	}
	leftStatic, leftOK := r.runtimeStaticSlice(left)
	rightStatic, rightOK := r.runtimeStaticSlice(right)
	if !leftOK || !rightOK {
		return false, false
	}
	if len(leftStatic) != len(rightStatic) {
		return false, true
	}
	for index := range leftStatic {
		leftMember, rightMember := leftStatic[index], rightStatic[index]
		if leftMember.kind != rightMember.kind || leftMember.name != rightMember.name || leftMember.integer != rightMember.integer ||
			leftMember.optional != rightMember.optional || leftMember.readonly != rightMember.readonly {
			return false, true
		}
		if answer, decided := r.runtimeEqualChild(left, leftMember.child, right, rightMember.child, path); !decided || !answer {
			return answer, decided
		}
	}
	if answer, decided := r.runtimeEqualChild(left, leftRow.key, right, rightRow.key, path); !decided || !answer {
		return answer, decided
	}
	if answer, decided := r.runtimeEqualChild(left, leftRow.value, right, rightRow.value, path); !decided || !answer {
		return answer, decided
	}
	return r.runtimeEqualChild(left, leftRow.metatable, right, rightRow.metatable, path)
}

func (r *Runtime) runtimeInterfaceEqual(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow, rightRow := &r.rows[left.index-1], &r.rows[right.index-1]
	if leftRow.name != rightRow.name || leftRow.methods.start > leftRow.methods.end || rightRow.methods.start > rightRow.methods.end ||
		uint64(leftRow.methods.end) > uint64(len(r.methods)) || uint64(rightRow.methods.end) > uint64(len(r.methods)) {
		return false, leftRow.name != rightRow.name
	}
	leftMethods := r.methods[leftRow.methods.start:leftRow.methods.end]
	rightMethods := r.methods[rightRow.methods.start:rightRow.methods.end]
	if len(leftMethods) != len(rightMethods) {
		return false, true
	}
	for index := range leftMethods {
		if leftMethods[index].name != rightMethods[index].name {
			return false, true
		}
		if answer, decided := r.runtimeEqualChild(left, leftMethods[index].child, right, rightMethods[index].child, path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeGenericEqual(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow, rightRow := &r.rows[left.index-1], &r.rows[right.index-1]
	if leftRow.name != rightRow.name {
		return false, true
	}
	leftParameters, leftOK := r.runtimeTypeParameterSlice(leftRow)
	rightParameters, rightOK := r.runtimeTypeParameterSlice(rightRow)
	if !leftOK || !rightOK {
		return false, false
	}
	if len(leftParameters) != len(rightParameters) {
		return false, true
	}
	for index := range leftParameters {
		leftParameter, rightParameter := leftParameters[index], rightParameters[index]
		if leftParameter.name != rightParameter.name {
			return false, true
		}
		if answer, decided := r.runtimeEqualChild(left, leftParameter.child, right, rightParameter.child, path); !decided || !answer {
			return answer, decided
		}
	}
	return r.runtimeEqualChild(left, leftRow.body, right, rightRow.body, path)
}

func (r *Runtime) runtimeInstantiatedEqual(left, right runtimeProofRef, path *runtimeProofPath) (bool, bool) {
	leftRow, rightRow := &r.rows[left.index-1], &r.rows[right.index-1]
	if answer, decided := r.runtimeEqualChild(left, leftRow.base, right, rightRow.base, path); !decided || !answer {
		return answer, decided
	}
	if leftRow.arguments.start > leftRow.arguments.end || rightRow.arguments.start > rightRow.arguments.end ||
		uint64(leftRow.arguments.end) > uint64(len(r.arguments)) || uint64(rightRow.arguments.end) > uint64(len(r.arguments)) {
		return false, false
	}
	leftCount := leftRow.arguments.end - leftRow.arguments.start
	rightCount := rightRow.arguments.end - rightRow.arguments.start
	if leftCount != rightCount {
		return false, true
	}
	for offset := uint32(0); offset < leftCount; offset++ {
		leftArgument := runtimeChild{inner: r.arguments[leftRow.arguments.start+offset], present: true}
		rightArgument := runtimeChild{inner: r.arguments[rightRow.arguments.start+offset], present: true}
		if answer, decided := r.runtimeEqualChild(left, leftArgument, right, rightArgument, path); !decided || !answer {
			return answer, decided
		}
	}
	return true, true
}

func (r *Runtime) runtimeTypeParameterSlice(row *runtimeRow) ([]runtimeNamedChild, bool) {
	if row == nil || row.typeParameters.start > row.typeParameters.end || uint64(row.typeParameters.end) > uint64(len(r.typeParameters)) {
		return nil, false
	}
	return r.typeParameters[row.typeParameters.start:row.typeParameters.end], true
}
