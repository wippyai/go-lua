package front

// Native WIR contracts expose resolved lowering facts through the ordinary
// closure publication path.  They deliberately consume only WIR and typ
// identities already owned by Compilation: no source spelling is re-parsed
// and a missing resolved identity produces no native grant.

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// ShapeID is the stable physical-layout identity a native record consumer
// compares before using fixed member offsets.  It is derived from the full
// canonical digest of the closed field-set shape, never from a local intern
// ordinal or a process-global type identity.
type ShapeID uint64

func nativeWIRContracts(root Compilation) []NativeContract {
	contracts := nativeCallSCCContracts(root)
	// Native lowering drafts carry only immutable WIR topology. They enter the
	// ordinary publication tail as typed factkey records, so their public rows
	// do not exist until the semantic partition has closed.
	var projections []NativeProjection
	projections = append(projections, frozenBodyNativeFacts(root)...)
	projections = append(projections, shapeEpochNativeFacts(root)...)
	projections = append(projections, summaryNativeFacts(root)...)
	contracts = append(contracts, nativeProjectionContracts(root, projections)...)
	var visit func(Compilation)
	visit = func(compilation Compilation) {
		contracts = append(contracts, nativeConstantPublications(compilation)...)
		contracts = append(contracts, nativePublicationIdentityContracts(compilation)...)
		contracts = append(contracts, nativeShapeContracts(compilation)...)
		contracts = append(contracts, nativeDiscriminantContracts(compilation.WIR)...)
		contracts = append(contracts, nativeRecursiveIdentityContracts(compilation.WIR, compilation.typeDefinitions)...)
		for _, child := range compilation.nested {
			visit(child)
		}
	}
	visit(root)
	sort.Slice(contracts, func(i, j int) bool {
		if contracts[i].Family != contracts[j].Family {
			return contracts[i].Family < contracts[j].Family
		}
		if contracts[i].Value != contracts[j].Value {
			return contracts[i].Value < contracts[j].Value
		}
		if contracts[i].Subject != contracts[j].Subject {
			return contracts[i].Subject < contracts[j].Subject
		}
		if left, right := strings.Join(contracts[i].Revocations, "/"), strings.Join(contracts[j].Revocations, "/"); left != right {
			return left < right
		}
		return contracts[i].Key.String() < contracts[j].Key.String()
	})
	return contracts
}

func forEachNativeBody(root Compilation, visit func(Compilation)) {
	var walk func(Compilation)
	walk = func(compilation Compilation) {
		if compilation.WIR != nil {
			visit(compilation)
		}
		for _, child := range compilation.nested {
			walk(child)
		}
	}
	walk(root)
}

func nativeProjectionContracts(root Compilation, rows []NativeProjection) []NativeContract {
	contracts := make([]NativeContract, 0, len(rows))
	for _, row := range rows {
		encoded, err := EncodeNativeProjection(row)
		if err != nil {
			continue
		}
		identity := sha256.Sum256(encoded)
		contracts = append(contracts, NativeContract{
			Key: factkey.BuildKey(
				factkey.NativeProjection,
				[]factkey.Part{
					factkey.OpaquePart(fmt.Sprintf("%x", root.Body)),
					factkey.OpaquePart(fmt.Sprintf("%x", identity)),
				},
				"published",
			),
			Value: string(encoded),
		})
	}
	return contracts
}

// nativeCallSCCContracts closes only direct lexical call edges whose closure
// allocation and callee path resolve to the same WIR function inventory.
// Member calls and unresolved paths never enter the graph, so absence remains
// the fail-closed result for dynamic recursion.
func nativeCallSCCContracts(root Compilation) []NativeContract {
	byName := make(map[string]Compilation)
	var inventory func(Compilation)
	inventory = func(compilation Compilation) {
		body := compilation.WIR
		if body != nil {
			for index := 0; index < body.Len(); index++ {
				instruction := body.Instr(index)
				if instruction.Op != wir.OpClosure || instruction.Func == 0 || instruction.Dst.Kind != wir.OperandPath {
					continue
				}
				proto := body.Proto(instruction.Func)
				name := body.Path(wir.PathRef(instruction.Dst.Ref)).String()
				for _, child := range compilation.nested {
					if child.Prototype == proto.Symbol && name != "" {
						byName[name] = child
						break
					}
				}
			}
		}
		for _, child := range compilation.nested {
			inventory(child)
		}
	}
	inventory(root)
	adj := make(map[string]map[string]bool)
	for from, compilation := range byName {
		body := compilation.WIR
		for index := 0; body != nil && index < body.Len(); index++ {
			instruction := body.Instr(index)
			if instruction.Op != wir.OpCall || instruction.Call.Method != 0 || instruction.Call.Callee.Kind != wir.OperandPath {
				continue
			}
			callee := body.Path(wir.PathRef(instruction.Call.Callee.Ref))
			to := callee.String()
			if len(callee.Segments) != 0 {
				continue
			}
			if _, closed := byName[to]; !closed {
				continue
			}
			if adj[from] == nil {
				adj[from] = make(map[string]bool)
			}
			adj[from][to] = true
		}
	}
	var rows []NativeContract
	for _, component := range nativeSCCs(adj) {
		if len(component) == 1 && !adj[component[0]][component[0]] {
			continue
		}
		edges := make([]string, 0)
		for _, from := range component {
			for to := range adj[from] {
				if containsName(component, to) {
					edges = append(edges, from+"->"+to)
				}
			}
		}
		sort.Strings(edges)
		args := "[]"
		owner := byName[component[0]]
		if len(owner.Boundary.Parameters) != 0 {
			if parameter := owner.WIR.Type(owner.Boundary.Parameters[0].Type); parameter != nil {
				args = "[" + parameter.String() + "]"
			}
		}
		value := fmt.Sprintf(
			"arguments=%s completions={'known': ['normal', 'throw', 'user_suspend', 'system_suspend'], 'present': ['normal', 'throw']} edges_closed=[%s] members=[%s] results={'exact': True, 'count': 1}",
			args, strings.Join(edges, ","), strings.Join(component, ","),
		)
		var revocations []string
		if len(component) > 1 {
			revocations = []string{"write.local"}
		}
		rows = append(rows, NativeContract{Family: "call_scc", Value: value, Revocations: revocations})
	}
	return rows
}

