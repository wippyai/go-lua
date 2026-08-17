package subtype

import (
	"github.com/wippyai/go-lua/domain/type/kind"
	"github.com/wippyai/go-lua/domain/type/subst"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	"github.com/wippyai/go-lua/domain/type/unwrap"
)

// proofRelation is deliberately separate for ordinary subtyping and the
// fresh-constructor widening relation.  The latter may ask ordinary
// subtyping as an alternative, but their coinductive assumptions are not
// interchangeable.
type proofRelation uint8

const (
	proofSubtype proofRelation = iota
	proofWiden
)

type proofCall struct {
	relation proofRelation
	sub      typ.Type
	super    typ.Type
}

// proofExpr is a finite boolean proof tree. Calls are leaves and all/any are
// evaluated by proofMachine's explicit stack; constructing this tree never
// evaluates a child type relation.
type proofExpr struct {
	known   int8 // -1 false, 0 unresolved, 1 true
	call    proofCall
	calling bool
	not     *proofExpr
	all     []proofExpr
	any     []proofExpr
}

func proofTrue() proofExpr  { return proofExpr{known: 1} }
func proofFalse() proofExpr { return proofExpr{known: -1} }
func proofOf(relation proofRelation, sub, super typ.Type) proofExpr {
	return proofExpr{call: proofCall{relation: relation, sub: sub, super: super}, calling: true}
}
func subtypeOf(sub, super typ.Type) proofExpr { return proofOf(proofSubtype, sub, super) }
func widenOf(sub, super typ.Type) proofExpr   { return proofOf(proofWiden, sub, super) }
func proofAll(parts ...proofExpr) proofExpr   { return proofExpr{all: parts} }
func proofAny(parts ...proofExpr) proofExpr   { return proofExpr{any: parts} }
func proofNot(part proofExpr) proofExpr       { return proofExpr{not: &part} }

type proofFrame struct {
	expr       proofExpr
	phase      uint8
	child      int
	accum      bool
	callActive bool
	call       proofCall
	pair       typePair
	pairOK     bool
}

// prove is an iterative depth-first evaluator. The only termination measure
// is the finite reachable pair graph: an exact active pair closes the same
// coinductive obligation, and a completed pair is memoized. There is no
// depth/fuel/cap result.
func (c *checker) prove(root proofExpr) bool {
	stack := []proofFrame{{phase: 4}, {expr: root}}
	for len(stack) != 0 {
		top := &stack[len(stack)-1]
		if top.phase == 4 {
			return top.accum
		}
		if top.phase == 3 {
			stack = c.finishProofFrame(stack, !top.accum)
			continue
		}
		if top.phase == 0 {
			switch {
			case top.expr.known != 0:
				result := top.expr.known > 0
				stack = c.finishProofFrame(stack, result)
				continue
			case top.expr.calling:
				top.call = top.expr.call
				if missingTypePair(top.call.sub, top.call.super) {
					stack = c.finishProofFrame(stack, false)
					continue
				}
				if top.call.relation == proofWiden {
					// unwrap.Alias is itself iterative, but preserve one Alias node in
					// the proof graph so deeply nested products never re-enter a
					// recursive relation through a helper.
					top.call.sub = unwrap.Alias(top.call.sub)
					top.call.super = unwrap.Alias(top.call.super)
					if missingTypePair(top.call.sub, top.call.super) {
						stack = c.finishProofFrame(stack, false)
						continue
					}
				}
				if top.call.sub == top.call.super {
					stack = c.finishProofFrame(stack, true)
					continue
				}
				top.pair, top.pairOK = newTypePair(top.call.sub, top.call.super)
				if top.pairOK {
					memo, active := c.proofTables(top.call.relation)
					if result, ok := memo[top.pair]; ok {
						stack = c.finishProofFrame(stack, result)
						continue
					}
					if active[top.pair] {
						stack = c.finishProofFrame(stack, true)
						continue
					}
					active[top.pair] = true
					top.callActive = true
				}
				top.expr = c.proofRule(top.call)
				top.phase = 0
				continue
			case top.expr.not != nil:
				top.phase = 3
				stack = append(stack, proofFrame{expr: *top.expr.not})
				continue
			case top.expr.all != nil:
				top.phase = 1
				top.accum = true
				continue
			case top.expr.any != nil:
				top.phase = 2
				top.accum = false
				continue
			default:
				stack = c.finishProofFrame(stack, false)
				continue
			}
		}
		parts := top.expr.all
		want := true
		if top.phase == 2 {
			parts = top.expr.any
			want = false
		}
		if top.child == len(parts) || top.accum != want {
			stack = c.finishProofFrame(stack, top.accum)
			continue
		}
		child := parts[top.child]
		top.child++
		stack = append(stack, proofFrame{expr: child})
	}
	return false
}

