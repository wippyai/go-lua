package metatable

import (
	"github.com/wippyai/go-lua/internal"
	"github.com/wippyai/go-lua/types/typ"
)

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
		r := &backEdgeRewriter{rec: rec, priorRec: priorRec, root: root, seen: make(map[typ.Type]typ.Type)}
		body := r.rewriteRecord(root, typ.NewGuard())
		rec.SetBody(body)
		return rec
	case instanceMetatableHasBackEdge(root):
		rec := typ.NewRecursivePlaceholder(classFamilyName(ownerKey))
		metaRoot, _ := unwrapRecord(root.Metatable)
		r := &backEdgeRewriter{rec: rec, priorRec: priorRec, root: metaRoot, seen: make(map[typ.Type]typ.Type)}
		sealed := r.rewriteInstance(root, typ.NewGuard())
		return sealed
	default:
		if priorRec != nil {
			return priorRec
		}
		return root
	}
}

// SealClassFamilyInterned seals a Lua OOP class type into the single interned
// recursive family owned by key in interner. Unlike SealClassFamily, which mints
// a fresh typ.Recursive per call, this binds every class back-edge to the one
// canonical family handle the interner owns for key and widens that family's body
// slot IN PLACE. Every observation of the same class (the class binding, a
// constructor's stored metatable edge) therefore resolves to the same handle, and
// when the class converges later in the inter-procedural fixpoint the widened body
// is visible through every prior reference. join is the body lattice join the
// producer supplies; it must be monotone and finite-height so the body converges.
//
// Returns class unchanged when the inputs are unusable or class carries no class
// back-edge (the producer evidence that the value is a class allocation).
func SealClassFamilyInterned(class typ.Type, key typ.FamilyKey, interner *typ.RecursiveFamilyInterner, join func(existing, candidate typ.Type) typ.Type) typ.Type {
	if class == nil || interner == nil {
		return class
	}
	if mu, ok := class.(*typ.Recursive); ok {
		if mu.Body == nil || mu.Body == mu {
			return class
		}
		root := classRootRecord(mu.Body)
		if root == nil {
			return class
		}
		return sealClassRootInterned(root, mu, key, interner, join)
	}
	root := classRootRecord(class)
	if root == nil {
		return class
	}
	return sealClassRootInterned(root, nil, key, interner, join)
}

