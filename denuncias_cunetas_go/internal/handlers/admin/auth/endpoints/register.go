package endpoints

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

type registerRequest struct {
	Usuario  string `json:"usuario"`
	Password string `json:"password"`
	Nombres  string `json:"nombres"`
}

// POST /api/admin/auth/register — creación interna de administradores.
// Exige sesión válida, contraseña actualizada y X-CSRF-Token.
func (s *Service) Register(w http.ResponseWriter, r *http.Request) {
	actor, ok := sessionFromRequest(w, r)
	if !ok {
		return
	}
	var input registerRequest
	if !decodeJSON(w, r, &input) {
		return
	}
	input.Usuario = strings.TrimSpace(input.Usuario)
	input.Nombres = strings.ToUpper(strings.TrimSpace(input.Nombres))
	if !usernamePattern.MatchString(input.Usuario) {
		fail(w, 400, "USUARIO_NO_VALIDO", "El usuario debe tener entre 3 y 50 caracteres: letras, números, punto, guion o guion bajo.")
		return
	}
	if !utf8.ValidString(input.Nombres) || strings.ContainsRune(input.Nombres, 0) || utf8.RuneCountInString(input.Nombres) < 1 || utf8.RuneCountInString(input.Nombres) > 150 {
		fail(w, 400, "NOMBRES_NO_VALIDOS", "Los nombres deben contener entre 1 y 150 caracteres.")
		return
	}
	if err := validatePassword(input.Password); err != nil {
		fail(w, 400, "PASSWORD_NO_VALIDO", err.Error())
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		internalError(w, "generar contraseña", err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		internalError(w, "iniciar registro", err)
		return
	}
	defer rollback(tx)
	if err := validateActor(ctx, tx, actor); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fail(w, 403, "SESION_NO_AUTORIZADA", "Su sesión ya no permite esta operación.")
		} else {
			internalError(w, "validar autor", err)
		}
		return
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO usuario(usuario,password_hash,nombres,activo,requiere_cambio_password) VALUES($1,$2,$3,TRUE,TRUE) RETURNING id`, input.Usuario, string(hash), input.Nombres).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			fail(w, 409, "USUARIO_EXISTENTE", "El nombre de usuario ya está registrado.")
		} else {
			internalError(w, "registrar usuario", err)
		}
		return
	}
	if err := audit(ctx, tx, r, actor.Usuario.ID, actor.Usuario.Usuario, "CREACION_USUARIO", "EXITO", "USUARIO", id, "USUARIO ADMINISTRATIVO CREADO CON CAMBIO OBLIGATORIO DE CONTRASEÑA."); err != nil {
		internalError(w, "auditar registro", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		internalError(w, "confirmar registro", err)
		return
	}
	respondJSON(w, 201, map[string]any{"mensaje": "Usuario creado correctamente.", "usuario": map[string]any{"id": id, "usuario": input.Usuario, "nombres": input.Nombres, "activo": true, "requiere_cambio_password": true}})
}