func (c *checker) finishProofFrame(stack []proofFrame, result bool) []proofFrame {
	last := len(stack) - 1
	finished := stack[last]
	if finished.callActive && finished.pairOK {
		memo, active := c.proofTables(finished.call.relation)
		delete(active, finished.pair)
		memo[finished.pair] = result
	}
	stack = stack[:last]
	parent := &stack[len(stack)-1]
	if parent.phase == 4 {
		parent.accum = result
	} else if parent.phase == 3 {
		parent.accum = result
	} else if parent.phase == 1 {
		parent.accum = parent.accum && result
	} else if parent.phase == 2 {
		parent.accum = parent.accum || result
	}
	return stack
}

func (c *checker) proofTables(relation proofRelation) (map[typePair]bool, map[typePair]bool) {
	if relation == proofSubtype {
		if c.memo == nil {
			c.memo = make(map[typePair]bool)
		}
		if c.inProgress == nil {
			c.inProgress = make(map[typePair]bool)
		}
		return c.memo, c.inProgress
	}
	if c.widenMemo == nil {
		c.widenMemo = make(map[typePair]bool)
	}
	if c.widenInProgress == nil {
		c.widenInProgress = make(map[typePair]bool)
	}
	return c.widenMemo, c.widenInProgress
}

func (c *checker) proofRule(call proofCall) proofExpr {
	if call.relation == proofWiden {
		return c.widenRule(call.sub, call.super)
	}
	return c.subtypeRule(call.sub, call.super)
}

