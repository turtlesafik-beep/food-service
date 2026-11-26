"use strict";

const CART_STORAGE_KEY = "food-service-cart-v1";

const menuList = document.querySelector("#menu-list");
const menuStatus = document.querySelector("#menu-status");
const floatingCart = document.querySelector("#floating-cart");
const floatingCartSummary = document.querySelector("[data-cart-summary]");
const cartCounters = document.querySelectorAll("[data-cart-count]");
const cartDrawer = document.querySelector("#cart-drawer");
const cartBackdrop = document.querySelector("#cart-backdrop");
const cartBody = document.querySelector("#cart-body");
const cartFoot = document.querySelector("#cart-foot");
const cartClose = document.querySelector("#cart-close");
const checkoutModal = document.querySelector("#checkout-modal");
const checkoutBackdrop = document.querySelector("#checkout-backdrop");
const checkoutSummary = document.querySelector("#checkout-summary");
const checkoutForm = document.querySelector("#checkout-form");
const checkoutError = document.querySelector("#checkout-error");
const checkoutSubmit = document.querySelector("#checkout-submit");
const checkoutBack = document.querySelector("#checkout-back");
const checkoutClose = document.querySelector("#checkout-close");
const customerName = document.querySelector("#customer-name");
const customerPhone = document.querySelector("#customer-phone");
const customerComment = document.querySelector("#customer-comment");
const nameError = document.querySelector("#name-error");
const phoneError = document.querySelector("#phone-error");
const toastStack = document.querySelector("#toast-stack");

const state = {
    dishes: [],
    menuLoaded: false,
    menuLoading: false,
    menuError: null,
    cart: [],
    isCartOpen: false,
    isCheckoutOpen: false,
    isSubmitting: false,
    checkoutDraft: {
        name: "",
        phone: "",
        comment: ""
    }
};

let lastCartTrigger = null;
let lastCheckoutTrigger = null;

function escapeHtml(value) {
    return String(value ?? "")
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#39;");
}

function formatMoney(value) {
    return `${(Number(value) || 0).toLocaleString("ru-RU")} ₽`;
}

function formatQuantity(value) {
    return (Number(value) || 0).toLocaleString("ru-RU", {
        maximumFractionDigits: 3
    });
}

function unitLabel(unit) {
    const labels = { g: "г", ml: "мл", pcs: "шт." };
    return labels[unit] || unit;
}

function plural(count, one, few, many) {
    const mod10 = count % 10;
    const mod100 = count % 100;

    if (mod10 === 1 && mod100 !== 11) {
        return one;
    }
    if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) {
        return few;
    }
    return many;
}

function toast(message, type = "info") {
    const element = document.createElement("div");
    element.className = `toast toast-${type}`;
    element.textContent = message;
    toastStack.appendChild(element);

    window.setTimeout(() => {
        element.style.opacity = "0";
        element.style.transform = "translateY(6px)";
        window.setTimeout(() => element.remove(), 220);
    }, 3200);
}

function loadCart() {
    try {
        const raw = localStorage.getItem(CART_STORAGE_KEY);
        const parsed = JSON.parse(raw || "[]");

        if (!Array.isArray(parsed)) {
            return [];
        }

        const merged = new Map();

        for (const item of parsed) {
            const dishId = Number(item && (item.dishId ?? item.dish_id));
            const quantity = Number(item && item.quantity);

            if (!Number.isInteger(dishId) || dishId <= 0) {
                continue;
            }
            if (!Number.isInteger(quantity) || quantity <= 0) {
                continue;
            }

            merged.set(dishId, (merged.get(dishId) || 0) + quantity);
        }

        return [...merged.entries()].map(([dishId, quantity]) => ({ dishId, quantity }));
    } catch (error) {
        return [];
    }
}

function saveCart() {
    try {
        localStorage.setItem(CART_STORAGE_KEY, JSON.stringify(state.cart));
    } catch (error) {
        // Приватный режим или переполненное хранилище: корзина продолжит работать в памяти.
    }
}

