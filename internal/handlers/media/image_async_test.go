package media

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"9router/proxy/internal/db"
	"9router/proxy/internal/handlers/chat"
)

func TestHandleImages_AutoPollAsyncJob(t *testing.T) {
	var pollCount int32

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/images/generations") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "img_async_123",
				"status": "queued",
				"object": "image.generation.job",
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/images/jobs/img_async_123") {
			count := atomic.AddInt32(&pollCount, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if count < 2 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id":     "img_async_123",
					"status": "processing",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "img_async_123",
				"status": "succeeded",
				"data": []map[string]any{
					{"url": "https://img.test.com/result.png"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	database, teardown := setupMultimodalTestDB(t)
	defer teardown()
	_, _ = database.Exec(`CREATE TABLE IF NOT EXISTS providerNodes (
		id TEXT PRIMARY KEY,
		type TEXT,
		name TEXT,
		data TEXT NOT NULL,
		createdAt TEXT NOT NULL,
		updatedAt TEXT NOT NULL
	)`)

	repo := db.NewRepo(database)
	chatH := chat.NewChatHandler(repo, nil)
	handler := NewMediaHandler(repo, nil, chatH)

	// Seed custom provider node with prefix "kovapi"
	nodeData, _ := json.Marshal(map[string]any{
		"prefix":  "kovapi",
		"apiType": "chat",
		"baseUrl": upstream.URL + "/v1",
	})
	if _, err := database.Exec(`INSERT INTO providerNodes (id, name, type, data, createdAt, updatedAt) VALUES
		('node-kovapi', 'KovAPI', 'openai-compatible', ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`, string(nodeData)); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	connData, _ := json.Marshal(map[string]any{
		"apiKey":  "sk-kovapi-test",
		"baseUrl": upstream.URL + "/v1",
		"providerSpecificData": map[string]any{
			"prefix":  "kovapi",
			"baseUrl": upstream.URL + "/v1",
		},
	})
	if _, err := database.Exec(`INSERT INTO providerConnections (id, provider, authType, name, priority, isActive, data, createdAt, updatedAt) VALUES
		('conn-kovapi', 'node-kovapi', 'apikey', 'KovAPI Conn', 1, 1, ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`, string(connData)); err != nil {
		t.Fatalf("seed conn: %v", err)
	}

	reqBody := `{"model":"gpt-image-2.5-flare","prompt":"a test flower"}`
	req := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleImages(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Data   []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "succeeded" {
		t.Errorf("expected status 'succeeded', got %q", resp.Status)
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://img.test.com/result.png" {
		t.Errorf("unexpected data: %+v", resp.Data)
	}
	if atomic.LoadInt32(&pollCount) < 2 {
		t.Errorf("expected at least 2 polls, got %d", atomic.LoadInt32(&pollCount))
	}
}

func TestHandleImageJobGet(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/images/jobs/img_job_456") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "img_job_456",
				"status": "succeeded",
				"data": []map[string]any{
					{"url": "https://img.test.com/job456.png"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	database, teardown := setupMultimodalTestDB(t)
	defer teardown()
	_, _ = database.Exec(`CREATE TABLE IF NOT EXISTS providerNodes (
		id TEXT PRIMARY KEY,
		type TEXT,
		name TEXT,
		data TEXT NOT NULL,
		createdAt TEXT NOT NULL,
		updatedAt TEXT NOT NULL
	)`)

	repo := db.NewRepo(database)
	chatH := chat.NewChatHandler(repo, nil)
	handler := NewMediaHandler(repo, nil, chatH)

	nodeData, _ := json.Marshal(map[string]any{
		"prefix":  "kovapi",
		"baseUrl": upstream.URL + "/v1",
	})
	_, _ = database.Exec(`INSERT INTO providerNodes (id, name, type, data, createdAt, updatedAt) VALUES
		('node-kovapi-2', 'KovAPI', 'openai-compatible', ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`, string(nodeData))

	connData, _ := json.Marshal(map[string]any{
		"apiKey":  "sk-kovapi-test",
		"baseUrl": upstream.URL + "/v1",
	})
	_, _ = database.Exec(`INSERT INTO providerConnections (id, provider, authType, name, priority, isActive, data, createdAt, updatedAt) VALUES
		('conn-kovapi-2', 'node-kovapi-2', 'apikey', 'KovAPI Conn', 1, 1, ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`, string(connData))

	req := httptest.NewRequest("GET", "/v1/images/jobs/img_job_456", nil)
	req.SetPathValue("id", "img_job_456")
	rec := httptest.NewRecorder()

	handler.HandleImageJobGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "https://img.test.com/job456.png") {
		t.Errorf("expected response to contain image url, got: %s", rec.Body.String())
	}
}
