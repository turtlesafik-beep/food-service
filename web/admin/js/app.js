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

const DEFAULT_DISH_EMOJI = "🍽️";

const MENU_TYPE_LABELS = {
    home: "Домашнее меню",
    banquet: "Банкетное меню",
    both: "Оба меню",
    hidden: "Скрыто"
};

const CATEGORY_MENU_LABELS = {
    home: "Домашнее меню",
    banquet: "Банкетное меню"
};

const DISH_FILTERS = [
    { value: "all", label: "Все" },
    { value: "home", label: "Домашнее меню" },
    { value: "banquet", label: "Банкетное меню" },
    { value: "both", label: "Оба меню" },
    { value: "hidden", label: "Скрытые" }
];

const EMOJI_CATEGORIES = [
    {
        id: "meat",
        label: "Мясо и птица",
        emojis: [
            { char: "🍗", name: "Куриная ножка" },
            { char: "🍖", name: "Мясо на кости" },
            { char: "🥩", name: "Стейк" },
            { char: "🥓", name: "Бекон" },
            { char: "🦃", name: "Индейка" },
            { char: "🐔", name: "Курица" },
            { char: "🐖", name: "Свинина" },
            { char: "🐄", name: "Говядина" },
            { char: "🦆", name: "Утка" },
            { char: "🐇", name: "Кролик" }
        ]
    },
    {
        id: "fish",
        label: "Рыба и морепродукты",
        emojis: [
            { char: "🐟", name: "Рыба" },
            { char: "🐠", name: "Тропическая рыба" },
            { char: "🐡", name: "Рыба-шар" },
            { char: "🦈", name: "Акула" },
            { char: "🐙", name: "Осьминог" },
            { char: "🦑", name: "Кальмар" },
            { char: "🦐", name: "Креветка" },
            { char: "🦞", name: "Лобстер" },
            { char: "🦀", name: "Краб" }
        ]
    },
    {
        id: "vegetables",
        label: "Овощи",
        emojis: [
            { char: "🥦", name: "Брокколи" },
            { char: "🥕", name: "Морковь" },
            { char: "🥒", name: "Огурец" },
            { char: "🍅", name: "Помидор" },
            { char: "🍆", name: "Баклажан" },
            { char: "🥔", name: "Картофель" },
            { char: "🌽", name: "Кукуруза" },
            { char: "🫑", name: "Болгарский перец" },
            { char: "🌶️", name: "Острый перец" },
            { char: "🧄", name: "Чеснок" },
            { char: "🧅", name: "Лук" },
            { char: "🍄", name: "Грибы" },
            { char: "🥑", name: "Авокадо" }
        ]
    },
    {
        id: "fruits",
        label: "Фрукты и ягоды",
        emojis: [
            { char: "🍎", name: "Яблоко" },
            { char: "🍐", name: "Груша" },
            { char: "🍊", name: "Мандарин" },
            { char: "🍋", name: "Лимон" },
            { char: "🍌", name: "Банан" },
            { char: "🍉", name: "Арбуз" },
            { char: "🍇", name: "Виноград" },
            { char: "🍓", name: "Клубника" },
            { char: "🫐", name: "Голубика" },
            { char: "🍒", name: "Вишня" },
            { char: "🍑", name: "Персик" },
            { char: "🥝", name: "Киви" },
            { char: "🍍", name: "Ананас" }
        ]
    },
    {
        id: "ready",
        label: "Готовые блюда",
        emojis: [
            { char: "🍲", name: "Суп" },
            { char: "🍛", name: "Карри" },
            { char: "🍜", name: "Лапша" },
            { char: "🍝", name: "Паста" },
            { char: "🍕", name: "Пицца" },
            { char: "🍔", name: "Бургер" },
            { char: "🌮", name: "Тако" },
            { char: "🌯", name: "Буритто" },
            { char: "🥙", name: "Шаурма" },
            { char: "🍣", name: "Суши" },
            { char: "🍱", name: "Бенто" },
            { char: "🍚", name: "Рис" },
            { char: "🥘", name: "Рагу" },
            { char: "🍳", name: "Яичница" },
            { char: "🥞", name: "Блины" },
            { char: "🥗", name: "Салат" }
        ]
    },
    {
        id: "bakery",
        label: "Выпечка и десерты",
        emojis: [
            { char: "🍞", name: "Хлеб" },
            { char: "🥖", name: "Багет" },
            { char: "🥐", name: "Круассан" },
            { char: "🥨", name: "Крендель" },
            { char: "🧁", name: "Капкейк" },
            { char: "🎂", name: "Торт" },
            { char: "🍰", name: "Кусок торта" },
            { char: "🍪", name: "Печенье" },
            { char: "🍩", name: "Пончик" },
            { char: "🥧", name: "Пирог" },
            { char: "🍫", name: "Шоколад" },
            { char: "🍦", name: "Мороженое" },
            { char: "🍨", name: "Мороженое в креманке" }
        ]
    },
    {
        id: "drinks",
        label: "Напитки",
        emojis: [
            { char: "☕", name: "Кофе" },
            { char: "🍵", name: "Чай" },
            { char: "🥤", name: "Газировка" },
            { char: "🧃", name: "Сок" },
            { char: "🧋", name: "Бабл-ти" },
            { char: "🍺", name: "Пиво" },
            { char: "🍷", name: "Вино" },
            { char: "🥂", name: "Шампанское" },
            { char: "🍸", name: "Коктейль" },
            { char: "🍹", name: "Тропический коктейль" },
            { char: "🥛", name: "Молоко" }
        ]
    },
    {
        id: "other",
        label: "Прочее",
        emojis: [
            { char: "🍽️", name: "Тарелка (по умолчанию)" },
            { char: "🥄", name: "Ложка" },
            { char: "🍴", name: "Вилка и нож" },
            { char: "🥢", name: "Палочки" },
            { char: "🔪", name: "Нож" },
            { char: "🧂", name: "Соль" },
            { char: "🧈", name: "Масло" },
            { char: "🧀", name: "Сыр" },
            { char: "🥚", name: "Яйцо" },
            { char: "🥜", name: "Орехи" },
            { char: "🍯", name: "Мёд" },
            { char: "🥣", name: "Миска" }
        ]
    }
];

const state = {
    page: "dishes",
    dishes: [],
    dishesFilter: "all",
    ingredients: [],
    ingredientByKey: new Map(),
    categories: {
        home: [],
        banquet: []
    },
    orders: {
        items: [],
        offset: 0,
        limit: 20,
        q: ""
    },
    backups: {
        restore: null,
        safety: null,
        safetyConfirmed: false,
        applying: false
    }
};

let closeModal = () => {};

function unitLabel(unit) {
    return UNIT_LABELS[unit] || unit;
}

