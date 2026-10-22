package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

type EvidenceInput struct {
	Name string
	Size int64
	Open func() (io.ReadCloser, error)
}

// WriteEvidenceBundle publica un expediente completo. El llamador mantiene
// LockExpedientes hasta resolver su transacción PostgreSQL.
// NAS se intenta primero; si falla, se reabren los originales para la copia local.
func WriteEvidenceBundle(ctx context.Context, nas *NASStorage, local *LocalStorage, number string, files []EvidenceInput) (location string, err error) {
	if len(files) == 0 {
		return "SIN_EVIDENCIAS", nil
	}
	if nas == nil || local == nil {
		return "", errors.New("almacenamientos no inicializados")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := nas.CheckConnection(); err == nil {
		if err := nas.writeBundle(ctx, number, files); err == nil {
			return "NAS", nil
		} else {
			log.Printf("[STORAGE] NAS falló; se reintentará en local numero=%s error=%v", number, err)
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var total int64
	for _, file := range files {
		if file.Size <= 0 {
			return "", errors.New("tamaño inválido")
		}
		total += file.Size
	}
	if err := local.EnsureCapacity(total); err != nil {
		return "", err
	}
	exists, err := local.ExpedienteExists(number)
	if err != nil {
		return "", err
	}
	if exists {
		return "", errors.New("ya existe el expediente local; no se sobrescribirá")
	}
	// Desde aquí solo se limpia la carpeta creada por este intento.
	defer func() {
		if err != nil {
			if e := local.DeleteExpediente(number); e != nil {
				log.Printf("[STORAGE] limpieza local pendiente numero=%s error=%v", number, e)
			}
		}
	}()
	for _, file := range files {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		var reader io.ReadCloser
		reader, err = file.Open()
		if err != nil {
			return "", err
		}
		_, err = local.SaveEvidence(number, file.Name, &contextReader{ctx: ctx, reader: reader}, file.Size)
		closeErr := reader.Close()
		if err != nil {
			return "", err
		}
		if closeErr != nil {
			err = closeErr
			return "", err
		}
	}
	return "LOCAL", nil
}

func (s *NASStorage) writeBundle(ctx context.Context, number string, files []EvidenceInput) error {
	final, err := s.GetExpedientePaths(number)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(final.Root); err == nil {
		return errors.New("el expediente NAS ya existe")
	} else if !os.IsNotExist(err) {
		return err
	}
	// La carpeta temporal no tiene un nombre de expediente válido.
	staging, err := os.MkdirTemp(s.root, ".incoming-*")
	if err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(staging); err != nil {
			log.Printf("[STORAGE] temporal NAS pendiente de limpieza %s: %v", staging, err)
		}
	}()
	tempPaths, err := buildExpedientePaths(staging, number)
	if err != nil {
		return err
	}
	if err := createExpedienteDirectories(tempPaths); err != nil {
		return err
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		name, err := sanitizeFileName(file.Name)
		if err != nil {
			return err
		}
		reader, err := file.Open()
		if err != nil {
			return err
		}
		err = writeSizedReaderToFile(filepath.Join(tempPaths.Evidencias, name), &contextReader{ctx: ctx, reader: reader}, file.Size)
		closeErr := reader.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.CheckConnection(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(final.Root), 0750); err != nil {
		return err
	}
	if err := os.Rename(tempPaths.Root, final.Root); err != nil {
		return fmt.Errorf("no se pudo publicar el expediente NAS: %w", err)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
