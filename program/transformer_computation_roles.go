package program

import (
	"github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

// Computation proofs are owner-fenced Program views. They expose only the
// semantic IDs and exact sites consumed by ProgramArtifact; authored Terms
// remain private to this package.
func (input TransformerInput) computationSpan(term keyspace.Term) (Span, Body, bool) {
	span, spanOK := input.Span(term)
	body, bodyOK := input.ContainingBody(term)
	return span, body, spanOK && bodyOK && input.OwnsSpan(span) && input.OwnsBody(body)
}

type UnaryOccurrence struct {
	input             TransformerInput
	span, operandSpan Span
	body              Body
	op                flowkind.UnaryOp
}

func (input TransformerInput) UnaryOccurrenceAt(index int) (UnaryOccurrence, bool) {
	if !input.Available() || index < 0 {
		return UnaryOccurrence{}, false
	}
	terms := input.owner.Flow().Authored().Operators().Unaries()
	term, ok := terms.At(index)
	if !ok || !input.owner.Flow().Executable().Contains(term) {
		return UnaryOccurrence{}, false
	}
	_, op, operand, ok := terms.Get(term)
	if !ok {
		return UnaryOccurrence{}, false
	}
	span, body, ok := input.computationSpan(term)
	if !ok {
		return UnaryOccurrence{}, false
	}
	operandSpan, operandOK := input.Span(operand)
	row := UnaryOccurrence{input: input, span: span, operandSpan: operandSpan, body: body, op: op}
	return row, operandOK && input.OwnsSpan(operandSpan) && row.Available()
}
func (input TransformerInput) UnaryOccurrenceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Authored().Operators().Unaries().Count()
}
func (row UnaryOccurrence) Available() bool {
	return row.input.Available() && row.input.OwnsSpan(row.span) && row.input.OwnsSpan(row.operandSpan) && row.input.OwnsBody(row.body)
}
func (row UnaryOccurrence) ContextID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.span.ContextID()
}
func (row UnaryOccurrence) BodyPathID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body.PathID()
}
func (row UnaryOccurrence) OperandID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.operandSpan.ContextID()
}
func (row UnaryOccurrence) Entry() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Entry()
}
func (row UnaryOccurrence) Finish() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Finish()
}
func (row UnaryOccurrence) Op() flowkind.UnaryOp {
	if !row.Available() {
		return 0
	}
	return row.op
}
func (row UnaryOccurrence) Span() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.span, true
}
func (row UnaryOccurrence) OperandSpan() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.operandSpan, true
}

type SelectOccurrence struct {
	input                     TransformerInput
	span, leftSpan, rightSpan Span
	body                      Body
	op                        flowkind.SelectOp
}

// BinaryEqualityOccurrence is Program's owner-fenced projection of one
// executable primitive ==/~= computation.  The ordered operands come from
// Flow's already-sealed BinaryPrimitive row; optional comparison geometry is
// retained only when the value is also the exact condition of a two-arm
// Branch.  No authored Term escapes this receipt.
type BinaryEqualityOccurrence struct {
	input                     TransformerInput
	span, leftSpan, rightSpan Span
	body                      Body
	op                        flowkind.BinaryOp
	branch                    keyspace.ContentID
	whenTrue, whenFalse       Body
	invert, hasComparison     bool
}

// BinaryArithmeticOccurrence is Program's owner-fenced projection of one
// executable primitive arithmetic computation.  It retains the exact
// authored operator and ordered operand spans, but no authored Term or
// analysis-domain value.  This is reusable transformer geometry: every Link
// mount consumes the same immutable row.
type BinaryArithmeticOccurrence struct {
	input                     TransformerInput
	span, leftSpan, rightSpan Span
	body                      Body
	op                        flowkind.BinaryOp
}

// BinaryOrderOccurrence is Program's owner-fenced projection of one
// executable primitive </<=/>/>= computation. The operator and ordered
// operands remain exactly as authored; Flow's normalized branch comparison
// is a separate causal proof and is not needed to compute the Boolean value.
// No authored Term escapes this receipt.
type BinaryOrderOccurrence struct {
	input                     TransformerInput
	span, leftSpan, rightSpan Span
	body                      Body
	op                        flowkind.BinaryOp
}

