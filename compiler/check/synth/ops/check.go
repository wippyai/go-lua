package ops

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// CheckResult contains the result of bidirectional type checking.
//
// Type is the checked type (may be the expected type if compatible, or
// synthesized type if checking fails). Errors contains field mismatches,
// missing required fields, and type incompatibilities.
type CheckResult struct {
	Type   typ.Type     // The checked/inferred type
	Errors []CheckError // Type errors detected
}

// CheckError describes a type error during checking.
type CheckError struct {
	Message  string
	Expected typ.Type
	Got      typ.Type
	Field    string // For field-specific errors
}

// CheckTable performs bidirectional type checking for table constructors.
//
// Given a table constructor with named fields and array elements, checks
// compatibility with an expected type and infers the result type.
//
// Behavior varies by expected type:
//   - nil: Pure synthesis mode, builds type from fields/elements
//   - Record: Checks field names/types match, reports missing/extra fields
//   - Array: Checks all elements are subtypes of element type
//   - Map: Checks keys and values against map types
//   - Tuple: Checks element count and position types
//   - Union: Finds best matching member, checks against it
//   - Intersection: Checks against all members
//   - Optional: Unwraps and checks inner type
//
// Returns the checked type and any errors found.
func CheckTable(fields []FieldDef, arrayElems []typ.Type, expected typ.Type) CheckResult {
	return CheckTableEntries(fieldDefEntries(fields), arrayElems, expected)
}

// CheckTableEntries performs bidirectional table-constructor checking over the
// canonical structural entry carrier.
func CheckTableEntries(entries []EntryDef, arrayElems []typ.Type, expected typ.Type) CheckResult {
	if expected == nil {
		// Pure synthesis mode
		return CheckResult{Type: tableConstructorEntries(entries, arrayElems)}
	}

	if rec := unwrap.Record(expected); rec != nil {
		return checkTableEntriesAsRecord(entries, arrayElems, rec)
	}

	if alias, ok := expected.(*typ.Alias); ok {
		return CheckTableEntries(entries, arrayElems, alias.Target)
	}

	if opt, ok := expected.(*typ.Optional); ok {
		inner := opt.Inner
		if inner == nil {
			inner = typ.Unknown
		}

		result := CheckTableEntries(entries, arrayElems, inner)
		if result.Type == nil {
			result.Type = inner
		}

		return result
	}

	if inst, ok := expected.(*typ.Instantiated); ok {
		if resolved, err := querycore.ResolveInstantiated(inst); err == nil {
			return CheckTableEntries(entries, arrayElems, resolved)
		}
	}

	// Handle any/unknown
	if expected.Kind().IsPlaceholder() {
		return CheckResult{Type: tableConstructorEntries(entries, arrayElems)}
	}

	if inter, ok := expected.(*typ.Intersection); ok {
		var errors []CheckError

		for _, member := range inter.Members {
			result := CheckTableEntries(entries, arrayElems, member)
			if rec := unwrap.Record(member); rec != nil {
				result = checkTableEntriesAsRecordAllowExtra(entries, arrayElems, rec)
			}

			if len(result.Errors) > 0 {
				errors = append(errors, result.Errors...)
			}
		}

		if len(errors) == 0 {
			return CheckResult{Type: expected}
		}

		return CheckResult{Type: expected, Errors: errors}
	}

	unwrapped := typ.UnwrapAnnotated(expected)

	switch unwrapped.Kind() {
	case kind.Array:
		return checkTableEntriesAsArray(entries, arrayElems, unwrapped.(*typ.Array))
	case kind.Map:
		return checkTableEntriesAsMap(entries, arrayElems, unwrapped.(*typ.Map))
	case kind.ReadonlyMap:
		return checkTableEntriesAsReadonlyMap(entries, arrayElems, unwrapped.(*typ.ReadonlyMap))
	case kind.Record:
		return checkTableEntriesAsRecord(entries, arrayElems, unwrapped.(*typ.Record))
	case kind.Tuple:
		return checkTableAsTuple(arrayElems, unwrapped.(*typ.Tuple))
	case kind.Union:
		return checkTableEntriesAsUnion(entries, arrayElems, unwrapped.(*typ.Union))
	default:
		// Try synthesis and check compatibility
		synthesized := tableConstructorEntries(entries, arrayElems)
		if subtype.Consistent(synthesized, expected) {
			return CheckResult{Type: synthesized}
		}

		return CheckResult{
			Type: synthesized,
			Errors: []CheckError{{
				Message:  "table not compatible with expected type",
				Expected: expected,
				Got:      synthesized,
			}},
		}
	}
}

