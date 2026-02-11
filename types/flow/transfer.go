// transfer.go implements the transfer functions for the flow analysis worklist algorithm.
//
// Transfer functions define how types change at each CFG point. This file handles:
//
//   - Phi node processing: Joining types from multiple incoming branches
//   - Assignment processing: Computing types for assignment targets
//   - Iterator derivation: Computing loop variable types from iterator sources
//   - Widening: Expanding types for table mutations and dynamic indexing
//
// The functions return changed keys to drive worklist propagation.
package flow

import (
	"sort"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow/join"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

// processPointReturnChangedKeys processes all type-changing operations at a CFG point.
//
// This is the main transfer function called by the worklist algorithm. It processes:
//   - Phi nodes: Joining types from multiple control flow paths
//   - Assignments: Computing and storing target types
//
// Returns the list of canonical keys whose types changed, used to determine
// which dependent points need re-processing.
func (s *Solution) processPointReturnChangedKeys(p cfg.Point) []string {
	c := s.inputs.Graph
	node := c.Node(p)
	if node == nil {
		return nil
	}

	var changedKeys []string

	// Process phi nodes at this point
	phiKeys := s.processJoinReturnChangedKeys(p)
	changedKeys = append(changedKeys, phiKeys...)

	assignKeys := s.processAssignmentReturnChangedKeys(p)
	changedKeys = append(changedKeys, assignKeys...)

	return changedKeys
}

// processAssignmentReturnChangedKeys processes all assignments at a CFG point.
//
// For each assignment targeting this point, computes the assigned type by:
//  1. Looking up source path type via NarrowedTypeAt (for flow propagation)
//  2. Deriving iterator variable types from IteratorSource
//  3. Deriving container element types from ContainerElementSource
//  4. Falling back to the statically extracted type
//
// Also handles special assignment types:
//   - IndexerAssignments: Dynamic index (t[k] = v) widening empty tables to maps
//   - TableMutatorAssignments: table.insert-like array element widening
//   - ContainerMutatorAssignments: channel.send-like element type widening
//
// Returns keys that changed, enabling worklist-driven convergence.
func (s *Solution) processAssignmentReturnChangedKeys(p cfg.Point) []string {
	var changedKeys []string

	for _, assign := range s.inputs.Assignments {
		if assign.Point != p {
			continue
		}

		var assignedType typ.Type
		if assign.SourcePath.HasSymbol() {
			// Use NarrowedTypeAt to propagate flow narrowing through assignments.
			// Point conditions are computed before the worklist (see Solve order),
			// so narrowing is available for source paths like `result.value`.
			assignedType = s.NarrowedTypeAt(p, assign.SourcePath)
		}
		if assign.IterSource != nil {
			if iterType := s.iterVarTypeAt(p, assign.IterSource); iterType != nil {
				assignedType = iterType
			}
		}
		// Derive type from container element type if ContainerElementSource is set
		if assign.ContainerElementSource != nil {
			if elemType := s.containerElementTypeAt(p, assign.ContainerElementSource); elemType != nil {
				assignedType = elemType
			}
		}
		if assignedType == nil {
			assignedType = assign.Type
		}

		if assignedType != nil {
			targetKey := s.pkResolver.KeyAt(p, assign.TargetPath)
			if targetKey != "" {
				old := s.values[string(targetKey)]
				if len(assign.TargetPath.Segments) > 0 && assignedType.Kind() == kind.Nil {
					assignedType = s.normalizeNilFieldAssignmentType(p, assign.TargetPath, old)
				}
				if !typ.TypeEquals(old, assignedType) {
					s.values[string(targetKey)] = assignedType
					changedKeys = append(changedKeys, string(targetKey))
				}
			}
		}
	}

	// Process indexer assignments to widen tables
	for _, ia := range s.inputs.IndexerAssignments {
		if ia.Point != p {
			continue
		}
		if key := s.processIndexerAssignmentReturnKey(p, ia); key != "" {
			changedKeys = append(changedKeys, key)
		}
	}

	// Process table mutator assignments (table.insert-like)
	for _, tm := range s.inputs.TableMutatorAssignments {
		if tm.Point != p {
			continue
		}
		if key := s.processTableMutatorAssignmentReturnKey(p, tm); key != "" {
			changedKeys = append(changedKeys, key)
		}
	}

	// Process container mutator assignments (channel.send-like)
	for _, cm := range s.inputs.ContainerMutatorAssignments {
		if cm.Point != p {
			continue
		}
		if key := s.processContainerMutatorAssignmentReturnKey(p, cm); key != "" {
			changedKeys = append(changedKeys, key)
		}
	}

	return changedKeys
}

func (s *Solution) normalizeNilFieldAssignmentType(p cfg.Point, targetPath constraint.Path, old typ.Type) typ.Type {
	if !typ.IsAbsentOrUnknown(old) {
		return typ.JoinPreferNonSoft(old, typ.Nil)
	}
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return typ.Nil
	}

	rootPath := constraint.Path{
		Root:    targetPath.Root,
		Symbol:  targetPath.Symbol,
		Version: targetPath.Version,
	}
	rootType := s.TypeAt(p, rootPath)
	if rootType == nil {
		return typ.Nil
	}
	derived, ok := s.deriveTypeFrom(rootType, targetPath.Segments)
	if !ok || typ.IsAbsentOrUnknown(derived) {
		return typ.Nil
	}
	return typ.JoinPreferNonSoft(derived, typ.Nil)
}

