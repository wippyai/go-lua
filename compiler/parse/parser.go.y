%{
package parse

import (
  "github.com/wippyai/go-lua/compiler/ast"
  "github.com/wippyai/go-lua/compiler/parse/numparse"
)

// parenthesizedType closes a parenthesized type. Parentheses around a type and
// the parameter list of a function type are the same token sequence, so both
// reduce through funcparamlist and the arrow that follows decides which one was
// written. Without an arrow the parentheses group exactly one type, so a
// parameter name, a variadic marker, or a second entry is a parameter list with
// no function type to belong to.
func parenthesizedType(yylex yyLexer, params functionTypeParams, open, close ast.Token) ast.TypeExpr {
  if params.Variadic == nil && len(params.Params) == 1 && params.Params[0].Name == "" && params.Params[0].Type != nil {
    typ := params.Params[0].Type
    typ.SetPosFromToken(open.Pos)
    typ.SetLastPosFromToken(close.Pos)
    return typ
  }
  yylex.(*Lexer).TokenError(open, "parameter list is not a type: expected -> after )")
  return &ast.PrimitiveTypeExpr{Name: "unknown"}
}

// functionTypeParams is parser-only assembly for a function-type parameter
// list. The AST retains fixed parameters and the optional variadic tail in
// their distinct semantic fields; no parameter-name sentinel represents `...`.
type functionTypeParams struct {
  Params            []ast.FunctionParamExpr
  Variadic          ast.TypeExpr
  VariadicPosition  ast.Position
}

func functionType(params functionTypeParams, returns []ast.TypeExpr) *ast.FunctionTypeExpr {
  return &ast.FunctionTypeExpr{
    Params:            params.Params,
    Variadic:          params.Variadic,
    VariadicPosition:  params.VariadicPosition,
    Returns:           returns,
  }
}

func appendFunctionTypeParam(yylex yyLexer, params functionTypeParams, param ast.FunctionParamExpr) functionTypeParams {
  if params.Variadic != nil {
    yylex.(*Lexer).TokenError(ast.Token{Str: "...", Pos: params.VariadicPosition}, "variadic function type parameter must be terminal")
  }
  params.Params = append(params.Params, param)
  return params
}

func setFunctionTypeVariadic(yylex yyLexer, params functionTypeParams, marker ast.Token, typ ast.TypeExpr) functionTypeParams {
  if params.Variadic != nil {
    yylex.(*Lexer).TokenError(marker, "function type has more than one variadic parameter")
  }
  params.Variadic = typ
  params.VariadicPosition = marker.Pos
  return params
}

func callExpr(callee, receiver ast.Expr, method ast.Token, typeArgs []ast.TypeExpr, args callArguments) ast.Expr {
  call := &ast.FuncCallExpr{
    Func: callee, Receiver: receiver,
    Method: method.Str, MethodPosition: method.Pos,
    TypeArgs: typeArgs, Args: args.values,
  }
  if receiver != nil {
    call.CopyPos(receiver)
  } else {
    call.CopyPos(callee)
  }
  call.SetLastPosFromToken(args.end)
  return call
}

func positionAtEnd(node ast.PositionHolder) ast.Position {
  return ast.Position{Line: node.LastLine(), Column: node.LastColumn()}
}

func typeReference(name ast.Token) *ast.TypeRefExpr {
  ref := &ast.TypeRefExpr{Path: []string{name.Str}, RootPosition: name.Pos}
  ref.SetPosFromToken(name.Pos)
  ref.SetLastPosFromToken(name.Pos)
  return ref
}

func qualifyTypeReference(ref *ast.TypeRefExpr, name ast.Token) *ast.TypeRefExpr {
  ref.Path = append(ref.Path, name.Str)
  ref.SetLastPosFromToken(name.Pos)
  return ref
}

func genericTypeReference(base *ast.TypeRefExpr, args []ast.TypeExpr, close ast.Token) *ast.GenericTypeExpr {
  generic := &ast.GenericTypeExpr{Base: base, Args: args}
  generic.CopyPos(base)
  generic.SetLastPosFromToken(close.Pos)
  return generic
}

// annotationExpr keeps the authored annotation range intact.  Its span begins
// at @ and ends at either the name or the closing parenthesis, rather than at
// an argument expression.
func annotationExpr(at, name, end ast.Token, args []ast.Expr) ast.AnnotationExpr {
  annotation := ast.AnnotationExpr{Name: name.Str, Args: args}
  annotation.SetPosFromToken(at.Pos)
  annotation.SetLastPosFromToken(end.Pos)
  return annotation
}

func annotationToken(annotation ast.AnnotationExpr, file string) ast.Token {
  return ast.Token{
    Str: "@" + annotation.Name,
    Pos: ast.Position{
      File:      file,
      Line:      annotation.Line(),
      Column:    annotation.Column(),
      EndLine:   annotation.LastLine(),
      EndColumn: annotation.LastColumn(),
    },
  }
}

func annotatedType(typ ast.TypeExpr, annotations []ast.AnnotationExpr) ast.TypeExpr {
  if len(annotations) == 0 {
    return typ
  }
  result := &ast.AnnotatedTypeExpr{Inner: typ, Annotations: annotations}
  result.CopyPos(typ)
  result.CopyLastPos(&annotations[len(annotations)-1])
  return result
}

func annotationError(yylex yyLexer, annotation ast.AnnotationExpr) {
  lexer := yylex.(*Lexer)
  lexer.TokenError(annotationToken(annotation, lexer.scanner.Pos.File), "annotations are only supported on primitive types and arrays")
}

// annotateDirectType owns ordinary postfix annotation policy. The narrow
// accepted set is intentional: unsupported source remains rejected rather
// than silently extending the language with a new annotation surface.
func annotateDirectType(yylex yyLexer, typ ast.TypeExpr, annotations []ast.AnnotationExpr) ast.TypeExpr {
  switch typ.(type) {
  case *ast.PrimitiveTypeExpr, *ast.ArrayTypeExpr:
    return annotatedType(typ, annotations)
  default:
    if len(annotations) != 0 {
      annotationError(yylex, annotations[0])
    }
    return typ
  }
}

// annotateFieldType keeps the pre-existing optional-tail surface while making
// the type expression itself, rather than its containing field, the owner.
func annotateFieldType(yylex yyLexer, typ ast.TypeExpr, annotations []ast.AnnotationExpr) ast.TypeExpr {
  if _, ok := typ.(*ast.OptionalTypeExpr); ok {
    return annotatedType(typ, annotations)
  }
  if len(annotations) != 0 {
    annotationError(yylex, annotations[0])
  }
  return typ
}

type returnTypeAnnotation struct {
  types []ast.TypeExpr
  known bool
  end ast.Position
}

type callArguments struct {
  values []ast.Expr
  end ast.Position
}

type interfaceBody struct {
  members []ast.InterfaceMember
}
%}
%type<stmts> chunk
%type<stmts> chunk1
%type<stmts> block
%type<stmt>  stat
%type<stmts> elseifs
%type<stmt>  laststat
%type<funcname> funcname
%type<funcname> funcname1
%type<exprlist> varlist
%type<expr> var
%type<namelist> namelist
%type<exprlist> exprlist
%type<expr> expr
%type<expr> string
%type<expr> prefixexp
%type<expr> functioncall
%type<expr> afunctioncall
%type<callargs> args
%type<expr> function
%type<funcexpr> funcbody
%type<parlist> parlist
%type<expr> tableconstructor
%type<fieldlist> fieldlist
%type<field> field
%type<fieldsep> fieldsep
%type<token> fieldname
%type<token> staticfieldname
%type<token> methodname

