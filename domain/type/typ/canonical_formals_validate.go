package typ

import (
	"bytes"
	"context"
	"fmt"

	"github.com/wippyai/go-lua/domain/type/kind"
)

// ValidateCanonicalFormals accepts only the exact scoped canonical bytes
// emitted by EncodeCanonicalFormals for a caller-owned formal scope of the
// supplied size. It validates the wire graph without materializing Type values
// or consulting a source type graph.
//
// The formal count is part of the receiving scope, rather than part of the
// payload: an external formal ordinal is valid only when it names one of those
// positions. The encoding itself carries no presentation names for formals.
func ValidateCanonicalFormals(encoded []byte, externalFormalCount int) (err error) {
	// A form this process already admitted at this external scope is valid, and
	// the bytes are the form's whole identity, so there is nothing left to
	// re-derive from them.
	if canonicalValidatedWireForms.admits(encoded, externalFormalCount) {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: validator panic: %v", ErrInvalidCanonicalType, recovered)
		}
	}()
	admission, admissionErr := newCanonicalFormalsAdmission(context.Background(), len(encoded))
	if admissionErr != nil {
		return admissionErr
	}
	_, _, err = validatedCanonicalFormalsGraph(nil, encoded, externalFormalCount, admission)
	return err
}

// validatedCanonicalFormalsGraph returns the one canonical scoped graph after
// checking its framing, lexical formal relations, and quotient form.  Both the
// public validator and scoped decoder use this path so a decoded value cannot
// silently accept a broader wire language than a validated artifact.
func validatedCanonicalFormalsGraph(ctx context.Context, encoded []byte, externalFormalCount int, admission *canonicalFormalsAdmission) ([]canonicalTypeNode, []canonicalFormalNodeShape, error) {
	if externalFormalCount < 0 {
		return nil, nil, invalidCanonicalFormals("negative external formal count")
	}

	reader := canonicalRawReader{raw: encoded}
	domain, ok := reader.frame()
	if !ok || !bytes.Equal(domain, []byte(canonicalScopedTypeDomain)) {
		return nil, nil, invalidCanonicalFormals("domain")
	}
	version, ok := reader.uvarint()
	if !ok || version != canonicalScopedTypeVersion {
		return nil, nil, invalidCanonicalFormals("version")
	}
	graphStart := reader.at

	nodes, shapes, err := parseCanonicalFormalsGraph(ctx, admission, &reader, uint64(externalFormalCount))
	if err != nil {
		return nil, nil, err
	}
	if reader.at != len(reader.raw) {
		return nil, nil, invalidCanonicalFormals("trailing bytes")
	}
	// The graph is parsed on every call because the caller may need its nodes,
	// but the laws below judge the byte string, not this parse of it. An
	// already admitted form carries their verdict.
	if canonicalValidatedWireForms.admits(encoded, externalFormalCount) {
		return nodes, shapes, nil
	}
	if err := validateCanonicalFormalRelations(ctx, admission, nodes, shapes); err != nil {
		return nil, nil, err
	}
	if err := validateCanonicalFormalConstraintCycles(ctx, admission, nodes, shapes); err != nil {
		return nil, nil, err
	}
	if err := validateCanonicalFormalMaterializability(ctx, admission, nodes, shapes); err != nil {
		return nil, nil, err
	}
	if err := validateCanonicalFormalGenericRecurrence(ctx, admission, nodes, shapes); err != nil {
		return nil, nil, err
	}
	if err := validateCanonicalFormalLexicalScope(ctx, admission, nodes, shapes); err != nil {
		return nil, nil, err
	}

	// The raw graph is intentionally fed through the existing quotient and
	// emitter rather than a second decoder model. This rejects duplicate
	// definitions, noncanonical preorder choices, and every other graph-level
	// variation that would encode to different canonical bytes.
	regenerated := CanonicalEncoder{nodes: nodes, ctx: ctx, admission: admission, scoped: true}
	if err := regenerated.refine(); err != nil {
		return nil, nil, fmt.Errorf("%w: graph refinement: %v", ErrInvalidCanonicalType, err)
	}
	if len(regenerated.classes) != len(nodes) || len(regenerated.classes) == 0 {
		return nil, nil, invalidCanonicalFormals("missing root class")
	}
	if err := canonicalFormalsPreflight(ctx, admission, &regenerated.steps, len(nodes), canonicalFormalsMapEntryBytes); err != nil {
		return nil, nil, err
	}
	regenerated.ordinals = make(map[int]uint64, len(nodes))
	if err := regenerated.emitClass(regenerated.classes[0], regenerated.ordinals); err != nil {
		return nil, nil, fmt.Errorf("%w: graph emission: %v", ErrInvalidCanonicalType, err)
	}
	equal, equalErr := canonicalFormalsEqual(ctx, admission, regenerated.out, encoded[graphStart:])
	if equalErr != nil {
		return nil, nil, equalErr
	}
	if !equal {
		return nil, nil, invalidCanonicalFormals("noncanonical graph")
	}
	canonicalValidatedWireForms.admit(encoded, externalFormalCount)
	return nodes, shapes, nil
}

