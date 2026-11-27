package products

import (
	"database/sql"
	"path/filepath"
	"testing"

	"food-service/internal/database"
)

func newTestService(t *testing.T) *Service {
	t.Helper()

	db, err := sql.Open(
		"sqlite",
		"file:"+filepath.Join(t.TempDir(), "test.db")+"?_pragma=foreign_keys(1)",
	)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	if err := database.InitSchema(db); err != nil {
		t.Fatalf("init test schema: %v", err)
	}

	return &Service{DB: db}
}

func createTestDish(t *testing.T, service *Service, name string, menuType *string) int64 {
	t.Helper()

	id, err := service.CreateDish(DishInput{
		Name:        name,
		Description: "test dish",
		Price:       100,
		MenuType:    menuType,
		Ingredients: []RecipeIngredientInput{
			{Name: "Test ingredient", Quantity: 100, Unit: "g"},
		},
	})
	if err != nil {
		t.Fatalf("create dish %q: %v", name, err)
	}

	return id
}

func stringPtr(value string) *string {
	return &value
}

func dishMenuTypes(t *testing.T, service *Service) map[int64]string {
	t.Helper()

	dishes, err := service.GetDishes()
	if err != nil {
		t.Fatalf("get dishes: %v", err)
	}

	result := make(map[int64]string, len(dishes))
	for _, dish := range dishes {
		result[dish.ID] = dish.MenuType
	}

	return result
}

func TestCreateDishMenuTypes(t *testing.T) {
	service := newTestService(t)

	homeID := createTestDish(t, service, "home", stringPtr(MenuTypeHome))
	banquetID := createTestDish(t, service, "banquet", stringPtr(MenuTypeBanquet))
	bothID := createTestDish(t, service, "both", stringPtr(MenuTypeBoth))
	hiddenID := createTestDish(t, service, "hidden", stringPtr(MenuTypeHidden))
	defaultID := createTestDish(t, service, "default", nil)

	menuTypes := dishMenuTypes(t, service)
	expected := map[int64]string{
		homeID:    MenuTypeHome,
		banquetID: MenuTypeBanquet,
		bothID:    MenuTypeBoth,
		hiddenID:  MenuTypeHidden,
		defaultID: DefaultMenuType,
	}

	for id, want := range expected {
		if got := menuTypes[id]; got != want {
			t.Errorf("dish %d menu_type = %q, want %q", id, got, want)
		}
	}
}

func TestUpdateDishMenuType(t *testing.T) {
	service := newTestService(t)

	id := createTestDish(t, service, "dish", stringPtr(MenuTypeHome))

	// Редактирование без menu_type не должно сбрасывать текущее значение.
	if err := service.UpdateDish(id, DishInput{
		Name:        "dish",
		Description: "updated",
		Price:       100,
	}); err != nil {
		t.Fatalf("update dish without menu type: %v", err)
	}
	if got := dishMenuTypes(t, service)[id]; got != MenuTypeHome {
		t.Fatalf("menu_type after update without field = %q, want %q", got, MenuTypeHome)
	}

	if err := service.UpdateDish(id, DishInput{
		Name:        "dish",
		Description: "updated",
		Price:       100,
		MenuType:    stringPtr(MenuTypeBanquet),
	}); err != nil {
		t.Fatalf("update dish menu type: %v", err)
	}
	if got := dishMenuTypes(t, service)[id]; got != MenuTypeBanquet {
		t.Fatalf("menu_type after update = %q, want %q", got, MenuTypeBanquet)
	}
}

func TestInvalidDishMenuType(t *testing.T) {
	service := newTestService(t)

	_, err := service.CreateDish(DishInput{
		Name:        "invalid",
		Description: "invalid menu type",
		Price:       100,
		MenuType:    stringPtr("lunch"),
		Ingredients: []RecipeIngredientInput{
			{Name: "Test ingredient", Quantity: 100, Unit: "g"},
		},
	})
	if err == nil {
		t.Fatal("expected validation error for invalid menu type")
	}
}