%type<typeexpr> typeexpr
%type<typeexpr> simpletypeexpr
%type<typeexpr> primarytypeexpr
%type<typeexprlist> typeexprlist
%type<typeexprlist> typeexprlist2
%type<typeexprlist> calltypeargs
%type<token> closegt
%type<returntype> returntypeannot
%type<typeparams> typeparams
%type<typeparams> optionaltypeparams
%type<typeparams> typeparamlist
%type<typeparam> typeparam
%type<recordfields> typefieldlist
%type<recordfield> typefield
%type<typeexpr> typefieldtype
%type<typednames> typednamelist
%type<typedname> typedname
%type<interfacebody> interfacebody
%type<ifacemember> interfacemethod
%type<typereflist> interfaceextends
%type<typeref> interfaceref
%type<typeref> qualifiedtyperef
%type<funcparam> funcparam
%type<functionparams> funcparamlist
%type<annotation> annotation
%type<annotations> annotations

%union {
  token  ast.Token

  stmts    []ast.Stmt
  stmt     ast.Stmt

  funcname *ast.FuncName
  funcexpr *ast.FunctionExpr

  exprlist []ast.Expr
  expr   ast.Expr

  fieldlist []*ast.Field
  field     *ast.Field
  fieldsep  string

  namelist []nameWithPos
  parlist  *ast.ParList

  typeexpr     ast.TypeExpr
  typeexprlist []ast.TypeExpr
  callargs     callArguments
  returntype   returnTypeAnnotation
  typeparam    ast.TypeParamExpr
  typeparams   []ast.TypeParamExpr
  recordfield  ast.RecordFieldExpr
  recordfields []ast.RecordFieldExpr
  typedname    typedNameEntry
  typednames   []typedNameEntry
  ifacemember  ast.InterfaceMember
  interfacebody interfaceBody
  typereflist  []*ast.TypeRefExpr
  typeref      *ast.TypeRefExpr
  funcparam    ast.FunctionParamExpr
  functionparams functionTypeParams
  annotation   ast.AnnotationExpr
  annotations  []ast.AnnotationExpr
}

/* Reserved words */
%token<token> TAnd TBreak TDo TElse TElseIf TEnd TFalse TFor TFunction TIf TIn TLocal TNil TNot TOr TReturn TRepeat TThen TTrue TUntil TWhile TGoto
%token TAssertsRefinement

/* Type system keywords */
%token<token> TInterface TReadonly TAs TAsserts TIs TTypeof TKeyof TExtends TFun

/* Literals */
%token<token> TEqeq TNeq TLte TGte T2Comma T3Comma T2Colon TTypeArgsOpen TLabel TIdent TNumber TString '@' '{' '}' '(' ')' '>'

/* Lua 5.3 operators */
%token<token> TShl TShr TIdiv

/* Index brackets carry their own token positions: an indexed expression's
   closing bracket is the authoritative end of its source span. */
%token<token> '[' ']'

/* Type annotation operators */
%token<token> TArrow TQuestion TBang TQuestionColon

/* Operators - Lua 5.3 precedence */
%left TOr
%left TAnd
%left '>' '<' TGte TLte TEqeq TNeq
%left '|'
%left '~'
%left '&'
%left TShl TShr
%right T2Comma
%left '+' '-'
%left '*' '/' '%' TIdiv
%right UNARY /* not # -(unary) ~(bnot) */
%right '^'
%nonassoc T2Colon /* :: cast - nonassoc to prefer reduce over shift for labels */
%left TAs TBang /* type cast (as) and non-nil assertion */
/* These resolve the two deliberate suffix continuations without expanding the
   inherited conflict budget: T @a[] continues as an element annotation, and
   asserts x is T continues as the refining assertion.  The refining rule uses
   an unprioritized synthetic marker so ordinary type-operator conflicts keep
   their inherited shift behavior. */
%nonassoc TAnnotationTail
%nonassoc '['
%nonassoc TAssertsBare
%nonassoc TIs

/* goyacc reports 29 inherited shift/reduce conflicts. The generated parser
   resolves them by shift; this grammar adds none. */

%%

chunk: 
        chunk1 {
            $$ = $1
            if l, ok := yylex.(*Lexer); ok {
                l.Stmts = $$
            }
        } |
        chunk1 laststat {
            $$ = append($1, $2)
            if l, ok := yylex.(*Lexer); ok {
                l.Stmts = $$
            }
        } | 
        chunk1 laststat ';' {
            $$ = append($1, $2)
            if l, ok := yylex.(*Lexer); ok {
                l.Stmts = $$
            }
        }

