package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"9router/proxy/internal/constants"
	"9router/proxy/internal/log"
	"9router/proxy/internal/providers"

	"github.com/anthropics/anthropic-sdk-go"
	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go"
	openaiOption "github.com/openai/openai-go/option"
)

// ForwardOpenAI sends a request to the provider endpoint using the official SDK client
// (Anthropic SDK for Claude endpoints, OpenAI SDK for OpenAI and OpenAI-compatible providers).
func ForwardOpenAI(ctx context.Context, client *http.Client, cfg *providers.ProviderConfig, apiKey string, body []byte, isStream bool) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if isAnthropicEndpoint(cfg) {
		return forwardAnthropicSDK(ctx, client, cfg, apiKey, body, isStream)
	}
	return forwardOpenAISDK(ctx, client, cfg, apiKey, body, isStream)
}

func isAnthropicEndpoint(cfg *providers.ProviderConfig) bool {
	if cfg == nil {
		return false
	}
	if cfg.Format == providers.FormatClaude {
		return true
	}
	lower := strings.ToLower(cfg.BaseURL)
	return strings.Contains(lower, "api.anthropic.com") || strings.HasSuffix(lower, "/messages")
}

func forwardAnthropicSDK(ctx context.Context, client *http.Client, cfg *providers.ProviderConfig, apiKey string, body []byte, isStream bool) (*http.Response, error) {
	baseURL := cfg.BaseURL
	path := "/messages"
	query := ""
	if idx := strings.Index(baseURL, "?"); idx != -1 {
		query = baseURL[idx:]
		baseURL = baseURL[:idx]
	}

	for _, lane := range []string{"/v1/messages", "/messages"} {
		if strings.HasSuffix(baseURL, lane) {
			baseURL = strings.TrimSuffix(baseURL, lane)
			path = lane
			break
		}
	}
	if query != "" {
		path += query
	}

	headers := map[string]string{}
	if !cfg.NoAuth {
		switch cfg.AuthScheme {
		case "bearer":
			headers[cfg.AuthHeader] = "Bearer " + apiKey
		case "raw":
			headers[cfg.AuthHeader] = apiKey
		default:
			headers["x-api-key"] = apiKey
		}
	}
	for k, v := range cfg.StaticHeaders {
		headers[k] = v
	}
	if isStream {
		headers["Accept"] = "text/event-stream"
	}
	if !hasHeaderFold(headers, "User-Agent") {
		headers["User-Agent"] = constants.DefaultClientUserAgent
	}
	if !hasHeaderFold(headers, "Accept") {
		headers["Accept"] = "*/*"
	}
	headers = providers.ApplyHeaderOverrides(headers, cfg.HeaderOverrides)
	if isStream && !hasHeaderFold(headers, "Accept-Encoding") {
		headers["Accept-Encoding"] = "identity"
	}

	baseURL = strings.TrimRight(baseURL, "/")
	return retryTransientUpstream(ctx, func(c context.Context) (*http.Response, error) {
		opts := []anthropicOption.RequestOption{
			anthropicOption.WithBaseURL(baseURL),
			anthropicOption.WithHTTPClient(client),
			anthropicOption.WithMaxRetries(0),
		}
		for k, v := range headers {
			opts = append(opts, anthropicOption.WithHeader(k, v))
		}

		sdkClient := anthropic.NewClient(opts...)
		var resp *http.Response
		err := sdkClient.Execute(c, http.MethodPost, path, bytes.NewReader(body), &resp)
		if resp != nil {
			UnwrapIdentityGzip(nil, resp)
			if resp.StatusCode != http.StatusOK {
				errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
				_ = resp.Body.Close()
				return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: errBody, Header: resp.Header, ReadErr: readErr}
			}
			return resp, nil
		}
		if err != nil {
			switch {
			case isCertificateFailure(err):
				if !strictTLSRequested() {
					log.Warn("proxy", "TLS verification failed, retrying without certificate verification", "url", cfg.BaseURL, "error", err)
					return replayInsecureTLS(c, proxyURLFor(client), "POST", cfg.BaseURL, headers, body)
				}
			case isProxyFailure(err, resp):
				if !strictOnly(c) && !forbidsDirectReplay(client) && !proxiesViaAssignment(client) {
					log.Warn("proxy", "request through the proxy failed, retrying direct", "url", cfg.BaseURL, "error", err)
					return replayDirect(c, "POST", cfg.BaseURL, headers, body)
				}
			}
			return nil, fmt.Errorf("anthropic sdk to %s: %w", cfg.BaseURL, err)
		}
		return nil, fmt.Errorf("anthropic sdk returned empty response")
	})
}

