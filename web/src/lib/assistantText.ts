// A link the model left unclosed ("[Shopping](/w/01a…" to the end of the
// line) shows as its words, not as an address full of ids.
export function tidy(text: string): string {
  return text.replace(/\[([^\]\n]+)\]\(\/[^)\s]*(?=\s|$)/g, '$1')
}