func checkTableAsRecordAllowExtra(fields []FieldDef, elems []typ.Type, expected *typ.Record) CheckResult {
	return checkTableEntriesAsRecordAllowExtra(fieldDefEntries(fields), elems, expected)
}

func checkTableEntriesAsRecordAllowExtra(entries []EntryDef, elems []typ.Type, expected *typ.Record) CheckResult {
	result := checkTableEntriesAsRecord(entries, elems, expected)
	if len(result.Errors) == 0 {
		return result
	}

	filtered := result.Errors[:0]

	for _, err := range result.Errors {
		if err.Message == "unexpected field" {
			continue
		}

		filtered = append(filtered, err)
	}

	result.Errors = filtered

	return result
}

// checkTableAsArray checks table against array type.
func checkTableAsArray(fields []FieldDef, elems []typ.Type, expected *typ.Array) CheckResult {
	return checkTableEntriesAsArray(fieldDefEntries(fields), elems, expected)
}

func checkTableEntriesAsArray(entries []EntryDef, elems []typ.Type, expected *typ.Array) CheckResult {
	var errors []CheckError

	for _, entry := range entries {
		slot := ExpectedTableEntryType(expected, entry.Key)
		if slot == nil {
			errors = append(errors, CheckError{
				Message: "keyed entry not allowed in array context",
				Got:     entry.Type,
				Field:   entryLabel(entry.Key),
			})
			continue
		}
		if !subtype.Consistent(entry.Type, slot) {
			errors = append(errors, CheckError{
				Message:  "entry type mismatch",
				Expected: slot,
				Got:      entry.Type,
				Field:    entryLabel(entry.Key),
			})
		}
	}

	// Check each element against expected element type
	for i, elem := range elems {
		if !subtype.Consistent(elem, expected.Element) {
			errors = append(errors, CheckError{
				Message:  "element type mismatch",
				Expected: expected.Element,
				Got:      elem,
				Field:    string(rune('0' + i + 1)), // "1", "2", etc.
			})
		}
	}

	return CheckResult{Type: expected, Errors: errors}
}

// checkTableAsMap checks table against map type.
func checkTableAsMap(fields []FieldDef, elems []typ.Type, expected *typ.Map) CheckResult {
	return checkTableEntriesAsMap(fieldDefEntries(fields), elems, expected)
}

func checkTableEntriesAsMap(entries []EntryDef, elems []typ.Type, expected *typ.Map) CheckResult {
	return checkTableEntriesAsKeyedView(entries, elems, expected, "map")
}

// checkTableAsReadonlyMap checks table against a read-only map view type. A
// fresh literal can satisfy the read-view contract, but this path does not
// expose write-side slots through the ReadonlyMap type itself.
func checkTableAsReadonlyMap(fields []FieldDef, elems []typ.Type, expected *typ.ReadonlyMap) CheckResult {
	return checkTableEntriesAsReadonlyMap(fieldDefEntries(fields), elems, expected)
}

func checkTableEntriesAsReadonlyMap(entries []EntryDef, elems []typ.Type, expected *typ.ReadonlyMap) CheckResult {
	return checkTableEntriesAsKeyedView(entries, elems, expected, "readonly map")
}

func checkTableAsKeyedView(fields []FieldDef, elems []typ.Type, expected typ.Type, context string) CheckResult {
	return checkTableEntriesAsKeyedView(fieldDefEntries(fields), elems, expected, context)
}

