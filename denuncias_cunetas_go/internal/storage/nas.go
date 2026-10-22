package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	evidenceDirectoryName      = "evidencias"
	documentationDirectoryName = "documentacion"

	initialProcessDirectoryName   = "inicio_proceso"
	technicalReportsDirectoryName = "informes_tecnicos"
	reviewResponsesDirectoryName  = "respuestas_organo_revision"
	notificationsDirectoryName    = "notificaciones"
	closureDirectoryName          = "cierre"
)

var (
	errEmptyStorageRoot = errors.New(
		"la ruta raíz del almacenamiento está vacía",
	)

	errInvalidComplaintNumber = errors.New(
		"el número de denuncia no tiene un formato válido",
	)

	// Formato esperado:
	//
	// DC-2026-000001
	// DC-2026-000002
	// ...
	//
	// Se permiten seis o más dígitos en el consecutivo
	// para no limitar el crecimiento futuro.
	complaintNumberPattern = regexp.MustCompile(
		`^DC-([0-9]{4})-([0-9]{6,})$`,
	)
)

// ============================================================
// RUTAS DEL EXPEDIENTE
// ============================================================

// ExpedientePaths contiene las rutas físicas y relativas
// correspondientes al expediente de una denuncia.
type ExpedientePaths struct {
	Root         string
	RelativeRoot string

	Evidencias    string
	Documentacion string

	InicioProceso    string
	InformesTecnicos string
	Respuestas       string
	Notificaciones   string
	Cierre           string
}

// ============================================================
// NAS STORAGE
// ============================================================

// NASStorage administra los expedientes almacenados
// en el recurso compartido del NAS.
type NASStorage struct {
	root string
}

// NewNASStorage crea una instancia del almacenamiento NAS.
//
// Importante:
//
// No crea la carpeta raíz automáticamente.
// La ruta debe corresponder a un recurso NAS previamente
// montado o disponible en el sistema operativo.
func NewNASStorage(
	root string,
) (*NASStorage, error) {

	root = strings.TrimSpace(
		root,
	)

	if root == "" {
		return nil, errEmptyStorageRoot
	}

	return &NASStorage{
		root: filepath.Clean(
			root,
		),
	}, nil
}

// Root devuelve la ruta raíz configurada para el NAS.
func (s *NASStorage) Root() string {

	if s == nil {
		return ""
	}

	return s.root
}

// ============================================================
// VERIFICAR CONEXIÓN
// ============================================================

// CheckConnection verifica:
//
//   - que la ruta configurada exista;
//   - que corresponda a un directorio;
//   - que permita crear archivos;
//   - que permita escribir;
//   - que permita sincronizar;
//   - que permita eliminar archivos.
//
// No crea automáticamente la carpeta raíz porque, si el NAS
// estuviera desmontado en Linux, podría terminar creando una
// carpeta local en el punto de montaje y dar la falsa impresión
// de que el NAS está disponible.
func (s *NASStorage) CheckConnection() error {

	if s == nil ||
		strings.TrimSpace(
			s.root,
		) == "" {

		return errEmptyStorageRoot
	}

	if err := verifyNASMount(s.root); err != nil {
		return err
	}

	info, err := os.Stat(
		s.root,
	)

	if err != nil {

		if os.IsNotExist(
			err,
		) {
			return fmt.Errorf(
				"la ruta del NAS no se encuentra disponible: %s",
				s.root,
			)
		}

		return fmt.Errorf(
			"no se pudo acceder a la ruta del NAS: %w",
			err,
		)
	}

	if !info.IsDir() {
		return fmt.Errorf(
			"la ruta configurada para el NAS no es un directorio: %s",
			s.root,
		)
	}

	// Archivo temporal utilizado únicamente
	// para comprobar permisos de escritura.
	testFile, err := os.CreateTemp(
		s.root,
		".dc_write_test_*",
	)

	if err != nil {
		return fmt.Errorf(
			"el NAS no permite escritura: %w",
			err,
		)
	}

	testFileName :=
		testFile.Name()

	testCompleted := false

	defer func() {

		_ = testFile.Close()

		if !testCompleted {
			_ = os.Remove(
				testFileName,
			)
		}
	}()

	if _, err := testFile.WriteString(
		"dc-storage-check",
	); err != nil {

		return fmt.Errorf(
			"no se pudo escribir el archivo de prueba del NAS: %w",
			err,
		)
	}

	if err := testFile.Sync(); err != nil {

		return fmt.Errorf(
			"no se pudo sincronizar el archivo de prueba del NAS: %w",
			err,
		)
	}

	if err := testFile.Close(); err != nil {

		return fmt.Errorf(
			"no se pudo cerrar el archivo de prueba del NAS: %w",
			err,
		)
	}

	if err := os.Remove(
		testFileName,
	); err != nil {

		return fmt.Errorf(
			"no se pudo eliminar el archivo de prueba del NAS: %w",
			err,
		)
	}

	testCompleted = true

	return nil
}

