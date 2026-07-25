package front

// Native WIR contracts expose resolved lowering facts through the ordinary
// closure publication path.  They deliberately consume only WIR and typ
// identities already owned by Compilation: no source spelling is re-parsed
// and a missing resolved identity produces no native grant.

import (
	"context"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

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
	var contracts []NativeContract
	var visit func(Compilation)
	visit = func(compilation Compilation) {
		contracts = append(contracts, nativeShapeContracts(compilation)...)
		contracts = append(contracts, nativeDiscriminantContracts(compilation.WIR)...)
		contracts = append(contracts, nativeRecursiveIdentityContracts(compilation.WIR, compilation.TypeDefinitions)...)
		for _, child := range compilation.Nested {
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
		return strings.Join(contracts[i].Revocations, "/") < strings.Join(contracts[j].Revocations, "/")
	})
	return contracts
}

func nativeShapeContracts(compilation Compilation) []NativeContract {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	var contracts []NativeContract
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

func nativeRecordConstructionContracts(body *wir.Body) []NativeContract {
	escaped := nativeEscapedTableConstructors(body)
	products := nativeMultiplicationResults(body)
	var contracts []NativeContract
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpMakeTable || !instruction.StaticStringKeysComplete {
			continue
		}
		entries := body.TableEntries(instruction.TableEntries)
		direct := 0
		for _, entry := range entries {
			if _, ok := segment.DirectFieldName(entry.Suffix.Segments); ok {
				direct++
			}
		}
		if direct == 0 {
			continue
		}
		// Every entry is already an exact WIR constructor slot.  The row says
		// nothing about optional-field presence beyond what the literal wrote.
		contract := NativeContract{Family: "record_construction", Value: nativeRecordConstructionValue(body, entries, direct, products)}
		if escaped[index] {
			contract.Revocations = []string{"escape"}
		}
		contracts = append(contracts, contract)
	}
	return contracts
}

// nativeRecordConstructionValue reads the storage class of each entry from the
// resolved producer of its operand.  A boolean entry has a canonical tagged
// slot; an entry produced by a multiplication keeps both runtime number arms,
// because an integer product may promote.  Neither is read from a source
// spelling: an entry whose producer is not resolved adds nothing.
func nativeRecordConstructionValue(body *wir.Body, entries []wir.TableEntry, direct int, products map[wir.Operand]bool) string {
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
	return value
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
