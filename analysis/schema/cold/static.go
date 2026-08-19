package cold

import "github.com/wippyai/go-lua/analysis/identity"

// The append-only slots these families occupy. They are derived from the last
// slot declared before them so a family added later cannot reuse a slot a
// consumer already addresses.
const (
	slotStaticTypeValue  = slotEnvironmentReset + 1
	slotStaticExpression = slotStaticTypeValue + 1
)

var (
	staticTypeValueFamily  = Family[StaticTypeValue]{slot: slotStaticTypeValue, name: "static-type-value"}
	staticExpressionFamily = Family[StaticExpression]{slot: slotStaticExpression, name: "static-expression"}
)

func StaticTypeValueFamily() Family[StaticTypeValue] { return staticTypeValueFamily }

func StaticExpressionFamily() Family[StaticExpression] { return staticExpressionFamily }

// StaticTypeValue is one authored type-value binding. The row is flat: every
// field is an identity the compiler issued while the Static proof was live,
// plus the authored name the binding is known by.
type StaticTypeValue struct {
	id        identity.ContentID
	body      identity.ContentID
	reference identity.ContentID
	root      identity.ContentID
	name      string
}

// NewStaticTypeValue copies one canonical StaticTypeValueRow.
func NewStaticTypeValue(id, body, reference, root identity.ContentID, name string) (StaticTypeValue, bool) {
	row := StaticTypeValue{id: id, body: body, reference: reference, root: root, name: name}
	return row, row.Available()
}

func (row StaticTypeValue) Available() bool {
	return row.id.Available() && row.body.Available() && row.reference.Available() && row.root.Available() && row.name != ""
}

func (row StaticTypeValue) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row StaticTypeValue) BodyPathID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.body
}

func (row StaticTypeValue) ReferenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.reference
}

func (row StaticTypeValue) RootID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.root
}

func (row StaticTypeValue) Name() string {
	if !row.Available() {
		return ""
	}
	return row.name
}

// StaticExpression is one authored type expression: the expression identity,
// the static node it references, and the owner that authored it.
type StaticExpression struct {
	id        identity.ContentID
	reference identity.ContentID
	owner     identity.ContentID
}

// NewStaticExpression copies one canonical StaticExpressionRow.
func NewStaticExpression(id, reference, owner identity.ContentID) (StaticExpression, bool) {
	row := StaticExpression{id: id, reference: reference, owner: owner}
	return row, row.Available()
}

func (row StaticExpression) Available() bool {
	return row.id.Available() && row.reference.Available() && row.owner.Available()
}

func (row StaticExpression) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row StaticExpression) ReferenceID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.reference
}

func (row StaticExpression) Owner() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.owner
}
