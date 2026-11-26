"use strict";

const root = document.querySelector("#order-root");

const UNIT_LABELS = {
    g: "г",
    ml: "мл",
    pcs: "шт"
};

function unitLabel(unit) {
    return UNIT_LABELS[unit] || unit;
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
    return `${(Number(value) || 0).toLocaleString("ru-RU")} ₽`;
}

function formatQuantity(value) {
    return (Number(value) || 0).toLocaleString("ru-RU", {
        maximumFractionDigits: 3
    });
}

function formatDate(value) {
    if (!value) {
        return "";
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
        month: "long",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit"
    });
}

function getToken() {
    const parts = window.location.pathname.split("/").filter(Boolean);

    if (parts[0] === "order" && parts[1]) {
        try {
            return decodeURIComponent(parts[1]);
        } catch (error) {
            return parts[1];
        }
    }

    return new URLSearchParams(window.location.search).get("token") || "";
}

async function copyText(text) {
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
        return true;
    } catch (error) {
        return false;
    }
}

function showError(title, message) {
    root.innerHTML = `
        <div class="order-error">
            <h1>${escapeHtml(title)}</h1>
            <p>${escapeHtml(message)}</p>
            <button class="order-button" type="button" id="retry-button">Обновить</button>
        </div>
    `;

    const retryButton = root.querySelector("#retry-button");
    if (retryButton) {
        retryButton.addEventListener("click", () => window.location.reload());
    }
}

function renderOrder(order, token) {
    const items = order.items.map((item) => `
        <tr>
            <td>
                <div class="order-item-name">${escapeHtml(item.dish_name)}</div>
                ${item.weight_grams ? `<div class="order-item-weight">${item.weight_grams} г</div>` : ""}
            </td>
            <td class="num">${formatMoney(item.unit_price)}</td>
            <td class="num">${item.quantity}</td>
            <td class="num">${formatMoney(item.line_total)}</td>
        </tr>
    `).join("");

    const storageKey = `food-order-${token}-checked`;
    let checked = new Set();

    try {
        checked = new Set(JSON.parse(localStorage.getItem(storageKey) || "[]"));
    } catch (error) {
        checked = new Set();
    }

    const sortedIngredients = [...order.ingredients].sort((a, b) =>
        a.name.localeCompare(b.name, "ru")
    );

    const ingredients = sortedIngredients.map((ingredient) => {
        const key = `${ingredient.name}|${ingredient.unit}`;
        const isChecked = checked.has(key) ? "checked" : "";

        return `
            <li class="shopping-item">
                <label>
                    <input type="checkbox" data-key="${escapeHtml(key)}" ${isChecked}>
                    <span class="shopping-name">${escapeHtml(ingredient.name)}</span>
                    <span class="shopping-quantity">${formatQuantity(ingredient.quantity)} ${unitLabel(ingredient.unit)}</span>
                </label>
            </li>
        `;
    }).join("");

    const currentLink = window.location.href.split("#")[0];

    root.innerHTML = `
        <article class="order-card">
            <header class="order-header">
                <div>
                    <div class="order-brand">Заказ</div>
                    <h1 class="order-title">Ваш заказ</h1>
                    <p class="order-subtitle">${formatDate(order.created_at)}</p>
                </div>
                <div class="order-number">
                    Номер заказа
                    <strong>#${order.order_id}</strong>
                </div>
            </header>

            <section class="order-section">
                <h2>Блюда</h2>
                <div class="order-table-wrap">
                    <table class="order-table">
                        <thead>
                            <tr>
                                <th>Блюдо</th>
                                <th class="num">Цена</th>
                                <th class="num">Кол-во</th>
                                <th class="num">Сумма</th>
                            </tr>
                        </thead>
                        <tbody>
                            ${items}
                        </tbody>
                    </table>
                </div>
                <div class="order-total">
                    Итого
                    <strong>${formatMoney(order.total)}</strong>
                </div>
            </section>

            <section class="order-section">
                <h2>Продукты, которые нужно купить</h2>
                ${ingredients
                    ? `<ul class="shopping-list">${ingredients}</ul>
                       <p class="order-note">Отмечайте купленные продукты — галочки сохранятся в этом браузере.</p>`
                    : `<p class="order-note">Для этого заказа продукты не требуются.</p>`}
            </section>

            <section class="order-section order-link-section">
                <h2>Ссылка на заказ</h2>
                <div class="order-link-box">
                    <a href="${escapeHtml(currentLink)}">${escapeHtml(currentLink)}</a>
                    <button class="order-button" type="button" id="copy-link">Копировать</button>
                </div>
                <p class="order-note">Сохраните эту ссылку — по ней заказ можно открыть снова.</p>
            </section>

            <div class="order-actions">
                <button class="order-button order-button-primary" type="button" id="print-order">
                    Печать / Сохранить PDF
                </button>
                <button class="order-button" type="button" id="copy-link-bottom">
                    Скопировать ссылку
                </button>
            </div>
        </article>
    `;

    const copyLink = async () => {
        const copied = await copyText(currentLink);
        const button = root.querySelector("#copy-link");
        const bottomButton = root.querySelector("#copy-link-bottom");

        if (copied) {
            if (button) button.textContent = "Скопировано";
            if (bottomButton) bottomButton.textContent = "Ссылка скопирована";
            window.setTimeout(() => {
                if (button) button.textContent = "Копировать";
                if (bottomButton) bottomButton.textContent = "Скопировать ссылку";
            }, 2500);
        } else {
            window.prompt("Скопируйте ссылку вручную:", currentLink);
        }
    };

    root.querySelector("#copy-link").addEventListener("click", copyLink);
    root.querySelector("#copy-link-bottom").addEventListener("click", copyLink);

    root.querySelector("#print-order").addEventListener("click", () => {
        window.print();
    });

    const checkboxes = root.querySelectorAll(".shopping-item input[type='checkbox']");

    checkboxes.forEach((checkbox) => {
        checkbox.addEventListener("change", () => {
            const current = new Set();

            checkboxes.forEach((item) => {
                if (item.checked) {
                    current.add(item.dataset.key);
                }
            });

            try {
                localStorage.setItem(storageKey, JSON.stringify([...current]));
            } catch (error) {
                // Приватный режим или переполнение хранилища — галочки просто не сохранятся.
            }
        });
    });
}

async function main() {
    const token = getToken();

    if (!token) {
        showError("Ссылка неполная", "Откройте ссылку на заказ из сообщения или скопируйте её полностью.");
        return;
    }

    try {
        const response = await fetch(`/api/orders/${encodeURIComponent(token)}`, { cache: "no-store" });

        if (response.status === 404) {
            showError("Заказ не найден", "Проверьте ссылку. Возможно, она скопирована не полностью.");
            return;
        }

        if (!response.ok) {
            showError("Не удалось загрузить заказ", `Сервер вернул ошибку ${response.status}. Попробуйте обновить страницу.`);
            return;
        }

        const order = await response.json();
        renderOrder(order, token);
    } catch (error) {
        showError("Не удалось загрузить заказ", "Проверьте подключение к интернету и обновите страницу.");
    }
}

main();
