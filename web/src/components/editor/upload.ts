import { Extension } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import type { Upload } from '@/lib/uploads'

// Files into a page in the visual editor: pasted, dropped, or picked with
// "/ Image or file" (editor.storage.fileUpload.pick()). Each is uploaded
// to the page, then goes in as an image or as a link to the file — in the
// Markdown, ![name](/api/v1/uploads/…) or [name](…).
export interface FileUploadOptions {
  upload: ((file: File) => Promise<Upload>) | null
  onError: (err: unknown) => void
}

export const FileUpload = Extension.create<FileUploadOptions, { pick: () => void }>({
  name: 'fileUpload',
  addOptions() {
    return { upload: null, onError: () => {} }
  },
  addStorage() {
    return { pick: () => {} }
  },
  onCreate() {
    const insert = async (files: File[], at: number | null) => {
      const upload = this.options.upload
      if (!upload) return
      let pos = at
      for (const file of files) {
        try {
          const u = await upload(file)
          const content = u.image
            ? { type: 'image', attrs: { src: u.url, alt: u.name } }
            : [{ type: 'text', text: u.name, marks: [{ type: 'link', attrs: { href: u.url } }] }, { type: 'text', text: ' ' }]
          const where = pos ?? this.editor.state.selection.to
          this.editor.chain().focus().insertContentAt(where, content).run()
          pos = null // the next one after this
        } catch (err) {
          this.options.onError(err)
        }
      }
    }
    this.storage.pick = () => {
      const input = document.createElement('input')
      input.type = 'file'
      input.multiple = true
      input.onchange = () => void insert([...(input.files ?? [])], null)
      input.click()
    }
    ;(this.storage as any).insert = insert
  },
  addProseMirrorPlugins() {
    const files = (list: FileList | null | undefined) => [...(list ?? [])]
    return [
      new Plugin({
        key: new PluginKey('fileUpload'),
        props: {
          handlePaste: (_view, event) => {
            const f = files(event.clipboardData?.files)
            if (!f.length || !this.options.upload) return false
            void (this.storage as any).insert(f, null)
            return true
          },
          handleDrop: (view, event) => {
            const f = files((event as DragEvent).dataTransfer?.files)
            if (!f.length || !this.options.upload) return false
            const at = view.posAtCoords({ left: (event as DragEvent).clientX, top: (event as DragEvent).clientY })
            void (this.storage as any).insert(f, at?.pos ?? null)
            return true
          },
        },
      }),
    ]
  },
})