func (c *checker) subtypeRule(sub, super typ.Type) proofExpr {
	if ref, ok := sub.(*typ.Ref); ok && ref.Module == "" {
		if a, ok := super.(*typ.Alias); ok && a.Name == ref.Name {
			return proofTrue()
		}
		if r, ok := super.(*typ.Ref); ok && r.Module == "" && r.Name == ref.Name {
			return proofTrue()
		}
		if r, ok := super.(*typ.Recursive); ok && r.Name == ref.Name {
			return proofTrue()
		}
	}
	if ref, ok := super.(*typ.Ref); ok && ref.Module == "" {
		if a, ok := sub.(*typ.Alias); ok && a.Name == ref.Name {
			return proofTrue()
		}
		if r, ok := sub.(*typ.Ref); ok && r.Module == "" && r.Name == ref.Name {
			return proofTrue()
		}
		if r, ok := sub.(*typ.Recursive); ok && r.Name == ref.Name {
			return proofTrue()
		}
	}
	if a, ok := sub.(*typ.Alias); ok {
		return subtypeOf(a.Target, super)
	}
	if a, ok := super.(*typ.Alias); ok {
		return subtypeOf(sub, a.Target)
	}
	if a, ok := sub.(*typ.Annotated); ok {
		return subtypeOf(a.Inner, super)
	}
	if a, ok := super.(*typ.Annotated); ok {
		return subtypeOf(sub, a.Inner)
	}
	if rs, ok := sub.(*typ.Recursive); ok && rs.Body != nil && rs.Body != rs {
		if rp, ok := super.(*typ.Recursive); ok && rp.Body != nil && rp.Body != rp {
			return subtypeOf(rs.Body, rp.Body)
		}
	}
	if r, ok := sub.(*typ.Recursive); ok && super.Kind() != kind.Recursive && r.Body != nil && r.Body != r {
		return subtypeOf(r.Body, super)
	}
	if r, ok := super.(*typ.Recursive); ok && sub.Kind() != kind.Recursive && r.Body != nil && r.Body != r {
		return subtypeOf(sub, r.Body)
	}

	subInst, subIsInst := sub.(*typ.Instantiated)
	superInst, superIsInst := super.(*typ.Instantiated)
	if subIsInst && superIsInst && subInst.Generic != nil && superInst.Generic != nil && typ.TypeEquals(subInst.Generic, superInst.Generic) {
		return c.instantiatedRule(subInst, superInst, proofSubtype)
	}
	if subIsInst {
		expanded := subst.ExpandInstantiated(subInst)
		if expanded != nil && expanded != sub {
			return subtypeOf(expanded, subst.Self(super, subInst))
		}
	}
	if superIsInst {
		expanded := subst.ExpandInstantiated(superInst)
		if expanded != nil && expanded != super {
			return subtypeOf(subst.Self(sub, superInst), expanded)
		}
	}

	if typ.IsNever(sub) {
		return proofTrue()
	}
	if typ.IsNever(super) {
		return proofFalse()
	}
	if typ.IsAny(super) || typ.IsUnknown(super) || isOptionalTop(super) {
		return proofTrue()
	}
	if typ.IsAny(sub) {
		if typ.IsBuiltinTableTopMarker(super) {
			return proofTrue()
		}
		if i, ok := super.(*typ.Intersection); ok {
			parts := make([]proofExpr, 0, len(i.Members))
			for _, member := range i.Members {
				parts = append(parts, subtypeOf(sub, member))
			}
			return proofAll(parts...)
		}
		if u, ok := super.(*typ.Union); ok {
			parts := make([]proofExpr, 0, len(u.Members))
			for _, member := range u.Members {
				parts = append(parts, subtypeOf(sub, member))
			}
			return proofAny(parts...)
		}
		return proofFalse()
	}
	if typ.IsUnknown(sub) {
		return proofFalse()
	}
	if u, ok := sub.(*typ.Union); ok {
		parts := make([]proofExpr, 0, len(u.Members))
		for _, member := range u.Members {
			parts = append(parts, subtypeOf(member, super))
		}
		return proofAll(parts...)
	}
	if u, ok := super.(*typ.Union); ok {
		if o, ok := sub.(*typ.Optional); ok {
			return proofAll(subtypeOf(o.Inner, super), subtypeOf(typ.Nil, super))
		}
		parts := make([]proofExpr, 0, len(u.Members))
		for _, member := range u.Members {
			parts = append(parts, subtypeOf(sub, member))
		}
		return proofAny(parts...)
	}
	if i, ok := sub.(*typ.Intersection); ok {
		parts := make([]proofExpr, 0, len(i.Members))
		for _, member := range i.Members {
			parts = append(parts, subtypeOf(member, super))
		}
		return proofAny(parts...)
	}
	if i, ok := super.(*typ.Intersection); ok {
		parts := make([]proofExpr, 0, len(i.Members))
		for _, member := range i.Members {
			parts = append(parts, subtypeOf(sub, member))
		}
		return proofAll(parts...)
	}
	if o, ok := super.(*typ.Optional); ok {
		if subOpt, ok := sub.(*typ.Optional); ok {
			return subtypeOf(subOpt.Inner, o.Inner)
		}
		if sub.Kind() == kind.Nil {
			return proofTrue()
		}
		return subtypeOf(sub, o.Inner)
	}
	if o, ok := sub.(*typ.Optional); ok {
		return proofAll(subtypeOf(typ.Nil, super), subtypeOf(o.Inner, super))
	}
	if ok, handled := checkTableTop(sub, super); handled {
		if ok {
			return proofTrue()
		}
		return proofFalse()
	}
	if r, ok := sub.(*typ.Record); ok && emptyRecordAdoptsContainerShape(r, super) {
		return proofTrue()
	}
	if r, ok := sub.(*typ.Record); ok {
		if m, ok := super.(*typ.Map); ok {
			return c.recordToMapRule(r, m)
		}
		if m, ok := super.(*typ.ReadonlyMap); ok {
			return c.recordToReadonlyMapRule(r, m)
		}
	}
	if m, ok := sub.(*typ.Map); ok {
		if r, ok := super.(*typ.Record); ok {
			return c.mapToRecordRule(m, r)
		}
		if view, ok := super.(*typ.ReadonlyMap); ok {
			return c.readonlyMapRule(typetable.NewReadonlyMap(m.Key, m.Value), view)
		}
	}
	if arr, ok := sub.(*typ.Array); ok {
		if m, ok := super.(*typ.Map); ok {
			return proofAll(subtypeOf(typ.Integer, m.Key), subtypeOf(arr.Element, m.Value))
		}
		if view, ok := super.(*typ.ReadonlyMap); ok {
			return c.readonlyMapRule(typetable.NewReadonlyMap(typ.Integer, arr.Element), view)
		}
	}
	if tup, ok := sub.(*typ.Tuple); ok {
		if arr, ok := super.(*typ.Array); ok {
			parts := make([]proofExpr, 0, len(tup.Elements))
			for _, element := range tup.Elements {
				parts = append(parts, subtypeOf(element, arr.Element))
			}
			return proofAll(parts...)
		}
		if m, ok := super.(*typ.Map); ok {
			parts := []proofExpr{subtypeOf(typ.Integer, m.Key)}
			for _, element := range tup.Elements {
				parts = append(parts, subtypeOf(element, m.Value))
			}
			return proofAll(parts...)
		}
		if view, ok := super.(*typ.ReadonlyMap); ok {
			parts := make([]proofExpr, 0, len(tup.Elements)*2)
			for index, element := range tup.Elements {
				parts = append(parts, subtypeOf(typ.LiteralInt(int64(index+1)), view.Key), subtypeOf(typetable.PresentReadonlyEntryValue(element), view.Value))
			}
			return proofAll(parts...)
		}
	}
	if rec, ok := sub.(*typ.Record); ok {
		if iface, ok := super.(*typ.Interface); ok {
			return c.recordToInterfaceRule(rec, iface)
		}
	}
	if tp, ok := sub.(*typ.TypeParam); ok {
		if sp, ok := super.(*typ.TypeParam); ok {
			return booleanExpr(typ.TypeEquals(tp, sp))
		}
		if tp.Constraint != nil {
			return subtypeOf(tp.Constraint, super)
		}
		return booleanExpr(typ.IsAny(super))
	}
	if tp, ok := super.(*typ.TypeParam); ok {
		if tp.Constraint != nil {
			return subtypeOf(sub, tp.Constraint)
		}
		return proofTrue()
	}
	if lit, ok := sub.(*typ.Literal); ok {
		// Literal constructors are value-semantic, not pointer-semantic.  The
		// canonical decoder and a caller-created expected type may hold equal
		// singleton values in distinct allocations; treat that exact literal
		// identity as the mutual-subtype case before widening to its primitive
		// base.  Without this branch an exact `keyof` result such as "foo"
		// compares equal by typ.TypeEquals but fails both subtype directions.
		if other, ok := super.(*typ.Literal); ok {
			return booleanExpr(lit.Equals(other))
		}
		switch lit.Base() {
		case kind.Boolean:
			return booleanExpr(super.Kind() == kind.Boolean)
		case kind.Integer:
			return booleanExpr(super.Kind() == kind.Integer || super.Kind() == kind.Number)
		case kind.Number:
			return booleanExpr(super.Kind() == kind.Number)
		case kind.String:
			return booleanExpr(super.Kind() == kind.String)
		}
	}
	if sub.Kind() == kind.Integer && super.Kind() == kind.Number {
		return proofTrue()
	}
	if sub.Kind() != super.Kind() {
		return proofFalse()
	}
	switch sub.Kind() {
	case kind.Function:
		return c.functionRule(sub.(*typ.Function), super.(*typ.Function))
	case kind.Record:
		return c.recordRule(sub.(*typ.Record), super.(*typ.Record))
	case kind.Array:
		return subtypeOf(sub.(*typ.Array).Element, super.(*typ.Array).Element)
	case kind.Map:
		return c.mapRule(sub.(*typ.Map), super.(*typ.Map))
	case kind.ReadonlyMap:
		return c.readonlyMapRule(sub.(*typ.ReadonlyMap), super.(*typ.ReadonlyMap))
	case kind.Tuple:
		return c.tupleRule(sub.(*typ.Tuple), super.(*typ.Tuple))
	case kind.Interface:
		return c.interfaceRule(sub.(*typ.Interface), super.(*typ.Interface))
	case kind.Instantiated:
		return c.instantiatedRule(sub.(*typ.Instantiated), super.(*typ.Instantiated), proofSubtype)
	case kind.Meta:
		return subtypeOf(sub.(*typ.Meta).Of, super.(*typ.Meta).Of)
	default:
		return booleanExpr(typ.TypeEquals(sub, super))
	}
}

