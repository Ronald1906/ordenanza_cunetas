package endpoints

import (
	"denuncias_cunetas_go/internal/middleware"
	"net/http"
)

// GET /api/admin/auth/me — permite consultar también una sesión restringida.
func (s *Service) Me(w http.ResponseWriter, r *http.Request) {
	session, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}
	respondJSON(w, 200, map[string]any{"usuario": session.Usuario, "requiere_cambio_password": session.Usuario.RequiereCambioPassword, "csrf_token": middleware.TokenCSRF(session.ID, s.JWTSecret), "expira_en": session.ExpiraEn})
}