function getDish(dishId) {
    return state.dishes.find((dish) => dish.id === dishId) || null;
}

function getCartDetails() {
    const items = [];

    for (const item of state.cart) {
        const dish = getDish(item.dishId);
        if (!dish) {
            continue;
        }

        items.push({
            dish,
            quantity: item.quantity,
            lineTotal: Number(dish.price || 0) * item.quantity
        });
    }

    return {
        items,
        totalQuantity: items.reduce((sum, item) => sum + item.quantity, 0),
        total: items.reduce((sum, item) => sum + item.lineTotal, 0)
    };
}

function reconcileCart() {
    if (!state.menuLoaded) {
        return;
    }

    const availableIds = new Set(state.dishes.map((dish) => dish.id));
    const before = state.cart.length;

    state.cart = state.cart.filter((item) => availableIds.has(item.dishId));

    if (state.cart.length !== before) {
        saveCart();
        toast("Некоторые блюда стали недоступны и удалены из корзины.", "error");
    }
}

function updateCartUI() {
    const details = getCartDetails();
    const lineCount = state.menuLoaded ? details.items.length : state.cart.length;
    const totalQuantity = state.menuLoaded
        ? details.totalQuantity
        : state.cart.reduce((sum, item) => sum + item.quantity, 0);

    cartCounters.forEach((counter) => {
        counter.textContent = String(totalQuantity);
    });

    if (lineCount === 0) {
        floatingCart.hidden = true;
        document.body.classList.remove("has-floating-cart");
    } else {
        floatingCart.hidden = false;
        document.body.classList.add("has-floating-cart");
        floatingCartSummary.textContent = `${lineCount} ${plural(lineCount, "позиция", "позиции", "позиций")} · ${
            state.menuLoaded ? formatMoney(details.total) : "—"
        }`;
    }

    if (state.isCartOpen) {
        renderCartDrawer();
    }
    if (state.isCheckoutOpen) {
        renderCheckoutSummary();
    }
}

function addToCart(dishId) {
    const dish = getDish(dishId);
    if (!dish) {
        return;
    }

    const item = state.cart.find((entry) => entry.dishId === dishId);

    if (item) {
        item.quantity += 1;
    } else {
        state.cart.push({ dishId, quantity: 1 });
    }

    saveCart();
    updateCartUI();
    toast(`«${dish.name}» добавлено в корзину`, "success");

    floatingCart.classList.remove("is-bumped");
    void floatingCart.offsetWidth;
    floatingCart.classList.add("is-bumped");
    window.setTimeout(() => floatingCart.classList.remove("is-bumped"), 360);
}

function changeQuantity(dishId, delta) {
    const item = state.cart.find((entry) => entry.dishId === dishId);
    if (!item) {
        return;
    }

    item.quantity += delta;

    if (item.quantity <= 0) {
        state.cart = state.cart.filter((entry) => entry.dishId !== dishId);
    }

    saveCart();
    updateCartUI();
}

function removeFromCart(dishId) {
    state.cart = state.cart.filter((entry) => entry.dishId !== dishId);
    saveCart();
    updateCartUI();
    toast("Блюдо удалено из корзины", "info");
}