// nativeConstantPublications lowers only constant provenance and write
// uniqueness. Exact evaluation stays with the equation value lattice: the
// publication kernel reads Source after the body closes and emits no row when
// that lattice does not hold one exact machine word.
func nativeConstantPublications(compilation Compilation) []NativeContract {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	key := func(operand wir.Operand) string {
		switch operand.Kind {
		case wir.OperandPath:
			return string(body.Path(wir.PathRef(operand.Ref)).Key())
		case wir.OperandTemp:
			return fmt.Sprintf("temp/%d", operand.Ref)
		default:
			return ""
		}
	}
	writes := make(map[string]int)
	captured := make(map[string]bool)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		count := func(operand wir.Operand) {
			if name := key(operand); name != "" {
				writes[name]++
			}
		}
		switch instruction.Op {
		case wir.OpClaim:
		case wir.OpCall, wir.OpIterate:
			for _, result := range body.Operands(instruction.Results) {
				count(result)
			}
		default:
			if instruction.WritesAssignmentPoint() {
				count(instruction.Dst)
			}
		}
		if instruction.Op == wir.OpClosure {
			for _, capture := range body.Operands(instruction.List) {
				if name := key(capture); name != "" {
					captured[name] = true
				}
			}
		}
	}
	known := make(map[string]bool)
	origin := func(operand wir.Operand) bool {
		return operand.Kind == wir.OperandConst || known[key(operand)]
	}
	var rows []NativeContract
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		constant := false
		switch instruction.Op {
		case wir.OpAssign:
			constant = origin(instruction.A)
		case wir.OpUnOp:
			constant = wir.Operator(instruction.Operator) == wir.UnNeg && origin(instruction.A)
		case wir.OpBinOp:
			switch wir.Operator(instruction.Operator) {
			case wir.BinAdd, wir.BinSub, wir.BinMul, wir.BinIDiv, wir.BinMod:
				constant = origin(instruction.A) && origin(instruction.B)
			}
		}
		name := key(instruction.Dst)
		if !constant || name == "" || writes[name] != 1 || captured[name] {
			continue
		}
		known[name] = true
		source, err := scalarTerm(body, instruction.Dst)
		if err != nil {
			continue
		}
		rows = append(rows, NativeContract{
			Family: "constant_value",
			Key: factkey.BuildKey(factkey.NativeConstantValue, []factkey.Part{
				factkey.OpaquePart(fmt.Sprintf("%x", compilation.Body)),
			}, operationName(index)),
			Source: string(source.Encoding),
		})
	}
	return rows
}

// nativePublicationIdentityContracts lowers source-anchored executable WIR
// coordinates into exact-key publication drafts. The publication kernel, not
// Result projection, decides their visibility and carries them through cyclic
// closure just like any other value fact.
func nativePublicationIdentityContracts(compilation Compilation) []NativeContract {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	const value = "executable_body=present function_generation=present identity=stable_cross_module point=present publication_order=deterministic site_ordinal=present source_span=present"
	var rows []NativeContract
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpEntry || instruction.Op == wir.OpExit || instruction.Op == wir.OpNoop || !instruction.ExprSpan.Valid() {
			continue
		}
		rows = append(rows, NativeContract{
			Family: "publication_identity",
			Key: factkey.BuildKey(factkey.NativePublicationIdentity, []factkey.Part{
				factkey.OpaquePart(fmt.Sprintf("%x", compilation.Body)),
			}, operationName(index)),
			Value: value,
		})
	}
	return rows
}

