// Package gorewrite provides deliberately narrow, type-aware mechanical Go
// rewrites for flash refactors.  It never infers semantic ownership: callers
// supply an exact mapping and it rejects forms whose evaluation or binding it
// cannot preserve.
package gorewrite

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	pathpkg "path"
	"strings"
)

// SelectorClass describes the binding represented by a selector.  Only
// FieldSelection is eligible for a field-route rewrite.
type SelectorClass uint8

const (
	SelectorUnknown SelectorClass = iota
	FieldSelection
	MethodInvocation
	MethodValue
	MethodExpression
	InterfaceMethod
	PackageSelection
)

func (c SelectorClass) String() string {
	switch c {
	case FieldSelection:
		return "field"
	case MethodInvocation:
		return "method-invocation"
	case MethodValue:
		return "method-value"
	case MethodExpression:
		return "method-expression"
	case InterfaceMethod:
		return "interface-method"
	case PackageSelection:
		return "package-selection"
	default:
		return "unknown"
	}
}

// RoutePlan is the complete, one-file rewrite authority. Consumer is exact:
// applying a plan to another parsed file is an error even if it has identical
// text.  The package/type objects bind the plan to the type-checker epoch that
// produced its lock, rather than to a spelling which might be shadowed.
type RoutePlan struct {
	Consumer *ast.File
	Imports  []ImportBinding
	Members  []MemberBinding
}

// ImportForm is the finite import-edit grammar. It deliberately does not have
// an inferred or "ensure" mode: additions, removals, and replacements are
// separate locked facts.
type ImportForm uint8

const (
	ImportUnknown ImportForm = iota
	ImportReplace
	ImportAdd
	ImportRemove
)

func (f ImportForm) String() string {
	switch f {
	case ImportReplace:
		return "replace"
	case ImportAdd:
		return "add"
	case ImportRemove:
		return "remove"
	default:
		return "unknown"
	}
}

// FuturePackage names one exact package expected after the transaction. It is
// intentionally a descriptor, not a current *types.Package: a flash cut can
// create the target package and the structured post-state resolver must prove
// this descriptor exists before commit.
type FuturePackage struct {
	Path string
	Name string
}

func (p FuturePackage) validate() error {
	if p.Path == "" || strings.TrimSpace(p.Path) != p.Path || strings.HasPrefix(p.Path, "/") || pathpkg.Clean(p.Path) != p.Path || p.Path == "." || p.Path == ".." || strings.HasPrefix(p.Path, "../") || strings.Contains(p.Path, "/../") || p.Path == "C" || p.Path == "reflect" || p.Path == "unsafe" {
		return fmt.Errorf("future package has unsafe or non-canonical import path")
	}
	if p.Name == "" || !token.IsIdentifier(p.Name) || p.Name == "_" || p.Name == "." {
		return fmt.Errorf("future package requires exact path and declared package name")
	}
	return nil
}

// FutureMember names one exact member expected on the post-cut authority.
// Its owner is determined by the ImportBinding or containment path in the
// surrounding MemberBinding, never by a name search.
type FutureMember struct {
	Name string
}

func (m FutureMember) validate() error {
	if m.Name == "" || !token.IsIdentifier(m.Name) || m.Name == "_" {
		return fmt.Errorf("future member requires an exact identifier")
	}
	return nil
}

// ImportBinding changes one import in RoutePlan.Consumer. Replace and remove
// retain a resolved source PkgName object; add has no source and instead names
// an exact future package. Alias is raw Go syntax: empty means emit an
// implicit import (ImportSpec.Name == nil), while a non-empty value is an
// explicit alias. The effective qualifier used for collisions and selectors is
// Alias when present, otherwise Target.Name. Dot and blank imports are never
// routable.
type ImportBinding struct {
	Form   ImportForm
	From   *types.PkgName
	Target FuturePackage
	Alias  string
}

func (b ImportBinding) validate() error {
	if b.Alias != "" && (!token.IsIdentifier(b.Alias) || b.Alias == "_" || b.Alias == ".") {
		return fmt.Errorf("import binding has invalid alias %q", b.Alias)
	}
	switch b.Form {
	case ImportReplace:
		if b.From == nil || b.From.Imported() == nil {
			return fmt.Errorf("import replacement requires resolved source package name")
		}
		if err := b.Target.validate(); err != nil {
			return err
		}
	case ImportAdd:
		if b.From != nil {
			return fmt.Errorf("import add cannot have a source package binding")
		}
		if err := b.Target.validate(); err != nil {
			return err
		}
	case ImportRemove:
		if b.From == nil || b.From.Imported() == nil {
			return fmt.Errorf("import removal requires resolved source package name")
		}
		if b.Target.Path != "" || b.Target.Name != "" || b.Alias != "" {
			return fmt.Errorf("import removal cannot declare a target or alias")
		}
	default:
		return fmt.Errorf("unknown import binding form %s", b.Form)
	}
	return nil
}

func (b ImportBinding) effectiveAlias() string {
	if b.Alias != "" {
		return b.Alias
	}
	return b.Target.Name
}

