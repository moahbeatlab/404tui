---
name: feedback-show-keys-for-modes
description: 404tui: whenever a mode/option is activated, show which keys work in it on screen
metadata:
  type: feedback
---

When a mode or option is switched on in 404tui (select mode, cut cursor, sliders...), the keys that work in it must be visible on screen (the key list under the grid / a hint beside the slider), not only in the README or help.

**Why:** user could not find `5 6` / `7 8` for roll sliders and asked for hints; stated "that should be a general rule" (2026-10-15). They are fine with the key list at the bottom swapping to the mode's keys, no need to duplicate it inline.
**How to apply:** for every new mode, swap the help group to that mode's keys and keep the keys beside settings; add only what was asked ([[project_404tui_slicer_roadmap]]).
