# STYLES.md

## 1. Core Aesthetic Directives
**Style Identity:** 1-Bit Retro Terminal & Neo-Brutalism.
**General Vibe:** Chunky, aliased, high-contrast, mechanical, and slightly aggressive. 
**Agent Instruction:** When generating React components, strictly avoid modern web design tropes. NO soft gradients, NO blurred drop shadows, NO rounded corners (`border-radius: 0`), and NO anti-aliased (smooth) geometric shapes. Everything must look like it was rendered on a low-resolution CRT monitor or early GUI operating system.

## 2. Global CSS & Theming Variables
When configuring the `@mantine/core` `MantineProvider`, use the following strict overrides:

* **Borders:** Extremely bold. All interactive elements, cards, and containers must have a minimum `2px solid #000000` border.
* **Corners:** Sharp. Set Mantine global `defaultRadius` to `0`. 
* **Shadows:** Hard offsets only. Replace all soft shadows with solid, brutalist offset shadows. 
  * *Example:* `box-shadow: 4px 4px 0px 0px #000000;`
* **Image Rendering:** Apply `image-rendering: pixelated;` to all canvas, image, and graphic elements to force hard, chunky edges.

## 3. Color Palette
The dashboard relies on a strict, limited palette to emulate 1-bit or low-color hardware. 

* **Primary Background:** `#FFFFFF` (Stark White) or `#F4F4F0` (Off-white paper)
* **Primary Foreground/Lines:** `#000000` (Pitch Black)
* **Cortisol Animation/Accent:** `#FF4E4E` (Terminal Red/Orange) - Use exclusively for the primary cortisol metrics and alerts.
* **Secondary Accents (for charts/tags):** `#A3E6D2` (Mint), `#B8C0FF` (CGA Blue), `#FFD166` (Retro Yellow). 
* **Agent Instruction:** Do not use shades of gray for borders or text. Use pure black (`#000`) to maintain the high-contrast 1-bit aesthetic.

## 4. Typography
* **Primary Font:** Use a monospace or pixelated font. Import a Google Font such as `VT323`, `Press Start 2P`, or `Space Mono`.
* **Font Weights:** Stick to normal (400) or extremely bold (700). Avoid thin or medium weights.
* **Letter Spacing:** Slightly tracked out (`letter-spacing: -0.05em` for blocky fonts, or `1px` for standard monospace) to emulate terminal spacing.
* **Text Transform:** Use `UPPERCASE` liberally for headers, labels, and table columns.

## 5. Component-Specific Styling Rules (Mantine)
Provide these exact overrides to the AI agent when generating Mantine components:

* **Cards / Paper (`<Paper>`, `<Card>`):** Must have a white background, `2px solid black` border, and a `4px 4px 0px black` box shadow.
* **Buttons (`<Button>`):** 
  * Background: Transparent or solid bold accent color.
  * Border: `2px solid black`.
  * Hover state: Invert colors (background becomes black, text becomes white) OR shift the hard shadow from `4px` to `2px` with a `translate(2px, 2px)` transform to mimic a mechanical button press.
* **Inputs (`<TextInput>`, `<Select>`):** Thick black borders. The focused state should not have a glowing ring; instead, it should invert the background color or thicken the border to `4px`.
* **Tables:** `<Table>` must have grid lines enabled (`withTableBorders`, `withColumnBorders`). Borders must be thick and black. Headers must be uppercase and bold.

## 6. Dashboard & Data Visualization (Recharts / Mantine Charts)
Because the project involves real-time metrics, standard chart smoothing will ruin the retro aesthetic. 

* **Line Charts (`<LineChart>`):** 
  * Set Recharts `<Line type="step"/>` or `<Line type="stepAfter"/>` to create blocky, digital step-graphs instead of smooth bezier curves.
  * If using `type="linear"`, set `strokeWidth={3}` or `{4}` for bold lines. Do not use `monotone`.
* **Bar Charts (`<BarChart>`):** 
  * Bars must have a `stroke="black"` with `strokeWidth={2}`.
  * Do not use border radiuses on the bars.
* **Grids:** Use `<CartesianGrid stroke="#000" strokeDasharray="3 3"/>` to create a dithered/pixelated background grid for the charts.
* **Tooltips:** Override the Recharts `<Tooltip>` content with a custom component that mimics a retro pop-up window (square corners, black borders, hard shadow).

## 7. The "Cortisol" Block Animation
* **Implementation:** When building the cortisol animation component, use an HTML `<canvas>` or a grid of `<div>` elements heavily utilizing CSS Grid. 
* **Styling:** Treat each "block" of the cortisol visual as a literal terminal character or oversized pixel. Use the Terminal Red (`#FF4E4E`) to fill the blocks, keeping the `2px` black grid lines visible between them to emphasize the low-resolution, blocky nature.