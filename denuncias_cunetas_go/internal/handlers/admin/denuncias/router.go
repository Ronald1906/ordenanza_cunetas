package denuncias

import (
	"net/http"

	"denuncias_cunetas_go/internal/handlers/admin/denuncias/endpoints"
	"denuncias_cunetas_go/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DenunciasRouter struct{ service *endpoints.Service }

func NewDenunciasRouter(db *pgxpool.Pool, nas *storage.NASStorage, local *storage.LocalStorage) *DenunciasRouter {
	return &DenunciasRouter{service: endpoints.NewService(db, nas, local)}
}
func (d *DenunciasRouter) Routes() http.Handler {
	r := chi.NewRouter()
	// El router padre /admin/gestion exige sesión, contraseña actualizada y CSRF.
	r.Get("/", d.service.List)
	r.Get("/estados", d.service.Estados)
	r.Get("/{id}", d.service.DetalleDenuncia)
	r.Patch("/{id}/estado", d.service.UpdateEstado)
	r.Get("/{id}/evidencias/{evidencia_id}", d.service.Evidencia)
	r.Head("/{id}/evidencias/{evidencia_id}", d.service.Evidencia)
	return r
}
