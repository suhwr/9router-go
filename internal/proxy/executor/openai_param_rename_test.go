package executor

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"testing"
)

// The openai provider rule from upstream paramSupport (7c42d8f8): current
// OpenAI models answer 400 "Use 'max_completion_tokens' instead", and clients
// send max_tokens.

func TestRenameMaxTokensBody(t *testing.T) {
	var m map[string]jsontext.Value

	out := renameMaxTokensBody([]byte(`{"model":"gpt-6.1","max_tokens":1024,"messages":[]}`))
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("renamed body does not parse: %v", err)
	}
	if _, ok := m["max_tokens"]; ok {
		t.Error("max_tokens must be removed")
	}
	if string(m["max_completion_tokens"]) != "1024" {
		t.Errorf("max_completion_tokens = %s, want 1024", m["max_completion_tokens"])
	}

	out = renameMaxTokensBody([]byte(`{"max_tokens":1024,"max_completion_tokens":2048}`))
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("body does not parse: %v", err)
	}
	if string(m["max_completion_tokens"]) != "2048" {
		t.Errorf("explicit max_completion_tokens = %s, want kept 2048", m["max_completion_tokens"])
	}
	if _, ok := m["max_tokens"]; ok {
		t.Error("max_tokens must be removed even when a target value exists")
	}

	untouched := []byte(`{"model":"gpt-4o","max_completion_tokens":512}`)
	if got := renameMaxTokensBody(untouched); string(got) != string(untouched) {
		t.Errorf("body without max_tokens changed: %s", got)
	}
	if got := renameMaxTokensBody([]byte(`not json`)); string(got) != "not json" {
		t.Errorf("unparsable body changed: %s", got)
	}
}

func TestSanitizeGeminiOpenAICompatBody(t *testing.T) {
	in := []byte(`{"model":"gemini-3.8-flash","messages":[{"role":"user","content":"hi"}],"frequency_penalty":0.5,"presence_penalty":0.3,"logit_bias":{"1":2},"logprobs":true,"top_logprobs":5,"temperature":0.7}`)
	out := SanitizeGeminiOpenAICompatBody(in)

	var m map[string]jsontext.Value
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("sanitized body does not parse: %v", err)
	}
	for _, key := range []string{"frequency_penalty", "presence_penalty", "logit_bias", "logprobs", "top_logprobs"} {
		if _, exists := m[key]; exists {
			t.Errorf("expected %s to be stripped", key)
		}
	}
	if string(m["model"]) != `"gemini-3.8-flash"` {
		t.Errorf("model changed: %s", m["model"])
	}
	if string(m["temperature"]) != "0.7" {
		t.Errorf("temperature changed: %s", m["temperature"])
	}

	untouched := []byte(`{"model":"gemini-3.8-flash","messages":[]}`)
	if got := SanitizeGeminiOpenAICompatBody(untouched); string(got) != string(untouched) {
		t.Errorf("body without unsupported fields changed: %s", got)
	}
}
