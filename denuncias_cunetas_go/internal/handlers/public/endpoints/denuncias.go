package endpoints

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"denuncias_cunetas_go/internal/storage"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxTotalEvidenceSize int64 = 120 << 20
	maxRequestSize       int64 = maxTotalEvidenceSize + (2 << 20)
	maxFileSize          int64 = 30 << 20
)

var errEvidenciaGrande = errors.New("Cada evidencia puede tener un tamaño máximo de 30 MiB.")

var identificationPattern = regexp.MustCompile(`^([0-9]{10}|[0-9]{13})$`)
var phonePattern = regexp.MustCompile(`^[0-9]{10}$`)
var numberPattern = regexp.MustCompile(`^DC-[0-9]{4}-[0-9]{6,}$`)
var catalogCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

type RegistroDenunciaResponse struct {
	Mensaje        string `json:"mensaje"`
	NumeroDenuncia string `json:"numero_denuncia"`
}
type SeguimientoResponse struct {
	NumeroDenuncia string             `json:"numero_denuncia"`
	FechaDenuncia  time.Time          `json:"fecha_denuncia"`
	Estado         string             `json:"estado"`
	EstadoNombre   string             `json:"estado_nombre"`
	Descripcion    string             `json:"descripcion"`
	Historial      []HistorialPublico `json:"historial"`
}
type HistorialPublico struct {
	Respuesta   *string   `json:"respuesta"`
	Estado      string    `json:"estado"`
	Nombre      string    `json:"nombre"`
	Observacion *string   `json:"observacion"`
	Fecha       time.Time `json:"fecha"`
}
type evidenciaPreparada struct {
	Header                                                *multipart.FileHeader
	NombreOriginal, NombreArchivo, RutaRelativa, TipoMime string
	TamanoBytes                                           int64
}
type denunciaInput struct {
	Tipo, TipoDenunciante, Identificacion, Nombres, Telefono, Email string
	Canton, Parroquia, Sector, Barrio, Referencia, Descripcion      string
	Latitud, Longitud                                               float64
	Problemas                                                       []string
}

