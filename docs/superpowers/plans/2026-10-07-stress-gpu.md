# NVIDIA Stress Application Implementation Plan

> For agentic workers: Use superpowers:executing-plans for native execution or superpowers:subagent-driven-development for delegated execution. Track steps with checkboxes.

Goal: Install a standalone interactive NVIDIA stress application from tb list, test it, and publish a verified tb release.

Architecture: A packaged Python runtime owns interactive configuration, CUDA workers, NVML telemetry, dashboard, and reports. A tb builtin installer stages an isolated environment and publishes a stable standalone wrapper. Spawned workers use a shared start gate and stop event.

Tech Stack: Existing Go toolbox, Python, CUDA-enabled PyTorch, NVIDIA NVML through nvidia-ml-py, Python unittest.

Spec: ../specs/2026-10-07-stress-gpu-design.md

## Global Constraints

- Installer: `install-stress-gpu`, System Utilities, visible in tb list. Application: `stress-gpu`.
- Load: 1% to 100% inclusive, default 100%, controlling compute duty cycle and initially available VRAM target.
- NVIDIA only. Respect CUDA visibility and identify telemetry by UUID.
- Native Linux, WSL, and Windows x64. Linux ARM64 requires a compatible CUDA distribution and actionable failure otherwise.
- Require a Python version supported by the selected PyTorch distribution. Verify supported versions and dependency APIs using official sources before choosing exact pins.
- Isolated current-user environment. No driver/toolkit installation, no changes to Monitor, and no argument parser.
- Preserve old installation on failure and refuse unrelated ownership conflicts.
- Timed runs begin after all selected workers are ready. Continuous runs stop with Ctrl+C.
- Approximately one dashboard refresh per second. Missing metrics are unavailable.
- Bounded shutdown, restored terminal, explicit errors, online statistics, disk-spooled raw samples.
- Optional report/sample export after the final report, with destination and overwrite prompts.
- Complete Google-style docstrings for all Python functions/methods, rationale comments, no future annotations import.

## Review Focus

1. CUDA device order differs from NVML order, especially visibility subsets. UUID mapping must remain correct.
2. A worker fails or hangs before readiness. Startup must time out and stop the entire run.
3. Available memory changes during allocation. OOM retries must reduce allocations without hiding other CUDA failures.
4. Ctrl+C arrives during startup or log export. Processes and terminal state must be cleaned up.
5. A previous installation or output path is unrelated or unwritable. Preserve it and allow repair/retry without data loss.

## Task 1: Discovery, configuration, and telemetry

Files: Create `packages/stress_gpu_runtime/stress_gpu_runtime/{__init__,devices,config}.py`, `packages/stress_gpu_runtime/tests/test_devices_config.py`, and dependency manifest.

Interfaces: `discover_devices(torch_module, nvml_module) -> list[dict]` returns CUDA ordinal, UUID, name, free/total memory, and device thresholds. `sample_device(nvml_module, uuid) -> dict` returns nullable measured metrics. `prompt_configuration(devices, input_fn) -> dict` returns selected device dictionaries, load_percent, and duration_seconds (None for continuous).

- [x] Write tests asserting reordered UUID mapping, visibility subsets, no CUDA, unsupported metrics, duplicate/out-of-range selections, finite positive duration, rejected NaN/infinity, and default load 100.
- [x] Run `PYTHONPATH=packages/stress_gpu_runtime python3 -m unittest discover -s packages/stress_gpu_runtime/tests -v`, expect missing implementation failures.
- [x] Verify official PyTorch installation/API and NVIDIA telemetry documentation. Implement the interfaces with lazy imports, input retries, and explicit dependency/device errors.
- [x] Repeat the command, expect these tests to pass. Commit discovery/configuration and dependency choices.

## Task 2: Adaptive workload and concurrent lifecycle

Files: Create `packages/stress_gpu_runtime/stress_gpu_runtime/{workload,session}.py`, `packages/stress_gpu_runtime/tests/test_workload_session.py`.

Interfaces: `allocate_workload(torch_module, device, load_percent) -> tuple` returns matrices and extra memory chunks. `run_worker(device, load_percent, start_event, stop_event, messages) -> None` emits ready/error/progress messages. `run_session(config, sampler, on_sample, process_factory=None) -> dict` manages workers and returns stop reason, elapsed seconds, and errors. Production defaults use the spawn multiprocessing context.

