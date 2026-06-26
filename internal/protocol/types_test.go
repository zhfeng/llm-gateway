package protocol

import "testing"

func TestMapAnthropicStopReasonRefusal(t *testing.T) {
	if got := MapAnthropicStopReason("refusal"); got != "content_filter" {
		t.Fatalf("MapAnthropicStopReason(refusal) = %q; want %q", got, "content_filter")
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