func normalizarTexto(value string) string { return strings.ToUpper(strings.TrimSpace(value)) }
func normalizarCodigo(value string) string {
	return strings.ReplaceAll(normalizarTexto(value), "-", "_")
}
func longitudValida(value string, min, max int) bool {
	n := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && !strings.ContainsRune(value, 0) && n >= min && n <= max
}
func valorNullable(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func leerDenuncia(r *http.Request) (denunciaInput, error) {
	// Solo leemos el cuerpo multipart; los parámetros de URL no sustituyen campos.
	get := func(name string) string { return r.PostForm.Get(name) }
	d := denunciaInput{Tipo: normalizarCodigo(get("tipoDenuncia")), TipoDenunciante: normalizarCodigo(get("tipoDenunciante")),
		Identificacion: strings.TrimSpace(get("identificacion")), Nombres: normalizarTexto(get("nombres")), Telefono: strings.TrimSpace(get("telefono")), Email: strings.TrimSpace(get("email")),
		Canton: normalizarCodigo(get("canton")), Parroquia: normalizarTexto(get("parroquia")), Sector: normalizarCodigo(get("sector")), Barrio: normalizarTexto(get("barrio")), Referencia: normalizarTexto(get("referencia")), Descripcion: normalizarTexto(get("descripcion"))}
	if d.Tipo != "IDENTIFICADA" && d.Tipo != "ANONIMA" {
		return d, errors.New("El tipo de denuncia no es válido.")
	}
	accepted := strings.ToLower(strings.TrimSpace(get("declaracion")))
	if accepted != "true" && accepted != "on" && accepted != "1" {
		return d, errors.New("Debe aceptar la declaración de veracidad.")
	}
	if !longitudValida(d.Canton, 1, 50) || !catalogCodePattern.MatchString(d.Canton) {
		return d, errors.New("Seleccione un cantón válido.")
	}
	if !longitudValida(d.Sector, 1, 30) || !catalogCodePattern.MatchString(d.Sector) {
		return d, errors.New("Seleccione un sector válido.")
	}
	if !longitudValida(d.Parroquia, 1, 150) {
		return d, errors.New("La parroquia debe contener entre 1 y 150 caracteres.")
	}
	if !longitudValida(d.Barrio, 1, 200) {
		return d, errors.New("El barrio debe contener entre 1 y 200 caracteres.")
	}
	if !longitudValida(d.Referencia, 0, 300) {
		return d, errors.New("La referencia admite hasta 300 caracteres.")
	}
	if !longitudValida(d.Descripcion, 20, 1000) {
		return d, errors.New("La descripción debe contener entre 20 y 1000 caracteres.")
	}
	var err error
	d.Latitud, err = strconv.ParseFloat(strings.TrimSpace(get("latitud")), 64)
	if err != nil || math.IsNaN(d.Latitud) || math.IsInf(d.Latitud, 0) || d.Latitud < -90 || d.Latitud > 90 {
		return d, errors.New("La latitud no es válida.")
	}
	d.Longitud, err = strconv.ParseFloat(strings.TrimSpace(get("longitud")), 64)
	if err != nil || math.IsNaN(d.Longitud) || math.IsInf(d.Longitud, 0) || d.Longitud < -180 || d.Longitud > 180 {
		return d, errors.New("La longitud no es válida.")
	}
	if d.Tipo == "ANONIMA" {
		d.TipoDenunciante = ""
		d.Identificacion = ""
		d.Nombres = ""
		d.Telefono = ""
		d.Email = ""
	} else {
		if !longitudValida(d.TipoDenunciante, 1, 50) || !catalogCodePattern.MatchString(d.TipoDenunciante) {
			return d, errors.New("Seleccione un tipo de denunciante válido.")
		}
		if !identificationPattern.MatchString(d.Identificacion) {
			return d, errors.New("La cédula debe tener 10 dígitos o el RUC 13 dígitos.")
		}
		if !longitudValida(d.Nombres, 1, 200) {
			return d, errors.New("Los nombres o razón social admiten entre 1 y 200 caracteres.")
		}
		if !phonePattern.MatchString(d.Telefono) {
			return d, errors.New("El teléfono debe tener 10 dígitos.")
		}
		if !longitudValida(d.Email, 0, 150) {
			return d, errors.New("El correo admite hasta 150 caracteres.")
		}
		if d.Email != "" {
			address, err := mail.ParseAddress(d.Email)
			if err != nil || address.Address != d.Email {
				return d, errors.New("El correo electrónico no es válido.")
			}
		}
	}
	seen := map[string]bool{}
	for _, value := range r.MultipartForm.Value["problemaReportado"] {
		code := normalizarCodigo(value)
		if !longitudValida(code, 1, 50) || !catalogCodePattern.MatchString(code) {
			return d, errors.New("Uno de los problemas no es válido.")
		}
		if !seen[code] {
			seen[code] = true
			d.Problemas = append(d.Problemas, code)
		}
	}
	if len(d.Problemas) == 0 {
		return d, errors.New("Seleccione al menos un problema.")
	}
	return d, nil
}

// validaCatalogos bloquea las filas seleccionadas mientras se registra.
// No acepta tablas suministradas por el cliente.
func validaCatalogos(ctx context.Context, tx pgx.Tx, d denunciaInput) (int16, error) {
	checks := []struct{ table, code string }{{"denuncia_tipo", d.Tipo}, {"canton", d.Canton}, {"sector_tipo", d.Sector}}
	if d.Tipo == "IDENTIFICADA" {
		checks = append(checks, struct{ table, code string }{"denunciante_tipo", d.TipoDenunciante})
	}
	for _, check := range checks {
		var id int16
		err := tx.QueryRow(ctx, "SELECT id FROM "+check.table+" WHERE codigo=$1 AND activo=TRUE FOR SHARE", check.code).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("opción no disponible: %s", check.code)
		}
		if err != nil {
			return 0, err
		}
	}
	var state int16
	err := tx.QueryRow(ctx, `SELECT id FROM denuncia_estado WHERE codigo='REGISTRADA' AND activo=TRUE FOR SHARE`).Scan(&state)
	return state, err
}

