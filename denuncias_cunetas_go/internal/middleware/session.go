package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// Database permite utilizar el pool pgx real o un doble de prueba.
type Database interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Begin(context.Context) (pgx.Tx, error)
}

const JWTIssuer = "denuncias_cunetas_go"
const JWTAudience = "administracion"

type Claims struct {
	UsuarioID int64  `json:"usuario_id"`
	Usuario   string `json:"usuario"`
	jwt.RegisteredClaims
}

type UsuarioSesion struct {
	ID                     int64   `json:"id"`
	Usuario                string  `json:"usuario"`
	Nombres                *string `json:"nombres"`
	RequiereCambioPassword bool    `json:"requiere_cambio_password"`
}

type Sesion struct {
	Usuario  UsuarioSesion
	ID       string
	ExpiraEn time.Time
}

type sessionContextKey struct{}

func SesionDesdeContexto(ctx context.Context) (Sesion, bool) {
	s, ok := ctx.Value(sessionContextKey{}).(Sesion)
	return s, ok
}

// RequerirSesion valida JWT y consulta el usuario y su sesión en PostgreSQL.
// La bandera de cambio obligatorio se lee de SQL, no se confía en el JWT.
func RequerirSesion(db Database, secret []byte, cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if db == nil || len(secret) < 32 {
				ResponderErrorSesion(w, 503, "SERVICIO_NO_DISPONIBLE", "La sesión no está configurada correctamente.")
				return
			}
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				ResponderErrorSesion(w, 401, "SESION_REQUERIDA", "Debe iniciar sesión.")
				return
			}
			claims, err := ValidarToken(cookie.Value, secret)
			if err != nil {
				ResponderErrorSesion(w, 401, "SESION_INVALIDA", "La sesión no es válida o ha expirado.")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			var sesion Sesion
			var nombres pgtype.Text
			err = db.QueryRow(ctx, `SELECT u.id,u.usuario,u.nombres,u.requiere_cambio_password,s.expira_en
    FROM usuario_sesion s JOIN usuario u ON u.id=s.usuario_id
    WHERE s.id=$1 AND s.usuario_id=$2 AND s.revocada_en IS NULL AND s.expira_en>NOW() AND u.activo=TRUE`, claims.ID, claims.UsuarioID).Scan(
				&sesion.Usuario.ID, &sesion.Usuario.Usuario, &nombres, &sesion.Usuario.RequiereCambioPassword, &sesion.ExpiraEn)
			cancel()
			if errors.Is(err, pgx.ErrNoRows) {
				ResponderErrorSesion(w, 401, "SESION_INVALIDA", "La sesión expiró, fue revocada o el usuario está inactivo.")
				return
			}
			if err != nil {
				ResponderErrorSesion(w, 503, "SERVICIO_NO_DISPONIBLE", "No fue posible verificar la sesión.")
				return
			}
			if nombres.Valid {
				value := nombres.String
				sesion.Usuario.Nombres = &value
			}
			sesion.ID = claims.ID
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, sesion)))
		})
	}
}

func ValidarToken(raw string, secret []byte) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired(), jwt.WithIssuedAt(),
		jwt.WithIssuer(JWTIssuer), jwt.WithAudience(JWTAudience))
	if err != nil {
		return nil, err
	}
	if !token.Valid || claims.UsuarioID <= 0 || claims.Subject != strconv.FormatInt(claims.UsuarioID, 10) || claims.IssuedAt == nil || len(claims.ID) != 64 {
		return nil, errors.New("claims inválidos")
	}
	for _, c := range claims.ID {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return nil, errors.New("identificador de sesión inválido")
		}
	}
	return claims, nil
}

func RequerirPasswordActualizada(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s, ok := SesionDesdeContexto(r.Context())
		if !ok {
			ResponderErrorSesion(w, 401, "SESION_REQUERIDA", "Debe iniciar sesión.")
			return
		}
		if s.Usuario.RequiereCambioPassword {
			ResponderErrorSesion(w, 403, "CAMBIO_PASSWORD_REQUERIDO", "Debe cambiar su contraseña antes de acceder a la administración.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Token CSRF ligado a la sesión; no revela la firma JWT ni la contraseña.
func TokenCSRF(sessionID string, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("denuncias-cunetas:csrf:" + sessionID))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func RequerirCSRF(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			s, ok := SesionDesdeContexto(r.Context())
			if !ok {
				ResponderErrorSesion(w, 401, "SESION_REQUERIDA", "Debe iniciar sesión.")
				return
			}
			received := r.Header.Get("X-CSRF-Token")
			if !hmac.Equal([]byte(received), []byte(TokenCSRF(s.ID, secret))) {
				ResponderErrorSesion(w, 403, "CSRF_INVALIDO", "La validación de la petición expiró. Consulte nuevamente su sesión.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Verifica Origin de peticiones que modifican datos. Las herramientas CLI pueden
// omitir Origin; los endpoints privados siguen exigiendo cookie y token CSRF.
func VerificarOrigen(origins []string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range origins {
		origin = strings.TrimSpace(origin)
		if origin != "" && origin != "*" {
			allowed[origin] = true
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			origin := r.Header.Get("Origin")
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if origin != "" && !allowed[origin] && origin != scheme+"://"+r.Host {
				ResponderErrorSesion(w, 403, "ORIGEN_NO_PERMITIDO", "El origen de la petición no está permitido.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func ResponderErrorSesion(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message, "codigo": code})
}
