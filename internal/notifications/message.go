package notifications

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"food-service/internal/orders"
)

const (
	// Telegram rejects messages longer than 4096 characters. The budget is
	// deliberately lower to leave room for the "part 1/2" prefix and for
	// characters that Telegram counts differently from Go runes.
	telegramMessageLimit  = 4096
	telegramMessageBudget = 3500
)

// The service is operated in Moscow time. A fixed offset avoids a tzdata
// dependency in the Alpine runtime image.
var orderTimeZone = time.FixedZone("MSK", 3*60*60)

// BuildOrderMessages renders an order into one or more Telegram HTML messages.
// It relies on the snapshots and aggregation already produced by
// orders.Service.GetAdminOrder: no prices, totals or ingredients are
// recalculated here.
func BuildOrderMessages(order orders.AdminOrderResponse, publicBaseURL string) []string {
	blocks := []string{
		fmt.Sprintf("🍽️ <b>Новый заказ #%d</b>", order.OrderID),
		"📅 " + formatOrderDate(order.CreatedAt),
		"👤 Клиент: " + escapeText(order.CustomerName),
		"📞 Телефон: " + escapeText(order.CustomerPhone),
	}

	if comment := singleLine(order.Comment); comment != "" {
		blocks = append(blocks, "💬 Комментарий: "+escapeText(comment))
	}

	blocks = append(blocks, "", "<b>Блюда:</b>")
	if len(order.Items) == 0 {
		blocks = append(blocks, "• Нет позиций")
	}
	for _, item := range order.Items {
		line := "• " + escapeText(item.DishName)
		if item.WeightGrams != nil && *item.WeightGrams > 0 {
			line += fmt.Sprintf(" (%d г)", *item.WeightGrams)
		}
		line += fmt.Sprintf(
			" — %d порц. × %s = %s",
			item.Quantity,
			formatMoney(item.UnitPrice),
			formatMoney(item.LineTotal),
		)
		blocks = append(blocks, line)
	}

	blocks = append(blocks, "", "<b>Ингредиенты:</b>")
	ingredients := append([]orders.OrderIngredient(nil), order.Ingredients...)
	sort.Slice(ingredients, func(i, j int) bool {
		left := strings.ToLower(ingredients[i].Name)
		right := strings.ToLower(ingredients[j].Name)
		if left != right {
			return left < right
		}
		return ingredients[i].Unit < ingredients[j].Unit
	})

	if len(ingredients) == 0 {
		blocks = append(blocks, "• Нет ингредиентов")
	}
	for _, ingredient := range ingredients {
		line := "• " + escapeText(ingredient.Name)
		quantity := formatQuantity(ingredient.Quantity)
		if label := unitLabel(ingredient.Unit); label != "" {
			line += " — " + quantity + " " + label
		} else {
			line += " — " + quantity
		}
		blocks = append(blocks, line)
	}

	blocks = append(blocks, "", fmt.Sprintf("<b>Итого: %s</b>", formatMoney(order.Total)))

	if token := strings.TrimSpace(order.Token); token != "" && strings.TrimSpace(publicBaseURL) != "" {
		link := strings.TrimRight(publicBaseURL, "/") + "/order/" + url.PathEscape(token)
		blocks = append(blocks, "", "🔗 Ссылка на заказ:", escapeText(link))
	} else {
		blocks = append(blocks, "", "🔗 Ссылка на заказ недоступна.")
	}

	parts := packBlocks(blocks, telegramMessageBudget)
	if len(parts) > 1 {
		for i := range parts {
			parts[i] = fmt.Sprintf(
				"📄 Заказ #%d · часть %d/%d\n\n%s",
				order.OrderID,
				i+1,
				len(parts),
				parts[i],
			)
		}
	}

	return parts
}

func formatOrderDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "—"
	}

	for _, layout := range []string{"2006-01-02 15:04:05", time.RFC3339} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed.In(orderTimeZone).Format("02.01.2006 15:04")
		}
	}

	return value
}