func TestGetPublicDishesMenuFilters(t *testing.T) {
	service := newTestService(t)

	homeID := createTestDish(t, service, "home", stringPtr(MenuTypeHome))
	banquetID := createTestDish(t, service, "banquet", stringPtr(MenuTypeBanquet))
	bothID := createTestDish(t, service, "both", stringPtr(MenuTypeBoth))
	createTestDish(t, service, "hidden", stringPtr(MenuTypeHidden))

	publicIDs := func(menu string) map[int64]bool {
		t.Helper()

		dishes, err := service.GetPublicDishes(menu)
		if err != nil {
			t.Fatalf("get public dishes for %q: %v", menu, err)
		}

		result := make(map[int64]bool, len(dishes))
		for _, dish := range dishes {
			result[dish.ID] = true
		}
		return result
	}

	all := publicIDs("")
	for _, id := range []int64{homeID, banquetID, bothID} {
		if !all[id] {
			t.Errorf("default public catalog missing dish %d", id)
		}
	}
	if len(all) != 3 {
		t.Errorf("default public catalog has %d dishes, want 3 (hidden must be excluded)", len(all))
	}

	home := publicIDs(MenuTypeHome)
	if !home[homeID] || !home[bothID] || home[banquetID] {
		t.Errorf("home menu has unexpected dishes: %v", home)
	}

	banquet := publicIDs(MenuTypeBanquet)
	if !banquet[banquetID] || !banquet[bothID] || banquet[homeID] {
		t.Errorf("banquet menu has unexpected dishes: %v", banquet)
	}

	if hidden := publicIDs(MenuTypeHidden); len(hidden) != 0 {
		t.Errorf("hidden menu must be empty, got %v", hidden)
	}

	if _, err := service.GetPublicDishes("lunch"); err == nil {
		t.Error("expected validation error for unknown public menu")
	}
}

