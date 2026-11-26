package products

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	maxDishPhotoBytes  = 5 << 20
	dishPhotoURLPrefix = "/media/dishes/"
)

type Handler struct {
	Service   *Service
	UploadDir string
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

func parsePhotoDishID(path string) (int64, error) {
	idValue := strings.TrimSuffix(
		strings.TrimPrefix(path, "/api/admin/dishes/"),
		"/photo",
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

	photoURL, err := h.Service.DeleteDish(id)
	if err != nil {
		writeProductError(w, err)
		return
	}

	h.removeDishPhotoFile(photoURL)

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

func (h *Handler) uploadDir() string {
	if h.UploadDir != "" {
		return h.UploadDir
	}
	return "data/uploads/dishes"
}

func (h *Handler) removeDishPhotoFile(photoURL string) {
	if !strings.HasPrefix(photoURL, dishPhotoURLPrefix) {
		return
	}

	filename := filepath.Base(strings.TrimPrefix(photoURL, dishPhotoURLPrefix))
	if filename == "" || filename == "." || filename == "/" {
		return
	}

	path := filepath.Join(h.uploadDir(), filename)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("remove dish photo %s: %v", path, err)
	}
}

func (h *Handler) UploadDishPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := parsePhotoDishID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxDishPhotoBytes+(1<<20))

	if err := r.ParseMultipartForm(maxDishPhotoBytes); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			http.Error(w, "Файл слишком большой. Максимум 5 МБ.", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Не удалось прочитать файл.", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("photo")
	if err != nil {
		http.Error(w, "Файл не выбран.", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxDishPhotoBytes+1))
	if err != nil {
		http.Error(w, "Не удалось прочитать файл.", http.StatusBadRequest)
		return
	}
	if len(data) == 0 {
		http.Error(w, "Файл пустой.", http.StatusBadRequest)
		return
	}
	if len(data) > maxDishPhotoBytes {
		http.Error(w, "Файл слишком большой. Максимум 5 МБ.", http.StatusRequestEntityTooLarge)
		return
	}

	var extension string
	switch http.DetectContentType(data) {
	case "image/jpeg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	case "image/webp":
		extension = ".webp"
	default:
		http.Error(w, "Поддерживаются только JPEG, PNG и WebP.", http.StatusUnsupportedMediaType)
		return
	}

	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		http.Error(w, "Не удалось сохранить фотографию.", http.StatusInternalServerError)
		return
	}

	filename := hex.EncodeToString(randomBytes) + extension
	dir := h.uploadDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("create dish photo dir: %v", err)
		http.Error(w, "Не удалось сохранить фотографию.", http.StatusInternalServerError)
		return
	}

	fullPath := filepath.Join(dir, filename)
	if err := os.WriteFile(fullPath, data, 0o644); err != nil {
		log.Printf("write dish photo: %v", err)
		http.Error(w, "Не удалось сохранить фотографию.", http.StatusInternalServerError)
		return
	}

	photoURL := dishPhotoURLPrefix + filename
	oldPhotoURL, err := h.Service.SetDishPhoto(id, photoURL)
	if err != nil {
		_ = os.Remove(fullPath)
		writeProductError(w, err)
		return
	}

	if oldPhotoURL != "" && oldPhotoURL != photoURL {
		h.removeDishPhotoFile(oldPhotoURL)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"photo_url": photoURL,
	})
}

func (h *Handler) DeleteDishPhoto(w http.ResponseWriter, r *http.Request) {
	id, err := parsePhotoDishID(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	oldPhotoURL, err := h.Service.ClearDishPhoto(id)
	if err != nil {
		writeProductError(w, err)
		return
	}

	h.removeDishPhotoFile(oldPhotoURL)

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ServeDishPhoto(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if filename == "" || filepath.Base(filename) != filename {
		http.NotFound(w, r)
		return
	}

	path := filepath.Join(h.uploadDir(), filename)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}
