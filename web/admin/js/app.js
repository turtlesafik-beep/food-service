"use strict";

const pageTitle = document.querySelector("#page-title");
const pageContent = document.querySelector("#page-content");
const navButtons = document.querySelectorAll(".nav-button");
const modalRoot = document.querySelector("#modal-root");
const toastRoot = document.querySelector("#toast-root");

const UNIT_LABELS = {
    g: "г",
    ml: "мл",
    pcs: "шт"
};

const UNIT_OPTIONS = ["g", "ml", "pcs"];

const state = {
    page: "dishes",
    dishes: [],
    ingredients: [],
    ingredientByKey: new Map(),
    orders: {
        items: [],
        offset: 0,
        limit: 20,
        q: ""
    }
};

let closeModal = () => {};

function unitLabel(unit) {
    return UNIT_LABELS[unit] || unit;
}

function normalizeName(value) {
    return String(value || "")
        .trim()
        .replace(/\s+/g, " ")
        .toLowerCase();
}

function escapeHtml(value) {
    return String(value ?? "")
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#39;");
}

function formatMoney(value) {
    const number = Number(value) || 0;
    return `${number.toLocaleString("ru-RU")} ₽`;
}

function formatQuantity(value) {
    const number = Number(value) || 0;
    return number.toLocaleString("ru-RU", { maximumFractionDigits: 3 });
}

function formatDate(value) {
    if (!value) {
        return "—";
    }

    let iso = String(value);
    if (!iso.includes("T")) {
        iso = iso.replace(" ", "T") + "Z";
    }

    const date = new Date(iso);
    if (Number.isNaN(date.getTime())) {
        return value;
    }

    return date.toLocaleString("ru-RU", {
        day: "2-digit",
        month: "2-digit",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit"
    });
}

function humanizeError(message) {
    if (!message) {
        return "Неизвестная ошибка";
    }

    const exact = {
        "dish name is required": "Укажите название блюда.",
        "dish description is required": "Укажите описание блюда.",
        "dish price cannot be negative": "Цена не может быть отрицательной.",
        "dish weight must be greater than zero": "Вес должен быть больше нуля.",
        "ingredient name is required": "Укажите название ингредиента.",
        "ingredient quantity must be greater than zero": "Количество должно быть больше нуля.",
        "invalid ingredient unit": "Недопустимая единица измерения.",
        "recipe must contain at least one ingredient": "В рецепте должен быть хотя бы один ингредиент.",
        "cannot publish a dish without ingredients": "Нельзя опубликовать блюдо без рецепта. Добавьте ингредиенты.",
        "is_available is required": "Укажите статус публикации.",
        "dish not found": "Блюдо не найдено. Обновите список.",
        "ingredient not found": "Ингредиент не найден. Обновите список.",
        "Invalid JSON": "Некорректные данные формы.",
        "Unauthorized": "Требуется авторизация."
    };

    if (exact[message]) {
        return exact[message];
    }

    let match = message.match(/^ingredient "(.+)" already exists with unit "(.+)", cannot use unit "(.+)"$/);
    if (match) {
        return `Ингредиент «${match[1]}» уже есть с единицей «${unitLabel(match[2])}». Нельзя использовать «${unitLabel(match[3])}».`;
    }

    match = message.match(/^ingredient "(.+)" already exists$/);
    if (match) {
        return `Ингредиент «${match[1]}» уже существует.`;
    }

    match = message.match(/^cannot change unit: ingredient is used in (\d+) dish\(es\)$/);
    if (match) {
        return `Нельзя изменить единицу: ингредиент используется в ${match[1]} блюд(ах).`;
    }

    match = message.match(/^cannot delete ingredient: it is used in (\d+) dish\(es\)$/);
    if (match) {
        return `Нельзя удалить: ингредиент используется в ${match[1]} блюд(ах).`;
    }

    match = message.match(/^duplicate ingredient: (.+)$/);
    if (match) {
        return `Ингредиент «${match[1]}» указан дважды.`;
    }

    return message;
}

async function api(path, options = {}) {
    const headers = new Headers(options.headers || {});

    if (options.body !== undefined && !headers.has("Content-Type")) {
        headers.set("Content-Type", "application/json");
    }

    let response;

    try {
        response = await fetch(path, { ...options, headers, cache: "no-store" });
    } catch (error) {
        throw new Error("Не удалось связаться с сервером.");
    }

    if (response.status === 401) {
        const error = new Error("Требуется авторизация. Введите логин и пароль в окне браузера и обновите страницу.");
        error.status = 401;
        throw error;
    }

    if (!response.ok) {
        const text = (await response.text()).trim();
        const error = new Error(text || `Ошибка ${response.status}`);
        error.status = response.status;
        throw error;
    }

    if (response.status === 204) {
        return null;
    }

    const text = await response.text();
    return text ? JSON.parse(text) : null;
}

