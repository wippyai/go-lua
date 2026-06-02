// Package mutator extracts table mutation operations from call sites.
//
// This package identifies calls that mutate table-like values and produces flow
// inputs that track element type and length constraints. It enables the legacy
// flow solver to infer table element types from usage patterns.
//
// # Table Mutators
//
// A table mutator is a function or syntax form that adds elements to a table:
//
//	table.insert(arr, value)  -- adds value to arr
//	map[key] = value          -- assigns value at key
//
// Spec-level container mutations such as channel:send(msg) are handled by the
// canonical product transfer through ContainerElementUnion effects, not by a
// legacy flow-input replay lane.
//
// # Flow Integration
//
// Extracted table.insert-like mutations become [flow.TableMutatorAssignment]
// records:
//
//   - Target: The container being mutated (path + symbol)
//   - Value: The element being added (expression + synthesized type)
//   - Point: The CFG location of the mutation
//
// These feed into the flow solver to constrain container element types.
//
// # Table Mutation
//
// Table-specific mutations (field assignments, index assignments) are also
// tracked here to support incremental table type construction where fields
// are assigned after initial table creation.
package mutator