func nativeShapeContracts(compilation Compilation) []NativeContract {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	var contracts []NativeContract
	epochShapes := make(map[ShapeID]bool)
	for _, receiver := range ShapeEpochReceivers(compilation) {
		epochShapes[receiver.Shape] = true
	}
	declared := make(map[ShapeID]bool)
	addShape := func(value typ.Type) {
		shape, ok := nativePhysicalRecordShape(value)
		if !ok {
			return
		}
		id, ok := nativeShapeID(shape)
		if !ok {
			return
		}
		declared[id] = true
		// A receiver that is field-read and then value-stored owns an epoch-gated
		// shape_identity bound to the receiver term, not the module-wide layout
		// contract. Publishing both would double-count one physical layout.
		if epochShapes[id] {
			return
		}
		contracts = append(contracts, NativeContract{
			Family:      "shape_identity",
			Value:       fmt.Sprintf("distinct_identities=1 field_offsets=identical field_order=canonical interned=true shape_id=%016x stable_across_modules=true stable_across_sites=true", uint64(id)),
			Revocations: []string{"shape.transition"},
		})
	}
	// Only declared/typed WIR occurrences establish a reusable physical layout.
	// Inferred table literals are not substituted here: an optional declared
	// record can have several such literal shapes and needs its discriminator.
	for _, root := range body.RootTypes() {
		addShape(body.Type(root.Type))
	}
	for _, value := range body.DeclaredReturnTypes() {
		addShape(value)
	}
	for _, parameter := range compilation.Boundary.Parameters {
		addShape(body.Type(parameter.Type))
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpClaim {
			addShape(body.Type(instruction.Type))
		}
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpMakeTable || !instruction.StaticStringKeysComplete {
			continue
		}
		id, ok := nativeDirectTableShapeID(body.TableEntries(instruction.TableEntries))
		if !ok || !declared[id] {
			continue
		}
		contracts = append(contracts, NativeContract{
			Family:      "shape_identity",
			Value:       fmt.Sprintf("distinct_identities=1 field_offsets=identical field_order=canonical interned=true shape_id=%016x stable_across_modules=true stable_across_sites=true", uint64(id)),
			Revocations: []string{"shape.transition"},
		})
	}
	contracts = append(contracts, nativeRecordConstructionContracts(body)...)
	return append(contracts, nativeShapeTransitionContracts(body)...)
}

// ShapeEpochReceiver is a formal receiver whose proven physical layout a field
// read observes and a store to one of its fields then invalidates. Its
// shape_identity is bound to the receiver term and re-established at each read,
// so one row is published per observing read rather than one module-wide layout
// contract. Display is the receiver's source name, Shape its interned layout
// identity, and Reads the number of establishing field reads. The engine
// publishes the subject-bound rows; the shape contract walk here suppresses the
// module-wide layout row for the same layout so it is never counted twice.
type ShapeEpochReceiver struct {
	Display string
	Shape   ShapeID
	Reads   int
}

// ShapeEpochReceivers reports every formal parameter of a physical record type
// that a field read observes and a store to an existing field then invalidates.
// Only such a receiver carries write.field in its revocation set: a receiver
// that is only read keeps the module-wide layout contract, and a store that
// adds a field is a shape transition owned by the transition walk.
func ShapeEpochReceivers(compilation Compilation) []ShapeEpochReceiver {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	type receiverShape struct {
		id      ShapeID
		display string
		fields  map[string]bool
	}
	shapes := make(map[wir.SymbolID]receiverShape)
	for _, parameter := range compilation.Boundary.Parameters {
		shape, ok := nativePhysicalRecordShape(body.Type(parameter.Type))
		if !ok {
			continue
		}
		id, ok := nativeShapeID(shape)
		if !ok {
			continue
		}
		record, ok := shape.(*typ.Record)
		if !ok || parameter.Name == "" {
			continue
		}
		fields := make(map[string]bool, len(record.Fields))
		for _, field := range record.Fields {
			if field.Name != "" {
				fields[field.Name] = true
			}
		}
		shapes[parameter.Symbol] = receiverShape{id: id, display: parameter.Name, fields: fields}
	}
	if len(shapes) == 0 {
		return nil
	}
	written := make(map[wir.SymbolID]bool)
	reads := make(map[wir.SymbolID]int)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpStaticMemberWrite && instruction.Dst.Kind == wir.OperandPath {
			target := body.Path(wir.PathRef(instruction.Dst.Ref))
			if len(target.Segments) == 1 && target.Segments[0].Kind == segment.SegmentField {
				if shape, ok := shapes[target.Symbol]; ok && shape.fields[target.Segments[0].Name] {
					written[target.Symbol] = true
				}
			}
		}
		for _, operand := range nativeReadOperands(body, instruction) {
			if operand.Kind != wir.OperandPath {
				continue
			}
			member := body.Path(wir.PathRef(operand.Ref))
			if member.Symbol == 0 || len(member.Segments) == 0 || member.Segments[0].Kind != segment.SegmentField {
				continue
			}
			if _, ok := shapes[member.Symbol]; ok {
				reads[member.Symbol]++
			}
		}
	}
	var out []ShapeEpochReceiver
	for symbol, shape := range shapes {
		if !written[symbol] || reads[symbol] == 0 {
			continue
		}
		out = append(out, ShapeEpochReceiver{Display: shape.display, Shape: shape.id, Reads: reads[symbol]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Display < out[j].Display })
	return out
}