- [x] Write fake-CUDA tests for proportional memory budgets, matrix sizing, OOM backoff, exhausted memory, non-OOM propagation, touched memory, and duty-cycle pacing at 25% and 100%.
- [x] Write fake-process lifecycle tests for concurrent readiness, duration starting after readiness, startup timeout, interruption, worker error, worker disappearance, missing telemetry, and forced termination of an unresponsive worker.
- [x] Run the Python suite, verify failures for missing workload/session behavior.
- [x] Implement adaptive allocations with explicit runtime headroom and bounded retries. Run repeated matmul and chunk operations, synchronize before pacing, and use interruptible sleeps. Gate workload start until all workers are ready. Use monotonic deadlines and bounded join/terminate/kill cleanup.
- [x] Run the suite, expect all lifecycle/workload tests to pass. Commit workload and session.

## Task 3: Dashboard, report, and optional export

Files: Create `packages/stress_gpu_runtime/stress_gpu_runtime/{reporting,__main__}.py`, `packages/stress_gpu_runtime/tests/test_reporting_cli.py`.

Interfaces: `summarize_sample(state, sample) -> None` updates online aggregates. `render_dashboard(config, state, elapsed) -> str` produces target/measured rows. `format_report(config, result, state) -> str` produces a readable final report. `save_results(directory, report, sample_file, overwrite=False) -> list[str]` exports report.txt and samples.jsonl. `main() -> int` wires prompts, discovery, session, report, and export.

- [x] Write tests for averages/peaks with missing values, temperature trend/threshold observations, target versus achieved load, in-place rendering, redirected output, cursor restoration after error/interruption, and bounded-memory spooling.
- [x] Write export tests for declined saving, path asked only after acceptance, tilde/relative paths, overwrite refusal, unwritable path retry, and interrupted export.
- [x] Run the Python suite and verify the new tests fail.
- [x] Implement online state and TemporaryFile JSONL samples, terminal-aware ANSI refresh/cleanup, final error/stop reporting, and transactional export with overwrite confirmation. Add `__main__` with multiprocessing-safe entry point.
- [x] Run the suite and a mocked end-to-end timed multi-GPU session. Commit UI/reporting.

## Task 4: Installer and tb integration

Files: Create `src/stress_gpu_install.go`, `src/stress_gpu_install_test.go`. Modify `src/installers.go`, `commands.json`, `src/catalog_test.go`, `src/app_test.go`, `src/testdata/help-*.txt`, `README.md`, `.github/workflows/ci.yml`, `.github/workflows/release.yml`, and release-payload tests if required by existing packaging.

Interfaces: `(*ToolboxBuiltins).installStressGPU() error` implements installation. Helper functions resolve platform paths, wrappers, ownership checks, dependency installation and runtime validation. Reuse existing toolbox helpers where appropriate without modifying Monitor behavior.

- [x] Write Go tests for list/catalog/dispatch visibility on supported environments, isolated venv creation, owned wrapper publication, existing installation repair, incompatible Python/CUDA dependency errors, unrelated ownership refusal, and rollback after dependency/self-check/activation failures.
- [x] Run `go test ./src`, verify new installer/integration failures.
- [x] Add platform-specific Python resolution, private staging, pinned dependency installation, GPU-independent package self-check plus actionable CUDA runtime guidance, ownership markers, rollback, Linux wrapper and Windows wrapper. Avoid venv relocation assumptions by invoking its interpreter directly and validate again after publication.
- [x] Register installer as builtin with list visibility and platform requirements. Update exact catalog/help/completion expectations. Include Python tests in CI/release verification and ensure application sources are included in release payloads.
- [x] Document installation, runtime prerequisites, load semantics, metrics limitations, repair/update, paths, and test commands.
- [x] Run Go/Python suites and relevant installer/release-payload checks, expect pass. Commit installer/integration.

## Task 5: Review, verification, and release

Files: Update implementation-plan checkboxes and any defects identified by verification. No unrelated refactoring.

- [ ] Read verification-before-completion and requesting-code-review skills. Review spec coverage, Python docstrings, worker cleanup, memory target semantics, runtime dependency compatibility, and installer rollback. Use selected execution method's required independent review and resolve material findings.
- [ ] Run `go test ./...`, `go test -race ./...`, the new Python suite, Python compilation, existing relevant Python suites, `sh scripts/build-release_test.sh`, `sh scripts/install_test.sh`, and existing release/version checks. Build Linux amd64/arm64 and Windows amd64 with CGO disabled. Record any environment limitation without claiming skipped checks passed.
- [ ] Probe NVIDIA access. If available, run short real single/multi-GPU tests at partial and full load and observe resource release. If unavailable, explicitly record hardware validation as unperformed.
- [ ] Check clean diff/status and main remote head. Push verified changes through authorized GitHub/git capabilities without overwriting intervening work. Dispatch existing Release workflow with exact main SHA.
- [ ] Monitor workflow completion, inspect failure logs if needed, and resolve failures before retrying. Verify published version, exact target commit, platform archives, checksums, and version.txt.
- [ ] Return release link, installation/application commands, verification results, and actual CUDA testing limits.
