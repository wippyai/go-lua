package generator

// This file deliberately contains no source or package loading code.  The
// ownership scanner deals in go/types values and uses this walker as its
// structural boundary.  In particular, do not use a type's package or name
// to decide who owns a fact: callers provide both Owner and Surface.

import (
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"sort"
	"strings"
)

var (
	ErrTypeWalkUnknown   = errors.New("link ownership: unknown go/types shape")
	ErrTypeWalkMalformed = errors.New("link ownership: malformed go/types value")
)

// universePackagePath is an explicit, impossible import path for objects in
// types.Universe. An empty package path is absence, never predeclared identity.
const universePackagePath = "<universe>"

// TypeWalkOptions is the complete context supplied to a walk.  Owner and
// Surface are opaque scanner identities; they are intentionally not derived
// from types.Named or types.Package.
type TypeWalkOptions struct {
	Owner         string
	Surface       string
	Mode          TypeWalkMode
	OpenNamedRoot bool
}

type TypeWalkMode uint8

const (
	WalkModeState TypeWalkMode = iota
	WalkModeReference
)

type FieldFact struct {
	Owner, Surface string
	Path, Name     string
	Type           string
	Embedded       bool
	Position       token.Pos
}

type ContainerFact struct {
	Owner, Surface string
	Path, Kind     string
	Type           string
	Element        string
	Key, Value     string
	Direction      string
	// ArrayLen is canonical for array containers and zero for all other
	// container kinds. The kind distinguishes a legitimate [0] length.
	ArrayLen int64
}

type ReferenceFact struct {
	Owner, Surface string
	Path, Kind     string
	Type           string
	Position       token.Pos
	// NamedPackagePath and NamedName are the canonical declaration identity
	// for a named type encountered by the walker. They are strings rather than
	// go/types objects so a stable projection can join the reference without
	// reparsing Type or retaining loader-owned pointers.
	NamedPackagePath       string
	NamedName              string
	NamedOriginPackagePath string
	NamedOriginName        string
	// MethodKey is a stable semantic identity for interface method references.
	// Named methods use their canonical receiver; anonymous methods use the
	// owning surface and structural source path. MethodPackagePath/MethodName
	// retain foreign and universe identity when no Link declaration is present.
	MethodKey         string
	MethodPackagePath string
	MethodName        string
	MethodReceiver    string
	MethodSource      string
}

type SurfaceRootFact struct {
	Owner, Surface string
	ParentSurface  string
	Path, Kind     string
	Position       token.Pos
	Type           string
}

type CycleFact struct {
	Owner, Surface string
	Path, Type     string
}

// The fact types keep owner and surface on every row so facts can be merged
// without relying on ambient scanner state.
type TypeWalkFacts struct {
	Owner, Surface string
	Fields         []FieldFact
	Containers     []ContainerFact
	References     []ReferenceFact
	Cycles         []CycleFact
	SurfaceRoots   []SurfaceRootFact
}

// WalkType walks one explicit owner/surface root. A named child is recorded as
// a reference and stopped; callers walk declared named types separately.
func WalkType(root types.Type, options TypeWalkOptions) (TypeWalkFacts, error) {
	return walkType(root, options)
}

// walkType is kept unexported so scanner code can use a strongly typed call
// without making the exported convenience wrapper part of its API.

