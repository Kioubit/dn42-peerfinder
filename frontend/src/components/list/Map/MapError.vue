<script setup lang="ts">
interface Props {
  error: Error
}
defineProps<Props>()
</script>

<template>
  <div class="card border-0 shadow-sm rounded-3 mb-4 overflow-hidden">
    <div class="card-header bg-white py-3">
      <h5 class="fw-bold mb-0 text-danger">Map Error</h5>
    </div>
    <div class="d-flex flex-column justify-content-center align-items-center bg-light" style="height: 420px;">
      <div class="alert alert-warning border-0 shadow-sm mx-4">
        <h6 class="alert-heading fw-bold">Failed to load component</h6>
        <p class="mb-0 small">{{ error.message || 'The connection timed out or the server is unreachable.' }}</p>
      </div>
    </div>
  </div>
</template>