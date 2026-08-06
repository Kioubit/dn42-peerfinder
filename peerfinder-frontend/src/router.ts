import {readonly, ref} from "vue";
import {isUIPage, type UIPage} from "@/types/main.ts";


const _page  = ref<UIPage>("main");
const _query = ref<Record<string, string>>({});

export const uiPage  = readonly(_page);
export const uiQuery = readonly(_query);
// ---------------------------------------------------------------------------------------------------------------------
function syncFromHash() {
    const [path, qs = ""] = window.location.hash.replace(/^#\/?/, "").split("?");
    _page.value  = path && isUIPage(path) ? path : "main";
    _query.value = Object.fromEntries(new URLSearchParams(qs));
}

function writeHash(page: UIPage, query: Record<string, string>, replace = true) {
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(query)) {
        if (v !== "" && v != null) params.set(k, String(v));
    }

    const qs   = params.toString();
    const hash = `#/${page}${qs ? `?${qs}` : ""}`;

    if (replace) {
        history.replaceState(null, "", hash);
        syncFromHash();              // replaceState fires no event -> sync manually
    } else {
        window.location.hash = hash; // fires hashchange -> syncFromHash
    }
}

/** Read a single query param (reactive via uiQuery). */
export const getParam = (name: string) => _query.value[name] ?? "";

/** Set/update params. Pass "" or null to delete one. Replaces history by default. */
export function setParams(updates = {}, { replace = true } = {}) {
    writeHash(_page.value, { ..._query.value, ...updates }, replace);
}

/** Navigate to a page (pushes a history entry). */
export function navigate(page: UIPage, params = {}) {
    writeHash(page, params, false);
}

export function initRouter() {
    syncFromHash();
    window.addEventListener("hashchange", syncFromHash);
}