// iterVarTypeAt derives iterator variable type from the iterator source at point p.
//
// For Lua's iteration constructs (for-in loops), variable types depend on the
// iteration kind and variable position:
//
//   - IterateIndexed (ipairs): Index is always integer, value is array element
//   - IterateKeyed (pairs): Key is table key type, value is table value type
//
// Special handling for keys-provenance variables: When iterating over a variable
// that holds sorted keys of another table (via sorted_keys() or similar), the
// element type is derived from the original table's key type rather than the
// intermediate array.
//
// Falls back to declared type when flow type is missing or imprecise, preserving
// annotated element types through widening operations.
func (s *Solution) iterVarTypeAt(p cfg.Point, iter *IteratorSource) typ.Type {
	if iter == nil || iter.Path.IsEmpty() {
		return nil
	}

	// Index variable for ipairs-style iteration is always integer.
	if iter.Kind == IterateIndexed && iter.VarIndex == 0 {
		return typ.Integer
	}

	// For indexed iteration (ipairs) over keys-provenance variables,
	// derive element type from the original table's key type.
	// Pattern: local keys = sorted_keys(t); for _, k in ipairs(keys) do
	if iter.Kind == IterateIndexed && iter.VarIndex == 1 {
		if origTableSym := s.keysProvenanceSource(iter.Path.Symbol); origTableSym != 0 {
			origPath := constraint.Path{Root: s.inputs.Graph.NameOf(origTableSym), Symbol: origTableSym}
			if origType := s.NarrowedTypeAt(p, origPath); origType != nil {
				if keyType := s.inputs.Decomposer.KeyType(origType); keyType != nil {
					return keyType
				}
			}
		}
	}

	srcType := s.NarrowedTypeAt(p, iter.Path)

	// When flow type is missing or imprecise, prefer the declared type
	// so that annotated element types are preserved through widening.
	if srcType == nil || srcType.Kind().IsPlaceholder() {
		if declType := s.lookupDeclaredType(iter.Path); declType != nil {
			if elem := s.elementTypeForIter(declType, iter); elem != nil {
				return elem
			}
		}
	}

	if srcType == nil {
		return nil
	}

	return s.elementTypeForIter(srcType, iter)
}

// keysProvenanceSource returns the original table symbol if sym holds keys of that table.
//
// Some patterns extract keys from a table into an array (e.g., local keys = sorted_keys(t)).
// When iterating over such an array, the element type should be the original table's key type,
// not simply string or the array's element type. This method looks up the provenance chain
// to find the original table.
func (s *Solution) keysProvenanceSource(sym cfg.SymbolID) cfg.SymbolID {
	if sym == 0 || s.inputs.KeysProvenance == nil {
		return 0
	}
	return s.inputs.KeysProvenance[sym]
}

