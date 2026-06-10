package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/narrow"
	"github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
)

func TestConditionProofDomain_ApplyCondition_UsesTypeNarrowedBaseForLeftoverFieldConstraint(t *testing.T) {
	allow := typ.NewAlias("Allow", typ.NewRecord().
		Field("kind", typ.LiteralString("allow")).
		Field("reason", typ.String).
		Build())
	deny := typ.NewAlias("Deny", typ.NewRecord().
		Field("kind", typ.LiteralString("deny")).
		Field("reason", typ.String).
		Build())
	deferType := typ.NewAlias("Defer", typ.NewRecord().
		Field("kind", typ.LiteralString("defer")).
		Field("queue", typ.String).
		Build())
	decision := typ.NewAlias("Decision", typ.NewUnion(allow, deny, deferType))
	baseType := typ.NewOptional(decision)

	path := constraint.Path{Root: "decision", Symbol: 1}
	key := path.Key()
	env := constraint.Env{
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			if k == key {
				return baseType
			}
			return nil
		},
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			return p.Key()
		},
		Resolver: &core.FuncResolver{
			FieldFunc: core.Field,
		},
	}

	dom := NewConditionProofDomain(env)
	ok := dom.ApplyCondition(constraint.FromConstraints(
		constraint.Truthy{Path: path},
		constraint.FieldEquals{Target: path, Field: "kind", Value: typ.LiteralString("defer")},
	))
	if !ok {
		t.Fatal("ApplyCondition returned false")
	}

	got := dom.TypeAt(key)
	if got == nil {
		t.Fatal("TypeAt(decision) returned nil")
	}
	if subtype.IsSubtype(typ.Nil, got) {
		t.Fatalf("TypeAt(decision) = %v, want non-nil Defer variant", got)
	}
	if queue, ok := core.Field(got, "queue"); !ok || !typ.TypeEquals(queue, typ.String) {
		t.Fatalf("queue field = %v, want string on narrowed Defer variant", queue)
	}
	if _, ok := core.Field(got, "reason"); ok {
		t.Fatalf("reason field should not remain on narrowed Defer variant: %v", got)
	}
}

func TestConditionProofDomain_IndexElementTypeProofDoesNotCollapseReachableParent(t *testing.T) {
	itemsPath := constraint.Path{Root: "raw", Symbol: 1}.Append(
		constraint.Segment{Kind: constraint.SegmentField, Name: "items"},
	)
	firstPath := itemsPath.Append(constraint.Segment{Kind: constraint.SegmentIndexInt, Index: 1})
	itemsKey := itemsPath.Key()
	itemsType := typ.NewTuple(typ.LiteralString("safe"), typ.LiteralInt(42))

	env := constraint.Env{
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			switch k {
			case itemsKey:
				return itemsType
			default:
				return nil
			}
		},
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			return p.Key()
		},
		Resolver: &core.FuncResolver{
			FieldFunc: core.Field,
			IndexFunc: core.Index,
		},
	}

	dom := NewConditionProofDomain(env)
	ok := dom.ApplyCondition(constraint.FromConstraints(
		constraint.HasType{Path: itemsPath, Type: narrow.BuiltinTypeKey("table")},
		constraint.HasType{Path: firstPath, Type: narrow.BuiltinTypeKey("string")},
	))
	if !ok {
		t.Fatal("ApplyCondition returned false for reachable table/index proof")
	}

	got := dom.ProjectedTypeAt(itemsKey, env.Resolver)
	if got == nil || typ.IsNever(got) {
		t.Fatalf("ProjectedTypeAt(items) = %v, want reachable parent tuple", got)
	}
	if !typ.TypeEquals(got, itemsType) {
		t.Fatalf("ProjectedTypeAt(items) = %v, want %v", got, itemsType)
	}
}

