package typ

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/wippyai/go-lua/domain/type/kind"
)

var (
	ErrInvalidCanonicalType = errors.New("typ: invalid canonical type encoding")
	// ErrCanonicalRecursiveIdentityUnavailable is deliberately distinct from a
	// malformed stream. Structural bytes alone cannot recreate the stricter
	// declaration identity required by a recursive typewitness.
	ErrCanonicalRecursiveIdentityUnavailable = errors.New("typ: canonical recursive identity is unavailable")
)

type decodedCanonicalNode struct {
	scalar []byte
	edges  []int
}

type canonicalDecodeFrame struct {
	node      int
	remaining uint64
}

// DecodeCanonical reconstructs the exact nonrecursive TypeEquals
// representative emitted by EncodeCanonical. It accepts shared acyclic DAGs,
// rejects malformed/forward/active references and recursive identity, and
// publishes only after an encode/decode/encode byte-equality check.
func DecodeCanonical(ctx context.Context, encoded []byte) (decoded Type, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			decoded = nil
			err = fmt.Errorf("%w: %v", ErrInvalidCanonicalType, recovered)
		}
	}()

	parser := canonicalRawReader{raw: encoded}
	domain, ok := parser.frame()
	if !ok || string(domain) != canonicalTypeDomain {
		return nil, fmt.Errorf("%w: domain", ErrInvalidCanonicalType)
	}
	version, ok := parser.uvarint()
	if !ok || version != canonicalTypeVersion {
		return nil, fmt.Errorf("%w: version", ErrInvalidCanonicalType)
	}
	nodes, root, err := decodeCanonicalGraph(ctx, &parser)
	if err != nil {
		return nil, err
	}
	if parser.at != len(parser.raw) {
		return nil, fmt.Errorf("%w: trailing bytes", ErrInvalidCanonicalType)
	}
	decoded, err = materializeCanonicalGraph(ctx, nodes, root)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	roundTrip, err := EncodeCanonical(ctx, decoded)
	if err != nil {
		return nil, fmt.Errorf("%w: reconstructed value cannot encode: %v", ErrInvalidCanonicalType, err)
	}
	if !bytes.Equal(roundTrip, encoded) {
		return nil, fmt.Errorf("%w: reconstructed value changed canonical bytes", ErrInvalidCanonicalType)
	}
	return decoded, nil
}

// DecodeCanonicalStructural decodes a canonical type graph for structural
// consumers that already hold the graph as a closed publication. Unlike
// DecodeCanonical, it permits Recursive nodes and restores them as fresh
// placeholders; it must not be used where declaration identity is authority.
func DecodeCanonicalStructural(ctx context.Context, encoded []byte) (decoded Type, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			decoded = nil
			err = fmt.Errorf("%w: %v", ErrInvalidCanonicalType, recovered)
		}
	}()

	parser := canonicalRawReader{raw: encoded}
	domain, ok := parser.frame()
	if !ok || string(domain) != canonicalTypeDomain {
		return nil, fmt.Errorf("%w: domain", ErrInvalidCanonicalType)
	}
	version, ok := parser.uvarint()
	if !ok || version != canonicalTypeVersion {
		return nil, fmt.Errorf("%w: version", ErrInvalidCanonicalType)
	}
	nodes, root, err := decodeCanonicalStructuralGraph(ctx, &parser)
	if err != nil {
		return nil, err
	}
	if parser.at != len(parser.raw) {
		return nil, fmt.Errorf("%w: trailing bytes", ErrInvalidCanonicalType)
	}
	decoded, err = materializeCanonicalStructuralGraph(ctx, nodes, root)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	roundTrip, err := EncodeCanonical(ctx, decoded)
	if err != nil {
		return nil, fmt.Errorf("%w: reconstructed value cannot encode: %v", ErrInvalidCanonicalType, err)
	}
	if !bytes.Equal(roundTrip, encoded) {
		return nil, fmt.Errorf("%w: reconstructed value changed canonical bytes", ErrInvalidCanonicalType)
	}
	return decoded, nil
}

func decodeCanonicalGraph(ctx context.Context, parser *canonicalRawReader) ([]decodedCanonicalNode, int, error) {
	return decodeCanonicalGraphMode(ctx, parser, false)
}