// elementTypeForIter extracts the appropriate element/key/value type.
//
// The extraction depends on iterator kind and variable position:
//
//	IterateIndexed (ipairs):
//	  VarIndex 0: Always integer (handled elsewhere)
//	  VarIndex 1: ElementType (array element)
//
//	IterateKeyed (pairs):
//	  VarIndex 0: KeyType (map/record key)
//	  VarIndex 1: ValueType (map/record value)
//
// Uses TypeDecomposer for extraction to support various container types.
func (s *Solution) elementTypeForIter(srcType typ.Type, iter *IteratorSource) typ.Type {
	d := s.inputs.Decomposer
	switch iter.Kind {
	case IterateIndexed:
		if iter.VarIndex == 1 {
			return d.ElementType(srcType)
		}
	case IterateKeyed:
		switch iter.VarIndex {
		case 0:
			return d.KeyType(srcType)
		case 1:
			return d.ValueType(srcType)
		}
	}
	return nil
}

// containerElementTypeAt derives the element type from a container at point p.
//
// Used for methods like channel:receive() where the return type depends on the
// container's element type at the call site. The element type is derived from:
//
//  1. Instantiated generics (e.g., Channel<T>): Uses first type argument
//  2. Arrays: Uses ElementType via decomposer
//  3. Other containers: Falls back to decomposer.ElementType
//
// This enables accurate return type inference for generic container methods
// even when the container was widened during flow analysis.
func (s *Solution) containerElementTypeAt(p cfg.Point, src *ContainerElementSource) typ.Type {
	if src == nil || src.ContainerPath.IsEmpty() {
		return nil
	}

	containerType := s.NarrowedTypeAt(p, src.ContainerPath)
	if containerType == nil {
		return nil
	}

	// Extract element type from instantiated generics (e.g., Channel<T>)
	if inst, ok := containerType.(*typ.Instantiated); ok {
		if len(inst.TypeArgs) > 0 {
			return inst.TypeArgs[0]
		}
	}

	// Fall back to ElementType for arrays and other containers
	return s.inputs.Decomposer.ElementType(containerType)
}

// processIndexerAssignmentReturnKey handles dynamic index assignments like t[k] = v.
//
// When a table is indexed with a non-constant key (t[k] = v), the table may need
// widening to accommodate dynamic keys. This method:
//
//  1. Skips annotated variables (explicit types are preserved)
//  2. Resolves key type from flow state or explicit override
//  3. Calls widenWithIndexer to expand the table type
//
// Widening rules:
//   - Empty record {} becomes map {[K]: V}
//   - Record with fields gains a map component
//   - Existing map widens key/value types via union
//
// Returns the changed key if widening occurred, empty string otherwise.
func (s *Solution) processIndexerAssignmentReturnKey(p cfg.Point, ia IndexerAssignment) string {
	if ia.Symbol == 0 || ia.ValType == nil {
		return ""
	}

	// Skip widening for variables with explicit type annotations
	if s.inputs.AnnotatedVars != nil {
		if s.inputs.AnnotatedVars[ia.Symbol] {
			return ""
		}
	}

	// Resolve key type from flow state or explicit override
	keyType := ia.KeyType
	if keyType == nil {
		keyType = s.resolveSymbolKeyType(p, ia.KeySymbol, ia.KeyVar)
	}
	if keyType == nil {
		keyType = typ.String // Fallback for unresolvable keys
	}

	// Get canonical key for the root variable
	iaPath := constraint.Path{Root: ia.Root, Symbol: ia.Symbol, Segments: ia.Segments}
	pathKey := s.pkResolver.KeyAt(p, iaPath)
	if pathKey == "" {
		return ""
	}

	// Get current type of the root
	currentType := s.values[string(pathKey)]

	// Compute the widened type
	newType := widenWithIndexer(currentType, keyType, ia.ValType)
	if newType == nil || typ.TypeEquals(currentType, newType) {
		return ""
	}

	s.values[string(pathKey)] = newType
	return string(pathKey)
}

