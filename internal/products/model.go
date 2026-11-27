package products

const (
	MenuTypeHome    = "home"
	MenuTypeBanquet = "banquet"
	MenuTypeBoth    = "both"
	MenuTypeHidden  = "hidden"

	DefaultMenuType = MenuTypeHome
)

type Dish struct {
	ID               int64              `json:"id"`
	Name             string             `json:"name"`
	Description      string             `json:"description"`
	WeightGrams      *int               `json:"weight_grams"`
	Price            int64              `json:"price"`
	IsAvailable      bool               `json:"is_available"`
	MenuType         string             `json:"menu_type"`
	PreviousMenuType string             `json:"previous_menu_type"`
	HasRecipe        bool               `json:"has_recipe"`
	Emoji            string             `json:"emoji"`
	PhotoURL         string             `json:"photo_url"`
	Ingredients      []RecipeIngredient `json:"ingredients,omitempty"`
}

// DishInput is used for creating and updating a dish together with its recipe.
// Ingredients == nil means "do not change the recipe" on update. An empty
// non-nil slice is rejected because a published dish must have a recipe.
type DishInput struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	WeightGrams *int                    `json:"weight_grams"`
	Price       int64                   `json:"price"`
	IsAvailable *bool                   `json:"is_available"`
	MenuType    *string                 `json:"menu_type"`
	Emoji       string                  `json:"emoji"`
	Ingredients []RecipeIngredientInput `json:"ingredients"`
}

type AvailabilityUpdate struct {
	IsAvailable *bool `json:"is_available"`
}

type Ingredient struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Unit string `json:"unit"`
}

type IngredientInput struct {
	Name string `json:"name"`
	Unit string `json:"unit"`
}
