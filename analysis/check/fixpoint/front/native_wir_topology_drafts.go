package front

// Native WIR topology drafts expose resolved lowering coordinates through the
// ordinary closure publication path. They deliberately consume only WIR and
// typ identities already owned by Compilation. Their closed record types have
// no semantic-conclusion carrier; the post-solve kernel owns every verdict.

import (
	"context"
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

const nativeClaimAssertDraftKind = "claim-kind/2"

func nativeWIRTopologyDrafts(root Compilation) []NativeTopologyDraft {
	var drafts []NativeTopologyDraft
	if graph := nativeCallGraphTopologyDraft(root); graph != nil {
		drafts = append(drafts, NativeTopologyDraft{Kind: NativeTopologyCallGraph, CallGraph: graph})
	}
	drafts = append(drafts, nativeFrozenKernelTopologyDrafts(root)...)
	var visit func(Compilation)
	visit = func(compilation Compilation) {
		drafts = append(drafts, nativeConstantTopologyDrafts(compilation)...)
		drafts = append(drafts, nativePublicationTopologyDrafts(compilation)...)
		drafts = append(drafts, nativeShapeTopologyDrafts(compilation)...)
		drafts = append(drafts, nativeShapeEpochTopologyDrafts(compilation)...)
		drafts = append(drafts, nativeRecordTopologyDrafts(compilation)...)
		drafts = append(drafts, nativeShapeTransitionTopologyDrafts(compilation)...)
		drafts = append(drafts, nativeDiscriminantTopologyDrafts(compilation)...)
		drafts = append(drafts, nativeRecursiveTopologyDrafts(compilation)...)
		if compilation.Prototype != 0 {
			if summary := nativeSummaryTopologyDraft(compilation); summary != nil {
				drafts = append(drafts, NativeTopologyDraft{Kind: NativeTopologySummary, Summary: summary})
			}
		}
		for _, child := range compilation.nested {
			visit(child)
		}
	}
	visit(root)
	return drafts
}

func nativeFrozenKernelTopologyDrafts(root Compilation) []NativeTopologyDraft {
	var drafts []NativeTopologyDraft
	var visit func(Compilation, bool)
	visit = func(compilation Compilation, evaluated bool) {
		if !evaluated {
			for index, operation := range compilation.Artifact.Equations {
				var kind NativeKernelOccurrence
				switch operation.Occurrence.Kind {
				case "eval-node":
					operands, err := nativeClosedOperandsByRole(operation.Operands, equation.RoleOperation)
					if err != nil {
						continue
					}
					switch string(operands[equation.RoleOperation]) {
					case "closure":
						kind = NativeKernelEvalClosure
					case "length":
						kind = NativeKernelEvalLength
					}
				case "claim":
					operands, err := nativeClosedOperandsByRole(operation.Operands, equation.RoleKind)
					if err == nil && string(operands[equation.RoleKind]) == nativeClaimAssertDraftKind {
						kind = NativeKernelClaimAssert
					}
				}
				if kind == 0 {
					continue
				}
				draft := NativeKernelOccurrenceDraft{
					Site: NativeInstructionReference{
						Body: [32]byte(compilation.Body), Position: uint32(index),
					},
					Operation: kind,
				}
				drafts = append(drafts, NativeTopologyDraft{
					Kind: NativeTopologyKernelOccurrence, KernelOccurrence: &draft,
				})
			}
		}
		for _, child := range compilation.nested {
			visit(child, false)
		}
	}
	visit(root, true)
	return drafts
}

func nativeClosedOperandsByRole(operands []equation.Operand, roles ...equation.OperandRole) (map[equation.OperandRole][]byte, error) {
	out := make(map[equation.OperandRole][]byte, len(roles))
	for _, role := range roles {
		for _, operand := range operands {
			if operand.Role == role && !operand.Term.Entry {
				out[role] = operand.Term.Encoding
				break
			}
		}
		if out[role] == nil {
			return nil, fmt.Errorf("front: missing closed artifact operand %q", role)
		}
	}
	return out, nil
}

func nativeDiscriminantTopologyDrafts(compilation Compilation) []NativeTopologyDraft {
	body := compilation.WIR
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
	var drafts []NativeTopologyDraft
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
			if !ok {
				continue
			}
			draft := NativeDiscriminantDraft{Body: [32]byte(compilation.Body), Field: field}
			for _, item := range domain.Cases {
				if item.Literal == nil {
					continue
				}
				encoded, err := typ.EncodeCanonical(context.Background(), item.Literal)
				if err != nil {
					continue
				}
				draft.Cases = append(draft.Cases, NativeDiscriminantCaseDraft{
					Ordinal: uint32(item.Index), Literal: encoded,
				})
			}
			matched := make(map[uint32]bool)
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
							matched[uint32(item.Index)] = true
						}
					}
				case wir.CheckTruthy:
					draft.TruthySites = append(draft.TruthySites, uint32(index))
				}
			}
			for ordinal := range matched {
				draft.MatchedCases = append(draft.MatchedCases, ordinal)
			}
			sort.Slice(draft.MatchedCases, func(i, j int) bool { return draft.MatchedCases[i] < draft.MatchedCases[j] })
			drafts = append(drafts, NativeTopologyDraft{Kind: NativeTopologyDiscriminant, Discriminant: &draft})
		}
	}
	return drafts
}