// resolveSymbolKeyType gets the flow-narrowed type for a symbol at point p.
//
// Used to look up key types for dynamic index operations. The symbol ID provides
// unique identity for SSA version lookup, and varName is used to construct
// the path for type lookup.
//
// Returns nil if symbol is zero, as identity-based lookup requires a valid Symbol.
func (s *Solution) resolveSymbolKeyType(p cfg.Point, sym cfg.SymbolID, varName string) typ.Type {
	if sym == 0 {
		return nil
	}
	keyPath := constraint.Path{Root: varName, Symbol: sym}
	if narrowed := s.NarrowedTypeAt(p, keyPath); narrowed != nil {
		return narrowed
	}
	return s.TypeAt(p, keyPath)
}

// processTableMutatorAssignmentReturnKey handles table.insert-like mutations.
//
// When table.insert(t, v) or similar operations are called, the array element
// type may need widening. This method:
//
//  1. Resolves value type from flow state or uses fallback
//  2. Skips annotated variables (explicit types are preserved)
//  3. For keyed targets (suites[name]), widens the map value's array element
//  4. For direct targets, widens the array element type
//
// Returns the changed key if widening occurred, empty string otherwise.
func (s *Solution) processTableMutatorAssignmentReturnKey(p cfg.Point, tm TableMutatorAssignment) string {
	if tm.Target.Symbol == 0 {
		return ""
	}

	// Resolve value type from flow state or use fallback
	valueType := tm.ValueType
	if tm.ValuePath.HasSymbol() {
		if resolved := s.NarrowedTypeAt(p, tm.ValuePath); !typ.IsAbsentOrUnknown(resolved) {
			valueType = resolved
		}
	}
	if valueType == nil {
		return ""
	}

	// Skip widening for variables with explicit type annotations
	if s.inputs.AnnotatedVars != nil {
		if s.inputs.AnnotatedVars[tm.Target.Symbol] {
			return ""
		}
	}

	pathKey := s.pkResolver.KeyAt(p, tm.Target)
	if pathKey == "" {
		return ""
	}

	currentType := s.values[string(pathKey)]

	var newType typ.Type
	if tm.KeySymbol != 0 || tm.KeyType != nil {
		keyType := tm.KeyType
		if keyType == nil {
			keyType = s.resolveSymbolKeyType(p, tm.KeySymbol, tm.KeyVar)
		}
		if keyType == nil {
			keyType = typ.String
		}
		newType = widenMapValueArray(currentType, keyType, valueType)
	} else {
		newType = WidenArrayElementType(currentType, valueType, typ.JoinPreferNonSoft)
	}

	if newType == nil || typ.TypeEquals(currentType, newType) {
		return ""
	}

	s.values[string(pathKey)] = newType
	return string(pathKey)
}

// processContainerMutatorAssignmentReturnKey handles container mutations.
//
// When container:send(v) or similar operations are called, the container's
// element type may need widening to include the sent value's type. This is
// controlled by ContainerElementUnion effects in function specs.
//
// The method:
//  1. Resolves value type from flow state (using ValuePath) or fallback
//  2. Applies WidenForInference to normalize the value type
//  3. Skips annotated variables (explicit types are preserved)
//  4. Widens the container's element type via union
//
// Returns the changed key if widening occurred, empty string otherwise.
func (s *Solution) processContainerMutatorAssignmentReturnKey(p cfg.Point, cm ContainerMutatorAssignment) string {
	if cm.Target.Symbol == 0 {
		return ""
	}

	// Resolve value type from flow state or use fallback
	valueType := cm.ValueType
	if cm.ValuePath.HasSymbol() {
		if resolved := s.NarrowedTypeAt(p, cm.ValuePath); !typ.IsAbsentOrUnknown(resolved) {
			valueType = resolved
		}
	}
	if valueType == nil {
		return ""
	}
	valueType = subtype.WidenForInference(valueType)

	// Skip widening for variables with explicit type annotations
	if s.inputs.AnnotatedVars != nil {
		if s.inputs.AnnotatedVars[cm.Target.Symbol] {
			return ""
		}
	}

	pathKey := s.pkResolver.KeyAt(p, cm.Target)
	if pathKey == "" {
		return ""
	}

	currentType := s.values[string(pathKey)]
	newType := widenContainerElementType(currentType, valueType)

	if newType == nil || typ.TypeEquals(currentType, newType) {
		return ""
	}

	s.values[string(pathKey)] = newType
	return string(pathKey)
}