func forwardOpenAISDK(ctx context.Context, client *http.Client, cfg *providers.ProviderConfig, apiKey string, body []byte, isStream bool) (*http.Response, error) {
	baseURL := cfg.BaseURL
	path := ""
	query := ""
	if idx := strings.Index(baseURL, "?"); idx != -1 {
		query = baseURL[idx:]
		baseURL = baseURL[:idx]
	}

	for _, lane := range []string{"/chat/completions", "/responses", "/completions", "/messages", "/chat"} {
		if strings.HasSuffix(baseURL, lane) {
			baseURL = strings.TrimSuffix(baseURL, lane)
			path = lane
			break
		}
	}
	if path == "" {
		path = "/chat/completions"
	}
	if query != "" {
		path += query
	}

	headers := map[string]string{}
	if !cfg.NoAuth {
		switch cfg.AuthScheme {
		case "bearer":
			headers[cfg.AuthHeader] = "Bearer " + apiKey
		case "raw":
			headers[cfg.AuthHeader] = apiKey
		default:
			headers["Authorization"] = "Bearer " + apiKey
		}
	}
	for k, v := range cfg.StaticHeaders {
		headers[k] = v
	}
	if isStream {
		headers["Accept"] = "text/event-stream"
	}
	if !hasHeaderFold(headers, "User-Agent") {
		headers["User-Agent"] = constants.DefaultClientUserAgent
	}
	if !hasHeaderFold(headers, "Accept") {
		headers["Accept"] = "*/*"
	}
	headers = providers.ApplyHeaderOverrides(headers, cfg.HeaderOverrides)
	if isStream && !hasHeaderFold(headers, "Accept-Encoding") {
		headers["Accept-Encoding"] = "identity"
	}

	baseURL = strings.TrimRight(baseURL, "/")
	return retryTransientUpstream(ctx, func(c context.Context) (*http.Response, error) {
		opts := []openaiOption.RequestOption{
			openaiOption.WithBaseURL(baseURL),
			openaiOption.WithHTTPClient(client),
			openaiOption.WithMaxRetries(0),
		}
		for k, v := range headers {
			opts = append(opts, openaiOption.WithHeader(k, v))
		}

		sdkClient := openai.NewClient(opts...)
		var resp *http.Response
		err := sdkClient.Execute(c, http.MethodPost, path, bytes.NewReader(body), &resp)
		if resp != nil {
			UnwrapIdentityGzip(nil, resp)
			if resp.StatusCode != http.StatusOK {
				errBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1*1024*1024))
				_ = resp.Body.Close()
				return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: errBody, Header: resp.Header, ReadErr: readErr}
			}
			return resp, nil
		}
		if err != nil {
			switch {
			case isCertificateFailure(err):
				if !strictTLSRequested() {
					log.Warn("proxy", "TLS verification failed, retrying without certificate verification", "url", cfg.BaseURL, "error", err)
					return replayInsecureTLS(c, proxyURLFor(client), "POST", cfg.BaseURL, headers, body)
				}
			case isProxyFailure(err, resp):
				if !strictOnly(c) && !forbidsDirectReplay(client) && !proxiesViaAssignment(client) {
					log.Warn("proxy", "request through the proxy failed, retrying direct", "url", cfg.BaseURL, "error", err)
					return replayDirect(c, "POST", cfg.BaseURL, headers, body)
				}
			}
			return nil, fmt.Errorf("openai sdk to %s: %w", cfg.BaseURL, err)
		}
		return nil, fmt.Errorf("openai sdk returned empty response")
	})
}

// ReadBody reads and returns the response body (capped to prevent
// unbounded memory use), closing it.
func ReadBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
}

// UpstreamBody reads the body and wraps non-200 as UpstreamError.
func UpstreamBody(resp *http.Response) ([]byte, error) {
	body, err := ReadBody(resp)
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &UpstreamError{StatusCode: resp.StatusCode, Body: body, Header: resp.Header}
	}
	return body, nil
}

// BuildURL joins a base URL with a path segment.
func BuildURL(base, path string) string {
	if base == "" {
		return path
	}
	if path == "" {
		return base
	}
	return fmt.Sprintf("%s/%s", base, path)
}

// hasHeaderFold reports whether headers already carries key under any casing.
func hasHeaderFold(headers map[string]string, key string) bool {
	for k := range headers {
		if strings.EqualFold(k, key) {
			return true
		}
	}
	return false
}