func TestConditionProofDomain_ApplyCondition_HasFieldUsesTruthyNarrowedOptionalInterface(t *testing.T) {
	errType := typ.NewInterface("Err", []typ.Method{
		{Name: "kind", Type: typ.Func().Returns(typ.String).Build()},
	})
	baseType := typ.NewOptional(errType)

	path := constraint.Path{Root: "err", Symbol: 1}
	key := path.Key()
	env := constraint.Env{
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			if k == key {
				return baseType
			}
			return nil
		},
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			return p.Key()
		},
		Resolver: &core.FuncResolver{
			FieldFunc: core.Field,
		},
	}

	dom := NewConditionProofDomain(env)
	ok := dom.ApplyCondition(constraint.FromConstraints(
		constraint.Truthy{Path: path},
		constraint.HasField{Path: path, Field: "kind"},
	))
	if !ok {
		t.Fatal("ApplyCondition returned false")
	}

	got := dom.TypeAt(key)
	if !typ.TypeEquals(got, errType) {
		t.Fatalf("TypeAt(err) = %v, want non-optional Err", got)
	}
}

func TestConditionProofDomain_ApplyCondition_JoinsProjectedTypeShapeCorrelation(t *testing.T) {
	spec := typ.NewRecord().
		Field("id", typ.String).
		OptField("alias", typ.String).
		Build()
	toolSpec := typ.NewAlias("ToolSpec", typ.NewUnion(typ.String, spec))
	toolSpecArray := typ.NewArray(toolSpec)
	baseType := typ.NewUnion(toolSpec, toolSpecArray)

	path := constraint.Path{Root: "tool_specs", Symbol: 1}
	key := path.Key()
	env := constraint.Env{
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			if k == key {
				return baseType
			}
			return nil
		},
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			return p.Key()
		},
		Resolver: &core.FuncResolver{
			FieldFunc: core.Field,
		},
	}

	dom := NewConditionProofDomain(env)
	ok := dom.ApplyCondition(constraint.FromDisjuncts([][]constraint.Constraint{
		{
			constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("string")},
		},
		{
			constraint.HasType{Path: path, Type: narrow.BuiltinTypeKey("table")},
			constraint.NotHasType{Path: path, Type: narrow.BuiltinTypeKey("string")},
			constraint.Truthy{Path: path.Field("id")},
			constraint.HasField{Path: path, Field: "id"},
		},
	}))
	if !ok {
		t.Fatal("ApplyCondition returned false")
	}

	got := dom.TypeAt(key)
	if got == nil {
		t.Fatal("TypeAt(tool_specs) returned nil")
	}
	if !subtype.IsSubtype(typ.String, got) {
		t.Fatalf("TypeAt(tool_specs) = %v, want string branch retained", got)
	}
	if !subtype.IsSubtype(spec, got) {
		t.Fatalf("TypeAt(tool_specs) = %v, want single ToolSpec record branch retained", got)
	}
	if subtype.IsSubtype(toolSpecArray, got) {
		t.Fatalf("TypeAt(tool_specs) = %v, array branch should be excluded by table+id branch evidence", got)
	}
}

func TestConditionProofDomain_KeyOfRejectsClosedEmptyRecord(t *testing.T) {
	tablePath := constraint.Path{Root: "blocks", Symbol: 1}
	keyPath := constraint.Path{Root: "index", Symbol: 2}
	tableKey := tablePath.Key()
	keyKey := keyPath.Key()
	env := constraint.Env{
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			switch k {
			case tableKey:
				return typ.NewRecord().Build()
			case keyKey:
				return typ.Integer
			default:
				return nil
			}
		},
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			return p.Key()
		},
	}

	dom := NewConditionProofDomain(env)
	cond := constraint.FromConstraints(constraint.KeyOf{Table: tablePath, Key: keyPath})
	if dom.ApplyCondition(cond) {
		t.Fatal("KeyOf on closed empty record should be unsatisfiable")
	}
}