// MemberForm is a deliberately finite selector rewrite grammar. There is no
// catch-all expression rewrite, bridge, wrapper, or interface dispatch case.
type MemberForm uint8

const (
	MemberUnknown MemberForm = iota
	MemberField
	MemberDirectMethodCall
	MemberPackageSelector
)

func (f MemberForm) String() string {
	switch f {
	case MemberField:
		return "field"
	case MemberDirectMethodCall:
		return "direct-method-call"
	case MemberPackageSelector:
		return "package-selector"
	default:
		return "unknown"
	}
}

// ReceiverForm defines the only supported future containment edges.
type ReceiverForm uint8

const (
	ReceiverUnknown ReceiverForm = iota
	ReceiverField
	ReceiverDirectView
)

func (f ReceiverForm) String() string {
	switch f {
	case ReceiverField:
		return "field"
	case ReceiverDirectView:
		return "direct-view"
	default:
		return "unknown"
	}
}

// ReceiverStep is one exact future containment edge. The post-state resolver
// proves the target field/view exists and that a direct view has its required
// zero-argument, single-result signature. The original receiver AST is reused
// exactly once; this descriptor does not invent an evaluation or a bridge.
type ReceiverStep struct {
	Form ReceiverForm
	Name string
}

func (s ReceiverStep) validate() error {
	if (s.Form != ReceiverField && s.Form != ReceiverDirectView) || s.Name == "" || !token.IsIdentifier(s.Name) || s.Name == "_" {
		return fmt.Errorf("receiver step requires a field or direct-view exact identifier")
	}
	return nil
}

// MemberBinding maps one resolved source member object to an exact future
// member descriptor. For field and method-call forms, Via is the containment
// route introduced between receiver and terminal member. For package-selector
// form, Package is the resolved old import name redirected by this RoutePlan.
//
// Method values, method expressions, and interface dispatch are intentionally
// absent. If a moved method occurs in one of those forms, the rewrite rejects
// the whole plan rather than preserving authority through an adapter.
type MemberBinding struct {
	Form    MemberForm
	From    types.Object
	Target  FutureMember
	Package *types.PkgName
	Via     []ReceiverStep
}

func (b MemberBinding) validate() error {
	if b.From == nil {
		return fmt.Errorf("member binding requires a resolved source object")
	}
	if err := b.Target.validate(); err != nil {
		return err
	}
	switch b.Form {
	case MemberField:
		if _, ok := b.From.(*types.Var); !ok {
			return fmt.Errorf("field binding source %s is not a field", b.From.Name())
		}
		if b.Package != nil || len(b.Via) == 0 {
			return fmt.Errorf("field binding requires a containment path and no package")
		}
	case MemberDirectMethodCall:
		if _, ok := b.From.(*types.Func); !ok {
			return fmt.Errorf("method binding source %s is not a method", b.From.Name())
		}
		if b.Package != nil || len(b.Via) == 0 {
			return fmt.Errorf("method binding requires a containment path and no package")
		}
	case MemberPackageSelector:
		if b.Package == nil || b.Package.Imported() == nil || len(b.Via) != 0 {
			return fmt.Errorf("package selector binding requires its source package and no receiver path")
		}
	default:
		return fmt.Errorf("unknown member binding form %s", b.Form)
	}
	for _, step := range b.Via {
		if err := step.validate(); err != nil {
			return err
		}
	}
	return nil
}

// FieldRelocation carries the declaration-local ownership edit. It is kept
// separate from object-bound member routing because declaration movement and
// consumer rewrites may have different transaction phases.
type FieldRelocation struct {
	Owner      string
	Child      string
	ChildField string
	Fields     map[string]struct{}
}

func (r FieldRelocation) validate() error {
	if r.Owner == "" || r.Child == "" || r.ChildField == "" || len(r.Fields) == 0 {
		return fmt.Errorf("field relocation requires owner, child, child field, and fields")
	}
	return nil
}

// IdentifierRename is object-keyed.  The type checker, rather than spelling,
// defines the rename boundary; shadowed local identifiers therefore remain
// untouched.
type IdentifierRename struct {
	Object types.Object
	To     string
}

// DeclarationSelector states exactly what may be removed from a source file.
// Names select type/var/const/function declarations; Methods selects receiver
// methods by their declared name; Tests selects top-level Test*/Benchmark*/
// Example* function names.  A declaration selected twice is an error.
type DeclarationSelector struct {
	Names   map[string]struct{}
	Methods map[string]struct{}
	Tests   map[string]struct{}
}

// ExtractionResult is a structural move, ready for the caller to format and
// write atomically.
type ExtractionResult struct {
	Moved []ast.Decl
}

// Residue is a remaining syntactic occurrence. It intentionally reports
// positions and kinds rather than claiming semantic equivalence.
type Residue struct {
	Pos  token.Position
	Kind string
	Text string
}

// Hazard is a construct that prevents a mechanical refactor from proving it
// has accounted for name-based access.
type Hazard struct {
	Pos       token.Position
	Kind      string
	Detail    string
	Authority bool
}
