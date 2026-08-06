import {computed, ref} from "vue";

const authToken = ref<string>("");

export function useAuth() {
    return {
        authToken,
        isLoggedIn: computed(() => authToken.value !== ""),
        setToken,
        clearToken
    }
}

function setToken(newToken: string) {
    authToken.value = newToken;
}

function clearToken() {
    authToken.value = "";
}
