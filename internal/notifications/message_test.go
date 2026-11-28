package notifications

import (
	"fmt"
	"strings"
	"testing"

	"food-service/internal/orders"
)

func TestBuildOrderMessagesFormatsAndEscapes(t *testing.T) {
	weight := 250
	order := orders.AdminOrderResponse{
		OrderID:       42,
		CreatedAt:     "2026-11-28 09:31:34",
		CustomerName:  `Илья <b> & "гость"`,
		CustomerPhone: "+7 900 000-00-00",
		Comment:       "Без <script> & острого",
		Token:         "token_abc",
		Items: []orders.OrderItem{
			{
				DishName:    "Борщ <острый> & вкусный",
				WeightGrams: &weight,
				Quantity:    2,
				UnitPrice:   300,
				LineTotal:   600,
			},
		},
		Ingredients: []orders.OrderIngredient{
			{Name: "Свёкла", Unit: "g", Quantity: 400.5},
			{Name: "Капуста", Unit: "g", Quantity: 300},
			{Name: "Молоко", Unit: "ml", Quantity: 50},
		},
		Total: 600,
	}

	parts := BuildOrderMessages(order, "https://sdahub.ru/")
	if len(parts) != 1 {
		t.Fatalf("parts = %d, want 1", len(parts))
	}
	message := parts[0]

	checks := []string{
		"Новый заказ #42",
		"28.11.2026 12:31",
		"Илья &lt;b&gt; &amp; \"гость\"",
		"Без &lt;script&gt; &amp; острого",
		"Борщ &lt;острый&gt; &amp; вкусный (250 г) — 2 порц. × 300\u00a0₽ = 600\u00a0₽",
		"Свёкла — 400,5 г",
		"Капуста — 300 г",
		"Молоко — 50 мл",
		"Итого: 600\u00a0₽",
		"https://sdahub.ru/order/token_abc",
	}
	for _, check := range checks {
		if !strings.Contains(message, check) {
			t.Errorf("message does not contain %q:\n%s", check, message)
		}
	}

	if strings.Contains(message, "<script>") {
		t.Fatalf("raw user HTML was not escaped:\n%s", message)
	}

	// Ingredients must be sorted by name.
	cabbage := strings.Index(message, "Капуста")
	milk := strings.Index(message, "Молоко")
	beetroot := strings.Index(message, "Свёкла")
	if !(cabbage < milk && milk < beetroot) {
		t.Fatalf("ingredients are not sorted: %d, %d, %d\n%s", cabbage, milk, beetroot, message)
	}
}

func TestBuildOrderMessagesUsesAggregatedSnapshots(t *testing.T) {
	db := newTestDB(t)

	result, err := db.Exec(
		`INSERT INTO orders (token_hash, token, customer_name, customer_phone, comment)
		 VALUES ('hash', 'token', 'Илья', '+70000000000', '')`,
	)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	orderID, _ := result.LastInsertId()

	itemIDs := make([]int64, 0, 2)
	for _, item := range []struct {
		name     string
		weight   int
		quantity int
		price    int
	}{
		{name: "Борщ", weight: 250, quantity: 2, price: 300},
		{name: "Салат", weight: 150, quantity: 1, price: 200},
	} {
		result, err := db.Exec(
			`INSERT INTO order_items (order_id, dish_name, weight_grams, quantity, unit_price)
			 VALUES (?, ?, ?, ?, ?)`,
			orderID,
			item.name,
			item.weight,
			item.quantity,
			item.price,
		)
		if err != nil {
			t.Fatalf("insert order item: %v", err)
		}
		itemID, _ := result.LastInsertId()
		itemIDs = append(itemIDs, itemID)
	}

	ingredients := []struct {
		itemIndex int
		name      string
		unit      string
		quantity  float64
	}{
		{itemIndex: 0, name: "Свёкла", unit: "g", quantity: 200},
		{itemIndex: 1, name: "Свёкла", unit: "g", quantity: 100},
		{itemIndex: 1, name: "Молоко", unit: "ml", quantity: 50},
	}
	for _, ingredient := range ingredients {
		if _, err := db.Exec(
			`INSERT INTO order_ingredients (order_item_id, ingredient_name, unit, quantity)
			 VALUES (?, ?, ?, ?)`,
			itemIDs[ingredient.itemIndex],
			ingredient.name,
			ingredient.unit,
			ingredient.quantity,
		); err != nil {
			t.Fatalf("insert order ingredient: %v", err)
		}
	}

	service := &orders.Service{DB: db}
	adminOrder, err := service.GetAdminOrder(orderID)
	if err != nil {
		t.Fatalf("get admin order: %v", err)
	}

	parts := BuildOrderMessages(adminOrder, "https://example.test")
	if len(parts) != 1 {
		t.Fatalf("parts = %d, want 1", len(parts))
	}
	message := parts[0]

	checks := []string{
		"2 порц. × 300\u00a0₽ = 600\u00a0₽",
		"1 порц. × 200\u00a0₽ = 200\u00a0₽",
		"Свёкла — 300 г",
		"Молоко — 50 мл",
		"Итого: 800\u00a0₽",
	}
	for _, check := range checks {
		if !strings.Contains(message, check) {
			t.Errorf("message does not contain %q:\n%s", check, message)
		}
	}

	if strings.Index(message, "Молоко") > strings.Index(message, "Свёкла") {
		t.Fatalf("ingredients are not sorted:\n%s", message)
	}
}

func TestBuildOrderMessagesSplitsLongMessages(t *testing.T) {
	order := orders.AdminOrderResponse{
		OrderID:       7,
		CreatedAt:     "2026-11-28 09:31:34",
		CustomerName:  "Илья",
		CustomerPhone: "+70000000000",
		Comment:       strings.Repeat("длинный ", 1000),
		Token:         "token_abc",
		Items: []orders.OrderItem{
			{DishName: "Борщ", Quantity: 1, UnitPrice: 300, LineTotal: 300},
		},
		Ingredients: []orders.OrderIngredient{
			{Name: "Свёкла", Unit: "g", Quantity: 100},
		},
		Total: 300,
	}

	parts := BuildOrderMessages(order, "https://example.test")
	if len(parts) < 2 {
		t.Fatalf("parts = %d, want more than 1", len(parts))
	}

	for i, part := range parts {
		if length := messageLength(part); length > telegramMessageLimit {
			t.Fatalf("part %d length = %d, want <= %d", i+1, length, telegramMessageLimit)
		}
		header := fmt.Sprintf("часть %d/%d", i+1, len(parts))
		if !strings.Contains(part, header) {
			t.Fatalf("part %d does not contain %q", i+1, header)
		}
	}

	joined := strings.Join(parts, "\n")
	if count := strings.Count(joined, "длинный"); count != 1000 {
		t.Fatalf("comment words = %d, want 1000", count)
	}

	last := parts[len(parts)-1]
	if !strings.Contains(last, "Итого: 300\u00a0₽") {
		t.Fatalf("last part does not contain the total:\n%s", last)
	}
	if !strings.Contains(last, "https://example.test/order/token_abc") {
		t.Fatalf("last part does not contain the order link:\n%s", last)
	}
}
