package typ

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"sort"
	"sync"
)

// CanonicalDigest is the full collision-resistant identity issued with one
// admitted canonical graph or scoped wire receipt.
type CanonicalDigest [sha256.Size]byte

const (
	// Version 2 makes numeric literal identity raw-IEEE-bit exact. Version 1
	// rejected NaNs and normalized signed zero, so accepting it under the new
	// equality relation would make old portable bytes ambiguous.
	canonicalScopedTypeDomain  = "wippy.analysis.type.typ.canonical-formals"
	canonicalScopedTypeVersion = uint64(2)
)

var canonicalEncoderPool = sync.Pool{
	New: func() any { return &canonicalEncoder{} },
}

// EncodeCanonicalFormals admits t through the canonical graph quotient and
// binds formals by their supplied ordinal rather than by presentation name.
// It is intended for portable authored schemas: a caller
// supplies the free parameters of t in semantic order; nested Function and
// Generic binders are discovered from t itself. Its scoped framing is the sole
// portable canonical byte domain.
//
// The returned receipt is the only trusted scoped-formal image. Callers that
// need a raw wire image must request an ownership-isolated copy from Bytes;
// receipt consumers must not pass a source graph back through a receipt to
// bypass its own graph admission.
func EncodeCanonicalFormals(ctx context.Context, t Type, formals []*TypeParam) (CanonicalFormalsReceipt, error) {
	encoder := canonicalEncoderPool.Get().(*canonicalEncoder)
	receipt, err := encoder.encodeFormals(ctx, t, formals)
	canonicalEncoderPool.Put(encoder)
	return receipt, err
}

// canonicalEncoder retains traversal scratch across calls. It is not safe for
// concurrent use.
type canonicalEncoder struct {
	nodes           []canonicalTypeNode
	seen            map[Type]int
	transparent     map[Type]bool
	recursiveID     map[uint64]*Recursive
	discoveryStack  []canonicalDiscoveryFrame
	classes         []int
	representatives []int
	out             []byte
	ordinals        map[int]uint64
	foldedClasses   map[string]int
	sccIndices      []int
	sccOf           []int
	sccLow          []int
	sccOnStack      []bool
	sccStack        []int
	sccMembers      []int
	sccStarts       []int
	sccFrames       []canonicalSCCFrame
	emissionStack   []canonicalEmissionFrame
	steps           uint64
	ctx             context.Context
	admission       *canonicalFormalsAdmission
	scoped          bool
	formals         map[*TypeParam]uint64
	formalScope     []*TypeParam
	binders         map[*TypeParam]canonicalFormalBinder
}

// encodeFormals is the whole scoped encode over one caller-owned encoder. It
// is the single implementation: EncodeCanonicalFormals supplies a pooled
// encoder to it. Like the pooled entry point it retains only scratch and is
// not safe for concurrent use.
func (e *canonicalEncoder) encodeFormals(ctx context.Context, t Type, formals []*TypeParam) (CanonicalFormalsReceipt, error) {
	admission, err := newCanonicalFormalsAdmission(ctx, 0)
	if err != nil {
		return CanonicalFormalsReceipt{}, err
	}
	if uint64(len(formals)) > uint64(^uint32(0)) {
		return CanonicalFormalsReceipt{}, invalidCanonicalFormals("external formal count")
	}
	encoded, err := e.encodeFormalsAdmission(ctx, t, formals, admission)
	if err != nil {
		return CanonicalFormalsReceipt{}, err
	}
	if err := admission.checkpoint(); err != nil {
		return CanonicalFormalsReceipt{}, err
	}
	return newCanonicalFormalsReceipt(encoded, uint32(len(formals))), nil
}

func (e *canonicalEncoder) encodeFormalsAdmission(ctx context.Context, t Type, formals []*TypeParam, admission *canonicalFormalsAdmission) ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("typ: nil canonical encoder")
	}
	e.reset(ctx, true, admission)
	defer e.release()
	if err := e.installFormals(formals); err != nil {
		e.abort()
		return nil, err
	}
	return e.encode(t)
}

func (e *canonicalEncoder) encode(t Type) ([]byte, error) {
	root, err := e.discover(t)
	if err != nil {
		e.abort()
		return nil, err
	}
	if e.scoped {
		if err := e.finalizeScopedTypeParams(); err != nil {
			e.abort()
			return nil, err
		}
	}
	if err := e.checkpoint(); err != nil {
		e.abort()
		return nil, err
	}
	if err := e.refine(); err != nil {
		e.abort()
		return nil, err
	}
	rootClass := e.classes[root]
	e.out = e.out[:0]
	if err := e.appendCanonicalOutputFrameString(canonicalScopedTypeDomain); err != nil {
		e.abort()
		return nil, err
	}
	if err := e.appendCanonicalOutputUvarint(canonicalScopedTypeVersion); err != nil {
		e.abort()
		return nil, err
	}
	if e.ordinals == nil {
		if err := e.reserve(len(e.nodes), canonicalFormalsMapEntryBytes); err != nil {
			e.abort()
			return nil, err
		}
		e.ordinals = make(map[int]uint64, len(e.nodes))
	}
	if err := e.emitClass(rootClass, e.ordinals); err != nil {
		e.abort()
		return nil, err
	}
	if e.scoped {
		if err := e.admitScopedForm(t); err != nil {
			e.abort()
			return nil, err
		}
	}
	var out []byte
	if e.admission != nil {
		out, err = canonicalFormalsClone(e.ctx, e.admission, e.out)
	} else {
		out = append([]byte(nil), e.out...)
	}
	if err != nil {
		e.abort()
		return nil, err
	}
	e.ctx = nil
	return out, nil
}

// admitScopedForm applies the scoped scope laws to the graph that produced
// e.out.
//
// The laws are applied after emission rather than before it because the emitted
// bytes are the form's complete identity, and emission is what produces them.
// Emission itself reads nothing the laws establish: it is the same bisimulation
// quotient and the same emitter the unscoped path already runs on an
// unvalidated discovered graph. A graph that violates a scope law emits bytes
// carrying that violation, so those bytes are never a form any validator
// admitted, and an already admitted byte string is the canonical spelling of a
// lawful graph bisimilar to this one.
func (e *canonicalEncoder) admitScopedForm(t Type) error {
	if err := validateCanonicalFormalNodeGraph(e.ctx, e.admission, e.nodes, uint64(len(e.formals))); err != nil {
		return err
	}
	if err := ValidateStaticGenericRecurrenceWithFormals(t, e.formalScope); err != nil {
		return fmt.Errorf("%w: static generic recurrence: %v", ErrInvalidCanonicalType, err)
	}
	return nil
}

type canonicalTypeNode struct {
	scalar    []byte
	edges     []int
	typeParam *TypeParam
	// source is construction-only provenance for owner-issued graph receipts.
	// It is cleared with encoder scratch after the receipt has copied the
	// quotient metadata.  Canonical bytes never depend on this field.
	source Type
}

type canonicalDiscoveryFrame struct {
	node     int
	children []Type
	next     int
}

// canonicalFormalBinder is one binder frame's claim on one parameter. A
// parameter installed in the external scope carries its external ordinal, so
// only binder-local parameters are read back from this claim.
type canonicalFormalBinder struct {
	owner   Type
	ordinal uint64
}

const (
	canonicalNil byte = iota + 1
	canonicalPrimitiveNil
	canonicalBoolean
	canonicalNumber
	canonicalInteger
	canonicalString
	canonicalAny
	canonicalUnknown
	canonicalNever
	canonicalSelf
	canonicalLiteral
	canonicalRef
	canonicalOptional
	canonicalUnion
	canonicalIntersection
	canonicalTuple
	canonicalArray
	canonicalMap
	canonicalReadonlyMap
	canonicalRecord
	canonicalFunction
	canonicalGeneric
	canonicalInstantiated
	canonicalTypeParam
	canonicalRecursive
	canonicalInterface
	canonicalMeta
)

const (
	canonicalScopedExternalFormal byte = iota + 1
	canonicalScopedLocalFormal
)

func (e *canonicalEncoder) reset(ctx context.Context, scoped bool, admission *canonicalFormalsAdmission) {
	e.clearCallState()
	e.ctx = ctx
	e.admission = admission
	e.steps = 0
	e.scoped = scoped
}

