package endpoints

import (
	"context"
	"net/http"
	"time"
)

// GET /api/admin/gestion/denuncias/estados
// Permite llenar el selector administrativo sin codificar los IDs en frontend.
func (s *Service) Estados(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSession(w, r); !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	rows, err := s.DB.Query(ctx, `SELECT id,codigo,nombre,es_final FROM denuncia_estado WHERE activo=TRUE ORDER BY orden,id`)
	if err != nil {
		dbError(w, "consultar estados", err)
		return
	}
	defer rows.Close()
	data := make([]Estado, 0)
	for rows.Next() {
		var item Estado
		if err := rows.Scan(&item.ID, &item.Codigo, &item.Nombre, &item.EsFinal); err != nil {
			dbError(w, "leer estado", err)
			return
		}
		data = append(data, item)
	}
	if err := rows.Err(); err != nil {
		dbError(w, "recorrer estados", err)
		return
	}
	jsonResponse(w, 200, map[string]any{"data": data})
}
