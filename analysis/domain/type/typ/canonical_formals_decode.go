package typ

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/internal/hash"
)

// DecodeCanonicalFormals reconstructs a scoped canonical graph using exactly
// the caller-owned external formal authority. External formal ordinals resolve
// to the supplied pointers; nested Function and Generic formals are recreated
// as fresh lexical binders. The wire format deliberately carries no
// presentation names for formals, so decoded local names are private,
// deterministic construction labels only and never semantic wire identity.
//
// This is a cold artifact/manifest boundary. It allocates the reconstructed
// semantic graph once; hot analysis identities remain the caller's existing
// dense authority handles rather than persistent local decoder handles.
//
// The decoder accepts only the unique bytes emitted by
// EncodeCanonicalFormals. It rejects malformed framing, out-of-scope external
// ordinals, malformed lexical ownership, cancellation, and any reconstruction
// whose scoped re-encoding differs byte-for-byte.
func DecodeCanonicalFormals(ctx context.Context, encoded []byte, formals []*TypeParam) (decoded Type, err error) {
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
	admission, err := newCanonicalFormalsAdmission(ctx, len(encoded))
	if err != nil {
		return nil, err
	}
	if err := validateCanonicalFormalAuthority(ctx, admission, formals); err != nil {
		return nil, err
	}
	nodes, shapes, err := validatedCanonicalFormalsGraph(ctx, encoded, len(formals), admission)
	if err != nil {
		return nil, err
	}
	decoded, err = materializeCanonicalFormalsGraph(ctx, admission, nodes, shapes, formals)
	if err != nil {
		return nil, err
	}
	if err := ValidateStaticGenericRecurrenceWithFormals(decoded, formals); err != nil {
		return nil, fmt.Errorf("%w: static generic recurrence: %v", ErrInvalidCanonicalType, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	roundTrip, err := encodeCanonicalFormalsAdmission(ctx, decoded, formals, admission)
	if err != nil {
		return nil, fmt.Errorf("%w: reconstructed value cannot encode: %v", ErrInvalidCanonicalType, err)
	}
	equal, equalErr := canonicalFormalsEqual(ctx, admission, roundTrip, encoded)
	if equalErr != nil {
		return nil, equalErr
	}
	if !equal {
		at, differenceErr := firstCanonicalByteDifference(ctx, admission, encoded, roundTrip)
		if differenceErr != nil {
			return nil, differenceErr
		}
		return nil, fmt.Errorf("%w: reconstructed value changed canonical bytes at %d", ErrInvalidCanonicalType, at)
	}
	return decoded, nil
}

func firstCanonicalByteDifference(ctx context.Context, admission *canonicalFormalsAdmission, left, right []byte) (int, error) {
	var steps uint64
	index := 0
	for index < len(left) && index < len(right) && left[index] == right[index] {
		if err := canonicalFormalsCheckpoint(ctx, admission, &steps); err != nil {
			return 0, err
		}
		index++
	}
	return index, nil
}

func validateCanonicalFormalAuthority(ctx context.Context, admission *canonicalFormalsAdmission, formals []*TypeParam) error {
	var steps uint64
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(formals), canonicalFormalsMapEntryBytes); err != nil {
		return err
	}
	seen := make(map[*TypeParam]struct{}, len(formals))
	for ordinal, formal := range formals {
		if err := canonicalFormalsCheckpoint(ctx, admission, &steps); err != nil {
			return err
		}
		if formal == nil {
			return fmt.Errorf("%w: nil external formal at ordinal %d", ErrInvalidCanonicalType, ordinal)
		}
		if _, duplicate := seen[formal]; duplicate {
			return fmt.Errorf("%w: duplicate external formal at ordinal %d", ErrInvalidCanonicalType, ordinal)
		}
		seen[formal] = struct{}{}
	}
	return nil
}

// materializeCanonicalFormalsGraph uses the graph and relation checker shared
// with ValidateCanonicalFormals. The explicit dependency walk has two
// deliberate exceptions: a local TypeParam's owner edge is lexical evidence,
// not a runtime child, and an external TypeParam resolves directly to its
// authority pointer. Those exceptions make binder identity available without
// manufacturing a second type representation or recursive Go calls.
func materializeCanonicalFormalsGraph(ctx context.Context, admission *canonicalFormalsAdmission, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape, formals []*TypeParam) (Type, error) {
	if len(nodes) == 0 || len(nodes) != len(shapes) {
		return nil, invalidCanonicalFormals("graph shape")
	}
	if err := canonicalFormalsPreflight(ctx, admission, nil, len(nodes), 96); err != nil {
		return nil, err
	}
	var steps uint64
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsTypeBytes); err != nil {
		return nil, err
	}
	built := make([]Type, len(nodes))
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsBoolBytes); err != nil {
		return nil, err
	}
	ready := make([]bool, len(nodes))
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsBoolBytes); err != nil {
		return nil, err
	}
	recursive := make([]bool, len(nodes))
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsBoolBytes); err != nil {
		return nil, err
	}
	generic := make([]bool, len(nodes))
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsBoolBytes); err != nil {
		return nil, err
	}
	genericFinalized := make([]bool, len(nodes))
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsIntBytes); err != nil {
		return nil, err
	}
	genericBodies := make([]int, len(nodes))
	for index := range genericBodies {
		if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
			return nil, err
		}
		genericBodies[index] = -1
	}

	// Recursive identities are the one explicitly sanctioned cyclic type node.
	// Allocate them before visiting ordinary dependencies; SetBody happens only
	// after the full graph has been built.
	for index, shape := range shapes {
		if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
			return nil, err
		}
		if shape.tag != canonicalRecursive {
			continue
		}
		hasBody, err := canonicalScopedRecursiveHeader(nodes[index].scalar)
		if err != nil {
			return nil, err
		}
		if len(nodes[index].edges) != boolChildCount(hasBody) {
			return nil, invalidCanonicalFormals("recursive child shape")
		}
		built[index] = NewRecursivePlaceholder("")
		ready[index], recursive[index] = true, true
	}

	// External parameters are already declaration identities. Their optional
	// constraint child remains part of the verified wire graph, but is not a
	// child of the receiver-owned TypeParam object; byte-equal re-encoding below
	// proves that the supplied authority has the exact compatible constraint.
	for index, shape := range shapes {
		if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
			return nil, err
		}
		if shape.tag != canonicalTypeParam || shape.formalMode != canonicalScopedExternalFormal {
			continue
		}
		if shape.formalOrdinal >= uint64(len(formals)) {
			return nil, invalidCanonicalFormals("external formal ordinal")
		}
		built[index], ready[index] = formals[shape.formalOrdinal], true
	}

	// Reverse dependency slices make ordinary reconstruction linear in graph
	// size. A repeated whole-graph readiness scan looks harmless on fixture
	// schemas but is quadratic for a 100k nested annotation chain.
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsIntBytes); err != nil {
		return nil, err
	}
	waiting := make([]int, len(nodes))
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsFrameBytes); err != nil {
		return nil, err
	}
	dependents := make([][]int, len(nodes))
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsIntBytes); err != nil {
		return nil, err
	}
	genericParamWaiting := make([]int, len(nodes))
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsFrameBytes); err != nil {
		return nil, err
	}
	genericParamDependents := make([][]int, len(nodes))
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsFrameBytes); err != nil {
		return nil, err
	}
	genericBodyWaiters := make([][]int, len(nodes))
	genericComponents, _, err := canonicalFormalGenericComponents(ctx, admission, nodes, shapes)
	if err != nil {
		return nil, err
	}
	var appendErr error
	for index := range nodes {
		if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
			return nil, err
		}
		if ready[index] {
			continue
		}
		for _, child := range canonicalFormalDependencies(index, nodes, shapes) {
			if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
				return nil, err
			}
			if child < 0 || child >= len(nodes) {
				return nil, invalidCanonicalFormals("edge outside graph")
			}
			if !ready[child] {
				waiting[index]++
				dependents[child], appendErr = canonicalFormalsAppend(ctx, admission, &steps, dependents[child], index, canonicalFormalsIntBytes)
				if appendErr != nil {
					return nil, appendErr
				}
			}
		}
	}
	for index, shape := range shapes {
		if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
			return nil, err
		}
		if ready[index] || shape.tag != canonicalGeneric {
			continue
		}
		for ordinal := 0; ordinal < int(shape.binderParams); ordinal++ {
			if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
				return nil, err
			}
			formal := nodes[index].edges[ordinal]
			if formal < 0 || formal >= len(nodes) {
				return nil, invalidCanonicalFormals("generic formal edge")
			}
			if !ready[formal] {
				genericParamWaiting[index]++
				genericParamDependents[formal], appendErr = canonicalFormalsAppend(ctx, admission, &steps, genericParamDependents[formal], index, canonicalFormalsIntBytes)
				if appendErr != nil {
					return nil, appendErr
				}
			}
		}
	}
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsIntBytes); err != nil {
		return nil, err
	}
	queue := make([]int, 0, len(nodes))
	if err := canonicalFormalsPreflight(ctx, admission, &steps, len(nodes), canonicalFormalsIntBytes); err != nil {
		return nil, err
	}
	genericQueue := make([]int, 0, len(nodes))
	for index := range nodes {
		if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
			return nil, err
		}
		if !ready[index] && waiting[index] == 0 {
			queue, appendErr = canonicalFormalsAppend(ctx, admission, &steps, queue, index, canonicalFormalsIntBytes)
			if appendErr != nil {
				return nil, appendErr
			}
		}
	}
	for index, shape := range shapes {
		if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
			return nil, err
		}
		if !ready[index] && shape.tag == canonicalGeneric && genericParamWaiting[index] == 0 {
			genericQueue, appendErr = canonicalFormalsAppend(ctx, admission, &steps, genericQueue, index, canonicalFormalsIntBytes)
			if appendErr != nil {
				return nil, appendErr
			}
		}
	}
	publish := func(index int, provisionalGeneric bool) error {
		for _, parent := range dependents[index] {
			if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
				return err
			}
			if provisionalGeneric && genericComponents[parent] != genericComponents[index] {
				continue
			}
			waiting[parent]--
			if waiting[parent] == 0 && !ready[parent] {
				queue, appendErr = canonicalFormalsAppend(ctx, admission, &steps, queue, parent, canonicalFormalsIntBytes)
				if appendErr != nil {
					return appendErr
				}
			}
		}
		for _, owner := range genericParamDependents[index] {
			if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
				return err
			}
			genericParamWaiting[owner]--
			if genericParamWaiting[owner] == 0 && !ready[owner] {
				genericQueue, appendErr = canonicalFormalsAppend(ctx, admission, &steps, genericQueue, owner, canonicalFormalsIntBytes)
				if appendErr != nil {
					return appendErr
				}
			}
		}
		return nil
	}
	finalizeGeneric := func(index int) error {
		if !generic[index] || genericFinalized[index] {
			return nil
		}
		bodyIndex := genericBodies[index]
		if bodyIndex < 0 || bodyIndex >= len(built) || !ready[bodyIndex] || built[bodyIndex] == nil {
			return invalidCanonicalFormals("generic body")
		}
		if err := canonicalOwnedSetGenericBody(ctx, admission, &steps, built[index].(*Generic), built[bodyIndex]); err != nil {
			return err
		}
		genericFinalized[index] = true
		return publish(index, false)
	}
	for {
		for head := 0; head < len(queue); head++ {
			if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
				return nil, err
			}
			index := queue[head]
			if ready[index] {
				continue
			}
			children, available, err := canonicalFormalChildren(ctx, admission, &steps, index, nodes, shapes, built, ready)
			if err != nil {
				return nil, err
			}
			if !available {
				return nil, invalidCanonicalFormals("dependency order")
			}
			value, err := materializeCanonicalFormalNode(ctx, admission, nodes[index].scalar, shapes[index], children, &steps, index)
			if err != nil {
				return nil, err
			}
			built[index], ready[index] = value, true
			if err := publish(index, false); err != nil {
				return nil, err
			}
			for _, owner := range genericBodyWaiters[index] {
				if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
					return nil, err
				}
				if err := finalizeGeneric(owner); err != nil {
					return nil, err
				}
			}
		}
		queue = queue[:0]
		opened := false
		for head := 0; head < len(genericQueue); head++ {
			if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
				return nil, err
			}
			index := genericQueue[head]
			if ready[index] || genericParamWaiting[index] != 0 {
				continue
			}
			bodyEdge := int(shapes[index].binderParams)
			if bodyEdge >= len(nodes[index].edges) || genericComponents[nodes[index].edges[bodyEdge]] != genericComponents[index] {
				// This body depends on another declaration's recurrence. Its
				// placeholder is intentionally invisible here; the owning Generic
				// will publish only after SetBody finalizes its semantic fields.
				continue
			}
			// A fully ready Generic was already handled by the ordinary queue.
			// Opening only an unresolved body is what preserves eager hashes for
			// acyclic declarations while making a true generic recurrence finite.
			if waiting[index] == 0 {
				// A prior opened Generic may have satisfied this candidate's body
				// during the same stalled phase. publish already placed it on the
				// ordinary queue; leave materialization there so its final body is
				// present before hash-bearing construction.
				continue
			}
			value, bodyIndex, err := openCanonicalFormalGeneric(ctx, admission, &steps, index, nodes, shapes, built, ready)
			if err != nil {
				return nil, err
			}
			built[index], ready[index], generic[index], genericBodies[index] = value, true, true, bodyIndex
			opened = true
			if err := publish(index, true); err != nil {
				return nil, err
			}
			if bodyIndex == index {
				if err := finalizeGeneric(index); err != nil {
					return nil, err
				}
			} else if bodyIndex >= 0 {
				genericBodyWaiters[bodyIndex], appendErr = canonicalFormalsAppend(ctx, admission, &steps, genericBodyWaiters[bodyIndex], index, canonicalFormalsIntBytes)
				if appendErr != nil {
					return nil, appendErr
				}
			} else {
				return nil, invalidCanonicalFormals("generic body")
			}
		}
		genericQueue = genericQueue[:0]
		if !opened && len(queue) == 0 {
			break
		}
	}

	for index := range nodes {
		if err := canonicalDecodeCheckpoint(ctx, &steps); err != nil {
			return nil, err
		}
		if !ready[index] {
			return nil, fmt.Errorf("%w: node %d", ErrCanonicalRecursiveIdentityUnavailable, index)
		}
		if generic[index] && !genericFinalized[index] {
			return nil, invalidCanonicalFormals("unfinalized generic")
		}
		if recursive[index] && len(nodes[index].edges) != 0 {
			body := built[nodes[index].edges[0]]
			if body == nil {
				return nil, invalidCanonicalFormals("recursive body")
			}
			built[index].(*Recursive).SetBody(body)
		}
	}
	return built[0], nil
}