func (e *canonicalEncoder) abort() {
	e.clearCallState()
	e.steps = 0
	e.ctx = nil
	e.admission = nil
	e.scoped = false
}

// clearCallState clears every graph-bearing slot while retaining capacity for
// future calls. It is used by reset, abort, and release: a direct reusable
// encoder has the same non-retention guarantee as the package pool.
func (e *canonicalEncoder) clearCallState() {
	// Never clear an oversized caller-derived buffer: clear itself is an
	// uninterruptible linear walk.  Dropping the backing array releases every
	// caller reference without retaining it in the package pool.
	if canonicalFormalsCapacityExceeds(cap(e.out), 1, canonicalFormalsRetainBytes) {
		e.out = nil
	} else {
		e.out = e.out[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.nodes), canonicalFormalsNodeBytes, canonicalFormalsRetainBytes) {
		e.nodes = nil
	} else {
		clear(e.nodes)
		e.nodes = e.nodes[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.discoveryStack), canonicalFormalsFrameBytes, canonicalFormalsRetainBytes) {
		e.discoveryStack = nil
	} else {
		clear(e.discoveryStack)
		e.discoveryStack = e.discoveryStack[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.classes), canonicalFormalsIntBytes, canonicalFormalsRetainBytes) {
		e.classes = nil
	} else {
		e.classes = e.classes[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.representatives), canonicalFormalsIntBytes, canonicalFormalsRetainBytes) {
		e.representatives = nil
	} else {
		e.representatives = e.representatives[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.sccIndices), canonicalFormalsIntBytes, canonicalFormalsRetainBytes) {
		e.sccIndices = nil
	} else {
		e.sccIndices = e.sccIndices[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.sccOf), canonicalFormalsIntBytes, canonicalFormalsRetainBytes) {
		e.sccOf = nil
	} else {
		e.sccOf = e.sccOf[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.sccLow), canonicalFormalsIntBytes, canonicalFormalsRetainBytes) {
		e.sccLow = nil
	} else {
		e.sccLow = e.sccLow[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.sccOnStack), canonicalFormalsBoolBytes, canonicalFormalsRetainBytes) {
		e.sccOnStack = nil
	} else {
		e.sccOnStack = e.sccOnStack[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.sccStack), canonicalFormalsIntBytes, canonicalFormalsRetainBytes) {
		e.sccStack = nil
	} else {
		e.sccStack = e.sccStack[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.sccMembers), canonicalFormalsIntBytes, canonicalFormalsRetainBytes) {
		e.sccMembers = nil
	} else {
		e.sccMembers = e.sccMembers[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.sccStarts), canonicalFormalsIntBytes, canonicalFormalsRetainBytes) {
		e.sccStarts = nil
	} else {
		e.sccStarts = e.sccStarts[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.sccFrames), canonicalFormalsFrameBytes, canonicalFormalsRetainBytes) {
		e.sccFrames = nil
	} else {
		e.sccFrames = e.sccFrames[:0]
	}
	if canonicalFormalsCapacityExceeds(cap(e.emissionStack), canonicalFormalsFrameBytes, canonicalFormalsRetainBytes) {
		e.emissionStack = nil
	} else {
		e.emissionStack = e.emissionStack[:0]
	}
	// Map capacity is opaque.  Retaining it would let a one-off hostile graph
	// pin an arbitrary bucket table forever, so map scratch is never pooled.
	e.seen = nil
	e.transparent = nil
	e.recursiveID = nil
	e.formals = nil
	e.formalScope = nil
	e.binders = nil
	e.ordinals = nil
	e.foldedClasses = nil
}

func (e *canonicalEncoder) reserve(count, elementBytes int) error {
	return canonicalFormalsPreflight(e.ctx, e.admission, &e.steps, count, elementBytes)
}

func (e *canonicalEncoder) ensureDiscoveryMaps() error {
	if e.seen == nil {
		if err := e.reserve(16, canonicalFormalsMapEntryBytes); err != nil {
			return err
		}
		e.seen = make(map[Type]int, 16)
	}
	if e.transparent == nil {
		if err := e.reserve(16, canonicalFormalsMapEntryBytes); err != nil {
			return err
		}
		e.transparent = make(map[Type]bool, 16)
	}
	if e.recursiveID == nil {
		if err := e.reserve(16, canonicalFormalsMapEntryBytes); err != nil {
			return err
		}
		e.recursiveID = make(map[uint64]*Recursive, 16)
	}
	return nil
}

// release drops references to caller-owned type graphs before the reusable
// scratch is retained for a subsequent direct call or returned to the pool.
func (e *canonicalEncoder) release() {
	e.abort()
}

func (e *canonicalEncoder) checkpoint() error {
	if e.admission != nil {
		return e.admission.checkpoint()
	}
	e.steps++
	if e.ctx != nil && (e.steps == 1 || e.steps&63 == 0) {
		return e.ctx.Err()
	}
	return nil
}

// discover builds the entire Type graph with an explicit DFS stack. Canonical
// graph discovery may receive adversarially deep schemas, so no host call
// frame is proportional to graph depth.
func (e *canonicalEncoder) discover(input Type) (int, error) {
	if err := e.ensureDiscoveryMaps(); err != nil {
		return 0, err
	}
	root, fresh, children, err := e.discoverNode(input)
	if err != nil {
		return 0, err
	}
	if fresh && len(children) != 0 {
		var appendErr error
		e.discoveryStack, appendErr = canonicalFormalsAppend(e.ctx, e.admission, &e.steps, e.discoveryStack, canonicalDiscoveryFrame{node: root, children: children}, canonicalFormalsFrameBytes)
		if appendErr != nil {
			return 0, appendErr
		}
	}
	for len(e.discoveryStack) != 0 {
		frameIndex := len(e.discoveryStack) - 1
		frame := &e.discoveryStack[frameIndex]
		if frame.next == len(frame.children) {
			clear(frame.children)
			frame.children = nil
			e.discoveryStack = e.discoveryStack[:frameIndex]
			continue
		}
		child := frame.children[frame.next]
		frame.next++
		childIndex, childFresh, childChildren, childErr := e.discoverNode(child)
		if childErr != nil {
			return 0, childErr
		}
		var appendErr error
		e.nodes[frame.node].edges, appendErr = canonicalFormalsAppend(e.ctx, e.admission, &e.steps, e.nodes[frame.node].edges, childIndex, canonicalFormalsIntBytes)
		if appendErr != nil {
			return 0, appendErr
		}
		if childFresh && len(childChildren) != 0 {
			e.discoveryStack, appendErr = canonicalFormalsAppend(e.ctx, e.admission, &e.steps, e.discoveryStack, canonicalDiscoveryFrame{node: childIndex, children: childChildren}, canonicalFormalsFrameBytes)
			if appendErr != nil {
				return 0, appendErr
			}
		}
	}
	return root, nil
}