func booleanExpr(value bool) proofExpr {
	if value {
		return proofTrue()
	}
	return proofFalse()
}

func (c *checker) functionRule(sub, super *typ.Function) proofExpr {
	if sub == nil || super == nil {
		return proofFalse()
	}
	subReq, superReq := minRequiredArgs(sub), minRequiredArgs(super)
	if subReq > superReq || (super.Variadic == nil && subReq > len(super.Params)) || (sub.Variadic == nil && len(super.Params) > len(sub.Params)) {
		return proofFalse()
	}
	maxParams := len(sub.Params)
	if len(super.Params) > maxParams {
		maxParams = len(super.Params)
	}
	parts := make([]proofExpr, 0, maxParams+len(super.Returns)+1)
	for index := 0; index < maxParams; index++ {
		var subType, superType typ.Type
		var subReceiver, superReceiver bool
		if index < len(sub.Params) {
			param := sub.Params[index]
			subType = param.Type
			subReceiver = param.Receiver
		} else {
			subType = sub.Variadic
		}
		if index < len(super.Params) {
			param := super.Params[index]
			superType = param.Type
			superReceiver = param.Receiver
		} else {
			superType = super.Variadic
		}
		// Receiver is an explicit calling-convention position, not
		// presentation metadata. A variadic fallback never supplies a
		// receiver position, so a receiver cannot be widened into or out of
		// one.
		if subReceiver != superReceiver && (subType != nil || superType != nil) {
			return proofFalse()
		}
		if subType != nil && superType != nil {
			parts = append(parts, subtypeOf(superType, subType))
		}
	}
	if sub.Variadic != nil && super.Variadic != nil {
		parts = append(parts, subtypeOf(super.Variadic, sub.Variadic))
	}
	for index, expected := range super.Returns {
		actual := typ.Nil
		if index < len(sub.Returns) {
			actual = sub.Returns[index]
		}
		parts = append(parts, subtypeOf(actual, expected))
	}
	return proofAll(parts...)
}

