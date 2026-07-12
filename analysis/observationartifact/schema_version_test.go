package observationartifact

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const expectedSchemaVersion1Hash = "7d50c7e7f6c2144dc91d5019e7956aac17b960e261814f23373564e8f8125c0c"
const expectedCanonicalSampleHash = "50e649df70b64bacb062856567f58d1f06a5b48de1d658021dc017836cc7162b"

func TestSchemaVersionPinsDurableSurface(t *testing.T) {
	parts := []string{schemaStruct(reflect.TypeOf(SourceAnchor{})), schemaStruct(reflect.TypeOf(Record{})), schemaStruct(reflect.TypeOf(Artifact{})), schemaStruct(reflect.TypeOf(Universe{}))}
	codec := reflect.TypeOf((*CanonicalValueCodec)(nil)).Elem()
	for i := 0; i < codec.NumMethod(); i++ {
		parts = append(parts, "codec."+codec.Method(i).Name+":"+codec.Method(i).Type.String())
	}
	surface := strings.Join(parts, "|")
	got := fmt.Sprintf("%x", sha256.Sum256([]byte(surface)))
	if expectedSchemaVersion1Hash == "" || got != expectedSchemaVersion1Hash {
		t.Fatalf("observation artifact schema v%d hash = %s; pin after review (surface %s)", SchemaVersion, got, surface)
	}
}

func TestSchemaVersionPinsCanonicalWireAndIdentityDomains(t *testing.T) {
	u := testUniverse(t, 19)
	record := testRecord(t, u, 7)
	raw, err := Encode(u, []Record{record})
	if err != nil {
		t.Fatal(err)
	}
	recordID, _ := record.Identity()
	occurrenceID, _ := record.OccurrenceIdentity()
	surface := append(append(append([]byte("schema=1|magic=WOBS|identity=wippy.observation.identity.v1|record=wippy.observation.record.v1|universe=wippy.observation.universe.v1|"), raw...), recordID[:]...), occurrenceID[:]...)
	got := fmt.Sprintf("%x", sha256.Sum256(surface))
	if expectedCanonicalSampleHash == "" || got != expectedCanonicalSampleHash {
		t.Fatalf("observation canonical sample hash = %s; bump schema and journal on intentional wire/domain change", got)
	}
}

func schemaStruct(t reflect.Type) string {
	parts := []string{t.PkgPath() + "." + t.Name()}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		parts = append(parts, field.Name+":"+field.Type.String())
	}
	return strings.Join(parts, ",")
}