// discoverNode creates one graph node and returns its child slots. The caller
// owns scheduling those slots; inserting the node into seen before returning
// preserves cycles exactly as the former recursive traversal did.
func (e *canonicalEncoder) discoverNode(input Type) (int, bool, []Type, error) {
	if err := e.checkpoint(); err != nil {
		return 0, false, nil, err
	}
	t, err := e.unwrapTransparent(input)
	if err != nil {
		return 0, false, nil, err
	}
	if t == nil {
		index := len(e.nodes)
		var appendErr error
		e.nodes, appendErr = canonicalFormalsAppend(e.ctx, e.admission, &e.steps, e.nodes, canonicalTypeNode{scalar: []byte{canonicalNil}}, canonicalFormalsNodeBytes)
		if appendErr != nil {
			return 0, false, nil, appendErr
		}
		return index, true, nil, nil
	}
	if dynamic := reflect.TypeOf(t); dynamic == nil || !dynamic.Comparable() {
		return 0, false, nil, fmt.Errorf("typ: unsupported non-comparable type implementation %T", t)
	}
	if index, ok := e.seen[t]; ok {
		return index, false, nil, nil
	}

	index := len(e.nodes)
	if err := e.reserve(1, canonicalFormalsMapEntryBytes); err != nil {
		return 0, false, nil, err
	}
	e.seen[t] = index
	var appendErr error
	e.nodes, appendErr = canonicalFormalsAppend(e.ctx, e.admission, &e.steps, e.nodes, canonicalTypeNode{}, canonicalFormalsNodeBytes)
	if appendErr != nil {
		return 0, false, nil, appendErr
	}
	node := &e.nodes[index]
	node.source = t
	var children []Type

	switch value := t.(type) {
	case *nilType:
		node.scalar = []byte{canonicalPrimitiveNil}
	case *booleanType:
		node.scalar = []byte{canonicalBoolean}
	case *numberType:
		node.scalar = []byte{canonicalNumber}
	case *integerType:
		node.scalar = []byte{canonicalInteger}
	case *stringType:
		node.scalar = []byte{canonicalString}
	case *anyType:
		node.scalar = []byte{canonicalAny}
	case *unknownType:
		node.scalar = []byte{canonicalUnknown}
	case *neverType:
		node.scalar = []byte{canonicalNever}
	case *selfType:
		node.scalar = []byte{canonicalSelf}
	case *Literal:
		if size, sizeErr := canonicalLiteralScalarSize(value); sizeErr != nil {
			return 0, false, nil, sizeErr
		} else if reserveErr := e.reserve(size, 1); reserveErr != nil {
			return 0, false, nil, reserveErr
		}
		node.scalar, err = canonicalLiteralScalar(value)
	case *Ref:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Ref")
		}
		if err := e.reserve(1+canonicalFormalsUvarintSize(uint64(len(value.Module)))+len(value.Module)+canonicalFormalsUvarintSize(uint64(len(value.Name)))+len(value.Name), 1); err != nil {
			return 0, false, nil, err
		}
		node.scalar = appendFrameString([]byte{canonicalRef}, value.Module)
		node.scalar = appendFrameString(node.scalar, value.Name)
	case *Optional:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Optional")
		}
		node.scalar, children = []byte{canonicalOptional}, []Type{value.Inner}
	case *Union:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Union")
		}
		node.scalar = appendCount([]byte{canonicalUnion}, len(value.Members))
		children, err = e.appendCanonicalTypes(children, value.Members)
	case *Intersection:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Intersection")
		}
		node.scalar = appendCount([]byte{canonicalIntersection}, len(value.Members))
		children, err = e.appendCanonicalTypes(children, value.Members)
	case *Tuple:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Tuple")
		}
		node.scalar = appendCount([]byte{canonicalTuple}, len(value.Elements))
		children, err = e.appendCanonicalTypes(children, value.Elements)
	case *Array:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Array")
		}
		node.scalar, children = []byte{canonicalArray}, []Type{value.Element}
	case *Map:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Map")
		}
		node.scalar, children = []byte{canonicalMap}, []Type{value.Key, value.Value}
	case *ReadonlyMap:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil ReadonlyMap")
		}
		node.scalar, children = []byte{canonicalReadonlyMap}, []Type{value.Key, value.Value}
	case *Record:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Record")
		}
		node.scalar, children, err = e.canonicalRecordParts(value)
	case *Function:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Function")
		}
		if err := e.registerBinder(t, value.TypeParams); err != nil {
			return 0, false, nil, err
		}
		node.scalar, children, err = e.canonicalFunctionParts(value)
	case *Generic:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Generic")
		}
		if err := e.registerBinder(t, value.TypeParams); err != nil {
			return 0, false, nil, err
		}
		scalarBytes := 1 + canonicalFormalsUvarintSize(uint64(len(value.Name))) + len(value.Name) + canonicalFormalsUvarintSize(uint64(len(value.TypeParams))) + 1
		if err := e.reserve(scalarBytes, 1); err != nil {
			return 0, false, nil, err
		}
		node.scalar = make([]byte, 0, scalarBytes)
		node.scalar = append(node.scalar, canonicalGeneric)
		if !e.scoped {
			node.scalar = appendFrameString(node.scalar, value.Name)
		} else {
			// Generic declaration names identify the declaration. Unlike binder
			// parameter names, they are semantic in a portable ABI.
			node.scalar = appendFrameString(node.scalar, value.Name)
		}
		node.scalar = appendCount(node.scalar, len(value.TypeParams))
		childCapacity := len(value.TypeParams) + 1
		if err := e.reserve(childCapacity, canonicalFormalsTypeBytes); err != nil {
			return 0, false, nil, err
		}
		children = make([]Type, 0, childCapacity)
		for _, param := range value.TypeParams {
			if err := e.checkpoint(); err != nil {
				return 0, false, nil, err
			}
			children = append(children, param)
		}
		node.scalar = appendBool(node.scalar, value.Body != nil)
		if value.Body != nil {
			children = append(children, value.Body)
		}
	case *Instantiated:
		if value == nil || value.Generic == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Instantiated or generic")
		}
		if err := e.reserve(1+canonicalFormalsUvarintSize(uint64(len(value.TypeArgs))), 1); err != nil {
			return 0, false, nil, err
		}
		node.scalar = appendCount([]byte{canonicalInstantiated}, len(value.TypeArgs))
		if err := e.reserve(len(value.TypeArgs)+1, canonicalFormalsTypeBytes); err != nil {
			return 0, false, nil, err
		}
		children = make([]Type, 0, len(value.TypeArgs)+1)
		children = append(children, value.Generic)
		children, err = e.appendCanonicalTypes(children, value.TypeArgs)
	case *TypeParam:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil TypeParam")
		}
		node.typeParam = value
		if value.Constraint != nil {
			children = []Type{value.Constraint}
		}
		if !e.scoped {
			node.scalar = appendFrameString([]byte{canonicalTypeParam}, value.Name)
			node.scalar = appendBool(node.scalar, value.Constraint != nil)
		}
	case *Recursive:
		if value == nil || value.ID == 0 {
			return 0, false, nil, fmt.Errorf("typ: recursive node has no well-formed identity")
		}
		if prior, ok := e.recursiveID[value.ID]; ok && prior != value {
			return 0, false, nil, fmt.Errorf("typ: recursive ID %d names distinct nodes", value.ID)
		}
		e.recursiveID[value.ID] = value
		// A recursive binder is a bound variable: its name is presentation and
		// its occurrences are graph edges. Writing the name would make one
		// fixed point reached through two declarations encode as two types,
		// splitting an identity the graph quotient has already joined.
		node.scalar = []byte{canonicalRecursive}
		node.scalar = appendBool(node.scalar, value.Body != nil)
		if value.Body != nil {
			children = []Type{value.Body}
		}
	case *Interface:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Interface")
		}
		scalarBytes := 1 + canonicalFormalsUvarintSize(uint64(len(value.Name))) + len(value.Name) + canonicalFormalsUvarintSize(uint64(len(value.Methods)))
		for _, method := range value.Methods {
			if err := e.checkpoint(); err != nil {
				return 0, false, nil, err
			}
			scalarBytes += canonicalFormalsUvarintSize(uint64(len(method.Name))) + len(method.Name)
		}
		if err := e.reserve(scalarBytes, 1); err != nil {
			return 0, false, nil, err
		}
		if err := e.reserve(len(value.Methods), canonicalFormalsTypeBytes); err != nil {
			return 0, false, nil, err
		}
		node.scalar = make([]byte, 0, scalarBytes)
		node.scalar = append(node.scalar, canonicalInterface)
		node.scalar = appendFrameString(node.scalar, value.Name)
		node.scalar = appendCount(node.scalar, len(value.Methods))
		children = make([]Type, 0, len(value.Methods))
		for _, method := range value.Methods {
			if err := e.checkpoint(); err != nil {
				return 0, false, nil, err
			}
			node.scalar = appendFrameString(node.scalar, method.Name)
			children = append(children, method.Type)
		}
	case *Meta:
		if value == nil {
			return 0, false, nil, fmt.Errorf("typ: nil Meta")
		}
		node.scalar, children = []byte{canonicalMeta}, []Type{value.Of}
	default:
		return 0, false, nil, fmt.Errorf("typ: unsupported canonical type implementation %T", t)
	}
	if err != nil {
		return 0, false, nil, err
	}
	return index, true, children, nil
}

