package protocol

import "encoding/json"

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"

	ContentText       = "text"
	ContentImage      = "image"
	ContentToolUse    = "tool_use"
	ContentToolResult = "tool_result"
	ContentThinking   = "thinking"

	StopEndTurn   = "end_turn"
	StopToolUse   = "tool_use"
	StopMaxTokens = "max_tokens"
	StopStopSeq   = "stop_sequence"
	StopRefusal   = "refusal"

	StreamTextDelta    = "text_delta"
	StreamThinking     = "thinking_delta"
	StreamToolCall     = "tool_call_delta"
	StreamMessageStart = "message_start"
	StreamMessageStop  = "message_stop"
	StreamError        = "error"
)

type Request struct {
	Model         string
	Messages      []Message
	System        string
	MaxTokens     int
	Temperature   *float64
	TopP          *float64
	StopSequences []string
	Stream        bool
	Tools         []Tool
	ToolChoice    any
	ProviderModel string
}

type Message struct {
	Role       string
	Content    []ContentBlock
	ToolCalls  []ToolCall
	ToolCallID string
	Name       string
}

type ContentBlock struct {
	Type      string
	Text      string
	MediaType string
	Data      string
	URL       string
	ID        string
	Name      string
	Input     json.RawMessage
	ToolUseID string
	IsError   bool
	// Content preserves the raw structured payload of a tool_result block
	// when the upstream client sent it as an array (e.g. multi-block text
	// plus inline images). When non-empty, downstream emitters should write
	// this verbatim instead of falling back to the flattened Text string.
	Content json.RawMessage
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
}

type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

type Response struct {
	ID         string
	Model      string
	Role       string
	Content    []ContentBlock
	ToolCalls  []ToolCall
	StopReason string
	Usage      Usage
}

type Usage struct {
	InputTokens                int `json:"input_tokens,omitempty"`
	OutputTokens               int `json:"output_tokens,omitempty"`
	CacheCreationInputTokens   int `json:"cache_creation_input_tokens,omitempty"`
	CacheCreation5mInputTokens int `json:"cache_creation_5m_input_tokens,omitempty"`
	CacheCreation1hInputTokens int `json:"cache_creation_1h_input_tokens,omitempty"`
	CacheReadInputTokens       int `json:"cache_read_input_tokens,omitempty"`
}

type StreamEvent struct {
	Type       string
	Text       string
	ToolCallID string
	ToolName   string
	ToolInput  string
	Usage      Usage
	Response   *Response
	Err        error
}

type ModelInfo struct {
	ID      string
	OwnedBy string
}

func TextContent(text string) []ContentBlock {
	if text == "" {
		return nil
	}
	return []ContentBlock{{Type: ContentText, Text: text}}
}

func ContentTextValue(blocks []ContentBlock) string {
	var out string
	for _, block := range blocks {
		if block.Type == ContentText {
			out += block.Text
		}
	}
	return out
}

// MapOpenAIStopReason normalizes a finish_reason coming from an
// OpenAI-compatible upstream into the canonical Anthropic-compatible stop
// reason used internally (and emitted verbatim by writeAnthropicMessage).
// The reverse OpenAI-facing translation lives in OpenAIStopReason.
func MapOpenAIStopReason(reason string) string {
	switch reason {
	case "stop":
		return StopEndTurn
	case "tool_calls", "function_call":
		return StopToolUse
	case "length":
		return StopMaxTokens
	case "content_filter":
		// Canonical Anthropic form is "refusal"; the OpenAI-only
		// "content_filter" token must not leak into Anthropic responses.
		return StopRefusal
	default:
		return reason
	}
}

// MapAnthropicStopReason normalizes a stop_reason coming from an Anthropic
// upstream into the canonical Anthropic-compatible form. Anthropic's own
// SDKs/clients consume this value verbatim through the
// Anthropic-compatible /v1/messages response, so any OpenAI-specific
// remapping (e.g. "refusal" -> "content_filter") must NOT happen here —
// that translation belongs in OpenAIStopReason.
func MapAnthropicStopReason(reason string) string {
	return reason
}

func OpenAIStopReason(reason string) string {
	switch reason {
	case StopEndTurn, StopStopSeq:
		return "stop"
	case StopToolUse:
		return "tool_calls"
	case StopMaxTokens:
		return "length"
	case StopRefusal:
		return "content_filter"
	default:
		if reason == "" {
			return "stop"
		}
		return reason
	}
}
