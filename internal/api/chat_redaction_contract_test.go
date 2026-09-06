package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestChatResponseDoesNotSerializeSensitiveSourceValues(t *testing.T) {
	response := ChatResponse{
		UserMessage: "身份证号为610125********1316，电话182****0799",
		Message:     "已按策略脱敏。",
		SensitiveInfos: []SensitiveInfoDisplay{{
			Type: "id_card", Redacted: "***", DetectionLayer: "regex", Confidence: 0.92,
		}, {
			Type: "phone", Redacted: "***", DetectionLayer: "regex", Confidence: 0.90,
		}},
	}

	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal chat response: %v", err)
	}
	serialized := string(payload)
	for _, forbidden := range []string{"610 125 20060322 131 6", "610125200603221316", "1-8291810799", "18291810799", "\"original\""} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("public chat response exposed sensitive source value %q: %s", forbidden, serialized)
		}
	}
}
