package factflow

type ExpressionOperationKind uint8

const (
	ExpressionOperationUnknown ExpressionOperationKind = iota
	ExpressionOperationUnary
	ExpressionOperationBinary
)

// ExpressionOperation describes a pure expression-level operation over
// already-lowered value sources. It keeps the engine independent from Lua AST.
type ExpressionOperation struct {
	kind  ExpressionOperationKind
	op    string
	left  ValueSource
	right ValueSource
}

func NewUnaryExpressionOperation(op string, operand ValueSource) (ExpressionOperation, bool) {
	if op == "" || !operand.Valid() {
		return ExpressionOperation{}, false
	}
	return ExpressionOperation{kind: ExpressionOperationUnary, op: op, left: operand}, true
}

func NewBinaryExpressionOperation(op string, left, right ValueSource) (ExpressionOperation, bool) {
	if op == "" || !left.Valid() || !right.Valid() {
		return ExpressionOperation{}, false
	}
	return ExpressionOperation{kind: ExpressionOperationBinary, op: op, left: left, right: right}, true
}

func (o ExpressionOperation) Kind() ExpressionOperationKind { return o.kind }
func (o ExpressionOperation) Op() string                    { return o.op }
func (o ExpressionOperation) Left() ValueSource             { return o.left }
func (o ExpressionOperation) Right() ValueSource            { return o.right }

func (o ExpressionOperation) copy() ExpressionOperation { return o }

func copyExpressionOperationMap(in map[ExprRef]ExpressionOperation) map[ExprRef]ExpressionOperation {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]ExpressionOperation, len(in))
	for ref, op := range in {
		if ref == 0 || op.kind == ExpressionOperationUnknown {
			continue
		}
		out[ref] = op.copy()
	}
	return out
}
