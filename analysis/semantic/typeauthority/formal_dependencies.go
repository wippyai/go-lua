package typeauthority

import (
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/static"
)

// formalDependencies derives the exact same-owner formal dependencies of one
// static term. It follows Program's sealed static graph only; no source names,
// AST nodes, or ambient type registry participate. The visited set makes the
// traversal finite even when declaration references are recursive.
func formalDependencies(p *program.Program, root, owner keyspace.Term) (map[keyspace.Term]struct{}, bool) {
	if p == nil || root == 0 || owner == 0 {
		return nil, false
	}
	view := p.Static()
	declarations := view.Declarations()
	types := view.Types()
	references := view.References()
	signatures := view.Signatures().TypeFunctions()
	operators := view.Operators()
	dependencies := make(map[keyspace.Term]struct{})
	seen := make(map[keyspace.Term]struct{})
	work := []keyspace.Term{root}
	for len(work) != 0 {
		last := len(work) - 1
		term := work[last]
		work = work[:last]
		if term == 0 {
			continue
		}
		if _, duplicate := seen[term]; duplicate {
			continue
		}
		seen[term] = struct{}{}
		if parameterOwner, _, constraint, ok := declarations.TypeParams().Get(term); ok {
			if parameterOwner == owner {
				dependencies[term] = struct{}{}
			}
			if constraint != 0 {
				work = append(work, constraint)
			}
			continue
		}
		if _, target, _, _, ok := declarations.Aliases().Get(term); ok {
			count, counted := declarations.Aliases().ParamCount(term)
			if !counted {
				return nil, false
			}
			work = append(work, target)
			for index := 0; index < count; index++ {
				parameter, ok := declarations.Aliases().ParamAt(term, index)
				if !ok {
					return nil, false
				}
				work = append(work, parameter)
			}
			continue
		}
		if _, _, _, ok := declarations.Interfaces().Get(term); ok {
			count, counted := declarations.Interfaces().ExtendCount(term)
			if !counted {
				return nil, false
			}
			for index := 0; index < count; index++ {
				extend, ok := declarations.Interfaces().ExtendAt(term, index)
				if !ok {
					return nil, false
				}
				work = append(work, extend)
			}
			members, counted := declarations.Interfaces().MemberCount(term)
			if !counted {
				return nil, false
			}
			for index := 0; index < members; index++ {
				member, ok := declarations.Interfaces().MemberAt(term, index)
				if !ok {
					return nil, false
				}
				if member.Kind == static.InterfaceField {
					_, fieldType, _, ok := types.Fields().Get(member.Field)
					if !ok {
						return nil, false
					}
					work = append(work, fieldType)
				} else if member.Kind == static.InterfaceMethod {
					work = append(work, member.Signature)
				} else {
					return nil, false
				}
			}
			continue
		}
		if inner, ok := types.Optionals().Get(term); ok {
			work = append(work, inner)
			continue
		}
		if count, ok := types.Unions().MemberCount(term); ok {
			for index := 0; index < count; index++ {
				member, ok := types.Unions().MemberAt(term, index)
				if !ok {
					return nil, false
				}
				work = append(work, member)
			}
			continue
		}
		if count, ok := types.Intersections().MemberCount(term); ok {
			for index := 0; index < count; index++ {
				member, ok := types.Intersections().MemberAt(term, index)
				if !ok {
					return nil, false
				}
				work = append(work, member)
			}
			continue
		}
		if _, target, _, ok := references.Get(term); ok {
			if target != 0 {
				work = append(work, target)
			}
			continue
		}
		if base, count, ok := types.Generics().Get(term); ok {
			work = append(work, base)
			for index := 0; index < count; index++ {
				arg, ok := types.Generics().ArgAt(term, index)
				if !ok {
					return nil, false
				}
				work = append(work, arg)
			}
			continue
		}
		if element, _, ok := types.Arrays().Get(term); ok {
			work = append(work, element)
			continue
		}
		if key, value, _, ok := types.Maps().Get(term); ok {
			work = append(work, key, value)
			continue
		}
		if _, count, ok := types.Records().Get(term); ok {
			for index := 0; index < count; index++ {
				field, ok := types.Records().FieldAt(term, index)
				if !ok {
					return nil, false
				}
				_, fieldType, _, ok := types.Fields().Get(field)
				if !ok {
					return nil, false
				}
				work = append(work, fieldType)
			}
			continue
		}
		if _, variadic, _, _, ok := signatures.Get(term); ok {
			if variadic != 0 {
				work = append(work, variadic)
			}
			count, counted := signatures.TypeParamCount(term)
			if !counted {
				return nil, false
			}
			for index := 0; index < count; index++ {
				parameter, ok := signatures.TypeParamAt(term, index)
				if !ok {
					return nil, false
				}
				work = append(work, parameter)
			}
			params, counted := signatures.ParameterCount(term)
			if !counted {
				return nil, false
			}
			for index := 0; index < params; index++ {
				parameter, ok := signatures.ParameterAt(term, index)
				if !ok {
					return nil, false
				}
				work = append(work, parameter.Type)
			}
			returns, counted := signatures.ReturnCount(term)
			if !counted {
				return nil, false
			}
			for index := 0; index < returns; index++ {
				result, ok := signatures.ReturnAt(term, index)
				if !ok {
					return nil, false
				}
				work = append(work, result)
			}
			continue
		}
		if _, _, _, _, narrow, ok := view.Signatures().Assertions().Get(term); ok {
			if narrow != 0 {
				work = append(work, narrow)
			}
			continue
		}
		if inner, ok := operators.KeyOfs().Get(term); ok {
			work = append(work, inner)
			continue
		}
		if object, index, ok := operators.IndexAccesses().Get(term); ok {
			work = append(work, object, index)
			continue
		}
		if check, extend, thenTerm, elseTerm, ok := operators.Conditionals().Get(term); ok {
			work = append(work, check, extend, thenTerm, elseTerm)
			continue
		}
		// Primitive/literal/typeof leaves contain no static type child.
		if _, ok := view.StaticTypes().Ref(term); !ok {
			return nil, false
		}
	}
	return dependencies, true
}
