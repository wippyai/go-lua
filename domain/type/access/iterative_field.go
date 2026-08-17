package access

import (
	graph "github.com/wippyai/go-lua/domain/type/internal/typegraph"
	"github.com/wippyai/go-lua/domain/type/normalize"
	"github.com/wippyai/go-lua/domain/type/subst"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

// resolveField and resolveMissing are the public projection engine.  A Field
// query has conjunction/disjunction result algebra (unlike a boolean subtype
// proof), so it keeps its own result-carrying frames rather than borrowing a
// recursion guard as a semantic depth limit.
func (q *query) resolveField(root typ.Type, name string) fieldResult {
	return q.runFieldFrames(fieldFrame{kind: fieldFrameField, t: root, name: name})
}

func (q *query) resolveMissing(root typ.Type) bool {
	result := q.runFieldFrames(fieldFrame{kind: fieldFrameMissing, t: root})
	return result.ok
}

type fieldFrameKind uint8

const (
	fieldFrameField fieldFrameKind = iota + 1
	fieldFrameMissing
)

type fieldFrame struct {
	kind fieldFrameKind
	t    typ.Type
	name string

	// cycle is the exact identity result selected by the parent operation.
	// Field-union is a must query (true identity); field-intersection is a may
	// query (false identity). Missing uses the same all/any distinction.
	cycle        fieldResult
	missingCycle bool

	phase    uint8
	entered  bool
	optional int
	members  []typ.Type
	next     int
	values   []typ.Type
	nilable  bool
	result   fieldResult
}

func (q *query) runFieldFrames(root fieldFrame) fieldResult {
	stack := []fieldFrame{{phase: 9}, root}
	for len(stack) > 1 {
		top := &stack[len(stack)-1]
		if top.phase == 0 {
			if top.kind == fieldFrameField {
				if top.t == nil {
					stack = q.finishFieldFrame(stack, fieldResult{})
					continue
				}
				key := queryKey{op: 1, t: top.t, name: top.name}
				if !q.enter(key) {
					stack = q.finishFieldFrame(stack, top.cycle)
					continue
				}
				top.entered = true
				if special, ok := SpecialAccessType(top.t); ok {
					stack = q.finishFieldFrame(stack, fieldResult{t: special, ok: true})
					continue
				}
				base, optional, ok := accessProjectionBase(top.t)
				if !ok {
					stack = q.finishFieldFrame(stack, fieldResult{})
					continue
				}
				top.optional = optional
				if special, ok := SpecialAccessType(base); ok {
					stack = q.finishFieldFrame(stack, optionalizeField(fieldResult{t: special, ok: true}, optional))
					continue
				}
				switch value := unwrap.Annotated(base).(type) {
				case *typ.Record:
					stack = q.finishFieldFrame(stack, optionalizeField(fieldInRecord(value, top.name), optional))
				case *typ.Interface:
					stack = q.finishFieldFrame(stack, optionalizeField(fieldInInterface(value, base, top.name), optional))
				case *typ.Map:
					stack = q.finishFieldFrame(stack, optionalizeField(fieldInMap(value.Key, value.Value, top.name), optional))
				case *typ.ReadonlyMap:
					stack = q.finishFieldFrame(stack, optionalizeField(fieldInMap(value.Key, value.Value, top.name), optional))
				case *typ.Union:
					if value == nil || len(value.Members) == 0 {
						stack = q.finishFieldFrame(stack, fieldResult{})
						continue
					}
					top.members, top.phase = value.Members, 1
				case *typ.Intersection:
					if value == nil {
						stack = q.finishFieldFrame(stack, fieldResult{})
						continue
					}
					top.members, top.phase = value.Members, 4
				default:
					stack = q.finishFieldFrame(stack, fieldResult{})
				}
				continue
			}

			if top.t == nil {
				stack = q.finishFieldFrame(stack, fieldResult{})
				continue
			}
			key := queryKey{op: 2, t: top.t}
			if !q.enter(key) {
				stack = q.finishFieldFrame(stack, booleanField(top.missingCycle))
				continue
			}
			top.entered = true
			base, _, ok := accessProjectionBase(top.t)
			if !ok {
				stack = q.finishFieldFrame(stack, fieldResult{})
				continue
			}
			switch value := unwrap.Annotated(base).(type) {
			case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple, *typ.Interface:
				stack = q.finishFieldFrame(stack, booleanField(true))
			case *typ.Union:
				if value == nil || len(value.Members) == 0 {
					stack = q.finishFieldFrame(stack, fieldResult{})
					continue
				}
				top.members, top.phase = value.Members, 6
			case *typ.Intersection:
				if value == nil {
					stack = q.finishFieldFrame(stack, fieldResult{})
					continue
				}
				top.members, top.phase = value.Members, 7
			default:
				stack = q.finishFieldFrame(stack, fieldResult{})
			}
			continue
		}

		switch top.phase {
		case 1: // field union: visit member field results
			if top.next == len(top.members) {
				if len(top.values) == 0 {
					if top.nilable {
						stack = q.finishFieldFrame(stack, optionalizeField(fieldResult{t: typ.Nil, ok: true}, top.optional))
					} else {
						stack = q.finishFieldFrame(stack, fieldResult{})
					}
				} else {
					stack = q.finishFieldFrame(stack, optionalizeField(fieldResult{t: normalize.UnionForEvidence(top.values...), ok: true, nilable: top.nilable}, top.optional))
				}
				continue
			}
			member := top.members[top.next]
			top.next++
			stack = append(stack, fieldFrame{kind: fieldFrameField, t: member, name: top.name, cycle: fieldResult{ok: true}, phase: 0})
		case 2: // union member failed: decide its missing-read branch
			stack = append(stack, fieldFrame{kind: fieldFrameMissing, t: top.members[top.next-1], missingCycle: true})
		case 3: // union failure that is not a missing read
			stack = q.finishFieldFrame(stack, fieldResult{})
		case 4: // field intersection: retain successful projection alternatives
			if top.next == len(top.members) {
				if len(top.values) == 0 {
					stack = q.finishFieldFrame(stack, fieldResult{})
				} else if len(top.values) == 1 {
					stack = q.finishFieldFrame(stack, optionalizeField(fieldResult{t: top.values[0], ok: true}, top.optional))
				} else {
					stack = q.finishFieldFrame(stack, optionalizeField(fieldResult{t: normalize.IntersectionForMeet(top.values...), ok: true}, top.optional))
				}
				continue
			}
			member := top.members[top.next]
			top.next++
			stack = append(stack, fieldFrame{kind: fieldFrameField, t: member, name: top.name, phase: 0})
		case 6: // missing union: every member must allow nil
			if top.next == len(top.members) {
				stack = q.finishFieldFrame(stack, booleanField(true))
				continue
			}
			member := top.members[top.next]
			top.next++
			stack = append(stack, fieldFrame{kind: fieldFrameMissing, t: member, missingCycle: true})
		case 7: // missing intersection: one member is enough
			if top.next == len(top.members) {
				stack = q.finishFieldFrame(stack, fieldResult{})
				continue
			}
			member := top.members[top.next]
			top.next++
			stack = append(stack, fieldFrame{kind: fieldFrameMissing, t: member, missingCycle: false})
		}
	}
	return stack[0].result
}

func (q *query) finishFieldFrame(stack []fieldFrame, result fieldResult) []fieldFrame {
	last := len(stack) - 1
	finished := stack[last]
	if finished.entered {
		if finished.kind == fieldFrameField {
			q.leave(queryKey{op: 1, t: finished.t, name: finished.name})
		} else {
			q.leave(queryKey{op: 2, t: finished.t})
		}
	}
	stack = stack[:last]
	parent := &stack[len(stack)-1]
	if parent.phase == 9 {
		parent.result = result
		return stack
	}
	switch parent.phase {
	case 1:
		if !result.ok {
			parent.phase = 2
		} else {
			if result.nilable {
				parent.nilable = true
			}
			if result.t != nil {
				parent.values = append(parent.values, result.t)
			}
		}
	case 2:
		if result.ok {
			parent.nilable = true
			parent.phase = 1
		} else {
			parent.phase = 3
		}
	case 4:
		if result.ok {
			if value, ok := result.materialize(); ok {
				parent.values = append(parent.values, value)
			}
		}
	case 6:
		if !result.ok {
			parent.phase = 3
		}
	case 7:
		if result.ok {
			parent.next = len(parent.members)
		}
	}
	return stack
}

func booleanField(value bool) fieldResult { return fieldResult{ok: value} }

func optionalizeField(result fieldResult, count int) fieldResult {
	if result.ok && count != 0 {
		result.nilable = true
	}
	return result
}

// accessProjectionBase owns only wrapper descent. It never recurses through a
// constructor and its local finite path detects malformed wrapper cycles.
func accessProjectionBase(value typ.Type) (typ.Type, int, bool) {
	var path graph.Path
	optional := 0
	for value != nil {
		if !path.Enter(value, 0) {
			return nil, optional, false
		}
		switch current := unwrap.Annotated(value).(type) {
		case *typ.Optional:
			optional++
			value = current.Inner
		case *typ.Alias:
			value = current.Target
		case *typ.TypeParam:
			if current.Constraint == nil {
				return nil, optional, false
			}
			value = current.Constraint
		case *typ.Recursive:
			if current.Body == nil || current.Body == value {
				return nil, optional, false
			}
			value = current.Body
		case *typ.Instantiated:
			expanded := subst.ExpandInstantiated(current)
			if expanded == nil || expanded == value {
				return nil, optional, false
			}
			value = expanded
		default:
			return value, optional, true
		}
	}
	return nil, optional, false
}
