import {computed, ref} from "vue";

const authToken = ref("");

export function useAuth() {
    return {
        authToken,
        isLoggedIn: computed(() => authToken.value !== ""),
        setToken,
        clearToken
    }
}

function setToken(newToken) {
    authToken.value = newToken;
}

function clearToken() {
    authToken.value = "";
}