package storage

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// En Linux valida el montaje más específico que contiene la raíz NAS.
// Para pruebas locales se requiere NAS_ALLOW_LOCAL_FOR_DEV=true explícitamente.
func verifyNASMount(root string) error {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("NAS_ALLOW_LOCAL_FOR_DEV")), "true") {
		return nil
	}
	if runtime.GOOS != "linux" {
		return errors.New("verificación NAS de producción disponible en Linux; para desarrollo local configure NAS_ALLOW_LOCAL_FOR_DEV=true")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return err
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	best, fsType, source := "", "", ""
	decode := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for scanner.Scan() {
		halves := strings.SplitN(scanner.Text(), " - ", 2)
		if len(halves) != 2 {
			continue
		}
		left, right := strings.Fields(halves[0]), strings.Fields(halves[1])
		if len(left) < 5 || len(right) < 2 {
			continue
		}
		mount := filepath.Clean(decode.Replace(left[4]))
		rel, err := filepath.Rel(mount, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if len(mount) > len(best) {
			best, fsType, source = mount, right[0], decode.Replace(right[1])
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if fsType != "cifs" && fsType != "nfs" && fsType != "nfs4" && fsType != "smb3" {
		return fmt.Errorf("la raíz NAS no está sobre un montaje SMB/NFS: %s", root)
	}
	expected := strings.TrimSpace(os.Getenv("NAS_MOUNT_SOURCE"))
	if expected == "" {
		return errors.New("falta NAS_MOUNT_SOURCE para verificar el recurso NAS de producción")
	}
	if source != expected {
		return fmt.Errorf("el origen montado no coincide con NAS_MOUNT_SOURCE: %s", source)
	}
	return nil
}
