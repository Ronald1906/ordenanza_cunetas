package endpoints

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"denuncias_cunetas_go/internal/middleware"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const SessionDuration = 8 * time.Hour
const maxJSONSize = 8 << 10

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{2,49}$`)
var dummyPasswordHash = func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("comparacion-interna-sin-usuario"), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return hash
}()

type Service struct {
	DB           middleware.Database
	JWTSecret    []byte
	CookieName   string
	CookieSecure bool
}

func NewService(db middleware.Database, secret []byte, name string, secure bool) *Service {
	return &Service{DB: db, JWTSecret: secret, CookieName: name, CookieSecure: secure}
}
func (s *Service) configured() bool {
	return s.DB != nil && len(s.JWTSecret) >= 32 && (&http.Cookie{Name: s.CookieName}).Valid() == nil
}

func respondJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		middleware.ResponderErrorSesion(w, 500, "ERROR_INTERNO", "No fue posible generar la respuesta.")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		log.Printf("[AUTH] respuesta: %v", err)
	}
}
func fail(w http.ResponseWriter, status int, code, message string) {
	middleware.ResponderErrorSesion(w, status, code, message)
}
func internalError(w http.ResponseWriter, operation string, err error) {
	log.Printf("[AUTH] %s: %v", operation, err)
	fail(w, 500, "ERROR_INTERNO", "No fue posible completar la operación.")
}
func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		fail(w, 415, "FORMATO_NO_VALIDO", "Debe enviar application/json.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONSize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			fail(w, 413, "CUERPO_DEMASIADO_GRANDE", "La petición supera el tamaño permitido.")
		} else {
			fail(w, 400, "JSON_NO_VALIDO", "El cuerpo JSON no es válido o contiene campos desconocidos.")
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		fail(w, 400, "JSON_NO_VALIDO", "Envíe un único objeto JSON.")
		return false
	}
	return true
}
func validatePassword(value string) error {
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) || utf8.RuneCountInString(value) < 12 || len(value) > 72 || strings.TrimSpace(value) == "" {
		return errors.New("La contraseña debe tener al menos 12 caracteres y como máximo 72 bytes UTF-8.")
	}
	return nil
}
func validateCurrentPassword(value string) bool {
	return value != "" && len(value) <= 72 && utf8.ValidString(value) && !strings.ContainsRune(value, 0)
}
func newSessionID() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}
func (s *Service) setCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{Name: s.CookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int(SessionDuration.Seconds())})
}
func (s *Service) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: s.CookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: time.Unix(1, 0), MaxAge: -1})
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func remoteIP(r *http.Request) any {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	return ip.String()
}
func userAgent(r *http.Request) string {
	value := strings.ToValidUTF8(r.UserAgent(), "")
	if len(value) > 2048 {
		value = value[:2048]
		value = strings.ToValidUTF8(value, "")
	}
	return value
}
func audit(ctx context.Context, tx pgx.Tx, r *http.Request, actorID any, actorName, action, result, entity string, entityID any, message string) error {
	_, err := tx.Exec(ctx, `INSERT INTO auditoria_log(usuario_id,usuario_nombre,accion,resultado,entidad,entidad_id,descripcion,ip,user_agent) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, actorID, actorName, action, result, entity, entityID, message, remoteIP(r), userAgent(r))
	return err
}

// Los intentos fallidos se registran sin cuerpo HTTP ni contraseñas.
func (s *Service) failedLogin(r *http.Request, username string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := s.DB.Exec(ctx, `INSERT INTO auditoria_log(usuario_nombre,accion,resultado,entidad,descripcion,ip,user_agent) VALUES($1,'LOGIN','ERROR','USUARIO','INTENTO DE AUTENTICACIÓN RECHAZADO.',$2,$3)`, username, remoteIP(r), userAgent(r))
	if err != nil {
		log.Printf("[AUTH] no se pudo registrar login rechazado: %v", err)
	}
}
func sessionFromRequest(w http.ResponseWriter, r *http.Request) (middleware.Sesion, bool) {
	s, ok := middleware.SesionDesdeContexto(r.Context())
	if !ok {
		fail(w, 401, "SESION_REQUERIDA", "Debe iniciar sesión.")
	}
	return s, ok
}

// Revalida al autor dentro de la transacción y evita crear usuarios con una
// sesión que fue revocada después de pasar el middleware.
func validateActor(ctx context.Context, tx pgx.Tx, s middleware.Sesion) error {
	var id int64
	return tx.QueryRow(ctx, `SELECT u.id FROM usuario u JOIN usuario_sesion s ON s.usuario_id=u.id
  WHERE u.id=$1 AND s.id=$2 AND u.activo=TRUE AND u.requiere_cambio_password=FALSE
  AND s.revocada_en IS NULL AND s.expira_en>NOW() FOR SHARE OF u,s`, s.Usuario.ID, s.ID).Scan(&id)
}