function toast(message, type = "info") {
    const element = document.createElement("div");
    element.className = `toast toast-${type}`;
    element.textContent = humanizeError(message);
    toastRoot.appendChild(element);

    window.setTimeout(() => {
        element.classList.add("toast-leaving");
        window.setTimeout(() => element.remove(), 220);
    }, 3800);
}

function openModal({ title, body, submitLabel = "Сохранить", onSubmit = null, wide = false, footer = "" }) {
    closeModal();

    const overlay = document.createElement("div");
    overlay.className = "modal-overlay";

    const isForm = typeof onSubmit === "function";
    const tag = isForm ? "form" : "div";

    const footerHtml = isForm
        ? `<button type="button" class="secondary-button" data-close>Отмена</button>
           <button type="submit" class="primary-button" id="modal-submit">${escapeHtml(submitLabel)}</button>`
        : `${footer}
           <button type="button" class="secondary-button" data-close>Закрыть</button>`;

    overlay.innerHTML = `
        <div class="modal ${wide ? "modal-wide" : ""}" role="dialog" aria-modal="true">
            <div class="modal-header">
                <h2>${escapeHtml(title)}</h2>
                <button type="button" class="modal-close" data-close aria-label="Закрыть">×</button>
            </div>
            <${tag} class="modal-form" id="modal-form">
                <div class="modal-body">${body}</div>
                <div class="modal-error" id="modal-error" hidden></div>
                <div class="modal-footer">${footerHtml}</div>
            </${tag}>
        </div>
    `;

    modalRoot.appendChild(overlay);
    document.body.classList.add("modal-open");

    const form = overlay.querySelector("#modal-form");
    const errorBox = overlay.querySelector("#modal-error");
    const submitButton = overlay.querySelector("#modal-submit");

    const close = () => {
        overlay.remove();
        document.body.classList.remove("modal-open");
        document.removeEventListener("keydown", onKeyDown);
        closeModal = () => {};
    };

    const onKeyDown = (event) => {
        if (event.key === "Escape") {
            close();
        }
    };

    closeModal = close;

    overlay.addEventListener("click", (event) => {
        if (event.target === overlay || event.target.closest("[data-close]")) {
            close();
        }
    });

    document.addEventListener("keydown", onKeyDown);

    const showError = (message) => {
        errorBox.hidden = false;
        errorBox.textContent = humanizeError(message);
    };

    const setLoading = (loading) => {
        if (!submitButton) {
            return;
        }
        submitButton.disabled = loading;
        submitButton.textContent = loading ? "Сохраняем..." : submitLabel;
    };

    if (isForm) {
        form.addEventListener("submit", async (event) => {
            event.preventDefault();
            errorBox.hidden = true;
            setLoading(true);

            try {
                const result = await onSubmit(form, { close, showError, setLoading, modal: overlay });
                if (!result || result.keepOpen !== true) {
                    close();
                }
            } catch (error) {
                showError(error.message);
                setLoading(false);
            }
        });
    }

    const firstInput = overlay.querySelector("input, textarea, select");
    if (firstInput) {
        firstInput.focus();
    }

    return { overlay, close, showError, setLoading };
}

async function copyText(text) {
    if (!text) {
        return;
    }

    try {
        if (navigator.clipboard && window.isSecureContext) {
            await navigator.clipboard.writeText(text);
        } else {
            const textarea = document.createElement("textarea");
            textarea.value = text;
            textarea.style.position = "fixed";
            textarea.style.opacity = "0";
            document.body.appendChild(textarea);
            textarea.select();
            document.execCommand("copy");
            textarea.remove();
        }

        toast("Ссылка скопирована", "success");
    } catch (error) {
        toast("Не удалось скопировать ссылку. Выделите её вручную.", "error");
    }
}

function buildOrderLink(token) {
    return `${window.location.origin}/order/${encodeURIComponent(token)}`;
}

async function loadIngredients() {
    state.ingredients = await api("/api/admin/ingredients");
    state.ingredientByKey = new Map(
        state.ingredients.map((ingredient) => [normalizeName(ingredient.name), ingredient])
    );
    return state.ingredients;
}

