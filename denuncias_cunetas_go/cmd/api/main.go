package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"denuncias_cunetas_go/internal/database"
	"denuncias_cunetas_go/internal/router"
	"denuncias_cunetas_go/internal/storage"

	"github.com/joho/godotenv"
)

const (
	defaultPort                       = "8015"
	defaultLocalStorageMaxBytes int64 = 20 * 1024 * 1024 * 1024
	defaultSyncInterval               = 5 * time.Minute
	serverShutdownTimeout             = 30 * time.Second
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("error iniciando la aplicación: %v", err)
	}
}
func run() error {
	if err := godotenv.Load(); err != nil {
		log.Println("Archivo .env no encontrado; se usarán variables del sistema.")
	}
	databaseURL, err := requiredEnv("DATABASE_URL")
	if err != nil {
		return err
	}
	jwtSecret, err := requiredEnv("JWT_SECRET")
	if err != nil {
		return err
	}
	nasRoot, err := requiredEnv("NAS_DENUNCIAS_ROOT")
	if err != nil {
		return err
	}
	localRoot, err := requiredEnv("LOCAL_DENUNCIAS_ROOT")
	if err != nil {
		return err
	}
	originsRaw, err := requiredEnv("CORS_ALLOWED_ORIGINS")
	if err != nil {
		return err
	}
	if err := validateStorageRoots(nasRoot, localRoot); err != nil {
		return err
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = defaultPort
	}
	maxBytes, err := getLocalStorageMaxBytes()
	if err != nil {
		return err
	}
	interval, err := getSyncInterval()
	if err != nil {
		return err
	}
	cookieSecure := false
	if value := strings.TrimSpace(os.Getenv("COOKIE_SECURE")); value != "" {
		cookieSecure, err = strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("COOKIE_SECURE inválido: %w", err)
		}
	}
	origins := make([]string, 0)
	for _, origin := range strings.Split(originsRaw, ",") {
		if origin = strings.TrimSpace(origin); origin != "" {
			origins = append(origins, origin)
		}
	}
	if len(origins) == 0 {
		return errors.New("CORS_ALLOWED_ORIGINS no contiene orígenes válidos")
	}
	db, err := database.ConnectDB(databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := database.EnsureAdmin(db); err != nil {
		return err
	}
	nas, err := storage.NewNASStorage(nasRoot)
	if err != nil {
		return err
	}
	if err := nas.CheckConnection(); err != nil {
		log.Printf("NAS no disponible; se usará contingencia local: %v", err)
	} else {
		log.Printf("NAS disponible: %s", nas.Root())
	}
	local, err := storage.NewLocalStorage(localRoot, maxBytes)
	if err != nil {
		return err
	}
	log.Printf("Contingencia local: %s; máximo=%d bytes", local.Root(), local.MaxBytes())
	appCtx, cancelApp := context.WithCancel(context.Background())
	defer cancelApp()
	syncService, err := storage.NewSyncService(nas, local, storage.NewPostgresSyncStatus(db), interval, log.Default())
	if err != nil {
		return err
	}
	syncDone := make(chan struct{})
	go func() { defer close(syncDone); syncService.Start(appCtx) }()
	server := &http.Server{
		Addr: ":" + port, Handler: router.New(db, []byte(jwtSecret), cookieSecure, nas, local, origins),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 3 * time.Minute, WriteTimeout: 4 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20,
	}
	serverErrors := make(chan error, 1)
	go func() { log.Printf("API en http://localhost:%s", port); serverErrors <- server.ListenAndServe() }()
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var serveErr error
	select {
	case <-signalCtx.Done():
		log.Println("Se recibió señal de cierre.")
	case serveErr = <-serverErrors:
	}
	// Primero deja de aceptar y drena peticiones. No cierra PostgreSQL antes del worker.
	cancelApp()
	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), serverShutdownTimeout)
	defer cancelShutdown()
	shutdownErr := server.Shutdown(shutdownCtx)
	if shutdownErr != nil {
		_ = server.Close()
	}
	select {
	case <-syncDone:
		log.Println("Sincronizador detenido.")
	case <-shutdownCtx.Done():
		log.Println("El NAS no liberó el sincronizador dentro del plazo de cierre.")
	}
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	if shutdownErr != nil {
		return shutdownErr
	}
	return nil
}
func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("falta la variable %s", name)
	}
	return value, nil
}
func getLocalStorageMaxBytes() (int64, error) {
	value := strings.TrimSpace(os.Getenv("LOCAL_STORAGE_MAX_BYTES"))
	if value == "" {
		return defaultLocalStorageMaxBytes, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return 0, errors.New("LOCAL_STORAGE_MAX_BYTES debe ser entero mayor que cero")
	}
	return n, nil
}
func getSyncInterval() (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv("SYNC_INTERVAL_MINUTES"))
	if value == "" {
		return defaultSyncInterval, nil
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 || n > int64((1<<63-1)/int64(time.Minute)) {
		return 0, errors.New("SYNC_INTERVAL_MINUTES no es válido")
	}
	return time.Duration(n) * time.Minute, nil
}
func validateStorageRoots(nasRoot, localRoot string) error {
	nas, err := filepath.Abs(nasRoot)
	if err != nil {
		return err
	}
	local, err := filepath.Abs(localRoot)
	if err != nil {
		return err
	}
	// Resuelve enlaces en las rutas existentes y en sus ancestros.
	resolve := func(path string) string {
		tail := []string{}
		for {
			if resolved, err := filepath.EvalSymlinks(path); err == nil {
				for i := len(tail) - 1; i >= 0; i-- {
					resolved = filepath.Join(resolved, tail[i])
				}
				return resolved
			}
			parent := filepath.Dir(path)
			if parent == path {
				return path
			}
			tail = append(tail, filepath.Base(path))
			path = parent
		}
	}
	nas, local = resolve(nas), resolve(local)
	for _, pair := range [][2]string{{nas, local}, {local, nas}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New("las raíces NAS y local deben ser distintas y no contenerse")
		}
	}
	return nil
}