func nativeRecursiveTopologyDrafts(compilation Compilation) []NativeTopologyDraft {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	recursives := make(map[*typ.Recursive]bool)
	body.ForEachType(func(value typ.Type) bool {
		nativeCollectRecursives(value, recursives, make(map[typ.Type]bool))
		return true
	})
	for _, definition := range compilation.typeDefinitions {
		nativeCollectRecursives(definition, recursives, make(map[typ.Type]bool))
	}
	var drafts []NativeTopologyDraft
	for recursive := range recursives {
		records := nativeRecursiveRecordNodes(recursive.Body)
		cycleCount := uint32(0)
		for record := range records {
			if nativeContainsRecursive(record, recursive) {
				cycleCount++
			}
		}
		draft := NativeRecursiveTopologyDraft{
			Body: [32]byte(compilation.Body), RecordNodes: uint32(len(records)), CycleRecordNodes: cycleCount,
		}
		drafts = append(drafts, NativeTopologyDraft{Kind: NativeTopologyRecursiveType, Recursive: &draft})
	}
	return drafts
}

func nativeSummaryTopologyDraft(compilation Compilation) *NativeSummaryTopologyDraft {
	body := compilation.WIR
	if body == nil || len(body.DeclaredReturnTypes()) == 0 {
		return nil
	}
	draft := &NativeSummaryTopologyDraft{
		Body: NativeBodyReference{
			Body: [32]byte(compilation.Body), Prototype: uint64(compilation.Prototype),
			Display: compilation.PrototypeName,
		},
	}
	for _, parameter := range compilation.Boundary.Parameters {
		if value := body.Type(parameter.Type); value != nil {
			if encoded, err := typ.EncodeCanonical(context.Background(), value); err == nil {
				draft.Parameters = append(draft.Parameters, encoded)
			}
		}
	}
	for _, result := range body.DeclaredReturnTypes() {
		if encoded, err := typ.EncodeCanonical(context.Background(), result); err == nil {
			draft.Results = append(draft.Results, encoded)
		}
	}
	for _, capture := range compilation.Boundary.Captures {
		if capture.Mutable {
			draft.MutableCaptures = append(draft.MutableCaptures, NativeSymbolReference{
				Display: capture.Name, Term: fmt.Sprintf("symbol/%d", capture.Symbol),
			})
		}
	}
	if len(draft.Results) == 0 {
		return nil
	}
	return draft
}

func nativeShapeTopologyDrafts(compilation Compilation) []NativeTopologyDraft {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	bodyID := [32]byte(compilation.Body)
	var drafts []NativeTopologyDraft
	add := func(value typ.Type, origin NativeShapeOrigin) {
		if shape, ok := nativeShapeTopology(bodyID, value, origin); ok {
			drafts = append(drafts, NativeTopologyDraft{Kind: NativeTopologyShape, Shape: &shape})
		}
	}
	for _, root := range body.RootTypes() {
		add(body.Type(root.Type), NativeShapeDeclaredRoot)
	}
	for _, value := range body.DeclaredReturnTypes() {
		add(value, NativeShapeDeclaredReturn)
	}
	for _, parameter := range compilation.Boundary.Parameters {
		add(body.Type(parameter.Type), NativeShapeParameter)
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpClaim {
			add(body.Type(instruction.Type), NativeShapeClaim)
		}
		if instruction.Op != wir.OpMakeTable {
			continue
		}
		item := NativeShapeTopologyDraft{
			Body: bodyID, Origin: NativeShapeConstructor,
			OpenParts: boolCount(!instruction.StaticStringKeysComplete),
		}
		for _, entry := range body.TableEntries(instruction.TableEntries) {
			field, ok := segment.DirectFieldName(entry.Suffix.Segments)
			if !ok || field == "" {
				item.MapParts++
				continue
			}
			item.Fields = append(item.Fields, NativeShapeFieldDraft{Name: field})
		}
		drafts = append(drafts, NativeTopologyDraft{Kind: NativeTopologyShape, Shape: &item})
	}
	return drafts
}

