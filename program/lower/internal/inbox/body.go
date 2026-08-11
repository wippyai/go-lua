package inbox

import (
	"fmt"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/lower/internal/phase"
	"github.com/wippyai/go-lua/program/source"
)

type StatementSequence struct {
	Statements []ast.Stmt
	Index      int
	Body       keyspace.Term
	Span       source.Span
}

type BodyPreparation struct {
	Statements []ast.Stmt
	Body       keyspace.Term
	Span       source.Span
}

type BodyClosure struct {
	Body keyspace.Term
	Span source.Span
}

// Bodies owns three distinct crossings. Separate queues and tokens keep their
// meanings out of mode flags and generic payloads.
type Bodies struct {
	phases       *phase.Stack
	statements   []StatementSequence
	preparations []BodyPreparation
	closures     []BodyClosure
}

func NewBodies(phases *phase.Stack) *Bodies {
	return &Bodies{phases: phases}
}

func (q *Bodies) PushStatements(statements []ast.Stmt, index int, body keyspace.Term, span source.Span) error {
	if q == nil || q.phases == nil || index < 0 || index > len(statements) || body == 0 || span.File == "" {
		return fmt.Errorf("programlower: invalid pending statement sequence")
	}
	q.statements = append(q.statements, StatementSequence{
		Statements: statements, Index: index, Body: body, Span: span,
	})
	q.phases.Push(phase.SyntaxStatements)
	return nil
}

func (q *Bodies) PopStatements() (StatementSequence, error) {
	if q == nil || len(q.statements) == 0 {
		return StatementSequence{}, fmt.Errorf("programlower: statement token has no payload")
	}
	last := len(q.statements) - 1
	request := q.statements[last]
	q.statements = q.statements[:last]
	return request, nil
}

func (q *Bodies) PushPrepare(statements []ast.Stmt, body keyspace.Term, span source.Span) error {
	if q == nil || q.phases == nil || body == 0 || span.File == "" {
		return fmt.Errorf("programlower: invalid pending Body preparation")
	}
	q.preparations = append(q.preparations, BodyPreparation{Statements: statements, Body: body, Span: span})
	q.phases.Push(phase.BodyPrepare)
	return nil
}

func (q *Bodies) PopPrepare() (BodyPreparation, error) {
	if q == nil || len(q.preparations) == 0 {
		return BodyPreparation{}, fmt.Errorf("programlower: Body preparation token has no payload")
	}
	last := len(q.preparations) - 1
	request := q.preparations[last]
	q.preparations = q.preparations[:last]
	return request, nil
}

func (q *Bodies) PushClose(body keyspace.Term, span source.Span) error {
	if q == nil || q.phases == nil || body == 0 || span.File == "" {
		return fmt.Errorf("programlower: invalid pending Body closure")
	}
	q.closures = append(q.closures, BodyClosure{Body: body, Span: span})
	q.phases.Push(phase.BodyClose)
	return nil
}

func (q *Bodies) PopClose() (BodyClosure, error) {
	if q == nil || len(q.closures) == 0 {
		return BodyClosure{}, fmt.Errorf("programlower: Body closure token has no payload")
	}
	last := len(q.closures) - 1
	request := q.closures[last]
	q.closures = q.closures[:last]
	return request, nil
}

func (q *Bodies) Clean() bool {
	return q != nil && len(q.statements) == 0 && len(q.preparations) == 0 && len(q.closures) == 0
}
