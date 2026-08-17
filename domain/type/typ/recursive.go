package typ

import (
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/wippyai/go-lua/domain/type/annotation"
	"github.com/wippyai/go-lua/domain/type/kind"
)

// recursiveIDCounter generates unique IDs for recursive types.
var recursiveIDCounter uint64

// Recursive represents a self-referential (mu) type.
// Recursive types are identified by a unique ID to allow cycle detection
// during equality comparison and hashing without infinite recursion.
//
// Example: type Node = { next: Node? } is represented as:
//
//	Recursive{ID: 1, Name: "Node", Body: Record{Fields: [{name: "next", type: <self-ref>}]}}
type Recursive struct {
	ID   uint64
	Name string
	Body Type
	rev  uint64

	// Recursive nodes are shared across concurrent solves after construction.
	// Derived values are therefore published as immutable memo records rather
	// than by mutating individual flag/hash fields. Duplicate first-use
	// computation is harmless; readers always observe either no memo or one
	// complete memo.
	containsMemo atomic.Pointer[recursiveContainsMemo]
	closedMemo   atomic.Pointer[recursiveClosedMemo]
	hashMemo     atomic.Pointer[recursiveHashMemo]
}

// RecursiveBuilder is used during construction to provide a self-reference.
type RecursiveBuilder func(self Type) Type

// NewRecursive creates a new recursive type.
// The builder function receives a placeholder that represents self-references
// and should return the body type using that placeholder where needed.
func NewRecursive(name string, builder RecursiveBuilder) *Recursive {
	id := atomic.AddUint64(&recursiveIDCounter, 1)

	rec := &Recursive{
		ID:   id,
		Name: name,
	}

	rec.SetBody(builder(rec))
	return rec
}

// NewRecursivePlaceholder creates an empty recursive type for deferred body assignment.
// Use SetBody to assign the body after creation. This is useful for mutual recursion.
func NewRecursivePlaceholder(name string) *Recursive {
	id := atomic.AddUint64(&recursiveIDCounter, 1)
	return &Recursive{
		ID:   id,
		Name: name,
	}
}

// SetBody assigns the body to a placeholder recursive type.
func (r *Recursive) SetBody(body Type) {
	r.Body = body
	r.rev++
	r.containsMemo.Store(nil)
	r.closedMemo.Store(nil)
	r.hashMemo.Store(nil)
}

func (r *Recursive) Kind() kind.Kind { return kind.Recursive }

func (r *Recursive) String() string {
	return renderTypeString(r)
}

// renderTypeString is the one public product renderer. Every composite Type
// String method enters here so child presentation never falls back to a
// recursive per-kind String implementation.
func renderTypeString(root Type) string {
	renderer := recursiveStringRenderer{active: make(map[*Recursive]string)}
	return renderer.render(root)
}

// recursiveStringRenderer is the canonical finite product renderer. Its
// explicit frame stack keeps deeply nested graphs off the Go call stack and
// writes every byte into one builder rather than concatenating child strings.
type recursiveStringRenderer struct {
	// active maps an open binder to the name its occurrences print. A binder
	// carries no name of its own once it comes back from the canonical codec,
	// so the renderer names it by its nesting depth and every occurrence under
	// it reads back the same name.
	active map[*Recursive]string
	stack  []recursiveStringFrame
	out    strings.Builder
}

type recursiveStringFrameKind uint8

const (
	renderTypeFrame recursiveStringFrameKind = iota
	writeTextFrame
	leaveRecursiveFrame
	writeTypeParamFrame
	writeAnnotationFrame
	writeRecordFieldFrame
	writeStaticMemberFrame
	writeRecordMapStartFrame
	writeFunctionParamFrame
	writeInterfaceMethodFrame
	writeNameAngleFrame
)

type recursiveStringFrame struct {
	kind       recursiveStringFrameKind
	typ        Type
	root       bool
	text       string
	recursive  *Recursive
	typeParam  *TypeParam
	annotated  *Annotated
	annotation int
	field      Field
	member     StaticMember
	separator  bool
	param      Param
	method     Method
}

func (p *recursiveStringRenderer) render(root Type) string {
	p.push(recursiveStringFrame{kind: renderTypeFrame, typ: root, root: true})
	for len(p.stack) > 0 {
		frame := p.stack[len(p.stack)-1]
		p.stack = p.stack[:len(p.stack)-1]
		p.renderFrame(frame)
	}
	return p.out.String()
}

func (p *recursiveStringRenderer) push(frame recursiveStringFrame) { p.stack = append(p.stack, frame) }
func (p *recursiveStringRenderer) pushText(text string) {
	p.push(recursiveStringFrame{kind: writeTextFrame, text: text})
}
func (p *recursiveStringRenderer) pushType(t Type) {
	p.push(recursiveStringFrame{kind: renderTypeFrame, typ: t})
}

