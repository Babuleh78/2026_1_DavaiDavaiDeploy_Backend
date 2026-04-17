// package cors

// import (
// 	"net/http"
// )

// func CorsMiddleware(next http.Handler) http.Handler {

// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		w.Header().Set("Access-Control-Allow-Methods", "POST,GET,OPTIONS")
// 		w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Csrf-Token")
// 		w.Header().Set("Access-Control-Allow-Credentials", "true")
// 		w.Header().Set("Access-Control-Expose-Headers", "Authorization,X-Csrf-Token")
// 		w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
// 		w.Header().Set("Access-Control-Max-Age", "86400")
// 		if r.Method == http.MethodOptions {
// 			w.WriteHeader(http.StatusOK)
// 			return
// 		}
// 		next.ServeHTTP(w, r)
// 	})
// }

package cors

import (
	"net/http"
	"strings"
)

func CorsMiddleware(next http.Handler) http.Handler {
    allowedOrigins := []string{
        "http://localhost",
        "http://127.0.0.1",
        "http://host.docker.internal", // Mac
    }

    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        origin := r.Header.Get("Origin")
        
        allowed := false
        for _, allowedOrigin := range allowedOrigins {
            if strings.HasPrefix(origin, allowedOrigin) {
                allowed = true
                break
            }
        }

        if allowed {
            w.Header().Set("Access-Control-Allow-Origin", origin)
            w.Header().Set("Vary", "Origin")
        }

        w.Header().Set("Access-Control-Allow-Methods", "POST,GET,OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Csrf-Token")
        w.Header().Set("Access-Control-Allow-Credentials", "true")
        w.Header().Set("Access-Control-Expose-Headers", "Authorization,X-Csrf-Token")
        w.Header().Set("Access-Control-Max-Age", "86400")

        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusOK)
            return
        }
        next.ServeHTTP(w, r)
    })
}