// widenContainerElementType widens a container's element type by unioning with a new value type.
//
// Supports various container types:
//
//   - Instantiated generics (Channel<T>): Widens first type argument
//   - Arrays: Widens element type
//   - Maps: Widens value type (key unchanged)
//   - Unions: Recursively widens contained containers
//
// Special case for Unknown element type: Rather than unioning (which would keep Unknown),
// replaces Unknown entirely with the value type. This enables precise inference when
// a container starts with unknown element type and is populated with concrete values.
//
// Returns the widened type, or the original if no widening is applicable.
func widenContainerElementType(containerType typ.Type, valueType typ.Type) typ.Type {
	if valueType == nil {
		return containerType
	}
	if containerType == nil {
		return nil
	}

	return typ.Visit(containerType, typ.Visitor[typ.Type]{
		Instantiated: func(inst *typ.Instantiated) typ.Type {
			// Handle instantiated generic containers (e.g., Channel<T>)
			if inst.Generic == nil || len(inst.TypeArgs) == 0 {
				return containerType
			}
			// Get current element type
			oldElem := inst.TypeArgs[0]
			// If old element is unknown, replace it with value type (not union)
			var newElem typ.Type
			if typ.IsAbsentOrUnknown(oldElem) { // Unknown replaced explicitly
				newElem = valueType
			} else {
				newElem = join.Types(oldElem, valueType)
			}
			if typ.TypeEquals(oldElem, newElem) {
				return containerType
			}
			// Create new instantiation with widened element type
			newArgs := make([]typ.Type, len(inst.TypeArgs))
			copy(newArgs, inst.TypeArgs)
			newArgs[0] = newElem
			return typ.Instantiate(inst.Generic, newArgs...)
		},
		Array: func(arr *typ.Array) typ.Type {
			// Handle array types
			oldElem := arr.Element
			var newElem typ.Type
			if typ.IsAbsentOrUnknown(oldElem) { // Unknown replaced explicitly
				newElem = valueType
			} else {
				newElem = join.Types(oldElem, valueType)
			}
			if typ.TypeEquals(oldElem, newElem) {
				return containerType
			}
			return typ.NewArray(newElem)
		},
		Map: func(m *typ.Map) typ.Type {
			// Handle map types (widen value type)
			newVal := join.Types(m.Value, valueType)
			if typ.TypeEquals(m.Value, newVal) {
				return containerType
			}
			return typ.NewMap(m.Key, newVal)
		},
		Union: func(u *typ.Union) typ.Type {
			// Handle unions containing containers
			var updated []typ.Type
			changed := false
			for _, m := range u.Members {
				widened := widenContainerElementType(m, valueType)
				if widened != nil && !typ.TypeEquals(m, widened) {
					updated = append(updated, widened)
					changed = true
				} else {
					updated = append(updated, m)
				}
			}
			if changed {
				return typ.NewUnion(updated...)
			}
			return containerType
		},
		Default: func(t typ.Type) typ.Type {
			// For unknown/any, cannot widen
			return containerType
		},
	})
}

