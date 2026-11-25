package orders

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

type Service struct {
	DB *sql.DB
}

func generateToken() (string, string, error) {
	randomBytes := make([]byte, 32)

	if _, err := rand.Read(randomBytes); err != nil {
		return "", "", fmt.Errorf("generate random token: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(randomBytes)

	hash := sha256.Sum256([]byte(token))
	tokenHash := fmt.Sprintf("%x", hash)

	return token, tokenHash, nil
}

func validateOrderRequest(request CreateOrderRequest) error {
	if request.CustomerName == "" {
		return fmt.Errorf("customer name is required")
	}

	if request.CustomerPhone == "" {
		return fmt.Errorf("customer phone is required")
	}

	if len(request.Items) == 0 {
		return fmt.Errorf("order must contain at least one item")
	}

	seenDishes := make(map[int64]bool)

	for _, item := range request.Items {
		if item.DishID <= 0 {
			return fmt.Errorf("dish ID must be positive")
		}

		if item.Quantity <= 0 {
			return fmt.Errorf("quantity must be positive")
		}

		if seenDishes[item.DishID] {
			return fmt.Errorf("duplicate dish ID: %d", item.DishID)
		}

		seenDishes[item.DishID] = true
	}

	return nil
}

func (s *Service) CreateOrder(request CreateOrderRequest) (CreateOrderResult, error) {
	request.CustomerName = strings.TrimSpace(request.CustomerName)
	request.CustomerPhone = strings.TrimSpace(request.CustomerPhone)
	request.Comment = strings.TrimSpace(request.Comment)
	if err := validateOrderRequest(request); err != nil {
		return CreateOrderResult{}, err
	}

	token, tokenHash, err := generateToken()
	if err != nil {
		return CreateOrderResult{}, err
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return CreateOrderResult{}, fmt.Errorf("begin order transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(
		`INSERT INTO orders (
        		token_hash,
        		customer_name,
        		customer_phone,
        		comment
    		) VALUES (?, ?, ?, ?)`,
		tokenHash,
		request.CustomerName,
		request.CustomerPhone,
		request.Comment,
	)
	if err != nil {
		return CreateOrderResult{}, fmt.Errorf("create order: %w", err)
	}

	orderID, err := result.LastInsertId()
	if err != nil {
		return CreateOrderResult{}, fmt.Errorf("get order ID: %w", err)
	}

	for _, item := range request.Items {
		var (
			dishName    string
			weightGrams sql.NullInt64
			price       int64
			isAvailable bool
		)

		err := tx.QueryRow(
			`SELECT name, weight_grams, price, is_available
		 FROM dishes
		 WHERE id = ?`,
			item.DishID,
		).Scan(&dishName, &weightGrams, &price, &isAvailable)

		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return CreateOrderResult{}, fmt.Errorf("dish not found: %d", item.DishID)
			}
			return CreateOrderResult{}, fmt.Errorf("get dish: %w", err)
		}

		if !isAvailable {
			return CreateOrderResult{}, fmt.Errorf("dish is unavailable: %d", item.DishID)
		}

		itemResult, err := tx.Exec(
			`INSERT INTO order_items (
			order_id, dish_id, dish_name, weight_grams, quantity, unit_price
			) VALUES (?, ?, ?, ?, ?, ?)`,
			orderID,
			item.DishID,
			dishName,
			weightGrams,
			item.Quantity,
			price,
		)
		if err != nil {
			return CreateOrderResult{}, fmt.Errorf("save order item: %w", err)
		}

		orderItemID, err := itemResult.LastInsertId()
		if err != nil {
			return CreateOrderResult{}, fmt.Errorf("get order item ID: %w", err)
		}

		rows, err := tx.Query(
			`SELECT i.name, i.unit, di.quantity
	 		FROM dish_ingredients di
	 		JOIN ingredients i ON i.id = di.ingredient_id
	 		WHERE di.dish_id = ?`,
			item.DishID,
		)
		if err != nil {
			return CreateOrderResult{}, fmt.Errorf("get dish recipe: %w", err)
		}

		type recipeIngredient struct {
			name     string
			unit     string
			quantity float64
		}

		var recipe []recipeIngredient

		for rows.Next() {
			var ingredient recipeIngredient

			err := rows.Scan(
				&ingredient.name,
				&ingredient.unit,
				&ingredient.quantity,
			)
			if err != nil {
				rows.Close()
				return CreateOrderResult{}, fmt.Errorf("read dish recipe: %w", err)
			}

			recipe = append(recipe, ingredient)
		}

		if err := rows.Err(); err != nil {
			rows.Close()
			return CreateOrderResult{}, fmt.Errorf("iterate dish recipe: %w", err)
		}

		if err := rows.Close(); err != nil {
			return CreateOrderResult{}, fmt.Errorf("close recipe rows: %w", err)
		}

		if len(recipe) == 0 {
			return CreateOrderResult{}, fmt.Errorf("dish has no ingredients: %d", item.DishID)
		}

		for _, ingredient := range recipe {
			totalQuantity := ingredient.quantity * float64(item.Quantity)

			_, err := tx.Exec(
				`INSERT INTO order_ingredients (
				order_item_id, ingredient_name, unit, quantity
				) VALUES (?, ?, ?, ?)`,
				orderItemID,
				ingredient.name,
				ingredient.unit,
				totalQuantity,
			)
			if err != nil {
				return CreateOrderResult{}, fmt.Errorf("save order ingredient: %w", err)
			}
		}

	}

	if err := tx.Commit(); err != nil {
		return CreateOrderResult{}, fmt.Errorf("commit order transaction: %w", err)
	}

	return CreateOrderResult{
		OrderID: orderID,
		Token:   token,
	}, nil
}

func (s *Service) GetOrder(token string) (OrderResponse, error) {
	var order OrderResponse

	tokenHash := sha256.Sum256([]byte(token))
	hash := fmt.Sprintf("%x", tokenHash)

	err := s.DB.QueryRow(
		`SELECT id, created_at
		FROM orders
		WHERE token_hash = ?`,
		hash,
	).Scan(&order.OrderID, &order.CreatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return OrderResponse{}, fmt.Errorf("order not found")
		}
		return OrderResponse{}, fmt.Errorf("get order: %w", err)
	}

	// Получаем блюда заказа.
	rows, err := s.DB.Query(
		`SELECT dish_name, weight_grams, quantity, unit_price
		FROM order_items
		WHERE order_id = ?`,
		order.OrderID,
	)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("get order items: %w", err)
	}

	order.Items = make([]OrderItem, 0)

	for rows.Next() {
		var item OrderItem

		var weightGrams sql.NullInt64

		if err := rows.Scan(
			&item.DishName,
			&weightGrams,
			&item.Quantity,
			&item.UnitPrice,
		); err != nil {
			rows.Close()
			return OrderResponse{}, fmt.Errorf("read order item: %w", err)
		}

		if weightGrams.Valid {
			weight := int(weightGrams.Int64)
			item.WeightGrams = &weight
		}

		item.LineTotal = item.UnitPrice * int64(item.Quantity)
		order.Total += item.LineTotal

		order.Items = append(order.Items, item)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return OrderResponse{}, fmt.Errorf("iterate order items: %w", err)
	}

	if err := rows.Close(); err != nil {
		return OrderResponse{}, fmt.Errorf("close order item rows: %w", err)
	}

	// Получаем ингредиенты и объединяем одинаковые.
	rows, err = s.DB.Query(
		`SELECT oi.ingredient_name, oi.unit, oi.quantity
		FROM order_ingredients oi
		JOIN order_items items ON items.id = oi.order_item_id
		WHERE items.order_id = ?`,
		order.OrderID,
	)
	if err != nil {
		return OrderResponse{}, fmt.Errorf("get order ingredients: %w", err)
	}

	type ingredientKey struct {
		name string
		unit string
	}

	ingredientTotals := make(map[ingredientKey]float64)

	for rows.Next() {
		var ingredient OrderIngredient

		if err := rows.Scan(
			&ingredient.Name,
			&ingredient.Unit,
			&ingredient.Quantity,
		); err != nil {
			rows.Close()
			return OrderResponse{}, fmt.Errorf("read order ingredient: %w", err)
		}

		key := ingredientKey{
			name: ingredient.Name,
			unit: ingredient.Unit,
		}

		ingredientTotals[key] += ingredient.Quantity
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return OrderResponse{}, fmt.Errorf("iterate order ingredients: %w", err)
	}

	if err := rows.Close(); err != nil {
		return OrderResponse{}, fmt.Errorf("close order ingredient rows: %w", err)
	}

	order.Ingredients = make([]OrderIngredient, 0, len(ingredientTotals))

	for key, quantity := range ingredientTotals {
		order.Ingredients = append(order.Ingredients, OrderIngredient{
			Name:     key.name,
			Unit:     key.unit,
			Quantity: quantity,
		})
	}

	return order, nil
}