func (input TransformerInput) BinaryArithmeticOccurrenceAt(index int) (BinaryArithmeticOccurrence, bool) {
	if !input.Available() || index < 0 {
		return BinaryArithmeticOccurrence{}, false
	}
	primitives := input.owner.Flow().BinaryPrimitives()
	arithmetic := primitives.Arithmetic()
	term, termOK := arithmetic.At(index)
	primitive, primitiveOK := primitives.Primitive(term)
	source, sourceOK := primitive.Source()
	operation, operationOK := primitive.Operation()
	if !termOK || !primitiveOK || !sourceOK || source != term || !operationOK || !binaryArithmeticOperator(operation.Op) {
		return BinaryArithmeticOccurrence{}, false
	}
	span, body, spanOK := input.computationSpan(term)
	left, leftOK := input.Span(operation.Left)
	right, rightOK := input.Span(operation.Right)
	row := BinaryArithmeticOccurrence{
		input: input, span: span, leftSpan: left, rightSpan: right, body: body, op: operation.Op,
	}
	if !spanOK || !leftOK || !rightOK || !input.OwnsSpan(left) || !input.OwnsSpan(right) {
		return BinaryArithmeticOccurrence{}, false
	}
	return row, row.Available()
}

func (input TransformerInput) BinaryArithmeticOccurrenceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().BinaryPrimitives().Arithmetic().Count()
}

func (row BinaryArithmeticOccurrence) Available() bool {
	return row.input.Available() && row.input.OwnsSpan(row.span) && row.input.OwnsSpan(row.leftSpan) &&
		row.input.OwnsSpan(row.rightSpan) && row.input.OwnsBody(row.body) && binaryArithmeticOperator(row.op)
}

func (row BinaryArithmeticOccurrence) ContextID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.span.ContextID()
}

func (row BinaryArithmeticOccurrence) BodyPathID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body.PathID()
}

func (row BinaryArithmeticOccurrence) LeftID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.leftSpan.ContextID()
}

func (row BinaryArithmeticOccurrence) RightID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.rightSpan.ContextID()
}

func (row BinaryArithmeticOccurrence) Op() flowkind.BinaryOp {
	if !row.Available() {
		return 0
	}
	return row.op
}

func (row BinaryArithmeticOccurrence) Entry() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Entry()
}

func (row BinaryArithmeticOccurrence) Finish() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Finish()
}

func (row BinaryArithmeticOccurrence) Span() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.span, true
}

func (row BinaryArithmeticOccurrence) LeftSpan() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.leftSpan, true
}

func (row BinaryArithmeticOccurrence) RightSpan() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.rightSpan, true
}

func binaryArithmeticOperator(op flowkind.BinaryOp) bool {
	return op >= flowkind.BinaryAdd && op <= flowkind.BinaryPow
}

func (input TransformerInput) BinaryOrderOccurrenceAt(index int) (BinaryOrderOccurrence, bool) {
	if !input.Available() || index < 0 {
		return BinaryOrderOccurrence{}, false
	}
	primitives := input.owner.Flow().BinaryPrimitives()
	order := primitives.Order()
	term, termOK := order.At(index)
	primitive, primitiveOK := primitives.Primitive(term)
	source, sourceOK := primitive.Source()
	operation, operationOK := primitive.Operation()
	if !termOK || !primitiveOK || !sourceOK || source != term || !operationOK || !binaryOrderOperator(operation.Op) {
		return BinaryOrderOccurrence{}, false
	}
	span, body, spanOK := input.computationSpan(term)
	left, leftOK := input.Span(operation.Left)
	right, rightOK := input.Span(operation.Right)
	row := BinaryOrderOccurrence{
		input: input, span: span, leftSpan: left, rightSpan: right, body: body, op: operation.Op,
	}
	if !spanOK || !leftOK || !rightOK || !input.OwnsSpan(left) || !input.OwnsSpan(right) {
		return BinaryOrderOccurrence{}, false
	}
	return row, row.Available()
}

