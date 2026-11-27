package products

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func normalizeCategoryName(name string) (string, string) {
	name = strings.Join(strings.Fields(name), " ")
	return name, strings.ToLower(name)
}

func isValidCategoryMenuType(menuType string) bool {
	switch menuType {
	case MenuTypeHome, MenuTypeBanquet:
		return true
	default:
		return false
	}
}

// categoryMenuTypesForDish returns the menus whose categories can be edited
// for a dish with the given menu type. A hidden dish keeps its assignments and
// uses the menu it will be restored to; links from other menus are preserved
// separately by saveDishCategories.
func categoryMenuTypesForDish(menuType, previousMenuType string) []string {
	switch menuType {
	case MenuTypeHome:
		return []string{MenuTypeHome}
	case MenuTypeBanquet:
		return []string{MenuTypeBanquet}
	case MenuTypeBoth:
		return []string{MenuTypeHome, MenuTypeBanquet}
	case MenuTypeHidden:
		previous := previousMenuTypeOrHome(previousMenuType)
		if previous == MenuTypeBoth {
			return []string{MenuTypeHome, MenuTypeBanquet}
		}
		return []string{previous}
	default:
		return nil
	}
}

func (s *Service) GetCategories(menuType string) ([]Category, error) {
	menuType = strings.TrimSpace(menuType)
	if !isValidCategoryMenuType(menuType) {
		return nil, validationError("invalid category menu type")
	}

	rows, err := s.DB.Query(`
		SELECT c.id, c.name, c.menu_type, COUNT(dc.id)
		FROM categories c
		LEFT JOIN dish_categories dc ON dc.category_id = c.id
		WHERE c.menu_type = ?
		GROUP BY c.id, c.name, c.menu_type
		ORDER BY c.name, c.id
	`, menuType)
	if err != nil {
		return nil, fmt.Errorf("get categories: %w", err)
	}
	defer rows.Close()

	categories := make([]Category, 0)

	for rows.Next() {
		var category Category

		if err := rows.Scan(
			&category.ID,
			&category.Name,
			&category.MenuType,
			&category.DishCount,
		); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}

		categories = append(categories, category)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate categories: %w", err)
	}

	return categories, nil
}

func (s *Service) CreateCategory(input CategoryInput) (int64, error) {
	menuType := strings.TrimSpace(input.MenuType)
	if !isValidCategoryMenuType(menuType) {
		return 0, validationError("invalid category menu type")
	}

	name, nameKey := normalizeCategoryName(input.Name)
	if name == "" {
		return 0, validationError("category name is required")
	}

	var existingID int64
	err := s.DB.QueryRow(`
		SELECT id
		FROM categories
		WHERE menu_type = ? AND name_key = ?
	`, menuType, nameKey).Scan(&existingID)
	if err == nil {
		return 0, conflictError("category %q already exists", name)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("check category: %w", err)
	}

	result, err := s.DB.Exec(`
		INSERT INTO categories (name, name_key, menu_type)
		VALUES (?, ?, ?)
	`, name, nameKey, menuType)
	if err != nil {
		return 0, fmt.Errorf("create category: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get created category ID: %w", err)
	}

	return id, nil
}

func (s *Service) UpdateCategory(id int64, input CategoryRenameInput) error {
	name, nameKey := normalizeCategoryName(input.Name)
	if name == "" {
		return validationError("category name is required")
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin category transaction: %w", err)
	}
	defer tx.Rollback()

	var menuType string
	err = tx.QueryRow(`
		SELECT menu_type
		FROM categories
		WHERE id = ?
	`, id).Scan(&menuType)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCategoryNotFound
	}
	if err != nil {
		return fmt.Errorf("check category: %w", err)
	}

	var duplicateID int64
	err = tx.QueryRow(`
		SELECT id
		FROM categories
		WHERE menu_type = ? AND name_key = ? AND id != ?
	`, menuType, nameKey, id).Scan(&duplicateID)
	if err == nil {
		return conflictError("category %q already exists", name)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check duplicate category: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE categories
		SET name = ?, name_key = ?
		WHERE id = ?
	`, name, nameKey, id); err != nil {
		return fmt.Errorf("update category: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit category transaction: %w", err)
	}

	return nil
}

func (s *Service) DeleteCategory(id int64) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return fmt.Errorf("begin category transaction: %w", err)
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRow(`SELECT 1 FROM categories WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrCategoryNotFound
	}
	if err != nil {
		return fmt.Errorf("check category: %w", err)
	}

	if _, err := tx.Exec(`
		DELETE FROM dish_categories
		WHERE category_id = ?
	`, id); err != nil {
		return fmt.Errorf("delete dish categories: %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM categories WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete category: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit category transaction: %w", err)
	}

	return nil
}

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

