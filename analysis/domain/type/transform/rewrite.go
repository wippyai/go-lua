package transform

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
)

// Rewrite traverses a type graph and applies fn at each node.  It is an
// iterative post-order walk: work items carry their result back to their
// parent, so product depth is bounded by heap storage rather than the Go call
// stack.  Memo entries preserve DAG sharing and recursive placeholders close
// finite μ graphs.
func Rewrite(t typ.Type, fn func(typ.Type) (typ.Type, bool)) typ.Type {
	if t == nil {
		return nil
	}
	memo := getRewriteMemo()
	defer putRewriteMemo(memo)
	m := rewriteMachine{fn: fn, memo: memo}
	root := m.newVisit(t, fn, memo)
	m.stack = append(m.stack, root)
	for len(m.stack) != 0 {
		i := len(m.stack) - 1
		work := m.stack[i]
		m.stack = m.stack[:i]
		m.step(work)
	}
	return root.result
}

const rewriteMemoMaxEntries = 4096

var rewriteMemoPool = sync.Pool{New: func() any { return make(map[rewriteKey]typ.Type, 64) }}

func getRewriteMemo() map[rewriteKey]typ.Type { return rewriteMemoPool.Get().(map[rewriteKey]typ.Type) }
func putRewriteMemo(m map[rewriteKey]typ.Type) {
	if len(m) > rewriteMemoMaxEntries {
		rewriteMemoPool.Put(make(map[rewriteKey]typ.Type, 64))
		return
	}
	clear(m)
	rewriteMemoPool.Put(m)
}

type rewriteKey struct{ t typ.Type }

type rewritePhase uint8

const (
	rewriteVisit rewritePhase = iota
	rewriteFinish
	rewriteFunctionBinders
	rewriteFunctionChildren
	rewriteGenericBinders
	rewriteGenericChildren
	rewriteRecursiveChildren
	rewriteBinder
	rewriteBinderFinish
)

type rewriteTask struct {
	phase rewritePhase
	t     typ.Type
	fn    func(typ.Type) (typ.Type, bool)
	memo  map[rewriteKey]typ.Type
	key   rewriteKey

	children []*rewriteTask
	binders  []*rewriteTask
	result   typ.Type
}

type rewriteMachine struct {
	fn    func(typ.Type) (typ.Type, bool)
	memo  map[rewriteKey]typ.Type
	stack []*rewriteTask
}

func (m *rewriteMachine) newVisit(t typ.Type, fn func(typ.Type) (typ.Type, bool), memo map[rewriteKey]typ.Type) *rewriteTask {
	return &rewriteTask{phase: rewriteVisit, t: t, fn: fn, memo: memo}
}

func (m *rewriteMachine) newBinder(p *typ.TypeParam, fn func(typ.Type) (typ.Type, bool), memo map[rewriteKey]typ.Type) *rewriteTask {
	return &rewriteTask{phase: rewriteBinder, t: p, fn: fn, memo: memo}
}

func (m *rewriteMachine) pushChildren(parent *rewriteTask, children []*rewriteTask) {
	parent.children = children
	m.stack = append(m.stack, parent)
	for i := len(children) - 1; i >= 0; i-- {
		m.stack = append(m.stack, children[i])
	}
}

func (m *rewriteMachine) step(w *rewriteTask) {
	switch w.phase {
	case rewriteVisit:
		m.visit(w)
	case rewriteFinish:
		m.finish(w)
	case rewriteFunctionBinders:
		m.functionBinders(w)
	case rewriteFunctionChildren:
		m.functionChildren(w)
	case rewriteGenericBinders:
		m.genericBinders(w)
	case rewriteGenericChildren:
		m.genericChildren(w)
	case rewriteRecursiveChildren:
		m.recursiveChildren(w)
	case rewriteBinder:
		m.binder(w)
	case rewriteBinderFinish:
		p := w.t.(*typ.TypeParam)
		constraint := w.children[0].result
		if constraint == p.Constraint {
			w.result = p
		} else {
			w.result = typ.NewTypeParam(p.Name, constraint)
		}
	}
}

