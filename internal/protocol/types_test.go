package protocol

import "testing"

func TestMapAnthropicStopReasonRefusalPassesThrough(t *testing.T) {
	// Anthropic clients must receive "refusal" verbatim — the OpenAI-only
	// remapping to "content_filter" lives in OpenAIStopReason.
	if got := MapAnthropicStopReason("refusal"); got != "refusal" {
		t.Fatalf("MapAnthropicStopReason(refusal) = %q; want %q", got, "refusal")
	}
}

func TestOpenAIStopReasonRefusal(t *testing.T) {
	if got := OpenAIStopReason("refusal"); got != "content_filter" {
		t.Fatalf("OpenAIStopReason(refusal) = %q; want %q", got, "content_filter")
	}
}

func TestMapOpenAIStopReasonContentFilterMapsToRefusal(t *testing.T) {
	// An OpenAI-compatible upstream returning finish_reason:"content_filter"
	// must be normalized to the canonical Anthropic "refusal" so it doesn't
	// leak into Anthropic-compatible /v1/messages responses verbatim.
	if got := MapOpenAIStopReason("content_filter"); got != "refusal" {
		t.Fatalf("MapOpenAIStopReason(content_filter) = %q; want %q", got, "refusal")
	}
}

func TestAnthropicStopReasonsMapToOpenAI(t *testing.T) {
	tests := []struct {
		anthropic string
		openAI    string
	}{
		{anthropic: StopEndTurn, openAI: "stop"},
		{anthropic: StopMaxTokens, openAI: "length"},
		{anthropic: StopStopSeq, openAI: "stop"},
		{anthropic: StopToolUse, openAI: "tool_calls"},
		{anthropic: "refusal", openAI: "content_filter"},
	}
	for _, tt := range tests {
		t.Run(tt.anthropic, func(t *testing.T) {
			mapped := MapAnthropicStopReason(tt.anthropic)
			if got := OpenAIStopReason(mapped); got != tt.openAI {
				t.Fatalf("OpenAIStopReason(MapAnthropicStopReason(%q)) = %q; want %q", tt.anthropic, got, tt.openAI)
			}
		})
	}
}