// IsAvailable indica si el NAS está disponible
// y permite operaciones de escritura.
func (s *NASStorage) IsAvailable() bool {

	if s == nil {
		return false
	}

	return s.CheckConnection() == nil
}

// ============================================================
// CREAR EXPEDIENTE
// ============================================================

// CreateExpediente crea toda la estructura base
// correspondiente a una denuncia.
func (s *NASStorage) CreateExpediente(
	numeroDenuncia string,
) (*ExpedientePaths, error) {

	if err := s.CheckConnection(); err != nil {
		return nil, err
	}

	paths, err := buildExpedientePaths(
		s.root,
		numeroDenuncia,
	)

	if err != nil {
		return nil, err
	}

	if err := createExpedienteDirectories(
		paths,
	); err != nil {

		return nil, err
	}

	return paths, nil
}

// ============================================================
// OBTENER RUTAS DEL EXPEDIENTE
// ============================================================

// GetExpedientePaths construye las rutas correspondientes
// al expediente sin crear directorios.
func (s *NASStorage) GetExpedientePaths(
	numeroDenuncia string,
) (*ExpedientePaths, error) {

	if s == nil ||
		strings.TrimSpace(
			s.root,
		) == "" {

		return nil, errEmptyStorageRoot
	}

	return buildExpedientePaths(
		s.root,
		numeroDenuncia,
	)
}

// ============================================================
// COMPROBAR EXISTENCIA
// ============================================================

// ExpedienteExists indica si el expediente existe físicamente
// dentro del NAS.
func (s *NASStorage) ExpedienteExists(
	numeroDenuncia string,
) (bool, error) {

	if err := s.CheckConnection(); err != nil {
		return false, err
	}

	paths, err := s.GetExpedientePaths(
		numeroDenuncia,
	)

	if err != nil {
		return false, err
	}

	info, err := os.Stat(
		paths.Root,
	)

	if err != nil {

		if os.IsNotExist(
			err,
		) {
			return false, nil
		}

		return false, fmt.Errorf(
			"no se pudo consultar el expediente en el NAS: %w",
			err,
		)
	}

	return info.IsDir(), nil
}

// ============================================================
// GUARDAR EVIDENCIA
// ============================================================

// SaveEvidence guarda una evidencia dentro de:
//
// <NAS>/<AÑO>/<NUMERO_DENUNCIA>/evidencias/<archivo>
//
// Ejemplo:
//
// Z:\denuncias_cunetas\2026\DC-2026-000002\evidencias\foto_001.jpg
func (s *NASStorage) SaveEvidence(
	numeroDenuncia string,
	fileName string,
	source io.Reader,
) (string, error) {

	if err := s.CheckConnection(); err != nil {
		return "", err
	}

	paths, err := s.CreateExpediente(
		numeroDenuncia,
	)

	if err != nil {
		return "", err
	}

	safeFileName, err := sanitizeFileName(
		fileName,
	)

	if err != nil {
		return "", err
	}

	destination := filepath.Join(
		paths.Evidencias,
		safeFileName,
	)

	if err := writeReaderToFile(
		destination,
		source,
	); err != nil {

		return "", err
	}

	return destination, nil
}

// ============================================================
// ELIMINAR EXPEDIENTE
// ============================================================

// DeleteExpediente elimina un expediente completo del NAS.
//
// Debe utilizarse únicamente como operación compensatoria
// cuando una transacción de negocio no pudo completarse.
//
// Nunca permite eliminar directamente la raíz configurada
// del almacenamiento.
func (s *NASStorage) DeleteExpediente(
	numeroDenuncia string,
) error {

	if err := s.CheckConnection(); err != nil {
		return err
	}

	paths, err := s.GetExpedientePaths(
		numeroDenuncia,
	)

	if err != nil {
		return err
	}

	if filepath.Clean(
		paths.Root,
	) == filepath.Clean(
		s.root,
	) {

		return errors.New(
			"no se puede eliminar la raíz del almacenamiento",
		)
	}

	if err := os.RemoveAll(
		paths.Root,
	); err != nil {

		return fmt.Errorf(
			"no se pudo eliminar el expediente del NAS: %w",
			err,
		)
	}

	return nil
}

// ============================================================
// CONSTRUIR RUTAS DEL EXPEDIENTE
// ============================================================

func buildExpedientePaths(
	root string,
	numeroDenuncia string,
) (*ExpedientePaths, error) {

	normalizedNumber, year, err :=
		normalizeComplaintNumber(
			numeroDenuncia,
		)

	if err != nil {
		return nil, err
	}

	// Ejemplo:
	//
	// Z:\denuncias_cunetas
	// └── 2026
	//     └── DC-2026-000002

	expedienteRoot := filepath.Join(
		root,
		year,
		normalizedNumber,
	)

	documentationRoot := filepath.Join(
		expedienteRoot,
		documentationDirectoryName,
	)

	return &ExpedientePaths{
		Root: expedienteRoot,

		RelativeRoot: filepath.ToSlash(
			filepath.Join(
				year,
				normalizedNumber,
			),
		) + "/",

		Evidencias: filepath.Join(
			expedienteRoot,
			evidenceDirectoryName,
		),

		Documentacion: documentationRoot,

		InicioProceso: filepath.Join(
			documentationRoot,
			initialProcessDirectoryName,
		),

		InformesTecnicos: filepath.Join(
			documentationRoot,
			technicalReportsDirectoryName,
		),

		Respuestas: filepath.Join(
			documentationRoot,
			reviewResponsesDirectoryName,
		),

		Notificaciones: filepath.Join(
			documentationRoot,
			notificationsDirectoryName,
		),

		Cierre: filepath.Join(
			documentationRoot,
			closureDirectoryName,
		),
	}, nil
}

