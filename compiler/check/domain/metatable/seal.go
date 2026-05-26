package metatable

import (
	"os"

	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/typ"
)

// sealDbg gates the class-sealing diagnostics used while verifying that the
// producer seal fires on a structurally-cyclic class allocation.
var sealDbg = os.Getenv("SEALDBG") != ""

// indexField names the Lua metatable __index slot that holds a class's
// prototype back-edge.
const indexField = "__index"

// SealClassFamily ties a Lua OOP class type into a single recursive family.
//
// A setmetatable-backed class is structurally cyclic: the metatable __index
// back-edge and every method self parameter denote the same class allocation,
// yet the value domain rebuilds that allocation as fresh, one-level-deeper
// records on every inter-procedural fixpoint iteration. Without a single
// recursion variable the metatable nesting, the self-param placeholder union,
// and field precision drift each diverge independently and the fixpoint never
// converges.
//
// SealClassFamily owns ONE typ.Recursive (mu X) per class allocation. The owner
// key is the class symbol identity supplied by the checker, so distinct classes
// receive distinct recursion variables and never merge even when their metatable
// shapes look alike. Every class back-edge inside the body is rewritten to that
// single X, then the body is sealed, producing
//
//	mu X.{ __index: X, run: fun(self: {Metatable: X}) -> ..., new: fun() -> {Metatable: X}, ... }
//
// The class back-edge is the value of an __index field and the metatable of any
// instance/self record below the root: in a setmetatable class those edges
// always denote the class/prototype allocation, so rewriting them to X is sound.
// Sealing is sound only because the caller proves, by producer evidence (the
// class symbol that owns this type), that the type is a class allocation.
// Callers without that evidence must keep the existing convergence path.
func SealClassFamily(class typ.Type, ownerKey string) typ.Type {
	if class == nil || ownerKey == "" {
		return class
	}

	// An already-folded observation arrives as a recursive placeholder (the
	// value domain's Inferred fold) whose body is the class record. Re-seal the
	// body under the stable owner name and bind the old fold's self-references to
	// the new family, so every iteration's fresh fold collapses to one stable
	// family instead of accumulating distinct placeholders.
	if mu, ok := class.(*typ.Recursive); ok {
		if mu.Body == nil || mu.Body == mu {
			return class
		}
		root := classRootRecord(mu.Body)
		if root == nil {
			return class
		}
		return sealClassRoot(root, mu, ownerKey)
	}

	root := classRootRecord(class)
	if root == nil {
		return class
	}
	return sealClassRoot(root, nil, ownerKey)
}

// sealClassRoot seals a class root record into one recursive family. priorRec,
// when non-nil, is a value-domain fold whose self-references are rebound to the
// new family so the unstable fold collapses to the stable owner family.
func sealClassRoot(root *typ.Record, priorRec *typ.Recursive, ownerKey string) typ.Type {
	// A class record owns the family directly: its body is the sealed class. An
	// instance ({instance fields, Metatable: class}) references the same family
	// through its metatable. Both produce a mu node named for the owner; the
	// value-domain canonicalizer maps structurally equal builds to one
	// representative (keyed by the owner name plus structure), so a class and its
	// instances collapse to a single family and the fixpoint sees a finite
	// representative. Distinct owners get distinct names and never merge.
	switch {
	case classRecordHasBackEdge(root):
		rec := typ.NewRecursivePlaceholder(classFamilyName(ownerKey))
		r := &backEdgeRewriter{rec: rec, priorRec: priorRec, seen: make(map[typ.Type]typ.Type)}
		body := r.rewriteRecord(root, typ.NewGuard())
		rec.SetBody(body)
		if sealDbg {
			println("SEALDBG SEALING class owner=", ownerKey, " class=", typ.FormatShort(root))
		}
		return rec
	case instanceMetatableHasBackEdge(root):
		rec := typ.NewRecursivePlaceholder(classFamilyName(ownerKey))
		r := &backEdgeRewriter{rec: rec, priorRec: priorRec, seen: make(map[typ.Type]typ.Type)}
		sealed := r.rewriteInstance(root, typ.NewGuard())
		if sealDbg {
			println("SEALDBG SEALING instance owner=", ownerKey, " instance=", typ.FormatShort(root))
		}
		return sealed
	default:
		if sealDbg {
			println("SEALDBG no-back-edge class=", typ.FormatShort(root))
		}
		if priorRec != nil {
			return priorRec
		}
		return root
	}
}