func TestConditionProofDomain_KeyOfFiltersEmptyRecordUnion(t *testing.T) {
	tablePath := constraint.Path{Root: "blocks", Symbol: 1}
	keyPath := constraint.Path{Root: "index", Symbol: 2}
	tableKey := tablePath.Key()
	keyKey := keyPath.Key()
	value := typ.NewRecord().Field("thinking", typ.String).Build()
	mapType := typ.NewMap(typ.Integer, value)
	baseType := typ.NewUnion(typ.NewRecord().Build(), mapType)
	env := constraint.Env{
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			switch k {
			case tableKey:
				return baseType
			case keyKey:
				return typ.Integer
			default:
				return nil
			}
		},
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			return p.Key()
		},
		Resolver: &core.FuncResolver{
			IndexFunc: core.Index,
		},
	}

	dom := NewConditionProofDomain(env)
	cond := constraint.FromConstraints(constraint.KeyOf{Table: tablePath, Key: keyPath})
	if !dom.ApplyCondition(cond) {
		t.Fatal("KeyOf should keep the map branch")
	}
	if got := dom.TypeAt(tableKey); !typ.TypeEquals(got, mapType) {
		t.Fatalf("TypeAt(blocks) = %v, want map branch %v", got, mapType)
	}
	if got := dom.TypeAt(keyKey); !typ.TypeEquals(got, typ.Integer) {
		t.Fatalf("TypeAt(index) = %v, want integer", got)
	}
}

func TestConditionProofDomain_CanSatisfyCondition_DoesNotJoinWitnessProducts(t *testing.T) {
	path := constraint.Path{Root: "x", Symbol: 1}
	key := path.Key()
	env := constraint.Env{
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			if k == key {
				return typ.NewOptional(typ.String)
			}
			return nil
		},
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			return p.Key()
		},
	}

	dom := NewConditionProofDomain(env)
	cond := constraint.FromDisjuncts([][]constraint.Constraint{
		{constraint.Truthy{Path: path}},
		{constraint.Falsy{Path: path}},
	})

	if !dom.CanSatisfyCondition(cond) {
		t.Fatal("CanSatisfyCondition returned false for satisfiable DNF")
	}
	if len(dom.Type.Narrowed) != 0 {
		t.Fatalf("CanSatisfyCondition mutated type narrowings: %v", dom.Type.Narrowed)
	}
	if !dom.IsTop() {
		t.Fatal("top satisfiability witness should leave product domain at top")
	}
}

func TestConditionProofDomain_CanSatisfyCondition_RejectsUnsatisfiableDNF(t *testing.T) {
	path := constraint.Path{Root: "x", Symbol: 1}
	key := path.Key()
	env := constraint.Env{
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			if k == key {
				return typ.NewOptional(typ.String)
			}
			return nil
		},
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			return p.Key()
		},
	}

	dom := NewConditionProofDomain(env)
	cond := constraint.FromDisjuncts([][]constraint.Constraint{
		{constraint.Truthy{Path: path}, constraint.Falsy{Path: path}},
		{constraint.IsNil{Path: path}, constraint.NotNil{Path: path}},
	})

	if dom.CanSatisfyCondition(cond) {
		t.Fatal("CanSatisfyCondition returned true for unsatisfiable DNF")
	}
}

func TestConditionProofDomain_CanSatisfyCondition_RejectsKeyOfClosedEmptyRecord(t *testing.T) {
	tablePath := constraint.Path{Root: "blocks", Symbol: 1}
	keyPath := constraint.Path{Root: "index", Symbol: 2}
	tableKey := tablePath.Key()
	keyKey := keyPath.Key()
	env := constraint.Env{
		PathTypeAt: func(k constraint.PathKey) typ.Type {
			switch k {
			case tableKey:
				return typ.NewRecord().Build()
			case keyKey:
				return typ.Integer
			default:
				return nil
			}
		},
		ResolvePath: func(p constraint.Path) constraint.PathKey {
			return p.Key()
		},
	}

	dom := NewConditionProofDomain(env)
	if dom.CanSatisfyCondition(constraint.FromConstraints(constraint.KeyOf{Table: tablePath, Key: keyPath})) {
		t.Fatal("CanSatisfyCondition should reject KeyOf on closed empty record")
	}
}
