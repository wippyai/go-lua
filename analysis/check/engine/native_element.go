package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Element classes a container's element domain can carry. They name the layout
// a native consumer may assume for every slot of the published prefix, so a
// class is published only when every store the body owns resolves to it.
const (
	nativeElementNumber = "number"
	nativeElementRecord = "record"
)

// nativeElementOrigin is a container whose array part one literal established
// as a dense prefix of a single element class. constructor is the instruction
// index that establishes it in the owning body, and is negative for a container
// this body inherited from an enclosing lexical body.
type nativeElementOrigin struct {
	class       string
	capacity    int
	constructor int
}

// elementNativeFacts projects the element domain of the containers the resolved
// WIR already establishes. It publishes three contracts, each anchored at the
// operation that establishes it and each carrying the deopt classes that end
// it: the element class of a loop-filled container, the per-element ownership
// of a pointer element domain, and the raw element address a constant-index
// read may take. Every input is lowering-owned topology; an unresolved producer,
// an intervening opaque call, or a non-uniform store leaves the row absent.
//
// Nested lexical bodies are independently frozen WIR publications and are
// visited exactly as the table projection visits them. A container established
// by an enclosing body travels down that walk so a child body can read an
// element of shared state it did not construct.
func elementNativeFacts(root front.Compilation) []NativeFact {
	var rows []NativeFact
	var visit func(front.Compilation, map[wir.SymbolID]nativeElementOrigin)
	visit = func(compilation front.Compilation, inherited map[wir.SymbolID]nativeElementOrigin) {
		rows = append(rows, elementBodyFacts(compilation, inherited)...)
		nested := nativeInheritedElementOrigins(compilation.WIR, inherited)
		for _, child := range compilation.Nested {
			visit(child, nested)
		}
	}
	visit(root, nil)
	return rows
}

func elementBodyFacts(compilation front.Compilation, inherited map[wir.SymbolID]nativeElementOrigin) []NativeFact {
	if compilation.WIR == nil {
		return nil
	}
	rows := elementDomainFacts(compilation)
	rows = append(rows, elementAddressFacts(compilation, inherited)...)
	return append(rows, guardedElementFacts(compilation)...)
}

// nativeElementRow builds one element contract. The revocation set travels in
// the key as a comma-separated suffix, so one contract stays one row with a set
// of deopt classes rather than one row per class.
func nativeElementRow(compilation front.Compilation, occurrence, discriminator, subject, value string, events []string) NativeFact {
	key := "table_element/" + fmt.Sprintf("%x", compilation.Body) + "/" + occurrence
	if discriminator != "" {
		key += "/" + discriminator
	}
	key += "/contract-revocation/" + strings.Join(events, ",")
	return NativeFact{
		Lane: NativeLaneValues, Family: "table_element", Key: key,
		Value: value, Subject: subject, Occurrence: occurrence, Trust: NativeTrustProven,
	}
}

// elementDomainFacts publishes the element class of a container whose whole
// element domain is written by one unit-stride numeric loop starting at 1. A
// pointer element domain additionally publishes its ownership mode and the
// write-barrier obligation, because a consumer may not install one without the
// other.
func elementDomainFacts(compilation front.Compilation) []NativeFact {
	body := compilation.WIR
	var rows []NativeFact
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpMakeTable || instruction.ListSpread || instruction.Dst.Kind != wir.OperandPath {
			continue
		}
		// A constructor that already carries entries owns element stores the
		// loop does not, so the loop is not the complete writer set.
		if len(body.TableEntries(instruction.TableEntries)) != 0 || len(body.Operands(instruction.List)) != 0 {
			continue
		}
		container := body.Path(wir.PathRef(instruction.Dst.Ref))
		class, uniform := nativeLoopFilledClass(body, container)
		if !uniform {
			continue
		}
		value := "element_class=" + class
		events := []string{"write.element", "write.length"}
		if nativeElementWritersClosed(body, container) {
			value += " mutations_closed=true"
			events = append(events, "meta.set")
		}
		value += " presence=dense_prefix"
		if nativeElementEscapes(body, container) {
			events = append(events, "call.opaque", "escape")
		}
		if class == nativeElementRecord {
			events = append(events, "shape.transition")
		}
		occurrence := nativeOccurrence(index)
		rows = append(rows, nativeElementRow(compilation, occurrence, "", container.String(), value, events))
		if class != nativeElementRecord {
			continue
		}
		// The ownership row states nothing about the sequence border or about
		// raw dispatch, so it carries neither the length nor the metatable class.
		rows = append(rows, nativeElementRow(compilation, occurrence, "pointer", container.String(),
			"element_class=record ownership=move write_barrier=required",
			[]string{"write.element", "call.opaque", "escape", "shape.transition"}))
	}
	return rows
}