// SealClassInstanceReturn seals a constructor's instance return into its class
// recursive family. It applies only to a plain instance record (a metatable
// class back-edge, no own __index, not already recursive), the shape a
// setmetatable constructor returns. Restricting to that shape keeps the seal
// from competing with the method self-type seal on already-folded returns.
func SealClassInstanceReturn(t typ.Type) typ.Type {
	root, ok := t.(*typ.Record)
	if !ok || !instanceMetatableHasBackEdge(root) {
		return t
	}
	owner := autoOwnerKey(t)
	if owner == "" {
		return t
	}
	return SealClassFamily(t, owner)
}

// SealClassFamilyAuto seals a class type or class instance when no explicit
// class symbol is available, deriving the owner key from the class metatable's
// field signature. Two values denoting the same class share that signature and
// seal to one family; the genuine __index/metatable back-edge is the producer
// evidence that the value is a class allocation, so this does not over-fold
// unrelated finite records. Returns the type unchanged when it carries no class
// back-edge.
func SealClassFamilyAuto(t typ.Type) typ.Type {
	owner := autoOwnerKey(t)
	if sealDbg {
		println("SEALDBG auto owner=", owner, " t=", typ.FormatShort(t))
	}
	if owner == "" {
		return t
	}
	return SealClassFamily(t, owner)
}

// autoOwnerKey derives a stable owner key from the class metatable signature of
// t (its own fields when t is a class record, or its metatable's fields when t
// is an instance). The signature is allocation-aligned: the same class produces
// the same key across observations.
func autoOwnerKey(t typ.Type) string {
	if mu, ok := t.(*typ.Recursive); ok {
		if mu.Body == nil || mu.Body == mu {
			return ""
		}
		return autoOwnerKey(mu.Body)
	}
	root, ok := unwrapRecord(t)
	if !ok || root == nil {
		return ""
	}
	// The recursion-variable name is a stable constant rather than the observed
	// method set: a class's method surface grows across fixpoint iterations
	// (run appears after new), so a method-derived name would drift and the same
	// class would seal to distinct families that never reconcile. With a constant
	// name, the value-domain canonicalizer keeps distinct classes apart by the
	// structural family verifier while folding the same class's iterations to one
	// representative once its body widens to a fixed point.
	if classRecordHasBackEdge(root) || instanceMetatableHasBackEdge(root) {
		return autoOwnerName
	}
	return ""
}

// autoOwnerName is the constant recursion-variable owner used when sealing
// without an explicit class symbol.
const autoOwnerName = "auto"


// classFamilyName builds the recursion-variable name from the class owner key.
// The name carries the owner identity so the sealed family stays a distinct
// nominal carrier from structurally similar but distinctly-owned classes.
func classFamilyName(ownerKey string) string {
	return "class$" + ownerKey
}

// classRootRecord returns the class record at the root of class. A class type is
// the record carrying the __index back-edge and the methods.
func classRootRecord(class typ.Type) *typ.Record {
	if rec, ok := class.(*typ.Record); ok {
		return rec
	}
	return nil
}

// classRecordHasBackEdge reports whether root is a Lua class record: it carries
// a record-typed __index prototype field. That field is the class self-cycle
// the seal closes. A record with only a metatable (and no own __index) is an
// instance, handled by instanceMetatableHasBackEdge.
func classRecordHasBackEdge(root *typ.Record) bool {
	if root == nil {
		return false
	}
	for _, f := range root.Fields {
		if f.Name == indexField {
			if _, ok := unwrapRecord(f.Type); ok {
				return true
			}
		}
	}
	return false
}

