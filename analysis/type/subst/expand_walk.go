package subst

import (
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/transform"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// expandInstantiatedGuardMode is a result-carrying graph walk.  In particular,
// its explicit stack keeps a regular recursive generic finite without putting
// the input product depth on Go's call stack.
func expandInstantiatedGuardMode(t typ.Type, state *expandState, mode expandMode) typ.Type {
	root := &expandTask{phase: expandVisit, input: t, mode: mode}
	m := expandMachine{state: state, stack: []*expandTask{root}}
	for len(m.stack) != 0 {
		i := len(m.stack) - 1
		work := m.stack[i]
		m.stack = m.stack[:i]
		m.step(work)
	}
	return root.result
}

type expandPhase uint8

const (
	expandVisit expandPhase = iota
	expandFinish
	expandGenericFinish
)

type expandTask struct {
	phase    expandPhase
	input    typ.Type
	orig     typ.Type
	mode     expandMode
	key      expandMemoKey
	children []*expandTask
	active   *activeInstantiation
	result   typ.Type
}

type expandMachine struct {
	state *expandState
	stack []*expandTask
}

func (m *expandMachine) child(t typ.Type, mode expandMode) *expandTask {
	return &expandTask{phase: expandVisit, input: t, mode: mode}
}

func (m *expandMachine) pushChildren(parent *expandTask, children []*expandTask) {
	parent.children = children
	m.stack = append(m.stack, parent)
	for i := len(children) - 1; i >= 0; i-- {
		m.stack = append(m.stack, children[i])
	}
}

func (m *expandMachine) step(w *expandTask) {
	switch w.phase {
	case expandVisit:
		m.visit(w)
	case expandFinish:
		m.finish(w)
	case expandGenericFinish:
		body := w.children[0].result
		m.state.active = m.state.active[:len(m.state.active)-1]
		if !w.active.used {
			w.result = body
		} else {
			w.active.mu.SetBody(body)
			w.result = w.active.mu
		}
		m.state.memo[w.key] = w.result
	}
}

func (m *expandMachine) visit(w *expandTask) {
	if w.input == nil {
		return
	}
	if w.mode == expandModeStructural && !typ.ContainsInstantiated(w.input) {
		w.result = w.input
		return
	}
	w.key = expandMemoKey{t: w.input, mode: w.mode}
	if cached, ok := m.state.memo[w.key]; ok {
		w.result = cached
		return
	}
	w.orig = w.input
	v := unwrap.Annotations(w.input)
	m.state.memo[w.key] = w.orig
	child := func(t typ.Type) *expandTask { return m.child(t, w.mode) }
	switch x := v.(type) {
	case *typ.Instantiated:
		m.expandGeneric(w, x)
	case *typ.Optional:
		w.phase = expandFinish
		m.pushChildren(w, []*expandTask{child(x.Inner)})
	case *typ.Union:
		w.phase = expandFinish
		m.pushChildren(w, expandChildren(x.Members, child))
	case *typ.Intersection:
		w.phase = expandFinish
		m.pushChildren(w, expandChildren(x.Members, child))
	case *typ.Array:
		w.phase = expandFinish
		m.pushChildren(w, []*expandTask{child(x.Element)})
	case *typ.Map:
		w.phase = expandFinish
		m.pushChildren(w, []*expandTask{child(x.Key), child(x.Value)})
	case *typ.ReadonlyMap:
		w.phase = expandFinish
		m.pushChildren(w, []*expandTask{child(x.Key), child(x.Value)})
	case *typ.Tuple:
		w.phase = expandFinish
		m.pushChildren(w, expandChildren(x.Elements, child))
	case *typ.Function:
		children := make([]*expandTask, 0, len(x.Params)+len(x.Returns)+1)
		for _, p := range x.Params {
			if isRecursiveInstantiated(p.Type) {
				children = append(children, &expandTask{result: p.Type})
			} else {
				children = append(children, child(p.Type))
			}
		}
		for _, r := range x.Returns {
			children = append(children, child(r))
		}
		if x.Variadic != nil {
			children = append(children, child(x.Variadic))
		}
		w.phase = expandFinish
		m.pushChildren(w, children)
	case *typ.Record:
		children := make([]*expandTask, 0, len(x.Fields)+len(x.StaticMembers)+3)
		for _, field := range x.Fields {
			children = append(children, child(field.Type))
		}
		for _, member := range x.StaticMembers {
			children = append(children, child(member.Type))
		}
		if x.Metatable != nil {
			children = append(children, child(x.Metatable))
		}
		if x.HasMapComponent() {
			children = append(children, child(x.MapKey), child(x.MapValue))
		}
		w.phase = expandFinish
		m.pushChildren(w, children)
	case *typ.Alias:
		w.phase = expandFinish
		m.pushChildren(w, []*expandTask{child(x.Target)})
	case *typ.Interface:
		children := make([]*expandTask, len(x.Methods))
		for i, method := range x.Methods {
			children[i] = child(method.Type)
		}
		w.phase = expandFinish
		m.pushChildren(w, children)
	default:
		w.result = w.orig
		m.state.memo[w.key] = w.result
	}
}

func expandChildren(values []typ.Type, makeChild func(typ.Type) *expandTask) []*expandTask {
	out := make([]*expandTask, len(values))
	for i, value := range values {
		out[i] = makeChild(value)
	}
	return out
}

func (m *expandMachine) expandGeneric(w *expandTask, v *typ.Instantiated) {
	if v.Generic == nil || len(v.TypeArgs) != len(v.Generic.TypeParams) || v.Generic.Body == nil {
		w.result = w.orig
		m.state.memo[w.key] = w.result
		return
	}
	if active := m.state.matchingActive(v); active != nil {
		active.used = true
		w.result = active.mu
		m.state.memo[w.key] = w.result
		return
	}
	if w.mode == expandModeRoot && len(m.state.active) != 0 {
		w.result = w.orig
		m.state.memo[w.key] = w.result
		return
	}
	if m.state.hasActiveGeneric(v.Generic) {
		w.result = w.orig
		m.state.memo[w.key] = w.result
		return
	}
	active := &activeInstantiation{generic: v.Generic, args: append([]typ.Type(nil), v.TypeArgs...), mu: typ.NewRecursivePlaceholder(v.Generic.Name)}
	m.state.active = append(m.state.active, active)
	body := Params(v.Generic.Body, v.Generic.TypeParams, v.TypeArgs)
	body = Self(body, w.orig)
	bodyMode := expandModeTablePolicy
	if w.mode == expandModeRoot {
		bodyMode = expandModeRoot
	}
	w.active = active
	w.phase = expandGenericFinish
	m.pushChildren(w, []*expandTask{m.child(body, bodyMode)})
}

func (m *expandMachine) finish(w *expandTask) {
	v := unwrap.Annotations(w.orig)
	result := w.orig
	switch x := v.(type) {
	case *typ.Optional:
		if w.children[0].result != x.Inner {
			result = typeexpr.Optional(w.children[0].result)
		}
	case *typ.Union:
		if expandValuesChanged(x.Members, w.children) {
			result = typeexpr.Union(expandResults(w.children)...)
		}
	case *typ.Intersection:
		if expandValuesChanged(x.Members, w.children) {
			result = typeexpr.Intersection(expandResults(w.children)...)
		}
	case *typ.Array:
		if w.children[0].result != x.Element {
			result = typ.NewArray(w.children[0].result)
		}
	case *typ.Map:
		if w.mode == expandModeTablePolicy || w.children[0].result != x.Key || w.children[1].result != x.Value {
			result = typetable.NewMap(w.children[0].result, w.children[1].result)
		}
	case *typ.ReadonlyMap:
		if w.mode == expandModeTablePolicy || w.children[0].result != x.Key || w.children[1].result != x.Value {
			result = typetable.NewReadonlyMap(w.children[0].result, w.children[1].result)
		}
	case *typ.Tuple:
		if expandValuesChanged(x.Elements, w.children) {
			result = typ.NewTuple(expandResults(w.children)...)
		}
	case *typ.Function:
		result = finishExpandFunction(w, x)
	case *typ.Record:
		result = finishExpandRecord(w, x)
	case *typ.Alias:
		if w.children[0].result != x.Target {
			result = typ.NewAlias(x.Name, w.children[0].result)
		}
	case *typ.Interface:
		var methods []typ.Method
		changed := false
		for i, task := range w.children {
			if fn, ok := task.result.(*typ.Function); ok && fn != x.Methods[i].Type {
				if methods == nil {
					methods = make([]typ.Method, len(x.Methods))
					copy(methods, x.Methods)
				}
				methods[i].Type = fn
				changed = true
			}
		}
		if changed {
			result = typ.NewInterface(x.Name, methods)
		}
	}
	w.result = result
	m.state.memo[w.key] = result
}

func finishExpandFunction(w *expandTask, x *typ.Function) typ.Type {
	i, changed := 0, false
	var params []typ.Param
	for n := range x.Params {
		r := w.children[i].result
		i++
		if r != x.Params[n].Type {
			if params == nil {
				params = make([]typ.Param, len(x.Params))
				copy(params, x.Params)
			}
			params[n].Type = r
			changed = true
		}
	}
	var returns []typ.Type
	for n := range x.Returns {
		r := w.children[i].result
		i++
		if r != x.Returns[n] {
			if returns == nil {
				returns = make([]typ.Type, len(x.Returns))
				copy(returns, x.Returns)
			}
			returns[n] = r
			changed = true
		}
	}
	variadic := x.Variadic
	if variadic != nil {
		r := w.children[i].result
		if r != variadic {
			variadic = r
			changed = true
		}
	}
	if !changed {
		return w.orig
	}
	paramsSrc := x.Params
	if params != nil {
		paramsSrc = params
	}
	returnsSrc := x.Returns
	if returns != nil {
		returnsSrc = returns
	}
	return typ.RebuildFunction(typ.FunctionParts{TypeParams: x.TypeParams, Params: paramsSrc, Variadic: variadic, Returns: returnsSrc})
}

func finishExpandRecord(w *expandTask, x *typ.Record) typ.Type {
	i, changed := 0, false
	var fields []typ.Field
	for n := range x.Fields {
		r := w.children[i].result
		i++
		if r != x.Fields[n].Type {
			if fields == nil {
				fields = make([]typ.Field, len(x.Fields))
				copy(fields, x.Fields)
			}
			fields[n].Type = r
			changed = true
		}
	}
	var statics []typ.StaticMember
	for n := range x.StaticMembers {
		r := w.children[i].result
		i++
		if r != x.StaticMembers[n].Type {
			if statics == nil {
				statics = make([]typ.StaticMember, len(x.StaticMembers))
				copy(statics, x.StaticMembers)
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
	key, value := x.MapKey, x.MapValue
	if x.HasMapComponent() {
		r := w.children[i].result
		i++
		if r != key {
			key = r
			changed = true
		}
		r = w.children[i].result
		if r != value {
			value = r
			changed = true
		}
	}
	if !changed && w.mode != expandModeTablePolicy {
		return w.orig
	}
	fieldsSrc := x.Fields
	if fields != nil {
		fieldsSrc = fields
	}
	staticsSrc := x.StaticMembers
	if statics != nil {
		staticsSrc = statics
	}
	return typetable.RebuildRecord(typ.RecordParts{Fields: fieldsSrc, StaticMembers: staticsSrc, Metatable: meta, MapKey: key, MapValue: value, Open: x.Open, AssumeSorted: true})
}

func expandResults(tasks []*expandTask) []typ.Type {
	out := make([]typ.Type, len(tasks))
	for i, task := range tasks {
		out[i] = task.result
	}
	return out
}
func expandValuesChanged(values []typ.Type, tasks []*expandTask) bool {
	for i, value := range values {
		if tasks[i].result != value {
			return true
		}
	}
	return false
}

func isRecursiveInstantiated(t typ.Type) bool {
	inst, ok := t.(*typ.Instantiated)
	return ok && (typ.ContainsRecursive(inst) || genericBodySelfInstantiates(inst.Generic))
}

func genericBodySelfInstantiates(g *typ.Generic) bool {
	if g == nil || g.Body == nil {
		return false
	}
	found := false
	transform.Rewrite(g.Body, func(n typ.Type) (typ.Type, bool) {
		if found {
			return n, true
		}
		if inst, ok := n.(*typ.Instantiated); ok && inst.Generic == g {
			found = true
			return n, true
		}
		return nil, false
	})
	return found
}
