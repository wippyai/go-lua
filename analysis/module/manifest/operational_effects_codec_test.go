package manifest

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/module/signature"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestOperationalEffectsWireFieldsMirrorSignatureFields(t *testing.T) {
	signatureFields := operationalEffectFieldNames(reflect.TypeOf(signature.OperationalEffects{}))
	wireFields := operationalEffectFieldNames(reflect.TypeOf(operationalEffectsWire{}))

	for field := range signatureFields {
		if _, ok := wireFields[field]; !ok {
			t.Fatalf("signature.OperationalEffects.%s has no operationalEffectsWire field", field)
		}
	}
	for field := range wireFields {
		if _, ok := signatureFields[field]; !ok {
			t.Fatalf("operationalEffectsWire.%s has no signature.OperationalEffects field", field)
		}
	}
}

func TestOperationalEffectsWireLaneRegistryCoversEveryWireField(t *testing.T) {
	wireFields := operationalEffectFieldNames(reflect.TypeOf(operationalEffectsWire{}))
	registered := make(map[string]struct{})
	for _, lane := range operationalEffectsWireLanes {
		if lane.fieldName == "" {
			t.Fatal("operational effects wire lane with empty field name")
		}
		if lane.encode == nil {
			t.Fatalf("operational effects wire lane %s has nil encoder", lane.fieldName)
		}
		if lane.decode == nil {
			t.Fatalf("operational effects wire lane %s has nil decoder", lane.fieldName)
		}
		if lane.canonicalize == nil {
			t.Fatalf("operational effects wire lane %s has nil canonicalizer", lane.fieldName)
		}
		if _, ok := registered[lane.fieldName]; ok {
			t.Fatalf("operational effects wire lane %s registered more than once", lane.fieldName)
		}
		if _, ok := wireFields[lane.fieldName]; !ok {
			t.Fatalf("operational effects wire lane references missing field %s", lane.fieldName)
		}
		registered[lane.fieldName] = struct{}{}
	}
	for field := range wireFields {
		if _, ok := registered[field]; !ok {
			t.Fatalf("operationalEffectsWire.%s has no registered wire lane", field)
		}
	}
}

func TestOperationalEffectsWireRefsHaveBoundaryOwners(t *testing.T) {
	wireFields := make(map[string]struct{})
	for _, lane := range operationalEffectsWireLanes {
		wireFields[lane.fieldName] = struct{}{}
	}

	owners := make(map[string][]string)
	collect := func(family string, refs []string) {
		for _, ref := range refs {
			if ref == "" {
				t.Fatalf("%s has an empty operational-effects wire reference", family)
			}
			if _, ok := wireFields[ref]; !ok {
				t.Fatalf("%s references missing operational-effects wire lane %q", family, ref)
			}
			owners[ref] = append(owners[ref], family)
		}
	}

	for _, desc := range callboundary.NormalReturnFactDescriptors() {
		collect("normal-return:"+string(desc.Kind), desc.WireRef)
	}
	for _, desc := range callpayload.CallOutcomeDescriptors() {
		collect("call-outcome:"+string(desc.Kind), desc.WireRef)
	}
	for _, desc := range summary.SummaryFactDescriptors() {
		collect("summary:"+string(desc.Kind), desc.WireRef)
	}

	localOnly := map[string]string{
		"ReturnAllocationTemplates": "allocation templates serialize signature return-shape construction plans, not call-boundary facts",
	}
	for field := range wireFields {
		if len(owners[field]) != 0 {
			continue
		}
		if reason := localOnly[field]; reason == "" {
			t.Fatalf("operational-effects wire lane %s has no boundary descriptor owner", field)
		}
	}
}

func TestEncodeDecodeOperationalEffectsUseLaneRegistry(t *testing.T) {
	file := parseOperationalEffectsCodecSource(t)
	fields := operationalEffectFieldNames(reflect.TypeOf(operationalEffectsWire{}))
	for _, name := range []string{"encodeOperationalEffects", "decodeOperationalEffects"} {
		fn := requireManifestFuncDecl(t, file, name)
		if !manifestFuncUsesIdent(fn, "operationalEffectsWireLanes") {
			t.Fatalf("%s must iterate operationalEffectsWireLanes", name)
		}
		if field := firstSelectedManifestField(fn, fields); field != "" {
			t.Fatalf("%s selects field %s directly; use operationalEffectsWireLanes", name, field)
		}
	}
}

func TestCanonicalizeOperationalEffectsWireUsesLaneRegistry(t *testing.T) {
	file := parseOperationalEffectsCodecSource(t)
	fn := requireManifestFuncDecl(t, file, "canonicalizeOperationalEffectsWire")
	if !manifestFuncUsesIdent(fn, "operationalEffectsWireLanes") {
		t.Fatal("canonicalizeOperationalEffectsWire must iterate operationalEffectsWireLanes")
	}
	if field := firstSelectedManifestField(fn, operationalEffectFieldNames(reflect.TypeOf(operationalEffectsWire{}))); field != "" {
		t.Fatalf("canonicalizeOperationalEffectsWire selects field %s directly; use operationalEffectsWireLanes", field)
	}
}

func TestCanonicalOperationalEffectsDigestBytesSupportsRecursiveTypes(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("next", typeexpr.Optional(self)).Build()
	})
	encoded, err := CanonicalOperationalEffectsDigestBytes(&signature.OperationalEffects{
		NormalReturnTypeRefinements: []signature.PathTypeRefinement{{
			Path: pathdom.NewPlaceholder(0),
			Type: node,
		}},
	})
	if err != nil {
		t.Fatalf("CanonicalOperationalEffectsDigestBytes: %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("CanonicalOperationalEffectsDigestBytes returned empty bytes")
	}
}

func TestCanonicalOperationalEffectsDigestIsDeterministicAcrossInputOrder(t *testing.T) {
	left, err := CanonicalOperationalEffectsDigest(context.Background(), operationalEffectsOrderA())
	if err != nil {
		t.Fatalf("CanonicalOperationalEffectsDigest(left): %v", err)
	}
	right, err := CanonicalOperationalEffectsDigest(context.Background(), operationalEffectsOrderB())
	if err != nil {
		t.Fatalf("CanonicalOperationalEffectsDigest(right): %v", err)
	}
	again, err := CanonicalOperationalEffectsDigest(context.Background(), operationalEffectsOrderA())
	if err != nil {
		t.Fatalf("CanonicalOperationalEffectsDigest(again): %v", err)
	}
	if left != right || left != again {
		t.Fatalf("canonical operational-effect digests = %d, %d, %d; want one stable value", left, right, again)
	}
}

func operationalEffectFieldNames(typ reflect.Type) map[string]struct{} {
	out := make(map[string]struct{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() == reflect.Bool || field.Type.Kind() == reflect.Slice {
			out[field.Name] = struct{}{}
		}
	}
	return out
}

func parseOperationalEffectsCodecSource(t *testing.T) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "operational_effects_codec.go", nil, 0)
	if err != nil {
		t.Fatalf("parse operational_effects_codec.go: %v", err)
	}
	return file
}

func requireManifestFuncDecl(t *testing.T, file *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return fn
		}
	}
	t.Fatalf("function %s not found", name)
	return nil
}

func manifestFuncUsesIdent(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		if ok && ident.Name == name {
			found = true
			return false
		}
		return true
	})
	return found
}

func firstSelectedManifestField(fn *ast.FuncDecl, fields map[string]struct{}) string {
	var found string
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if ok {
			if _, isField := fields[sel.Sel.Name]; isField {
				found = sel.Sel.Name
				return false
			}
		}
		return true
	})
	return found
}