// instanceMetatableHasBackEdge reports whether root is an instance of a class:
// it has no own __index prototype field but its metatable is a class allocation
// (a record carrying its own __index/method back-edge, or a prototype method
// surface). new() returns such an instance and method self parameters carry it.
func instanceMetatableHasBackEdge(root *typ.Record) bool {
	if root == nil || root.Metatable == nil {
		return false
	}
	for _, f := range root.Fields {
		if f.Name == indexField {
			return false
		}
	}
	meta, ok := unwrapRecord(root.Metatable)
	if !ok {
		return false
	}
	return classRecordHasBackEdge(meta) || metaHasMethodSurface(meta)
}

// classFamilyBody reports whether a recursive body denotes a class allocation
// (a class record or a class instance), so a nested value-domain fold of the
// same family can be bound to the one recursion variable during sealing.
func classFamilyBody(body typ.Type) bool {
	root, ok := unwrapRecord(body)
	if !ok || root == nil {
		return false
	}
	return classRecordHasBackEdge(root) || instanceMetatableHasBackEdge(root)
}

// metaHasMethodSurface reports whether a metatable prototype carries a callable
// method field, the surface a class metatable provides to its instances.
func metaHasMethodSurface(meta *typ.Record) bool {
	for _, f := range meta.Fields {
		if f.Name == indexField {
			continue
		}
		if _, ok := f.Type.(*typ.Function); ok {
			return true
		}
	}
	return false
}

// backEdgeRewriter rewrites class back-edges to a single recursion variable. A
// back-edge is the record value of an __index field or the metatable of any
// record below the root; both denote the class/prototype allocation.
type backEdgeRewriter struct {
	rec      *typ.Recursive
	priorRec *typ.Recursive
	seen     map[typ.Type]typ.Type
}

// rewriteInstance seals an instance whose metatable is the class: the metatable
// becomes the recursion variable (with the sealed class as its body), and the
// instance keeps its own fields. The result references the same family the
// class record seals to, so a class and its instances collapse to one mu.
func (r *backEdgeRewriter) rewriteInstance(root *typ.Record, guard internal.RecursionGuard) typ.Type {
	meta, _ := unwrapRecord(root.Metatable)
	body := r.rewriteRecord(meta, guard)
	r.rec.SetBody(body)
	return r.rewriteRecord(root, guard)
}

// foldBackEdge replaces a record-typed back-edge target with the recursion
// variable; non-record targets are walked normally.
func (r *backEdgeRewriter) foldBackEdge(t typ.Type, guard internal.RecursionGuard) typ.Type {
	if _, ok := unwrapRecord(t); ok {
		return r.rec
	}
	return r.rewrite(t, guard)
}

