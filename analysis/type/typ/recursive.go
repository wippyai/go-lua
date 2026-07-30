package typ

import (
	"strings"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/type/kind"
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
	return recursiveStringRenderer{active: make(map[*Recursive]bool)}.render(r)
}

// recursiveStringRenderer renders a recursive graph as a finite μ expression.
// A backedge to an active binder is written as that binder's name rather than
// recursively expanding the graph again.
type recursiveStringRenderer struct {
	active map[*Recursive]bool
}

func (p recursiveStringRenderer) render(t Type) string {
	if t == nil {
		return "unknown"
	}
	switch t := t.(type) {
	case *Recursive:
		if t == nil {
			return "unknown"
		}
		if p.active[t] {
			return t.Name
		}
		p.active[t] = true
		defer delete(p.active, t)
		if t.Body == nil {
			return "μ" + t.Name
		}
		return "μ" + t.Name + ". " + p.render(t.Body)
	case *Optional:
		return p.render(t.Inner) + "?"
	case *Union:
		return p.join(t.Members, " | ", "nil")
	case *Intersection:
		return p.join(t.Members, " & ", "unknown")
	case *Array:
		return p.render(t.Element) + "[]"
	case *Map:
		return "{[" + p.render(t.Key) + "]: " + p.render(t.Value) + "}"
	case *ReadonlyMap:
		return "readonly {[" + p.render(t.Key) + "]: " + p.render(t.Value) + "}"
	case *Tuple:
		return "(" + p.join(t.Elements, ", ", "unknown") + ")"
	case *Record:
		return p.record(t)
	case *Function:
		return p.function(t)
	case *Interface:
		if t.Name != "" {
			return t.Name
		}
		parts := make([]string, len(t.Methods))
		for i, method := range t.Methods {
			parts[i] = method.Name + ": " + p.function(method.Type)
		}
		return "interface { " + strings.Join(parts, "; ") + " }"
	case *Meta:
		return "typeof(" + p.render(t.Of) + ")"
	case *Instantiated:
		return t.Generic.Name + "<" + p.join(t.TypeArgs, ", ", "unknown") + ">"
	case *TypeParam:
		if t.Constraint != nil {
			return t.Name + " : " + p.render(t.Constraint)
		}
		return t.Name
	default:
		return t.String()
	}
}

func (p recursiveStringRenderer) join(types []Type, sep, nilLabel string) string {
	parts := make([]string, len(types))
	for i, t := range types {
		if t == nil {
			parts[i] = nilLabel
			continue
		}
		parts[i] = p.render(t)
	}
	return strings.Join(parts, sep)
}

func (p recursiveStringRenderer) record(record *Record) string {
	var sb strings.Builder
	sb.WriteString("{")
	writeSeparator := func(written *bool) {
		if *written {
			sb.WriteString(", ")
		}
		*written = true
	}
	written := false
	for _, field := range record.Fields {
		writeSeparator(&written)
		if field.Readonly {
			sb.WriteString("readonly ")
		}
		sb.WriteString(field.Name)
		if field.Optional {
			sb.WriteString("?")
		}
		sb.WriteString(": ")
		sb.WriteString(p.render(field.Type))
	}
	for _, member := range record.StaticMembers {
		writeSeparator(&written)
		if member.Readonly {
			sb.WriteString("readonly ")
		}
		WriteStaticMemberKey(&sb, member)
		if member.Optional {
			sb.WriteString("?")
		}
		sb.WriteString(": ")
		sb.WriteString(p.render(member.Type))
	}
	if record.HasMapComponent() {
		writeSeparator(&written)
		sb.WriteString("[")
		sb.WriteString(p.render(record.MapKey))
		sb.WriteString("]: ")
		sb.WriteString(p.render(record.MapValue))
	}
	if record.Open {
		writeSeparator(&written)
		sb.WriteString("...")
	}
	sb.WriteString("}")
	return sb.String()
}

func (p recursiveStringRenderer) function(fn *Function) string {
	if fn == nil {
		return "unknown"
	}
	var sb strings.Builder
	sb.WriteString("fun")
	if len(fn.TypeParams) > 0 {
		params := make([]string, len(fn.TypeParams))
		for i, param := range fn.TypeParams {
			params[i] = p.render(param)
		}
		sb.WriteString("<")
		sb.WriteString(strings.Join(params, ", "))
		sb.WriteString(">")
	}
	params := make([]string, 0, len(fn.Params)+1)
	for _, param := range fn.Params {
		value := p.render(param.Type)
		if param.Name != "" {
			value = param.Name + ": " + value
		}
		if param.Optional {
			value += "?"
		}
		params = append(params, value)
	}
	if fn.Variadic != nil {
		params = append(params, "..."+p.render(fn.Variadic))
	}
	sb.WriteString("(")
	sb.WriteString(strings.Join(params, ", "))
	sb.WriteString(")")
	if len(fn.Returns) == 1 {
		sb.WriteString(" -> ")
		sb.WriteString(p.render(fn.Returns[0]))
	} else if len(fn.Returns) > 1 {
		sb.WriteString(" -> (")
		sb.WriteString(p.join(fn.Returns, ", ", "unknown"))
		sb.WriteString(")")
	}
	return sb.String()
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
