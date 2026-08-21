package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// rejectDuplicateJSONKeys checks a complete manifest-wire value before it is
// decoded into Go structs. encoding/json otherwise applies the last value for
// a repeated object member, including for members matched case-insensitively
// to an exported Go field. That is unsafe for authority-bearing operation and
// publication declarations: two byte strings could otherwise decode to one
// semantic declaration.
//
// This is deliberately scoped to the manifest wire boundary. It is not a
// replacement JSON framework; it only walks the already-tokenized value and
// rejects repeated object members before the existing strict decoders run.
func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	first, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := scanJSONValueForDuplicateKeys(decoder, first); err != nil {
		return err
	}
	if trailing, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("manifest: multiple JSON values beginning with %v", trailing)
		}
		return err
	}
	return nil
}

func scanJSONValueForDuplicateKeys(decoder *json.Decoder, token json.Token) error {
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}

	switch delimiter {
	case '{':
		seen := make([]string, 0, 4)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("manifest: JSON object key is not a string")
			}
			for _, previous := range seen {
				// encoding/json matches field names case-insensitively. Reject
				// those spellings as well as exact repeats so they cannot be
				// used to select different last-write-wins values.
				if strings.EqualFold(previous, key) {
					return fmt.Errorf("manifest: duplicate JSON field %q", key)
				}
			}
			seen = append(seen, key)

			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := scanJSONValueForDuplicateKeys(decoder, value); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err

	case '[':
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := scanJSONValueForDuplicateKeys(decoder, value); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err

	default:
		return fmt.Errorf("manifest: unexpected JSON delimiter %q", delimiter)
	}
}
