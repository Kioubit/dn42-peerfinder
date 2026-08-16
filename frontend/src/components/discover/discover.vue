<script setup lang="ts">
import {computed, ref, watch} from 'vue'
import { FontAwesomeIcon } from "@fortawesome/vue-fontawesome";
import {
  faCompass, faDownload, faServer, faPlus,
  faSpinner, faCheck, faExclamationTriangle, faCopy, faPlay,
  faClock, faBan, faTrashCan, faPenToSquare, faFloppyDisk, faXmark,
  faCircle, faChartLine, faEyeSlash, faEye, faVial, faTerminal
} from "@fortawesome/free-solid-svg-icons";
import Ping from "./ping.vue";
import {useAuth} from "@/composables/useAuth.ts";
import {
  type AgentInfo,
  type AgentStatistics, type EditAgentPayload,
  isRegisterAgentResponse,
  parseAgentInfo, parseAgentStatistics,
  type RegisterAgentResponse
} from "@/types/api/agents.ts";
import {isNetwork, type Server} from "@/types/api/directory.ts";
import {isUnknownArray} from "@/types/main.ts";
import {copyText} from "@/util.ts";

type AgentMetadata = {
  showSecret: boolean;
  editing: boolean;
  editName: string;
  editEndpoint: string;
  saving: boolean;
  testing: boolean;
  testResult: string | null;
  testSuccess: boolean;
};

const agentMetadataDefaults: AgentMetadata = {
  showSecret: false,
  editing: false,
  editName: '',
  editEndpoint: '',
  saving: false,
  testing: false,
  testResult: null,
  testSuccess: false,
};

type AgentWithMetadata = AgentInfo & AgentMetadata;

function addAgentMetadata(agent: AgentInfo): AgentWithMetadata {
  return {
    ...agent,
    ...agentMetadataDefaults,
  };
}

const AGENT_VERSION = "1.2.0";

const {authToken, isLoggedIn} = useAuth();

const agentUrl = "agent/peerfinder-agent.py";
const agentSystemdUrl = "agent/peerfinder-agent.service";
const agentInstallUrl = "agent/install.sh";

const installCommand = computed(() =>
    `curl -fsSL ${window.location.origin}/${agentInstallUrl} | sh`
);

const installWithSecretCommand = computed(() => {
  const key = registeredAgent.value?.hmac_key;
  return key
      ? `${installCommand.value} --secret '${key}'`
      : installCommand.value;
});

function toggleSecret(agent: AgentWithMetadata) {
  agent.showSecret = !agent.showSecret
}

const agentFormData = ref({ name: '', endpoint: '' })
const registerLoading = ref(false)
const registerError = ref<string|null>(null)
const registeredAgent = ref<RegisterAgentResponse|null>(null)

function addAuthHeader(options: RequestInit = {}) {
  const headers = new Headers(options.headers || {});
  headers.set('kauth-token', authToken.value);

  return {
    ...options,
    headers
  }
}

async function registerAgent() {
  registerError.value = null
  registerLoading.value = true
  try {
    const res = await fetch(`api/agents/register`, addAuthHeader({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: agentFormData.value.name.trim(),
        endpoint: agentFormData.value.endpoint.trim()
      })
    }))
    if (!res.ok) {
      const text = await res.text()
      throw new Error(text || ('Error ' + res.status))
    }
    const data: unknown = await res.json()
    if (!isRegisterAgentResponse(data)) {
      throw new Error("invalid data")
    }
    registeredAgent.value = data;
    agentFormData.value = { name: '', endpoint: '' }
    loadAgents().then()
  } catch (e) {
    registerError.value = e instanceof Error ? e.message : String(e);
  } finally {
    registerLoading.value = false
  }
}

const agents = ref<AgentWithMetadata[]>([]);
const agentsLoading = ref(false);
const agentsError = ref<string|null>(null);

