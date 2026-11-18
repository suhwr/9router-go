package media

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"os/exec"
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
		if strings.HasPrefix(req.Model, "gemini-") || strings.HasPrefix(req.Model, "gemini") {
			modelInfo = &chat.ModelInfo{Provider: "gemini", Model: req.Model}
			err = nil
		} else {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
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
	preferredConnID := r.Header.Get("x-connection-id")
	if preferredConnID == "" {
		preferredConnID = r.Header.Get("x-provider-connection-id")
	}
	if preferredConnID == "" {
		preferredConnID = modelInfo.ConnectionID
	}
	usePinned := preferredConnID != ""
	excludeIDs := []string{}
	var lastStatus int
	var lastMsg string

	provider := modelInfo.Provider
	if provider == "" || provider == "openai" {
		provider = "gemini"
	}

	for {
		conn, connData, err := h.ChatH.GetBestConnectionWithContext(r.Context(), provider, preferredConnID, excludeIDs, modelInfo.Model)
		if err != nil || conn == nil || connData == nil {
			if lastMsg != "" {
				handlerutil.WriteJSONError(w, lastStatus, lastMsg)
				return
			}
			handlerutil.WriteJSONError(w, http.StatusNotFound, fmt.Sprintf("no active connections for provider: %s", provider))
			return
		}

		status, msg, done := h.tryGeminiTTSConn(w, r, modelInfo, input, voice, respFmt, conn, connData)
		if done {
			return
		}
		lastStatus = status
		lastMsg = msg
		if usePinned {
			handlerutil.WriteJSONError(w, status, msg)
			return
		}
		excludeIDs = append(excludeIDs, conn.ID)
	}
}