async function loadDishes() {
    const tbody = document.querySelector("#dishes-list");
    if (!tbody) {
        return;
    }

    tbody.innerHTML = `<tr><td colspan="6" class="empty-cell">Загрузка блюд...</td></tr>`;

    try {
        const dishes = await api("/api/admin/dishes");
        state.dishes = dishes;
        renderDishesTable(dishes);
    } catch (error) {
        tbody.innerHTML = `<tr><td colspan="6" class="empty-cell error-text">${escapeHtml(humanizeError(error.message))}</td></tr>`;
    }
}

function renderDishesTable(dishes) {
    const tbody = document.querySelector("#dishes-list");
    if (!tbody) {
        return;
    }

    if (dishes.length === 0) {
        tbody.innerHTML = `<tr><td colspan="6" class="empty-cell">Блюд пока нет. Нажмите «Добавить блюдо».</td></tr>`;
        return;
    }

    tbody.innerHTML = dishes.map((dish) => {
        const status = dish.is_available
            ? `<span class="status-badge status-published">Опубликовано</span>`
            : `<span class="status-badge status-hidden">Скрыто</span>`;

        const recipe = dish.has_recipe
            ? `<span class="status-badge status-ok">Есть</span>`
            : `<span class="status-badge status-warning">Нет</span>`;

        return `
            <tr>
                <td>
                    <div class="dish-name">${escapeHtml(dish.emoji)} ${escapeHtml(dish.name)}</div>
                    <div class="muted small">${escapeHtml(dish.description)}</div>
                </td>
                <td>${dish.weight_grams ? `${dish.weight_grams} г` : "—"}</td>
                <td>${formatMoney(dish.price)}</td>
                <td>${recipe}</td>
                <td>${status}</td>
                <td class="actions-cell">
                    <button class="action-button" data-action="edit" data-id="${dish.id}">
                        Редактировать
                    </button>
                    <button class="action-button" data-action="toggle" data-id="${dish.id}">
                        ${dish.is_available ? "Скрыть" : "Опубликовать"}
                    </button>
                    <button class="action-button action-danger" data-action="delete" data-id="${dish.id}">
                        Удалить
                    </button>
                </td>
            </tr>
        `;
    }).join("");
}

async function handleDishAction(event) {
    const button = event.target.closest("button[data-action]");
    if (!button) {
        return;
    }

    const dish = state.dishes.find((item) => item.id === Number(button.dataset.id));
    if (!dish) {
        return;
    }

    if (button.dataset.action === "edit") {
        await openDishForm(dish);
    } else if (button.dataset.action === "toggle") {
        await toggleDish(dish);
    } else if (button.dataset.action === "delete") {
        await deleteDish(dish);
    }
}

async function toggleDish(dish) {
    try {
        await api(`/api/admin/dishes/${dish.id}`, {
            method: "PATCH",
            body: JSON.stringify({ is_available: !dish.is_available })
        });

        toast(dish.is_available ? "Блюдо скрыто" : "Блюдо опубликовано", "success");
        await loadDishes();
    } catch (error) {
        toast(error.message, "error");
    }
}

async function deleteDish(dish) {
    const confirmed = window.confirm(
        `Удалить блюдо «${dish.name}»?\n\nОно исчезнет из меню. Старые заказы сохранятся.`
    );

    if (!confirmed) {
        return;
    }

    try {
        await api(`/api/admin/dishes/${dish.id}`, { method: "DELETE" });
        toast("Блюдо удалено", "success");
        await loadDishes();
    } catch (error) {
        toast(error.message, "error");
    }
}

async function openDishForm(dish) {
    try {
        await loadIngredients();

        let recipe = [];
        if (dish) {
            recipe = await api(`/api/admin/dishes/${dish.id}/recipe`);
        }

        renderDishForm(dish, recipe);
    } catch (error) {
        toast(error.message, "error");
    }
}

function recipeRowHtml(item) {
    const unitOptions = UNIT_OPTIONS.map((unit) => {
        const selected = item.unit === unit ? "selected" : "";
        return `<option value="${unit}" ${selected}>${unitLabel(unit)}</option>`;
    }).join("");

    return `
        <div class="recipe-row">
            <input
                class="recipe-name"
                list="ingredient-options"
                value="${escapeHtml(item.name || "")}"
                placeholder="Ингредиент"
                autocomplete="off"
            >
            <input
                class="recipe-quantity"
                type="number"
                min="0.001"
                step="0.001"
                value="${item.quantity ?? ""}"
                placeholder="Кол-во"
            >
            <select class="recipe-unit">${unitOptions}</select>
            <button type="button" class="icon-button recipe-remove" title="Удалить ингредиент">×</button>
        </div>
    `;
}

