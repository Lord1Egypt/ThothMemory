# Mini shutdown reliability campaign (issue #15), 2026-10-14

Subject: published `lord1egypt/thoth-memory-mini@sha256:32e9e4c94adc36e8f781202ab55db1862bcdb5ac0a2ec6d10d156947955a7e57`
(Mini 0.1.2), disposable container and volume, `scripts/mini-shutdown-campaign.sh`. One JSON line per iteration; the last line
of each complete file is the summary. Outcome = what the supervisor's durable `/data/shutdown.log` recorded for that stop.

| File | Platform | Iterations | Result |
|---|---|---|---|
| `amd64-native.jsonl` | linux/amd64 native | 60 `docker restart`, 30 `docker stop`/`start`, 20 `docker kill` | 90/90 graceful stops recorded `clean`; all 30 `docker stop` exits were 0; all 20 SIGKILLs left **no** `clean` record (exit 137); 0 orphan mariadbd; 0 not-ready |
| `arm64-emulated-run2.jsonl` | linux/arm64 under QEMU (**emulated**) | 40 `docker restart` | 40/40 `clean`; 0 orphans; 0 not-ready |
| `arm64-emulated-run1-aborted.jsonl` | linux/arm64 under QEMU (**emulated**) | 14 clean restarts, then iteration 15 **not ready** | the script aborted at 15 (it then lacked diagnostics): after `docker restart` the service was not ready within 60 s and the container was no longer running. Cause **not established** (no logs or state were captured). The same image then ran 40 consecutive emulated restarts without recurrence. |

Instrumentation note: the first amd64 attempt reported 40 "missing" records; that was the campaign comparing line counts
against a log capped at 50 lines (category: missing evidence due to test instrumentation). It now compares the newest line.

Categories seen: clean (all graceful stops); expected-missing (SIGKILL, correct: nothing may claim a clean stop); not-ready
(one, emulated arm64, unexplained). Never seen: `exited-before-stop`, `error`, `killed-after-60s`.

Verdict: the amd64 path shows no defect in 110 iterations and the durable record behaves as designed. The one emulated-arm64
not-ready event is unexplained, so #15 stays open as a monitoring issue; it is not claimed fixed.