func walkType(root types.Type, options TypeWalkOptions) (facts TypeWalkFacts, err error) {
	owner, surface := options.Owner, options.Surface
	facts.Owner, facts.Surface = owner, surface
	if owner == "" || surface == "" {
		return facts, fmt.Errorf("%w: owner and surface are required", ErrTypeWalkMalformed)
	}
	if root == nil {
		return facts, fmt.Errorf("%w: nil root", ErrTypeWalkMalformed)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			facts = TypeWalkFacts{Owner: owner, Surface: surface}
			if walkErr, ok := recovered.(error); ok {
				err = walkErr
			} else {
				err = fmt.Errorf("%w: %v", ErrTypeWalkMalformed, recovered)
			}
		}
	}()
	w := walker{
		owner: owner, surface: surface, baseSurface: surface, openNamedRoot: options.OpenNamedRoot,
		active:    make(map[string]bool),
		fieldSeen: make(map[string]struct{}), containerSeen: make(map[string]struct{}),
		referenceSeen: make(map[string]struct{}), cycleSeen: make(map[string]struct{}), surfaceRootSeen: make(map[string]struct{}),
	}
	if options.Mode == WalkModeReference {
		w.referenceWalk(root, "", true, w.surface)
	} else if options.Mode == WalkModeState {
		named, ok := unalias(root).(*types.Named)
		if !ok || named == nil {
			return facts, fmt.Errorf("%w: state mode requires a named root", ErrTypeWalkMalformed)
		}
		if _, ok := named.Underlying().(*types.Struct); !ok {
			return facts, fmt.Errorf("%w: state mode requires a named struct root", ErrTypeWalkMalformed)
		}
		w.stateRoot = map[string]struct{}{canonicalType(named): {}}
		if origin := named.Origin(); origin != nil {
			w.stateRoot[canonicalType(origin)] = struct{}{}
		}
		w.stateWalk(named, "", true, w.surface, token.Pos(0))
	} else {
		return facts, fmt.Errorf("%w: unknown walk mode %d", ErrTypeWalkMalformed, options.Mode)
	}
	facts.Fields, facts.Containers, facts.References, facts.Cycles, facts.SurfaceRoots = w.fields, w.containers, w.references, w.cycles, w.surfaceRoots
	sort.Slice(facts.Fields, func(i, j int) bool {
		return fieldLess(facts.Fields[i], facts.Fields[j])
	})
	sort.Slice(facts.Containers, func(i, j int) bool {
		return containerLess(facts.Containers[i], facts.Containers[j])
	})
	sort.Slice(facts.References, func(i, j int) bool {
		return referenceLess(facts.References[i], facts.References[j])
	})
	sort.Slice(facts.Cycles, func(i, j int) bool {
		return cycleLess(facts.Cycles[i], facts.Cycles[j])
	})
	sort.Slice(facts.SurfaceRoots, func(i, j int) bool {
		return surfaceRootLess(facts.SurfaceRoots[i], facts.SurfaceRoots[j])
	})
	return facts, nil
}

type walker struct {
	owner, surface  string
	baseSurface     string
	openNamedRoot   bool
	stateRoot       map[string]struct{}
	active          map[string]bool
	fields          []FieldFact
	containers      []ContainerFact
	references      []ReferenceFact
	cycles          []CycleFact
	surfaceRoots    []SurfaceRootFact
	fieldSeen       map[string]struct{}
	containerSeen   map[string]struct{}
	referenceSeen   map[string]struct{}
	cycleSeen       map[string]struct{}
	surfaceRootSeen map[string]struct{}
}