function syncRecipeRow(row) {
    const nameInput = row.querySelector(".recipe-name");
    const unitSelect = row.querySelector(".recipe-unit");
    const ingredient = state.ingredientByKey.get(normalizeName(nameInput.value));

    if (ingredient) {
        unitSelect.value = ingredient.unit;
        unitSelect.disabled = true;
        unitSelect.title = "Единица измерения задаётся в справочнике ингредиентов";
        row.classList.add("recipe-row-locked");
    } else {
        unitSelect.disabled = false;
        unitSelect.title = "";
        row.classList.remove("recipe-row-locked");
    }
}

function addRecipeRow(container, item = { name: "", quantity: "", unit: "g" }) {
    const wrapper = document.createElement("div");
    wrapper.innerHTML = recipeRowHtml(item).trim();
    const row = wrapper.firstElementChild;
    container.appendChild(row);
    syncRecipeRow(row);
    return row;
}

function renderDishForm(dish, recipe) {
    const isEditing = Boolean(dish);
    const rows = recipe.length > 0 ? recipe : [{ name: "", quantity: "", unit: "g" }];

    const datalist = state.ingredients.map((ingredient) => (
        `<option value="${escapeHtml(ingredient.name)}"></option>`
    )).join("");

    const body = `
        <div class="form-grid">
            <div class="form-field form-field-wide">
                <label for="dish-name">Название *</label>
                <input id="dish-name" name="name" maxlength="120" required value="${escapeHtml(dish?.name || "")}">
            </div>

            <div class="form-field form-field-wide">
                <label for="dish-description">Описание *</label>
                <textarea id="dish-description" name="description" rows="3" maxlength="1000" required>${escapeHtml(dish?.description || "")}</textarea>
            </div>

            <div class="form-field">
                <label for="dish-weight">Вес порции, г</label>
                <input id="dish-weight" name="weight" type="number" min="1" step="1" value="${dish?.weight_grams ?? ""}">
            </div>

            <div class="form-field">
                <label for="dish-price">Цена, ₽ *</label>
                <input id="dish-price" name="price" type="number" min="0" step="1" required value="${dish?.price ?? ""}">
            </div>

            <div class="form-field">
                <label for="dish-emoji">Эмодзи</label>
                <input id="dish-emoji" name="emoji" maxlength="8" value="${escapeHtml(dish?.emoji || "🍽️")}">
            </div>

            <div class="form-field form-field-checkbox">
                <label class="checkbox-label">
                    <input id="dish-available" type="checkbox" ${!dish || dish.is_available ? "checked" : ""}>
                    Показывать клиентам
                </label>
                <p class="hint">Блюдо без рецепта нельзя опубликовать.</p>
            </div>
        </div>

        <div class="recipe-section">
            <div class="section-heading">
                <h3>Рецепт и продукты</h3>
                <button type="button" class="secondary-button" id="add-recipe-row">+ Ингредиент</button>
            </div>
            <p class="hint">
                Количество указывается на одну порцию. Итоговый список продуктов для клиента считается автоматически.
            </p>
            <div id="recipe-rows" class="recipe-rows"></div>
            <datalist id="ingredient-options">${datalist}</datalist>
        </div>
    `;

    const { overlay } = openModal({
        title: isEditing ? "Редактировать блюдо" : "Новое блюдо",
        body,
        submitLabel: "Сохранить",
        wide: true,
        onSubmit: async (form) => {
            const name = form.querySelector("#dish-name").value.trim();
            const description = form.querySelector("#dish-description").value.trim();
            const weightRaw = form.querySelector("#dish-weight").value.trim();
            const priceRaw = form.querySelector("#dish-price").value.trim();
            const emoji = form.querySelector("#dish-emoji").value.trim();
            const isAvailable = form.querySelector("#dish-available").checked;

            if (!name) {
                throw new Error("Укажите название блюда.");
            }
            if (!description) {
                throw new Error("Укажите описание блюда.");
            }
            if (priceRaw === "") {
                throw new Error("Укажите цену блюда.");
            }

            const price = Number(priceRaw);
            if (!Number.isFinite(price) || price < 0) {
                throw new Error("Цена не может быть отрицательной.");
            }

            let weightGrams = null;
            if (weightRaw !== "") {
                weightGrams = Number(weightRaw);
                if (!Number.isFinite(weightGrams) || weightGrams <= 0) {
                    throw new Error("Вес должен быть больше нуля.");
                }
            }

            const ingredients = [];

            for (const row of form.querySelectorAll(".recipe-row")) {
                const ingredientName = row.querySelector(".recipe-name").value.trim();
                const quantityRaw = row.querySelector(".recipe-quantity").value.trim();
                const unit = row.querySelector(".recipe-unit").value;

                if (!ingredientName && !quantityRaw) {
                    continue;
                }
                if (!ingredientName) {
                    throw new Error("Укажите название ингредиента.");
                }
                if (quantityRaw === "") {
                    throw new Error(`Укажите количество для «${ingredientName}».`);
                }

                const quantity = Number(quantityRaw);
                if (!Number.isFinite(quantity) || quantity <= 0) {
                    throw new Error(`Количество для «${ingredientName}» должно быть больше нуля.`);
                }

                ingredients.push({
                    name: ingredientName,
                    quantity,
                    unit
                });
            }

            if (isAvailable && ingredients.length === 0) {
                throw new Error("Добавьте хотя бы один ингредиент или снимите публикацию.");
            }

            const payload = {
                name,
                description,
                weight_grams: weightGrams,
                price,
                emoji: emoji || "🍽️",
                is_available: isAvailable,
                ingredients
            };

            if (isEditing) {
                await api(`/api/admin/dishes/${dish.id}`, {
                    method: "PUT",
                    body: JSON.stringify(payload)
                });
                toast("Блюдо сохранено", "success");
            } else {
                await api("/api/admin/dishes", {
                    method: "POST",
                    body: JSON.stringify(payload)
                });
                toast("Блюдо добавлено", "success");
            }

            if (state.page === "dishes") {
                await loadDishes();
            }
        }
    });

    const rowsContainer = overlay.querySelector("#recipe-rows");
    rows.forEach((item) => addRecipeRow(rowsContainer, item));

    overlay.querySelector("#add-recipe-row").addEventListener("click", () => {
        const row = addRecipeRow(rowsContainer);
        row.querySelector(".recipe-name").focus();
    });

    rowsContainer.addEventListener("input", (event) => {
        const row = event.target.closest(".recipe-row");
        if (row && event.target.classList.contains("recipe-name")) {
            syncRecipeRow(row);
        }
    });

    rowsContainer.addEventListener("click", (event) => {
        const removeButton = event.target.closest(".recipe-remove");
        if (removeButton) {
            removeButton.closest(".recipe-row").remove();
        }
    });
}

