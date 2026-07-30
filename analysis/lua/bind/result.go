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
	declarations          map[symbol.ID]Declaration
	occurrences           map[symbol.ID][]Occurrence
	implicitGlobalUses    map[*ast.IdentExpr]struct{}
	implicitGlobalSymbols map[symbol.ID]struct{}
	typeValueRefs         map[*ast.IdentExpr]TypeDecl

	directCalls            map[symbol.ID][]*ast.FuncCallExpr
	runtimeUseScanComplete bool

	nextSymbolID symbol.ID

	names map[symbol.ID]string
	kinds map[symbol.ID]symbol.Kind

	globals map[string]globalSymbol

	functionSymbols   map[*ast.FunctionExpr]symbol.ID
	functionsBySymbol map[symbol.ID]*ast.FunctionExpr
	functions         []*ast.FunctionExpr
	nestedFunctions   map[*ast.FunctionExpr][]*ast.FunctionExpr
	functionOrigins   map[*ast.FunctionExpr]FunctionOrigin
	// functionTargetIndex maps a value binding to the sole function identity
	// introduced for that binding. A present zero value is an ambiguity marker.
	// It is built once with the function-origin table and is never exposed.
	functionTargetIndex map[symbol.ID]symbol.ID
	functionIndex       map[*ast.FunctionExpr]int
	functionSubtreeEnd  map[*ast.FunctionExpr]int
	declaringFunctions  map[symbol.ID]*ast.FunctionExpr
	directCaptures      map[*ast.FunctionExpr][]Capture
	directCaptureSeen   map[*ast.FunctionExpr]map[symbol.ID]struct{}
	directGlobalReads   map[*ast.FunctionExpr][]symbol.ID
	directGlobalSeen    map[*ast.FunctionExpr]map[symbol.ID]struct{}
	chunkGlobalReads    []symbol.ID
	chunkGlobalSeen     map[symbol.ID]struct{}

	paramSymbols  map[*ast.FunctionExpr][]symbol.ID
	varargSymbols map[*ast.FunctionExpr]symbol.ID
	paramSlots    map[*ast.FunctionExpr][]ParamSlot
	// symbolAnnotations retains exact declaration syntax when a symbol has no
	// runtime declaring-function relation, such as a source-only signature.
	symbolAnnotations map[symbol.ID]ast.TypeExpr
	localSymbols      map[*ast.LocalAssignStmt][]symbol.ID
	numForSymbols     map[*ast.NumberForStmt]symbol.ID
	genericForSymbols map[*ast.GenericForStmt][]symbol.ID

	gotoTargets   map[*ast.GotoStmt]*ast.LabelStmt
	controlIssues []ControlIssue

	nextTypeDeclID       TypeDeclID
	typeRefs             map[*ast.TypeRefExpr]TypeDecl
	primitiveTypeRefs    map[*ast.PrimitiveTypeExpr]TypeDecl
	typeDefDecls         map[*ast.TypeDefStmt]TypeDecl
	interfaceDecls       map[*ast.InterfaceDefStmt]TypeDecl
	typeDefParams        map[*ast.TypeDefStmt][]TypeDecl
	functionTypeParams   map[ast.PositionHolder][]TypeDecl
	methodReceiverTypes  map[*ast.FunctionExpr]TypeDecl
	assertedParams       map[*ast.AssertsTypeExpr]int
	qualifiedTypeRefs    map[*ast.TypeRefExpr]QualifiedTypeAlias
	qualifiedTypeAliases map[qualifiedTypeAliasKey]QualifiedTypeAlias
	// qualifiedTypeRoots records value-namespace roots used only by qualified
	// type references in a function (for example protocol.User where protocol
	// is an outer local initialized by require("protocol")).
	qualifiedTypeRoots map[*ast.FunctionExpr]map[string]symbol.ID
}

func newResult(opts Options) *Result {
	r := &Result{
		identSymbols:          make(map[*ast.IdentExpr]symbol.ID),
		readIdents:            make(map[symbol.ID][]*ast.IdentExpr),
		writeIdents:           make(map[symbol.ID][]*ast.IdentExpr),
		declarations:          make(map[symbol.ID]Declaration),
		occurrences:           make(map[symbol.ID][]Occurrence),
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
		functionTargetIndex:   make(map[symbol.ID]symbol.ID),
		functionIndex:         make(map[*ast.FunctionExpr]int),
		functionSubtreeEnd:    make(map[*ast.FunctionExpr]int),
		declaringFunctions:    make(map[symbol.ID]*ast.FunctionExpr),
		directCaptures:        make(map[*ast.FunctionExpr][]Capture),
		directCaptureSeen:     make(map[*ast.FunctionExpr]map[symbol.ID]struct{}),
		directGlobalReads:     make(map[*ast.FunctionExpr][]symbol.ID),
		directGlobalSeen:      make(map[*ast.FunctionExpr]map[symbol.ID]struct{}),
		chunkGlobalSeen:       make(map[symbol.ID]struct{}),
		paramSymbols:          make(map[*ast.FunctionExpr][]symbol.ID),
		varargSymbols:         make(map[*ast.FunctionExpr]symbol.ID),
		paramSlots:            make(map[*ast.FunctionExpr][]ParamSlot),
		symbolAnnotations:     make(map[symbol.ID]ast.TypeExpr),
		localSymbols:          make(map[*ast.LocalAssignStmt][]symbol.ID),
		numForSymbols:         make(map[*ast.NumberForStmt]symbol.ID),
		genericForSymbols:     make(map[*ast.GenericForStmt][]symbol.ID),
		typeRefs:              make(map[*ast.TypeRefExpr]TypeDecl),
		primitiveTypeRefs:     make(map[*ast.PrimitiveTypeExpr]TypeDecl),
		typeDefDecls:          make(map[*ast.TypeDefStmt]TypeDecl),
		interfaceDecls:        make(map[*ast.InterfaceDefStmt]TypeDecl),
		typeDefParams:         make(map[*ast.TypeDefStmt][]TypeDecl),
		functionTypeParams:    make(map[ast.PositionHolder][]TypeDecl),
		methodReceiverTypes:   make(map[*ast.FunctionExpr]TypeDecl),
		qualifiedTypeRefs:     make(map[*ast.TypeRefExpr]QualifiedTypeAlias),
		qualifiedTypeAliases:  make(map[qualifiedTypeAliasKey]QualifiedTypeAlias),
		qualifiedTypeRoots:    make(map[*ast.FunctionExpr]map[string]symbol.ID),
	}
	r.runtimeUseScanComplete = true
	for _, name := range normalizeNames(opts.Globals) {
		r.global(name, true)
	}
	return r
}
