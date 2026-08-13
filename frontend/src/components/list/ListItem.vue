<script setup lang="ts">
import {ref, computed, onMounted} from 'vue'
import {FontAwesomeIcon} from '@fortawesome/vue-fontawesome'
import {
  faServer,
  faCopy,
  faCheck,
  faExternalLinkAlt,
  faLink
} from '@fortawesome/free-solid-svg-icons'
import {isServerList, type ListResponseNetwork, type Server} from "@/types/api/directory.ts";
import * as flagIconsRaw from 'country-flag-icons/string/3x2'
import {copyText} from "@/util.ts";
const flagIcons = flagIconsRaw as Record<string, string>

interface Props {
  item: ListResponseNetwork
  autoExpand?: boolean
}
const props = defineProps<Props>()

const showServers = ref(false);
const servers = ref<readonly Server[]>([]);
const serversLoading = ref(false)
const serversError = ref<string | null>(null)

// Whether the expanded server list (once loaded) has any entries.
const hasServers = computed(() => {
  return servers.value && servers.value.length > 0
})

// Total server count comes from the list payload (serverCount), so the badge
// and button label are correct without expanding the row.
const serverCount = computed(() => {
  return Number(props.item.serverCount) || 0
})

const toggleServers = () => {
  if (!showServers.value) {
    loadServers()
  }
  showServers.value = !showServers.value
}

const loadServers = async () => {
  if (hasServers.value || serversLoading.value || serversError.value) return
  serversLoading.value = true
  serversError.value = null
  try {
    const response = await fetch(`api/directory/network/${props.item.asn}/servers`)
    if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`)
    const data: unknown = await response.json()
    if (!isServerList(data)) {
      throw new Error("invalid data");
    }
    servers.value = data ?? [];
  } catch (e) {
    serversError.value = `Failed to load servers: ${e instanceof Error? e.message: String(e)}`
    console.error('Error fetching servers:', e)
  } finally {
    serversLoading.value = false
  }
}

// Direct, shareable link to this network
const networkLink = computed(() => {
  const base = window.location.origin;
  return `${base}/#?asn=${props.item.asn}`;
})

// When arriving via a direct ASN link, auto-expand the network's servers.
// The 'Servers' field is already populated.
onMounted(() => {
  if (props.autoExpand) {
    showServers.value = true
    if (props.item.Servers) {
      servers.value = props.item.Servers;
    }
  }
})
</script>

