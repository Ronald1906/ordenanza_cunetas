package endpoints

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type updateEstadoRequest struct {
	EstadoID       int16  `json:"estado_id"`
	EstadoActualID *int16 `json:"estado_actual_id,omitempty"`
	Observacion    string `json:"observacion"`
	Respuesta      string `json:"respuesta"`
	EsPublico      *bool  `json:"es_publico,omitempty"`
}

func validateStateInput(input *updateEstadoRequest) error {
	input.Observacion = normalizeText(input.Observacion)
	input.Respuesta = normalizeText(input.Respuesta)
	if input.EstadoID <= 0 {
		return errors.New("Seleccione un estado válido.")
	}
	if input.EstadoActualID != nil && *input.EstadoActualID <= 0 {
		return errors.New("El estado actual no es válido.")
	}
	if !validText(input.Observacion, 2000) {
		return errors.New("La observación admite hasta 2000 caracteres.")
	}
	if !validText(input.Respuesta, 4000) {
		return errors.New("La respuesta admite hasta 4000 caracteres.")
	}
	return nil
}

// PATCH /api/admin/gestion/denuncias/{id}/estado
// Conserva en el historial cada cambio y su respuesta. No reescribe eventos.
func (s *Service) UpdateEstado(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireSession(w, r)
	if !ok {
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		fail(w, 400, "ID_NO_VALIDO", "El identificador de denuncia no es válido.")
		return
	}
	var input updateEstadoRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := validateStateInput(&input); err != nil {
		fail(w, 400, "DATOS_NO_VALIDOS", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		dbError(w, "iniciar cambio de estado", err)
		return
	}
	defer rollback(tx)
	if err := validateActor(ctx, tx, actor); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 403, "SESION_NO_AUTORIZADA", "Su sesión ya no permite modificar denuncias.")
		} else {
			dbError(w, "validar autor", err)
		}
		return
	}
	var anterior Estado
	var number string
	err = tx.QueryRow(ctx, `SELECT d.numero_denuncia,e.id,e.codigo,e.nombre,e.es_final FROM denuncia d JOIN denuncia_estado e ON e.id=d.estado_id WHERE d.id=$1 FOR UPDATE OF d`, id).Scan(&number, &anterior.ID, &anterior.Codigo, &anterior.Nombre, &anterior.EsFinal)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "DENUNCIA_NO_ENCONTRADA", "La denuncia no existe.")
		return
	}
	if err != nil {
		dbError(w, "consultar estado actual", err)
		return
	}
	if input.EstadoActualID != nil && *input.EstadoActualID != anterior.ID {
		fail(w, 409, "ESTADO_MODIFICADO", "Otro usuario modificó el estado. Actualice el detalle antes de continuar.")
		return
	}
	if input.EstadoID == anterior.ID {
		fail(w, 409, "ESTADO_SIN_CAMBIOS", "La denuncia ya tiene ese estado. No se generó otro registro.")
		return
	}
	var nuevo Estado
	err = tx.QueryRow(ctx, `SELECT id,codigo,nombre,es_final FROM denuncia_estado WHERE id=$1 AND activo=TRUE FOR SHARE`, input.EstadoID).Scan(&nuevo.ID, &nuevo.Codigo, &nuevo.Nombre, &nuevo.EsFinal)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 400, "ESTADO_NO_DISPONIBLE", "El estado no existe o está inactivo.")
		return
	}
	if err != nil {
		dbError(w, "consultar nuevo estado", err)
		return
	}
	if nuevo.EsFinal && input.Respuesta == "" {
		fail(w, 400, "RESPUESTA_REQUERIDA", "Debe registrar una respuesta al ciudadano para finalizar la denuncia.")
		return
	}
	// Una respuesta es siempre pública; la observación permanece como nota interna
	// cuando se proporciona respuesta. El seguimiento público filtra esa nota.
	publico := input.EsPublico != nil && *input.EsPublico
	if input.Respuesta != "" {
		publico = true
	}
	var updated time.Time
	if err := tx.QueryRow(ctx, `UPDATE denuncia SET estado_id=$2 WHERE id=$1 RETURNING fecha_actualizacion`, id, nuevo.ID).Scan(&updated); err != nil {
		dbError(w, "actualizar estado", err)
		return
	}
	var historyID int64
	var changedAt time.Time
	err = tx.QueryRow(ctx, `INSERT INTO denuncia_historial_estado(denuncia_id,estado_anterior_id,estado_nuevo_id,usuario_id,observacion,respuesta,es_publico,fecha_cambio)
 VALUES($1,$2,$3,$4,$5,$6,$7,clock_timestamp()) RETURNING id,fecha_cambio`, id, anterior.ID, nuevo.ID, actor.Usuario.ID, nullable(input.Observacion), nullable(input.Respuesta), publico).Scan(&historyID, &changedAt)
	if err != nil {
		dbError(w, "guardar historial", err)
		return
	}
	if err := auditState(ctx, tx, r, actor, id, anterior.Codigo, nuevo.Codigo); err != nil {
		dbError(w, "auditar cambio de estado", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		dbError(w, "confirmar cambio de estado", err)
		return
	}
	jsonResponse(w, 200, map[string]any{"mensaje": "Estado actualizado correctamente.", "id": id, "numero_denuncia": number, "estado_anterior": anterior, "estado": nuevo, "fecha_actualizacion": updated, "historial": map[string]any{"id": historyID, "fecha": changedAt, "observacion": nullable(input.Observacion), "respuesta": nullable(input.Respuesta), "es_publico": publico, "usuario_id": actor.Usuario.ID}})
}
