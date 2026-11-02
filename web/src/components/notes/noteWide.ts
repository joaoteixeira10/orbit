// Whether quick notes open big (wide, with room to write), remembered on this
// device; the capture box and the editor share it.
import { ref, watch } from 'vue'

const KEY = 'orbit_note_wide'
function read(): boolean {
  try {
    return localStorage.getItem(KEY) === '1'
  } catch {
    return false
  }
}
export const noteWide = ref(read())
watch(noteWide, (v) => {
  try {
    localStorage.setItem(KEY, v ? '1' : '0')
  } catch {
    /* private mode: for this visit only */
  }
})