type canonicalFormalNodeShape struct {
	tag           byte
	children      uint64
	formalMode    byte
	formalOrdinal uint64
	binderParams  uint64
}

type canonicalFormalParseFrame struct {
	node      int
	remaining uint64
}

// validateCanonicalFormalNodeGraph is the encoder-side counterpart of the raw
// parser. Discovery has already produced graph edges, so it reuses the exact
// scalar and scope laws rather than trusting source construction to have kept
// local formals inside their owners.
func validateCanonicalFormalNodeGraph(ctx context.Context, admission *canonicalFormalsAdmission, nodes []canonicalTypeNode, externalFormalCount uint64) error {
	var steps uint64
	if len(nodes) == 0 {
		return invalidCanonicalFormals("graph shape")
	}
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsShapeBytes); err != nil {
		return err
	}
	shapes := make([]canonicalFormalNodeShape, len(nodes))
	for index, node := range nodes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		shape, err := validateCanonicalFormalScalar(ctx, admission, &steps, node.scalar, externalFormalCount)
		if err != nil {
			return err
		}
		if shape.children != uint64(len(node.edges)) {
			return invalidCanonicalFormals("scalar child count")
		}
		shapes[index] = shape
	}
	if err := validateCanonicalFormalRelations(ctx, admission, nodes, shapes); err != nil {
		return err
	}
	if err := validateCanonicalFormalConstraintCycles(ctx, admission, nodes, shapes); err != nil {
		return err
	}
	if err := validateCanonicalFormalMaterializability(ctx, admission, nodes, shapes); err != nil {
		return err
	}
	if err := validateCanonicalFormalGenericRecurrence(ctx, admission, nodes, shapes); err != nil {
		return err
	}
	return validateCanonicalFormalLexicalScope(ctx, admission, nodes, shapes)
}