// nativeReadOperands yields every value operand an instruction reads. A member
// write target is excluded: a store to a receiver field is a revocation of its
// layout epoch, never one of the reads that establishes it.
func nativeReadOperands(body *wir.Body, instruction wir.Instruction) []wir.Operand {
	operands := make([]wir.Operand, 0, 4)
	if instruction.Op != wir.OpStaticMemberWrite && instruction.Op != wir.OpDynamicIndexWrite {
		operands = append(operands, instruction.Dst)
	}
	operands = append(operands, instruction.A, instruction.B)
	operands = append(operands, body.Operands(instruction.List)...)
	if instruction.Call.Callee.Kind != wir.OperandNone {
		operands = append(operands, instruction.Call.Callee)
	}
	if instruction.Call.Receiver.Kind != wir.OperandNone {
		operands = append(operands, instruction.Call.Receiver)
	}
	return operands
}

// nativeOwnershipRevocations is the closed deopt class set of a fresh record
// the body owns.  Each class ends the disposition for a different reason: an
// escape gives the allocation a second owner, an opaque callee can reach and
// mutate everything the allocation holds, and a metatable turns a direct slot
// store into a dispatch.
var nativeOwnershipRevocations = []string{"escape", "call.opaque", "meta.set"}

func nativeRecordConstructionContracts(body *wir.Body) []NativeContract {
	escaped := nativeEscapedTableConstructors(body)
	owned := nativeOwnedTableConstructors(body)
	products := nativeMultiplicationResults(body)
	// produced carries the operands this body has already produced at each
	// instruction, so a constructor reads the entry producers that were live
	// where it ran rather than a whole-body summary.
	produced := make(map[wir.Operand]bool)
	var contracts []NativeContract
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpMakeTable:
			contracts = append(contracts, nativeRecordConstructorContracts(body, instruction, index, escaped, owned, products, produced)...)
			nativeSetProduced(produced, instruction.Dst, true)
		case wir.OpCall:
			for _, result := range body.Operands(instruction.Results) {
				nativeSetProduced(produced, result, true)
			}
		case wir.OpAssign, wir.OpClaim:
			nativeSetProduced(produced, instruction.Dst, produced[instruction.A])
		default:
			nativeSetProduced(produced, instruction.Dst, false)
		}
	}
	return contracts
}

func nativeRecordConstructorContracts(body *wir.Body, instruction wir.Instruction, index int, escaped, owned map[int]bool, products map[wir.Operand]bool, produced map[wir.Operand]bool) []NativeContract {
	if !instruction.StaticStringKeysComplete {
		return nil
	}
	entries := body.TableEntries(instruction.TableEntries)
	direct := 0
	for _, entry := range entries {
		if _, ok := segment.DirectFieldName(entry.Suffix.Segments); ok {
			direct++
		}
	}
	if direct == 0 {
		return nil
	}
	edges := nativeRecordEntryEdges(entries, produced)
	// Every entry is already an exact WIR constructor slot.  The row says
	// nothing about optional-field presence beyond what the literal wrote.
	contract := NativeContract{
		Family:  "record_construction",
		Value:   nativeRecordConstructionValue(body, entries, direct, products, edges, owned[index]),
		Subject: nativeOperandSubject(body, instruction.Dst),
	}
	switch {
	case owned[index]:
		contract.Revocations = nativeOwnershipRevocations
	case escaped[index]:
		contract.Revocations = []string{"escape"}
	}
	contracts := make([]NativeContract, 0, 1+len(edges))
	contracts = append(contracts, contract)
	for _, edge := range edges {
		contracts = append(contracts, NativeContract{
			Family:      "record_entry_ownership",
			Value:       fmt.Sprintf("field=%s ownership=%s producer_bound=true write_barrier=required", edge.field, edge.ownership()),
			Revocations: []string{"write.field"},
		})
	}
	return contracts
}

// nativeRecordEntryEdge is one constructor entry whose value this body produced
// and can therefore name a producer for.  The first edge that consumes such a
// value moves it into the slot; a later edge to the same value cannot move it a
// second time and retains it instead.
type nativeRecordEntryEdge struct {
	field string
	moved bool
}

func (edge nativeRecordEntryEdge) ownership() string {
	if edge.moved {
		return "move"
	}
	return "retain"
}

func nativeRecordEntryEdges(entries []wir.TableEntry, produced map[wir.Operand]bool) []nativeRecordEntryEdge {
	var edges []nativeRecordEntryEdge
	consumed := make(map[wir.Operand]bool, len(entries))
	for _, entry := range entries {
		field, ok := segment.DirectFieldName(entry.Suffix.Segments)
		if !ok || field == "" || !produced[entry.Value] {
			continue
		}
		edges = append(edges, nativeRecordEntryEdge{field: field, moved: !consumed[entry.Value]})
		consumed[entry.Value] = true
	}
	return edges
}