function menuTypeLabel(menuType) {
    return MENU_TYPE_LABELS[menuType] || MENU_TYPE_LABELS.home;
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
        "invalid menu type": "Выберите корректную принадлежность к меню.",
        "ingredient name is required": "Укажите название ингредиента.",
        "ingredient quantity must be greater than zero": "Количество должно быть больше нуля.",
        "invalid ingredient unit": "Недопустимая единица измерения.",
        "category name is required": "Укажите название категории.",
        "invalid category menu type": "Выберите домашнее или банкетное меню.",
        "category not found": "Категория не найдена. Обновите список.",
        "invalid category id": "Некорректная категория.",
        "categories are not available for this menu type": "Для этого типа меню категории недоступны.",
        "recipe must contain at least one ingredient": "В рецепте должен быть хотя бы один ингредиент.",
        "cannot publish a dish without ingredients": "Нельзя опубликовать блюдо без рецепта. Добавьте ингредиенты.",
        "is_available is required": "Укажите статус публикации.",
        "dish not found": "Блюдо не найдено. Обновите список.",
        "ingredient not found": "Ингредиент не найден. Обновите список.",
        "Invalid JSON": "Некорректные данные формы.",
        "Order not found": "Заказ не найден. Возможно, он уже был удалён.",
        "Invalid order ID": "Некорректный идентификатор заказа.",
        "Failed to delete order": "Не удалось удалить заказ. Попробуйте ещё раз.",
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

    match = message.match(/^category "(.+)" already exists$/);
    if (match) {
        return `Категория «${match[1]}» уже существует в этом меню.`;
    }

    match = message.match(/^category not found: (\d+)$/);
    if (match) {
        return "Категория не найдена. Обновите список.";
    }

    match = message.match(/^category (\d+) is not available for this menu type$/);
    if (match) {
        return "Категория не подходит к выбранному типу меню.";
    }

    return message;
}

