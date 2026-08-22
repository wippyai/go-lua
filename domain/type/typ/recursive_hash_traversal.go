package typ

import (
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/internal/hash"
)

// The recursive hash is a structural value, but its evaluation is a finite
// graph walk. hashMachine keeps the old mixing order while carrying every
// pending child result on an explicit stack.
type hashMode uint8

const (
	hashWithNode hashMode = iota
	hashBodyNode
)

type hashPhase uint8

const (
	hashEnter hashPhase = iota
	hashFinishWithPlain
	hashFinishRecursive
	hashFinishAlias
	hashRunOps
	hashFinishChild
)

type hashOp struct {
	child   Type
	value   uint64
	isChild bool
}

type hashFrame struct {
	mode       hashMode
	phase      hashPhase
	input      Type
	node       Type
	rec        *Recursive
	generic    *Generic
	result     uint64
	child      uint64
	hash       uint64
	ops        []hashOp
	next       int
	activeNode Type
	// transparent marks an *Instantiated node's frame: it is an application of
	// its Generic, not a declaration of its own, so it never registers in
	// active/memo. Its value is always a fresh function of the Generic's
	// current (possibly cycle-anchored) hash and its own TypeArgs, which keeps
	// it independent of whichever Instantiated wrapper started the traversal.
	transparent bool
}

type hashMachine struct {
	scratch *recursiveHashScratch
	stack   []*hashFrame
}

func hashWithVisitedMemo(t Type, scratch *recursiveHashScratch) uint64 {
	return hashTypeGraph(t, scratch, hashWithNode)
}

func hashBodyWithVisitedMemo(t Type, scratch *recursiveHashScratch) uint64 {
	return hashTypeGraph(t, scratch, hashBodyNode)
}

func hashTypeGraph(t Type, scratch *recursiveHashScratch, mode hashMode) uint64 {
	root := &hashFrame{mode: mode, phase: hashEnter, input: t}
	machine := hashMachine{scratch: scratch, stack: []*hashFrame{root}}
	for len(machine.stack) != 0 {
		top := machine.stack[len(machine.stack)-1]
		machine.step(top)
	}
	return root.result
}

func (m *hashMachine) step(frame *hashFrame) {
	switch frame.phase {
	case hashEnter:
		m.enter(frame)
	case hashFinishWithPlain:
		m.scratch.memoSet(frame.input, frame.child)
		m.complete(frame, frame.child)
	case hashFinishRecursive:
		m.scratch.visitedPop(frame.rec)
		result := hash.MixHash(frame.hash, frame.child)
		m.scratch.memoSet(frame.rec, result)
		m.complete(frame, result)
	case hashFinishAlias:
		m.complete(frame, frame.child)
	case hashRunOps:
		if frame.next == len(frame.ops) {
			switch {
			case frame.generic != nil:
				m.scratch.memoSet(frame.node, frame.hash)
				m.scratch.visitedPopGeneric(frame.generic)
			case frame.transparent:
				// No registration: see the *Instantiated case in enterBody.
			default:
				m.scratch.memoSet(frame.node, frame.hash)
				m.scratch.activePop(frame.activeNode)
			}
			m.complete(frame, frame.hash)
			return
		}
		op := frame.ops[frame.next]
		frame.next++
		if !op.isChild {
			frame.hash = hash.MixHash(frame.hash, op.value)
			return
		}
		frame.phase = hashFinishChild
		m.push(hashBodyNode, op.child)
	case hashFinishChild:
		frame.hash = hash.MixHash(frame.hash, frame.child)
		frame.phase = hashRunOps
	}
}

func (m *hashMachine) enter(frame *hashFrame) {
	if !m.scratch.checkpoint() {
		m.complete(frame, 0)
		return
	}
	if frame.mode == hashWithNode {
		m.enterWith(frame)
		return
	}
	m.enterBody(frame)
}

