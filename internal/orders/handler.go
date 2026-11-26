package orders

import (
	"encoding/json"
	"net/http"
	"strconv"
)

type Handler struct {
	Service *Service
}

func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var request CreateOrderRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	result, err := h.Service.CreateOrder(request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)

	json.NewEncoder(w).Encode(result)
}

func (h *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")

	if token == "" {
		http.Error(w, "Order token is required", http.StatusBadRequest)
		return
	}

	order, err := h.Service.GetOrder(token)
	if err != nil {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	json.NewEncoder(w).Encode(order)
}

func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	limit := 10
	offset := 0

	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 || parsed > 100 {
			http.Error(w, "Invalid limit", http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	if value := r.URL.Query().Get("offset"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 {
			http.Error(w, "Invalid offset", http.StatusBadRequest)
			return
		}
		offset = parsed
	}

	orders, err := h.Service.GetOrders(limit, offset, r.URL.Query().Get("q"))
	if err != nil {
		http.Error(w, "Failed to get orders", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(orders)
}

func (h *Handler) GetAdminOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "Invalid order ID", http.StatusBadRequest)
		return
	}

	order, err := h.Service.GetAdminOrder(id)
	if err != nil {
		http.Error(w, "Order not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(order)
}
