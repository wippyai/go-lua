package framing

import (
	"bytes"
	"errors"
	"testing"
)

func TestReaderRequiresExactFramingAndCompleteConsumption(t *testing.T) {
	var data bytes.Buffer
	var writer Writer
	if err := writer.Reset(&data, "reader/law", 7); err != nil {
		t.Fatal(err)
	}
	if err := writer.Record(4); err != nil {
		t.Fatal(err)
	}
	if err := writer.Uint(99); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); err != nil {
		t.Fatal(err)
	}

	reader, err := NewReader(data.Bytes(), len(data.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Header("reader/law", 7); err != nil {
		t.Fatal(err)
	}
	if record, err := reader.Record(); err != nil || record != 4 {
		t.Fatalf("Record = %d/%v", record, err)
	}
	if value, err := reader.Uint(); err != nil || value != 99 {
		t.Fatalf("Uint = %d/%v", value, err)
	}
	if err := reader.Finish(); err != nil {
		t.Fatal(err)
	}

	for _, malformed := range [][]byte{
		data.Bytes()[:len(data.Bytes())-1],
		append(append([]byte(nil), data.Bytes()...), 0),
		{tagDomain, 0x80},
		{tagDomain, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	} {
		reader, err := NewReader(malformed, len(malformed))
		if err != nil {
			continue
		}
		if err := reader.Header("reader/law", 7); err == nil {
			if _, err := reader.Record(); err == nil {
				if _, err := reader.Uint(); err == nil && reader.Finish() == nil {
					t.Fatal("malformed stream was accepted")
				}
			}
		}
	}
}

func TestReaderEnforcesDeclaredPayloadLimit(t *testing.T) {
	data := []byte{tagBytes, 3, 1, 2, 3}
	reader, err := NewReader(data, len(data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Bytes(2); !errors.Is(err, ErrLimit) {
		t.Fatalf("Bytes limit = %v", err)
	}
	if _, err := NewReader(data, len(data)-1); !errors.Is(err, ErrLimit) {
		t.Fatalf("stream limit = %v", err)
	}
}