func (w *walker) referenceWalk(input types.Type, path string, root bool, surface string) {
	t := unalias(input)
	if t == nil {
		w.fail(fmt.Errorf("%w: nil child at %q", ErrTypeWalkMalformed, path))
	}
	identity := typeIdentity(t)
	if w.active[identity] {
		w.addCycle(surface, path, canonicalType(t))
		return
	}
	w.active[identity] = true
	defer delete(w.active, identity)

	switch value := t.(type) {
	case *types.Basic:
		return
	case *types.Pointer:
		w.addReference(surface, path, "pointer", canonicalType(t), token.Pos(0))
		w.referenceWalk(value.Elem(), path+".*", false, surface)
	case *types.Array:
		w.addContainer(surface, ContainerFact{Owner: w.owner, Surface: surface, Path: path, Kind: "array", Type: canonicalType(t), Element: canonicalType(value.Elem()), ArrayLen: value.Len()})
		w.referenceWalk(value.Elem(), path+"[]", false, surface)
	case *types.Slice:
		w.addContainer(surface, ContainerFact{Owner: w.owner, Surface: surface, Path: path, Kind: "slice", Type: canonicalType(t), Element: canonicalType(value.Elem())})
		w.referenceWalk(value.Elem(), path+"[]", false, surface)
	case *types.Chan:
		w.addContainer(surface, ContainerFact{Owner: w.owner, Surface: surface, Path: path, Kind: "chan", Direction: chanDir(value.Dir()), Type: canonicalType(t), Element: canonicalType(value.Elem())})
		w.referenceWalk(value.Elem(), path+"<-", false, surface)
	case *types.Map:
		w.addContainer(surface, ContainerFact{Owner: w.owner, Surface: surface, Path: path, Kind: "map", Type: canonicalType(t), Key: canonicalType(value.Key()), Value: canonicalType(value.Elem())})
		// Both sides are intentional. Map keys frequently carry authority and
		// values frequently carry row/reference evidence.
		w.referenceWalk(value.Key(), path+"{key}", false, surface)
		w.referenceWalk(value.Elem(), path+"{value}", false, surface)
	case *types.Named:
		w.addNamedReference(surface, path, "named", value, token.Pos(0))
		origin := value.Origin()
		if origin == nil {
			w.fail(fmt.Errorf("%w: named type has nil origin", ErrTypeWalkMalformed))
		}
		if origin != value {
			w.addNamedReference(surface, path+"{origin}", "named-origin", origin, token.Pos(0))
		}
		if params := value.TypeParams(); params != nil {
			w.typeParamList(params, path+"{typeparam}", surface)
		}
		if args := value.TypeArgs(); args != nil {
			for i := 0; i < args.Len(); i++ {
				w.referenceWalk(args.At(i), fmt.Sprintf("%s[typearg:%d]", path, i), false, surface)
			}
		}
		if root && w.openNamedRoot {
			if iface, ok := unalias(value.Underlying()).(*types.Interface); ok {
				w.referenceInterface(iface, path, surface, namedReceiverIdentity(value))
			} else {
				w.referenceWalk(value.Underlying(), path, true, surface)
			}
		}
	case *types.Struct:
		if !root {
			w.addReference(surface, path, "anonymous-struct", canonicalType(t), token.Pos(0))
		}
		for i := 0; i < value.NumFields(); i++ {
			field := value.Field(i)
			if field == nil || field.Type() == nil {
				w.fail(fmt.Errorf("%w: malformed struct field %d", ErrTypeWalkMalformed, i))
			}
			fieldPath := joinPath(path, field.Name())
			w.addTypeReference(surface, fieldPath, "member", field.Type(), field.Pos())
			w.referenceWalk(field.Type(), fieldPath, false, surface)
		}
	case *types.Interface:
		w.referenceInterface(value, path, surface, "")
	case *types.Signature:
		w.addReference(surface, path, "signature", canonicalType(t), token.NoPos)
		if recv := value.Recv(); recv != nil {
			w.referenceWalk(recv.Type(), path+"(recv)", false, surface)
		}
		if params := value.RecvTypeParams(); params != nil {
			w.typeParamList(params, path+"(recvtype)", surface)
		}
		if params := value.TypeParams(); params != nil {
			w.typeParamList(params, path+"(type)", surface)
		}
		w.discoveryTuple(value.Params(), path+"(params)", surface)
		w.discoveryTuple(value.Results(), path+"(results)", surface)
	case *types.Tuple:
		w.discoveryTuple(value, path, surface)
	case *types.TypeParam:
		w.addReference(surface, path, "type-param", canonicalType(t), token.Pos(0))
		w.constraint(value.Constraint(), path+"(constraint)", surface)
	case *types.Union:
		for i := 0; i < value.Len(); i++ {
			term := value.Term(i)
			if term == nil || term.Type() == nil {
				w.fail(fmt.Errorf("%w: malformed union term %d", ErrTypeWalkMalformed, i))
			}
			kind := "union-term"
			if term.Tilde() {
				kind = "union-tilde-term"
			}
			w.addTypeReference(surface, fmt.Sprintf("%s[term:%d]", path, i), kind, term.Type(), token.Pos(0))
			w.referenceWalk(term.Type(), fmt.Sprintf("%s[term:%d]", path, i), false, surface)
		}
	default:
		w.fail(fmt.Errorf("%w: %T", ErrTypeWalkUnknown, input))
	}
}

// referenceInterface retains named declaration authority when receiver is
// non-empty. Methods of an anonymous interface have no DeclarationInfo owner;
// they remain closed structural evidence on the containing path.
func (w *walker) referenceInterface(value *types.Interface, path, surface, receiver string) {
	if value == nil {
		w.fail(fmt.Errorf("%w: nil interface at %q", ErrTypeWalkMalformed, path))
	}
	value.Complete()
	for i := 0; i < value.NumExplicitMethods(); i++ {
		method := value.ExplicitMethod(i)
		kind := "interface-method-local"
		source := interfaceMethodSource(surface, path)
		if receiver != "" {
			kind = "method"
			source = ""
		}
		methodPath := joinPath(path, method.Name())
		w.addMethodReference(surface, methodPath, kind, method, receiver, source)
		w.referenceWalk(method.Type(), methodPath, false, surface)
	}
	for i := 0; i < value.NumEmbeddeds(); i++ {
		w.embeddedReference(value.EmbeddedType(i), fmt.Sprintf("%s{embedded:%d}", path, i), surface)
	}
}

