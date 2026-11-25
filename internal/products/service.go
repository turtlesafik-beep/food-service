package products

import (
	"database/sql"
	"fmt"
	"strings"
)

type Service struct {
	DB *sql.DB
}

func (s *Service) CreateDish(dish Dish) (int64, error) {
	dish.Name = strings.TrimSpace(dish.Name)
	dish.Description = strings.TrimSpace(dish.Description)

	if dish.Emoji == "" {
		dish.Emoji = "🍽️"
	}

	if dish.Name == "" {
		return 0, fmt.Errorf("dish name is required")
	}

	if dish.Description == "" {
		return 0, fmt.Errorf("dish description is required")
	}

	if dish.Price < 0 {
		return 0, fmt.Errorf("dish price cannot be negative")
	}

	if dish.WeightGrams != nil && *dish.WeightGrams <= 0 {
		return 0, fmt.Errorf("dish weight must be greater than zero")
	}

	result, err := s.DB.Exec(`
		INSERT INTO dishes (
			name,
			description,
			weight_grams,
			price,
			is_available,
			emoji
		)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		dish.Name,
		dish.Description,
		dish.WeightGrams,
		dish.Price,
		dish.IsAvailable,
		dish.Emoji,
	)

	if err != nil {
		return 0, fmt.Errorf("create dish: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get created dish ID: %w", err)
	}

	return id, nil
}

func (s *Service) GetDishes() ([]Dish, error) {
	rows, err := s.DB.Query(`
		SELECT
			id,
			name,
			description,
			weight_grams,
			price,
			is_available,
			emoji
		FROM dishes
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("get dishes: %w", err)
	}
	defer rows.Close()

	dishes := make([]Dish, 0)

	for rows.Next() {
		var dish Dish
		var weight sql.NullInt64
		var available int

		err := rows.Scan(
			&dish.ID,
			&dish.Name,
			&dish.Description,
			&weight,
			&dish.Price,
			&available,
			&dish.Emoji,
		)
		if err != nil {
			return nil, fmt.Errorf("scan dish: %w", err)
		}

		if weight.Valid {
			value := int(weight.Int64)
			dish.WeightGrams = &value
		}

		dish.IsAvailable = available == 1

		dishes = append(dishes, dish)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dishes: %w", err)
	}

	return dishes, nil
}

func (s *Service) GetPublicDishes() ([]Dish, error) {
	rows, err := s.DB.Query(`
		SELECT
			id,
			name,
			description,
			weight_grams,
			price,
			is_available,
			emoji
		FROM dishes
		WHERE is_available = 1
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("get dishes: %w", err)
	}
	defer rows.Close()

	dishes := make([]Dish, 0)

	for rows.Next() {
		var dish Dish
		var weight sql.NullInt64
		var available int

		err := rows.Scan(
			&dish.ID,
			&dish.Name,
			&dish.Description,
			&weight,
			&dish.Price,
			&available,
			&dish.Emoji,
		)
		if err != nil {
			return nil, fmt.Errorf("scan dish: %w", err)
		}

		if weight.Valid {
			value := int(weight.Int64)
			dish.WeightGrams = &value
		}

		dish.IsAvailable = available == 1

		recipe, err := s.GetRecipe(dish.ID)
		if err != nil {
			return nil, fmt.Errorf("get recipe for dish %d: %w", dish.ID, err)
		}

		dish.Ingredients = recipe

		dishes = append(dishes, dish)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dishes: %w", err)
	}

	return dishes, nil
}

func (s *Service) UpdateDish(id int64, dish Dish) error {
	dish.Name = strings.TrimSpace(dish.Name)
	dish.Description = strings.TrimSpace(dish.Description)

	if dish.Emoji == "" {
		dish.Emoji = "🍽️"
	}

	if dish.Name == "" {
		return fmt.Errorf("dish name is required")
	}

	if dish.Description == "" {
		return fmt.Errorf("dish description is required")
	}

	if dish.Price < 0 {
		return fmt.Errorf("dish price cannot be negative")
	}

	if dish.WeightGrams != nil && *dish.WeightGrams <= 0 {
		return fmt.Errorf("dish weight must be greater than zero")
	}

	result, err := s.DB.Exec(`
		UPDATE dishes
		SET
			name = ?,
			description = ?,
			weight_grams = ?,
			price = ?,
			emoji = ?
		WHERE id = ?
	`,
		dish.Name,
		dish.Description,
		dish.WeightGrams,
		dish.Price,
		dish.Emoji,
		id,
	)

	if err != nil {
		return fmt.Errorf("update dish: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("dish not found")
	}

	return nil
}

func (s *Service) SetDishAvailability(id int64, available bool) error {
	result, err := s.DB.Exec(`
		UPDATE dishes
		SET is_available = ?
		WHERE id = ?
	`, available, id)

	if err != nil {
		return fmt.Errorf("update dish availability: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("dish not found")
	}

	return nil
}

func (s *Service) GetIngredients() ([]Ingredient, error) {
	rows, err := s.DB.Query(`
		SELECT id, name, unit
		FROM ingredients
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("get ingredients: %w", err)
	}
	defer rows.Close()

	ingredients := make([]Ingredient, 0)

	for rows.Next() {
		var ingredient Ingredient

		err := rows.Scan(
			&ingredient.ID,
			&ingredient.Name,
			&ingredient.Unit,
		)
		if err != nil {
			return nil, fmt.Errorf("scan ingredient: %w", err)
		}

		ingredients = append(ingredients, ingredient)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ingredients: %w", err)
	}

	return ingredients, nil
}

