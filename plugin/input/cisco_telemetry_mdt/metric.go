package cisco_telemetry_mdt

import (
	"fmt"
	"strings"
)

type row struct {
	Timestamp float64
	Content   any
	Keys      any
}

type metric struct {
	Rows      []row
	Telemetry map[string]any
	Source    string
}

func NewCiscoTelemetryMetric(sourceIP string) *metric {
	return &metric{
		Telemetry: make(map[string]interface{}, 0),
		Source:    sourceIP,
	}
}

func (m *metric) AddField() {
}

func (m *metric) parseRow(value any) {
	for _, arr := range value.([]interface{}) {
		var row row
		data := arr.(map[string]interface{})
		if data[GBPVALUE] == nil && data[GBPFIELDS] != nil {
			if data["timestamp"] != nil {
				row.Timestamp = data["timestamp"].(float64)
			}
			field := m.parseFields(data[GBPFIELDS])
			if _, ok := field["content"]; ok {
				row.Content = field["content"]
			} else {
				fmt.Errorf("no field named rows")
			}
			if _, ok := field["keys"]; ok {
				row.Keys = field["keys"]
			} else {
				fmt.Errorf("no field named keys")
			}
			m.Rows = append(m.Rows, row)
		}
	}
}

func (m *metric) parseFields(v any) map[string]any {
	s := make(map[string]interface{})
	placeInArrayMap := map[string]bool{}
	for _, arr := range v.([]interface{}) {
		field := arr.(map[string]interface{})
		var fieldVal interface{}
		var hint int
		var key string
		if field[GBPNAME] == nil && field[GBPFIELDS] != nil {
			// nx-os every field have a map like this {"": {}}
			key = Nexus
		} else {
			key = field[GBPNAME].(string)
		}
		existingEntry, exists := s[key]
		_, placeInArray := placeInArrayMap[key]
		_, children := field[GBPFIELDS]
		if !children {
			fieldVal = field[GBPVALUE]
			if fieldVal != nil {
				for _, value := range fieldVal.(map[string]interface{}) {
					fieldVal = value
				}
			}
			hint = 10
		} else {
			fieldVal = m.parseFields(field[GBPFIELDS])
			for nilK, nilV := range fieldVal.(map[string]interface{}) {
				if nilK == Nexus {
					fieldVal = nilV
				}
			}
			hint = len(field[GBPFIELDS].([]interface{}))
		}

		if !placeInArray && !exists {
			// this is the common case by far!
			s[key] = fieldVal
		} else {
			newName := key + "_arr"
			if exists {
				if !placeInArray {
					// Create list
					s[newName] = make([]interface{}, 0, hint)
					// Remember that this field name is arrayified(!?)
					placeInArrayMap[key] = true
					// Add existing entry to new array)
					s[newName] = append(s[newName].([]interface{}), existingEntry)
					// Delete existing entry from old
					delete(s, key)
					placeInArray = true
				} else {
					fmt.Errorf("gbpkv inconsistency, processing repeated field names")
				}
			}
			if placeInArray && fieldVal != nil {
				s[newName] = append(s[newName].([]interface{}), fieldVal)
			}
		}
	}
	return s
}

func (m *metric) parseTelemetry(key string, value any) {
	if e, ok := value.(map[string]interface{}); ok {
		for k, v := range e {
			k = CamelCaseToUnderscore(k)
			m.Telemetry[k] = m.decodeValue(v)
		}
	} else {
		key = CamelCaseToUnderscore(key)
		m.Telemetry[key] = m.decodeValue(value)
	}
}

func CamelCaseToUnderscore(s string) string {
	data := make([]byte, 0, len(s)*2)
	j := false
	num := len(s)
	for i := 0; i < num; i++ {
		d := s[i]
		if i > 0 && d >= 'A' && d <= 'Z' && j {
			data = append(data, '_')
		}
		if d != '_' {
			j = true
		}
		data = append(data, d)
	}
	return strings.ToLower(string(data[:]))
}

func (m *metric) decodeValue(v interface{}) interface{} {
	switch v := v.(type) {
	case float64:
		return v
	case int64:
		return v
	case string:
		return v
	case bool:
		return v
	case int:
		return int64(v)
	case uint:
		return uint64(v)
	case uint64:
		return v
	case []byte:
		return string(v)
	case int32:
		return int64(v)
	case int16:
		return int64(v)
	case int8:
		return int64(v)
	case uint32:
		return uint64(v)
	case uint16:
		return uint64(v)
	case uint8:
		return uint64(v)
	case float32:
		return float64(v)
	case *float64:
		if v != nil {
			return *v
		}
	case *int64:
		if v != nil {
			return *v
		}
	case *string:
		if v != nil {
			return *v
		}
	case *bool:
		if v != nil {
			return *v
		}
	case *int:
		if v != nil {
			return int64(*v)
		}
	case *uint:
		if v != nil {
			return uint64(*v)
		}
	case *uint64:
		if v != nil {
			return *v
		}
	case *[]byte:
		if v != nil {
			return string(*v)
		}
	case *int32:
		if v != nil {
			return int64(*v)
		}
	case *int16:
		if v != nil {
			return int64(*v)
		}
	case *int8:
		if v != nil {
			return int64(*v)
		}
	case *uint32:
		if v != nil {
			return uint64(*v)
		}
	case *uint16:
		if v != nil {
			return uint64(*v)
		}
	case *uint8:
		if v != nil {
			return uint64(*v)
		}
	case *float32:
		if v != nil {
			return float64(*v)
		}
	default:
		return nil
	}
	return nil
}