func (m *hashMachine) enterWith(frame *hashFrame) {
	if frame.input == nil {
		m.complete(frame, 0)
		return
	}
	if rec, ok := frame.input.(*Recursive); ok {
		if m.scratch.visitedContains(rec) {
			m.scratch.sawCycle = true
			m.complete(frame, hash.MixHash(uint64(kind.Recursive), hash.FnvString("$self")))
			return
		}
		if value, ok := m.scratch.memoGet(rec); ok {
			m.complete(frame, value)
			return
		}
		m.scratch.visitedPush(rec)
		frame.rec = rec
		// The binder name is presentation, so it is not hashed: equality joins
		// two spellings of one fixed point and the hash has to join them too.
		frame.hash = hash.MixHash(uint64(kind.Recursive), 0)
		if rec.Body == nil {
			m.scratch.visitedPop(rec)
			m.scratch.memoSet(rec, frame.hash)
			m.complete(frame, frame.hash)
			return
		}
		frame.phase = hashFinishRecursive
		m.push(hashBodyNode, rec.Body)
		return
	}
	if g, ok := frame.input.(*Generic); ok {
		// A self-referential generic declaration (e.g. List<T> = {..., tail:
		// List<T>}) is reached again through its own Body during this walk.
		// Anchoring the cycle break on g itself, rather than on whichever
		// Instantiated wrapper is active, makes the closed hash the same
		// regardless of which application in the component started the walk.
		if m.scratch.visitedContainsGeneric(g) {
			m.scratch.sawCycle = true
			m.complete(frame, hash.MixHash(uint64(kind.Generic), hash.FnvString("$self")))
			return
		}
		if value, ok := m.scratch.memoGet(g); ok {
			m.complete(frame, value)
			return
		}
		m.scratch.visitedPushGeneric(g)
		frame.node = g
		frame.generic = g
		frame.hash, frame.ops = hashNodeOperations(g)
		frame.phase = hashRunOps
		return
	}
	if value, ok := m.scratch.memoGet(frame.input); ok {
		m.complete(frame, value)
		return
	}
	frame.phase = hashFinishWithPlain
	m.push(hashBodyNode, frame.input)
}

func (m *hashMachine) enterBody(frame *hashFrame) {
	value := NormalizeNil(frame.input)
	if value == nil {
		m.complete(frame, 0)
		return
	}
	value = UnwrapTransparentWrappers(value)
	if alias, ok := value.(*Alias); ok {
		frame.phase = hashFinishAlias
		m.push(hashBodyNode, alias.UnaliasedTarget())
		return
	}
	if rec, ok := value.(*Recursive); ok {
		frame.phase = hashFinishAlias
		m.push(hashWithNode, rec)
		return
	}
	if g, ok := value.(*Generic); ok {
		// A closed g whose own computation never crossed a productive cycle
		// (equalityHashCache.interior) hashes the same from every position, so
		// reading its published value directly - instead of re-walking its
		// whole body - is what keeps a chain of N nested declarations linear
		// rather than quadratic. A self-referential g is excluded: entering it
		// from its own body sees itself already active and returns the $self
		// sentinel, which a query rooted at g itself never does, so its value
		// is position-dependent and only trustworthy as its own query root.
		if h, ok := cachedEqualityHashInterior(g); ok {
			m.complete(frame, h)
			return
		}
		frame.phase = hashFinishAlias
		m.push(hashWithNode, g)
		return
	}
	if inst, ok := value.(*Instantiated); ok {
		// An Instantiated node is an application, not a declaration: it never
		// anchors a cycle and is not registered in active. A self-application
		// reachable from its own Generic's Body would otherwise be caught by
		// the general active check below whenever it happens to be the
		// traversal root, producing a different hash than when the same
		// application is reached as an interior node; the Generic branch above
		// is the sole cycle anchor for both cases, so the result is the same
		// regardless of which application started the walk. Its published
		// cache is consulted under the same interior-safety rule as Generic.
		if h, ok := cachedEqualityHashInterior(inst); ok {
			m.complete(frame, h)
			return
		}
		frame.node = inst
		frame.transparent = true
		frame.hash, frame.ops = hashNodeOperations(inst)
		frame.phase = hashRunOps
		return
	}
	// Record/Function reach here (Optional/Union/... have no cache and always
	// miss); same interior-safety rule as Generic and Instantiated.
	if h, ok := cachedEqualityHashInterior(value); ok {
		m.complete(frame, h)
		return
	}
	if result, ok := m.scratch.memoGet(value); ok {
		m.complete(frame, result)
		return
	}
	if m.scratch.activeContains(value) {
		m.scratch.sawCycle = true
		m.complete(frame, hash.MixHash(uint64(value.Kind()), hash.FnvString("$cycle")))
		return
	}
	m.scratch.activePush(value)
	frame.node = value
	frame.activeNode = value
	frame.hash, frame.ops = hashNodeOperations(value)
	frame.phase = hashRunOps
}

func (m *hashMachine) push(mode hashMode, input Type) {
	m.stack = append(m.stack, &hashFrame{mode: mode, phase: hashEnter, input: input})
}

func (m *hashMachine) complete(frame *hashFrame, result uint64) {
	frame.result = result
	last := len(m.stack) - 1
	m.stack = m.stack[:last]
	if len(m.stack) != 0 {
		m.stack[len(m.stack)-1].child = result
	}
}

