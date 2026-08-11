package core

import "math"

func (d *artifactDecoder) cell(index uint32) error {
	global, err := d.r.Bool()
	if err != nil {
		return err
	}
	cell := makeTerm(tagCell, index)
	if !global {
		storage, err := d.term()
		if err != nil {
			return err
		}
		d.b.cells = append(d.b.cells, cellRow{storage: storage})
		return nil
	}
	key, err := d.key()
	if err != nil || key == 0 {
		return ErrArtifactCanonical
	}
	if d.b.globalLookup == nil {
		d.b.globalLookup = make(map[Key]Term)
	}
	if d.b.globalLookup[key] != 0 {
		return ErrArtifactCanonical
	}
	storage, ok := encodeGlobalCellOrdinal(len(d.b.globalKeys))
	if !ok {
		return ErrArtifactCanonical
	}
	d.b.cells = append(d.b.cells, cellRow{storage: storage})
	d.b.globalKeys = append(d.b.globalKeys, key)
	d.b.globalCells = append(d.b.globalCells, cell)
	d.b.globalLookup[key] = cell
	return nil
}

func (d *artifactDecoder) function() error {
	owner, err := d.term()
	if err != nil {
		return err
	}
	body, err := d.term()
	if err != nil {
		return err
	}
	vararg, err := d.term()
	if err != nil {
		return err
	}
	formals, err := d.terms(&d.b.formalTerms)
	if err != nil {
		return err
	}
	captures, err := d.captures()
	if err != nil {
		return err
	}
	params, err := d.terms(&d.b.typeParamTerms)
	if err != nil {
		return err
	}
	paramsSet, err := d.r.Bool()
	if err != nil {
		return err
	}
	returnsKnown, err := d.r.Bool()
	if err != nil {
		return err
	}
	returns, err := d.terms(&d.b.staticTypeTerms)
	if err != nil {
		return err
	}
	returnsSet, err := d.r.Bool()
	if err != nil {
		return err
	}
	d.b.functions = append(d.b.functions, functionRow{owner: owner, body: body, vararg: vararg, formals: formals, captures: captures, typeParams: params, typeParamsSet: paramsSet, returnsKnown: returnsKnown, returns: returns, returnsSet: returnsSet})
	return nil
}

func (d *artifactDecoder) captures() (captureRange, error) {
	count, err := d.countAtLeast(artifactTermWireMin * 2)
	if err != nil {
		return captureRange{}, err
	}
	start, end, ok := boundedRange(len(d.b.captures), int(count))
	if !ok {
		return captureRange{}, ErrArtifactCanonical
	}
	for index := uint64(0); index < count; index++ {
		inner, err := d.term()
		if err != nil {
			return captureRange{}, err
		}
		outer, err := d.term()
		if err != nil {
			return captureRange{}, err
		}
		d.b.captures = append(d.b.captures, captureRow{inner: inner, outer: outer})
	}
	return captureRange{start: start, end: end}, nil
}

func (d *artifactDecoder) loop() error {
	owner, err := d.term()
	if err != nil {
		return err
	}
	body, err := d.term()
	if err != nil {
		return err
	}
	control, err := d.term()
	if err != nil {
		return err
	}
	kind, err := d.r.Uint()
	if err != nil || kind > math.MaxUint8 {
		return ErrArtifactCanonical
	}
	cells, err := d.terms(&d.b.loopCells)
	if err != nil {
		return err
	}
	d.b.loopOwners = append(d.b.loopOwners, owner)
	d.b.loopBodies = append(d.b.loopBodies, body)
	d.b.loopControls = append(d.b.loopControls, control)
	d.b.loopKinds = append(d.b.loopKinds, LoopKind(kind))
	d.b.loopCellRanges = append(d.b.loopCellRanges, cells)
	// Causal entries/successors are Seal-derived, but Builder owns one dense
	// zero placeholder per authored Loop before that proof can fill it.
	d.b.loopEntries = append(d.b.loopEntries, 0)
	d.b.loopNext = append(d.b.loopNext, 0)
	return nil
}

func (d *artifactDecoder) alias() error {
	owner, err := d.term()
	if err != nil {
		return err
	}
	target, err := d.term()
	if err != nil {
		return err
	}
	name, err := d.key()
	if err != nil {
		return err
	}
	nameSpan, err := d.span()
	if err != nil {
		return err
	}
	params, err := d.terms(&d.b.typeParamTerms)
	if err != nil {
		return err
	}
	paramsSet, err := d.r.Bool()
	if err != nil {
		return err
	}
	filled, err := d.r.Bool()
	if err != nil {
		return err
	}
	d.b.typeAliases = append(d.b.typeAliases, typeAliasRow{owner: owner, target: target, name: name, nameSpan: nameSpan, params: params, paramsSet: paramsSet, filled: filled})
	return nil
}