// installFormals installs the one caller-owned scope for a scoped encoding.
// A pointer is an identity only while encoding: no pointer-derived value is
// emitted, and release clears the map before this encoder can be reused.
func (e *canonicalEncoder) installFormals(formals []*TypeParam) error {
	e.formalScope = formals
	if e.formals == nil {
		if err := e.reserve(len(formals), canonicalFormalsMapEntryBytes); err != nil {
			return err
		}
		e.formals = make(map[*TypeParam]uint64, len(formals))
	}
	for ordinal, formal := range formals {
		if err := e.checkpoint(); err != nil {
			return err
		}
		if formal == nil {
			return fmt.Errorf("typ: nil canonical formal at ordinal %d", ordinal)
		}
		if _, exists := e.formals[formal]; exists {
			return fmt.Errorf("typ: duplicate canonical formal at ordinal %d", ordinal)
		}
		if err := e.reserve(1, canonicalFormalsMapEntryBytes); err != nil {
			return err
		}
		e.formals[formal] = uint64(ordinal)
	}
	return nil
}

// registerBinder records the lexical owner of every nested formal before its
// children are explored. The value node is later wired back to this owner in
// finalizeScopedTypeParams, making outer-vs-inner references explicit to the
// graph quotient without ever serializing a Go pointer or generated ID.
//
// A scoped root is one cut of a declaration graph, so the declaration owning
// the installed external scope is reachable again from inside its own body.
// That binder re-enters the external scope instead of introducing parameters,
// so its parameters keep their external ordinals and the claim recorded here
// is read back only for a binder-local parameter.
func (e *canonicalEncoder) registerBinder(owner Type, params []*TypeParam) error {
	if !e.scoped {
		return nil
	}
	if e.binders == nil {
		if err := e.reserve(len(params), canonicalFormalsMapEntryBytes); err != nil {
			return err
		}
		e.binders = make(map[*TypeParam]canonicalFormalBinder, len(params))
	}
	if err := e.externalScopeBinder(params); err != nil {
		return err
	}
	for ordinal, param := range params {
		if err := e.checkpoint(); err != nil {
			return err
		}
		if prior, exists := e.binders[param]; exists {
			if prior.owner == owner {
				return fmt.Errorf("typ: duplicate local canonical formal at ordinal %d", ordinal)
			}
			return fmt.Errorf("typ: canonical formal %q is owned by multiple binders", param.Name)
		}
		if err := e.reserve(1, canonicalFormalsMapEntryBytes); err != nil {
			return err
		}
		e.binders[param] = canonicalFormalBinder{owner: owner, ordinal: uint64(ordinal)}
	}
	return nil
}

// externalScopeBinder admits one binder frame against the installed external
// scope. Every parameter installed externally makes the frame the owner of that
// scope, re-entered through a cycle in the cut graph; no parameter installed
// externally makes it an ordinary introducing frame. A frame drawn from both
// would leave a parameter without a single owner. A frame is classified only
// once each of its parameters is a parameter, so an absent one is reported as
// itself rather than as the mixture it produces.
func (e *canonicalEncoder) externalScopeBinder(params []*TypeParam) error {
	installed := 0
	for ordinal, param := range params {
		if err := e.checkpoint(); err != nil {
			return err
		}
		if param == nil {
			return fmt.Errorf("typ: nil local canonical formal at ordinal %d", ordinal)
		}
		if _, external := e.formals[param]; external {
			installed++
		}
	}
	if installed != 0 && installed != len(params) {
		return fmt.Errorf("typ: canonical binder mixes external and locally bound formals")
	}
	return nil
}

// finalizeScopedTypeParams replaces presentation-bearing TypeParam scalars
// after discovery has found every reachable binder. Owner edges make lexical
// scope structural: an inner binder's ordinal zero cannot be mistaken for an
// outer binder's ordinal zero by bisimulation.
func (e *canonicalEncoder) finalizeScopedTypeParams() error {
	for index := range e.nodes {
		if err := e.checkpoint(); err != nil {
			return err
		}
		node := &e.nodes[index]
		param := node.typeParam
		if param == nil {
			continue
		}
		if len(node.edges) > 1 {
			return fmt.Errorf("typ: malformed canonical TypeParam constraint graph")
		}
		if externalOrdinal, external := e.formals[param]; external {
			scalarBytes := 3 + canonicalFormalsUvarintSize(externalOrdinal)
			if err := e.reserve(scalarBytes, 1); err != nil {
				return err
			}
			node.scalar = make([]byte, 0, scalarBytes)
			node.scalar = append(node.scalar, canonicalTypeParam)
			node.scalar = append(node.scalar, canonicalScopedExternalFormal)
			node.scalar = binary.AppendUvarint(node.scalar, externalOrdinal)
			node.scalar = appendBool(node.scalar, param.Constraint != nil)
			continue
		}
		binding, owned := e.binders[param]
		if !owned {
			return fmt.Errorf("typ: canonical TypeParam %q is neither external nor locally bound", param.Name)
		}
		ownerIndex, reachable := e.seen[binding.owner]
		if !reachable {
			return fmt.Errorf("typ: canonical TypeParam %q has unreachable binder", param.Name)
		}
		scalarBytes := 3 + canonicalFormalsUvarintSize(binding.ordinal)
		if err := e.reserve(scalarBytes, 1); err != nil {
			return err
		}
		node.scalar = make([]byte, 0, scalarBytes)
		node.scalar = append(node.scalar, canonicalTypeParam)
		node.scalar = append(node.scalar, canonicalScopedLocalFormal)
		node.scalar = binary.AppendUvarint(node.scalar, binding.ordinal)
		node.scalar = appendBool(node.scalar, param.Constraint != nil)
		constraint := node.edges
		if err := e.reserve(len(constraint)+1, canonicalFormalsIntBytes); err != nil {
			return err
		}
		node.edges = make([]int, 0, len(constraint)+1)
		node.edges = append(node.edges, ownerIndex)
		node.edges = append(node.edges, constraint...)
	}
	return nil
}

func (e *canonicalEncoder) unwrapTransparent(t Type) (Type, error) {
	var path []Type
	defer func() {
		for _, wrapper := range path {
			delete(e.transparent, wrapper)
		}
	}()
	for {
		if err := e.checkpoint(); err != nil {
			return nil, err
		}
		if isTypedNilCanonicalType(t) {
			return nil, fmt.Errorf("typ: typed nil canonical type %T", t)
		}
		t = NormalizeNil(t)
		if t == nil {
			return nil, nil
		}
		switch value := t.(type) {
		case *Annotated:
			if value == nil || value.Inner == nil || value.Inner == t {
				return nil, fmt.Errorf("typ: malformed transparent annotation")
			}
			if e.transparent[t] {
				return nil, fmt.Errorf("typ: cyclic transparent wrapper")
			}
			e.transparent[t] = true
			var appendErr error
			path, appendErr = canonicalFormalsAppend(e.ctx, e.admission, &e.steps, path, t, canonicalFormalsTypeBytes)
			if appendErr != nil {
				return nil, appendErr
			}
			t = value.Inner
		case *Alias:
			if value == nil {
				return nil, fmt.Errorf("typ: nil Alias")
			}
			if e.transparent[t] {
				return nil, fmt.Errorf("typ: cyclic alias")
			}
			e.transparent[t] = true
			var appendErr error
			path, appendErr = canonicalFormalsAppend(e.ctx, e.admission, &e.steps, path, t, canonicalFormalsTypeBytes)
			if appendErr != nil {
				return nil, appendErr
			}
			t = value.UnaliasedTarget()
		default:
			return t, nil
		}
	}
}

func isTypedNilCanonicalType(t Type) bool {
	if t == nil {
		return false
	}
	v := reflect.ValueOf(t)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func canonicalLiteralScalar(literal *Literal) ([]byte, error) {
	if literal == nil {
		return nil, fmt.Errorf("typ: nil Literal")
	}
	out := []byte{canonicalLiteral, byte(literal.base)}
	switch literal.base {
	case Boolean.Kind():
		value, ok := literal.value.(bool)
		if !ok {
			return nil, fmt.Errorf("typ: malformed boolean literal payload %T", literal.value)
		}
		return appendBool(out, value), nil
	case Integer.Kind():
		value, ok := literal.value.(int64)
		if !ok {
			return nil, fmt.Errorf("typ: malformed integer literal payload %T", literal.value)
		}
		return binary.AppendVarint(out, value), nil
	case Number.Kind():
		value, ok := literal.value.(float64)
		if !ok {
			return nil, fmt.Errorf("typ: malformed number literal payload %T", literal.value)
		}
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], math.Float64bits(value))
		return append(out, encoded[:]...), nil
	case String.Kind():
		value, ok := literal.value.(string)
		if !ok {
			return nil, fmt.Errorf("typ: malformed string literal payload %T", literal.value)
		}
		return appendFrameString(out, value), nil
	default:
		return nil, fmt.Errorf("typ: unsupported literal base %d", literal.base)
	}
}

