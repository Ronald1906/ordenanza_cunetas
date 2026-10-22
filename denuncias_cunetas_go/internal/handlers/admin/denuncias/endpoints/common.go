package endpoints

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"denuncias_cunetas_go/internal/middleware"
	"denuncias_cunetas_go/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Database interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Begin(context.Context) (pgx.Tx, error)
	BeginTx(context.Context, pgx.TxOptions) (pgx.Tx, error)
}
type Service struct {
	DB    Database
	NAS   *storage.NASStorage
	Local *storage.LocalStorage
}

func NewService(db Database, nas *storage.NASStorage, local *storage.LocalStorage) *Service {
	return &Service{db, nas, local}
}

type Estado struct {
	ID      int16  `json:"id"`
	Codigo  string `json:"codigo"`
	Nombre  string `json:"nombre"`
	EsFinal bool   `json:"es_final"`
}
type Opcion struct {
	Codigo string `json:"codigo"`
	Nombre string `json:"nombre"`
}

type DenunciaResumen struct {
	ID                 int64     `json:"id"`
	NumeroDenuncia     string    `json:"numero_denuncia"`
	TipoDenuncia       string    `json:"tipo_denuncia"`
	FechaDenuncia      string    `json:"fecha_denuncia"`
	Descripcion        string    `json:"descripcion"`
	Canton             Opcion    `json:"canton"`
	Parroquia          string    `json:"parroquia"`
	Sector             Opcion    `json:"sector"`
	Barrio             string    `json:"barrio"`
	Estado             Estado    `json:"estado"`
	TotalEvidencias    int64     `json:"total_evidencias"`
	FechaCreacion      time.Time `json:"fecha_creacion"`
	FechaActualizacion time.Time `json:"fecha_actualizacion"`
}

func jsonResponse(w http.ResponseWriter, status int, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		middleware.ResponderErrorSesion(w, 500, "ERROR_INTERNO", "No fue posible generar la respuesta.")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		log.Printf("[ADMIN DENUNCIAS] respuesta: %v", err)
	}
}
func fail(w http.ResponseWriter, status int, code, message string) {
	middleware.ResponderErrorSesion(w, status, code, message)
}
func dbError(w http.ResponseWriter, operation string, err error) {
	log.Printf("[ADMIN DENUNCIAS] %s: %v", operation, err)
	fail(w, 500, "ERROR_INTERNO", "No fue posible completar la operación.")
}
func requireSession(w http.ResponseWriter, r *http.Request) (middleware.Sesion, bool) {
	s, ok := middleware.SesionDesdeContexto(r.Context())
	if !ok {
		fail(w, 401, "SESION_REQUERIDA", "Debe iniciar sesión.")
		return s, false
	}
	if s.Usuario.RequiereCambioPassword {
		fail(w, 403, "CAMBIO_PASSWORD_REQUERIDO", "Debe cambiar su contraseña.")
		return s, false
	}
	return s, true
}
func parseID(r *http.Request, key string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, key), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("identificador no válido")
	}
	return id, nil
}
func rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func ptrText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	s := value.String
	return &s
}
func ptrTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}
func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func normalizeText(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }
func validText(value string, max int) bool {
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= max
}
func decodeJSON(w http.ResponseWriter, r *http.Request, data any) bool {
	content, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || content != "application/json" {
		fail(w, 415, "FORMATO_NO_VALIDO", "Debe enviar application/json.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(data); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			fail(w, 413, "CUERPO_DEMASIADO_GRANDE", "La petición supera el tamaño permitido.")
		} else {
			fail(w, 400, "JSON_NO_VALIDO", "El JSON no es válido o contiene campos desconocidos.")
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		fail(w, 400, "JSON_NO_VALIDO", "Envíe un único objeto JSON.")
		return false
	}
	return true
}
func peerIP(r *http.Request) any {
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
func validateActor(ctx context.Context, tx pgx.Tx, session middleware.Sesion) error {
	var id int64
	return tx.QueryRow(ctx, `SELECT u.id FROM usuario u JOIN usuario_sesion s ON s.usuario_id=u.id
 WHERE u.id=$1 AND s.id=$2 AND u.activo=TRUE AND u.requiere_cambio_password=FALSE
 AND s.revocada_en IS NULL AND s.expira_en>NOW() FOR SHARE OF u,s`, session.Usuario.ID, session.ID).Scan(&id)
}
func auditState(ctx context.Context, tx pgx.Tx, r *http.Request, actor middleware.Sesion, id int64, oldCode, newCode string) error {
	agent := strings.ReplaceAll(strings.ToValidUTF8(r.UserAgent(), ""), "\x00", "")
	if len(agent) > 2048 {
		agent = strings.ToValidUTF8(agent[:2048], "")
	}
	_, err := tx.Exec(ctx, `INSERT INTO auditoria_log(usuario_id,usuario_nombre,accion,resultado,entidad,entidad_id,descripcion,ip,user_agent)
 VALUES($1,$2,'CAMBIO_ESTADO_DENUNCIA','EXITO','DENUNCIA',$3,$4,$5,$6)`, actor.Usuario.ID, actor.Usuario.Usuario, id, "CAMBIO DE ESTADO: "+oldCode+" → "+newCode+".", peerIP(r), agent)
	return err
}
