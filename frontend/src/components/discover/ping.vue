<script setup lang="ts">
import {computed, onUnmounted, ref, shallowRef} from "vue";
import {
  faBan, faClock,
  faExclamationTriangle, faFlagCheckered, faLink, faNetworkWired,
  faPlay,
  faSatelliteDish,
  faSpinner,
  faUserPlus
} from "@fortawesome/free-solid-svg-icons";
import {FontAwesomeIcon} from "@fortawesome/vue-fontawesome";
import NetworkDialog from "../list/NetworkDialog.vue";
import {useAuth} from "@/composables/useAuth";
import {
  isPingErrorResponse, isPingMeta,
  isPingResult,
  isPingStartResponse,
  type PingMeta, type PingResult
} from "@/types/api/ping.ts";


type PingResultWithMetadata = PingResult & PingResultMetadata;

type PingResultMetadata = {
  metric: number;
}

const {authToken, isLoggedIn} = useAuth();

// --- Ping measurement (single authenticated SSE via EventSource) ---
const pingIp = ref('')
const pingLoading = ref(false)
const pingActive = ref(false)
const pingError = ref<string|null>(null)
const pingStatus = ref('')
const pingMetadata = ref<PingMeta>({})
const pingProgress = ref(0)
const pingReceived = ref(0)
const pingTotal = ref(0)

let pingSource: EventSource|null = null;
const pingResults = shallowRef<PingResultWithMetadata[]>([]);

let resultBuffer: PingResultWithMetadata[] = [];
let flushTimer: number|null = null;

function addResult(r: PingResult) {
  const metric = (r.reachable && r.latency != null) ? (r.latency + (r.jitter || 0)) : Infinity;
  resultBuffer.push({...r, metric})
  if (!flushTimer) {
    flushTimer = setTimeout(flushResults, 500)  // flush every 500ms
  }
}

function flushResults() {
  flushTimer = null
  if (resultBuffer.length) {
    pingResults.value = [...pingResults.value, ...resultBuffer]
    resultBuffer = []
  }
}

function clearFlushTimer() {
  if (flushTimer) {
    clearTimeout(flushTimer);
    flushTimer = null;
  }
  flushResults() // flush any remaining buffer immediately
}


// Group ping results by ASN (metadata output once per group), ordering groups
// by the lowest latency+jitter responding node
const groupedResults = computed(() => {
  const groupsMap = new Map();
  const metadata = pingMetadata.value;
  const results = pingResults.value;

  for (const r of results) {
    const asnKey = r.asn || 'unassigned';
    let g = groupsMap.get(asnKey);
    if (!g) {
      const meta = metadata[asnKey];
      g = {
        network: meta?.network || null,
        asn: r.asn || null,
        description: meta?.description || null,
        servers: meta?.servers || null,
        groupKey: asnKey,
        results: [],
        minMetric: Infinity
      };
      groupsMap.set(asnKey,g);
    }

    g.results.push(r);

    if (r.metric < g.minMetric) {
      g.minMetric = r.metric;
    }
  }

  const groupsArray = Array.from(groupsMap.values());
  for (let i = 0; i < groupsArray.length; i++) {
    groupsArray[i].results.sort((a: PingResultWithMetadata, b: PingResultWithMetadata) => a.metric - b.metric);
  }

  groupsArray.sort((a, b) => a.minMetric - b.minMetric);
  return groupsArray;
})

