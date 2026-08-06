<script setup lang="ts">
import {ref, computed, onMounted, watch} from 'vue'
import { FontAwesomeIcon } from '@fortawesome/vue-fontawesome'
import {
  faSave,
  faTrash,
  faPlus,
  faServer,
  faGlobe,
  faUser,
  faLink,
  faSpinner,
  faCheck,
  faExclamationTriangle,
  faTags, faEdit, faArrowUp, faArrowDown, faGripLines, faUserPlus, faInfoCircle,
  faTrashAlt, faTimes
} from '@fortawesome/free-solid-svg-icons'
import {useAuth} from "@/composables/useAuth";
import {knownTags, isNetwork, type Network, type Server, type Tag} from "@/types/api/directory.ts";
import * as flagIconsRaw from 'country-flag-icons/string/3x2'
const flagIcons = flagIconsRaw as Record<string, string>

type EditableServer = Omit<Server, 'Tags'> & { Tags: Tag[]; _expanded: boolean };
type EditableForm   = Omit<Network, 'Tags' | 'Servers'> & { Tags: Tag[]; Servers: EditableServer[] };

const {authToken, isLoggedIn} = useAuth();

function addAuthHeader(options: RequestInit = {}): RequestInit {
  const headers = new Headers(options.headers || {});
  headers.set('kauth-token', authToken.value);

  return {
    ...options,
    headers
  }
}

// State
const loading = ref(false);
const error = ref<string|null>(null);
const success = ref(false);
const form = ref<EditableForm>(blankForm());
const deleteConfirm = ref(false);

function blankForm(): EditableForm {
  return { Name: '', Mnt: '', URL: '', Description: '', Tags: [], Servers: [] }
}

function blankServer() : EditableServer {
  return { ID: '', Address: '', CountryCode: '', City: '', Tags: [], _expanded: true }
}

// --- Tags ---

function toggleTag(tags: Tag[] | null, tag: Tag) {
  if (tags === null) return;
  const i = tags.indexOf(tag);
  i > -1 ? tags.splice(i, 1) : tags.push(tag);
}

const fetchCurrentData = async () => {
  if (!isLoggedIn.value) {
    error.value = "Not authenticated";
    return;
  }
  loading.value = true;
  error.value = null;
  try {
    const response = await fetch(`api/directory/self`, addAuthHeader());
    if (!response.ok) throw new Error(`Failed to load: ${response.status}`);
    const data: unknown = await response.json();
    if (!isNetwork(data)) {
      throw new Error("Invalid network data received from server");
    }

    form.value = {
      ...blankForm(),
      ...data,
      Tags: [...(data.Tags ?? [])],
      Servers: (data.Servers ?? []).map(s => ({
        ...blankServer(), ...s, Tags: [...(s.Tags ?? [])], _expanded: false
      }))
    };
    snapshot.value = JSON.stringify(buildPayload());
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false;
  }
}

// Initialize on mount
onMounted(() => {
  fetchCurrentData();
})

// Fetch when user authenticates
watch(() => isLoggedIn.value, (val)=> {
  if (val) fetchCurrentData();
})

// --- Network Deletion ---

// Two-click confirmation: first click arms the button, second click confirms.
function requestDelete() {
  if (!deleteConfirm.value) {
    deleteConfirm.value = true;
    return;
  }
  confirmDelete();
}

async function confirmDelete() {
  deleteConfirm.value = false;
  loading.value = true;
  error.value = null;
  success.value = false;

  try {
    const res = await fetch(`api/directory/self`, addAuthHeader({
      method: 'DELETE',
    }));
    const text = await res.text();
    if (!res.ok) throw new Error(`Server error: ${res.status} - ${text}`)
    await fetchCurrentData();
    success.value = true;
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false
  }
}

// Cancel an armed delete confirmation
function cancelDelete() {
  deleteConfirm.value = false;
}

// --- Server Management ---

function addServer() {
  form.value.Servers.push(blankServer());
}

function removeServer(i: number) {
  form.value.Servers.splice(i, 1);
}

function moveServer(i: number, dir: number) {
  const j = i + dir;
  if (j < 0 || j >= form.value.Servers.length) {
    return;
  }
  [form.value.Servers[i], form.value.Servers[j]!] = [form.value.Servers[j]!, form.value.Servers[i]!];
}

