<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import FileViewer from '@/components/files/FileViewer.vue'

// A file on a page of its own (/w/:ws/f/:id, or /files/f/:id for one's own):
// where a link to it, or a search, leads.
const route = useRoute()
const id = computed(() => String(route.params.id))
</script>

<template>
  <div class="file-page"><FileViewer :id="id" /></div>
</template>

<style scoped>
.file-page {
  display: flex;
  height: 100%;
  min-height: calc(100vh - 0px);
  flex-direction: column;
}
.file-page > :deep(*) {
  flex: 1;
  min-height: 0;
}
</style>