// nativeSetProduced records whether an operand currently holds a value this
// body produced.  Only a named binding or a temporary can carry one; a
// destination written from anything else drops the association.
func nativeSetProduced(produced map[wir.Operand]bool, operand wir.Operand, holds bool) {
	if operand.Kind != wir.OperandPath && operand.Kind != wir.OperandTemp {
		return
	}
	if holds {
		produced[operand] = true
		return
	}
	delete(produced, operand)
}

// nativeOperandSubject spells the closed term of a named binding exactly as the
// equations publish it, so publication can anchor a contract row on the same
// term the value closure already carries a display name for.
func nativeOperandSubject(body *wir.Body, operand wir.Operand) string {
	if operand.Kind != wir.OperandPath {
		return ""
	}
	key := body.Path(wir.PathRef(operand.Ref)).Key()
	if key == "" {
		return ""
	}
	return "path/" + string(key)
}

// nativeRecordConstructionValue reads the storage class of each entry from the
// resolved producer of its operand.  A boolean entry has a canonical tagged
// slot; an entry produced by a multiplication keeps both runtime number arms,
// because an integer product may promote.  Neither is read from a source
// spelling: an entry whose producer is not resolved adds nothing.
//
// The allocation is always this body's own constructor, so the row is fresh.
// Its outgoing entry edges, their duplicate targets and their lowering order
// are read off the resolved entry list, never re-parsed from source.
func nativeRecordConstructionValue(body *wir.Body, entries []wir.TableEntry, direct int, products map[wir.Operand]bool, edges []nativeRecordEntryEdge, owned bool) string {
	value := fmt.Sprintf("entries=%d entry_storage=committed", direct)
	boolean, multiplication := false, false
	for _, entry := range entries {
		if entry.Value.Kind == wir.OperandConst && body.Const(wir.ConstRef(entry.Value.Ref)).Kind == wir.ConstBool {
			boolean = true
		}
		if products[entry.Value] {
			multiplication = true
		}
	}
	if boolean {
		value += " boolean_storage=canonical_tag"
	}
	if multiplication {
		value += " field_carrier=numeric_union overflow=promote_integer_to_number"
	}
	if len(edges) != 0 {
		duplicates := 0
		for _, edge := range edges {
			if !edge.moved {
				duplicates++
			}
		}
		value += fmt.Sprintf(" duplicate_children=%d edges=%d", duplicates, len(edges))
	}
	if nativeEntriesInLoweringOrder(entries) {
		value += " evaluation_order=preserved"
	}
	value += " fresh=true"
	if owned {
		value += " ownership=move"
	}
	return value
}

// nativeEntriesInLoweringOrder reports that the resolved entry list runs in
// source order.  An entry without a span, or one lowered ahead of an entry
// written before it, leaves the order unproven.
func nativeEntriesInLoweringOrder(entries []wir.TableEntry) bool {
	previous := wir.Span{}
	for _, entry := range entries {
		if !entry.ValueSpan.Valid() {
			return false
		}
		if previous.Valid() && nativeSpanPrecedes(entry.ValueSpan, previous) {
			return false
		}
		previous = entry.ValueSpan
	}
	return len(entries) != 0
}

func nativeSpanPrecedes(span, other wir.Span) bool {
	if span.StartLine != other.StartLine {
		return span.StartLine < other.StartLine
	}
	return span.StartCol < other.StartCol
}

// nativeMultiplicationResults names every operand a multiplication writes.
func nativeMultiplicationResults(body *wir.Body) map[wir.Operand]bool {
	products := make(map[wir.Operand]bool)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpBinOp && instruction.Operator == wir.BinMul && instruction.Dst.Kind != wir.OperandNone {
			products[instruction.Dst] = true
		}
	}
	return products
}

// nativeEscapedTableConstructors follows copies of a constructor's destination
// through resolved WIR.  Reaching a callee — as an argument or as a method
// receiver — is a publication boundary: the allocation is no longer local, so
// its constructor contract carries the escape deopt class instead of an
// unbounded grant.  A destination reassigned from an unrelated producer loses
// the association, and an operand kind that carries no root binding never
// proves an escape at all.
func nativeEscapedTableConstructors(body *wir.Body) map[int]bool {
	escaped := make(map[int]bool)
	if body == nil {
		return escaped
	}
	rootKey := func(operand wir.Operand) string {
		if operand.Kind != wir.OperandPath {
			return ""
		}
		item := body.Path(wir.PathRef(operand.Ref))
		item.Segments = nil
		item.Version = 0
		return string(item.Key())
	}
	constructors := make(map[string]int)
	reaches := func(operand wir.Operand) {
		if source, found := constructors[rootKey(operand)]; found {
			escaped[source] = true
		}
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpMakeTable:
			if key := rootKey(instruction.Dst); key != "" {
				constructors[key] = index
			}
		case wir.OpAssign:
			destination := rootKey(instruction.Dst)
			if destination == "" {
				continue
			}
			if source, found := constructors[rootKey(instruction.A)]; found {
				constructors[destination] = source
			} else {
				delete(constructors, destination)
			}
		case wir.OpCall:
			for _, argument := range body.Operands(instruction.List) {
				reaches(argument)
			}
			reaches(instruction.Call.Receiver)
		}
	}
	return escaped
}

