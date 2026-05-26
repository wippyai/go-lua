package value

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestHigherOrderGrowthRisk_DetectsFunctionReturningFunction(t *testing.T) {
	tp := typ.Func().
		Returns(typ.Func().Returns(typ.String).Build()).
		Build()
	if !HasHigherOrderGrowthRisk(tp) {
		t.Fatalf("expected higher-order growth risk to be detected")
	}
}

func TestHigherOrderGrowthRisk_UsesReturnedCallableSurfaceNotDataTree(t *testing.T) {
	var nested typ.Type = typ.Func().Returns(typ.String).Build()
	for i := 0; i < typ.DefaultRecursionDepth+16; i++ {
		nested = typ.NewRecord().
			Field("next", nested).
			Build()
	}
	tp := typ.Func().Returns(nested).Build()
	if HasHigherOrderGrowthRisk(tp) {
		t.Fatalf("deep data payload should not be treated as a higher-order return surface")
	}

	surface := typ.Func().Returns(typ.NewRecord().
		Field("make", typ.Func().Returns(typ.String).Build()).
		Build()).Build()
	if !HasHigherOrderGrowthRisk(surface) {
		t.Fatalf("expected returned callable surface to be detected")
	}
}

func TestHigherOrderGrowthRisk_UsesParameterCallableSurfaceNotDataTree(t *testing.T) {
	var data typ.Type = typ.Func().Returns(typ.Func().Returns(typ.String).Build()).Build()
	for i := 0; i < typ.DefaultRecursionDepth+16; i++ {
		data = typ.NewRecord().
			Field("next", data).
			Build()
	}
	if HasHigherOrderGrowthRisk(typ.Func().Param("payload", data).Build()) {
		t.Fatalf("deep parameter data payload should not be treated as higher-order growth risk")
	}

	callableParam := typ.Func().
		Param("make", typ.Func().Returns(typ.Func().Returns(typ.String).Build()).Build()).
		Build()
	if !HasHigherOrderGrowthRisk(callableParam) {
		t.Fatalf("direct callable parameter surface should be treated as higher-order growth risk")
	}
}

func TestContainsFunction_CoinductiveRecursiveProductsTerminate(t *testing.T) {
	rec := typ.NewRecursivePlaceholder("Node")
	rec.SetBody(
		typ.NewRecord().
			Field("next", rec).
			Field("make", typ.Func().Returns(typ.String).Build()).
			Build(),
	)
	if !newGrowthScanState().containsFunction(rec.Body, typ.NewGuard()) {
		t.Fatalf("expected function inside recursive product body to be detected")
	}
}

func TestContainsFunction_IgnoresInterfaceMethodSignatures(t *testing.T) {
	iface := typ.NewInterface("Reader", []typ.Method{
		{
			Name: "next",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(typ.Func().Returns(typ.String).Build()).
				Build(),
		},
	})
	if newGrowthScanState().containsFunction(iface, typ.NewGuard()) {
		t.Fatalf("expected interface method signatures to be ignored, got true")
	}
}

func TestMethodTypeHasSelfRecursiveReturn_IgnoresInterfaceMethods(t *testing.T) {
	owner := typ.NewRecord().Field("id", typ.String).Build()
	methodType := typ.NewInterface("HasBuild", []typ.Method{
		{
			Name: "build",
			Type: typ.Func().
				Param("self", typ.Self).
				Returns(owner).
				Build(),
		},
	})
	if newGrowthScanState().methodTypeHasSelfRecursiveReturn(methodType, owner, typ.NewGuard()) {
		t.Fatalf("expected interface method signatures to be ignored for self-recursive detection")
	}
}

func TestMethodTypeHasSelfRecursiveReturn_DetectsOwnerRecordReturnStructurally(t *testing.T) {
	owner := typ.NewRecord().
		Field("id", typ.String).
		Build()
	methodType := typ.Func().
		Param("self", typ.Self).
		Returns(owner).
		Build()

	if !newGrowthScanState().methodTypeHasSelfRecursiveReturn(methodType, owner, typ.NewGuard()) {
		t.Fatalf("expected owner record return to be detected structurally")
	}
}

func TestMethodTypeHasSelfRecursiveReturn_DetectsOwnerRecordInContainer(t *testing.T) {
	owner := typ.NewRecord().
		Field("id", typ.String).
		Build()
	methodType := typ.Func().
		Param("self", typ.Self).
		Returns(typ.NewArray(owner)).
		Build()

	if !newGrowthScanState().methodTypeHasSelfRecursiveReturn(methodType, owner, typ.NewGuard()) {
		t.Fatalf("expected owner record return inside container to be detected structurally")
	}
}