// parseCanonicalFormalsGraph consumes exactly one preorder graph. Definitions
// are appended before their child events, which permits canonical active
// back-references without a recursive Go call frame.
func parseCanonicalFormalsGraph(ctx context.Context, admission *canonicalFormalsAdmission, reader *canonicalRawReader, externalFormalCount uint64) ([]canonicalTypeNode, []canonicalFormalNodeShape, error) {
	if reader == nil {
		return nil, nil, invalidCanonicalFormals("nil graph reader")
	}
	if err := canonicalFormalsPreflight(ctx, admission, nil, 16, canonicalFormalsNodeBytes); err != nil {
		return nil, nil, err
	}
	nodes := make([]canonicalTypeNode, 0, 16)
	if err := canonicalFormalsPreflight(ctx, admission, nil, 16, canonicalFormalsShapeBytes); err != nil {
		return nil, nil, err
	}
	shapes := make([]canonicalFormalNodeShape, 0, 16)
	if err := canonicalFormalsPreflight(ctx, admission, nil, 16, canonicalFormalsFrameBytes); err != nil {
		return nil, nil, err
	}
	stack := make([]canonicalFormalParseFrame, 0, 16)
	root := -1
	var steps uint64
	var appendErr error

	for root < 0 || len(stack) != 0 {
		if err := canonicalFormalsCheckpoint(ctx, admission, &steps); err != nil {
			return nil, nil, err
		}
		opcode, ok := reader.byte()
		if !ok {
			return nil, nil, invalidCanonicalFormals("missing graph node")
		}
		ordinal, ok := reader.uvarint()
		if !ok || ordinal > uint64(maxInt()) {
			return nil, nil, invalidCanonicalFormals("node ordinal")
		}
		index := int(ordinal)
		definition := false
		var childCount uint64

		switch opcode {
		case 0:
			if index >= len(nodes) {
				return nil, nil, invalidCanonicalFormals("forward node reference")
			}
		case 1:
			if index != len(nodes) {
				return nil, nil, invalidCanonicalFormals("non-dense node definition")
			}
			scalar, framed := reader.frame()
			if !framed || len(scalar) == 0 {
				return nil, nil, invalidCanonicalFormals("node scalar")
			}
			childCount, ok = reader.uvarint()
			// Every immediate child event has at least an opcode and an ordinal.
			// This is a representational bound, not a caller-tunable decode cap.
			if !ok || childCount > uint64(maxInt()) || childCount > uint64((len(reader.raw)-reader.at)/2) {
				return nil, nil, invalidCanonicalFormals("child count")
			}
			shape, shapeErr := validateCanonicalFormalScalar(ctx, admission, &steps, scalar, externalFormalCount)
			if shapeErr != nil {
				return nil, nil, shapeErr
			}
			if shape.children != childCount {
				return nil, nil, invalidCanonicalFormals("scalar child count")
			}
			if err := canonicalFormalsPreflight(ctx, admission, &steps, int(childCount), canonicalFormalsIntBytes); err != nil {
				return nil, nil, err
			}
			node := canonicalTypeNode{
				scalar: scalar,
				edges:  make([]int, 0, int(childCount)),
			}
			nodes, appendErr = canonicalFormalsAppend(ctx, admission, &steps, nodes, node, canonicalFormalsNodeBytes)
			if appendErr != nil {
				return nil, nil, appendErr
			}
			shapes, appendErr = canonicalFormalsAppend(ctx, admission, &steps, shapes, shape, canonicalFormalsShapeBytes)
			if appendErr != nil {
				return nil, nil, appendErr
			}
			definition = true
		default:
			return nil, nil, invalidCanonicalFormals("graph opcode")
		}

		if root < 0 {
			if !definition || index != 0 {
				return nil, nil, invalidCanonicalFormals("root is not definition zero")
			}
			root = index
		} else {
			if len(stack) == 0 || stack[len(stack)-1].remaining == 0 {
				return nil, nil, invalidCanonicalFormals("unowned graph node")
			}
			parent := &stack[len(stack)-1]
			nodes[parent.node].edges, appendErr = canonicalFormalsAppend(ctx, admission, &steps, nodes[parent.node].edges, index, canonicalFormalsIntBytes)
			if appendErr != nil {
				return nil, nil, appendErr
			}
			parent.remaining--
		}

		if definition && childCount != 0 {
			if err := canonicalFormalsPreflight(ctx, admission, &steps, 1, canonicalFormalsFrameBytes); err != nil {
				return nil, nil, err
			}
			stack, appendErr = canonicalFormalsAppend(ctx, admission, &steps, stack, canonicalFormalParseFrame{node: index, remaining: childCount}, canonicalFormalsFrameBytes)
			if appendErr != nil {
				return nil, nil, appendErr
			}
		}
		for len(stack) != 0 && stack[len(stack)-1].remaining == 0 {
			stack = stack[:len(stack)-1]
		}
	}
	return nodes, shapes, nil
}

func canonicalFormalValidationCheckpoint(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64) error {
	return canonicalFormalsCheckpoint(ctx, admission, steps)
}