chunk1: 
        {
            $$ = []ast.Stmt{}
        } |
        chunk1 stat {
            $$ = append($1, $2)
        } | 
        chunk1 ';' {
            $$ = $1
        }

block: 
        chunk {
            $$ = $1
        }

stat:
        varlist '=' exprlist {
            $$ = &ast.AssignStmt{Lhs: $1, Rhs: $3}
            $$.CopyPos($1[0])
        } |
        /* 'stat = functioncal' causes a reduce/reduce conflict */
        prefixexp {
            if _, ok := $1.(*ast.FuncCallExpr); !ok {
               yylex.(*Lexer).Error("parse error")
            } else {
              $$ = &ast.FuncCallStmt{Expr: $1}
              $$.CopyPos($1)
            }
        } |
        TDo block TEnd {
            $$ = &ast.DoBlockStmt{Stmts: $2}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($3.Pos)
        } |
        TWhile expr TDo block TEnd {
            $$ = &ast.WhileStmt{Condition: $2, Stmts: $4}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($5.Pos)
        } |
        TRepeat block TUntil expr {
            $$ = &ast.RepeatStmt{Condition: $4, Stmts: $2}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastLine($4.Line())
        } |
        TIf expr TThen block elseifs TEnd {
            $$ = &ast.IfStmt{Condition: $2, Then: $4, EndPosition: $6.Pos}
            cur := $$
            for _, elseif := range $5 {
                cur.(*ast.IfStmt).Else = []ast.Stmt{elseif}
                cur = elseif
            }
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($6.Pos)
        } |
        TIf expr TThen block elseifs TElse block TEnd {
            $$ = &ast.IfStmt{Condition: $2, Then: $4, EndPosition: $8.Pos}
            cur := $$
            for _, elseif := range $5 {
                cur.(*ast.IfStmt).Else = []ast.Stmt{elseif}
                cur = elseif
            }
			cur.(*ast.IfStmt).Else = $7
			cur.(*ast.IfStmt).HasElse = true
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($8.Pos)
        } |
        TFor TIdent '=' expr ',' expr TDo block TEnd {
            $$ = &ast.NumberForStmt{Name: $2.Str, NamePosition: $2.Pos, Init: $4, Limit: $6, Stmts: $8}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($9.Pos)
        } |
        TFor TIdent '=' expr ',' expr ',' expr TDo block TEnd {
            $$ = &ast.NumberForStmt{Name: $2.Str, NamePosition: $2.Pos, Init: $4, Limit: $6, Step:$8, Stmts: $10}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($11.Pos)
        } |
        TFor namelist TIn exprlist TDo block TEnd {
            names, positions := splitNameList($2)
            $$ = &ast.GenericForStmt{Names: names, NamePositions: positions, Exprs:$4, Stmts: $6}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($7.Pos)
        } |
        TFunction funcname funcbody {
            $$ = &ast.FuncDefStmt{Name: $2, Func: $3}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($3)
        } |
        TLocal TFunction TIdent funcbody {
            $$ = &ast.LocalAssignStmt{Names:[]string{$3.Str}, NamePositions: []ast.Position{$3.Pos}, Exprs: []ast.Expr{$4}, LocalFunction: true}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($4)
        } |
        TLocal typednamelist '=' exprlist {
            names, positions, types := splitTypedNames($2)
            $$ = &ast.LocalAssignStmt{Names: names, NamePositions: positions, Types: types, Exprs: $4}
            $$.SetPosFromToken($1.Pos)
        } |
        TLocal typednamelist {
            names, positions, types := splitTypedNames($2)
            $$ = &ast.LocalAssignStmt{Names: names, NamePositions: positions, Types: types, Exprs: []ast.Expr{}}
            $$.SetPosFromToken($1.Pos)
        } |
        TLabel {
            $$ = &ast.LabelStmt{Name: $1.Str}
            $$.SetPosFromToken($1.Pos)
        } |
        TGoto TIdent {
            $$ = &ast.GotoStmt{Label: $2.Str}
            $$.SetPosFromToken($1.Pos)
        } |
        TIdent TIdent '=' typeexpr {
            if $1.Str != "type" {
                yylex.(*Lexer).Error("unexpected identifier")
            }
            $$ = &ast.TypeDefStmt{Name: $2.Str, NamePosition: $2.Pos, Type: $4}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($4)
        } |
        TIdent TIdent typeparams '=' typeexpr {
            if $1.Str != "type" {
                yylex.(*Lexer).Error("unexpected identifier")
            }
            $$ = &ast.TypeDefStmt{Name: $2.Str, NamePosition: $2.Pos, TypeParams: $3, Type: $5}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($5)
        } |
        TInterface TIdent interfaceextends interfacebody TEnd {
            $$ = &ast.InterfaceDefStmt{Name: $2.Str, NamePosition: $2.Pos, Extends: $3, Members: $4.members}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($5.Pos)
        }

elseifs: 
        {
            $$ = []ast.Stmt{}
        } | 
        elseifs TElseIf expr TThen block {
            $$ = append($1, &ast.IfStmt{Condition: $3, Then: $5})
            $$[len($$)-1].SetPosFromToken($2.Pos)
        }

laststat:
        TReturn {
            $$ = &ast.ReturnStmt{Exprs:nil}
            $$.SetPosFromToken($1.Pos)
        } |
        TReturn exprlist {
            $$ = &ast.ReturnStmt{Exprs:$2}
            $$.SetPosFromToken($1.Pos)
        } |
        TBreak  {
            $$ = &ast.BreakStmt{}
            $$.SetPosFromToken($1.Pos)
        }

funcname:
        funcname1 {
            $$ = $1
        } |
        funcname1 ':' methodname {
            $$ = &ast.FuncName{Func:nil, Receiver:$1.Func, Method: $3.Str, MethodPosition: $3.Pos}
        }

