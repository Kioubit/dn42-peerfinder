package kauth

import (
	"net/http"
	"net/url"
)

func WithAuth(h func(w http.ResponseWriter, r *http.Request, session *AuthenticationInfo)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("kauth-token")
		if token == "" {
			token = r.URL.Query().Get("kauth_token")
		}

		tokenMap, err := url.ParseQuery(token)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusBadRequest)
			return
		}

		signature := tokenMap.Get("signature")
		params := tokenMap.Get("params")
		userData, err := VerifyAuthToken(signature, params, 3600)
		if err != nil {
			http.Error(w, "Token verification failed", http.StatusUnauthorized)
			return
		}
		h(w, r, &userData)
	})
}