func decodeCanonicalStructuralGraph(ctx context.Context, parser *canonicalRawReader) ([]decodedCanonicalNode, int, error) {
	return decodeCanonicalGraphMode(ctx, parser, true)
}

func decodeCanonicalGraphMode(ctx context.Context, parser *canonicalRawReader, structural bool) ([]decodedCanonicalNode, int, error) {
	nodes := make([]decodedCanonicalNode, 0, 16)
	complete := make([]bool, 0, 16)
	stack := make([]canonicalDecodeFrame, 0, 16)
	root := -1
	var steps uint64
	for root < 0 || len(stack) != 0 || !complete[root] {
		if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
			return nil, -1, err
		}
		kindByte, ok := parser.byte()
		if !ok {
			return nil, -1, fmt.Errorf("%w: missing graph node", ErrInvalidCanonicalType)
		}
		ordinal, ok := parser.uvarint()
		if !ok || ordinal > uint64(maxInt()) {
			return nil, -1, fmt.Errorf("%w: node ordinal", ErrInvalidCanonicalType)
		}
		index := int(ordinal)
		definition := false
		var childCount uint64
		switch kindByte {
		case 0:
			if index >= len(nodes) {
				return nil, -1, fmt.Errorf("%w: forward node reference %d", ErrInvalidCanonicalType, index)
			}
			if !structural && !complete[index] {
				return nil, -1, ErrCanonicalRecursiveIdentityUnavailable
			}
		case 1:
			if index != len(nodes) {
				return nil, -1, fmt.Errorf("%w: non-dense node definition %d", ErrInvalidCanonicalType, index)
			}
			scalar, framed := parser.frame()
			if !framed || len(scalar) == 0 {
				return nil, -1, fmt.Errorf("%w: node scalar", ErrInvalidCanonicalType)
			}
			if !structural && scalar[0] == canonicalRecursive {
				return nil, -1, ErrCanonicalRecursiveIdentityUnavailable
			}
			childCount, ok = parser.uvarint()
			// Every direct child needs at least an opcode and an ordinal. This
			// structural bound prevents a tiny malformed stream from requesting a
			// huge edge allocation; it rejects no valid encoding.
			if !ok || childCount > uint64(maxInt()) || childCount > uint64((len(parser.raw)-parser.at)/2) {
				return nil, -1, fmt.Errorf("%w: child count", ErrInvalidCanonicalType)
			}
			nodes = append(nodes, decodedCanonicalNode{scalar: append([]byte(nil), scalar...), edges: make([]int, 0, int(childCount))})
			complete = append(complete, childCount == 0)
			definition = true
		default:
			return nil, -1, fmt.Errorf("%w: graph opcode %d", ErrInvalidCanonicalType, kindByte)
		}

		if root < 0 {
			if !definition || index != 0 {
				return nil, -1, fmt.Errorf("%w: root is not definition zero", ErrInvalidCanonicalType)
			}
			root = index
		} else {
			if len(stack) == 0 || stack[len(stack)-1].remaining == 0 {
				return nil, -1, fmt.Errorf("%w: unowned graph node", ErrInvalidCanonicalType)
			}
			parent := &stack[len(stack)-1]
			nodes[parent.node].edges = append(nodes[parent.node].edges, index)
			parent.remaining--
		}
		if definition && childCount != 0 {
			stack = append(stack, canonicalDecodeFrame{node: index, remaining: childCount})
		}
		for len(stack) != 0 && stack[len(stack)-1].remaining == 0 {
			completed := stack[len(stack)-1].node
			stack = stack[:len(stack)-1]
			complete[completed] = true
		}
	}
	return nodes, root, nil
}