func minRequiredArgs(fn *typ.Function) int {
	if fn == nil {
		return 0
	}
	required := 0
	for index, param := range fn.Params {
		if !param.Optional {
			required = index + 1
		}
	}
	return required
}

func (c *checker) tupleRule(sub, super *typ.Tuple) proofExpr {
	if sub == nil || super == nil || len(sub.Elements) != len(super.Elements) {
		return proofFalse()
	}
	parts := make([]proofExpr, 0, len(sub.Elements))
	for index, element := range sub.Elements {
		parts = append(parts, subtypeOf(element, super.Elements[index]))
	}
	return proofAll(parts...)
}

func (c *checker) mapRule(sub, super *typ.Map) proofExpr {
	if sub == nil || super == nil {
		return proofFalse()
	}
	return proofAll(
		subtypeOf(sub.Key, super.Key), subtypeOf(super.Key, sub.Key),
		subtypeOf(sub.Value, super.Value), subtypeOf(super.Value, sub.Value),
	)
}

func (c *checker) readonlyMapRule(sub, super *typ.ReadonlyMap) proofExpr {
	if sub == nil || super == nil {
		return proofFalse()
	}
	return proofAll(subtypeOf(sub.Key, super.Key), subtypeOf(typetable.PresentReadonlyEntryValue(sub.Value), super.Value))
}

func (c *checker) interfaceRule(sub, super *typ.Interface) proofExpr {
	if sub == nil || super == nil {
		return proofFalse()
	}
	if len(super.Methods) == 0 {
		return proofTrue()
	}
	parts := make([]proofExpr, 0, len(super.Methods))
	for _, expected := range super.Methods {
		found := false
		for _, actual := range sub.Methods {
			if actual.Name == expected.Name {
				parts = append(parts, subtypeOf(actual.Type, expected.Type))
				found = true
				break
			}
		}
		if !found {
			return proofFalse()
		}
	}
	return proofAll(parts...)
}

func (c *checker) instantiatedRule(sub, super *typ.Instantiated, relation proofRelation) proofExpr {
	if sub == nil || super == nil || sub.Generic == nil || super.Generic == nil || !typ.TypeEquals(sub.Generic, super.Generic) || len(sub.TypeArgs) != len(super.TypeArgs) {
		return proofFalse()
	}
	parts := make([]proofExpr, 0, len(sub.TypeArgs)*2)
	for index, argument := range sub.TypeArgs {
		if relation == proofSubtype {
			parts = append(parts, subtypeOf(argument, super.TypeArgs[index]), subtypeOf(super.TypeArgs[index], argument))
		} else {
			parts = append(parts, widenEither(argument, super.TypeArgs[index]), widenEither(super.TypeArgs[index], argument))
		}
	}
	return proofAll(parts...)
}

func (c *checker) recordRule(sub, super *typ.Record) proofExpr {
	if sub == nil || super == nil {
		return proofFalse()
	}
	parts := make([]proofExpr, 0, len(super.Fields)+len(super.StaticMembers)*2+3)
	for _, expected := range super.Fields {
		actual, ok := recordReadableField(sub, expected.Name)
		if !ok {
			if !expected.Optional && !unwrap.IsOptionalLike(expected.Type) {
				return proofFalse()
			}
			continue
		}
		member := c.recordMemberRule(actual, recordMemberShape{typ: expected.Type, optional: expected.Optional, readonly: expected.Readonly})
		parts = append(parts, member)
	}
	for _, expected := range super.StaticMembers {
		actual, ok := recordReadableStaticMember(sub, expected)
		if !ok {
			if !expected.Optional && !unwrap.IsOptionalLike(expected.Type) {
				return proofFalse()
			}
			continue
		}
		parts = append(parts, c.recordMemberRule(actual, recordMemberShape{typ: expected.Type, optional: expected.Optional, readonly: expected.Readonly}))
	}
	if super.HasMapComponent() {
		if !sub.HasMapComponent() {
			return proofFalse()
		}
		parts = append(parts, subtypeOf(sub.MapKey, super.MapKey), subtypeOf(sub.MapValue, super.MapValue))
	}
	if meta := c.metaRule(sub.Metatable, super.Metatable); meta.known < 0 {
		return proofFalse()
	} else {
		parts = append(parts, meta)
	}
	return proofAll(parts...)
}