// canonicalFormalChildren converts graph edges to materialization children.
// A local formal's first edge is its lexical owner, which validates scope but
// is not a semantic child of TypeParam. External formals are authority values,
// so their optional encoded constraint is verified/re-encoded rather than
// reconstructed into a competing pointer identity.
func canonicalFormalChildren(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, index int, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape, built []Type, ready []bool) ([]Type, bool, error) {
	shape := shapes[index]
	if shape.tag == canonicalTypeParam {
		switch shape.formalMode {
		case canonicalScopedExternalFormal:
			return nil, true, nil
		case canonicalScopedLocalFormal:
			if len(nodes[index].edges) == 1 {
				return nil, true, nil
			}
			constraint := nodes[index].edges[1]
			if constraint < 0 || constraint >= len(built) || !ready[constraint] {
				return nil, false, nil
			}
			return []Type{built[constraint]}, true, nil
		}
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, len(nodes[index].edges), canonicalFormalsTypeBytes); err != nil {
		return nil, false, err
	}
	children := make([]Type, len(nodes[index].edges))
	for position, child := range nodes[index].edges {
		if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
			return nil, false, err
		}
		if child < 0 || child >= len(built) || !ready[child] {
			return nil, false, nil
		}
		children[position] = built[child]
	}
	return children, true, nil
}