async function renderDishesPage() {
    pageContent.innerHTML = `
        <div class="page-actions">
            <button class="primary-button" id="add-dish-button">+ Добавить блюдо</button>
        </div>

        <div class="table-container">
            <table class="dishes-table">
                <thead>
                    <tr>
                        <th>Блюдо</th>
                        <th>Вес</th>
                        <th>Цена</th>
                        <th>Рецепт</th>
                        <th>Статус</th>
                        <th>Действия</th>
                    </tr>
                </thead>
                <tbody id="dishes-list">
                    <tr><td colspan="6" class="empty-cell">Загрузка блюд...</td></tr>
                </tbody>
            </table>
        </div>
    `;

    pageContent.querySelector("#add-dish-button").addEventListener("click", () => openDishForm(null));
    pageContent.querySelector("#dishes-list").addEventListener("click", handleDishAction);

    await loadDishes();
}

async function loadIngredientsPage() {
    const tbody = document.querySelector("#ingredients-list");
    if (!tbody) {
        return;
    }

    tbody.innerHTML = `<tr><td colspan="3" class="empty-cell">Загрузка ингредиентов...</td></tr>`;

    try {
        const ingredients = await loadIngredients();
        renderIngredientsTable(ingredients);
    } catch (error) {
        tbody.innerHTML = `<tr><td colspan="3" class="empty-cell error-text">${escapeHtml(humanizeError(error.message))}</td></tr>`;
    }
}