// embeddedReference follows an embedded interface's original declarations.
// Its methods are references on the embedding surface, never declarations
// there; this preserves a single declaration row for join construction.
func (w *walker) embeddedReference(input types.Type, path, surface string) {
	t := unalias(input)
	if t == nil {
		w.fail(fmt.Errorf("%w: nil embedded interface at %q", ErrTypeWalkMalformed, path))
	}
	identity := "embedded:" + typeIdentity(t)
	if w.active[identity] {
		w.addCycle(surface, path, canonicalType(t))
		return
	}
	w.active[identity] = true
	defer delete(w.active, identity)
	receiver := ""
	if named, ok := t.(*types.Named); ok {
		w.addNamedReference(surface, path, "named", named, token.Pos(0))
		receiver = namedReceiverIdentity(named)
		t = named.Underlying()
	}
	iface, ok := t.(*types.Interface)
	if !ok {
		w.referenceWalk(t, path, false, surface)
		return
	}
	iface.Complete()
	for i := 0; i < iface.NumExplicitMethods(); i++ {
		method := iface.ExplicitMethod(i)
		methodPath := joinPath(path, method.Name())
		kind := "interface-method-local"
		source := interfaceMethodSource(surface, path)
		if receiver != "" {
			kind = "method-reference"
			source = ""
		}
		w.addMethodReference(surface, methodPath, kind, method, receiver, source)
		w.referenceWalk(method.Type(), methodPath, false, surface)
	}
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		w.embeddedReference(iface.EmbeddedType(i), fmt.Sprintf("%s{embedded:%d}", path, i), surface)
	}
}

// stateWalk follows only declared struct storage. Named children are emitted
// as references and must be walked as their own declared roots; anonymous
// composites below stored fields receive independent synthetic surfaces.
func (w *walker) stateWalk(input types.Type, path string, root bool, surface string, position token.Pos) {
	t := unalias(input)
	if t == nil {
		w.fail(fmt.Errorf("%w: nil state child at %q", ErrTypeWalkMalformed, path))
	}
	identity := "state:" + typeIdentity(t)
	if w.active[identity] {
		w.addCycle(surface, path, canonicalType(t))
		return
	}
	w.active[identity] = true
	defer delete(w.active, identity)
	switch value := t.(type) {
	case *types.Named:
		w.addNamedReference(surface, path, "named", value, position)
		if args := value.TypeArgs(); args != nil {
			for i := 0; i < args.Len(); i++ {
				w.referenceWalk(args.At(i), fmt.Sprintf("%s[typearg:%d]", path, i), false, surface)
			}
		}
		if !root {
			_, sameRoot := w.stateRoot[canonicalType(value)]
			if !sameRoot && value.Origin() != nil {
				_, sameRoot = w.stateRoot[canonicalType(value.Origin())]
			}
			if sameRoot {
				w.addCycle(surface, path, canonicalType(t))
			}
			return
		}
		w.stateWalk(value.Underlying(), path, true, surface, position)
	case *types.Pointer:
		w.addReference(surface, path, "pointer", canonicalType(t), token.Pos(0))
		w.stateWalk(value.Elem(), path+".*", false, surface, position)
	case *types.Array:
		w.addContainer(surface, ContainerFact{Owner: w.owner, Surface: surface, Path: path, Kind: "array", Type: canonicalType(t), Element: canonicalType(value.Elem()), ArrayLen: value.Len()})
		w.stateWalk(value.Elem(), path+"[]", false, surface, position)
	case *types.Slice:
		w.addContainer(surface, ContainerFact{Owner: w.owner, Surface: surface, Path: path, Kind: "slice", Type: canonicalType(t), Element: canonicalType(value.Elem())})
		w.stateWalk(value.Elem(), path+"[]", false, surface, position)
	case *types.Chan:
		w.addContainer(surface, ContainerFact{Owner: w.owner, Surface: surface, Path: path, Kind: "chan", Direction: chanDir(value.Dir()), Type: canonicalType(t), Element: canonicalType(value.Elem())})
		w.stateWalk(value.Elem(), path+"<-", false, surface, position)
	case *types.Map:
		w.addContainer(surface, ContainerFact{Owner: w.owner, Surface: surface, Path: path, Kind: "map", Type: canonicalType(t), Key: canonicalType(value.Key()), Value: canonicalType(value.Elem())})
		w.stateWalk(value.Key(), path+"{key}", false, surface, position)
		w.stateWalk(value.Elem(), path+"{value}", false, surface, position)
	case *types.Struct:
		if !root {
			parentSurface := surface
			surface = syntheticSurface(w.baseSurface, path)
			w.addSurfaceRoot(SurfaceRootFact{Owner: w.owner, Surface: surface, ParentSurface: parentSurface, Path: path, Kind: "anonymous-state", Position: position, Type: canonicalType(t)})
		}
		fieldSurface := surface
		for i := 0; i < value.NumFields(); i++ {
			field := value.Field(i)
			if field == nil || field.Type() == nil {
				w.fail(fmt.Errorf("%w: malformed struct field %d", ErrTypeWalkMalformed, i))
			}
			fieldPath := joinPath(path, field.Name())
			w.addField(FieldFact{Owner: w.owner, Surface: fieldSurface, Path: fieldPath, Name: field.Name(), Type: canonicalType(field.Type()), Embedded: field.Embedded(), Position: field.Pos()})
			child := unalias(field.Type())
			switch child.(type) {
			case *types.Signature, *types.Interface:
				w.referenceWalk(child, fieldPath, false, fieldSurface)
			default:
				w.stateWalk(child, fieldPath, false, surface, field.Pos())
			}
		}
	case *types.Basic:
		return
	default:
		w.referenceWalk(t, path, false, surface)
	}
}

