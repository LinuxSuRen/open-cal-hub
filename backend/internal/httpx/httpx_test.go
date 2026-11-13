package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := NewRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.Allow("k") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if rl.Allow("k") {
		t.Fatal("4th request in window should be denied")
	}
	if !rl.Allow("other") {
		t.Fatal("different key has independent bucket")
	}
}

func TestRateLimiterRefill(t *testing.T) {
	rl := NewRateLimiter(2, 40*time.Millisecond)
	rl.Allow("k")
	rl.Allow("k")
	if rl.Allow("k") {
		t.Fatal("should be empty")
	}
	time.Sleep(60 * time.Millisecond) // 至少补充 1 个
	if !rl.Allow("k") {
		t.Fatal("token should refill after window")
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := rl.Middleware("login", ok)
	ip := ""
	req := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		if ip != "" {
			r.RemoteAddr = ip
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := req(); w.Code != 200 {
		t.Fatalf("1st: %d", w.Code)
	}
	if w := req(); w.Code != 200 {
		t.Fatalf("2nd: %d", w.Code)
	}
	w := req()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd should be 429, got %d", w.Code)
	}
	if got := w.Body.String(); got == "" || !contains(got, "common/rate_limited") {
		t.Fatalf("body should carry error envelope: %s", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	if got := ClientIP(r); got != "192.0.2.1" {
		t.Fatalf("got %s", got)
	}
	r.RemoteAddr = "no-port"
	if got := ClientIP(r); got != "no-port" {
		t.Fatalf("fallback got %s", got)
	}
}

func TestMetricsMiddlewareAndHandler(t *testing.T) {
	m := NewMetrics()
	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	r := httptest.NewRequest("GET", "/api/v1/meta", nil)
	h.ServeHTTP(httptest.NewRecorder(), r)
	r2 := httptest.NewRequest("GET", "/api/v1/auth/sessions/42", nil)
	h.ServeHTTP(httptest.NewRecorder(), r2)

	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	body := w.Body.String()
	if !contains(body, `http_requests_total{code="201",method="GET",route="/api/v1/meta"} 1`) {
		t.Fatalf("counter missing:\n%s", body)
	}
	if !contains(body, `route="/api/v1/auth/sessions/{id}"`) {
		t.Fatalf("route not normalized:\n%s", body)
	}
	if !contains(body, "http_request_duration_seconds_bucket") ||
		!contains(body, `le="+Inf"`) || !contains(body, "http_request_duration_seconds_sum") {
		t.Fatalf("histogram incomplete:\n%s", body)
	}
}

func TestNormalizeRoute(t *testing.T) {
	cases := map[string]string{
		"/api/v1/me":                     "/api/v1/me",
		"/api/v1/events/123":             "/api/v1/events/{id}",
		"/u/rick/intro":                  "/u/rick/intro",
		"/api/v1/pub/share/9f2a1c4de5b6": "/api/v1/pub/share/{id}",
	}
	for in, want := range cases {
		if got := NormalizeRoute(in); got != want {
			t.Errorf("NormalizeRoute(%s)=%s want %s", in, got, want)
		}
	}
}
