# 8. Terminal UI (TUI) & Text-Based Rendering

**Agent Instruction:** When generating layouts for the text-based terminal interface, strictly use monospaced layout math (padding via space characters) and Unicode box-drawing characters. Do not attempt to use HTML/CSS borders for structural terminal elements; build the structure out of text.

## Layout & Borders (Heavy Box Drawing)

To maintain the bold, brutalist aesthetic in text, exclusively use the Heavy Unicode box-drawing characters. Avoid the light/standard line characters.

* **Corners:** ┏ (Top-Left), ┓ (Top-Right), ┗ (Bottom-Left), ┛ (Bottom-Right)
* **Edges:** ━ (Horizontal), ┃ (Vertical)
* **Intersections:** ┣, ┫, ┳, ┻, ╋

### Example Panel:

```text
┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃ METRICS DASHBOARD                   ┃
┣━━━━━━━━━━━━━━━━━━━━━━━━┳━━━━━━━━━━━━┫
┃ STATUS: ACTIVE         ┃ NODE: 01   ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━┻━━━━━━━━━━━━┛
```

## Block Elements & "Pixels"

Treat Unicode block characters as your literal pixels for rendering the "Cortisol" animation and progress bars. By stacking these, you create the chunky, low-res look.

* **Full Block:** █ (Use for solid bars and the core cortisol visual)
* **Shading Blocks:** ▓ (Dark shade), ▒ (Medium shade), ░ (Light shade/background grids)
* **Half Blocks:** ▀ (Upper half), ▄ (Lower half) — useful for doubling vertical resolution within a single character line.

## Terminal UI Components

* **Command Prompt:** Keep it mechanical and stark. Use a classic shell prompt structure, e.g., `vibe-coding:~$` or simply `>_`. The cursor should be a solid block (█), never a blinking vertical line.
* **Buttons / Selectable Items:**
  * **Inactive:** Encased in hard brackets `[ DEPLOY ]` or angled brackets `> DEPLOY <`.
  * **Active/Hover:** Achieved strictly through color inversion (swap the foreground and background ANSI colors).
* **Progress Bars:** Construct using heavy brackets and block characters.
  ```text
  [████████░░░░░░░░] 50%
  ```

## Color Palette (ANSI Mapping)

Standard terminal styling relies on ANSI escape codes. Map the project's color palette to these constraints to maintain the high-contrast aesthetic:

* **Terminal Background:** White (`\x1b[47m`) or Off-White.
* **Primary Text/Lines:** Black (`\x1b[30m`).
* **Cortisol Accent:** Red (`\x1b[31m`) for critical metrics and the active block animation.
* **Focus State / Selection:** Invert colors to Black Background (`\x1b[40m`) and White Text (`\x1b[37m`).

## The Cortisol Animation (Text Implementation)

When rendering the cortisol animation via standard output stream, use an array of █ characters colored with the red accent hex. To simulate the "blocks" updating, redraw the specific lines in the terminal buffer (using ANSI cursor positioning like `\x1b[<r>;<c>H`) rather than printing new lines continuously. Use ░ or space characters for the empty grid areas to emphasize the step-by-step, mechanical nature of the metric changes.
