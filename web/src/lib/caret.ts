// caretRect is where character `at` of a text area is on screen: a hidden
// copy of the area, with the same text and style, measured up to it.
export function caretRect(el: HTMLTextAreaElement, at: number): DOMRect {
  const cs = getComputedStyle(el)
  const mirror = document.createElement('div')
  for (const p of ['boxSizing', 'width', 'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft', 'borderTopWidth', 'borderRightWidth', 'borderBottomWidth', 'borderLeftWidth',
    'fontFamily', 'fontSize', 'fontWeight', 'fontStyle', 'letterSpacing', 'lineHeight', 'tabSize', 'textTransform', 'wordSpacing', 'textIndent'] as const) {
    mirror.style[p] = cs[p]
  }
  Object.assign(mirror.style, { position: 'absolute', visibility: 'hidden', whiteSpace: 'pre-wrap', overflowWrap: 'break-word', top: '0', left: '-9999px' })
  mirror.textContent = el.value.slice(0, at)
  const mark = document.createElement('span')
  mark.textContent = '​'
  mirror.append(mark)
  document.body.append(mirror)
  const box = el.getBoundingClientRect()
  const top = box.top + mark.offsetTop - el.scrollTop + parseFloat(cs.borderTopWidth || '0')
  const left = box.left + mark.offsetLeft - el.scrollLeft
  const h = mark.offsetHeight || parseFloat(cs.lineHeight) || 18
  mirror.remove()
  return new DOMRect(left, top, 1, h)
}
