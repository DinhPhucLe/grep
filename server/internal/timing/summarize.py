"""Print stage timings from TUI JSONL and server log files (stdlib only)."""

import argparse
import json
from collections import defaultdict


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("logs", nargs="+")
    parser.add_argument("--trace", help="Show only this prompt trace ID")
    args = parser.parse_args()
    traces = defaultdict(list)
    for filename in args.logs:
        with open(filename, encoding="utf-8") as stream:
            for line in stream:
                if "timing {" in line:
                    line = line.split("timing ", 1)[1]
                try:
                    event = json.loads(line)
                except (ValueError, TypeError):
                    continue
                if not isinstance(event, dict) or not all(
                    key in event for key in ("trace_id", "stage", "duration_ms", "time")
                ):
                    continue
                if args.trace and event["trace_id"] != args.trace:
                    continue
                traces[event["trace_id"]].append(event)
    if not traces:
        print("No timing events found. Enable --timing-log and submit a prompt.")
        return
    for trace, events in traces.items():
        print(f"\nPrompt trace: {trace}")
        print(f"{'Request':8}  {'Operation':14}  {'Stage':28}  {'ms':>11}  Outcome / counts")
        for event in sorted(events, key=lambda item: item["time"]):
            print(
                f"{event.get('request_id', '')[:8]:8}  "
                f"{event.get('operation', ''):14}  {event['stage']:28}  "
                f"{event['duration_ms']:11.3f}  {event.get('outcome', '')} "
                + " ".join(f"{key}={value}" for key, value in event.get("counts", {}).items())
            )
    print("\nSpans overlap: do not sum all rows. See internal/timing/README.md.")


if __name__ == "__main__":
    main()
