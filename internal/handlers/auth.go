package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"warehousecore/internal/middleware"
	"warehousecore/internal/models"
	"warehousecore/internal/repository"
	"warehousecore/internal/services"

	"golang.org/x/crypto/bcrypt"
)

// LoginRequest represents login credentials
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents login response
type LoginResponse struct {
	Success             bool         `json:"success"`
	Message             string       `json:"message"`
	User                *models.User `json:"user,omitempty"`
	ForcePasswordChange bool         `json:"force_password_change,omitempty"`
}

// Login handles user authentication
func Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Invalid request format",
		})
		return
	}

	// Find user
	db := repository.GetDB()
	var user models.User
	err := db.Where("username = ? AND is_active = ?", req.Username, true).First(&user).Error
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Invalid username or password",
		})
		return
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Invalid username or password",
		})
		return
	}

	// Create session
	sessionID := generateSessionID()
	session := models.Session{
		SessionID: sessionID,
		UserID:    user.UserID,
		ExpiresAt: time.Now().Add(24 * time.Hour), // 24 hours
		CreatedAt: time.Now(),
	}

	if err := db.Create(&session).Error; err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(LoginResponse{
			Success: false,
			Message: "Failed to create session",
		})
		return
	}

	// Update last login
	now := time.Now()
	user.LastLogin = &now
	db.Save(&user)

	// Set cookie
	cookieDomain := getCookieDomain(r)
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		Domain:   cookieDomain,
		MaxAge:   86400, // 24 hours
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})

	// Return success (without password hash)
	user.PasswordHash = ""
	rbacService := services.NewRBACService()
	if roles, err := rbacService.GetUserRoles(user.UserID); err == nil {
		user.Roles = roles
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		Success:             true,
		Message:             "Login successful",
		User:                &user,
		ForcePasswordChange: user.ForcePasswordChange,
	})
}

// Logout handles user logout
func Logout(w http.ResponseWriter, r *http.Request) {
	// Get session cookie
	cookie, err := r.Cookie("session_id")
	if err == nil && cookie.Value != "" {
		// Delete session from database
		db := repository.GetDB()
		db.Where("session_id = ?", cookie.Value).Delete(&models.Session{})
	}

	// Clear cookie
	cookieDomain := getCookieDomain(r)
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		Domain:   cookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "cores_token",
		Value:    "",
		Path:     "/",
		Domain:   cookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cookieDomain != "",
		SameSite: http.SameSiteLaxMode,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Logged out successfully",
	})
}

// GetCurrentUser returns the currently authenticated user
func GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Not authenticated",
		})
		return
	}

	// Return user (without password hash)
	user.PasswordHash = ""
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// generateSessionID creates a new session ID
func generateSessionID() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// getCookieDomain determines the cookie domain based on environment and request
func getCookieDomain(r *http.Request) string {
	// Check if COOKIE_DOMAIN is set in environment
	if domain := os.Getenv("COOKIE_DOMAIN"); domain != "" {
		return domain
	}

	// Auto-detect from request host
	host := r.Host
	// Remove port if present
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// If host is a subdomain (e.g., storage.server-nt.de), use parent domain
	parts := strings.Split(host, ".")
	if len(parts) >= 3 {
		// Return .server-nt.de for storage.server-nt.de
		return "." + strings.Join(parts[len(parts)-2:], ".")
	}

	// For localhost or simple domains, don't set domain (browser default)
	if host == "localhost" || host == "127.0.0.1" {
		return ""
	}

	return host
}

// ChangePasswordRequest represents password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangePassword handles password change requests
func ChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.GetUserFromContext(r)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Not authenticated",
		})
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Invalid request format",
		})
		return
	}

	// Validate new password
	if len(req.NewPassword) < 6 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Password must be at least 6 characters",
		})
		return
	}

	// Verify current password
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Current password is incorrect",
		})
		return
	}

	// Hash new password
	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Failed to process new password",
		})
		return
	}

	// Update password and clear force_password_change flag
	db := repository.GetDB()
	err = db.Model(&models.User{}).Where("userid = ?", user.UserID).Updates(map[string]interface{}{
		"password_hash":         string(newHash),
		"force_password_change": false,
		"updated_at":            time.Now(),
	}).Error

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"message": "Failed to update password",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Password changed successfully",
	})
}
