package endpoints

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type OpcionCatalogoDenuncia struct {
	ID                      int16   `json:"id"`
	Codigo                  string  `json:"codigo"`
	Nombre                  string  `json:"nombre"`
	Descripcion             *string `json:"descripcion,omitempty"`
	RequiereDatosPersonales *bool   `json:"requiere_datos_personales,omitempty"`
}

func ConsultarCatalogosDenuncia(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			responderError(w, 503, "El servicio no está disponible.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := db.Query(ctx, `SELECT catalogo,id,codigo,nombre,descripcion,requiere_datos_personales FROM (
   SELECT 'tipo-denuncia'::TEXT catalogo,id,codigo,nombre,descripcion::TEXT descripcion,requiere_datos_personales,orden FROM denuncia_tipo WHERE activo=TRUE
   UNION ALL SELECT 'tipo-denunciante',id,codigo,nombre,descripcion::TEXT,NULL::BOOLEAN,orden FROM denunciante_tipo WHERE activo=TRUE
   UNION ALL SELECT 'canton',id,codigo,nombre,NULL::TEXT,NULL::BOOLEAN,orden FROM canton WHERE activo=TRUE
   UNION ALL SELECT 'sector',id,codigo,nombre,NULL::TEXT,NULL::BOOLEAN,orden FROM sector_tipo WHERE activo=TRUE
   UNION ALL SELECT 'problema-reportado',id,codigo,nombre,descripcion::TEXT,NULL::BOOLEAN,orden FROM problema_tipo WHERE activo=TRUE
  ) opciones ORDER BY catalogo,orden,id`)
		if err != nil {
			log.Printf("[CATALOGOS] %v", err)
			responderError(w, 500, "No fue posible consultar los catálogos.")
			return
		}
		defer rows.Close()
		catalogs := map[string][]OpcionCatalogoDenuncia{}
		for _, key := range []string{"tipo-denuncia", "tipo-denunciante", "canton", "sector", "problema-reportado"} {
			catalogs[key] = make([]OpcionCatalogoDenuncia, 0)
		}
		for rows.Next() {
			var key string
			var item OpcionCatalogoDenuncia
			if err := rows.Scan(&key, &item.ID, &item.Codigo, &item.Nombre, &item.Descripcion, &item.RequiereDatosPersonales); err != nil {
				log.Printf("[CATALOGOS] %v", err)
				responderError(w, 500, "No fue posible consultar los catálogos.")
				return
			}
			catalogs[key] = append(catalogs[key], item)
		}
		if err := rows.Err(); err != nil {
			log.Printf("[CATALOGOS] %v", err)
			responderError(w, 500, "No fue posible consultar los catálogos.")
			return
		}
		responderJSON(w, 200, map[string]any{"catalogos": catalogs})
	}
}
