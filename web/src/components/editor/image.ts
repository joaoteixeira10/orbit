import { Image } from '@tiptap/extension-image'
import { i18n } from '@/i18n'
import { remoteHost, reveal, safeImageSrc, showsAtOnce } from '@/lib/media'
import { isUploadSrc, uploadBlobUrl } from '@/lib/uploads'

// A page's image in the visual editor. One from another site waits behind
// the same placeholder as in the reading view (lib/media.ts) until clicked;
// the Markdown is unchanged either way.
export const PageImage = Image.extend({
  addNodeView() {
    return ({ node }) => {
      const dom = document.createElement('span')
      dom.className = 'pm-image'
      let current = node
      const render = () => {
        dom.replaceChildren()
        const src = String(current.attrs.src ?? '')
        if (isUploadSrc(src)) {
          // Uploaded to Orbit: fetched with the sign-in token.
          const img = document.createElement('img')
          img.alt = current.attrs.alt ?? ''
          void uploadBlobUrl(src).then((url) => (img.src = url)).catch(() => img.classList.add('broken'))
          dom.append(img)
          return
        }
        if (showsAtOnce(src)) {
          const img = document.createElement('img')
          img.src = safeImageSrc(src)
          img.alt = current.attrs.alt ?? ''
          if (current.attrs.title) img.title = current.attrs.title
          img.referrerPolicy = 'no-referrer'
          dom.append(img)
          return
        }
        const ph = document.createElement('span')
        ph.className = 'md-remote-img'
        ph.contentEditable = 'false'
        ph.textContent = i18n.global.t('editor.remoteImage', { host: remoteHost(src) })
        ph.addEventListener('mousedown', (e) => {
          e.preventDefault()
          reveal(src)
          render()
        })
        dom.append(ph)
      }
      render()
      return {
        dom,
        update(n) {
          if (n.type !== current.type) return false
          current = n
          render()
          return true
        },
      }
    }
  },
})
