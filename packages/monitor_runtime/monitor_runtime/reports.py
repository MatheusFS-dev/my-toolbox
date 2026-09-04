"""Run report persistence and deterministic resource plots."""

import csv
import json
from pathlib import Path
from typing import Any, Dict, Iterable, List


def write_reports(run_directory: Path, samples: List[Dict[str, Any]], lifecycle: List[Dict[str, Any]], summary: Dict[str, Any]) -> None:
    """Write machine-readable samples/lifecycle data and JSON/text summaries."""
    directory = Path(run_directory)
    fields = ["elapsed_seconds", "cpu_percent", "ram_mib", "gpu_percent", "gpu_memory_mib"]
    with (directory / "samples.csv").open("w", newline="", encoding="utf-8") as stream:
        writer = csv.DictWriter(stream, fieldnames=fields, extrasaction="ignore")
        writer.writeheader()
        writer.writerows(samples)
    (directory / "lifecycle.jsonl").write_text("".join(json.dumps(item, ensure_ascii=False, sort_keys=True) + "\n" for item in lifecycle), encoding="utf-8")
    (directory / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    (directory / "summary.txt").write_text("\n".join("{}: {}".format(key, value) for key, value in sorted(summary.items())) + "\n", encoding="utf-8")
    if samples:
        _write_plots(directory, samples)


def _write_plots(directory: Path, samples: List[Dict[str, Any]]) -> None:
    try:
        import matplotlib
        matplotlib.use("Agg")
        import matplotlib.pyplot as plt
    except ImportError:
        return
    palette = {"cpu": "#557a95", "ram": "#718355", "gpu": "#8d6a9f"}
    plots = [
        ("cpu_percent", "CPU usage", "CPU (%)", "cpu.png", palette["cpu"]),
        ("ram_mib", "Memory usage", "RAM (MiB)", "ram.png", palette["ram"]),
        ("gpu_percent", "GPU usage", "GPU (%)", "gpu.png", palette["gpu"]),
    ]
    x = [sample["elapsed_seconds"] for sample in samples]
    for field, title, ylabel, filename, color in plots:
        values = [sample.get(field) for sample in samples]
        if all(value is None for value in values):
            continue
        figure, axis = plt.subplots(figsize=(5.25, 3.0), facecolor="white")
        axis.set_facecolor("white")
        axis.plot(x, values, color=color, linewidth=1.4)
        axis.set_title(title, fontfamily="DejaVu Serif", fontsize=9)
        axis.set_xlabel("Elapsed time (s)", fontfamily="DejaVu Serif", fontsize=8)
        axis.set_ylabel(ylabel, fontfamily="DejaVu Serif", fontsize=8)
        axis.tick_params(labelsize=7)
        axis.grid(axis="y", linestyle="--", linewidth=0.5, alpha=0.35)
        axis.spines["top"].set_visible(False)
        axis.spines["right"].set_visible(False)
        axis.spines["left"].set_linewidth(0.7)
        axis.spines["bottom"].set_linewidth(0.7)
        figure.tight_layout()
        figure.savefig(str(directory / filename), dpi=180, bbox_inches="tight", facecolor="white")
        plt.close(figure)
