package orders

type CreateOrderRequest struct {
	CustomerName  string           `json:"customer_name"`
	CustomerPhone string           `json:"customer_phone"`
	Comment       string           `json:"comment"`
	Items         []OrderItemInput `json:"items"`
}

type OrderItemInput struct {
	DishID   int64 `json:"dish_id"`
	Quantity int   `json:"quantity"`
}

type CreateOrderResult struct {
	OrderID int64  `json:"order_id"`
	Token   string `json:"token"`
}

type OrderResponse struct {
	OrderID     int64             `json:"order_id"`
	CreatedAt   string            `json:"created_at"`
	Items       []OrderItem       `json:"items"`
	Ingredients []OrderIngredient `json:"ingredients"`
	Total       int64             `json:"total"`
}

type OrderItem struct {
	DishName    string `json:"dish_name"`
	WeightGrams *int   `json:"weight_grams"`
	Quantity    int    `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
	LineTotal   int64  `json:"line_total"`
}

type OrderIngredient struct {
	Name     string  `json:"name"`
	Unit     string  `json:"unit"`
	Quantity float64 `json:"quantity"`
}

type AdminOrder struct {
	OrderID       int64  `json:"order_id"`
	CreatedAt     string `json:"created_at"`
	CustomerName  string `json:"customer_name"`
	CustomerPhone string `json:"customer_phone"`
	Token         string `json:"token"`
	ItemCount     int    `json:"item_count"`
	Total         int64  `json:"total"`
}

type AdminOrderResponse struct {
	OrderID       int64             `json:"order_id"`
	CreatedAt     string            `json:"created_at"`
	CustomerName  string            `json:"customer_name"`
	CustomerPhone string            `json:"customer_phone"`
	Comment       string            `json:"comment"`
	Token         string            `json:"token"`
	Items         []OrderItem       `json:"items"`
	Ingredients   []OrderIngredient `json:"ingredients"`
	Total         int64             `json:"total"`
}
