// Package static lowers parser-authored static type syntax into Program.
package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/coord"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/lexical"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Writer owns static declaration identity while one Program is unfinished.
// Declaration IDs, rather than source names, are the sole lexical reference
// authority; names are retained only as authored reference spelling.
type Writer struct {
	static     *assembly.Collector
	flow       *assembly.Collector
	binding    *bind.Result
	scopes     *lexical.Bodies
	sourceName string
	terms      map[bind.TypeDeclID]keyspace.Term
	// rootBody hosts the ambient namespace declarations. It is the chunk Body,
	// captured when that Body is prepared, because an ambient name is in scope
	// for the whole Program rather than for the Body that first names it.
	rootBody         keyspace.Term
	children         []keyspace.Term
	fields           []keyspace.Term
	interfaceMembers []assembly.StaticInterfaceMember
	params           []assembly.StaticParameter
	generics         []keyspace.Term

	// phases and steps are the static vertical's one iterative walk. They are
	// deliberately private: source selects only the next owner token and never
	// carries a static constructor's cursor, child range, or containment
	// judgment.
	phases      *continuation.Stack
	expressions *continuation.Expressions
	evaluations *eval.Values
	steps       []walkStep
	staticDepth int
}

// StaticDepth is positive precisely while an ordinary expression or Values
// continuation is an operand of authored static syntax. Function lowering
// uses this fact to validate the binder's static-containment evidence.
func (w *Writer) StaticDepth() int {
	if w == nil {
		return 0
	}
	return w.staticDepth
}

// Mark starts one LIFO ordered child range for an iterative type walk.
func (w *Writer) Mark() int {
	if w == nil {
		return -1
	}
	return len(w.children)
}

func (w *Writer) FieldMark() int {
	if w == nil {
		return -1
	}
	return len(w.fields)
}

// InterfaceMemberMark starts one LIFO range for an interface's exact authored
// member sequence. It is deliberately separate from record-field scratch: a
// record has only fields, while an interface's sequence contains both variants.
func (w *Writer) InterfaceMemberMark() int {
	if w == nil {
		return -1
	}
	return len(w.interfaceMembers)
}

// Append retains one completed static type child in source order.
func (w *Writer) Append(term keyspace.Term) error {
	if w == nil || term == 0 {
		return fmt.Errorf("lualower: invalid static type child")
	}
	w.children = append(w.children, term)
	return nil
}

func (w *Writer) AppendField(term keyspace.Term) error {
	if w == nil || term == 0 {
		return fmt.Errorf("lualower: invalid static field child")
	}
	w.fields = append(w.fields, term)
	return nil
}

func (w *Writer) AppendInterfaceField(term keyspace.Term) error {
	if w == nil || term == 0 {
		return fmt.Errorf("lualower: invalid interface field child")
	}
	w.interfaceMembers = append(w.interfaceMembers, assembly.StaticInterfaceMember{
		Kind: staticdecl.InterfaceField, Field: term,
	})
	return nil
}

func (w *Writer) AppendInterfaceMethod(name string, namePosition ast.Position, signature keyspace.Term) error {
	if w == nil || name == "" || signature == 0 {
		return fmt.Errorf("lualower: invalid interface method child")
	}
	w.interfaceMembers = append(w.interfaceMembers, assembly.StaticInterfaceMember{
		Kind:      staticdecl.InterfaceMethod,
		Name:      name,
		Span:      w.nameSpan(namePosition),
		Signature: signature,
	})
	return nil
}

// Take completes one scalar child hold.
func (w *Writer) Take(mark int) (keyspace.Term, error) {
	if w == nil || mark < 0 || mark != len(w.children)-1 {
		return 0, fmt.Errorf("lualower: incomplete static type child")
	}
	term := w.children[mark]
	w.children = w.children[:mark]
	return term, nil
}

// TakeCallTypeArgs releases one ordered static argument range for an already
// declared executable Call. Keeping this range here ensures static children
// never enter runtime Values assembly.
func (w *Writer) TakeCallTypeArgs(mark, count int) ([]keyspace.Term, error) {
	return w.rangeTerms(mark, count)
}