func nativeShapeTopology(body [32]byte, value typ.Type, origin NativeShapeOrigin) (NativeShapeTopologyDraft, bool) {
	value = unwrap.Alias(value)
	if recursive, ok := value.(*typ.Recursive); ok {
		if recursive == nil || recursive.Body == nil || recursive.Body == value {
			return NativeShapeTopologyDraft{}, false
		}
		value = unwrap.Alias(recursive.Body)
	}
	record, ok := value.(*typ.Record)
	if !ok || record == nil {
		return NativeShapeTopologyDraft{}, false
	}
	draft := NativeShapeTopologyDraft{
		Body: body, Origin: origin,
		OpenParts: boolCount(record.Open), MapParts: boolCount(record.HasMapComponent()),
		MetatableRefs: boolCount(record.Metatable != nil), StaticMembers: uint32(len(record.StaticMembers)),
	}
	for _, field := range record.Fields {
		if field.Name == "" {
			draft.MapParts++
			continue
		}
		draft.Fields = append(draft.Fields, NativeShapeFieldDraft{
			Name: field.Name, Readonly: boolByte(field.Readonly),
			Optional: boolByte(field.Optional || nativeOptionalType(field.Type)),
		})
	}
	return draft, true
}

func boolCount(value bool) uint32 {
	if value {
		return 1
	}
	return 0
}

func boolByte(value bool) uint8 {
	if value {
		return 1
	}
	return 0
}

func nativeShapeEpochTopologyDrafts(compilation Compilation) []NativeTopologyDraft {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	type receiver struct {
		reference NativeSymbolReference
		fields    []NativeShapeFieldDraft
	}
	receivers := make(map[wir.SymbolID]receiver)
	for _, parameter := range compilation.Boundary.Parameters {
		shape, ok := nativeShapeTopology([32]byte(compilation.Body), body.Type(parameter.Type), NativeShapeParameter)
		if !ok || parameter.Name == "" {
			continue
		}
		receivers[parameter.Symbol] = receiver{
			reference: NativeSymbolReference{Display: parameter.Name}, fields: shape.Fields,
		}
	}
	if len(receivers) == 0 {
		return nil
	}
	reads := make(map[wir.SymbolID][]uint32)
	writes := make(map[wir.SymbolID][]uint32)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpStaticMemberWrite && instruction.Dst.Kind == wir.OperandPath {
			target := body.Path(wir.PathRef(instruction.Dst.Ref))
			if len(target.Segments) == 1 && target.Segments[0].Kind == segment.SegmentField {
				if candidate, ok := receivers[target.Symbol]; ok && nativeDraftHasField(candidate.fields, target.Segments[0].Name) {
					writes[target.Symbol] = append(writes[target.Symbol], uint32(index))
				}
			}
		}
		for _, operand := range nativeReadOperands(body, instruction) {
			if operand.Kind != wir.OperandPath {
				continue
			}
			member := body.Path(wir.PathRef(operand.Ref))
			if member.Symbol != 0 && len(member.Segments) != 0 && member.Segments[0].Kind == segment.SegmentField {
				if _, ok := receivers[member.Symbol]; ok {
					reads[member.Symbol] = append(reads[member.Symbol], uint32(index))
				}
			}
		}
	}
	symbols := make([]int, 0, len(receivers))
	for symbol := range receivers {
		symbols = append(symbols, int(symbol))
	}
	sort.Ints(symbols)
	var drafts []NativeTopologyDraft
	for _, encoded := range symbols {
		symbol := wir.SymbolID(encoded)
		item := receivers[symbol]
		draft := NativeShapeEpochTopologyDraft{
			Body: [32]byte(compilation.Body), Receiver: item.reference, Fields: item.fields,
			ReadSites: reads[symbol], WriteSites: writes[symbol],
		}
		drafts = append(drafts, NativeTopologyDraft{Kind: NativeTopologyShapeEpoch, ShapeEpoch: &draft})
	}
	return drafts
}