func materializeCanonicalGraph(ctx context.Context, nodes []decodedCanonicalNode, root int) (Type, error) {
	if root < 0 || root >= len(nodes) {
		return nil, fmt.Errorf("%w: missing root", ErrInvalidCanonicalType)
	}
	parents := make([][]int, len(nodes))
	remaining := make([]int, len(nodes))
	ready := make([]int, 0, len(nodes))
	for parent, node := range nodes {
		remaining[parent] = len(node.edges)
		if len(node.edges) == 0 {
			ready = append(ready, parent)
		}
		for _, child := range node.edges {
			if child < 0 || child >= len(nodes) {
				return nil, fmt.Errorf("%w: edge outside graph", ErrInvalidCanonicalType)
			}
			parents[child] = append(parents[child], parent)
		}
	}
	built := make([]Type, len(nodes))
	builtReady := make([]bool, len(nodes))
	var steps uint64
	for head := 0; head < len(ready); head++ {
		if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
			return nil, err
		}
		index := ready[head]
		children := make([]Type, len(nodes[index].edges))
		for position, child := range nodes[index].edges {
			if !builtReady[child] {
				return nil, fmt.Errorf("%w: non-acyclic dependency", ErrInvalidCanonicalType)
			}
			children[position] = built[child]
		}
		value, err := materializeCanonicalNode(ctx, nodes[index].scalar, children, &steps)
		if err != nil {
			return nil, err
		}
		built[index], builtReady[index] = value, true
		for _, parent := range parents[index] {
			remaining[parent]--
			if remaining[parent] == 0 {
				ready = append(ready, parent)
			}
		}
	}
	if !builtReady[root] {
		return nil, ErrCanonicalRecursiveIdentityUnavailable
	}
	return built[root], nil
}

// materializeCanonicalStructuralGraph is the recursive counterpart of
// materializeCanonicalGraph. It allocates Recursive placeholders first, which
// makes their backedges available to ordinary product-node reconstruction.
// Any cycle not anchored by a Recursive node remains unmaterializable and
// therefore fails closed.
func materializeCanonicalStructuralGraph(ctx context.Context, nodes []decodedCanonicalNode, root int) (Type, error) {
	if root < 0 || root >= len(nodes) {
		return nil, fmt.Errorf("%w: missing root", ErrInvalidCanonicalType)
	}
	built := make([]Type, len(nodes))
	ready := make([]bool, len(nodes))
	recursive := make([]bool, len(nodes))
	generic := make([]bool, len(nodes))
	genericBodies := make([]int, len(nodes))
	for index := range genericBodies {
		genericBodies[index] = -1
	}
	var steps uint64

	for index, node := range nodes {
		tag, ok := canonicalNodeTag(node.scalar)
		if !ok {
			return nil, fmt.Errorf("%w: empty scalar", ErrInvalidCanonicalType)
		}
		if tag != canonicalRecursive {
			continue
		}
		hasBody, err := canonicalRecursiveHeader(node.scalar)
		if err != nil {
			return nil, err
		}
		childCount := 0
		if hasBody {
			childCount = 1
		}
		if len(node.edges) != childCount {
			return nil, fmt.Errorf("%w: recursive child shape", ErrInvalidCanonicalType)
		}
		built[index] = NewRecursivePlaceholder("")
		ready[index], recursive[index] = true, true
	}

	for {
		progress := false
		for index, node := range nodes {
			if ready[index] {
				continue
			}
			children := make([]Type, len(node.edges))
			allReady := true
			for position, child := range node.edges {
				if child < 0 || child >= len(nodes) || !ready[child] {
					allReady = false
					break
				}
				children[position] = built[child]
			}
			if !allReady {
				continue
			}
			value, err := materializeCanonicalNode(ctx, node.scalar, children, &steps)
			if err != nil {
				return nil, err
			}
			built[index], ready[index], progress = value, true, true
		}
		if progress {
			continue
		}
		// Every node whose children already exist is materialized, so what
		// remains is a cycle. A generic declaration whose body reaches the
		// declaration itself is the only such cycle a canonical graph can carry
		// besides a recursive binder, so open exactly those binders and resume.
		// A generic opened this way carries a provisional hash until its body is
		// back-patched, which is why it is opened only when the ordinary
		// dependency order cannot complete it.
		opened, err := openStalledCanonicalGenerics(nodes, built, ready, generic, genericBodies)
		if err != nil {
			return nil, err
		}
		if !opened {
			break
		}
	}

	for index, node := range nodes {
		if !ready[index] {
			return nil, ErrCanonicalRecursiveIdentityUnavailable
		}
		if generic[index] && genericBodies[index] >= 0 {
			body := built[genericBodies[index]]
			if body == nil {
				return nil, fmt.Errorf("%w: generic body", ErrInvalidCanonicalType)
			}
			built[index].(*Generic).SetBody(body)
		}
		if !recursive[index] || len(node.edges) == 0 {
			continue
		}
		body := built[node.edges[0]]
		if body == nil {
			return nil, fmt.Errorf("%w: recursive body", ErrInvalidCanonicalType)
		}
		built[index].(*Recursive).SetBody(body)
	}
	return built[root], nil
}