funcname1:
        TIdent {
            $$ = &ast.FuncName{Func: &ast.IdentExpr{Value:$1.Str}}
            $$.Func.SetPosFromToken($1.Pos)
        } | 
        funcname1 '.' TIdent {
            key:= &ast.StringExpr{Value:$3.Str}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            fn := &ast.AttrGetExpr{Object: $1.Func, Key: key, KeySyntax: ast.AttrKeyDot}
            fn.CopyPos($1.Func)
            fn.SetLastPosFromToken($3.Pos)
            $$ = &ast.FuncName{Func: fn}
        }

varlist:
        var {
            $$ = []ast.Expr{$1}
        } | 
        varlist ',' var {
            $$ = append($1, $3)
        }

var:
        TIdent {
            $$ = &ast.IdentExpr{Value:$1.Str}
            $$.SetPosFromToken($1.Pos)
        } |
        prefixexp '[' expr ']' {
            $$ = &ast.AttrGetExpr{Object: $1, Key: $3, KeySyntax: ast.AttrKeyIndex}
            $$.CopyPos($1)
            $$.SetLastLine($4.Pos.Line)
            $$.SetLastColumn($4.Pos.Column + 1)
        } | 
        prefixexp '.' TIdent {
            key := &ast.StringExpr{Value:$3.Str}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key, KeySyntax: ast.AttrKeyDot}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        prefixexp '.' TInterface {
            key := &ast.StringExpr{Value:"interface"}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key, KeySyntax: ast.AttrKeyDot}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        prefixexp '.' TReadonly {
            key := &ast.StringExpr{Value:"readonly"}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key, KeySyntax: ast.AttrKeyDot}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        prefixexp '.' TAs {
            key := &ast.StringExpr{Value:"as"}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key, KeySyntax: ast.AttrKeyDot}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        prefixexp '.' TAsserts {
            key := &ast.StringExpr{Value:"asserts"}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key, KeySyntax: ast.AttrKeyDot}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        prefixexp '.' TIs {
            key := &ast.StringExpr{Value:"is"}
            key.SetPosFromToken($3.Pos)
            key.SetLastPosFromToken($3.Pos)
            $$ = &ast.AttrGetExpr{Object: $1, Key: key, KeySyntax: ast.AttrKeyDot}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        }

namelist:
        TIdent {
            $$ = []nameWithPos{{Name: $1.Str, Pos: $1.Pos}}
        } |
        namelist ','  TIdent {
            $$ = append($1, nameWithPos{Name: $3.Str, Pos: $3.Pos})
        }

exprlist:
        expr {
            $$ = []ast.Expr{$1}
        } |
        exprlist ',' expr {
            $$ = append($1, $3)
        }

