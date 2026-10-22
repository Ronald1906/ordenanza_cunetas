package endpoints

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type Denunciante struct {
	Tipo           *string `json:"tipo"`
	TipoNombre     *string `json:"tipo_nombre"`
	Identificacion *string `json:"identificacion"`
	Nombres        *string `json:"nombres_razon_social"`
	Telefono       *string `json:"telefono"`
	Email          *string `json:"email"`
}
type Problema struct {
	ID     int16  `json:"id"`
	Codigo string `json:"codigo"`
	Nombre string `json:"nombre"`
}
type Evidencia struct {
	ID             int64     `json:"id"`
	NombreOriginal string    `json:"nombre_original"`
	NombreArchivo  string    `json:"nombre_archivo"`
	TipoMime       string    `json:"tipo_mime"`
	TamanoBytes    int64     `json:"tamano_bytes"`
	FechaCreacion  time.Time `json:"fecha_creacion"`
	URL            string    `json:"url"`
}
type Historial struct {
	ID             int64     `json:"id"`
	EstadoAnterior *Estado   `json:"estado_anterior"`
	EstadoNuevo    Estado    `json:"estado_nuevo"`
	UsuarioID      *int64    `json:"usuario_id"`
	Usuario        *string   `json:"usuario"`
	Nombres        *string   `json:"nombres"`
	Observacion    *string   `json:"observacion"`
	Respuesta      *string   `json:"respuesta"`
	EsPublico      bool      `json:"es_publico"`
	Fecha          time.Time `json:"fecha"`
}
type Almacenamiento struct {
	Ubicacion            string     `json:"ubicacion"`
	RutaRelativa         *string    `json:"ruta_relativa"`
	SincronizacionEstado string     `json:"sincronizacion_estado"`
	Intentos             int32      `json:"sincronizacion_intentos"`
	UltimoIntento        *time.Time `json:"sincronizacion_ultimo_intento"`
	FechaSincronizacion  *time.Time `json:"sincronizacion_fecha"`
}
type DetalleDenuncia struct {
	DenunciaResumen
	Denunciante         *Denunciante   `json:"denunciante"`
	Referencia          *string        `json:"referencia_ubicacion"`
	Latitud             float64        `json:"latitud"`
	Longitud            float64        `json:"longitud"`
	DeclaracionAceptada bool           `json:"declaracion_aceptada"`
	Almacenamiento      Almacenamiento `json:"almacenamiento"`
	Problemas           []Problema     `json:"problemas"`
	Evidencias          []Evidencia    `json:"evidencias"`
	Historial           []Historial    `json:"historial"`
	RespuestaFinal      *string        `json:"respuesta_final"`
}

