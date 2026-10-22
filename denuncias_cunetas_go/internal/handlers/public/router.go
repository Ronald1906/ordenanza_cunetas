package public

import (
	"net/http"

	"denuncias_cunetas_go/internal/handlers/public/endpoints"
	"denuncias_cunetas_go/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ============================================================
// PUBLIC ROUTER
// ============================================================

type PublicRouter struct {
	db           *pgxpool.Pool
	nasStorage   *storage.NASStorage
	localStorage *storage.LocalStorage
}

// ============================================================
// CONSTRUCTOR
// ============================================================

func NewPublicRouter(
	db *pgxpool.Pool,
	nasStorage *storage.NASStorage,
	localStorage *storage.LocalStorage,
) *PublicRouter {
	return &PublicRouter{
		db:           db,
		nasStorage:   nasStorage,
		localStorage: localStorage,
	}
}

// ============================================================
// RUTAS
// ============================================================

func (p *PublicRouter) Routes() http.Handler {
	r := chi.NewRouter()

	// Catálogos del formulario ciudadano.
	r.Get(
		"/denuncias/catalogos",
		endpoints.ConsultarCatalogosDenuncia(p.db),
	)

	// Registro de una denuncia.
	r.Post(
		"/denuncias",
		endpoints.RegistrarDenuncia(
			p.db,
			p.nasStorage,
			p.localStorage,
		),
	)

	// Consulta pública de seguimiento.
	r.Get(
		"/denuncias/seguimiento/{numero}",
		endpoints.ConsultarSeguimiento(p.db),
	)

	return r
}
