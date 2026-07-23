package typ

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"reflect"
	"sort"
)

// CanonicalDigest is the full collision-resistant digest of one framed
// canonical type graph. Canonical bytes, rather than this digest, remain the
// final authority when a consumer must distinguish adversarial collisions.
type CanonicalDigest [sha256.Size]byte

const (
	canonicalTypeDomain  = "wippy.analysis.type.typ.canonical"
	canonicalTypeVersion = uint64(1)
)

// EncodeCanonical encodes the TypeEquals semantic graph without consulting
// node addresses, recursive IDs, cached hashes, revisions, or String output.
// Unsupported or malformed graphs fail closed and return no bytes.
func EncodeCanonical(ctx context.Context, t Type) ([]byte, error) {
	var encoder CanonicalEncoder
	return encoder.Encode(ctx, t)
}

// DigestCanonical returns the typed full digest of EncodeCanonical.
func DigestCanonical(ctx context.Context, t Type) (CanonicalDigest, error) {
	var encoder CanonicalEncoder
	return encoder.Digest(ctx, t)
}

// CanonicalEncoder retains traversal scratch across calls. It is not safe for
// concurrent use. Returned byte slices are ownership-isolated from the
// encoder and remain valid after the next call.
type CanonicalEncoder struct {
	nodes           []canonicalTypeNode
	seen            map[Type]int
	transparent     map[Type]bool
	recursiveID     map[uint64]*Recursive
	classes         []int
	representatives []int
	out             []byte
	steps           uint64
	ctx             context.Context
}

// Encode is the reusable form of EncodeCanonical.
func (e *CanonicalEncoder) Encode(ctx context.Context, t Type) ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("typ: nil canonical encoder")
	}
	e.reset(ctx)
	root, err := e.visit(t)
	if err != nil {
		e.abort()
		return nil, err
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
	e.out = appendFrameString(e.out, canonicalTypeDomain)
	e.out = binary.AppendUvarint(e.out, canonicalTypeVersion)
	ordinals := make(map[int]uint64, len(e.nodes))
	if err := e.emitClass(rootClass, ordinals); err != nil {
		e.abort()
		return nil, err
	}
	out := append([]byte(nil), e.out...)
	e.ctx = nil
	return out, nil
}

// Digest is the reusable form of DigestCanonical.
func (e *CanonicalEncoder) Digest(ctx context.Context, t Type) (CanonicalDigest, error) {
	encoded, err := e.Encode(ctx, t)
	if err != nil {
		return CanonicalDigest{}, err
	}
	return CanonicalDigest(sha256.Sum256(encoded)), nil
}

