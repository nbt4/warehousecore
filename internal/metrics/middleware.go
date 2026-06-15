package metrics

import (
	"net/http"
	"strconv"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Middleware is a gorilla/mux-compatible middleware that tracks HTTP request metrics.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := newResponseWriter(w)

		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		path := r.URL.Path
		method := r.Method
		status := strconv.Itoa(rw.statusCode)

		HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
	})
}