function dishCardHtml(dish) {
    const ingredients = Array.isArray(dish.ingredients) ? dish.ingredients : [];
    const photoUrl = String(dish.photo_url || "").trim();
    const emoji = String(dish.emoji || "").trim() || "🍽️";
    const name = String(dish.name || "").trim() || "Блюдо";
    const description = String(dish.description || "").trim() || "Описание уточняется.";
    const weight = Number(dish.weight_grams) > 0 ? `${dish.weight_grams} г` : "";
    const price = Number(dish.price) || 0;

    const previewChips = ingredients.slice(0, 4).map((ingredient) => `
        <span class="recipe-chip">${escapeHtml(ingredient.name)}</span>
    `).join("");

    const moreChip = ingredients.length > 4
        ? `<span class="recipe-chip recipe-more">+${ingredients.length - 4}</span>`
        : "";

    const fullChips = ingredients.map((ingredient) => `
        <span class="recipe-chip">
            <strong>${escapeHtml(ingredient.name)}</strong>
            ${formatQuantity(ingredient.quantity)} ${unitLabel(ingredient.unit)}
        </span>
    `).join("");

    const recipeHtml = ingredients.length > 0
        ? `
            <div class="dish-recipe">
                <span class="dish-recipe-label">Состав</span>
                <div class="recipe-chips recipe-chips-preview">${previewChips}${moreChip}</div>
                <div class="recipe-chips recipe-chips-full">${fullChips}</div>
            </div>
        `
        : `
            <div class="dish-recipe">
                <span class="dish-recipe-label">Состав</span>
                <p class="dish-recipe-empty">Состав уточняется.</p>
            </div>
        `;

    const photoHtml = photoUrl
        ? `
            <div class="dish-photo-reveal">
                <div class="dish-photo-reveal-inner">
                    <div class="dish-photo">
                        <img
                            src="${escapeHtml(photoUrl)}"
                            alt="Фотография блюда «${escapeHtml(name)}»"
                            loading="lazy"
                        >
                    </div>
                </div>
            </div>
        `
        : "";

    return `
        <article class="dish-card" id="dish-card-${dish.id}" data-dish-id="${dish.id}">
            <div class="dish-head">
                <span class="dish-emoji" aria-hidden="true">${escapeHtml(emoji)}</span>
                <div class="dish-title-wrap">
                    <h3 class="dish-title">${escapeHtml(name)}</h3>
                    <p class="dish-description">${escapeHtml(description)}</p>
                </div>
            </div>

            ${recipeHtml}
            ${photoHtml}

            <div class="dish-footer">
                <div class="dish-price-block">
                    <span class="dish-price">${formatMoney(price)}</span>
                    ${weight ? `<span class="dish-weight">${weight}</span>` : ""}
                </div>

                <div class="dish-actions">
                    <button
                        class="dish-details-button"
                        type="button"
                        data-action="details"
                        aria-expanded="false"
                    >Подробнее</button>
                    <button
                        class="dish-add"
                        type="button"
                        data-action="add"
                        data-dish-id="${dish.id}"
                        aria-label="Добавить «${escapeHtml(name)}» в корзину"
                    >+</button>
                </div>
            </div>
        </article>
    `;
}

function renderMenu() {
    if (state.dishes.length === 0) {
        menuList.innerHTML = `
            <div class="menu-empty">
                <div class="menu-empty-icon" aria-hidden="true">🌿</div>
                <h3>Меню пока пусто</h3>
                <p>Загляните позже — мы обновляем меню и готовим новые блюда.</p>
            </div>
        `;
        menuStatus.textContent = "Пока нет доступных блюд.";
        menuList.setAttribute("aria-busy", "false");
        return;
    }

    menuList.innerHTML = state.dishes.map(dishCardHtml).join("");
    menuStatus.textContent = `${state.dishes.length} ${plural(
        state.dishes.length,
        "блюдо",
        "блюда",
        "блюд"
    )} в меню`;
    menuList.setAttribute("aria-busy", "false");

    menuList.querySelectorAll(".dish-photo img").forEach((image) => {
        image.addEventListener("error", () => {
            const reveal = image.closest(".dish-photo-reveal");
            if (reveal) {
                reveal.remove();
            } else {
                image.closest(".dish-photo")?.remove();
            }
        }, { once: true });
    });
}

function renderMenuSkeleton() {
    menuList.innerHTML = Array.from({ length: 3 }, () => `
        <article class="dish-card skeleton-card" aria-hidden="true">
            <div class="skeleton-line skeleton-title"></div>
            <div class="skeleton-line"></div>
            <div class="skeleton-line skeleton-short"></div>
            <div class="skeleton-chips">
                <span class="skeleton-chip"></span>
                <span class="skeleton-chip"></span>
                <span class="skeleton-chip"></span>
            </div>
        </article>
    `).join("");
}

