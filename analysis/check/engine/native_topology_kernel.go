package engine

import (
	"context"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

type nativeTopologyConclusion struct {
	family      factkey.Family
	value       string
	subject     string
	revocations []string
}

func nativeTopologyContractKey(
	family factkey.Family,
	ordinal int,
	subject string,
	revocations []string,
) string {
	key := factkey.BuildSubjectKey(
		family, factkey.OpaquePart("contract"), fmt.Sprintf("%d", ordinal),
	).String()
	if subject != "" {
		key += "/" + subject
	}
	if len(revocations) != 0 {
		key += "/contract-revocation/" + strings.Join(revocations, ",")
	}
	return key
}

// nativeTopologyKernelFacts is the sole semantic owner for front topology
// drafts. It runs only in the publication transaction after the semantic tail
// has closed. Draft fields identify structure; every returned value is built
// here from that structure and, where applicable, the closed partition.
func nativeTopologyKernelFacts(
	operation equation.BoundEquation,
	partition equation.Partition,
	drafts []front.NativeTopologyDraft,
) ([]equation.Fact, error) {
	ordered := append([]front.NativeTopologyDraft(nil), drafts...)
	sort.SliceStable(ordered, func(i, j int) bool {
		leftBody, leftPosition := nativeTopologyCoordinate(ordered[i])
		rightBody, rightPosition := nativeTopologyCoordinate(ordered[j])
		if leftBody != rightBody {
			return leftBody < rightBody
		}
		if leftPosition != rightPosition {
			return leftPosition < rightPosition
		}
		return ordered[i].Kind < ordered[j].Kind
	})

	knownConstants := make(map[string]bool)
	var facts []equation.Fact
	facts = append(facts, nativeShapeAndRecordConclusions(operation, ordered)...)
	facts = append(facts, nativeOtherTopologyConclusions(operation, ordered)...)
	for _, draft := range ordered {
		switch draft.Kind {
		case front.NativeTopologyCallGraph:
			rows := nativeCallGraphConclusions(draft.CallGraph)
			facts = append(facts, rows...)

		case front.NativeTopologyConstantCandidate:
			row := draft.Constant
			if row == nil || len(row.WriteSites) != 1 || len(row.CaptureSites) != 0 ||
				!nativeConstantInputsKnown(row.Inputs, row.Site.Body, knownConstants) {
				continue
			}
			term := row.Destination.Term
			if term == "" {
				continue
			}
			knownConstants[nativeTopologyTermKey(row.Site.Body, term)] = true
			published, err := resolveCurrentValue([]byte(term), partition)
			if err != nil {
				continue
			}
			word, ok := nativePublishedConstantWord(published)
			if !ok {
				continue
			}
			key := factkey.BuildKey(
				factkey.NativeConstantValue,
				[]factkey.Part{factkey.OpaquePart(fmt.Sprintf("%x", row.Site.Body))},
				nativeTopologyOperationName(row.Site.Position),
			)
			facts = append(facts, equation.Fact{
				Key: key.String(), Value: []byte("representation=" + word.representation + " value=" + word.text),
			})

		case front.NativeTopologyPublicationSite:
			row := draft.Publication
			if row == nil || row.Span.StartLine == 0 {
				continue
			}
			key := factkey.BuildKey(
				factkey.NativePublicationIdentity,
				[]factkey.Part{factkey.OpaquePart(fmt.Sprintf("%x", row.Site.Body))},
				nativeTopologyOperationName(row.Site.Position),
			)
			facts = append(facts, equation.Fact{
				Key:   key.String(),
				Value: []byte("executable_body=present function_generation=present identity=stable_cross_module point=present publication_order=deterministic site_ordinal=present source_span=present"),
			})

		case front.NativeTopologyKernelOccurrence:
			row := draft.KernelOccurrence
			if row == nil {
				continue
			}
			var family factkey.Family
			var value string
			switch row.Operation {
			case front.NativeKernelEvalClosure:
				family, value = factkey.EvalNode, "operation=closure"
			case front.NativeKernelEvalLength:
				family, value = factkey.EvalNode, "operation=length"
			case front.NativeKernelClaimAssert:
				family, value = factkey.ThrowTemplate, claimAssertThrowTemplateValue
			default:
				return nil, fmt.Errorf("engine: unsupported native kernel occurrence %d", row.Operation)
			}
			facts = append(facts, equation.Fact{
				Key: factkey.BuildSubjectKey(
					family, factkey.CoordinatePart(fmt.Sprintf("%x", row.Site.Body)),
					nativeTopologyOperationName(row.Site.Position),
				).String(),
				Value: []byte(value),
			})

		case front.NativeTopologyRecordConstruction, front.NativeTopologyShape,
			front.NativeTopologyShapeEpoch, front.NativeTopologyShapeTransition,
			front.NativeTopologyDiscriminant, front.NativeTopologyRecursiveType,
			front.NativeTopologySummary, front.NativeTopologyFunctionEntry,
			front.NativeTopologyCallee, front.NativeTopologyEffect:
			// These families share shape identities and constructor producer
			// edges, so they are derived together before the per-row pass.

		default:
			return nil, fmt.Errorf("engine: unsupported native topology kind %d", draft.Kind)
		}
	}
	return facts, nil
}

func nativeCallGraphConclusions(draft *front.NativeCallGraphDraft) []equation.Fact {
	if draft == nil {
		return nil
	}
	adj := make(map[string]map[string]bool, len(draft.Bodies))
	for _, body := range draft.Bodies {
		if body.Display != "" {
			adj[body.Display] = make(map[string]bool)
		}
	}
	for _, edge := range draft.Edges {
		if adj[edge.From] != nil && adj[edge.To] != nil {
			adj[edge.From][edge.To] = true
		}
	}
	types := make(map[string]front.NativeBodyTypeDraft, len(draft.Types))
	for _, item := range draft.Types {
		types[item.Display] = item
	}
	var facts []equation.Fact
	for ordinal, component := range nativeTopologySCCs(adj) {
		if len(component) == 1 && !adj[component[0]][component[0]] {
			continue
		}
		var edges []string
		for _, from := range component {
			for to := range adj[from] {
				if nativeTopologyContains(component, to) {
					edges = append(edges, from+"->"+to)
				}
			}
		}
		sort.Strings(edges)
		arguments := "[]"
		bodyType := types[component[0]]
		if len(bodyType.FirstParameter) != 0 {
			if parameter, err := typ.DecodeCanonical(context.Background(), bodyType.FirstParameter); err == nil && parameter != nil {
				arguments = "[" + parameter.String() + "]"
			}
		}
		resultSlots := bodyType.ResultSlots
		value := fmt.Sprintf(
			"arguments=%s completions={'known': ['normal', 'throw', 'user_suspend', 'system_suspend'], 'present': ['normal', 'throw']} edges_closed=[%s] members=[%s] results={'exact': True, 'count': %d}",
			arguments, strings.Join(edges, ","), strings.Join(component, ","), resultSlots,
		)
		var revocations []string
		if len(component) > 1 {
			revocations = []string{"write.local"}
		}
		key := nativeTopologyContractKey(factkey.CallSCC, ordinal, "", revocations)
		facts = append(facts, equation.Fact{Key: key, Value: []byte(value)})
	}
	return facts
}

func nativeShapeAndRecordConclusions(operation equation.BoundEquation, drafts []front.NativeTopologyDraft) []equation.Fact {
	epochIDs := make(map[uint64]bool)
	declaredIDs := make(map[uint64]bool)
	for _, draft := range drafts {
		if draft.ShapeEpoch != nil {
			if id, ok := nativeShapeDraftID(draft.ShapeEpoch.Fields); ok &&
				len(draft.ShapeEpoch.ReadSites) != 0 && len(draft.ShapeEpoch.WriteSites) != 0 {
				epochIDs[id] = true
			}
		}
		if draft.Shape != nil && draft.Shape.Origin != front.NativeShapeConstructor {
			if id, ok := nativePhysicalShapeDraftID(draft.Shape); ok {
				declaredIDs[id] = true
			}
		}
	}

	var conclusions []nativeTopologyConclusion
	var projected []equation.Fact
	for _, draft := range drafts {
		switch {
		case draft.Shape != nil:
			id, ok := nativePhysicalShapeDraftID(draft.Shape)
			if !ok || epochIDs[id] {
				continue
			}
			if draft.Shape.Origin == front.NativeShapeConstructor && !declaredIDs[id] {
				continue
			}
			conclusions = append(conclusions, nativeTopologyConclusion{
				family:      factkey.ShapeIdentity,
				value:       fmt.Sprintf("distinct_identities=1 field_offsets=identical field_order=canonical interned=true shape_id=%016x stable_across_modules=true stable_across_sites=true", id),
				revocations: []string{"shape.transition"},
			})

		case draft.ShapeEpoch != nil:
			row := draft.ShapeEpoch
			id, ok := nativeShapeDraftID(row.Fields)
			if !ok || len(row.ReadSites) == 0 || len(row.WriteSites) == 0 {
				continue
			}
			value := fmt.Sprintf("epoch=field_read field_offsets=identical interned=true shape_id=%016x stable=true", id)
			for read := range row.ReadSites {
				projection := front.NativeProjection{
					Key: factkey.ShapeIdentity.Key().String() + fmt.Sprintf("%x", row.Body) + "/epoch/" +
						row.Receiver.Display + "/" + fmt.Sprintf("%d", read) +
						"/contract-revocation/write.field,shape.transition,meta.set,call.opaque",
					Value: value, Subject: row.Receiver.Display,
				}
				if fact, ok := nativeProjectionFact(operation, projection); ok {
					projected = append(projected, fact)
				}
			}

		case draft.ShapeTransition != nil:
			row := draft.ShapeTransition
			oldID, oldOK := nativeShapeDraftID(row.Before)
			after := append([]front.NativeShapeFieldDraft(nil), row.Before...)
			after = append(after, front.NativeShapeFieldDraft{Name: row.AddedField})
			newID, newOK := nativeShapeDraftID(after)
			if !oldOK || !newOK || oldID == newID {
				continue
			}
			for _, id := range []uint64{oldID, newID} {
				conclusions = append(conclusions, nativeTopologyConclusion{
					family:      factkey.ShapeIdentity,
					value:       fmt.Sprintf("field_offsets=identical field_order=canonical interned=true shape_id=%016x", id),
					revocations: []string{"shape.transition"},
				})
			}
			conclusions = append(conclusions, nativeTopologyConclusion{
				family:      factkey.ShapeTransition,
				value:       "new_shape=published old_shape=published same_object_policy=published storage_offset=published transition_edge=published new_identity=minted old_identity_reused=false",
				revocations: []string{"shape.transition"},
			})

		case draft.Record != nil:
			record, edges, ok := nativeRecordConclusion(draft.Record)
			if !ok {
				continue
			}
			conclusions = append(conclusions, record)
			conclusions = append(conclusions, edges...)
		}
	}
	sort.SliceStable(conclusions, func(i, j int) bool {
		if conclusions[i].family.ID != conclusions[j].family.ID {
			return conclusions[i].family.ID < conclusions[j].family.ID
		}
		if conclusions[i].value != conclusions[j].value {
			return conclusions[i].value < conclusions[j].value
		}
		if conclusions[i].subject != conclusions[j].subject {
			return conclusions[i].subject < conclusions[j].subject
		}
		return strings.Join(conclusions[i].revocations, "/") < strings.Join(conclusions[j].revocations, "/")
	})
	var previous factkey.FamilyID
	ordinal := 0
	for index, row := range conclusions {
		if index == 0 || row.family.ID != previous {
			previous = row.family.ID
			ordinal = 0
		}
		key := nativeTopologyContractKey(row.family, ordinal, row.subject, row.revocations)
		projected = append(projected, equation.Fact{Key: key, Value: []byte(row.value)})
		ordinal++
	}
	return projected
}

func nativePhysicalShapeDraftID(draft *front.NativeShapeTopologyDraft) (uint64, bool) {
	if draft == nil || draft.OpenParts != 0 || draft.MapParts != 0 || draft.MetatableRefs != 0 || draft.StaticMembers != 0 {
		return 0, false
	}
	return nativeShapeDraftID(draft.Fields)
}

func nativeShapeDraftID(fields []front.NativeShapeFieldDraft) (uint64, bool) {
	if len(fields) == 0 {
		return 0, false
	}
	ordered := append([]front.NativeShapeFieldDraft(nil), fields...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	recordFields := make([]typ.Field, 0, len(ordered))
	for index, field := range ordered {
		if field.Name == "" || field.Optional != 0 ||
			(index > 0 && ordered[index-1].Name == field.Name) {
			return 0, false
		}
		recordFields = append(recordFields, typ.Field{
			Name: field.Name, Type: typ.Unknown, Readonly: field.Readonly != 0,
		})
	}
	shape := typ.RebuildRecord(typ.RecordParts{Fields: recordFields})
	digest, err := typ.DigestCanonical(context.Background(), shape)
	if err != nil {
		return 0, false
	}
	id := binary.BigEndian.Uint64(digest[:8])
	return id, id != 0
}

func nativeRecordConclusion(record *front.NativeRecordTopologyDraft) (
	nativeTopologyConclusion,
	[]nativeTopologyConclusion,
	bool,
) {
	if record == nil || record.EntrySlots == 0 || record.KeySlots != record.EntrySlots {
		return nativeTopologyConclusion{}, nil, false
	}
	direct := 0
	boolean, multiplication := false, false
	type edge struct {
		field string
		value string
		moved bool
	}
	var edges []edge
	consumed := make(map[string]bool)
	previous := front.NativeSpanDraft{}
	order := true
	for _, entry := range record.Entries {
		if entry.Field != "" {
			direct++
		}
		if entry.Value.Shape == front.NativeOperandLiteral {
			if scalar, ok := shapefact.DecodeScalar(entry.Value.Literal); ok && scalar.Kind == shapefact.ScalarBool {
				boolean = true
			}
		}
		if entry.ProducerOp == front.NativeProducerMultiply {
			multiplication = true
		}
		if entry.Field != "" && entry.ProducerSite != 0 {
			identity := entry.Value.Term
			if identity == "" {
				identity = string(entry.Value.Literal)
			}
			edges = append(edges, edge{field: entry.Field, value: identity, moved: !consumed[identity]})
			consumed[identity] = true
		}
		if entry.ValueSpan.StartLine == 0 ||
			(previous.StartLine != 0 && nativeTopologySpanBefore(entry.ValueSpan, previous)) {
			order = false
		}
		previous = entry.ValueSpan
	}
	if direct == 0 {
		return nativeTopologyConclusion{}, nil, false
	}
	value := fmt.Sprintf("entries=%d entry_storage=committed", direct)
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
	if order {
		value += " evaluation_order=preserved"
	}
	value += " fresh=true"
	var revocations []string
	if len(record.MemberWrites) != 0 && len(record.AliasSites) == 0 {
		value += " ownership=move"
		revocations = []string{"escape", "call.opaque", "meta.set"}
	} else if len(record.CallUses) != 0 {
		revocations = []string{"escape"}
	}
	main := nativeTopologyConclusion{
		family: factkey.RecordConstruction, value: value,
		subject: record.Destination.Term, revocations: revocations,
	}
	entryRows := make([]nativeTopologyConclusion, 0, len(edges))
	for _, edge := range edges {
		ownership := "retain"
		if edge.moved {
			ownership = "move"
		}
		entryRows = append(entryRows, nativeTopologyConclusion{
			family:      factkey.RecordEntryOwnership,
			value:       fmt.Sprintf("field=%s ownership=%s producer_bound=true write_barrier=required", edge.field, ownership),
			revocations: []string{"write.field"},
		})
	}
	return main, entryRows, true
}

func nativeTopologySpanBefore(left, right front.NativeSpanDraft) bool {
	if left.StartLine != right.StartLine {
		return left.StartLine < right.StartLine
	}
	return left.StartCol < right.StartCol
}

func nativeOtherTopologyConclusions(operation equation.BoundEquation, drafts []front.NativeTopologyDraft) []equation.Fact {
	var conclusions []nativeTopologyConclusion
	var projected []equation.Fact
	for _, draft := range drafts {
		switch {
		case draft.Discriminant != nil:
			row, ok := nativeDiscriminantConclusion(draft.Discriminant)
			if ok {
				conclusions = append(conclusions, row)
			}
		case draft.Recursive != nil:
			row := draft.Recursive
			if row.CycleRecordNodes == 0 {
				continue
			}
			value := "fixpoint=reached identity_equal_to_subject=true identity_stable=true traversal_caches=1"
			if row.CycleRecordNodes > 1 {
				value += " mutual=true"
			}
			for count := uint32(0); count < row.CycleRecordNodes; count++ {
				conclusions = append(conclusions, nativeTopologyConclusion{
					family: factkey.RecursiveTypeIdentity, value: value,
					revocations: []string{"shape.transition"},
				})
			}
		case draft.Summary != nil:
			projection, ok := nativeSummaryConclusion(draft.Summary)
			if !ok {
				continue
			}
			if fact, encoded := nativeProjectionFact(operation, projection); encoded {
				projected = append(projected, fact)
			}
		case draft.FunctionEntry != nil:
			conclusions = append(conclusions, nativeFunctionEntryConclusion(draft.FunctionEntry))
		case draft.Callee != nil:
			conclusions = append(conclusions, nativeCalleeConclusion(draft.Callee))
		case draft.Effect != nil:
			if row, ok := nativeEffectConclusion(draft.Effect); ok {
				conclusions = append(conclusions, row)
			}
		}
	}
	sort.SliceStable(conclusions, func(i, j int) bool {
		if conclusions[i].family.ID != conclusions[j].family.ID {
			return conclusions[i].family.ID < conclusions[j].family.ID
		}
		return conclusions[i].value < conclusions[j].value
	})
	var previous factkey.FamilyID
	ordinal := 0
	for index, row := range conclusions {
		if index == 0 || row.family.ID != previous {
			previous = row.family.ID
			ordinal = 0
		}
		key := nativeTopologyContractKey(row.family, ordinal, "", row.revocations)
		projected = append(projected, equation.Fact{Key: key, Value: []byte(row.value)})
		ordinal++
	}
	return projected
}

func nativeFunctionEntryConclusion(draft *front.NativeFunctionEntryDraft) nativeTopologyConclusion {
	params := fmt.Sprintf("{'exact': True, 'count': %d}", draft.Parameters)
	if draft.Varargs != 0 {
		params = fmt.Sprintf("{'exact': False, 'prefix': %d, 'open_tail': True}", draft.Parameters)
	}
	present := "['normal']"
	if len(draft.ErrorCalls) != 0 {
		present = "['normal', 'throw']"
	}
	results := "{'exact': True, 'count': 0}"
	if len(draft.Returns) != 0 {
		open := false
		for _, row := range draft.Returns {
			open = open || row.OpenTail != 0
		}
		if open {
			results = "{'exact': False, 'prefix': 0, 'open_tail': True}"
		} else {
			results = fmt.Sprintf("{'exact': True, 'count': %d}", draft.Returns[0].Slots)
		}
	}
	return nativeTopologyConclusion{
		family: factkey.FunctionEntry,
		value: fmt.Sprintf("params=%s completions={'known': ['normal', 'throw', 'user_suspend', 'system_suspend'], 'present': %s} results=%s",
			params, present, results),
	}
}

func nativeCalleeConclusion(draft *front.NativeCalleeTopologyDraft) nativeTopologyConclusion {
	row := nativeTopologyConclusion{family: factkey.CalleeSet, value: "completeness=unknown"}
	switch draft.Topology {
	case front.NativeCalleeDirectLexical:
		row.value = "cardinality=1 completeness=complete"
	case front.NativeCalleeLocalAlternatives:
		row.value = fmt.Sprintf("cardinality=%d completeness=incomplete", len(draft.TargetSymbols))
		row.revocations = []string{"write.local"}
	case front.NativeCalleeParameter:
		row.revocations = []string{"escape"}
	case front.NativeCalleeLiteralMember:
		row.value = "cardinality=1 completeness=complete"
		row.revocations = []string{"write.field", "meta.set"}
	case front.NativeCalleeOpen:
		if len(draft.ModuleLoadSites) != 0 {
			row.revocations = []string{"write.field", "load.dynamic"}
		}
	}
	return row
}

func nativeEffectConclusion(draft *front.NativeEffectTopologyDraft) (nativeTopologyConclusion, bool) {
	row := nativeTopologyConclusion{family: factkey.EffectRow}
	switch draft.Operation {
	case front.NativeEffectFunction:
		if len(draft.OpenCallSites) != 0 {
			row.value = "exhaustive=false"
			return row, true
		}
		allocation, errorState := "absent", "absent"
		if len(draft.AllocationSites) != 0 {
			allocation = "present"
		}
		if len(draft.ErrorCallSites) != 0 {
			errorState = "present"
		}
		row.value = "allocation=" + allocation + " error=" + errorState + " exhaustive=true yield=absent"
	case front.NativeEffectChannelSelect:
		row.value = "exhaustive=true safepoint=required suspension=published yield=present"
	case front.NativeEffectCoroutineYield:
		row.value = "control_transfer=suspend exhaustive=true suspension=published yield=present"
	case front.NativeEffectCoroutineResume:
		row.value = "control_transfer=resume exhaustive=true safepoint=required suspension=published yield=present"
	case front.NativeEffectRegisteredSuspend:
		row.value = "exhaustive=true safepoint=required suspension=published yield=present"
	case front.NativeEffectStringGsub:
		row.value = "allocation=present composed_from_callback=true control_transfer=callback exhaustive=true"
	case front.NativeEffectTableSort:
		row.value = "control_transfer=callback error=present exhaustive=true safepoint=required"
	case front.NativeEffectModuleLoad:
		row.value = "exhaustive=false module_loading=present"
	case front.NativeEffectProtectedCall:
		if len(draft.ArgumentShapes) != 0 && draft.ArgumentShapes[0] == front.NativeOperandSymbol {
			row.value = "composed_from_callback=true error=absent exhaustive=true"
		} else {
			row.value = "exhaustive=false"
		}
	case front.NativeEffectDirectLexicalCall:
		row.value = "allocation=absent error=absent exhaustive=true safepoint=not_required yield=absent"
	case front.NativeEffectOpenCall:
		row.value = "exhaustive=false"
	default:
		return nativeTopologyConclusion{}, false
	}
	return row, true
}

func nativeDiscriminantConclusion(draft *front.NativeDiscriminantDraft) (nativeTopologyConclusion, bool) {
	if draft == nil || draft.Field == "" || len(draft.Cases) < 2 {
		return nativeTopologyConclusion{}, false
	}
	ordered := append([]front.NativeDiscriminantCaseDraft(nil), draft.Cases...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Ordinal < ordered[j].Ordinal })
	booleanDomain := len(ordered) == 2
	seenTrue, seenFalse := false, false
	for index, item := range ordered {
		if item.Ordinal != uint32(index) || len(item.Literal) == 0 {
			return nativeTopologyConclusion{}, false
		}
		literal, err := typ.DecodeCanonical(context.Background(), item.Literal)
		if err != nil || literal == nil {
			return nativeTopologyConclusion{}, false
		}
		if value, ok := literal.(*typ.Literal); ok {
			if boolean, ok := value.Value.(bool); ok {
				seenTrue = seenTrue || boolean
				seenFalse = seenFalse || !boolean
				continue
			}
		}
		booleanDomain = false
	}
	covered := make(map[uint32]bool)
	for _, ordinal := range draft.MatchedCases {
		if ordinal < uint32(len(ordered)) {
			covered[ordinal] = true
		}
	}
	if len(draft.TruthySites) != 0 && booleanDomain && seenTrue && seenFalse {
		for _, item := range ordered {
			covered[item.Ordinal] = true
		}
	}
	if len(covered) == 0 {
		return nativeTopologyConclusion{}, false
	}
	exhaustive := len(covered) == len(ordered)
	mapping := make([]string, len(covered))
	for index := range mapping {
		mapping[index] = fmt.Sprintf("%d", index)
	}
	return nativeTopologyConclusion{
		family: factkey.DiscriminantSelect,
		value: fmt.Sprintf("cases=%d default_required=%t dense_mapping=[%s] discriminant_field=%s exhaustive=%t",
			len(covered), !exhaustive, strings.Join(mapping, ","), draft.Field, exhaustive),
		revocations: []string{"write.field"},
	}, true
}

func nativeSummaryConclusion(draft *front.NativeSummaryTopologyDraft) (front.NativeProjection, bool) {
	if draft == nil || len(draft.Results) == 0 {
		return front.NativeProjection{}, false
	}
	generic := false
	for _, encoded := range draft.Parameters {
		value, err := typ.DecodeCanonical(context.Background(), encoded)
		if err != nil || value == nil {
			return front.NativeProjection{}, false
		}
		generic = generic || typ.ContainsTypeParam(value)
	}
	exact := true
	for _, encoded := range draft.Results {
		value, err := typ.DecodeCanonical(context.Background(), encoded)
		if err != nil || value == nil {
			return front.NativeProjection{}, false
		}
		generic = generic || typ.ContainsTypeParam(value)
		exact = exact && nativeTopologyExactResultType(value)
	}
	invariance, revocation := "caller_invariant", ""
	if generic {
		invariance = "context_sensitive"
	}
	if len(draft.MutableCaptures) != 0 {
		invariance, revocation = "context_sensitive", "write.upvalue"
	}
	exactness := "over_approximation"
	if exact && !generic {
		exactness = "exact"
	}
	key := factkey.InterprocSummary.Key().String() + fmt.Sprintf("%x", draft.Body.Body)
	if revocation != "" {
		key += "/contract-revocation/" + revocation
	}
	return front.NativeProjection{
		Key: key, Value: "exactness=" + exactness + " invariance=" + invariance,
		Subject: draft.Body.Display,
	}, true
}

func nativeTopologyExactResultType(result typ.Type) bool {
	result = unwrap.Alias(result)
	if result == nil || typ.AbsentOrTopLike(result) || typ.ContainsTypeParam(result) {
		return false
	}
	switch result.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Literal,
		kind.Record, kind.Array, kind.Map, kind.ReadonlyMap, kind.Tuple, kind.Function:
		return true
	default:
		return false
	}
}