func RegistrarDenuncia(db *pgxpool.Pool, nas *storage.NASStorage, local *storage.LocalStorage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil || nas == nil || local == nil {
			responderError(w, 503, "El servicio no está disponible.")
			return
		}
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "multipart/form-data" {
			responderError(w, 400, "Debe enviar multipart/form-data.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)
		parseErr := r.ParseMultipartForm(8 << 20)
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		if parseErr != nil {
			var sizeErr *http.MaxBytesError
			if errors.As(parseErr, &sizeErr) {
				responderError(w, 413, "La petición supera el tamaño permitido.")
			} else {
				responderError(w, 400, "No se pudo procesar el formulario.")
			}
			return
		}
		d, err := leerDenuncia(r)
		if err != nil {
			responderError(w, 400, err.Error())
			return
		}
		files, err := prepararEvidencias(r.MultipartForm.File["evidencias"])
		if err != nil {
			if errors.Is(err, errEvidenciaGrande) {
				responderError(w, 413, err.Error())
				return
			}
			responderError(w, 400, err.Error())
			return
		}
		var total int64
		for _, f := range files {
			total += f.TamanoBytes
		}
		if total > maxTotalEvidenceSize {
			responderError(w, 413, "Las evidencias superan 120 MiB en total.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		// También bloquea la sincronización. Solo soporta una instancia por raíz local.
		unlock := storage.LockExpedientes()
		defer unlock()
		if err := ctx.Err(); err != nil {
			responderError(w, 503, "Se agotó el tiempo de registro.")
			return
		}
		tx, err := db.Begin(ctx)
		if err != nil {
			registroError(w, "iniciar transacción", err)
			return
		}
		// El rollback no depende de un contexto HTTP ya cancelado.
		defer func() {
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tx.Rollback(c)
		}()
		state, err := validaCatalogos(ctx, tx, d)
		if err != nil {
			if strings.HasPrefix(err.Error(), "opción no disponible:") {
				responderError(w, 400, "Una opción seleccionada ya no está disponible.")
			} else {
				registroError(w, "validar catálogos", err)
			}
			return
		}
		// Los problemas se consultan antes de escribir archivos, sin listas fijas.
		problemIDs := make([]int16, 0, len(d.Problemas))
		for _, code := range d.Problemas {
			var id int16
			err := tx.QueryRow(ctx, `SELECT id FROM problema_tipo WHERE codigo=$1 AND activo=TRUE FOR SHARE`, code).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				responderError(w, 400, "Uno de los problemas ya no está disponible.")
				return
			}
			if err != nil {
				registroError(w, "validar problema", err)
				return
			}
			problemIDs = append(problemIDs, id)
		}
		var id int64
		var number string
		err = tx.QueryRow(ctx, `INSERT INTO denuncia(tipo_denuncia,tipo_denunciante,identificacion,nombres_razon_social,telefono,email,canton,parroquia,sector,barrio_recinto_comunidad,referencia_ubicacion,latitud,longitud,descripcion,estado_id,declaracion_aceptada)
   VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,TRUE) RETURNING id,numero_denuncia`,
			d.Tipo, valorNullable(d.TipoDenunciante), valorNullable(d.Identificacion), valorNullable(d.Nombres), valorNullable(d.Telefono), valorNullable(d.Email), d.Canton, d.Parroquia, d.Sector, d.Barrio, valorNullable(d.Referencia), d.Latitud, d.Longitud, d.Descripcion, state).Scan(&id, &number)
		if err != nil {
			registroError(w, "insertar denuncia", err)
			return
		}
		for _, problemID := range problemIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO denuncia_problema(denuncia_id,problema_tipo_id) VALUES($1,$2)`, id, problemID); err != nil {
				registroError(w, "insertar problema", err)
				return
			}
		}
		inputs := make([]storage.EvidenceInput, 0, len(files))
		for _, f := range files {
			header := f.Header
			inputs = append(inputs, storage.EvidenceInput{Name: f.NombreArchivo, Size: f.TamanoBytes, Open: func() (io.ReadCloser, error) { return header.Open() }})
		}
		location, err := storage.WriteEvidenceBundle(ctx, nas, local, number, inputs)
		if err != nil {
			log.Printf("[REGISTRO] almacenamiento numero=%s error=%v", number, err)
			responderError(w, 503, "No fue posible guardar las evidencias. Intente nuevamente.")
			return
		}
		preserve := false
		defer func() {
			if preserve || location == "SIN_EVIDENCIAS" {
				return
			}
			var err error
			if location == "NAS" {
				err = nas.DeleteExpediente(number)
			} else {
				err = local.DeleteExpediente(number)
			}
			if err != nil {
				log.Printf("[REGISTRO] limpieza pendiente numero=%s: %v", number, err)
			}
		}()
		var relative any
		syncState := "NO_APLICA"
		if len(files) > 0 {
			paths, err := local.GetExpedientePaths(number)
			if err != nil {
				registroError(w, "preparar rutas", err)
				return
			}
			relative = paths.RelativeRoot
			if location == "LOCAL" {
				syncState = "PENDIENTE"
			}
			for _, f := range files {
				path := filepath.ToSlash(filepath.Join(paths.RelativeRoot, "evidencias", f.NombreArchivo))
				if _, err := tx.Exec(ctx, `INSERT INTO denuncia_evidencia(denuncia_id,nombre_original,nombre_archivo,ruta_relativa,tipo_mime,tamano_bytes) VALUES($1,$2,$3,$4,$5,$6)`, id, f.NombreOriginal, f.NombreArchivo, path, f.TipoMime, f.TamanoBytes); err != nil {
					registroError(w, "insertar evidencia", err)
					return
				}
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE denuncia SET almacenamiento_ubicacion=$2,almacenamiento_ruta_relativa=$3,sincronizacion_estado=$4 WHERE id=$1`, id, location, relative, syncState); err != nil {
			registroError(w, "guardar ubicación", err)
			return
		}
		if _, err := tx.Exec(ctx, `INSERT INTO denuncia_historial_estado(denuncia_id,estado_nuevo_id,observacion,es_publico) VALUES($1,$2,'DENUNCIA REGISTRADA MEDIANTE FORMULARIO CIUDADANO.',TRUE)`, id, state); err != nil {
			registroError(w, "insertar historial", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			// Un error de red no prueba que el COMMIT haya fallado. Conservamos archivos.
			preserve = true
			log.Printf("[REGISTRO] COMMIT por verificar numero=%s error=%v", number, err)
			checkCtx, checkCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer checkCancel()
			var exists bool
			checkErr := db.QueryRow(checkCtx, `SELECT EXISTS(SELECT 1 FROM denuncia WHERE id=$1 AND numero_denuncia=$2)`, id, number).Scan(&exists)
			if checkErr != nil || !exists {
				responderJSON(w, 503, map[string]string{"error": "No fue posible confirmar el registro. Consulte el número indicado antes de reenviar.", "numero_referencia": number})
				return
			}
		}
		preserve = true
		responderJSON(w, 201, RegistroDenunciaResponse{Mensaje: "Denuncia registrada correctamente.", NumeroDenuncia: number})
	}
}

func registroError(w http.ResponseWriter, step string, err error) {
	log.Printf("[REGISTRO] %s: %v", step, err)
	responderError(w, 500, "No fue posible completar el registro de la denuncia.")
}

func ConsultarSeguimiento(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if db == nil {
			responderError(w, 503, "El servicio no está disponible.")
			return
		}
		number := normalizarTexto(chi.URLParam(r, "numero"))
		if len(number) > 30 || !numberPattern.MatchString(number) {
			responderError(w, 400, "El número de denuncia no es válido.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		// Dos consultas con una misma instantánea de PostgreSQL.
		tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
		if err != nil {
			registroError(w, "iniciar seguimiento", err)
			return
		}
		defer func() {
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tx.Rollback(c)
		}()
		var id int64
		var response SeguimientoResponse
		err = tx.QueryRow(ctx, `SELECT d.id,d.numero_denuncia,d.fecha_denuncia,e.codigo,e.nombre,d.descripcion FROM denuncia d JOIN denuncia_estado e ON e.id=d.estado_id WHERE d.numero_denuncia=$1`, number).Scan(&id, &response.NumeroDenuncia, &response.FechaDenuncia, &response.Estado, &response.EstadoNombre, &response.Descripcion)
		if errors.Is(err, pgx.ErrNoRows) {
			responderError(w, 404, "No se encontró una denuncia con ese número.")
			return
		}
		if err != nil {
			registroError(w, "consultar seguimiento", err)
			return
		}
		rows, err := tx.Query(ctx, `SELECT e.codigo,e.nombre,CASE WHEN h.respuesta IS NULL THEN h.observacion ELSE NULL END,h.respuesta,h.fecha_cambio FROM denuncia_historial_estado h JOIN denuncia_estado e ON e.id=h.estado_nuevo_id WHERE h.denuncia_id=$1 AND h.es_publico=TRUE ORDER BY h.fecha_cambio,h.id`, id)
		if err != nil {
			registroError(w, "consultar historial", err)
			return
		}
		response.Historial = make([]HistorialPublico, 0)
		for rows.Next() {
			var item HistorialPublico
			if err := rows.Scan(&item.Estado, &item.Nombre, &item.Observacion, &item.Respuesta, &item.Fecha); err != nil {
				rows.Close()
				registroError(w, "leer historial", err)
				return
			}
			response.Historial = append(response.Historial, item)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			registroError(w, "leer historial", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			registroError(w, "terminar seguimiento", err)
			return
		}
		responderJSON(w, 200, response)
	}
}

func prepararEvidencias(files []*multipart.FileHeader) ([]evidenciaPreparada, error) {
	result := make([]evidenciaPreparada, 0, len(files))
	for index, header := range files {
		if header != nil && header.Size > maxFileSize {
			return nil, errEvidenciaGrande
		}
		if header == nil || header.Size <= 0 {
			return nil, errors.New("Cada evidencia debe tener contenido y no superar 30 MiB.")
		}
		if !longitudValida(header.Filename, 1, 255) {
			return nil, errors.New("El nombre original admite hasta 255 caracteres.")
		}
		source, err := header.Open()
		if err != nil {
			return nil, errors.New("No se pudo abrir una evidencia.")
		}
		buffer := make([]byte, 512)
		n, readErr := io.ReadFull(source, buffer)
		closeErr := source.Close()
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return nil, errors.New("No se pudo leer una evidencia.")
		}
		if closeErr != nil {
			return nil, closeErr
		}
		detected := http.DetectContentType(buffer[:n])
		prefix, extension := "", ""
		switch detected {
		case "image/jpeg":
			prefix, extension = "foto", ".jpg"
		case "image/png":
			prefix, extension = "foto", ".png"
		case "video/mp4":
			prefix, extension = "video", ".mp4"
		default:
			return nil, fmt.Errorf("El archivo %s no tiene un formato permitido.", header.Filename)
		}
		result = append(result, evidenciaPreparada{Header: header, NombreOriginal: header.Filename, NombreArchivo: fmt.Sprintf("%s_%03d%s", prefix, index+1, extension), TipoMime: detected, TamanoBytes: header.Size})
	}
	return result, nil
}
func responderJSON(w http.ResponseWriter, status int, data any) {
	content, err := json.Marshal(data)
	if err != nil {
		status = 500
		content = []byte(`{"error":"No se pudo generar la respuesta."}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if _, err := w.Write(content); err != nil {
		log.Printf("[HTTP] respuesta: %v", err)
	}
}
func responderError(w http.ResponseWriter, status int, message string) {
	responderJSON(w, status, map[string]string{"error": message})
}