function renderMenuError(error) {
    menuList.innerHTML = `
        <div class="menu-empty">
            <div class="menu-empty-icon" aria-hidden="true">🍽️</div>
            <h3>Не удалось загрузить меню</h3>
            <p>Проверьте подключение к интернету и попробуйте ещё раз.</p>
            <button class="button button-ghost" type="button" data-menu-retry>Повторить</button>
        </div>
    `;
    menuStatus.textContent = "";
    menuList.setAttribute("aria-busy", "false");
}

async function loadMenu({ silent = false } = {}) {
    if (state.menuLoading) {
        return;
    }

    state.menuLoading = true;
    state.menuError = null;

    if (!silent) {
        renderMenuSkeleton();
        menuStatus.textContent = "Загружаем меню…";
    }

    menuList.setAttribute("aria-busy", "true");

    if (state.isCartOpen) {
        renderCartDrawer();
    }

    try {
        const response = await fetch("/api/dishes", { cache: "no-store" });

        if (!response.ok) {
            throw new Error(`Ошибка ${response.status}`);
        }

        const dishes = await response.json();

        if (!Array.isArray(dishes)) {
            throw new Error("Некорректный ответ сервера");
        }

        state.dishes = dishes;
        state.menuLoaded = true;
        state.menuError = null;
        state.menuLoading = false;

        reconcileCart();
        renderMenu();
        updateCartUI();
    } catch (error) {
        state.menuLoading = false;
        state.menuError = error;

        if (!state.menuLoaded) {
            state.dishes = [];
            renderMenuError(error);
            updateCartUI();
        } else {
            toast("Не удалось обновить меню. Показано последнее загруженное.", "error");
            menuList.setAttribute("aria-busy", "false");
        }

        if (state.isCartOpen) {
            renderCartDrawer();
        }
        if (state.isCheckoutOpen) {
            renderCheckoutSummary();
        }
    }
}

function handleMenuClick(event) {
    const retryButton = event.target.closest("[data-menu-retry]");
    if (retryButton) {
        loadMenu();
        return;
    }

    const actionButton = event.target.closest("[data-action]");
    if (!actionButton) {
        return;
    }

    const card = actionButton.closest(".dish-card");
    if (!card) {
        return;
    }

    if (actionButton.dataset.action === "details") {
        const expanded = card.classList.toggle("is-expanded");
        actionButton.setAttribute("aria-expanded", String(expanded));
        actionButton.textContent = expanded ? "Свернуть" : "Подробнее";
        return;
    }

    if (actionButton.dataset.action === "add") {
        addToCart(Number(actionButton.dataset.dishId || card.dataset.dishId));
    }
}

function cartItemHtml(item) {
    const dish = item.dish;
    const weight = Number(dish.weight_grams) > 0 ? `${dish.weight_grams} г · ` : "";

    return `
        <div class="cart-item" data-dish-id="${dish.id}">
            <div>
                <h3 class="cart-item-title">${escapeHtml(dish.name)}</h3>
                <p class="cart-item-meta">${weight}${formatMoney(dish.price)} за порцию</p>
                <p class="cart-item-price">${formatMoney(item.lineTotal)}</p>
            </div>

            <div class="cart-item-controls">
                <div class="quantity-control" aria-label="Количество блюда «${escapeHtml(dish.name)}»">
                    <button
                        class="quantity-button"
                        type="button"
                        data-cart-action="decrease"
                        data-dish-id="${dish.id}"
                        aria-label="Уменьшить количество «${escapeHtml(dish.name)}»"
                    >−</button>
                    <span class="quantity-value">${item.quantity}</span>
                    <button
                        class="quantity-button"
                        type="button"
                        data-cart-action="increase"
                        data-dish-id="${dish.id}"
                        aria-label="Увеличить количество «${escapeHtml(dish.name)}»"
                    >+</button>
                </div>

                <button
                    class="cart-remove"
                    type="button"
                    data-cart-action="remove"
                    data-dish-id="${dish.id}"
                >Удалить</button>
            </div>
        </div>
    `;
}

