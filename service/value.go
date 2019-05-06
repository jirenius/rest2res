package service

import (
	"encoding/json"
	"errors"
)

type valueType byte

const (
	valueTypeString valueType = iota
	valueTypeNumber
	valueTypeObject
	valueTypeArray
	valueTypeTrue
	valueTypeFalse
	valueTypeNull
)

var (
	valueObject = []byte(`{}`)
	valueArray  = []byte(`[]`)
	valueTrue   = []byte(`true`)
	valueFalse  = []byte(`false`)
	valueNull   = []byte(`null`)
)

type value struct {
	typ valueType
	raw json.RawMessage
	arr []value
	obj map[string]value
}

func (v *value) UnmarshalJSON(b []byte) error {
	switch b[0] {
	case '{':
		v.typ = valueTypeObject
		var obj map[string]value
		err := json.Unmarshal(b, &obj)
		if err != nil {
			return err
		}
		v.obj = obj
	case '[':
		v.typ = valueTypeArray
		var arr []value
		err := json.Unmarshal(b, &arr)
		if err != nil {
			return err
		}
		v.arr = arr
	case 't':
		v.typ = valueTypeTrue
	case 'f':
		v.typ = valueTypeFalse
	case 'n':
		v.typ = valueTypeNull
	case '"':
		v.typ = valueTypeString
		v.raw = make(json.RawMessage, len(b))
		copy(v.raw, b)
	default: // number
		v.typ = valueTypeNumber
		v.raw = make(json.RawMessage, len(b))
		copy(v.raw, b)
	}
	return nil
}

func (v value) MarshalJSON() ([]byte, error) {
	switch v.typ {
	case valueTypeObject:
		return valueObject, nil
	case valueTypeArray:
		return valueArray, nil
	case valueTypeTrue:
		return valueTrue, nil
	case valueTypeFalse:
		return valueFalse, nil
	case valueTypeNull:
		return valueNull, nil
	case valueTypeString:
		fallthrough
	case valueTypeNumber:
		return v.raw, nil
	}
	return nil, errors.New("invalid value type")
}