func (s *Service) GetOrders(limit, offset int) ([]AdminOrder, error) {
	rows, err := s.DB.Query(`
		SELECT
			o.id,
			o.created_at,
			o.customer_name,
			COALESCE(SUM(oi.unit_price * oi.quantity), 0) AS total
		FROM orders o
		LEFT JOIN order_items oi ON oi.order_id = o.id
		GROUP BY o.id
		ORDER BY o.id DESC
		LIMIT ? OFFSET ?
	`, limit, offset)

	if err != nil {
		return nil, fmt.Errorf("get orders: %w", err)
	}
	defer rows.Close()

	orders := make([]AdminOrder, 0)

	for rows.Next() {
		var order AdminOrder

		err := rows.Scan(
			&order.OrderID,
			&order.CreatedAt,
			&order.CustomerName,
			&order.Total,
		)
		if err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}

		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate orders: %w", err)
	}

	return orders, nil
}

func (s *Service) GetAdminOrder(id int64) (AdminOrderResponse, error) {
	var order AdminOrderResponse

	err := s.DB.QueryRow(`
		SELECT id, created_at, customer_name, customer_phone, comment
		FROM orders
		WHERE id = ?
	`, id).Scan(
		&order.OrderID,
		&order.CreatedAt,
		&order.CustomerName,
		&order.CustomerPhone,
		&order.Comment,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AdminOrderResponse{}, fmt.Errorf("order not found")
		}
		return AdminOrderResponse{}, fmt.Errorf("get order: %w", err)
	}

	// Получаем блюда заказа.
	rows, err := s.DB.Query(`
		SELECT dish_name, weight_grams, quantity, unit_price
		FROM order_items
		WHERE order_id = ?
	`, id)
	if err != nil {
		return AdminOrderResponse{}, fmt.Errorf("get order items: %w", err)
	}

	order.Items = make([]OrderItem, 0)

	for rows.Next() {
		var item OrderItem
		var weightGrams sql.NullInt64

		if err := rows.Scan(
			&item.DishName,
			&weightGrams,
			&item.Quantity,
			&item.UnitPrice,
		); err != nil {
			rows.Close()
			return AdminOrderResponse{}, fmt.Errorf("read order item: %w", err)
		}

		if weightGrams.Valid {
			weight := int(weightGrams.Int64)
			item.WeightGrams = &weight
		}

		item.LineTotal = item.UnitPrice * int64(item.Quantity)
		order.Total += item.LineTotal

		order.Items = append(order.Items, item)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOrderResponse{}, fmt.Errorf("iterate order items: %w", err)
	}

	if err := rows.Close(); err != nil {
		return AdminOrderResponse{}, fmt.Errorf("close order item rows: %w", err)
	}

	// Получаем ингредиенты и объединяем одинаковые.
	rows, err = s.DB.Query(`
		SELECT oi.ingredient_name, oi.unit, oi.quantity
		FROM order_ingredients oi
		JOIN order_items items ON items.id = oi.order_item_id
		WHERE items.order_id = ?
	`, id)
	if err != nil {
		return AdminOrderResponse{}, fmt.Errorf("get order ingredients: %w", err)
	}

	type ingredientKey struct {
		name string
		unit string
	}

	ingredientTotals := make(map[ingredientKey]float64)

	for rows.Next() {
		var ingredient OrderIngredient

		if err := rows.Scan(
			&ingredient.Name,
			&ingredient.Unit,
			&ingredient.Quantity,
		); err != nil {
			rows.Close()
			return AdminOrderResponse{}, fmt.Errorf("read order ingredient: %w", err)
		}

		key := ingredientKey{
			name: ingredient.Name,
			unit: ingredient.Unit,
		}

		ingredientTotals[key] += ingredient.Quantity
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return AdminOrderResponse{}, fmt.Errorf("iterate order ingredients: %w", err)
	}

	if err := rows.Close(); err != nil {
		return AdminOrderResponse{}, fmt.Errorf("close order ingredient rows: %w", err)
	}

	order.Ingredients = make([]OrderIngredient, 0, len(ingredientTotals))

	for key, quantity := range ingredientTotals {
		order.Ingredients = append(order.Ingredients, OrderIngredient{
			Name:     key.name,
			Unit:     key.unit,
			Quantity: quantity,
		})
	}

	return order, nil
}