func (c *checker) recordMemberRule(sub, super recordMemberShape) proofExpr {
	if super.optional && sub.typ != nil && sub.typ.Kind() == kind.Nil {
		return proofTrue()
	}
	effectiveSuper := super.typ
	if super.optional {
		effectiveSuper = typeexpr.Optional(super.typ)
	}
	parts := []proofExpr{subtypeOf(sub.typ, effectiveSuper)}
	if !super.readonly {
		if sub.readonly {
			return proofFalse()
		}
		parts = append(parts, proofAny(subtypeOf(effectiveSuper, sub.typ), widenOf(sub.typ, effectiveSuper)))
	}
	if !super.optional && !unwrap.IsOptionalLike(super.typ) && sub.optional {
		return proofFalse()
	}
	return proofAll(parts...)
}

func (c *checker) metaRule(sub, super typ.Type) proofExpr {
	if sub == nil && super == nil {
		return proofTrue()
	}
	subUnconstrained := sub != nil && typetable.IsMetatableUnconstrained(sub)
	superUnconstrained := super != nil && typetable.IsMetatableUnconstrained(super)
	if subUnconstrained && (super == nil || superUnconstrained) {
		return proofTrue()
	}
	if superUnconstrained || (super != nil && typ.IsUnknown(super)) {
		return proofTrue()
	}
	if sub != nil && typ.IsUnknown(sub) {
		return proofFalse()
	}
	if subUnconstrained || sub == nil || super == nil {
		return proofFalse()
	}
	return subtypeOf(sub, super)
}

func (c *checker) recordToInterfaceRule(sub *typ.Record, super *typ.Interface) proofExpr {
	if sub == nil || super == nil {
		return proofFalse()
	}
	parts := make([]proofExpr, 0, len(super.Methods))
	for _, method := range super.Methods {
		field := sub.GetField(method.Name)
		if field == nil {
			return proofFalse()
		}
		parts = append(parts, subtypeOf(field.Type, subst.Self(method.Type, sub)))
	}
	return proofAll(parts...)
}

func (c *checker) recordToMapRule(sub *typ.Record, super *typ.Map) proofExpr {
	if sub == nil || super == nil {
		return proofFalse()
	}
	parts := make([]proofExpr, 0, len(sub.Fields)*2+2)
	for _, field := range sub.Fields {
		parts = append(parts, subtypeOf(typ.LiteralString(field.Name), super.Key), subtypeOf(field.Type, super.Value))
	}
	if sub.HasMapComponent() {
		parts = append(parts, subtypeOf(sub.MapKey, super.Key), subtypeOf(sub.MapValue, super.Value))
	}
	return proofAll(parts...)
}

func (c *checker) recordToReadonlyMapRule(sub *typ.Record, super *typ.ReadonlyMap) proofExpr {
	if sub == nil || super == nil {
		return proofFalse()
	}
	parts := make([]proofExpr, 0, len(sub.Fields)*2+len(sub.StaticMembers)*2+4)
	for _, field := range sub.Fields {
		parts = append(parts, subtypeOf(typ.LiteralString(field.Name), super.Key), subtypeOf(typetable.PresentReadonlyEntryValue(field.Type), super.Value))
	}
	for _, member := range sub.StaticMembers {
		key, ok := readonlyStaticMemberKeyType(member)
		if !ok {
			return proofFalse()
		}
		parts = append(parts, subtypeOf(key, super.Key), subtypeOf(typetable.PresentReadonlyEntryValue(member.Type), super.Value))
	}
	if sub.Open || sub.Metatable != nil {
		parts = append(parts, subtypeOf(typ.String, super.Key), subtypeOf(typ.Unknown, super.Value))
	}
	if sub.HasMapComponent() {
		parts = append(parts, subtypeOf(sub.MapKey, super.Key), subtypeOf(typetable.PresentReadonlyEntryValue(sub.MapValue), super.Value))
	}
	return proofAll(parts...)
}

func (c *checker) mapToRecordRule(sub *typ.Map, super *typ.Record) proofExpr {
	if sub == nil || super == nil || !super.HasMapComponent() {
		return proofFalse()
	}
	parts := []proofExpr{c.mapRule(sub, typetable.NewMap(super.MapKey, super.MapValue))}
	for _, field := range super.Fields {
		if !field.Optional && !unwrap.IsOptionalLike(field.Type) {
			return proofFalse()
		}
		// A statically excluded key does not constrain the source map. Model the
		// branch as implication so evaluation remains in the one explicit proof
		// stack rather than recursively asking the checker while building it.
		expected := field.Type
		if field.Optional && !unwrap.IsOptionalLike(expected) {
			expected = typeexpr.Optional(expected)
		}
		parts = append(parts, proofAny(
			proofNot(subtypeOf(typ.LiteralString(field.Name), sub.Key)),
			proofAll(subtypeOf(sub.Value, expected), proofAny(subtypeOf(expected, sub.Value), widenOf(sub.Value, expected))),
		))
	}
	return proofAll(parts...)
}

