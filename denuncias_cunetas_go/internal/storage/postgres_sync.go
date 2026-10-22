package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresSyncStatus struct{ db *pgxpool.Pool }

func NewPostgresSyncStatus(db *pgxpool.Pool) *PostgresSyncStatus { return &PostgresSyncStatus{db: db} }
func (s *PostgresSyncStatus) CanSync(ctx context.Context, number string) (bool, error) {
	var ready bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM denuncia WHERE numero_denuncia=$1 AND (almacenamiento_ubicacion='LOCAL' OR (almacenamiento_ubicacion='NAS' AND sincronizacion_estado='SINCRONIZADA')))`, number).Scan(&ready)
	return ready, err
}

// Confirma que la carpeta tiene exactamente las evidencias registradas.
// No migra archivos desconocidos ni temporales de cargas incompletas.
func (s *PostgresSyncStatus) ValidateLocal(ctx context.Context, number, root string) error {
	rows, err := s.db.Query(ctx, `SELECT nombre_archivo,tamano_bytes FROM denuncia_evidencia e JOIN denuncia d ON d.id=e.denuncia_id WHERE d.numero_denuncia=$1`, number)
	if err != nil {
		return err
	}
	expected := map[string]int64{}
	for rows.Next() {
		var name string
		var size int64
		if err := rows.Scan(&name, &size); err != nil {
			rows.Close()
			return err
		}
		expected[name] = size
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(expected) == 0 {
		return errors.New("expediente sin evidencias confirmadas; se conserva local")
	}
	seen := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("enlace simbólico no permitido")
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		// El módulo administrativo aún no escribe documentos. Cuando se incorpore,
		// deberá registrar y coordinar también esos documentos antes de habilitarlos.
		if !strings.HasPrefix(rel, "evidencias/") || strings.Count(rel, "/") != 1 {
			return fmt.Errorf("archivo sin registro de sincronización: %s", rel)
		}
		name := strings.TrimPrefix(rel, "evidencias/")
		size, ok := expected[name]
		if !ok {
			return fmt.Errorf("evidencia desconocida: %s", name)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() != size {
			return fmt.Errorf("tamaño o tipo incorrecto: %s", name)
		}
		seen[name] = true
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(expected) {
		return errors.New("faltan evidencias del expediente")
	}
	return nil
}
func (s *PostgresSyncStatus) MarkSyncing(ctx context.Context, number string) error {
	// Si quedó la copia local tras un fallo de borrado, permite verificar y limpiar.
	tag, err := s.db.Exec(ctx, `UPDATE denuncia SET sincronizacion_estado=CASE WHEN almacenamiento_ubicacion='LOCAL' THEN 'SINCRONIZANDO' ELSE sincronizacion_estado END,sincronizacion_intentos=sincronizacion_intentos+1,sincronizacion_ultimo_intento=NOW(),sincronizacion_error=NULL WHERE numero_denuncia=$1 AND (almacenamiento_ubicacion='LOCAL' OR (almacenamiento_ubicacion='NAS' AND sincronizacion_estado='SINCRONIZADA'))`, number)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("expediente no elegible")
	}
	return nil
}
func (s *PostgresSyncStatus) MarkSynchronized(ctx context.Context, number string) error {
	tag, err := s.db.Exec(ctx, `UPDATE denuncia SET almacenamiento_ubicacion='NAS',sincronizacion_estado='SINCRONIZADA',sincronizacion_fecha=COALESCE(sincronizacion_fecha,NOW()),sincronizacion_error=NULL WHERE numero_denuncia=$1 AND (almacenamiento_ubicacion='LOCAL' OR (almacenamiento_ubicacion='NAS' AND sincronizacion_estado='SINCRONIZADA'))`, number)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("no se actualizó el expediente")
	}
	return nil
}
func (s *PostgresSyncStatus) MarkSyncError(ctx context.Context, number, message string) error {
	_, err := s.db.Exec(ctx, `UPDATE denuncia SET sincronizacion_estado='ERROR',sincronizacion_error=$2 WHERE numero_denuncia=$1 AND almacenamiento_ubicacion='LOCAL'`, number, message)
	return err
}
