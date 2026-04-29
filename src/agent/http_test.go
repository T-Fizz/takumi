package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRealHTTPPostJSON_ForwardsHeadersAndBody verifies the real poster sets
// content-type, copies caller headers, and serializes the body as JSON.
func TestRealHTTPPostJSON_ForwardsHeadersAndBody(t *testing.T) {
	var gotMethod, gotContentType, gotAuth string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, &gotBody))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	got, err := realHTTPPostJSON(srv.URL, map[string]string{
		"Authorization": "Bearer secret",
	}, map[string]any{"hello": "world"})

	require.NoError(t, err)
	assert.Equal(t, "POST", gotMethod)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, "Bearer secret", gotAuth)
	assert.Equal(t, map[string]any{"hello": "world"}, gotBody)
	assert.JSONEq(t, `{"ok":true}`, string(got))
}

// TestRealHTTPPostJSON_NonOKStatus_ReturnsErrorWithBody verifies that a 4xx/5xx
// response surfaces both the status code and the response body in the error.
func TestRealHTTPPostJSON_NonOKStatus_ReturnsErrorWithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	_, err := realHTTPPostJSON(srv.URL, nil, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error 401")
	assert.Contains(t, err.Error(), "bad key")
}

// TestRealHTTPPostJSON_UnreachableHost_ReturnsRequestError verifies the
// transport-level failure is wrapped, not silently swallowed.
func TestRealHTTPPostJSON_UnreachableHost_ReturnsRequestError(t *testing.T) {
	// Use a port that's almost certainly closed — connection refused.
	_, err := realHTTPPostJSON("http://127.0.0.1:1/nope", nil, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API request failed")
}

// TestRealHTTPPostJSON_UnmarshalableBody_ReturnsMarshalError verifies that
// non-JSON-serializable bodies (channels, funcs) are caught before the request.
func TestRealHTTPPostJSON_UnmarshalableBody_ReturnsMarshalError(t *testing.T) {
	_, err := realHTTPPostJSON("http://example.com", nil, make(chan int))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshaling request")
}

// withMockHTTP installs a fake httpPostJSON for the duration of the test and
// returns a captor that records the requests for assertion.
type httpCall struct {
	url     string
	headers map[string]string
	body    any
}

func withMockHTTP(t *testing.T, responses ...[]byte) *[]httpCall {
	t.Helper()
	calls := []httpCall{}
	idx := 0
	original := httpPostJSON
	httpPostJSON = func(url string, headers map[string]string, body any) ([]byte, error) {
		calls = append(calls, httpCall{url: url, headers: headers, body: body})
		if idx >= len(responses) {
			t.Fatalf("unexpected http call #%d to %s — only %d response(s) registered", idx+1, url, len(responses))
		}
		resp := responses[idx]
		idx++
		return resp, nil
	}
	t.Cleanup(func() { httpPostJSON = original })
	return &calls
}