func nativeDraftHasField(fields []NativeShapeFieldDraft, name string) bool {
	for _, field := range fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

type nativeRecordUseTopology struct {
	aliases []NativeInstructionReference
	writes  []NativeInstructionReference
	calls   []NativeInstructionReference
}

func nativeRecordTopologyDrafts(compilation Compilation) []NativeTopologyDraft {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	bodyID := [32]byte(compilation.Body)
	uses := nativeRecordUses(bodyID, body)
	produced := make(map[wir.Operand]uint32)
	producerOps := make(map[uint32]NativeProducerOperation)
	producerKind := func(instruction wir.Instruction) NativeProducerOperation {
		switch {
		case instruction.Op == wir.OpMakeTable:
			return NativeProducerTable
		case instruction.Op == wir.OpCall:
			return NativeProducerCall
		case instruction.Op == wir.OpBinOp && instruction.Operator == wir.BinMul:
			return NativeProducerMultiply
		default:
			return NativeProducerOther
		}
	}
	var drafts []NativeTopologyDraft
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpMakeTable:
			entries := body.TableEntries(instruction.TableEntries)
			draft := NativeRecordTopologyDraft{
				Site: NativeInstructionReference{Body: bodyID, Position: uint32(index)},
				Destination: NativeSymbolReference{
					Term:    nativeOperandSubject(body, instruction.Dst),
					Display: nativeTopologyOperandDisplay(body, instruction.Dst),
				},
				KeySlots:   boolCount(instruction.StaticStringKeysComplete) * uint32(len(entries)),
				EntrySlots: uint32(len(entries)),
				AliasSites: uses[index].aliases, MemberWrites: uses[index].writes, CallUses: uses[index].calls,
			}
			for _, entry := range entries {
				field, _ := segment.DirectFieldName(entry.Suffix.Segments)
				producer := produced[entry.Value]
				draft.Entries = append(draft.Entries, NativeRecordEntryDraft{
					Field: field, Value: nativeTopologyOperand(body, entry.Value),
					ValueSpan:    nativeTopologySpan(entry.ValueSpan),
					ProducerSite: producer, ProducerOp: producerOps[producer],
				})
			}
			drafts = append(drafts, NativeTopologyDraft{Kind: NativeTopologyRecordConstruction, Record: &draft})
			produced[instruction.Dst] = uint32(index) + 1
			producerOps[uint32(index)+1] = producerKind(instruction)
		case wir.OpCall:
			for _, result := range body.Operands(instruction.Results) {
				produced[result] = uint32(index) + 1
				producerOps[uint32(index)+1] = producerKind(instruction)
			}
		case wir.OpAssign, wir.OpClaim:
			if source := produced[instruction.A]; source != 0 {
				produced[instruction.Dst] = source
			} else {
				delete(produced, instruction.Dst)
			}
		default:
			if instruction.Dst.Kind != wir.OperandNone {
				produced[instruction.Dst] = uint32(index) + 1
				producerOps[uint32(index)+1] = producerKind(instruction)
			}
		}
	}
	return drafts
}

func nativeTopologyOperandDisplay(body *wir.Body, operand wir.Operand) string {
	if operand.Kind != wir.OperandPath {
		return ""
	}
	return body.Path(wir.PathRef(operand.Ref)).String()
}

