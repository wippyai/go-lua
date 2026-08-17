package static

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
)

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
