package handler

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"
)

const sessionCookieName = "br_session"

type contextKey string

const userIDContextKey contextKey = "user_id"

// App holds shared dependencies for HTTP handlers.
type App struct {
	DB *sql.DB
}

// RecoverAndLog wraps an http.Handler with panic recovery and basic request logging.
func RecoverAndLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("PANIC %s %s: %v", r.Method, r.URL.Path, rec)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
		log.Printf("%s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// SessionMiddleware resolves the br_session cookie into context if valid.
func SessionMiddleware(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookieName)
		if err == nil && c.Value != "" {
			var userID string
			var expiresAt int64
			err := db.QueryRowContext(r.Context(),
				"SELECT user_id, expires_at FROM sessions WHERE token = ?",
				c.Value,
			).Scan(&userID, &expiresAt)
			if err == nil && expiresAt > time.Now().Unix() {
				ctx := context.WithValue(r.Context(), userIDContextKey, userID)
				r = r.WithContext(ctx)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// CurrentUserID returns the authenticated user ID from context, or empty string.
func CurrentUserID(r *http.Request) string {
	v, _ := r.Context().Value(userIDContextKey).(string)
	return v
}
