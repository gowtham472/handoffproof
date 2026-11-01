// Package agent is the Meta-model reasoning client (hp-meta-api-client).
// It proposes, plans, diagnoses and explains. It never issues verdicts:
// there is deliberately no tool or function here that sets pass or fail.
package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Spec is the loaded handoffproof_agent_prompt.json.
type Spec struct {
	Name            string                     `json:"name"`
	SystemPrompt    string                     // rendered by RenderSystem
	Tools           []ToolDef                  `json:"-"`
	Schemas         map[string]json.RawMessage `json:"-"`
	MaxOutputTokens int                        `json:"-"`
	raw             rawSpec
}

type rawTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Access      string         `json:"access"`
	Parameters  map[string]any `json:"parameters"`
}

type rawSpec struct {
	Model struct {
		Parameters struct {
			MaxOutputTokens int `json:"max_output_tokens"`
		} `json:"parameters"`
	} `json:"model"`
	SystemPrompt  map[string]json.RawMessage `json:"system_prompt"`
	Tools         []rawTool                  `json:"tools"`
	OutputSchemas map[string]json.RawMessage `json:"output_schemas"`
}

// ToolDef is one allowlisted tool from the spec.
type ToolDef struct {
	Name        string
	Description string
	Access      string
	Parameters  map[string]any
}

// LoadSpec reads and renders the agent spec JSON.
func LoadSpec(path string) (*Spec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read spec: %w", err)
	}
	var rs rawSpec
	var meta struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &rs); err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, fmt.Errorf("parse spec meta: %w", err)
	}
	s := &Spec{Name: meta.Name, Schemas: rs.OutputSchemas, MaxOutputTokens: rs.Model.Parameters.MaxOutputTokens, raw: rs}
	for _, t := range rs.Tools {
		s.Tools = append(s.Tools, ToolDef{Name: t.Name, Description: t.Description, Access: t.Access, Parameters: t.Parameters})
	}
	sys, err := renderSystem(rs.SystemPrompt)
	if err != nil {
		return nil, err
	}
	s.SystemPrompt = sys
	if s.MaxOutputTokens < 16000 {
		// The spec's 4096 truncates reasoning + dependency maps mid-JSON.
		// The cap covers reasoning and visible output combined (Meta docs).
		s.MaxOutputTokens = 16000
	}
	return s, nil
}

func joinArr(m map[string]json.RawMessage, key string, b *strings.Builder) {
	raw, ok := m[key]
	if !ok {
		return
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return
	}
	for _, line := range arr {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
}

// RenderSystem joins the spec's prompt arrays into one system prompt.
// Repository content is never part of this prompt; it arrives via tools,
// labelled as untrusted data.
func renderSystem(m map[string]json.RawMessage) (string, error) {
	var b strings.Builder
	b.WriteString("You are the HandoffProof reasoning agent. Follow these rules exactly.\n\n## Identity\n")
	joinArr(m, "identity", &b)
	b.WriteString("\n## Mission\n")
	joinArr(m, "mission", &b)
	b.WriteString("\n## Hard rules\n")
	joinArr(m, "hard_rules", &b)
	b.WriteString("\n## Communication style\n")
	joinArr(m, "communication_style", &b)
	b.WriteString("\n## Security and privacy\n")
	joinArr(m, "security_and_privacy", &b)
	if raw, ok := m["trust_model"]; ok {
		b.WriteString("\n## Trust model\n")
		b.Write(raw)
		b.WriteString("\n")
	}
	if raw, ok := m["evidence_taxonomy"]; ok {
		b.WriteString("\n## Evidence taxonomy\n")
		b.Write(raw)
		b.WriteString("\n")
	}
	if raw, ok := m["supported_scope_v1"]; ok {
		b.WriteString("\n## Supported scope (v1)\n")
		b.Write(raw)
		b.WriteString("\n")
	}
	if raw, ok := m["drill_library"]; ok {
		b.WriteString("\n## Drill library\n")
		b.Write(raw)
		b.WriteString("\n")
	}
	if raw, ok := m["output_contract"]; ok {
		b.WriteString("\n## Output contract\n")
		b.Write(raw)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// ResponsesTools converts spec tools to Responses API function tools.
// The spec writes per-property "required": false flags, which are not valid
// JSON Schema (required must be an array); strip them for the wire.
func (s *Spec) ResponsesTools() []map[string]any {
	out := make([]map[string]any, 0, len(s.Tools))
	for _, t := range s.Tools {
		props := map[string]any{}
		for k, v := range t.Parameters {
			if m, ok := v.(map[string]any); ok {
				cp := map[string]any{}
				for mk, mv := range m {
					if mk == "required" {
						if _, isArr := mv.([]any); !isArr {
							continue
						}
					}
					cp[mk] = mv
				}
				props[k] = cp
			} else {
				props[k] = v
			}
		}
		out = append(out, map[string]any{
			"type":        "function",
			"name":        t.Name,
			"description": t.Description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": props,
			},
		})
	}
	return out
}

// ToolByName returns the allowlist entry for a tool name.
func (s *Spec) ToolByName(name string) (ToolDef, bool) {
	for _, t := range s.Tools {
		if t.Name == name {
			return t, true
		}
	}
	return ToolDef{}, false
}