expr:
        TNil {
            $$ = &ast.NilExpr{}
            $$.SetPosFromToken($1.Pos)
        } | 
        TFalse {
            $$ = &ast.FalseExpr{}
            $$.SetPosFromToken($1.Pos)
        } | 
        TTrue {
            $$ = &ast.TrueExpr{}
            $$.SetPosFromToken($1.Pos)
        } | 
        TNumber {
            $$ = &ast.NumberExpr{Value: $1.Str}
            $$.SetPosFromToken($1.Pos)
        } | 
        T3Comma {
            $$ = &ast.Comma3Expr{}
            $$.SetPosFromToken($1.Pos)
        } |
        function {
            $$ = $1
        } | 
        prefixexp {
            $$ = $1
        } |
        string {
            $$ = $1
        } |
        tableconstructor {
            $$ = $1
        } |
        expr TOr expr {
            $$ = &ast.LogicalOpExpr{Lhs: $1, Operator: "or", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TAnd expr {
            $$ = &ast.LogicalOpExpr{Lhs: $1, Operator: "and", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '>' expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: ">", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '<' expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: "<", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TGte expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: ">=", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TLte expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: "<=", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TEqeq expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: "==", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TNeq expr {
            $$ = &ast.RelationalOpExpr{Lhs: $1, Operator: "~=", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr T2Comma expr {
            $$ = &ast.StringConcatOpExpr{Lhs: $1, Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '+' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "+", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '-' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "-", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '*' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "*", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '/' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "/", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '%' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "%", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '^' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "^", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '&' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "&", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '|' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "|", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr '~' expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "~", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TShl expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "<<", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TShr expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: ">>", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TIdiv expr {
            $$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "//", Rhs: $3}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        '-' expr %prec UNARY {
            $$ = &ast.UnaryMinusOpExpr{Expr: $2}
            $$.CopyPos($2)
            $$.CopyLastPos($2)
        } |
        TNot expr %prec UNARY {
            $$ = &ast.UnaryNotOpExpr{Expr: $2}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($2)
        } |
        '#' expr %prec UNARY {
            $$ = &ast.UnaryLenOpExpr{Expr: $2}
            $$.CopyPos($2)
            $$.CopyLastPos($2)
        } |
        '~' expr %prec UNARY {
            $$ = &ast.UnaryBNotOpExpr{Expr: $2}
            $$.CopyPos($2)
            $$.CopyLastPos($2)
        } |
        expr TAs typeexpr {
            $$ = &ast.CastExpr{Expr: $1, Type: $3, Syntax: ast.CastSyntaxAs}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr T2Colon typeexpr {
            $$ = &ast.CastExpr{Expr: $1, Type: $3, Syntax: ast.CastSyntaxColonColon}
            $$.CopyPos($1)
            $$.CopyLastPos($3)
        } |
        expr TBang {
            $$ = &ast.NonNilAssertExpr{Expr: $1}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($2.Pos)
        }

string: 
        TString {
            $$ = &ast.StringExpr{Value: $1.Str}
            $$.SetPosFromToken($1.Pos)
        } 

prefixexp:
        var {
            $$ = $1
        } |
        afunctioncall {
            $$ = $1
        } |
        functioncall {
            $$ = $1
        } |
        '(' expr ')' {
            if ex, ok := $2.(*ast.Comma3Expr); ok {
                ex.AdjustRet = true
            }
            $$ = $2
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($3.Pos)
        }

afunctioncall:
        '(' functioncall ')' {
            call := $2.(*ast.FuncCallExpr)
            call.AdjustRet = true
            call.SetPosFromToken($1.Pos)
            call.SetLastPosFromToken($3.Pos)
            $$ = call
        }

functioncall:
        prefixexp args {
            $$ = callExpr($1, nil, ast.Token{}, nil, $2)
        } |
        /* Explicit generic calls use the unambiguous turbofish ::<...>:
           f::<T>(), obj.method::<T>(), and obj:method::<T>(). */
        prefixexp calltypeargs args {
            $$ = callExpr($1, nil, ast.Token{}, $2, $3)
        } |
        prefixexp ':' methodname args {
            $$ = callExpr(nil, $1, $3, nil, $4)
        } |
        prefixexp ':' methodname calltypeargs args {
            $$ = callExpr(nil, $1, $3, $4, $5)
        }

calltypeargs:
        TTypeArgsOpen typeexprlist closegt {
            $$ = $2
        }

args:
        '(' ')' {
            if yylex.(*Lexer).PNewLine {
               yylex.(*Lexer).TokenError($1, "ambiguous syntax (function call x new statement)")
            }
            $$ = callArguments{values: []ast.Expr{}, end: $2.Pos}
        } |
        '(' exprlist ')' {
            if yylex.(*Lexer).PNewLine {
               yylex.(*Lexer).TokenError($1, "ambiguous syntax (function call x new statement)")
            }
            $$ = callArguments{values: $2, end: $3.Pos}
        } |
        tableconstructor {
            $$ = callArguments{values: []ast.Expr{$1}, end: positionAtEnd($1)}
        } | 
        string {
            $$ = callArguments{values: []ast.Expr{$1}, end: positionAtEnd($1)}
        }

function:
        TFunction funcbody {
            $$ = &ast.FunctionExpr{TypeParams: $2.TypeParams, ParList:$2.ParList, ReturnTypes: $2.ReturnTypes, ReturnsKnown: $2.ReturnsKnown, Stmts: $2.Stmts}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($2)
        }

funcbody:
        '(' parlist ')' returntypeannot block TEnd {
            $$ = &ast.FunctionExpr{ParList: $2, ReturnTypes: $4.types, ReturnsKnown: $4.known, Stmts: $5}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($6.Pos)
        } |
        '(' ')' returntypeannot block TEnd {
            $$ = &ast.FunctionExpr{ParList: &ast.ParList{HasVargs: false, Names: []string{}}, ReturnTypes: $3.types, ReturnsKnown: $3.known, Stmts: $4}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($5.Pos)
        } |
        typeparams '(' parlist ')' returntypeannot block TEnd {
            $$ = &ast.FunctionExpr{TypeParams: $1, ParList: $3, ReturnTypes: $5.types, ReturnsKnown: $5.known, Stmts: $6}
            $$.SetPosFromToken($2.Pos)
            $$.SetLastPosFromToken($7.Pos)
        } |
        typeparams '(' ')' returntypeannot block TEnd {
            $$ = &ast.FunctionExpr{TypeParams: $1, ParList: &ast.ParList{HasVargs: false, Names: []string{}}, ReturnTypes: $4.types, ReturnsKnown: $4.known, Stmts: $5}
            $$.SetPosFromToken($2.Pos)
            $$.SetLastPosFromToken($6.Pos)
        }

parlist:
        T3Comma {
            $$ = &ast.ParList{HasVargs: true, VarargPosition: $1.Pos, Names: []string{}}
        } |
        T3Comma ':' typeexpr {
            $$ = &ast.ParList{HasVargs: true, VarargType: $3, VarargPosition: $1.Pos, Names: []string{}}
        } |
        typednamelist {
            names, positions, types := splitTypedNames($1)
            $$ = &ast.ParList{HasVargs: false, Names: names, NamePositions: positions, Types: types}
        } |
        typednamelist ',' T3Comma {
            names, positions, types := splitTypedNames($1)
            $$ = &ast.ParList{HasVargs: true, VarargPosition: $3.Pos, Names: names, NamePositions: positions, Types: types}
        } |
        typednamelist ',' T3Comma ':' typeexpr {
            names, positions, types := splitTypedNames($1)
            $$ = &ast.ParList{HasVargs: true, VarargType: $5, VarargPosition: $3.Pos, Names: names, NamePositions: positions, Types: types}
        }


tableconstructor:
        '{' '}' {
            $$ = &ast.TableExpr{Fields: []*ast.Field{}}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($2.Pos)
        } |
        '{' fieldlist '}' {
            $$ = &ast.TableExpr{Fields: $2}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($3.Pos)
        }


fieldlist:
        field {
            $$ = []*ast.Field{$1}
        } | 
        fieldlist fieldsep field {
            $$ = append($1, $3)
        } | 
        fieldlist fieldsep {
            $$ = $1
        }

field:
        fieldname '=' expr {
            key := &ast.StringExpr{Value: $1.Str}
            key.SetPosFromToken($1.Pos)
            $$ = &ast.Field{Key: key, KeySyntax: ast.AttrKeyDot, Value: $3}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($3)
        } |
        '[' expr ']' '=' expr {
            $$ = &ast.Field{Key: $2, KeySyntax: ast.AttrKeyIndex, Value: $5}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($5)
        } |
        expr {
            $$ = &ast.Field{Value: $1}
            $$.CopyPos($1)
            $$.CopyLastPos($1)
        }

/* fieldname allows identifiers and contextual keywords as table field names */
fieldname:
        TIdent {
            $$ = $1
        } |
        TInterface {
            $$ = $1
        } |
        TReadonly {
            $$ = $1
        } |
        TAs {
            $$ = $1
        } |
        TAsserts {
            $$ = $1
        } |
        TIs {
            $$ = $1
        } |
        TKeyof {
            $$ = $1
        } |
        TExtends {
            $$ = $1
        }

/* Static record and interface fields use the same name vocabulary. */
staticfieldname:
        TIdent {
            $$ = $1
        } |
        TInterface {
            $$ = $1
        } |
        TReadonly {
            $$ = $1
        } |
        TAs {
            $$ = $1
        } |
        TAsserts {
            $$ = $1
        } |
        TIs {
            $$ = $1
        } |
        TKeyof {
            $$ = $1
        } |
        TExtends {
            $$ = $1
        } |
        TTypeof {
            $$ = $1
        }

/* Runtime method definitions/calls and interface methods share this narrower
   vocabulary.  Keep type-only contextual tokens out of runtime methods. */
methodname:
        TIdent {
            $$ = $1
        } |
        TInterface {
            $$ = $1
        } |
        TReadonly {
            $$ = $1
        } |
        TAs {
            $$ = $1
        } |
        TAsserts {
            $$ = $1
        } |
        TIs {
            $$ = $1
        }

fieldsep:
        ',' {
            $$ = ","
        } |
        ';' {
            $$ = ";"
        }

/* Type expressions */

returntypeannot:
        /* empty */ {
            $$ = returnTypeAnnotation{}
        } |
        ':' typeexprlist {
            last := $2[len($2)-1]
            $$ = returnTypeAnnotation{
                types: $2,
                known: true,
                end: ast.Position{
                    Line: last.LastLine(), Column: last.LastColumn(),
                    EndLine: last.LastLine(), EndColumn: last.LastColumn(),
                },
            }
        } |
        ':' '(' ')' {
            $$ = returnTypeAnnotation{known: true, end: $3.Pos}
        } |
        ':' '(' typeexpr ',' typeexprlist ')' {
            $$ = returnTypeAnnotation{types: append([]ast.TypeExpr{$3}, $5...), known: true, end: $6.Pos}
        }

typeexpr:
        simpletypeexpr {
            $$ = $1
        } |
        typeexpr '|' simpletypeexpr {
            if union, ok := $1.(*ast.UnionTypeExpr); ok {
                union.Types = append(union.Types, $3)
                $$ = union
            } else {
                $$ = &ast.UnionTypeExpr{Types: []ast.TypeExpr{$1, $3}}
                $$.CopyPos($1)
            }
            $$.CopyLastPos($3)
        } |
        typeexpr '&' simpletypeexpr {
            if inter, ok := $1.(*ast.IntersectionTypeExpr); ok {
                inter.Types = append(inter.Types, $3)
                $$ = inter
            } else {
                $$ = &ast.IntersectionTypeExpr{Types: []ast.TypeExpr{$1, $3}}
                $$.CopyPos($1)
            }
            $$.CopyLastPos($3)
        } |
        simpletypeexpr TExtends simpletypeexpr TQuestion typeexpr ':' typeexpr {
            $$ = &ast.ConditionalTypeExpr{Check: $1, Extends: $3, Then: $5, Else: $7}
            $$.CopyPos($1)
            $$.CopyLastPos($7)
        }

simpletypeexpr:
        primarytypeexpr {
            $$ = $1
        } |
        primarytypeexpr annotations %prec TAnnotationTail {
            $$ = annotateDirectType(yylex, $1, $2)
        } |
        simpletypeexpr TQuestion {
            $$ = &ast.OptionalTypeExpr{Inner: $1}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($2.Pos)
        }

primarytypeexpr:
        TNil {
            $$ = &ast.PrimitiveTypeExpr{Name: "nil"}
            $$.SetPosFromToken($1.Pos)
        } |
        TTrue {
            $$ = &ast.LiteralTypeExpr{Value: true}
            $$.SetPosFromToken($1.Pos)
        } |
        TFalse {
            $$ = &ast.LiteralTypeExpr{Value: false}
            $$.SetPosFromToken($1.Pos)
        } |
        TString {
            $$ = &ast.LiteralTypeExpr{Value: $1.Str}
            $$.SetPosFromToken($1.Pos)
        } |
        TNumber {
            if i, ok := numparse.ParseIntegerLiteral($1.Str); ok {
                $$ = &ast.LiteralTypeExpr{Value: i}
            } else if f, ok := numparse.ParseFloatLiteral($1.Str); ok {
                $$ = &ast.LiteralTypeExpr{Value: f}
            } else {
                $$ = &ast.LiteralTypeExpr{}
            }
            $$.SetPosFromToken($1.Pos)
        } |
        TIdent {
            $$ = &ast.PrimitiveTypeExpr{Name: $1.Str}
            $$.SetPosFromToken($1.Pos)
        } |
        qualifiedtyperef %prec TAnd {
            $$ = $1
        } |
        TIdent '<' typeexprlist closegt {
            $$ = genericTypeReference(typeReference($1), $3, $4)
        } |
        qualifiedtyperef '<' typeexprlist closegt {
            $$ = genericTypeReference($1, $3, $4)
        } |
        '{' typeexpr '}' {
            $$ = &ast.ArrayTypeExpr{Element: $2}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($3.Pos)
        } |
        '{' '[' typeexpr ']' ':' typeexpr '}' {
            $$ = &ast.MapTypeExpr{Key: $3, Value: $6}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($7.Pos)
        } |
        '{' typefieldlist '}' {
            $$ = &ast.RecordTypeExpr{Fields: $2}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($3.Pos)
        } |
        '(' funcparamlist ')' TArrow '(' typeexprlist2 ')' {
            $$ = functionType($2, $6)
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($7.Pos)
        } |
        '(' funcparamlist ')' TArrow '(' ')' {
            $$ = functionType($2, []ast.TypeExpr{})
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($6.Pos)
        } |
        '(' funcparamlist ')' TArrow typeexpr {
            $$ = functionType($2, []ast.TypeExpr{$5})
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($5)
        } |
        '(' ')' TArrow '(' typeexprlist2 ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: $5}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($6.Pos)
        } |
        '(' ')' TArrow typeexpr {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: []ast.TypeExpr{$4}}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($4)
        } |
        '(' ')' TArrow '(' ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: []ast.TypeExpr{}}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($5.Pos)
        } |
        '(' funcparamlist ')' {
            $$ = parenthesizedType(yylex, $2, $1, $3)
        } |
        TFun '(' funcparamlist ')' ':' typeexpr {
            $$ = functionType($3, []ast.TypeExpr{$6})
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($6)
        } |
        TFun '(' funcparamlist ')' ':' '(' typeexprlist2 ')' {
            $$ = functionType($3, $7)
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($8.Pos)
        } |
        TFun '(' ')' ':' typeexpr {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: []ast.TypeExpr{$5}}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($5)
        } |
        TFun '(' ')' ':' '(' typeexprlist2 ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: $6}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($7.Pos)
        } |
        TFun '(' funcparamlist ')' ':' '(' ')' {
            $$ = functionType($3, []ast.TypeExpr{})
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($7.Pos)
        } |
        TFun '(' ')' ':' '(' ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: []ast.TypeExpr{}}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($6.Pos)
        } |
        TFun '(' funcparamlist ')' {
            $$ = functionType($3, nil)
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($4.Pos)
        } |
        TFun '(' ')' {
            $$ = &ast.FunctionTypeExpr{Params: []ast.FunctionParamExpr{}, Returns: nil}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($3.Pos)
        } |
        TInterface '{' typefieldlist '}' {
            $$ = &ast.RecordTypeExpr{Fields: $3}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($4.Pos)
        } |
        TInterface '{' '}' {
            $$ = &ast.RecordTypeExpr{Fields: nil}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($3.Pos)
        } |
        primarytypeexpr '[' ']' {
            $$ = &ast.ArrayTypeExpr{Element: $1}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($3.Pos)
        } |
        /* T @annotation[] is the compact array spelling whose annotations
           belong to T.  The annotation occurs before the array brackets, so
           it cannot be confused with annotations on the array itself. */
        primarytypeexpr annotations '[' ']' {
            $$ = &ast.ArrayTypeExpr{Element: annotatedType($1, $2)}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($4.Pos)
        } |
        TReadonly '{' typeexpr '}' {
            arr := &ast.ArrayTypeExpr{Element: $3, Readonly: true}
            arr.SetPosFromToken($1.Pos)
            arr.SetLastPosFromToken($4.Pos)
            $$ = arr
        } |
        TReadonly '{' '[' typeexpr ']' ':' typeexpr '}' {
            m := &ast.MapTypeExpr{Key: $4, Value: $7, Readonly: true}
            m.SetPosFromToken($1.Pos)
            m.SetLastPosFromToken($8.Pos)
            $$ = m
        } |
        TReadonly '{' typefieldlist '}' {
            $$ = &ast.RecordTypeExpr{Fields: $3, Readonly: true}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($4.Pos)
        } |
        '{' '}' {
            $$ = &ast.RecordTypeExpr{Fields: nil}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($2.Pos)
        } |
        TAsserts TIdent %prec TAssertsBare {
            $$ = &ast.AssertsTypeExpr{ParamName: $2.Str, ParamPosition: $2.Pos, NarrowTo: nil}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($2.Pos)
        } |
        TAsserts TIdent TIs typeexpr %prec TAssertsRefinement {
            $$ = &ast.AssertsTypeExpr{ParamName: $2.Str, ParamPosition: $2.Pos, NarrowTo: $4}
            $$.SetPosFromToken($1.Pos)
            $$.CopyLastPos($4)
        } |
        TTypeof '(' expr ')' {
            $$ = &ast.TypeOfExpr{Expr: $3}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($4.Pos)
        } |
        TKeyof '(' typeexpr ')' {
            $$ = &ast.KeyOfExpr{Inner: $3}
            $$.SetPosFromToken($1.Pos)
            $$.SetLastPosFromToken($4.Pos)
        } |
        primarytypeexpr '[' typeexpr ']' {
            $$ = &ast.IndexAccessExpr{Object: $1, Index: $3}
            $$.CopyPos($1)
            $$.SetLastPosFromToken($4.Pos)
        }