func canonicalFormalDependencies(index int, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape) []int {
	shape := shapes[index]
	if shape.tag != canonicalTypeParam {
		return nodes[index].edges
	}
	switch shape.formalMode {
	case canonicalScopedExternalFormal:
		return nil
	case canonicalScopedLocalFormal:
		if len(nodes[index].edges) == 2 {
			return nodes[index].edges[1:]
		}
		return nil
	default:
		return nil
	}
}

func materializeCanonicalFormalNode(ctx context.Context, admission *canonicalFormalsAdmission, scalar []byte, shape canonicalFormalNodeShape, children []Type, steps *uint64, index int) (Type, error) {
	if shape.tag == canonicalTypeParam {
		if shape.formalMode != canonicalScopedLocalFormal {
			return nil, invalidCanonicalFormals("formal mode")
		}
		var constraint Type
		if len(children) == 1 {
			constraint = children[0]
		} else if len(children) != 0 {
			return nil, invalidCanonicalFormals("local formal child count")
		}
		return NewTypeParam(canonicalDecodedLocalFormalName(index), constraint), nil
	}
	if shape.tag == canonicalOptional {
		if len(children) != 1 || len(scalar) != 1 {
			return nil, invalidCanonicalFormals("optional child shape")
		}
		return canonicalScopedOptional(children[0]), nil
	}
	switch shape.tag {
	case canonicalTuple, canonicalUnion, canonicalIntersection, canonicalRecord, canonicalFunction, canonicalGeneric, canonicalInstantiated, canonicalInterface:
		return materializeCanonicalFormalsVariableNode(ctx, admission, scalar, shape, children, steps)
	}
	return materializeCanonicalNode(ctx, scalar, children, steps)
}