func normalizeIngredientName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	return strings.ToLower(name)
}

func (s *Service) CreateIngredient(input IngredientInput) (int64, error) {
	name := strings.Join(strings.Fields(input.Name), " ")
	nameKey := normalizeIngredientName(name)

	if name == "" {
		return 0, fmt.Errorf("ingredient name is required")
	}

	if input.Unit != "g" && input.Unit != "ml" && input.Unit != "pcs" {
		return 0, fmt.Errorf("invalid ingredient unit")
	}

	result, err := s.DB.Exec(`
        INSERT INTO ingredients (name, name_key, unit)
        VALUES (?, ?, ?)
    `, name, nameKey, input.Unit)

	if err != nil {
		return 0, fmt.Errorf("create ingredient: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get created ingredient ID: %w", err)
	}

	return id, nil
}

func (s *Service) GetRecipe(dishID int64) ([]RecipeIngredient, error) {
	rows, err := s.DB.Query(`
		SELECT
			i.id,
			i.name,
			di.quantity,
			i.unit
		FROM dish_ingredients di
		JOIN ingredients i
			ON di.ingredient_id = i.id
		WHERE di.dish_id = ?
		ORDER BY i.name
	`, dishID)

	if err != nil {
		return nil, fmt.Errorf("get recipe: %w", err)
	}
	defer rows.Close()

	recipe := make([]RecipeIngredient, 0)

	for rows.Next() {
		var ingredient RecipeIngredient

		err := rows.Scan(
			&ingredient.IngredientID,
			&ingredient.Name,
			&ingredient.Quantity,
			&ingredient.Unit,
		)
		if err != nil {
			return nil, fmt.Errorf("scan recipe ingredient: %w", err)
		}

		recipe = append(recipe, ingredient)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recipe: %w", err)
	}

	return recipe, nil
}

func (s *Service) UpdateRecipe(dishID int64, update RecipeUpdate) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin recipe transaction: %w", err)
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRow(
		"SELECT 1 FROM dishes WHERE id = ?",
		dishID,
	).Scan(&exists)

	if err == sql.ErrNoRows {
		return fmt.Errorf("dish not found")
	}
	if err != nil {
		return fmt.Errorf("check dish: %w", err)
	}

	type recipeEntry struct {
		ingredientID int64
		quantity     float64
	}

	entries := make([]recipeEntry, 0)
	seen := make(map[string]bool)

	for _, item := range update.Ingredients {
		name := strings.Join(strings.Fields(item.Name), " ")
		nameKey := normalizeIngredientName(name)

		if name == "" {
			return fmt.Errorf("ingredient name is required")
		}
		if item.Quantity <= 0 {
			return fmt.Errorf("ingredient quantity must be greater than zero")
		}
		if item.Unit != "g" && item.Unit != "ml" && item.Unit != "pcs" {
			return fmt.Errorf("invalid ingredient unit")
		}
		if seen[nameKey] {
			return fmt.Errorf("duplicate ingredient: %s", name)
		}
		seen[nameKey] = true

		_, err := tx.Exec(`
			INSERT INTO ingredients (name, name_key, unit)
			VALUES (?, ?, ?)
			ON CONFLICT(name_key) DO NOTHING
		`, name, nameKey, item.Unit)

		if err != nil {
			return fmt.Errorf("create ingredient: %w", err)
		}

		var ingredientID int64
		var existingUnit string

		err = tx.QueryRow(`
			SELECT id, unit
			FROM ingredients
			WHERE name_key = ?
		`, nameKey).Scan(&ingredientID, &existingUnit)

		if err != nil {
			return fmt.Errorf("find ingredient: %w", err)
		}

		if existingUnit != item.Unit {
			return fmt.Errorf("ingredient unit does not match existing ingredient")
		}

		entries = append(entries, recipeEntry{
			ingredientID: ingredientID,
			quantity:     item.Quantity,
		})
	}

	_, err = tx.Exec(`
		DELETE FROM dish_ingredients
		WHERE dish_id = ?
	`, dishID)

	if err != nil {
		return fmt.Errorf("delete old recipe: %w", err)
	}

	for _, entry := range entries {
		_, err := tx.Exec(`
			INSERT INTO dish_ingredients (
				dish_id,
				ingredient_id,
				quantity
			)
			VALUES (?, ?, ?)
		`, dishID, entry.ingredientID, entry.quantity)

		if err != nil {
			return fmt.Errorf("save recipe ingredient: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recipe transaction: %w", err)
	}

	return nil
}
