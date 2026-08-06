<script setup lang="ts">
import {computed, defineAsyncComponent, onMounted, onUnmounted, ref, shallowRef, useTemplateRef, watch} from 'vue'
import ListItem from './ListItem.vue'
import {FontAwesomeIcon} from '@fortawesome/vue-fontawesome'
import {
  faChevronLeft,
  faChevronRight,
  faCircleExclamation,
  faFilter,
  faSearch,
  faTag,
  faXmark
} from '@fortawesome/free-solid-svg-icons'
import MapLoading from "./Map/MapLoading.vue";
import MapError from "./Map/MapError.vue";
import NetworkDialog from "./NetworkDialog.vue";
import {setParams, uiQuery} from "@/router.ts";
import {
  isAvailableCountriesResponse,
  isNetworkList,
  knownTags,
  type ListResponseNetwork,
} from "@/types/api/directory.ts";
import * as flagIconsRaw from 'country-flag-icons/string/3x2'

const flagIcons = flagIconsRaw as Record<string, string>

const AsyncServerMap = defineAsyncComponent({
  loader: () => import('./Map/MapView.vue'),
  loadingComponent: MapLoading,
  errorComponent: MapError,
  delay: 200,
  timeout: 5000
})

const items = shallowRef<ListResponseNetwork[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(5);
const loading = ref(true);
const error = ref<string | null>(null);

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)));
const rangeStart = computed(() => (total.value === 0 ? 0 : (page.value - 1) * pageSize.value + 1));
const rangeEnd = computed(() => Math.min(page.value * pageSize.value, total.value));

// Compact, windowed page list with ellipsis for the pager
const pageNumbers = computed(() => {
  const tp = totalPages.value;
  const cur = page.value;
  const out = [];
  const window = 2;
  const start = Math.max(1, cur - window);
  const end = Math.min(tp, cur + window);
  if (start > 1) {
    out.push(1);
    if (start > 2) out.push('...');
  }
  for (let p = start; p <= end; p++) out.push(p);
  if (end < tp) {
    if (end < tp - 1) out.push('...');
    out.push(tp);
  }
  return out;
})

const searchQuery = ref("");
const availableCountries = ref<string[]>([]);
const selectedCountry = ref<string>('');
const selectedCity = ref('');
const selectedTags = ref<string[]>([]);

// Guards against out-of-order responses: only the newest request may commit
let controller: AbortController | null = null;

const fetchList = async () => {
  if (controller) controller.abort();
  controller = new AbortController();

  try {
    loading.value = true;
    error.value = null;

    const params = new URLSearchParams({
      page: String(page.value),
      pageSize: String(pageSize.value),
    });
    if (searchQuery.value) params.set('q', searchQuery.value);
    if (selectedTags.value.length) params.set('tags', selectedTags.value.join(','));
    if (selectedCountry.value) params.set('country', selectedCountry.value);
    if (selectedCity.value) params.set('city', selectedCity.value.toUpperCase());

    const response = await fetch(`api/directory/list?${params.toString()}`, {
      signal: controller.signal
    });
    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const data: unknown = await response.json();

    if (!isNetworkList(data)) throw new Error("invalid data");

    items.value = data.items;
    total.value = data.total;

    // Clamp if we ended up past the last page (e.g. after server data changes between fetches).
    if (page.value > totalPages.value) {
      page.value = totalPages.value;
      await fetchList();
    }
  } catch (e) {
    if (!(e instanceof Error)) return;
    if (e.name === 'AbortError') return;
    error.value = `Failed to load data: ${e.message}`;
    console.error('Error fetching list:', e);
  } finally {
    if (controller?.signal.aborted === false) {
      loading.value = false;
    }
  }
}

const fetchCountries = async () => {
  try {
    const res = await fetch(`api/directory/countries`);
    if (res.ok) {
      const data: unknown = await res.json();
      if (!isAvailableCountriesResponse(data)) {
        throw new Error("invalid data");
      }
      availableCountries.value = data.countries;
    }
  } catch (e) {
    console.error('Error fetching countries:', e);
  }
}

const goToPage = (p: number) => {
  if (typeof p !== 'number') return;
  if (p < 1 || p > totalPages.value || p === page.value) return;
  page.value = p;
  fetchList();
  scrollToNetworkList();
}

const networkListAnchor = useTemplateRef("networkListAnchor");
const scrollToNetworkList = () => {
  networkListAnchor.value?.scrollIntoView({behavior: 'smooth', block: 'center'});
}

const toggleTag = (tag: string) => {
  const index = selectedTags.value.indexOf(tag);
  if (index === -1) {
    selectedTags.value.push(tag);
  } else {
    selectedTags.value.splice(index, 1);
  }
  page.value = 1;
  fetchList();
}

const clearAllFilters = () => {
  searchQuery.value = '';
  selectedTags.value = [];
  selectedCountry.value = '';
  selectedCity.value = '';
  page.value = 1;
  fetchList();
}

// Changing the country filter resets to page 1 and refetches.
const changeCountry = () => {
  selectedCity.value = '';
  page.value = 1;
  fetchList();
}

