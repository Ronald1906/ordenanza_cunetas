package endpoints

import (
	"context"
	"errors"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denuncias_cunetas_go/internal/storage"

	"github.com/jackc/pgx/v5"
)

// openEvidence no acepta rutas del cliente: solo un nombre generado y asociado
// a una denuncia en SQL. Comprueba que los enlaces no salgan de la raíz.
func openEvidence(root, number, name string, expectedSize int64) (*os.File, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") || expectedSize <= 0 {
		return nil, errors.New("metadatos de evidencia no válidos")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	resolvedRoot, err = filepath.Abs(resolvedRoot)
	if err != nil {
		return nil, err
	}
	// GetExpedientePaths valida el formato del número antes de construir carpetas.
	local, err := storage.NewNASStorage(resolvedRoot)
	if err != nil {
		return nil, err
	}
	paths, err := local.GetExpedientePaths(number)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(paths.Evidencias, name))
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(paths.Evidencias, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, errors.New("la evidencia sale de su raíz")
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != expectedSize {
		file.Close()
		return nil, errors.New("tipo o tamaño de evidencia no coincide")
	}
	return file, nil
}

// GET/HEAD /api/admin/gestion/denuncias/{id}/evidencias/{evidencia_id}
// ServeContent soporta Range y seek para reproducción de videos.
func (s *Service) Evidencia(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSession(w, r); !ok {
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		fail(w, 400, "ID_NO_VALIDO", "El identificador de denuncia no es válido.")
		return
	}
	evidenceID, err := parseID(r, "evidencia_id")
	if err != nil {
		fail(w, 400, "ID_NO_VALIDO", "El identificador de evidencia no es válido.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	var number, location, name, original, contentType string
	var size int64
	err = s.DB.QueryRow(ctx, `SELECT d.numero_denuncia,d.almacenamiento_ubicacion,e.nombre_archivo,e.nombre_original,e.tipo_mime,e.tamano_bytes
 FROM denuncia_evidencia e JOIN denuncia d ON d.id=e.denuncia_id WHERE e.id=$1 AND e.denuncia_id=$2`, evidenceID, id).Scan(&number, &location, &name, &original, &contentType, &size)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 404, "EVIDENCIA_NO_ENCONTRADA", "La evidencia no pertenece a esa denuncia o no existe.")
		return
	}
	if err != nil {
		dbError(w, "consultar evidencia", err)
		return
	}
	if contentType != "image/jpeg" && contentType != "image/png" && contentType != "video/mp4" {
		fail(w, 409, "EVIDENCIA_NO_VALIDA", "El formato registrado no está permitido.")
		return
	}
	// Coordina solo la apertura. No bloquea cargas/sincronización mientras se
	// reproduce el video. El descriptor abierto permanece válido en Linux;
	// en Windows una eliminación ocupada se conserva para un reintento del sync.
	unlock := storage.LockExpedientes()
	var file *os.File
	var nasUnavailable bool
	tryLocal := func() {
		if file == nil && s.Local != nil {
			f, err := openEvidence(s.Local.Root(), number, name, size)
			if err == nil {
				file = f
			} else if !os.IsNotExist(err) {
				log.Printf("[ADMIN EVIDENCIA] local denuncia=%d evidencia=%d: %v", id, evidenceID, err)
			}
		}
	}
	tryNAS := func() {
		if file != nil {
			return
		}
		if s.NAS == nil {
			nasUnavailable = true
			return
		}
		if err := s.NAS.CheckReadConnection(); err != nil {
			nasUnavailable = true
			log.Printf("[ADMIN EVIDENCIA] NAS no disponible: %v", err)
			return
		}
		f, err := openEvidence(s.NAS.Root(), number, name, size)
		if err == nil {
			file = f
		} else if !os.IsNotExist(err) {
			log.Printf("[ADMIN EVIDENCIA] NAS denuncia=%d evidencia=%d: %v", id, evidenceID, err)
		}
	}
	switch location {
	case "LOCAL":
		tryLocal()
		tryNAS()
	case "NAS":
		tryNAS()
		tryLocal()
	default:
		unlock()
		fail(w, 409, "UBICACION_NO_VALIDA", "La denuncia no tiene una ubicación válida para esta evidencia.")
		return
	}
	unlock()
	if file == nil {
		if nasUnavailable {
			fail(w, 503, "ALMACENAMIENTO_NO_DISPONIBLE", "El almacenamiento no está disponible y no existe una copia local accesible.")
		} else {
			fail(w, 404, "ARCHIVO_NO_DISPONIBLE", "La evidencia está registrada pero su archivo no está disponible.")
		}
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		dbError(w, "leer archivo", err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": original}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	http.ServeContent(w, r, name, info.ModTime(), file)
}