// constraint preserves union/interface evidence from a type parameter's
// declared constraint, but does not open arbitrary named representations.
// This is the one intentional named-underlying exception: constraint terms
// are part of the generic declaration's evidence, not the foreign type's
// component state.
func (w *walker) constraint(input types.Type, path, surface string) {
	t := unalias(input)
	if named, ok := t.(*types.Named); ok {
		w.addNamedReference(surface, path, "named", named, token.Pos(0))
		return
	}
	w.referenceWalk(t, path, false, surface)
}

func (w *walker) typeParamList(list *types.TypeParamList, path, surface string) {
	for i := 0; i < list.Len(); i++ {
		param := list.At(i)
		if param == nil {
			w.fail(fmt.Errorf("%w: nil type parameter %d", ErrTypeWalkMalformed, i))
		}
		w.referenceWalk(param, fmt.Sprintf("%s[%d]", path, i), false, surface)
	}
}

func (w *walker) discoveryTuple(tuple *types.Tuple, path, surface string) {
	if tuple == nil {
		// go/types represents an empty result list on a function signature as
		// a nil Tuple. It is an empty shape, not malformed input.
		return
	}
	w.addReference(surface, path, "tuple", canonicalType(tuple), token.NoPos)
	for i := 0; i < tuple.Len(); i++ {
		variable := tuple.At(i)
		if variable == nil || variable.Type() == nil {
			w.fail(fmt.Errorf("%w: malformed tuple variable %d", ErrTypeWalkMalformed, i))
		}
		w.referenceWalk(variable.Type(), fmt.Sprintf("%s[%d]", path, i), false, surface)
	}
}

func (w *walker) fail(err error) { panic(err) }

func (w *walker) addField(f FieldFact) {
	key := strings.Join([]string{f.Owner, f.Surface, f.Path, f.Name, f.Type, fmt.Sprint(f.Embedded)}, "\x00")
	if _, ok := w.fieldSeen[key]; ok {
		return
	}
	w.fieldSeen[key] = struct{}{}
	w.fields = append(w.fields, f)
}

func (w *walker) addSurfaceRoot(f SurfaceRootFact) {
	key := strings.Join([]string{f.Owner, f.Surface, f.ParentSurface, f.Path, f.Kind, f.Type}, "\x00")
	if _, ok := w.surfaceRootSeen[key]; ok {
		return
	}
	w.surfaceRootSeen[key] = struct{}{}
	w.surfaceRoots = append(w.surfaceRoots, f)
}