func (h *MediaHandler) tryGeminiTTSConn(w http.ResponseWriter, r *http.Request, modelInfo *chat.ModelInfo, input, voice, respFmt string, conn *models.ProviderConnection, connData *chat.ConnectionData) (int, string, bool) {
	apiKey := chat.ExtractAPIKey(connData)
	if apiKey == "" {
		return http.StatusUnauthorized, "no API key found for gemini connection " + conn.ID, false
	}

	rawModel := modelInfo.Model
	for _, p := range []string{"gemini/", "gm/", "ag/", "antigravity/"} {
		rawModel = strings.TrimPrefix(rawModel, p)
	}
	parts := strings.Split(rawModel, "/")
	cleanModel := parts[0]
	if len(parts) > 1 && (voice == "" || voice == "alloy" || voice == "default") {
		voice = parts[1]
	}
	if cleanModel == "" || cleanModel == "tts" || cleanModel == "speech" || cleanModel == "gemini" {
		cleanModel = "gemini-2.5-flash-preview-tts"
	}

	// Map voice to Gemini prebuilt voices
	voiceName := "Kore"
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
			voiceName = strings.ToUpper(voice[:1]) + strings.ToLower(voice[1:])
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
		return http.StatusInternalServerError, "failed to marshal gemini tts payload", false
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

	targetURL := fmt.Sprintf("%s/models/%s:generateContent", baseURL, cleanModel)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, targetURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return http.StatusInternalServerError, "failed to create gemini tts request", false
	}
	req.Header.Set("Content-Type", "application/json")
	if conn != nil && conn.AuthType == "oauth" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		req.Header.Set("x-goog-api-key", apiKey)
	}

	client, clientErr := h.ChatH.GetClientForConnection(connData)
	if clientErr != nil {
		return http.StatusBadGateway, clientErr.Error(), false
	}
	resp, err := client.Do(req)
	if err != nil {
		return http.StatusBadGateway, "gemini tts request failed: " + err.Error(), false
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return http.StatusBadGateway, "failed to read gemini tts response", false
	}

	if resp.StatusCode != http.StatusOK {
		errText := string(respBody[:min(500, len(respBody))])
		log.Warn("media", "gemini tts upstream error", "conn", conn.Name, "status", resp.StatusCode, "body", errText)
		if h.Repo != nil {
			backoff := h.Repo.GetConnectionBackoffLevel(conn.ID)
			classification := providers.ClassifyError(resp.StatusCode, errText, backoff)
			cooldownSec := max(classification.CooldownMs/1000, 1)
			if resetSec, ok := geminiQuotaResetSeconds(respBody); ok {
				cooldownSec = int(resetSec)
			}
			if !handlerutil.IsProbeContext(r.Context()) {
				_ = h.Repo.LockConnectionModel(conn.ID, modelInfo.Model, cooldownSec, classification.NewBackoffLevel)
				if cleanModel != modelInfo.Model {
					_ = h.Repo.LockConnectionModel(conn.ID, cleanModel, cooldownSec, classification.NewBackoffLevel)
				}
			}
		}
		return resp.StatusCode, errText, false
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
		return http.StatusBadGateway, "failed to parse gemini tts response", false
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
		return http.StatusBadGateway, "no audio data returned from gemini", false
	}

	audioBytes, err := base64.StdEncoding.DecodeString(audioB64)
	if err != nil {
		return http.StatusBadGateway, "failed to decode gemini audio base64", false
	}

	// Raw PCM returned by Gemini (e.g. audio/l16; rate=24000) needs RIFF WAV header
	if strings.Contains(strings.ToLower(audioMime), "pcm") || strings.Contains(strings.ToLower(audioMime), "l16") {
		sampleRate := 24000
		if idx := strings.Index(strings.ToLower(audioMime), "rate="); idx > 0 {
			var r int
			if _, err := fmt.Sscanf(audioMime[idx+5:], "%d", &r); err == nil && r > 0 {
				sampleRate = r
			}
		}
		audioBytes = pcmToWav(audioBytes, sampleRate, 1)
		audioMime = "audio/wav"
	}

	ext := "wav"
	if respFmt == "mp3" || respFmt == "" {
		if mp3Bytes, err := wavToMP3(audioBytes); err == nil {
			audioBytes = mp3Bytes
			audioMime = "audio/mpeg"
			ext = "mp3"
		}
	} else if strings.Contains(audioMime, "mpeg") || strings.Contains(audioMime, "mp3") {
		ext = "mp3"
	} else if strings.Contains(audioMime, "ogg") {
		ext = "ogg"
	}

	if h.Repo != nil {
		h.Repo.UpdateConnectionLastUsed(conn.ID)
		if !handlerutil.IsProbeContext(r.Context()) {
			_ = h.Repo.UnlockConnectionModel(conn.ID, modelInfo.Model)
			if cleanModel != modelInfo.Model {
				_ = h.Repo.UnlockConnectionModel(conn.ID, cleanModel)
			}
		}
	}
	h.trackTTSUsage(modelInfo.Provider, modelInfo.Model)
	h.writeTTSResponse(w, r, respFmt, audioBytes, audioMime, ext)
	return 0, "", true
}

func pcmToWav(pcmData []byte, sampleRate int, numChannels int) []byte {
	if sampleRate <= 0 {
		sampleRate = 24000
	}
	if numChannels <= 0 {
		numChannels = 1
	}
	const bitsPerSample = 16
	dataSize := len(pcmData)
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataSize))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(numChannels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	byteRate := uint32(sampleRate * numChannels * (bitsPerSample / 8))
	binary.LittleEndian.PutUint32(header[28:32], byteRate)
	blockAlign := uint16(numChannels * (bitsPerSample / 8))
	binary.LittleEndian.PutUint16(header[32:34], blockAlign)
	binary.LittleEndian.PutUint16(header[34:36], bitsPerSample)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataSize))

	return append(header, pcmData...)
}

func wavToMP3(wavData []byte) ([]byte, error) {
	cmd := exec.Command("ffmpeg", "-y", "-f", "wav", "-i", "pipe:0", "-f", "mp3", "-b:a", "128k", "pipe:1")
	cmd.Stdin = bytes.NewReader(wavData)
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg mp3 conversion: %w (%s)", err, strings.TrimSpace(errOut.String()))
	}
	return out.Bytes(), nil
}