func (input TransformerInput) BinaryOrderOccurrenceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().BinaryPrimitives().Order().Count()
}

func (row BinaryOrderOccurrence) Available() bool {
	return row.input.Available() && row.input.OwnsSpan(row.span) && row.input.OwnsSpan(row.leftSpan) &&
		row.input.OwnsSpan(row.rightSpan) && row.input.OwnsBody(row.body) && binaryOrderOperator(row.op)
}

func (row BinaryOrderOccurrence) ContextID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.span.ContextID()
}

func (row BinaryOrderOccurrence) BodyPathID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body.PathID()
}

func (row BinaryOrderOccurrence) LeftID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.leftSpan.ContextID()
}

func (row BinaryOrderOccurrence) RightID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.rightSpan.ContextID()
}

func (row BinaryOrderOccurrence) Op() flowkind.BinaryOp {
	if !row.Available() {
		return 0
	}
	return row.op
}

func (row BinaryOrderOccurrence) Entry() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Entry()
}

func (row BinaryOrderOccurrence) Finish() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Finish()
}

func (row BinaryOrderOccurrence) Span() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.span, true
}

func (row BinaryOrderOccurrence) LeftSpan() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.leftSpan, true
}

func (row BinaryOrderOccurrence) RightSpan() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.rightSpan, true
}

func binaryOrderOperator(op flowkind.BinaryOp) bool {
	return op >= flowkind.BinaryLess && op <= flowkind.BinaryGreaterEqual
}

func (input TransformerInput) BinaryEqualityOccurrenceAt(index int) (BinaryEqualityOccurrence, bool) {
	if !input.Available() || index < 0 {
		return BinaryEqualityOccurrence{}, false
	}
	primitives := input.owner.Flow().BinaryPrimitives()
	equality := primitives.Equality()
	term, termOK := equality.At(index)
	primitive, primitiveOK := primitives.Primitive(term)
	source, sourceOK := primitive.Source()
	operation, operationOK := primitive.Operation()
	if !termOK || !primitiveOK || !sourceOK || source != term || !operationOK ||
		(operation.Op != flowkind.BinaryEqual && operation.Op != flowkind.BinaryNotEqual) {
		return BinaryEqualityOccurrence{}, false
	}
	span, body, spanOK := input.computationSpan(term)
	left, leftOK := input.Span(operation.Left)
	right, rightOK := input.Span(operation.Right)
	row := BinaryEqualityOccurrence{
		input: input, span: span, leftSpan: left, rightSpan: right, body: body, op: operation.Op,
	}
	if !spanOK || !leftOK || !rightOK || !input.OwnsSpan(left) || !input.OwnsSpan(right) {
		return BinaryEqualityOccurrence{}, false
	}
	if comparison, comparisonOK := primitive.Comparison(); comparisonOK {
		if comparison.Left != operation.Left || comparison.Right != operation.Right || comparison.Invert != (operation.Op == flowkind.BinaryNotEqual) {
			return BinaryEqualityOccurrence{}, false
		}
		branch, branchOK := input.owner.Flow().SemanticTermPath(comparison.Branch)
		whenTrue, trueOK := input.ContainingBody(comparison.TrueBody)
		whenFalse, falseOK := input.ContainingBody(comparison.FalseBody)
		// Equality output semantics are complete without duplicating the
		// structural branch plane. Retain the optional branch/body projection
		// only when all three portable identities are available; Artifact's
		// existing guarded EnvironmentEdges remain the authority otherwise.
		if branchOK && branch.Available() && trueOK && falseOK && input.OwnsBody(whenTrue) && input.OwnsBody(whenFalse) {
			row.branch, row.whenTrue, row.whenFalse = branch, whenTrue, whenFalse
			row.invert, row.hasComparison = comparison.Invert, true
		}
	}
	return row, row.Available()
}

func (input TransformerInput) BinaryEqualityOccurrenceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().BinaryPrimitives().Equality().Count()
}