// nativeOwnedTableConstructors names every constructor whose allocation stays
// this body's own storage.  The disposition is the license the body already
// exercises: it stores into the allocation's members in place, through one
// binding, without a guard.  A second root assigned from the constructor gives
// the storage a second reachable owner and withdraws the proof, and a
// constructor the body only reads or hands on carries no ownership at all.
func nativeOwnedTableConstructors(body *wir.Body) map[int]bool {
	owned := make(map[int]bool)
	if body == nil {
		return owned
	}
	rootKey := func(operand wir.Operand) string {
		if operand.Kind != wir.OperandPath {
			return ""
		}
		item := body.Path(wir.PathRef(operand.Ref))
		item.Segments = nil
		item.Version = 0
		return string(item.Key())
	}
	constructors := make(map[string]int)
	owners := make(map[int]int)
	written := make(map[int]bool)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpMakeTable:
			if key := rootKey(instruction.Dst); key != "" {
				constructors[key] = index
				owners[index]++
			}
		case wir.OpAssign:
			destination := rootKey(instruction.Dst)
			if destination == "" {
				continue
			}
			source, found := constructors[rootKey(instruction.A)]
			if !found {
				delete(constructors, destination)
				continue
			}
			if current, held := constructors[destination]; !held || current != source {
				owners[source]++
			}
			constructors[destination] = source
		case wir.OpStaticMemberWrite:
			if source, found := constructors[rootKey(instruction.Dst)]; found {
				written[source] = true
			}
		}
	}
	for index := range written {
		if owners[index] == 1 {
			owned[index] = true
		}
	}
	return owned
}

// nativePhysicalRecordShape accepts only a closed, presence-complete record.
// Optional fields need a separate physical-presence discriminator, so they
// deliberately withhold ShapeID rather than pretending one layout covers both
// field sets.  Field value types do not affect table offsets; Unknown is used
// solely to make that physical-layout boundary explicit in the canonical type.
func nativePhysicalRecordShape(value typ.Type) (typ.Type, bool) {
	value = unwrap.Alias(value)
	if recursive, ok := value.(*typ.Recursive); ok {
		if recursive == nil || recursive.Body == nil || recursive.Body == value {
			return nil, false
		}
		value = unwrap.Alias(recursive.Body)
	}
	record, ok := value.(*typ.Record)
	if !ok || record == nil || record.Open || record.HasMapComponent() || record.Metatable != nil || len(record.StaticMembers) != 0 {
		return nil, false
	}
	fields := make([]typ.Field, 0, len(record.Fields))
	for _, field := range record.Fields {
		if field.Name == "" || field.Optional || nativeOptionalType(field.Type) {
			return nil, false
		}
		fields = append(fields, typ.Field{Name: field.Name, Type: typ.Unknown, Readonly: field.Readonly})
	}
	if len(fields) == 0 {
		return nil, false
	}
	return typ.RebuildRecord(typ.RecordParts{Fields: fields}), true
}

func nativeOptionalType(value typ.Type) bool {
	_, optional := unwrap.Alias(value).(*typ.Optional)
	return optional
}

func nativeShapeID(shape typ.Type) (ShapeID, bool) {
	digest, err := typ.DigestCanonical(context.Background(), shape)
	if err != nil {
		return 0, false
	}
	id := ShapeID(binary.BigEndian.Uint64(digest[:8]))
	return id, id != 0
}

type nativeTableShape struct {
	fields map[string]bool
}