func validateCanonicalFormalScalar(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, scalar []byte, externalFormalCount uint64) (canonicalFormalNodeShape, error) {
	reader := canonicalRawReader{raw: scalar}
	tag, ok := reader.byte()
	if !ok {
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("empty scalar")
	}
	shape := canonicalFormalNodeShape{tag: tag}
	finish := func() error {
		if reader.at != len(reader.raw) {
			return invalidCanonicalFormals("trailing node scalar")
		}
		return nil
	}
	leaf := func() (canonicalFormalNodeShape, error) {
		if err := finish(); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		return shape, nil
	}

	switch tag {
	case canonicalNil,
		canonicalPrimitiveNil,
		canonicalBoolean,
		canonicalNumber,
		canonicalInteger,
		canonicalString,
		canonicalAny,
		canonicalUnknown,
		canonicalNever,
		canonicalSelf:
		return leaf()

	case canonicalLiteral:
		base, valid := reader.byte()
		if !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("literal base")
		}
		switch kind.Kind(base) {
		case kind.Boolean:
			if _, valid = reader.bool(); !valid {
				return canonicalFormalNodeShape{}, invalidCanonicalFormals("boolean literal")
			}
		case kind.Integer:
			if _, valid = reader.varint(); !valid {
				return canonicalFormalNodeShape{}, invalidCanonicalFormals("integer literal")
			}
		case kind.Number:
			if _, fixed := reader.fixed64(); !fixed {
				return canonicalFormalNodeShape{}, invalidCanonicalFormals("number literal")
			}
		case kind.String:
			if _, valid = reader.frame(); !valid {
				return canonicalFormalNodeShape{}, invalidCanonicalFormals("string literal")
			}
		default:
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("literal base")
		}
		return leaf()

	case canonicalRef:
		if _, valid := reader.frame(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("reference module")
		}
		if _, valid := reader.frame(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("reference name")
		}
		return leaf()

	case canonicalOptional, canonicalArray, canonicalMeta:
		shape.children = 1
		if err := finish(); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		return shape, nil

	case canonicalMap, canonicalReadonlyMap:
		shape.children = 2
		if err := finish(); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		return shape, nil

	case canonicalUnion, canonicalIntersection, canonicalTuple:
		count, valid := reader.uvarint()
		if !valid || count > uint64(maxInt()) {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("aggregate arity")
		}
		shape.children = count
		if err := finish(); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		return shape, nil

	case canonicalRecord:
		return validateCanonicalFormalRecordScalar(ctx, admission, steps, reader, shape)

	case canonicalFunction:
		return validateCanonicalFormalFunctionScalar(ctx, admission, steps, reader, shape)

	case canonicalGeneric:
		if _, valid := reader.frame(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("generic name")
		}
		paramCount, valid := reader.uvarint()
		hasBody, bodyValid := reader.bool()
		if !valid || !bodyValid || paramCount > uint64(maxInt()) {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("generic header")
		}
		shape.binderParams = paramCount
		shape.children = paramCount
		if hasBody {
			var added bool
			shape.children, added = checkedCanonicalFormalAdd(shape.children, 1)
			if !added || shape.children > uint64(maxInt()) {
				return canonicalFormalNodeShape{}, invalidCanonicalFormals("generic child count")
			}
		}
		if err := finish(); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		return shape, nil

	case canonicalInstantiated:
		argumentCount, valid := reader.uvarint()
		if !valid || argumentCount >= uint64(maxInt()) {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("instantiated argument count")
		}
		shape.children = argumentCount + 1 // Generic is always child zero.
		if err := finish(); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		return shape, nil

	case canonicalTypeParam:
		mode, valid := reader.byte()
		ordinal, ordinalValid := reader.uvarint()
		hasConstraint, constraintValid := reader.bool()
		if !valid || !ordinalValid || !constraintValid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("formal header")
		}
		shape.formalMode = mode
		shape.formalOrdinal = ordinal
		switch mode {
		case canonicalScopedExternalFormal:
			if ordinal >= externalFormalCount {
				return canonicalFormalNodeShape{}, invalidCanonicalFormals("external formal ordinal")
			}
			if hasConstraint {
				shape.children = 1
			}
		case canonicalScopedLocalFormal:
			shape.children = 1 // Child zero is the lexical binder.
			if hasConstraint {
				var added bool
				shape.children, added = checkedCanonicalFormalAdd(shape.children, 1)
				if !added {
					return canonicalFormalNodeShape{}, invalidCanonicalFormals("local formal child count")
				}
			}
		default:
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("formal mode")
		}
		if err := finish(); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		return shape, nil

	case canonicalRecursive:
		hasBody, valid := reader.bool()
		if !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("recursive header")
		}
		if hasBody {
			shape.children = 1
		}
		if err := finish(); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		return shape, nil

	case canonicalInterface:
		if _, valid := reader.frame(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("interface name")
		}
		methodCount, valid := reader.uvarint()
		if !valid || methodCount > uint64(maxInt()) || methodCount > uint64(len(reader.raw)-reader.at) {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("interface method count")
		}
		for index := uint64(0); index < methodCount; index++ {
			if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
				return canonicalFormalNodeShape{}, err
			}
			if _, valid := reader.frame(); !valid {
				return canonicalFormalNodeShape{}, invalidCanonicalFormals("interface method")
			}
		}
		shape.children = methodCount
		if err := finish(); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		return shape, nil
	default:
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("unknown scalar tag")
	}
}

