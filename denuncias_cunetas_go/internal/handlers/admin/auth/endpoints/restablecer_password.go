package endpoints

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

type restablecerPasswordRequest struct {
	PasswordTemporal string `json:"password_temporal"`
}

// POST /api/admin/auth/usuarios/{id}/restablecer-password
// Los usuarios actuales son administradores con el mismo nivel de acceso.
func (s *Service) RestablecerPassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "ID_NO_VALIDO", "El identificador no es válido.")
		return
	}
	var input restablecerPasswordRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := validatePassword(input.PasswordTemporal); err != nil {
		fail(w, 400, "PASSWORD_NO_VALIDO", err.Error())
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(input.PasswordTemporal), bcrypt.DefaultCost)
	if err != nil {
		internalError(w, "generar contraseña temporal", err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internalError(w, "iniciar restablecimiento", err)
		return
	}
	defer rollback(tx)
	// Bloqueo exclusivo por ID ordenado: evita que dos administradores que se
	// restablecen entre sí tomen los bloqueos de usuario en orden inverso.
	var actorFound, targetFound bool
	first, second := actor.Usuario.ID, id
	if first > second {
		first, second = second, first
	}
	for _, userID := range []int64{first, second} {
		if userID == second && first == second && actorFound {
			continue
		}
		var foundID int64
		err := tx.QueryRow(ctx, `SELECT id FROM usuario WHERE id=$1 AND activo=TRUE FOR UPDATE`, userID).Scan(&foundID)
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 404, "USUARIO_NO_DISPONIBLE", "El usuario no existe o está inactivo.")
			return
		}
		if err != nil {
			internalError(w, "bloquear usuario", err)
			return
		}
		if foundID == actor.Usuario.ID {
			actorFound = true
		}
		if foundID == id {
			targetFound = true
		}
	}
	if !actorFound || !targetFound {
		fail(w, 404, "USUARIO_NO_DISPONIBLE", "El usuario no está disponible.")
		return
	}
	if err := validateActor(ctx, tx, actor); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 403, "SESION_NO_AUTORIZADA", "Su sesión ya no permite esta operación.")
		} else {
			internalError(w, "validar autor", err)
		}
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE usuario SET password_hash=$2,requiere_cambio_password=TRUE WHERE id=$1`, id, string(newHash)); err != nil {
		internalError(w, "restablecer contraseña", err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE usuario_sesion SET revocada_en=NOW() WHERE usuario_id=$1 AND revocada_en IS NULL`, id); err != nil {
		internalError(w, "revocar sesiones", err)
		return
	}
	if err := audit(ctx, tx, r, actor.Usuario.ID, actor.Usuario.Usuario, "RESTABLECIMIENTO_PASSWORD", "EXITO", "USUARIO", id, "CONTRASEÑA TEMPORAL ESTABLECIDA. SE EXIGE CAMBIO EN EL PRÓXIMO ACCESO."); err != nil {
		internalError(w, "auditar restablecimiento", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w, "confirmar restablecimiento", err)
		return
	}
	own := id == actor.Usuario.ID
	if own {
		s.clearCookie(w)
	}
	respondJSON(w, 200, map[string]any{"mensaje": "Contraseña temporal establecida correctamente.", "requiere_cambio_password": true, "requiere_login": own})
}
