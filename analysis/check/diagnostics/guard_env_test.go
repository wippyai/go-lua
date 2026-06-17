package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
)

func TestGuardEnvWithoutDescendantFactsPreservesRootOnlyFacts(t *testing.T) {
	root := path.NewPath(1, "x")
	descendant := path.NewPath(2, "box").Field("value")
	env := guardEnv{
		constraints: []literalConstraint{
			{target: descendant, value: "ready"},
			{target: root, value: "string"},
		},
		typeChecks: []runtimeTypeConstraint{
			{target: descendant, name: "string"},
			{target: root, name: "string"},
		},
		present:  []path.Path{descendant, root},
		truthy:   []path.Path{descendant, root},
		falsy:    []path.Path{descendant, root},
		nilPaths: []path.Path{descendant, root},
	}

	got := env.withoutDescendantFacts()
	if len(got.constraints) != 1 || !got.constraints[0].target.Equal(root) || got.constraints[0].value != "string" {
		t.Fatalf("constraints = %#v, want only root literal constraint", got.constraints)
	}
	if len(got.typeChecks) != 1 || !got.typeChecks[0].target.Equal(root) || got.typeChecks[0].name != "string" {
		t.Fatalf("type checks = %#v, want only root runtime type check", got.typeChecks)
	}
	if len(got.present) != 1 || !got.present[0].Equal(root) {
		t.Fatalf("present = %#v, want only root presence", got.present)
	}
	if len(got.truthy) != 1 || !got.truthy[0].Equal(root) {
		t.Fatalf("truthy = %#v, want only root truthy fact", got.truthy)
	}
	if len(got.falsy) != 1 || !got.falsy[0].Equal(root) {
		t.Fatalf("falsy = %#v, want only root falsy fact", got.falsy)
	}
	if len(got.nilPaths) != 1 || !got.nilPaths[0].Equal(root) {
		t.Fatalf("nil paths = %#v, want only root nil fact", got.nilPaths)
	}
}

func TestGuardEnvInvalidatesAssignedPathAndDescendants(t *testing.T) {
	root := path.NewPath(1, "box")
	child := root.Field("value")
	grandchild := child.Field("name")
	sibling := root.Field("other")
	env := guardEnv{
		constraints: []literalConstraint{
			{target: child, value: "ready"},
			{target: grandchild, value: "nested"},
			{target: sibling, value: "kept"},
		},
		typeChecks: []runtimeTypeConstraint{
			{target: child, name: "string"},
			{target: grandchild, name: "string"},
			{target: sibling, name: "string"},
		},
		present:  []path.Path{root, child, grandchild, sibling},
		truthy:   []path.Path{root, child, grandchild, sibling},
		falsy:    []path.Path{child, grandchild, sibling},
		nilPaths: []path.Path{child, grandchild, sibling},
	}

	got := env.withoutFactsForPath(child)
	if len(got.constraints) != 1 || !got.constraints[0].target.Equal(sibling) {
		t.Fatalf("constraints = %#v, want only sibling", got.constraints)
	}
	if len(got.typeChecks) != 1 || !got.typeChecks[0].target.Equal(sibling) {
		t.Fatalf("type checks = %#v, want only sibling", got.typeChecks)
	}
	if !got.hasTruthy(root) || !got.hasTruthy(sibling) || got.hasTruthy(child) || got.hasTruthy(grandchild) {
		t.Fatalf("truthy facts = %#v, want root/sibling only", got.truthy)
	}
	if !got.hasPresent(root) || !got.hasPresent(sibling) || got.hasPresent(child) || got.hasPresent(grandchild) {
		t.Fatalf("present facts = %#v, want root/sibling only", got.present)
	}
	if got.hasFalsy(child) || got.hasFalsy(grandchild) || got.hasNil(child) || got.hasNil(grandchild) {
		t.Fatalf("path facts kept assigned child: falsy=%#v nil=%#v", got.falsy, got.nilPaths)
	}
}