func canonicalLiteralScalarSize(literal *Literal) (int, error) {
	if literal == nil {
		return 0, fmt.Errorf("typ: nil Literal")
	}
	switch literal.base {
	case Boolean.Kind():
		if _, ok := literal.value.(bool); !ok {
			return 0, fmt.Errorf("typ: malformed boolean literal payload %T", literal.value)
		}
		return 3, nil
	case Integer.Kind():
		if _, ok := literal.value.(int64); !ok {
			return 0, fmt.Errorf("typ: malformed integer literal payload %T", literal.value)
		}
		return 2 + binary.MaxVarintLen64, nil
	case Number.Kind():
		if _, ok := literal.value.(float64); !ok {
			return 0, fmt.Errorf("typ: malformed number literal payload %T", literal.value)
		}
		return 10, nil
	case String.Kind():
		value, ok := literal.value.(string)
		if !ok {
			return 0, fmt.Errorf("typ: malformed string literal payload %T", literal.value)
		}
		return 2 + canonicalFormalsUvarintSize(uint64(len(value))) + len(value), nil
	default:
		return 0, fmt.Errorf("typ: unsupported literal base %d", literal.base)
	}
}

func (e *canonicalEncoder) canonicalRecordParts(record *Record) ([]byte, []Type, error) {
	scalarBytes := 2 + canonicalFormalsUvarintSize(uint64(len(record.Fields))) + canonicalFormalsUvarintSize(uint64(len(record.StaticMembers)))
	for _, field := range record.Fields {
		if err := e.checkpoint(); err != nil {
			return nil, nil, err
		}
		scalarBytes += canonicalFormalsUvarintSize(uint64(len(field.Name))) + len(field.Name) + 2
	}
	for _, member := range record.StaticMembers {
		if err := e.checkpoint(); err != nil {
			return nil, nil, err
		}
		scalarBytes += 1 + canonicalFormalsUvarintSize(uint64(len(member.Name))) + len(member.Name) + binary.MaxVarintLen64 + 2
	}
	if err := e.reserve(scalarBytes, 1); err != nil {
		return nil, nil, err
	}
	if err := e.reserve(len(record.Fields)+len(record.StaticMembers)+3, canonicalFormalsTypeBytes); err != nil {
		return nil, nil, err
	}
	out := make([]byte, 0, scalarBytes)
	out = append(out, canonicalRecord)
	out = appendBool(out, record.Open)
	out = appendCount(out, len(record.Fields))
	children := make([]Type, 0, len(record.Fields)+len(record.StaticMembers)+3)
	for _, field := range record.Fields {
		if err := e.checkpoint(); err != nil {
			return nil, nil, err
		}
		out = appendFrameString(out, field.Name)
		out = appendBool(out, field.Optional)
		out = appendBool(out, field.Readonly)
		children = append(children, field.Type)
	}
	out = appendCount(out, len(record.StaticMembers))
	for _, member := range record.StaticMembers {
		if err := e.checkpoint(); err != nil {
			return nil, nil, err
		}
		out = append(out, byte(member.Kind))
		out = appendFrameString(out, member.Name)
		out = binary.AppendVarint(out, member.Index)
		out = appendBool(out, member.Optional)
		out = appendBool(out, member.Readonly)
		children = append(children, member.Type)
	}
	hasMap := record.HasMapComponent()
	if (record.MapKey == nil) != (record.MapValue == nil) {
		return nil, nil, fmt.Errorf("typ: malformed partial record map component")
	}
	out = appendBool(out, hasMap)
	if hasMap {
		children = append(children, record.MapKey, record.MapValue)
	}
	out = appendBool(out, record.Metatable != nil)
	if record.Metatable != nil {
		children = append(children, record.Metatable)
	}
	return out, children, nil
}

func (e *canonicalEncoder) canonicalFunctionParts(function *Function) ([]byte, []Type, error) {
	scalarBytes := 1 + canonicalFormalsUvarintSize(uint64(len(function.TypeParams))) + canonicalFormalsUvarintSize(uint64(len(function.Params))) + len(function.Params)*2 + 1 + canonicalFormalsUvarintSize(uint64(len(function.Returns)))
	if err := e.reserve(scalarBytes, 1); err != nil {
		return nil, nil, err
	}
	if err := e.reserve(len(function.TypeParams)+len(function.Params)+len(function.Returns)+1, canonicalFormalsTypeBytes); err != nil {
		return nil, nil, err
	}
	out := make([]byte, 0, scalarBytes)
	out = append(out, canonicalFunction)
	out = appendCount(out, len(function.TypeParams))
	children := make([]Type, 0, len(function.TypeParams)+len(function.Params)+len(function.Returns)+1)
	for _, param := range function.TypeParams {
		if err := e.checkpoint(); err != nil {
			return nil, nil, err
		}
		children = append(children, param)
	}
	out = appendCount(out, len(function.Params))
	for _, param := range function.Params {
		if err := e.checkpoint(); err != nil {
			return nil, nil, err
		}
		// Param.Name is presentation-only under TypeEquals.
		out = appendBool(out, param.Optional)
		out = appendBool(out, param.Receiver)
		children = append(children, param.Type)
	}
	out = appendBool(out, function.Variadic != nil)
	if function.Variadic != nil {
		children = append(children, function.Variadic)
	}
	out = appendCount(out, len(function.Returns))
	for _, result := range function.Returns {
		if err := e.checkpoint(); err != nil {
			return nil, nil, err
		}
		children = append(children, result)
	}
	return out, children, nil
}

func (e *canonicalEncoder) refine() error {
	finder, err := newCanonicalSCCFinder(e)
	if err != nil {
		return err
	}
	findErr := finder.find()
	e.sccIndices = finder.indices
	e.sccLow = finder.low
	e.sccOnStack = finder.onStack
	e.sccStack = finder.stack
	e.sccMembers = finder.members
	e.sccStarts = finder.componentStart
	e.sccFrames = finder.frames
	e.sccOf = finder.sccOf
	if findErr != nil {
		return findErr
	}
	if err := e.classifyByRank(finder.componentStart, finder.members, finder.sccOf); err != nil {
		return err
	}
	// ZZPROBE: M1 intra-graph duplication ratio (nodes discovered vs
	// distinct bisimulation classes). e.classes holds a contiguous class id
	// per node; nil unless typprobe hook installed.
	if zzProbeRefineHook != nil {
		classes := 0
		for _, c := range e.classes {
			if c+1 > classes {
				classes = c + 1
			}
		}
		zzProbeRefine(len(e.nodes), classes)
	}
	return e.buildClassRepresentatives()
}

type canonicalSCCFinder struct {
	encoder        *canonicalEncoder
	next           int
	indices        []int
	low            []int
	onStack        []bool
	stack          []int
	members        []int
	componentStart []int
	sccOf          []int
	frames         []canonicalSCCFrame
}

type canonicalSCCFrame struct {
	node     int
	nextEdge int
}

func newCanonicalSCCFinder(encoder *canonicalEncoder) (canonicalSCCFinder, error) {
	count := len(encoder.nodes)
	if err := encoder.reserve(count, canonicalFormalsIntBytes*4+canonicalFormalsBoolBytes+canonicalFormalsFrameBytes*2); err != nil {
		return canonicalSCCFinder{}, err
	}
	indices := growInts(encoder.sccIndices, count)
	for index := range indices {
		if err := encoder.checkpoint(); err != nil {
			return canonicalSCCFinder{}, err
		}
		indices[index] = -1
	}
	low := growInts(encoder.sccLow, count)
	sccOf := growInts(encoder.sccOf, count)
	onStack := growBools(encoder.sccOnStack, count)
	clear(onStack)
	if err := encoder.checkpoint(); err != nil {
		return canonicalSCCFinder{}, err
	}
	if cap(encoder.sccStack) < count {
		encoder.sccStack = make([]int, 0, count)
	} else {
		encoder.sccStack = encoder.sccStack[:0]
	}
	if err := encoder.checkpoint(); err != nil {
		return canonicalSCCFinder{}, err
	}
	if cap(encoder.sccMembers) < count {
		encoder.sccMembers = make([]int, 0, count)
	} else {
		encoder.sccMembers = encoder.sccMembers[:0]
	}
	if err := encoder.checkpoint(); err != nil {
		return canonicalSCCFinder{}, err
	}
	if cap(encoder.sccStarts) < count+1 {
		encoder.sccStarts = make([]int, 0, count+1)
	} else {
		encoder.sccStarts = encoder.sccStarts[:0]
	}
	if err := encoder.checkpoint(); err != nil {
		return canonicalSCCFinder{}, err
	}
	if cap(encoder.sccFrames) < count {
		encoder.sccFrames = make([]canonicalSCCFrame, 0, count)
	} else {
		encoder.sccFrames = encoder.sccFrames[:0]
	}
	return canonicalSCCFinder{
		encoder:        encoder,
		indices:        indices,
		low:            low,
		onStack:        onStack,
		stack:          encoder.sccStack,
		members:        encoder.sccMembers,
		componentStart: append(encoder.sccStarts, 0),
		sccOf:          sccOf,
		frames:         encoder.sccFrames,
	}, nil
}