func TestRecordHasSelfRecursiveMethod_DoesNotScanDataFieldsAsMethodSlots(t *testing.T) {
	owner := typ.NewRecord().
		Field("id", typ.String).
		Build()
	var data typ.Type = typ.Func().Returns(owner).Build()
	for i := 0; i < typ.DefaultRecursionDepth+16; i++ {
		data = typ.NewRecord().
			Field("next", data).
			Build()
	}
	if newGrowthScanState().methodTypeHasSelfRecursiveReturn(data, owner, typ.NewGuard()) {
		t.Fatalf("data product should not be treated as a callable method slot")
	}
	if !newGrowthScanState().methodTypeHasSelfRecursiveReturn(typ.Func().Returns(owner).Build(), owner, typ.NewGuard()) {
		t.Fatalf("expected direct callable method slot to be detected")
	}
}

func TestRecordHasSelfRecursiveMethod_MemoizesCallableSurfaceGate(t *testing.T) {
	state := newGrowthScanState()
	record := typ.NewRecord().
		Field("id", typ.String).
		Field("method", typ.Func().Returns(typ.String).Build()).
		Build()

	if state.recordHasSelfRecursiveMethod(record) {
		t.Fatalf("non-self-returning method should not be self-recursive")
	}
	recordSeen := len(state.recordSeen)
	callableSeen := len(state.callableSeen)
	for i := 0; i < 128; i++ {
		if state.recordHasSelfRecursiveMethod(record) {
			t.Fatalf("non-self-returning method became self-recursive on pass %d", i)
		}
	}
	if len(state.recordSeen) != recordSeen {
		t.Fatalf("record callable-surface memo grew from %d to %d", recordSeen, len(state.recordSeen))
	}
	if len(state.callableSeen) != callableSeen {
		t.Fatalf("callable-surface memo grew from %d to %d", callableSeen, len(state.callableSeen))
	}
}

func TestMethodTypeHasSelfRecursiveReturn_DeepReturnShapeUsesIdentityTraversal(t *testing.T) {
	owner := typ.NewRecord().
		Field("id", typ.String).
		Build()
	var returned typ.Type = owner
	for i := 0; i < typ.DefaultRecursionDepth+16; i++ {
		returned = typ.NewRecord().
			Field("next", returned).
			Field("method", typ.Func().Returns(returned).Build()).
			Build()
	}
	method := typ.Func().Returns(returned).Build()
	state := newGrowthScanState()
	for i := 0; i < 64; i++ {
		if !state.methodTypeHasSelfRecursiveReturn(method, owner, typ.NewGuard()) {
			t.Fatalf("deep returned owner shape should remain recognized on pass %d", i)
		}
	}
}

func TestCallableSurfaceScanUsesGrowthMemoForUnionWrappers(t *testing.T) {
	members := make([]typ.Type, 0, 256)
	for i := 0; i < cap(members); i++ {
		members = append(members,
			typ.NewRecord().
				Field("id", typ.LiteralInt(int64(i))).
				Field("payload", typ.NewArray(typ.String)).
				Build(),
		)
	}
	union := typ.NewUnion(members...)
	state := newGrowthScanState()

	if state.hasCallableTypeSurface(union) {
		t.Fatalf("data-only union should not have a callable surface")
	}
	callableSeen := len(state.callableSeen)
	for i := 0; i < 64; i++ {
		if state.hasCallableTypeSurface(union) {
			t.Fatalf("data-only union became callable on pass %d", i)
		}
	}
	if len(state.callableSeen) != callableSeen {
		t.Fatalf("callable-surface memo grew from %d to %d", callableSeen, len(state.callableSeen))
	}
}

func TestHigherOrderGrowthRisk_DataRecordsDoNotEnterMethodRecursionScan(t *testing.T) {
	members := make([]typ.Type, 0, 128)
	for i := 0; i < cap(members); i++ {
		members = append(members,
			typ.NewRecord().
				Field("id", typ.LiteralInt(int64(i))).
				Field("payload", typ.NewArray(typ.String)).
				Build(),
		)
	}
	state := newGrowthScanState()

	if state.hasHigherOrderGrowthRisk(typ.NewUnion(members...), typ.NewGuard()) {
		t.Fatalf("data-only union should not have higher-order growth risk")
	}
	if len(state.selfMethodSeen) != 0 {
		t.Fatalf("data-only records entered self-method scan %d times", len(state.selfMethodSeen))
	}
	if len(state.recordSeen) != 0 {
		t.Fatalf("data-only records entered callable record scan %d times", len(state.recordSeen))
	}
}

func TestHigherOrderGrowthRisk_RecordUsesCallableSurfaceNotDataTree(t *testing.T) {
	var data typ.Type = typ.Func().Returns(typ.Func().Returns(typ.String).Build()).Build()
	for i := 0; i < typ.DefaultRecursionDepth+16; i++ {
		data = typ.NewRecord().
			Field("next", data).
			Build()
	}
	record := typ.NewRecord().
		Field("data", data).
		Build()
	if HasHigherOrderGrowthRisk(record) {
		t.Fatalf("data product should not make the parent record higher-order risky")
	}
	module := typ.NewRecord().
		Field("make", typ.Func().Returns(typ.Func().Returns(typ.String).Build()).Build()).
		Build()
	if !HasHigherOrderGrowthRisk(module) {
		t.Fatalf("direct callable surface returning a function should be higher-order risky")
	}
}