type canonicalTypeNode struct {
	scalar []byte
	edges  []int
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

func (e *CanonicalEncoder) reset(ctx context.Context) {
	e.ctx = ctx
	e.steps = 0
	e.nodes = e.nodes[:0]
	clear(e.seen)
	if e.seen == nil {
		e.seen = make(map[Type]int)
	}
	clear(e.transparent)
	if e.transparent == nil {
		e.transparent = make(map[Type]bool)
	}
	clear(e.recursiveID)
	if e.recursiveID == nil {
		e.recursiveID = make(map[uint64]*Recursive)
	}
}

func (e *CanonicalEncoder) abort() {
	e.out = e.out[:0]
	e.ctx = nil
}

func (e *CanonicalEncoder) checkpoint() error {
	e.steps++
	if e.ctx != nil && (e.steps == 1 || e.steps&63 == 0) {
		return e.ctx.Err()
	}
	return nil
}

func (e *CanonicalEncoder) visit(input Type) (int, error) {
	if err := e.checkpoint(); err != nil {
		return 0, err
	}
	t, err := e.unwrapTransparent(input)
	if err != nil {
		return 0, err
	}
	if t == nil {
		return e.leaf(nil, canonicalNil, nil)
	}
	if dynamic := reflect.TypeOf(t); dynamic == nil || !dynamic.Comparable() {
		return 0, fmt.Errorf("typ: unsupported non-comparable type implementation %T", t)
	}
	if index, ok := e.seen[t]; ok {
		return index, nil
	}

	index := len(e.nodes)
	e.seen[t] = index
	e.nodes = append(e.nodes, canonicalTypeNode{})
	node := &e.nodes[index]
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
		node.scalar, err = canonicalLiteralScalar(value)
	case *Ref:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Ref")
		}
		node.scalar = appendFrameString([]byte{canonicalRef}, value.Module)
		node.scalar = appendFrameString(node.scalar, value.Name)
	case *Optional:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Optional")
		}
		node.scalar, children = []byte{canonicalOptional}, []Type{value.Inner}
	case *Union:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Union")
		}
		node.scalar = appendCount([]byte{canonicalUnion}, len(value.Members))
		children = append(children, value.Members...)
	case *Intersection:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Intersection")
		}
		node.scalar = appendCount([]byte{canonicalIntersection}, len(value.Members))
		children = append(children, value.Members...)
	case *Tuple:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Tuple")
		}
		node.scalar = appendCount([]byte{canonicalTuple}, len(value.Elements))
		children = append(children, value.Elements...)
	case *Array:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Array")
		}
		node.scalar, children = []byte{canonicalArray}, []Type{value.Element}
	case *Map:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Map")
		}
		node.scalar, children = []byte{canonicalMap}, []Type{value.Key, value.Value}
	case *ReadonlyMap:
		if value == nil {
			return 0, fmt.Errorf("typ: nil ReadonlyMap")
		}
		node.scalar, children = []byte{canonicalReadonlyMap}, []Type{value.Key, value.Value}
	case *Record:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Record")
		}
		node.scalar, children, err = canonicalRecordParts(value)
	case *Function:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Function")
		}
		node.scalar, children = canonicalFunctionParts(value)
	case *Generic:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Generic")
		}
		node.scalar = appendFrameString([]byte{canonicalGeneric}, value.Name)
		node.scalar = appendCount(node.scalar, len(value.TypeParams))
		children = make([]Type, 0, len(value.TypeParams)+1)
		for _, param := range value.TypeParams {
			children = append(children, param)
		}
		node.scalar = appendBool(node.scalar, value.Body != nil)
		if value.Body != nil {
			children = append(children, value.Body)
		}
	case *Instantiated:
		if value == nil || value.Generic == nil {
			return 0, fmt.Errorf("typ: nil Instantiated or generic")
		}
		node.scalar = appendCount([]byte{canonicalInstantiated}, len(value.TypeArgs))
		children = append(children, value.Generic)
		children = append(children, value.TypeArgs...)
	case *TypeParam:
		if value == nil {
			return 0, fmt.Errorf("typ: nil TypeParam")
		}
		node.scalar = appendFrameString([]byte{canonicalTypeParam}, value.Name)
		node.scalar = appendBool(node.scalar, value.Constraint != nil)
		if value.Constraint != nil {
			children = []Type{value.Constraint}
		}
	case *Recursive:
		if value == nil || value.ID == 0 {
			return 0, fmt.Errorf("typ: recursive node has no well-formed identity")
		}
		if prior, ok := e.recursiveID[value.ID]; ok && prior != value {
			return 0, fmt.Errorf("typ: recursive ID %d names distinct nodes", value.ID)
		}
		e.recursiveID[value.ID] = value
		node.scalar = appendFrameString([]byte{canonicalRecursive}, value.Name)
		node.scalar = appendBool(node.scalar, value.Body != nil)
		if value.Body != nil {
			children = []Type{value.Body}
		}
	case *Interface:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Interface")
		}
		node.scalar = appendFrameString([]byte{canonicalInterface}, value.Name)
		node.scalar = appendCount(node.scalar, len(value.Methods))
		for _, method := range value.Methods {
			node.scalar = appendFrameString(node.scalar, method.Name)
			children = append(children, method.Type)
		}
	case *Meta:
		if value == nil {
			return 0, fmt.Errorf("typ: nil Meta")
		}
		node.scalar, children = []byte{canonicalMeta}, []Type{value.Of}
	default:
		return 0, fmt.Errorf("typ: unsupported canonical type implementation %T", t)
	}
	if err != nil {
		return 0, err
	}
	edges := make([]int, 0, len(children))
	for _, child := range children {
		childIndex, childErr := e.visit(child)
		if childErr != nil {
			return 0, childErr
		}
		edges = append(edges, childIndex)
	}
	e.nodes[index].edges = edges
	return index, nil
}