function renderCartDrawer() {
    if (!state.menuLoaded) {
        if (state.menuLoading) {
            cartBody.innerHTML = `
                <div class="cart-empty">
                    <div class="cart-empty-icon" aria-hidden="true">⏳</div>
                    <h3>Загружаем меню</h3>
                    <p>Это займёт несколько секунд.</p>
                </div>
            `;
            cartFoot.innerHTML = "";
            return;
        }

        cartBody.innerHTML = `
            <div class="cart-empty">
                <div class="cart-empty-icon" aria-hidden="true">🍽️</div>
                <h3>Меню недоступно</h3>
                <p>Не удалось загрузить блюда. Проверьте соединение и попробуйте снова.</p>
                <button class="button button-ghost" type="button" data-cart-action="retry">Повторить</button>
            </div>
        `;
        cartFoot.innerHTML = "";
        return;
    }

    const details = getCartDetails();

    if (details.items.length === 0) {
        cartBody.innerHTML = `
            <div class="cart-empty">
                <div class="cart-empty-icon" aria-hidden="true">🌿</div>
                <h3>В корзине пока пусто</h3>
                <p>Выберите блюда из меню — они появятся здесь.</p>
                <button class="button button-ghost" type="button" data-cart-action="go-menu">Перейти к меню</button>
            </div>
        `;
        cartFoot.innerHTML = "";
        return;
    }

    cartBody.innerHTML = details.items.map(cartItemHtml).join("");

    cartFoot.innerHTML = `
        <div class="cart-total">
            <span class="cart-total-label">Всего порций: ${details.totalQuantity}</span>
            <span class="cart-total-value">${formatMoney(details.total)}</span>
        </div>
        <button class="button button-primary" type="button" data-cart-action="checkout">
            Отправить заказ
        </button>
        <p class="cart-foot-hint">Стоимость и наличие проверим при отправке.</p>
    `;
}

function renderCheckoutSummary() {
    const details = getCartDetails();

    if (details.items.length === 0) {
        checkoutSummary.innerHTML = `<p class="checkout-summary-title">Корзина пуста</p>`;
        return;
    }

    checkoutSummary.innerHTML = `
        <p class="checkout-summary-title">
            Ваш заказ · ${details.totalQuantity} ${plural(details.totalQuantity, "порция", "порции", "порций")}
        </p>
        <ul class="checkout-summary-list">
            ${details.items.map((item) => `
                <li class="checkout-summary-row">
                    <span>${escapeHtml(item.dish.name)} × ${item.quantity}</span>
                    <strong>${formatMoney(item.lineTotal)}</strong>
                </li>
            `).join("")}
        </ul>
        <div class="checkout-summary-total">
            <span>Итого</span>
            <span>${formatMoney(details.total)}</span>
        </div>
    `;
}

function syncScrollLock() {
    document.body.classList.toggle("no-scroll", state.isCartOpen || state.isCheckoutOpen);
}

function openCart() {
    if (state.isCheckoutOpen) {
        return;
    }

    lastCartTrigger = document.activeElement instanceof HTMLElement ? document.activeElement : null;

    state.isCartOpen = true;
    cartDrawer.hidden = false;
    cartBackdrop.hidden = false;
    renderCartDrawer();
    syncScrollLock();
    cartClose.focus();
}

function closeCart({ restoreFocus = true } = {}) {
    state.isCartOpen = false;
    cartDrawer.hidden = true;
    cartBackdrop.hidden = true;
    syncScrollLock();

    if (restoreFocus && lastCartTrigger && lastCartTrigger.isConnected) {
        lastCartTrigger.focus();
    }
}

function clearFieldError(input, errorElement) {
    if (errorElement) {
        errorElement.hidden = true;
        errorElement.textContent = "";
    }
    if (input) {
        input.removeAttribute("aria-invalid");
    }
}

