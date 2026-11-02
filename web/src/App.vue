<script setup lang="ts">
import { onMounted } from 'vue'
import { usePrefsStore } from '@/stores/prefs'
import { useAuthStore } from '@/stores/auth'
import Toasts from '@/components/ui/Toasts.vue'
import ForcePasswordChange from '@/components/ForcePasswordChange.vue'
import ForceOtpSetup from '@/components/ForceOtpSetup.vue'
import StepUpModal from '@/components/ui/StepUpModal.vue'
import KonamiEgg from '@/components/KonamiEgg.vue'
import TomasEgg from '@/components/TomasEgg.vue'
import EmbedEditor from '@/components/embeds/EmbedEditor.vue'
import FileViewerOverlay from '@/components/files/FileViewerOverlay.vue'

const prefs = usePrefsStore()
const auth = useAuthStore()

onMounted(() => {
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (prefs.theme === 'system') prefs.applyTheme()
  })
})
</script>

<template>
  <RouterView />
  <Toasts />
  <ForcePasswordChange v-if="auth.isAuthenticated && auth.user?.must_change_password" />
  <ForceOtpSetup v-else-if="auth.isAuthenticated && auth.mustSetupOtp" />
  <StepUpModal />
  <KonamiEgg />
  <TomasEgg />
  <EmbedEditor v-if="auth.isAuthenticated" />
  <FileViewerOverlay v-if="auth.isAuthenticated" />
</template>
