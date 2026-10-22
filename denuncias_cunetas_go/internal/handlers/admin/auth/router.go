package auth

import (
	"net/http"
	"os"
	"strings"

	"denuncias_cunetas_go/internal/handlers/admin/auth/endpoints"
	"denuncias_cunetas_go/internal/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AuthRouter struct {
	service    *endpoints.Service
	db         *pgxpool.Pool
	secret     []byte
	cookieName string
}

func NewAuthRouter(db *pgxpool.Pool, secret []byte, cookieName string, cookieSecure bool) *AuthRouter {
	return &AuthRouter{service: endpoints.NewService(db, secret, cookieName, cookieSecure), db: db, secret: secret, cookieName: cookieName}
}
func (a *AuthRouter) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.VerificarOrigen(strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",")))
	r.With(middleware.LimitarLogin()).Post("/login", a.service.Login)
	r.Group(func(private chi.Router) {
		private.Use(middleware.RequerirSesion(a.db, a.secret, a.cookieName))
		private.Use(middleware.RequerirCSRF(a.secret))
		private.Get("/me", a.service.Me)
		private.Post("/logout", a.service.Logout)
		private.Post("/cambiar-password", a.service.CambiarPassword)
		private.Group(func(full chi.Router) {
			full.Use(middleware.RequerirPasswordActualizada)
			full.Post("/register", a.service.Register)
			full.Post("/usuarios/{id}/restablecer-password", a.service.RestablecerPassword)
		})
	})
	return r
}
