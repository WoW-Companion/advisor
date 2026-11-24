package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Endpoint is where an adapter sends its calls. Key may be empty for a local server.
type Endpoint struct {
	BaseURL string
	Key     string
	HTTP    *http.Client // nil: http.DefaultClient (the caller's context sets the timeout)
	// Extra fields merged into every request body (for example reasoning_effort).
	Extra map[string]any
}

func (e Endpoint) client() *http.Client {
	if e.HTTP != nil {
		return e.HTTP
	}
	return http.DefaultClient
}

func (e Endpoint) url(path string) string { return strings.TrimRight(e.BaseURL, "/") + path }

// do sends a JSON body (or no body, for GET) and returns the response when it is a 2xx. Other
// statuses become errors (see httpError); a connection failure is a StartError.
func (e Endpoint) do(ctx context.Context, method, path string, body any, headers map[string]string) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, e.url(path), rd)
	if err != nil {
		return nil, &StartError{"The provider's address isn't valid."}
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := e.client().Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var ue *url.Error
		reason := err.Error()
		if errors.As(err, &ue) {
			reason = ue.Err.Error()
		}
		return nil, &StartError{fmt.Sprintf("Couldn't reach %s: %s", req.URL.Host, scrub(reason, e.Key))}
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		return nil, httpError(resp.StatusCode, b, e.Key)
	}
	return resp, nil
}

func scrub(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "…")
}

// merge adds the endpoint's extra fields to a request body map.
func (e Endpoint) merge(m map[string]any) map[string]any {
	for k, v := range e.Extra {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return m
}

// ModelList asks an OpenAI-style /models endpoint (also Anthropic's, which uses the same shape) for model ids.
func (e Endpoint) ModelList(ctx context.Context, headers map[string]string) ([]string, error) {
	resp, err := e.do(ctx, http.MethodGet, "/models", nil, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return nil, errors.New("the provider's model list wasn't in the expected form")
	}
	var ids []string
	for _, d := range out.Data {
		ids = append(ids, d.ID)
	}
	if len(ids) == 0 {
		for _, m := range out.Models {
			ids = append(ids, m.Name)
		}
	}
	return ids, nil
}
