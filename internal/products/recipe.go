package products

type RecipeIngredient struct {
	IngredientID int64   `json:"ingredient_id"`
	Name         string  `json:"name"`
	Quantity     float64 `json:"quantity"`
	Unit         string  `json:"unit"`
}

type RecipeIngredientInput struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
}

type RecipeUpdate struct {
	Ingredients []RecipeIngredientInput `json:"ingredients"`
}