func checkTableEntriesAsKeyedView(entries []EntryDef, elems []typ.Type, expected typ.Type, context string) CheckResult {
	var errors []CheckError

	for _, entry := range entries {
		slot := ExpectedTableEntryType(expected, entry.Key)
		if slot == nil {
			errors = append(errors, CheckError{
				Message: "keyed entry not allowed in " + context + " context",
				Got:     entry.Type,
				Field:   entryLabel(entry.Key),
			})
			continue
		}
		if !subtype.Consistent(entry.Type, slot) {
			errors = append(errors, CheckError{
				Message:  "field value type mismatch",
				Expected: slot,
				Got:      entry.Type,
				Field:    entryLabel(entry.Key),
			})
		}
	}

	for i, elem := range elems {
		slot := ExpectedTableElementType(expected, i)
		if slot == nil {
			errors = append(errors, CheckError{
				Message: "array element not allowed in " + context + " context",
				Got:     elem,
				Field:   string(rune('0' + i + 1)),
			})
			continue
		}
		if !subtype.Consistent(elem, slot) {
			errors = append(errors, CheckError{
				Message:  "element type mismatch",
				Expected: slot,
				Got:      elem,
				Field:    string(rune('0' + i + 1)),
			})
		}
	}

	return CheckResult{Type: expected, Errors: errors}
}

// checkTableAsUnion checks table against union type by finding the best-matching member.
func checkTableAsUnion(fields []FieldDef, arrayElems []typ.Type, expected *typ.Union) CheckResult {
	return checkTableEntriesAsUnion(fieldDefEntries(fields), arrayElems, expected)
}

func checkTableEntriesAsUnion(entries []EntryDef, arrayElems []typ.Type, expected *typ.Union) CheckResult {
	if len(expected.Members) == 0 {
		return CheckResult{
			Type:   tableConstructorEntries(entries, arrayElems),
			Errors: []CheckError{{Message: "empty union type"}},
		}
	}

	// Find the union member with the best field match
	bestMember := findBestUnionMemberEntries(entries, expected.Members)
	if bestMember != nil {
		result := CheckTableEntries(entries, arrayElems, bestMember)
		if len(result.Errors) == 0 {
			return result
		}
	}

	// Try each member and return first success
	for _, member := range expected.Members {
		result := CheckTableEntries(entries, arrayElems, member)
		if len(result.Errors) == 0 {
			return result
		}
	}

	// No member matched - synthesize and report error
	synthesized := tableConstructorEntries(entries, arrayElems)

	return CheckResult{
		Type: synthesized,
		Errors: []CheckError{{
			Message:  "table not compatible with any union member",
			Expected: expected,
			Got:      synthesized,
		}},
	}
}

// findBestUnionMember finds the union member whose record fields best match the provided fields.
func findBestUnionMember(fields []FieldDef, members []typ.Type) typ.Type {
	return findBestUnionMemberEntries(fieldDefEntries(fields), members)
}

func findBestUnionMemberEntries(entries []EntryDef, members []typ.Type) typ.Type {
	if len(entries) == 0 {
		return nil
	}

	provided := make(map[constraint.Segment]bool)
	for _, entry := range entries {
		provided[entry.Key] = true
	}

	var bestMember typ.Type

	bestScore := -1

	for _, member := range members {
		rec := unwrap.Record(member)
		if rec == nil {
			continue
		}

		score := 0

		for _, rf := range rec.Fields {
			if provided[constraint.Segment{Kind: constraint.SegmentField, Name: rf.Name}] {
				score++
			}
		}
		for _, member := range rec.StaticMembers {
			seg, ok := staticMemberSegment(member)
			if ok && provided[seg] {
				score++
			}
		}

		if score > bestScore {
			bestScore = score
			bestMember = member
		}
	}

	return bestMember
}

// checkTableAsRecord checks table against record type.
func checkTableAsRecord(fields []FieldDef, elems []typ.Type, expected *typ.Record) CheckResult {
	return checkTableEntriesAsRecord(fieldDefEntries(fields), elems, expected)
}

