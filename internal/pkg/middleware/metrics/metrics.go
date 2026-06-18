package metrics

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	appmetrics "DDDance/internal/pkg/metrics"
)

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("ResponseWriter does not implement http.Hijacker")
}

func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		path := routePath(r)
		duration := time.Since(start).Seconds()
		statusCode := fmt.Sprintf("%d", rw.status)

		appmetrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, statusCode).Inc()
		appmetrics.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}

func routePath(r *http.Request) string {
	if route := mux.CurrentRoute(r); route != nil {
		if tmpl, err := route.GetPathTemplate(); err == nil {
			return tmpl
		}
	}
	return r.URL.Path
}
