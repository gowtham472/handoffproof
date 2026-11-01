package agent

import (
	"encoding/json"
	"fmt"
	"sort"
)

type schemaNode struct {
	Type     any                    `json:"type"`
	Required []string               `json:"required"`
	Props    map[string]*schemaNode `json:"properties"`
	Items    *schemaNode            `json:"items"`
	Enum     []any                  `json:"enum"`
	Const    any                    `json:"const"`
	HasConst bool                   `json:"-"`
	Minimum  *float64               `json:"minimum"`
	Maximum  *float64               `json:"maximum"`
}

func (n *schemaNode) UnmarshalJSON(raw []byte) error {
	type plain schemaNode
	var p plain
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}
	*n = schemaNode(p)
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err == nil {
		if c, ok := probe["const"]; ok {
			var v any
			if err := json.Unmarshal(c, &v); err == nil {
				n.Const = v
				n.HasConst = true
			}
		}
	}
	return nil
}

// ValidateSchema checks v against the JSON-Schema subset used by the spec
// (type, required, properties, items, enum, const, minimum, maximum).
// It returns human-readable violations; empty means valid.
func ValidateSchema(schema json.RawMessage, v any) []string {
	var root schemaNode
	if err := json.Unmarshal(schema, &root); err != nil {
		return []string{"schema parse: " + err.Error()}
	}
	var probs []string
	check(&root, v, "$", &probs)
	sort.Strings(probs)
	return probs
}

func typeName(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	}
	return "unknown"
}

func typeMatches(want any, v any) bool {
	switch w := want.(type) {
	case string:
		if w == "integer" {
			f, ok := v.(float64)
			return ok && f == float64(int64(f))
		}
		return typeName(v) == w
	case []any:
		for _, t := range w {
			if typeMatches(t, v) {
				return true
			}
		}
	}
	return true
}

func check(n *schemaNode, v any, path string, probs *[]string) {
	if n.HasConst {
		if fmt.Sprint(n.Const) != fmt.Sprint(v) {
			*probs = append(*probs, fmt.Sprintf("%s: must be %v", path, n.Const))
		}
		return
	}
	if n.Type != nil && !typeMatches(n.Type, v) {
		*probs = append(*probs, fmt.Sprintf("%s: want type %v, got %s", path, n.Type, typeName(v)))
		return
	}
	if len(n.Enum) > 0 {
		ok := false
		for _, e := range n.Enum {
			if fmt.Sprint(e) == fmt.Sprint(v) {
				ok = true
			}
		}
		if !ok {
			*probs = append(*probs, fmt.Sprintf("%s: %v not in enum %v", path, v, n.Enum))
		}
	}
	if f, ok := v.(float64); ok {
		if n.Minimum != nil && f < *n.Minimum {
			*probs = append(*probs, fmt.Sprintf("%s: %v below minimum %v", path, f, *n.Minimum))
		}
		if n.Maximum != nil && f > *n.Maximum {
			*probs = append(*probs, fmt.Sprintf("%s: %v above maximum %v", path, f, *n.Maximum))
		}
	}
	if obj, ok := v.(map[string]any); ok {
		for _, req := range n.Required {
			if _, has := obj[req]; !has {
				*probs = append(*probs, fmt.Sprintf("%s.%s: required", path, req))
			}
		}
		for k, sub := range n.Props {
			if val, has := obj[k]; has {
				check(sub, val, path+"."+k, probs)
			}
		}
	}
	if arr, ok := v.([]any); ok && n.Items != nil {
		for i, el := range arr {
			check(n.Items, el, fmt.Sprintf("%s[%d]", path, i), probs)
		}
	}
}
