package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Options configures lexical binding.
type Options struct {
	Globals []string
}

// Result records lexical declaration identities for identifier occurrences.
type Result struct {
	identSymbols          map[*ast.IdentExpr]symbol.ID
	readIdents            map[symbol.ID][]*ast.IdentExpr
	writeIdents           map[symbol.ID][]*ast.IdentExpr
	implicitGlobalUses    map[*ast.IdentExpr]struct{}
	implicitGlobalSymbols map[symbol.ID]struct{}
	typeValueRefs         map[*ast.IdentExpr]TypeDecl

	names map[symbol.ID]string
	kinds map[symbol.ID]symbol.Kind

	globals map[string]globalSymbol

	functionSymbols    map[*ast.FunctionExpr]symbol.ID
	functionsBySymbol  map[symbol.ID]*ast.FunctionExpr
	functions          []*ast.FunctionExpr
	nestedFunctions    map[*ast.FunctionExpr][]*ast.FunctionExpr
	functionOrigins    map[*ast.FunctionExpr]FunctionOrigin
	declaringFunctions map[symbol.ID]*ast.FunctionExpr
	directCaptures     map[*ast.FunctionExpr][]Capture
	directCaptureSeen  map[*ast.FunctionExpr]map[symbol.ID]struct{}

	paramSymbols      map[*ast.FunctionExpr][]symbol.ID
	varargSymbols     map[*ast.FunctionExpr]symbol.ID
	paramSlots        map[*ast.FunctionExpr][]ParamSlot
	localSymbols      map[*ast.LocalAssignStmt][]symbol.ID
	numForSymbols     map[*ast.NumberForStmt]symbol.ID
	genericForSymbols map[*ast.GenericForStmt][]symbol.ID

	nextTypeDeclID      TypeDeclID
	typeRefs            map[*ast.TypeRefExpr]TypeDecl
	primitiveTypeRefs   map[*ast.PrimitiveTypeExpr]TypeDecl
	typeDefDecls        map[*ast.TypeDefStmt]TypeDecl
	interfaceDecls      map[*ast.InterfaceDefStmt]TypeDecl
	typeDefParams       map[*ast.TypeDefStmt][]TypeDecl
	functionTypeParams  map[*ast.FunctionExpr][]TypeDecl
	methodReceiverTypes map[*ast.FunctionExpr]TypeDecl
}

func newResult(opts Options) *Result {
	r := &Result{
		identSymbols:          make(map[*ast.IdentExpr]symbol.ID),
		readIdents:            make(map[symbol.ID][]*ast.IdentExpr),
		writeIdents:           make(map[symbol.ID][]*ast.IdentExpr),
		implicitGlobalUses:    make(map[*ast.IdentExpr]struct{}),
		implicitGlobalSymbols: make(map[symbol.ID]struct{}),
		typeValueRefs:         make(map[*ast.IdentExpr]TypeDecl),
		names:                 make(map[symbol.ID]string),
		kinds:                 make(map[symbol.ID]symbol.Kind),
		globals:               make(map[string]globalSymbol),
		functionSymbols:       make(map[*ast.FunctionExpr]symbol.ID),
		functionsBySymbol:     make(map[symbol.ID]*ast.FunctionExpr),
		nestedFunctions:       make(map[*ast.FunctionExpr][]*ast.FunctionExpr),
		functionOrigins:       make(map[*ast.FunctionExpr]FunctionOrigin),
		declaringFunctions:    make(map[symbol.ID]*ast.FunctionExpr),
		directCaptures:        make(map[*ast.FunctionExpr][]Capture),
		directCaptureSeen:     make(map[*ast.FunctionExpr]map[symbol.ID]struct{}),
		paramSymbols:          make(map[*ast.FunctionExpr][]symbol.ID),
		varargSymbols:         make(map[*ast.FunctionExpr]symbol.ID),
		paramSlots:            make(map[*ast.FunctionExpr][]ParamSlot),
		localSymbols:          make(map[*ast.LocalAssignStmt][]symbol.ID),
		numForSymbols:         make(map[*ast.NumberForStmt]symbol.ID),
		genericForSymbols:     make(map[*ast.GenericForStmt][]symbol.ID),
		typeRefs:              make(map[*ast.TypeRefExpr]TypeDecl),
		primitiveTypeRefs:     make(map[*ast.PrimitiveTypeExpr]TypeDecl),
		typeDefDecls:          make(map[*ast.TypeDefStmt]TypeDecl),
		interfaceDecls:        make(map[*ast.InterfaceDefStmt]TypeDecl),
		typeDefParams:         make(map[*ast.TypeDefStmt][]TypeDecl),
		functionTypeParams:    make(map[*ast.FunctionExpr][]TypeDecl),
		methodReceiverTypes:   make(map[*ast.FunctionExpr]TypeDecl),
	}
	for _, name := range normalizeNames(opts.Globals) {
		r.global(name, true)
	}
	return r
}
