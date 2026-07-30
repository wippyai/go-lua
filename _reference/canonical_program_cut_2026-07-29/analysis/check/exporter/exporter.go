// Package exporter projects closed equation facts into a module export type.
package exporter

import (
	"context"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/module/exportrelation"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Derive returns the first static return type from evaluated facts.
func Derive(result engine.Result) typ.Type {
	order := operationOrder(result.Artifact)
	var alternatives []typ.Type
	for _, fact := range result.ReturnCandidates {
		candidate, slot, ok := returnCandidate(fact)
		if !ok || slot != 0 {
			continue
		}
		alternatives = append(alternatives, deriveValue(fact.Value, candidate, result, order))
	}
	if len(alternatives) == 0 {
		return typ.Unknown
	}
	return typ.MaterializeUnion(alternatives)
}

// DeriveSummary projects only relations established by engine facts.
func DeriveSummary(result engine.Result) exportrelation.Summary {
	export := Derive(result)
	return exportrelation.Summary{Type: export, Functions: deriveFunctions(result, export)}
}

func deriveFunctions(result engine.Result, export typ.Type) []exportrelation.Function {
	providers := publishedProviderRelations(result)
	paths := make(map[string]bool, len(result.FunctionEscapes)+len(result.FunctionAllocatedReturns)+len(result.FunctionReturnTuples)+len(result.FunctionReturnTemplates)+len(result.FunctionConditionalReturns)+len(result.FunctionForwardedReturns)+len(providers))
	for path := range result.FunctionEscapes {
		paths[path] = true
	}
	for path := range result.FunctionAllocatedReturns {
		paths[path] = true
	}
	for path := range result.FunctionReturnTuples {
		paths[path] = true
	}
	for path := range result.FunctionReturnTemplates {
		paths[path] = true
	}
	for path := range result.FunctionConditionalReturns {
		paths[path] = true
	}
	for path := range result.FunctionForwardedReturns {
		paths[path] = true
	}
	for path := range providers {
		paths[path] = true
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	out := make([]exportrelation.Function, 0, len(ordered))
	for _, path := range ordered {
		if provider, found := providers[path]; found {
			if provider.Valid() {
				out = append(out, provider)
			}
			continue
		}
		function, ok := publishedFunction(export, path)
		if !ok || function.Variadic != nil {
			continue
		}
		relation := exportrelation.Function{
			Path:            path,
			Arity:           len(function.Params),
			AllocatedReturn: result.FunctionAllocatedReturns[path],
		}
		store, borrow := escapeRelations(result.FunctionEscapes[path], relation.Arity)
		if store != nil {
			relation.Store = store
		} else if tuples, ok := completeEvaluatedReturnTuples(result.FunctionReturnTuples[path]); ok {
			relation.ReturnTuples = tuples
		} else if forwarded, ok := result.FunctionForwardedReturns[path]; ok && forwarded.Valid(relation.Arity) {
			relation.Return = forwarded
			relation.Forwarded = true
		} else if templates := result.FunctionReturnTemplates[path]; len(templates) == 1 && templates[0].Valid(relation.Arity) {
			relation.Return = templates[0]
		} else if conditional := result.FunctionConditionalReturns[path]; conditional.Valid(relation.Arity) {
			relation.Conditional = conditional
		} else if len(borrow) != 0 {
			relation.Borrow = borrow
		}
		if relation.Valid() {
			out = append(out, relation)
		}
	}
	return out
}

// publishedFunction resolves a relation path only through the already-derived
// module export type. It supplies arity and rejects stale or optional members.
func publishedFunction(export typ.Type, path string) (*typ.Function, bool) {
	if path == "" {
		return nil, false
	}
	current := export
	for _, name := range strings.Split(path, ".") {
		record, ok := unwrap.Alias(current).(*typ.Record)
		if !ok || record == nil {
			return nil, false
		}
		field := record.GetField(name)
		if field == nil || field.Optional || field.Type == nil {
			return nil, false
		}
		current = field.Type
	}
	function, ok := unwrap.Alias(current).(*typ.Function)
	return function, ok && function != nil
}

// publishedProviderRelations follows provider identities already carried by
// equation write facts into the returned module root. The provider's callable
// and effect contract comes from the signature registry; no provider name,
// arity, or parameter role is owned by this package.
func publishedProviderRelations(result engine.Result) map[string]exportrelation.Function {
	roots := make(map[string]bool)
	for _, fact := range result.ReturnCandidates {
		candidate, slot, ok := returnCandidate(fact)
		if !ok || slot != 0 {
			continue
		}
		if root, found := returnRoot(result.Artifact, candidate); found {
			roots[root] = true
		}
	}
	if len(roots) == 0 {
		return nil
	}
	providers := make(map[string]string)
	for _, operation := range result.Artifact.Equations {
		target := operationOperand(operation.Operands, "target")
		if target == "" {
			continue
		}
		switch operation.Occurrence.Kind {
		case "environment-write":
			invalidateProviderTerms(providers, target)
			name := operationOperand(operation.Operands, "source-display")
			if _, ok := providerOwnershipRelation(name, ""); ok {
				providers[target] = name
			}
		case "path-replacement":
			value := operationOperand(operation.Operands, "value")
			invalidateProviderTerms(providers, target)
			if name := providers[value]; name != "" {
				providers[target] = name
			}
		}
	}
	out := make(map[string]exportrelation.Function)
	for term, name := range providers {
		for root := range roots {
			path, found := strings.CutPrefix(term, root+".")
			if !found || path == "" {
				continue
			}
			relation, ok := providerOwnershipRelation(name, path)
			if ok {
				out[path] = relation
			}
		}
	}
	return out
}

func operationOperand(operands []equation.Operand, role string) string {
	for _, operand := range operands {
		if operand.Role.Wire() == role && !operand.Term.Entry {
			return string(operand.Term.Encoding)
		}
	}
	return ""
}

func invalidateProviderTerms(providers map[string]string, target string) {
	for term := range providers {
		if term == target || strings.HasPrefix(term, target+".") {
			delete(providers, term)
		}
	}
}

func providerOwnershipRelation(name, path string) (exportrelation.Function, bool) {
	if name == "" {
		return exportrelation.Function{}, false
	}
	provider, found := (signaturelookup.Source{IncludeStdlib: true}).LookupView(name)
	if !found || provider.Type == nil || provider.Type.Variadic != nil {
		return exportrelation.Function{}, false
	}
	arity := len(provider.Type.Params)
	for _, label := range provider.Effect.Labels {
		store, ok := effect.NormalizeLabel(label).(ownership.Store)
		if !ok || store.Param.Index < 0 || store.Param.Index >= arity ||
			store.Into.Index < 0 || store.Into.Index >= arity || store.Param.Index == store.Into.Index {
			continue
		}
		relation := exportrelation.Function{
			Path:  path,
			Arity: arity,
			Store: &exportrelation.OwnershipStore{Value: store.Param.Index, Owner: store.Into.Index},
		}
		return relation, path == "" || relation.Valid()
	}
	return exportrelation.Function{}, false
}

// escapeRelations consumes the complete manifest escape vocabulary. Borrow,
// Retain, and Store have representable cross-module call optimizations.
// None carries no relation; Send, Export, and Opaque intentionally reserve no
// optimization because the export relation has no shared/return/opaque
// ownership form. Those cases remain conservative instead of being mistaken
// for a borrow.
func escapeRelations(escapes []signature.ParamRelation, arity int) (*exportrelation.OwnershipStore, []int) {
	var borrow []int
	for _, relation := range escapes {
		if relation.Param < 0 || relation.Param >= arity {
			continue
		}
		switch relation.EscapeClass {
		case placement.None:
			// No cross-frame relation.
		case placement.Borrow:
			borrow = append(borrow, relation.Param)
		case placement.Store, placement.Retain:
			if relation.HasStoredInto && relation.StoredInto >= 0 && relation.StoredInto < arity && relation.StoredInto != relation.Param {
				return &exportrelation.OwnershipStore{Value: relation.Param, Owner: relation.StoredInto}, nil
			}
			return &exportrelation.OwnershipStore{Value: relation.Param, EscapingRoot: true}, nil
		case placement.Send, placement.Export, placement.Opaque:
			// Shared, return-through, and opaque escapes deliberately publish no
			// importer-side ownership optimization.
		}
	}
	sort.Ints(borrow)
	return nil, borrow
}

// completeEvaluatedReturnTuples converts only the engine's complete candidate
// catalog into the small cross-module relation language. Exact scalars retain
// their values; a closed non-nil shape retains only its presence proof.
func completeEvaluatedReturnTuples(candidates [][][]byte) ([]exportrelation.ReturnTuple, bool) {
	if len(candidates) == 0 {
		return nil, false
	}
	tuples := make([]exportrelation.ReturnTuple, 0, len(candidates))
	arity := -1
	for _, candidate := range candidates {
		if len(candidate) < 2 || (arity >= 0 && len(candidate) != arity) {
			return nil, false
		}
		arity = len(candidate)
		tuple := exportrelation.ReturnTuple{Values: make([]exportrelation.Value, len(candidate)), Present: make([]bool, len(candidate))}
		for index, value := range candidate {
			item, present, ok := evaluatedReturnValue(value)
			if !ok {
				return nil, false
			}
			tuple.Values[index] = item
			tuple.Present[index] = present
		}
		tuples = append(tuples, tuple)
	}
	return tuples, true
}

func evaluatedReturnValue(value []byte) (exportrelation.Value, bool, bool) {
	scalar := exportrelation.Value{Scalar: string(value)}
	if scalar.Closed() {
		return scalar, false, true
	}
	target, ok := shapefact.DecodeTarget(value)
	if !ok || target == nil || typevalue.ProjectionHasNil(target) {
		return exportrelation.Value{}, false, false
	}
	switch unwrap.Alias(target).Kind() {
	case kind.Any, kind.Unknown, kind.Nil:
		return exportrelation.Value{}, false, false
	default:
		return exportrelation.Value{}, true, true
	}
}

func returnCandidate(fact equation.Fact) (candidate string, slot int, ok bool) {
	value, decoded := equation.DecodeFamilyValue(factkey.ReturnCandidate, fact)
	if !decoded {
		return "", 0, false
	}
	row, valid := value.ReturnCandidate()
	if !valid || row.Field != equation.ReturnCandidateSlot {
		return "", 0, false
	}
	return row.Candidate, row.Index, true
}

func operationOrder(artifact equation.Artifact) map[string]int {
	out := make(map[string]int, len(artifact.Equations))
	for index, operation := range artifact.Equations {
		out[operation.Target.Name] = index
	}
	return out
}

func deriveValue(value []byte, candidate string, result engine.Result, order map[string]int) typ.Type {
	if shape, ok := shapefact.DecodeTable(value); ok {
		fields := decodeTableFields(shape)
		if root, ok := returnRoot(result.Artifact, candidate); ok {
			overlayStaticWrites(fields, root, candidate, result.ValueFacts, order)
			if hasDynamicMutation(result.Artifact, root, candidate, order) {
				return typ.Unknown
			}
		}
		enrichInferredMemberReturns(fields, result.ValueFacts)
		return buildRecord(fields)
	}
	return decodeType(value)
}

// enrichInferredMemberReturns attaches an exported member closure's engine
// inferred return to a function field that declared none. The engine published
// exactly one summary per returned member suffix; a declared return keeps the
// closure's own canonical signature and is left untouched here.
func enrichInferredMemberReturns(fields map[fieldKey]typ.Type, valueFacts []equation.Fact) {
	var summaries map[string][]byte
	for _, fact := range valueFacts {
		if !strings.HasPrefix(fact.Key, returnMemberSummaryFieldPrefix) {
			continue
		}
		name := strings.TrimPrefix(fact.Key, returnMemberSummaryFieldPrefix)
		if name == "" || strings.ContainsAny(name, ".[]") {
			continue
		}
		if summaries == nil {
			summaries = make(map[string][]byte)
		}
		summaries[name] = fact.Value
	}
	if summaries == nil {
		return
	}
	for key, current := range fields {
		if key.kind != segment.SegmentField {
			continue
		}
		encoded, ok := summaries[key.name]
		if !ok {
			continue
		}
		function, ok := unwrap.Alias(current).(*typ.Function)
		if !ok || function == nil || len(function.Returns) != 0 || function.Variadic != nil {
			continue
		}
		returnType, err := typ.DecodeCanonicalStructural(context.Background(), encoded)
		if err != nil || returnType == nil {
			continue
		}
		fields[key] = typ.RebuildFunction(typ.FunctionParts{
			TypeParams: function.TypeParams,
			Params:     function.Params,
			Variadic:   function.Variadic,
			Returns:    []typ.Type{returnType},
		})
	}
}

// returnMemberSummaryFieldPrefix selects only named-field member summaries. The
// engine publishes each summary under the member's formatted suffix, so a
// record field name follows the leading dot.
const returnMemberSummaryFieldPrefix = "return-member-summary/."

func hasDynamicMutation(artifact equation.Artifact, root, candidate string, order map[string]int) bool {
	returnOrder, exists := order[candidate]
	if !exists {
		return true
	}
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "index-mutation" || order[operation.Target.Name] >= returnOrder {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role.Wire() == "container" && string(operand.Term.Encoding) == root {
				return true
			}
		}
	}
	return false
}

type fieldKey struct {
	kind  segment.SegmentKind
	name  string
	index int
}

func decodeTableFields(shape shapefact.Table) map[fieldKey]typ.Type {
	fields := make(map[fieldKey]typ.Type)
	for _, member := range shape.Members {
		if !member.Present {
			continue
		}
		segments, ok := segment.ParseFormattedSegments(member.Suffix)
		if !ok || len(segments) != 1 {
			continue
		}
		part := segments[0]
		fields[fieldKey{kind: part.Kind, name: part.Name, index: part.Index}] = decodeType([]byte(member.Value))
	}
	return fields
}

func returnRoot(artifact equation.Artifact, candidate string) (string, bool) {
	for _, operation := range artifact.Equations {
		if operation.Target.Name != candidate || operation.Occurrence.Kind != "publication" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == equation.IndexedRole(equation.RoleFamilyReturnValue, 0) && strings.HasPrefix(string(operand.Term.Encoding), "path/") {
				return string(operand.Term.Encoding), true
			}
		}
	}
	return "", false
}

