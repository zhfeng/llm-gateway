package protocol

import "testing"

func TestMapAnthropicStopReasonRefusalPassesThrough(t *testing.T) {
	// Anthropic clients must receive StopRefusal verbatim — the OpenAI-only
	// remapping to "content_filter" lives in OpenAIStopReason.
	if got := MapAnthropicStopReason(StopRefusal); got != StopRefusal {
		t.Fatalf("MapAnthropicStopReason(%q) = %q; want %q", StopRefusal, got, StopRefusal)
	}
}

func TestOpenAIStopReasonRefusal(t *testing.T) {
	if got := OpenAIStopReason(StopRefusal); got != "content_filter" {
		t.Fatalf("OpenAIStopReason(%q) = %q; want %q", StopRefusal, got, "content_filter")
	}
}

func TestMapOpenAIStopReasonContentFilterMapsToRefusal(t *testing.T) {
	// An OpenAI-compatible upstream returning finish_reason:"content_filter"
	// must be normalized to the canonical Anthropic StopRefusal so it doesn't
	// leak into Anthropic-compatible /v1/messages responses verbatim.
	if got := MapOpenAIStopReason("content_filter"); got != StopRefusal {
		t.Fatalf("MapOpenAIStopReason(content_filter) = %q; want %q", got, StopRefusal)
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
		{anthropic: StopRefusal, openAI: "content_filter"},
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
