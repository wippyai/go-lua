// Package kind defines type kind enumeration for efficient type discrimination.
//
// Kind values enable fast type classification without reflection, supporting
// switch-based dispatch throughout the type system. Each type in the typ package
// returns its Kind via the Kind() method.
//
// # Categories
//
// Primitive kinds: Nil, Boolean, Number, Integer, String
// Represent base Lua types with value semantics.
//
// Top/Bottom types: Any (top), Unknown (unresolved), Never (bottom)
// Special types for type system boundaries and error handling.
//
// Composite kinds: Optional, Union, Intersection, Tuple, Function, Array, Map, Record
// Structural types built from other types.
//
// Nominal kinds: Interface, Alias, Generic, Instantiated
// Named types with identity-based semantics.
//
// Deferred kinds: Ref, TypeParam
// Types requiring resolution (forward references and generics).
//
// Other kinds: Literal, Self, Meta, Refined, Recursive
// Specialized types for literal values and advanced features.
package kind

// Kind identifies the structural category of a type for efficient dispatch.
// Returned by typ.Type.Kind() to enable type classification without casts.
type Kind int

const (
	Nil Kind = iota
	Boolean
	Number
	Integer
	String
	Any
	Unknown
	Never
	Optional
	Union
	Intersection
	Tuple
	Function
	Array
	Map
	Record
	_
	Interface
	Alias
	Generic
	Instantiated
	_
	Literal
	Self
	Ref
	Meta
	TypeParam
	_
	Refined
	_
	_
	Recursive
	ReadonlyMap
)

var kindNames = [...]string{
	Nil:          "nil",
	Boolean:      "boolean",
	Number:       "number",
	Integer:      "integer",
	String:       "string",
	Any:          "any",
	Unknown:      "unknown",
	Never:        "never",
	Optional:     "optional",
	Union:        "union",
	Intersection: "intersection",
	Tuple:        "tuple",
	Function:     "function",
	Array:        "array",
	Map:          "map",
	Record:       "record",
	Interface:    "interface",
	Alias:        "alias",
	Generic:      "generic",
	Instantiated: "instantiated",
	Literal:      "literal",
	Self:         "self",
	Ref:          "ref",
	Meta:         "meta",
	TypeParam:    "typeparam",
	Refined:      "refined",
	Recursive:    "recursive",
	ReadonlyMap:  "readonlymap",
}

func (k Kind) String() string {
	if int(k) >= 0 && int(k) < len(kindNames) {
		if name := kindNames[k]; name != "" {
			return name
		}
	}

	return "unknown"
}

// IsPlaceholder returns true for Any or Unknown kinds.
// These represent unresolved or open type positions.
func (k Kind) IsPlaceholder() bool {
	return k == Any || k == Unknown
}

// IsNever returns true for the Never kind (bottom type).
func (k Kind) IsNever() bool {
	return k == Never
}
