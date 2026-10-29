package run

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// Decode closed wire documents without losing duplicate keys or required zero fields.
func decodeWire(b []byte, dest any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	value, err := jsonValue(d)
	if err != nil {
		return err
	}
	if _, err = d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return err
	}
	typed := json.NewDecoder(bytes.NewReader(canonical))
	typed.DisallowUnknownFields()
	if err = typed.Decode(dest); err != nil {
		return err
	}
	rendered, err := json.Marshal(dest)
	if err != nil {
		return err
	}
	rd := json.NewDecoder(bytes.NewReader(rendered))
	rd.UseNumber()
	expected, err := jsonValue(rd)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(value, expected) {
		return fmt.Errorf("missing or noncanonical wire field")
	}
	return nil
}
func jsonValue(d *json.Decoder) (any, error) {
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return token, nil
	}
	switch delim {
	case '{':
		object := map[string]any{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("invalid object key")
			}
			if _, exists := object[name]; exists {
				return nil, fmt.Errorf("duplicate JSON key %s", name)
			}
			value, err := jsonValue(d)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		if _, err = d.Token(); err != nil {
			return nil, err
		}
		return object, nil
	case '[':
		array := []any{}
		for d.More() {
			value, err := jsonValue(d)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err = d.Token(); err != nil {
			return nil, err
		}
		return array, nil
	}
	return nil, fmt.Errorf("unexpected delimiter")
}