// openStalledCanonicalGenerics allocates body-less generic declarations for the
// generic nodes a stalled dependency walk cannot complete, recording each body
// edge for the later back-patch. It reports whether any binder was opened, so a
// stall with no remaining generic ends the walk instead of looping.
func openStalledCanonicalGenerics(nodes []decodedCanonicalNode, built []Type, ready, generic []bool, genericBodies []int) (bool, error) {
	opened := false
	for index, node := range nodes {
		if ready[index] {
			continue
		}
		tag, ok := canonicalNodeTag(node.scalar)
		if !ok {
			return false, fmt.Errorf("%w: empty scalar", ErrInvalidCanonicalType)
		}
		if tag != canonicalGeneric {
			continue
		}
		name, paramCount, hasBody, err := canonicalGenericHeader(node.scalar)
		if err != nil {
			return false, err
		}
		childCount := paramCount
		if hasBody {
			childCount++
		}
		if len(node.edges) != childCount {
			return false, fmt.Errorf("%w: generic child shape", ErrInvalidCanonicalType)
		}
		params := make([]*TypeParam, paramCount)
		for position := range params {
			child := node.edges[position]
			if child < 0 || child >= len(nodes) || !ready[child] {
				params = nil
				break
			}
			param, valid := built[child].(*TypeParam)
			if !valid || param == nil {
				return false, fmt.Errorf("%w: generic type parameter", ErrInvalidCanonicalType)
			}
			params[position] = param
		}
		if params == nil {
			continue
		}
		built[index] = NewGeneric(name, params, nil)
		ready[index], generic[index], opened = true, true, true
		if hasBody {
			genericBodies[index] = node.edges[paramCount]
		}
	}
	return opened, nil
}

func canonicalNodeTag(scalar []byte) (byte, bool) {
	if len(scalar) == 0 {
		return 0, false
	}
	return scalar[0], true
}

// canonicalRecursiveHeader reads one binder frame. The binder carries no name:
// it is a bound variable whose occurrences are graph edges, so the decoded
// placeholder is anonymous and presentation is restored by the caller that
// owns a declaration name.
func canonicalRecursiveHeader(scalar []byte) (bool, error) {
	r := canonicalRawReader{raw: scalar}
	tag, ok := r.byte()
	if !ok || tag != canonicalRecursive {
		return false, fmt.Errorf("%w: recursive scalar", ErrInvalidCanonicalType)
	}
	hasBody, bodyOK := r.bool()
	if !bodyOK || r.at != len(r.raw) {
		return false, fmt.Errorf("%w: recursive shape", ErrInvalidCanonicalType)
	}
	return hasBody, nil
}

func canonicalGenericHeader(scalar []byte) (string, int, bool, error) {
	r := canonicalRawReader{raw: scalar}
	tag, ok := r.byte()
	if !ok || tag != canonicalGeneric {
		return "", 0, false, fmt.Errorf("%w: generic scalar", ErrInvalidCanonicalType)
	}
	name, nameOK := r.frame()
	paramCount, paramsOK := r.uvarint()
	hasBody, bodyOK := r.bool()
	if !nameOK || !paramsOK || !bodyOK || paramCount > uint64(maxInt()) || r.at != len(r.raw) {
		return "", 0, false, fmt.Errorf("%w: generic shape", ErrInvalidCanonicalType)
	}
	return string(name), int(paramCount), hasBody, nil
}