func (row BinaryEqualityOccurrence) Available() bool {
	if !row.input.Available() || !row.input.OwnsSpan(row.span) || !row.input.OwnsSpan(row.leftSpan) ||
		!row.input.OwnsSpan(row.rightSpan) || !row.input.OwnsBody(row.body) ||
		(row.op != flowkind.BinaryEqual && row.op != flowkind.BinaryNotEqual) {
		return false
	}
	if !row.hasComparison {
		return !row.branch.Available() && !row.whenTrue.Available() && !row.whenFalse.Available() && !row.invert
	}
	return row.branch.Available() && row.input.OwnsBody(row.whenTrue) && row.input.OwnsBody(row.whenFalse) &&
		row.whenTrue.PathID() != row.whenFalse.PathID() && row.invert == (row.op == flowkind.BinaryNotEqual)
}

func (row BinaryEqualityOccurrence) ContextID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.span.ContextID()
}

func (row BinaryEqualityOccurrence) BodyPathID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body.PathID()
}

func (row BinaryEqualityOccurrence) LeftID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.leftSpan.ContextID()
}

func (row BinaryEqualityOccurrence) RightID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.rightSpan.ContextID()
}

func (row BinaryEqualityOccurrence) Op() flowkind.BinaryOp {
	if !row.Available() {
		return 0
	}
	return row.op
}

func (row BinaryEqualityOccurrence) Entry() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Entry()
}

func (row BinaryEqualityOccurrence) Finish() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Finish()
}

func (row BinaryEqualityOccurrence) Span() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.span, true
}

func (row BinaryEqualityOccurrence) LeftSpan() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.leftSpan, true
}

func (row BinaryEqualityOccurrence) RightSpan() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.rightSpan, true
}

// Comparison returns the exact branch identity and two body identities only
// when Flow sealed this equality as a Branch condition.
func (row BinaryEqualityOccurrence) Comparison() (branch, whenTrue, whenFalse keyspace.ContentID, invert bool, ok bool) {
	if !row.Available() || !row.hasComparison {
		return keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, false, false
	}
	return row.branch, row.whenTrue.PathID(), row.whenFalse.PathID(), row.invert, true
}

func (input TransformerInput) SelectOccurrenceAt(index int) (SelectOccurrence, bool) {
	if !input.Available() || index < 0 {
		return SelectOccurrence{}, false
	}
	terms := input.owner.Flow().Authored().Operators().Selects()
	term, ok := terms.At(index)
	if !ok || !input.owner.Flow().Executable().Contains(term) {
		return SelectOccurrence{}, false
	}
	_, op, left, right, ok := terms.Get(term)
	if !ok {
		return SelectOccurrence{}, false
	}
	span, body, ok := input.computationSpan(term)
	if !ok {
		return SelectOccurrence{}, false
	}
	leftSpan, leftOK := input.Span(left)
	rightSpan, rightOK := input.Span(right)
	row := SelectOccurrence{input: input, span: span, leftSpan: leftSpan, rightSpan: rightSpan, body: body, op: op}
	return row, leftOK && rightOK && input.OwnsSpan(leftSpan) && input.OwnsSpan(rightSpan) && row.Available()
}
func (input TransformerInput) SelectOccurrenceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Authored().Operators().Selects().Count()
}
func (row SelectOccurrence) Available() bool {
	return row.input.Available() && row.input.OwnsSpan(row.span) && row.input.OwnsSpan(row.leftSpan) && row.input.OwnsSpan(row.rightSpan) && row.input.OwnsBody(row.body)
}
func (row SelectOccurrence) ContextID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.span.ContextID()
}
func (row SelectOccurrence) BodyPathID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body.PathID()
}
func (row SelectOccurrence) LeftID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.leftSpan.ContextID()
}
func (row SelectOccurrence) RightID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.rightSpan.ContextID()
}
func (row SelectOccurrence) Entry() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Entry()
}
func (row SelectOccurrence) Finish() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Finish()
}
func (row SelectOccurrence) Op() flowkind.SelectOp {
	if !row.Available() {
		return 0
	}
	return row.op
}
func (row SelectOccurrence) Span() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.span, true
}
func (row SelectOccurrence) LeftSpan() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.leftSpan, true
}
func (row SelectOccurrence) RightSpan() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.rightSpan, true
}