function renderIngredientsTable(ingredients) {
    const tbody = document.querySelector("#ingredients-list");
    if (!tbody) {
        return;
    }

    if (ingredients.length === 0) {
        tbody.innerHTML = `<tr><td colspan="3" class="empty-cell">Ингредиентов пока нет.</td></tr>`;
        return;
    }

    tbody.innerHTML = ingredients.map((ingredient) => `
        <tr>
            <td>${escapeHtml(ingredient.name)}</td>
            <td>${unitLabel(ingredient.unit)}</td>
            <td class="actions-cell">
                <button class="action-button" data-action="edit" data-id="${ingredient.id}">
                    Редактировать
                </button>
                <button class="action-button action-danger" data-action="delete" data-id="${ingredient.id}">
                    Удалить
                </button>
            </td>
        </tr>
    `).join("");
}

async function handleIngredientAction(event) {
    const button = event.target.closest("button[data-action]");
    if (!button) {
        return;
    }

    const ingredient = state.ingredients.find((item) => item.id === Number(button.dataset.id));
    if (!ingredient) {
        return;
    }

    if (button.dataset.action === "edit") {
        openIngredientForm(ingredient);
    } else if (button.dataset.action === "delete") {
        await deleteIngredient(ingredient);
    }
}

function openIngredientForm(ingredient) {
    const isEditing = Boolean(ingredient);

    const unitOptions = UNIT_OPTIONS.map((unit) => {
        const selected = (ingredient?.unit || "g") === unit ? "selected" : "";
        return `<option value="${unit}" ${selected}>${unitLabel(unit)}</option>`;
    }).join("");

    const body = `
        <div class="form-grid">
            <div class="form-field form-field-wide">
                <label for="ingredient-name">Название *</label>
                <input id="ingredient-name" maxlength="120" required value="${escapeHtml(ingredient?.name || "")}">
            </div>

            <div class="form-field">
                <label for="ingredient-unit">Единица измерения *</label>
                <select id="ingredient-unit">${unitOptions}</select>
            </div>
        </div>
        <p class="hint">
            Единицу нельзя изменить, если ингредиент уже используется в рецептах. Сначала уберите его из блюд.
        </p>
    `;

    openModal({
        title: isEditing ? "Редактировать ингредиент" : "Новый ингредиент",
        body,
        submitLabel: "Сохранить",
        onSubmit: async (form) => {
            const name = form.querySelector("#ingredient-name").value.trim();
            const unit = form.querySelector("#ingredient-unit").value;

            if (!name) {
                throw new Error("Укажите название ингредиента.");
            }

            const payload = { name, unit };

            if (isEditing) {
                await api(`/api/admin/ingredients/${ingredient.id}`, {
                    method: "PUT",
                    body: JSON.stringify(payload)
                });
                toast("Ингредиент сохранён", "success");
            } else {
                await api("/api/admin/ingredients", {
                    method: "POST",
                    body: JSON.stringify(payload)
                });
                toast("Ингредиент добавлен", "success");
            }

            if (state.page === "ingredients") {
                await loadIngredientsPage();
            }
        }
    });
}

async function deleteIngredient(ingredient) {
    const confirmed = window.confirm(`Удалить ингредиент «${ingredient.name}»?`);
    if (!confirmed) {
        return;
    }

    try {
        await api(`/api/admin/ingredients/${ingredient.id}`, { method: "DELETE" });
        toast("Ингредиент удалён", "success");
        await loadIngredientsPage();
    } catch (error) {
        toast(error.message, "error");
    }
}

async function renderIngredientsPage() {
    pageContent.innerHTML = `
        <div class="page-actions">
            <button class="primary-button" id="add-ingredient-button">+ Добавить ингредиент</button>
        </div>

        <div class="table-container">
            <table class="dishes-table">
                <thead>
                    <tr>
                        <th>Название</th>
                        <th>Единица</th>
                        <th>Действия</th>
                    </tr>
                </thead>
                <tbody id="ingredients-list">
                    <tr><td colspan="3" class="empty-cell">Загрузка ингредиентов...</td></tr>
                </tbody>
            </table>
        </div>

        <p class="hint page-hint">
            Единицу измерения нельзя изменить, если ингредиент уже используется в рецептах.
        </p>
    `;

    pageContent.querySelector("#add-ingredient-button").addEventListener("click", () => openIngredientForm(null));
    pageContent.querySelector("#ingredients-list").addEventListener("click", handleIngredientAction);

    await loadIngredientsPage();
}

