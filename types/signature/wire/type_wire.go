package wire

type TypeWire struct {
	Kind string `json:"kind"`

	Base   string   `json:"base,omitempty"`
	Bool   *bool    `json:"bool,omitempty"`
	Int    *int64   `json:"int,omitempty"`
	Number *float64 `json:"number,omitempty"`
	String *string  `json:"string,omitempty"`

	Element  *TypeWire   `json:"element,omitempty"`
	Key      *TypeWire   `json:"key,omitempty"`
	Value    *TypeWire   `json:"value,omitempty"`
	Elements []*TypeWire `json:"elements,omitempty"`
	Members  []*TypeWire `json:"members,omitempty"`

	Fields        []fieldWire        `json:"fields,omitempty"`
	StaticMembers []staticMemberWire `json:"staticMembers,omitempty"`
	Metatable     *TypeWire          `json:"metatable,omitempty"`
	MapKey        *TypeWire          `json:"mapKey,omitempty"`
	MapValue      *TypeWire          `json:"mapValue,omitempty"`
	Open          bool               `json:"open,omitempty"`

	TypeParams []typeParamWire `json:"typeParams,omitempty"`
	Params     []paramWire     `json:"params,omitempty"`
	Variadic   *TypeWire       `json:"variadic,omitempty"`
	Returns    []*TypeWire     `json:"returns,omitempty"`

	Module string    `json:"module,omitempty"`
	Name   string    `json:"name,omitempty"`
	Binder uint64    `json:"binder,omitempty"`
	Target *TypeWire `json:"target,omitempty"`
	Of     *TypeWire `json:"of,omitempty"`

	Body     *TypeWire   `json:"body,omitempty"`
	Generic  *TypeWire   `json:"generic,omitempty"`
	TypeArgs []*TypeWire `json:"typeArgs,omitempty"`

	Annotations []annotationWire `json:"annotations,omitempty"`
	Methods     []methodWire     `json:"methods,omitempty"`
}

type fieldWire struct {
	Name     string    `json:"name"`
	Type     *TypeWire `json:"type,omitempty"`
	Optional bool      `json:"optional,omitempty"`
	Readonly bool      `json:"readonly,omitempty"`
}

type staticMemberWire struct {
	Kind     string    `json:"kind"`
	Name     string    `json:"name,omitempty"`
	Index    *int64    `json:"index,omitempty"`
	Type     *TypeWire `json:"type,omitempty"`
	Optional bool      `json:"optional,omitempty"`
	Readonly bool      `json:"readonly,omitempty"`
}

type typeParamWire struct {
	Name       string    `json:"name"`
	Constraint *TypeWire `json:"constraint,omitempty"`
}

type paramWire struct {
	Name     string    `json:"name,omitempty"`
	Type     *TypeWire `json:"type,omitempty"`
	Optional bool      `json:"optional,omitempty"`
}

type annotationWire struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind,omitempty"`
	String  *string  `json:"string,omitempty"`
	Bool    *bool    `json:"bool,omitempty"`
	Int     *int     `json:"int,omitempty"`
	Int64   *int64   `json:"int64,omitempty"`
	Float64 *float64 `json:"float64,omitempty"`
}

type methodWire struct {
	Name string    `json:"name"`
	Type *TypeWire `json:"type,omitempty"`
}
