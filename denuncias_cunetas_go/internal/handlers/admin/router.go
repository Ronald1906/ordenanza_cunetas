package admin

import (
	"net/http"
	"os"
	"strings"

	authrouter "denuncias_cunetas_go/internal/handlers/admin/auth"
	denunciasrouter "denuncias_cunetas_go/internal/handlers/admin/denuncias"
	"denuncias_cunetas_go/internal/middleware"
	"denuncias_cunetas_go/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AdminRouter struct {
	db           *pgxpool.Pool
	jwtSecret    []byte
	cookieName   string
	cookieSecure bool
	nasStorage   *storage.NASStorage
	localStorage *storage.LocalStorage
}

func NewAdminRouter(db *pgxpool.Pool, jwtSecret []byte, cookieName string, cookieSecure bool, nasStorage *storage.NASStorage, localStorage *storage.LocalStorage) *AdminRouter {
	return &AdminRouter{db: db, jwtSecret: jwtSecret, cookieName: cookieName, cookieSecure: cookieSecure, nasStorage: nasStorage, localStorage: localStorage}
}
func (a *AdminRouter) Routes() http.Handler {
	r := chi.NewRouter()
	auth := authrouter.NewAuthRouter(a.db, a.jwtSecret, a.cookieName, a.cookieSecure)
	r.Mount("/auth", auth.Routes())
	r.Route("/gestion", func(gestion chi.Router) {
		gestion.Use(middleware.VerificarOrigen(strings.Split(os.Getenv("CORS_ALLOWED_ORIGINS"), ",")))
		gestion.Use(middleware.RequerirSesion(a.db, a.jwtSecret, a.cookieName))
		gestion.Use(middleware.RequerirPasswordActualizada)
		gestion.Use(middleware.RequerirCSRF(a.jwtSecret))
		denuncias := denunciasrouter.NewDenunciasRouter(a.db, a.nasStorage, a.localStorage)
		gestion.Mount("/denuncias", denuncias.Routes())
	})
	return r
}