func (p *recursiveStringRenderer) pushJoined(types []Type, separator, nilLabel string) {
	for i := len(types) - 1; i >= 0; i-- {
		if types[i] == nil {
			p.pushText(nilLabel)
		} else {
			p.pushType(types[i])
		}
		if i > 0 {
			p.pushText(separator)
		}
	}
}

func (p *recursiveStringRenderer) renderFrame(frame recursiveStringFrame) {
	switch frame.kind {
	case writeTextFrame:
		p.out.WriteString(frame.text)
	case leaveRecursiveFrame:
		delete(p.active, frame.recursive)
	case writeTypeParamFrame:
		p.out.WriteString(frame.typeParam.Name)
		if frame.typeParam.Constraint != nil {
			p.out.WriteString(" : ")
			p.pushType(frame.typeParam.Constraint)
		}
	case writeAnnotationFrame:
		p.writeAnnotation(frame.annotated.Annotations[frame.annotation])
	case writeRecordFieldFrame:
		if frame.separator {
			p.out.WriteString(", ")
		}
		if frame.field.Readonly {
			p.out.WriteString("readonly ")
		}
		p.out.WriteString(frame.field.Name)
		if frame.field.Optional {
			p.out.WriteString("?")
		}
		p.out.WriteString(": ")
	case writeStaticMemberFrame:
		if frame.separator {
			p.out.WriteString(", ")
		}
		if frame.member.Readonly {
			p.out.WriteString("readonly ")
		}
		WriteStaticMemberKey(&p.out, frame.member)
		if frame.member.Optional {
			p.out.WriteString("?")
		}
		p.out.WriteString(": ")
	case writeRecordMapStartFrame:
		if frame.separator {
			p.out.WriteString(", ")
		}
		p.out.WriteString("[")
	case writeFunctionParamFrame:
		if frame.separator {
			p.out.WriteString(", ")
		}
		if frame.param.Name != "" {
			p.out.WriteString(frame.param.Name)
			p.out.WriteString(": ")
		}
	case writeInterfaceMethodFrame:
		if frame.separator {
			p.out.WriteString("; ")
		}
		p.out.WriteString(frame.method.Name)
		p.out.WriteString(": ")
	case writeNameAngleFrame:
		p.out.WriteString(frame.text)
		p.out.WriteString("<")
	case renderTypeFrame:
		p.renderType(frame.typ, frame.root)
	}
}

func (p *recursiveStringRenderer) renderType(t Type, root bool) {
	if t == nil {
		p.out.WriteString("unknown")
		return
	}
	switch t := t.(type) {
	case *Recursive:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		if name, open := p.active[t]; open {
			p.out.WriteString(name)
			return
		}
		name := t.Name
		if name == "" {
			name = "_" + strconv.Itoa(len(p.active))
		}
		p.active[t] = name
		p.out.WriteString("μ")
		p.out.WriteString(name)
		if t.Body == nil {
			delete(p.active, t)
			return
		}
		p.push(recursiveStringFrame{kind: leaveRecursiveFrame, recursive: t})
		p.pushType(t.Body)
		p.out.WriteString(". ")
	case *Optional:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		// Optional.String historically spelled an invalid root Optional as
		// "nil?", while the recursive renderer spelled an invalid child as
		// "unknown?". Keep both public spellings while using one traversal.
		if root && t.Inner == nil {
			p.out.WriteString("nil?")
			return
		}
		p.pushText("?")
		p.pushType(t.Inner)
	case *Union:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		p.pushJoined(t.Members, " | ", "nil")
	case *Intersection:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		p.pushJoined(t.Members, " & ", "unknown")
	case *Array:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		p.pushText("[]")
		p.pushType(t.Element)
	case *Map:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		p.pushText("}")
		p.pushType(t.Value)
		p.pushText("]: ")
		p.pushType(t.Key)
		p.pushText("{[")
	case *ReadonlyMap:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		p.pushText("}")
		p.pushType(t.Value)
		p.pushText("]: ")
		p.pushType(t.Key)
		p.pushText("readonly {[")
	case *Tuple:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		p.pushText(")")
		p.pushJoined(t.Elements, ", ", "unknown")
		p.pushText("(")
	case *Record:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		p.pushRecord(t)
	case *Function:
		p.pushFunction(t)
	case *Interface:
		p.pushInterface(t)
	case *Meta:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		p.pushText(")")
		p.pushType(t.Of)
		p.pushText("typeof(")
	case *Instantiated:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		p.pushText(">")
		p.pushJoined(t.TypeArgs, ", ", "unknown")
		p.push(recursiveStringFrame{kind: writeNameAngleFrame, text: t.Generic.Name})
	case *Generic:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		p.pushText(">")
		p.pushTypeParams(t.TypeParams)
		p.push(recursiveStringFrame{kind: writeNameAngleFrame, text: t.Name})
	case *TypeParam:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		p.push(recursiveStringFrame{kind: writeTypeParamFrame, typeParam: t})
	case *Annotated:
		if t == nil {
			p.out.WriteString("unknown")
			return
		}
		for i := len(t.Annotations) - 1; i >= 0; i-- {
			p.push(recursiveStringFrame{kind: writeAnnotationFrame, annotated: t, annotation: i})
		}
		p.pushType(t.Inner)
	default:
		p.out.WriteString(t.String())
	}
}

