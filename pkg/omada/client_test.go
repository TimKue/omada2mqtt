package omada

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testToken = "test-access-token"

// newTestServer returns an httptest server speaking the Omada envelope, plus a
// counter of how many token requests it served. devicesBody is written as the
// "result" of the devices endpoint (already-encoded JSON), letting a test
// choose the array or paginated shape.
func newTestServer(t *testing.T, devicesBody string) (*httptest.Server, *int32) {
	t.Helper()
	var tokenHits int32

	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/authorize/token", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenHits, 1)
		if r.Method != http.MethodPost {
			t.Errorf("token: method = %s, want POST", r.Method)
		}
		if got := r.URL.Query().Get("grant_type"); got != "client_credentials" {
			t.Errorf("token: grant_type = %q", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("token: decode body: %v", err)
		}
		if body["client_id"] == "" || body["client_secret"] == "" || body["omadacId"] == "" {
			t.Errorf("token: incomplete credentials in body: %v", body)
		}
		writeEnvelope(w, 0, "", `{"accessToken":"`+testToken+`","tokenType":"bearer","expiresIn":7200}`)
	})

	mux.HandleFunc("/openapi/v1/omadac/sites/s1/devices", func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "AccessToken="+testToken {
			t.Errorf("devices: Authorization = %q, want AccessToken=%s", got, testToken)
		}
		writeEnvelope(w, 0, "", devicesBody)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &tokenHits
}

func writeEnvelope(w http.ResponseWriter, code int, msg, result string) {
	w.Header().Set("Content-Type", "application/json")
	body := `{"errorCode":` + itoa(code) + `,"msg":"` + msg + `"`
	if result != "" {
		body += `,"result":` + result
	}
	body += `}`
	_, _ = w.Write([]byte(body))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

func newTestClient(url string) *Client {
	return NewClient(url, "omadac", "client-id", "client-secret")
}

func TestListDevicesPaginated(t *testing.T) {
	body := `{"totalRows":1,"currentPage":1,"currentSize":1,"data":[{"name":"Core","type":"switch","mac":"AA-BB-CC-00-00-01","status":1,"cpuUtil":10,"memUtil":20}]}`
	srv, _ := newTestServer(t, body)
	c := newTestClient(srv.URL)

	devs, err := c.ListDevices(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devs) != 1 {
		t.Fatalf("got %d devices, want 1", len(devs))
	}
	d := devs[0]
	if d.Name != "Core" || d.Type != "switch" || !d.Online() || d.CPUUtil != 10 {
		t.Errorf("unexpected device: %+v", d)
	}
}

func TestListDevicesBareArray(t *testing.T) {
	body := `[{"name":"AP","type":"ap","mac":"AA-BB-CC-00-00-02","status":0}]`
	srv, _ := newTestServer(t, body)
	c := newTestClient(srv.URL)

	devs, err := c.ListDevices(context.Background(), "s1")
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devs) != 1 || devs[0].Online() {
		t.Fatalf("unexpected devices: %+v", devs)
	}
}

func TestTokenIsCached(t *testing.T) {
	body := `[{"name":"AP","type":"ap","mac":"AA-BB-CC-00-00-02"}]`
	srv, tokenHits := newTestServer(t, body)
	c := newTestClient(srv.URL)

	for i := 0; i < 3; i++ {
		if _, err := c.ListDevices(context.Background(), "s1"); err != nil {
			t.Fatalf("ListDevices #%d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(tokenHits); got != 1 {
		t.Errorf("token requested %d times, want 1 (cached)", got)
	}
}

func TestAPIErrorEnvelope(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/authorize/token", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, 0, "", `{"accessToken":"`+testToken+`","expiresIn":7200}`)
	})
	mux.HandleFunc("/openapi/v1/omadac/sites/s1/devices", func(w http.ResponseWriter, _ *http.Request) {
		writeEnvelope(w, -44112, "token invalid", "")
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := newTestClient(srv.URL)
	_, err := c.ListDevices(context.Background(), "s1")
	if err == nil {
		t.Fatal("expected error from non-zero errorCode")
	}
	if !strings.Contains(err.Error(), "44112") {
		t.Errorf("error should carry the API code, got: %v", err)
	}
}
