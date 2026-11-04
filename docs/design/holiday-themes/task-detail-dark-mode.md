# Task detail sheet: dark-mode fix (design 5a)

The current dark task detail has a few problems:

- A black settings well under a grey header.
- A muddy brown due banner.
- A near-invisible Issue/PR icon.
- Dim placeholders.
- A disabled-looking Send button.

The fix is one surface colour for the whole sheet, slightly lighter cards on top of it, and tinted (not filled) status colours.

| Token | Light | Dark | Used for |
|---|---|---|---|
| backdrop | rgba(0,0,0,0.28) | rgba(0,0,0,0.55) | Scrim behind the sheet |
| modal | #fff | #1c1c1e | Right (activity) pane |
| side | #f2f2f7 | #1c1c1e | Left pane body (**not black**) |
| head | #fff | #1c1c1e | Left header + footer bar |
| card | #fff | #2c2c2e | Grouped setting cards, file card, comment box |
| fill | #f2f2f7 | #3a3a3c | Pills (date, assignee, tags) |
| line | #e5e5ea | rgba(255,255,255,0.09) | Pane borders |
| hair | #ececee | rgba(255,255,255,0.08) | Row dividers |
| hover | #fafafa | rgba(255,255,255,0.04) | Row hover |
| text | #1d1d1f | #f5f5f7 | Primary text |
| text2 | #3a3a3c | #e5e5ea | Notes, pill text |
| sub / muted | #6e6e73 / #86868b | #98989d | Section labels, meta |
| muted2 | #aeaeb2 | #8e8e93 | "Never", hints, placeholders |
| faint | #c7c7cc | #636366 | Chevrons, empty check ring, dashed + |
| link | #0a6cff | #4c9bff | "Add time", "Pick task", links |
| accent | #0a6cff | #0a84ff | Send / primary button (always full strength) |
| seg / segOn | #eeeef0 / #fff | rgba(118,118,128,0.24) / #636366 | Priority segmented control |
| quick / quickText | rgba(255,255,255,0.85) / #1d1d1f | rgba(255,255,255,0.1) / #f5f5f7 | Today / Tomorrow / Next week buttons |
| inputLine | #e3e3e6 | rgba(255,255,255,0.12) | Comment box border |
| chipOn / chipOnText | #1d1d1f / #fff | #f5f5f7 / #1c1c1e | Selected status/estimate chip |
| chipOff / chipLine | #fff / #e3e3e6 | transparent / rgba(255,255,255,0.14) | Unselected chip |
| switchOff | #e5e5ea | #39393d | Toggle off track |
| iconDark | #1d1d1f | #636366 | Issue/PR icon box (was invisible) |
| danger / dangerTint | #e5372b / #fdeceb | #ff453a / rgba(255,69,58,0.16) | Delete, PDF badge |
| late / lateBg | #c4271c / #fdeceb | #ff6961 / rgba(255,69,58,0.16) | Overdue banner |
| soon / soonBg | #a35c00 / #fff4e0 | #ffb340 / rgba(255,159,10,0.16) | Due today/tomorrow banner |
| later / laterBg | #0a5bd6 / #eef4ff | #64a8ff / rgba(10,132,255,0.18) | Due later banner |
| bubble | #f2f2f7 | #2c2c2e | Comment bubbles |

Also make sure:

- Textareas and inputs have `background: transparent; color: var(--text)`.
- Placeholders use `muted2`.
- The coloured icon squares keep their fills in both modes.