// WidenArrayElementType widens an array type by joining a new element type.
//
// This function handles the common pattern of widening arrays as elements are
// added. The join function controls how types are combined:
//
//   - For union: joinFn = join.Two (creates union types)
//   - For intersection: joinFn = narrow.Intersect (narrows types)
//
// Type handling:
//   - Array: Joins element type with new type
//   - Empty record: Converts to array with new element type
//   - Union: Widens first array member found
//   - nil/placeholder: Creates new array with element type
//   - Other types: Returns unchanged (not an array)
//
// Returns the widened type.
func WidenArrayElementType(arrayType typ.Type, elementType typ.Type, joinFn func(a, b typ.Type) typ.Type) typ.Type {
	if elementType == nil {
		return arrayType
	}

	if arrayType == nil {
		return typ.NewArray(elementType)
	}

	return typ.Visit(arrayType, typ.Visitor[typ.Type]{
		Array: func(arr *typ.Array) typ.Type {
			return typ.NewArray(joinFn(arr.Element, elementType))
		},
		Record: func(rec *typ.Record) typ.Type {
			if len(rec.Fields) == 0 {
				return typ.NewArray(elementType)
			}
			return arrayType
		},
		Union: func(u *typ.Union) typ.Type {
			var updated []typ.Type
			found := false
			for _, m := range u.Members {
				if arr, ok := m.(*typ.Array); ok && !found {
					updated = append(updated, typ.NewArray(joinFn(arr.Element, elementType)))
					found = true
				} else {
					updated = append(updated, m)
				}
			}
			if found {
				return typ.NewUnion(updated...)
			}
			return arrayType
		},
		Default: func(t typ.Type) typ.Type {
			if arrayType.Kind().IsPlaceholder() {
				return typ.NewArray(elementType)
			}
			return arrayType
		},
	})
}

// widenMapValueArray widens a map's value type by adding an element to its array component.
//
// This handles the pattern: suites[name] = suites[name] or {}; table.insert(suites[name], test)
// where suites is {[string]: Test[]} and we need to widen the array element type.
//
// Type handling:
//   - Map: Widens key type and array element of value type
//   - Empty record: Converts to map with array value
//   - Union: Widens first map member found
//   - Placeholder types: Creates new map with array value
//   - Other types: Returns unchanged
//
// Returns the widened type.
func widenMapValueArray(mapType typ.Type, keyType, elementType typ.Type) typ.Type {
	if mapType == nil {
		return typ.NewMap(keyType, typ.NewArray(elementType))
	}

	return typ.Visit(mapType, typ.Visitor[typ.Type]{
		Map: func(m *typ.Map) typ.Type {
			newKey := typ.JoinPreferNonSoft(m.Key, keyType)
			newVal := WidenArrayElementType(m.Value, elementType, typ.JoinPreferNonSoft)
			if newVal == nil {
				return mapType
			}
			if typ.TypeEquals(m.Key, newKey) && typ.TypeEquals(m.Value, newVal) {
				return mapType
			}
			return typ.NewMap(newKey, newVal)
		},
		Record: func(r *typ.Record) typ.Type {
			if len(r.Fields) == 0 {
				return typ.NewMap(keyType, typ.NewArray(elementType))
			}
			return mapType
		},
		Union: func(u *typ.Union) typ.Type {
			var updated []typ.Type
			found := false
			for _, m := range u.Members {
				if mp, ok := m.(*typ.Map); ok && !found {
					newKey := typ.JoinPreferNonSoft(mp.Key, keyType)
					newVal := WidenArrayElementType(mp.Value, elementType, typ.JoinPreferNonSoft)
					if newVal == nil {
						updated = append(updated, m)
					} else {
						updated = append(updated, typ.NewMap(newKey, newVal))
					}
					found = true
				} else {
					updated = append(updated, m)
				}
			}
			if found {
				return typ.NewUnion(updated...)
			}
			return mapType
		},
		Default: func(t typ.Type) typ.Type {
			if mapType.Kind().IsPlaceholder() {
				return typ.NewMap(keyType, typ.NewArray(elementType))
			}
			return mapType
		},
	})
}

// WidenMapValueArrayType widens a map's value type by adding an element to its array component.
//
// This exported helper mirrors the flow solver's logic and is used by
// pre-flow inference to keep map value arrays in sync with mutations.
func WidenMapValueArrayType(mapType typ.Type, keyType, elementType typ.Type) typ.Type {
	return widenMapValueArray(mapType, keyType, elementType)
}

