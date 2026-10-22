package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ExpedienteGuard coordina el registro y la sincronización en una instancia.
// Debe mantenerse bloqueado desde antes de escribir hasta resolver COMMIT.
// Una misma raíz local NO debe compartirse entre procesos de la aplicación.
var expedienteGuard sync.Mutex

func LockExpedientes() func() {
	expedienteGuard.Lock()
	return expedienteGuard.Unlock
}

type LocalStorage struct {
	root     string
	maxBytes int64
	mu       sync.Mutex
}

func NewLocalStorage(root string, maxBytes int64) (*LocalStorage, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errEmptyStorageRoot
	}
	if maxBytes < 0 {
		return nil, errors.New("el límite local no puede ser negativo")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absolute, 0750); err != nil {
		return nil, err
	}
	s := &LocalStorage{root: absolute, maxBytes: maxBytes}
	if err := s.CheckConnection(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *LocalStorage) Root() string {
	if s == nil {
		return ""
	}
	return s.root
}
func (s *LocalStorage) MaxBytes() int64 {
	if s == nil {
		return 0
	}
	return s.maxBytes
}
func (s *LocalStorage) CheckConnection() error {
	if s == nil || s.root == "" {
		return errEmptyStorageRoot
	}
	info, err := os.Stat(s.root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("la raíz local no es un directorio")
	}
	f, err := os.CreateTemp(s.root, ".local_check_*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.WriteString("check"); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Remove(name)
}
func (s *LocalStorage) GetExpedientePaths(number string) (*ExpedientePaths, error) {
	if s == nil || s.root == "" {
		return nil, errEmptyStorageRoot
	}
	return buildExpedientePaths(s.root, number)
}
func (s *LocalStorage) CreateExpediente(number string) (*ExpedientePaths, error) {
	if err := s.CheckConnection(); err != nil {
		return nil, err
	}
	p, err := s.GetExpedientePaths(number)
	if err != nil {
		return nil, err
	}
	if err := createExpedienteDirectories(p); err != nil {
		return nil, err
	}
	return p, nil
}
func (s *LocalStorage) ExpedienteExists(number string) (bool, error) {
	p, err := s.GetExpedientePaths(number)
	if err != nil {
		return false, err
	}
	return directoryExists(p.Root)
}

// expectedSize debe ser el tamaño exacto. Ya no se acepta 0 para omitirlo.
func (s *LocalStorage) SaveEvidence(number, fileName string, source io.Reader, expectedSize int64) (string, error) {
	if s == nil {
		return "", errEmptyStorageRoot
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if expectedSize <= 0 {
		return "", errors.New("tamaño de evidencia inválido")
	}
	if err := s.EnsureCapacity(expectedSize); err != nil {
		return "", err
	}
	p, err := s.CreateExpediente(number)
	if err != nil {
		return "", err
	}
	name, err := sanitizeFileName(fileName)
	if err != nil {
		return "", err
	}
	destination := filepath.Join(p.Evidencias, name)
	if err := writeSizedReaderToFile(destination, source, expectedSize); err != nil {
		return "", err
	}
	return destination, nil
}
func (s *LocalStorage) EnsureCapacity(additional int64) error {
	if s == nil {
		return errEmptyStorageRoot
	}
	if additional < 0 {
		return errors.New("tamaño adicional inválido")
	}
	if s.maxBytes == 0 {
		return nil
	}
	used, err := s.UsedBytes()
	if err != nil {
		return err
	}
	if used > s.maxBytes || additional > s.maxBytes-used {
		return fmt.Errorf("capacidad local insuficiente: usado=%d adicional=%d máximo=%d", used, additional, s.maxBytes)
	}
	return nil
}
func (s *LocalStorage) UsedBytes() (int64, error) {
	if s == nil {
		return 0, errEmptyStorageRoot
	}
	var total int64
	err := filepath.WalkDir(s.root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("enlace simbólico no permitido: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("archivo no regular: %s", path)
		}
		total += info.Size()
		return nil
	})
	return total, err
}
func (s *LocalStorage) ListPendingExpedientes() ([]string, error) {
	if err := s.CheckConnection(); err != nil {
		return nil, err
	}
	years, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	pending := make([]string, 0)
	for _, year := range years {
		if !year.IsDir() {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(s.root, year.Name()))
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			number, extracted, err := normalizeComplaintNumber(e.Name())
			if err == nil && extracted == year.Name() {
				pending = append(pending, number)
			}
		}
	}
	return pending, nil
}
func (s *LocalStorage) DeleteExpediente(number string) error {
	p, err := s.GetExpedientePaths(number)
	if err != nil {
		return err
	}
	if filepath.Clean(p.Root) == filepath.Clean(s.root) {
		return errors.New("no se puede borrar la raíz local")
	}
	if err := os.RemoveAll(p.Root); err != nil {
		return err
	}
	_ = os.Remove(filepath.Dir(p.Root)) // Solo elimina el año si está vacío.
	return nil
}

// Verifica el tamaño antes de publicar el nombre final.
func writeSizedReaderToFile(destination string, source io.Reader, size int64) error {
	if source == nil || size <= 0 {
		return errors.New("evidencia inválida")
	}
	limited := io.LimitReader(source, size+1)
	counted := &exactSizeReader{reader: limited, expected: size}
	return writeReaderToFile(destination, counted)
}

type exactSizeReader struct {
	reader          io.Reader
	count, expected int64
}

func (r *exactSizeReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.count += int64(n)
	if r.count > r.expected {
		return n, errors.New("el archivo supera el tamaño esperado")
	}
	if err == io.EOF && r.count != r.expected {
		return n, errors.New("el archivo no coincide con el tamaño esperado")
	}
	return n, err
}
