---
name: landman-heap-profile
description: Capture and compare Go heap profiles for slow or memory-heavy Landman ingestion and API workloads. Use Delve for local stack inspection when a heap profile does not explain the symptom.
---

# Landman heap profiles

A heap profile samples Go allocations by call stack. It does not contain a full copy of process memory. Use Go `pprof` to capture profiles without stopping the process with a debugger. Use Delve to inspect a stopped process, not to measure heap size or latency.

## A. Start a local target

Use a Unix host with Go and curl. Use an approved test database and approved inputs. Do not start a live county import or API load test without approval. If a process did not start with profiling enabled, ask before you restart it. Set `LANDMAN_HEAP_PROFILE_DIR` for the target process only. Do not put it in `.env`.

From the repository root, create a private directory and use a normal build. Set the usual application environment first. These commands do not load `.env`.

```sh
run_dir=$(mktemp -d)
go build -o "$run_dir/api" ./cmd/api
go build -o "$run_dir/landman" ./cmd/landman
```

Start one target. The ingestion example reads stored data and requires a prepared database. Replace the job with the job under test.

```sh
LANDMAN_HEAP_PROFILE_DIR="$run_dir" "$run_dir/api"
# Or:
LANDMAN_HEAP_PROFILE_DIR="$run_dir" "$run_dir/landman" run union.qualification.assess
```

Record the socket path and PID from `Heap profile socket` in the startup log. Use that path, not a socket from a previous run. The application creates one private Unix socket per process. It does not serve profiles on the API port. Keep the target running during capture.

## B. Capture and compare

Set `socket` to the logged path. Run from the repository root. The command prints a path only after it captures and validates the profile. It puts the profile in a new private temporary directory.

```sh
capture=.agents/skills/landman-heap-profile/scripts/capture.sh
baseline=$(sh "$capture" "$socket")
# Run the approved workload. Capture again while the symptom occurs.
symptom=$(sh "$capture" "$socket")
go tool pprof -top -inuse_space -base "$baseline" "$symptom"
go tool pprof -top -alloc_space -base "$baseline" "$symptom"
```

- **B1. Retained memory:** `inuse_space` shows bytes in live objects as of the most recent completed garbage collection. Capture again after the workload is idle to check whether objects remain. This endpoint does not force a collection.
- **B2. Allocation volume:** `alloc_space` shows cumulative allocated bytes, including objects that Go has freed. Compare samples from the same process to estimate the bytes allocated between captures.
- **B3. Limits:** A heap profile does not measure total process memory, database memory, or query time. Use process memory measurements or query timings if the profile does not explain the symptom.

Report the Go version, build revision, capture times, and workload. For ingestion, include the job ID, attempt ID, and stage from the logs. For the API, include the route, request count, and concurrency. If you miss the relevant stage, report that limit and use a new approved run. Keep profiles private. Remove the profiles and their temporary directories when no longer required. Stop the test process normally. It removes its socket directory. Remove the build directory when the test is complete.

## C. Inspect local stacks with Delve

Get approval before you pause a process. Confirm the PID and executable. Use a Delve version that supports the target Go version. Run `dlv attach "$pid" "$binary"` with an interactive terminal. Use `goroutines` to identify a goroutine and `goroutine <id> bt` to show its stack. Use `exit -d` to detach and leave the process running. Confirm that the process resumes. Keep debugger output private. Do not expose a Delve network listener or change host tracing permissions. If attachment is denied, use an approved local test run under `dlv debug`. Do not compare timings from a debugger build with a normal build.

## D. Deployed services

Deployed access is not configured. Before use, confirm the runtime, approved local access to the target process, and private profile storage. Do not expose the heap socket or Delve through a public endpoint. Do not attach Delve to a deployed process without separate approval.