// widenWithIndexer widens a type based on dynamic index assignment (t[k] = v).
//
// Dynamic indexing (non-constant keys) indicates the type needs a map component.
// This function transforms types to accommodate dynamic access:
//
//   - nil base: Creates map {[K]: V}
//   - Empty record {}: Converts to map {[K]: V}
//   - Record with fields: Adds or widens map component
//   - Existing map: Widens key/value types via union
//   - Placeholder types: Creates map {[K]: V}
//   - Other types: Returns unchanged
//
// Nil values are skipped: In Lua, t[k] = nil deletes the key rather than storing nil.
// Map access already returns Optional to represent potentially missing keys.
func widenWithIndexer(t typ.Type, keyType, valType typ.Type) typ.Type {
	if valType != nil && valType.Kind() == kind.Nil {
		return t
	}

	if t == nil {
		return typ.NewMap(keyType, valType)
	}

	return typ.Visit(t, typ.Visitor[typ.Type]{
		Record: func(r *typ.Record) typ.Type {
			// Empty record {} with no map component becomes a map (backward compat)
			if len(r.Fields) == 0 && !r.HasMapComponent() {
				return typ.NewMap(keyType, valType)
			}
			// Record with fields: add or widen map component
			if r.HasMapComponent() {
				newKey := typ.JoinPreferNonSoft(r.MapKey, keyType)
				newVal := typ.JoinPreferNonSoft(r.MapValue, valType)
				if typ.TypeEquals(r.MapKey, newKey) && typ.TypeEquals(r.MapValue, newVal) {
					return t
				}
				return rebuildRecordWithMapComponent(r, newKey, newVal)
			}
			// Record with fields but no map component: add map component
			return rebuildRecordWithMapComponent(r, keyType, valType)
		},
		Map: func(m *typ.Map) typ.Type {
			// Widen existing map by unioning key/value types, preferring non-soft.
			newKey := typ.JoinPreferNonSoft(m.Key, keyType)
			newVal := typ.JoinPreferNonSoft(m.Value, valType)
			if typ.TypeEquals(m.Key, newKey) && typ.TypeEquals(m.Value, newVal) {
				return t
			}
			return typ.NewMap(newKey, newVal)
		},
		Default: func(t typ.Type) typ.Type {
			// For other types (unknown, any), create a map
			if t.Kind().IsPlaceholder() {
				return typ.NewMap(keyType, valType)
			}
			return t
		},
	})
}

// rebuildRecordWithMapComponent creates a new record with an added or updated map component.
//
// Lua tables can have both named fields (record component) and dynamic keys
// (map component). This function preserves all fields, flags (optional, readonly),
// and metatable from the original record while adding or replacing the map component.
//
// The resulting type represents a table that can be accessed both by known field
// names and by dynamic keys of type mapKey.
func rebuildRecordWithMapComponent(rec *typ.Record, mapKey, mapVal typ.Type) typ.Type {
	builder := typ.NewRecord()
	if rec.Open {
		builder.SetOpen(true)
	}
	for _, f := range rec.Fields {
		switch {
		case f.Optional && f.Readonly:
			builder.OptReadonlyField(f.Name, f.Type)
		case f.Optional:
			builder.OptField(f.Name, f.Type)
		case f.Readonly:
			builder.ReadonlyField(f.Name, f.Type)
		default:
			builder.Field(f.Name, f.Type)
		}
	}
	if rec.Metatable != nil {
		builder.Metatable(rec.Metatable)
	}
	builder.MapComponent(mapKey, mapVal)
	return builder.Build()
}

