package storage

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultSyncInterval = 5 * time.Minute
)

// ============================================================
// ACTUALIZACIÓN DE ESTADO DE SINCRONIZACIÓN
// ============================================================

// SyncStatusUpdater permite actualizar PostgreSQL durante
// el proceso de sincronización.
//
// Puede permanecer en nil inicialmente.
//
// Más adelante podremos crear una implementación asociada
// a PostgreSQL para registrar estados como:
//
//   - LOCAL
//   - SINCRONIZANDO
//   - NAS
//   - ERROR
type SyncStatusUpdater interface {
	// Solo permite expedientes confirmados y valida sus evidencias contra SQL.
	CanSync(context.Context, string) (bool, error)
	ValidateLocal(context.Context, string, string) error

	MarkSyncing(
		ctx context.Context,
		numeroDenuncia string,
	) error

	MarkSynchronized(
		ctx context.Context,
		numeroDenuncia string,
	) error

	MarkSyncError(
		ctx context.Context,
		numeroDenuncia string,
		errorMessage string,
	) error
}

// ============================================================
// SERVICIO DE SINCRONIZACIÓN
// ============================================================

type SyncService struct {
	nas      *NASStorage
	local    *LocalStorage
	status   SyncStatusUpdater
	interval time.Duration
	logger   *log.Logger

	mu      sync.Mutex
	running bool
}

// ============================================================
// REPORTE DE SINCRONIZACIÓN
// ============================================================

type SyncReport struct {
	Found        int      `json:"encontrados"`
	Synchronized int      `json:"sincronizados"`
	Failed       int      `json:"fallidos"`
	Errors       []string `json:"errores,omitempty"`
}

// ============================================================
// MANIFIESTO DE ARCHIVOS
// ============================================================

type fileManifestEntry struct {
	RelativePath string
	Size         int64
	Hash         [sha256.Size]byte
}

// ============================================================
// CONSTRUCTOR
// ============================================================

func NewSyncService(
	nas *NASStorage,
	local *LocalStorage,
	status SyncStatusUpdater,
	interval time.Duration,
	logger *log.Logger,
) (*SyncService, error) {

	if nas == nil {
		return nil, errors.New(
			"el almacenamiento NAS es obligatorio",
		)
	}

	if local == nil {
		return nil, errors.New(
			"el almacenamiento local es obligatorio",
		)
	}

	if status == nil {
		return nil, errors.New("el control PostgreSQL de sincronización es obligatorio")
	}

	if interval <= 0 {
		interval = defaultSyncInterval
	}

	if logger == nil {
		logger = log.Default()
	}

	return &SyncService{
		nas:      nas,
		local:    local,
		status:   status,
		interval: interval,
		logger:   logger,
	}, nil
}

// ============================================================
// INICIAR SERVICIO
// ============================================================

// Start inicia la sincronización periódica.
//
// Debe ejecutarse desde una goroutine:
//
//	go syncService.Start(ctx)
func (s *SyncService) Start(
	ctx context.Context,
) {

	if s == nil {
		return
	}

	ticker := time.NewTicker(
		s.interval,
	)

	defer ticker.Stop()

	s.logger.Printf(
		"sincronización de contingencia iniciada; intervalo=%s",
		s.interval,
	)

	/*
		Se intenta sincronizar inmediatamente al iniciar
		la aplicación.

		Esto permite procesar expedientes pendientes sin
		esperar al primer intervalo del ticker.
	*/
	s.runAndLog(
		ctx,
	)

	for {
		select {

		case <-ctx.Done():

			s.logger.Println(
				"sincronización de contingencia detenida",
			)

			return

		case <-ticker.C:

			s.runAndLog(
				ctx,
			)
		}
	}
}

// ============================================================
// EJECUTAR Y REGISTRAR RESULTADO
// ============================================================

func (s *SyncService) runAndLog(
	ctx context.Context,
) {

	report, err := s.RunOnce(
		ctx,
	)

	if err != nil {

		s.logger.Printf(
			"no se pudo ejecutar la sincronización: %v",
			err,
		)

		return
	}

	if report.Found == 0 {
		return
	}

	s.logger.Printf(
		"sincronización terminada: encontrados=%d sincronizados=%d fallidos=%d",
		report.Found,
		report.Synchronized,
		report.Failed,
	)
}