<template>
  <div class="card shadow-sm overflow-hidden hover-lift rounded-3 mb-2">
    <!-- Header Section -->
    <div class="card-body p-4">
      <div class="d-flex justify-content-between align-items-start flex-wrap gap-3">

        <!-- Left -->
        <div class="flex-grow-1">
          <div class="d-flex align-items-center gap-2 mb-2 flex-wrap">
            <h5 class="card-title fw-bold mb-0 text-dark">{{ item.Name }}</h5>
            <span class="badge bg-primary-subtle text-primary border border-primary rounded-pill px-2 py-1 fw-semibold">{{ item.Mnt }}</span>
          </div>

          <p class="text-muted mb-3 small">
            {{ item.Description }}
          </p>
          <p>
            <span v-for="(tag, index) in item.Tags"
                  :key="index"
                  class="badge me-1 bg-secondary bg-opacity-10 text-secondary border border-secondary-subtle rounded-pill px-2 py-1 fw-normal small"
            >{{tag}}</span>
          </p>

          <div class="d-flex gap-2 flex-wrap">
            <a
                v-if="item.URL"
                :href="item.URL"
                target="_blank"
                class="btn btn-sm btn-outline-secondary text-decoration-none d-inline-flex align-items-center gap-1"
            >
              <FontAwesomeIcon :icon="faExternalLinkAlt" />
              Website
            </a>

            <button
                class="btn btn-sm d-inline-flex align-items-center gap-1 btn-outline-secondary"
                @click="copyText(networkLink, $event, 'btn-success', 'btn-outline-secondary')"
                :title="networkLink"
            >
              <FontAwesomeIcon :icon="faLink" data-copy-toggle />
              <FontAwesomeIcon :icon="faCheck" data-copy-toggle hidden />
              <span data-copy-toggle>Copy Link</span>
              <span data-copy-toggle hidden>Copied</span>
            </button>

            <button
                v-if="(!serversError || hasServers) && !props.autoExpand"
                @click="toggleServers"
                class="btn btn-sm d-inline-flex align-items-center gap-2 fw-semibold transition-all"
                :class="showServers ? 'btn-dark' : 'btn-outline-dark'"
            >
              <FontAwesomeIcon :icon="faServer" />
              <span v-if="!showServers">View {{ serverCount }} Server{{ serverCount > 1 ? 's' : '' }}</span>
              <span v-else>Hide Servers</span>
            </button>
          </div>
        </div>

        <!-- Right -->
        <div class="d-flex align-items-center bg-light rounded-3 px-3 py-2 border">
          <div class="text-center">
            <small class="text-muted d-block text-uppercase" style="font-size: 0.65rem; letter-spacing: 1px;">Servers</small>
            <span class="h5 fw-bold mb-0 text-primary">{{ serverCount }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Server List Section (Expands on click) -->
    <transition name="fade">
      <div v-if="showServers" class="bg-light border-top">
        <div class="p-3 bg-white">
          <div class="d-flex align-items-center mb-3">
            <FontAwesomeIcon :icon="faServer" class="text-muted me-2" />
            <span class="fw-bold text-secondary">Server List</span>
          </div>

          <div v-if="serversLoading" class="text-center text-muted py-3">
            <span class="spinner-border spinner-border-sm me-2"></span>Loading servers…
          </div>
          <div v-else-if="serversError" class="alert alert-danger mb-0">{{ serversError }}</div>
          <div v-else-if="hasServers" class="table-responsive">
            <table class="table table-hover align-middle mb-0">
              <thead class="table-light">
              <tr>
                <th scope="col" class="border-0">ID</th>
                <th scope="col" class="border-0">Address</th>
                <th scope="col" class="border-0">Country</th>
                <th scope="col" class="border-0">City</th>
                <th scope="col" class="border-0">Tags</th>
                <th scope="col" class="border-0 text-end">Action</th>
              </tr>
              </thead>
              <tbody>
              <tr
                  v-for="server in servers"
                  :key="server.ID"
              >
                <td class="fw-semibold">{{ server.ID }}</td>
                <td>
                  <code v-if="server.Address" class="text-muted bg-light px-2 py-1 rounded">{{ server.Address }}</code>
                  <span v-else class="text-muted">—</span>
                </td>
                <td>
              <span
                  v-if="server.CountryCode"
                  class="badge bg-secondary bg-opacity-10 text-dark"
              >
                <span v-html="flagIcons[server.CountryCode] ?? ''" class="flag-svg"></span>
                 {{ server.CountryCode }}
              </span>
                  <span v-else class="text-muted">—</span>
                </td>
                <td>
              <span
                  v-if="server.City"
                  class="badge bg-secondary bg-opacity-10 text-dark"
              >
                {{ server.City }}
              </span>
                  <span v-else class="text-muted">—</span>
                </td>
                <td>
                  <span v-if="server.Tags && server.Tags.length > 0"
                        v-for="tag in server.Tags"
                        :key="tag"
                        class="badge bg-secondary bg-opacity-10 text-secondary border border-secondary-subtle rounded-pill fw-normal small me-1 text-truncate d-inline-block align-bottom"
                        style="max-width: 120px;"
                  >{{ tag }}</span>
                  <span v-else class="text-muted">—</span>
                </td>
                <td class="text-end">
                  <button
                      v-if="server.Address"
                      class="btn btn-sm d-inline-flex align-items-center gap-1 btn-outline-secondary"
                      @click="copyText(server.Address, $event, 'btn-success', 'btn-outline-secondary')"
                  >
                    <FontAwesomeIcon :icon="faCopy" data-copy-toggle />
                    <FontAwesomeIcon :icon="faCheck" data-copy-toggle hidden />
                    <span data-copy-toggle>Copy</span>
                    <span data-copy-toggle hidden>Copied</span>
                  </button>
                </td>
              </tr>
              </tbody>
            </table>
          </div>
          <div v-else class="text-center text-muted py-3">No servers listed.</div>
        </div>
      </div>
    </transition>
  </div>
</template>

<style scoped>
.hover-lift {
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.hover-lift:hover {
  transform: translateY(-3px);
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.1) !important;
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.3s ease, transform 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}
</style>