function createPing() {
  if (pingLoading.value || pingActive.value) {
    pingStatus.value = "Measurement stopped";
    closePingSource();
    return
  }
  clearFlushTimer();

  if (!pingIp.value.trim()) {
    pingError.value = 'An IP address is required';
    return
  }
  pingError.value = null;
  pingLoading.value = true;
  pingResults.value = [];
  pingMetadata.value = {};
  pingReceived.value = 0;
  pingTotal.value = 0;
  pingProgress.value = 0;
  pingStatus.value = '';
  currentGroupPage.value = 1;


  const pingTarget = pingIp.value.trim();
  const qs = new URLSearchParams({ ip: pingTarget, kauth_token: authToken.value }).toString()
  pingSource = new EventSource(`api/agents/ping?${qs}`)

  pingSource.onopen = () => {
    pingLoading.value = false
    pingActive.value = true
  }

  pingSource.addEventListener('meta', (evt) => {
    // Merge incoming metadata for ASNs
    const meta: unknown = JSON.parse(evt.data);
    if (!isPingMeta(meta)) {
      console.error("invalid meta", meta);
      return;
    }
    pingMetadata.value = { ...pingMetadata.value, ...meta };
  })

  pingSource.addEventListener('start', (evt) => {
    const data: unknown = JSON.parse(evt.data);
    if (!isPingStartResponse(data)) {
      pingTotal.value = 0;
      return
    }
    pingTotal.value = data.total;
    pingStatus.value = pingTotal.value
        ? `Contacting ${pingTotal.value} node(s)...`
        : 'No measurement nodes are registered yet.';
  })

  pingSource.addEventListener('result', (evt) => {
    pingReceived.value++;
    if (pingTotal.value > 0) {
      pingProgress.value = Math.min(100, Math.round((pingReceived.value / pingTotal.value) * 100));
    }
    pingStatus.value = `${pingReceived.value} node(s) reported`;
    const data: unknown = JSON.parse(evt.data);
    if (isPingResult(data)) {
      addResult(data);
    } else {
      console.error("invalid ping result", data)
    }
  })

  pingSource.addEventListener('done', () => {
    pingProgress.value = 100
    pingStatus.value = `Completed · ${pingResults.value.length} node(s) reported · Contacted: ${pingTotal.value} · Target IP: ${pingTarget}`
    closePingSource()
  })

  pingSource.addEventListener('error', (evt) => {
    if (evt instanceof MessageEvent) {
      // Server-sent named "error" event with a JSON payload
      let data: unknown;
      try {
        data = JSON.parse(evt.data);
      } catch (_) {
        data = {};
      }
      pingError.value = isPingErrorResponse(data) && data.message
          ? data.message
          : 'The measurement failed.';
    } else if (!pingError.value) {
      // Transport/connection-level error (plain Event, no data)
      pingError.value = 'Connection to the measurement stream was lost.'
      console.error(evt)
    }
    closePingSource()
  })
}

function closePingSource() {
  clearFlushTimer()
  if (pingSource) {
    pingSource.close()
    pingSource = null
  }
  pingLoading.value = false
  pingActive.value = false
}

onUnmounted(() => { closePingSource() })

function fmt(v: string|null) { return v == null ? '—' : v }

function lossPercent(r: PingResultWithMetadata) {
  if (r.sent === 0) return 100;
  return Math.round(((r.sent - r.recv) / r.sent) * 100);
}

function lossClass(r: PingResultWithMetadata) {
  const l = lossPercent(r);
  if (l === 0) {
    return 'text-success bg-success-subtle border border-success-subtle';
  } else if (l < 50) {
    return 'text-warning bg-warning-subtle border border-warning-subtle'
  } else {
    return 'text-danger bg-danger-subtle border border-danger-subtle';
  }
}

// Group pagination
const GROUPS_PER_PAGE = 50
const currentGroupPage = ref(1)

const totalGroupPages = computed(() =>
    Math.max(1, Math.ceil(groupedResults.value.length / GROUPS_PER_PAGE))
)

const paginatedGroups = computed(() => {
  const start = (currentGroupPage.value - 1) * GROUPS_PER_PAGE
  return groupedResults.value.slice(start, start + GROUPS_PER_PAGE)
})

const exploreAsn = ref('');
</script>

