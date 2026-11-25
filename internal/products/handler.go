package products

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	Service *Service
}

func (h *Handler) CreateDish(w http.ResponseWriter, r *http.Request) {
	var dish Dish

	err := json.NewDecoder(r.Body).Decode(&dish)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	id, err := h.Service.CreateDish(dish)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
		http.Error(w, "Failed to get dishes", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(dishes)
	if err != nil {
		http.Error(w, "Failed to encode dishes", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) GetPublicDishes(w http.ResponseWriter, r *http.Request) {
	dishes, err := h.Service.GetPublicDishes()
	if err != nil {
		http.Error(w, "Failed to get dishes", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(dishes)
	if err != nil {
		http.Error(w, "Failed to encode dishes", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) UpdateDish(w http.ResponseWriter, r *http.Request) {
	idValue := strings.TrimPrefix(r.URL.Path, "/api/admin/dishes/")

	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Invalid dish ID", http.StatusBadRequest)
		return
	}

	var dish Dish

	err = json.NewDecoder(r.Body).Decode(&dish)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	err = h.Service.UpdateDish(id, dish)
	if err != nil {
		if err.Error() == "dish not found" {
			http.Error(w, "Dish not found", http.StatusNotFound)
			return
		}

		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateDishAvailability(w http.ResponseWriter, r *http.Request) {
	idValue := strings.TrimPrefix(r.URL.Path, "/api/admin/dishes/")

	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Invalid dish ID", http.StatusBadRequest)
		return
	}

	var update AvailabilityUpdate

	err = json.NewDecoder(r.Body).Decode(&update)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if update.IsAvailable == nil {
		http.Error(w, "is_available is required", http.StatusBadRequest)
		return
	}

	err = h.Service.SetDishAvailability(id, *update.IsAvailable)
	if err != nil {
		if err.Error() == "dish not found" {
			http.Error(w, "Dish not found", http.StatusNotFound)
			return
		}

		http.Error(w, "Failed to update dish availability", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GetIngredients(w http.ResponseWriter, r *http.Request) {
	ingredients, err := h.Service.GetIngredients()
	if err != nil {
		http.Error(w, "Failed to get ingredients", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(ingredients)
	if err != nil {
		http.Error(w, "Failed to encode ingredients", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) CreateIngredient(w http.ResponseWriter, r *http.Request) {
	var input IngredientInput

	err := json.NewDecoder(r.Body).Decode(&input)
	if err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	id, err := h.Service.CreateIngredient(input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Ingredient created successfully",
		"id":      id,
	})
}

func (h *Handler) GetRecipe(w http.ResponseWriter, r *http.Request) {
	idValue := strings.TrimSuffix(
		strings.TrimPrefix(r.URL.Path, "/api/admin/dishes/"),
		"/recipe",
	)

	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Invalid dish ID", http.StatusBadRequest)
		return
	}

	recipe, err := h.Service.GetRecipe(id)
	if err != nil {
		http.Error(w, "Failed to get recipe", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	err = json.NewEncoder(w).Encode(recipe)
	if err != nil {
		http.Error(w, "Failed to encode recipe", http.StatusInternalServerError)
		return
	}
}

func (h *Handler) UpdateRecipe(w http.ResponseWriter, r *http.Request) {
	idValue := strings.TrimSuffix(
		strings.TrimPrefix(r.URL.Path, "/api/admin/dishes/"),
		"/recipe",
	)

	id, err := strconv.ParseInt(idValue, 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Invalid dish ID", http.StatusBadRequest)
		return
	}

	var update RecipeUpdate

	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	err = h.Service.UpdateRecipe(id, update)
	if err != nil {
		if err.Error() == "dish not found" {
			http.Error(w, "Dish not found", http.StatusNotFound)
			return
		}

		if strings.HasPrefix(err.Error(), "begin recipe transaction:") ||
			strings.HasPrefix(err.Error(), "check dish:") ||
			strings.HasPrefix(err.Error(), "create ingredient:") ||
			strings.HasPrefix(err.Error(), "find ingredient:") ||
			strings.HasPrefix(err.Error(), "delete old recipe:") ||
			strings.HasPrefix(err.Error(), "save recipe ingredient:") ||
			strings.HasPrefix(err.Error(), "commit recipe transaction:") {
			http.Error(w, "Failed to update recipe", http.StatusInternalServerError)
			return
		}

		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