// sealClassRootInterned seals a class root record into the interned family owned
// by key. It mirrors sealClassRoot but obtains the recursion variable from the
// interner and widens the family body in place instead of minting a fresh
// placeholder per call.
func sealClassRootInterned(root *typ.Record, priorRec *typ.Recursive, key typ.FamilyKey, interner *typ.RecursiveFamilyInterner, join func(existing, candidate typ.Type) typ.Type) typ.Type {
	switch {
	case classRecordHasBackEdge(root):
		rec := interner.Intern(key)
		r := &backEdgeRewriter{rec: rec, priorRec: priorRec, root: root, seen: make(map[typ.Type]typ.Type)}
		body := r.rewriteRecord(root, typ.NewGuard())
		interner.Widen(rec, body, join)
		return rec
	case instanceMetatableHasBackEdge(root):
		rec := interner.Intern(key)
		metaRoot, _ := unwrapRecord(root.Metatable)
		r := &backEdgeRewriter{rec: rec, priorRec: priorRec, root: metaRoot, seen: make(map[typ.Type]typ.Type)}
		metaBody := r.rewriteRecord(metaRoot, typ.NewGuard())
		interner.Widen(rec, metaBody, join)
		return r.rewriteRecord(root, typ.NewGuard())
	case classPrototypeShape(root):
		// Producer evidence proved this is the class allocation even though the
		// self back-edge through __index has not converged. When __index is still
		// degraded (no method-bearing target), force-fold it to the family so the
		// self-cycle closes; the body widens to the converged class as __index
		// resolves on later iterations. When __index already points at a
		// method-bearing prototype, that prototype IS the class surface, so it is
		// rewritten in place (its self-edges fold) and force-folding is not applied,
		// keeping the method surface reachable through __index.
		rec := interner.Intern(key)
		r := &backEdgeRewriter{rec: rec, priorRec: priorRec, root: root, seen: make(map[typ.Type]typ.Type), forceIndexFold: !indexTargetHasMethodSurface(root)}
		body := r.rewriteRecord(root, typ.NewGuard())
		interner.Widen(rec, body, join)
		return rec
	default:
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
	if zzSealOff {
		return t
	}
	out := SealClassFamily(t, owner)
	if zzSealDbg {
		println("ZZSEAL in=", zzDump(t, 0), " out=", zzDump(out, 0))
	}
	return out
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
	if owner == "" {
		return t
	}
	return SealClassFamily(t, owner)
}

// IsClassShaped reports whether t is a Lua class allocation: a class record
// carrying a genuine __index/method back-edge, a class instance whose metatable
// is such a record, or a class prototype record whose __index back-edge has not
// yet resolved to a record but which already carries an __index slot and a method
// surface. The last case is the pre-convergence shape of a setmetatable class
// where the self back-edge through __index is still degraded to unknown; the
// caller pairs it with producer evidence (a resolved class symbol) so this does
// not over-fold unrelated records. A plain record with no __index slot and no
// method surface returns false. A recursive node is inspected through its body.
func IsClassShaped(t typ.Type) bool {
	if mu, ok := t.(*typ.Recursive); ok {
		if mu.Body == nil || mu.Body == mu {
			return false
		}
		return IsClassShaped(mu.Body)
	}
	root, ok := unwrapRecord(t)
	if !ok || root == nil {
		return false
	}
	return classRecordHasBackEdge(root) || instanceMetatableHasBackEdge(root) || classPrototypeShape(root)
}

// classPrototypeShape reports whether root is the prototype shape of a
// setmetatable class whose self back-edge has not yet converged: it carries an
// __index slot (the prototype back-edge, possibly still degraded to unknown) and
// a method surface. The method surface is either a non-__index function field on
// root itself (the self-referential class where methods sit beside __index) or a
// method field on the record the __index slot points at (the class whose methods
// live in the prototype delegate). This is the structural signature of a class
// metatable independent of whether __index has resolved to the class record;
// sealing folds the __index slot to the class family so the cycle closes once the
// body widens to a fixed point.
func classPrototypeShape(root *typ.Record) bool {
	if root == nil {
		return false
	}
	hasIndex := false
	hasMethod := false
	for _, f := range root.Fields {
		if f.Name == indexField {
			hasIndex = true
			if idx, ok := unwrapRecord(f.Type); ok && metaHasMethodSurface(idx) {
				hasMethod = true
			}
			continue
		}
		if _, ok := f.Type.(*typ.Function); ok {
			hasMethod = true
		}
	}
	return hasIndex && hasMethod
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

// classRecordHasBackEdge reports whether root is a Lua class record carrying a
// genuine class self-cycle through its __index prototype. The seal fires when
// __index target either is root itself, structurally equals root, contains a
// direct self-reference (a self-referential prototype), or when root duplicates
// the prototype's surface (the seal-test cyclic class shape where both class
// and __index proto carry the same method surface and the seal collapses them
// to one mu via __index folding).
//
// The folding misfire it guards against is the split-pattern metatable
// {__index = T} where T is a separate, flat methods record whose self params
// reference back through this metatable: folding __index in that case would
// erase T's method surface. Such a metatable has only the __index field, no
// peer method surface duplicated alongside __index, so the seal does not fire
// on that metatable shape.
//
// A record with only a metatable (and no own __index) is an instance, handled
// by instanceMetatableHasBackEdge.
func classRecordHasBackEdge(root *typ.Record) bool {
	if root == nil {
		return false
	}
	for _, f := range root.Fields {
		if f.Name != indexField {
			continue
		}
		idx, ok := unwrapRecord(f.Type)
		if !ok {
			continue
		}
		if idx == root || typ.SameNode(idx, root) || typ.TypeEquals(idx, root) {
			return true
		}
		if recordContainsSelfReference(idx) {
			return true
		}
		// Root carries its own method surface alongside __index: the seal-test
		// cyclic class shape where class.{new, run, ...} == proto.{new, run, ...}
		// and __index points at proto. Folding __index closes the cycle into one
		// mu in that case. A flat metatable {__index = T} (no peer fields) does
		// not match and remains unsealed so T's methods are preserved.
		if rootDuplicatesPrototypeSurface(root, idx) {
			return true
		}
	}
	return false
}

// rootDuplicatesPrototypeSurface reports whether root carries the same method
// surface as the __index prototype: root has at least one non-__index field
// and every non-__index field of idx that is a function also appears as a
// function field on root. This is the structural signature of a class allocation
// where __index folds the cycle onto one mu without losing prototype methods.
func rootDuplicatesPrototypeSurface(root, idx *typ.Record) bool {
	if root == nil || idx == nil {
		return false
	}
	rootFunctions := 0
	rootFields := make(map[string]bool, len(root.Fields))
	for _, f := range root.Fields {
		if f.Name == indexField {
			continue
		}
		rootFields[f.Name] = true
		if _, ok := f.Type.(*typ.Function); ok {
			rootFunctions++
		}
	}
	if rootFunctions == 0 {
		return false
	}
	for _, f := range idx.Fields {
		if f.Name == indexField {
			continue
		}
		if _, ok := f.Type.(*typ.Function); !ok {
			continue
		}
		if !rootFields[f.Name] {
			return false
		}
	}
	return true
}

// recordContainsSelfReference reports whether t is a record whose structure
// embeds a DIRECT back-reference to itself: a record-typed back-edge below
// the top of t (a method's self-param metatable, a nested record metatable)
// points to t. This is the closed-family condition the class seal needs — a
// prototype that recurses to itself directly is its own family.
//
// A reference to t reachable only through ANOTHER record's __index back-edge
// (the split-pattern class_mt = {__index = T}, where T's methods reference
// instances whose metatable is class_mt and class_mt's __index is T) is NOT
// a direct self-cycle on T; T is a separate prototype allocation reachable
// through class_mt, and folding T to the seal's recursion variable would
// erase T's method surface.
func recordContainsSelfReference(t *typ.Record) bool {
	if t == nil {
		return false
	}
	seen := make(map[*typ.Record]bool)
	// Walk children only; matching the top against itself is not a cycle.
	for _, f := range t.Fields {
		if f.Name == indexField {
			// A direct back-edge on t's own __index does denote the
			// self-referential class pattern. Other __index fields are followed
			// normally so the search stops at the next allocation boundary.
			if rec, ok := unwrapRecord(f.Type); ok && rec == t {
				return true
			}
			continue
		}
		if directRecordContainsRecord(f.Type, t, seen) {
			return true
		}
	}
	if t.Metatable != nil && directRecordContainsRecord(t.Metatable, t, seen) {
		return true
	}
	if t.HasMapComponent() {
		if directRecordContainsRecord(t.MapKey, t, seen) || directRecordContainsRecord(t.MapValue, t, seen) {
			return true
		}
	}
	return false
}

// directRecordContainsRecord reports whether t contains a structural reference
// to target without following any __index back-edge of an intermediate record.
// Each __index field denotes a SEPARATE allocation boundary: a reference to
// target reachable only by descending into an intermediate __index target is a
// cross-allocation reference, not a self-cycle on target.
func directRecordContainsRecord(t typ.Type, target *typ.Record, seen map[*typ.Record]bool) bool {
	if t == nil || target == nil {
		return false
	}
	switch v := t.(type) {
	case *typ.Record:
		if v == target {
			return true
		}
		if seen[v] {
			return false
		}
		seen[v] = true
		for _, f := range v.Fields {
			if f.Name == indexField {
				continue
			}
			if directRecordContainsRecord(f.Type, target, seen) {
				return true
			}
		}
		if v.Metatable != nil && directRecordContainsRecord(v.Metatable, target, seen) {
			return true
		}
		if v.HasMapComponent() {
			if directRecordContainsRecord(v.MapKey, target, seen) || directRecordContainsRecord(v.MapValue, target, seen) {
				return true
			}
		}
		return false
	case *typ.Function:
		for _, p := range v.Params {
			if directRecordContainsRecord(p.Type, target, seen) {
				return true
			}
		}
		for _, r := range v.Returns {
			if directRecordContainsRecord(r, target, seen) {
				return true
			}
		}
		if v.Variadic != nil && directRecordContainsRecord(v.Variadic, target, seen) {
			return true
		}
		return false
	case *typ.Optional:
		return directRecordContainsRecord(v.Inner, target, seen)
	case *typ.Union:
		for _, m := range v.Members {
			if directRecordContainsRecord(m, target, seen) {
				return true
			}
		}
		return false
	case *typ.Array:
		return directRecordContainsRecord(v.Element, target, seen)
	case *typ.Map:
		return directRecordContainsRecord(v.Key, target, seen) || directRecordContainsRecord(v.Value, target, seen)
	case *typ.Tuple:
		for _, e := range v.Elements {
			if directRecordContainsRecord(e, target, seen) {
				return true
			}
		}
		return false
	case *typ.Alias:
		return directRecordContainsRecord(v.Target, target, seen)
	default:
		return false
	}
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

// indexTargetHasMethodSurface reports whether root's own __index slot points at a
// record that carries a method surface. The prototype delegate of such a class
// holds the methods, so its surface must be preserved during sealing.
func indexTargetHasMethodSurface(root *typ.Record) bool {
	if root == nil {
		return false
	}
	for _, f := range root.Fields {
		if f.Name != indexField {
			continue
		}
		idx, ok := unwrapRecord(f.Type)
		if !ok {
			return false
		}
		return metaHasMethodSurface(idx)
	}
	return false
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
// record below the root; both denote the class/prototype allocation. The root
// the seal owns is recorded so a back-edge target that denotes a SEPARATE
// prototype allocation (split-pattern class_mt = {__index = Class}) is not
// folded into the recursion variable and Class's method surface is preserved.
type backEdgeRewriter struct {
	rec      *typ.Recursive
	priorRec *typ.Recursive
	root     *typ.Record
	seen     map[typ.Type]typ.Type
	// forceIndexFold folds the root record's own __index slot to the recursion
	// variable unconditionally. It is set only with producer evidence (a resolved
	// class symbol), for the pre-convergence prototype shape whose __index has not
	// yet resolved to the class record, so the self-cycle still closes on one node.
	forceIndexFold bool
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

// foldBackEdge replaces a class back-edge target with the recursion variable.
// The back-edge is the value of an __index field or the metatable slot of a
// record below the root: both denote the prototype/metatable allocation the
// seal owns. For the self-referential class pattern (Class = setmetatable(Class,
// {__index = Class}), or the same closed family observed as two views — a class
// wrapping its __index prototype where the prototype's methods reference
// instances with metatable = prototype) the back-edge value is structurally
// part of the same recursion family, so folding to the recursion variable
// closes the cycle.
//
// For the split-pattern (class_mt = {__index = Class}, Class = {methods})
// folding the __index value erases Class's method surface; the recursion
// belongs to class_mt (the metatable) and Class is a separate prototype
// allocation reachable through the back-edge but not itself recursive. Only
// fold when the target is the recursion family — the root the seal started
// with, structurally equal to it, the prior fold, or a record that is itself
// structurally self-referential and so denotes its own closed family.
func (r *backEdgeRewriter) foldBackEdge(t typ.Type, guard internal.RecursionGuard) typ.Type {
	if r.priorRec != nil && typ.IsRecursiveRef(t, r.priorRec) {
		return r.rec
	}
	rec, ok := unwrapRecord(t)
	if !ok {
		return r.rewrite(t, guard)
	}
	if r.targetIsRecursionFamily(rec) {
		return r.rec
	}
	return r.rewrite(t, guard)
}

// targetIsRecursionFamily reports whether a record reached through a class
// back-edge denotes the recursion family the seal owns. The root the seal
// started with, any record structurally equal to that root, a record that
// contains a direct self-reference, and a prototype whose method surface is
// duplicated on the root (a self-referential class shape sharing the prototype
// methods at both levels) all qualify. A separate, flat prototype allocation
// whose body only references the seal's metatable family (the split-pattern
// Class where Class has methods but its self params reference back through a
// metatable, not through Class itself) does not.
func (r *backEdgeRewriter) targetIsRecursionFamily(rec *typ.Record) bool {
	if rec == nil {
		return false
	}
	if rec == r.root {
		return true
	}
	if r.root != nil && typ.TypeEquals(typ.Type(rec), typ.Type(r.root)) {
		return true
	}
	if recordContainsSelfReference(rec) {
		return true
	}
	if r.root != nil && rootDuplicatesPrototypeSurface(r.root, rec) {
		return true
	}
	return false
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
			if r.forceIndexFold && v == r.root {
				ft = r.rec
			} else {
				ft = r.foldBackEdge(f.Type, guard)
			}
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