// materializeCanonicalUnionNode rebuilds a published union node.
//
// Member order in a union node is fixed by hash at construction time, and the
// hash of a recursive binder is a function of its body. A structural decode
// allocates every binder as an open placeholder and only closes it once the
// whole graph exists, so re-sorting a union that reaches a binder would order
// it by placeholder hashes that no longer hold once the bodies are set. The
// published order is the graph's own order and is retained verbatim in that
// case; without an open binder the member hashes are already final and the
// ordinary canonicalizing constructor still rejects a reordered stream.
func materializeCanonicalUnionNode(children []Type) Type {
	filtered := filterNilTypes(children)
	if !containsOpenRecursiveMember(filtered) {
		return MaterializeUnion(children)
	}
	unique, hashes := deduplicateTypesWithHashes(filtered)
	return newCanonicalUnion(unique, hashes)
}

// materializeCanonicalIntersectionNode is the intersection counterpart of
// materializeCanonicalUnionNode and retains the same published order rule.
func materializeCanonicalIntersectionNode(children []Type) Type {
	filtered := filterNilTypes(children)
	if !containsOpenRecursiveMember(filtered) {
		return MaterializeIntersection(children)
	}
	unique, hashes := deduplicateTypesWithHashes(filtered)
	return newCanonicalIntersection(unique, hashes)
}

func containsOpenRecursiveMember(members []Type) bool {
	for _, member := range members {
		if mayContainOpenRecursive(member) {
			return true
		}
	}
	return false
}

