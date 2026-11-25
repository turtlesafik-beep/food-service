const navButtons = document.querySelectorAll(".nav-button");
const pageTitle = document.querySelector("#page-title");
const pageContent = document.querySelector("#page-content");


const pages = {
    dishes: {
        title: "Блюда",
        content: `
            <div class="page-actions">
                <button class="primary-button" id="add-dish-button">
                    + Добавить блюдо
                </button>
            </div>

            <div class="table-container">
                <table class="dishes-table">
                    <thead>
                        <tr>
                            <th>Блюдо</th>
                            <th>Вес порции</th>
                            <th>Цена</th>
                            <th>Статус</th>
                            <th>Действия</th>
                        </tr>
                    </thead>
                    <tbody id="dishes-list">
                        <tr>
                            <td colspan="5">Загрузка блюд...</td>
                        </tr>
                    </tbody>
                </table>
            </div>
        `
    },

    ingredients: {
        title: "Ингредиенты",
        content: "<p>Здесь будет управление ингредиентами.</p>"
    },

    orders: {
        title: "Заказы",
        content: "<p>Здесь будет история заказов.</p>"
    }
};


async function loadDishes() {
    const dishesList = document.querySelector("#dishes-list");

    try {
        const response = await fetch("/api/admin/dishes");

        if (!response.ok) {
            throw new Error("Не удалось загрузить блюда");
        }

        const dishes = await response.json();

        if (dishes.length === 0) {
            dishesList.innerHTML = `
                <tr>
                    <td colspan="5">Блюд пока нет</td>
                </tr>
            `;
            return;
        }

        dishesList.innerHTML = dishes.map(dish => `
            <tr>
                <td>${dish.emoji} ${dish.name}</td>
                <td>${dish.weight_grams ? dish.weight_grams + " г" : "Не указан"}</td>
                <td>${dish.price}</td>
                <td>${dish.is_available ? "Доступно" : "Скрыто"}</td>
                <td>
                    <button class="action-button" data-action="edit" data-id="${dish.id}">
                        Редактировать
                    </button>
                    <button class="action-button" data-action="recipe" data-id="${dish.id}">
                        Рецепт
                    </button>
                </td>
            </tr>
        `).join("");

    } catch (error) {
        console.error(error);

        dishesList.innerHTML = `
            <tr>
                <td colspan="5">Ошибка загрузки блюд</td>
            </tr>
        `;
    }
}


navButtons.forEach(button => {
    button.addEventListener("click", () => {
        const page = button.dataset.page;
        const selectedPage = pages[page];

        if (!selectedPage) {
            return;
        }

        navButtons.forEach(item => {
            item.classList.remove("active");
        });

        button.classList.add("active");

        pageTitle.textContent = selectedPage.title;
        pageContent.innerHTML = selectedPage.content;
	    
	if (page === "dishes") {
    		loadDishes();
	}
    });
});

document.querySelector(".nav-button.active").click();
