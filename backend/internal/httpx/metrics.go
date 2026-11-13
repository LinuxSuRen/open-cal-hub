// 零依赖 Prometheus 指标：HTTP 请求计数与时长直方图，文本格式输出。
// 不引入 client_golang（仓库纪律：不新增第三方依赖），只实现服务端观测所需的最小面。
package httpx

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

var latencyBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

type histogram struct {
	buckets []uint64 // 与 latencyBuckets 对齐（累积）
	sum     float64
	count   uint64
}

func (h *histogram) observe(v float64) {
	for i, ub := range latencyBuckets {
		if v <= ub {
			h.buckets[i]++
		}
	}
	h.sum += v
	h.count++
}

type seriesKey struct{ method, route, code string }

// Metrics 进程内指标聚合器。
type Metrics struct {
	mu       sync.Mutex
	counters map[seriesKey]uint64
	histo    map[seriesKey]*histogram
}

func NewMetrics() *Metrics {
	return &Metrics{counters: map[seriesKey]uint64{}, histo: map[seriesKey]*histogram{}}
}

// Middleware 记录经它转发的请求：计数（method/route/code）+ 时长直方图。
// /metrics 自身不计入，避免抓取流量混入业务观测。
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(rec, r)
		m.observe(r.Method, NormalizeRoute(r.URL.Path), rec.code, time.Since(start).Seconds())
	})
}

func (m *Metrics) observe(method, route string, code int, seconds float64) {
	key := seriesKey{method: method, route: route, code: fmt.Sprintf("%d", code)}
	m.mu.Lock()
	m.counters[key]++
	h := m.histo[key]
	if h == nil {
		h = &histogram{buckets: make([]uint64, len(latencyBuckets))}
		m.histo[key] = h
	}
	h.observe(seconds)
	m.mu.Unlock()
}

type statusRecorder struct {
	http.ResponseWriter
	code      int
	wrote     bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.code = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

// NormalizeRoute 把路径中的数值/十六进制段归一为 {id}，控制标签基数。
func NormalizeRoute(path string) string {
	if len(path) > 200 {
		path = path[:200]
	}
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if s == "" {
			continue
		}
		if isIDLike(s) {
			segs[i] = "{id}"
		} else if len(s) > 32 {
			segs[i] = s[:32]
		}
	}
	return strings.Join(segs, "/")
}

func isIDLike(s string) bool {
	if len(s) > 64 {
		return true
	}
	digits, hex := 0, 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			digits++
			hex++
			continue
		}
		if (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			hex++
			continue
		}
		return false
	}
	return digits == len(s) || (hex == len(s) && len(s) >= 8)
}

// Handler 输出 Prometheus 文本格式（GET /metrics）。
func (m *Metrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b strings.Builder
		b.WriteString("# HELP http_requests_total HTTP 请求总数。\n# TYPE http_requests_total counter\n")
		m.mu.Lock()
		keys := make([]seriesKey, 0, len(m.counters))
		for k := range m.counters {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, b2 := keys[i], keys[j]
			if a.route != b2.route {
				return a.route < b2.route
			}
			if a.method != b2.method {
				return a.method < b2.method
			}
			return a.code < b2.code
		})
		for _, k := range keys {
			// %q 自带标签值转义（反斜杠/引号/换行）。
			fmt.Fprintf(&b, "http_requests_total{code=%q,method=%q,route=%q} %d\n", k.code, k.method, k.route, m.counters[k])
		}
		b.WriteString("# HELP http_request_duration_seconds HTTP 请求时延（秒）。\n# TYPE http_request_duration_seconds histogram\n")
		hkeys := make([]seriesKey, 0, len(m.histo))
		for k := range m.histo {
			hkeys = append(hkeys, k)
		}
		sort.Slice(hkeys, func(i, j int) bool {
			a, b2 := hkeys[i], hkeys[j]
			if a.route != b2.route {
				return a.route < b2.route
			}
			if a.method != b2.method {
				return a.method < b2.method
			}
			return a.code < b2.code
		})
		for _, k := range hkeys {
			h := m.histo[k]
			for i, ub := range latencyBuckets {
				fmt.Fprintf(&b, "http_request_duration_seconds_bucket{code=%q,method=%q,route=%q,le=\"%s\"} %d\n",
					k.code, k.method, k.route, trimFloat(ub), h.buckets[i])
			}
			fmt.Fprintf(&b, "http_request_duration_seconds_bucket{code=%q,method=%q,route=%q,le=\"+Inf\"} %d\n",
				k.code, k.method, k.route, h.count)
			fmt.Fprintf(&b, "http_request_duration_seconds_sum{code=%q,method=%q,route=%q} %s\n",
				k.code, k.method, k.route, trimFloat(h.sum))
			fmt.Fprintf(&b, "http_request_duration_seconds_count{code=%q,method=%q,route=%q} %d\n",
				k.code, k.method, k.route, h.count)
		}
		m.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(b.String()))
	})
}

func trimFloat(f float64) string {
	s := fmt.Sprintf("%g", f)
	return s
}