func materializeCanonicalNode(ctx context.Context, scalar []byte, children []Type, steps *uint64) (Type, error) {
	r := canonicalRawReader{raw: scalar}
	tag, ok := r.byte()
	if !ok {
		return nil, fmt.Errorf("%w: empty scalar", ErrInvalidCanonicalType)
	}
	wantChildren := func(count int) error {
		if len(children) != count {
			return fmt.Errorf("%w: tag %d has %d children, want %d", ErrInvalidCanonicalType, tag, len(children), count)
		}
		return nil
	}
	finish := func() error {
		if r.at != len(r.raw) {
			return fmt.Errorf("%w: trailing node scalar", ErrInvalidCanonicalType)
		}
		return nil
	}
	leaf := func(value Type) (Type, error) {
		if err := wantChildren(0); err != nil {
			return nil, err
		}
		if err := finish(); err != nil {
			return nil, err
		}
		return value, nil
	}

	switch tag {
	case canonicalNil:
		return leaf(nil)
	case canonicalPrimitiveNil:
		return leaf(Nil)
	case canonicalBoolean:
		return leaf(Boolean)
	case canonicalNumber:
		return leaf(Number)
	case canonicalInteger:
		return leaf(Integer)
	case canonicalString:
		return leaf(String)
	case canonicalAny:
		return leaf(Any)
	case canonicalUnknown:
		return leaf(Unknown)
	case canonicalNever:
		return leaf(Never)
	case canonicalSelf:
		return leaf(Self)
	case canonicalLiteral:
		if err := wantChildren(0); err != nil {
			return nil, err
		}
		base, ok := r.byte()
		if !ok {
			return nil, fmt.Errorf("%w: literal base", ErrInvalidCanonicalType)
		}
		var value Type
		switch kind.Kind(base) {
		case kind.Boolean:
			v, valid := r.bool()
			if !valid {
				return nil, fmt.Errorf("%w: boolean literal", ErrInvalidCanonicalType)
			}
			value = LiteralBool(v)
		case kind.Integer:
			v, valid := r.varint()
			if !valid {
				return nil, fmt.Errorf("%w: integer literal", ErrInvalidCanonicalType)
			}
			value = LiteralInt(v)
		case kind.Number:
			bits, valid := r.fixed64()
			if !valid {
				return nil, fmt.Errorf("%w: number literal", ErrInvalidCanonicalType)
			}
			value = LiteralNumber(math.Float64frombits(bits))
		case kind.String:
			v, valid := r.frame()
			if !valid {
				return nil, fmt.Errorf("%w: string literal", ErrInvalidCanonicalType)
			}
			value = LiteralString(string(v))
		default:
			return nil, fmt.Errorf("%w: literal base %d", ErrInvalidCanonicalType, base)
		}
		if err := finish(); err != nil {
			return nil, err
		}
		return value, nil
	case canonicalRef:
		if err := wantChildren(0); err != nil {
			return nil, err
		}
		module, okModule := r.frame()
		name, okName := r.frame()
		if !okModule || !okName || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: reference", ErrInvalidCanonicalType)
		}
		return NewRef(string(module), string(name)), nil
	case canonicalOptional:
		if err := wantChildren(1); err != nil {
			return nil, err
		}
		if err := finish(); err != nil {
			return nil, err
		}
		return MaterializeOptional(children[0]), nil
	case canonicalUnion, canonicalIntersection, canonicalTuple:
		count, ok := r.uvarint()
		if !ok || count != uint64(len(children)) || count > uint64(maxInt()) || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: aggregate arity", ErrInvalidCanonicalType)
		}
		switch tag {
		case canonicalUnion:
			return materializeCanonicalUnionNode(children), nil
		case canonicalIntersection:
			return materializeCanonicalIntersectionNode(children), nil
		default:
			for range children {
				if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
					return nil, err
				}
			}
			return NewTuple(children...), nil
		}
	case canonicalArray:
		if err := wantChildren(1); err != nil {
			return nil, err
		}
		if err := finish(); err != nil {
			return nil, err
		}
		return NewArray(children[0]), nil
	case canonicalMap, canonicalReadonlyMap:
		if err := wantChildren(2); err != nil {
			return nil, err
		}
		if err := finish(); err != nil {
			return nil, err
		}
		if tag == canonicalMap {
			return RebuildMap(children[0], children[1]), nil
		}
		return RebuildReadonlyMap(children[0], children[1]), nil
	case canonicalRecord:
		open, ok := r.bool()
		fieldCount, fieldsOK := r.uvarint()
		// name-frame + optional + readonly is at least three bytes per field.
		if !ok || !fieldsOK || fieldCount > uint64(maxInt()) || fieldCount > uint64((len(r.raw)-r.at)/3) || fieldCount > uint64(len(children)) {
			return nil, fmt.Errorf("%w: record header", ErrInvalidCanonicalType)
		}
		fields := make([]Field, int(fieldCount))
		for index := range fields {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			name, nameOK := r.frame()
			optional, optionalOK := r.bool()
			readonly, readonlyOK := r.bool()
			if !nameOK || !optionalOK || !readonlyOK {
				return nil, fmt.Errorf("%w: record field", ErrInvalidCanonicalType)
			}
			fields[index] = Field{Name: string(name), Optional: optional, Readonly: readonly}
		}
		staticCount, ok := r.uvarint()
		// kind + name-frame + index + optional + readonly is at least five
		// bytes per static member.
		if !ok || staticCount > uint64(maxInt()) || staticCount > uint64((len(r.raw)-r.at)/5) ||
			staticCount > uint64(len(children))-fieldCount {
			return nil, fmt.Errorf("%w: record static count", ErrInvalidCanonicalType)
		}
		members := make([]StaticMember, int(staticCount))
		for index := range members {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			memberKind, kindOK := r.byte()
			name, nameOK := r.frame()
			memberIndex, indexOK := r.varint()
			optional, optionalOK := r.bool()
			readonly, readonlyOK := r.bool()
			if !kindOK || !nameOK || !indexOK || !optionalOK || !readonlyOK ||
				StaticMemberKind(memberKind) < StaticMemberStringIndex || StaticMemberKind(memberKind) > StaticMemberIntIndex {
				return nil, fmt.Errorf("%w: record static member", ErrInvalidCanonicalType)
			}
			members[index] = StaticMember{Kind: StaticMemberKind(memberKind), Name: string(name), Index: memberIndex, Optional: optional, Readonly: readonly}
		}
		hasMap, mapOK := r.bool()
		hasMeta, metaOK := r.bool()
		childCount := int(fieldCount + staticCount)
		if hasMap {
			childCount += 2
		}
		if hasMeta {
			childCount++
		}
		if !mapOK || !metaOK || r.at != len(r.raw) || childCount != len(children) {
			return nil, fmt.Errorf("%w: record child shape", ErrInvalidCanonicalType)
		}
		at := 0
		for index := range fields {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			fields[index].Type = children[at]
			at++
		}
		for index := range members {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			members[index].Type = children[at]
			at++
		}
		parts := RecordParts{Fields: fields, StaticMembers: members, Open: open, AssumeSorted: true}
		if hasMap {
			parts.MapKey, parts.MapValue = children[at], children[at+1]
			at += 2
		}
		if hasMeta {
			parts.Metatable = children[at]
		}
		return RebuildRecord(parts), nil
	case canonicalFunction:
		typeParamCount, ok := r.uvarint()
		paramCount, paramsOK := r.uvarint()
		if !ok || !paramsOK || typeParamCount > uint64(maxInt()) || paramCount > uint64(maxInt()) ||
			typeParamCount > uint64(len(children)) || paramCount > uint64(len(children))-typeParamCount ||
			paramCount > uint64((len(r.raw)-r.at)/2) {
			return nil, fmt.Errorf("%w: function header", ErrInvalidCanonicalType)
		}
		optional := make([]bool, int(paramCount))
		receiver := make([]bool, int(paramCount))
		for index := range optional {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			optional[index], ok = r.bool()
			receiver[index], paramsOK = r.bool()
			if !ok || !paramsOK {
				return nil, fmt.Errorf("%w: function parameter", ErrInvalidCanonicalType)
			}
		}
		hasVariadic, variadicOK := r.bool()
		returnCount, returnsOK := r.uvarint()
		childCount := typeParamCount + paramCount
		if hasVariadic {
			childCount++
		}
		if !variadicOK || !returnsOK || returnCount > uint64(maxInt()) || childCount > uint64(len(children)) ||
			returnCount != uint64(len(children))-childCount || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: function child shape", ErrInvalidCanonicalType)
		}
		at := 0
		typeParams := make([]*TypeParam, int(typeParamCount))
		for index := range typeParams {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			param, valid := children[at].(*TypeParam)
			if !valid || param == nil {
				return nil, fmt.Errorf("%w: function type parameter", ErrInvalidCanonicalType)
			}
			typeParams[index], at = param, at+1
		}
		params := make([]Param, int(paramCount))
		for index := range params {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			name := ""
			if receiver[index] {
				name = "self"
			}
			params[index] = Param{Name: name, Type: children[at], Optional: optional[index], Receiver: receiver[index]}
			at++
		}
		var variadic Type
		if hasVariadic {
			variadic, at = children[at], at+1
		}
		returns := make([]Type, len(children)-at)
		for index := range returns {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			returns[index] = children[at+index]
		}
		return RebuildFunction(FunctionParts{TypeParams: typeParams, Params: params, Variadic: variadic, Returns: returns}), nil
	case canonicalGeneric:
		name, nameOK := r.frame()
		paramCount, paramsOK := r.uvarint()
		hasBody, bodyOK := r.bool()
		childCount := paramCount
		if hasBody {
			childCount++
		}
		if !nameOK || !paramsOK || !bodyOK || paramCount > uint64(maxInt()) || paramCount > uint64(len(children)) || childCount != uint64(len(children)) || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: generic shape", ErrInvalidCanonicalType)
		}
		params := make([]*TypeParam, int(paramCount))
		for index := range params {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			param, valid := children[index].(*TypeParam)
			if !valid || param == nil {
				return nil, fmt.Errorf("%w: generic type parameter", ErrInvalidCanonicalType)
			}
			params[index] = param
		}
		var body Type
		if hasBody {
			body = children[len(children)-1]
		}
		return NewGeneric(string(name), params, body), nil
	case canonicalInstantiated:
		argCount, ok := r.uvarint()
		if !ok || argCount > uint64(maxInt()) || argCount+1 != uint64(len(children)) || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: instantiated shape", ErrInvalidCanonicalType)
		}
		generic, valid := children[0].(*Generic)
		if !valid || generic == nil {
			return nil, fmt.Errorf("%w: instantiated generic", ErrInvalidCanonicalType)
		}
		return Instantiate(generic, children[1:]...), nil
	case canonicalTypeParam:
		name, nameOK := r.frame()
		hasConstraint, constraintOK := r.bool()
		want := 0
		if hasConstraint {
			want = 1
		}
		if !nameOK || !constraintOK || len(children) != want || r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: type parameter", ErrInvalidCanonicalType)
		}
		var constraint Type
		if hasConstraint {
			constraint = children[0]
		}
		return NewTypeParam(string(name), constraint), nil
	case canonicalRecursive:
		return nil, ErrCanonicalRecursiveIdentityUnavailable
	case canonicalInterface:
		name, nameOK := r.frame()
		methodCount, methodsOK := r.uvarint()
		if !nameOK || !methodsOK || methodCount > uint64(maxInt()) || methodCount != uint64(len(children)) {
			return nil, fmt.Errorf("%w: interface shape", ErrInvalidCanonicalType)
		}
		methods := make([]Method, int(methodCount))
		for index := range methods {
			if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
				return nil, err
			}
			methodName, ok := r.frame()
			fn, valid := children[index].(*Function)
			if !ok || !valid || fn == nil {
				return nil, fmt.Errorf("%w: interface method", ErrInvalidCanonicalType)
			}
			methods[index] = Method{Name: string(methodName), Type: fn}
		}
		if r.at != len(r.raw) {
			return nil, fmt.Errorf("%w: interface scalar", ErrInvalidCanonicalType)
		}
		return NewInterface(string(name), methods), nil
	case canonicalMeta:
		if err := wantChildren(1); err != nil {
			return nil, err
		}
		if err := finish(); err != nil {
			return nil, err
		}
		return NewMeta(children[0]), nil
	default:
		return nil, fmt.Errorf("%w: unknown scalar tag %d", ErrInvalidCanonicalType, tag)
	}
}

