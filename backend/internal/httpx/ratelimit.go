// 内存令牌桶限流：按「路由键 + 客户端 IP」独立计数，无外部依赖。
package httpx

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	tokens   float64
	lastFill time.Time
}

// RateLimiter 进程内令牌桶集合。容量=limit，按 limit/window 的速率连续补充；
// 首个窗口允许 limit 的突发，符合「登录 10/min、注册 5/h」语义。
type RateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	limit   float64
	window  time.Duration
	lastSweep time.Time
}

// NewRateLimiter limit 个每 window；sweep 惰性清理闲置桶，避免内存增长。
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		buckets:   map[string]*bucket{},
		limit:     float64(limit),
		window:    window,
		lastSweep: time.Now(),
	}
}

// Allow 取一次配额；key 由调用方拼接（通常是 routeKey + IP）。
func (rl *RateLimiter) Allow(key string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	b, ok := rl.buckets[key]
	if !ok {
		if len(rl.buckets) >= 4096 && now.Sub(rl.lastSweep) > 30*time.Second {
			rl.sweepLocked(now)
		}
		b = &bucket{tokens: rl.limit, lastFill: now}
		rl.buckets[key] = b
	}
	rate := rl.limit / rl.window.Seconds()
	b.tokens += now.Sub(b.lastFill).Seconds() * rate
	if b.tokens > rl.limit {
		b.tokens = rl.limit
	}
	b.lastFill = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (rl *RateLimiter) sweepLocked(now time.Time) {
	idle := rl.window * 2
	if idle < time.Hour {
		idle = time.Hour
	}
	for k, b := range rl.buckets {
		if now.Sub(b.lastFill) > idle {
			delete(rl.buckets, k)
		}
	}
	rl.lastSweep = now
}

// Middleware 给 handler 加限流；routeKey 是限流维度键（如 "login"）。
// 超限返回 common/rate_limited。
func (rl *RateLimiter) Middleware(routeKey string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(routeKey + "|" + ClientIP(r)) {
			WriteError(w, nil, &Error{Code: "common/rate_limited", Message: "请求过于频繁，请稍后再试"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ClientIP 取客户端 IP：仅直接对端地址（不信任可伪造的 XFF；反代场景由运维在
// 网络层保证 RemoteAddr 真实性）。解析失败返回整个 RemoteAddr。
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