qualifiedtyperef:
        TIdent '.' TIdent {
            $$ = qualifyTypeReference(typeReference($1), $3)
        } |
        qualifiedtyperef '.' TIdent {
            $$ = qualifyTypeReference($1, $3)
        }

typeexprlist:
        typeexpr {
            $$ = []ast.TypeExpr{$1}
        } |
        typeexprlist ',' typeexpr {
            $$ = append($1, $3)
        }

typeexprlist2:
        typeexpr ',' typeexpr {
            $$ = []ast.TypeExpr{$1, $3}
        } |
        typeexprlist2 ',' typeexpr {
            $$ = append($1, $3)
        }

closegt:
        '>' {
            $$ = $1
        } |
        TShr {
            $$ = ast.Token{
                Type: '>',
                Str:  ">",
                Pos:  ast.Position{File: $1.Pos.File, Line: $1.Pos.Line, Column: $1.Pos.Column},
            }
            yylex.(*Lexer).PendingGT = &ast.Token{
                Type: '>',
                Str:  ">",
                Pos:  ast.Position{File: $1.Pos.File, Line: $1.Pos.Line, Column: $1.Pos.Column + 1},
            }
        }

funcparam:
        TIdent ':' typeexpr {
            $$ = ast.FunctionParamExpr{Name: $1.Str, NamePosition: $1.Pos, Type: $3}
        } |
        typeexpr {
            $$ = ast.FunctionParamExpr{Name: "", Type: $1}
        }