func (w *walker) addContainer(_ string, f ContainerFact) {
	key := strings.Join([]string{f.Owner, f.Surface, f.Path, f.Kind, f.Direction, f.Type, f.Element, f.Key, f.Value, fmt.Sprint(f.ArrayLen)}, "\x00")
	if _, ok := w.containerSeen[key]; ok {
		return
	}
	w.containerSeen[key] = struct{}{}
	w.containers = append(w.containers, f)
}

func (w *walker) addReference(surface, path, kind, typ string, position token.Pos) {
	f := ReferenceFact{Owner: w.owner, Surface: surface, Path: path, Kind: kind, Type: typ, Position: position}
	w.addReferenceFact(f)
}

func (w *walker) addTypeReference(surface, path, kind string, typ types.Type, position token.Pos) {
	if typ == nil {
		w.fail(fmt.Errorf("%w: nil reference type at %q", ErrTypeWalkMalformed, path))
	}
	f := ReferenceFact{Owner: w.owner, Surface: surface, Path: path, Kind: kind, Type: canonicalType(typ), Position: position}
	if named, ok := unalias(typ).(*types.Named); ok {
		f.NamedPackagePath, f.NamedName = namedIdentity(named)
		if origin := named.Origin(); origin != nil {
			f.NamedOriginPackagePath, f.NamedOriginName = namedIdentity(origin)
		}
	}
	w.addReferenceFact(f)
}

func (w *walker) addMethodReference(surface, path, kind string, method *types.Func, receiver, source string) {
	if method == nil || method.Type() == nil {
		w.fail(fmt.Errorf("%w: nil method reference at %q", ErrTypeWalkMalformed, path))
	}
	if source == "" {
		receiver = methodReceiverEvidence(method, receiver)
	}
	position := method.Pos()
	packagePath := universePackagePath
	if method.Pkg() != nil {
		packagePath = method.Pkg().Path()
	}
	if packagePath != w.owner {
		// A foreign method's token.Pos belongs to another package FileSet. It
		// is not valid provenance for this owner; retain exact object identity
		// while making the absent local coordinate explicit.
		position = token.NoPos
	}
	f := ReferenceFact{
		Owner: w.owner, Surface: surface, Path: path, Kind: kind,
		Type: canonicalType(method.Type()), Position: position,
		MethodKey: methodObjectKey(method, receiver, source), MethodName: method.Name(),
		MethodPackagePath: packagePath, MethodReceiver: receiver, MethodSource: source,
	}
	w.addReferenceFact(f)
}

func (w *walker) addNamedReference(surface, path, kind string, typ *types.Named, position token.Pos) {
	if typ == nil {
		w.fail(fmt.Errorf("%w: nil named reference at %q", ErrTypeWalkMalformed, path))
	}
	f := ReferenceFact{Owner: w.owner, Surface: surface, Path: path, Kind: kind, Type: canonicalType(typ), Position: position}
	f.NamedPackagePath, f.NamedName = namedIdentity(typ)
	if origin := typ.Origin(); origin != nil {
		f.NamedOriginPackagePath, f.NamedOriginName = namedIdentity(origin)
	}
	w.addReferenceFact(f)
}

func (w *walker) addReferenceFact(f ReferenceFact) {
	key := strings.Join([]string{f.Owner, f.Surface, f.Path, f.Kind, f.Type, f.NamedPackagePath, f.NamedName, f.NamedOriginPackagePath, f.NamedOriginName, f.MethodKey, f.MethodPackagePath, f.MethodName, f.MethodReceiver, f.MethodSource}, "\x00")
	if _, ok := w.referenceSeen[key]; ok {
		return
	}
	w.referenceSeen[key] = struct{}{}
	w.references = append(w.references, f)
}

func namedIdentity(named *types.Named) (string, string) {
	if named == nil || named.Obj() == nil {
		return "", ""
	}
	object := named.Obj()
	if object.Pkg() == nil {
		return universePackagePath, object.Name()
	}
	return object.Pkg().Path(), object.Name()
}

func namedReceiverIdentity(named *types.Named) string {
	if named == nil {
		return ""
	}
	if origin := named.Origin(); origin != nil {
		named = origin
	}
	packagePath, name := namedIdentity(named)
	if packagePath == "" || name == "" {
		return ""
	}
	return packagePath + "." + name
}

