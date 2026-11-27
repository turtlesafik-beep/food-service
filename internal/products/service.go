package products

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type Service struct {
	DB *sql.DB
}

type recipeEntry struct {
	ingredientID int64
	quantity     float64
}

func normalizeDishInput(input DishInput) (DishInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)

	if input.Emoji == "" {
		input.Emoji = "🍽️"
	}

	if input.Name == "" {
		return input, validationError("dish name is required")
	}

	if input.Description == "" {
		return input, validationError("dish description is required")
	}

	if input.Price < 0 {
		return input, validationError("dish price cannot be negative")
	}

	if input.WeightGrams != nil && *input.WeightGrams <= 0 {
		return input, validationError("dish weight must be greater than zero")
	}

	if input.MenuType != nil {
		menuType := strings.TrimSpace(*input.MenuType)
		if menuType == "" {
			// Пустое значение трактуем как «не указано»: при создании будет
			// использовано значение по умолчанию, при редактировании — текущее.
			input.MenuType = nil
		} else {
			if !isValidMenuType(menuType) {
				return input, validationError("invalid menu type")
			}
			input.MenuType = &menuType
		}
	}

	return input, nil
}

func isValidMenuType(menuType string) bool {
	switch menuType {
	case MenuTypeHome, MenuTypeBanquet, MenuTypeBoth, MenuTypeHidden:
		return true
	default:
		return false
	}
}

// previousMenuTypeOrHome returns a valid visible menu type to restore after a
// dish has been hidden. Old hidden dishes may not have a saved previous value.
func previousMenuTypeOrHome(menuType string) string {
	switch menuType {
	case MenuTypeHome, MenuTypeBanquet, MenuTypeBoth:
		return menuType
	default:
		return DefaultMenuType
	}
}

func validateUnit(unit string) error {
	switch unit {
	case "g", "ml", "pcs":
		return nil
	default:
		return validationError("invalid ingredient unit")
	}
}

func resolveRecipe(tx *sql.Tx, inputs []RecipeIngredientInput) ([]recipeEntry, error) {
	if len(inputs) == 0 {
		return nil, validationError("recipe must contain at least one ingredient")
	}

	entries := make([]recipeEntry, 0, len(inputs))
	seen := make(map[string]bool)

	for _, item := range inputs {
		name := strings.Join(strings.Fields(item.Name), " ")
		nameKey := normalizeIngredientName(name)

		if name == "" {
			return nil, validationError("ingredient name is required")
		}
		if item.Quantity <= 0 {
			return nil, validationError("ingredient quantity must be greater than zero")
		}
		if err := validateUnit(item.Unit); err != nil {
			return nil, err
		}
		if seen[nameKey] {
			return nil, validationError("duplicate ingredient: %s", name)
		}
		seen[nameKey] = true

		_, err := tx.Exec(`
			INSERT INTO ingredients (name, name_key, unit)
			VALUES (?, ?, ?)
			ON CONFLICT(name_key) DO NOTHING
		`, name, nameKey, item.Unit)
		if err != nil {
			return nil, fmt.Errorf("create ingredient: %w", err)
		}

		var ingredientID int64
		var existingUnit string

		err = tx.QueryRow(`
			SELECT id, unit
			FROM ingredients
			WHERE name_key = ?
		`, nameKey).Scan(&ingredientID, &existingUnit)
		if err != nil {
			return nil, fmt.Errorf("find ingredient: %w", err)
		}

		if existingUnit != item.Unit {
			return nil, validationError(
				"ingredient %q already exists with unit %q, cannot use unit %q",
				name, existingUnit, item.Unit,
			)
		}

		entries = append(entries, recipeEntry{
			ingredientID: ingredientID,
			quantity:     item.Quantity,
		})
	}

	return entries, nil
}