// ============================================================
// SINCRONIZAR TODOS LOS PENDIENTES
// ============================================================

// RunOnce sincroniza todos los expedientes encontrados
// en el almacenamiento local de contingencia.
func (s *SyncService) RunOnce(
	ctx context.Context,
) (*SyncReport, error) {

	if s == nil {
		return nil, errors.New(
			"el servicio de sincronización no está inicializado",
		)
	}

	if !s.tryStartRun() {
		return nil, errors.New(
			"ya existe una sincronización en ejecución",
		)
	}

	defer s.finishRun()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// ========================================================
	// VERIFICAR NAS
	// ========================================================

	if err := s.nas.CheckConnection(); err != nil {
		return nil, fmt.Errorf(
			"el NAS no está disponible: %w",
			err,
		)
	}

	// ========================================================
	// OBTENER EXPEDIENTES PENDIENTES
	// ========================================================

	pending, err :=
		s.local.ListPendingExpedientes()

	if err != nil {
		return nil, err
	}

	sort.Strings(
		pending,
	)

	report := &SyncReport{
		Found: len(
			pending,
		),
		Errors: make(
			[]string,
			0,
		),
	}

	// ========================================================
	// SINCRONIZAR CADA EXPEDIENTE
	// ========================================================

	for _, numeroDenuncia := range pending {

		if err := ctx.Err(); err != nil {
			return report, err
		}

		err := s.SyncExpediente(
			ctx,
			numeroDenuncia,
		)

		if err != nil {

			report.Failed++

			message := fmt.Sprintf(
				"%s: %v",
				numeroDenuncia,
				err,
			)

			report.Errors = append(
				report.Errors,
				message,
			)

			s.logger.Printf(
				"error sincronizando %s: %v",
				numeroDenuncia,
				err,
			)

			continue
		}

		report.Synchronized++

		s.logger.Printf(
			"expediente %s sincronizado correctamente",
			numeroDenuncia,
		)
	}

	return report, nil
}

// ============================================================
// SINCRONIZAR UN EXPEDIENTE
// ============================================================

