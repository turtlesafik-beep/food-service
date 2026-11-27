package products

import (
	"testing"
)

func createTestCategory(t *testing.T, service *Service, name, menuType string) int64 {
	t.Helper()

	id, err := service.CreateCategory(CategoryInput{
		Name:     name,
		MenuType: menuType,
	})
	if err != nil {
		t.Fatalf("create category %q for %q: %v", name, menuType, err)
	}

	return id
}

func categoryIDsPtr(ids ...int64) *[]int64 {
	return &ids
}

func createTestDishWithCategories(
	t *testing.T,
	service *Service,
	name string,
	menuType *string,
	categoryIDs *[]int64,
) int64 {
	t.Helper()

	id, err := service.CreateDish(DishInput{
		Name:        name,
		Description: "test dish",
		Price:       100,
		MenuType:    menuType,
		CategoryIDs: categoryIDs,
		Ingredients: []RecipeIngredientInput{
			{Name: "Test ingredient", Quantity: 100, Unit: "g"},
		},
	})
	if err != nil {
		t.Fatalf("create dish %q: %v", name, err)
	}

	return id
}

func updateTestDishMenu(
	t *testing.T,
	service *Service,
	id int64,
	menuType string,
	categoryIDs *[]int64,
) {
	t.Helper()

	err := service.UpdateDish(id, DishInput{
		Name:        "dish",
		Description: "test dish",
		Price:       100,
		MenuType:    stringPtr(menuType),
		CategoryIDs: categoryIDs,
	})
	if err != nil {
		t.Fatalf("update dish %d menu type %q: %v", id, menuType, err)
	}
}

func publicDish(t *testing.T, service *Service, menu string, id int64) Dish {
	t.Helper()

	dishes, err := service.GetPublicDishes(menu)
	if err != nil {
		t.Fatalf("get public dishes for %q: %v", menu, err)
	}

	for _, dish := range dishes {
		if dish.ID == id {
			return dish
		}
	}

	t.Fatalf("dish %d not found in public menu %q", id, menu)
	return Dish{}
}