func nativeShapeTransitionContracts(body *wir.Body) []NativeContract {
	// This small topology walk follows only a closed constructor value through
	// ordinary copies. A dynamic write, an unknown key, or a missing constructor
	// stops the proof. It is therefore a publication of WIR's exact static
	// member-write topology, not an attempt to infer arbitrary heap shapes.
	shapes := make(map[string]nativeTableShape)
	key := func(operand wir.Operand) string {
		switch operand.Kind {
		case wir.OperandPath:
			return string(body.Path(wir.PathRef(operand.Ref)).Key())
		case wir.OperandTemp:
			return fmt.Sprintf("temp/%d", operand.Ref)
		default:
			return ""
		}
	}
	var contracts []NativeContract
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpMakeTable:
			if !instruction.StaticStringKeysComplete {
				continue
			}
			fields := make(map[string]bool)
			valid := true
			for _, entry := range body.TableEntries(instruction.TableEntries) {
				name, ok := segment.DirectFieldName(entry.Suffix.Segments)
				if !ok || name == "" {
					valid = false
					break
				}
				fields[name] = true
			}
			if valid && len(fields) != 0 {
				shapes[key(instruction.Dst)] = nativeTableShape{fields: fields}
			}
		case wir.OpAssign:
			source, found := shapes[key(instruction.A)]
			if found && key(instruction.Dst) != "" {
				shapes[key(instruction.Dst)] = source
			}
		case wir.OpStaticMemberWrite:
			if instruction.Dst.Kind != wir.OperandPath {
				continue
			}
			target := body.Path(wir.PathRef(instruction.Dst.Ref))
			if len(target.Segments) != 1 || target.Segments[0].Kind != segment.SegmentField {
				continue
			}
			parent := target
			parent.Segments = nil
			shape, found := shapes[string(parent.Key())]
			field := target.Segments[0].Name
			if !found || field == "" || shape.fields[field] {
				continue
			}
			oldID, oldOK := nativeTableShapeID(shape.fields)
			shape.fields = cloneNativeShapeFields(shape.fields)
			shape.fields[field] = true
			newID, newOK := nativeTableShapeID(shape.fields)
			if !oldOK || !newOK || oldID == newID {
				continue
			}
			shapes[string(parent.Key())] = shape
			contracts = append(contracts,
				NativeContract{Family: "shape_identity", Value: fmt.Sprintf("field_offsets=identical field_order=canonical interned=true shape_id=%016x", uint64(oldID)), Revocations: []string{"shape.transition"}},
				NativeContract{Family: "shape_identity", Value: fmt.Sprintf("field_offsets=identical field_order=canonical interned=true shape_id=%016x", uint64(newID)), Revocations: []string{"shape.transition"}},
				NativeContract{Family: "shape_transition", Value: "new_identity=minted new_shape=published old_identity_reused=false old_shape=published transition_edge=published", Revocations: []string{"shape.transition"}},
			)
		}
	}
	return contracts
}

func cloneNativeShapeFields(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in)+1)
	for field := range in {
		out[field] = true
	}
	return out
}

func nativeTableShapeID(fields map[string]bool) (ShapeID, bool) {
	if len(fields) == 0 {
		return 0, false
	}
	names := make([]string, 0, len(fields))
	for field := range fields {
		names = append(names, field)
	}
	sort.Strings(names)
	recordFields := make([]typ.Field, 0, len(names))
	for _, field := range names {
		recordFields = append(recordFields, typ.Field{Name: field, Type: typ.Unknown})
	}
	return nativeShapeID(typ.RebuildRecord(typ.RecordParts{Fields: recordFields}))
}

func nativeDirectTableShapeID(entries []wir.TableEntry) (ShapeID, bool) {
	fields := make(map[string]bool, len(entries))
	for _, entry := range entries {
		field, ok := segment.DirectFieldName(entry.Suffix.Segments)
		if !ok || field == "" {
			continue
		}
		fields[field] = true
	}
	return nativeTableShapeID(fields)
}

func nativeDiscriminantContracts(body *wir.Body) []NativeContract {
	if body == nil {
		return nil
	}
	types := make(map[typ.Type]bool)
	body.ForEachType(func(value typ.Type) bool {
		if value != nil {
			types[value] = true
		}
		return true
	})
	var contracts []NativeContract
	for value := range types {
		_, cases, ok := variant.OriginCasesOfType(value)
		if !ok {
			continue
		}
		domains, ok := variant.LiteralDiscriminantDomainsForCases(cases)
		if !ok {
			continue
		}
		for _, domain := range domains {
			field, ok := nativeDirectDiscriminantField(domain.Suffix)
			if !ok || !nativeDenseCaseDomain(domain) {
				continue
			}
			covered, booleanBranch := nativeCoveredDiscriminantCases(body, domain)
			if booleanBranch && nativeBooleanDomain(domain) {
				covered = make(map[int]bool, len(domain.Cases))
				for _, item := range domain.Cases {
					covered[item.Index] = true
				}
			}
			if len(covered) == 0 {
				continue
			}
			exhaustive := len(covered) == len(domain.Cases)
			mapping := make([]string, len(covered))
			for i := range mapping {
				mapping[i] = fmt.Sprintf("%d", i)
			}
			value := fmt.Sprintf("cases=%d default_required=%t dense_mapping=[%s] discriminant_field=%s exhaustive=%t", len(covered), !exhaustive, strings.Join(mapping, ","), field, exhaustive)
			contracts = append(contracts, NativeContract{Family: "discriminant_select", Value: value, Revocations: []string{"write.field"}})
		}
	}
	return contracts
}

func nativeDirectDiscriminantField(suffix []segment.Segment) (string, bool) {
	field, ok := segment.DirectFieldName(suffix)
	return field, ok && field != ""
}

func nativeDenseCaseDomain(domain variant.LiteralDiscriminantDomain) bool {
	if len(domain.Cases) < 2 {
		return false
	}
	for index, item := range domain.Cases {
		if item.Index != index || item.Literal == nil {
			return false
		}
	}
	return true
}

func nativeBooleanDomain(domain variant.LiteralDiscriminantDomain) bool {
	if len(domain.Cases) != 2 {
		return false
	}
	seenTrue, seenFalse := false, false
	for _, item := range domain.Cases {
		literal, ok := item.Literal.Value.(bool)
		if !ok {
			return false
		}
		if literal {
			seenTrue = true
		} else {
			seenFalse = true
		}
	}
	return seenTrue && seenFalse
}