const changePageSize = () => {
  page.value = 1;
  fetchList();
}

const handleMapSelect = (loCode: string) => {
  if (!loCode || loCode.length < 2) return;
  selectedCountry.value = loCode.slice(0, 2).toUpperCase();
  if (loCode.length === 5) {
    selectedCity.value = loCode.slice(2).toUpperCase();
  } else {
    selectedCity.value = "";
  }
  page.value = 1;
  fetchList();
  scrollToNetworkList();
}

// Debounce search so we don't fire a request on every keystroke.
let debounceTimer: number | null = null;

function onSearchInput() {
  if (debounceTimer) clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => {
    page.value = 1;
    fetchList();
  }, 300);
}

onUnmounted(() => {
  if (debounceTimer) clearTimeout(debounceTimer);
})

// --- Network dialog ---------------------
const searchAsn = ref('');

watch(() => uiQuery.value.asn, (val) => {
  searchAsn.value = val ?? "";
}, {immediate: true})

const handleNetworkDialogClose = () => {
  searchAsn.value = '';
  setParams({"asn": ""});
  fetchList();
}
// -------------------------------------

onMounted(() => {
  // If an ASN was found in the URL, the NetworkDialog will open
  // and fetchList is deferred until the dialog closes.
  if (!searchAsn.value) {
    fetchList();
  }
});
fetchCountries();
</script>

