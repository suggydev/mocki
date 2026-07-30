package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mocki/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st := store.New()
	if _, err := st.Create("users", store.Item{"id": 1.0, "name": "Ada", "role": "admin"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create("users", store.Item{"id": 2.0, "name": "Bob", "role": "user"}); err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(New(st, Options{CORS: true}))
}

func doJSON(t *testing.T, method, url, body string) (*http.Response, map[string]any) {
	t.Helper()
	var req *http.Request
	var err error
	if body != "" {
		req, err = http.NewRequest(method, url, strings.NewReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestIndex(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	resp, body := doJSON(t, "GET", ts.URL+"/", "")
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	links := body["resources"].(map[string]any)
	if links["users"] != "/users" {
		t.Fatalf("resources = %v", links)
	}
}

func TestListWithTotal(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/users")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("X-Total-Count") != "2" {
		t.Fatalf("X-Total-Count = %q", resp.Header.Get("X-Total-Count"))
	}
	var items []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&items)
	if len(items) != 2 {
		t.Fatalf("items = %d", len(items))
	}
}

func TestListFilterQuery(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/users?role=admin")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var items []map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&items)
	if len(items) != 1 || items[0]["name"] != "Ada" {
		t.Fatalf("filtered = %v", items)
	}
}

func TestGetAnd404(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, body := doJSON(t, "GET", ts.URL+"/users/1", "")
	if resp.StatusCode != 200 || body["name"] != "Ada" {
		t.Fatalf("get = %d %v", resp.StatusCode, body)
	}

	resp, _ = doJSON(t, "GET", ts.URL+"/users/999", "")
	if resp.StatusCode != 404 {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestCreatePatchDelete(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()

	resp, body := doJSON(t, "POST", ts.URL+"/users", `{"name": "Cleo"}`)
	if resp.StatusCode != 201 || body["id"] != 3.0 {
		t.Fatalf("create = %d %v", resp.StatusCode, body)
	}

	resp, body = doJSON(t, "PATCH", ts.URL+"/users/3", `{"role": "admin"}`)
	if resp.StatusCode != 200 || body["role"] != "admin" || body["name"] != "Cleo" {
		t.Fatalf("patch = %d %v", resp.StatusCode, body)
	}

	req, _ := http.NewRequest("DELETE", ts.URL+"/users/3", nil)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 204 {
		t.Fatalf("delete status = %d", resp2.StatusCode)
	}
}

func TestCORSHeaders(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	req, _ := http.NewRequest("OPTIONS", ts.URL+"/users", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("CORS origin = %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
	if resp.StatusCode != 204 {
		t.Fatalf("OPTIONS status = %d", resp.StatusCode)
	}
}

func TestLatency(t *testing.T) {
	st := store.New()
	_, _ = st.Create("users", store.Item{"id": 1.0})
	ts := httptest.NewServer(New(st, Options{Latency: 100 * time.Millisecond}))
	defer ts.Close()
	start := time.Now()
	resp, err := http.Get(ts.URL + "/users")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
		t.Fatalf("latency not applied: %v", elapsed)
	}
}