// nativeLoopFilledClass reports the single class every element store into the
// container writes, when exactly one unit-stride numeric loop from 1 owns those
// stores. A store whose value has no resolved producer, a second class, or an
// element write outside the loop leaves the domain unpublished.
func nativeLoopFilledClass(body *wir.Body, container path.Path) (string, bool) {
	if !nativeDenseLoopWrite(body, container, body.Len()) {
		return "", false
	}
	class := ""
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpStaticMemberWrite:
			if instruction.Dst.Kind != wir.OperandPath {
				continue
			}
			member := body.Path(wir.PathRef(instruction.Dst.Ref))
			if root, indexed := nativeIndexedParent(member); indexed && root.EqualIgnoringVersion(container) {
				return "", false
			}
		case wir.OpDynamicIndexWrite:
			if instruction.Dst.Kind != wir.OperandPath || !body.Path(wir.PathRef(instruction.Dst.Ref)).EqualIgnoringVersion(container) {
				continue
			}
			if !nativeUnitStrideLoopIndex(body, instruction.A) {
				return "", false
			}
			stored := nativeElementValueClass(body, instruction.B, 0)
			if stored == "" || (class != "" && stored != class) {
				return "", false
			}
			class = stored
		}
	}
	return class, class != ""
}

// elementAddressFacts publishes the raw address of a constant element index
// read out of a container this compilation owns. The address is published only
// where the container's validity is intact at the read: an opaque call, a
// select, or a suspension between the establishing store and the read leaves
// the fact unpublished rather than reestablished.
func elementAddressFacts(compilation front.Compilation, inherited map[wir.SymbolID]nativeElementOrigin) []NativeFact {
	body := compilation.WIR
	local := nativeElementLiterals(body)
	suspends := nativeElementSuspends(body)
	published := make(map[string]bool)
	var rows []NativeFact
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		for _, operand := range nativeElementReadOperands(body, instruction) {
			member := body.Path(wir.PathRef(operand.Ref))
			container, indexed := nativeIndexedParent(member)
			if !indexed || len(container.Segments) != 0 {
				continue
			}
			last, _ := member.LastSegment()
			origin, external, found := nativeElementOriginOf(local, inherited, container)
			if !found || last.Index < 1 || last.Index > origin.capacity {
				continue
			}
			if !nativeElementAddressIntact(body, container, origin, external, index) {
				continue
			}
			events := []string{"write.element", "write.length", "grow"}
			if nativeElementGrowthStore(body, container) {
				events = append(events, "gc.relocate")
			}
			if external || nativeElementEscapes(body, container) {
				events = append(events, "call.opaque", "escape")
			}
			// Shared state another actor can observe does not survive a
			// suspension: the element class it carried is not this body's to keep.
			if external && suspends {
				events = append(events, "suspend")
			}
			row := nativeElementRow(compilation, nativeOccurrence(index), "address/"+strconv.Itoa(last.Index),
				container.String(), "address=raw element_class="+origin.class+" presence=dense_prefix", events)
			if published[row.Key] {
				continue
			}
			published[row.Key] = true
			rows = append(rows, row)
		}
	}
	return rows
}

