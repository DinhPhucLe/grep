# Cortisol dashboard

This directory contains the frontend-only dashboard for the Vibe Coding Observatory. It currently renders a deterministic mock response and does not require the Go server or make any network requests.

The existing Go server remains responsible for collecting observations, calculating metrics, creating histogram bins, generating display text, and classifying code changes. The dashboard is responsible for validating the received response, arranging sections, applying the theme, and rendering generic visualization types.

## Run the mock dashboard

Prerequisites:

- Node.js 22.12 or newer
- npm

From the repository root:

```bash
cd dashboard
npm install
npm run dev
```

Open the URL printed by Vite, normally <http://localhost:5173>.

The page should show a complete session with significant metrics, descriptive numbers, a qualitative bullet summary, and a selectable three-file code heatmap. Use the sun/moon button in the header to switch themes.

Useful verification commands:

```bash
npm run typecheck
npm test
npm run build
npm run preview
```

## Change the mock input

The current input is [`src/mocks/dashboard.mock.json`](src/mocks/dashboard.mock.json). Edit that file and reload the page. The mock uses the same `DashboardSnapshot` shape expected from the Go server.

[`src/data/loadDashboard.ts`](src/data/loadDashboard.ts) is the only file that chooses the current data source. It parses the mock and passes the resulting snapshot to the dashboard. It does not calculate or reinterpret metrics.

Malformed input displays a startup error instead of partially rendering misleading values.

## Data expected from the Go server

The authoritative TypeScript contract is in [`src/contracts/dashboard.ts`](src/contracts/dashboard.ts). The root response has this shape:

```ts
interface DashboardSnapshot {
  schemaVersion: "dashboard.v1";
  sequence: number;
  generatedAt: string;
  session: {
    id: string;
    startedAt: string;
    observedThrough: string;
    state: "active" | "complete";
  };
  metrics: DashboardMetric[];
  summary: {
    status: "available" | "unknown";
    bullets: string[];
  };
  heatmap: CodeHeatmap;
}
```

All timestamps are ISO 8601 strings. Metric status must be one of `available`, `no_data`, `unknown`, or `invalid`. For an unavailable value, the server should send the appropriate status and a display value such as `—`; the client does not turn missing data into zero.

### Stat metric

Use a stat for a display-only scalar such as total plans or estimated active time:

```json
{
  "id": "total_plans",
  "category": "descriptive",
  "label": "Distinct plans",
  "status": "available",
  "display": {
    "primary": "3"
  },
  "visualization": {
    "type": "stat"
  }
}
```

### Ratio metric

The server sends the calculated ratio and all contextual counts:

```json
{
  "id": "acceptance_ratio",
  "category": "significant",
  "label": "Acceptance ratio",
  "description": "Explicitly accepted suggestions out of suggestions shown.",
  "status": "available",
  "display": {
    "primary": "65%",
    "secondary": "13 of 20 shown"
  },
  "visualization": {
    "type": "ratio",
    "value": 0.65,
    "minimum": 0,
    "maximum": 1,
    "segments": [
      { "label": "Accepted", "value": 13 },
      { "label": "Rejected", "value": 5 },
      { "label": "Unresolved", "value": 2 }
    ]
  }
}
```

### Histogram metric

The server calculates the median and bins. The frontend only draws them:

```json
{
  "id": "median_time_to_approval",
  "category": "significant",
  "label": "Median time to approval",
  "status": "available",
  "display": {
    "primary": "18.4s",
    "secondary": "13 accepted suggestions"
  },
  "visualization": {
    "type": "histogram",
    "median": {
      "value": 18400,
      "unit": "milliseconds"
    },
    "bins": [
      { "label": "0–5s", "value": 2 },
      { "label": "5–15s", "value": 4 },
      { "label": "15–30s", "value": 5 },
      { "label": "30s+", "value": 2 }
    ]
  }
}
```

The frontend selects a renderer from `visualization.type`. It never switches on IDs such as `acceptance_ratio`, so a new metric using an existing visualization type requires no new component.

### Summary

The server supplies the final bullet strings:

```json
{
  "status": "available",
  "bullets": [
    "The user supplied goals and expected behavior.",
    "The available telemetry cannot show whether the user inspected every retained edit."
  ]
}
```

The text must describe observable evidence and uncertainty. It must not diagnose comprehension or behavior.

### Code heatmap

The server supplies already-classified rows:

```json
{
  "mode": "history",
  "files": [
    {
      "path": "internal/metrics/acceptance.go",
      "rows": [
        {
          "rowId": "row-1",
          "oldLineNumber": null,
          "newLineNumber": 19,
          "content": "\tif shown == 0 {",
          "state": "retained_change",
          "churnCount": 0,
          "intensity": 0
        }
      ]
    }
  ]
}
```

Row states:

- `context`: unchanged surrounding code.
- `retained_change`: introduced or changed during the session and still present; rendered green.
- `deleted_or_replaced`: touched and later removed or replaced; rendered red.

`intensity` is `0` through `4`. For deleted/replaced rows, `1` is light red and `4` is the strongest red. When intermediate snapshots are unavailable, send `mode: "final_diff"`, use only intensity `1` for deleted rows, and do not invent repeated churn.

React renders `content` as escaped text; the field must contain plain code, not HTML.

## Connect the existing Go server later

Have the existing Go server return a `DashboardSnapshot` from a route such as `GET /api/v1/dashboard`. Then replace the body of `loadDashboard`:

```ts
export async function loadDashboard(): Promise<DashboardSnapshot> {
  const response = await fetch('/api/v1/dashboard');
  if (!response.ok) {
    throw new Error(`Dashboard request failed: ${response.status}`);
  }
  return parseDashboardSnapshot(await response.json());
}
```

During local development, configure the Vite proxy or enable the exact dashboard origin on the existing Go server. Do not add a second dashboard server.

A later WebSocket implementation should emit an initial full `DashboardSnapshot`. Subsequent update messages can replace metric, summary, or heatmap sections, but must produce the same final client-side snapshot contract. Static v1 intentionally has no WebSocket client or external state store.

## Responsibility boundary

The Go server owns:

- Raw observations and event correlation
- Metric formulas and null/unknown decisions
- Labels, descriptions, display values, and supporting counts
- Median and histogram-bin calculation
- Qualitative summary generation
- Baseline/history diffing and heatmap classification

The dashboard owns:

- Responsive section layout
- Generic stat, ratio, and histogram renderers
- Light/dark theme and visual tokens
- File selection and heatmap presentation
- Boundary validation and data-source loading

The dashboard must not calculate metrics from raw observations. Descriptive numbers and code-change evidence must not be used to calculate or imply a physiological cortisol level.