func validateCanonicalFormalRecordScalar(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, reader canonicalRawReader, shape canonicalFormalNodeShape) (canonicalFormalNodeShape, error) {
	if _, valid := reader.bool(); !valid {
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("record open flag")
	}
	fieldCount, valid := reader.uvarint()
	if !valid || fieldCount > uint64(maxInt()) || fieldCount > uint64((len(reader.raw)-reader.at)/3) {
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("record field count")
	}
	for index := uint64(0); index < fieldCount; index++ {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		if _, valid := reader.frame(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("record field name")
		}
		if _, valid := reader.bool(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("record field optional")
		}
		if _, valid := reader.bool(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("record field readonly")
		}
	}
	staticCount, valid := reader.uvarint()
	if !valid || staticCount > uint64(maxInt()) || staticCount > uint64((len(reader.raw)-reader.at)/5) {
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("record static count")
	}
	for index := uint64(0); index < staticCount; index++ {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		memberKind, kindValid := reader.byte()
		if !kindValid || StaticMemberKind(memberKind) < StaticMemberStringIndex || StaticMemberKind(memberKind) > StaticMemberIntIndex {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("record static member kind")
		}
		if _, valid := reader.frame(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("record static member name")
		}
		if _, valid := reader.varint(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("record static member index")
		}
		if _, valid := reader.bool(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("record static member optional")
		}
		if _, valid := reader.bool(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("record static member readonly")
		}
	}
	hasMap, mapValid := reader.bool()
	hasMeta, metaValid := reader.bool()
	if !mapValid || !metaValid || reader.at != len(reader.raw) {
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("record shape")
	}
	shape.children = fieldCount
	var added bool
	shape.children, added = checkedCanonicalFormalAdd(shape.children, staticCount)
	if !added {
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("record child count")
	}
	if hasMap {
		shape.children, added = checkedCanonicalFormalAdd(shape.children, 2)
		if !added {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("record map child count")
		}
	}
	if hasMeta {
		shape.children, added = checkedCanonicalFormalAdd(shape.children, 1)
		if !added {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("record meta child count")
		}
	}
	if shape.children > uint64(maxInt()) {
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("record child count")
	}
	return shape, nil
}

func validateCanonicalFormalFunctionScalar(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, reader canonicalRawReader, shape canonicalFormalNodeShape) (canonicalFormalNodeShape, error) {
	typeParamCount, typeParamsValid := reader.uvarint()
	paramCount, paramsValid := reader.uvarint()
	if !typeParamsValid || !paramsValid || typeParamCount > uint64(maxInt()) || paramCount > uint64(maxInt()) ||
		paramCount > uint64((len(reader.raw)-reader.at)/2) {
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("function header")
	}
	for index := uint64(0); index < paramCount; index++ {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, steps); err != nil {
			return canonicalFormalNodeShape{}, err
		}
		if _, valid := reader.bool(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("function parameter optional")
		}
		if _, valid := reader.bool(); !valid {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("function parameter receiver")
		}
	}
	hasVariadic, variadicValid := reader.bool()
	returnCount, returnsValid := reader.uvarint()
	if !variadicValid || !returnsValid || returnCount > uint64(maxInt()) {
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("function result header")
	}
	shape.binderParams = typeParamCount
	shape.children = typeParamCount
	var added bool
	shape.children, added = checkedCanonicalFormalAdd(shape.children, paramCount)
	if !added {
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("function child count")
	}
	if hasVariadic {
		shape.children, added = checkedCanonicalFormalAdd(shape.children, 1)
		if !added {
			return canonicalFormalNodeShape{}, invalidCanonicalFormals("function variadic child count")
		}
	}
	shape.children, added = checkedCanonicalFormalAdd(shape.children, returnCount)
	if !added || shape.children > uint64(maxInt()) || reader.at != len(reader.raw) {
		return canonicalFormalNodeShape{}, invalidCanonicalFormals("function child shape")
	}
	return shape, nil
}

