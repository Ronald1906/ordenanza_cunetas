package endpoints

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"denuncias_cunetas_go/internal/middleware"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Usuario  string `json:"usuario"`
	Password string `json:"password"`
}

// POST /api/admin/auth/login — público, con JSON y origen permitido.
func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	if !s.configured() {
		fail(w, 503, "SERVICIO_NO_DISPONIBLE", "Revise JWT_SECRET y la configuración de sesión.")
		return
	}
	var input loginRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Usuario = strings.TrimSpace(input.Usuario)
	if (input.Usuario == "" || utf8.RuneCountInString(input.Usuario) > 50 || !utf8.ValidString(input.Usuario) || strings.ContainsRune(input.Usuario, 0)) || !validateCurrentPassword(input.Password) {
		fail(w, 400, "DATOS_NO_VALIDOS", "Ingrese un usuario válido y su contraseña.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internalError(w, "iniciar login", err)
		return
	}
	defer rollback(tx)
	var user middleware.UsuarioSesion
	var nombres pgtype.Text
	var hash string
	var active bool
	err = tx.QueryRow(ctx, `SELECT id,usuario,nombres,password_hash,activo,requiere_cambio_password FROM usuario WHERE usuario=$1 FOR UPDATE`, input.Usuario).Scan(&user.ID, &user.Usuario, &nombres, &hash, &active, &user.RequiereCambioPassword)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(input.Password))
		rollback(tx)
		s.failedLogin(r, input.Usuario)
		fail(w, 401, "CREDENCIALES_INVALIDAS", "Usuario o contraseña incorrectos.")
		return
	}
	if err != nil {
		internalError(w, "consultar usuario", err)
		return
	}
	if nombres.Valid {
		value := nombres.String
		user.Nombres = &value
	}
	passwordError := bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password))
	if passwordError != nil || !active {
		rollback(tx)
		s.failedLogin(r, input.Usuario)
		fail(w, 401, "CREDENCIALES_INVALIDAS", "Usuario o contraseña incorrectos.")
		return
	}
	sessionID, err := newSessionID()
	if err != nil {
		internalError(w, "generar sesión", err)
		return
	}
	now := time.Now().UTC().Truncate(time.Second)
	expires := now.Add(SessionDuration)
	claims := middleware.Claims{UsuarioID: user.ID, Usuario: user.Usuario, RegisteredClaims: jwt.RegisteredClaims{
		Issuer: middleware.JWTIssuer, Subject: strconv.FormatInt(user.ID, 10), Audience: jwt.ClaimStrings{middleware.JWTAudience}, ID: sessionID,
		IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires),
	}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.JWTSecret)
	if err != nil {
		internalError(w, "firmar sesión", err)
		return
	}
	if _, err := tx.Exec(ctx, `INSERT INTO usuario_sesion(id,usuario_id,expira_en) VALUES($1,$2,$3)`, sessionID, user.ID, expires); err != nil {
		internalError(w, "guardar sesión", err)
		return
	}
	if _, err := tx.Exec(ctx, `UPDATE usuario SET ultimo_acceso=NOW() WHERE id=$1`, user.ID); err != nil {
		internalError(w, "actualizar acceso", err)
		return
	}
	if err := audit(ctx, tx, r, user.ID, user.Usuario, "LOGIN", "EXITO", "USUARIO", user.ID, "INICIO DE SESIÓN ADMINISTRATIVA."); err != nil {
		internalError(w, "auditar login", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w, "confirmar login", err)
		return
	}
	s.setCookie(w, token, expires)
	respondJSON(w, 200, map[string]any{"mensaje": "Sesión iniciada correctamente.", "usuario": user, "requiere_cambio_password": user.RequiereCambioPassword, "csrf_token": middleware.TokenCSRF(sessionID, s.JWTSecret), "expira_en": expires})
}
