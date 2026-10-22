package endpoints

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type cambiarPasswordRequest struct {
	PasswordActual string `json:"password_actual"`
	PasswordNueva  string `json:"password_nueva"`
	Confirmacion   string `json:"confirmacion_password"`
}

// POST /api/admin/auth/cambiar-password
// Se permite aunque requiera_cambio_password esté activo.
func (s *Service) CambiarPassword(w http.ResponseWriter, r *http.Request) {
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}
	var input cambiarPasswordRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if !validateCurrentPassword(input.PasswordActual) {
		fail(w, 400, "PASSWORD_ACTUAL_REQUERIDO", "Ingrese la contraseña actual.")
		return
	}
	if err := validatePassword(input.PasswordNueva); err != nil {
		fail(w, 400, "PASSWORD_NO_VALIDO", err.Error())
		return
	}
	if input.PasswordNueva != input.Confirmacion {
		fail(w, 400, "PASSWORD_NO_COINCIDE", "La confirmación no coincide con la nueva contraseña.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internalError(w, "iniciar cambio de contraseña", err)
		return
	}
	defer rollback(tx)
	var hash string
	err = tx.QueryRow(ctx, `SELECT u.password_hash FROM usuario u JOIN usuario_sesion s ON s.usuario_id=u.id
  WHERE u.id=$1 AND u.activo=TRUE AND s.id=$2 AND s.revocada_en IS NULL AND s.expira_en>NOW() FOR UPDATE OF u`, session.Usuario.ID, session.ID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 401, "SESION_INVALIDA", "La sesión ya no es válida.")
		return
	}
	if err != nil {
		internalError(w, "consultar contraseña", err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.PasswordActual)) != nil {
		fail(w, 400, "PASSWORD_ACTUAL_INCORRECTO", "La contraseña actual es incorrecta.")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.PasswordNueva)) == nil {
		fail(w, 400, "PASSWORD_REPETIDO", "La nueva contraseña debe ser diferente de la actual.")
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(input.PasswordNueva), bcrypt.DefaultCost)
	if err != nil {
		internalError(w, "generar nueva contraseña", err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE usuario SET password_hash=$2,requiere_cambio_password=FALSE WHERE id=$1`, session.Usuario.ID, string(newHash)); err != nil {
		internalError(w, "guardar contraseña", err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE usuario_sesion SET revocada_en=NOW() WHERE usuario_id=$1 AND revocada_en IS NULL`, session.Usuario.ID); err != nil {
		internalError(w, "revocar sesiones anteriores", err)
		return
	}
	if err := audit(ctx, tx, r, session.Usuario.ID, session.Usuario.Usuario, "CAMBIO_PASSWORD", "EXITO", "USUARIO", session.Usuario.ID, "CONTRASEÑA ACTUALIZADA POR SU TITULAR. SESIONES ANTERIORES REVOCADAS."); err != nil {
		internalError(w, "auditar cambio de contraseña", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w, "confirmar contraseña", err)
		return
	}
	s.clearCookie(w)
	respondJSON(w, 200, map[string]any{"mensaje": "Contraseña actualizada. Inicie sesión con su nueva contraseña.", "requiere_login": true, "requiere_cambio_password": false})
}
