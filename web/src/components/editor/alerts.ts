// A callout is one of GitHub's five alerts (how it's written in Markdown:
// > [!TIP]); its emoji says which.
export const ALERTS = { NOTE: 'ℹ️', TIP: '💡', IMPORTANT: '❗', WARNING: '⚠️', CAUTION: '🛑' } as const
export type AlertType = keyof typeof ALERTS
export function alertOf(emoji: string): AlertType {
  return (Object.keys(ALERTS) as AlertType[]).find((k) => ALERTS[k] === emoji) ?? 'NOTE'
}