function agentStatus(a: AgentInfo) {
  if (a.disabled) {
    return { text: "Disabled", class: "text-secondary bg-light" }
  } else if (!a.last_seen) {
    if (!a.last_probed) {
      return { text: "Inactive (never probed)", class: "text-secondary bg-light" }
    } else {
      return { text: "Never connected", class: "text-danger bg-danger-subtle" }
    }
  } else if (a.last_probed
      ? new Date(a.last_seen).getTime() >= new Date(a.last_probed).getTime()
      : true
  ) {
    return { text: `Active (${timeAgoPlain(a.last_seen)})`, class: "text-success bg-success-subtle" }
  } else {
    return { text: `Inactive (${timeAgoPlain(a.last_seen)})`, class: "text-secondary bg-light" }
  }
}

async function loadAgents() {
  if (!isLoggedIn.value) return;
  agentsLoading.value = true;
  agentsError.value = null;
  try {
    const agentListPromise = fetch("api/agents/self", addAuthHeader())
        .then(async res => {
          if (!res.ok) throw new Error("Error " + res.status);
          const data: unknown = await res.json();
          if (!isUnknownArray(data)) {
            throw new Error("Expected agent list");
          }
          return data.map(parseAgentInfo).map(addAgentMetadata);
        });
    const unknownAgentsPromise = fetchDirectoryServers();
    agents.value = await agentListPromise;
    await unknownAgentsPromise;
  } catch (e) {
    agentsError.value = (e instanceof Error)? e.message: String(e);
  } finally {
    agentsLoading.value = false;
  }
}

const directoryServers = ref<readonly Server[]>([]);
const unknownAgents = computed(() => {
  let cnt = 0;
  agents.value.forEach((a) => {
    if (!directoryServers.value.some((s) => {
      return s.ID === a.id;
    })) {
      cnt++;
    }
  })
  return cnt;
})

async function fetchDirectoryServers() {
  try {
    const response = await fetch(`api/directory/self`, addAuthHeader());
    if (!response.ok) throw new Error(`Failed to load: ${response.status}`);
    const data: unknown = await response.json();
    if (!isNetwork(data)) {
      throw new Error("invalid server list")
    }

    directoryServers.value = data.Servers ?? [];
  } catch (e) {
    console.warn("fetchDirectoryServers", e);
  }
}

async function deleteAgent(a: AgentInfo) {
  if (!confirm(`Delete node "${a.id}"? This cannot be undone.`)) return;
  agentsError.value = null;
  try {
    const res = await fetch(`api/agents/${a.uuid}`, addAuthHeader({ method: 'DELETE' }))
    if (!res.ok) {
      const text = await res.text()
      throw new Error(text || ('Error ' + res.status))
    }
    agents.value = agents.value.filter(x => x.uuid !== a.uuid)
  } catch (e) {
    agentsError.value = (e instanceof Error)? e.message: String(e);
  }
}

async function updateAgent(a: AgentInfo, payload: EditAgentPayload) {
  agentsError.value = null
  try {
    const res = await fetch(`api/agents/${a.uuid}/edit`, addAuthHeader({
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload)
    }));
    if (!res.ok) throw new Error(await res.text() || 'Error ' + res.status)
    return true
  } catch (e) {
    agentsError.value = (e instanceof Error)? e.message: String(e);
    return false
  }
}
async function toggleDisable(a: AgentWithMetadata) {
  if (a.saving) return
  a.saving = true
  const success = await updateAgent(a, { name: a.id, endpoint: a.endpoint, disabled: !a.disabled })
  if (success) a.disabled = !a.disabled
  a.saving = false
}
function startEdit(a: AgentWithMetadata) {
  a.editing = true;
  a.editName = a.id
  a.editEndpoint = a.endpoint || '';
}
function cancelEdit(a: AgentWithMetadata) {
  a.editing = false;
  a.editName = '';
  a.editEndpoint = '';
  agentsError.value = null;
}

