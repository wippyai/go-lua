package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func rawRecordType() *typ.Record {
	return typetable.NewRecord().Field("id", typ.String).Field("amount", typ.Number).Build()
}

func witnessType(payload typ.Type) *typ.Record {
	return typetable.NewRecord().
		Field("decode", typ.Func().Param("raw", typ.Any).Returns(payload).Build()).
		Build()
}

func TestInferImportedTypeArgsBindsFromChannelApplication(t *testing.T) {
	parameter := typ.NewTypeParam("T", nil)
	expected := typ.Instantiate(ambient.ChannelGeneric(), parameter)
	actual := typ.Instantiate(ambient.ChannelGeneric(), rawRecordType())

	bindings := map[string]typ.Type{}
	if !inferImportedTypeArgs(expected, actual, map[string]bool{"T": true}, bindings) {
		t.Fatalf("Channel<T> against Channel<RawRecord> did not unify")
	}
	if bound := bindings["T"]; bound == nil || !typ.TypeEquals(bound, rawRecordType()) {
		t.Fatalf("T bound to %v, want the channel payload", bound)
	}
}

func TestInferImportedTypeArgsBindsThroughNestedOptionsRecord(t *testing.T) {
	parameter := typ.NewTypeParam("T", nil)
	expected := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), parameter)).
		Field("decode", witnessType(parameter)).
		Build()
	actual := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), rawRecordType())).
		Field("decode", witnessType(rawRecordType())).
		Build()

	bindings := map[string]typ.Type{}
	if !inferImportedTypeArgs(expected, actual, map[string]bool{"T": true}, bindings) {
		t.Fatalf("ListenOptions<T> against its call-site literal did not unify")
	}
	if bound := bindings["T"]; bound == nil || !typ.TypeEquals(bound, rawRecordType()) {
		t.Fatalf("T bound to %v, want the record both members agree on", bound)
	}
}

func TestInferImportedTypeArgsRefusesConflictingApplicationBindings(t *testing.T) {
	parameter := typ.NewTypeParam("T", nil)
	timer := typetable.NewRecord().Field("elapsed", typ.Number).Build()
	expected := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), parameter)).
		Field("decode", witnessType(parameter)).
		Build()
	actual := typetable.NewRecord().
		Field("channel", typ.Instantiate(ambient.ChannelGeneric(), rawRecordType())).
		Field("decode", witnessType(timer)).
		Build()

	if inferImportedTypeArgs(expected, actual, map[string]bool{"T": true}, map[string]typ.Type{}) {
		t.Fatal("disagreeing channel and witness payloads produced a binding")
	}
}

func TestInferImportedTypeArgsRefusesForeignGenericApplication(t *testing.T) {
	parameter := typ.NewTypeParam("T", nil)
	foreign := typ.NewGeneric("other.Channel", []*typ.TypeParam{typ.NewTypeParam("T", nil)}, typ.NewInterface("other.Channel", nil))
	expected := typ.Instantiate(ambient.ChannelGeneric(), parameter)
	actual := typ.Instantiate(foreign, rawRecordType())

	bindings := map[string]typ.Type{}
	if inferImportedTypeArgs(expected, actual, map[string]bool{"T": true}, bindings) {
		t.Fatal("a foreign generic of the same arity was accepted as a channel")
	}
	if bindings["T"] != nil {
		t.Fatalf("T bound to %v from a foreign application", bindings["T"])
	}
}

