package main

import (
	"crypto/subtle"
	"net/http"
	"os"
)

func adminAuth(next http.HandlerFunc) http.HandlerFunc {
	user := os.Getenv("ADMIN_USER")
	if user == "" {
		user = "admin"
	}

	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		// Локальная разработка: админка открыта. В продакшене ADMIN_PASSWORD
		// обязателен, иначе ссылки на заказы сможет выгрузить кто угодно.
		return next
	}

	return func(w http.ResponseWriter, r *http.Request) {
		requestUser, requestPassword, ok := r.BasicAuth()

		userMatches := subtle.ConstantTimeCompare([]byte(requestUser), []byte(user)) == 1
		passwordMatches := subtle.ConstantTimeCompare([]byte(requestPassword), []byte(password)) == 1

		if !ok || !userMatches || !passwordMatches {
			w.Header().Set("WWW-Authenticate", `Basic realm="Food Service Admin", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}