// find groups every node into its strongly connected component. Components are
// laid out in members contiguously and in reverse-topological order, so the
// quotient reaches a component only after every component below it is sealed.
func (f *canonicalSCCFinder) find() error {
	for node := range f.encoder.nodes {
		if err := f.encoder.checkpoint(); err != nil {
			return err
		}
		if f.indices[node] >= 0 {
			continue
		}
		if err := f.walk(node); err != nil {
			return err
		}
	}
	return nil
}

// walk is iterative Tarjan DFS. Its completion order intentionally matches
// recursive Tarjan's sink-first condensation order.
func (f *canonicalSCCFinder) walk(root int) error {
	if err := f.enter(root); err != nil {
		return err
	}
	for len(f.frames) != 0 {
		if err := f.encoder.checkpoint(); err != nil {
			return err
		}
		frameIndex := len(f.frames) - 1
		frame := &f.frames[frameIndex]
		node := frame.node
		edges := f.encoder.nodes[node].edges
		if frame.nextEdge < len(edges) {
			edge := edges[frame.nextEdge]
			frame.nextEdge++
			if f.indices[edge] < 0 {
				if err := f.enter(edge); err != nil {
					return err
				}
				continue
			}
			if f.onStack[edge] && f.indices[edge] < f.low[node] {
				f.low[node] = f.indices[edge]
			}
			continue
		}

		f.frames = f.frames[:frameIndex]
		if f.low[node] == f.indices[node] {
			component := len(f.componentStart) - 1
			for {
				if err := f.encoder.checkpoint(); err != nil {
					return err
				}
				last := len(f.stack) - 1
				member := f.stack[last]
				f.stack = f.stack[:last]
				f.onStack[member] = false
				f.sccOf[member] = component
				f.members = append(f.members, member)
				if member == node {
					break
				}
			}
			f.componentStart = append(f.componentStart, len(f.members))
		}
		if len(f.frames) != 0 {
			parent := f.frames[len(f.frames)-1].node
			if f.low[node] < f.low[parent] {
				f.low[parent] = f.low[node]
			}
		}
	}
	return nil
}

func (f *canonicalSCCFinder) enter(node int) error {
	if err := f.encoder.checkpoint(); err != nil {
		return err
	}
	f.indices[node] = f.next
	f.low[node] = f.next
	f.next++
	f.stack = append(f.stack, node)
	f.onStack[node] = true
	f.frames = append(f.frames, canonicalSCCFrame{node: node})
	return nil
}

// canonicalRankUnbounded is the rank of a node no finite unfolding depth
// bounds: every path leaving it stays inside the cyclic region, so it reaches
// no well-founded child at all.
const canonicalRankUnbounded = math.MinInt

// classifyByRank assigns one contiguous bisimulation class id per discovered
// node, over one class-number space.
//
// Components arrive sink-first. A component is well founded when it is a single
// node without a self edge and every component it reaches is well founded; such
// a node unfolds to a finite tree, its children are already final when it is
// reached, and hash-consing (scalar, child class vector) therefore decides its
// class exactly. That fold is the whole classifier for an acyclic graph.
//
// Every other node unfolds infinitely, so it can be bisimilar only to another
// infinitely unfolding node, and bisimilar nodes carry equal rank. The
// non-well-founded components are refined one rank stratum at a time, lowest
// rank first, with every lower stratum already sealed and spelled by class id.
// Stratifying by rank rather than by component is what lets a cyclic component
// merge with a bisimilar cycle it reaches: equal rank puts both in one
// refinement, where no per-component canonical key could relate them.
func (e *canonicalEncoder) classifyByRank(componentStart, members, sccOf []int) error {
	count := len(e.nodes)
	componentCount := len(componentStart) - 1
	if err := e.reserve(count, canonicalFormalsIntBytes); err != nil {
		return err
	}
	e.classes = growInts(e.classes, count)
	for index := range e.classes {
		if err := e.checkpoint(); err != nil {
			return err
		}
		e.classes[index] = -1
	}
	if count == 0 {
		return nil
	}
	if err := e.reserve(componentCount, canonicalFormalsIntBytes+canonicalFormalsBoolBytes); err != nil {
		return err
	}
	clear(e.foldedClasses)
	if e.foldedClasses == nil {
		if err := e.reserve(count, canonicalFormalsMapEntryBytes); err != nil {
			return err
		}
		e.foldedClasses = make(map[string]int, count)
	}

	wellFounded := make([]bool, componentCount)
	rank := make([]int, componentCount)
	nextClass, deferred, maxStratum := 0, 0, 0
	for component := range componentCount {
		memberList := members[componentStart[component]:componentStart[component+1]]
		cyclic := len(memberList) > 1
		reachesOnlyWellFounded := true
		exits := false
		componentRank := canonicalRankUnbounded
		// A well-founded component reads its children's final class ids, so the
		// same edge walk that ranks it also sizes its fold key.
		childBytes := 0
		for _, node := range memberList {
			for _, edge := range e.nodes[node].edges {
				if err := e.checkpoint(); err != nil {
					return err
				}
				target := sccOf[edge]
				if target == component {
					cyclic = true
					continue
				}
				if target > component {
					return fmt.Errorf("typ: canonical condensation is not reverse-topological")
				}
				exits = true
				candidate := rank[target]
				if wellFounded[target] {
					candidate++
				} else {
					reachesOnlyWellFounded = false
				}
				if candidate > componentRank {
					componentRank = candidate
				}
				if e.classes[edge] >= 0 {
					childBytes += canonicalFormalsUvarintSize(uint64(e.classes[edge]))
				}
			}
		}
		founded := !cyclic && reachesOnlyWellFounded
		if founded && !exits {
			componentRank = 0
		}
		wellFounded[component] = founded
		rank[component] = componentRank
		if founded {
			minted, err := e.foldWellFoundedNode(memberList[0], childBytes, nextClass)
			if err != nil {
				return err
			}
			if minted {
				nextClass++
			}
			continue
		}
		stratum, err := canonicalRankStratum(componentRank, count)
		if err != nil {
			return err
		}
		if stratum > maxStratum {
			maxStratum = stratum
		}
		deferred += len(memberList)
	}
	if deferred == 0 {
		return nil
	}
	return e.refineRankStrata(count, componentCount, componentStart, members, wellFounded, rank, deferred, maxStratum, nextClass)
}

// foldWellFoundedNode hash-conses one finitely unfolding node against its
// scalar and its children's final class ids, and reports whether the node
// minted the candidate class rather than joining an existing one. childBytes is
// the encoded width of the child class ids, measured while the node was ranked.
func (e *canonicalEncoder) foldWellFoundedNode(nodeIndex, childBytes, candidate int) (bool, error) {
	node := e.nodes[nodeIndex]
	keyBytes := canonicalFormalsUvarintSize(uint64(len(node.scalar))) + len(node.scalar) + canonicalFormalsUvarintSize(uint64(len(node.edges))) + childBytes
	if err := e.reserve(keyBytes, 1); err != nil {
		return false, err
	}
	key := make([]byte, 0, keyBytes)
	key = appendFrameBytes(key, node.scalar)
	key = appendCount(key, len(node.edges))
	for _, edge := range node.edges {
		if err := e.checkpoint(); err != nil {
			return false, err
		}
		if e.classes[edge] < 0 {
			return false, fmt.Errorf("typ: canonical condensation is not sink-first")
		}
		key = binary.AppendUvarint(key, uint64(e.classes[edge]))
	}
	class, exists := e.foldedClasses[string(key)]
	if !exists {
		class = candidate
		if err := e.reserve(1, canonicalFormalsMapEntryBytes); err != nil {
			return false, err
		}
		e.foldedClasses[string(key)] = class
	}
	e.classes[nodeIndex] = class
	return !exists, nil
}

