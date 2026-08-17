package subst

import (
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// substituteParamsIterative is the non-recursive counterpart of Params.  A
// task owns its result slot; parents rebuild only after every child has filled
// its slot, which closes recursive graphs through the memoized μ placeholder.
func substituteParamsIterative(t typ.Type, subs []paramSubstitution) typ.Type {
	if t == nil || len(subs) == 0 {
		return t
	}
	root := &paramTask{phase: paramVisit, input: t, subs: subs}
	m := paramMachine{memo: make(map[typ.Type]typ.Type), stack: []*paramTask{root}}
	for len(m.stack) != 0 {
		i := len(m.stack) - 1
		w := m.stack[i]
		m.stack = m.stack[:i]
		m.step(w)
	}
	return root.result
}

type paramPhase uint8

const (
	paramVisit paramPhase = iota
	paramFinish
	paramFunctionFinish
	paramGenericStart
	paramGenericFinish
	paramRecursiveFinish
)

type paramTask struct {
	phase       paramPhase
	input       typ.Type
	subs        []paramSubstitution
	children    []*paramTask
	result      typ.Type
	replacement *typ.Recursive
	protected   map[*typ.TypeParam]*typ.TypeParam
	typeParams  []*typ.TypeParam
}
type paramMachine struct {
	memo  map[typ.Type]typ.Type
	stack []*paramTask
}

func (m *paramMachine) child(t typ.Type, subs []paramSubstitution) *paramTask {
	return &paramTask{phase: paramVisit, input: t, subs: subs}
}
func (m *paramMachine) childFrom(parent *paramTask, t typ.Type, subs []paramSubstitution) *paramTask {
	child := m.child(t, subs)
	child.protected = parent.protected
	return child
}
func (m *paramMachine) push(w *paramTask, children []*paramTask) {
	w.children = children
	m.stack = append(m.stack, w)
	for i := len(children) - 1; i >= 0; i-- {
		m.stack = append(m.stack, children[i])
	}
}

func (m *paramMachine) step(w *paramTask) {
	switch w.phase {
	case paramVisit:
		m.visit(w)
	case paramFinish:
		m.finish(w)
	case paramFunctionFinish:
		m.finishFunction(w)
	case paramGenericStart:
		m.startGeneric(w)
	case paramGenericFinish:
		m.finishGeneric(w)
	case paramRecursiveFinish:
		m.finishRecursive(w)
	}
}

func (m *paramMachine) visit(w *paramTask) {
	if w.input == nil {
		return
	}
	if cached, ok := m.memo[w.input]; ok {
		w.result = cached
		return
	}
	if tp, ok := w.input.(*typ.TypeParam); ok {
		if replacement, found := w.protected[tp]; found {
			w.result = replacement
			return
		}
	}
	if tp, ok := w.input.(*typ.TypeParam); ok {
		if arg, ok := lookupParamSubstitution(tp, w.subs); ok {
			m.memo[w.input] = arg
			w.result = arg
			return
		}
	}
	v := typ.UnwrapTransparentWrappers(w.input)
	if !paramCanDescend(v) {
		w.result = w.input
		return
	}
	m.memo[w.input] = w.input
	child := func(t typ.Type) *paramTask { return m.childFrom(w, t, w.subs) }
	switch x := v.(type) {
	case *typ.TypeParam:
		w.phase = paramFinish
		m.push(w, []*paramTask{child(x.Constraint)})
	case *typ.Function:
		owned := functionOwnsSubstitutions(x, w.subs)
		bodySubs := w.subs
		kept := make([]*typ.TypeParam, 0, len(x.TypeParams))
		for _, p := range x.TypeParams {
			if p == nil {
				continue
			}
			if owned[p] {
				continue
			}
			bodySubs = removeShadowedSubstitutions(bodySubs, p)
			kept = append(kept, p)
		}
		children := make([]*paramTask, 0, len(x.Params)+len(x.Returns)+1)
		for _, p := range x.Params {
			children = append(children, m.childFrom(w, p.Type, bodySubs))
		}
		for _, r := range x.Returns {
			children = append(children, m.childFrom(w, r, bodySubs))
		}
		if x.Variadic != nil {
			children = append(children, m.childFrom(w, x.Variadic, bodySubs))
		}
		w.subs = append([]paramSubstitution(nil), bodySubs...)
		w.phase = paramFunctionFinish
		w.children = append([]*paramTask{&paramTask{result: functionBinderMarker(kept)}}, children...)
		m.stack = append(m.stack, w)
		for i := len(children) - 1; i >= 0; i-- {
			m.stack = append(m.stack, children[i])
		}
	case *typ.Generic:
		// Generic binders shadow matching outer substitutions.  Their constraints
		// are declaration-owned and are left intact, matching Params' function
		// binder policy while preserving body scope.
		children := make([]*paramTask, len(x.TypeParams))
		for i, param := range x.TypeParams {
			children[i] = m.childFrom(w, param.Constraint, w.subs)
		}
		w.phase = paramGenericStart
		m.push(w, children)
	case *typ.Recursive:
		if x.Body == nil {
			w.result = w.input
			return
		}
		r := typ.NewRecursivePlaceholder(x.Name)
		m.memo[w.input] = r
		w.replacement = r
		w.phase = paramRecursiveFinish
		m.push(w, []*paramTask{m.childFrom(w, x.Body, w.subs)})
	case *typ.Optional:
		w.phase = paramFinish
		m.push(w, []*paramTask{child(x.Inner)})
	case *typ.Array:
		w.phase = paramFinish
		m.push(w, []*paramTask{child(x.Element)})
	case *typ.Map:
		w.phase = paramFinish
		m.push(w, []*paramTask{child(x.Key), child(x.Value)})
	case *typ.ReadonlyMap:
		w.phase = paramFinish
		m.push(w, []*paramTask{child(x.Key), child(x.Value)})
	case *typ.Union:
		w.phase = paramFinish
		m.push(w, paramChildren(x.Members, child))
	case *typ.Intersection:
		w.phase = paramFinish
		m.push(w, paramChildren(x.Members, child))
	case *typ.Tuple:
		w.phase = paramFinish
		m.push(w, paramChildren(x.Elements, child))
	case *typ.Alias:
		w.phase = paramFinish
		m.push(w, []*paramTask{child(x.Target)})
	case *typ.Meta:
		w.phase = paramFinish
		m.push(w, []*paramTask{child(x.Of)})
	case *typ.Instantiated:
		w.phase = paramFinish
		m.push(w, paramChildren(x.TypeArgs, child))
	case *typ.Interface:
		children := make([]*paramTask, len(x.Methods))
		for i, method := range x.Methods {
			children[i] = child(method.Type)
		}
		w.phase = paramFinish
		m.push(w, children)
	case *typ.Record:
		children := make([]*paramTask, 0, len(x.Fields)+len(x.StaticMembers)+3)
		for _, f := range x.Fields {
			children = append(children, child(f.Type))
		}
		for _, s := range x.StaticMembers {
			children = append(children, child(s.Type))
		}
		if x.Metatable != nil {
			children = append(children, child(x.Metatable))
		}
		if x.HasMapComponent() {
			children = append(children, child(x.MapKey), child(x.MapValue))
		}
		w.phase = paramFinish
		m.push(w, children)
	default:
		w.result = w.input
	}
}

// A private marker avoids widening paramTask just to retain the kept binders.
func functionBinderMarker(params []*typ.TypeParam) typ.Type { return typ.NewGeneric("", params, nil) }

func (m *paramMachine) finishFunction(w *paramTask) {
	x := typ.UnwrapTransparentWrappers(w.input).(*typ.Function)
	kept := w.children[0].result.(*typ.Generic).TypeParams
	i := 1
	changed := len(kept) != len(x.TypeParams)
	params := make([]typ.Param, len(x.Params))
	for n, p := range x.Params {
		r := w.children[i].result
		i++
		params[n] = typ.Param{Name: p.Name, Type: r, Optional: p.Optional, Receiver: p.Receiver}
		changed = changed || r != p.Type
	}
	returns := make([]typ.Type, len(x.Returns))
	for n, r := range x.Returns {
		got := w.children[i].result
		i++
		returns[n] = got
		changed = changed || got != r
	}
	variadic := x.Variadic
	if variadic != nil {
		variadic = w.children[i].result
		changed = changed || variadic != x.Variadic
	}
	if !changed {
		w.result = w.input
	} else {
		b := typ.Func()
		for _, p := range kept {
			b.TypeParamRef(p)
		}
		for _, p := range params {
			if p.Optional {
				b.OptParam(p.Name, p.Type)
			} else {
				b.Param(p.Name, p.Type)
			}
		}
		if variadic != nil {
			b.Variadic(variadic)
		}
		if len(returns) > 0 {
			b.Returns(returns...)
		}
		w.result = b.Build()
	}
	m.memo[w.input] = w.result
}
func (m *paramMachine) startGeneric(w *paramTask) {
	x := typ.UnwrapTransparentWrappers(w.input).(*typ.Generic)
	params := x.TypeParams
	paramsChanged := false
	var replacements map[*typ.TypeParam]*typ.TypeParam
	for i, original := range x.TypeParams {
		constraint := w.children[i].result
		if constraint == original.Constraint {
			continue
		}
		if !paramsChanged {
			params = append([]*typ.TypeParam(nil), x.TypeParams...)
			paramsChanged = true
		}
		rewritten := typ.NewTypeParam(original.Name, constraint)
		params[i] = rewritten
		if replacements == nil {
			replacements = make(map[*typ.TypeParam]*typ.TypeParam)
		}
		replacements[original] = rewritten
	}
	bodySubs := w.subs
	for _, param := range x.TypeParams {
		bodySubs = removeShadowedSubstitutions(bodySubs, param)
	}
	bodyTask := m.childFrom(w, x.Body, bodySubs)
	if len(replacements) != 0 {
		protected := make(map[*typ.TypeParam]*typ.TypeParam, len(w.protected)+len(replacements))
		for original, replacement := range w.protected {
			protected[original] = replacement
		}
		for original, replacement := range replacements {
			protected[original] = replacement
		}
		bodyTask.protected = protected
	}
	w.typeParams = params
	w.children = []*paramTask{bodyTask}
	w.phase = paramGenericFinish
	m.push(w, w.children)
}

func (m *paramMachine) finishGeneric(w *paramTask) {
	x := typ.UnwrapTransparentWrappers(w.input).(*typ.Generic)
	body := w.children[0].result
	params := w.typeParams
	paramsChanged := len(params) != len(x.TypeParams)
	if !paramsChanged {
		for i := range params {
			if params[i] != x.TypeParams[i] {
				paramsChanged = true
				break
			}
		}
	}
	if body == x.Body && !paramsChanged {
		w.result = w.input
	} else {
		w.result = typ.NewGeneric(x.Name, params, body)
	}
	m.memo[w.input] = w.result
}
func (m *paramMachine) finishRecursive(w *paramTask) {
	x := typ.UnwrapTransparentWrappers(w.input).(*typ.Recursive)
	body := w.children[0].result
	if body == x.Body {
		w.result = w.input
	} else {
		w.replacement.SetBody(body)
		w.result = w.replacement
	}
	m.memo[w.input] = w.result
}

func (m *paramMachine) finish(w *paramTask) {
	x := typ.UnwrapTransparentWrappers(w.input)
	out := w.input
	rs := paramResults(w.children)
	switch v := x.(type) {
	case *typ.Optional:
		if rs[0] != v.Inner {
			out = typeexpr.Optional(rs[0])
		}
	case *typ.Array:
		if rs[0] != v.Element {
			out = typ.NewArray(rs[0])
		}
	case *typ.Map:
		if rs[0] != v.Key || rs[1] != v.Value {
			out = typetable.NewMap(rs[0], rs[1])
		}
	case *typ.ReadonlyMap:
		if rs[0] != v.Key || rs[1] != v.Value {
			out = typetable.NewReadonlyMap(rs[0], rs[1])
		}
	case *typ.Union:
		if paramChanged(v.Members, w.children) {
			out = typeexpr.Union(rs...)
		}
	case *typ.Intersection:
		if paramChanged(v.Members, w.children) {
			out = typeexpr.Intersection(rs...)
		}
	case *typ.Tuple:
		if paramChanged(v.Elements, w.children) {
			out = typ.NewTuple(rs...)
		}
	case *typ.Alias:
		if rs[0] != v.Target {
			out = typ.NewAlias(v.Name, rs[0])
		}
	case *typ.Meta:
		if rs[0] != v.Of {
			out = typ.NewMeta(rs[0])
		}
	case *typ.TypeParam:
		if rs[0] != v.Constraint {
			out = typ.NewTypeParam(v.Name, rs[0])
		}
	case *typ.Instantiated:
		if paramChanged(v.TypeArgs, w.children) {
			out = typ.Instantiate(v.Generic, rs...)
		}
	case *typ.Interface:
		var methods []typ.Method
		for i, r := range rs {
			if f, ok := r.(*typ.Function); ok && f != v.Methods[i].Type {
				if methods == nil {
					methods = append([]typ.Method(nil), v.Methods...)
				}
				methods[i].Type = f
			}
		}
		if methods != nil {
			out = typ.NewInterface(v.Name, methods)
		}
	case *typ.Record:
		out = paramRecord(w, v)
	}
	w.result = out
	m.memo[w.input] = out
}
func paramRecord(w *paramTask, x *typ.Record) typ.Type {
	i := 0
	changed := false
	var fields []typ.Field
	for n, f := range x.Fields {
		r := w.children[i].result
		i++
		if r != f.Type {
			if fields == nil {
				fields = append([]typ.Field(nil), x.Fields...)
			}
			fields[n].Type = r
			changed = true
		}
	}
	var statics []typ.StaticMember
	for n, s := range x.StaticMembers {
		r := w.children[i].result
		i++
		if r != s.Type {
			if statics == nil {
				statics = append([]typ.StaticMember(nil), x.StaticMembers...)
			}
			statics[n].Type = r
			changed = true
		}
	}
	meta := x.Metatable
	if meta != nil {
		r := w.children[i].result
		i++
		if r != meta {
			meta = r
			changed = true
		}
	}
	key, val := x.MapKey, x.MapValue
	if x.HasMapComponent() {
		r := w.children[i].result
		i++
		if r != key {
			key = r
			changed = true
		}
		r = w.children[i].result
		if r != val {
			val = r
			changed = true
		}
	}
	if !changed {
		return w.input
	}
	fs := x.Fields
	if fields != nil {
		fs = fields
	}
	ss := x.StaticMembers
	if statics != nil {
		ss = statics
	}
	return typetable.RebuildRecord(typ.RecordParts{Fields: fs, StaticMembers: ss, Metatable: meta, MapKey: key, MapValue: val, Open: x.Open, AssumeSorted: true})
}
func paramChildren(values []typ.Type, makeChild func(typ.Type) *paramTask) []*paramTask {
	out := make([]*paramTask, len(values))
	for i, v := range values {
		out[i] = makeChild(v)
	}
	return out
}
func paramResults(tasks []*paramTask) []typ.Type {
	out := make([]typ.Type, len(tasks))
	for i, t := range tasks {
		out[i] = t.result
	}
	return out
}
func paramChanged(values []typ.Type, tasks []*paramTask) bool {
	for i, v := range values {
		if tasks[i].result != v {
			return true
		}
	}
	return false
}
func paramCanDescend(t typ.Type) bool {
	switch t.(type) {
	case *typ.Optional, *typ.Array, *typ.Map, *typ.ReadonlyMap, *typ.Union, *typ.Intersection, *typ.Tuple, *typ.Function, *typ.Generic, *typ.Recursive, *typ.Alias, *typ.Meta, *typ.TypeParam, *typ.Instantiated, *typ.Interface, *typ.Record:
		return true
	}
	return false
}
