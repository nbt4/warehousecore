package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"warehousecore/internal/models"
)

func TestEnsureCoresTokenPromotesLocalSession(t *testing.T) {
	t.Setenv("CORES_JWT_SECRET", "test-secret-with-enough-entropy-for-tests")
	t.Setenv("COOKIE_DOMAIN", ".tsunami-events.de")

	request := httptest.NewRequest("GET", "https://warehouse.tsunami-events.de/api/v1/auth/me", nil)
	response := httptest.NewRecorder()
	user := &models.User{UserID: 20, Username: "mschuck", IsAdmin: true}
	ensureCoresToken(response, request, user)

	for _, cookie := range response.Result().Cookies() {
		if cookie.Name != "cores_token" {
			continue
		}
		if strings.TrimPrefix(cookie.Domain, ".") != "tsunami-events.de" {
			t.Fatalf("cookie domain = %q", cookie.Domain)
		}
		if userID, ok := validateCoresToken(cookie.Value); !ok || userID != user.UserID {
			t.Fatalf("promoted token user=%d valid=%v", userID, ok)
		}
		return
	}
	t.Fatal("cores_token cookie was not set")
}