// refineRankStrata refines the non-well-founded nodes rank by rank, lowest
// first. Ranks are bucketed rather than sorted: an unbounded rank is the lowest
// stratum, and a finite rank never exceeds the node count because it grows by
// at most one per well-founded level.
func (e *canonicalEncoder) refineRankStrata(count, componentCount int, componentStart, members []int, wellFounded []bool, rank []int, deferred, maxStratum, nextClass int) error {
	if err := e.reserve(2*(maxStratum+2)+deferred+count, canonicalFormalsIntBytes); err != nil {
		return err
	}
	stratumStart := make([]int, maxStratum+2)
	for component := range componentCount {
		if err := e.checkpoint(); err != nil {
			return err
		}
		if wellFounded[component] {
			continue
		}
		stratum, err := canonicalRankStratum(rank[component], count)
		if err != nil {
			return err
		}
		stratumStart[stratum] += componentStart[component+1] - componentStart[component]
	}
	cursor := make([]int, maxStratum+2)
	total := 0
	for stratum := range maxStratum + 1 {
		if err := e.checkpoint(); err != nil {
			return err
		}
		size := stratumStart[stratum]
		stratumStart[stratum] = total
		cursor[stratum] = total
		total += size
	}
	stratumStart[maxStratum+1] = total
	stratumNodes := make([]int, total)
	for component := range componentCount {
		if err := e.checkpoint(); err != nil {
			return err
		}
		if wellFounded[component] {
			continue
		}
		stratum, err := canonicalRankStratum(rank[component], count)
		if err != nil {
			return err
		}
		for _, node := range members[componentStart[component]:componentStart[component+1]] {
			stratumNodes[cursor[stratum]] = node
			cursor[stratum]++
		}
	}

	localIndex := make([]int, count)
	for index := range localIndex {
		if err := e.checkpoint(); err != nil {
			return err
		}
		localIndex[index] = -1
	}
	for stratum := range maxStratum + 1 {
		if err := e.checkpoint(); err != nil {
			return err
		}
		nodes := stratumNodes[stratumStart[stratum]:stratumStart[stratum+1]]
		if len(nodes) == 0 {
			continue
		}
		classes, err := e.refineStratum(nodes, localIndex, nextClass)
		if err != nil {
			return err
		}
		nextClass += classes
	}
	return nil
}

// canonicalRankStratum orders ranks into refinement passes: the unbounded rank
// is the lowest stratum, and finite rank r follows at r+1.
func canonicalRankStratum(rank, count int) (int, error) {
	if rank == canonicalRankUnbounded {
		return 0, nil
	}
	if rank < 0 || rank > count {
		return 0, fmt.Errorf("typ: canonical rank %d is outside the %d node graph", rank, count)
	}
	return rank + 1, nil
}

type canonicalPredecessor struct {
	node     int
	position int
}

// refineStratum runs labeled partition refinement over one rank stratum and
// returns the number of classes it minted from classBase. Nodes outside the
// stratum are already final and enter the initial key by class id; Hopcroft's
// smaller-half schedule then splits on in-stratum edges by edge position, so a
// class fixes both the scalar and the whole child class vector.
func (e *canonicalEncoder) refineStratum(stratum, localIndex []int, classBase int) (int, error) {
	size := len(stratum)
	if err := e.reserve(size, canonicalFormalsIntBytes*2+canonicalFormalsBoolBytes*2+canonicalFormalsFrameBytes*4); err != nil {
		return 0, err
	}
	for index, node := range stratum {
		if err := e.checkpoint(); err != nil {
			return 0, err
		}
		localIndex[node] = index
	}
	defer func() {
		for _, node := range stratum {
			localIndex[node] = -1
		}
	}()

	classOf := make([]int, size)
	blocks := make([][]int, 0, size)
	if err := e.reserve(size, canonicalFormalsMapEntryBytes); err != nil {
		return 0, err
	}
	initial := make(map[string]int, size)
	for index, nodeIndex := range stratum {
		if err := e.checkpoint(); err != nil {
			return 0, err
		}
		node := e.nodes[nodeIndex]
		key := appendFrameBytes(nil, node.scalar)
		key = appendCount(key, len(node.edges))
		for _, edge := range node.edges {
			if err := e.checkpoint(); err != nil {
				return 0, err
			}
			if localIndex[edge] >= 0 {
				key = binary.AppendUvarint(key, 0)
				continue
			}
			if e.classes[edge] < 0 {
				return 0, fmt.Errorf("typ: canonical rank stratum reads an unsealed class")
			}
			key = binary.AppendUvarint(key, uint64(e.classes[edge])+1)
		}
		class, exists := initial[string(key)]
		if !exists {
			class = len(blocks)
			if err := e.reserve(1, canonicalFormalsMapEntryBytes); err != nil {
				return 0, err
			}
			initial[string(key)] = class
			blocks = append(blocks, nil)
		}
		classOf[index] = class
		blocks[class] = append(blocks[class], index)
	}

	if err := e.reserve(size, canonicalFormalsFrameBytes); err != nil {
		return 0, err
	}
	predecessors := make([][]canonicalPredecessor, size)
	for index, nodeIndex := range stratum {
		if err := e.checkpoint(); err != nil {
			return 0, err
		}
		for position, edge := range e.nodes[nodeIndex].edges {
			if err := e.checkpoint(); err != nil {
				return 0, err
			}
			target := localIndex[edge]
			if target < 0 {
				continue
			}
			predecessors[target] = append(predecessors[target], canonicalPredecessor{node: index, position: position})
		}
	}
	if err := e.reserve(len(blocks), canonicalFormalsIntBytes+canonicalFormalsBoolBytes); err != nil {
		return 0, err
	}
	queue := make([]int, len(blocks))
	queued := make([]bool, len(blocks), size)
	for class := range blocks {
		if err := e.checkpoint(); err != nil {
			return 0, err
		}
		queue[class] = class
		queued[class] = true
	}
	marked := make([]bool, size)

	for head := 0; head < len(queue); head++ {
		if err := e.checkpoint(); err != nil {
			return 0, err
		}
		splitter := queue[head]
		queued[splitter] = false
		if err := e.reserve(len(blocks), canonicalFormalsMapEntryBytes); err != nil {
			return 0, err
		}
		byPosition := make(map[int][]int)
		for _, target := range blocks[splitter] {
			if err := e.checkpoint(); err != nil {
				return 0, err
			}
			for _, predecessor := range predecessors[target] {
				if err := e.checkpoint(); err != nil {
					return 0, err
				}
				byPosition[predecessor.position] = append(byPosition[predecessor.position], predecessor.node)
			}
		}
		positions := make([]int, 0, len(byPosition))
		for position := range byPosition {
			if err := e.checkpoint(); err != nil {
				return 0, err
			}
			positions = append(positions, position)
		}
		if err := e.sortCanonicalIntKeys(positions); err != nil {
			return 0, err
		}
		for _, position := range positions {
			if err := e.checkpoint(); err != nil {
				return 0, err
			}
			predecessorNodes := byPosition[position]
			if err := e.reserve(len(predecessorNodes), canonicalFormalsMapEntryBytes); err != nil {
				return 0, err
			}
			byBlock := make(map[int][]int)
			for _, predecessor := range predecessorNodes {
				if err := e.checkpoint(); err != nil {
					return 0, err
				}
				class := classOf[predecessor]
				byBlock[class] = append(byBlock[class], predecessor)
			}
			touched := make([]int, 0, len(byBlock))
			for class := range byBlock {
				if err := e.checkpoint(); err != nil {
					return 0, err
				}
				touched = append(touched, class)
			}
			if err := e.sortCanonicalIntKeys(touched); err != nil {
				return 0, err
			}
			for _, class := range touched {
				if err := e.checkpoint(); err != nil {
					return 0, err
				}
				subset := byBlock[class]
				if len(subset) == len(blocks[class]) {
					continue
				}
				for _, node := range subset {
					if err := e.checkpoint(); err != nil {
						return 0, err
					}
					marked[node] = true
				}
				inside := make([]int, 0, len(subset))
				outside := make([]int, 0, len(blocks[class])-len(subset))
				for _, node := range blocks[class] {
					if err := e.checkpoint(); err != nil {
						return 0, err
					}
					if marked[node] {
						inside = append(inside, node)
					} else {
						outside = append(outside, node)
					}
				}
				for _, node := range subset {
					if err := e.checkpoint(); err != nil {
						return 0, err
					}
					marked[node] = false
				}
				blocks[class] = outside
				newClass := len(blocks)
				blocks = append(blocks, inside)
				queued = append(queued, false)
				for _, node := range inside {
					if err := e.checkpoint(); err != nil {
						return 0, err
					}
					classOf[node] = newClass
				}
				if queued[class] {
					queue = append(queue, newClass)
					queued[newClass] = true
				} else if len(inside) < len(outside) {
					queue = append(queue, newClass)
					queued[newClass] = true
				} else {
					queue = append(queue, class)
					queued[class] = true
				}
			}
		}
	}

	for index, nodeIndex := range stratum {
		if err := e.checkpoint(); err != nil {
			return 0, err
		}
		e.classes[nodeIndex] = classBase + classOf[index]
	}
	return len(blocks), nil
}