function setFieldError(input, errorElement, message) {
    if (errorElement) {
        errorElement.hidden = false;
        errorElement.textContent = message;
    }
    if (input) {
        input.setAttribute("aria-invalid", "true");
    }
}

function clearCheckoutErrors() {
    checkoutError.hidden = true;
    checkoutError.textContent = "";
    clearFieldError(customerName, nameError);
    clearFieldError(customerPhone, phoneError);
}

function fillCheckoutDraft() {
    customerName.value = state.checkoutDraft.name;
    customerPhone.value = state.checkoutDraft.phone;
    customerComment.value = state.checkoutDraft.comment;
}

function setSubmitting(submitting) {
    state.isSubmitting = submitting;
    checkoutSubmit.disabled = submitting;
    checkoutSubmit.textContent = submitting ? "Отправляем…" : "Отправить заказ";
}

function openCheckout() {
    if (state.cart.length === 0) {
        toast("Корзина пуста.", "error");
        renderCartDrawer();
        return;
    }

    lastCheckoutTrigger = document.activeElement instanceof HTMLElement ? document.activeElement : null;

    closeCart({ restoreFocus: false });

    state.isCheckoutOpen = true;
    checkoutModal.hidden = false;
    checkoutBackdrop.hidden = false;

    renderCheckoutSummary();
    fillCheckoutDraft();
    clearCheckoutErrors();
    setSubmitting(false);

    syncScrollLock();
    customerName.focus();
}

function closeCheckout({ restoreFocus = true } = {}) {
    state.isCheckoutOpen = false;
    checkoutModal.hidden = true;
    checkoutBackdrop.hidden = true;
    syncScrollLock();

    if (restoreFocus) {
        const target = !floatingCart.hidden ? floatingCart : document.querySelector(".nav-cart");
        target?.focus();
    } else if (lastCheckoutTrigger && lastCheckoutTrigger.isConnected) {
        lastCheckoutTrigger.focus();
    }
}

function orderErrorMessage(message) {
    if (message === "customer name is required") {
        return "Укажите имя.";
    }
    if (message === "customer phone is required") {
        return "Укажите телефон или WhatsApp.";
    }
    if (message === "order must contain at least one item") {
        return "Корзина пуста. Вернитесь в меню и выберите блюда.";
    }
    if (/^dish (is unavailable|not found|has no ingredients):/.test(message)) {
        return "Одно из блюд стало недоступно. Мы обновили меню — проверьте корзину.";
    }
    if (/^duplicate dish ID:/.test(message) || message === "quantity must be positive") {
        return "Проверьте количество блюд в корзине.";
    }
    return "Не удалось отправить заказ. Попробуйте ещё раз.";
}

async function handleCheckoutSubmit(event) {
    event.preventDefault();

    if (state.isSubmitting) {
        return;
    }

    clearCheckoutErrors();

    const name = customerName.value.trim();
    const phone = customerPhone.value.trim();
    const comment = customerComment.value.trim();

    let firstInvalid = null;

    if (!name) {
        setFieldError(customerName, nameError, "Укажите имя.");
        firstInvalid = firstInvalid || customerName;
    }

    if (!phone) {
        setFieldError(customerPhone, phoneError, "Укажите телефон или WhatsApp.");
        firstInvalid = firstInvalid || customerPhone;
    }

    if (firstInvalid) {
        firstInvalid.focus();
        return;
    }

    if (state.cart.length === 0) {
        checkoutError.hidden = false;
        checkoutError.textContent = "Корзина пуста. Вернитесь в меню и выберите блюда.";
        return;
    }

    setSubmitting(true);

    const payload = {
        customer_name: name,
        customer_phone: phone,
        comment,
        items: state.cart.map((item) => ({
            dish_id: item.dishId,
            quantity: item.quantity
        }))
    };

    try {
        const response = await fetch("/api/orders", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify(payload),
            cache: "no-store"
        });

        if (!response.ok) {
            const rawMessage = (await response.text()).trim();
            const message = orderErrorMessage(rawMessage);

            checkoutError.hidden = false;
            checkoutError.textContent = message;

            if (/^dish (is unavailable|not found|has no ingredients):/.test(rawMessage)) {
                await loadMenu({ silent: true });
                renderCheckoutSummary();

                if (state.cart.length === 0) {
                    closeCheckout({ restoreFocus: false });
                    openCart();
                }
            }

            setSubmitting(false);
            return;
        }

        const result = await response.json();

        if (!result || !result.token) {
            throw new Error("Сервер не вернул ссылку на заказ.");
        }

        state.cart = [];
        state.checkoutDraft = { name: "", phone: "", comment: "" };
        saveCart();
        updateCartUI();
        setSubmitting(false);
        closeCheckout({ restoreFocus: false });

        window.location.assign(`/order/${encodeURIComponent(result.token)}`);
    } catch (error) {
        checkoutError.hidden = false;
        checkoutError.textContent = error.message && error.message.startsWith("Сервер")
            ? error.message
            : "Не удалось связаться с сервером. Проверьте соединение и попробуйте снова.";
        setSubmitting(false);
    }
}