funcparamlist:
        funcparam {
            $$ = functionTypeParams{Params: []ast.FunctionParamExpr{$1}}
        } |
        funcparamlist ',' funcparam {
            $$ = appendFunctionTypeParam(yylex, $1, $3)
        } |
        T3Comma typeexpr {
            $$ = setFunctionTypeVariadic(yylex, functionTypeParams{}, $1, $2)
        } |
        funcparamlist ',' T3Comma typeexpr {
            $$ = setFunctionTypeVariadic(yylex, $1, $3, $4)
        }

typefieldlist:
        typefield {
            $$ = []ast.RecordFieldExpr{$1}
        } |
        typefieldlist ',' typefield {
            $$ = append($1, $3)
        } |
        typefieldlist ',' {
            $$ = $1
        }

typefield:
        staticfieldname ':' typefieldtype {
            $$ = ast.RecordFieldExpr{Name: $1.Str, NamePosition: $1.Pos, Type: $3, Optional: false}
        } |
        staticfieldname TQuestionColon typefieldtype {
            $$ = ast.RecordFieldExpr{Name: $1.Str, NamePosition: $1.Pos, Type: $3, Optional: true}
        }

typefieldtype:
        typeexpr {
            $$ = $1
        } |
        typeexpr annotations {
            $$ = annotateFieldType(yylex, $1, $2)
        }

