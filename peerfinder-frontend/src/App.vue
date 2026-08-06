<script setup lang="ts">
import {faCompass, faList, faLocationCrosshairs, faPencil} from "@fortawesome/free-solid-svg-icons";
import {defineAsyncComponent, useTemplateRef} from "vue";
import {FontAwesomeIcon} from "@fortawesome/vue-fontawesome";
import ErrorComponent from "./components/ErrorComponent.vue";
import LoadingComponent from "./components/LoadingComponent.vue";
import {initRouter, uiPage} from "./router.js";
import {useAuth} from "./composables/useAuth.js";
import type {UIPage} from "@/types/main.ts";

const listViewAsync = defineAsyncComponent({
  loader: () => import('./components/list/ListView.vue'),
  loadingComponent: LoadingComponent,
  errorComponent: ErrorComponent,
  delay: 200,
  timeout: 8000
});

const discoverAsync = defineAsyncComponent({
  loader: () => import('./components/discover/discover.vue'),
  loadingComponent: LoadingComponent,
  errorComponent: ErrorComponent,
  delay: 200,
  timeout: 8000
});

const editAsync = defineAsyncComponent({
  loader: () => import('./components/edit/edit.vue'),
  loadingComponent: LoadingComponent,
  errorComponent: ErrorComponent,
  delay: 200,
  timeout: 8000
});


const pageComponents: Record<UIPage, unknown> = {
  "main": listViewAsync,
  "edit": editAsync,
  "discover": discoverAsync,
};

const {setToken, clearToken} = useAuth();
const authDialog = useTemplateRef("authDialog");

function handleAuthProgress(ev: CustomEvent<{ isLoading: boolean }>) {
  if (ev.detail?.isLoading) {
    authDialog.value?.showModal();
  } else {
    authDialog.value?.close();
  }
}

initRouter();
</script>

<template>
  <dialog ref="authDialog" class="auth-dialog border-0 shadow rounded-4 p-4" closedby="none">
    <div class="d-flex flex-column align-items-center gap-3 text-center" style="min-width: 240px;">
      <div class="d-flex flex-column gap-1">
        <span class="fw-semibold fs-6 text-body">Authenticating…</span>
        <small class="text-muted">Please check the opened window</small>
      </div>
    </div>
  </dialog>

  <header
      class="d-flex justify-content-center justify-content-sm-between align-items-center flex-wrap flex-sm-nowrap mb-4 mx-3 mt-3 gap-3">
    <h1 class="d-flex align-items-center m-0 fs-3 fw-bold">
      <FontAwesomeIcon :icon="faLocationCrosshairs" class="text-primary me-2"/>
      DN42 Peer finder
    </h1>

    <div>
      <kioubit-auth-btn-window
          return="/auth-success.html"
          validity="60"
          localStoragePrefix="login"
          @loadingchange="handleAuthProgress"
          @authsuccess="setToken($event.detail.token)"
          @authreset="clearToken()"
      />
    </div>
  </header>

  <div class="mx-2">
    <ul class="nav nav-tabs nav-fill mt-3 mb-4">
      <li class="nav-item">
        <a class="nav-link" :class="{ active: uiPage === 'main' }" href="#/main" role="button">
          <FontAwesomeIcon :icon="faList"></FontAwesomeIcon>
          Network list
        </a>
      </li>
      <li class="nav-item">
        <a class="nav-link" :class="{ active: uiPage === 'discover' }" href="#/discover" role="button">
          <FontAwesomeIcon :icon="faCompass"></FontAwesomeIcon>
          Discover peers
        </a>
      </li>
      <li class="nav-item">
        <a class="nav-link" :class="{ active: uiPage === 'edit' }" href="#/edit" role="button">
          <FontAwesomeIcon :icon="faPencil"></FontAwesomeIcon>
          Add your network
        </a>
      </li>
    </ul>

    <keep-alive>
      <component :is="pageComponents[uiPage]"/>
    </keep-alive>
  </div>
</template>

<style scoped>
.auth-dialog::backdrop {
  background: rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(2px);
}
</style>