func TestInitSchemaAddsMenuTypeToExistingDishes(t *testing.T) {
	db, err := sql.Open(
		"sqlite",
		"file:"+filepath.Join(t.TempDir(), "legacy.db")+"?_pragma=foreign_keys(1)",
	)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE dishes (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			weight_grams INTEGER CHECK(weight_grams > 0),
			price INTEGER NOT NULL CHECK(price >= 0),
			is_available INTEGER NOT NULL DEFAULT 1,
			emoji TEXT NOT NULL DEFAULT '🍽️',
			photo_url TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO dishes (name, description, price)
		VALUES ('legacy dish', 'created before menu types', 100);
	`)
	if err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	if err := database.InitSchema(db); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}

	var menuType string
	if err := db.QueryRow(`SELECT menu_type FROM dishes WHERE name = 'legacy dish'`).Scan(&menuType); err != nil {
		t.Fatalf("read migrated menu type: %v", err)
	}
	if menuType != DefaultMenuType {
		t.Fatalf("legacy dish menu_type = %q, want %q", menuType, DefaultMenuType)
	}
}

func getTestDish(t *testing.T, service *Service, id int64) Dish {
	t.Helper()

	dishes, err := service.GetDishes()
	if err != nil {
		t.Fatalf("get dishes: %v", err)
	}

	for _, dish := range dishes {
		if dish.ID == id {
			return dish
		}
	}

	t.Fatalf("dish %d not found", id)
	return Dish{}
}

func publicDishIDs(t *testing.T, service *Service, menu string) map[int64]bool {
	t.Helper()

	dishes, err := service.GetPublicDishes(menu)
	if err != nil {
		t.Fatalf("get public dishes for %q: %v", menu, err)
	}

	result := make(map[int64]bool, len(dishes))
	for _, dish := range dishes {
		result[dish.ID] = true
	}
	return result
}

func TestUpdateDishHiddenUnpublishesAndRemembersMenu(t *testing.T) {
	service := newTestService(t)

	id := createTestDish(t, service, "both", stringPtr(MenuTypeBoth))

	if err := service.UpdateDish(id, DishInput{
		Name:        "both",
		Description: "updated",
		Price:       100,
		MenuType:    stringPtr(MenuTypeHidden),
	}); err != nil {
		t.Fatalf("hide dish via update: %v", err)
	}

	dish := getTestDish(t, service, id)
	if dish.MenuType != MenuTypeHidden {
		t.Errorf("menu_type = %q, want %q", dish.MenuType, MenuTypeHidden)
	}
	if dish.IsAvailable {
		t.Error("hidden dish must be unpublished")
	}
	if dish.PreviousMenuType != MenuTypeBoth {
		t.Errorf("previous_menu_type = %q, want %q", dish.PreviousMenuType, MenuTypeBoth)
	}
	if publicDishIDs(t, service, "")[id] {
		t.Error("hidden dish must not be in the public catalog")
	}
}

func TestSetDishAvailabilityKeepsMenuTypeInSync(t *testing.T) {
	service := newTestService(t)

	id := createTestDish(t, service, "both", stringPtr(MenuTypeBoth))

	if err := service.SetDishAvailability(id, false); err != nil {
		t.Fatalf("hide dish: %v", err)
	}

	dish := getTestDish(t, service, id)
	if dish.MenuType != MenuTypeHidden {
		t.Errorf("menu_type after hide = %q, want %q", dish.MenuType, MenuTypeHidden)
	}
	if dish.IsAvailable {
		t.Error("dish must be unpublished after hide")
	}
	if dish.PreviousMenuType != MenuTypeBoth {
		t.Errorf("previous_menu_type after hide = %q, want %q", dish.PreviousMenuType, MenuTypeBoth)
	}
	if publicDishIDs(t, service, "")[id] {
		t.Error("hidden dish must not be in the public catalog")
	}

	if err := service.SetDishAvailability(id, true); err != nil {
		t.Fatalf("publish dish: %v", err)
	}

	dish = getTestDish(t, service, id)
	if dish.MenuType != MenuTypeBoth {
		t.Errorf("menu_type after publish = %q, want %q", dish.MenuType, MenuTypeBoth)
	}
	if !dish.IsAvailable {
		t.Error("dish must be published after publish")
	}
	if !publicDishIDs(t, service, MenuTypeHome)[id] || !publicDishIDs(t, service, MenuTypeBanquet)[id] {
		t.Error("restored dish must appear in both public menus")
	}
}

func TestUpdateDishReturnFromHiddenPublishes(t *testing.T) {
	service := newTestService(t)

	id := createTestDish(t, service, "banquet", stringPtr(MenuTypeBanquet))

	if err := service.UpdateDish(id, DishInput{
		Name:        "banquet",
		Description: "updated",
		Price:       100,
		MenuType:    stringPtr(MenuTypeHidden),
	}); err != nil {
		t.Fatalf("hide dish: %v", err)
	}

	// Возврат из скрытых: выбран прежний тип меню, is_available не передан.
	if err := service.UpdateDish(id, DishInput{
		Name:        "banquet",
		Description: "updated",
		Price:       100,
		MenuType:    stringPtr(MenuTypeBanquet),
	}); err != nil {
		t.Fatalf("return dish from hidden: %v", err)
	}

	dish := getTestDish(t, service, id)
	if dish.MenuType != MenuTypeBanquet {
		t.Errorf("menu_type after return = %q, want %q", dish.MenuType, MenuTypeBanquet)
	}
	if !dish.IsAvailable {
		t.Error("dish must be published after returning from hidden")
	}
	if !publicDishIDs(t, service, MenuTypeBanquet)[id] {
		t.Error("returned dish must appear in the banquet menu")
	}
}

func TestUpdateDishUnpublishMovesDishToHidden(t *testing.T) {
	service := newTestService(t)

	id := createTestDish(t, service, "home", stringPtr(MenuTypeHome))

	if err := service.UpdateDish(id, DishInput{
		Name:        "home",
		Description: "updated",
		Price:       100,
		MenuType:    stringPtr(MenuTypeHome),
		IsAvailable: boolPtr(false),
	}); err != nil {
		t.Fatalf("unpublish dish: %v", err)
	}

	dish := getTestDish(t, service, id)
	if dish.MenuType != MenuTypeHidden {
		t.Errorf("menu_type = %q, want %q", dish.MenuType, MenuTypeHidden)
	}
	if dish.IsAvailable {
		t.Error("dish must be unpublished")
	}
	if dish.PreviousMenuType != MenuTypeHome {
		t.Errorf("previous_menu_type = %q, want %q", dish.PreviousMenuType, MenuTypeHome)
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestInitSchemaNormalizesLegacyVisibility(t *testing.T) {
	db, err := sql.Open(
		"sqlite",
		"file:"+filepath.Join(t.TempDir(), "legacy-visibility.db")+"?_pragma=foreign_keys(1)",
	)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE dishes (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			weight_grams INTEGER CHECK(weight_grams > 0),
			price INTEGER NOT NULL CHECK(price >= 0),
			is_available INTEGER NOT NULL DEFAULT 1,
			menu_type TEXT NOT NULL DEFAULT 'home'
				CHECK(menu_type IN ('home', 'banquet', 'both', 'hidden')),
			emoji TEXT NOT NULL DEFAULT '🍽️',
			photo_url TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO dishes (name, description, price, is_available, menu_type)
		VALUES
			('published both', 'old dish', 100, 1, 'both'),
			('hidden but published', 'old dish', 100, 1, 'hidden'),
			('unpublished banquet', 'old dish', 100, 0, 'banquet');
	`)
	if err != nil {
		t.Fatalf("create legacy data: %v", err)
	}

	if err := database.InitSchema(db); err != nil {
		t.Fatalf("migrate legacy rows: %v", err)
	}

	rows, err := db.Query(`
		SELECT name, is_available, menu_type, previous_menu_type
		FROM dishes
		ORDER BY id
	`)
	if err != nil {
		t.Fatalf("read migrated dishes: %v", err)
	}
	defer rows.Close()

	type row struct {
		available bool
		menuType  string
		previous  string
	}
	got := make(map[string]row)

	for rows.Next() {
		var (
			name      string
			available int
			menuType  string
			previous  string
		)
		if err := rows.Scan(&name, &available, &menuType, &previous); err != nil {
			t.Fatalf("scan migrated dish: %v", err)
		}
		got[name] = row{available == 1, menuType, previous}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migrated dishes: %v", err)
	}

	if item := got["published both"]; !item.available || item.menuType != MenuTypeBoth || item.previous != MenuTypeBoth {
		t.Errorf("published row migrated to %+v", item)
	}
	if item := got["hidden but published"]; item.available || item.menuType != MenuTypeHidden || item.previous != MenuTypeHome {
		t.Errorf("hidden row migrated to %+v", item)
	}
	if item := got["unpublished banquet"]; item.available || item.menuType != MenuTypeHidden || item.previous != MenuTypeBanquet {
		t.Errorf("unpublished row migrated to %+v", item)
	}
}

func TestCreateDishKeepsVisibilityInSync(t *testing.T) {
	service := newTestService(t)

	hiddenID := createTestDish(t, service, "hidden", stringPtr(MenuTypeHidden))
	if dish := getTestDish(t, service, hiddenID); dish.IsAvailable {
		t.Error("new hidden dish must be unpublished")
	}
	if publicDishIDs(t, service, "")[hiddenID] {
		t.Error("new hidden dish must not be in the public catalog")
	}

	unpublished := false
	draftID, err := service.CreateDish(DishInput{
		Name:        "draft both",
		Description: "unpublished on creation",
		Price:       100,
		IsAvailable: &unpublished,
		MenuType:    stringPtr(MenuTypeBoth),
	})
	if err != nil {
		t.Fatalf("create unpublished dish: %v", err)
	}

	draft := getTestDish(t, service, draftID)
	if draft.MenuType != MenuTypeHidden || draft.IsAvailable {
		t.Errorf("unpublished new dish = menu_type %q, available %v; want hidden and unpublished", draft.MenuType, draft.IsAvailable)
	}
	if draft.PreviousMenuType != MenuTypeBoth {
		t.Errorf("previous_menu_type = %q, want %q", draft.PreviousMenuType, MenuTypeBoth)
	}
}
