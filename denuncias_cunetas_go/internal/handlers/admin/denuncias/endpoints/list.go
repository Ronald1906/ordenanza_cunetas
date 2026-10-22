package endpoints

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const pageSize int64 = 10

type listFilters struct {
	Page                              int64
	Search, Parroquia, Sector, Barrio string
	EstadoID                          int64
}

func parseFilters(r *http.Request) (listFilters, error) {
	q := r.URL.Query()
	f := listFilters{Page: 1, Search: strings.TrimSpace(q.Get("search")), Parroquia: strings.TrimSpace(q.Get("parroquia")), Sector: strings.ReplaceAll(normalizeText(q.Get("sector")), "-", "_"), Barrio: strings.TrimSpace(q.Get("barrio"))}
	if value := q.Get("page"); value != "" {
		n, err := strconv.ParseInt(value, 10, 32)
		if err != nil || n < 1 {
			return f, fmt.Errorf("page debe ser un entero positivo")
		}
		f.Page = n
	}
	if !validText(f.Search, 200) || !validText(f.Parroquia, 150) || !validText(f.Sector, 30) || !validText(f.Barrio, 200) {
		return f, fmt.Errorf("uno de los filtros supera la longitud permitida o no es válido")
	}
	if value := q.Get("estado_id"); value != "" {
		n, err := strconv.ParseInt(value, 10, 16)
		if err != nil || n <= 0 {
			return f, fmt.Errorf("estado_id no es válido")
		}
		f.EstadoID = n
	}
	return f, nil
}

// Escapa comodines: buscar % o _ busca esos caracteres literalmente.
func containsPattern(value string) string {
	return "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value) + "%"
}
func buildWhere(f listFilters) (string, []any) {
	clauses := []string{"TRUE"}
	args := []any{}
	add := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	if f.Search != "" {
		param := add(containsPattern(f.Search))
		clauses = append(clauses, "(d.numero_denuncia ILIKE "+param+" ESCAPE E'\\\\' OR d.descripcion ILIKE "+param+" ESCAPE E'\\\\')")
	}
	if f.Parroquia != "" {
		clauses = append(clauses, "d.parroquia ILIKE "+add(containsPattern(f.Parroquia))+" ESCAPE E'\\\\'")
	}
	if f.Sector != "" {
		clauses = append(clauses, "d.sector = "+add(f.Sector))
	}
	if f.Barrio != "" {
		clauses = append(clauses, "d.barrio_recinto_comunidad ILIKE "+add(containsPattern(f.Barrio))+" ESCAPE E'\\\\'")
	}
	if f.EstadoID > 0 {
		clauses = append(clauses, "d.estado_id = "+add(f.EstadoID))
	}
	return strings.Join(clauses, " AND "), args
}

// GET /api/admin/gestion/denuncias?page=1&search=basura&parroquia=...&sector=RURAL&barrio=...
func (s *Service) List(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSession(w, r); !ok {
		return
	}
	f, err := parseFilters(r)
	if err != nil {
		fail(w, 400, "FILTROS_NO_VALIDOS", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		dbError(w, "iniciar listado", err)
		return
	}
	defer rollback(tx)
	where, args := buildWhere(f)
	var total int64
	if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM denuncia d WHERE "+where, args...).Scan(&total); err != nil {
		dbError(w, "contar denuncias", err)
		return
	}
	args = append(args, (f.Page-1)*pageSize)
	query := `SELECT d.id,d.numero_denuncia,d.tipo_denuncia,TO_CHAR(d.fecha_denuncia,'YYYY-MM-DD'),d.descripcion,
 d.canton,c.nombre,d.parroquia,d.sector,st.nombre,d.barrio_recinto_comunidad,
 e.id,e.codigo,e.nombre,e.es_final,
 (SELECT COUNT(*) FROM denuncia_evidencia ev WHERE ev.denuncia_id=d.id),d.fecha_creacion,d.fecha_actualizacion
 FROM denuncia d JOIN canton c ON c.codigo=d.canton JOIN sector_tipo st ON st.codigo=d.sector
 JOIN denuncia_estado e ON e.id=d.estado_id WHERE ` + where + fmt.Sprintf(" ORDER BY d.fecha_creacion DESC,d.id DESC LIMIT 10 OFFSET $%d", len(args))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		dbError(w, "listar denuncias", err)
		return
	}
	items := make([]DenunciaResumen, 0, 10)
	for rows.Next() {
		var item DenunciaResumen
		if err := rows.Scan(&item.ID, &item.NumeroDenuncia, &item.TipoDenuncia, &item.FechaDenuncia, &item.Descripcion, &item.Canton.Codigo, &item.Canton.Nombre, &item.Parroquia, &item.Sector.Codigo, &item.Sector.Nombre, &item.Barrio, &item.Estado.ID, &item.Estado.Codigo, &item.Estado.Nombre, &item.Estado.EsFinal, &item.TotalEvidencias, &item.FechaCreacion, &item.FechaActualizacion); err != nil {
			rows.Close()
			dbError(w, "leer denuncia", err)
			return
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		dbError(w, "recorrer denuncias", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		dbError(w, "terminar listado", err)
		return
	}
	pages := (total + pageSize - 1) / pageSize
	jsonResponse(w, 200, map[string]any{"data": items, "paginacion": map[string]any{"pagina": f.Page, "por_pagina": pageSize, "total_registros": total, "total_paginas": pages, "hay_anterior": f.Page > 1, "hay_siguiente": f.Page < pages}})
}
