package middleware

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type loginWindow struct {
	count int
	until time.Time
}

// Límite por IP en una instancia: 20 intentos cada 5 minutos.
// No confía directamente en X-Forwarded-For.
func LimitarLogin() func(http.Handler) http.Handler {
	var mu sync.Mutex
	windows := map[string]loginWindow{}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				ip = r.RemoteAddr
			}
			now := time.Now()
			mu.Lock()
			current, exists := windows[ip]
			if !exists || !now.Before(current.until) {
				if len(windows) >= 10000 {
					for key, value := range windows {
						if !now.Before(value.until) {
							delete(windows, key)
						}
					}
				}
				if !exists && len(windows) >= 10000 {
					mu.Unlock()
					w.Header().Set("Retry-After", "300")
					ResponderErrorSesion(w, 429, "DEMASIADOS_INTENTOS", "Espere antes de volver a intentar.")
					return
				}
				current = loginWindow{until: now.Add(5 * time.Minute)}
			}
			current.count++
			windows[ip] = current
			blocked := current.count > 20
			mu.Unlock()
			if blocked {
				w.Header().Set("Retry-After", "300")
				ResponderErrorSesion(w, 429, "DEMASIADOS_INTENTOS", "Demasiados intentos. Espere cinco minutos.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