func overlayStaticWrites(fields map[fieldKey]typ.Type, root, candidate string, values []equation.Fact, order map[string]int) {
	returnOrder, exists := order[candidate]
	if !exists {
		return
	}
	type latest struct {
		order int
		value []byte
	}
	latestByField := make(map[fieldKey]latest)
	for _, fact := range values {
		parsed, ok := factkey.Value.ParseKey(fact.Key)
		if !ok {
			continue
		}
		rest, descendant := strings.CutPrefix(parsed.Subject.Spelling(), root)
		if !descendant || rest == "" {
			continue
		}
		segments, ok := segment.ParseFormattedSegments(rest)
		if !ok || len(segments) != 1 {
			continue
		}
		writeOrder, exists := order[parsed.Occurrence]
		if !exists || writeOrder >= returnOrder {
			continue
		}
		part := segments[0]
		key := fieldKey{kind: part.Kind, name: part.Name, index: part.Index}
		if prior, exists := latestByField[key]; !exists || writeOrder > prior.order {
			latestByField[key] = latest{order: writeOrder, value: fact.Value}
		}
	}
	for key, value := range latestByField {
		if string(value.value) == shapefact.ScalarNilWire {
			delete(fields, key)
			continue
		}
		fields[key] = decodeType(value.value)
	}
}

func buildRecord(fields map[fieldKey]typ.Type) typ.Type {
	builder := table.NewRecord().SetOpen(true)
	for key, value := range fields {
		switch key.kind {
		case segment.SegmentField:
			builder.Field(key.name, value)
		case segment.SegmentIndexString:
			builder.StaticStringIndex(key.name, value)
		case segment.SegmentIndexInt:
			builder.StaticIntIndex(int64(key.index), value)
		}
	}
	return builder.Build()
}

func decodeType(value []byte) typ.Type {
	if shape, ok := shapefact.DecodeTable(value); ok {
		return buildRecord(decodeTableFields(shape))
	}
	scalar, ok := shapefact.DecodeScalar(value)
	if !ok {
		return typ.Unknown
	}
	payload := shapefact.Payload{Form: shapefact.PayloadScalar, Scalar: scalar}
	if scalar.Kind == shapefact.ScalarFunction {
		if function, ok := payload.FunctionType(); ok {
			return function
		}
		return unknownFunction()
	}
	witness, ok := payload.WitnessType()
	if !ok {
		return typ.Unknown
	}
	return witness
}

func unknownFunction() typ.Type {
	return typ.Func().Variadic(typ.Unknown).Returns(typ.Unknown).Build()
}