func (e *CanonicalEncoder) leaf(identity Type, tag byte, payload []byte) (int, error) {
	if identity != nil {
		if index, ok := e.seen[identity]; ok {
			return index, nil
		}
	}
	index := len(e.nodes)
	node := canonicalTypeNode{scalar: append([]byte{tag}, payload...)}
	e.nodes = append(e.nodes, node)
	if identity != nil {
		e.seen[identity] = index
	}
	return index, nil
}

func (e *CanonicalEncoder) unwrapTransparent(t Type) (Type, error) {
	path := make([]Type, 0, 4)
	defer func() {
		for _, wrapper := range path {
			delete(e.transparent, wrapper)
		}
	}()
	for {
		if err := e.checkpoint(); err != nil {
			return nil, err
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
			path = append(path, t)
			t = value.Inner
		case *Alias:
			if value == nil {
				return nil, fmt.Errorf("typ: nil Alias")
			}
			if e.transparent[t] {
				return nil, fmt.Errorf("typ: cyclic alias")
			}
			e.transparent[t] = true
			path = append(path, t)
			t = value.UnaliasedTarget()
		default:
			return t, nil
		}
	}
}

func canonicalLiteralScalar(literal *Literal) ([]byte, error) {
	if literal == nil {
		return nil, fmt.Errorf("typ: nil Literal")
	}
	out := []byte{canonicalLiteral, byte(literal.Base)}
	switch literal.Base {
	case Boolean.Kind():
		value, ok := literal.Value.(bool)
		if !ok {
			return nil, fmt.Errorf("typ: malformed boolean literal payload %T", literal.Value)
		}
		return appendBool(out, value), nil
	case Integer.Kind():
		value, ok := literal.Value.(int64)
		if !ok {
			return nil, fmt.Errorf("typ: malformed integer literal payload %T", literal.Value)
		}
		return binary.AppendVarint(out, value), nil
	case Number.Kind():
		value, ok := literal.Value.(float64)
		if !ok || math.IsNaN(value) {
			return nil, fmt.Errorf("typ: malformed or non-reflexive number literal")
		}
		if value == 0 {
			value = 0
		}
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], math.Float64bits(value))
		return append(out, encoded[:]...), nil
	case String.Kind():
		value, ok := literal.Value.(string)
		if !ok {
			return nil, fmt.Errorf("typ: malformed string literal payload %T", literal.Value)
		}
		return appendFrameString(out, value), nil
	default:
		return nil, fmt.Errorf("typ: unsupported literal base %d", literal.Base)
	}
}