func (p *recursiveStringRenderer) pushTypeParams(params []*TypeParam) {
	for i := len(params) - 1; i >= 0; i-- {
		p.push(recursiveStringFrame{kind: writeTypeParamFrame, typeParam: params[i]})
		if i > 0 {
			p.pushText(", ")
		}
	}
}

func (p *recursiveStringRenderer) pushRecord(record *Record) {
	p.pushText("}")
	fields, members, hasMap := len(record.Fields), len(record.StaticMembers), record.HasMapComponent()
	if record.Open {
		if fields+members > 0 || hasMap {
			p.pushText(", ...")
		} else {
			p.pushText("...")
		}
	}
	if hasMap {
		p.pushType(record.MapValue)
		p.pushText("]: ")
		p.pushType(record.MapKey)
		p.push(recursiveStringFrame{kind: writeRecordMapStartFrame, separator: fields+members > 0})
	}
	for i := members - 1; i >= 0; i-- {
		member := record.StaticMembers[i]
		p.pushType(member.Type)
		p.push(recursiveStringFrame{kind: writeStaticMemberFrame, member: member, separator: fields+i > 0})
	}
	for i := fields - 1; i >= 0; i-- {
		field := record.Fields[i]
		p.pushType(field.Type)
		p.push(recursiveStringFrame{kind: writeRecordFieldFrame, field: field, separator: i > 0})
	}
	p.pushText("{")
}

func (p *recursiveStringRenderer) pushFunction(fn *Function) {
	if fn == nil {
		p.out.WriteString("unknown")
		return
	}
	if len(fn.Returns) == 1 {
		p.pushType(fn.Returns[0])
		p.pushText(" -> ")
	} else if len(fn.Returns) > 1 {
		p.pushText(")")
		p.pushJoined(fn.Returns, ", ", "unknown")
		p.pushText(" -> (")
	}
	p.pushText(")")
	if fn.Variadic != nil {
		p.pushType(fn.Variadic)
		p.pushText("...")
		if len(fn.Params) > 0 {
			p.pushText(", ")
		}
	}
	for i := len(fn.Params) - 1; i >= 0; i-- {
		param := fn.Params[i]
		if param.Optional {
			p.pushText("?")
		}
		p.pushType(param.Type)
		p.push(recursiveStringFrame{kind: writeFunctionParamFrame, param: param, separator: i > 0})
	}
	p.pushText("(")
	if len(fn.TypeParams) > 0 {
		p.pushText(">")
		p.pushTypeParams(fn.TypeParams)
		p.pushText("<")
	}
	p.pushText("fun")
}

func (p *recursiveStringRenderer) pushInterface(iface *Interface) {
	if iface == nil {
		p.out.WriteString("unknown")
		return
	}
	if iface.Name != "" {
		p.out.WriteString(iface.Name)
		return
	}
	p.pushText(" }")
	for i := len(iface.Methods) - 1; i >= 0; i-- {
		method := iface.Methods[i]
		p.pushType(method.Type)
		p.push(recursiveStringFrame{kind: writeInterfaceMethodFrame, method: method, separator: i > 0})
	}
	p.pushText("interface { ")
}

func (p *recursiveStringRenderer) writeAnnotation(ann annotation.Annotation) {
	p.out.WriteString(" @")
	p.out.WriteString(ann.Name)
	if ann.Arg.IsNone() {
		return
	}
	p.out.WriteString("(")
	if value, ok := ann.Arg.AsString(); ok {
		p.out.WriteString("\"")
		p.out.WriteString(value)
		p.out.WriteString("\"")
	} else if value, ok := ann.Arg.AsFloat64(); ok {
		p.out.WriteString(formatFloat(value))
	} else if value, ok := ann.Arg.AsInt64(); ok {
		p.out.WriteString(strconv.FormatInt(value, 10))
	} else if value, ok := ann.Arg.AsInt(); ok {
		p.out.WriteString(strconv.FormatInt(int64(value), 10))
	} else if value, ok := ann.Arg.AsBool(); ok {
		p.out.WriteString(strconv.FormatBool(value))
	} else {
		p.out.WriteString("...")
	}
	p.out.WriteString(")")
}

// Equals compares two recursive types by their structural identity.
// Two recursive types are equal if they have the same structure when
// the self-references are treated as equivalent.
func (r *Recursive) Equals(other Type) bool {
	return typeEquals(r, other)
}

// IsRecursiveRef returns true if t is a reference to the given recursive type.
func IsRecursiveRef(t Type, rec *Recursive) bool {
	if t == rec {
		return true
	}
	if r, ok := t.(*Recursive); ok {
		return r.ID == rec.ID
	}
	return false
}