func nativeRecordUses(bodyID [32]byte, body *wir.Body) map[int]nativeRecordUseTopology {
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
	uses := make(map[int]nativeRecordUseTopology)
	appendUse := func(index int, kind byte, site NativeInstructionReference) {
		item := uses[index]
		switch kind {
		case 'a':
			item.aliases = append(item.aliases, site)
		case 'w':
			item.writes = append(item.writes, site)
		case 'c':
			item.calls = append(item.calls, site)
		}
		uses[index] = item
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		site := NativeInstructionReference{Body: bodyID, Position: uint32(index)}
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
			source, found := constructors[rootKey(instruction.A)]
			if !found {
				delete(constructors, destination)
				continue
			}
			if current, held := constructors[destination]; !held || current != source {
				appendUse(source, 'a', site)
			}
			constructors[destination] = source
		case wir.OpStaticMemberWrite:
			if source, found := constructors[rootKey(instruction.Dst)]; found {
				appendUse(source, 'w', site)
			}
		case wir.OpCall:
			for _, argument := range body.Operands(instruction.List) {
				if source, found := constructors[rootKey(argument)]; found {
					appendUse(source, 'c', site)
				}
			}
			if source, found := constructors[rootKey(instruction.Call.Receiver)]; found {
				appendUse(source, 'c', site)
			}
		}
	}
	return uses
}

func nativeShapeTransitionTopologyDrafts(compilation Compilation) []NativeTopologyDraft {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	shapes := make(map[string][]NativeShapeFieldDraft)
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
	var drafts []NativeTopologyDraft
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpMakeTable:
			if !instruction.StaticStringKeysComplete {
				continue
			}
			var fields []NativeShapeFieldDraft
			valid := true
			for _, entry := range body.TableEntries(instruction.TableEntries) {
				name, ok := segment.DirectFieldName(entry.Suffix.Segments)
				if !ok || name == "" {
					valid = false
					break
				}
				if !nativeDraftHasField(fields, name) {
					fields = append(fields, NativeShapeFieldDraft{Name: name})
				}
			}
			if valid && len(fields) != 0 {
				shapes[key(instruction.Dst)] = fields
			}
		case wir.OpAssign:
			if source, found := shapes[key(instruction.A)]; found && key(instruction.Dst) != "" {
				shapes[key(instruction.Dst)] = append([]NativeShapeFieldDraft(nil), source...)
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
			fields, found := shapes[string(parent.Key())]
			field := target.Segments[0].Name
			if !found || field == "" || nativeDraftHasField(fields, field) {
				continue
			}
			draft := NativeShapeTransitionDraft{
				Body: [32]byte(compilation.Body), Site: uint32(index),
				Before: append([]NativeShapeFieldDraft(nil), fields...), AddedField: field,
			}
			drafts = append(drafts, NativeTopologyDraft{Kind: NativeTopologyShapeTransition, ShapeTransition: &draft})
			fields = append(append([]NativeShapeFieldDraft(nil), fields...), NativeShapeFieldDraft{Name: field})
			shapes[string(parent.Key())] = fields
		}
	}
	return drafts
}

func nativeCallGraphTopologyDraft(root Compilation) *NativeCallGraphDraft {
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
	if len(byName) == 0 {
		return nil
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	draft := &NativeCallGraphDraft{}
	for _, name := range names {
		compilation := byName[name]
		draft.Bodies = append(draft.Bodies, NativeBodyReference{
			Body: [32]byte(compilation.Body), Prototype: uint64(compilation.Prototype), Display: name,
		})
		item := NativeBodyTypeDraft{Display: name, ResultSlots: uint32(len(compilation.WIR.DeclaredReturnTypes()))}
		if len(compilation.Boundary.Parameters) != 0 {
			value := compilation.WIR.Type(compilation.Boundary.Parameters[0].Type)
			if value != nil {
				if encoded, err := typ.EncodeCanonical(context.Background(), value); err == nil {
					item.FirstParameter = encoded
				}
			}
		}
		draft.Types = append(draft.Types, item)
	}
	for _, from := range names {
		body := byName[from].WIR
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
			if _, closed := byName[to]; closed {
				draft.Edges = append(draft.Edges, NativeCallEdgeDraft{From: from, To: to})
			}
		}
	}
	sort.Slice(draft.Edges, func(i, j int) bool {
		if draft.Edges[i].From != draft.Edges[j].From {
			return draft.Edges[i].From < draft.Edges[j].From
		}
		return draft.Edges[i].To < draft.Edges[j].To
	})
	return draft
}