// sortCanonicalIntKeys orders sparse refinement keys without creating an
// uninterruptible cancellation island. Chunks are deliberately bounded; merge
// passes checkpoint each copied key and retain ordinary O(k log k) behavior.
func (e *canonicalEncoder) sortCanonicalIntKeys(keys []int) error {
	const chunk = 256
	for start := 0; start < len(keys); start += chunk {
		if err := e.checkpoint(); err != nil {
			return err
		}
		end := start + chunk
		if end > len(keys) {
			end = len(keys)
		}
		sort.Ints(keys[start:end])
	}
	if len(keys) <= chunk {
		return nil
	}
	if err := e.reserve(len(keys), canonicalFormalsIntBytes); err != nil {
		return err
	}
	scratch := make([]int, len(keys))
	for width := chunk; width < len(keys); width *= 2 {
		for start := 0; start < len(keys); start += 2 * width {
			middle := start + width
			if middle > len(keys) {
				middle = len(keys)
			}
			end := start + 2*width
			if end > len(keys) {
				end = len(keys)
			}
			left, right, out := start, middle, start
			for left < middle && right < end {
				if err := e.checkpoint(); err != nil {
					return err
				}
				if keys[left] <= keys[right] {
					scratch[out], left = keys[left], left+1
				} else {
					scratch[out], right = keys[right], right+1
				}
				out++
			}
			for left < middle {
				if err := e.checkpoint(); err != nil {
					return err
				}
				scratch[out], left, out = keys[left], left+1, out+1
			}
			for right < end {
				if err := e.checkpoint(); err != nil {
					return err
				}
				scratch[out], right, out = keys[right], right+1, out+1
			}
			for copied := start; copied < end; copied += chunk {
				if err := e.checkpoint(); err != nil {
					return err
				}
				copyEnd := copied + chunk
				if copyEnd > end {
					copyEnd = end
				}
				copy(keys[copied:copyEnd], scratch[copied:copyEnd])
			}
		}
	}
	return nil
}

func (e *canonicalEncoder) buildClassRepresentatives() error {
	classCount := 0
	for _, class := range e.classes {
		if err := e.checkpoint(); err != nil {
			return err
		}
		if class+1 > classCount {
			classCount = class + 1
		}
	}
	if err := e.reserve(classCount, canonicalFormalsIntBytes); err != nil {
		return err
	}
	e.representatives = growInts(e.representatives, classCount)
	for class := range e.representatives {
		if err := e.checkpoint(); err != nil {
			return err
		}
		e.representatives[class] = -1
	}
	for node, class := range e.classes {
		if err := e.checkpoint(); err != nil {
			return err
		}
		if e.representatives[class] < 0 {
			e.representatives[class] = node
		}
	}
	return nil
}

type canonicalEmissionFrame struct {
	node     int
	nextEdge int
}

// emitClass writes the existing preorder reference/definition stream with an
// explicit stack. A definition is written before its children exactly as in
// the prior recursive encoder, so ordinary Canonical bytes remain unchanged.
func (e *canonicalEncoder) emitClass(class int, ordinals map[int]uint64) error {
	if err := e.emitEnter(class, ordinals); err != nil {
		return err
	}
	for len(e.emissionStack) != 0 {
		frameIndex := len(e.emissionStack) - 1
		frame := &e.emissionStack[frameIndex]
		node := e.nodes[frame.node]
		if frame.nextEdge == len(node.edges) {
			e.emissionStack = e.emissionStack[:frameIndex]
			continue
		}
		edge := node.edges[frame.nextEdge]
		frame.nextEdge++
		if err := e.emitEnter(e.classes[edge], ordinals); err != nil {
			return err
		}
	}
	return nil
}

func (e *canonicalEncoder) emitEnter(class int, ordinals map[int]uint64) error {
	if err := e.checkpoint(); err != nil {
		return err
	}
	if ordinal, ok := ordinals[class]; ok {
		if err := e.appendCanonicalOutput([]byte{0}); err != nil {
			return err
		}
		if err := e.appendCanonicalOutputUvarint(ordinal); err != nil {
			return err
		}
		return nil
	}
	ordinal := uint64(len(ordinals))
	if err := e.reserve(1, canonicalFormalsMapEntryBytes); err != nil {
		return err
	}
	ordinals[class] = ordinal
	if class < 0 || class >= len(e.representatives) || e.representatives[class] < 0 {
		return fmt.Errorf("typ: missing canonical class %d", class)
	}
	representative := e.representatives[class]
	node := e.nodes[representative]
	if err := e.appendCanonicalOutput([]byte{1}); err != nil {
		return err
	}
	if err := e.appendCanonicalOutputUvarint(ordinal); err != nil {
		return err
	}
	if err := e.appendCanonicalOutputFrame(node.scalar); err != nil {
		return err
	}
	if err := e.appendCanonicalOutputUvarint(uint64(len(node.edges))); err != nil {
		return err
	}
	if err := e.reserve(1, canonicalFormalsFrameBytes); err != nil {
		return err
	}
	e.emissionStack = append(e.emissionStack, canonicalEmissionFrame{node: representative})
	return nil
}

func (e *canonicalEncoder) appendCanonicalOutput(parts ...[]byte) error {
	out, err := canonicalFormalsAppendBytes(e.ctx, e.admission, &e.steps, e.out, parts...)
	if err != nil {
		return err
	}
	e.out = out
	return nil
}

func (e *canonicalEncoder) appendCanonicalOutputUvarint(value uint64) error {
	var encoded [binary.MaxVarintLen64]byte
	count := binary.PutUvarint(encoded[:], value)
	return e.appendCanonicalOutput(encoded[:count])
}

func (e *canonicalEncoder) appendCanonicalOutputFrame(value []byte) error {
	var length [binary.MaxVarintLen64]byte
	count := binary.PutUvarint(length[:], uint64(len(value)))
	return e.appendCanonicalOutput(length[:count], value)
}

func (e *canonicalEncoder) appendCanonicalOutputFrameString(value string) error {
	var length [binary.MaxVarintLen64]byte
	count := binary.PutUvarint(length[:], uint64(len(value)))
	return e.appendCanonicalOutput(length[:count], []byte(value))
}

func appendBool(out []byte, value bool) []byte {
	if value {
		return append(out, 1)
	}
	return append(out, 0)
}

func appendCount(out []byte, count int) []byte {
	return binary.AppendUvarint(out, uint64(count))
}

func appendFrameString(out []byte, value string) []byte {
	out = appendCount(out, len(value))
	return append(out, value...)
}

func appendFrameBytes(out, value []byte) []byte {
	out = appendCount(out, len(value))
	return append(out, value...)
}

func growInts(in []int, size int) []int {
	if cap(in) < size {
		return make([]int, size)
	}
	return in[:size]
}

func growBools(in []bool, size int) []bool {
	if cap(in) < size {
		return make([]bool, size)
	}
	return in[:size]
}
