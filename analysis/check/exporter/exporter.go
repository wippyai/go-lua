// Package exporter projects closed equation facts into a module export type.
package exporter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/engine"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// Derive returns the sound static type of a module's first return value. It
// consumes only evaluated equation facts. Opaque results become Unknown rather
// than a guessed structure.
func Derive(result engine.Result) typ.Type {
	order := operationOrder(result.Artifact)
	var alternatives []typ.Type
	for _, fact := range result.ReturnCandidates {
		candidate, slot, ok := returnCandidate(fact)
		if !ok || slot != "0" {
			continue
		}
		alternatives = append(alternatives, deriveValue(fact.Value, candidate, result, order))
	}
	if len(alternatives) == 0 {
		return typ.Unknown
	}
	return typ.MaterializeUnion(alternatives)
}

func returnCandidate(fact equation.Fact) (candidate, slot string, ok bool) {
	parts := strings.Split(fact.Key, "/")
	if len(parts) != 3 || parts[0] != "return-candidate" || parts[1] == "" || parts[2] == "arity" {
		return "", "", false
	}
	return parts[1], parts[2], true
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
		fields := tableFields(shape)
		if root, ok := returnRoot(result.Artifact, candidate); ok {
			overlayStaticWrites(fields, root, candidate, result.ValueFacts, order)
			if hasDynamicMutation(result.Artifact, root, candidate, order) {
				return typ.Unknown
			}
		}
		return buildRecord(fields)
	}
	return scalarType(value)
}

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
			if operand.Role == "container" && string(operand.Term.Encoding) == root {
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

func tableFields(shape shapefact.Table) map[fieldKey]typ.Type {
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
		fields[fieldKey{kind: part.Kind, name: part.Name, index: part.Index}] = scalarOrTableType([]byte(member.Value))
	}
	return fields
}

func returnRoot(artifact equation.Artifact, candidate string) (string, bool) {
	for _, operation := range artifact.Equations {
		if operation.Target.Name != candidate || operation.Occurrence.Kind != "publication" {
			continue
		}
		for _, operand := range operation.Operands {
			if operand.Role == "return-value-00000000" && strings.HasPrefix(string(operand.Term.Encoding), "path/") {
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
	prefix := "value/" + root
	for _, fact := range values {
		if !strings.HasPrefix(fact.Key, prefix) {
			continue
		}
		rest := strings.TrimPrefix(fact.Key, prefix)
		cut := strings.LastIndexByte(rest, '/')
		if cut <= 0 {
			continue
		}
		segments, ok := segment.ParseFormattedSegments(rest[:cut])
		if !ok || len(segments) != 1 {
			continue
		}
		writeOrder, exists := order[rest[cut+1:]]
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
		if string(value.value) == "scalar/nil" {
			delete(fields, key)
			continue
		}
		fields[key] = scalarOrTableType(value.value)
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

func scalarOrTableType(value []byte) typ.Type {
	if shape, ok := shapefact.DecodeTable(value); ok {
		return buildRecord(tableFields(shape))
	}
	return scalarType(value)
}

func scalarType(value []byte) typ.Type {
	encoded := string(value)
	switch {
	case strings.HasPrefix(encoded, "scalar/function/"):
		wire, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, "scalar/function/"))
		if err != nil {
			return unknownFunction()
		}
		var signature struct {
			Canonical string `json:"canonical,omitempty"`
		}
		if json.Unmarshal(wire, &signature) != nil || signature.Canonical == "" {
			return unknownFunction()
		}
		canonical, err := base64.RawURLEncoding.DecodeString(signature.Canonical)
		if err != nil {
			return unknownFunction()
		}
		function, err := typ.DecodeCanonical(context.Background(), canonical)
		if err != nil {
			return unknownFunction()
		}
		if _, ok := function.(*typ.Function); ok {
			return function
		}
		return unknownFunction()
	case encoded == "scalar/nil":
		return typ.Nil
	case encoded == "scalar/bool/true":
		return typ.True
	case encoded == "scalar/bool/false":
		return typ.False
	case strings.HasPrefix(encoded, "scalar/number/"):
		number := strings.TrimPrefix(encoded, "scalar/number/")
		if integer, err := strconv.ParseInt(number, 10, 64); err == nil {
			return typ.LiteralInt(integer)
		}
		if floating, err := strconv.ParseFloat(number, 64); err == nil {
			return typ.LiteralNumber(floating)
		}
		return typ.Unknown
	case strings.HasPrefix(encoded, "scalar/string/"):
		text, err := strconv.Unquote(strings.TrimPrefix(encoded, "scalar/string/"))
		if err != nil {
			return typ.Unknown
		}
		return typ.LiteralString(text)
	case strings.HasPrefix(encoded, "scalar/function"):
		return unknownFunction()
	default:
		return typ.Unknown
	}
}

func unknownFunction() typ.Type {
	return typ.Func().Variadic(typ.Unknown).Returns(typ.Unknown).Build()
}
