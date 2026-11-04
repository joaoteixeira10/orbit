# Orbit — Holiday Themes handoff

Everything Claude Code needs to add seasonal themes to Orbit's **Today** screen: Christmas, Halloween, New Year's Eve, Easter, Santos Populares and Lunar New Year. Each theme has a light and a dark variant.

## What's in this folder

| File | What it is |
|---|---|
| `README.md` | This file, plus the prompt to paste into Claude Code |
| `SPEC.md` | The full spec: architecture, token roles, decorations, motion, behaviour, acceptance checklist |
| `holiday-themes.ts` | Every value in one typed config: tokens (light + dark), SVG art, decoration and particle settings, copy, date windows, generators |
| `holiday-themes.css` | Keyframes and the base CSS-variable contract |
| `task-detail-dark-mode.md` | Bonus: the dark-mode token fix for the task detail sheet (design 5a) |
| `reference/*.dc.html` | The design source files. They are for reading values only and will not run outside the design canvas |

## How to use it

1. Copy this folder into the repo, for example at `docs/design/holiday-themes/`.
2. Open Claude Code in the repo and paste the prompt below.

---

### Prompt for Claude Code

```
Implement seasonal "holiday themes" for Orbit's Today screen, following the
handoff in docs/design/holiday-themes/ (read README.md, then SPEC.md, then
holiday-themes.ts and holiday-themes.css).

Before writing code:
1. Explore how theming works today (light/dark, CSS variables or theme
   objects, where the sidebar, Today card, Up Next card, quick-capture
   input, task row checkbox and Today tile are implemented). Report what
   you find and map each token role in SPEC.md §3 to the real component or
   variable it should drive.
2. Propose where the config (holiday-themes.ts) and the decoration
   components should live, in the project's conventions and language.

Then implement in this order, committing after each:
  a. Theme engine: resolve the active holiday (auto by date window, or a
     user override, or off) and apply its tokens on top of the current
     light/dark mode (SPEC §2, §3, §8).
  b. Static styling: colours, Today tile, logo, icon boxes, countdown chip,
     card-corner art, completed-task check, event bars (SPEC §4).
  c. Decorations layer: hanging decor (lights / drops / bunting) and the
     particle overlay, pointer-events: none, behind the
     effects toggle and prefers-reduced-motion (SPEC §5, §6).
  d. Settings: "Holiday themes: Auto / Off / <pick one>" and "Effects"
     (SPEC §8).
  e. Walk the acceptance checklist in SPEC §10 and report each item.

Keep values exactly as given in holiday-themes.ts; do not invent new colours.
Copy strings are English; route them through the app's i18n if it has one.
The task and event names in the reference designs are mock data, so do not ship them.
```

---

The design canvas these come from is the "Task detail redesign" project on claude.ai (pages: Christmas Mode, Halloween Mode, More Holidays). Share it with whoever reviews the implementation.
