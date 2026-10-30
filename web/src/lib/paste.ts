// What's pasted into a page from other apps, tidied before the editor reads
// it: Google Docs wraps everything in <b style="font-weight:normal"
// id="docs-internal-guid-…"> (which would make it all bold), Word and Excel
// add <o:p> and comments. Formatting itself — bold, italic, links, lists,
// tables — the editor keeps.
export function cleanPastedHTML(html: string): string {
  return html
    .replace(/<!--[\s\S]*?-->/g, '')
    .replace(/<\/?o:p>/gi, '')
    .replace(/<b\b[^>]*id="docs-internal-guid-[^"]*"[^>]*>([\s\S]*)<\/b>/i, '$1')
    .replace(/<meta[^>]*>/gi, '')
}
