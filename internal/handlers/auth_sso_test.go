package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	commonjwt "github.com/nbt4/cores-common/pkg/jwt"
	"warehousecore/internal/models"
)

func TestSetCoresTokenCreatesSharedDomainCookie(t *testing.T) {
	t.Setenv("CORES_JWT_SECRET", "test-secret-with-enough-entropy-for-tests")
	t.Setenv("COOKIE_DOMAIN", ".tsunami-events.de")

	request := httptest.NewRequest("POST", "https://warehouse.tsunami-events.de/api/v1/auth/login", nil)
	response := httptest.NewRecorder()
	user := &models.User{UserID: 20, Username: "mschuck", IsAdmin: true}

	if err := setCoresToken(response, request, user, 3600); err != nil {
		t.Fatalf("setCoresToken returned error: %v", err)
	}

	var tokenCookieFound bool
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name != "cores_token" {
			continue
		}
		tokenCookieFound = true
		if strings.TrimPrefix(cookie.Domain, ".") != "tsunami-events.de" {
			t.Fatalf("cookie domain = %q, want .tsunami-events.de", cookie.Domain)
		}
		if !cookie.HttpOnly || !cookie.Secure {
			t.Fatalf("shared cookie must be HttpOnly and Secure: %+v", cookie)
		}
		claims, ok := commonjwt.ValidateToken(cookie.Value)
		if !ok || claims.UserID != user.UserID || !claims.IsAdmin {
			t.Fatalf("unexpected token claims: %+v, valid=%v", claims, ok)
		}
	}
	if !tokenCookieFound {
		t.Fatal("cores_token cookie was not set")
	}
}