func TestGuardEnvInvalidatesRootAndKeepsUnrelatedRoots(t *testing.T) {
	root := path.NewPath(1, "box")
	child := root.Field("value")
	other := path.NewPath(2, "other")
	otherChild := other.Field("value")
	env := guardEnv{
		constraints: []literalConstraint{
			{target: child, value: "drop"},
			{target: otherChild, value: "keep"},
		},
		typeChecks: []runtimeTypeConstraint{
			{target: child, name: "string"},
			{target: otherChild, name: "string"},
		},
		present:  []path.Path{root, child, other, otherChild},
		truthy:   []path.Path{root, child, other, otherChild},
		falsy:    []path.Path{child, otherChild},
		nilPaths: []path.Path{child, otherChild},
	}

	got := env.withoutFactsForPath(root)
	if got.hasPresent(root) || got.hasTruthy(root) || got.hasTruthy(child) || got.hasFalsy(child) || got.hasNil(child) {
		t.Fatalf("root facts leaked after invalidation: present=%#v truthy=%#v falsy=%#v nil=%#v", got.present, got.truthy, got.falsy, got.nilPaths)
	}
	if len(got.constraints) != 1 || !got.constraints[0].target.Equal(otherChild) {
		t.Fatalf("constraints = %#v, want only unrelated root child", got.constraints)
	}
	if len(got.typeChecks) != 1 || !got.typeChecks[0].target.Equal(otherChild) {
		t.Fatalf("type checks = %#v, want only unrelated root child", got.typeChecks)
	}
	if !got.hasPresent(other) || !got.hasPresent(otherChild) || !got.hasTruthy(other) || !got.hasTruthy(otherChild) ||
		!got.hasFalsy(otherChild) || !got.hasNil(otherChild) {
		t.Fatalf("unrelated root facts were dropped: present=%#v truthy=%#v falsy=%#v nil=%#v", got.present, got.truthy, got.falsy, got.nilPaths)
	}
}

func TestGuardEnvInvalidatingMissingPathIsNoop(t *testing.T) {
	root := path.NewPath(1, "box")
	child := root.Field("value")
	env := guardEnv{
		constraints: []literalConstraint{{target: child, value: "ready"}},
		typeChecks:  []runtimeTypeConstraint{{target: child, name: "string"}},
		present:     []path.Path{root, child},
		truthy:      []path.Path{root, child},
	}

	got := env.withoutFactsForPath(path.NewPath(2, "other").Field("value"))
	if !guardEnvEqual(got, env) {
		t.Fatalf("guard env changed after unrelated invalidation: got=%#v want=%#v", got, env)
	}
}

func TestGuardEnvDynamicIndexInvalidatesAllDescendantsConservatively(t *testing.T) {
	box := path.NewPath(1, "box")
	child := box.Field("value")
	otherRoot := path.NewPath(2, "other").Field("value")
	env := guardEnv{
		constraints: []literalConstraint{
			{target: child, value: "ready"},
			{target: otherRoot, value: "ready"},
		},
		typeChecks: []runtimeTypeConstraint{
			{target: child, name: "string"},
			{target: otherRoot, name: "string"},
		},
		present: []path.Path{box, child, otherRoot},
		truthy:  []path.Path{box, child, otherRoot},
	}

	got := env.withoutDescendantFacts()
	if len(got.constraints) != 0 {
		t.Fatalf("constraints = %#v, want descendant constraints dropped", got.constraints)
	}
	if len(got.typeChecks) != 0 {
		t.Fatalf("type checks = %#v, want descendant type checks dropped", got.typeChecks)
	}
	if !got.hasTruthy(box) || got.hasTruthy(otherRoot) || got.hasTruthy(child) {
		t.Fatalf("truthy facts = %#v, want only root-only container fact kept", got.truthy)
	}
}