// GET /api/admin/gestion/denuncias/{id}
// Incluye referencias privadas a evidencias, no bytes base64 ni raíces físicas.
func (s *Service) DetalleDenuncia(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSession(w, r); !ok {
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		fail(w, 400, "ID_NO_VALIDO", "El identificador de denuncia no es válido.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		dbError(w, "iniciar detalle", err)
		return
	}
	defer rollback(tx)
	var d DetalleDenuncia
	var tipo, tipoNombre, identificacion, nombres, telefono, email, referencia, ruta pgtype.Text
	var ultimo, fechaSync pgtype.Timestamptz
	err = tx.QueryRow(ctx, `SELECT d.id,d.numero_denuncia,d.tipo_denuncia,TO_CHAR(d.fecha_denuncia,'YYYY-MM-DD'),d.descripcion,
 d.canton,c.nombre,d.parroquia,d.sector,st.nombre,d.barrio_recinto_comunidad,
 e.id,e.codigo,e.nombre,e.es_final,d.fecha_creacion,d.fecha_actualizacion,
 d.tipo_denunciante,dt.nombre,d.identificacion,d.nombres_razon_social,d.telefono,d.email,
 d.referencia_ubicacion,d.latitud::DOUBLE PRECISION,d.longitud::DOUBLE PRECISION,d.declaracion_aceptada,
 d.almacenamiento_ubicacion,d.almacenamiento_ruta_relativa,d.sincronizacion_estado,d.sincronizacion_intentos,d.sincronizacion_ultimo_intento,d.sincronizacion_fecha
 FROM denuncia d JOIN canton c ON c.codigo=d.canton JOIN sector_tipo st ON st.codigo=d.sector JOIN denuncia_estado e ON e.id=d.estado_id
 LEFT JOIN denunciante_tipo dt ON dt.codigo=d.tipo_denunciante WHERE d.id=$1`, id).Scan(
		&d.ID, &d.NumeroDenuncia, &d.TipoDenuncia, &d.FechaDenuncia, &d.Descripcion, &d.Canton.Codigo, &d.Canton.Nombre, &d.Parroquia, &d.Sector.Codigo, &d.Sector.Nombre, &d.Barrio,
		&d.Estado.ID, &d.Estado.Codigo, &d.Estado.Nombre, &d.Estado.EsFinal, &d.FechaCreacion, &d.FechaActualizacion,
		&tipo, &tipoNombre, &identificacion, &nombres, &telefono, &email, &referencia, &d.Latitud, &d.Longitud, &d.DeclaracionAceptada,
		&d.Almacenamiento.Ubicacion, &ruta, &d.Almacenamiento.SincronizacionEstado, &d.Almacenamiento.Intentos, &ultimo, &fechaSync)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "DENUNCIA_NO_ENCONTRADA", "La denuncia no existe.")
		return
	}
	if err != nil {
		dbError(w, "consultar detalle", err)
		return
	}
	if d.TipoDenuncia == "IDENTIFICADA" {
		d.Denunciante = &Denunciante{ptrText(tipo), ptrText(tipoNombre), ptrText(identificacion), ptrText(nombres), ptrText(telefono), ptrText(email)}
	}
	d.Referencia = ptrText(referencia)
	d.Almacenamiento.RutaRelativa = ptrText(ruta)
	d.Almacenamiento.UltimoIntento = ptrTime(ultimo)
	d.Almacenamiento.FechaSincronizacion = ptrTime(fechaSync)
	d.Problemas = make([]Problema, 0)
	d.Evidencias = make([]Evidencia, 0)
	d.Historial = make([]Historial, 0)
	rows, err := tx.Query(ctx, `SELECT p.id,p.codigo,p.nombre FROM denuncia_problema dp JOIN problema_tipo p ON p.id=dp.problema_tipo_id WHERE dp.denuncia_id=$1 ORDER BY p.orden,p.id`, id)
	if err != nil {
		dbError(w, "consultar problemas", err)
		return
	}
	for rows.Next() {
		var item Problema
		if err := rows.Scan(&item.ID, &item.Codigo, &item.Nombre); err != nil {
			rows.Close()
			dbError(w, "leer problema", err)
			return
		}
		d.Problemas = append(d.Problemas, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		dbError(w, "recorrer problemas", err)
		return
	}
	rows, err = tx.Query(ctx, `SELECT id,nombre_original,nombre_archivo,tipo_mime,tamano_bytes,fecha_creacion FROM denuncia_evidencia WHERE denuncia_id=$1 ORDER BY id`, id)
	if err != nil {
		dbError(w, "consultar evidencias", err)
		return
	}
	for rows.Next() {
		var item Evidencia
		if err := rows.Scan(&item.ID, &item.NombreOriginal, &item.NombreArchivo, &item.TipoMime, &item.TamanoBytes, &item.FechaCreacion); err != nil {
			rows.Close()
			dbError(w, "leer evidencia", err)
			return
		}
		item.URL = fmt.Sprintf("/api/admin/gestion/denuncias/%d/evidencias/%d", id, item.ID)
		d.Evidencias = append(d.Evidencias, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		dbError(w, "recorrer evidencias", err)
		return
	}
	d.TotalEvidencias = int64(len(d.Evidencias))
	rows, err = tx.Query(ctx, `SELECT h.id,an.id,an.codigo,an.nombre,an.es_final,n.id,n.codigo,n.nombre,n.es_final,
 h.usuario_id,u.usuario,u.nombres,h.observacion,h.respuesta,h.es_publico,h.fecha_cambio
 FROM denuncia_historial_estado h LEFT JOIN denuncia_estado an ON an.id=h.estado_anterior_id JOIN denuncia_estado n ON n.id=h.estado_nuevo_id
 LEFT JOIN usuario u ON u.id=h.usuario_id WHERE h.denuncia_id=$1 ORDER BY h.fecha_cambio,h.id`, id)
	if err != nil {
		dbError(w, "consultar historial", err)
		return
	}
	for rows.Next() {
		var item Historial
		var oldID pgtype.Int2
		var oldCode, oldName, user, userName, observacion, respuesta pgtype.Text
		var oldFinal pgtype.Bool
		var userID pgtype.Int8
		if err := rows.Scan(&item.ID, &oldID, &oldCode, &oldName, &oldFinal, &item.EstadoNuevo.ID, &item.EstadoNuevo.Codigo, &item.EstadoNuevo.Nombre, &item.EstadoNuevo.EsFinal, &userID, &user, &userName, &observacion, &respuesta, &item.EsPublico, &item.Fecha); err != nil {
			rows.Close()
			dbError(w, "leer historial", err)
			return
		}
		if oldID.Valid {
			item.EstadoAnterior = &Estado{oldID.Int16, oldCode.String, oldName.String, oldFinal.Bool}
		}
		if userID.Valid {
			value := userID.Int64
			item.UsuarioID = &value
		}
		item.Usuario = ptrText(user)
		item.Nombres = ptrText(userName)
		item.Observacion = ptrText(observacion)
		item.Respuesta = ptrText(respuesta)
		d.Historial = append(d.Historial, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		dbError(w, "recorrer historial", err)
		return
	}
	if d.Estado.EsFinal && len(d.Historial) > 0 {
		last := d.Historial[len(d.Historial)-1]
		if last.EstadoNuevo.ID == d.Estado.ID {
			d.RespuestaFinal = last.Respuesta
		}
	}
	if err := tx.Commit(ctx); err != nil {
		dbError(w, "terminar detalle", err)
		return
	}
	jsonResponse(w, 200, map[string]any{"data": d})
}
