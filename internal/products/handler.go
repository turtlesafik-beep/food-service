package products

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	Service *Service
}

func writeProductError(w http.ResponseWriter, err error) {
	var validationErr *ValidationError
	var conflictErr *ConflictError

	switch {
	case errors.Is(err, ErrDishNotFound), errors.Is(err, ErrIngredientNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.As(err, &conflictErr):
		http.Error(w, conflictErr.Message, http.StatusConflict)
	case errors.As(err, &validationErr):
		http.Error(w, validationErr.Message, http.StatusBadRequest)
	default:
		log.Printf("products API error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func parseDishID(path string) (int64, error) {
	idValue := strings.TrimPrefix(path, "/api/admin/dishes/")

	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid dish ID")
	}

	return id, nil
}

func parseRecipeDishID(path string) (int64, error) {
	idValue := strings.TrimSuffix(
		strings.TrimPrefix(path, "/api/admin/dishes/"),
		"/recipe",
	)

	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid dish ID")
	}

	return id, nil
}

func parseIngredientID(path string) (int64, error) {
	idValue := strings.TrimPrefix(path, "/api/admin/ingredients/")

	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid ingredient ID")
	}

	return id, nil
}

func (h *Handler) CreateDish(w http.ResponseWriter, r *http.Request) {
	var input DishInput

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	id, err := h.Service.CreateDish(input)
	if err != nil {
		writeProductError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Dish created successfully",
		"id":      id,
	})
}

func (h *Handler) GetDishes(w http.ResponseWriter, r *http.Request) {
	dishes, err := h.Service.GetDishes()
	if err != nil {
		writeProductError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(dishes); err != nil {
		writeProductError(w, err)
	}
}

func (h *Handler) GetPublicDishes(w http.ResponseWriter, r *http.Request) {
	dishes, err := h.Service.GetPublicDishes()
	if err != nil {
		writeProductError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(dishes); err != nil {
		writeProductError(w, err)
	}
}

func (h *Handler) UpdateDish(w http.ResponseWriter, r *http.Request) {
	id, err := parseDishID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var input DishInput

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := h.Service.UpdateDish(id, input); err != nil {
		writeProductError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateDishAvailability(w http.ResponseWriter, r *http.Request) {
	id, err := parseDishID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var update AvailabilityUpdate

	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if update.IsAvailable == nil {
		http.Error(w, "is_available is required", http.StatusBadRequest)
		return
	}

	if err := h.Service.SetDishAvailability(id, *update.IsAvailable); err != nil {
		writeProductError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteDish(w http.ResponseWriter, r *http.Request) {
	id, err := parseDishID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.Service.DeleteDish(id); err != nil {
		writeProductError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetIngredients(w http.ResponseWriter, r *http.Request) {
	ingredients, err := h.Service.GetIngredients()
	if err != nil {
		writeProductError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(ingredients); err != nil {
		writeProductError(w, err)
	}
}

func (h *Handler) CreateIngredient(w http.ResponseWriter, r *http.Request) {
	var input IngredientInput

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	id, err := h.Service.CreateIngredient(input)
	if err != nil {
		writeProductError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Ingredient created successfully",
		"id":      id,
	})
}

func (h *Handler) UpdateIngredient(w http.ResponseWriter, r *http.Request) {
	id, err := parseIngredientID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var input IngredientInput

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := h.Service.UpdateIngredient(id, input); err != nil {
		writeProductError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) DeleteIngredient(w http.ResponseWriter, r *http.Request) {
	id, err := parseIngredientID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.Service.DeleteIngredient(id); err != nil {
		writeProductError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetRecipe(w http.ResponseWriter, r *http.Request) {
	id, err := parseRecipeDishID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	recipe, err := h.Service.GetRecipe(id)
	if err != nil {
		writeProductError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(recipe); err != nil {
		writeProductError(w, err)
	}
}

func (h *Handler) UpdateRecipe(w http.ResponseWriter, r *http.Request) {
	id, err := parseRecipeDishID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var update RecipeUpdate

	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.Service.UpdateRecipe(id, update); err != nil {
		writeProductError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