<template>
  <NetworkDialog :asn="exploreAsn" @close="exploreAsn = ''"></NetworkDialog>
  <div class="card border-0 shadow-sm rounded-3 mb-4 card-glow">
    <div class="card-body p-4 position-relative">
      <h5 class="fw-bold mb-1 card-heading d-flex align-items-center">
        <FontAwesomeIcon :icon="faSatelliteDish" class="text-primary me-2"/>
        Global Latency Measurement
      </h5>
      <p class="text-muted small mb-4">
        Sends measurement requests to all registered peering nodes to run real-time endpoint latency tests.
      </p>

      <form class="row g-2 align-items-end" @submit.prevent="createPing">
        <div class="col-md-10">
          <label class="form-label form-label-sm text-secondary fw-semibold small">IP Target</label>
          <input
              type="text"
              class="form-control"
              v-model="pingIp"
              placeholder="e.g. 100.20.0.1 or 2001:678:d78::1"
              :disabled="pingLoading || pingActive || !isLoggedIn"
              required
          >
        </div>
        <div class="col-md-2 mt-3 mt-md-0">
          <button
              type="submit"
              class="btn w-100 fw-semibold"
              :class="pingActive ? 'btn-danger' : 'btn-primary'"
              :disabled="!isLoggedIn"
          >
            <template v-if="pingLoading">
              <FontAwesomeIcon :icon="faSpinner" class="fa-spin me-1"/> Cancel
            </template>
            <template v-else-if="pingActive">
              <FontAwesomeIcon :icon="faBan" class="me-1"/> Stop
            </template>
            <template v-else>
              <FontAwesomeIcon :icon="faPlay" class="me-1"/> Start
            </template>
          </button>
        </div>
      </form>

      <div v-if="!isLoggedIn" class="rounded-3 mt-3 p-4 bg-light border border-dashed text-center">
        <div class="fw-bold mb-1 text-dark">
          <FontAwesomeIcon :icon="faUserPlus" class="text-warning me-2"/>
          Access Restricted
        </div>
        <p class="text-muted small mx-auto max-width-xs mb-3">
          Global measurement requests require authentication
        </p>
      </div>

      <div v-if="pingError" class="alert alert-danger mt-3 d-flex align-items-center rounded-3 border-0">
        <FontAwesomeIcon :icon="faExclamationTriangle" class="me-2"/>
        <span>{{ pingError }}</span>
      </div>

      <div v-if="isLoggedIn && (pingStatus || pingResults.length)" class="mt-4 pt-4 border-top">
        <div class="d-flex justify-content-between align-items-center mb-2 small text-secondary">
                <span class="d-flex align-items-center">
                  <FontAwesomeIcon v-if="pingActive" :icon="faClock" class="me-2 text-primary"/>
                  <FontAwesomeIcon v-else :icon="faFlagCheckered" class="me-2 text-primary"></FontAwesomeIcon>
                  <span class="fw-semibold text-dark">{{ pingStatus }}</span>
                </span>
          <span class="fw-bold">{{ pingProgress }}%</span>
        </div>
        <div class="progress mb-2" style="height: 8px;">
          <div
              role="progressbar"
              class="progress-bar progress-bar-striped"
              :class="pingActive ? ['bg-primary', 'progress-bar-animated'] : 'bg-success'"
              :style="{ width: pingProgress + '%' }"
              :aria-valuenow="pingProgress"
          ></div>
        </div>

        <div class="table-responsive border rounded-3 overflow-x-auto overflow-y-hidden mt-3 shadow-xs bg-white">
          <table class="table table-hover table-sm align-middle mb-0">
            <thead class="small-header bg-light">
            <tr>
              <th class="ps-3 text-secondary py-2 border-0">Node Name</th>
              <th class="text-secondary py-2 border-0">Location</th>
              <th class="text-secondary py-2 border-0">Latency</th>
              <th class="text-secondary py-2 border-0">Jitter</th>
              <th class="text-secondary py-2 border-0 text-center">Min / Max</th>
              <th class="text-secondary py-2 border-0 text-center">Recv / Sent</th>
              <th class="text-secondary py-2 border-0 text-end pe-3">Packet Loss</th>
            </tr>
            </thead>

            <template v-for="g in paginatedGroups" :key="g.groupKey">
              <tr class="table-group-divider bg-light-subtle">
                <td colspan="4" class="ps-3 py-2 border-0">
                  <div class="d-flex flex-column">
                    <div class="d-inline-flex align-items-center">
                      <FontAwesomeIcon :icon="faNetworkWired" class="text-primary me-2 small"/>
                      <span class="fw-bold text-dark d-inline-block text-truncate max-width-sm">{{ g.network || 'Unknown Network' }}</span>
                      <span class="badge border bg-white text-muted ms-2 x-small">ASN: {{ g.asn || '—' }}</span>
                    </div>
                    <div v-if="g.description" class="small text-muted fst-italic mt-1">{{ g.description }}</div>
                  </div>
                </td>
                <td colspan="3" class="text-end pe-3 py-1 border-0">
                  <a v-if="g.asn" :href="`/#?asn=${g.asn}`" @click.prevent="exploreAsn = g.asn" class="btn btn-xs btn-outline-primary py-0 px-2 fw-semibold text-decoration-none">
                    <FontAwesomeIcon :icon="faLink" class="me-1 smallest"/> Explore Network
                  </a>
                </td>
              </tr>

              <tr v-for="r in g.results" :key="r.node">
                <td class="ps-4">
                  <div class="fw-semibold text-secondary-emphasis">{{ r.node || 'Unknown Agent' }}</div>
                </td>
                <td class="small py-2 text-muted-subtle">
                  <template v-if="g.servers && g.servers[r.node]">
                    {{ g.servers[r.node].CountryCode || 'Unknown' }}{{ g.servers[r.node].City ? ', ' + g.servers[r.node].City : '' }}
                  </template>
                  <template v-else>
                    Unknown
                  </template>
                </td>
                <td>
                  <span v-if="r.reachable" :class="[r.latency > 100 ? 'text-warning' : 'text-success', 'fw-bold', 'd-inline-flex', 'align-items-baseline']">
                    {{ fmt(r.latency) }}<span class="smallest fw-normal ms-1 text-muted">ms</span>
                  </span>
                  <span v-else-if="r.sent === 0" class="text-danger fw-semibold">error</span>
                  <span v-else class="text-danger fw-semibold">unreachable</span>
                </td>
                <td class="text-muted small">{{ fmt(r.jitter) }}</td>
                <td class="text-center small text-muted-subtle"><code>{{ fmt(r.min_rtt) }}</code> / <code>{{ fmt(r.max_rtt) }}</code></td>
                <td class="text-center small text-muted-subtle"><span>{{ r.recv }} / {{ r.sent }}</span></td>
                <td class="text-end pe-3">
                  <span class="badge rounded-pill x-small px-2 py-1"  :class="lossClass(r)">
                    {{ lossPercent(r) }}% Loss
                  </span>
                </td>
              </tr>
            </template>

            <tbody v-if="!pingResults.length">
            <tr><td colspan="7" class="text-center text-muted py-4 small">Waiting for measurements...</td></tr>
            </tbody>
          </table>
          <div v-if="totalGroupPages > 1" class="d-flex justify-content-between align-items-center mt-3 px-2 border-top">
            <span class="small text-muted">
              Page {{ currentGroupPage }} of {{ totalGroupPages }}
              · {{ groupedResults.length }} networks
            </span>
            <div class="btn-group btn-group-sm my-2">
              <button
                  class="btn btn-outline-secondary"
                  :disabled="currentGroupPage === 1 || pingActive || pingLoading"
                  @click="currentGroupPage--"
              >
                Previous
              </button>
              <button
                  class="btn btn-outline-secondary"
                  :disabled="currentGroupPage === totalGroupPages || pingActive || pingLoading"
                  @click="currentGroupPage++"
              >
                Next
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.small-header th { font-size: 0.75rem; font-weight: 700; letter-spacing: 0.05em; }
.border-dashed { border-style: dashed !important; }
.btn-xs { padding: 0.15rem 0.4rem; font-size: 0.75rem; }
.smallest { font-size: 0.7rem; }
.x-small { font-size: 0.75rem; }

.max-width-xs { max-width: 320px; }
.max-width-sm { max-width: 200px; }
.text-muted-subtle { color: #6c757d; }
.progress { background-color: #e9ecef; }
</style>