async function saveEdit(a: AgentWithMetadata) {
  a.saving = true
  const payload = { name: a.editName.trim(), endpoint: a.editEndpoint.trim(), disabled: a.disabled }
  if (await updateAgent(a, payload)) {
    a.id = payload.name
    a.endpoint = payload.endpoint
    a.editing = false
  }
  a.saving = false
}

function timeAgoPlain(date: Date | null) {
  if (!date) return 'N/A'

  const diff = Date.now() - date.getTime()
  const m = Math.floor(diff / 60000)

  if (m < 1) return 'just now'
  if (m < 60) return m + 'm ago'

  const h = Math.floor(m / 60)
  if (h < 24) return h + 'h ago'

  const d = Math.floor(h / 24)
  return d + 'd ago'
}

function isOutdated(a: AgentInfo) {
  return a.version && a.version !== AGENT_VERSION
}

watch(() => isLoggedIn.value, (val) => { if (val) loadAgents() })
if (isLoggedIn.value) loadAgents()

// Agent statistics
const statistics = ref<AgentStatistics|null>(null)
async function loadStatistics() {
  try {
    const res = await fetch(`api/agents/statistics`)
    if (res.ok) {
      const data: unknown = await res.json()
      statistics.value = parseAgentStatistics(data);
    }
  } catch (e) {}
}
loadStatistics();

// Agent test
async function testAgent(a: AgentWithMetadata) {
  if (a.testing) return
  a.testing = true
  a.testResult = null
  try {
    const res = await fetch(`api/agents/${a.uuid}/test`, addAuthHeader())
    if (!res.ok) {
      a.testResult = await res.text() || ('Error ' + res.status)
      a.testSuccess = false
    } else {
      a.testSuccess = true
      a.testResult = 'Agent responded. Note: actual ping functionality was not tested.'
    }
  } catch (e) {
    a.testResult = (e instanceof Error)? e.message: String(e);
    a.testSuccess = false
  } finally {
    a.testing = false
  }
}
</script>

