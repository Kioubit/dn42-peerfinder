<script setup lang="ts">
import {onMounted, ref, useTemplateRef, watch} from 'vue'
import ListItem from './ListItem.vue'
import {FontAwesomeIcon} from '@fortawesome/vue-fontawesome'
import {faTag, faCircleExclamation} from '@fortawesome/free-solid-svg-icons'
import {isNetwork, type ListResponseNetwork} from "@/types/api/directory.ts";
interface Props {
  asn: string
}
const props = defineProps<Props>()

const emit = defineEmits(['close']);

const dialogRef = useTemplateRef('dialog');
const loading = ref(false);
const item = ref<ListResponseNetwork | null>(null);
const error = ref<string | null>(null);

let controller: AbortController | null = null;

function close() {
  if (dialogRef.value?.isConnected) {
    dialogRef.value?.close();
  }
}

const openAndFetch = async (asn: string) => {
  if (!dialogRef.value?.isConnected) {
    return;
  }

  if (!dialogRef.value?.open) {
    dialogRef.value?.showModal();
  }

  loading.value = true;
  item.value = null;
  error.value = null;

  if (controller) controller.abort();
  controller = new AbortController();

  try {
    const response = await fetch(
        `api/directory/network/${encodeURIComponent(asn)}`,
        {signal: controller.signal}
    );

    if (!response.ok) {
      if (response.status === 404) {
        throw new Error("Network linked to was not found in the directory");
      }
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const data: unknown = await response.json();
    if (!isNetwork(data)) {
      throw new Error("invalid data");
    }

    item.value = {...data, "asn": asn, "serverCount": data.Servers? data.Servers.length: 0};
  } catch (e) {
    if (!(e instanceof Error)) return;
    if (e.name === 'AbortError') return;
    error.value = e.message;
    console.warn('Error fetching network:', e);
  } finally {
    if (controller?.signal.aborted === false) {
      loading.value = false;
    }
  }
}

// Watch "asn" prop.
// Non-empty -> open + fetch.  Empty -> close (if open).
watch(() => props.asn, (newVal) => {
  if (newVal) {
    openAndFetch(newVal);
  } else if (dialogRef.value?.open) {
    close();
  }
})

onMounted(() => {
  if (props.asn !== "") {
    openAndFetch(props.asn);
  }
})

// Native <dialog> fires "close" on Escape, backdrop, or programmatic close().
const handleClose = () => {
  if (controller) controller.abort();
  item.value = null;
  loading.value = false;
  error.value = null;
  emit('close');
}
</script>

<template>
  <dialog
      ref="dialog"
      class="asn-dialog border-0 rounded-3 p-0 shadow-lg w-75"
      @close="handleClose"
      closedby="any"
  >
    <div class="asn-dialog__content d-flex flex-column overflow-hidden rounded-3">
      <div class="d-flex align-items-center justify-content-between p-3 bg-light flex-shrink-0">
        <h5 class="m-0 fw-bold fs-5">
          <FontAwesomeIcon :icon="faTag" class="me-2 text-primary"/>
          Network View
        </h5>
        <button type="button" class="btn-close" @click="close"></button>
      </div>

      <div class="overflow-y-auto p-4 flex-grow-1">
        <!-- Loading skeleton -->
        <div v-if="loading" class="placeholder-glow">
          <div class="card border-0">
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

        <!-- Error / not-found state -->
        <div v-else-if="error" class="text-center py-4">
          <FontAwesomeIcon :icon="faCircleExclamation" class="fa-2x text-danger mb-2"/>
          <p class="text-muted mb-0">{{ error }}</p>
        </div>

        <!-- Network details -->
        <ListItem v-else-if="item" :item="item" :autoExpand="true"/>
      </div>
    </div>
  </dialog>
</template>
<style scoped>
.asn-dialog {
  min-width: 20rem;
  max-width: 50rem;
}

.asn-dialog::backdrop {
  background: rgba(0, 0, 0, 0.5);
}

.asn-dialog__content {
  max-height: 90vh;
}
</style>