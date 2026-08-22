package typ

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"math"
	"testing"

	"github.com/wippyai/go-lua/domain/type/kind"
)

func TestDecodeCanonicalFormalsRoundTripsScopedCorpus(t *testing.T) {
	sourceExternal := NewTypeParam("SourceExternal", LiteralString("bound"))
	receiverExternal := NewTypeParam("ReceiverExternal", LiteralString("bound"))
	local := NewTypeParam("Local", MaterializeUnion([]Type{String, Integer}))
	function := Func().TypeParamRef(local).Param("value", local).Returns(sourceExternal, local).Build()
	item := NewTypeParam("Item", nil)
	channel := NewGeneric("Channel", []*TypeParam{item}, NewArray(item))
	recursive := NewRecursive("presentation", func(self Type) Type {
		return NewTuple(self, sourceExternal)
	})
	recursiveParam := NewTypeParam("T", nil)
	recursiveGeneric := NewGeneric("Recursive", []*TypeParam{recursiveParam}, nil)
	recursiveGeneric.SetBody(RebuildRecord(RecordParts{Fields: []Field{{Name: "next", Type: Instantiate(recursiveGeneric, recursiveParam)}}}))
	record := RebuildRecord(RecordParts{
		Fields: []Field{{Name: "value", Type: sourceExternal, Optional: true}},
		MapKey: String, MapValue: channel, Metatable: NewMeta(function), Open: true, AssumeSorted: true,
	})
	iface := NewInterface("Reader", []Method{{Name: "read", Type: function}})

	corpus := []struct {
		name  string
		value Type
	}{
		{"external", NewTuple(sourceExternal, NewArray(sourceExternal))},
		{"function", function},
		{"generic", channel},
		{"instantiated", Instantiate(channel, String)},
		{"record", record},
		{"interface", iface},
		{"recursive", recursive},
		{"recursive-generic", recursiveGeneric},
		{"nested-binders", canonicalScopedNested("Outer", "Inner")},
	}
	for _, test := range corpus {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := EncodeCanonicalFormals(context.Background(), test.value, []*TypeParam{sourceExternal})
			if err != nil {
				t.Fatalf("EncodeCanonicalFormals: %v", err)
			}
			decoded, err := DecodeCanonicalFormals(context.Background(), encoded, []*TypeParam{receiverExternal})
			if err != nil {
				t.Fatalf("DecodeCanonicalFormals: %v", err)
			}
			roundTrip, err := EncodeCanonicalFormals(context.Background(), decoded, []*TypeParam{receiverExternal})
			if err != nil || !bytes.Equal(encoded.Bytes(), roundTrip.Bytes()) {
				t.Fatalf("scoped bytes changed: %x / %x / %v", encoded.Bytes(), roundTrip.Bytes(), err)
			}
		})
	}

	encoded, err := EncodeCanonicalFormals(context.Background(), NewTuple(sourceExternal, NewArray(sourceExternal)), []*TypeParam{sourceExternal})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCanonicalFormals(context.Background(), encoded, []*TypeParam{receiverExternal})
	if err != nil {
		t.Fatal(err)
	}
	tuple, ok := decoded.(*Tuple)
	if !ok || len(tuple.Elements) != 2 || tuple.Elements[0] != receiverExternal {
		t.Fatalf("external authority identity = %#v, want supplied receiver pointer", decoded)
	}
}

func TestDecodeCanonicalFormalsRoundTripsRawIEEEFloatLiterals(t *testing.T) {
	for _, bits := range []uint64{
		0x0000000000000000,
		0x8000000000000000,
		0x7ff0000000000000,
		0xfff0000000000000,
		0x7ff8000000000001,
		0x7ff8000000000002,
		0x7ff0000000000001,
	} {
		original := LiteralNumber(math.Float64frombits(bits))
		encoded, err := EncodeCanonicalFormals(context.Background(), original, nil)
		if err != nil {
			t.Fatalf("encode %#x: %v", bits, err)
		}
		decoded, err := DecodeCanonicalFormals(context.Background(), encoded, nil)
		if err != nil {
			t.Fatalf("decode %#x: %v", bits, err)
		}
		literal, ok := decoded.(*Literal)
		if !ok || literal.Base() != kind.Number || math.Float64bits(literal.Value().(float64)) != bits || !TypeEquals(original, decoded) {
			t.Fatalf("scoped decode lost %#x: %#v", bits, decoded)
		}
		roundTrip, err := EncodeCanonicalFormals(context.Background(), decoded, nil)
		if err != nil || !bytes.Equal(encoded.Bytes(), roundTrip.Bytes()) {
			t.Fatalf("scoped bytes changed for %#x: %x / %x / %v", bits, encoded.Bytes(), roundTrip.Bytes(), err)
		}
	}
}