func (d *artifactDecoder) iface() error {
	owner, err := d.term()
	if err != nil {
		return err
	}
	name, err := d.key()
	if err != nil {
		return err
	}
	nameSpan, err := d.span()
	if err != nil {
		return err
	}
	extends, err := d.terms(&d.b.staticTypeTerms)
	if err != nil {
		return err
	}
	count, err := d.countAtLeast(21)
	if err != nil {
		return err
	}
	start, end, ok := boundedRange(len(d.b.interfaceMembers), int(count))
	if !ok {
		return ErrArtifactCanonical
	}
	for index := uint64(0); index < count; index++ {
		kind, err := d.r.Uint()
		if err != nil || kind > math.MaxUint8 {
			return ErrArtifactCanonical
		}
		field, err := d.term()
		if err != nil {
			return err
		}
		memberName, err := d.key()
		if err != nil {
			return err
		}
		memberSpan, err := d.span()
		if err != nil {
			return err
		}
		signature, err := d.term()
		if err != nil {
			return err
		}
		d.b.interfaceMembers = append(d.b.interfaceMembers, interfaceMemberRow{kind: InterfaceMemberKind(kind), field: field, name: memberName, nameSpan: memberSpan, signature: signature})
	}
	d.b.interfaces = append(d.b.interfaces, interfaceRow{owner: owner, name: name, nameSpan: nameSpan, extends: extends, members: interfaceMemberRange{start: start, end: end}, filled: true})
	return nil
}

func (d *artifactDecoder) typeRef() error {
	resolution, err := d.r.Uint()
	if err != nil || resolution > math.MaxUint8 {
		return ErrArtifactCanonical
	}
	target, err := d.term()
	if err != nil {
		return err
	}
	root, err := d.term()
	if err != nil {
		return err
	}
	source, err := d.keys(&d.b.typeRefSourceKeys)
	if err != nil {
		return err
	}
	canonical, err := d.keys(&d.b.typeRefResolutionKeys)
	if err != nil {
		return err
	}
	d.b.typeRefs = append(d.b.typeRefs, typeRefRow{resolution: TypeRefResolution(resolution), target: target, root: root, source: source, canonical: canonical})
	return nil
}

func (d *artifactDecoder) keys(pool *[]Key) (keyRange, error) {
	count, err := d.countAtLeast(artifactTermWireMin)
	if err != nil {
		return keyRange{}, err
	}
	start, end, ok := boundedRange(len(*pool), int(count))
	if !ok {
		return keyRange{}, ErrArtifactCanonical
	}
	for index := uint64(0); index < count; index++ {
		key, err := d.key()
		if err != nil {
			return keyRange{}, err
		}
		*pool = append(*pool, key)
	}
	return keyRange{start: start, end: end}, nil
}

func (d *artifactDecoder) signature() error {
	scope, err := d.term()
	if err != nil {
		return err
	}
	params, err := d.terms(&d.b.typeParamTerms)
	if err != nil {
		return err
	}
	paramsSet, err := d.r.Bool()
	if err != nil {
		return err
	}
	count, err := d.countAtLeast(18)
	if err != nil {
		return err
	}
	start, end, ok := boundedRange(len(d.b.signatureParams), int(count))
	if !ok {
		return ErrArtifactCanonical
	}
	for index := uint64(0); index < count; index++ {
		name, err := d.key()
		if err != nil {
			return err
		}
		nameSpan, err := d.span()
		if err != nil {
			return err
		}
		typ, err := d.term()
		if err != nil {
			return err
		}
		d.b.signatureParams = append(d.b.signatureParams, signatureParamRow{name: name, nameSpan: nameSpan, typ: typ})
	}
	variadic, err := d.term()
	if err != nil {
		return err
	}
	variadicSpan, err := d.span()
	if err != nil {
		return err
	}
	returnsKnown, err := d.r.Bool()
	if err != nil {
		return err
	}
	returns, err := d.terms(&d.b.staticTypeTerms)
	if err != nil {
		return err
	}
	filled, err := d.r.Bool()
	if err != nil {
		return err
	}
	d.b.signatures = append(d.b.signatures, signatureRow{scope: scope, typeParams: params, typeParamsSet: paramsSet, params: signatureParamRange{start: start, end: end}, variadic: variadic, variadicSpan: variadicSpan, returnsKnown: returnsKnown, returns: returns, filled: filled})
	return nil
}