func methodReceiverEvidence(method *types.Func, declaredReceiver string) string {
	if declaredReceiver != "" {
		return declaredReceiver
	}
	if method != nil {
		if signature, ok := method.Type().(*types.Signature); ok {
			if receiver := signature.Recv(); receiver != nil && receiver.Type() != nil {
				if named, ok := unalias(receiver.Type()).(*types.Named); ok {
					return namedReceiverIdentity(named)
				}
				return canonicalType(receiver.Type())
			}
		}
	}
	return declaredReceiver
}

func interfaceMethodSource(surface, path string) string {
	return "interface:" + surface + "\x00" + path
}

func methodObjectKey(method *types.Func, receiver, source string) string {
	if method == nil {
		return ""
	}
	if origin := method.Origin(); origin != nil {
		method = origin
	}
	packagePath := universePackagePath
	if method.Pkg() != nil {
		packagePath = method.Pkg().Path()
	}
	return strings.Join([]string{packagePath, method.Name(), canonicalType(method.Type()), receiver, source}, "\x00")
}

func (w *walker) addCycle(surface, path, typ string) {
	f := CycleFact{Owner: w.owner, Surface: surface, Path: path, Type: typ}
	key := strings.Join([]string{f.Owner, f.Surface, f.Path, f.Type}, "\x00")
	if _, ok := w.cycleSeen[key]; ok {
		return
	}
	w.cycleSeen[key] = struct{}{}
	w.cycles = append(w.cycles, f)
}

func unalias(t types.Type) types.Type {
	if t == nil {
		return nil
	}
	return types.Unalias(t)
}

func canonicalType(t types.Type) string {
	if t == nil {
		return "<nil>"
	}
	t = types.Unalias(t)
	return types.TypeString(t, func(pkg *types.Package) string {
		if pkg == nil {
			return ""
		}
		return pkg.Path()
	})
}

func typeIdentity(t types.Type) string {
	if t == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T:%s", t, canonicalType(t))
}

func joinPath(parent, child string) string {
	if parent == "" {
		return child
	}
	if child == "" {
		return parent
	}
	return parent + "." + child
}

func syntheticSurface(parent, path string) string {
	return parent + "#" + path
}

func chanDir(dir types.ChanDir) string {
	switch dir {
	case types.SendRecv:
		return "chan"
	case types.SendOnly:
		return "chan<-"
	case types.RecvOnly:
		return "<-chan"
	default:
		return "chan?"
	}
}

func fieldLess(a, b FieldFact) bool {
	return compare(a.Owner, b.Owner, a.Surface, b.Surface, a.Path, b.Path, a.Name, b.Name, a.Type, b.Type, fmt.Sprint(a.Embedded), fmt.Sprint(b.Embedded)) < 0
}
func containerLess(a, b ContainerFact) bool {
	return compare(a.Owner, b.Owner, a.Surface, b.Surface, a.Path, b.Path, a.Kind, b.Kind, a.Type, b.Type, a.Element, b.Element, a.Key, b.Key, a.Value, b.Value, a.Direction, b.Direction, fmt.Sprint(a.ArrayLen), fmt.Sprint(b.ArrayLen)) < 0
}
func referenceLess(a, b ReferenceFact) bool {
	return compare(a.Owner, b.Owner, a.Surface, b.Surface, a.Path, b.Path, a.Kind, b.Kind, a.Type, b.Type,
		a.NamedPackagePath, b.NamedPackagePath,
		a.NamedName, b.NamedName, a.NamedOriginPackagePath, b.NamedOriginPackagePath,
		a.NamedOriginName, b.NamedOriginName, a.MethodKey, b.MethodKey,
		a.MethodPackagePath, b.MethodPackagePath, a.MethodName, b.MethodName,
		a.MethodReceiver, b.MethodReceiver, a.MethodSource, b.MethodSource) < 0
}
func cycleLess(a, b CycleFact) bool {
	return compare(a.Owner, b.Owner, a.Surface, b.Surface, a.Path, b.Path, a.Type, b.Type) < 0
}
func surfaceRootLess(a, b SurfaceRootFact) bool {
	return compare(a.Owner, b.Owner, a.Surface, b.Surface, a.ParentSurface, b.ParentSurface, a.Path, b.Path, a.Kind, b.Kind, a.Type, b.Type) < 0
}
func compare(values ...string) int {
	for i := 0; i+1 < len(values); i += 2 {
		if values[i] < values[i+1] {
			return -1
		}
		if values[i] > values[i+1] {
			return 1
		}
	}
	return 0
}