func saveRecipe(tx *sql.Tx, dishID int64, entries []recipeEntry) error {
	_, err := tx.Exec(`
		DELETE FROM dish_ingredients
		WHERE dish_id = ?
	`, dishID)
	if err != nil {
		return fmt.Errorf("delete old recipe: %w", err)
	}

	for _, entry := range entries {
		_, err := tx.Exec(`
			INSERT INTO dish_ingredients (dish_id, ingredient_id, quantity)
			VALUES (?, ?, ?)
		`, dishID, entry.ingredientID, entry.quantity)
		if err != nil {
			return fmt.Errorf("save recipe ingredient: %w", err)
		}
	}

	return nil
}

func countRecipe(tx *sql.Tx, dishID int64) (int, error) {
	var count int
	if err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM dish_ingredients
		WHERE dish_id = ?
	`, dishID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func scanDishRows(rows *sql.Rows) ([]Dish, error) {
	dishes := make([]Dish, 0)

	for rows.Next() {
		var dish Dish
		var weight sql.NullInt64
		var available int
		var hasRecipe int

		err := rows.Scan(
			&dish.ID,
			&dish.Name,
			&dish.Description,
			&weight,
			&dish.Price,
			&available,
			&dish.MenuType,
			&dish.PreviousMenuType,
			&hasRecipe,
			&dish.Emoji,
			&dish.PhotoURL,
		)
		if err != nil {
			return nil, fmt.Errorf("scan dish: %w", err)
		}

		if weight.Valid {
			value := int(weight.Int64)
			dish.WeightGrams = &value
		}

		dish.IsAvailable = available == 1
		dish.HasRecipe = hasRecipe == 1

		dishes = append(dishes, dish)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dishes: %w", err)
	}

	return dishes, nil
}

func (s *Service) CreateDish(input DishInput) (int64, error) {
	input, err := normalizeDishInput(input)
	if err != nil {
		return 0, err
	}

	isAvailable := len(input.Ingredients) > 0
	if input.IsAvailable != nil {
		isAvailable = *input.IsAvailable
	}
	if isAvailable && len(input.Ingredients) == 0 {
		return 0, validationError("cannot publish a dish without ingredients")
	}

	menuType := DefaultMenuType
	if input.MenuType != nil {
		menuType = *input.MenuType
	}

	previousMenuType := menuType
	if menuType == MenuTypeHidden {
		previousMenuType = DefaultMenuType
	}
	if !isAvailable || menuType == MenuTypeHidden {
		if menuType != MenuTypeHidden {
			previousMenuType = menuType
		}
		menuType = MenuTypeHidden
		isAvailable = false
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin dish transaction: %w", err)
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		INSERT INTO dishes (
			name, description, weight_grams, price, is_available, menu_type,
			previous_menu_type, emoji
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		input.Name,
		input.Description,
		input.WeightGrams,
		input.Price,
		isAvailable,
		menuType,
		previousMenuType,
		input.Emoji,
	)
	if err != nil {
		return 0, fmt.Errorf("create dish: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get created dish ID: %w", err)
	}

	if len(input.Ingredients) > 0 {
		entries, err := resolveRecipe(tx, input.Ingredients)
		if err != nil {
			return 0, err
		}
		if err := saveRecipe(tx, id, entries); err != nil {
			return 0, err
		}
	}

	if input.CategoryIDs != nil {
		allowedMenuTypes := categoryMenuTypesForDish(menuType, previousMenuType)
		if err := s.saveDishCategories(tx, id, *input.CategoryIDs, allowedMenuTypes); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit dish transaction: %w", err)
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
			menu_type,
			previous_menu_type,
			EXISTS(
				SELECT 1 FROM dish_ingredients di WHERE di.dish_id = dishes.id
			) AS has_recipe,
			emoji,
			photo_url
		FROM dishes
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("get dishes: %w", err)
	}
	defer rows.Close()

	dishes, err := scanDishRows(rows)
	if err != nil {
		return nil, err
	}

	if err := s.attachAdminDishCategories(dishes); err != nil {
		return nil, err
	}

	return dishes, nil
}

// GetPublicDishes returns published dishes for the public catalog. An empty
// menu means "all visible dishes", which keeps the pre-existing /api/dishes
// behaviour. Explicit home/banquet requests include dishes shared by both menus.
func (s *Service) GetPublicDishes(menu string) ([]Dish, error) {
	menu = strings.TrimSpace(menu)

	filter := ""
	args := make([]any, 0, 2)

	switch menu {
	case "", "all":
		// Все опубликованные блюда, кроме скрытых.
	case MenuTypeHome:
		filter = " AND menu_type IN (?, ?)"
		args = append(args, MenuTypeHome, MenuTypeBoth)
	case MenuTypeBanquet:
		filter = " AND menu_type IN (?, ?)"
		args = append(args, MenuTypeBanquet, MenuTypeBoth)
	case MenuTypeHidden:
		// Скрытые блюда не попадают в публичный каталог ни в каком меню.
		filter = " AND 0"
	default:
		return nil, validationError("invalid menu type")
	}

	rows, err := s.DB.Query(`
		SELECT
			id,
			name,
			description,
			weight_grams,
			price,
			is_available,
			menu_type,
			previous_menu_type,
			EXISTS(
				SELECT 1 FROM dish_ingredients di WHERE di.dish_id = dishes.id
			) AS has_recipe,
			emoji,
			photo_url
		FROM dishes
		WHERE is_available = 1
		  AND menu_type <> 'hidden'
		  AND EXISTS(
			SELECT 1 FROM dish_ingredients di WHERE di.dish_id = dishes.id
		  )`+filter+`
		ORDER BY id
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("get public dishes: %w", err)
	}

	dishes, err := scanDishRows(rows)
	if err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close dish rows: %w", err)
	}

	if err := s.attachPublicDishCategories(dishes, menu); err != nil {
		return nil, err
	}

	for i := range dishes {
		recipe, err := s.GetRecipe(dishes[i].ID)
		if err != nil {
			return nil, fmt.Errorf("get recipe for dish %d: %w", dishes[i].ID, err)
		}
		dishes[i].Ingredients = recipe
	}

	return dishes, nil
}