// canonicalScopedOptional retains the published graph edge verbatim. The
// ordinary Optional constructor intentionally collapses nested optionals for
// source construction, but a scoped artifact can faithfully contain a deep
// already-published chain. Collapsing it here would make a valid 100k-node
// graph decode to one node and fail the mandatory byte-equality proof.
func canonicalScopedOptional(inner Type) Type {
	h := uint64(kind.Optional)
	if inner != nil {
		h = hash.MixHash(h, inner.Hash())
	}
	return &Optional{
		Inner:          inner,
		hash:           h,
		typeProperties: typePropertiesOf(inner),
	}
}

func openCanonicalFormalGeneric(ctx context.Context, admission *canonicalFormalsAdmission, steps *uint64, index int, nodes []canonicalTypeNode, shapes []canonicalFormalNodeShape, built []Type, ready []bool) (*Generic, int, error) {
	shape := shapes[index]
	if shape.tag != canonicalGeneric {
		return nil, -1, invalidCanonicalFormals("generic tag")
	}
	name, paramCount, hasBody, err := canonicalGenericHeader(nodes[index].scalar)
	if err != nil {
		return nil, -1, err
	}
	if uint64(paramCount) != shape.binderParams || len(nodes[index].edges) != paramCount+boolChildCount(hasBody) {
		return nil, -1, invalidCanonicalFormals("generic child shape")
	}
	if err := canonicalFormalsPreflight(ctx, admission, steps, paramCount, canonicalFormalsPointerBytes); err != nil {
		return nil, -1, err
	}
	params := make([]*TypeParam, paramCount)
	for ordinal := range params {
		if err := canonicalDecodeCheckpoint(ctx, steps); err != nil {
			return nil, -1, err
		}
		formalIndex := nodes[index].edges[ordinal]
		if formalIndex < 0 || formalIndex >= len(built) || !ready[formalIndex] {
			return nil, -1, invalidCanonicalFormals("generic type parameter")
		}
		param, valid := built[formalIndex].(*TypeParam)
		if !valid || param == nil {
			return nil, -1, invalidCanonicalFormals("generic type parameter")
		}
		params[ordinal] = param
	}
	bodyIndex := -1
	if hasBody {
		bodyIndex = nodes[index].edges[paramCount]
	}
	opened, err := canonicalOwnedGeneric(ctx, admission, steps, name, params, nil)
	if err != nil {
		return nil, -1, err
	}
	return opened, bodyIndex, nil
}

func canonicalScopedRecursiveHeader(scalar []byte) (bool, error) {
	reader := canonicalRawReader{raw: scalar}
	tag, ok := reader.byte()
	hasBody, bodyOK := reader.bool()
	if !ok || tag != canonicalRecursive || !bodyOK || reader.at != len(reader.raw) {
		return false, invalidCanonicalFormals("recursive shape")
	}
	return hasBody, nil
}

func canonicalDecodedLocalFormalName(index int) string {
	// This is intentionally a private construction label: scoped bytes replace
	// it with lexical (owner, ordinal) identity before any graph refinement.
	return fmt.Sprintf("\x00scoped-local-%d", index)
}

func boolChildCount(value bool) int {
	if value {
		return 1
	}
	return 0
}