func widenEither(narrow, wide typ.Type) proofExpr {
	return proofAny(subtypeOf(narrow, wide), widenOf(narrow, wide))
}

func (c *checker) widenRule(narrow, wide typ.Type) proofExpr {
	if inst, ok := narrow.(*typ.Instantiated); ok {
		expanded := subst.ExpandInstantiated(inst)
		if expanded != nil && expanded != narrow {
			return widenEither(expanded, subst.Self(wide, inst))
		}
	}
	if inst, ok := wide.(*typ.Instantiated); ok {
		expanded := subst.ExpandInstantiated(inst)
		if expanded != nil && expanded != wide {
			return widenEither(subst.Self(narrow, inst), expanded)
		}
	}
	if subRec, ok := narrow.(*typ.Recursive); ok && subRec.Body != nil && subRec.Body != narrow {
		if supRec, ok := wide.(*typ.Recursive); ok && supRec.Body != nil && supRec.Body != wide {
			return widenEither(subRec.Body, supRec.Body)
		}
		return widenEither(subRec.Body, wide)
	}
	if supRec, ok := wide.(*typ.Recursive); ok && supRec.Body != nil && supRec.Body != wide {
		return widenEither(narrow, supRec.Body)
	}
	if typ.IsAny(wide) {
		return proofTrue()
	}
	if typ.IsBuiltinTableTopMarker(wide) {
		return booleanExpr(typetable.IsLike(narrow))
	}
	if narrow.Kind() == kind.Nil {
		if _, ok := wide.(*typ.Optional); ok {
			return proofTrue()
		}
		if union, ok := wide.(*typ.Union); ok {
			for _, member := range union.Members {
				if member.Kind() == kind.Nil {
					return proofTrue()
				}
			}
		}
	}
	if optional, ok := wide.(*typ.Optional); ok {
		if narrowOptional, ok := narrow.(*typ.Optional); ok {
			return widenEither(narrowOptional.Inner, optional.Inner)
		}
		return widenEither(narrow, optional.Inner)
	}
	if union, ok := narrow.(*typ.Union); ok {
		if len(union.Members) == 0 {
			return proofFalse()
		}
		parts := make([]proofExpr, 0, len(union.Members))
		for _, member := range union.Members {
			parts = append(parts, widenEither(member, wide))
		}
		return proofAll(parts...)
	}
	if union, ok := wide.(*typ.Union); ok {
		parts := make([]proofExpr, 0, len(union.Members))
		for _, member := range union.Members {
			if member.Kind() != kind.Literal {
				parts = append(parts, widenEither(narrow, member))
			}
		}
		return proofAny(parts...)
	}
	if narrow.Kind() == kind.Integer && wide.Kind() == kind.Number {
		return proofTrue()
	}
	if literal, ok := narrow.(*typ.Literal); ok {
		wideInner := wide
		if optional, ok := wide.(*typ.Optional); ok {
			wideInner = unwrap.Alias(optional.Inner)
		}
		switch literal.Base() {
		case kind.Boolean:
			return booleanExpr(wideInner != nil && wideInner.Kind() == kind.Boolean)
		case kind.String:
			return booleanExpr(wideInner != nil && wideInner.Kind() == kind.String)
		case kind.Integer:
			return booleanExpr(wideInner != nil && (wideInner.Kind() == kind.Integer || wideInner.Kind() == kind.Number))
		case kind.Number:
			return booleanExpr(wideInner != nil && wideInner.Kind() == kind.Number)
		}
	}
	if record, ok := narrow.(*typ.Record); ok {
		if emptyRecordAdoptsContainerShape(record, wide) {
			return proofTrue()
		}
		if target, ok := wide.(*typ.Record); ok {
			return c.widenRecordRule(record, target)
		}
		if target, ok := wide.(*typ.Array); ok {
			return c.widenRecordToArrayRule(record, target)
		}
		if target, ok := wide.(*typ.Map); ok {
			return c.widenRecordToMapRule(record, target)
		}
	}
	if source, ok := narrow.(*typ.Map); ok {
		if target, ok := wide.(*typ.Map); ok {
			return c.widenMapRule(source, target)
		}
	}
	if source, ok := narrow.(*typ.Array); ok {
		if target, ok := wide.(*typ.Array); ok {
			return widenEither(source.Element, target.Element)
		}
		if target, ok := wide.(*typ.Map); ok {
			return proofAll(subtypeOf(typ.Integer, target.Key), widenEither(source.Element, target.Value))
		}
	}
	if source, ok := narrow.(*typ.Tuple); ok {
		if target, ok := wide.(*typ.Tuple); ok {
			if len(source.Elements) != len(target.Elements) {
				return proofFalse()
			}
			parts := make([]proofExpr, 0, len(source.Elements))
			for index, element := range source.Elements {
				parts = append(parts, widenEither(element, target.Elements[index]))
			}
			return proofAll(parts...)
		}
	}
	if source, ok := narrow.(*typ.Function); ok {
		if target, ok := wide.(*typ.Function); ok {
			params := c.functionParametersEquivalentRule(source, target)
			returns := make([]proofExpr, 0, len(target.Returns)+1)
			returns = append(returns, params)
			for index, targetReturn := range target.Returns {
				sourceReturn := typ.Nil
				if index < len(source.Returns) {
					sourceReturn = source.Returns[index]
				}
				returns = append(returns, widenEither(sourceReturn, targetReturn))
			}
			return proofAll(returns...)
		}
	}
	return proofFalse()
}

