package plan

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Dependency is a named expression-to-relation declaration. The name is only
// a debug label; the owner-issued ID is the logical identity. It keeps the
// expression's nominal ID while the canonical expression registry owns the
// full DAG. Read/write sets are copied and canonicalized projections; W1
// recomputes them from the DAG and rejects mismatches. DefineDependency
// intentionally keeps malformed values for the checker.
type Dependency struct {
	id         model.DependencyID
	name       string
	expression model.ExpressionID
	reads      []RelationRef
	writes     []RelationRef
	digest     identity.ContentID
}

func DefineDependency(id model.DependencyID, expression model.ExpressionID, reads, writes []RelationRef, name string) Dependency {
	dependency := Dependency{
		id: id, name: name, expression: expression,
		reads: relationRefs(reads), writes: relationRefs(writes),
	}
	dependency.digest = digestDependency(dependency)
	return dependency
}

func (dependency Dependency) Available() bool                { return dependency.digest.Available() }
func (dependency Dependency) ID() model.DependencyID         { return dependency.id }
func (dependency Dependency) Name() string                   { return dependency.name }
func (dependency Dependency) Expression() model.ExpressionID { return dependency.expression }
func (dependency Dependency) Reads() []RelationRef {
	return append([]RelationRef(nil), dependency.reads...)
}
func (dependency Dependency) Writes() []RelationRef {
	return append([]RelationRef(nil), dependency.writes...)
}
func (dependency Dependency) Digest() identity.ContentID { return dependency.digest }

// DependencyRef is a stable edge endpoint with the nominal dependency
// identity. It contains no registry digest or local graph ordinal.
type DependencyRef struct{ id model.DependencyID }

func DefineDependencyRef(id model.DependencyID) DependencyRef { return DependencyRef{id: id} }

func (ref DependencyRef) Available() bool            { return ref.id.Available() }
func (ref DependencyRef) ID() model.DependencyID     { return ref.id }
func (ref DependencyRef) Digest() identity.ContentID { return digestDependencyRef(ref) }

// DependencyEdge joins two logical dependency declarations. Define methods
// do not reject zero endpoints; the checker owns endpoint validity.
type DependencyEdge struct {
	from, to DependencyRef
	digest   identity.ContentID
}

func DefineDependencyEdge(from, to DependencyRef) DependencyEdge {
	edge := DependencyEdge{from: from, to: to}
	edge.digest = digestEdge(edge)
	return edge
}

func (edge DependencyEdge) Available() bool            { return edge.digest.Available() }
func (edge DependencyEdge) From() DependencyRef        { return edge.from }
func (edge DependencyEdge) To() DependencyRef          { return edge.to }
func (edge DependencyEdge) Digest() identity.ContentID { return edge.digest }

// WideningHead identifies the logical relation at a recurrence head.
type WideningHead struct {
	dependency DependencyRef
	relation   RelationRef
	digest     identity.ContentID
}

func DefineWideningHead(dependency DependencyRef, relation RelationRef) WideningHead {
	head := WideningHead{dependency: dependency, relation: relation}
	head.digest = digestWideningHead(head)
	return head
}

func (head WideningHead) Available() bool            { return head.digest.Available() }
func (head WideningHead) Dependency() DependencyRef  { return head.dependency }
func (head WideningHead) Relation() RelationRef      { return head.relation }
func (head WideningHead) Digest() identity.ContentID { return head.digest }

// Recurrence is the declared policy attached to an SCC. DefineRecurrence
// retains invalid kinds and malformed heads for independent checking.
type RecurrenceKind uint8

const (
	RecurrenceInvalid RecurrenceKind = iota
	Acyclic
	Positive
)

func (kind RecurrenceKind) Available() bool { return kind == Acyclic || kind == Positive }

type Recurrence struct {
	kind   RecurrenceKind
	heads  []WideningHead
	digest identity.ContentID
}

func DefineRecurrence(kind RecurrenceKind, heads []WideningHead) Recurrence {
	canonical := append([]WideningHead(nil), heads...)
	sort.Slice(canonical, func(left, right int) bool {
		return contentLess(canonical[left].Digest(), canonical[right].Digest())
	})
	recurrence := Recurrence{kind: kind, heads: canonical}
	recurrence.digest = digestRecurrence(recurrence)
	return recurrence
}

func (recurrence Recurrence) Available() bool      { return recurrence.digest.Available() }
func (recurrence Recurrence) Kind() RecurrenceKind { return recurrence.kind }
func (recurrence Recurrence) Heads() []WideningHead {
	return append([]WideningHead(nil), recurrence.heads...)
}
func (recurrence Recurrence) Digest() identity.ContentID { return recurrence.digest }

// SCC is a positive/acyclic recurrence component. Members and edges are
// canonical stable references; no free-form name or schedule order is stored.
// Edges are a precomputed projection: W1 recomputes dependency adjacency from
// the expression registry and rejects any mismatch. DefineSCC retains
// malformed members, edges, and recurrence policy.
type SCC struct {
	members    []DependencyRef
	edges      []DependencyEdge
	recurrence Recurrence
	digest     identity.ContentID
}

func DefineSCC(members []DependencyRef, edges []DependencyEdge, recurrence Recurrence) SCC {
	canonicalMembers := append([]DependencyRef(nil), members...)
	sort.Slice(canonicalMembers, func(left, right int) bool {
		return dependencyRefLess(canonicalMembers[left], canonicalMembers[right])
	})
	canonicalEdges := append([]DependencyEdge(nil), edges...)
	sort.Slice(canonicalEdges, func(left, right int) bool {
		return contentLess(canonicalEdges[left].Digest(), canonicalEdges[right].Digest())
	})
	scc := SCC{members: canonicalMembers, edges: canonicalEdges, recurrence: recurrence}
	scc.digest = digestSCC(scc)
	return scc
}

func (scc SCC) Available() bool            { return scc.digest.Available() }
func (scc SCC) Members() []DependencyRef   { return append([]DependencyRef(nil), scc.members...) }
func (scc SCC) Edges() []DependencyEdge    { return append([]DependencyEdge(nil), scc.edges...) }
func (scc SCC) Recurrence() Recurrence     { return scc.recurrence }
func (scc SCC) Digest() identity.ContentID { return scc.digest }

func relationRefs(source []RelationRef) []RelationRef {
	result := append([]RelationRef(nil), source...)
	sort.Slice(result, func(left, right int) bool { return relationLess(result[left], result[right]) })
	return result
}

func dependencyRefLess(left, right DependencyRef) bool {
	if contentLess(left.id.Owner().Content(), right.id.Owner().Content()) {
		return true
	}
	return left.id.Owner() == right.id.Owner() && contentLess(left.id.Content(), right.id.Content())
}