func nativeTopologySCCs(adj map[string]map[string]bool) [][]string {
	var out [][]string
	index := 0
	indices, low := map[string]int{}, map[string]int{}
	stack := []string{}
	on := map[string]bool{}
	var visit func(string)
	visit = func(vertex string) {
		index++
		indices[vertex], low[vertex] = index, index
		stack = append(stack, vertex)
		on[vertex] = true
		edges := make([]string, 0, len(adj[vertex]))
		for next := range adj[vertex] {
			edges = append(edges, next)
		}
		sort.Strings(edges)
		for _, next := range edges {
			if indices[next] == 0 {
				visit(next)
				if low[next] < low[vertex] {
					low[vertex] = low[next]
				}
			} else if on[next] && indices[next] < low[vertex] {
				low[vertex] = indices[next]
			}
		}
		if low[vertex] != indices[vertex] {
			return
		}
		var component []string
		for {
			item := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			on[item] = false
			component = append(component, item)
			if item == vertex {
				break
			}
		}
		sort.Strings(component)
		out = append(out, component)
	}
	keys := make([]string, 0, len(adj))
	for key := range adj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if indices[key] == 0 {
			visit(key)
		}
	}
	return out
}

func nativeTopologyContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func nativeConstantInputsKnown(inputs []front.NativeOperandDraft, body [32]byte, known map[string]bool) bool {
	if len(inputs) == 0 {
		return false
	}
	for _, input := range inputs {
		switch input.Shape {
		case front.NativeOperandLiteral:
			if len(input.Literal) == 0 {
				return false
			}
		case front.NativeOperandSymbol, front.NativeOperandTemporary:
			if input.Term == "" || !known[nativeTopologyTermKey(body, input.Term)] {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func nativeTopologyTermKey(body [32]byte, term string) string {
	return string(body[:]) + "\x00" + term
}

func nativeTopologyCoordinate(draft front.NativeTopologyDraft) (string, uint32) {
	switch draft.Kind {
	case front.NativeTopologyCallGraph:
		return "", 0
	case front.NativeTopologyConstantCandidate:
		if draft.Constant != nil {
			return string(draft.Constant.Site.Body[:]), draft.Constant.Site.Position
		}
	case front.NativeTopologyPublicationSite:
		if draft.Publication != nil {
			return string(draft.Publication.Site.Body[:]), draft.Publication.Site.Position
		}
	case front.NativeTopologyKernelOccurrence:
		if draft.KernelOccurrence != nil {
			return string(draft.KernelOccurrence.Site.Body[:]), draft.KernelOccurrence.Site.Position
		}
	}
	return "", 0
}

func nativeTopologyOperationName(position uint32) string {
	return fmt.Sprintf("op-%08d", position)
}