// nativeElementReadOperands names the path operands an instruction reads. A
// write destination is deliberately excluded: it addresses a slot the operation
// stores into rather than a value it loads.
func nativeElementReadOperands(body *wir.Body, instruction wir.Instruction) []wir.Operand {
	operands := make([]wir.Operand, 0, 4+instruction.List.Len)
	for _, operand := range []wir.Operand{instruction.A, instruction.B, instruction.Call.Callee, instruction.Call.Receiver} {
		if operand.Kind == wir.OperandPath {
			operands = append(operands, operand)
		}
	}
	for _, operand := range body.Operands(instruction.List) {
		if operand.Kind == wir.OperandPath {
			operands = append(operands, operand)
		}
	}
	return operands
}

// nativeElementAddressIntact reports that the container's backing storage is
// the one the establishing operation produced. A call or a select can run
// arbitrary code over the container and ends the interval; an element store the
// body owns opens a fresh one.
func nativeElementAddressIntact(body *wir.Body, container path.Path, origin nativeElementOrigin, external bool, read int) bool {
	established := external
	for index := 0; index < read; index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpCall, wir.OpSelect:
			established = false
		case wir.OpMakeTable:
			if index == origin.constructor {
				established = true
			}
		case wir.OpDynamicIndexWrite:
			if instruction.Dst.Kind == wir.OperandPath && body.Path(wir.PathRef(instruction.Dst.Ref)).EqualIgnoringVersion(container) {
				established = true
			}
		case wir.OpStaticMemberWrite:
			if instruction.Dst.Kind != wir.OperandPath {
				continue
			}
			if root, indexed := nativeIndexedParent(body.Path(wir.PathRef(instruction.Dst.Ref))); indexed && root.EqualIgnoringVersion(container) {
				established = true
			}
		}
	}
	return established
}

// nativeElementGrowthStore reports an element store whose index the body does
// not carry as a dense loop position. Such a store can require new backing
// storage, so an address taken around it survives only across a relocation
// guard.
func nativeElementGrowthStore(body *wir.Body, container path.Path) bool {
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpDynamicIndexWrite || instruction.Dst.Kind != wir.OperandPath {
			continue
		}
		if !body.Path(wir.PathRef(instruction.Dst.Ref)).EqualIgnoringVersion(container) {
			continue
		}
		if !nativeUnitStrideLoopIndex(body, instruction.A) {
			return true
		}
	}
	return false
}

// nativeGuardedElementDeopts is the deopt vocabulary of a guarded element read.
// The contract names the element domain, the array border, the index binding the
// guard proved a bound for, raw index dispatch, and any callee that can reach
// the container. Both publishers of the family speak this one set, so a consumer
// reads the same contract whichever substrate proved the presence.
var nativeGuardedElementDeopts = []string{"write.element", "write.length", "write.local", "meta.set", "call.opaque"}

// guardedElementFacts publishes the result of an indexed read into an
// opaque-origin table with a declared array element type. A proven floor of one
// is the whole contract when no upper bound is proven: presence stays withheld
// and the read remains optional.
func guardedElementFacts(compilation front.Compilation) []NativeFact {
	body := compilation.WIR
	types := nativePathTypes(body)
	var rows []NativeFact
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpDynamicIndexRead || instruction.A.Kind != wir.OperandPath || instruction.B.Kind != wir.OperandPath {
			continue
		}
		container := body.Path(wir.PathRef(instruction.A.Ref))
		key := body.Path(wir.PathRef(instruction.B.Ref))
		if len(container.Segments) != 0 || len(key.Segments) != 0 || nativeMadeTable(body, container) {
			continue
		}
		if !nativeDeclaredArray(container, types) {
			continue
		}
		floor, ceiling := nativeElementIndexBounds(body, container, key, index)
		if !floor {
			continue
		}
		value := "result_nilability=maybe_nil"
		if ceiling {
			value = "presence=proven result_nilability=non_nil"
		}
		rows = append(rows, nativeElementRow(compilation, nativeOccurrence(index), "guarded", container.String(), value,
			nativeGuardedElementDeopts))
	}
	return rows
}

