package products

type Dish struct {
	ID          int64              `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	WeightGrams *int               `json:"weight_grams"`
	Price       int64              `json:"price"`
	IsAvailable bool               `json:"is_available"`
	Emoji       string             `json:"emoji"`
	Ingredients []RecipeIngredient `json:"ingredients,omitempty"`
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
