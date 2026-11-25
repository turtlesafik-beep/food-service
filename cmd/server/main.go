package main

import (
	"fmt"
	"log"
	"net/http"
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

	productHandler := &products.Handler{
		Service: productService,
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

	http.HandleFunc("/api/admin/dishes", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			productHandler.GetDishes(w, r)
		case http.MethodPost:
			productHandler.CreateDish(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("GET /api/dishes", productHandler.GetPublicDishes)

	http.HandleFunc("GET /api/admin/orders", orderHandler.GetOrders)

	http.HandleFunc("GET /api/admin/orders/{id}", orderHandler.GetAdminOrder)

	http.HandleFunc("/api/admin/dishes/", func(w http.ResponseWriter, r *http.Request) {
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

		switch r.Method {
		case http.MethodPut:
			productHandler.UpdateDish(w, r)
		case http.MethodPatch:
			productHandler.UpdateDishAvailability(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	http.HandleFunc("/api/admin/ingredients", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			productHandler.GetIngredients(w, r)
		case http.MethodPost:
			productHandler.CreateIngredient(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	log.Println("Server started on :3000")

	err = http.ListenAndServe(":3000", nil)
	if err != nil {
		log.Fatal(err)
	}
}