func nativePublicationTopologyDrafts(compilation Compilation) []NativeTopologyDraft {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	bodyID := [32]byte(compilation.Body)
	var drafts []NativeTopologyDraft
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpEntry || instruction.Op == wir.OpExit || instruction.Op == wir.OpNoop || !instruction.ExprSpan.Valid() {
			continue
		}
		drafts = append(drafts, NativeTopologyDraft{
			Kind: NativeTopologyPublicationSite,
			Publication: &NativePublicationSiteDraft{
				Site: NativeInstructionReference{Body: bodyID, Position: uint32(index)},
				Span: nativeTopologySpan(instruction.ExprSpan),
			},
		})
	}
	return drafts
}

func nativeConstantTopologyDrafts(compilation Compilation) []NativeTopologyDraft {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	bodyID := [32]byte(compilation.Body)
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
	writes := make(map[string][]NativeInstructionReference)
	captures := make(map[string][]NativeInstructionReference)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		site := NativeInstructionReference{Body: bodyID, Position: uint32(index)}
		count := func(operand wir.Operand) {
			if name := key(operand); name != "" {
				writes[name] = append(writes[name], site)
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
					captures[name] = append(captures[name], site)
				}
			}
		}
	}
	var drafts []NativeTopologyDraft
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		var operator NativeConstantOperator
		var inputs []wir.Operand
		switch instruction.Op {
		case wir.OpAssign:
			operator, inputs = NativeConstantAssign, []wir.Operand{instruction.A}
		case wir.OpUnOp:
			if wir.Operator(instruction.Operator) == wir.UnNeg {
				operator, inputs = NativeConstantNegate, []wir.Operand{instruction.A}
			}
		case wir.OpBinOp:
			switch wir.Operator(instruction.Operator) {
			case wir.BinAdd:
				operator = NativeConstantAdd
			case wir.BinSub:
				operator = NativeConstantSubtract
			case wir.BinMul:
				operator = NativeConstantMultiply
			case wir.BinIDiv:
				operator = NativeConstantFloorDivide
			case wir.BinMod:
				operator = NativeConstantModulo
			}
			if operator != 0 {
				inputs = []wir.Operand{instruction.A, instruction.B}
			}
		}
		name := key(instruction.Dst)
		if operator == 0 || name == "" {
			continue
		}
		item := NativeConstantCandidateDraft{
			Site:         NativeInstructionReference{Body: bodyID, Position: uint32(index)},
			Destination:  nativeTopologyOperand(body, instruction.Dst),
			Operator:     operator,
			WriteSites:   append([]NativeInstructionReference(nil), writes[name]...),
			CaptureSites: append([]NativeInstructionReference(nil), captures[name]...),
		}
		for _, input := range inputs {
			item.Inputs = append(item.Inputs, nativeTopologyOperand(body, input))
		}
		drafts = append(drafts, NativeTopologyDraft{Kind: NativeTopologyConstantCandidate, Constant: &item})
	}
	return drafts
}

func nativeTopologyOperand(body *wir.Body, operand wir.Operand) NativeOperandDraft {
	switch operand.Kind {
	case wir.OperandConst:
		term, err := scalarTerm(body, operand)
		if err != nil {
			return NativeOperandDraft{}
		}
		return NativeOperandDraft{Shape: NativeOperandLiteral, Literal: append([]byte(nil), term.Encoding...)}
	case wir.OperandPath:
		return NativeOperandDraft{Shape: NativeOperandSymbol, Term: "path/" + string(body.Path(wir.PathRef(operand.Ref)).Key())}
	case wir.OperandTemp:
		return NativeOperandDraft{Shape: NativeOperandTemporary, Term: fmt.Sprintf("temp/%d", operand.Ref)}
	default:
		return NativeOperandDraft{}
	}
}

func nativeTopologySpan(span wir.Span) NativeSpanDraft {
	if !span.Valid() {
		return NativeSpanDraft{}
	}
	return NativeSpanDraft{
		StartLine: uint32(span.StartLine), StartCol: uint32(span.StartCol),
		EndLine: uint32(span.EndLine), EndCol: uint32(span.EndCol),
	}
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

// nativeOperandSubject spells the closed term of a named binding exactly as the
// equations publish it, so the topology draft can refer to the same term the
// value closure already carries a display name for.
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

func nativeOptionalType(value typ.Type) bool {
	_, optional := unwrap.Alias(value).(*typ.Optional)
	return optional
}

func nativeDirectDiscriminantField(suffix []segment.Segment) (string, bool) {
	field, ok := segment.DirectFieldName(suffix)
	return field, ok && field != ""
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