// nativeElementIndexBounds reads the bounds the normalized guard descriptor
// proves for the index on the edge the read is lowered onto. The guard is the
// branch the read follows, and the run between them must touch neither the
// index nor the container: a call, a loop, a further branch, or a rebinding
// leaves the read unguarded here rather than guarded by an older proof.
func nativeElementIndexBounds(body *wir.Body, container, key path.Path, read int) (floor, ceiling bool) {
	guard := -1
	for index := read - 1; index >= 0; index-- {
		if body.Instr(index).Op == wir.OpBranch {
			guard = index
			break
		}
	}
	if guard < 0 {
		return false, false
	}
	for index := guard + 1; index < read; index++ {
		if !nativeElementGuardTransparent(body, body.Instr(index), container, key) {
			return false, false
		}
	}
	consider := func(check wir.Check) {
		if check.Negated {
			return
		}
		switch check.Kind {
		case wir.CheckNumGe:
			if check.Path.EqualIgnoringVersion(key) && check.NumFloor >= 1 {
				floor = true
			}
		case wir.CheckIndexInRange:
			if check.Path.EqualIgnoringVersion(key) && check.OtherPath.EqualIgnoringVersion(container) {
				ceiling = true
			}
		}
	}
	instruction := body.Instr(guard)
	consider(body.Check(instruction.Check))
	for _, implied := range body.ImpliedChecks(instruction.ImpliedChecks) {
		if implied.Edge && implied.Polarity {
			consider(implied.Check)
		}
	}
	return floor, ceiling
}

func nativeElementGuardTransparent(body *wir.Body, instruction wir.Instruction, container, key path.Path) bool {
	switch instruction.Op {
	case wir.OpCall, wir.OpSelect, wir.OpIterate, wir.OpReturn, wir.OpBranch, wir.OpClosure:
		return false
	}
	if instruction.Dst.Kind != wir.OperandPath {
		return true
	}
	target := body.Path(wir.PathRef(instruction.Dst.Ref))
	return !target.SameRootIgnoringVersion(container) && !target.SameRootIgnoringVersion(key)
}

func nativeDeclaredArray(container path.Path, types map[string]typ.Type) bool {
	value, found := types[string(container.Key())]
	if !found || value == nil {
		return false
	}
	array, ok := unwrap.Alias(value).(*typ.Array)
	return ok && array.Element != nil
}

// nativeElementSuspends reports a point where another actor can run: a
// recognized select, or a method call on a value carrying the runtime channel
// ABI. The receiver's resolved type is the authority, never the method name.
func nativeElementSuspends(body *wir.Body) bool {
	types := nativePathTypes(body)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpSelect {
			return true
		}
		if instruction.Op != wir.OpCall || instruction.Call.Method == 0 || instruction.Call.Receiver.Kind != wir.OperandPath {
			continue
		}
		receiver := body.Path(wir.PathRef(instruction.Call.Receiver.Ref))
		value, found := types[string(receiver.Key())]
		if !found {
			continue
		}
		if _, channel := ambient.ChannelPayloadType(value); channel {
			return true
		}
	}
	return false
}

