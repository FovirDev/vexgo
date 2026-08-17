package sso

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Handler exposes the sso domain over HTTP.
type Handler struct {
	svc *Service
}

// NewHandler creates an sso HTTP handler with the given dependencies.
func NewHandler(deps Deps) *Handler {
	return &Handler{svc: NewService(deps)}
}

// SSOProviders returns which SSO providers are currently enabled.
// This is a public endpoint — no authentication required.
//
// GET /api/sso/providers
//
// Response:
//
//	{
//	  "providers": ["github", "google"],   // only enabled ones
//	  "allow_local_login": true
//	}
func (h *Handler) SSOProviders(c *gin.Context) {
	enabled, allowLocalLogin := h.svc.Providers()
	c.JSON(http.StatusOK, gin.H{
		"providers":         enabled,
		"allow_local_login": allowLocalLogin,
	})
}

// SSOLoginRedirect starts the OAuth2 authorization flow.
//
// GET /api/sso/:provider/login?method=sso_get_token|get_sso_id
//
//   - sso_get_token  → full login, issues a JWT on callback
//   - get_sso_id     → only returns the provider-side ID (used to bind SSO
//     to an existing account from the settings page)
func (h *Handler) SSOLoginRedirect(c *gin.Context) {
	provider := c.Param("provider")
	method := c.DefaultQuery("method", "sso_get_token")

	authURL, status, message := h.svc.LoginRedirect(c, provider, method)
	if message != "" {
		c.JSON(status, gin.H{"error": message})
		return
	}

	c.Redirect(http.StatusFound, authURL)
}

// SSOCallback handles the OAuth2 callback for all providers.
// The popup window calls postMessage to pass data back to the opener, then closes.
//
// GET /api/sso/:provider/callback?method=...&code=...&state=...
func (h *Handler) SSOCallback(c *gin.Context) {
	provider := c.Param("provider")
	payload, message := h.svc.Callback(c, provider, c.Query("state"), c.Query("code"))
	if message != "" {
		respondError(c, message)
		return
	}
	respondPostMessage(c, payload)
}

// SSO_STORAGE_KEY must match the constant in the frontend ssoLogin() helper.
const ssoStorageKey = "sso_callback_result"

// respondPostMessage writes the result to localStorage so the opener window
// can pick it up via the 'storage' event. Using localStorage instead of
// postMessage avoids the window.opener=null issue caused by cross-origin
// redirects during the OAuth2 / OIDC flow.
func respondPostMessage(c *gin.Context, data map[string]string) {
	pairs := make([]string, 0, len(data))
	for k, v := range data {
		pairs = append(pairs, fmt.Sprintf(`%q:%q`, k, v))
	}
	payload := "{" + strings.Join(pairs, ",") + "}"
	html := fmt.Sprintf(`<!DOCTYPE html>
<head></head>
<body>
<script>
try { localStorage.setItem(%q, JSON.stringify(%s)) } catch(e) {}
window.close()
</script>
</body>`, ssoStorageKey, payload)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// respondError writes an error result to localStorage and closes the popup.
func respondError(c *gin.Context, msg string) {
	html := fmt.Sprintf(`<!DOCTYPE html>
<head></head>
<body>
<script>
try { localStorage.setItem(%q, JSON.stringify({"error":%q})) } catch(e) {}
window.close()
</script>
</body>`, ssoStorageKey, msg)
	c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(html))
}