type canonicalRawReader struct {
	raw []byte
	at  int
}

func (r *canonicalRawReader) byte() (byte, bool) {
	if r == nil || r.at >= len(r.raw) {
		return 0, false
	}
	value := r.raw[r.at]
	r.at++
	return value, true
}

func (r *canonicalRawReader) bool() (bool, bool) {
	value, ok := r.byte()
	return value == 1, ok && value <= 1
}

func (r *canonicalRawReader) uvarint() (uint64, bool) {
	if r == nil || r.at >= len(r.raw) {
		return 0, false
	}
	start := r.at
	value, used := binary.Uvarint(r.raw[start:])
	if used <= 0 {
		return 0, false
	}
	var canonical [binary.MaxVarintLen64]byte
	canonicalLength := binary.PutUvarint(canonical[:], value)
	if canonicalLength != used || !bytes.Equal(canonical[:canonicalLength], r.raw[start:start+used]) {
		return 0, false
	}
	r.at += used
	return value, true
}

func (r *canonicalRawReader) varint() (int64, bool) {
	if r == nil || r.at >= len(r.raw) {
		return 0, false
	}
	start := r.at
	value, used := binary.Varint(r.raw[start:])
	if used <= 0 {
		return 0, false
	}
	var canonical [binary.MaxVarintLen64]byte
	canonicalLength := binary.PutVarint(canonical[:], value)
	if canonicalLength != used || !bytes.Equal(canonical[:canonicalLength], r.raw[start:start+used]) {
		return 0, false
	}
	r.at += used
	return value, true
}

func (r *canonicalRawReader) fixed64() (uint64, bool) {
	if r == nil || len(r.raw)-r.at < 8 {
		return 0, false
	}
	value := binary.BigEndian.Uint64(r.raw[r.at : r.at+8])
	r.at += 8
	return value, true
}

func (r *canonicalRawReader) frame() ([]byte, bool) {
	length, ok := r.uvarint()
	if !ok || length > uint64(len(r.raw)-r.at) {
		return nil, false
	}
	value := r.raw[r.at : r.at+int(length)]
	r.at += int(length)
	return value, true
}

func canonicalDecodeCheckpoint(ctx context.Context, steps *uint64) error {
	*steps++
	if *steps == 1 || *steps&63 == 0 {
		return ctx.Err()
	}
	return nil
}

func maxInt() int { return int(^uint(0) >> 1) }
