package wire

type effectRowWire struct {
	Labels []effectLabelWire `json:"labels,omitempty"`
	Tail   *string           `json:"tail,omitempty"`
}

type effectLabelWire struct {
	Kind string `json:"kind"`

	ReturnIndex *int  `json:"returnIndex,omitempty"`
	ValueIndex  *int  `json:"valueIndex,omitempty"`
	ErrorIndex  *int  `json:"errorIndex,omitempty"`
	Indices     []int `json:"indices,omitempty"`
	FromParam   *int  `json:"fromParam,omitempty"`
	Delta       *int  `json:"delta,omitempty"`

	Target *paramRefWire `json:"target,omitempty"`
	Source *paramRefWire `json:"source,omitempty"`
	Param  *paramRefWire `json:"param,omitempty"`
	Into   *paramRefWire `json:"into,omitempty"`
	Value  *paramRefWire `json:"value,omitempty"`

	IteratorKind string                `json:"iteratorKind,omitempty"`
	Transform    *effectTransformWire  `json:"transform,omitempty"`
	ReturnType   *effectReturnWire     `json:"returnType,omitempty"`
	Length       *exprWire             `json:"length,omitempty"`
	Refinement   *effectRefinementWire `json:"refinement,omitempty"`

	Protocol string   `json:"protocol,omitempty"`
	From     string   `json:"from,omitempty"`
	To       string   `json:"to,omitempty"`
	Final    string   `json:"final,omitempty"`
	Finals   []string `json:"finals,omitempty"`
}

type effectTransformWire struct {
	Kind      string        `json:"kind"`
	Source    *paramRefWire `json:"source,omitempty"`
	Container *paramRefWire `json:"container,omitempty"`
	Value     *paramRefWire `json:"value,omitempty"`
	Element   *paramRefWire `json:"element,omitempty"`
}

type effectRefinementWire struct {
	Kind string `json:"kind"`
}

type effectReturnWire struct {
	Kind          string               `json:"kind"`
	Source        *paramRefWire        `json:"source,omitempty"`
	Cases         *paramRefWire        `json:"cases,omitempty"`
	Default       *paramRefWire        `json:"default,omitempty"`
	CallbackParam *paramRefWire        `json:"callbackParam,omitempty"`
	Format        *paramRefWire        `json:"format,omitempty"`
	Projection    []projectionStepWire `json:"projection,omitempty"`
	When          *TypeWire            `json:"when,omitempty"`
	Then          *TypeWire            `json:"then,omitempty"`
}

type paramRefWire struct {
	Index *int `json:"index"`
}

type exprWire struct {
	Kind  string    `json:"kind"`
	Name  string    `json:"name,omitempty"`
	Value int64     `json:"value,omitempty"`
	Index *int      `json:"index,omitempty"`
	Op    string    `json:"op,omitempty"`
	Left  *exprWire `json:"left,omitempty"`
	Right *exprWire `json:"right,omitempty"`
}

type projectionStepWire struct {
	Kind  string    `json:"kind"`
	Field string    `json:"field,omitempty"`
	Index *int      `json:"index,omitempty"`
	Type  *TypeWire `json:"type,omitempty"`
}
