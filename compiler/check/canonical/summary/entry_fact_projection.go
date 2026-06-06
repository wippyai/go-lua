package summary

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
)

// DirectCallEntryFactInput is the call-boundary projection for point-local path
// facts. It consumes normalized runtime arguments plus caller-side argument
// paths and emits parameter-relative boundary facts for the callee entry key.
type DirectCallEntryFactInput struct {
	Call   *ast.FuncCallExpr
	Callee FuncRef

	ParamSlot EntryValueParamSlot
	ArgPath   EntryReferenceArgPath

	KeyPresence flow.KeyPresenceFacts
	Num         *numeric.State
	IndexWrites flow.IndexWriteAdmissionFacts
}

// DirectCallEntryFacts projects caller point-local path facts into callee
// parameter-relative entry facts. Facts are emitted only when every referenced
// path is structurally owned by one of the runtime argument paths.
func DirectCallEntryFacts(in DirectCallEntryFactInput) flow.BoundaryFacts {
	if in.Call == nil || in.ParamSlot == nil || in.ArgPath == nil {
		return flow.BoundaryFactsDomain.Top()
	}
	var keyPresence []flow.BoundaryKeyPresenceFact
	var keyArrays []flow.BoundaryKeyArrayFact
	var keyArrayValues []flow.BoundaryKeyArrayValueFact
	var appendKeys []flow.BoundaryAppendKeyFact
	var appendOrigins []flow.BoundaryAppendElementFieldOriginFact
	if !in.KeyPresence.IsBottom() {
		for _, fact := range in.KeyPresence.Entries() {
			table, ok := entryBoundaryPathForCallerKey(in, fact.Table)
			if !ok {
				continue
			}
			key, ok := entryBoundaryPathForCallerKey(in, fact.Key)
			if !ok {
				continue
			}
			keyPresence = append(keyPresence, flow.BoundaryKeyPresenceFact{Table: table, Key: key})
		}
		for _, fact := range in.KeyPresence.KeyArrayEntries() {
			array, ok := entryBoundaryPathForCallerKey(in, fact.Array)
			if !ok {
				continue
			}
			table, ok := entryBoundaryPathForCallerKey(in, fact.Table)
			if !ok {
				continue
			}
			keyArrays = append(keyArrays, flow.BoundaryKeyArrayFact{Array: array, Table: table})
		}
		for _, fact := range in.KeyPresence.KeyArrayValueEntries() {
			array, ok := entryBoundaryPathForCallerKey(in, fact.Array)
			if !ok {
				continue
			}
			table, ok := entryBoundaryPathForCallerKey(in, fact.Table)
			if !ok {
				continue
			}
			keyArrayValues = append(keyArrayValues, flow.BoundaryKeyArrayValueFact{
				Array: array,
				Table: table,
				Value: fact.Value,
			})
		}
		for _, fact := range in.KeyPresence.AppendedKeyEntries() {
			array, ok := entryBoundaryPathForCallerKey(in, fact.Array)
			if !ok {
				continue
			}
			key, ok := entryBoundaryPathForCallerKey(in, fact.Key)
			if !ok {
				continue
			}
			appendKeys = append(appendKeys, flow.BoundaryAppendKeyFact{Array: array, Key: key})
		}
		for _, fact := range in.KeyPresence.AppendElementFieldOriginEntries() {
			array, ok := entryBoundaryPathForCallerKey(in, fact.Array)
			if !ok {
				continue
			}
			field, ok := flow.AppendElementFieldSegments(fact.Field)
			if !ok {
				continue
			}
			source, ok := entryBoundaryPathForCallerKey(in, fact.Source)
			if !ok {
				continue
			}
			sourceField, _ := flow.AppendElementFieldSegments(fact.SourceField)
			appendOrigins = append(appendOrigins, flow.BoundaryAppendElementFieldOriginFact{
				Array:       array,
				Field:       field,
				Source:      source,
				SourceField: sourceField,
			})
		}
	}
	var lenLower []flow.BoundaryLengthLowerBound
	if in.Num != nil && !in.Num.IsUnsat() {
		in.Num.ForEachLenBound(func(key constraint.PathKey, lower, _ int64) bool {
			if lower <= 0 {
				return true
			}
			target, ok := entryBoundaryPathForCallerKey(in, key)
			if ok {
				lenLower = append(lenLower, flow.BoundaryLengthLowerBound{Target: target, Lower: lower})
			}
			return true
		})
	}
	var indexWrites []flow.BoundaryIndexWriteFact
	if !in.IndexWrites.IsBottom() {
		for _, fact := range in.IndexWrites.Entries() {
			if fact.Target == "" || fact.KeyPath == "" || fact.Value.IsZero() {
				continue
			}
			table, ok := entryBoundaryPathForCallerKey(in, fact.Target)
			if !ok {
				continue
			}
			key, ok := entryBoundaryPathForCallerKey(in, fact.KeyPath)
			if !ok {
				continue
			}
			indexWrites = append(indexWrites, flow.BoundaryIndexWriteFact{
				Table: table,
				Key:   key,
				Value: fact.Value,
			})
		}
	}
	return flow.BoundaryFactsOf(keyPresence, keyArrays, keyArrayValues, appendKeys, lenLower, indexWrites).
		WithAppendElementFieldOrigins(appendOrigins)
}

func entryBoundaryPathForCallerKey(in DirectCallEntryFactInput, key constraint.PathKey) (flow.BoundaryPath, bool) {
	path, ok := flow.StablePathFromKey(key)
	if !ok {
		return flow.BoundaryPath{}, false
	}
	for _, arg := range entryRuntimeArgs(in.Callee, in.Call, in.ParamSlot) {
		source, ok := in.ArgPath(arg.RuntimeIdx, arg.Expr)
		if !ok || source.IsEmpty() {
			continue
		}
		if arg.Slot < 0 {
			continue
		}
		suffix, ok := entryFactPathSuffix(path, source)
		if !ok {
			continue
		}
		return flow.BoundaryPath{
			Kind:     flow.BoundaryPathParam,
			Index:    arg.Slot,
			Segments: suffix,
		}, true
	}
	return flow.BoundaryPath{}, false
}

func entryFactPathSuffix(path, source constraint.Path) ([]constraint.Segment, bool) {
	if !entryFactSameRoot(path, source) || len(source.Segments) > len(path.Segments) {
		return nil, false
	}
	for i := range source.Segments {
		if source.Segments[i] != path.Segments[i] {
			return nil, false
		}
	}
	if len(source.Segments) == len(path.Segments) {
		return nil, true
	}
	return append([]constraint.Segment(nil), path.Segments[len(source.Segments):]...), true
}

func entryFactSameRoot(path, source constraint.Path) bool {
	if path.Symbol != 0 || source.Symbol != 0 {
		return path.Symbol != 0 && path.Symbol == source.Symbol
	}
	return path.Root != "" && path.Root == source.Root
}