// saveDishCategories replaces only the assignments that belong to the menus
// currently available for the dish. Assignments from other menus are left
// untouched so a temporary menu type change never destroys them.
func (s *Service) saveDishCategories(
	tx *sql.Tx,
	dishID int64,
	categoryIDs []int64,
	allowedMenuTypes []string,
) error {
	uniqueIDs := make([]int64, 0, len(categoryIDs))
	seen := make(map[int64]bool, len(categoryIDs))

	for _, id := range categoryIDs {
		if id <= 0 {
			return validationError("invalid category id")
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		uniqueIDs = append(uniqueIDs, id)
	}

	allowed := make(map[string]bool, len(allowedMenuTypes))
	for _, menuType := range allowedMenuTypes {
		allowed[menuType] = true
	}

	if len(uniqueIDs) > 0 {
		args := make([]any, 0, len(uniqueIDs))
		for _, id := range uniqueIDs {
			args = append(args, id)
		}

		rows, err := tx.Query(fmt.Sprintf(`
			SELECT id, menu_type
			FROM categories
			WHERE id IN (%s)
		`, placeholders(len(uniqueIDs))), args...)
		if err != nil {
			return fmt.Errorf("check categories: %w", err)
		}

		found := make(map[int64]string, len(uniqueIDs))
		for rows.Next() {
			var (
				categoryID   int64
				categoryMenu string
			)
			if err := rows.Scan(&categoryID, &categoryMenu); err != nil {
				rows.Close()
				return fmt.Errorf("scan category: %w", err)
			}
			found[categoryID] = categoryMenu
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate categories: %w", err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close category rows: %w", err)
		}

		for _, id := range uniqueIDs {
			menuType, ok := found[id]
			if !ok {
				return validationError("category not found: %d", id)
			}
			if !allowed[menuType] {
				return validationError("category %d is not available for this menu type", id)
			}
		}
	}

	if len(allowedMenuTypes) == 0 {
		if len(uniqueIDs) > 0 {
			return validationError("categories are not available for this menu type")
		}
		return nil
	}

	menuArgs := make([]any, 0, len(allowedMenuTypes)+1)
	menuArgs = append(menuArgs, dishID)
	for _, menuType := range allowedMenuTypes {
		menuArgs = append(menuArgs, menuType)
	}

	if _, err := tx.Exec(fmt.Sprintf(`
		DELETE FROM dish_categories
		WHERE dish_id = ?
		  AND category_id IN (
			SELECT id FROM categories WHERE menu_type IN (%s)
		  )
	`, placeholders(len(allowedMenuTypes))), menuArgs...); err != nil {
		return fmt.Errorf("delete old dish categories: %w", err)
	}

	for _, categoryID := range uniqueIDs {
		if _, err := tx.Exec(`
			INSERT INTO dish_categories (dish_id, category_id)
			VALUES (?, ?)
		`, dishID, categoryID); err != nil {
			return fmt.Errorf("save dish category: %w", err)
		}
	}

	return nil
}

func (s *Service) getDishCategoryIDs(dishID int64, categoryMenuType string) ([]int64, error) {
	query := `
		SELECT c.id
		FROM dish_categories dc
		JOIN categories c ON c.id = dc.category_id
		WHERE dc.dish_id = ?
	`
	args := []any{dishID}

	if categoryMenuType != "" {
		query += " AND c.menu_type = ?"
		args = append(args, categoryMenuType)
	}

	query += " ORDER BY c.id"

	rows, err := s.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get dish categories: %w", err)
	}
	defer rows.Close()

	ids := make([]int64, 0)

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan dish category: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dish categories: %w", err)
	}

	return ids, nil
}

func (s *Service) attachAdminDishCategories(dishes []Dish) error {
	for i := range dishes {
		ids, err := s.getDishCategoryIDs(dishes[i].ID, "")
		if err != nil {
			return err
		}
		dishes[i].CategoryIDs = ids
	}
	return nil
}

func publicCategoryMenuType(requestedMenu, dishMenuType string) string {
	switch requestedMenu {
	case MenuTypeHome:
		return MenuTypeHome
	case MenuTypeBanquet:
		return MenuTypeBanquet
	}

	switch dishMenuType {
	case MenuTypeHome:
		return MenuTypeHome
	case MenuTypeBanquet:
		return MenuTypeBanquet
	default:
		return ""
	}
}

func (s *Service) attachPublicDishCategories(dishes []Dish, requestedMenu string) error {
	for i := range dishes {
		menuType := publicCategoryMenuType(requestedMenu, dishes[i].MenuType)
		ids, err := s.getDishCategoryIDs(dishes[i].ID, menuType)
		if err != nil {
			return err
		}
		dishes[i].CategoryIDs = ids
	}
	return nil
}