func hashNodeOperations(t Type) (uint64, []hashOp) {
	child := func(value Type) hashOp { return hashOp{child: value, isChild: true} }
	constant := func(value uint64) hashOp { return hashOp{value: value} }
	switch value := t.(type) {
	case *Optional:
		return uint64(kind.Optional), []hashOp{child(value.Inner)}
	case *Union:
		ops := make([]hashOp, len(value.Members))
		for i, member := range value.Members {
			ops[i] = child(member)
		}
		return uint64(kind.Union), ops
	case *Intersection:
		ops := make([]hashOp, len(value.Members))
		for i, member := range value.Members {
			ops[i] = child(member)
		}
		return uint64(kind.Intersection), ops
	case *Record:
		capacity := len(value.Fields)*4 + len(value.StaticMembers)*6 + 4
		ops := make([]hashOp, 0, capacity)
		for _, field := range value.Fields {
			ops = append(ops, constant(hash.FnvString(field.Name)), child(field.Type))
			if field.Optional {
				ops = append(ops, constant(1))
			}
			if field.Readonly {
				ops = append(ops, constant(2))
			}
		}
		for _, member := range value.StaticMembers {
			ops = append(ops, constant(recordStaticHash), constant(uint64(member.Kind)))
			switch member.Kind {
			case StaticMemberStringIndex:
				ops = append(ops, constant(hash.FnvString(member.Name)))
			case StaticMemberIntIndex:
				ops = append(ops, constant(uint64(member.Index)))
			}
			ops = append(ops, child(member.Type))
			if member.Optional {
				ops = append(ops, constant(1))
			}
			if member.Readonly {
				ops = append(ops, constant(2))
			}
		}
		if value.Metatable != nil {
			ops = append(ops, child(value.Metatable))
		}
		if value.Open {
			ops = append(ops, constant(3))
		}
		if value.HasMapComponent() {
			ops = append(ops, constant(hash.FnvString("$mapKey")), child(value.MapKey), constant(hash.FnvString("$mapValue")), child(value.MapValue))
		}
		return uint64(kind.Record), ops
	case *Array:
		return uint64(kind.Array), []hashOp{child(value.Element)}
	case *Map:
		return uint64(kind.Map), []hashOp{child(value.Key), child(value.Value)}
	case *ReadonlyMap:
		return uint64(kind.ReadonlyMap), []hashOp{child(value.Key), child(value.Value)}
	case *Tuple:
		ops := make([]hashOp, len(value.Elements))
		for i, element := range value.Elements {
			ops[i] = child(element)
		}
		return uint64(kind.Tuple), ops
	case *Function:
		capacity := len(value.TypeParams) + len(value.Params)*3 + len(value.Returns) + 1
		ops := make([]hashOp, 0, capacity)
		for _, parameter := range value.TypeParams {
			ops = append(ops, child(parameter))
		}
		for _, parameter := range value.Params {
			ops = append(ops, child(parameter.Type))
			if parameter.Receiver {
				ops = append(ops, constant(2))
			}
			if parameter.Optional {
				ops = append(ops, constant(1))
			}
		}
		if value.Variadic != nil {
			ops = append(ops, child(value.Variadic))
		}
		for _, result := range value.Returns {
			ops = append(ops, child(result))
		}
		return uint64(kind.Function), ops
	case *Meta:
		return uint64(kind.Meta), []hashOp{child(value.Of)}
	case *Generic:
		ops := make([]hashOp, 0, len(value.TypeParams)+1)
		for _, parameter := range value.TypeParams {
			ops = append(ops, child(parameter))
		}
		if value.Body != nil {
			ops = append(ops, child(value.Body))
		}
		return hash.MixHash(uint64(kind.Generic), hash.FnvString(value.Name)), ops
	case *Instantiated:
		ops := make([]hashOp, 0, len(value.TypeArgs)+1)
		ops = append(ops, child(value.Generic))
		for _, argument := range value.TypeArgs {
			ops = append(ops, child(argument))
		}
		return uint64(kind.Instantiated), ops
	case *TypeParam:
		ops := make([]hashOp, 0, 1)
		if value.Constraint != nil {
			ops = append(ops, child(value.Constraint))
		}
		// A formal's spelling is presentation: identity is (binder, ordinal),
		// so the name is absent here exactly as it is absent from equality.
		return uint64(kind.TypeParam), ops
	case *Interface:
		ops := make([]hashOp, 0, len(value.Methods)*2)
		for _, method := range value.Methods {
			ops = append(ops, constant(hash.FnvString(method.Name)), child(method.Type))
		}
		return hash.MixHash(uint64(kind.Interface), hash.FnvString(value.Name)), ops
	default:
		return t.Hash(), nil
	}
}