// SyncExpediente migra un expediente individual desde
// el almacenamiento local hasta el NAS.
//
// Proceso:
//
//  1. Comprueba el NAS.
//  2. Comprueba el expediente local.
//  3. Marca el expediente como SINCRONIZANDO.
//  4. Copia a un directorio temporal dentro del NAS.
//  5. Compara tamaño y SHA-256 de cada archivo.
//  6. Confirma el directorio mediante Rename.
//  7. Marca PostgreSQL como sincronizado.
//  8. Elimina la copia local.
func (s *SyncService) SyncExpediente(
	ctx context.Context,
	numeroDenuncia string,
) error {
	unlock := LockExpedientes()
	defer unlock()
	ready, err := s.status.CanSync(ctx, numeroDenuncia)
	if err != nil {
		return err
	}
	if !ready {
		return errors.New("expediente no confirmado o no elegible; se conserva local")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// ========================================================
	// COMPROBAR NAS
	// ========================================================

	if err := s.nas.CheckConnection(); err != nil {
		return err
	}

	// ========================================================
	// RUTAS LOCALES
	// ========================================================

	localPaths, err :=
		s.local.GetExpedientePaths(
			numeroDenuncia,
		)

	if err != nil {
		return err
	}

	localInfo, err := os.Stat(
		localPaths.Root,
	)

	if err != nil {

		if os.IsNotExist(
			err,
		) {

			return errors.New(
				"el expediente local no existe",
			)
		}

		return fmt.Errorf(
			"no se pudo acceder al expediente local: %w",
			err,
		)
	}

	if !localInfo.IsDir() {
		return errors.New(
			"la ruta local del expediente no es un directorio",
		)
	}

	// ========================================================
	// RUTAS NAS
	// ========================================================

	nasPaths, err :=
		s.nas.GetExpedientePaths(
			numeroDenuncia,
		)

	if err != nil {
		return err
	}

	if err := s.status.ValidateLocal(ctx, numeroDenuncia, localPaths.Root); err != nil {
		return err
	}

	// ========================================================
	// MARCAR SINCRONIZANDO
	// ========================================================

	if err := s.markSyncing(
		ctx,
		numeroDenuncia,
	); err != nil {

		return err
	}

	// ========================================================
	// COPIAR Y COMPROBAR
	// ========================================================

	syncErr := s.copyAndConfirm(
		ctx,
		localPaths.Root,
		nasPaths.Root,
		numeroDenuncia,
	)

	if syncErr != nil {

		_ = s.markSyncError(
			ctx,
			numeroDenuncia,
			syncErr,
		)

		return syncErr
	}

	// ========================================================
	// ACTUALIZAR ESTADO
	// ========================================================

	/*
		Primero se actualiza PostgreSQL.

		Si esto falla, la copia local se conserva para evitar
		perder la posibilidad de reintentar la operación.
	*/
	if err := s.markSynchronized(
		ctx,
		numeroDenuncia,
	); err != nil {

		return fmt.Errorf(
			"los archivos llegaron al NAS, pero no se pudo actualizar su estado: %w",
			err,
		)
	}

	// ========================================================
	// ELIMINAR CONTINGENCIA LOCAL
	// ========================================================

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := s.local.DeleteExpediente(
		numeroDenuncia,
	); err != nil {

		return fmt.Errorf(
			"el expediente fue sincronizado, pero no se pudo eliminar la copia local: %w",
			err,
		)
	}

	return nil
}

// ============================================================
// COPIAR Y CONFIRMAR EXPEDIENTE
// ============================================================

func (s *SyncService) copyAndConfirm(
	ctx context.Context,
	sourceRoot string,
	finalRoot string,
	numeroDenuncia string,
) error {

	finalParent := filepath.Dir(
		finalRoot,
	)

	if err := os.MkdirAll(
		finalParent,
		0750,
	); err != nil {

		return fmt.Errorf(
			"no se pudo crear el directorio anual en el NAS: %w",
			err,
		)
	}

	temporaryRoot, err := os.MkdirTemp(finalParent, ".syncing-*")
	if err != nil {
		return err
	}

	confirmed := false

	defer func() {

		if !confirmed {
			_ = os.RemoveAll(
				temporaryRoot,
			)
		}
	}()

	// ========================================================
	// ¿YA EXISTE EN NAS?
	// ========================================================

	finalExists, err :=
		directoryExists(
			finalRoot,
		)

	if err != nil {
		return err
	}

	if finalExists {

		equal, err :=
			compareDirectoryTrees(
				ctx,
				sourceRoot,
				finalRoot,
			)

		if err != nil {
			return err
		}

		if !equal {
			return fmt.Errorf(
				"ya existe un expediente diferente en el NAS para %s",
				numeroDenuncia,
			)
		}

		/*
			El contenido ya existe y coincide exactamente.

			Esto permite que una sincronización sea idempotente
			si el proceso se detuvo después de copiar al NAS
			pero antes de eliminar la contingencia local.
		*/
		_ = os.RemoveAll(temporaryRoot)
		confirmed = true

		return nil
	}

	// ========================================================
	// COPIA TEMPORAL
	// ========================================================

	if err := copyDirectory(
		ctx,
		sourceRoot,
		temporaryRoot,
	); err != nil {

		return err
	}

	// ========================================================
	// VERIFICACIÓN SHA-256
	// ========================================================

	equal, err := compareDirectoryTrees(
		ctx,
		sourceRoot,
		temporaryRoot,
	)

	if err != nil {
		return err
	}

	if !equal {
		return errors.New(
			"la verificación del expediente copiado no fue satisfactoria",
		)
	}

	// ========================================================
	// CONFIRMAR EXPEDIENTE
	// ========================================================

	if err := os.Rename(
		temporaryRoot,
		finalRoot,
	); err != nil {

		return fmt.Errorf(
			"no se pudo confirmar el expediente en el NAS: %w",
			err,
		)
	}

	confirmed = true

	return nil
}

// ============================================================
// COPIAR DIRECTORIO
// ============================================================

func copyDirectory(
	ctx context.Context,
	sourceRoot string,
	destinationRoot string,
) error {

	return filepath.WalkDir(
		sourceRoot,
		func(
			currentPath string,
			entry os.DirEntry,
			walkErr error,
		) error {

			if walkErr != nil {
				return walkErr
			}

			if err := ctx.Err(); err != nil {
				return err
			}

			relativePath, err :=
				filepath.Rel(
					sourceRoot,
					currentPath,
				)

			if err != nil {
				return err
			}

			destinationPath :=
				filepath.Join(
					destinationRoot,
					relativePath,
				)

			// No se permiten symlinks.
			if entry.Type()&os.ModeSymlink != 0 {

				return fmt.Errorf(
					"no se permiten enlaces simbólicos: %s",
					currentPath,
				)
			}

			// =================================================
			// DIRECTORIO
			// =================================================

			if entry.IsDir() {

				return os.MkdirAll(
					destinationPath,
					0750,
				)
			}

			// =================================================
			// ARCHIVO
			// =================================================

			info, err :=
				entry.Info()

			if err != nil {
				return err
			}

			if !info.Mode().IsRegular() {

				return fmt.Errorf(
					"se encontró un archivo no regular: %s",
					currentPath,
				)
			}

			return copyFile(
				ctx,
				currentPath,
				destinationPath,
			)
		},
	)
}

// ============================================================
// COPIAR ARCHIVO
// ============================================================

func copyFile(
	ctx context.Context,
	sourcePath string,
	destinationPath string,
) error {

	source, err := os.Open(
		sourcePath,
	)

	if err != nil {

		return fmt.Errorf(
			"no se pudo abrir el archivo origen: %w",
			err,
		)
	}

	defer source.Close()

	if err := os.MkdirAll(
		filepath.Dir(
			destinationPath,
		),
		0750,
	); err != nil {

		return fmt.Errorf(
			"no se pudo crear el directorio del archivo destino: %w",
			err,
		)
	}

	destination, err := os.OpenFile(
		destinationPath,
		os.O_CREATE|
			os.O_WRONLY|
			os.O_TRUNC,
		0640,
	)

	if err != nil {

		return fmt.Errorf(
			"no se pudo crear el archivo de destino: %w",
			err,
		)
	}

	copyErr := copyWithContext(
		ctx,
		destination,
		source,
	)

	syncErr :=
		destination.Sync()

	closeErr :=
		destination.Close()

	if copyErr != nil {
		return fmt.Errorf(
			"no se pudo copiar el archivo: %w",
			copyErr,
		)
	}

	if syncErr != nil {

		return fmt.Errorf(
			"no se pudo sincronizar el archivo copiado: %w",
			syncErr,
		)
	}

	if closeErr != nil {

		return fmt.Errorf(
			"no se pudo cerrar el archivo copiado: %w",
			closeErr,
		)
	}

	return nil
}

// ============================================================
// COPIA RESPETANDO CANCELACIÓN DEL CONTEXTO
// ============================================================

func copyWithContext(
	ctx context.Context,
	destination io.Writer,
	source io.Reader,
) error {

	buffer := make(
		[]byte,
		1024*1024,
	)

	for {

		if err := ctx.Err(); err != nil {
			return err
		}

		readCount, readErr :=
			source.Read(
				buffer,
			)

		if readCount > 0 {

			writeCount, writeErr :=
				destination.Write(
					buffer[:readCount],
				)

			if writeErr != nil {
				return writeErr
			}

			if writeCount != readCount {
				return io.ErrShortWrite
			}
		}

		if errors.Is(
			readErr,
			io.EOF,
		) {
			return nil
		}

		if readErr != nil {
			return readErr
		}
	}
}

// ============================================================
// COMPARAR DOS EXPEDIENTES
// ============================================================

func compareDirectoryTrees(
	ctx context.Context,
	firstRoot string,
	secondRoot string,
) (bool, error) {

	firstManifest, err :=
		buildDirectoryManifest(
			ctx,
			firstRoot,
		)

	if err != nil {
		return false, err
	}

	secondManifest, err :=
		buildDirectoryManifest(
			ctx,
			secondRoot,
		)

	if err != nil {
		return false, err
	}

	if len(firstManifest) !=
		len(secondManifest) {

		return false, nil
	}

	for index := range firstManifest {

		first :=
			firstManifest[index]

		second :=
			secondManifest[index]

		if first.RelativePath != second.RelativePath ||
			first.Size != second.Size ||
			first.Hash != second.Hash {

			return false, nil
		}
	}

	return true, nil
}

// ============================================================
// CONSTRUIR MANIFIESTO
// ============================================================

func buildDirectoryManifest(
	ctx context.Context,
	root string,
) ([]fileManifestEntry, error) {

	manifest := make(
		[]fileManifestEntry,
		0,
	)

	err := filepath.WalkDir(
		root,
		func(
			currentPath string,
			entry os.DirEntry,
			walkErr error,
		) error {

			if walkErr != nil {
				return walkErr
			}

			if err := ctx.Err(); err != nil {
				return err
			}

			if entry.IsDir() {
				return nil
			}

			if entry.Type()&os.ModeSymlink != 0 {

				return fmt.Errorf(
					"no se permiten enlaces simbólicos: %s",
					currentPath,
				)
			}

			info, err :=
				entry.Info()

			if err != nil {
				return err
			}

			if !info.Mode().IsRegular() {

				return fmt.Errorf(
					"se encontró un archivo no regular: %s",
					currentPath,
				)
			}

			relativePath, err :=
				filepath.Rel(
					root,
					currentPath,
				)

			if err != nil {
				return err
			}

			hash, err :=
				calculateFileHash(
					ctx,
					currentPath,
				)

			if err != nil {
				return err
			}

			manifest = append(
				manifest,
				fileManifestEntry{
					RelativePath: filepath.ToSlash(
						relativePath,
					),

					Size: info.Size(),

					Hash: hash,
				},
			)

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	sort.Slice(
		manifest,
		func(
			i,
			j int,
		) bool {

			return manifest[i].RelativePath <
				manifest[j].RelativePath
		},
	)

	return manifest, nil
}

// ============================================================
// CALCULAR SHA-256
// ============================================================

func calculateFileHash(
	ctx context.Context,
	path string,
) ([sha256.Size]byte, error) {

	var emptyHash [sha256.Size]byte

	file, err := os.Open(
		path,
	)

	if err != nil {
		return emptyHash, err
	}

	defer file.Close()

	hasher :=
		sha256.New()

	if err := copyWithContext(
		ctx,
		hasher,
		file,
	); err != nil {

		return emptyHash, err
	}

	var result [sha256.Size]byte

	copy(
		result[:],
		hasher.Sum(nil),
	)

	return result, nil
}

// ============================================================
// COMPROBAR DIRECTORIO
// ============================================================

func directoryExists(
	path string,
) (bool, error) {

	info, err := os.Stat(
		path,
	)

	if err != nil {

		if os.IsNotExist(
			err,
		) {
			return false, nil
		}

		return false, fmt.Errorf(
			"no se pudo comprobar el directorio %s: %w",
			path,
			err,
		)
	}

	if !info.IsDir() {

		return false, fmt.Errorf(
			"la ruta existe pero no es un directorio: %s",
			path,
		)
	}

	return true, nil
}

// ============================================================
// ESTADO: SINCRONIZANDO
// ============================================================

func (s *SyncService) markSyncing(
	ctx context.Context,
	numeroDenuncia string,
) error {

	if s.status == nil {
		return nil
	}

	if err := s.status.MarkSyncing(
		ctx,
		numeroDenuncia,
	); err != nil {

		return fmt.Errorf(
			"no se pudo marcar el expediente como SINCRONIZANDO: %w",
			err,
		)
	}

	return nil
}

// ============================================================
// ESTADO: SINCRONIZADO
// ============================================================

func (s *SyncService) markSynchronized(
	ctx context.Context,
	numeroDenuncia string,
) error {

	if s.status == nil {
		return nil
	}

	if err := s.status.MarkSynchronized(
		ctx,
		numeroDenuncia,
	); err != nil {

		return fmt.Errorf(
			"no se pudo marcar el expediente como sincronizado: %w",
			err,
		)
	}

	return nil
}

// ============================================================
// ESTADO: ERROR
// ============================================================

func (s *SyncService) markSyncError(
	ctx context.Context,
	numeroDenuncia string,
	syncErr error,
) error {

	if s.status == nil {
		return nil
	}

	if syncErr == nil {
		return nil
	}

	errorMessage := strings.TrimSpace(
		syncErr.Error(),
	)

	return s.status.MarkSyncError(
		ctx,
		numeroDenuncia,
		errorMessage,
	)
}

// ============================================================
// CONTROL DE CONCURRENCIA
// ============================================================

func (s *SyncService) tryStartRun() bool {

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return false
	}

	s.running = true

	return true
}

func (s *SyncService) finishRun() {

	s.mu.Lock()
	defer s.mu.Unlock()

	s.running = false
}
