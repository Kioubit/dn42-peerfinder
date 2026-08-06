<script setup lang="ts">
interface Props {
  error: Error
}
defineProps<Props>()
</script>

<template>
  <div class="alert alert-danger m-3 mb-0" role="alert">
    <div>Failed to load component. Please try refreshing the page.</div>
    <small class="text-muted">{{ error.message }}</small>
  </div>
</template>