function renderOrdersTable(orders) {
    const tbody = document.querySelector("#orders-list");
    if (!tbody) {
        return;
    }

    if (orders.length === 0) {
        tbody.innerHTML = `<tr><td colspan="8" class="empty-cell">Заказов пока нет.</td></tr>`;
        return;
    }

    tbody.innerHTML = orders.map((order) => {
        const link = order.token ? buildOrderLink(order.token) : "";
        const linkCell = link
            ? `<a href="${escapeHtml(link)}" target="_blank" rel="noopener">Открыть</a>
               <button class="action-button" data-copy-link="${escapeHtml(link)}">Копировать</button>`
            : `<span class="muted">нет ссылки</span>`;

        const phone = order.customer_phone
            ? `<a href="tel:${escapeHtml(order.customer_phone)}">${escapeHtml(order.customer_phone)}</a>`
            : "—";

        return `
            <tr>
                <td>#${order.order_id}</td>
                <td>${formatDate(order.created_at)}</td>
                <td>${escapeHtml(order.customer_name || "—")}</td>
                <td>${phone}</td>
                <td>${order.item_count}</td>
                <td>${formatMoney(order.total)}</td>
                <td class="order-link-cell">${linkCell}</td>
                <td class="actions-cell">
                    <button class="action-button" data-action="details" data-id="${order.order_id}">
                        Подробнее
                    </button>
                </td>
            </tr>
        `;
    }).join("");
}

async function loadOrders() {
    const tbody = document.querySelector("#orders-list");
    if (!tbody) {
        return;
    }

    tbody.innerHTML = `<tr><td colspan="8" class="empty-cell">Загрузка заказов...</td></tr>`;

    const params = new URLSearchParams({
        limit: String(state.orders.limit),
        offset: String(state.orders.offset)
    });

    if (state.orders.q) {
        params.set("q", state.orders.q);
    }

    try {
        const orders = await api(`/api/admin/orders?${params.toString()}`);
        state.orders.items = orders;
        renderOrdersTable(orders);
        updateOrdersPagination(orders);
    } catch (error) {
        tbody.innerHTML = `<tr><td colspan="8" class="empty-cell error-text">${escapeHtml(humanizeError(error.message))}</td></tr>`;
        updateOrdersPagination([]);
    }
}

function updateOrdersPagination(orders) {
    const prev = document.querySelector("#orders-prev");
    const next = document.querySelector("#orders-next");

    if (prev) {
        prev.disabled = state.orders.offset === 0;
    }
    if (next) {
        next.disabled = orders.length < state.orders.limit;
    }
}

async function openOrderDetails(orderId) {
    try {
        const order = await api(`/api/admin/orders/${orderId}`);
        const link = order.token ? buildOrderLink(order.token) : "";

        const items = order.items.map((item) => `
            <tr>
                <td>${escapeHtml(item.dish_name)}</td>
                <td>${item.weight_grams ? `${item.weight_grams} г` : "—"}</td>
                <td>${item.quantity}</td>
                <td>${formatMoney(item.unit_price)}</td>
                <td>${formatMoney(item.line_total)}</td>
            </tr>
        `).join("");

        const ingredients = order.ingredients.map((ingredient) => `
            <tr>
                <td>${escapeHtml(ingredient.name)}</td>
                <td>${formatQuantity(ingredient.quantity)} ${unitLabel(ingredient.unit)}</td>
            </tr>
        `).join("");

        const linkBlock = link
            ? `<div class="order-link-block">
                   <div class="muted small">Ссылка для клиента</div>
                   <a href="${escapeHtml(link)}" target="_blank" rel="noopener">${escapeHtml(link)}</a>
                   <button class="action-button" data-copy-link="${escapeHtml(link)}">Копировать</button>
               </div>`
            : `<p class="muted">У этого заказа нет ссылки — он создан до включения ссылок.</p>`;

        const body = `
            <div class="order-meta">
                <div><span class="muted">Клиент:</span> ${escapeHtml(order.customer_name || "—")}</div>
                <div><span class="muted">Телефон:</span> ${escapeHtml(order.customer_phone || "—")}</div>
                <div><span class="muted">Дата:</span> ${formatDate(order.created_at)}</div>
                <div><span class="muted">Комментарий:</span> ${escapeHtml(order.comment || "—")}</div>
            </div>

            ${linkBlock}

            <h3 class="details-title">Блюда</h3>
            <div class="table-container compact-container">
                <table class="dishes-table compact">
                    <thead>
                        <tr>
                            <th>Блюдо</th>
                            <th>Вес</th>
                            <th>Кол-во</th>
                            <th>Цена</th>
                            <th>Сумма</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${items || `<tr><td colspan="5" class="empty-cell">Нет позиций</td></tr>`}
                    </tbody>
                </table>
            </div>

            <h3 class="details-title">Продукты для закупки</h3>
            <div class="table-container compact-container">
                <table class="dishes-table compact">
                    <thead>
                        <tr>
                            <th>Продукт</th>
                            <th>Количество</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${ingredients || `<tr><td colspan="2" class="empty-cell">Нет продуктов</td></tr>`}
                    </tbody>
                </table>
            </div>

            <div class="order-total">
                Итого: <strong>${formatMoney(order.total)}</strong>
            </div>
        `;

        openModal({
            title: `Заказ #${order.order_id}`,
            body,
            wide: true,
            footer: ""
        });
    } catch (error) {
        toast(error.message, "error");
    }
}

