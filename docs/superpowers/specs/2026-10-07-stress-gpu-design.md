# NVIDIA GPU stress application

## Approved intent

Add an installable application to my-toolbox for stressing selected NVIDIA GPUs concurrently. Provide interactive selection, relative load defaulting to 100%, timed or continuous operation, an updating terminal monitor, and an end-of-run report with optional saved logs. The installer appears in tb list, like install-monitor, and creates an isolated virtual environment.

## Entry points and installation

Register `install-stress-gpu` under System Utilities with list visibility. The installed standalone command is `stress-gpu`. Support native Linux and WSL, plus Windows x64 where a compatible CUDA-enabled PyTorch distribution is available. Linux ARM64 must validate distribution and CUDA support and return actionable errors when unavailable. Require a Python version supported by the selected PyTorch distribution. Do not install or replace NVIDIA drivers or a system CUDA toolkit.

Package application sources and dependency metadata with tb. Install into a dedicated current-user directory and virtual environment. Stage and validate installation before activation. Preserve a working installation on failure, refuse unrelated wrapper/runtime conflicts, and allow repair by rerunning the installer. The application remains installed after tb uninstall, matching Monitor's independence. Do not modify Monitor's implementation or configuration. Document rerunning the installer to update this application.

## Interactive configuration

Enumerate available NVIDIA CUDA devices with stable UUID identity and show names, total/free memory, and available temperature data. Respect CUDA visibility restrictions and map selected CUDA devices to telemetry by UUID rather than assuming index equality. Reject missing GPUs, duplicate selections, and invalid input. Select one or several GPUs, a shared relative load from 1% to 100% inclusive (default 100%), and a positive duration or continuous operation until Ctrl+C. No argument parser is required.

Load controls both the compute duty cycle and the proportion of initially available VRAM allocated. At 100%, workers run continuously and allocate as much initially free memory as practicable, leaving only necessary runtime/workspace headroom. At lower loads, use measured work duration to pace compute, and scale the memory target. Show requested and measured values separately. Never claim exact utilization or total VRAM occupancy is guaranteed.

## Workload and monitoring

Use one independent spawned worker per selected GPU and CUDA-enabled PyTorch. Adapt matrix dimensions to device resources, use sustained matrix multiplication for compute stress, and repeatedly touch the additional memory allocation so the test exercises allocated VRAM. Reduce allocation after CUDA out-of-memory failures, with a lower bound and a clear error if no usable workload fits. Initialize allocations before starting the measured shared test duration. Keep worker startup bounded and communicate readiness, progress, and errors to the parent.

Refresh the dashboard in place approximately once per second. Display each selected GPU's utilization, used/total VRAM, temperature, power, clocks, available throttling indicators, worker state, target load, and elapsed/remaining time. Unsupported telemetry is unavailable, never zero. Temperature analysis uses measured trends and device-reported thresholds where supported. A threshold or thermal-throttle observation is reported as such, not interpreted as a universal pass/fail hardware diagnosis. Do not alter clocks, voltage, fan settings, or power limits.

## Lifecycle, reporting, and logs

Stop on duration expiry or Ctrl+C. Worker failure or loss of monitoring produces an explicit incomplete/error result and stops all workers. Bound shutdown and terminate unresponsive children, restore the terminal, release worker-owned GPU allocations, and preserve collected observations. Handle interruption during initialization too.

Maintain online summary statistics and spool raw samples to a temporary file so indefinite runs do not accumulate unbounded RAM. Final reports include run configuration, stop reason, elapsed duration, per-GPU measured utilization/memory/temperature/power averages and peaks, temperature trend, threshold/throttle observations, and worker errors. Reports describe achieved stress, not proof that a GPU has no hardware faults.

After showing the report, ask whether to save logs. Only if accepted, ask for a destination directory, expand user-relative paths, validate it, and save a readable report plus structured samples. Ask before overwriting existing output. Save failures leave the displayed report available and permit another path. No logs are committed to the repository.

## Verification and release

Use dependency-injected/fake CUDA and telemetry for deterministic CPU-only tests of discovery/mapping, invalid input, memory sizing, out-of-memory recovery, duty-cycle pacing, simultaneous worker readiness, timed/manual stop, worker failure, bounded shutdown, missing telemetry, terminal restoration, summary statistics, and log export. Test installer isolation, ownership conflicts, failure rollback, repair, catalog listing, and platform behavior. Run relevant Go/Python suites, cross-platform builds, and release checks. If no NVIDIA GPU is available, clearly state that actual sustained CUDA load was not measured.

Publish the verified exact main commit through the existing Release workflow, which builds native readers, determines the next version, and publishes release archives. Verify workflow completion and release assets before claiming publication.