const anyExpanded = computed(() =>
    form.value.Servers.length > 0 && form.value.Servers.some(s => s._expanded)
)
function toggleEdit(s: EditableServer){
  s._expanded = !s._expanded;
}
function toggleEditAll() {
  const anyExpanded_start = anyExpanded.value;
  form.value.Servers.forEach(s => s._expanded = !anyExpanded_start);
}

function buildPayload(): Network {
  return {
    Name:        form.value.Name,
    Mnt:         form.value.Mnt,
    URL:         form.value.URL,
    Description: form.value.Description,
    Tags:        form.value.Tags,
    Servers:     form.value.Servers.map(({ _expanded, ...s }: EditableServer) => s)
  };
}

// --- Submit ---
async function saveChanges() {
  loading.value = true;
  error.value = null;
  success.value = false;

  try {
    const payload = buildPayload();
    const res = await fetch(`api/directory/self`, addAuthHeader({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    }));
    const text = await res.text();
    if (!res.ok) throw new Error(`Server error: ${res.status} - ${text}`)

    snapshot.value = JSON.stringify(buildPayload())
    success.value = true
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  } finally {
    loading.value = false
  }
}

const snapshot = ref<string | null>(null);
const hasChanged = computed(() =>
    snapshot.value !== null &&
    JSON.stringify(buildPayload()) !== snapshot.value
);

// -- Drag and drop (servers) --
const dragIndex = ref<number | null>(null);

function onDragStart(i: number) {
  dragIndex.value = i;
}
function onDragOver(e: Event) {
  e.preventDefault();
}
function onDrop(i: number) {
  if (dragIndex.value === null || dragIndex.value === i) return;
  const moved = form.value.Servers.splice(dragIndex.value, 1);
  if (moved[0]) {
    form.value.Servers.splice(i, 0, moved[0]);
  }
  dragIndex.value = null;
}
function onDragEnd() {
  dragIndex.value = null;
}
const canDrag = ref(false);

// -- Tag drag and drop --
const tagDrag = ref<{ tags: Tag[]; i: number } | null>(null);

function onTagDragStart(tags: Tag[], i: number) {
  tagDrag.value = { tags, i };
}
function onTagDrop(tags: Tag[], i: number) {
  if (!tagDrag.value || tagDrag.value.tags !== tags) return;
  if (tagDrag.value.i === i) return;
  const moved = tags.splice(tagDrag.value.i, 1);
  if (moved[0]) {
    tags.splice(i, 0, moved[0]);
  }
  tagDrag.value = null;
}
function onTagDragEnd() {
  tagDrag.value = null;
}

// style issues checks
const noNetworkTagsButServerTags = computed(() =>
    (form.value.Tags == null || form.value.Tags.length === 0) &&
    form.value.Servers.some(s =>
        s.Tags != null && s.Tags.length > 0
    )
);

const allServersMissingLocode = computed(() => {
  const servers = form.value.Servers;
  if (servers.length === 0) return false;
  const hasFilledServer = servers.some(s => (s.ID && s.ID.trim()) && (s.CountryCode && s.CountryCode.trim()));
  const allMissingLocode = servers.every(s => !s.City || s.City.trim() === '');
  return hasFilledServer && allMissingLocode;
})
</script>