func formatMoney(value int64) string {
	negative := value < 0
	if negative {
		value = -value
	}

	digits := strconv.FormatInt(value, 10)
	var builder strings.Builder

	for i, digit := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			builder.WriteRune('\u00a0')
		}
		builder.WriteRune(digit)
	}

	if negative {
		return "-" + builder.String() + "\u00a0₽"
	}

	return builder.String() + "\u00a0₽"
}

func formatQuantity(value float64) string {
	text := strconv.FormatFloat(value, 'f', 3, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" || text == "-0" {
		text = "0"
	}

	return strings.ReplaceAll(text, ".", ",")
}

func unitLabel(unit string) string {
	switch unit {
	case "g":
		return "г"
	case "ml":
		return "мл"
	case "pcs":
		return "шт"
	default:
		return strings.TrimSpace(unit)
	}
}

var htmlEscaper = strings.NewReplacer(
	"&", "&amp;",
	"<", "&lt;",
	">", "&gt;",
)

func escapeText(value string) string {
	return htmlEscaper.Replace(singleLine(value))
}

// singleLine collapses newlines and repeated spaces so every order field
// occupies exactly one message line. The textual content is preserved.
func singleLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// packBlocks greedily joins blocks into chunks that fit the budget. Blocks are
// already valid single-line HTML fragments; a block that is too long is split
// by words, which keeps HTML entities intact.
func packBlocks(blocks []string, limit int) []string {
	var chunks []string

	var current strings.Builder
	currentLen := 0

	flush := func() {
		if currentLen > 0 {
			chunks = append(chunks, strings.TrimRight(current.String(), "\n"))
		}
		current.Reset()
		currentLen = 0
	}

	for _, block := range blocks {
		for _, piece := range splitBlock(block, limit) {
			pieceLen := messageLength(piece)
			if currentLen > 0 && currentLen+1+pieceLen > limit {
				flush()
			}
			if currentLen > 0 {
				current.WriteByte('\n')
				currentLen++
			}
			current.WriteString(piece)
			currentLen += pieceLen
		}
	}

	flush()

	if len(chunks) == 0 {
		return []string{""}
	}

	return chunks
}

func splitBlock(block string, limit int) []string {
	if messageLength(block) <= limit {
		return []string{block}
	}

	words := strings.Fields(block)
	if len(words) == 0 {
		return []string{""}
	}

	var parts []string
	var current strings.Builder
	currentLen := 0

	appendWord := func(word string) {
		wordLen := messageLength(word)
		if currentLen > 0 && currentLen+1+wordLen > limit {
			parts = append(parts, current.String())
			current.Reset()
			currentLen = 0
		}
		if currentLen > 0 {
			current.WriteByte(' ')
			currentLen++
		}
		current.WriteString(word)
		currentLen += wordLen
	}

	for _, word := range words {
		for messageLength(word) > limit {
			head, tail := splitByMessageLength(word, limit)
			appendWord(head)
			word = tail
		}
		if word != "" {
			appendWord(word)
		}
	}

	if currentLen > 0 {
		parts = append(parts, current.String())
	}

	if len(parts) == 0 {
		return []string{""}
	}

	return parts
}

// messageLength mirrors Telegram's 4096-character limit, which counts UTF-16
// code units rather than Go runes or bytes.
func messageLength(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func splitByMessageLength(value string, limit int) (string, string) {
	runes := []rune(value)
	units := 0
	cut := 0

	for index, char := range runes {
		charUnits := 1
		if char > 0xFFFF {
			charUnits = 2
		}
		if units+charUnits > limit {
			break
		}
		units += charUnits
		cut = index + 1
	}

	if cut == 0 {
		cut = 1
	}

	return string(runes[:cut]), string(runes[cut:])
}

func truncate(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}

	return string(runes[:maxRunes])
}