// nativeElementLiterals indexes every container a literal in this body built
// with a dense array part of one element class. A binding a body constructs
// twice carries no single origin and is withheld.
func nativeElementLiterals(body *wir.Body) map[string]nativeElementOrigin {
	origins := make(map[string]nativeElementOrigin)
	rebuilt := make(map[string]bool)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpMakeTable || instruction.ListSpread || instruction.Dst.Kind != wir.OperandPath {
			continue
		}
		container := body.Path(wir.PathRef(instruction.Dst.Ref))
		name := string(container.Key())
		if _, known := origins[name]; known {
			delete(origins, name)
			rebuilt[name] = true
		}
		if rebuilt[name] {
			continue
		}
		array, keys, exact := nativeTableCapacity(body, instruction)
		values := body.Operands(instruction.List)
		if !exact || array == 0 || keys != 0 || len(values) != array {
			continue
		}
		class := ""
		for _, value := range values {
			stored := nativeElementValueClass(body, value, 0)
			if stored == "" || (class != "" && stored != class) {
				class = ""
				break
			}
			class = stored
		}
		if class == "" {
			continue
		}
		origins[name] = nativeElementOrigin{class: class, capacity: array, constructor: index}
	}
	return origins
}

// nativeInheritedElementOrigins carries the containers an enclosing body
// established down to its nested bodies, keyed by the lexical symbol the child
// captures. Only a container this body leaves untouched after construction
// travels: a later store or a call that can reach it revokes the origin here
// rather than inside the child.
func nativeInheritedElementOrigins(body *wir.Body, inherited map[wir.SymbolID]nativeElementOrigin) map[wir.SymbolID]nativeElementOrigin {
	if body == nil {
		return inherited
	}
	nested := make(map[wir.SymbolID]nativeElementOrigin, len(inherited))
	for symbol, origin := range inherited {
		nested[symbol] = origin
	}
	for name, origin := range nativeElementLiterals(body) {
		container := body.Instr(origin.constructor)
		if container.Dst.Kind != wir.OperandPath {
			continue
		}
		item := body.Path(wir.PathRef(container.Dst.Ref))
		if item.Symbol == 0 || string(item.Key()) != name || !nativeElementStable(body, item) {
			continue
		}
		nested[item.Symbol] = nativeElementOrigin{class: origin.class, capacity: origin.capacity, constructor: -1}
	}
	return nested
}

// nativeElementStable reports that the body performs no element store into the
// container and hands it to no callee after building it.
func nativeElementStable(body *wir.Body, container path.Path) bool {
	if nativeTableEscapesAt(body, container, body.Len()) {
		return false
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpDynamicIndexWrite && instruction.Op != wir.OpStaticMemberWrite {
			continue
		}
		if instruction.Dst.Kind != wir.OperandPath {
			continue
		}
		target := body.Path(wir.PathRef(instruction.Dst.Ref))
		if target.SameRootIgnoringVersion(container) {
			return false
		}
	}
	return true
}

func nativeElementOriginOf(local map[string]nativeElementOrigin, inherited map[wir.SymbolID]nativeElementOrigin, container path.Path) (nativeElementOrigin, bool, bool) {
	if origin, found := local[string(container.Key())]; found {
		return origin, false, true
	}
	if container.Symbol == 0 {
		return nativeElementOrigin{}, false, false
	}
	origin, found := inherited[container.Symbol]
	return origin, found, found
}

// nativeElementEscapes reports that the container leaves this body's control:
// an opaque callee receives it, a return or a closure carries it out, or it is
// a global the whole program can name.
func nativeElementEscapes(body *wir.Body, container path.Path) bool {
	if container.Symbol != 0 && body.IsImplicitGlobalSymbol(container.Symbol) {
		return true
	}
	if nativeTableEscapesAt(body, container, body.Len()) {
		return true
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpReturn && instruction.Op != wir.OpClosure {
			continue
		}
		for _, operand := range body.Operands(instruction.List) {
			if operand.Kind == wir.OperandPath && body.Path(wir.PathRef(operand.Ref)).EqualIgnoringVersion(container) {
				return true
			}
		}
	}
	return false
}

// nativeElementWritersClosed reports that no opaque callee can join the writer
// set of the container's element domain: no call in this body receives it, no
// closure captures it, and it is not a global. A container the body only
// returns keeps a closed writer set for the interval the escape revokes.
func nativeElementWritersClosed(body *wir.Body, container path.Path) bool {
	if container.Symbol != 0 && body.IsImplicitGlobalSymbol(container.Symbol) {
		return false
	}
	return !nativeTableEscapesAt(body, container, body.Len()) && !nativePathIsCaptured(body, container)
}