func (s *Service) UpdateDish(id int64, input DishInput) error {
	input, err := normalizeDishInput(input)
	if err != nil {
		return err
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin dish transaction: %w", err)
	}
	defer tx.Rollback()

	var currentAvailable int
	var currentMenuType, currentPreviousMenuType string
	err = tx.QueryRow(`
		SELECT is_available, menu_type, previous_menu_type
		FROM dishes
		WHERE id = ?
	`, id).Scan(&currentAvailable, &currentMenuType, &currentPreviousMenuType)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDishNotFound
	}
	if err != nil {
		return fmt.Errorf("check dish: %w", err)
	}

	recipeCount := 0
	if input.Ingredients != nil {
		if len(input.Ingredients) > 0 {
			entries, err := resolveRecipe(tx, input.Ingredients)
			if err != nil {
				return err
			}
			if err := saveRecipe(tx, id, entries); err != nil {
				return err
			}
			recipeCount = len(entries)
		} else {
			// Очистить рецепт можно, но только если блюдо не публикуется.
			if err := saveRecipe(tx, id, nil); err != nil {
				return err
			}
			recipeCount = 0
		}
	} else {
		recipeCount, err = countRecipe(tx, id)
		if err != nil {
			return fmt.Errorf("count recipe: %w", err)
		}
	}

	isAvailable := currentAvailable == 1
	if input.IsAvailable != nil {
		isAvailable = *input.IsAvailable
	}

	// Selecting a visible menu for a hidden dish is an explicit request to
	// return it to the catalog, even if is_available was omitted.
	if input.IsAvailable == nil && input.MenuType != nil &&
		*input.MenuType != MenuTypeHidden && currentMenuType == MenuTypeHidden {
		isAvailable = true
	}

	menuType := currentMenuType
	if input.MenuType != nil {
		menuType = *input.MenuType
	}

	previousMenuType := previousMenuTypeOrHome(currentPreviousMenuType)
	if menuType != MenuTypeHidden {
		previousMenuType = menuType
	}

	// Both ways of hiding a dish are kept in sync: "hidden" means the dish is
	// unpublished, and unpublishing a dish moves it to the hidden menu.
	if !isAvailable || menuType == MenuTypeHidden {
		if currentMenuType != MenuTypeHidden {
			if menuType != MenuTypeHidden {
				previousMenuType = menuType
			} else {
				previousMenuType = previousMenuTypeOrHome(currentMenuType)
			}
		}
		menuType = MenuTypeHidden
		isAvailable = false
	}

	if isAvailable && recipeCount == 0 {
		return validationError("cannot publish a dish without ingredients")
	}

	result, err := tx.Exec(`
		UPDATE dishes
		SET
			name = ?,
			description = ?,
			weight_grams = ?,
			price = ?,
			is_available = ?,
			menu_type = ?,
			previous_menu_type = ?,
			emoji = ?
		WHERE id = ?
	`,
		input.Name,
		input.Description,
		input.WeightGrams,
		input.Price,
		isAvailable,
		menuType,
		previousMenuType,
		input.Emoji,
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
		return ErrDishNotFound
	}

	if input.CategoryIDs != nil {
		allowedMenuTypes := categoryMenuTypesForDish(menuType, previousMenuType)
		if err := s.saveDishCategories(tx, id, *input.CategoryIDs, allowedMenuTypes); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dish transaction: %w", err)
	}

	return nil
}