// processJoinReturnChangedKeys processes phi nodes at point p.
//
// Phi nodes occur at control flow join points where a variable may have different
// values from different predecessors. This method:
//
//  1. Collects types from each phi operand (applying edge conditions)
//  2. Joins all incoming types into a single result type
//  3. Stores the joined type under the phi target's canonical key
//  4. Processes field suffixes to propagate nested field narrowings through phis
//
// Field suffix processing enables patterns like:
//
//	if x.ok then x = {ok: true, v: 1} else x = {ok: false, e: ""} end
//	-- phi for x.ok merges true and false paths
//
// Returns keys that changed, driving worklist propagation.
func (s *Solution) processJoinReturnChangedKeys(p cfg.Point) []string {
	if s.inputs.Graph == nil {
		return nil
	}

	var changedKeys []string

	for _, phi := range s.inputs.Graph.PhiNodes() {
		if phi.Point != p {
			continue
		}

		// Construct path for this phi symbol
		path := constraint.Path{
			Root:   phi.Target.Root,
			Symbol: phi.Target.Symbol,
		}

		// Collect types from operands, applying edge conditions
		types := s.scratchTypes[:0]
		for _, op := range phi.Operands {
			opKey := s.pkResolver.KeyAtVersion(op.Version.Symbol, op.Version.ID, nil)
			if opKey == "" {
				continue
			}
			opType := s.values[string(opKey)]
			if opType == nil {
				continue
			}

			// Apply edge condition from predecessor to phi point
			edgeK := edgeKey{from: op.From, to: p}
			if cond, ok := s.edgeConditions[edgeK]; ok && cond.HasConstraints() {
				if narrowed := s.applyCondition(op.From, opType, path, cond); narrowed != nil {
					opType = narrowed
				}
			}
			types = append(types, opType)
		}

		if len(types) == 0 {
			continue
		}

		// Join and store under phi target canonical key
		joined := join.Types(types...)
		targetKey := s.pkResolver.KeyAtVersion(phi.Target.Symbol, phi.Target.ID, nil)
		if targetKey == "" {
			continue
		}
		old := s.values[string(targetKey)]
		if !typ.TypeEquals(old, joined) {
			s.values[string(targetKey)] = joined
			changedKeys = append(changedKeys, string(targetKey))
		}

		// Also process field suffixes from phi operands
		suffixMap := s.collectPhiOperandSuffixes(phi)
		suffixes := make([]string, 0, len(suffixMap))
		for suffix := range suffixMap {
			suffixes = append(suffixes, suffix)
		}
		sort.Strings(suffixes)
		for _, suffix := range suffixes {
			types = types[:0]
			for _, op := range phi.Operands {
				opBaseKey := s.pkResolver.KeyAtVersion(op.Version.Symbol, op.Version.ID, nil)
				if opBaseKey == "" {
					continue
				}
				opKey := string(opBaseKey) + suffix
				if opType := s.values[opKey]; opType != nil {
					types = append(types, opType)
				}
			}
			if len(types) == 0 {
				continue
			}
			joined = join.Types(types...)
			fullKey := string(targetKey) + suffix
			old = s.values[fullKey]
			if !typ.TypeEquals(old, joined) {
				s.values[fullKey] = joined
				changedKeys = append(changedKeys, fullKey)
			}
		}
	}

	return changedKeys
}

// collectPhiOperandSuffixes collects field suffixes from phi operand canonical keys.
//
// When a phi merges versions of a variable, field assignments made on different
// branches also need to be joined. This method scans the values map to find all
// field/index suffixes that have types for any phi operand.
//
// For example, if phi merges x@1 and x@2, and values contains:
//   - x@1.foo -> string
//   - x@2.bar -> number
//
// This returns {".foo", ".bar"} so both field types can be joined at the phi.
//
// Uses scratch buffer s.scratchSuffix to reduce allocations in the hot path.
func (s *Solution) collectPhiOperandSuffixes(phi cfg.PhiNode) map[string]struct{} {
	out := s.scratchSuffix
	clear(out)

	for _, op := range phi.Operands {
		baseKey := s.pkResolver.KeyAtVersion(op.Version.Symbol, op.Version.ID, nil)
		if baseKey == "" {
			continue
		}
		baseKeyStr := string(baseKey)
		baseLen := len(baseKeyStr)
		for key := range s.values {
			if len(key) <= baseLen {
				continue
			}
			// Check prefix match
			if key[:baseLen] != baseKeyStr {
				continue
			}
			// Extract suffix (must start with . or [)
			suffix := key[baseLen:]
			if len(suffix) > 0 && (suffix[0] == '.' || suffix[0] == '[') {
				out[suffix] = struct{}{}
			}
		}
	}
	return out
}