<template>
  <template v-if="isLoggedIn">
    <div class="card border-0 overflow-hidden mx-auto" style="max-width: 110rem;">
      <div class="card-header bg-white border-0 pt-1 px-4 pb-0">
        <div class="d-flex align-items-center mb-2">
          <div class="bg-primary text-white rounded-circle d-flex align-items-center justify-content-center me-3" style="width: 40px; height: 40px;">
            <FontAwesomeIcon :icon="faGlobe" />
          </div>
          <h4 class="mb-0 fw-bold">My Network</h4>
        </div>
      </div>

      <div class="card-body px-4">

        <!-- Loading State -->
        <div v-if="loading && !form.Mnt && !error" class="text-center py-5">
          <div class="spinner-border text-primary mb-3" role="status"></div>
          <p class="text-muted">Loading your network data...</p>
        </div>

        <!-- Error State -->
        <div v-else-if="error && !form.Mnt" class="alert alert-danger text-center py-5">
          <p>{{ error }}</p>
          <button class="btn btn-sm btn-outline-danger mt-2" @click="fetchCurrentData">Retry</button>
        </div>

        <!-- Form -->
        <form v-else @submit.prevent="saveChanges">
          <div class="row g-3 mb-4">
            <div class="col-md-6">
              <label class="form-label fw-semibold">Network Name <span class="text-danger">*</span></label>
              <div class="input-group">
                <span class="input-group-text bg-light"><FontAwesomeIcon :icon="faGlobe" /></span>
                <input type="text" class="form-control" v-model="form.Name" required :disabled="loading" />
              </div>
            </div>

            <div class="col-md-6">
              <label class="form-label fw-semibold">Maintainer <span class="text-danger">*</span></label>
              <div class="input-group">
                <span class="input-group-text bg-light"><FontAwesomeIcon :icon="faUser" /></span>
                <input type="text" class="form-control" v-model="form.Mnt" required disabled />
              </div>
            </div>

            <div class="col-12">
              <label class="form-label fw-semibold">Website URL</label>
              <div class="input-group">
                <span class="input-group-text bg-light"><FontAwesomeIcon :icon="faLink" /></span>
                <input type="url" class="form-control" v-model="form.URL" :disabled="loading" />
              </div>
            </div>

            <div class="col-12">
              <label class="form-label fw-semibold">Description</label>
              <textarea class="form-control" v-model="form.Description" rows="2" :disabled="loading"></textarea>
            </div>

            <!-- Network Tags Section -->
            <div class="col-12">
              <label class="form-label fw-semibold d-flex align-items-center gap-2">
                <FontAwesomeIcon :icon="faTags" class="text-primary" /> Network Tags
              </label>

              <!-- Selected tags — drag to reorder -->
              <div v-if="form.Tags.length > 0" class="d-flex flex-wrap gap-1 mb-2">
                <div v-for="(tag, i) in form.Tags" :key="tag"
                     class="badge bg-primary rounded-pill d-inline-flex align-items-center gap-1 py-2 px-3"
                     draggable="true"
                     @dragstart="onTagDragStart(form.Tags, i)"
                     @dragover.prevent
                     @drop="onTagDrop(form.Tags, i)"
                     @dragend="onTagDragEnd"
                     :class="{ 'opacity-50': tagDrag && tagDrag.tags === form.Tags && tagDrag.i === i }"
                     style="cursor: grab;">
                  <FontAwesomeIcon :icon="faGripLines" class="small" />
                  {{ tag }}
                </div>
              </div>

              <!-- Available tags — click to toggle -->
              <div class="d-flex flex-wrap gap-1 border rounded-3 p-2 bg-light">
                <button
                    v-for="tag in knownTags"
                    :key="tag"
                    type="button"
                    class="btn btn-sm rounded-pill"
                    :class="form.Tags.includes(tag) ? 'btn-primary' : 'btn-outline-secondary'"
                    @click="toggleTag(form.Tags, tag)"
                    :disabled="loading"
                >
                  {{ tag }}
                </button>
              </div>
              <small v-if="form.Tags.length > 0" class="text-muted d-block mt-1">
                <FontAwesomeIcon :icon="faInfoCircle" class="me-1" />Drag selected tags above to set their order.
              </small>
            </div>
            <div v-if="noNetworkTagsButServerTags" class="col-12">
              <div class="alert alert-danger d-flex align-items-center rounded-3 border-0 mb-0">
                <FontAwesomeIcon :icon="faExclamationTriangle" class="me-2" />
                <span>You have selected tags on servers but no <b>network tags</b>. Consider adding relevant network tags so your network can be filtered and discovered.</span>
              </div>
            </div>
          </div>

          <!-- Servers Section -->
          <div class="border-top pt-4 mb-4 mx-auto">
            <div class="d-flex justify-content-between align-items-center mb-2">
              <h5 class="fw-bold mb-0">
                <FontAwesomeIcon :icon="faServer" class="text-primary me-2" />Servers
                <span class="badge bg-primary rounded-pill ms-1">{{ form.Servers.length }}</span>
              </h5>
              <div class="d-flex gap-2">
                <button v-if="form.Servers.length > 0" type="button" class="btn btn-sm btn-outline-secondary rounded-pill" @click="toggleEditAll">
                  <FontAwesomeIcon :icon="faEdit" /> {{ anyExpanded ? 'Collapse All' : 'Edit All' }}
                </button>
                <button type="button" class="btn btn-sm btn-primary rounded-pill d-flex align-items-center gap-1" @click="addServer" :disabled="loading">
                  <FontAwesomeIcon :icon="faPlus" /> Add
                </button>
              </div>
            </div>

            <p class="text-muted small mb-3">
              <FontAwesomeIcon :icon="faInfoCircle" class="me-1" />
              Server tags are <b>not used for network filtering</b>. For properties you would like to be searchable, use <b>network tags</b>.
            </p>

            <div v-if="allServersMissingLocode" class="alert alert-warning d-flex align-items-center rounded-3 border-0 mb-3">
              <FontAwesomeIcon :icon="faExclamationTriangle" class="me-2" />
              <span>None of your servers have a <b>City (UN/LOCODE)</b> set. Adding a LOCODE is recommended.</span>
            </div>

            <div v-if="form.Servers.length === 0" class="text-center py-4 rounded-3 border border-1 text-muted">
              <FontAwesomeIcon :icon="faServer" size="2x" class="mb-2 opacity-50" />
              <p class="mb-0 small">No servers added</p>
            </div>

            <div v-else class="list-group list-group-flush overflow-x-auto mx-auto" style="max-width: 100rem;">
              <div v-for="(server, index) in form.Servers" :key="index"
                   class="list-group-item px-0 py-2"
                   :class="{ 'opacity-50': dragIndex === index }"
                   :draggable="canDrag"
                   @dragstart="onDragStart(index)"
                   @dragover.prevent="onDragOver($event)"
                   @drop="onDrop(index)"
                   @dragend="canDrag = false; onDragEnd()"
              >
                <!-- Compact summary row -->
                <div class="d-flex align-items-center gap-2">
                  <div class="text-muted px-2 drag-handle"
                       style="cursor: grab;"
                       @mousedown="canDrag = true"
                       @mouseup="canDrag = false">
                    <FontAwesomeIcon :icon="faGripLines" />
                  </div>

                  <div class="flex-grow-1 d-flex align-items-center gap-2 flex-wrap">
                    <span class="fw-bold">{{ server.ID || '—' }}</span>
                    <span class="text-muted small">{{ server.Address || 'No address' }}</span>
                    <span v-if="server.CountryCode" class="badge text-bg-light">
                      <span v-html="flagIcons[server.CountryCode] ?? ''" class="flag-svg"></span>
                      {{ server.CountryCode }}
                    </span>
                    <span v-if="server.City" class="text-muted small">{{ server.City }}</span>
                    <span v-for="tag in server.Tags?.slice(0, 5)" :key="tag" class="badge text-bg-dark">{{ tag }}</span>
                    <span v-if="server.Tags && server.Tags.length > 5" class="badge text-bg-secondary">+{{ server.Tags.length - 5 }}</span>
                  </div>
                  <button type="button" class="btn btn-sm btn-outline-secondary py-0"
                          @click="moveServer(index, -1)" :disabled="index === 0">
                    <FontAwesomeIcon :icon="faArrowUp" />
                  </button>
                  <button type="button" class="btn btn-sm btn-outline-secondary py-0"
                          @click="moveServer(index, 1)" :disabled="index === form.Servers.length - 1">
                    <FontAwesomeIcon :icon="faArrowDown" />
                  </button>
                  <button type="button" class="btn btn-sm btn-outline-secondary py-0" @click="toggleEdit(server)">
                    <FontAwesomeIcon :icon="faEdit" /> {{ server._expanded ? 'Close' : 'Edit' }}
                  </button>
                  <button type="button" class="btn btn-sm btn-outline-danger py-0" @click="removeServer(index)">
                    <FontAwesomeIcon :icon="faTrash" />
                  </button>
                </div>

                <!-- Inline edit panel -->
                <div v-if="server._expanded" class="row g-2 mt-2 mx-0">
                  <div class="col-md-2">
                    <label class="form-label small text-muted fw-semibold mb-1">ID <span class="text-danger">*</span></label>
                    <input type="text" class="form-control form-control-sm" v-model="server.ID" placeholder="ID" required>
                  </div>
                  <div class="col-md-5">
                    <label class="form-label small text-muted fw-semibold mb-1">Endpoint Address (hostname or IP)</label>
                    <input type="text" class="form-control form-control-sm" v-model="server.Address" placeholder="example.org">
                  </div>
                  <div class="col-md-2">
                    <label class="form-label small text-muted fw-semibold mb-1">Country <span class="text-danger">*</span></label>
                    <input type="text" class="form-control form-control-sm" v-model="server.CountryCode" placeholder="GB" maxlength="2">
                  </div>
                  <div class="col-md-3">
                    <label class="form-label small text-muted fw-semibold mb-1">City (UN/LOCODE)</label>
                    <input type="text" class="form-control form-control-sm" v-model="server.City" placeholder="e.g. LON" maxlength="3">
                    <small class="text-muted d-block mt-1">
                      <FontAwesomeIcon :icon="faInfoCircle" class="me-1" />
                      Look up codes at
                      <a href="https://unece.org/trade/cefact/unlocode-code-list-country-and-territory" target="_blank">
                        unece.org
                      </a>
                    </small>
                  </div>
                  <div class="col-12">
                    <!-- Selected tags — drag to reorder -->
                    <div v-if="server.Tags && server.Tags.length > 0" class="d-flex flex-wrap gap-1 mb-1">
                      <div v-for="(tag, i) in server.Tags" :key="tag"
                           class="badge text-bg-dark rounded-pill d-inline-flex align-items-center gap-1 py-2 px-3"
                           draggable="true"
                           @dragstart="onTagDragStart(server.Tags, i)"
                           @dragover.prevent
                           @drop="onTagDrop(server.Tags, i)"
                           @dragend="onTagDragEnd"
                           :class="{ 'opacity-50': tagDrag && tagDrag.tags === server.Tags && tagDrag.i === i }"
                           style="cursor: grab;">
                        <FontAwesomeIcon :icon="faGripLines" class="small" />
                        {{ tag }}
                      </div>
                    </div>
                    <!-- Available tags — click to toggle -->
                    <div class="d-flex flex-wrap gap-1 mt-1">
                      <button
                          v-for="tag in knownTags"
                          :key="tag"
                          type="button"
                          class="btn btn-sm rounded-pill"
                          :class="server.Tags && server.Tags.includes(tag) ? 'btn-dark' : 'btn-outline-secondary'"
                          @click="toggleTag(server.Tags, tag)"
                          :disabled="loading"
                      >{{ tag }}</button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Status Alerts -->
          <div v-if="error" class="alert alert-danger d-flex align-items-center rounded-3 border-0 mb-3">
            <FontAwesomeIcon :icon="faExclamationTriangle" class="me-2" />
            <span>{{ error }}</span>
          </div>

          <div v-if="success" class="alert alert-success d-flex align-items-center rounded-3 border-0 mb-3">
            <FontAwesomeIcon :icon="faCheck" class="me-2" />
            <span>Saved successfully! The updates may take a while to appear in the network list due to caching.</span>
            <button
                type="button"
                class="btn-close ms-2"
                aria-label="Close"
                @click="success = false"
            ></button>
          </div>

          <!-- Actions -->
          <div class="d-flex justify-content-between align-items-center pt-2 border-top gap-2 flex-wrap">
            <!-- Delete Network -->
            <div class="d-flex align-items-center gap-2">
              <button
                  type="button"
                  class="btn d-flex align-items-center gap-2"
                  :class="deleteConfirm ? 'btn-danger' : 'btn-outline-danger'"
                  :disabled="loading"
                  @click="requestDelete"
              >
                <FontAwesomeIcon v-if="loading" :icon="faSpinner" class="fa-spin" />
                <FontAwesomeIcon v-else-if="deleteConfirm" :icon="faExclamationTriangle" />
                <FontAwesomeIcon v-else :icon="faTrashAlt" />
                <span v-if="deleteConfirm">Click again to confirm deletion</span>
                <span v-else>Delete Network</span>
              </button>
              <button
                  v-if="deleteConfirm"
                  type="button"
                  class="btn btn-outline-secondary"
                  @click="cancelDelete"
              >
                <FontAwesomeIcon :icon="faTimes" /> Cancel
              </button>
            </div>

            <!-- Save -->
            <button type="submit" class="btn btn-primary px-4 fw-semibold d-flex align-items-center gap-2" :disabled="loading || !hasChanged">
              <FontAwesomeIcon v-if="loading" :icon="faSpinner" class="fa-spin" />
              <FontAwesomeIcon v-else :icon="faSave" />
              {{ loading ? 'Saving...' : 'Save Changes' }}
            </button>
          </div>

        </form>
      </div>
    </div>
  </template>
  <template v-else>
    <div class="text-center">
      <FontAwesomeIcon :icon="faUserPlus" size="3x" class="text-warning mb-3" />
      <h3 class="fw-semibold mb-0">Access Restricted</h3>
      <p class="text-secondary mb-0">Authentication is required to add or edit your network information</p>
    </div>
  </template>
</template>

<style scoped>
@media (pointer: coarse) {
  .drag-handle {
    display: none !important;
  }
}
</style>