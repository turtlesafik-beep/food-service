package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"food-service/internal/backup"
	"food-service/internal/database"
	"food-service/internal/maintenance"
	"food-service/internal/notifications"
	"food-service/internal/orders"
	"food-service/internal/products"
)

func main() {
	db, err := database.Connect()
	if err != nil {
		log.Fatal("Database connection failed: ", err)
	}
	defer db.Close()

	log.Println("Database connected successfully")

	if err := database.InitSchema(db); err != nil {
		log.Fatal("Database schema initialization failed: ", err)
	}

	log.Println("Database schema initialized successfully")

	dataGate := maintenance.New()

	backupService, err := backup.NewService(backup.Config{
		DB:              db,
		DBPath:          database.Path(),
		UploadDir:       configuredUploadDir(),
		TempDir:         os.Getenv("TMP_DIR"),
		PublicBaseURL:   strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")),
		MaxArchiveBytes: envInt64("BACKUP_MAX_BYTES", 0),
	}, dataGate)
	if err != nil {
		log.Fatal("Backup service initialization failed: ", err)
	}
	if err := backupService.RecoverOnStartup(context.Background()); err != nil {
		log.Fatal("Restore recovery failed: ", err)
	}
	go backupService.Run(context.Background())

	productService := &products.Service{
		DB: db,
	}

	productHandler := &products.Handler{
		Service:   productService,
		UploadDir: configuredUploadDir(),
	}

	orderService := &orders.Service{
		DB: db,
	}

	if telegramConfig, err := notifications.LoadConfig(); err != nil {
		log.Printf("WARNING: Telegram notifications disabled: %v", err)
	} else {
		notifier := notifications.NewNotifier(db, orderService, telegramConfig)
		notifier.SetGate(dataGate)
		orderService.Notifications = notifier
		go notifier.Run(context.Background())
	}

	orderHandler := &orders.Handler{
		Service: orderService,
	}

	mux := http.NewServeMux()

	// Backup/restore routes manage maintenance mode themselves. They are
	// deliberately kept outside the normal request gate.
	mux.HandleFunc("POST /api/admin/backups", adminBackupAuth(backupService.HandlePrepareBackup))
	mux.HandleFunc("GET /api/admin/backups/{id}", adminBackupAuth(backupService.HandleBackupInfo))
	mux.HandleFunc("GET /api/admin/backups/{id}/download", adminBackupAuth(backupService.HandleDownloadBackup))
	mux.HandleFunc("POST /api/admin/backups/{id}/confirm", adminBackupAuth(backupService.HandleConfirmBackup))
	mux.HandleFunc("POST /api/admin/restores/validate", adminBackupAuth(backupService.HandleValidateRestore))
	mux.HandleFunc("POST /api/admin/restores/{id}/apply", adminBackupAuth(backupService.HandleApplyRestore))
	mux.HandleFunc("DELETE /api/admin/restores/{id}", adminBackupAuth(backupService.HandleCancelRestore))

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Food Service is healthy!")
	})

	mux.HandleFunc("/api/orders", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			orderHandler.CreateOrder(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("GET /api/orders/{token}", orderHandler.GetOrder)

	mux.HandleFunc("/api/admin/dishes", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			productHandler.GetDishes(w, r)
		case http.MethodPost:
			productHandler.CreateDish(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("GET /api/dishes", productHandler.GetPublicDishes)

	mux.HandleFunc("GET /api/categories", productHandler.GetCategories)

	mux.HandleFunc("GET /media/dishes/{filename}", productHandler.ServeDishPhoto)

	mux.HandleFunc("GET /api/admin/orders", adminAuth(orderHandler.GetOrders))

	mux.HandleFunc("GET /api/admin/orders/{id}", adminAuth(orderHandler.GetAdminOrder))
	mux.HandleFunc("DELETE /api/admin/orders/{id}", adminAuth(orderHandler.DeleteOrder))

	mux.HandleFunc("/api/admin/dishes/", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/recipe") {
			switch r.Method {
			case http.MethodGet:
				productHandler.GetRecipe(w, r)
			case http.MethodPut:
				productHandler.UpdateRecipe(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		if strings.HasSuffix(r.URL.Path, "/photo") {
			switch r.Method {
			case http.MethodPost:
				productHandler.UploadDishPhoto(w, r)
			case http.MethodDelete:
				productHandler.DeleteDishPhoto(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}

		switch r.Method {
		case http.MethodPut:
			productHandler.UpdateDish(w, r)
		case http.MethodPatch:
			productHandler.UpdateDishAvailability(w, r)
		case http.MethodDelete:
			productHandler.DeleteDish(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/admin/ingredients", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			productHandler.GetIngredients(w, r)
		case http.MethodPost:
			productHandler.CreateIngredient(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/admin/ingredients/", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			productHandler.UpdateIngredient(w, r)
		case http.MethodDelete:
			productHandler.DeleteIngredient(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/admin/categories", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			productHandler.CreateCategory(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	mux.HandleFunc("/api/admin/categories/", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			productHandler.UpdateCategory(w, r)
		case http.MethodDelete:
			productHandler.DeleteCategory(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	if os.Getenv("ADMIN_PASSWORD") == "" {
		log.Println("WARNING: ADMIN_PASSWORD is not set; admin API is unprotected")
	}

	log.Println("Server started on :3000")

	err = http.ListenAndServe(":3000", gatedHandler(dataGate, mux))
	if err != nil {
		log.Fatal(err)
	}
}

func gatedHandler(gate *maintenance.Gate, mux *http.ServeMux) http.Handler {
	guarded := gate.Middleware(mux)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/admin/backups") ||
			strings.HasPrefix(r.URL.Path, "/api/admin/restores") {
			// Do not start another backup/restore while maintenance is active.
			// The apply request itself passed this check before taking the gate.
			if gate.Active() {
				w.Header().Set("Retry-After", "5")
				http.Error(w, "Идёт обслуживание данных. Повторите запрос через несколько секунд.", http.StatusServiceUnavailable)
				return
			}
			mux.ServeHTTP(w, r)
			return
		}
		guarded.ServeHTTP(w, r)
	})
}

func adminBackupAuth(next http.HandlerFunc) http.HandlerFunc {
	if os.Getenv("ADMIN_PASSWORD") == "" {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Резервное копирование отключено: ADMIN_PASSWORD не задан.", http.StatusServiceUnavailable)
		}
	}

	return adminAuth(next)
}

func configuredUploadDir() string {
	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "data/uploads/dishes"
	}
	return uploadDir
}

func envInt64(name string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		log.Printf("WARNING: invalid %s=%q, using default", name, value)
		return fallback
	}
	return parsed
}