async function api(path, options = {}) {
    const headers = new Headers(options.headers || {});
    const isFormData = options.body instanceof FormData;

    if (options.body !== undefined && !isFormData && !headers.has("Content-Type")) {
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

function formatBytes(value) {
    const bytes = Number(value) || 0;
    if (bytes < 1024) {
        return `${bytes} Б`;
    }

    const units = ["КБ", "МБ", "ГБ"];
    let size = bytes / 1024;
    let unit = units[0];

    for (let i = 1; i < units.length && size >= 1024; i += 1) {
        size /= 1024;
        unit = units[i];
    }

    return `${size.toLocaleString("ru-RU", { maximumFractionDigits: 1 })} ${unit}`;
}

function backupFilename(isoDate) {
    const date = isoDate ? new Date(isoDate) : new Date();
    const value = Number.isNaN(date.getTime()) ? new Date() : date;
    const stamp = value.toISOString().replace(/[-:]/g, "").replace(/\.\d{3}Z$/, "Z");
    return `food-service-backup-${stamp}.zip`;
}

function startBackupDownload(sessionId, isoDate) {
    const link = document.createElement("a");
    link.href = `/api/admin/backups/${encodeURIComponent(sessionId)}/download`;
    link.download = backupFilename(isoDate);
    link.style.display = "none";
    document.body.appendChild(link);
    link.click();
    link.remove();
}

function uploadRestoreArchive(file, onProgress) {
    return new Promise((resolve, reject) => {
        const request = new XMLHttpRequest();
        request.open("POST", "/api/admin/restores/validate");
        request.responseType = "text";
        request.setRequestHeader("Content-Type", "application/zip");

        request.upload.addEventListener("progress", (event) => {
            if (event.lengthComputable && typeof onProgress === "function") {
                onProgress(Math.round((event.loaded / event.total) * 100));
            }
        });

        request.addEventListener("load", () => {
            let payload = null;
            const text = (request.responseText || "").trim();
            if (text) {
                try {
                    payload = JSON.parse(text);
                } catch (error) {
                    payload = null;
                }
            }

            if (request.status >= 200 && request.status < 300 && payload) {
                resolve(payload);
                return;
            }

            if (request.status === 401) {
                reject(new Error("Требуется авторизация. Обновите страницу и введите логин и пароль."));
                return;
            }

            reject(new Error(text || `Ошибка проверки архива (${request.status}).`));
        });

        request.addEventListener("error", () => {
            reject(new Error("Не удалось загрузить архив на сервер."));
        });

        request.send(file);
    });
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

async function loadCategories() {
    const [home, banquet] = await Promise.all([
        api("/api/categories?menu=home"),
        api("/api/categories?menu=banquet")
    ]);

    state.categories = {
        home: Array.isArray(home) ? home : [],
        banquet: Array.isArray(banquet) ? banquet : []
    };

    return state.categories;
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
        renderDishesTable();
    } catch (error) {
        tbody.innerHTML = `<tr><td colspan="6" class="empty-cell error-text">${escapeHtml(humanizeError(error.message))}</td></tr>`;
    }
}

function isDishPublished(dish) {
    return Boolean(dish.is_available) && dish.menu_type !== "hidden";
}

function isDishHidden(dish) {
    return dish.menu_type === "hidden" || !dish.is_available;
}

function filteredDishes() {
    if (state.dishesFilter === "all") {
        return state.dishes;
    }

    if (state.dishesFilter === "hidden") {
        return state.dishes.filter(isDishHidden);
    }

    return state.dishes.filter((dish) => {
        const matchesMenu = dish.menu_type === state.dishesFilter
            || (dish.menu_type === "both" && ["home", "banquet"].includes(state.dishesFilter));
        return matchesMenu && isDishPublished(dish);
    });
}

function renderDishesTable() {
    const tbody = document.querySelector("#dishes-list");
    if (!tbody) {
        return;
    }

    const dishes = filteredDishes();

    if (dishes.length === 0) {
        const message = state.dishesFilter === "all"
            ? "Блюд пока нет. Нажмите «Добавить блюдо»."
            : "В этой категории блюд пока нет.";
        tbody.innerHTML = `<tr><td colspan="6" class="empty-cell">${message}</td></tr>`;
        return;
    }

    tbody.innerHTML = dishes.map((dish) => {
        const published = isDishPublished(dish);

        const status = published
            ? `<span class="status-badge status-published">Опубликовано</span>`
            : `<span class="status-badge status-hidden">Не опубликовано</span>`;

        const recipe = dish.has_recipe
            ? `<span class="status-badge status-ok">Есть</span>`
            : `<span class="status-badge status-warning">Нет</span>`;

        const menuType = dish.menu_type || "home";

        return `
            <tr>
                <td class="cell-primary" data-label="Блюдо">
                    <div class="cell-value">
                        <div class="dish-name">${escapeHtml(dish.emoji)} ${escapeHtml(dish.name)}</div>
                        <div class="muted small">${escapeHtml(dish.description)}</div>
                        <span class="menu-type-badge menu-type-${escapeHtml(menuType)}">${escapeHtml(menuTypeLabel(menuType))}</span>
                    </div>
                </td>
                <td data-label="Вес">
                    <div class="cell-value">${dish.weight_grams ? `${dish.weight_grams} г` : "—"}</div>
                </td>
                <td data-label="Цена">
                    <div class="cell-value">${formatMoney(dish.price)}</div>
                </td>
                <td data-label="Рецепт">
                    <div class="cell-value">${recipe}</div>
                </td>
                <td data-label="Статус">
                    <div class="cell-value">${status}</div>
                </td>
                <td class="actions-cell" data-label="Действия">
                    <div class="cell-value">
                        <button class="action-button" data-action="edit" data-id="${dish.id}">
                            Редактировать
                        </button>
                        <button class="action-button" data-action="toggle" data-id="${dish.id}">
                            ${published ? "Скрыть" : "Опубликовать"}
                        </button>
                        <button class="action-button action-danger" data-action="delete" data-id="${dish.id}">
                            Удалить
                        </button>
                    </div>
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
    const published = isDishPublished(dish);

    try {
        await api(`/api/admin/dishes/${dish.id}`, {
            method: "PATCH",
            body: JSON.stringify({ is_available: !published })
        });

        toast(published ? "Блюдо скрыто" : "Блюдо опубликовано", "success");
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
        await Promise.all([loadIngredients(), loadCategories()]);

        let recipe = [];
        if (dish) {
            recipe = await api(`/api/admin/dishes/${dish.id}/recipe`);
        }

        renderDishForm(dish, recipe, state.categories);
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

function setupEmojiPicker(overlay, initialEmoji) {
    const field = overlay.querySelector("#dish-emoji-field");
    const trigger = overlay.querySelector("#dish-emoji-button");
    const currentDisplay = overlay.querySelector("#dish-emoji-current");
    const hiddenInput = overlay.querySelector("#dish-emoji");
    const picker = overlay.querySelector("#dish-emoji-picker");
    const categoriesBox = overlay.querySelector("#dish-emoji-categories");
    const grid = overlay.querySelector("#dish-emoji-grid");
    const closeButton = overlay.querySelector("#dish-emoji-close");
    const closeInsideButton = overlay.querySelector("#dish-emoji-close-inside");
    const resetButton = overlay.querySelector("#dish-emoji-reset");
    const resetInsideButton = overlay.querySelector("#dish-emoji-reset-inside");

    if (!field || !trigger || !currentDisplay || !hiddenInput || !picker || !categoriesBox || !grid) {
        return;
    }

    let activeCategoryId = EMOJI_CATEGORIES[0].id;
    let isOpen = false;

    function findCategoryId(emoji) {
        const category = EMOJI_CATEGORIES.find((item) =>
            item.emojis.some((entry) => entry.char === emoji)
        );
        return category ? category.id : EMOJI_CATEGORIES[0].id;
    }

    function updateSelectedEmoji(emoji) {
        hiddenInput.value = emoji;
        currentDisplay.textContent = emoji;
    }

    function renderCategories() {
        categoriesBox.innerHTML = EMOJI_CATEGORIES.map((category) => `
            <button
                type="button"
                class="emoji-category ${category.id === activeCategoryId ? "active" : ""}"
                data-category="${category.id}"
                role="tab"
                aria-selected="${category.id === activeCategoryId}"
            >${escapeHtml(category.label)}</button>
        `).join("");
    }

    function renderGrid() {
        const category = EMOJI_CATEGORIES.find((item) => item.id === activeCategoryId) || EMOJI_CATEGORIES[0];
        const selectedEmoji = hiddenInput.value;

        grid.innerHTML = category.emojis.map((entry) => {
            const selected = entry.char === selectedEmoji;

            return `
                <button
                    type="button"
                    class="emoji-option ${selected ? "selected" : ""}"
                    data-emoji="${escapeHtml(entry.char)}"
                    title="${escapeHtml(entry.name)}"
                    aria-label="${escapeHtml(entry.name)}"
                    aria-selected="${selected}"
                    role="option"
                >
                    <span class="emoji-option-char">${entry.char}</span>
                </button>
            `;
        }).join("");
    }

    function updateCategoryButtons() {
        categoriesBox.querySelectorAll("[data-category]").forEach((button) => {
            const active = button.dataset.category === activeCategoryId;
            button.classList.toggle("active", active);
            button.setAttribute("aria-selected", String(active));
        });
    }

    function focusGridOption() {
        const target = grid.querySelector(".emoji-option.selected") || grid.querySelector(".emoji-option");
        if (target) {
            target.focus();
        }
    }

    function setActiveCategory(categoryId) {
        activeCategoryId = categoryId;
        updateCategoryButtons();
        renderGrid();
        focusGridOption();
    }

    function openPicker() {
        if (isOpen) {
            return;
        }

        isOpen = true;
        activeCategoryId = findCategoryId(hiddenInput.value);
        renderCategories();
        renderGrid();
        picker.hidden = false;
        trigger.setAttribute("aria-expanded", "true");
        focusGridOption();
        picker.scrollIntoView({ block: "nearest" });
    }

    function closePicker() {
        if (!isOpen) {
            return;
        }

        isOpen = false;
        picker.hidden = true;
        trigger.setAttribute("aria-expanded", "false");
    }

    function chooseEmoji(emoji) {
        updateSelectedEmoji(emoji);
        closePicker();
        trigger.focus();
    }

    function resetEmoji() {
        updateSelectedEmoji(DEFAULT_DISH_EMOJI);
        closePicker();
        trigger.focus();
    }

    updateSelectedEmoji(initialEmoji);
    trigger.setAttribute("aria-expanded", "false");

    trigger.addEventListener("click", () => {
        if (isOpen) {
            closePicker();
        } else {
            openPicker();
        }
    });

    if (resetButton) {
        resetButton.addEventListener("click", resetEmoji);
    }
    if (resetInsideButton) {
        resetInsideButton.addEventListener("click", resetEmoji);
    }
    if (closeButton) {
        closeButton.addEventListener("click", closePicker);
    }
    if (closeInsideButton) {
        closeInsideButton.addEventListener("click", closePicker);
    }

    categoriesBox.addEventListener("click", (event) => {
        const button = event.target.closest("[data-category]");
        if (button) {
            setActiveCategory(button.dataset.category);
        }
    });

    grid.addEventListener("click", (event) => {
        const button = event.target.closest(".emoji-option");
        if (button) {
            chooseEmoji(button.dataset.emoji);
        }
    });

    field.addEventListener("keydown", (event) => {
        if (event.key === "Escape" && isOpen) {
            event.stopPropagation();
            closePicker();
            trigger.focus();
        }
    });

    overlay.addEventListener("click", (event) => {
        if (!isOpen) {
            return;
        }

        // composedPath фиксирует путь события до возможной перерисовки DOM.
        // contains(event.target) ломается, если target уже отсоединён от DOM.
        const clickedInsideField = typeof event.composedPath === "function"
            ? event.composedPath().includes(field)
            : field.contains(event.target);

        if (!clickedInsideField) {
            closePicker();
        }
    });
}

function renderDishForm(dish, recipe, categories = state.categories) {
    const isEditing = Boolean(dish);
    const selectedCategoryIds = new Set(
        (Array.isArray(dish?.category_ids) ? dish.category_ids : []).map(Number)
    );
    const rows = recipe.length > 0 ? recipe : [{ name: "", quantity: "", unit: "g" }];
    const currentEmoji = String(dish?.emoji || "").trim() || DEFAULT_DISH_EMOJI;
    const currentMenuType = dish?.menu_type || "home";
    const previousMenuType = ["home", "banquet", "both"].includes(dish?.previous_menu_type)
        ? dish.previous_menu_type
        : "";
    const previousMenuHint = currentMenuType === "hidden" && previousMenuType
        ? ` Прежнее меню: «${escapeHtml(menuTypeLabel(previousMenuType))}».`
        : "";

    function categoryMenuTypes(menuType) {
        if (menuType === "both") {
            return ["home", "banquet"];
        }
        if (menuType === "home" || menuType === "banquet") {
            return [menuType];
        }
        if (menuType === "hidden") {
            const previous = ["home", "banquet", "both"].includes(previousMenuType)
                ? previousMenuType
                : "home";
            return previous === "both" ? ["home", "banquet"] : [previous];
        }
        return [];
    }
    const existingPhotoURL = String(dish?.photo_url || "").trim();
    let selectedPhotoFile = null;
    let removeExistingPhoto = false;
    let selectedPhotoPreviewURL = "";

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

            <div class="form-field form-field-wide">
                <label for="dish-menu-type">Принадлежность к меню *</label>
                <select id="dish-menu-type" name="menu_type">
                    ${DISH_FILTERS.filter((filter) => filter.value !== "all").map((filter) => `
                        <option value="${filter.value}" ${filter.value === currentMenuType ? "selected" : ""}>
                            ${filter.label}
                        </option>
                    `).join("")}
                </select>
                <p class="hint">
                    «Скрыто» автоматически снимает блюдо с публикации и убирает его из публичного каталога.
                    Чтобы вернуть блюдо, выберите домашнее, банкетное или оба меню.${previousMenuHint}
                </p>
            </div>

            <div class="form-field form-field-wide" id="dish-categories-field">
                <label>Категории</label>
                <p class="hint" id="dish-categories-hint">
                    Можно выбрать несколько категорий или не выбирать ни одной.
                </p>
                <div class="category-choice-list" id="dish-category-options"></div>
            </div>

            <div class="form-field form-field-wide emoji-field" id="dish-emoji-field">
                <label for="dish-emoji-button">Эмодзи блюда</label>

                <div class="emoji-control">
                    <button
                        type="button"
                        class="emoji-trigger"
                        id="dish-emoji-button"
                        aria-haspopup="dialog"
                        aria-expanded="false"
                        aria-controls="dish-emoji-picker"
                    >
                        <span class="emoji-current" id="dish-emoji-current" aria-hidden="true">${escapeHtml(currentEmoji)}</span>
                        <span class="emoji-trigger-text">Выбрать эмодзи</span>
                        <span class="emoji-trigger-chevron" aria-hidden="true">▾</span>
                    </button>

                    <button type="button" class="secondary-button emoji-reset" id="dish-emoji-reset">
                        Сбросить
                    </button>
                </div>

                <input type="hidden" id="dish-emoji" name="emoji" value="${escapeHtml(currentEmoji)}">

                <p class="hint">Нажмите на поле и выберите картинку из каталога.</p>

                <div
                    class="emoji-picker"
                    id="dish-emoji-picker"
                    role="dialog"
                    aria-label="Выбор эмодзи"
                    hidden
                >
                    <div class="emoji-picker-header">
                        <div>
                            <div class="emoji-picker-title">Выберите эмодзи</div>
                            <div class="emoji-picker-subtitle">Каталог продуктов, блюд и напитков</div>
                        </div>
                        <button type="button" class="modal-close emoji-picker-close" id="dish-emoji-close" aria-label="Закрыть выбор эмодзи">×</button>
                    </div>

                    <div class="emoji-categories" id="dish-emoji-categories" role="tablist" aria-label="Категории эмодзи"></div>
                    <div class="emoji-grid" id="dish-emoji-grid" role="listbox" aria-label="Эмодзи"></div>

                    <div class="emoji-picker-footer">
                        <button type="button" class="secondary-button" id="dish-emoji-reset-inside">
                            Сбросить на ${DEFAULT_DISH_EMOJI}
                        </button>
                        <button type="button" class="secondary-button" id="dish-emoji-close-inside">
                            Закрыть
                        </button>
                    </div>
                </div>
            </div>

            <div class="form-field form-field-wide dish-photo-field" id="dish-photo-field">
                <label for="dish-photo-input">Фотография блюда</label>

                <div class="dish-photo-editor">
                    <div class="dish-photo-preview" id="dish-photo-preview">
                        ${existingPhotoURL
                            ? `<img src="${escapeHtml(existingPhotoURL)}" alt="Фотография блюда">`
                            : `<div class="dish-photo-empty">Фото не добавлено</div>`}
                    </div>

                    <div class="dish-photo-controls">
                        <input id="dish-photo-input" type="file" accept="image/jpeg,image/png,image/webp">
                        <button type="button" class="secondary-button" id="dish-photo-remove" disabled>
                            Удалить фото
                        </button>
                        <p class="hint">
                            JPEG, PNG или WebP, до 5 МБ. Фото необязательно: в меню оно показывается только при раскрытии блюда.
                        </p>
                    </div>
                </div>
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
            const menuType = form.querySelector("#dish-menu-type").value;
            let isAvailable = form.querySelector("#dish-available").checked;

            if (menuType === "hidden") {
                isAvailable = false;
            }

            if (!name) {
                throw new Error("Укажите название блюда.");
            }
            if (!MENU_TYPE_LABELS[menuType]) {
                throw new Error("Выберите принадлежность к меню.");
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

            const allowedCategoryMenus = categoryMenuTypes(menuType);
            const allowedCategoryIds = new Set();

            for (const allowedMenu of allowedCategoryMenus) {
                for (const category of categories[allowedMenu] || []) {
                    allowedCategoryIds.add(Number(category.id));
                }
            }

            const categoryIds = [...selectedCategoryIds]
                .filter((categoryId) => allowedCategoryIds.has(categoryId));

            const payload = {
                name,
                description,
                weight_grams: weightGrams,
                price,
                emoji: emoji || DEFAULT_DISH_EMOJI,
                is_available: isAvailable,
                menu_type: menuType,
                category_ids: categoryIds,
                ingredients
            };

            let savedDishId = dish?.id;
            let savedMessage = "";

            if (isEditing) {
                await api(`/api/admin/dishes/${dish.id}`, {
                    method: "PUT",
                    body: JSON.stringify(payload)
                });
                savedMessage = "Блюдо сохранено";
            } else {
                const created = await api("/api/admin/dishes", {
                    method: "POST",
                    body: JSON.stringify(payload)
                });
                savedDishId = created.id;
                savedMessage = "Блюдо добавлено";
            }

            let photoError = "";

            if (selectedPhotoFile) {
                try {
                    const formData = new FormData();
                    formData.append("photo", selectedPhotoFile, selectedPhotoFile.name || "dish-photo");
                    await api(`/api/admin/dishes/${savedDishId}/photo`, {
                        method: "POST",
                        body: formData
                    });
                } catch (error) {
                    photoError = error.message;
                }
            } else if (removeExistingPhoto && existingPhotoURL) {
                try {
                    await api(`/api/admin/dishes/${savedDishId}/photo`, {
                        method: "DELETE"
                    });
                } catch (error) {
                    photoError = error.message;
                }
            }

            toast(savedMessage, "success");

            if (photoError) {
                toast(`Фото не сохранено: ${humanizeError(photoError)}`, "error");
            }

            if (state.page === "dishes") {
                await loadDishes();
            }
        }
    });

    setupEmojiPicker(overlay, currentEmoji);

    const menuTypeSelect = overlay.querySelector("#dish-menu-type");
    const availableCheckbox = overlay.querySelector("#dish-available");

    function syncVisibilityControls() {
        if (!menuTypeSelect || !availableCheckbox) {
            return;
        }

        const hidden = menuTypeSelect.value === "hidden";
        availableCheckbox.disabled = hidden;

        if (hidden) {
            availableCheckbox.checked = false;
        }
    }

    if (menuTypeSelect && availableCheckbox) {
        menuTypeSelect.addEventListener("change", () => {
            if (menuTypeSelect.value !== "hidden") {
                availableCheckbox.checked = true;
            }
            syncVisibilityControls();
            renderCategoryChoices();
        });
        syncVisibilityControls();
    }

    const categoryOptions = overlay.querySelector("#dish-category-options");
    const categoriesHint = overlay.querySelector("#dish-categories-hint");

    function renderCategoryChoices() {
        if (!categoryOptions || !menuTypeSelect) {
            return;
        }

        const menuTypes = categoryMenuTypes(menuTypeSelect.value);

        if (menuTypes.length === 0) {
            categoryOptions.innerHTML = `<p class="hint">Для этого типа меню категории недоступны.</p>`;
            return;
        }

        categoryOptions.innerHTML = menuTypes.map((menuType) => {
            const list = categories[menuType] || [];
            const heading = menuTypes.length > 1
                ? `<div class="category-choice-title">${escapeHtml(CATEGORY_MENU_LABELS[menuType])}</div>`
                : "";

            if (list.length === 0) {
                return `
                    <div class="category-choice-group">
                        ${heading}
                        <p class="hint">Категорий этого меню пока нет. Добавьте их во вкладке «Фильтры».</p>
                    </div>
                `;
            }

            const options = list.map((category) => {
                const categoryId = Number(category.id);
                const checked = selectedCategoryIds.has(categoryId) ? "checked" : "";

                return `
                    <label class="category-choice">
                        <input type="checkbox" data-category-id="${categoryId}" ${checked}>
                        <span>${escapeHtml(category.name)}</span>
                    </label>
                `;
            }).join("");

            return `
                <div class="category-choice-group">
                    ${heading}
                    <div class="category-choice-items">${options}</div>
                </div>
            `;
        }).join("");

        if (categoriesHint) {
            if (menuTypeSelect.value === "hidden") {
                categoriesHint.textContent = "Блюдо скрыто: категории не показываются клиентам. Доступны категории прежнего меню.";
            } else if (menuTypes.length > 1) {
                categoriesHint.textContent = "Доступны категории домашнего и банкетного меню. Можно выбрать несколько.";
            } else {
                categoriesHint.textContent = `Доступны категории меню «${CATEGORY_MENU_LABELS[menuTypes[0]]}». Можно выбрать несколько.`;
            }
        }
    }

    if (categoryOptions) {
        categoryOptions.addEventListener("change", (event) => {
            const input = event.target.closest("[data-category-id]");
            if (!input) {
                return;
            }

            const categoryId = Number(input.dataset.categoryId);
            if (input.checked) {
                selectedCategoryIds.add(categoryId);
            } else {
                selectedCategoryIds.delete(categoryId);
            }
        });

        renderCategoryChoices();
    }

    const photoInput = overlay.querySelector("#dish-photo-input");
    const photoPreview = overlay.querySelector("#dish-photo-preview");
    const photoRemoveButton = overlay.querySelector("#dish-photo-remove");

    function revokeSelectedPhotoPreview() {
        if (selectedPhotoPreviewURL) {
            URL.revokeObjectURL(selectedPhotoPreviewURL);
            selectedPhotoPreviewURL = "";
        }
    }

    function updatePhotoRemoveButton() {
        if (!photoRemoveButton) {
            return;
        }

        if (selectedPhotoFile) {
            photoRemoveButton.disabled = false;
            photoRemoveButton.textContent = "Отменить выбор";
        } else if (existingPhotoURL && !removeExistingPhoto) {
            photoRemoveButton.disabled = false;
            photoRemoveButton.textContent = "Удалить фото";
        } else if (removeExistingPhoto) {
            photoRemoveButton.disabled = false;
            photoRemoveButton.textContent = "Вернуть фото";
        } else {
            photoRemoveButton.disabled = true;
            photoRemoveButton.textContent = "Удалить фото";
        }
    }

    function renderPhotoPreview() {
        if (!photoPreview) {
            return;
        }

        revokeSelectedPhotoPreview();

        let url = "";
        if (selectedPhotoFile) {
            selectedPhotoPreviewURL = URL.createObjectURL(selectedPhotoFile);
            url = selectedPhotoPreviewURL;
        } else if (existingPhotoURL && !removeExistingPhoto) {
            url = existingPhotoURL;
        }

        if (url) {
            photoPreview.innerHTML = `<img src="${escapeHtml(url)}" alt="Фотография блюда">`;
            const image = photoPreview.querySelector("img");
            if (image) {
                image.addEventListener("error", () => {
                    photoPreview.innerHTML = `<div class="dish-photo-empty">Не удалось загрузить фото</div>`;
                }, { once: true });
            }
        } else {
            photoPreview.innerHTML = `
                <div class="dish-photo-empty">
                    ${removeExistingPhoto ? "Фото будет удалено после сохранения" : "Фото не добавлено"}
                </div>
            `;
        }

        updatePhotoRemoveButton();
    }

    if (photoInput) {
        photoInput.addEventListener("change", () => {
            const file = photoInput.files && photoInput.files[0] ? photoInput.files[0] : null;

            if (!file) {
                selectedPhotoFile = null;
                renderPhotoPreview();
                return;
            }

            const allowedTypes = ["image/jpeg", "image/png", "image/webp"];
            if (file.type && !allowedTypes.includes(file.type)) {
                photoInput.value = "";
                toast("Поддерживаются только JPEG, PNG и WebP.", "error");
                return;
            }

            if (file.size > 5 * 1024 * 1024) {
                photoInput.value = "";
                toast("Файл слишком большой. Максимум 5 МБ.", "error");
                return;
            }

            selectedPhotoFile = file;
            removeExistingPhoto = false;
            renderPhotoPreview();
        });
    }

    if (photoRemoveButton) {
        photoRemoveButton.addEventListener("click", () => {
            if (selectedPhotoFile) {
                selectedPhotoFile = null;
                if (photoInput) {
                    photoInput.value = "";
                }
                removeExistingPhoto = false;
            } else if (existingPhotoURL && !removeExistingPhoto) {
                removeExistingPhoto = true;
            } else if (removeExistingPhoto) {
                removeExistingPhoto = false;
            }

            renderPhotoPreview();
        });
    }

    renderPhotoPreview();

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
    const filterButtons = DISH_FILTERS.map((filter) => `
        <button
            type="button"
            class="filter-button ${filter.value === state.dishesFilter ? "active" : ""}"
            data-dish-filter="${filter.value}"
            role="tab"
            aria-selected="${filter.value === state.dishesFilter}"
        >${filter.label}</button>
    `).join("");

    pageContent.innerHTML = `
        <div class="page-actions dishes-actions">
            <div class="dish-filters" id="dish-filters" role="tablist" aria-label="Фильтр блюд по меню">
                ${filterButtons}
            </div>

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

    pageContent.querySelector("#dish-filters").addEventListener("click", (event) => {
        const button = event.target.closest("[data-dish-filter]");
        if (!button) {
            return;
        }

        const filter = button.dataset.dishFilter;
        if (filter === state.dishesFilter) {
            return;
        }

        state.dishesFilter = filter;

        pageContent.querySelectorAll("[data-dish-filter]").forEach((filterButton) => {
            const active = filterButton.dataset.dishFilter === filter;
            filterButton.classList.toggle("active", active);
            filterButton.setAttribute("aria-selected", String(active));
        });

        renderDishesTable();
    });

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
            <td class="cell-primary" data-label="Название">
                <div class="cell-value cell-title">${escapeHtml(ingredient.name)}</div>
            </td>
            <td data-label="Единица">
                <div class="cell-value">${unitLabel(ingredient.unit)}</div>
            </td>
            <td class="actions-cell" data-label="Действия">
                <div class="cell-value">
                    <button class="action-button" data-action="edit" data-id="${ingredient.id}">
                        Редактировать
                    </button>
                    <button class="action-button action-danger" data-action="delete" data-id="${ingredient.id}">
                        Удалить
                    </button>
                </div>
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

function renderCategoriesTable(menu, categories) {
    const tbody = document.querySelector(`#categories-${menu}-list`);
    if (!tbody) {
        return;
    }

    if (categories.length === 0) {
        tbody.innerHTML = `<tr><td colspan="3" class="empty-cell">Категорий пока нет. Нажмите «Добавить».</td></tr>`;
        return;
    }

    tbody.innerHTML = categories.map((category) => {
        const usage = Number(category.dish_count) || 0;

        return `
            <tr>
                <td class="cell-primary" data-label="Название">
                    <div class="cell-value cell-title">${escapeHtml(category.name)}</div>
                </td>
                <td data-label="Блюд">
                    <div class="cell-value">${usage > 0 ? usage : "—"}</div>
                </td>
                <td class="actions-cell" data-label="Действия">
                    <div class="cell-value">
                        <button class="action-button" data-action="edit" data-id="${category.id}" data-menu="${menu}">
                            Редактировать
                        </button>
                        <button class="action-button action-danger" data-action="delete" data-id="${category.id}" data-menu="${menu}">
                            Удалить
                        </button>
                    </div>
                </td>
            </tr>
        `;
    }).join("");
}

async function loadCategoriesPage() {
    const bodies = ["home", "banquet"]
        .map((menu) => document.querySelector(`#categories-${menu}-list`))
        .filter(Boolean);

    bodies.forEach((tbody) => {
        tbody.innerHTML = `<tr><td colspan="3" class="empty-cell">Загрузка категорий...</td></tr>`;
    });

    try {
        const categories = await loadCategories();
        renderCategoriesTable("home", categories.home);
        renderCategoriesTable("banquet", categories.banquet);
    } catch (error) {
        bodies.forEach((tbody) => {
            tbody.innerHTML = `<tr><td colspan="3" class="empty-cell error-text">${escapeHtml(humanizeError(error.message))}</td></tr>`;
        });
    }
}

async function handleCategoryAction(event) {
    const button = event.target.closest("button[data-action]");
    if (!button) {
        return;
    }

    const menu = button.dataset.menu;
    const category = (state.categories[menu] || [])
        .find((item) => item.id === Number(button.dataset.id));

    if (!category) {
        return;
    }

    if (button.dataset.action === "edit") {
        openCategoryForm(category, menu);
    } else if (button.dataset.action === "delete") {
        await deleteCategory(category, menu);
    }
}

function openCategoryForm(category, menu) {
    const isEditing = Boolean(category);
    const menuLabel = CATEGORY_MENU_LABELS[menu] || "";

    const body = `
        <div class="form-grid">
            <div class="form-field form-field-wide">
                <label for="category-name">Название *</label>
                <input id="category-name" maxlength="120" required value="${escapeHtml(category?.name || "")}">
            </div>
        </div>
        <p class="hint">
            Категория относится к меню «${escapeHtml(menuLabel)}». Одинаковые названия в другом меню считаются отдельными категориями.
        </p>
    `;

    openModal({
        title: isEditing ? `Переименовать категорию · ${menuLabel}` : `Новая категория · ${menuLabel}`,
        body,
        submitLabel: "Сохранить",
        onSubmit: async (form) => {
            const name = form.querySelector("#category-name").value.trim();

            if (!name) {
                throw new Error("Укажите название категории.");
            }

            if (isEditing) {
                await api(`/api/admin/categories/${category.id}`, {
                    method: "PUT",
                    body: JSON.stringify({ name })
                });
                toast("Категория сохранена", "success");
            } else {
                await api("/api/admin/categories", {
                    method: "POST",
                    body: JSON.stringify({ name, menu_type: menu })
                });
                toast("Категория добавлена", "success");
            }

            if (state.page === "categories") {
                await loadCategoriesPage();
            }
        }
    });
}

async function deleteCategory(category, menu) {
    const usage = Number(category.dish_count) || 0;
    let message = `Удалить категорию «${category.name}»?`;

    if (usage > 0) {
        message += `\n\nКатегория назначена ${usage} блюд(ам). При удалении она будет снята с этих блюд, сами блюда сохранятся.`;
    }

    if (!window.confirm(message)) {
        return;
    }

    try {
        await api(`/api/admin/categories/${category.id}`, { method: "DELETE" });
        toast("Категория удалена", "success");

        if (state.page === "categories") {
            await loadCategoriesPage();
        }
    } catch (error) {
        toast(error.message, "error");
    }
}

function categoriesGroupHtml(menu) {
    return `
        <section class="category-group">
            <div class="category-group-head">
                <h2>${escapeHtml(CATEGORY_MENU_LABELS[menu])}</h2>
                <button class="primary-button" data-add-category="${menu}">+ Добавить</button>
            </div>

            <div class="table-container">
                <table class="dishes-table">
                    <thead>
                        <tr>
                            <th>Название</th>
                            <th>Блюд</th>
                            <th>Действия</th>
                        </tr>
                    </thead>
                    <tbody id="categories-${menu}-list">
                        <tr><td colspan="3" class="empty-cell">Загрузка категорий...</td></tr>
                    </tbody>
                </table>
            </div>
        </section>
    `;
}

async function renderCategoriesPage() {
    pageContent.innerHTML = `
        <div class="category-groups">
            ${categoriesGroupHtml("home")}
            ${categoriesGroupHtml("banquet")}
        </div>

        <p class="hint page-hint">
            Категории домашнего и банкетного меню независимы. Клиент видит их как фильтры над каталогом выбранного меню.
        </p>
    `;

    pageContent.querySelectorAll("[data-add-category]").forEach((button) => {
        button.addEventListener("click", () => openCategoryForm(null, button.dataset.addCategory));
    });

    ["home", "banquet"].forEach((menu) => {
        pageContent.querySelector(`#categories-${menu}-list`)
            .addEventListener("click", handleCategoryAction);
    });

    await loadCategoriesPage();
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
                <td class="cell-primary" data-label="Заказ">
                    <div class="cell-value cell-title"><span class="order-number">#${order.order_id}</span></div>
                </td>
                <td data-label="Дата">
                    <div class="cell-value">${formatDate(order.created_at)}</div>
                </td>
                <td data-label="Клиент">
                    <div class="cell-value">${escapeHtml(order.customer_name || "—")}</div>
                </td>
                <td data-label="Телефон">
                    <div class="cell-value">${phone}</div>
                </td>
                <td data-label="Позиции">
                    <div class="cell-value">${order.item_count}</div>
                </td>
                <td data-label="Сумма">
                    <div class="cell-value">${formatMoney(order.total)}</div>
                </td>
                <td class="order-link-cell" data-label="Ссылка">
                    <div class="cell-value">${linkCell}</div>
                </td>
                <td class="actions-cell" data-label="Действия">
                    <div class="cell-value">
                        <div class="order-row-actions">
                            <button class="action-button" data-action="details" data-id="${order.order_id}">
                                Подробнее
                            </button>
                            <button class="action-button action-danger" data-action="delete" data-id="${order.order_id}">
                                Удалить
                            </button>
                        </div>
                    </div>
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
                <td class="cell-primary" data-label="Блюдо">
                    <div class="cell-value cell-title">${escapeHtml(item.dish_name)}</div>
                </td>
                <td data-label="Вес">
                    <div class="cell-value">${item.weight_grams ? `${item.weight_grams} г` : "—"}</div>
                </td>
                <td data-label="Кол-во">
                    <div class="cell-value">${item.quantity}</div>
                </td>
                <td data-label="Цена">
                    <div class="cell-value">${formatMoney(item.unit_price)}</div>
                </td>
                <td data-label="Сумма">
                    <div class="cell-value">${formatMoney(item.line_total)}</div>
                </td>
            </tr>
        `).join("");

        const ingredients = order.ingredients.map((ingredient) => `
            <tr>
                <td class="cell-primary" data-label="Продукт">
                    <div class="cell-value cell-title">${escapeHtml(ingredient.name)}</div>
                </td>
                <td data-label="Количество">
                    <div class="cell-value">${formatQuantity(ingredient.quantity)} ${unitLabel(ingredient.unit)}</div>
                </td>
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

async function deleteOrder(orderId, button) {
    const confirmed = window.confirm(
        "Вы действительно хотите удалить этот заказ? Это действие нельзя отменить"
    );

    if (!confirmed) {
        return;
    }

    button.disabled = true;

    try {
        await api(`/api/admin/orders/${orderId}`, { method: "DELETE" });
        toast(`Заказ #${orderId} удалён`, "success");
        await loadOrders();
    } catch (error) {
        toast(error.message, "error");
        button.disabled = false;
    }
}

async function handleOrderAction(event) {
    const button = event.target.closest("button[data-action]");
    if (!button) {
        return;
    }

    if (button.dataset.action === "details") {
        await openOrderDetails(Number(button.dataset.id));
    } else if (button.dataset.action === "delete") {
        await deleteOrder(Number(button.dataset.id), button);
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

function restoreSummaryHtml(summary) {
    const counts = summary.counts || {};
    const items = [
        ["Заказы", counts.orders || 0],
        ["Позиции заказов", counts.order_items || 0],
        ["Блюда", counts.dishes || 0],
        ["Категории", counts.categories || 0],
        ["Ингредиенты", counts.ingredients || 0],
        ["Фотографии", summary.photo_count || counts.photos || 0]
    ];

    return `
        <div class="backup-summary-grid">
            ${items.map(([label, value]) => `
                <div class="backup-summary-item">
                    <span class="backup-summary-label">${escapeHtml(label)}</span>
                    <strong>${Number(value).toLocaleString("ru-RU")}</strong>
                </div>
            `).join("")}
        </div>
        <div class="backup-summary-details">
            <div><span>Дата создания</span><strong>${escapeHtml(formatDate(summary.created_at))}</strong></div>
            <div><span>Размер архива</span><strong>${formatBytes(summary.archive_size)}</strong></div>
            <div><span>Размер базы</span><strong>${formatBytes(summary.database_size)}</strong></div>
            <div><span>Версия схемы</span><strong>${escapeHtml(String(summary.schema_version || "—"))}</strong></div>
        </div>
    `;
}

async function renderBackupsPage() {
    const backupState = state.backups;

    pageContent.innerHTML = `
        <div class="backup-page">
            <section class="backup-card">
                <div>
                    <h2>Скачать резервную копию</h2>
                    <p class="backup-description">
                        ZIP содержит все заказы и связанные данные, блюда, категории, ингредиенты
                        и загруженные фотографии. Служебная очередь Telegram-уведомлений,
                        пароли и настройки сервера в архив не попадают.
                    </p>
                </div>
                <div class="backup-actions">
                    <button class="primary-button" id="download-backup-button" type="button">
                        Скачать резервную копию
                    </button>
                </div>
            </section>

            <section class="backup-card backup-restore-card">
                <div>
                    <h2>Восстановить из копии</h2>
                    <p class="backup-description">
                        Загрузите ZIP, созданный этой админ-панелью. Перед применением сервер
                        проверит структуру архива, SQLite, внешние ключи и наличие всех фотографий.
                    </p>
                </div>

                <div class="backup-upload-row">
                    <input type="file" id="restore-file" accept=".zip,application/zip">
                    <button class="secondary-button" id="validate-restore-button" type="button" disabled>
                        Проверить архив
                    </button>
                </div>
                <div class="backup-progress" id="restore-progress" hidden>
                    <div class="backup-progress-bar" id="restore-progress-bar"></div>
                    <span id="restore-progress-text">Загрузка архива…</span>
                </div>

                <div class="backup-summary" id="restore-summary" hidden></div>

                <div class="backup-confirm" id="restore-confirm" hidden>
                    <div class="backup-danger-warning">
                        <strong>Внимание: полная замена данных.</strong>
                        Текущие заказы, блюда, категории, ингредиенты и фотографии будут
                        заменены содержимым архива. Это действие нельзя отменить без
                        страховочной копии.
                    </div>

                    <div class="backup-step">
                        <h3>1. Скачайте текущую страховочную копию</h3>
                        <p class="hint">
                            Скачайте ZIP с текущими данными и сохраните его на компьютере.
                            Только после этого восстановление будет разрешено.
                        </p>
                        <button class="secondary-button" id="download-safety-button" type="button">
                            Скачать текущую копию
                        </button>
                        <p class="backup-safety-status" id="safety-status"></p>
                        <label class="backup-checkbox">
                            <input type="checkbox" id="confirm-safety-checkbox">
                            <span>Я скачал(а) и сохранил(а) страховочную копию</span>
                        </label>
                    </div>

                    <div class="backup-step">
                        <h3>2. Подтвердите замену данных</h3>
                        <p class="hint">Введите точную фразу: <code>ЗАМЕНИТЬ ДАННЫЕ</code></p>
                        <input
                            type="text"
                            id="restore-phrase"
                            class="backup-phrase-input"
                            autocomplete="off"
                            spellcheck="false"
                            placeholder="ЗАМЕНИТЬ ДАННЫЕ"
                        >
                        <button class="primary-button backup-apply-button" id="apply-restore-button" type="button" disabled>
                            Восстановить данные
                        </button>
                    </div>
                </div>
            </section>
        </div>
    `;

    const fileInput = pageContent.querySelector("#restore-file");
    const validateButton = pageContent.querySelector("#validate-restore-button");
    const progress = pageContent.querySelector("#restore-progress");
    const progressBar = pageContent.querySelector("#restore-progress-bar");
    const progressText = pageContent.querySelector("#restore-progress-text");
    const summaryBox = pageContent.querySelector("#restore-summary");
    const confirmBox = pageContent.querySelector("#restore-confirm");
    const safetyButton = pageContent.querySelector("#download-safety-button");
    const safetyStatus = pageContent.querySelector("#safety-status");
    const safetyCheckbox = pageContent.querySelector("#confirm-safety-checkbox");
    const phraseInput = pageContent.querySelector("#restore-phrase");
    const applyButton = pageContent.querySelector("#apply-restore-button");

    const updateApplyState = () => {
        const phraseMatches = phraseInput.value === "ЗАМЕНИТЬ ДАННЫЕ";
        applyButton.disabled = backupState.applying
            || !backupState.safetyConfirmed
            || !phraseMatches;

        if (backupState.applying) {
            applyButton.textContent = "Восстановление…";
        } else {
            applyButton.textContent = "Восстановить данные";
        }
        safetyButton.disabled = backupState.applying;
    };

    const showSummary = () => {
        if (!backupState.restore) {
            summaryBox.hidden = true;
            confirmBox.hidden = true;
            updateApplyState();
            return;
        }

        summaryBox.hidden = false;
        summaryBox.innerHTML = restoreSummaryHtml(backupState.restore);
        confirmBox.hidden = false;

        if (backupState.safety) {
            safetyStatus.textContent = `Страховочная копия создана: ${formatDate(backupState.safety.created_at)}`;
        } else {
            safetyStatus.textContent = "";
        }
        safetyCheckbox.checked = backupState.safetyConfirmed;
        safetyCheckbox.disabled = !backupState.safety || backupState.applying;
        phraseInput.disabled = backupState.applying;
        updateApplyState();
    };

    fileInput.addEventListener("change", () => {
        validateButton.disabled = !fileInput.files || fileInput.files.length === 0;
        backupState.restore = null;
        backupState.safety = null;
        backupState.safetyConfirmed = false;
        showSummary();
    });

    pageContent.querySelector("#download-backup-button").addEventListener("click", async (event) => {
        const button = event.currentTarget;
        button.disabled = true;
        try {
            const info = await api("/api/admin/backups", { method: "POST" });
            startBackupDownload(info.id, info.created_at);
            toast("Скачивание резервной копии началось.", "success");
        } catch (error) {
            toast(error.message, "error");
        } finally {
            button.disabled = false;
        }
    });

    validateButton.addEventListener("click", async () => {
        const file = fileInput.files && fileInput.files[0];
        if (!file) {
            toast("Выберите ZIP-файл.", "error");
            return;
        }

        backupState.restore = null;
        backupState.safety = null;
        backupState.safetyConfirmed = false;
        showSummary();

        validateButton.disabled = true;
        progress.hidden = false;
        progressBar.style.width = "0%";
        progressText.textContent = "Загрузка архива…";

        try {
            const summary = await uploadRestoreArchive(file, (percent) => {
                progressBar.style.width = `${percent}%`;
                progressText.textContent = `Загрузка архива… ${percent}%`;
            });
            backupState.restore = summary;
            progressBar.style.width = "100%";
            progressText.textContent = "Архив проверен.";
            showSummary();
            toast("Архив проверен. Можно переходить к восстановлению.", "success");
        } catch (error) {
            progressText.textContent = "Проверка не удалась.";
            toast(error.message, "error");
        } finally {
            progress.hidden = true;
            validateButton.disabled = false;
        }
    });

    safetyButton.addEventListener("click", async () => {
        if (!backupState.restore) {
            return;
        }

        safetyButton.disabled = true;
        backupState.safetyConfirmed = false;
        safetyCheckbox.checked = false;
        try {
            const info = await api("/api/admin/backups", { method: "POST" });
            backupState.safety = info;
            startBackupDownload(info.id, info.created_at);
            safetyStatus.textContent = "Дождитесь завершения скачивания, затем подтвердите сохранение копии.";
            safetyCheckbox.disabled = false;
            updateApplyState();
        } catch (error) {
            toast(error.message, "error");
        } finally {
            safetyButton.disabled = false;
        }
    });

    safetyCheckbox.addEventListener("change", async () => {
        if (!safetyCheckbox.checked) {
            backupState.safetyConfirmed = false;
            updateApplyState();
            return;
        }
        if (!backupState.safety) {
            safetyCheckbox.checked = false;
            return;
        }

        safetyCheckbox.disabled = true;
        try {
            await api(`/api/admin/backups/${encodeURIComponent(backupState.safety.id)}/confirm`, {
                method: "POST"
            });
            backupState.safetyConfirmed = true;
            safetyStatus.textContent = `Страховочная копия подтверждена: ${formatDate(backupState.safety.created_at)}`;
            toast("Сохранение страховочной копии подтверждено.", "success");
        } catch (error) {
            safetyCheckbox.checked = false;
            backupState.safetyConfirmed = false;
            toast(error.message, "error");
        } finally {
            safetyCheckbox.disabled = false;
            updateApplyState();
        }
    });

    phraseInput.addEventListener("input", updateApplyState);

    applyButton.addEventListener("click", async () => {
        if (!backupState.restore || !backupState.safety || !backupState.safetyConfirmed) {
            return;
        }
        if (phraseInput.value !== "ЗАМЕНИТЬ ДАННЫЕ") {
            toast("Введите точную фразу: ЗАМЕНИТЬ ДАННЫЕ", "error");
            return;
        }

        const confirmed = window.confirm(
            "Заменить все текущие данные содержимым архива? Это действие изменяет заказы, блюда и фотографии."
        );
        if (!confirmed) {
            return;
        }

        backupState.applying = true;
        updateApplyState();
        try {
            const result = await api(`/api/admin/restores/${encodeURIComponent(backupState.restore.restore_id)}/apply`, {
                method: "POST",
                body: JSON.stringify({
                    confirmation: phraseInput.value,
                    safety_backup_id: backupState.safety.id
                })
            });
            toast(result.message || "Данные успешно восстановлены.", "success");
            state.backups = {
                restore: null,
                safety: null,
                safetyConfirmed: false,
                applying: false
            };
            await renderBackupsPage();
        } catch (error) {
            toast(error.message, "error");
        } finally {
            backupState.applying = false;
            updateApplyState();
        }
    });

    showSummary();
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
    categories: {
        title: "Фильтры",
        render: renderCategoriesPage
    },
    orders: {
        title: "Заказы",
        render: renderOrdersPage
    },
    backups: {
        title: "Резервные копии",
        render: renderBackupsPage
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