func validateCanonicalFormalRelations(ctx context.Context, admission *canonicalFormalsAdmission, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape) error {
	if len(nodes) == 0 || len(nodes) != len(shapes) {
		return invalidCanonicalFormals("graph shape")
	}
	var steps uint64
	for nodeIndex, node := range nodes {
		if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		shape := shapes[nodeIndex]
		switch shape.tag {
		case canonicalTypeParam:
			if shape.formalMode != canonicalScopedLocalFormal {
				continue
			}
			if len(node.edges) == 0 {
				return invalidCanonicalFormals("local formal owner")
			}
			ownerIndex := node.edges[0]
			if ownerIndex < 0 || ownerIndex >= len(nodes) {
				return invalidCanonicalFormals("local formal owner")
			}
			ownerShape := shapes[ownerIndex]
			if ownerShape.tag != canonicalFunction && ownerShape.tag != canonicalGeneric {
				return invalidCanonicalFormals("local formal owner tag")
			}
			if shape.formalOrdinal >= ownerShape.binderParams || shape.formalOrdinal >= uint64(len(nodes[ownerIndex].edges)) {
				return invalidCanonicalFormals("local formal ordinal")
			}
			if nodes[ownerIndex].edges[int(shape.formalOrdinal)] != nodeIndex {
				return invalidCanonicalFormals("local formal reverse owner")
			}

		case canonicalFunction, canonicalGeneric:
			if shape.binderParams > uint64(len(node.edges)) {
				return invalidCanonicalFormals("binder parameter count")
			}
			// A binder frame either introduces every parameter as a fresh
			// local formal, or owns the receiving external scope and is
			// re-entered through a cycle in the cut graph. A frame drawn from
			// both leaves a parameter without a single owner.
			var mode byte
			for ordinal := uint64(0); ordinal < shape.binderParams; ordinal++ {
				if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
					return err
				}
				formalIndex := node.edges[int(ordinal)]
				if formalIndex < 0 || formalIndex >= len(nodes) {
					return invalidCanonicalFormals("binder formal edge")
				}
				formal := shapes[formalIndex]
				if formal.tag != canonicalTypeParam {
					return invalidCanonicalFormals("binder formal relation")
				}
				if ordinal == 0 {
					mode = formal.formalMode
				} else if formal.formalMode != mode {
					return invalidCanonicalFormals("binder frame mixes external and local formals")
				}
				switch formal.formalMode {
				case canonicalScopedLocalFormal:
					if formal.formalOrdinal != ordinal || len(nodes[formalIndex].edges) == 0 || nodes[formalIndex].edges[0] != nodeIndex {
						return invalidCanonicalFormals("binder formal relation")
					}
				case canonicalScopedExternalFormal:
					for prior := uint64(0); prior < ordinal; prior++ {
						if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
							return err
						}
						if node.edges[int(prior)] == formalIndex {
							return invalidCanonicalFormals("binder repeats an external formal")
						}
					}
				default:
					return invalidCanonicalFormals("binder formal relation")
				}
			}

		case canonicalInstantiated:
			if len(node.edges) == 0 || shapes[node.edges[0]].tag != canonicalGeneric {
				return invalidCanonicalFormals("instantiated generic")
			}

		case canonicalInterface:
			for _, method := range node.edges {
				if err := canonicalFormalValidationCheckpoint(ctx, admission, &steps); err != nil {
					return err
				}
				if method < 0 || method >= len(nodes) || shapes[method].tag != canonicalFunction {
					return invalidCanonicalFormals("interface method type")
				}
			}
		}
	}
	return nil
}

func checkedCanonicalFormalAdd(left, right uint64) (uint64, bool) {
	if ^uint64(0)-left < right {
		return 0, false
	}
	return left + right, true
}

func invalidCanonicalFormals(reason string) error {
	return fmt.Errorf("%w: scoped formals %s", ErrInvalidCanonicalType, reason)
}
