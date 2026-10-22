package endpoints

import (
	"context"
	"net/http"
	"time"
)

// POST /api/admin/auth/logout — revoca la sesión en SQL antes de borrar cookie.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internalError(w, "iniciar cierre", err)
		return
	}
	defer rollback(tx)
	// Mismo orden de bloqueo que login y cambio de contraseña: usuario, sesión.
	var lockedID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM usuario WHERE id=$1 FOR UPDATE`, session.Usuario.ID).Scan(&lockedID); err != nil {
		internalError(w, "bloquear usuario al cerrar sesión", err)
		return
	}
	tag, err := tx.Exec(ctx, `UPDATE usuario_sesion SET revocada_en=NOW() WHERE id=$1 AND usuario_id=$2 AND revocada_en IS NULL`, session.ID, session.Usuario.ID)
	if err != nil {
		internalError(w, "revocar sesión", err)
		return
	}
	if tag.RowsAffected() > 0 {
		if err := audit(ctx, tx, r, session.Usuario.ID, session.Usuario.Usuario, "LOGOUT", "EXITO", "USUARIO", session.Usuario.ID, "CIERRE DE SESIÓN ADMINISTRATIVA."); err != nil {
			internalError(w, "auditar cierre", err)
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w, "confirmar cierre", err)
		return
	}
	s.clearCookie(w)
	respondJSON(w, 200, map[string]string{"mensaje": "Sesión cerrada correctamente."})
}
