package proxy

import (
	"fmt"
)

type UnmarshalWrongTypeError struct {
	Field, Expected string
	Got             interface{}
}

func (e *UnmarshalWrongTypeError) Error() string {
	return fmt.Sprintf("Wrong type for %s field, expected %s, but got %T", e.Field, e.Expected, e.Got)
}

type jsonObject map[string]interface{}

func (j jsonObject) Object(key string) (jsonObject, error) {
	iface, ok := j[key]
	if !ok || iface == nil {
		result := jsonObject{}
		j[key] = result
		return result, nil
	}

	result, ok := iface.(map[string]interface{})
	if !ok {
		return nil, &UnmarshalWrongTypeError{key, "object", iface}
	}

	return jsonObject(result), nil
}

// For parsing json objects. Look up (or create) a string in the given map.
func (j jsonObject) String(key string, ks ...string) (string, error) {
	if len(ks) > 0 {
		j, err := j.Object(key)
		if err != nil {
			return "", err
		}
		return j.String(ks[0], ks[1:]...)
	}

	iface, ok := j[key]
	if !ok || iface == nil {
		return "", nil
	}

	result, ok := iface.(string)
	if !ok {
		return "", &UnmarshalWrongTypeError{key, "string", iface}
	}

	return result, nil
}

// For parsing json objects. Look up (or create) a string array in the given map.
func (j jsonObject) StringArray(key string) ([]string, error) {
	iface, ok := j[key]
	if !ok || iface == nil {
		return nil, nil
	}

	switch o := iface.(type) {
	case []string:
		return o, nil
	case []interface{}:
		result := []string{}
		for _, s := range o {
			if s, ok := s.(string); ok {
				result = append(result, s)
			} else {
				return nil, &UnmarshalWrongTypeError{key, "array of strings", iface}
			}
		}
		return result, nil
	}
	return nil, &UnmarshalWrongTypeError{key, "array of strings", iface}
}
