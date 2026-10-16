import { CodeBlockLowlight } from '@tiptap/extension-code-block-lowlight'
import { VueNodeViewRenderer } from '@tiptap/vue-3'
import CodeBlockView from './CodeBlockView.vue'
import { lowlight } from '@/lib/syntax'

// A code block, highlighted as in the reading view (lib/syntax.ts), with a
// language picker. Its Markdown is the fence (```go), as before.
export const CodeBlock = CodeBlockLowlight.extend({
  addNodeView() {
    return VueNodeViewRenderer(CodeBlockView)
  },
}).configure({ lowlight, defaultLanguage: null })
