<script setup lang="ts">
import { onMounted } from 'vue'
import { usePrefsStore } from '@/stores/prefs'
import { useAuthStore } from '@/stores/auth'
import Toasts from '@/components/ui/Toasts.vue'
import ForcePasswordChange from '@/components/ForcePasswordChange.vue'
import ForceOtpSetup from '@/components/ForceOtpSetup.vue'
import StepUpModal from '@/components/ui/StepUpModal.vue'
import KonamiEgg from '@/components/KonamiEgg.vue'
import EmbedEditor from '@/components/embeds/EmbedEditor.vue'
import FileViewerOverlay from '@/components/files/FileViewerOverlay.vue'

const prefs = usePrefsStore()
const auth = useAuthStore()

onMounted(() => {
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
    if (prefs.theme === 'system') prefs.applyTheme()
  })
  // Day and night: checked as each minute begins (so "dark from 20:02" turns
  // at 20:02:00, not up to a minute later), and when the app comes back to screen.
  const onTheMinute = () => {
    prefs.tickTheme()
    setTimeout(onTheMinute, 60_000 - (Date.now() % 60_000) + 50)
  }
  setTimeout(onTheMinute, 60_000 - (Date.now() % 60_000) + 50)
  document.addEventListener('visibilitychange', () => document.visibilityState === 'visible' && prefs.tickTheme())
})
</script>

<template>
  <RouterView />
  <Toasts />
  <ForcePasswordChange v-if="auth.isAuthenticated && auth.user?.must_change_password" />
  <ForceOtpSetup v-else-if="auth.isAuthenticated && auth.mustSetupOtp" />
  <StepUpModal />
  <KonamiEgg />
  <EmbedEditor v-if="auth.isAuthenticated" />
  <FileViewerOverlay v-if="auth.isAuthenticated" />
</template>
