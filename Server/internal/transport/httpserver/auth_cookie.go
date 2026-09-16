package httpserver

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

func loginStateCookie(c *gin.Context) (string, bool) {
	value, err := c.Cookie(loginCookieName)
	return value, err == nil
}

func sessionToken(c *gin.Context) (string, bool) {
	value, err := c.Cookie(sessionCookieName)
	return value, err == nil
}

func (h *authHandler) setSessionCookies(c *gin.Context, sessionToken, csrfToken string) {
	writeCookie(c, h.sessionCookie(sessionCookieName, sessionToken, true))
	writeCookie(c, h.sessionCookie(csrfCookieName, csrfToken, false))
}

func (h *authHandler) clearSessionCookies(c *gin.Context) {
	writeCookie(c, h.expiredCookie(sessionCookieName, "/", true))
	writeCookie(c, h.expiredCookie(csrfCookieName, "/", false))
}

func (h *authHandler) loginCookie(value string) http.Cookie {
	return http.Cookie{
		Name: loginCookieName, Value: value, Path: "/api/v1/auth/callback",
		MaxAge: int(h.loginStateTTL.Seconds()), HttpOnly: true, Secure: h.cookieSecure,
	}
}

func (h *authHandler) sessionCookie(name, value string, httpOnly bool) http.Cookie {
	return http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: int(h.sessionTTL.Seconds()),
		HttpOnly: httpOnly, Secure: h.cookieSecure,
	}
}

func (h *authHandler) expiredCookie(name, path string, httpOnly bool) http.Cookie {
	return http.Cookie{
		Name: name, Path: path, MaxAge: -1, HttpOnly: httpOnly, Secure: h.cookieSecure,
	}
}

func writeCookie(c *gin.Context, cookie http.Cookie) {
	cookie.SameSite = http.SameSiteLaxMode
	http.SetCookie(c.Writer, &cookie)
}

func sameOrigin(request *http.Request, target string) bool {
	targetURL, err := url.Parse(target)
	if err != nil || targetURL.Scheme == "" || targetURL.Host == "" {
		return false
	}
	source := request.Header.Get("Origin")
	if source == "" {
		source = request.Referer()
	}
	sourceURL, err := url.Parse(source)
	return err == nil && sourceURL.Scheme == targetURL.Scheme && sourceURL.Host == targetURL.Host
}