func TestDecodeCanonicalFormalsRejectsAuthorityAndWireMismatches(t *testing.T) {
	source := NewTypeParam("Source", String)
	encoded, err := EncodeCanonicalFormals(context.Background(), NewTuple(source, NewArray(source)), []*TypeParam{source})
	if err != nil {
		t.Fatal(err)
	}
	wrongConstraint := NewTypeParam("Receiver", Number)
	duplicate := NewTypeParam("Duplicate", nil)

	cases := []struct {
		name    string
		receipt CanonicalFormalsReceipt
		formals []*TypeParam
	}{
		{"wrong external count", encoded, nil},
		{"foreign external constraint", encoded, []*TypeParam{wrongConstraint}},
		{"nil external formal", encoded, []*TypeParam{nil}},
		{"duplicate external formal", encoded, []*TypeParam{duplicate, duplicate}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			decoded, err := DecodeCanonicalFormals(context.Background(), test.receipt, test.formals)
			if decoded != nil || !errors.Is(err, ErrInvalidCanonicalType) {
				t.Fatalf("DecodeCanonicalFormals = %T, %v; want invalid canonical error", decoded, err)
			}
		})
	}
	for name, raw := range map[string][]byte{
		"trailing data": append(append([]byte(nil), encoded.Bytes()...), 0),
		"wrong framing": appendCanonicalFormalDefinition(nil, 0, []byte{canonicalString}, 0),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := AdmitCanonicalFormals(context.Background(), raw, 1); !errors.Is(err, ErrInvalidCanonicalType) {
				t.Fatalf("AdmitCanonicalFormals = %v; want invalid canonical error", err)
			}
		})
	}

	badLocalOwner := appendCanonicalFormalsVersion(nil)
	badLocalOwner = appendCanonicalFormalDefinition(badLocalOwner, 0, []byte{canonicalTypeParam, canonicalScopedLocalFormal, 0, 0}, 1)
	badLocalOwner = appendCanonicalFormalReference(badLocalOwner, 0)
	if _, err := AdmitCanonicalFormals(context.Background(), badLocalOwner, 0); !errors.Is(err, ErrInvalidCanonicalType) {
		t.Fatalf("malformed local owner = %v", err)
	}
}