func TestHigherOrderGrowthRisk_LargeStructuralUnionUsesNodeIdentityTraversal(t *testing.T) {
	members := make([]typ.Type, 0, 512)
	for i := 0; i < cap(members); i++ {
		members = append(members,
			typ.NewRecord().
				Field("id", typ.String).
				Field("payload", typ.NewRecord().
					Field("next", typ.NewArray(typ.NewRecord().Field("value", typ.Integer).Build())).
					Build()).
				Build(),
		)
	}

	if HasHigherOrderGrowthRisk(typ.NewUnion(members...)) {
		t.Fatalf("large structural data union should not be classified as higher-order growth risk")
	}
}

func TestWidenForConvergence_CoalescesProductUnionBeforeGrowthScan(t *testing.T) {
	members := make([]typ.Type, 0, 128)
	for i := 0; i < cap(members); i++ {
		members = append(members,
			typ.NewRecord().
				Field("name", typ.LiteralString(string(rune('a'+i%26)))).
				Field("line", typ.LiteralInt(int64(i))).
				Field("children", typ.NewArray(typ.NewRecord().Field("name", typ.String).Build())).
				Build(),
		)
	}

	got := WidenForConvergence(typ.NewUnion(members...))
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("WidenForConvergence(product union) = %T, want compact record", got)
	}
	nameField := rec.GetField("name")
	if nameField == nil || !typ.TypeEquals(nameField.Type, typ.String) {
		t.Fatalf("ordinary literal name field should widen to string, got %v", nameField)
	}
	lineField := rec.GetField("line")
	if lineField == nil || !typ.TypeEquals(lineField.Type, typ.Integer) {
		t.Fatalf("ordinary literal line field should widen to integer, got %v", lineField)
	}
}

func TestWidenForConvergence_FoldsSelfReturningMethodRecord(t *testing.T) {
	seed := typ.NewRecord().
		Field("new", typ.Func().Returns(typ.Unknown).Build()).
		Field("method", typ.Func().Returns(typ.Unknown).Build()).
		Build()
	owner := typ.NewRecord().
		Field("new", typ.Func().Returns(seed).Build()).
		Field("method", typ.Func().Param("self", seed).Returns(seed).Build()).
		Build()

	widened := WidenForConvergence(owner)
	rec, ok := widened.(*typ.Recursive)
	if !ok {
		t.Fatalf("WidenForConvergence() = %T, want recursive product", widened)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T, want record", rec.Body)
	}
	methodField := body.GetField("method")
	if methodField == nil {
		t.Fatal("recursive body lost method field")
	}
	methodType, ok := methodField.Type.(*typ.Function)
	if !ok {
		t.Fatalf("method field = %T, want function", methodField.Type)
	}
	if len(methodType.Params) != 1 || !typ.IsRecursiveRef(methodType.Params[0].Type, rec) {
		t.Fatalf("method self param should be recursive ref, got %#v", methodType.Params)
	}
	if len(methodType.Returns) != 1 || !typ.IsRecursiveRef(methodType.Returns[0], rec) {
		t.Fatalf("method return should be recursive ref, got %#v", methodType.Returns)
	}
}

func TestWidenForConvergence_FoldsSetmetatableModulePattern(t *testing.T) {
	seed := typ.NewRecord().
		Field("new", typ.Func().Returns(typ.Unknown).Build()).
		Field("method", typ.Func().Returns(typ.Unknown).Build()).
		Build()
	instance := typ.NewRecord().
		Metatable(typ.NewRecord().Field("__index", seed).Build()).
		Build()
	owner := typ.NewRecord().
		Field("new", typ.Func().Returns(instance).Build()).
		Field("method", typ.Func().Param("self", seed).Returns(seed).Build()).
		Build()

	widened := WidenForConvergence(owner)
	rec, ok := widened.(*typ.Recursive)
	if !ok {
		t.Fatalf("WidenForConvergence() = %T, want recursive product", widened)
	}
	body, ok := rec.Body.(*typ.Record)
	if !ok {
		t.Fatalf("recursive body = %T, want record", rec.Body)
	}
	newField := body.GetField("new")
	if newField == nil {
		t.Fatal("recursive body lost constructor field")
	}
	newType, ok := newField.Type.(*typ.Function)
	if !ok {
		t.Fatalf("constructor field = %T, want function", newField.Type)
	}
	if len(newType.Returns) != 1 {
		t.Fatalf("constructor returns %#v, want one return", newType.Returns)
	}
	instanceBody, ok := newType.Returns[0].(*typ.Record)
	if !ok || instanceBody.Metatable == nil {
		t.Fatalf("constructor return = %T, want record with metatable", newType.Returns[0])
	}
	meta, ok := instanceBody.Metatable.(*typ.Record)
	if !ok {
		t.Fatalf("metatable = %T, want record", instanceBody.Metatable)
	}
	index := meta.GetField("__index")
	if index == nil || !typ.IsRecursiveRef(index.Type, rec) {
		t.Fatalf("metatable __index should be recursive ref, got %#v", index)
	}
}