function bindEvents() {
    document.querySelectorAll("[data-open-cart]").forEach((button) => {
        button.addEventListener("click", openCart);
    });

    menuList.addEventListener("click", handleMenuClick);

    cartClose.addEventListener("click", () => closeCart());
    cartBackdrop.addEventListener("click", () => closeCart());

    cartBody.addEventListener("click", (event) => {
        const button = event.target.closest("[data-cart-action]");
        if (!button) {
            return;
        }

        const action = button.dataset.cartAction;

        if (action === "retry") {
            loadMenu();
            return;
        }

        if (action === "go-menu") {
            closeCart();
            document.querySelector("#menu")?.scrollIntoView({ behavior: "smooth" });
            return;
        }

        const dishId = Number(button.dataset.dishId);
        if (!dishId) {
            return;
        }

        if (action === "increase") {
            changeQuantity(dishId, 1);
        } else if (action === "decrease") {
            changeQuantity(dishId, -1);
        } else if (action === "remove") {
            removeFromCart(dishId);
        }

        const nextButton = cartBody.querySelector(
            `[data-cart-action="${action}"][data-dish-id="${dishId}"]`
        );

        (nextButton || cartClose).focus();
    });

    cartFoot.addEventListener("click", (event) => {
        const button = event.target.closest("[data-cart-action]");
        if (button && button.dataset.cartAction === "checkout") {
            openCheckout();
        }
    });

    checkoutBackdrop.addEventListener("click", () => closeCheckout());
    checkoutClose.addEventListener("click", () => closeCheckout());

    checkoutBack.addEventListener("click", () => {
        closeCheckout({ restoreFocus: false });
        openCart();
    });

    checkoutForm.addEventListener("submit", handleCheckoutSubmit);

    customerName.addEventListener("input", () => {
        state.checkoutDraft.name = customerName.value;
        clearFieldError(customerName, nameError);
    });

    customerPhone.addEventListener("input", () => {
        state.checkoutDraft.phone = customerPhone.value;
        clearFieldError(customerPhone, phoneError);
    });

    customerComment.addEventListener("input", () => {
        state.checkoutDraft.comment = customerComment.value;
    });

    document.addEventListener("keydown", (event) => {
        if (event.key !== "Escape") {
            return;
        }

        if (state.isCheckoutOpen) {
            event.preventDefault();
            closeCheckout();
        } else if (state.isCartOpen) {
            event.preventDefault();
            closeCart();
        }
    });

    window.addEventListener("storage", (event) => {
        if (event.key !== CART_STORAGE_KEY) {
            return;
        }

        state.cart = loadCart();

        if (state.menuLoaded) {
            reconcileCart();
        }

        updateCartUI();
    });
}

function init() {
    state.cart = loadCart();
    updateCartUI();
    bindEvents();
    loadMenu();
}

init();