func checkTableEntriesAsRecord(entries []EntryDef, elems []typ.Type, expected *typ.Record) CheckResult {
	var errors []CheckError

	for i, elem := range elems {
		slot := ExpectedTableElementType(expected, i)
		if slot == nil {
			errors = append(errors, CheckError{
				Message: "array element not allowed in record context",
				Got:     elem,
				Field:   string(rune('0' + i + 1)),
			})
			continue
		}
		if !subtype.Consistent(elem, slot) {
			errors = append(errors, CheckError{
				Message:  "element type mismatch",
				Expected: slot,
				Got:      elem,
				Field:    string(rune('0' + i + 1)),
			})
		}
	}

	provided := make(map[constraint.Segment]typ.Type)
	for _, entry := range entries {
		provided[entry.Key] = entry.Type
	}

	// Check each expected field
	for _, ef := range expected.Fields {
		_, ok := provided[constraint.Segment{Kind: constraint.SegmentField, Name: ef.Name}]
		if !ok {
			if !ef.Optional && !unwrap.IsOptionalLike(ef.Type) {
				errors = append(errors, CheckError{
					Message:  "missing required field",
					Expected: ef.Type,
					Field:    ef.Name,
				})
			}

			continue
		}
	}

	for _, member := range expected.StaticMembers {
		_, ok := staticMemberProvided(member, provided)
		if !ok && !member.Optional {
			errors = append(errors, CheckError{
				Message:  "missing required static member",
				Expected: member.Type,
				Field:    staticMemberLabel(member),
			})
		}
	}

	for _, entry := range entries {
		expectedFieldType := ExpectedTableEntryType(expected, entry.Key)
		if expectedFieldType == nil {
			errors = append(errors, CheckError{
				Message: "unexpected field",
				Got:     entry.Type,
				Field:   entryLabel(entry.Key),
			})
			continue
		}
		if !subtype.Consistent(entry.Type, expectedFieldType) {
			errors = append(errors, CheckError{
				Message:  "field type mismatch",
				Expected: expectedFieldType,
				Got:      entry.Type,
				Field:    entryLabel(entry.Key),
			})
		}
	}

	return CheckResult{Type: expected, Errors: errors}
}

func staticMemberProvided(member typ.StaticMember, provided map[constraint.Segment]typ.Type) (typ.Type, bool) {
	seg, ok := staticMemberSegment(member)
	if !ok {
		return nil, false
	}
	t, ok := provided[seg]
	return t, ok
}

func staticMemberSegment(member typ.StaticMember) (constraint.Segment, bool) {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return constraint.Segment{Kind: constraint.SegmentIndexString, Name: member.Name}, true
	case typ.StaticMemberIntIndex:
		return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: int(member.Index)}, true
	default:
		return constraint.Segment{}, false
	}
}

func entryLabel(key constraint.Segment) string {
	switch key.Kind {
	case constraint.SegmentField:
		return key.Name
	case constraint.SegmentIndexString:
		return "[" + typ.LiteralString(key.Name).String() + "]"
	case constraint.SegmentIndexInt:
		return typ.LiteralInt(int64(key.Index)).String()
	default:
		return ""
	}
}

func staticMemberLabel(member typ.StaticMember) string {
	switch member.Kind {
	case typ.StaticMemberStringIndex:
		return "[" + typ.LiteralString(member.Name).String() + "]"
	case typ.StaticMemberIntIndex:
		return typ.LiteralInt(member.Index).String()
	default:
		return ""
	}
}

// checkTableAsTuple checks table against tuple type.
func checkTableAsTuple(elems []typ.Type, expected *typ.Tuple) CheckResult {
	var errors []CheckError

	// Check element count
	if len(elems) < len(expected.Elements) {
		errors = append(errors, CheckError{
			Message: "not enough elements for tuple",
		})
	} else if len(elems) > len(expected.Elements) {
		errors = append(errors, CheckError{
			Message: "too many elements for tuple",
		})
	}

	// Check each element
	for i := 0; i < len(elems) && i < len(expected.Elements); i++ {
		if !subtype.Consistent(elems[i], expected.Elements[i]) {
			errors = append(errors, CheckError{
				Message:  "element type mismatch",
				Expected: expected.Elements[i],
				Got:      elems[i],
				Field:    string(rune('0' + i + 1)),
			})
		}
	}

	return CheckResult{Type: expected, Errors: errors}
}