type ClaimOccurrence struct {
	input             TransformerInput
	span, operandSpan Span
	body              Body
	kind              flowkind.ValueClaimKind
}

func (input TransformerInput) ClaimOccurrenceAt(index int) (ClaimOccurrence, bool) {
	if !input.Available() || index < 0 {
		return ClaimOccurrence{}, false
	}
	terms := input.owner.Flow().Authored().Claims()
	term, ok := terms.At(index)
	if !ok || !input.owner.Flow().Executable().Contains(term) {
		return ClaimOccurrence{}, false
	}
	_, operand, kind, ok := terms.Get(term)
	if !ok {
		return ClaimOccurrence{}, false
	}
	span, body, ok := input.computationSpan(term)
	if !ok {
		return ClaimOccurrence{}, false
	}
	operandSpan, operandOK := input.Span(operand)
	row := ClaimOccurrence{input: input, span: span, operandSpan: operandSpan, body: body, kind: kind}
	return row, operandOK && input.OwnsSpan(operandSpan) && row.Available()
}
func (input TransformerInput) ClaimOccurrenceCount() int {
	if !input.Available() {
		return 0
	}
	return input.owner.Flow().Authored().Claims().Count()
}
func (row ClaimOccurrence) Available() bool {
	return row.input.Available() && row.input.OwnsSpan(row.span) && row.input.OwnsSpan(row.operandSpan) && row.input.OwnsBody(row.body)
}
func (row ClaimOccurrence) ContextID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.span.ContextID()
}
func (row ClaimOccurrence) BodyPathID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body.PathID()
}
func (row ClaimOccurrence) OperandID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.operandSpan.ContextID()
}
func (row ClaimOccurrence) Entry() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Entry()
}
func (row ClaimOccurrence) Finish() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Finish()
}
func (row ClaimOccurrence) Kind() flowkind.ValueClaimKind {
	if !row.Available() {
		return 0
	}
	return row.kind
}
func (row ClaimOccurrence) Span() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.span, true
}
func (row ClaimOccurrence) OperandSpan() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.operandSpan, true
}

type ReturnOccurrence struct {
	input            TransformerInput
	span, valuesSpan Span
	body             Body
}

func (input TransformerInput) ReturnOccurrenceAt(index int) (ReturnOccurrence, bool) {
	if !input.Available() || input.owner.returnCatalog == nil || !input.owner.returnCatalog.valid() || index < 0 || index >= len(input.owner.returnCatalog.rows) {
		return ReturnOccurrence{}, false
	}
	receipt := input.owner.returnCatalog
	stored := receipt.rows[index]
	row := ReturnOccurrence{input: input, span: stored.span, valuesSpan: stored.valuesSpan, body: stored.body}
	return row, row.Available()
}
func (input TransformerInput) ReturnOccurrenceCount() int {
	if !input.Available() || input.owner.returnCatalog == nil || !input.owner.returnCatalog.valid() {
		return 0
	}
	return len(input.owner.returnCatalog.rows)
}
func (row ReturnOccurrence) Available() bool {
	return row.input.Available() && row.input.OwnsSpan(row.span) && row.input.OwnsSpan(row.valuesSpan) && row.input.OwnsBody(row.body)
}
func (row ReturnOccurrence) ContextID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.span.ContextID()
}
func (row ReturnOccurrence) BodyPathID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.body.PathID()
}
func (row ReturnOccurrence) ValuesID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.valuesSpan.ContextID()
}
func (row ReturnOccurrence) Entry() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Entry()
}
func (row ReturnOccurrence) Finish() (flow.Site, bool) {
	if !row.Available() {
		return flow.Site{}, false
	}
	return row.span.Finish()
}
func (row ReturnOccurrence) Span() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.span, true
}
func (row ReturnOccurrence) ValuesSpan() (Span, bool) {
	if !row.Available() {
		return Span{}, false
	}
	return row.valuesSpan, true
}