// ============================================================
// CREAR DIRECTORIOS
// ============================================================

func createExpedienteDirectories(
	paths *ExpedientePaths,
) error {

	if paths == nil {
		return errors.New(
			"las rutas del expediente no están definidas",
		)
	}

	directories := []string{
		paths.Evidencias,
		paths.InicioProceso,
		paths.InformesTecnicos,
		paths.Respuestas,
		paths.Notificaciones,
		paths.Cierre,
	}

	for _, directory := range directories {

		if err := os.MkdirAll(
			directory,
			0750,
		); err != nil {

			return fmt.Errorf(
				"no se pudo crear el directorio %s: %w",
				directory,
				err,
			)
		}
	}

	return nil
}

// ============================================================
// NORMALIZAR NÚMERO DE DENUNCIA
// ============================================================

func normalizeComplaintNumber(
	numeroDenuncia string,
) (
	normalized string,
	year string,
	err error,
) {

	normalized = strings.ToUpper(
		strings.TrimSpace(
			numeroDenuncia,
		),
	)

	matches :=
		complaintNumberPattern.FindStringSubmatch(
			normalized,
		)

	if len(matches) != 3 {
		return "", "", errInvalidComplaintNumber
	}

	return normalized, matches[1], nil
}

// ============================================================
// SANITIZAR NOMBRE DE ARCHIVO
// ============================================================

func sanitizeFileName(
	fileName string,
) (string, error) {

	fileName = strings.TrimSpace(
		fileName,
	)

	if fileName == "" {
		return "", errors.New(
			"el nombre del archivo está vacío",
		)
	}

	if fileName == ".." || strings.ContainsAny(fileName, `/\`) || fileName == "." ||
		fileName == string(
			filepath.Separator,
		) {

		return "", errors.New(
			"el nombre del archivo no es válido",
		)
	}

	fileName = strings.ReplaceAll(
		fileName,
		"\x00",
		"",
	)

	if strings.TrimSpace(
		fileName,
	) == "" {

		return "", errors.New(
			"el nombre del archivo no es válido",
		)
	}

	return fileName, nil
}

// ============================================================
// ESCRIBIR ARCHIVO
// ============================================================

// writeReaderToFile escribe primero en un archivo temporal
// dentro del mismo directorio y finalmente utiliza Rename.
//
// De esta forma se reduce la posibilidad de que quede un
// archivo final parcialmente escrito.
func writeReaderToFile(
	destination string,
	source io.Reader,
) error {

	if source == nil {
		return errors.New(
			"el contenido del archivo no está disponible",
		)
	}

	if err := os.MkdirAll(
		filepath.Dir(
			destination,
		),
		0750,
	); err != nil {

		return fmt.Errorf(
			"no se pudo crear el directorio del archivo: %w",
			err,
		)
	}

	temporaryFile, err := os.CreateTemp(
		filepath.Dir(
			destination,
		),
		".upload_*",
	)

	if err != nil {
		return fmt.Errorf(
			"no se pudo crear el archivo temporal: %w",
			err,
		)
	}

	temporaryName :=
		temporaryFile.Name()

	completed := false

	defer func() {

		_ = temporaryFile.Close()

		if !completed {
			_ = os.Remove(
				temporaryName,
			)
		}
	}()

	if _, err := io.Copy(
		temporaryFile,
		source,
	); err != nil {

		return fmt.Errorf(
			"no se pudo escribir el archivo: %w",
			err,
		)
	}

	if err := temporaryFile.Sync(); err != nil {

		return fmt.Errorf(
			"no se pudo sincronizar el archivo: %w",
			err,
		)
	}

	if err := temporaryFile.Close(); err != nil {

		return fmt.Errorf(
			"no se pudo cerrar el archivo: %w",
			err,
		)
	}

	// Evita sobrescribir accidentalmente
	// una evidencia existente.
	if _, err := os.Stat(
		destination,
	); err == nil {

		return fmt.Errorf(
			"ya existe un archivo con el nombre %s",
			filepath.Base(
				destination,
			),
		)

	} else if !os.IsNotExist(
		err,
	) {

		return fmt.Errorf(
			"no se pudo comprobar el archivo de destino: %w",
			err,
		)
	}

	if err := os.Rename(
		temporaryName,
		destination,
	); err != nil {

		return fmt.Errorf(
			"no se pudo confirmar el archivo guardado: %w",
			err,
		)
	}

	completed = true

	return nil
}
