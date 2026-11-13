package auth

import (
	"net/http"
)

// OptionalMiddleware 尽力认证：携带合法 Bearer 时注入上下文用户，
// 否则放行（由各处理器自行兜底校验）。公开端点与受保护端点共用同一 mux 时使用。
func OptionalMiddleware(signer *Signer, load UserLoader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token := bearerToken(r); token != "" {
				if claims, err := signer.Verify(token); err == nil {
					if u, err := load(r.Context(), claims.UserID); err == nil && u != nil {
						r = r.WithContext(WithUser(r.Context(), u))
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
