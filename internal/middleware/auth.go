package middleware

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"time"

	commonjwt "github.com/nbt4/cores-common/pkg/jwt"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"
)

type contextKey string

const UserContextKey = contextKey("user")

const authDebugLogsEnabled = false

func authDebugLog(format string, args ...interface{}) {
	if !authDebugLogsEnabled {
		return
	}
	log.Printf(format, args...)
}

// validateCoresToken uses the shared cores-common JWT validator.
func validateCoresToken(tokenStr string) (uint, bool) {
	claims, ok := commonjwt.ValidateToken(tokenStr)
	if !ok {
		return 0, false
	}
	return claims.UserID, true
}

// AuthMiddleware validates session and loads user
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Debug: Log all cookies
		authDebugLog("DEBUG [WarehouseCore]: AuthMiddleware - Path: %s, Cookies: %+v", r.URL.Path, r.Cookies())

		db := repository.GetDB()
		if db == nil {
			http.Error(w, `{"error":"Database unavailable"}`, http.StatusInternalServerError)
			return
		}

		// 1. Try session_id cookie (existing auth)
		if cookie, err := r.Cookie("session_id"); err == nil && cookie.Value != "" {
			sessionID, decodeErr := url.QueryUnescape(cookie.Value)
			if decodeErr == nil {
				authDebugLog("DEBUG [WarehouseCore]: Found session_id cookie: %s (decoded: %s) for path: %s", cookie.Value, sessionID, r.URL.Path)
				var session models.Session
				err = db.Preload("User").
					Where("session_id = ? AND expires_at > ?", sessionID, time.Now()).
					First(&session).Error
				if err == nil && session.User.IsActive {
					authDebugLog("DEBUG [WarehouseCore]: Session valid for user: %s (ID: %d)", session.User.Username, session.User.UserID)
					rbacService := services.NewRBACService()
					if roles, roleErr := rbacService.GetUserRoles(session.User.UserID); roleErr == nil {
						session.User.Roles = roles
					} else {
						authDebugLog("DEBUG [WarehouseCore]: Failed to load roles for user %d: %v", session.User.UserID, roleErr)
					}
					ctx := context.WithValue(r.Context(), UserContextKey, &session.User)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				authDebugLog("DEBUG [WarehouseCore]: Session validation failed for %s: %v", sessionID, err)
			} else {
				authDebugLog("DEBUG [WarehouseCore]: Failed to decode cookie for %s: %v", r.URL.Path, decodeErr)
			}
		} else {
			authDebugLog("DEBUG [WarehouseCore]: No session_id cookie found for %s (error: %v)", r.URL.Path, err)
		}

		// 2. Try cores_token JWT cookie (SSO from cores-dashboard)
		if coresCookie, err := r.Cookie("cores_token"); err == nil && coresCookie.Value != "" {
			if userID, ok := validateCoresToken(coresCookie.Value); ok {
				var user models.User
				err := db.Where("userID = ? AND is_active = ?", userID, true).First(&user).Error
				if err == nil {
					rbacService := services.NewRBACService()
					if roles, roleErr := rbacService.GetUserRoles(user.UserID); roleErr == nil {
						user.Roles = roles
					}
					ctx := context.WithValue(r.Context(), UserContextKey, &user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				authDebugLog("DEBUG [WarehouseCore]: cores_token user lookup failed for userID %d: %v", userID, err)
			}
		}

		// 3. Unauthorized
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
	})
}

// OptionalAuthMiddleware loads user if session exists, but doesn't require it
func OptionalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get session cookie
		cookie, err := r.Cookie("session_id")
		if err != nil || cookie.Value == "" {
			// No session - continue without user
			next.ServeHTTP(w, r)
			return
		}

		// URL-decode the cookie value
		sessionID, err := url.QueryUnescape(cookie.Value)
		if err != nil {
			// Failed to decode - continue without user
			next.ServeHTTP(w, r)
			return
		}

		// Try to validate session
		db := repository.GetDB()
		if db != nil {
			var session models.Session
			err = db.Preload("User").
				Where("session_id = ? AND expires_at > ?", sessionID, time.Now()).
				First(&session).Error

			if err == nil && session.User.IsActive {
				// Load roles for optional contexts too
				rbacService := services.NewRBACService()
				if roles, roleErr := rbacService.GetUserRoles(session.User.UserID); roleErr == nil {
					session.User.Roles = roles
				}
				// Valid session - add user to context
				ctx := context.WithValue(r.Context(), UserContextKey, &session.User)
				r = r.WithContext(ctx)
			}
		}

		next.ServeHTTP(w, r)
	})
}

// GetUserFromContext retrieves the user from request context
func GetUserFromContext(r *http.Request) (*models.User, bool) {
	user, ok := r.Context().Value(UserContextKey).(*models.User)
	return user, ok
}