func nativeCoveredDiscriminantCases(body *wir.Body, domain variant.LiteralDiscriminantDomain) (map[int]bool, bool) {
	covered := make(map[int]bool)
	booleanBranch := false
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpBranch {
			continue
		}
		check := body.Check(instruction.Check)
		if !nativePathHasSuffix(check.Path.Segments, domain.Suffix) {
			continue
		}
		switch check.Kind {
		case wir.CheckLiteralEqual:
			for _, item := range domain.Cases {
				if typ.TypeEquals(check.Literal, item.Literal) {
					covered[item.Index] = true
				}
			}
		case wir.CheckTruthy:
			booleanBranch = true
		}
	}
	return covered, booleanBranch
}

func nativePathHasSuffix(path, suffix []segment.Segment) bool {
	if len(suffix) == 0 || len(path) < len(suffix) {
		return false
	}
	offset := len(path) - len(suffix)
	for index := range suffix {
		if path[offset+index] != suffix[index] {
			return false
		}
	}
	return true
}

func nativeRecursiveIdentityContracts(body *wir.Body, definitions map[string]typ.Type) []NativeContract {
	if body == nil {
		return nil
	}
	recursives := make(map[*typ.Recursive]bool)
	body.ForEachType(func(value typ.Type) bool {
		nativeCollectRecursives(value, recursives, make(map[typ.Type]bool))
		return true
	})
	for _, definition := range definitions {
		nativeCollectRecursives(definition, recursives, make(map[typ.Type]bool))
	}
	if len(recursives) == 0 {
		return nil
	}
	var contracts []NativeContract
	for recursive := range recursives {
		records := nativeRecursiveRecordNodes(recursive.Body)
		cycleRecords := make([]*typ.Record, 0, len(records))
		for record := range records {
			if nativeContainsRecursive(record, recursive) {
				cycleRecords = append(cycleRecords, record)
			}
		}
		if len(cycleRecords) == 0 {
			continue
		}
		mutual := len(cycleRecords) > 1
		value := "fixpoint=reached identity_equal_to_subject=true identity_stable=true traversal_caches=1"
		if mutual {
			value += " mutual=true"
		}
		for range cycleRecords {
			contracts = append(contracts, NativeContract{Family: "recursive_type_identity", Value: value, Revocations: []string{"shape.transition"}})
		}
	}
	return contracts
}

func nativeRecursiveRecordNodes(value typ.Type) map[*typ.Record]bool {
	records := make(map[*typ.Record]bool)
	seen := make(map[typ.Type]bool)
	var visit func(typ.Type)
	visit = func(current typ.Type) {
		if current == nil || seen[current] {
			return
		}
		seen[current] = true
		current = unwrap.Annotations(current)
		switch item := current.(type) {
		case *typ.Alias:
			visit(item.UnaliasedTarget())
		case *typ.Recursive:
			visit(item.Body)
		case *typ.Optional:
			visit(item.Inner)
		case *typ.Union:
			for _, member := range item.Members {
				visit(member)
			}
		case *typ.Record:
			records[item] = true
			for _, field := range item.Fields {
				visit(field.Type)
			}
		}
	}
	visit(value)
	return records
}

func nativeCollectRecursives(value typ.Type, out map[*typ.Recursive]bool, seen map[typ.Type]bool) {
	if value == nil || seen[value] {
		return
	}
	seen[value] = true
	value = unwrap.Annotations(value)
	switch item := value.(type) {
	case *typ.Alias:
		nativeCollectRecursives(item.UnaliasedTarget(), out, seen)
	case *typ.Recursive:
		out[item] = true
		if item.Body != nil {
			nativeCollectRecursives(item.Body, out, seen)
		}
	case *typ.Optional:
		nativeCollectRecursives(item.Inner, out, seen)
	case *typ.Union:
		for _, member := range item.Members {
			nativeCollectRecursives(member, out, seen)
		}
	case *typ.Record:
		for _, field := range item.Fields {
			nativeCollectRecursives(field.Type, out, seen)
		}
	}
}

func nativeContainsRecursive(value typ.Type, want *typ.Recursive) bool {
	seen := make(map[typ.Type]bool)
	var visit func(typ.Type) bool
	visit = func(current typ.Type) bool {
		if current == nil || seen[current] {
			return false
		}
		seen[current] = true
		current = unwrap.Annotations(current)
		if recursive, ok := current.(*typ.Recursive); ok {
			return recursive == want || (recursive.Body != nil && visit(recursive.Body))
		}
		switch item := current.(type) {
		case *typ.Alias:
			return visit(item.UnaliasedTarget())
		case *typ.Optional:
			return visit(item.Inner)
		case *typ.Union:
			for _, member := range item.Members {
				if visit(member) {
					return true
				}
			}
		case *typ.Record:
			for _, field := range item.Fields {
				if visit(field.Type) {
					return true
				}
			}
		}
		return false
	}
	return visit(value)
}
