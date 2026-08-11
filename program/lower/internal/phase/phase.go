// Package phase owns the single continuation stack used while lowering one
// source file. Every semantic owner retains its own typed pending work; this
// package records only which owner runs next.
package phase

import "github.com/wippyai/go-lua/program/keyspace"

// Owner is the closed set of continuation owners.
type Owner uint8

const (
	Source Owner = iota + 1
	Lexical
	Eval
	Store
	Control
	Call
	Static
	Function
	Table

	SyntaxExpression
	SyntaxStatements
	BodyPrepare
	BodyClose
	StaticType
	StaticDeclaredCellType
)

// Stack is the sole source-lowering continuation and scalar-result state.
type Stack struct {
	owners []Owner
	result keyspace.Term
	open   bool
}

// Push schedules one continuation owner.
func (s *Stack) Push(owner Owner) {
	if owner == 0 {
		panic("programlower: zero phase owner")
	}
	s.owners = append(s.owners, owner)
}

// Pop returns the next continuation in LIFO order.
func (s *Stack) Pop() (Owner, bool) {
	if len(s.owners) == 0 {
		return 0, false
	}
	last := len(s.owners) - 1
	owner := s.owners[last]
	s.owners = s.owners[:last]
	return owner, true
}

// Result returns the completed expression identity and whether it may retain
// an unadjusted value tail in the immediately enclosing Values context.
func (s *Stack) Result() (keyspace.Term, bool) {
	return s.result, s.open
}

// SetResult publishes one completed expression for the next continuation.
// Only Call and Vararg syntax may publish open=true.
func (s *Stack) SetResult(term keyspace.Term, open bool) {
	s.result = term
	s.open = open
}

// Clean reports whether every continuation completed.
func (s *Stack) Clean() bool {
	return len(s.owners) == 0
}