<template>
  <div class="dash-layout">

    <div class="row mb-5">
      <div class="col-12">
        <div class="card border-0 bg-light-subtle shadow-sm border-start border-primary border-4 py-2">
          <div class="card-body">
            <div class="d-md-flex justify-content-between align-items-center">
              <div>
                <div class="h5 fw-bold d-flex align-items-center mb-1">
                  <FontAwesomeIcon class="text-primary me-2" :icon="faCompass"/>
                  Local Network Node Discovery
                </div>
                <div class="text-muted small">
                  Pings all peering nodes locally using a local copy of this Node directory. Easily locate new peers.
                </div>
              </div>
              <div class="mt-3 mt-md-0">
                <a href="api/directory/download_script" class="btn btn-outline-primary fw-semibold btn-md shadow-xs text-nowrap">
                  <FontAwesomeIcon :icon="faDownload" class="me-2"/> Download Script
                </a>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="row g-4">
      <div class="col-xl-8 col-lg-7">
        <ping></ping>
      </div>

      <div class="col-xl-4 col-lg-5">
        <div class="card border-0 shadow-sm mb-4">
          <div class="card-body p-4">
            <h6 class="fw-bold text-uppercase tracking-wider small text-muted mb-3 d-flex align-items-center">
              <FontAwesomeIcon :icon="faServer" class="text-primary me-2"/>
              Live node contribution agent
            </h6>
            <div class="d-flex flex-wrap align-items-center gap-2 mb-3">
                <span class="badge rounded-pill bg-success-subtle text-success px-3 py-2 x-small text-nowrap">
                  <FontAwesomeIcon :icon="faCircle" class="smallest me-1"/>
                  {{ statistics?.active ?? '—' }} Active
                </span>
              <span class="badge rounded-pill bg-primary-subtle text-primary-emphasis px-3 py-2 x-small text-nowrap">
                  <FontAwesomeIcon :icon="faServer" class="smallest me-1"/>
                  {{ statistics?.registered ?? '—' }} Registered
                </span>
              <span  class="smallest text-muted">
                  updated {{ statistics ? timeAgoPlain(statistics?.update_time): '—' }}
                </span>
            </div>
            <p class="text-muted small">
              Deploy <code>peerfinder-agent.py</code> on your nodes to feed live latency measurements back to this page. The agent listens on a TCP port and only accepts requests signed by the peerfinder with your secret key.
              <span class="d-block"><a :href="agentSystemdUrl" target="_blank">Example systemd config</a></span>
            </p>
            <div class="d-grid gap-2">
              <a :href="agentUrl" download class="btn btn-outline-dark btn-sm py-2">
                <FontAwesomeIcon :icon="faDownload" class="text-primary me-2"/>
                Download agent script (python)
              </a>
            </div>

            <!-- One-click installer -->
            <div class="mt-4 p-3 rounded-3 border bg-light-subtle">
              <div class="d-flex align-items-center mb-2">
                <FontAwesomeIcon :icon="faTerminal" class="text-primary me-2"/>
                <span class="fw-bold small">One-click Install</span>
              </div>
              <p class="smallest text-muted mb-2">
                Run this directly on your node to download and install the agent automatically:
              </p>
              <div class="input-group input-group-sm mb-1">
                <input class="form-control bg-white font-monospace" :value="installCommand" readonly>
                <button class="btn btn-outline-primary" type="button" @click="copyText(installCommand, $event)">
                  <FontAwesomeIcon :icon="faCopy" class="me-1"/>
                  <span data-copy-toggle>Copy</span>
                  <span data-copy-toggle hidden>Copied!</span>
                </button>
              </div>
            </div>

            <template v-if="isLoggedIn">
              <div class="border-top my-4 pt-4">
                <h6 class="fw-bold small text-muted mb-2 text-uppercase tracking-wider">Register Node</h6>
                <p class="smallest text-muted mb-2">The agent listens on a TCP port. Enter the host and port reachable by the peerfinder.</p>
                <div v-if="registerError" class="alert alert-danger mb-3 p-2 small border-0 d-flex align-items-center rounded-3">
                  <FontAwesomeIcon :icon="faExclamationTriangle" class="me-2"/>
                  <span>{{ registerError }}</span>
                </div>
                <form @submit.prevent="registerAgent">
                  <div class="mb-2">
                    <label class="smallest fw-bold text-secondary mb-1">Node ID (Must match the node ID in the directory)</label>
                    <input type="text"
                           class="form-control form-control-sm"
                           v-model="agentFormData.name"
                           placeholder="e.g. dc1-us-east"
                           :disabled="registerLoading"
                           required>
                  </div>

                  <div class="mb-3">
                    <label class="smallest fw-bold text-secondary mb-1">Internet Endpoint</label>
                    <input type="text"
                           class="form-control form-control-sm"
                           v-model="agentFormData.endpoint"
                           placeholder="node.example.org:9000"
                           :disabled="registerLoading"
                           required>
                  </div>

                  <button type="submit" class="btn btn-sm btn-primary w-100 fw-bold shadow-sm" :disabled="registerLoading || !agentFormData.name">
                    <FontAwesomeIcon v-if="registerLoading" :icon="faSpinner" class="fa-spin me-2"/>
                    <FontAwesomeIcon v-else :icon="faPlus" class="me-2"/>
                    Add Measurement Node
                  </button>
                </form>
              </div>

              <div v-if="registeredAgent" class="mt-3 p-3 bg-success-subtle rounded-3 border-0 small text-success-emphasis position-relative">
                <button type="button" class="btn-close position-absolute top-0 end-0 mt-2 me-2" @click="registeredAgent = null"></button>
                <div class="d-flex align-items-center mb-2 pe-4">
                  <FontAwesomeIcon :icon="faCheck" class="me-2 text-success"/>
                  <span class="fw-semibold">Node Registered!</span>
                </div>
                <div class="smallest text-muted mb-2">
                  Run <code>peerfinder-agent.py</code> on your node. It must be reachable at <code>{{ registeredAgent.endpoint }}</code>.
                  Make sure to set the environment variable with your secret key.
                </div>
                <div class="input-group input-group-sm mb-2">
                  <span class="input-group-text bg-white border">Secret key</span>
                  <input class="form-control bg-white" :value="registeredAgent?.hmac_key" readonly>
                  <button class="btn btn-outline-secondary" @click="copyText(registeredAgent?.hmac_key, $event)">
                    <FontAwesomeIcon :icon="faCopy"/>
                    <span data-copy-toggle>Copy</span>
                    <span data-copy-toggle hidden>Copied!</span>
                  </button>
                </div>

                <div class="smallest text-muted mb-1 mt-3">
                  Or run the one-click installer with your key pre-filled:
                </div>
                <div class="input-group input-group-sm">
                  <input class="form-control bg-white font-monospace" :value="installWithSecretCommand" readonly>
                  <button class="btn btn-outline-success" type="button" @click="copyText(installWithSecretCommand, $event)">
                    <FontAwesomeIcon :icon="faCopy" class="me-1"/>
                    <span data-copy-toggle>Copy</span>
                    <span data-copy-toggle hidden>Copied!</span>
                  </button>
                </div>
              </div>
            </template>
            <div v-else class="p-3 mt-4 rounded-3 bg-light border border-dashed text-center">
              You must authenticate to register your node
            </div>
          </div>
        </div>

        <div v-if="isLoggedIn" class="card border-0 shadow-sm">
          <div class="card-header bg-white border-0 pt-4 pb-2 d-flex justify-content-between align-items-center">
            <h6 class="fw-bold mb-0 text-muted small text-uppercase tracking-wider">
              <FontAwesomeIcon :icon="faChartLine" class="text-primary me-2"/>
              Contributed Nodes
            </h6>
            <button class="btn btn-link p-0 text-decoration-none small text-primary fw-semibold" @click="loadAgents" :disabled="agentsLoading">
              <FontAwesomeIcon :icon="faSpinner" :class="{'fa-spin': agentsLoading}" class="me-1"/> Reload
            </button>
          </div>
          <div class="card-body px-0 pt-0">
            <div v-if="agentsError" class="alert alert-danger mx-4 mt-2 mb-3 p-2 small border-0 d-flex align-items-center">
              <FontAwesomeIcon :icon="faExclamationTriangle" class="me-2"/>
              <span class="me-auto">{{ agentsError }}</span>
              <button type="button" class="btn-close ms-2" style="font-size: 0.5rem" @click="agentsError = null"></button>
            </div>
            <div v-if="agentsLoading" class="text-center py-4 text-muted small">
              <FontAwesomeIcon :icon="faSpinner" class="fa-spin me-1"/>Refreshing...
            </div>
            <div v-else-if="!agents.length" class="text-center text-muted py-5 px-3 small">No nodes contributed.</div>
            <template v-else>
              <div v-if="unknownAgents > 0" class="alert alert-danger mx-4 mt-2 mb-3 p-2 small border-0 d-flex align-items-center">
                <FontAwesomeIcon :icon="faExclamationTriangle" class="me-2"/>
                {{unknownAgents}} of the registered node IDs could not be matched to entries in the directory.
              </div>
              <div class="table-responsive border-bottom">
                <table class="table align-middle mb-0">
                  <thead>
                  <tr class="border-light">
                    <th class="ps-4 py-2 text-uppercase x-small text-muted fw-semibold">Agent</th>
                    <th class="py-2 text-uppercase x-small text-muted fw-semibold">Status</th>
                    <th class="pe-4 py-2 text-end text-uppercase x-small text-muted fw-semibold">Actions</th>
                  </tr>
                  </thead>
                  <tbody>
                  <tr v-for="a in agents" :key="a.uuid" class="border-light hover-bg">
                    <td class="ps-4 py-3">
                      <div v-if="!a.editing">
                        <div class="d-flex align-items-center gap-2">
                          <strong class="text-dark small">{{ a.id }}</strong>
                          <span v-if="a.version"
                                class="badge x-small border"
                                :class="isOutdated(a) ? 'bg-warning-subtle text-warning-emphasis' : 'bg-light text-secondary'"
                                :title="isOutdated(a) ? `Outdated - latest is v${AGENT_VERSION}` : ''">
                          <FontAwesomeIcon v-if="isOutdated(a)" :icon="faExclamationTriangle" class="me-1 smallest"/>
                          v{{ a.version }}
                        </span>
                        </div>

                        <div class="d-flex align-items-center gap-1 mt-1">
                          <FontAwesomeIcon :icon="a.showSecret ? faEye : faEyeSlash"
                                           class="smallest text-muted" role="button"
                                           @click="toggleSecret(a)"/>
                          <code v-if="a.showSecret"
                                class="x-small text-primary text-break user-select-all">
                            {{ a.hmac_key }}
                          </code>
                          <code v-else
                                class="x-small text-muted" role="button"
                                @click="toggleSecret(a)">click to reveal secret key</code>
                        </div>

                        <div v-if="a.endpoint" class="d-flex align-items-center gap-1 mt-1 text-muted x-small text-nowrap">
                          <FontAwesomeIcon :icon="faServer" class="smallest"/>
                          <code class="x-small text-muted">{{ a.endpoint }}</code>
                        </div>

                        <div v-if="a.added_at" class="d-flex align-items-center gap-1 mt-1 text-muted x-small text-nowrap">
                          <FontAwesomeIcon :icon="faClock" class="smallest"/>
                          added {{ timeAgoPlain(a.added_at) }}
                        </div>
                        <div v-if="a.testing" class="mt-2">
                          <div class="alert p-2 rounded-3 small border-0 d-flex align-items-center gap-2 bg-info-subtle text-info-emphasis">
                            <FontAwesomeIcon :icon="faSpinner" class="fa-spin me-1 smallest flex-shrink-0"/>
                            <span class="small">Testing agent...</span>
                          </div>
                        </div>
                        <div v-if="a.testResult" class="mt-2">
                          <div class="alert p-2 rounded-3 small border-0 d-flex align-items-start gap-2"
                               :class="a.testSuccess ? 'bg-success-subtle text-success-emphasis' : 'bg-danger-subtle text-danger-emphasis'">
                            <FontAwesomeIcon :icon="a.testSuccess ? faCheck : faExclamationTriangle" class="me-1 mt-1 smallest flex-shrink-0"/>
                            <span class="small">{{ a.testResult }}</span>
                            <button type="button" class="btn-close btn-sm ms-auto flex-shrink-0" @click="a.testResult = null"></button>
                          </div>
                        </div>
                      </div>

                      <div v-else class="py-1">
                        <div class="mb-2">
                          <label class="smallest text-muted fw-bold d-block mb-1">Node ID</label>
                          <input type="text" class="form-control form-control-sm" v-model="a.editName">
                        </div>

                        <div class="mb-2">
                          <label class="smallest text-muted fw-bold d-block mb-1">Endpoint</label>
                          <input type="text" class="form-control form-control-sm" v-model="a.editEndpoint" placeholder="host:port">
                        </div>

                        <div class="d-flex gap-1 mt-2">
                          <button class="btn btn-success btn-xs text-white flex-grow-1 py-1" @click="saveEdit(a)" :disabled="a.saving">
                            <FontAwesomeIcon :icon="a.saving ? faSpinner : faFloppyDisk" :class="{'fa-spin': a.saving}" class="me-1"/>
                            {{ a.saving ? 'Saving...' : 'Save' }}
                          </button>
                          <button class="btn btn-light border btn-xs text-secondary py-1" @click="cancelEdit(a)">
                            <FontAwesomeIcon :icon="faXmark"/>
                          </button>
                        </div>
                      </div>
                    </td>

                    <td class="text-nowrap">
                    <span class="badge rounded-pill x-small text-nowrap px-3 py-2" :class="agentStatus(a).class">
                      <FontAwesomeIcon :icon="faCircle" class="smallest me-1"/>
                        {{ agentStatus(a).text }}
                    </span>
                    </td>

                    <td class="text-end pe-4 text-nowrap">
                      <template v-if="!a.editing">
                        <div class="btn-group btn-group-xs shadow-xs rounded border">
                          <button class="btn btn-white btn-sm" title="Edit" @click="startEdit(a)"><FontAwesomeIcon :icon="faPenToSquare"/></button>
                          <button class="btn btn-white btn-sm text-info" title="Test agent" @click="testAgent(a)" :disabled="a.testing">
                            <FontAwesomeIcon :icon="a.testing ? faSpinner : faVial" :class="{'fa-spin': a.testing}"/>
                          </button>
                          <button class="btn btn-white btn-sm"
                                  :class="a.disabled ? 'text-success' : 'text-warning'"
                                  :title="a.disabled ? 'Re-enable node' : 'Disable node'"
                                  @click="toggleDisable(a)"
                                  :disabled="a.saving">
                            <FontAwesomeIcon :icon="a.saving ? faSpinner : (a.disabled ? faPlay : faBan)" :class="{'fa-spin': a.saving}"/>
                          </button>
                          <button class="btn btn-white btn-sm text-danger" title="Delete" @click="deleteAgent(a)"><FontAwesomeIcon :icon="faTrashCan"/></button>
                        </div>
                      </template>
                      <template v-else>
                        <div class="btn-group btn-group-xs">
                          <button class="btn btn-success btn-xs text-white" title="Save" @click="saveEdit(a)" :disabled="a.saving">
                            <FontAwesomeIcon :icon="a.saving ? faSpinner : faFloppyDisk" :class="{'fa-spin': a.saving}"/>
                          </button>
                          <button class="btn btn-outline-secondary btn-xs" title="Cancel" @click="cancelEdit(a)" :disabled="a.saving"><FontAwesomeIcon :icon="faXmark"/></button>
                        </div>
                      </template>
                    </td>
                  </tr>
                  </tbody>
                </table>
              </div>
            </template>
            <div class="px-4 pt-2">
              <div class="text-muted x-small">
                <ul class="mb-1 ps-3">
                  <li><strong>Health checks:</strong> Nodes are probed periodically. Unresponsive nodes are
                    auto-disabled after 30 days and may later be removed.</li>
                  <li><strong>Source IPs:</strong> Source IPs used for ping requests may change at any time.</li>
                  <li>
                    <strong>Security:</strong> The peerfinder agent is designed to be
                    secure and resistant to denial-of-service attacks, making it safe to
                    run on the public internet.
                  </li>
                  <li>
                    <strong>Rate limiting:</strong> Ping requests are throttled to avoid
                    excessive load on your node.
                  </li>
                  <li>
                    <strong>Supported software:</strong> Only the official peerfinder
                    agent is recommended and supported.
                  </li>
                </ul>
                Current Agent version: {{AGENT_VERSION}}
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.dash-layout {
  padding: 0 0.75rem 4rem;
  max-width: 140rem;
  margin: 0 auto;
}
.small-header th { font-size: 0.75rem; font-weight: 700; letter-spacing: 0.05em; }
.tracking-wider { letter-spacing: 0.06em; }
.smallest { font-size: 0.7rem; }
.x-small { font-size: 0.75rem; }
.btn-xs { padding: 0.15rem 0.4rem; font-size: 0.75rem; }
.btn-group-xs > .btn { padding: 0.2rem 0.4rem; font-size: 0.75rem; }
.btn-white { background: #ffffff; border: none; }
.btn-white:hover { background: #f8f9fa; }
.hover-bg:hover { background-color: #fafbfc; }
.border-dashed { border-style: dashed !important; }
</style>