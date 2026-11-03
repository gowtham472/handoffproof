package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// turnCall is one model-requested tool invocation, provider-neutral.
type turnCall struct {
	ID        string
	Name      string
	Arguments string
}

// turnResult is one model turn: either tool calls or final text, plus usage.
type turnResult struct {
	Text  string
	Calls []turnCall
	Usage Usage
}

// turnState carries the provider-specific conversation across turns of one
// phase. The phase loop in loop.go only calls newTurnState, runTurn,
// applyToolOutputs and applySchemaFeedback; it never inspects the wire.
type turnState struct {
	route           PhaseRoute
	spec            *Spec
	schema          json.RawMessage
	phase           string
	maxOutputTokens int
	http            *http.Client

	respInput  []any
	respPrevID string

	chatMessages []chatMessage
}

// newTurnState seeds the conversation with the phase task. The system prompt
// travels as instructions on the Responses path and as the system message on
// the chat path; both carry the same text.
func newTurnState(route PhaseRoute, spec *Spec, schema json.RawMessage, phase, task string, maxOutputTokens int, hc *http.Client) *turnState {
	return &turnState{
		route:           route,
		spec:            spec,
		schema:          schema,
		phase:           phase,
		maxOutputTokens: maxOutputTokens,
		http:            hc,
		respInput:       []any{userText(task)},
		chatMessages: []chatMessage{
			{Role: "system", Content: spec.SystemPrompt},
			{Role: "user", Content: task},
		},
	}
}

// runTurn performs one model turn behind the phase's configured provider.
func runTurn(ctx context.Context, st *turnState) (*turnResult, error) {
	switch st.route.Provider {
	case ProviderMetaResponses:
		return responsesTurn(ctx, st)
	case ProviderOpenAIChat:
		return chatTurn(ctx, st)
	default:
		return nil, fmt.Errorf("phase %s: unknown provider kind %q (want %q or %q)",
			st.phase, st.route.Provider, ProviderMetaResponses, ProviderOpenAIChat)
	}
}

// toolOutput is one executed tool result fed back to the model.
type toolOutput struct {
	CallID string
	Output string
}

// applyToolOutputs feeds tool results back into the conversation: as
// function_call_output items on the Responses path and as role=tool messages
// on the chat path.
func (st *turnState) applyToolOutputs(outs []toolOutput) {
	resp := make([]any, 0, len(outs))
	msgs := make([]chatMessage, 0, len(outs))
	for _, o := range outs {
		resp = append(resp, map[string]any{
			"type":    "function_call_output",
			"call_id": o.CallID,
			"output":  o.Output,
		})
		msgs = append(msgs, chatMessage{Role: "tool", ToolCallID: o.CallID, Content: o.Output})
	}
	st.respInput = resp
	st.chatMessages = append(st.chatMessages, msgs...)
}

// applySchemaFeedback replaces the pending input with the validation error,
// mirroring the Responses retry behaviour on the chat path.
func (st *turnState) applySchemaFeedback(violations string) {
	st.respInput = []any{userText("Your previous output failed schema validation:\n" + violations + "\nReturn corrected JSON only.")}
	st.chatMessages = append(st.chatMessages, chatMessage{
		Role:    "user",
		Content: "Your previous output failed schema validation:\n" + violations + "\nReturn corrected JSON only.",
	})
}

// responsesTurn is the Meta Responses API turn: byte-for-byte the request
// shape the loop has always sent.
func responsesTurn(ctx context.Context, st *turnState) (*turnResult, error) {
	req := responseRequest{
		Model:           st.route.ModelID,
		Instructions:    st.spec.SystemPrompt,
		Input:           st.respInput,
		Tools:           st.spec.ResponsesTools(),
		Text:            Format(st.phase, st.schema),
		MaxOutputTokens: st.maxOutputTokens,
		PreviousID:      st.respPrevID,
	}
	raw, err := postJSON(ctx, st.http, st.route.BaseURL+"/responses", st.route.APIKey, req)
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	out := &turnResult{Usage: resp.Usage}
	for _, c := range resp.Calls() {
		out.Calls = append(out.Calls, turnCall{ID: c.CallID, Name: c.Name, Arguments: c.Arguments})
	}
	out.Text = strings.TrimSpace(resp.Text())
	st.respPrevID = resp.ID
	return out, nil
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatRequest struct {
	Model          string           `json:"model"`
	Messages       []chatMessage    `json:"messages"`
	Tools          []map[string]any `json:"tools,omitempty"`
	ResponseFormat map[string]any   `json:"response_format,omitempty"`
	MaxTokens      int              `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content   string         `json:"content"`
			ToolCalls []chatToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// chatResponseFormat mirrors Format for chat-completions: structured output
// travels as response_format, never as text.format (the endpoint rejects
// text.format with HTTP 400).
func chatResponseFormat(phase string, schema json.RawMessage) map[string]any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   phase,
			"schema": json.RawMessage(schema),
			"strict": false,
		},
	}
}

// chatTurn is one OpenAI-compatible chat-completions turn. Flat spec tools
// map to function tools; the model answers with content or tool_calls, and
// tool results return as role=tool messages paired with the assistant turn.
func chatTurn(ctx context.Context, st *turnState) (*turnResult, error) {
	req := chatRequest{
		Model:          st.route.ModelID,
		Messages:       st.chatMessages,
		Tools:          st.spec.ChatTools(),
		ResponseFormat: chatResponseFormat(st.phase, st.schema),
		MaxTokens:      st.maxOutputTokens,
	}
	raw, err := postJSON(ctx, st.http, st.route.BaseURL+"/chat/completions", st.route.APIKey, req)
	if err != nil {
		return nil, err
	}
	var resp chatResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parse chat response: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("chat response has no choices")
	}
	msg := resp.Choices[0].Message
	out := &turnResult{
		Text: strings.TrimSpace(msg.Content),
		Usage: Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
	}
	for _, c := range msg.ToolCalls {
		if c.Function.Name == "" {
			continue
		}
		out.Calls = append(out.Calls, turnCall{ID: c.ID, Name: c.Function.Name, Arguments: c.Function.Arguments})
	}
	// Keep the full assistant turn (with tool_calls) so the next request
	// replays the strict tool-call pairing the endpoint requires.
	kept := chatMessage{Role: "assistant", Content: msg.Content, ToolCalls: msg.ToolCalls}
	st.chatMessages = append(st.chatMessages, kept)
	return out, nil
}