func (s *Service) SetDishAvailability(id int64, available bool) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin dish transaction: %w", err)
	}
	defer tx.Rollback()

	var currentMenuType, currentPreviousMenuType string
	err = tx.QueryRow(`
		SELECT menu_type, previous_menu_type
		FROM dishes
		WHERE id = ?
	`, id).Scan(&currentMenuType, &currentPreviousMenuType)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDishNotFound
	}
	if err != nil {
		return fmt.Errorf("check dish: %w", err)
	}

	menuType := currentMenuType
	previousMenuType := previousMenuTypeOrHome(currentPreviousMenuType)

	if available {
		if menuType == MenuTypeHidden {
			menuType = previousMenuType
		}
		previousMenuType = menuType

		count, err := countRecipe(tx, id)
		if err != nil {
			return fmt.Errorf("count recipe: %w", err)
		}
		if count == 0 {
			return validationError("cannot publish a dish without ingredients")
		}
	} else {
		if menuType != MenuTypeHidden {
			previousMenuType = menuType
		}
		menuType = MenuTypeHidden
	}

	if _, err := tx.Exec(`
		UPDATE dishes
		SET is_available = ?, menu_type = ?, previous_menu_type = ?
		WHERE id = ?
	`, available, menuType, previousMenuType, id); err != nil {
		return fmt.Errorf("update dish availability: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dish transaction: %w", err)
	}

	return nil
}

func (s *Service) DeleteDish(id int64) (string, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return "", fmt.Errorf("begin dish transaction: %w", err)
	}
	defer tx.Rollback()

	var photoURL string
	err = tx.QueryRow(`SELECT photo_url FROM dishes WHERE id = ?`, id).Scan(&photoURL)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrDishNotFound
	}
	if err != nil {
		return "", fmt.Errorf("check dish: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE order_items
		SET dish_id = NULL
		WHERE dish_id = ?
	`, id); err != nil {
		return "", fmt.Errorf("detach dish from orders: %w", err)
	}

	if _, err := tx.Exec(`
		DELETE FROM dish_ingredients
		WHERE dish_id = ?
	`, id); err != nil {
		return "", fmt.Errorf("delete dish recipe: %w", err)
	}

	if _, err := tx.Exec(`
		DELETE FROM dish_categories
		WHERE dish_id = ?
	`, id); err != nil {
		return "", fmt.Errorf("delete dish categories: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM dishes WHERE id = ?`, id); err != nil {
		return "", fmt.Errorf("delete dish: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit dish transaction: %w", err)
	}

	return photoURL, nil
}

