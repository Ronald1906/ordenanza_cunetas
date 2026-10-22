package storage

import (
	"errors"
	"os"
	"strings"
)

// CheckReadConnection verifica el montaje y la raíz, sin exigir escritura.
// Permite consultar evidencias existentes aunque el NAS esté en solo lectura.
func (s *NASStorage) CheckReadConnection() error {
	if s == nil || strings.TrimSpace(s.Root()) == "" {
		return errEmptyStorageRoot
	}
	if err := verifyNASMount(s.Root()); err != nil {
		return err
	}
	info, err := os.Stat(s.Root())
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("la raíz NAS no es un directorio")
	}
	return nil
}