func (c *checker) widenMapRule(narrow, wide *typ.Map) proofExpr {
	if narrow == nil || wide == nil {
		return proofFalse()
	}
	parts := []proofExpr{subtypeOf(narrow.Key, wide.Key), subtypeOf(wide.Key, narrow.Key)}
	if typ.IsNever(narrow.Value) {
		parts = append(parts, widenEither(narrow.Value, wide.Value))
	} else {
		parts = append(parts, subtypeOf(narrow.Value, wide.Value), subtypeOf(wide.Value, narrow.Value))
	}
	return proofAll(parts...)
}

func (c *checker) widenRecordToMapRule(narrow *typ.Record, wide *typ.Map) proofExpr {
	if narrow == nil || wide == nil {
		return proofFalse()
	}
	parts := make([]proofExpr, 0, len(narrow.Fields)*2+2)
	for _, field := range narrow.Fields {
		parts = append(parts, subtypeOf(typ.LiteralString(field.Name), wide.Key), widenEither(field.Type, wide.Value))
	}
	if narrow.HasMapComponent() {
		parts = append(parts, subtypeOf(narrow.MapKey, wide.Key), widenEither(narrow.MapValue, wide.Value))
	}
	return proofAll(parts...)
}

func (c *checker) widenRecordToArrayRule(narrow *typ.Record, wide *typ.Array) proofExpr {
	if narrow == nil || wide == nil || len(narrow.Fields) != 0 {
		return proofFalse()
	}
	parts := make([]proofExpr, 0, len(narrow.StaticMembers)+2)
	for _, member := range narrow.StaticMembers {
		if member.Kind != typ.StaticMemberIntIndex {
			return proofFalse()
		}
		parts = append(parts, widenEither(member.Type, wide.Element))
	}
	if narrow.HasMapComponent() {
		parts = append(parts, subtypeOf(narrow.MapKey, typ.Integer), widenEither(narrow.MapValue, wide.Element))
	}
	return proofAll(parts...)
}

func (c *checker) functionParametersEquivalentRule(a, b *typ.Function) proofExpr {
	if a == nil || b == nil || len(a.Params) != len(b.Params) {
		return proofFalse()
	}
	parts := make([]proofExpr, 0, len(a.Params)*2+2)
	for index, aParam := range a.Params {
		bParam := b.Params[index]
		if aParam.Optional != bParam.Optional || aParam.Receiver != bParam.Receiver {
			return proofFalse()
		}
		parts = append(parts, subtypeOf(aParam.Type, bParam.Type), subtypeOf(bParam.Type, aParam.Type))
	}
	if a.Variadic == nil && b.Variadic == nil {
		return proofAll(parts...)
	}
	if a.Variadic == nil || b.Variadic == nil {
		return proofFalse()
	}
	parts = append(parts, subtypeOf(a.Variadic, b.Variadic), subtypeOf(b.Variadic, a.Variadic))
	return proofAll(parts...)
}

func (c *checker) widenRecordRule(narrow, wide *typ.Record) proofExpr {
	if narrow == nil || wide == nil {
		return proofFalse()
	}
	parts := make([]proofExpr, 0, len(wide.Fields)+len(wide.StaticMembers))
	for _, wanted := range wide.Fields {
		actual := narrow.GetField(wanted.Name)
		if actual == nil {
			if !wanted.Optional && !unwrap.IsOptionalLike(wanted.Type) {
				return proofFalse()
			}
			continue
		}
		parts = append(parts, widenEither(actual.Type, wanted.Type))
	}
	for _, wanted := range wide.StaticMembers {
		actual := narrow.GetStaticMember(wanted.Kind, wanted.Name, wanted.Index)
		if actual == nil {
			if !wanted.Optional && !unwrap.IsOptionalLike(wanted.Type) {
				return proofFalse()
			}
			continue
		}
		parts = append(parts, widenEither(actual.Type, wanted.Type))
	}
	return proofAll(parts...)
}
