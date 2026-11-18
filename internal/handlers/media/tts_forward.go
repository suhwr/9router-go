package media

import (
	"bytes"
	"encoding/base64"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"9router/proxy/internal/handlers/chat"
	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/log"
	"9router/proxy/internal/models"
	"9router/proxy/internal/providers"
	"9router/proxy/internal/usagetracker"
)

func (h *MediaHandler) forwardTTSRequest(w http.ResponseWriter, r *http.Request, body []byte) {
	if handlerutil.GetServiceKind(r.Context()) == "" {
		r = r.WithContext(handlerutil.WithServiceKind(r.Context(), "tts"))
	}
	var req struct {
		Model          string `json:"model"`
		Input          string `json:"input"`
		Voice          string `json:"voice"`
		ResponseFormat string `json:"response_format"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Input) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "Missing required field: input")
		return
	}

	// Gate here, not in forwardMediaRequest: the edge-tts and google-tts
	// branches below resolve the model and synthesise locally, never reaching
	// the shared funnel. Gating only the funnel would leave those two paths
	// serving a restricted key.
	if !h.ChatH.EnforceModelAccess(w, r, req.Model) {
		return
	}
	modelInfo, err := h.ChatH.ResolveModel(req.Model)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 1. edge-tts
	if modelInfo.Provider == "edge-tts" {
		voice := req.Voice
		if voice == "" || voice == "alloy" || voice == "default" {
			voice = strings.Split(modelInfo.Model, "/")[0]
		}
		if voice == "" || voice == "edge-tts" {
			voice = "en-US-AriaNeural"
		}
		audio, err := SynthesizeEdgeTTS(r.Context(), h.Client, req.Input, voice)
		if err != nil {
			log.Error("media", "edge-tts synthesis failed", "error", err)
			handlerutil.WriteJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		h.writeTTSResponse(w, r, req.ResponseFormat, audio, "audio/mpeg", "mp3")
		h.trackTTSUsage(modelInfo.Provider, modelInfo.Model)
		return
	}

	// 2. google-tts
	if modelInfo.Provider == "google-tts" {
		lang := req.Voice
		if lang == "" || lang == "alloy" || lang == "default" {
			lang = strings.Split(modelInfo.Model, "/")[0]
		}
		if lang == "" || lang == "google-tts" {
			lang = "en"
		}
		audio, err := SynthesizeGoogleTTS(r.Context(), h.Client, req.Input, lang)
		if err != nil {
			log.Error("media", "google-tts synthesis failed", "error", err)
			handlerutil.WriteJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		h.writeTTSResponse(w, r, req.ResponseFormat, audio, "audio/mpeg", "mp3")
		h.trackTTSUsage(modelInfo.Provider, modelInfo.Model)
		return
	}

	// 3. nvidia
	if modelInfo.Provider == "nvidia" {
		h.forwardNvidiaTTS(w, r, body, modelInfo, req.Input, req.Voice, req.ResponseFormat)
		return
	}

	// 4. gemini / antigravity native audio TTS
	if modelInfo.Provider == "gemini" || modelInfo.Provider == "gm" || modelInfo.Provider == "antigravity" || modelInfo.Provider == "ag" || strings.HasPrefix(modelInfo.Model, "gemini") {
		h.forwardGeminiTTS(w, r, body, modelInfo, req.Input, req.Voice, req.ResponseFormat)
		return
	}

	// 5. Default OpenAI-compatible TTS
	h.forwardMediaRequest(w, r, body, "tts-1", "/v1/audio/speech")
}

func (h *MediaHandler) trackTTSUsage(provider, model string) {
	usagetracker.GetTracker().PushRecent(usagetracker.RecentRequest{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Model:     model,
		Provider:  provider,
		Status:    "ok",
	}, h.Repo)
}

func (h *MediaHandler) writeTTSResponse(w http.ResponseWriter, r *http.Request, reqFormat string, audio []byte, mimeType, ext string) {
	isJSON := reqFormat == "json" || r.URL.Query().Get("response_format") == "json"
	if isJSON {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		jsonBytes, _ := json.Marshal(map[string]any{
			"format": ext,
			"audio":  base64.StdEncoding.EncodeToString(audio),
		})
		_, _ = w.Write(jsonBytes)
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", strconv.Itoa(len(audio)))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"speech.%s\"", ext))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
}

func (h *MediaHandler) forwardNvidiaTTS(w http.ResponseWriter, r *http.Request, body []byte, modelInfo *chat.ModelInfo, input, voice, respFmt string) {
	preferredConnID := r.Header.Get("x-connection-id")
	if preferredConnID == "" {
		preferredConnID = r.Header.Get("x-provider-connection-id")
	}
	if preferredConnID == "" {
		preferredConnID = modelInfo.ConnectionID
	}
	// Upstream parity (tts.js credential loop): rotate accounts, lock failures.
	usePinned := preferredConnID != ""
	excludeIDs := []string{}
	var lastStatus int
	var lastMsg string
	for {
		conn, connData, err := h.ChatH.GetBestConnectionWithContext(r.Context(), "nvidia", preferredConnID, excludeIDs, modelInfo.Model)
		if err != nil || connData == nil {
			if lastMsg != "" {
				handlerutil.WriteJSONError(w, lastStatus, lastMsg)
				return
			}
			handlerutil.WriteJSONError(w, http.StatusNotFound, "no active connections for provider: nvidia")
			return
		}
		status, msg, done := h.tryNvidiaTTSConn(w, r, body, modelInfo, input, voice, respFmt, conn, connData)
		if done {
			return
		}
		lastStatus, lastMsg = status, msg
		if usePinned {
			handlerutil.WriteJSONError(w, status, msg)
			return
		}
		excludeIDs = append(excludeIDs, conn.ID)
	}
}

func (h *MediaHandler) tryNvidiaTTSConn(w http.ResponseWriter, r *http.Request, body []byte, modelInfo *chat.ModelInfo, input, voice, respFmt string, conn *models.ProviderConnection, connData *chat.ConnectionData) (int, string, bool) {
	apiKey := chat.ExtractAPIKey(connData)
	if apiKey == "" {
		return http.StatusUnauthorized, "no API key found for nvidia", false
	}

	modelID := strings.Split(modelInfo.Model, "/")[0]
	if modelID == "" {
		modelID = "fastpitch"
	}
	voiceID := voice
	if voiceID == "" || voiceID == "alloy" {
		parts := strings.Split(modelInfo.Model, "/")
		if len(parts) > 1 {
			voiceID = parts[1]
		} else {
			voiceID = "default"
		}
	}

	payload := map[string]any{
		"input": map[string]string{
			"text": input,
		},
		"voice": voiceID,
		"model": modelID,
	}
	payloadBytes, _ := json.Marshal(payload)

	targetURL := nvidiaTTSURL(h.ChatH, connData)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return http.StatusInternalServerError, "failed to create nvidia tts request", false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client, clientErr := h.ChatH.GetClientForConnection(connData)
	if clientErr != nil {
		return http.StatusBadGateway, clientErr.Error(), false
	}
	resp, err := client.Do(req)
	if err != nil {
		return http.StatusBadGateway, "nvidia tts request failed: " + err.Error(), true
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		errText := strings.TrimSpace(string(errBytes))
		if errText == "" {
			errText = resp.Status
		}
		log.Warn("media", "nvidia tts upstream error", "status", resp.StatusCode, "body", errText)
		if h.Repo != nil {
			backoff := h.Repo.GetConnectionBackoffLevel(conn.ID)
			if classification := providers.ClassifyError(resp.StatusCode, errText, backoff); classification.ShouldFallback {
				cooldownSec := max(classification.CooldownMs/1000, 1)
				if !handlerutil.IsProbeContext(r.Context()) {
					_ = h.Repo.LockConnectionModel(conn.ID, modelInfo.Model, cooldownSec, classification.NewBackoffLevel)
				}
				return resp.StatusCode, errText, true
			}
		}
		return resp.StatusCode, errText, false
	}

	audioBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return http.StatusBadGateway, "failed to read nvidia audio response", true
	}

	if h.Repo != nil {
		h.Repo.UpdateConnectionLastUsed(conn.ID)
		// A probe must not clear production state: the lock belongs to a real
		// failure, and one click of Test would erase the backoff the
		// operator is relying on. UpdateConnectionLastUsed still runs — the
		// probe really did reach the provider.
		if !handlerutil.IsProbeContext(r.Context()) {
			_ = h.Repo.UnlockConnectionModel(conn.ID, modelInfo.Model)
		}
	}
	h.trackTTSUsage("nvidia", modelInfo.Model)
	h.writeTTSResponse(w, r, respFmt, audioBytes, "audio/wav", "wav")
	return 0, "", true
}

// nvidiaTTSURL honors provider/connection base URL overrides (upstream
// genericFormats nvidia uses cfg.baseUrl); falls back to NVIDIA NIM cloud.
func nvidiaTTSURL(chatH *chat.ChatHandler, connData *chat.ConnectionData) string {
	if connData != nil && connData.BaseURL != "" {
		return strings.TrimRight(connData.BaseURL, "/")
	}
	if cfg, err := chatH.GetProviderConfig("nvidia", connData); err == nil && cfg != nil && cfg.BaseURL != "" {
		return strings.TrimRight(cfg.BaseURL, "/")
	}
	return "https://integrate.api.nvidia.com/v1/audio/speech"
}

func (h *MediaHandler) forwardGeminiTTS(w http.ResponseWriter, r *http.Request, body []byte, modelInfo *chat.ModelInfo, input, voice, respFmt string) {
	conn, connData, err := h.ChatH.GetBestConnectionWithContext(r.Context(), modelInfo.Provider, modelInfo.ConnectionID, nil, modelInfo.Model)
	if err != nil || connData == nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, fmt.Sprintf("no active connection for %s", modelInfo.Provider))
		return
	}
	apiKey := chat.ExtractAPIKey(connData)
	if apiKey == "" {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "no API key found for gemini")
		return
	}

	model := modelInfo.Model
	if model == "" || model == "tts" || model == "speech" || model == "gemini" {
		model = "gemini-2.5-flash-preview-tts"
	}
	// Map voice to Gemini prebuilt voices (Puck, Charon, Kore, Fenrir, Aoede)
	voiceName := "Puck"
	switch strings.ToLower(voice) {
	case "alloy", "puck":
		voiceName = "Puck"
	case "echo", "charon":
		voiceName = "Charon"
	case "fable", "kore", "shimmer":
		voiceName = "Kore"
	case "onyx", "fenrir":
		voiceName = "Fenrir"
	case "nova", "aoede":
		voiceName = "Aoede"
	default:
		if voice != "" && voice != "default" {
			voiceName = voice
		}
	}

	payload := map[string]any{
		"contents": []any{
			map[string]any{
				"parts": []any{
					map[string]any{"text": input},
				},
			},
		},
		"generationConfig": map[string]any{
			"responseModalities": []string{"AUDIO"},
			"speechConfig": map[string]any{
				"voiceConfig": map[string]any{
					"prebuiltVoiceConfig": map[string]any{
						"voiceName": voiceName,
					},
				},
			},
		},
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to marshal gemini tts payload")
		return
	}

	providerCfg, _ := h.ChatH.GetProviderConfig(modelInfo.Provider, connData)
	baseURL := "https://generativelanguage.googleapis.com/v1beta"
	if providerCfg != nil && providerCfg.BaseURL != "" {
		baseURL = strings.TrimRight(providerCfg.BaseURL, "/")
		baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
		baseURL = strings.TrimSuffix(baseURL, "/openai")
	}
	if connData != nil && connData.BaseURL != "" {
		baseURL = strings.TrimRight(connData.BaseURL, "/")
		baseURL = strings.TrimSuffix(baseURL, "/chat/completions")
		baseURL = strings.TrimSuffix(baseURL, "/openai")
	}

	targetURL := fmt.Sprintf("%s/models/%s:generateContent", baseURL, model)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to create gemini tts request")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if providerCfg != nil && providerCfg.AuthHeader != "" {
		handlerutil.SetAuthHeader(req, apiKey, providerCfg.AuthHeader, providerCfg.AuthScheme)
	} else {
		req.Header.Set("x-goog-api-key", apiKey)
	}

	client, clientErr := h.ChatH.GetClientForConnection(connData)
	if clientErr != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, clientErr.Error())
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "gemini tts request failed: "+err.Error())
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to read gemini tts response")
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Warn("media", "gemini tts upstream error", "status", resp.StatusCode, "body", string(respBody[:min(300, len(respBody))]))
		handlerutil.WriteJSONError(w, resp.StatusCode, fmt.Sprintf("gemini tts upstream error %d: %s", resp.StatusCode, string(respBody[:min(300, len(respBody))])))
		return
	}

	var geminiResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					InlineData struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to parse gemini tts response")
		return
	}

	var audioB64 string
	audioMime := "audio/mp3"
	if len(geminiResp.Candidates) > 0 {
		for _, part := range geminiResp.Candidates[0].Content.Parts {
			if part.InlineData.Data != "" {
				audioB64 = part.InlineData.Data
				if part.InlineData.MimeType != "" {
					audioMime = part.InlineData.MimeType
				}
				break
			}
		}
	}
	if audioB64 == "" {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "no audio data returned from gemini")
		return
	}

	audioBytes, err := base64.StdEncoding.DecodeString(audioB64)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "failed to decode gemini audio base64")
		return
	}

	ext := "mp3"
	if strings.Contains(audioMime, "wav") {
		ext = "wav"
	} else if strings.Contains(audioMime, "ogg") {
		ext = "ogg"
	}

	if h.Repo != nil {
		h.Repo.UpdateConnectionLastUsed(conn.ID)
	}
	h.trackTTSUsage(modelInfo.Provider, modelInfo.Model)
	h.writeTTSResponse(w, r, respFmt, audioBytes, audioMime, ext)
}