func TestDecodeCanonicalFormalsRestoresNestedBinderIdentity(t *testing.T) {
	original := canonicalScopedNested("Outer", "Inner")
	encoded, err := EncodeCanonicalFormals(context.Background(), original, nil)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCanonicalFormals(context.Background(), encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	outer, ok := decoded.(*Generic)
	if !ok || len(outer.TypeParams) != 1 {
		t.Fatalf("outer binder = %#v", decoded)
	}
	function, ok := outer.Body.(*Function)
	if !ok || len(function.TypeParams) != 1 || len(function.Params) != 1 || len(function.Returns) != 2 {
		t.Fatalf("nested function = %#v", outer.Body)
	}
	outerFormal, innerFormal := outer.TypeParams[0], function.TypeParams[0]
	if outerFormal == innerFormal || function.Params[0].Type != outerFormal || function.Returns[0] != innerFormal || function.Returns[1] != outerFormal {
		t.Fatalf("lexical binder identities were not reconstructed: %#v", function)
	}
}

func TestCanonicalFormalsRejectEscapedLocalBinders(t *testing.T) {
	newOwner := func(name string) (*TypeParam, *Generic) {
		formal := NewTypeParam("T", nil)
		return formal, NewGeneric(name, []*TypeParam{formal}, NewArray(formal))
	}
	formal, owner := newOwner("Owner")
	direct := NewTuple(formal, owner)
	if receipt, err := EncodeCanonicalFormals(context.Background(), direct, nil); err == nil || receipt.Valid() {
		t.Fatalf("direct local-formal escape encoded as %x, %v", receipt.Bytes(), err)
	}
	// Canonical bytes emitted by the previous encoder for direct. Keeping the
	// artifact fixed proves the shared raw validator, not only source encoding,
	// rejects the historical scope leak.
	unsafe, err := hex.DecodeString("2977697070792e616e616c797369732e747970652e7479702e63616e6f6e6963616c2d666f726d616c7301010002100202010104180200000101020916054f776e65720101020001010301110100010002")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AdmitCanonicalFormals(context.Background(), unsafe, 0); !errors.Is(err, ErrInvalidCanonicalType) {
		t.Fatalf("admitter accepted direct local-formal escape: %v", err)
	}
	// p is the graph root and reaches its owner through the local-owner edge.
	// Checking only owner-dominates-predecessor would miss this virtual use.
	rootScalar := appendFrameString([]byte{canonicalGeneric}, "G")
	rootScalar = appendCount(rootScalar, 1)
	rootScalar = appendBool(rootScalar, true)
	rootFormal := appendCanonicalFormalsVersion(nil)
	rootFormal = appendCanonicalFormalDefinition(rootFormal, 0, []byte{canonicalTypeParam, canonicalScopedLocalFormal, 0, 0}, 1)
	rootFormal = appendCanonicalFormalDefinition(rootFormal, 1, rootScalar, 2)
	rootFormal = appendCanonicalFormalReference(rootFormal, 0)
	rootFormal = appendCanonicalFormalDefinition(rootFormal, 2, []byte{canonicalArray}, 1)
	rootFormal = appendCanonicalFormalReference(rootFormal, 0)
	if _, err := AdmitCanonicalFormals(context.Background(), rootFormal, 0); !errors.Is(err, ErrInvalidCanonicalType) {
		t.Fatalf("admitter accepted root local formal: %v", err)
	}

	formal, left := newOwner("Left")
	right := NewGeneric("Right", nil, NewArray(formal))
	if receipt, err := EncodeCanonicalFormals(context.Background(), NewTuple(left, right), nil); err == nil || receipt.Valid() {
		t.Fatalf("cross-sibling local-formal escape encoded as %x, %v", receipt.Bytes(), err)
	}

	formal, nestedOwner := newOwner("Nested")
	if receipt, err := EncodeCanonicalFormals(context.Background(), NewTuple(NewArray(formal), nestedOwner), nil); err == nil || receipt.Valid() {
		t.Fatalf("later-owner local-formal escape encoded as %x, %v", receipt.Bytes(), err)
	}
}

func TestCanonicalFormalsAcceptNestedAndRecursiveLocalScopes(t *testing.T) {
	nested := canonicalScopedNested("Outer", "Inner")
	formal := NewTypeParam("T", nil)
	owner := NewGeneric("Owner", []*TypeParam{formal}, NewArray(formal))
	recursive := NewRecursive("R", func(self Type) Type {
		return NewTuple(owner, self)
	})
	for name, value := range map[string]Type{"nested": nested, "recursive": recursive} {
		t.Run(name, func(t *testing.T) {
			encoded, err := EncodeCanonicalFormals(context.Background(), value, nil)
			if err != nil {
				t.Fatalf("EncodeCanonicalFormals: %v", err)
			}
			if _, err := AdmitCanonicalFormals(context.Background(), encoded.Bytes(), 0); err != nil {
				t.Fatalf("AdmitCanonicalFormals: %v", err)
			}
		})
	}
}

func TestCanonicalFormalsRejectNonMuFormalConstraintCycle(t *testing.T) {
	formal := NewTypeParam("T", nil)
	formal.Constraint = formal // Deliberately malformed mutable construction.
	owner := NewGeneric("Owner", []*TypeParam{formal}, NewArray(formal))
	if receipt, err := EncodeCanonicalFormals(context.Background(), owner, nil); err == nil || receipt.Valid() {
		t.Fatalf("self-constrained local formal encoded as %x, %v", receipt.Bytes(), err)
	}
	if receipt, err := EncodeCanonicalFormals(context.Background(), formal, []*TypeParam{formal}); err == nil || receipt.Valid() {
		t.Fatalf("self-constrained external formal encoded as %x, %v", receipt.Bytes(), err)
	}

	rootScalar := appendFrameString([]byte{canonicalGeneric}, "Owner")
	rootScalar = appendCount(rootScalar, 1)
	rootScalar = appendBool(rootScalar, true)
	formalScalar := []byte{canonicalTypeParam, canonicalScopedLocalFormal, 0, 1}
	wire := appendCanonicalFormalsVersion(nil)
	wire = appendCanonicalFormalDefinition(wire, 0, rootScalar, 2)
	wire = appendCanonicalFormalDefinition(wire, 1, formalScalar, 2)
	wire = appendCanonicalFormalReference(wire, 0) // lexical owner
	wire = appendCanonicalFormalReference(wire, 1) // formal constraint itself
	wire = appendCanonicalFormalDefinition(wire, 2, []byte{canonicalArray}, 1)
	wire = appendCanonicalFormalReference(wire, 1)
	if _, err := AdmitCanonicalFormals(context.Background(), wire, 0); !errors.Is(err, ErrInvalidCanonicalType) {
		t.Fatalf("admitter accepted non-Mu formal cycle: %v", err)
	}
}

func TestDecodeCanonicalFormalsAllowsExplicitMuConstraintBoundary(t *testing.T) {
	formal := NewTypeParam("T", nil)
	recursive := NewRecursivePlaceholder("R")
	formal.Constraint = recursive // Construction-only mutation for this legal Mu graph.
	owner := NewGeneric("Owner", []*TypeParam{formal}, NewArray(formal))
	recursive.SetBody(formal)
	encoded, err := EncodeCanonicalFormals(context.Background(), owner, nil)
	if err != nil {
		t.Fatalf("EncodeCanonicalFormals explicit Mu boundary: %v", err)
	}
	decoded, err := DecodeCanonicalFormals(context.Background(), encoded, nil)
	if err != nil {
		t.Fatalf("DecodeCanonicalFormals explicit Mu boundary: %v", err)
	}
	roundTrip, err := EncodeCanonicalFormals(context.Background(), decoded, nil)
	if err != nil || !bytes.Equal(encoded.Bytes(), roundTrip.Bytes()) {
		t.Fatalf("explicit Mu bytes changed: %x / %x / %v", encoded.Bytes(), roundTrip.Bytes(), err)
	}
}

func TestCanonicalFormalsRejectMutualGenericRecurrence(t *testing.T) {
	firstParam := NewTypeParam("T", nil)
	secondParam := NewTypeParam("U", nil)
	first := NewGeneric("G0", []*TypeParam{firstParam}, nil)
	second := NewGeneric("G1", []*TypeParam{secondParam}, nil)
	first.SetBody(second)
	second.SetBody(NewTuple(first, Any))
	if receipt, err := EncodeCanonicalFormals(context.Background(), first, nil); err == nil || receipt.Valid() {
		t.Fatalf("mutual Generic recurrence encoded as %x, %v", receipt.Bytes(), err)
	}

	firstScalar := appendFrameString([]byte{canonicalGeneric}, "G0")
	firstScalar = appendCount(firstScalar, 1)
	firstScalar = appendBool(firstScalar, true)
	secondScalar := appendFrameString([]byte{canonicalGeneric}, "G1")
	secondScalar = appendCount(secondScalar, 1)
	secondScalar = appendBool(secondScalar, true)
	formalScalar := []byte{canonicalTypeParam, canonicalScopedLocalFormal, 0, 0}
	wire := appendCanonicalFormalsVersion(nil)
	wire = appendCanonicalFormalDefinition(wire, 0, firstScalar, 2)
	wire = appendCanonicalFormalDefinition(wire, 1, formalScalar, 1)
	wire = appendCanonicalFormalReference(wire, 0)
	wire = appendCanonicalFormalDefinition(wire, 2, secondScalar, 2)
	wire = appendCanonicalFormalDefinition(wire, 3, formalScalar, 1)
	wire = appendCanonicalFormalReference(wire, 2)
	wire = appendCanonicalFormalDefinition(wire, 4, []byte{canonicalTuple, 2}, 2)
	wire = appendCanonicalFormalReference(wire, 0)
	wire = appendCanonicalFormalDefinition(wire, 5, []byte{canonicalAny}, 0)
	if _, err := AdmitCanonicalFormals(context.Background(), wire, 0); !errors.Is(err, ErrInvalidCanonicalType) {
		t.Fatalf("admitter accepted mutual Generic recurrence: %v", err)
	}
}

func TestDecodeCanonicalFormalsFinalizesSelfGenericSemanticFlags(t *testing.T) {
	formal := NewTypeParam("T", nil)
	generic := NewGeneric("G", []*TypeParam{formal}, nil)
	generic.SetBody(NewTuple(generic, Any))
	encoded, err := EncodeCanonicalFormals(context.Background(), generic, nil)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCanonicalFormals(context.Background(), encoded, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := decoded.(*Generic)
	if !ok || resolved.Body == nil || resolved.Hash() == 0 || !ContainsAny(resolved) {
		t.Fatalf("self Generic semantic state not finalized: %#v", decoded)
	}
	body, ok := resolved.Body.(*Tuple)
	if !ok || len(body.Elements) != 2 || body.Elements[0] != resolved || body.Elements[1] != Any {
		t.Fatalf("self Generic body identity = %#v", resolved.Body)
	}
}

func TestDecodeCanonicalFormalsDefersNewlyReadyGenericCandidate(t *testing.T) {
	firstFormal := NewTypeParam("T", nil)
	first := NewGeneric("G1", []*TypeParam{firstFormal}, nil)
	first.SetBody(NewTuple(first, Any)) // Direct body edge: deliberately not an Instantiate.
	secondFormal := NewTypeParam("U", nil)
	second := NewGeneric("G0", []*TypeParam{secondFormal}, first)
	root := NewTuple(first, second)
	encoded, err := EncodeCanonicalFormals(context.Background(), root, nil)
	if err != nil {
		t.Fatalf("EncodeCanonicalFormals: %v", err)
	}
	decoded, err := DecodeCanonicalFormals(context.Background(), encoded, nil)
	if err != nil {
		t.Fatalf("DecodeCanonicalFormals direct-body scheduling: %v", err)
	}
	tuple, ok := decoded.(*Tuple)
	if !ok || len(tuple.Elements) != 2 {
		t.Fatalf("decoded direct-body root = %#v", decoded)
	}
	secondDecoded, ok := tuple.Elements[1].(*Generic)
	if !ok || !ContainsAny(secondDecoded) || secondDecoded.Hash() == 0 {
		t.Fatalf("dependent Generic captured open body state: %#v", tuple.Elements[1])
	}
	roundTrip, err := EncodeCanonicalFormals(context.Background(), decoded, nil)
	if err != nil || !bytes.Equal(encoded.Bytes(), roundTrip.Bytes()) {
		t.Fatalf("direct-body scheduling bytes changed: %x / %x / %v", encoded.Bytes(), roundTrip.Bytes(), err)
	}
}

func TestCanonicalFormalsRejectCyclesWithoutMaterializableHead(t *testing.T) {
	optional := &Optional{}
	optional.Inner = optional
	array := &Array{}
	array.Element = array
	function := &Function{}
	function.Returns = []Type{function}
	genericSibling := NewGeneric("G", nil, optional)
	for name, value := range map[string]Type{
		"optional":        optional,
		"array":           array,
		"function":        function,
		"generic-sibling": NewTuple(genericSibling, optional),
	} {
		t.Run(name, func(t *testing.T) {
			if receipt, err := EncodeCanonicalFormals(context.Background(), value, nil); err == nil || receipt.Valid() {
				t.Fatalf("unheaded cycle encoded as %x, %v", receipt.Bytes(), err)
			}
		})
	}

	wire := appendCanonicalFormalsVersion(nil)
	wire = appendCanonicalFormalDefinition(wire, 0, []byte{canonicalOptional}, 1)
	wire = appendCanonicalFormalReference(wire, 0)
	if _, err := AdmitCanonicalFormals(context.Background(), wire, 0); !errors.Is(err, ErrInvalidCanonicalType) {
		t.Fatalf("admitter accepted unheaded cycle: %v", err)
	}
}

func TestDecodeCanonicalFormalsCancellationAndDeepGraph(t *testing.T) {
	const depth = 100_001
	var value Type = String
	for range depth {
		value = &Optional{Inner: value}
	}
	encoded, err := EncodeCanonicalFormals(context.Background(), value, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if decoded, err := DecodeCanonicalFormals(ctx, encoded, nil); decoded != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled DecodeCanonicalFormals = %T, %v", decoded, err)
	}
	decoded, err := DecodeCanonicalFormals(context.Background(), encoded, nil)
	if err != nil {
		depth := 0
		for current := decoded; current != nil; {
			optional, ok := current.(*Optional)
			if !ok {
				break
			}
			depth++
			current = optional.Inner
		}
		t.Fatalf("deep DecodeCanonicalFormals (optional depth %d): %v", depth, err)
	}
	roundTrip, err := EncodeCanonicalFormals(context.Background(), decoded, nil)
	if err != nil || !bytes.Equal(encoded.Bytes(), roundTrip.Bytes()) {
		at := 0
		encodedBytes, roundTripBytes := encoded.Bytes(), roundTrip.Bytes()
		for at < len(encodedBytes) && at < len(roundTripBytes) && encodedBytes[at] == roundTripBytes[at] {
			at++
		}
		t.Fatalf("deep bytes changed: %d / %d at %d: %v", len(encodedBytes), len(roundTripBytes), at, err)
	}
}

// TestDecodeCanonicalFormalsCancelsDuringPostParseLaws distinguishes a
// pre-cancelled request from cancellation after parsing has completed. The
// context permits the entry check plus every checkpoint in the known deep
// preorder parse, then cancels at the first relation-law checkpoint.
func TestDecodeCanonicalFormalsCancelsDuringPostParseLaws(t *testing.T) {
	const depth = 100_001
	var value Type = String
	for range depth {
		value = &Optional{Inner: value}
	}
	encoded, err := EncodeCanonicalFormals(context.Background(), value, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Decode checks Err once at entry. parseCanonicalFormalsGraph then checks
	// at event 1 and every 64 events. There are depth Optional definitions plus
	// the String leaf.
	nodeCount := depth + 1
	parseChecks := 1 + (nodeCount-1)/64
	ctx := &canonicalFormalCancelAfterContext{
		Context:         context.Background(),
		allowedErrCalls: 1 + parseChecks,
		done:            make(chan struct{}),
	}
	decoded, err := DecodeCanonicalFormals(ctx, encoded, nil)
	if decoded != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("post-parse cancellation = %T, %v (Err calls %d)", decoded, err, ctx.errCalls)
	}
	if ctx.errCalls != ctx.allowedErrCalls+1 {
		t.Fatalf("cancellation was not observed at first post-parse checkpoint: calls %d allowed %d", ctx.errCalls, ctx.allowedErrCalls)
	}
}

type canonicalFormalCancelAfterContext struct {
	context.Context
	allowedErrCalls int
	errCalls        int
	done            chan struct{}
	canceled        bool
}

func (ctx *canonicalFormalCancelAfterContext) Done() <-chan struct{} { return ctx.done }

func (ctx *canonicalFormalCancelAfterContext) Err() error {
	ctx.errCalls++
	if ctx.errCalls <= ctx.allowedErrCalls {
		return nil
	}
	if !ctx.canceled {
		close(ctx.done)
		ctx.canceled = true
	}
	return context.Canceled
}

func TestCanonicalFormalCancellationCheckpointsInsideGraphPhases(t *testing.T) {
	const width = 512
	chain := make([]canonicalTypeNode, width)
	shapes := make([]canonicalFormalNodeShape, width)
	for index := range chain {
		chain[index].scalar = []byte{canonicalOptional}
		shapes[index].tag = canonicalOptional
		if index+1 < len(chain) {
			chain[index].edges = []int{index + 1}
		}
	}
	for name, law := range map[string]func(context.Context) error{
		"constraint-scc": func(ctx context.Context) error {
			return validateCanonicalFormalConstraintCycles(ctx, nil, chain, shapes)
		},
		"generic-scc": func(ctx context.Context) error {
			return validateCanonicalFormalGenericRecurrence(ctx, nil, chain, shapes)
		},
		"materializability": func(ctx context.Context) error {
			return validateCanonicalFormalMaterializability(ctx, nil, chain, shapes)
		},
		"lexical-dominators": func(ctx context.Context) error {
			return validateCanonicalFormalLexicalScope(ctx, nil, chain, shapes)
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx := &canonicalFormalCancelAfterContext{Context: context.Background(), done: make(chan struct{})}
			if err := law(ctx); !errors.Is(err, context.Canceled) {
				t.Fatalf("%s cancellation = %v", name, err)
			}
		})
	}
}

func TestCanonicalFormalCancellationCheckpointsInsideScalarMetadata(t *testing.T) {
	const fields = 512
	scalar := appendBool([]byte{canonicalRecord}, false)
	scalar = appendCount(scalar, fields)
	for index := 0; index < fields; index++ {
		scalar = appendFrameString(scalar, "field")
		scalar = appendBool(scalar, false)
		scalar = appendBool(scalar, false)
	}
	scalar = appendCount(scalar, 0)
	scalar = appendBool(scalar, false)
	scalar = appendBool(scalar, false)
	ctx := &canonicalFormalCancelAfterContext{Context: context.Background(), done: make(chan struct{})}
	var steps uint64
	if _, err := validateCanonicalFormalScalar(ctx, nil, &steps, scalar, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("scalar metadata cancellation = %v", err)
	}
}

func TestCanonicalFormalCancellationCheckpointsInsideCanonicalQuotient(t *testing.T) {
	const edges = 512
	node := canonicalTypeNode{scalar: []byte{canonicalOptional}, edges: make([]int, edges)}
	for index := range node.edges {
		node.edges[index] = 0
	}
	t.Run("tarjan-edge-fanout", func(t *testing.T) {
		ctx := &canonicalFormalCancelAfterContext{Context: context.Background(), allowedErrCalls: 2, done: make(chan struct{})}
		encoder := &canonicalEncoder{nodes: []canonicalTypeNode{node}, ctx: ctx}
		finder, err := newCanonicalSCCFinder(encoder)
		if err != nil {
			t.Fatalf("new finder: %v", err)
		}
		if err := finder.find(); !errors.Is(err, context.Canceled) {
			t.Fatalf("Tarjan cancellation = %v", err)
		}
	})
	t.Run("refinement-predecessors", func(t *testing.T) {
		ctx := &canonicalFormalCancelAfterContext{Context: context.Background(), allowedErrCalls: 1, done: make(chan struct{})}
		encoder := &canonicalEncoder{nodes: []canonicalTypeNode{node}, classes: []int{-1}, ctx: ctx}
		if _, err := encoder.refineStratum([]int{0}, []int{-1}, 0); !errors.Is(err, context.Canceled) {
			t.Fatalf("refinement cancellation = %v", err)
		}
	})
	t.Run("class-representatives", func(t *testing.T) {
		classes := make([]int, edges)
		ctx := &canonicalFormalCancelAfterContext{Context: context.Background(), allowedErrCalls: 1, done: make(chan struct{})}
		encoder := &canonicalEncoder{classes: classes, ctx: ctx}
		if err := encoder.buildClassRepresentatives(); !errors.Is(err, context.Canceled) {
			t.Fatalf("representatives cancellation = %v", err)
		}
	})
}

func TestCanonicalFormalCancellationCheckpointsInsideMaterializationFanout(t *testing.T) {
	const width = 512
	ctx := func() *canonicalFormalCancelAfterContext {
		return &canonicalFormalCancelAfterContext{Context: context.Background(), allowedErrCalls: 1, done: make(chan struct{})}
	}
	t.Run("materialization-children", func(t *testing.T) {
		nodes := []canonicalTypeNode{{edges: make([]int, width)}}
		for index := range nodes[0].edges {
			nodes[0].edges[index] = index + 1
			nodes = append(nodes, canonicalTypeNode{})
		}
		built, ready := make([]Type, len(nodes)), make([]bool, len(nodes))
		for index := 1; index < len(nodes); index++ {
			built[index], ready[index] = String, true
		}
		var steps uint64
		if _, _, err := canonicalFormalChildren(ctx(), nil, &steps, 0, nodes, make([]canonicalFormalNodeShape, len(nodes)), built, ready); !errors.Is(err, context.Canceled) {
			t.Fatalf("children fanout cancellation = %v", err)
		}
	})
	t.Run("generic-parameters", func(t *testing.T) {
		scalar := appendFrameString([]byte{canonicalGeneric}, "G")
		scalar = appendCount(scalar, width)
		scalar = appendBool(scalar, false)
		nodes := make([]canonicalTypeNode, width+1)
		nodes[0] = canonicalTypeNode{scalar: scalar, edges: make([]int, width)}
		shapes := make([]canonicalFormalNodeShape, len(nodes))
		shapes[0] = canonicalFormalNodeShape{tag: canonicalGeneric, binderParams: width, children: width}
		built, ready := make([]Type, len(nodes)), make([]bool, len(nodes))
		for index := 1; index < len(nodes); index++ {
			nodes[0].edges[index-1] = index
			built[index], ready[index] = NewTypeParam("T", nil), true
		}
		var steps uint64
		if _, _, err := openCanonicalFormalGeneric(ctx(), nil, &steps, 0, nodes, shapes, built, ready); !errors.Is(err, context.Canceled) {
			t.Fatalf("generic parameter cancellation = %v", err)
		}
	})
	t.Run("scoped-union", func(t *testing.T) {
		members := make([]Type, width)
		for index := range members {
			members[index] = String
		}
		var steps uint64
		if _, err := canonicalScopedUnion(ctx(), nil, &steps, members); !errors.Is(err, context.Canceled) {
			t.Fatalf("union materialization cancellation = %v", err)
		}
	})
}

func TestCanonicalFormalCancellationCheckpointsInsideCompositeNodeMaterialization(t *testing.T) {
	const width = 512
	newContext := func() *canonicalFormalCancelAfterContext {
		return &canonicalFormalCancelAfterContext{Context: context.Background(), allowedErrCalls: 1, done: make(chan struct{})}
	}
	strings := make([]Type, width)
	for index := range strings {
		strings[index] = String
	}
	t.Run("tuple", func(t *testing.T) {
		var steps uint64
		scalar := appendCount([]byte{canonicalTuple}, width)
		if _, err := materializeCanonicalNode(newContext(), scalar, strings, &steps); !errors.Is(err, context.Canceled) {
			t.Fatalf("tuple cancellation = %v", err)
		}
	})
	t.Run("function", func(t *testing.T) {
		var steps uint64
		scalar := appendCount([]byte{canonicalFunction}, 0)
		scalar = appendCount(scalar, width)
		for range width {
			scalar = appendBool(scalar, false)
			scalar = appendBool(scalar, false)
		}
		scalar = appendBool(scalar, false)
		scalar = appendCount(scalar, 0)
		if _, err := materializeCanonicalNode(newContext(), scalar, strings, &steps); !errors.Is(err, context.Canceled) {
			t.Fatalf("function cancellation = %v", err)
		}
	})
	t.Run("interface", func(t *testing.T) {
		var steps uint64
		scalar := appendFrameString([]byte{canonicalInterface}, "I")
		scalar = appendCount(scalar, width)
		methods := make([]Type, width)
		for index := range methods {
			scalar = appendFrameString(scalar, "method")
			methods[index] = Func().Build()
		}
		if _, err := materializeCanonicalNode(newContext(), scalar, methods, &steps); !errors.Is(err, context.Canceled) {
			t.Fatalf("interface cancellation = %v", err)
		}
	})
}

func TestCanonicalEncoderCancellationCheckpointsInsideEncodingAndInitialization(t *testing.T) {
	const width = 512
	newContext := func() *canonicalFormalCancelAfterContext {
		return &canonicalFormalCancelAfterContext{Context: context.Background(), allowedErrCalls: 1, done: make(chan struct{})}
	}
	formals := make([]*TypeParam, width)
	for index := range formals {
		formals[index] = NewTypeParam("T", nil)
	}
	t.Run("external-formals", func(t *testing.T) {
		encoder := &canonicalEncoder{ctx: newContext(), formals: make(map[*TypeParam]uint64)}
		if err := encoder.installFormals(formals); !errors.Is(err, context.Canceled) {
			t.Fatalf("install formals cancellation = %v", err)
		}
	})
	t.Run("binder-parameters", func(t *testing.T) {
		encoder := &canonicalEncoder{ctx: newContext(), scoped: true, formals: make(map[*TypeParam]uint64), binders: make(map[*TypeParam]canonicalFormalBinder)}
		if err := encoder.registerBinder(String, formals); !errors.Is(err, context.Canceled) {
			t.Fatalf("register binder cancellation = %v", err)
		}
	})
	t.Run("record-metadata", func(t *testing.T) {
		fields := make([]Field, width)
		for index := range fields {
			fields[index] = Field{Name: "field", Type: String}
		}
		encoder := &canonicalEncoder{ctx: newContext()}
		if _, _, err := encoder.canonicalRecordParts(&Record{Fields: fields}); !errors.Is(err, context.Canceled) {
			t.Fatalf("record parts cancellation = %v", err)
		}
	})
	t.Run("function-metadata", func(t *testing.T) {
		params := make([]Param, width)
		for index := range params {
			params[index] = Param{Type: String}
		}
		encoder := &canonicalEncoder{ctx: newContext()}
		if _, _, err := encoder.canonicalFunctionParts(&Function{Params: params}); !errors.Is(err, context.Canceled) {
			t.Fatalf("function parts cancellation = %v", err)
		}
	})
	t.Run("scc-index-initialization", func(t *testing.T) {
		nodes := make([]canonicalTypeNode, width)
		encoder := &canonicalEncoder{ctx: newContext(), nodes: nodes}
		if _, err := newCanonicalSCCFinder(encoder); !errors.Is(err, context.Canceled) {
			t.Fatalf("SCC initialization cancellation = %v", err)
		}
	})
	t.Run("well-founded-class-initialization", func(t *testing.T) {
		nodes, members, starts, sccOf := make([]canonicalTypeNode, width), make([]int, width), make([]int, width+1), make([]int, width)
		for index := range members {
			members[index] = index
			starts[index] = index
			sccOf[index] = index
			nodes[index].scalar = []byte{canonicalString}
		}
		starts[width] = width
		encoder := &canonicalEncoder{ctx: newContext()}
		encoder.nodes = nodes
		if err := encoder.classifyByRank(starts, members, sccOf); !errors.Is(err, context.Canceled) {
			t.Fatalf("class initialization cancellation = %v", err)
		}
	})
}

func TestCanonicalQuotientSparseKeyOrderingLaw(t *testing.T) {
	const count = 4096
	keys := make([]int, count)
	for index := range keys {
		// Key magnitude is deliberately unrelated to cardinality: refinement
		// must order only these touched keys, never scan an integer universe.
		keys[index] = 1<<30 - index*131071
	}
	encoder := &canonicalEncoder{ctx: context.Background()}
	if err := encoder.sortCanonicalIntKeys(keys); err != nil {
		t.Fatal(err)
	}
	for index := 1; index < len(keys); index++ {
		if keys[index-1] > keys[index] {
			t.Fatalf("sparse keys not ordered: %v", keys)
		}
	}
	ctx := &canonicalFormalCancelAfterContext{Context: context.Background(), allowedErrCalls: 1, done: make(chan struct{})}
	if err := (&canonicalEncoder{ctx: ctx}).sortCanonicalIntKeys(append([]int(nil), keys...)); !errors.Is(err, context.Canceled) {
		t.Fatalf("checkpointed sparse merge cancellation = %v", err)
	}
}

func TestCanonicalFormalsRejectsUnrepresentableChildDeclarationBeforeAllocation(t *testing.T) {
	// The graph has only a scalar and count declaration. Its count is valid as
	// a uvarint but cannot be represented by the remaining input, so parsing
	// must reject it before allocating the corresponding edge slice.
	wire := appendCanonicalFormalsVersion(nil)
	scalar := appendCount([]byte{canonicalTuple}, maxInt())
	wire = appendCanonicalFormalDefinition(wire, 0, scalar, uint64(maxInt()))
	if _, err := AdmitCanonicalFormals(context.Background(), wire, 0); !errors.Is(err, ErrInvalidCanonicalType) {
		t.Fatalf("unrepresentable edge declaration = %v", err)
	}

	// This declaration exceeds the host slice cardinality outright. The scalar
	// itself has a matching small arity so the failure is specifically the
	// child-count representability check, before an int conversion or make.
	overflow := appendCanonicalFormalsVersion(nil)
	overflow = appendCanonicalFormalDefinition(overflow, 0, appendCount([]byte{canonicalTuple}, 0), uint64(maxInt())+1)
	if _, err := AdmitCanonicalFormals(context.Background(), overflow, 0); !errors.Is(err, ErrInvalidCanonicalType) {
		t.Fatalf("overflowing edge declaration = %v", err)
	}
}

func TestCanonicalFormalsEncoderDropsOversizedScratch(t *testing.T) {
	const depth = 100_001
	var value Type = String
	for range depth {
		value = &Optional{Inner: value}
	}
	encoder := &canonicalEncoder{}
	if _, err := encoder.encodeFormals(context.Background(), value, nil); err != nil {
		t.Fatal(err)
	}
	if cap(encoder.nodes)*64 > canonicalFormalsRetainBytes || cap(encoder.out) > canonicalFormalsRetainBytes || cap(encoder.discoveryStack)*32 > canonicalFormalsRetainBytes {
		t.Fatalf("oversized scoped scratch retained: nodes=%d out=%d discovery=%d", cap(encoder.nodes), cap(encoder.out), cap(encoder.discoveryStack))
	}
}

func FuzzDecodeCanonicalFormals(f *testing.F) {
	formal := NewTypeParam("T", nil)
	seed, err := EncodeCanonicalFormals(context.Background(), NewTuple(formal, String), []*TypeParam{formal})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed.Bytes(), uint8(1))
	f.Add([]byte{}, uint8(0))
	f.Add([]byte{0xff, 0x80, 0x00}, uint8(2))
	f.Fuzz(func(t *testing.T, encoded []byte, count uint8) {
		formals := make([]*TypeParam, int(count))
		for index := range formals {
			formals[index] = NewTypeParam("T", nil)
		}
		receipt, err := AdmitCanonicalFormals(context.Background(), encoded, int(count))
		if err == nil {
			_, _ = DecodeCanonicalFormals(context.Background(), receipt, formals)
		}
	})
}