func (r *backEdgeRewriter) rewrite(t typ.Type, guard internal.RecursionGuard) typ.Type {
	if r.priorRec != nil && typ.IsRecursiveRef(t, r.priorRec) {
		return r.rec
	}
	if t == nil {
		return nil
	}
	if out, ok := r.seen[t]; ok {
		return out
	}
	next, ok := guard.Enter(t)
	if !ok {
		return t
	}
	switch v := t.(type) {
	case *typ.Recursive:
		// A nested value-domain fold of the same class family (a distinct Inferred
		// placeholder whose body is a class record or instance) collapses to the
		// one recursion variable, so the scattered fresh placeholders the value
		// domain generates each iteration reconcile to a single representative.
		if v == r.rec {
			return v
		}
		if v.Body != nil && v.Body != v && classFamilyBody(v.Body) {
			r.seen[t] = r.rec
			return r.rec
		}
		return v
	case *typ.Record:
		out := r.rewriteRecord(v, next)
		r.seen[t] = out
		return out
	case *typ.Function:
		return r.rewriteFunction(v, next)
	case *typ.Optional:
		inner := r.rewrite(v.Inner, next)
		if typ.SameNode(inner, v.Inner) {
			return t
		}
		return typ.NewOptional(inner)
	case *typ.Union:
		members := make([]typ.Type, len(v.Members))
		changed := false
		for i, m := range v.Members {
			members[i] = r.rewrite(m, next)
			if !typ.SameNode(members[i], m) {
				changed = true
			}
		}
		if !changed {
			return t
		}
		return typ.NewUnion(members...)
	case *typ.Array:
		elem := r.rewrite(v.Element, next)
		if typ.SameNode(elem, v.Element) {
			return t
		}
		return typ.NewArray(elem)
	case *typ.Map:
		val := r.rewrite(v.Value, next)
		if typ.SameNode(val, v.Value) {
			return t
		}
		return typ.NewMap(v.Key, val)
	case *typ.Tuple:
		elems := make([]typ.Type, len(v.Elements))
		changed := false
		for i, e := range v.Elements {
			elems[i] = r.rewrite(e, next)
			if !typ.SameNode(elems[i], e) {
				changed = true
			}
		}
		if !changed {
			return t
		}
		return typ.NewTuple(elems...)
	default:
		return t
	}
}

func (r *backEdgeRewriter) rewriteRecord(v *typ.Record, guard internal.RecursionGuard) typ.Type {
	builder := typ.NewRecord()
	for _, f := range v.Fields {
		var ft typ.Type
		if f.Name == indexField {
			ft = r.foldBackEdge(f.Type, guard)
		} else {
			ft = r.rewrite(f.Type, guard)
		}
		switch {
		case f.Optional && f.Readonly:
			builder.OptReadonlyField(f.Name, ft)
		case f.Optional:
			builder.OptField(f.Name, ft)
		case f.Readonly:
			builder.ReadonlyField(f.Name, ft)
		default:
			builder.Field(f.Name, ft)
		}
	}
	if v.Metatable != nil {
		builder.Metatable(r.foldBackEdge(v.Metatable, guard))
	}
	if v.HasMapComponent() {
		builder.MapComponent(v.MapKey, r.rewrite(v.MapValue, guard))
	}
	return builder.SetOpen(v.Open).Build()
}

func (r *backEdgeRewriter) rewriteFunction(v *typ.Function, guard internal.RecursionGuard) typ.Type {
	builder := typ.Func().ReserveParams(len(v.Params))
	for _, tp := range v.TypeParams {
		builder.TypeParam(tp.Name, tp.Constraint)
	}
	for _, p := range v.Params {
		pt := r.rewrite(p.Type, guard)
		if p.Optional {
			builder.OptParam(p.Name, pt)
		} else {
			builder.Param(p.Name, pt)
		}
	}
	if v.Variadic != nil {
		builder.Variadic(r.rewrite(v.Variadic, guard))
	}
	if len(v.Returns) > 0 {
		rets := make([]typ.Type, len(v.Returns))
		for i, ret := range v.Returns {
			rets[i] = r.rewrite(ret, guard)
		}
		builder.Returns(rets...)
	}
	if v.Effects != nil {
		builder.Effects(v.Effects)
	}
	if v.Spec != nil {
		builder.Spec(v.Spec)
	}
	if v.Refinement != nil {
		builder.WithRefinement(v.Refinement)
	}
	return builder.Build()
}

// unwrapRecord returns the record under transparent wrappers, reporting whether
// t denotes a record allocation eligible to be a class back-edge.
func unwrapRecord(t typ.Type) (*typ.Record, bool) {
	switch v := t.(type) {
	case *typ.Record:
		return v, true
	case *typ.Alias:
		return unwrapRecord(v.Target)
	default:
		return nil, false
	}
}