func (m *rewriteMachine) visit(w *rewriteTask) {
	if w.t == nil {
		return
	}
	if !rewriteCanDescend(w.t) {
		if r, ok := w.fn(w.t); ok {
			w.result = r
		} else {
			w.result = w.t
		}
		return
	}
	w.key = rewriteKey{t: w.t}
	if cached, ok := w.memo[w.key]; ok {
		w.result = cached
		return
	}
	if r, ok := w.fn(w.t); ok {
		w.memo[w.key] = r
		w.result = r
		return
	}
	w.memo[w.key] = w.t
	v := typ.UnwrapTransparentWrappers(w.t)
	child := func(t typ.Type) *rewriteTask { return m.newVisit(t, w.fn, w.memo) }
	switch x := v.(type) {
	case *typ.Optional:
		if x.Inner == nil {
			w.result = w.t
			w.memo[w.key] = w.result
			return
		}
		w.phase = rewriteFinish
		m.pushChildren(w, []*rewriteTask{child(x.Inner)})
	case *typ.Union:
		w.phase = rewriteFinish
		m.pushChildren(w, rewriteChildren(x.Members, child))
	case *typ.Intersection:
		w.phase = rewriteFinish
		m.pushChildren(w, rewriteChildren(x.Members, child))
	case *typ.Array:
		w.phase = rewriteFinish
		m.pushChildren(w, []*rewriteTask{child(x.Element)})
	case *typ.Map:
		w.phase = rewriteFinish
		m.pushChildren(w, []*rewriteTask{child(x.Key), child(x.Value)})
	case *typ.ReadonlyMap:
		w.phase = rewriteFinish
		m.pushChildren(w, []*rewriteTask{child(x.Key), child(x.Value)})
	case *typ.Tuple:
		w.phase = rewriteFinish
		m.pushChildren(w, rewriteChildren(x.Elements, child))
	case *typ.Function:
		w.binders = make([]*rewriteTask, len(x.TypeParams))
		for i, p := range x.TypeParams {
			w.binders[i] = m.newBinder(p, w.fn, w.memo)
		}
		w.phase = rewriteFunctionBinders
		m.stack = append(m.stack, w)
		for i := len(w.binders) - 1; i >= 0; i-- {
			m.stack = append(m.stack, w.binders[i])
		}
	case *typ.Record:
		children := make([]*rewriteTask, 0, len(x.Fields)+len(x.StaticMembers)+3)
		for _, f := range x.Fields {
			children = append(children, child(f.Type))
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
		w.phase = rewriteFinish
		m.pushChildren(w, children)
	case *typ.Meta:
		w.phase = rewriteFinish
		m.pushChildren(w, []*rewriteTask{child(x.Of)})
	case *typ.TypeParam:
		w.phase = rewriteFinish
		m.pushChildren(w, []*rewriteTask{child(x.Constraint)})
	case *typ.Generic:
		w.binders = make([]*rewriteTask, len(x.TypeParams))
		for i, p := range x.TypeParams {
			w.binders[i] = m.newBinder(p, w.fn, w.memo)
		}
		w.phase = rewriteGenericBinders
		m.stack = append(m.stack, w)
		for i := len(w.binders) - 1; i >= 0; i-- {
			m.stack = append(m.stack, w.binders[i])
		}
	case *typ.Recursive:
		if x.Body == nil {
			w.result = w.t
			w.memo[w.key] = w.result
			return
		}
		replacement := typ.NewRecursivePlaceholder(x.Name)
		w.memo[w.key] = replacement
		selfAware := func(t typ.Type) (typ.Type, bool) {
			if typ.IsRecursiveRef(t, x) {
				return replacement, true
			}
			return w.fn(t)
		}
		w.phase = rewriteRecursiveChildren
		m.pushChildren(w, []*rewriteTask{m.newVisit(x.Body, selfAware, w.memo)})
	case *typ.Alias:
		w.phase = rewriteFinish
		m.pushChildren(w, []*rewriteTask{child(x.Target)})
	case *typ.Instantiated:
		w.phase = rewriteFinish
		m.pushChildren(w, rewriteChildren(x.TypeArgs, child))
	case *typ.Interface:
		children := make([]*rewriteTask, len(x.Methods))
		for i, method := range x.Methods {
			children[i] = child(method.Type)
		}
		w.phase = rewriteFinish
		m.pushChildren(w, children)
	default:
		w.result = w.t
		w.memo[w.key] = w.result
	}
}

func rewriteChildren(values []typ.Type, makeChild func(typ.Type) *rewriteTask) []*rewriteTask {
	children := make([]*rewriteTask, len(values))
	for i, value := range values {
		children[i] = makeChild(value)
	}
	return children
}

func (m *rewriteMachine) binder(w *rewriteTask) {
	p, _ := w.t.(*typ.TypeParam)
	if p == nil {
		return
	}
	if r, ok := w.fn(p); ok {
		if replacement, ok := r.(*typ.TypeParam); ok {
			w.result = replacement
			return
		}
	}
	w.phase = rewriteBinderFinish
	m.pushChildren(w, []*rewriteTask{m.newVisit(p.Constraint, w.fn, w.memo)})
}

func (m *rewriteMachine) functionBinders(w *rewriteTask) {
	v := typ.UnwrapTransparentWrappers(w.t).(*typ.Function)
	_, replacements, _ := binderResults(v.TypeParams, w.binders)
	childFn := rewriteFnWithTypeParamScope(w.fn, v.TypeParams, replacements)
	childMemo := rewriteMemoForTypeParamScope(w.memo, v.TypeParams, replacements)
	children := make([]*rewriteTask, 0, len(v.Params)+len(v.Returns)+1)
	for _, p := range v.Params {
		children = append(children, m.newVisit(p.Type, childFn, childMemo))
	}
	for _, r := range v.Returns {
		children = append(children, m.newVisit(r, childFn, childMemo))
	}
	if v.Variadic != nil {
		children = append(children, m.newVisit(v.Variadic, childFn, childMemo))
	}
	w.children = children
	w.result = nil
	w.phase = rewriteFunctionChildren
	m.pushChildren(w, children)
}

func (m *rewriteMachine) functionChildren(w *rewriteTask) {
	// The original node remains in w.t; binder results are read directly.
	v := typ.UnwrapTransparentWrappers(w.t).(*typ.Function)
	params, _, changed := binderResults(v.TypeParams, w.binders)
	i := 0
	var newParams []typ.Param
	for j := range v.Params {
		r := w.children[i].result
		i++
		if r != v.Params[j].Type {
			if newParams == nil {
				newParams = make([]typ.Param, len(v.Params))
				copy(newParams, v.Params)
			}
			newParams[j].Type = r
			changed = true
		}
	}
	var newReturns []typ.Type
	for j := range v.Returns {
		r := w.children[i].result
		i++
		if r != v.Returns[j] {
			if newReturns == nil {
				newReturns = make([]typ.Type, len(v.Returns))
				copy(newReturns, v.Returns)
			}
			newReturns[j] = r
			changed = true
		}
	}
	variadic := v.Variadic
	if v.Variadic != nil {
		r := w.children[i].result
		if r != variadic {
			variadic = r
			changed = true
		}
	}
	if !changed {
		w.result = w.t
	} else {
		paramsSrc := v.Params
		if newParams != nil {
			paramsSrc = newParams
		}
		returnsSrc := v.Returns
		if newReturns != nil {
			returnsSrc = newReturns
		}
		w.result = typ.RebuildFunction(typ.FunctionParts{TypeParams: params, Params: paramsSrc, Variadic: variadic, Returns: returnsSrc})
	}
	w.memo[w.key] = w.result
}

func (m *rewriteMachine) genericBinders(w *rewriteTask) {
	v := typ.UnwrapTransparentWrappers(w.t).(*typ.Generic)
	params, replacements, _ := binderResults(v.TypeParams, w.binders)
	childFn := rewriteFnWithTypeParamScope(w.fn, v.TypeParams, replacements)
	childMemo := rewriteMemoForTypeParamScope(w.memo, v.TypeParams, replacements)
	w.children = []*rewriteTask{m.newVisit(v.Body, childFn, childMemo)}
	w.phase = rewriteGenericChildren
	// params are recomputed in genericChildren; this keeps the work item compact.
	_ = params
	m.pushChildren(w, w.children)
}

func (m *rewriteMachine) genericChildren(w *rewriteTask) {
	v := typ.UnwrapTransparentWrappers(w.t).(*typ.Generic)
	params, _, changed := binderResults(v.TypeParams, w.binders)
	body := w.children[0].result
	if body != v.Body {
		changed = true
	}
	if !changed {
		w.result = w.t
	} else {
		w.result = typ.NewGeneric(v.Name, params, body)
	}
	w.memo[w.key] = w.result
}

func (m *rewriteMachine) recursiveChildren(w *rewriteTask) {
	v := typ.UnwrapTransparentWrappers(w.t).(*typ.Recursive)
	replacement := w.memo[w.key].(*typ.Recursive)
	body := w.children[0].result
	if body == v.Body {
		w.result = w.t
	} else {
		replacement.SetBody(body)
		w.result = replacement
	}
	w.memo[w.key] = w.result
}

func binderResults(original []*typ.TypeParam, binders []*rewriteTask) ([]*typ.TypeParam, map[*typ.TypeParam]*typ.TypeParam, bool) {
	var out []*typ.TypeParam
	var replacements map[*typ.TypeParam]*typ.TypeParam
	changed := false
	for i, p := range original {
		result, _ := binders[i].result.(*typ.TypeParam)
		if result == nil {
			result = p
		}
		if result == p {
			continue
		}
		if out == nil {
			out = make([]*typ.TypeParam, len(original))
			copy(out, original)
		}
		out[i] = result
		changed = true
		if replacements == nil {
			replacements = make(map[*typ.TypeParam]*typ.TypeParam)
		}
		replacements[p] = result
	}
	if out == nil {
		out = original
	}
	return out, replacements, changed
}

func (m *rewriteMachine) finish(w *rewriteTask) {
	v := typ.UnwrapTransparentWrappers(w.t)
	result := w.t
	switch x := v.(type) {
	case *typ.Optional:
		if w.children[0].result != x.Inner {
			result = typeexpr.Optional(w.children[0].result)
		}
	case *typ.Union:
		if valuesChanged(x.Members, w.children) {
			result = typeexpr.Union(taskResults(w.children)...)
		}
	case *typ.Intersection:
		if valuesChanged(x.Members, w.children) {
			result = typeexpr.Intersection(taskResults(w.children)...)
		}
	case *typ.Array:
		if w.children[0].result != x.Element {
			result = typ.NewArray(w.children[0].result)
		}
	case *typ.Map:
		if w.children[0].result != x.Key || w.children[1].result != x.Value {
			result = typetable.NewMap(w.children[0].result, w.children[1].result)
		}
	case *typ.ReadonlyMap:
		if w.children[0].result != x.Key || w.children[1].result != x.Value {
			result = typetable.NewReadonlyMap(w.children[0].result, w.children[1].result)
		}
	case *typ.Tuple:
		if valuesChanged(x.Elements, w.children) {
			result = typ.NewTuple(taskResults(w.children)...)
		}
	case *typ.Record:
		result = finishRecord(w, x)
	case *typ.Meta:
		if w.children[0].result != x.Of {
			result = typ.NewMeta(w.children[0].result)
		}
	case *typ.TypeParam:
		if w.children[0].result != x.Constraint {
			result = typ.NewTypeParam(x.Name, w.children[0].result)
		}
	case *typ.Alias:
		if w.children[0].result != x.Target {
			result = typ.NewAlias(x.Name, w.children[0].result)
		}
	case *typ.Instantiated:
		args := taskResults(w.children)
		changed := valuesChanged(x.TypeArgs, w.children)
		generic := x.Generic
		if replacement, ok := w.fn(generic); ok {
			if resolved, ok := replacement.(*typ.Generic); ok && resolved != nil {
				generic = resolved
				changed = true
			}
		}
		if changed {
			result = typ.Instantiate(generic, args...)
		}
	case *typ.Interface:
		var methods []typ.Method
		changed := false
		for i, c := range w.children {
			if f, ok := c.result.(*typ.Function); ok && f != x.Methods[i].Type {
				if methods == nil {
					methods = make([]typ.Method, len(x.Methods))
					copy(methods, x.Methods)
				}
				methods[i].Type = f
				changed = true
			}
		}
		if changed {
			result = typ.NewInterface(x.Name, methods)
		}
	}
	w.result = result
	w.memo[w.key] = result
}

func finishRecord(w *rewriteTask, x *typ.Record) typ.Type {
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
	if !changed {
		return w.t
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

func taskResults(tasks []*rewriteTask) []typ.Type {
	out := make([]typ.Type, len(tasks))
	for i, task := range tasks {
		out[i] = task.result
	}
	return out
}
func valuesChanged(values []typ.Type, tasks []*rewriteTask) bool {
	for i, value := range values {
		if tasks[i].result != value {
			return true
		}
	}
	return false
}

func rewriteCanDescend(t typ.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Optional, kind.Union, kind.Intersection, kind.Array, kind.Map, kind.ReadonlyMap, kind.Tuple, kind.Function, kind.Record, kind.Meta, kind.TypeParam, kind.Generic, kind.Recursive, kind.Alias, kind.Instantiated, kind.Interface:
		return true
	default:
		return false
	}
}

func rewriteFnWithTypeParamScope(fn func(typ.Type) (typ.Type, bool), binders []*typ.TypeParam, replacements map[*typ.TypeParam]*typ.TypeParam) func(typ.Type) (typ.Type, bool) {
	if len(replacements) == 0 && len(binders) == 0 {
		return fn
	}
	return func(t typ.Type) (typ.Type, bool) {
		if tp, ok := t.(*typ.TypeParam); ok {
			if replacement, ok := replacements[tp]; ok {
				return replacement, true
			}
			if typeParamShadowsBinder(tp, binders) {
				return nil, false
			}
		}
		return fn(t)
	}
}

func typeParamShadowsBinder(tp *typ.TypeParam, binders []*typ.TypeParam) bool {
	if tp == nil {
		return false
	}
	for _, binder := range binders {
		if binder != nil && (tp == binder || tp.Name == binder.Name || tp.Equals(binder)) {
			return true
		}
	}
	return false
}

func rewriteMemoForTypeParamScope(memo map[rewriteKey]typ.Type, binders []*typ.TypeParam, replacements map[*typ.TypeParam]*typ.TypeParam) map[rewriteKey]typ.Type {
	if len(binders) == 0 && len(replacements) == 0 {
		return memo
	}
	return make(map[rewriteKey]typ.Type)
}