<template>
  <div class="container my-4">

    <NetworkDialog :asn="searchAsn" @close="handleNetworkDialogClose" />

    <!-- UN/loCode server-density map -->
    <Suspense v-if="!error">
      <AsyncServerMap @select-locode="handleMapSelect"/>
      <template #fallback>
        <MapLoading></MapLoading>
      </template>
    </Suspense>

    <!-- Header Section -->
    <div class="row mb-4 align-items-end">
      <div class="col-md-6">
        <h2 class="fw-bold text-dark mb-1">Network List</h2>
        <p class="text-muted small mb-0">View dn42 networks already listed in the node directory</p>
      </div>

      <!-- Search & Actions -->
      <div class="col-md-6">
        <div class="d-flex gap-2 justify-content-md-end mt-3 mt-md-0">

          <!-- Search Input Group -->
          <div class="input-group shadow-sm">
            <span class="input-group-text bg-white border-end-0 rounded-start-3">
              <FontAwesomeIcon :icon="faSearch" class="text-muted"/>
            </span>
            <input
                type="text"
                class="form-control border-start-0 ps-0"
                placeholder="Search name, maintainer, description..."
                v-model="searchQuery"
                @input="onSearchInput"
            >
            <button
                v-if="searchQuery"
                @click="searchQuery = ''; onSearchInput()"
                class="btn btn-light border-start-0 px-2 text-muted"
            >
              <FontAwesomeIcon :icon="faXmark"/>
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Tag Filter Selection -->
    <div class="row mb-3" v-if="!error">
      <div class="col-12">
        <div class="d-flex align-items-center gap-2 flex-wrap px-1">
          <span class="badge bg-light text-dark border me-2 py-2">
            <FontAwesomeIcon :icon="faFilter" class="me-1"/> Filter:
          </span>

          <button
              v-for="tag in knownTags"
              :key="tag"
              @click="toggleTag(tag)"
              class="btn btn-sm rounded-pill px-3 transition-all"
              :class="selectedTags.includes(tag) ? 'btn-dark' : 'btn-outline-secondary'"
          >
            {{ tag }}
          </button>

          <!-- Country & City filters -->
          <div class="ms-auto d-flex align-items-center gap-2" v-if="availableCountries.length">
            <span class="small text-muted d-none d-md-inline">Country</span>
            <select
                v-model="selectedCountry"
                @change="changeCountry"
                class="form-select form-select-sm w-auto"
            >
              <option value="">All countries</option>
              <option
                  v-for="code in availableCountries"
                  :key="code"
                  :value="code"
              >
                {{ code.toUpperCase() }}
              </option>
            </select>

            <span class="small text-muted d-none d-md-inline ms-1"
                  :class="{ 'opacity-50': !selectedCountry }">City</span>
            <input
                type="text"
                class="form-control form-control-sm text-uppercase"
                :placeholder="selectedCountry ? 'e.g. FRA' : '—'"
                v-model="selectedCity"
                @input="onSearchInput"
                :disabled="!selectedCountry"
                :title="!selectedCountry ? 'Select a country first to filter by city' : ''"
                style="width: 80px;"
                maxlength="3"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- Active Filters / Status Bar -->
    <div class="d-flex justify-content-between align-items-center mb-3 px-1 flex-wrap gap-2" v-if="!error">
      <span class="badge rounded-pill bg-light text-dark border fw-medium">
        <template v-if="total > 0">{{ rangeStart }}–{{ rangeEnd }} of {{ total }} Entries</template>
        <template v-else>{{ total }} Entries</template>
      </span>

      <div class="d-flex align-items-center gap-2 flex-wrap">
        <!-- Individual tag pills shown when active -->
        <span v-for="tag in selectedTags" :key="tag" class="badge bg-primary-subtle text-primary border border-primary">
           <FontAwesomeIcon :icon="faTag" class="me-1"/>{{ tag }}
        </span>

        <!-- Active country pill -->
        <span
            v-if="selectedCountry"
            class="badge bg-success-subtle text-success border border-success d-inline-flex align-items-center gap-1"
        >
          <span v-html="flagIcons[selectedCountry] ?? ''" class="flag-svg"></span>
          {{ selectedCountry }}
        </span>

        <!-- Active city pill -->
        <span
            v-if="selectedCity"
            class="badge bg-info-subtle text-info border border-info d-inline-flex align-items-center gap-1"
        >
          City: {{ selectedCity.toUpperCase() }}
        </span>

        <!-- Clear button -->
        <span
            v-if="searchQuery || selectedTags.length || selectedCountry || selectedCity"
            class="text-danger small fw-semibold ms-2"
            @click="clearAllFilters"
            role="button"
        >
           Clear All Filters
         </span>

        <!-- Page size selector -->
        <div class="d-flex align-items-center gap-1 ms-2">
          <span class="small text-muted">Per page</span>
          <select v-model.number="pageSize" @change="changePageSize" class="form-select form-select-sm w-auto">
            <option :value="5">5</option>
            <option :value="10">10</option>
            <option :value="25">25</option>
            <option :value="50">50</option>
          </select>
        </div>
      </div>
    </div>

    <div ref="networkListAnchor"></div>

    <!-- While the ASN dialog is open, hide the list area -->
    <div v-if="searchAsn"></div>

    <!-- Loading State (Skeleton) -->
    <div v-else-if="loading" class="row g-3">
      <div class="col-12" v-for="n in 1" :key="n">
        <div class="card border-0 shadow-sm rounded-3 overflow-hidden placeholder-glow">
          <div class="card-body p-4">
            <div class="row align-items-center">
              <div class="col-lg-8">
                <span class="placeholder col-6 bg-secondary rounded mb-2 d-block"></span>
                <span class="placeholder col-10 bg-light rounded mb-3"></span>
                <span class="placeholder col-3 bg-primary rounded d-inline-block"></span>
              </div>
              <div class="col-lg-4">
                <span class="placeholder col-12 bg-dark rounded-3" style="height: 40px;"></span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Error State -->
    <div v-else-if="error" class="row justify-content-center mt-5">
      <div class="col-md-8">
        <div class="alert alert-danger d-flex align-items-center rounded-3 border-0 shadow-sm" role="alert">
          <FontAwesomeIcon :icon="faCircleExclamation" class="fa-2x me-3 text-danger"/>
          <div>
            <h5 class="alert-heading fw-bold mb-1">Connection Error</h5>
            <p class="mb-0 small">{{ error }}</p>
          </div>
        </div>
        <div class="text-center mt-3">
          <button class="btn btn-outline-danger" @click="fetchList">Retry Connection</button>
        </div>
      </div>
    </div>

    <!-- List Items -->
    <div v-else class="row g-3">
      <div
          v-for="item in items"
          :key="item.Mnt + '/' + item.Name"
          class="col-12 animate-up"
      >
        <ListItem :item="item" />
      </div>

      <!-- Empty State -->
      <div v-if="items.length === 0" class="col-12 text-center py-5 opacity-75">
        <div class="mb-3">
          <FontAwesomeIcon :icon="faSearch" class="fa-3x text-muted"/>
        </div>
        <h5 class="fw-bold">No results found</h5>
        <p class="text-muted">Try adjusting your search terms or tag filters.</p>
      </div>
    </div>

    <!-- Pagination -->
    <div v-if="!loading && !error && totalPages > 1" class="overflow-x-auto pb-3">
      <nav class="mt-4 d-flex justify-content-center" style="justify-content: safe center!important;">
        <ul class="pagination shadow-sm mb-0">
          <li class="page-item" :class="{ disabled: page <= 1 }">
            <a class="page-link" href="#" @click.prevent="goToPage(page - 1)">
              <FontAwesomeIcon :icon="faChevronLeft"/>
            </a>
          </li>

          <li
              v-for="(p, i) in pageNumbers"
              :key="i"
              class="page-item"
              :class="{ active: p === page, disabled: p === '...' }"
          >
            <span v-if="p === '...'" class="page-link">…</span>
            <a v-else class="page-link" href="#" @click.prevent="typeof p === 'number' && goToPage(p)">{{ p }}</a>
          </li>

          <li class="page-item" :class="{ disabled: page >= totalPages }">
            <a class="page-link" href="#" @click.prevent="goToPage(page + 1)">
              <FontAwesomeIcon :icon="faChevronRight"/>
            </a>
          </li>
        </ul>
      </nav>
    </div>
  </div>

</template>

<style scoped>
.animate-up {
  animation: slideUp 0.4s ease-out forwards;
  opacity: 0;
  transform: translateY(10px);
}

@keyframes slideUp {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>