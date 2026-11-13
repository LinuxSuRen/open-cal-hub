// GET /api/v1/meta：公开元信息（契约 §1）。
package user

import (
	"net/http"

	"github.com/linuxsuren/open-cal-hub/backend/internal/httpx"
	"github.com/linuxsuren/open-cal-hub/backend/internal/version"
)

func (h *handler) meta(w http.ResponseWriter, r *http.Request) {
	mode, err := h.a.EffectiveRegistrationMode(r.Context())
	if err != nil {
		h.a.Log.Error("read_setting_failed", "key", "registration_mode", "error", err)
		mode = "open"
	}
	httpx.WriteData(w, http.StatusOK, map[string]any{
		"site_name":         h.siteName(),
		"version":           version.Version,
		"registration_mode": mode,
		"features":          map[string]bool{"booking": true},
		"default_locale":    "zh-CN",
	})
}