func canonicalRecordParts(record *Record) ([]byte, []Type, error) {
	out := appendBool([]byte{canonicalRecord}, record.Open)
	out = appendCount(out, len(record.Fields))
	children := make([]Type, 0, len(record.Fields)+len(record.StaticMembers)+3)
	for _, field := range record.Fields {
		out = appendFrameString(out, field.Name)
		out = appendBool(out, field.Optional)
		out = appendBool(out, field.Readonly)
		children = append(children, field.Type)
	}
	out = appendCount(out, len(record.StaticMembers))
	for _, member := range record.StaticMembers {
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

func canonicalFunctionParts(function *Function) ([]byte, []Type) {
	out := appendCount([]byte{canonicalFunction}, len(function.TypeParams))
	children := make([]Type, 0, len(function.TypeParams)+len(function.Params)+len(function.Returns)+1)
	for _, param := range function.TypeParams {
		children = append(children, param)
	}
	out = appendCount(out, len(function.Params))
	for _, param := range function.Params {
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
	children = append(children, function.Returns...)
	return out, children
}

func (e *CanonicalEncoder) refine() error {
	finder := newCanonicalSCCFinder(e)
	components, cyclic, err := finder.find()
	if err != nil {
		return err
	}
	if cyclic {
		// Bisimulation may cross SCC boundaries (for example X -> X and
		// an acyclic Y -> X denote the same regular tree). Use exact labeled
		// partition refinement for the cyclic frontier rather than assigning
		// SCC identities semantic meaning. Hopcroft's smaller-half schedule
		// avoids the graph-wide propagation rounds that made long chains
		// quadratic.
		err = e.refineCyclicGraph()
	} else {
		// Tarjan emits the acyclic condensation sink-first. Every child class
		// is therefore final when its parent is interned, so a DAG needs one
		// linear reverse-topological pass and no fixpoint rounds.
		err = e.classifyAcyclic(components)
	}
	if err != nil {
		return err
	}
	e.buildClassRepresentatives()
	return nil
}

type canonicalSCCFinder struct {
	encoder    *CanonicalEncoder
	next       int
	indices    []int
	low        []int
	onStack    []bool
	stack      []int
	components [][]int
}

func newCanonicalSCCFinder(encoder *CanonicalEncoder) *canonicalSCCFinder {
	count := len(encoder.nodes)
	indices := make([]int, count)
	for index := range indices {
		indices[index] = -1
	}
	return &canonicalSCCFinder{
		encoder: encoder,
		indices: indices,
		low:     make([]int, count),
		onStack: make([]bool, count),
		stack:   make([]int, 0, count),
	}
}

func (f *canonicalSCCFinder) find() ([][]int, bool, error) {
	for node := range f.encoder.nodes {
		if f.indices[node] >= 0 {
			continue
		}
		if err := f.visit(node); err != nil {
			return nil, false, err
		}
	}
	cyclic := false
	for _, component := range f.components {
		if len(component) > 1 {
			cyclic = true
			break
		}
		node := component[0]
		for _, edge := range f.encoder.nodes[node].edges {
			if edge == node {
				cyclic = true
				break
			}
		}
		if cyclic {
			break
		}
	}
	return f.components, cyclic, nil
}

func (f *canonicalSCCFinder) visit(node int) error {
	if err := f.encoder.checkpoint(); err != nil {
		return err
	}
	f.indices[node] = f.next
	f.low[node] = f.next
	f.next++
	f.stack = append(f.stack, node)
	f.onStack[node] = true
	for _, edge := range f.encoder.nodes[node].edges {
		if f.indices[edge] < 0 {
			if err := f.visit(edge); err != nil {
				return err
			}
			if f.low[edge] < f.low[node] {
				f.low[node] = f.low[edge]
			}
		} else if f.onStack[edge] && f.indices[edge] < f.low[node] {
			f.low[node] = f.indices[edge]
		}
	}
	if f.low[node] != f.indices[node] {
		return nil
	}
	component := make([]int, 0, 1)
	for {
		last := len(f.stack) - 1
		member := f.stack[last]
		f.stack = f.stack[:last]
		f.onStack[member] = false
		component = append(component, member)
		if member == node {
			break
		}
	}
	f.components = append(f.components, component)
	return nil
}

func (e *CanonicalEncoder) classifyAcyclic(components [][]int) error {
	e.classes = growInts(e.classes, len(e.nodes))
	for index := range e.classes {
		e.classes[index] = -1
	}
	classes := make(map[string]int, len(e.nodes))
	for _, component := range components {
		if err := e.checkpoint(); err != nil {
			return err
		}
		if len(component) != 1 {
			return fmt.Errorf("typ: cyclic component reached acyclic classifier")
		}
		nodeIndex := component[0]
		node := e.nodes[nodeIndex]
		key := appendFrameBytes(nil, node.scalar)
		key = appendCount(key, len(node.edges))
		for _, edge := range node.edges {
			if e.classes[edge] < 0 {
				return fmt.Errorf("typ: acyclic condensation is not sink-first")
			}
			key = binary.AppendUvarint(key, uint64(e.classes[edge]))
		}
		class, exists := classes[string(key)]
		if !exists {
			class = len(classes)
			classes[string(key)] = class
		}
		e.classes[nodeIndex] = class
	}
	return nil
}

type canonicalPredecessor struct {
	node     int
	position int
}

func (e *CanonicalEncoder) refineCyclicGraph() error {
	count := len(e.nodes)
	e.classes = growInts(e.classes, count)
	blocks := make([][]int, 0, count)
	initial := make(map[string]int, count)
	for nodeIndex, node := range e.nodes {
		if err := e.checkpoint(); err != nil {
			return err
		}
		key := appendFrameBytes(nil, node.scalar)
		key = appendCount(key, len(node.edges))
		class, exists := initial[string(key)]
		if !exists {
			class = len(blocks)
			initial[string(key)] = class
			blocks = append(blocks, nil)
		}
		e.classes[nodeIndex] = class
		blocks[class] = append(blocks[class], nodeIndex)
	}

	predecessors := make([][]canonicalPredecessor, count)
	for nodeIndex, node := range e.nodes {
		for position, edge := range node.edges {
			predecessors[edge] = append(predecessors[edge], canonicalPredecessor{node: nodeIndex, position: position})
		}
	}
	queue := make([]int, len(blocks))
	queued := make([]bool, len(blocks), count)
	for class := range blocks {
		queue[class] = class
		queued[class] = true
	}
	marked := make([]bool, count)

	for head := 0; head < len(queue); head++ {
		if err := e.checkpoint(); err != nil {
			return err
		}
		splitter := queue[head]
		queued[splitter] = false
		byPosition := make(map[int][]int)
		for _, target := range blocks[splitter] {
			for _, predecessor := range predecessors[target] {
				byPosition[predecessor.position] = append(byPosition[predecessor.position], predecessor.node)
			}
		}
		positions := make([]int, 0, len(byPosition))
		for position := range byPosition {
			positions = append(positions, position)
		}
		sort.Ints(positions)
		for _, position := range positions {
			byBlock := make(map[int][]int)
			for _, predecessor := range byPosition[position] {
				class := e.classes[predecessor]
				byBlock[class] = append(byBlock[class], predecessor)
			}
			touched := make([]int, 0, len(byBlock))
			for class := range byBlock {
				touched = append(touched, class)
			}
			sort.Ints(touched)
			for _, class := range touched {
				subset := byBlock[class]
				if len(subset) == len(blocks[class]) {
					continue
				}
				for _, node := range subset {
					marked[node] = true
				}
				inside := make([]int, 0, len(subset))
				outside := make([]int, 0, len(blocks[class])-len(subset))
				for _, node := range blocks[class] {
					if marked[node] {
						inside = append(inside, node)
					} else {
						outside = append(outside, node)
					}
				}
				for _, node := range subset {
					marked[node] = false
				}
				blocks[class] = outside
				newClass := len(blocks)
				blocks = append(blocks, inside)
				queued = append(queued, false)
				for _, node := range inside {
					e.classes[node] = newClass
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
	return nil
}

func (e *CanonicalEncoder) buildClassRepresentatives() {
	classCount := 0
	for _, class := range e.classes {
		if class+1 > classCount {
			classCount = class + 1
		}
	}
	e.representatives = growInts(e.representatives, classCount)
	for class := range e.representatives {
		e.representatives[class] = -1
	}
	for node, class := range e.classes {
		if e.representatives[class] < 0 {
			e.representatives[class] = node
		}
	}
}

func (e *CanonicalEncoder) emitClass(class int, ordinals map[int]uint64) error {
	if err := e.checkpoint(); err != nil {
		return err
	}
	if ordinal, ok := ordinals[class]; ok {
		e.out = append(e.out, 0)
		e.out = binary.AppendUvarint(e.out, ordinal)
		return nil
	}
	ordinal := uint64(len(ordinals))
	ordinals[class] = ordinal
	if class < 0 || class >= len(e.representatives) || e.representatives[class] < 0 {
		return fmt.Errorf("typ: missing canonical class %d", class)
	}
	representative := e.representatives[class]
	node := e.nodes[representative]
	e.out = append(e.out, 1)
	e.out = binary.AppendUvarint(e.out, ordinal)
	e.out = appendFrameBytes(e.out, node.scalar)
	e.out = appendCount(e.out, len(node.edges))
	for _, edge := range node.edges {
		if err := e.emitClass(e.classes[edge], ordinals); err != nil {
			return err
		}
	}
	return nil
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