async function handleOrderAction(event) {
    const button = event.target.closest("button[data-action]");
    if (!button) {
        return;
    }

    if (button.dataset.action === "details") {
        await openOrderDetails(Number(button.dataset.id));
    }
}

async function renderOrdersPage() {
    pageContent.innerHTML = `
        <div class="page-actions orders-actions">
            <form id="order-search-form" class="search-form">
                <input id="order-search-input" placeholder="Поиск по телефону или имени" value="${escapeHtml(state.orders.q)}">
                <button class="primary-button" type="submit">Найти</button>
            </form>
        </div>

        <div class="table-container">
            <table class="dishes-table">
                <thead>
                    <tr>
                        <th>№</th>
                        <th>Дата</th>
                        <th>Клиент</th>
                        <th>Телефон</th>
                        <th>Позиции</th>
                        <th>Сумма</th>
                        <th>Ссылка</th>
                        <th></th>
                    </tr>
                </thead>
                <tbody id="orders-list">
                    <tr><td colspan="8" class="empty-cell">Загрузка заказов...</td></tr>
                </tbody>
            </table>
        </div>

        <div class="pagination">
            <button class="secondary-button" id="orders-prev" type="button">Назад</button>
            <button class="secondary-button" id="orders-next" type="button">Вперёд</button>
        </div>
    `;

    const searchForm = pageContent.querySelector("#order-search-form");
    const searchInput = pageContent.querySelector("#order-search-input");

    searchForm.addEventListener("submit", async (event) => {
        event.preventDefault();
        state.orders.q = searchInput.value.trim();
        state.orders.offset = 0;
        await loadOrders();
    });

    pageContent.querySelector("#orders-prev").addEventListener("click", async () => {
        if (state.orders.offset === 0) {
            return;
        }
        state.orders.offset = Math.max(0, state.orders.offset - state.orders.limit);
        await loadOrders();
    });

    pageContent.querySelector("#orders-next").addEventListener("click", async () => {
        state.orders.offset += state.orders.limit;
        await loadOrders();
    });

    pageContent.querySelector("#orders-list").addEventListener("click", handleOrderAction);

    await loadOrders();
}

const pages = {
    dishes: {
        title: "Блюда",
        render: renderDishesPage
    },
    ingredients: {
        title: "Ингредиенты",
        render: renderIngredientsPage
    },
    orders: {
        title: "Заказы",
        render: renderOrdersPage
    }
};

let currentPage = null;

async function navigate(pageName, updateHash = true) {
    const page = pages[pageName];
    if (!page) {
        return;
    }

    closeModal();

    navButtons.forEach((button) => {
        button.classList.toggle("active", button.dataset.page === pageName);
    });

    currentPage = pageName;
    state.page = pageName;
    pageTitle.textContent = page.title;

    if (updateHash) {
        const hash = `#${pageName}`;
        if (window.location.hash !== hash) {
            window.history.replaceState(null, "", hash);
        }
    }

    try {
        await page.render();
    } catch (error) {
        pageContent.innerHTML = `<div class="page-error">${escapeHtml(humanizeError(error.message))}</div>`;
    }
}

navButtons.forEach((button) => {
    button.addEventListener("click", () => navigate(button.dataset.page));
});

window.addEventListener("hashchange", () => {
    const pageName = window.location.hash.replace(/^#/, "");

    if (pages[pageName] && pageName !== currentPage) {
        navigate(pageName, false);
    }
});

document.addEventListener("click", (event) => {
    const copyButton = event.target.closest("[data-copy-link]");
    if (copyButton) {
        copyText(copyButton.dataset.copyLink);
    }
});

const initialPage = window.location.hash.replace(/^#/, "");
navigate(pages[initialPage] ? initialPage : "dishes");