annotations:
        annotation {
            $$ = []ast.AnnotationExpr{$1}
        } |
        annotations annotation {
            $$ = append($1, $2)
        }

annotation:
        '@' TIdent {
            $$ = annotationExpr($1, $2, $2, nil)
        } |
        '@' TIdent '(' ')' {
            $$ = annotationExpr($1, $2, $4, nil)
        } |
        '@' TIdent '(' exprlist ')' {
            $$ = annotationExpr($1, $2, $5, $4)
        }

typeparams:
        '<' typeparamlist '>' {
            $$ = $2
        }

optionaltypeparams:
        /* empty */ {
            $$ = nil
        } |
        typeparams {
            $$ = $1
        }

typeparamlist:
        typeparam {
            $$ = []ast.TypeParamExpr{$1}
        } |
        typeparamlist ',' typeparam {
            $$ = append($1, $3)
        }

typeparam:
        TIdent {
            $$ = ast.TypeParamExpr{Name: $1.Str, NamePosition: $1.Pos, Constraint: nil}
        } |
        TIdent ':' typeexpr {
            $$ = ast.TypeParamExpr{Name: $1.Str, NamePosition: $1.Pos, Constraint: $3}
        }

typednamelist:
        typedname {
            $$ = []typedNameEntry{$1}
        } |
        typednamelist ',' typedname {
            $$ = append($1, $3)
        }

typedname:
        TIdent {
            $$ = typedNameEntry{Name: $1.Str, Pos: $1.Pos, Type: nil}
        } |
        TIdent ':' typeexpr {
            $$ = typedNameEntry{Name: $1.Str, Pos: $1.Pos, Type: $3}
        }

interfaceextends:
        /* empty */ {
            $$ = nil
        } |
        ':' interfaceref {
            $$ = []*ast.TypeRefExpr{$2}
        } |
        interfaceextends ',' interfaceref {
            $$ = append($1, $3)
        }

interfaceref:
        TIdent {
            $$ = typeReference($1)
        } |
        qualifiedtyperef {
            $$ = $1
        }

interfacebody:
        /* empty */ {
            $$ = interfaceBody{}
        } |
        interfacebody typefield {
            $$ = $1
            $$.members = append($$.members, ast.InterfaceMember{
                Kind: ast.InterfaceFieldMember,
                Name: $2.Name, NamePosition: $2.NamePosition, Type: $2.Type,
                Optional: $2.Optional,
            })
        } |
        interfacebody interfacemethod {
            $$ = $1
            $$.members = append($$.members, $2)
        }

interfacemethod:
        TFunction methodname optionaltypeparams '(' ')' returntypeannot {
            returns := $6.types
            if $6.known && returns == nil {
                returns = []ast.TypeExpr{}
            }
            typ := &ast.FunctionTypeExpr{TypeParams: $3, Params: []ast.FunctionParamExpr{}, Returns: returns}
            typ.SetPosFromToken($1.Pos)
            if $6.end.Line != 0 {
                typ.SetLastPosFromToken($6.end)
            } else {
                typ.SetLastPosFromToken($5.Pos)
            }
            $$ = ast.InterfaceMember{
                Kind: ast.InterfaceMethodMember,
                Name: $2.Str, NamePosition: $2.Pos, Type: typ,
            }
        } |
        TFunction methodname optionaltypeparams '(' typednamelist ')' returntypeannot {
            returns := $7.types
            if $7.known && returns == nil {
                returns = []ast.TypeExpr{}
            }
            typ := &ast.FunctionTypeExpr{TypeParams: $3, Params: toFuncParams($5), Returns: returns}
            typ.SetPosFromToken($1.Pos)
            if $7.end.Line != 0 {
                typ.SetLastPosFromToken($7.end)
            } else {
                typ.SetLastPosFromToken($6.Pos)
            }
            $$ = ast.InterfaceMember{
                Kind: ast.InterfaceMethodMember,
                Name: $2.Str, NamePosition: $2.Pos, Type: typ,
            }
        }

%%

// nameWithPos holds a name with its token position
type nameWithPos struct {
	Name string
	Pos  ast.Position
}

// splitNameList extracts names and positions from nameWithPos entries
func splitNameList(entries []nameWithPos) ([]string, []ast.Position) {
	names := make([]string, len(entries))
	positions := make([]ast.Position, len(entries))
	for i, e := range entries {
		names[i] = e.Name
		positions[i] = e.Pos
	}
	return names, positions
}

// typedNameEntry holds a name with optional type annotation
type typedNameEntry struct {
	Name string
	Pos  ast.Position
	Type ast.TypeExpr
}

// splitTypedNames extracts names, positions, and types from typed name entries
func splitTypedNames(entries []typedNameEntry) ([]string, []ast.Position, []ast.TypeExpr) {
	names := make([]string, len(entries))
	positions := make([]ast.Position, len(entries))
	var types []ast.TypeExpr
	hasTypes := false
	for i, e := range entries {
		names[i] = e.Name
		positions[i] = e.Pos
		if e.Type != nil {
			hasTypes = true
		}
	}
	if hasTypes {
		types = make([]ast.TypeExpr, len(entries))
		for i, e := range entries {
			types[i] = e.Type
		}
	}
	return names, positions, types
}

// toFuncParams converts typedNameEntry slice to FunctionParamExpr slice
func toFuncParams(entries []typedNameEntry) []ast.FunctionParamExpr {
	params := make([]ast.FunctionParamExpr, len(entries))
	for i, e := range entries {
		params[i] = ast.FunctionParamExpr{Name: e.Name, NamePosition: e.Pos, Type: e.Type}
	}
	return params
}

func TokenName(c int) string {
	if c >= TAnd && c-TAnd < len(yyToknames) {
		if yyToknames[c-TAnd] != "" {
			return yyToknames[c-TAnd]
		}
	}
    return string([]byte{byte(c)})
}