// Clean reports whether the iterative type-child scratch is empty.
func (w *Writer) Clean() bool {
	return w != nil && len(w.children) == 0 && len(w.fields) == 0 &&
		len(w.interfaceMembers) == 0 && len(w.params) == 0 && len(w.generics) == 0 &&
		len(w.steps) == 0 && w.staticDepth == 0
}

// New creates a static writer over the typed Collector Static and Flow
// capabilities. Flow is used only for executable function contracts and the
// binder-owned global Cell selector; all authored static rows use Static.
func New(
	phases *continuation.Stack,
	staticRoot *assembly.Collector,
	flowRoot *assembly.Collector,
	binding *bind.Result,
	scopes *lexical.Bodies,
	expressions *continuation.Expressions,
	evaluations *eval.Values,
	sourceName string,
) *Writer {
	return &Writer{
		static:      staticRoot,
		flow:        flowRoot,
		binding:     binding,
		scopes:      scopes,
		sourceName:  sourceName,
		phases:      phases,
		expressions: expressions,
		evaluations: evaluations,
	}
}

func (w *Writer) term(term keyspace.Term, what string) (keyspace.Term, error) {
	if term == 0 {
		return 0, fmt.Errorf("lualower: could not create %s", what)
	}
	return term, nil
}

func (w *Writer) rangeTerms(mark, count int) ([]keyspace.Term, error) {
	if w == nil || mark < 0 || count < 0 || mark > len(w.children) || len(w.children)-mark != count {
		return nil, fmt.Errorf("lualower: incomplete static type children")
	}
	terms := w.children[mark:]
	w.children = w.children[:mark]
	return terms, nil
}

func (w *Writer) span(holder ast.PositionHolder) source.Span {
	return coord.Span(w.sourceName, holder)
}

func (w *Writer) nameSpan(position ast.Position) source.Span {
	file := position.File
	if file == "" {
		file = w.sourceName
	}
	span, ok := coord.Build(file, position.Line, position.Column, position.EndLine, position.EndColumn)
	if !ok {
		return coord.Invalid(file)
	}
	return span
}

// walkKind is private to the static vertical. The shared phase stack records
// only that Static runs next; it never becomes a second semantic instruction
// language for type syntax.
type walkKind uint8

const (
	aliasConstraintsWalk walkKind = iota + 1
	finishAliasWalk
	interfaceExtendsWalk
	interfaceMembersWalk
	typeWalk
	finishAnnotatedWalk
	typeListWalk
	appendTypeWalk
	finishOptionalWalk
	finishUnionWalk
	finishIntersectionWalk
	finishGenericBaseWalk
	finishGenericWalk
	finishTypeOfWalk
	finishKeyOfWalk
	indexChildrenWalk
	finishIndexWalk
	conditionalChildrenWalk
	finishConditionalWalk
	annotationsWalk
	finishAnnotationWalk
	finishArrayWalk
	finishMapKeyWalk
	finishMapWalk
	recordFieldsWalk
	finishFieldWalk
	finishInterfaceFieldWalk
	appendInterfaceMethodWalk
	signatureGenericsWalk
	signatureParamsWalk
	finishSignatureVariadicWalk
	signatureReturnsWalk
	finishSignatureWalk
	finishAssertionWalk
	finishDeclaredCellTypeWalk
	finishParamWalk
)

type walkStep struct {
	kind walkKind

	alias       *ast.TypeDefStmt
	iface       *ast.InterfaceDefStmt
	typeExpr    ast.TypeExpr
	types       []ast.TypeExpr
	typeParam   bind.TypeDecl
	typeParams  []bind.TypeDecl
	annotations []ast.AnnotationExpr
	field       ast.RecordFieldExpr
	member      ast.InterfaceMember
	node        ast.PositionHolder

	index      int
	ordinal    int
	mark       int
	staticMark int
	typeHost   keyspace.Term
	typeBase   keyspace.Term
	annotation keyspace.Term
	variadic   keyspace.Term
	body       keyspace.Term
	span       source.Span
}