// nativeElementValueClass classifies one stored value by the producer this body
// owns for it. Anything the body does not produce, or produces more than once,
// leaves the class unresolved.
func nativeElementValueClass(body *wir.Body, operand wir.Operand, depth int) string {
	if depth > 8 {
		return ""
	}
	if operand.Kind == wir.OperandConst {
		if body.Const(wir.ConstRef(operand.Ref)).Kind == wir.ConstNumber {
			return nativeElementNumber
		}
		return ""
	}
	if nativeLoopIndex(body, operand) {
		return nativeElementNumber
	}
	producer, found := nativeElementProducer(body, operand)
	if !found {
		return ""
	}
	switch producer.Op {
	case wir.OpMakeTable:
		if nativeFreshRecord(body, producer) {
			return nativeElementRecord
		}
	case wir.OpBinOp:
		if nativeArithmeticOperator(producer.Operator) &&
			nativeElementValueClass(body, producer.A, depth+1) == nativeElementNumber &&
			nativeElementValueClass(body, producer.B, depth+1) == nativeElementNumber {
			return nativeElementNumber
		}
	case wir.OpUnOp:
		switch producer.Operator {
		case wir.UnLen:
			return nativeElementNumber
		case wir.UnNeg, wir.UnBNot:
			return nativeElementValueClass(body, producer.A, depth+1)
		}
	case wir.OpAssign:
		if producer.A.Kind != wir.OperandNone {
			return nativeElementValueClass(body, producer.A, depth+1)
		}
	}
	return ""
}

// nativeElementProducer names the single instruction this body writes an
// operand with. Two writers leave the operand without one producer, so the
// value it carries is not this projection's to classify.
func nativeElementProducer(body *wir.Body, operand wir.Operand) (wir.Instruction, bool) {
	if operand.Kind != wir.OperandTemp && operand.Kind != wir.OperandPath {
		return wir.Instruction{}, false
	}
	var producer wir.Instruction
	found := false
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpStaticMemberWrite || instruction.Op == wir.OpDynamicIndexWrite {
			continue
		}
		if instruction.Dst != operand {
			continue
		}
		if found {
			return wir.Instruction{}, false
		}
		producer, found = instruction, true
	}
	return producer, found
}

// nativeFreshRecord reports a constructor that allocates a record at the store
// site: a complete static string key set and no array part at all.
func nativeFreshRecord(body *wir.Body, instruction wir.Instruction) bool {
	if instruction.ListSpread || !instruction.StaticStringKeysComplete {
		return false
	}
	array, keys, exact := nativeTableCapacity(body, instruction)
	return exact && keys > 0 && array == 0
}

func nativeArithmeticOperator(operator wir.Operator) bool {
	switch operator {
	case wir.BinAdd, wir.BinSub, wir.BinMul, wir.BinDiv, wir.BinIDiv, wir.BinMod, wir.BinPow,
		wir.BinBAnd, wir.BinBOr, wir.BinBXor, wir.BinShl, wir.BinShr:
		return true
	default:
		return false
	}
}

// nativeUnitStrideLoopIndex reports an iteration variable of a numeric loop
// that walks 1, 2, 3, ... — the only shape whose stores fill a dense prefix.
func nativeUnitStrideLoopIndex(body *wir.Body, operand wir.Operand) bool {
	if !nativeLoopIndex(body, operand) {
		return false
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpIterate || instruction.Iter != wir.IterNumeric {
			continue
		}
		values := body.Operands(instruction.List)
		if len(values) != 3 || nativeIntegerConstant(body, values[0]) != 1 || nativeIntegerConstant(body, values[2]) != 1 {
			continue
		}
		for _, result := range body.Operands(instruction.Results) {
			if result == operand {
				return true
			}
		}
	}
	return false
}
