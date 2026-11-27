package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"food-service/internal/database"
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

	productService := &products.Service{
		DB: db,
	}

	uploadDir := os.Getenv("UPLOAD_DIR")
	if uploadDir == "" {
		uploadDir = "data/uploads/dishes"
	}

	productHandler := &products.Handler{
		Service:   productService,
		UploadDir: uploadDir,
	}

	orderService := &orders.Service{
		DB: db,
	}

	orderHandler := &orders.Handler{
		Service: orderService,
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Food Service is healthy!")
	})

	http.HandleFunc("/api/orders", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			orderHandler.CreateOrder(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("GET /api/orders/{token}", orderHandler.GetOrder)

	http.HandleFunc("/api/admin/dishes", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			productHandler.GetDishes(w, r)
		case http.MethodPost:
			productHandler.CreateDish(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	http.HandleFunc("GET /api/dishes", productHandler.GetPublicDishes)

	http.HandleFunc("GET /api/categories", productHandler.GetCategories)

	http.HandleFunc("GET /media/dishes/{filename}", productHandler.ServeDishPhoto)

	http.HandleFunc("GET /api/admin/orders", adminAuth(orderHandler.GetOrders))

	http.HandleFunc("GET /api/admin/orders/{id}", adminAuth(orderHandler.GetAdminOrder))
	http.HandleFunc("DELETE /api/admin/orders/{id}", adminAuth(orderHandler.DeleteOrder))

	http.HandleFunc("/api/admin/dishes/", adminAuth(func(w http.ResponseWriter, r *http.Request) {
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

	http.HandleFunc("/api/admin/ingredients", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			productHandler.GetIngredients(w, r)
		case http.MethodPost:
			productHandler.CreateIngredient(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	http.HandleFunc("/api/admin/ingredients/", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			productHandler.UpdateIngredient(w, r)
		case http.MethodDelete:
			productHandler.DeleteIngredient(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	http.HandleFunc("/api/admin/categories", adminAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			productHandler.CreateCategory(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	http.HandleFunc("/api/admin/categories/", adminAuth(func(w http.ResponseWriter, r *http.Request) {
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

	err = http.ListenAndServe(":3000", nil)
	if err != nil {
		log.Fatal(err)
	}
}
