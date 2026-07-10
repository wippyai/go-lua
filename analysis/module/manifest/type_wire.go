package manifest

type typeWire struct {
	Kind string `json:"kind"`

	Base   string   `json:"base,omitempty"`
	Bool   *bool    `json:"bool,omitempty"`
	Int    *int64   `json:"int,omitempty"`
	Number *float64 `json:"number,omitempty"`
	String *string  `json:"string,omitempty"`

	Element  *typeWire   `json:"element,omitempty"`
	Key      *typeWire   `json:"key,omitempty"`
	Value    *typeWire   `json:"value,omitempty"`
	Elements []*typeWire `json:"elements,omitempty"`
	Members  []*typeWire `json:"members,omitempty"`

	Fields        []fieldWire        `json:"fields,omitempty"`
	StaticMembers []staticMemberWire `json:"staticMembers,omitempty"`
	Metatable     *typeWire          `json:"metatable,omitempty"`
	MapKey        *typeWire          `json:"mapKey,omitempty"`
	MapValue      *typeWire          `json:"mapValue,omitempty"`
	Open          bool               `json:"open,omitempty"`

	TypeParams []typeParamWire `json:"typeParams,omitempty"`
	Params     []paramWire     `json:"params,omitempty"`
	Variadic   *typeWire       `json:"variadic,omitempty"`
	Returns    []*typeWire     `json:"returns,omitempty"`

	Module string    `json:"module,omitempty"`
	Name   string    `json:"name,omitempty"`
	Target *typeWire `json:"target,omitempty"`
	Of     *typeWire `json:"of,omitempty"`

	Body     *typeWire   `json:"body,omitempty"`
	Generic  *typeWire   `json:"generic,omitempty"`
	TypeArgs []*typeWire `json:"typeArgs,omitempty"`

	Annotations []annotationWire `json:"annotations,omitempty"`
	Methods     []methodWire     `json:"methods,omitempty"`
}

type fieldWire struct {
	Name     string    `json:"name"`
	Type     *typeWire `json:"type,omitempty"`
	Optional bool      `json:"optional,omitempty"`
	Readonly bool      `json:"readonly,omitempty"`
}

type staticMemberWire struct {
	Kind     string    `json:"kind"`
	Name     string    `json:"name,omitempty"`
	Index    *int64    `json:"index,omitempty"`
	Type     *typeWire `json:"type,omitempty"`
	Optional bool      `json:"optional,omitempty"`
	Readonly bool      `json:"readonly,omitempty"`
}

type typeParamWire struct {
	Name       string    `json:"name"`
	Constraint *typeWire `json:"constraint,omitempty"`
}

type paramWire struct {
	Name     string    `json:"name,omitempty"`
	Type     *typeWire `json:"type,omitempty"`
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
	Type *typeWire `json:"type,omitempty"`
}
