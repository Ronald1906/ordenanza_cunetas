package router

import (
	"net/http"
	"os"
	"strings"

	adminrouter "denuncias_cunetas_go/internal/handlers/admin"
	publicrouter "denuncias_cunetas_go/internal/handlers/public"
	"denuncias_cunetas_go/internal/storage"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
)

func New(db *pgxpool.Pool, jwtSecret []byte, cookieSecure bool, nasStorage *storage.NASStorage, localStorage *storage.LocalStorage, allowedOrigins []string) http.Handler {
	r := chi.NewRouter()
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true, MaxAge: 300,
	}))
	r.Use(chimiddleware.RequestID)
	// Se conserva RemoteAddr del socket para auditoría y límite de login.
	// No se confía en X-Forwarded-For de clientes sin un proxy de confianza configurado.
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	name := strings.TrimSpace(os.Getenv("COOKIE_NAME"))
	if name == "" {
		name = "dc_admin"
	}
	r.Route("/api", func(api chi.Router) {
		admin := adminrouter.NewAdminRouter(db, jwtSecret, name, cookieSecure, nasStorage, localStorage)
		api.Mount("/admin", admin.Routes())
		public := publicrouter.NewPublicRouter(db, nasStorage, localStorage)
		api.Mount("/public", public.Routes())
	})
	return r
}
