package diagnostics

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func guardLiteral(value string) typ.Type {
	return typ.LiteralString(value)
}

func TestGuardEnvWithoutDescendantFactsPreservesRootOnlyFacts(t *testing.T) {
	root := path.NewPath(1, "x")
	descendant := path.NewPath(2, "box").Field("value")
	env := guardEnv{
		constraints: []literalConstraint{
			{target: descendant, value: guardLiteral("ready")},
			{target: root, value: guardLiteral("string")},
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
	if len(got.constraints) != 1 || !got.constraints[0].target.Equal(root) || !typ.TypeEquals(got.constraints[0].value, guardLiteral("string")) {
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
			{target: child, value: guardLiteral("ready")},
			{target: grandchild, value: guardLiteral("nested")},
			{target: sibling, value: guardLiteral("kept")},
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

func TestGuardEnvPathAssignmentKeepsSameRootSiblingsAndClearsAliasableDescendants(t *testing.T) {
	root := path.NewPath(1, "box")
	child := root.Field("value")
	grandchild := child.Field("name")
	sibling := root.Field("other")
	aliasRoot := path.NewPath(2, "alias")
	aliasChild := aliasRoot.Field("value")
	env := guardEnv{
		constraints: []literalConstraint{
			{target: child, value: guardLiteral("drop")},
			{target: grandchild, value: guardLiteral("drop-nested")},
			{target: sibling, value: guardLiteral("keep")},
			{target: aliasChild, value: guardLiteral("drop-alias")},
		},
		typeChecks: []runtimeTypeConstraint{
			{target: child, name: "string"},
			{target: grandchild, name: "string"},
			{target: sibling, name: "string"},
			{target: aliasChild, name: "string"},
		},
		present:  []path.Path{root, child, grandchild, sibling, aliasRoot, aliasChild},
		truthy:   []path.Path{root, child, grandchild, sibling, aliasRoot, aliasChild},
		falsy:    []path.Path{child, grandchild, sibling, aliasChild},
		nilPaths: []path.Path{child, grandchild, sibling, aliasChild},
	}

	got := env.withoutFactsForPathAssignment(child)
	if len(got.constraints) != 1 || !got.constraints[0].target.Equal(sibling) {
		t.Fatalf("constraints = %#v, want only same-root sibling", got.constraints)
	}
	if len(got.typeChecks) != 1 || !got.typeChecks[0].target.Equal(sibling) {
		t.Fatalf("type checks = %#v, want only same-root sibling", got.typeChecks)
	}
	if !got.hasTruthy(root) || !got.hasTruthy(sibling) || !got.hasTruthy(aliasRoot) {
		t.Fatalf("root/sibling truthy facts lost: %#v", got.truthy)
	}
	if got.hasTruthy(child) || got.hasTruthy(grandchild) || got.hasTruthy(aliasChild) {
		t.Fatalf("assigned or aliasable descendant truthy facts leaked: %#v", got.truthy)
	}
	if got.hasFalsy(child) || got.hasFalsy(grandchild) || !got.hasFalsy(sibling) || got.hasFalsy(aliasChild) {
		t.Fatalf("falsy facts = %#v, want only same-root sibling among descendants", got.falsy)
	}
	if got.hasNil(child) || got.hasNil(grandchild) || !got.hasNil(sibling) || got.hasNil(aliasChild) {
		t.Fatalf("nil facts = %#v, want only same-root sibling among descendants", got.nilPaths)
	}
}

func TestGuardEnvInvalidatesRootAndKeepsUnrelatedRoots(t *testing.T) {
	root := path.NewPath(1, "box")
	child := root.Field("value")
	other := path.NewPath(2, "other")
	otherChild := other.Field("value")
	env := guardEnv{
		constraints: []literalConstraint{
			{target: child, value: guardLiteral("drop")},
			{target: otherChild, value: guardLiteral("keep")},
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
		constraints: []literalConstraint{{target: child, value: guardLiteral("ready")}},
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
			{target: child, value: guardLiteral("ready")},
			{target: otherRoot, value: guardLiteral("ready")},
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

func TestGuardEnvCallOutcomePathInvalidationClearsDescendantGuard(t *testing.T) {
	result := runDiagnosticsResult(t, `
		type Box = { value: string? }

		local function clear(box: Box, key: string): ()
			box[key] = nil
		end

		local box: Box = { value = "ready" }
		if box.value then
			clear(box, "value")
			local after: string = box.value
		end
	`)
	point, expr := requireLocalAssignmentExprByName(t, result, "after")
	readPath, ok := result.ExpressionPath(expr)
	if !ok {
		t.Fatalf("after source path unavailable")
	}
	env := guardEnvironments(result)[point]
	if env.hasTruthy(readPath) || env.hasPresent(readPath) {
		t.Fatalf("guard env at after kept invalidated facts for %s: present=%#v truthy=%#v", readPath, env.present, env.truthy)
	}
}

func TestGuardEnvCallOutcomeSkipsUnboundPathInvalidation(t *testing.T) {
	result := runDiagnosticsResult(t, `
		type Box = { value: string? }

		local function clear(x): ()
			x.value = nil
		end

		local box: Box = { value = "ready" }
		if box.value then
			clear("not-a-path")
			local after: string = box.value
		end
	`)
	point, expr := requireLocalAssignmentExprByName(t, result, "after")
	readPath, ok := result.ExpressionPath(expr)
	if !ok {
		t.Fatalf("after source path unavailable")
	}
	env := guardEnvironments(result)[point]
	if !env.hasTruthy(readPath) || !env.hasPresent(readPath) {
		t.Fatalf("guard env at after dropped unrelated facts for %s: present=%#v truthy=%#v", readPath, env.present, env.truthy)
	}
}

func TestGuardEnvJoinPreservesOriginWhenAllPathsAgree(t *testing.T) {
	value := path.NewPath(1, "value")
	origin := diagnostic.Span{StartLine: 3, StartCol: 4, EndLine: 3, EndCol: 9}
	left := guardEnv{}.withTruthyAt(value, origin)
	right := guardEnv{}.withTruthyAt(value, origin)

	got := joinGuardEnvs(left, right)
	if !got.hasTruthy(value) {
		t.Fatalf("truthy facts = %#v, want joined truthy fact", got.truthy)
	}
	if !spanEqual(got.truthyOrigin(value), origin) {
		t.Fatalf("truthy origin = %#v, want %#v", got.truthyOrigin(value), origin)
	}
	if !spanEqual(got.presentOrigin(value), origin) {
		t.Fatalf("present origin = %#v, want %#v", got.presentOrigin(value), origin)
	}
}

func TestGuardEnvJoinDropsAmbiguousOriginButKeepsFact(t *testing.T) {
	value := path.NewPath(1, "value")
	left := guardEnv{}.withTruthyAt(value, diagnostic.Span{StartLine: 3, StartCol: 4, EndLine: 3, EndCol: 9})
	right := guardEnv{}.withTruthyAt(value, diagnostic.Span{StartLine: 5, StartCol: 4, EndLine: 5, EndCol: 9})

	got := joinGuardEnvs(left, right)
	if !got.hasTruthy(value) {
		t.Fatalf("truthy facts = %#v, want joined truthy fact", got.truthy)
	}
	if got.truthyOrigin(value).Valid() {
		t.Fatalf("truthy origin = %#v, want ambiguous origin dropped", got.truthyOrigin(value))
	}
	if got.presentOrigin(value).Valid() {
		t.Fatalf("present origin = %#v, want ambiguous origin dropped", got.presentOrigin(value))
	}
}