func (d *artifactDecoder) finish() error {
	if d.b.sourceName == "" || len(d.b.spans[tagOutcome]) != 0 || d.counts[tagOutcome] != 0 {
		return ErrArtifactCanonical
	}
	var total uint64
	for tag := uint8(1); tag < tagCount; tag++ {
		count, ok := d.builderCount(tag)
		if !ok || count != int(d.counts[tag]) || len(d.b.spans[tag]) != count {
			return ErrArtifactCanonical
		}
		total += uint64(count)
	}
	if total > uint64(math.MaxInt) {
		return ErrArtifactCanonical
	}
	d.b.termCount = int(total)
	d.b.nilNext = make([]Term, len(d.b.nils))
	d.b.valueEntries = make([]Term, len(d.b.valueTerms))
	// Lens and TableField normalized keys are Seal inputs, but they are exact
	// functions of authored operand rows. Recreate them here rather than place
	// a second persisted key plane in the artifact.
	for index := range d.b.lensExact {
		row := &d.b.lensExact[index]
		if !d.b.validLensExactRow(row.owner, row.base, row.source, row.kind) {
			return ErrArtifactCanonical
		}
		switch row.kind {
		case FieldName:
			row.exact = d.b.keys[row.source.index()-1].exact
		case FieldExact:
			key, _ := d.b.normalizedExactKey(row.source)
			row.exact = key
		default:
			return ErrArtifactCanonical
		}
	}
	for index := range d.b.tableFields {
		row := &d.b.tableFields[index]
		if !d.b.validTableFieldRow(row.table, row.key, row.values, row.kind) {
			return ErrArtifactCanonical
		}
		switch row.kind {
		case FieldList, FieldName:
			row.normalized = d.b.keys[row.key.index()-1].exact
		case FieldExact:
			key, _ := d.b.normalizedExactKey(row.key)
			row.normalized = key
		case FieldKey:
		default:
			return ErrArtifactCanonical
		}
	}
	return nil
}

func (d *artifactDecoder) builderCount(tag uint8) (int, bool) {
	b := d.b
	switch tag {
	case tagNil:
		return len(b.nils), true
	case tagBool:
		return len(b.bools), true
	case tagInteger:
		return len(b.integers), true
	case tagFloat:
		return len(b.floats), true
	case tagString:
		return len(b.strings), true
	case tagValues:
		return len(b.values), true
	case tagLensExact:
		return len(b.lensExact), true
	case tagLensKey:
		return len(b.lensKeys), true
	case tagReturn:
		return len(b.returns), true
	case tagBreak:
		return len(b.breaks), true
	case tagLabel:
		return len(b.labelOwners), true
	case tagGoto:
		return len(b.gotoOwners), true
	case tagBody:
		return len(b.bodies), true
	case tagCell:
		return len(b.cells), true
	case tagRead:
		return len(b.reads), true
	case tagVararg:
		return len(b.varargs), true
	case tagUnary:
		return len(b.unaries), true
	case tagBinary:
		return len(b.binaries), true
	case tagSelect:
		return len(b.selects), true
	case tagBind:
		return len(b.binds), true
	case tagAssign:
		return len(b.assigns), true
	case tagFunction:
		return len(b.functions), true
	case tagCall:
		return len(b.calls), true
	case tagBranch:
		return len(b.branches), true
	case tagLoop:
		return len(b.loopOwners), true
	case tagTable:
		return len(b.tables), true
	case tagKey:
		return len(b.keys), true
	case tagTypeAlias:
		return len(b.typeAliases), true
	case tagTypeInterface:
		return len(b.interfaces), true
	case tagTypeParam:
		return len(b.typeParams), true
	case tagTypePrimitive:
		return len(b.primitiveTypes), true
	case tagTypeLiteral:
		return len(b.literalTypes), true
	case tagTypeOptional:
		return len(b.optionalTypes), true
	case tagTypeUnion:
		return len(b.unionTypes), true
	case tagTypeIntersection:
		return len(b.intersectionTypes), true
	case tagTypeRef:
		return len(b.typeRefs), true
	case tagTypeGeneric:
		return len(b.genericTypes), true
	case tagTypeArray:
		return len(b.arrayTypes), true
	case tagTypeMap:
		return len(b.mapTypes), true
	case tagTypeRecord:
		return len(b.recordTypes), true
	case tagTypeField:
		return len(b.typeFields), true
	case tagTypeFunction:
		return len(b.signatures), true
	case tagTypeAsserts:
		return len(b.assertions), true
	case tagDeclaredType:
		return len(b.declaredTypes), true
	case tagTypePublication:
		return len(b.typePublications), true
	case tagTypeValue:
		return len(b.typeValues), true
	case tagValueClaim:
		return len(b.valueClaims), true
	case tagAnnotation:
		return len(b.annotations), true
	case tagTypeOf:
		return len(b.typeOfs), true
	case tagTypeKeyOf:
		return len(b.keyOfTypes), true
	case tagTypeIndexAccess:
		return len(b.indexAccessTypes), true
	case tagTypeConditional:
		return len(b.conditionalTypes), true
	case tagWrite:
		return len(b.writes), true
	case tagTableField:
		return len(b.tableFields), true
	case tagOutcome:
		return 0, true
	case tagControlFault:
		return len(b.controlFaults), true
	case tagImport:
		return len(b.imports), true
	default:
		return 0, false
	}
}
