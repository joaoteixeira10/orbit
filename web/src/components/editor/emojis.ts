// A small, curated emoji set for page icons and callouts: common enough that
// most pages find one, small enough to skip a picker library.
export const EMOJIS = [
  '📄', '📝', '📌', '📎', '📚', '📖', '📓', '🗂️', '🗓️', '📅', '⏰', '✅',
  '☑️', '📋', '📊', '📈', '💡', '🧠', '🎯', '🚀', '⭐', '✨', '🔥', '⚡',
  '❤️', '💜', '💙', '💚', '💛', '🧡', '🏠', '🏢', '🏫', '🎓', '💼', '💻',
  '🖥️', '📱', '⚙️', '🔧', '🧪', '🔬', '🧮', '💰', '🛒', '🎁', '🍎', '🍕',
  '☕', '🍳', '🥗', '🏃', '🏋️', '🧘', '🚲', '⚽', '🎵', '🎨', '📷', '🎬',
  '🎮', '✈️', '🌍', '🏖️', '⛰️', '🌱', '🌸', '🌙', '☀️', '🌈', '🐶', '🐱',
  '👋', '🙂', '😎', '🤔', '🎉', '🏆', '🔒', '🔑', '❗', '❓', '⚠️', '💬',
]

export const randomEmoji = () => EMOJIS[Math.floor(Math.random() * EMOJIS.length)]