func hasID(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestCategoriesAreIndependentPerMenu(t *testing.T) {
	service := newTestService(t)

	homeID := createTestCategory(t, service, "  Обед  ", MenuTypeHome)
	banquetID := createTestCategory(t, service, "Обед", MenuTypeBanquet)

	if homeID == banquetID {
		t.Fatal("categories in different menus must have different IDs")
	}

	homeCategories, err := service.GetCategories(MenuTypeHome)
	if err != nil {
		t.Fatalf("get home categories: %v", err)
	}
	if len(homeCategories) != 1 || homeCategories[0].ID != homeID || homeCategories[0].Name != "Обед" {
		t.Fatalf("unexpected home categories: %+v", homeCategories)
	}

	banquetCategories, err := service.GetCategories(MenuTypeBanquet)
	if err != nil {
		t.Fatalf("get banquet categories: %v", err)
	}
	if len(banquetCategories) != 1 || banquetCategories[0].ID != banquetID {
		t.Fatalf("unexpected banquet categories: %+v", banquetCategories)
	}

	if _, err := service.CreateCategory(CategoryInput{Name: " обед ", MenuType: MenuTypeHome}); err == nil {
		t.Fatal("duplicate category in the same menu must be rejected")
	}

	if _, err := service.CreateCategory(CategoryInput{Name: "   ", MenuType: MenuTypeHome}); err == nil {
		t.Fatal("empty category name must be rejected")
	}

	if _, err := service.CreateCategory(CategoryInput{Name: "Новое", MenuType: MenuTypeBoth}); err == nil {
		t.Fatal("invalid category menu type must be rejected")
	}

	if _, err := service.GetCategories("both"); err == nil {
		t.Fatal("invalid category menu query must be rejected")
	}
}

func TestUpdateCategoryKeepsDishLinks(t *testing.T) {
	service := newTestService(t)

	categoryID := createTestCategory(t, service, "Обед", MenuTypeHome)
	dishID := createTestDishWithCategories(
		t,
		service,
		"Суп",
		stringPtr(MenuTypeHome),
		categoryIDsPtr(categoryID),
	)

	if err := service.UpdateCategory(categoryID, CategoryRenameInput{Name: "Ужин"}); err != nil {
		t.Fatalf("rename category: %v", err)
	}

	dish := getTestDish(t, service, dishID)
	if !hasID(dish.CategoryIDs, categoryID) {
		t.Fatalf("dish categories after rename = %v, want category %d", dish.CategoryIDs, categoryID)
	}

	public := publicDish(t, service, MenuTypeHome, dishID)
	if !hasID(public.CategoryIDs, categoryID) {
		t.Fatalf("public dish categories after rename = %v, want category %d", public.CategoryIDs, categoryID)
	}

	categories, err := service.GetCategories(MenuTypeHome)
	if err != nil {
		t.Fatalf("get categories: %v", err)
	}
	if len(categories) != 1 || categories[0].Name != "Ужин" {
		t.Fatalf("renamed category not returned: %+v", categories)
	}
}

func TestDeleteCategoryKeepsDish(t *testing.T) {
	service := newTestService(t)

	categoryID := createTestCategory(t, service, "Обед", MenuTypeHome)
	dishID := createTestDishWithCategories(
		t,
		service,
		"Суп",
		stringPtr(MenuTypeHome),
		categoryIDsPtr(categoryID),
	)

	if err := service.DeleteCategory(categoryID); err != nil {
		t.Fatalf("delete category: %v", err)
	}

	dish := getTestDish(t, service, dishID)
	if len(dish.CategoryIDs) != 0 {
		t.Fatalf("dish categories after category deletion = %v, want none", dish.CategoryIDs)
	}

	if !publicDishIDs(t, service, MenuTypeHome)[dishID] {
		t.Fatal("dish must stay in the public catalog after category deletion")
	}

	if err := service.DeleteCategory(categoryID); err != ErrCategoryNotFound {
		t.Fatalf("second delete error = %v, want ErrCategoryNotFound", err)
	}
}

func TestDishCategoryAssignmentRules(t *testing.T) {
	service := newTestService(t)

	homeCategory := createTestCategory(t, service, "Обед", MenuTypeHome)
	homeCategory2 := createTestCategory(t, service, "Завтрак", MenuTypeHome)
	banquetCategory := createTestCategory(t, service, "Фуршет", MenuTypeBanquet)

	homeDish := createTestDishWithCategories(
		t,
		service,
		"home",
		stringPtr(MenuTypeHome),
		categoryIDsPtr(homeCategory, homeCategory2),
	)
	banquetDish := createTestDishWithCategories(
		t,
		service,
		"banquet",
		stringPtr(MenuTypeBanquet),
		categoryIDsPtr(banquetCategory),
	)
	bothDish := createTestDishWithCategories(
		t,
		service,
		"both",
		stringPtr(MenuTypeBoth),
		categoryIDsPtr(homeCategory, banquetCategory),
	)
	noCategoriesDish := createTestDishWithCategories(
		t,
		service,
		"none",
		stringPtr(MenuTypeHome),
		categoryIDsPtr(),
	)

	if got := getTestDish(t, service, homeDish).CategoryIDs; len(got) != 2 {
		t.Fatalf("home dish categories = %v, want two categories", got)
	}
	if got := getTestDish(t, service, banquetDish).CategoryIDs; len(got) != 1 || got[0] != banquetCategory {
		t.Fatalf("banquet dish categories = %v, want [%d]", got, banquetCategory)
	}
	if got := getTestDish(t, service, bothDish).CategoryIDs; len(got) != 2 {
		t.Fatalf("both dish categories = %v, want two categories", got)
	}
	if got := getTestDish(t, service, noCategoriesDish).CategoryIDs; len(got) != 0 {
		t.Fatalf("dish without categories has %v, want none", got)
	}

	if _, err := service.CreateDish(DishInput{
		Name:        "wrong home",
		Description: "test dish",
		Price:       100,
		MenuType:    stringPtr(MenuTypeHome),
		CategoryIDs: categoryIDsPtr(banquetCategory),
		Ingredients: []RecipeIngredientInput{
			{Name: "Test ingredient", Quantity: 100, Unit: "g"},
		},
	}); err == nil {
		t.Fatal("banquet category must not be assigned to a home dish")
	}

	if _, err := service.CreateDish(DishInput{
		Name:        "wrong banquet",
		Description: "test dish",
		Price:       100,
		MenuType:    stringPtr(MenuTypeBanquet),
		CategoryIDs: categoryIDsPtr(homeCategory),
		Ingredients: []RecipeIngredientInput{
			{Name: "Test ingredient", Quantity: 100, Unit: "g"},
		},
	}); err == nil {
		t.Fatal("home category must not be assigned to a banquet dish")
	}

	if _, err := service.CreateDish(DishInput{
		Name:        "unknown category",
		Description: "test dish",
		Price:       100,
		MenuType:    stringPtr(MenuTypeHome),
		CategoryIDs: categoryIDsPtr(999999),
		Ingredients: []RecipeIngredientInput{
			{Name: "Test ingredient", Quantity: 100, Unit: "g"},
		},
	}); err == nil {
		t.Fatal("unknown category ID must be rejected")
	}
}

func TestDishCategoryMenuTypeChangePreservesLinks(t *testing.T) {
	service := newTestService(t)

	homeCategory := createTestCategory(t, service, "Обед", MenuTypeHome)
	banquetCategory := createTestCategory(t, service, "Фуршет", MenuTypeBanquet)

	dishID := createTestDishWithCategories(
		t,
		service,
		"both",
		stringPtr(MenuTypeBoth),
		categoryIDsPtr(homeCategory, banquetCategory),
	)

	// Форма блюда home отправляет только домашние категории. Банкетная связь
	// должна сохраниться в базе, хотя публично больше не возвращается.
	updateTestDishMenu(t, service, dishID, MenuTypeHome, categoryIDsPtr(homeCategory))

	dish := getTestDish(t, service, dishID)
	if !hasID(dish.CategoryIDs, homeCategory) || !hasID(dish.CategoryIDs, banquetCategory) {
		t.Fatalf("category links after switching to home = %v, want both links preserved", dish.CategoryIDs)
	}

	homePublic := publicDish(t, service, MenuTypeHome, dishID)
	if !hasID(homePublic.CategoryIDs, homeCategory) || hasID(homePublic.CategoryIDs, banquetCategory) {
		t.Fatalf("home public categories = %v, want only home category", homePublic.CategoryIDs)
	}

	// Возврат в both делает сохранённую банкетную категорию снова доступной.
	updateTestDishMenu(
		t,
		service,
		dishID,
		MenuTypeBoth,
		categoryIDsPtr(homeCategory, banquetCategory),
	)

	homePublic = publicDish(t, service, MenuTypeHome, dishID)
	banquetPublic := publicDish(t, service, MenuTypeBanquet, dishID)

	if !hasID(homePublic.CategoryIDs, homeCategory) || hasID(homePublic.CategoryIDs, banquetCategory) {
		t.Fatalf("home public categories after restoring both = %v", homePublic.CategoryIDs)
	}
	if !hasID(banquetPublic.CategoryIDs, banquetCategory) || hasID(banquetPublic.CategoryIDs, homeCategory) {
		t.Fatalf("banquet public categories after restoring both = %v", banquetPublic.CategoryIDs)
	}
}

func TestPublicDishCategoriesFilteredByMenu(t *testing.T) {
	service := newTestService(t)

	homeCategory := createTestCategory(t, service, "Обед", MenuTypeHome)
	banquetCategory := createTestCategory(t, service, "Фуршет", MenuTypeBanquet)

	bothID := createTestDishWithCategories(
		t,
		service,
		"both",
		stringPtr(MenuTypeBoth),
		categoryIDsPtr(homeCategory, banquetCategory),
	)
	homeID := createTestDishWithCategories(
		t,
		service,
		"home",
		stringPtr(MenuTypeHome),
		categoryIDsPtr(homeCategory),
	)

	homePublic := publicDish(t, service, MenuTypeHome, bothID)
	if !hasID(homePublic.CategoryIDs, homeCategory) || hasID(homePublic.CategoryIDs, banquetCategory) {
		t.Fatalf("both dish home categories = %v", homePublic.CategoryIDs)
	}

	banquetPublic := publicDish(t, service, MenuTypeBanquet, bothID)
	if !hasID(banquetPublic.CategoryIDs, banquetCategory) || hasID(banquetPublic.CategoryIDs, homeCategory) {
		t.Fatalf("both dish banquet categories = %v", banquetPublic.CategoryIDs)
	}

	if !publicDishIDs(t, service, MenuTypeHome)[homeID] {
		t.Fatal("home dish must be present in the home public menu")
	}
	if publicDishIDs(t, service, MenuTypeBanquet)[homeID] {
		t.Fatal("home dish must not be present in the banquet public menu")
	}
}

func TestHiddenDishCategoriesArePreservedAndNotPublic(t *testing.T) {
	service := newTestService(t)

	homeCategory := createTestCategory(t, service, "Обед", MenuTypeHome)
	banquetCategory := createTestCategory(t, service, "Фуршет", MenuTypeBanquet)

	dishID := createTestDishWithCategories(
		t,
		service,
		"both",
		stringPtr(MenuTypeBoth),
		categoryIDsPtr(homeCategory, banquetCategory),
	)

	updateTestDishMenu(t, service, dishID, MenuTypeHidden, nil)

	dish := getTestDish(t, service, dishID)
	if dish.MenuType != MenuTypeHidden || dish.IsAvailable {
		t.Fatalf("hidden dish = menu %q, available %v", dish.MenuType, dish.IsAvailable)
	}
	if !hasID(dish.CategoryIDs, homeCategory) || !hasID(dish.CategoryIDs, banquetCategory) {
		t.Fatalf("hidden dish category links = %v, want both preserved", dish.CategoryIDs)
	}
	if publicDishIDs(t, service, "")[dishID] {
		t.Fatal("hidden dish must not be in the public catalog")
	}

	updateTestDishMenu(
		t,
		service,
		dishID,
		MenuTypeBoth,
		categoryIDsPtr(homeCategory, banquetCategory),
	)
	if !publicDishIDs(t, service, MenuTypeHome)[dishID] ||
		!publicDishIDs(t, service, MenuTypeBanquet)[dishID] {
		t.Fatal("restored dish must be visible in both public menus")
	}
}