func (s *Service) updateDishPhoto(id int64, photoURL string) (string, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return "", fmt.Errorf("begin dish photo transaction: %w", err)
	}
	defer tx.Rollback()

	var oldPhotoURL string
	err = tx.QueryRow(`SELECT photo_url FROM dishes WHERE id = ?`, id).Scan(&oldPhotoURL)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrDishNotFound
	}
	if err != nil {
		return "", fmt.Errorf("check dish: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE dishes
		SET photo_url = ?
		WHERE id = ?
	`, photoURL, id); err != nil {
		return "", fmt.Errorf("update dish photo: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit dish photo transaction: %w", err)
	}

	return oldPhotoURL, nil
}

func (s *Service) SetDishPhoto(id int64, photoURL string) (string, error) {
	return s.updateDishPhoto(id, photoURL)
}

func (s *Service) ClearDishPhoto(id int64) (string, error) {
	return s.updateDishPhoto(id, "")
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
		return 0, validationError("ingredient name is required")
	}
	if err := validateUnit(input.Unit); err != nil {
		return 0, err
	}

	var existingID int64
	err := s.DB.QueryRow(`
		SELECT id
		FROM ingredients
		WHERE name_key = ?
	`, nameKey).Scan(&existingID)
	if err == nil {
		return 0, conflictError("ingredient %q already exists", name)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("check ingredient: %w", err)
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

func (s *Service) UpdateIngredient(id int64, input IngredientInput) error {
	name := strings.Join(strings.Fields(input.Name), " ")
	nameKey := normalizeIngredientName(name)

	if name == "" {
		return validationError("ingredient name is required")
	}
	if err := validateUnit(input.Unit); err != nil {
		return err
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin ingredient transaction: %w", err)
	}
	defer tx.Rollback()

	var currentUnit string
	err = tx.QueryRow(`
		SELECT unit
		FROM ingredients
		WHERE id = ?
	`, id).Scan(&currentUnit)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrIngredientNotFound
	}
	if err != nil {
		return fmt.Errorf("check ingredient: %w", err)
	}

	var duplicateID int64
	err = tx.QueryRow(`
		SELECT id
		FROM ingredients
		WHERE name_key = ? AND id != ?
	`, nameKey, id).Scan(&duplicateID)
	if err == nil {
		return conflictError("ingredient %q already exists", name)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check duplicate ingredient: %w", err)
	}

	if currentUnit != input.Unit {
		var usage int
		if err := tx.QueryRow(`
			SELECT COUNT(*)
			FROM dish_ingredients
			WHERE ingredient_id = ?
		`, id).Scan(&usage); err != nil {
			return fmt.Errorf("count ingredient usage: %w", err)
		}

		if usage > 0 {
			return conflictError(
				"cannot change unit: ingredient is used in %d dish(es)",
				usage,
			)
		}
	}

	if _, err := tx.Exec(`
		UPDATE ingredients
		SET name = ?, name_key = ?, unit = ?
		WHERE id = ?
	`, name, nameKey, input.Unit, id); err != nil {
		return fmt.Errorf("update ingredient: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ingredient transaction: %w", err)
	}

	return nil
}

func (s *Service) DeleteIngredient(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin ingredient transaction: %w", err)
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRow(`SELECT 1 FROM ingredients WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrIngredientNotFound
	}
	if err != nil {
		return fmt.Errorf("check ingredient: %w", err)
	}

	var usage int
	if err := tx.QueryRow(`
		SELECT COUNT(*)
		FROM dish_ingredients
		WHERE ingredient_id = ?
	`, id).Scan(&usage); err != nil {
		return fmt.Errorf("count ingredient usage: %w", err)
	}

	if usage > 0 {
		return conflictError(
			"cannot delete ingredient: it is used in %d dish(es)",
			usage,
		)
	}

	if _, err := tx.Exec(`DELETE FROM ingredients WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete ingredient: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit ingredient transaction: %w", err)
	}

	return nil
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
	if len(update.Ingredients) == 0 {
		return validationError("recipe must contain at least one ingredient")
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin recipe transaction: %w", err)
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRow(`SELECT 1 FROM dishes WHERE id = ?`, dishID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrDishNotFound
	}
	if err != nil {
		return fmt.Errorf("check dish: %w", err)
	}

	entries, err := resolveRecipe(tx, update.Ingredients)
	if err != nil {
		return err
	}
	if err := saveRecipe(tx, dishID, entries); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recipe transaction: %w", err)
	}

	return nil
}