func TestProviderArgumentTypeRecoversKeyedLiteralRecord(t *testing.T) {
	channel, ok := shapefact.EncodeTarget(typ.Instantiate(ambient.ChannelGeneric(), rawRecordType()))
	if !ok {
		t.Fatal("encoding the channel member target failed")
	}
	witness, ok := shapefact.EncodeTarget(witnessType(rawRecordType()))
	if !ok {
		t.Fatal("encoding the witness member target failed")
	}
	literal, ok := shapefact.EncodeTable(shapefact.Table{
		Closed: true,
		Members: []shapefact.Member{
			{Suffix: ".channel", Present: true, Value: string(channel)},
			{Suffix: ".decode", Present: true, Value: string(witness)},
		},
	})
	if !ok {
		t.Fatal("encoding the options literal failed")
	}

	argument, decoded := providerArgumentType(literal)
	if !decoded {
		t.Fatal("a closed keyed literal published no argument witness")
	}
	record, isRecord := argument.(*typ.Record)
	if !isRecord {
		t.Fatalf("argument witness is %T, want a record", argument)
	}
	field := record.GetField("channel")
	if field == nil || !typ.TypeEquals(field.Type, typ.Instantiate(ambient.ChannelGeneric(), rawRecordType())) {
		t.Fatalf("channel member reconstructed as %v", field)
	}
	if field := record.GetField("decode"); field == nil || !typ.TypeEquals(field.Type, witnessType(rawRecordType())) {
		t.Fatalf("decode member reconstructed as %v", field)
	}
}

func TestProviderArgumentTypeRecoversNestedKeyedLiteralRecord(t *testing.T) {
	witness, ok := shapefact.EncodeTarget(witnessType(rawRecordType()))
	if !ok {
		t.Fatal("encoding the witness member target failed")
	}
	schema, ok := shapefact.EncodeTable(shapefact.Table{
		Closed:  true,
		Members: []shapefact.Member{{Suffix: ".witness", Present: true, Value: string(witness)}},
	})
	if !ok {
		t.Fatal("encoding the schema literal failed")
	}
	channel, ok := shapefact.EncodeTarget(typ.Instantiate(ambient.ChannelGeneric(), rawRecordType()))
	if !ok {
		t.Fatal("encoding the channel member target failed")
	}
	literal, ok := shapefact.EncodeTable(shapefact.Table{
		Closed: true,
		Members: []shapefact.Member{
			{Suffix: ".channel", Present: true, Value: string(channel)},
			{Suffix: ".schema", Present: true, Value: string(schema)},
			{Suffix: ".schema.witness", Present: true, Value: string(witness)},
		},
	})
	if !ok {
		t.Fatal("encoding the nested options literal failed")
	}

	argument, decoded := providerArgumentType(literal)
	if !decoded {
		t.Fatal("a closed literal carrying a projected member path published no argument witness")
	}
	record, isRecord := argument.(*typ.Record)
	if !isRecord {
		t.Fatalf("argument witness is %T, want a record", argument)
	}
	nested := record.GetField("schema")
	if nested == nil {
		t.Fatal("nested schema member missing from the reconstructed record")
	}
	inner, isRecord := nested.Type.(*typ.Record)
	if !isRecord {
		t.Fatalf("schema member reconstructed as %T, want a record", nested.Type)
	}
	if field := inner.GetField("witness"); field == nil || !typ.TypeEquals(field.Type, witnessType(rawRecordType())) {
		t.Fatalf("schema.witness reconstructed as %v", field)
	}
}

func TestProviderArgumentTypeRefusesUnrootedMemberPath(t *testing.T) {
	witness, ok := shapefact.EncodeTarget(witnessType(rawRecordType()))
	if !ok {
		t.Fatal("encoding the witness member target failed")
	}
	literal, ok := shapefact.EncodeTable(shapefact.Table{
		Closed:  true,
		Members: []shapefact.Member{{Suffix: ".schema.witness", Present: true, Value: string(witness)}},
	})
	if !ok {
		t.Fatal("encoding the unrooted literal failed")
	}
	if _, decoded := providerArgumentType(literal); decoded {
		t.Fatal("a member path with no top-level member published an argument witness")
	}
}

func TestProviderArgumentTypeRefusesOpenKeyedLiteral(t *testing.T) {
	witness, ok := shapefact.EncodeTarget(witnessType(rawRecordType()))
	if !ok {
		t.Fatal("encoding the witness member target failed")
	}
	literal, ok := shapefact.EncodeTable(shapefact.Table{
		Members: []shapefact.Member{{Suffix: ".decode", Present: true, Value: string(witness)}},
	})
	if !ok {
		t.Fatal("encoding the open literal failed")
	}
	if _, decoded := providerArgumentType(literal); decoded {
		t.Fatal("an open literal published an argument witness")
	}
}
