# Project Brief: llama-swap Launcher & Monitor (macOS)

## Goal

Build a signed, notarized macOS menu-bar app in Go that launches, supervises, and monitors [llama-swap](https://github.com/mostlygeek/llama-swap), a local LLM model manager and router. It should also ship a small CLI so scripts and agents (e.g. Claude Code) can control it. The app will be distributed free on GitHub, and later as a Homebrew cask.

The app does **not** bundle or redistribute llama-swap. The user installs llama-swap separately, and this project is only the launcher/monitor.

## Current status and decisions

- **Stage:** milestones 0 to 5 passed on 2026-09-28 (see their results sections). The notarized 0.1.0 release runs on both the development Mac and a second Mac, the LAN peer. The icons are done (2026-09-29; see Components → Icons). Every TODO item up to 22 was done by 2026-10-04 (version 0.2.0), and the scrubbed history pushed to GitHub. A security assessment the same day added items 23 to 32 and a publishing checklist (the end of "TODO"); they come before the repository is made public. Then: milestone 6 (GitHub Actions). The milestone-0 test app stays in `spike/localnet/` for reference.
- **The primary goal is met** (2026-09-28): llama-swap runs under a signed, notarized app and reaches the LAN from anywhere, tmux included. What's left is polish and publishing, with the aim of sharing the project with llama-swap's developer. The order:
  1. the items in "TODO" below: all of them, before the project goes public on GitHub (decided 2026-10-03; the Preferences dialog is done);
  2. publishing: done 2026-10-04 as a single fresh commit of the scrubbed tree, rather than rewriting history, so nothing depends on a rewrite being complete. The detailed history stays on a local branch that's never pushed, and personal context (machines, hostnames, the setup before the launcher) is in `CLAUDE.local.md`, which Claude Code loads alongside this file and `.gitignore` keeps out of the repo. That file also has the check to run before any push. The Makefile's `SIGN_IDENTITY` default carries the maintainer's name and team ID; both are in `LICENSE` and in every signed binary already, so it stays. `README.md` (usage, the full `llsl` reference, settings) and `LICENSE` (MIT, like llama-swap's) were added 2026-09-28;
  3. milestone 6 (GitHub Actions) and milestone 7 (Homebrew cask).
- **Machines and the real setup:** a development Mac, and a second Mac on the LAN running its own llama-swap (the peer, also through the release app). Before the launcher, llama-swap ran from the maintainer's shell scripts, with a config whose `peers:` entry is the LAN connection that gets blocked. The details are in `CLAUDE.local.md`.
- **Signing:** Developer ID Application certificate installed and backed up (team ID `P56KW9H72P`); a hardened-runtime, timestamped test signature verified on 2026-09-28. Notarization credentials (App Store Connect API key) are saved in the keychain as notarytool profile `my-wang` (`--keychain-profile my-wang`), verified 2026-09-28. Account setup is complete.
- **Toolchain:** Go 1.27 and Xcode 27, but the active developer directory deliberately stays on the Command Line Tools. Call Apple's tools through `xcrun` so either setup works.
- **Git:** `origin` is https://github.com/alron/llama-swap-mac-launcher, still private (the maintainer's safety net: a slip can be wiped and re-pushed). `main` was pushed 2026-10-04 as one fresh commit; the old history is the local branch `private-history`, which a local pre-push hook refuses to push. Commits use the account's noreply address (`566136+alron@users.noreply.github.com`, set in this repo's config), and GitHub blocks pushes that would expose the real one. Git borrows `gh`'s login for this repo (a repo-local credential helper).
- **Language:** Go, because it's the maintainer's language. Don't propose Swift. Keep Objective-C to a few small shims (see Components §1).
- **Names:** the app is `Llama Swap Launcher.app` (dev builds: `Llama Swap Launcher Dev.app`). The executable inside is `llama-swap-launcher`. The CLI is `llsl` (no clashes on PATH or in Homebrew as of 2026-09-28).
- **Bundle ID:** `com.my-wang.llama-swap-launcher`, final as of 2026-09-28 (`com.my-wang` is the prefix for all of the maintainer's signed work). Never change it. Dev builds use `com.my-wang.llama-swap-launcher.dev`. It's defined in exactly one place, the Makefile.
- **Releases:** a notarized `.zip` (`Llama-Swap-Launcher-<version>.zip`) for GitHub Releases and later a Homebrew cask. A `.dmg` could be added alongside it later. The first version was 0.1.0; 0.1.1 added the real icons and the status-coloured menu bar; 0.1.2 the Preferences dialog, the first the maintainer was happy to share with close friends (2026-09-30). 0.1.x was proof of concept and pre-release; **0.2.0** (2026-10-04), with every TODO item done, is the first public release: built and notarized with `make release`, installed on both Macs, and checked hands-on (Settings…, the new options off until enabled). The Makefile's `VERSION` sets it.
- **Architecture:** arm64 (Apple Silicon) only. Intel Macs are impractical for local models and x86_64 macOS is on its way out. Build with `GOARCH=arm64`; the Homebrew cask gets `depends_on arch: :arm64`.

## TODO

Each change to `llsl` or the preferences also needs `README.md` updated.

- **A Preferences dialog** (requested 2026-09-28; **done 2026-09-30**, verified hands-on: saving and persisting, paste and the other editing keys, Tab between fields, the error alert and reopened dialog, the restart offer, only Quit enabled while it's open, the menu-bar icon updating while it's open, and Quit with the dialog, a file panel or the error alert open; see Components §1). "Edit Preferences…" should open a modal window in the app instead of opening `prefs.json` in a text editor. It should cover every field in `internal/prefs`: binary and config (with Choose… buttons), listen address, extra args, env vars, auto-start, and the two health-check settings. Keep `prefs.json` as the storage, so hand edits still work. This means more Objective-C in `internal/macos`; one approach is an `NSAlert` with a form as its accessory view, built from an `NSStackView` of rows or an `NSGridView` (which lines up the labels and fields in columns). Keep it as small and commented as the other shims.
  - **Menu changes with it** (decided 2026-09-30): remove Choose Binary… and Choose Config… from the menu, and the greyed-out "Binary: …" and "Config: …" lines too, since the dialog covers all of them. The long config path is also what makes the menu wide today; without it, the status line ("llama-swap: ready (pid …)") sets the width. Launch at Login stays in the menu. Update the README's menu section and Components §1 to match.

From real use of `llsl` (2026-09-28):

1. **`-load` on a peer model ID returns OK without loading anything** (**done 2026-10-04** with item 4: `-load` refuses a peer's model and a name not in the config (exit 2), and succeeds only when the events report the model ready; verified against the real config's peer models and a local one). `/upstream/<peer>/<model>/health` is answered by the *peer's* llama-swap (`OK`), not by a model server (`{"status":"ok"}`). This matches the code: `loadModel` in `cmd/llama-swap-launcher/control.go` accepts any HTTP 200. Either make it load the model for real (a request that needs it, such as a one-token completion), or refuse `-load` for peer IDs (`health.Model.PeerID` is set for them). If it's refused, item 4 covers what it checks today.
2. **`restart`, `stop` and `unload` refuse while requests are in flight; `-force` overrides** (**done 2026-10-04**: the monitor follows the `inflight` events (checked live: a snapshot with `requests` on connect, `upsert` as a request starts and progresses, `remove` with its `id`; only inference requests (`/v1/…`) count, not `/upstream/…`, so the app's own `-load` and peer checks never block anything). Refusals exit 7 (`busy`); when the app can't see llama-swap, commands go ahead. Verified during a real completion on gemma4-e2b-q4: all four refused with llama-swap untouched, and `restart -force` went ahead. Found then: with a request in flight, llama-swap waits for it after SIGTERM, so a forced stop or the menu's Stop takes the full 15 s stop timeout before the kill and cleanup). llama-swap v260 already sends `inflight` events on `/api/events`: a `snapshot` on connect, then `upsert` and `remove` per request. The monitor ignores them today. The menu bar stays the unconditional stop. This needs a new exit code (and README entry) for "busy".
3. **Optional auto-unload of a model whose GPU backend failed** (**built 2026-10-04**: the `unloadOnGpuFault` preference, a checkbox on the General tab, default off; `lastGpuFault` in `status -json` and a line in `llsl status`; a "Last GPU fault: MODEL at 14:32" menu line, with the time of day rather than "2h ago", since the menu only changes when something else does. All in memory, since the app started. Verified with the faulting stand-in model: detected, unloaded automatically, recorded; the menu line and the checkbox verified hands-on; **done**). Today it's a dialog nobody answers during an overnight run. Also add a timestamped record to `status -json`, since the fault itself clears on unload, and show it in the menu too ("Last GPU fault: gemma4-31b-q8, 2h ago"). This is short-term; the long-term home is llama-swap itself, if its author wants it.
4. **Peer health in `llsl status`** (**done 2026-10-04**: `checkPeers` in `cmd/llama-swap-launcher/peers.go`, one `/upstream/<peer>/<model>/health` per peer through llama-swap, in parallel, 3 s each, for `status` requests only since every response carries a status; `peers` in the JSON. Tested against a real llama-swap with a working, a refusing and a silent peer, and against the real peer: 72 ms for the whole `status`)**:** each peer's `/health`, reported in its own field rather than the exit code, with the Local Network hint on "no route to host". The app knows peer model IDs (`peer/model`) from the events but not the peers' URLs; find out whether llama-swap's API exposes them before resorting to parsing the config.
5. **Behind a config boolean: flag a loaded model whose `llama-server` build differs from the binary now on disk** (after a `brew upgrade`; **how, checked 2026-10-04**: `kern.procargs2` gives the path a process was started as (`/opt/homebrew/bin/llama-server`, a symlink into `Cellar/llama.cpp/<version>/`), readable from Go with `unix.SysctlRaw`, so no Objective-C. Resolve it now and compare the file's ctime with the process's start: a later ctime means the file changed after the process started. ctime, not mtime: a bottle's files can carry an old mtime, but extraction sets ctime. Simulated with a symlinked Go program repointed to a "version 2" with a 2020 mtime and version 1 deleted: flagged; the real running llama-server: not. A model's process is found as the listener on the port in its `/running` proxy URL, with item 9's lsof lookup. **Built 2026-10-04**: `supervisor.ServerChanged`, checked every minute while the `markUpdatedModels` preference (General tab, default off) is on; "— outdated build, unload to update" in the menu, `updated` in `llsl status -json`, a log note. End to end with a symlinked stand-in server in a fake Cellar: not flagged for a minute, flagged at the first check after the symlink was repointed, and cleared at once when the model was unloaded and loaded again (a model's mark is forgotten when it stops, rather than at the next check). The menu line and checkbox verified hands-on; **done**). Work out how to compare them before building it, for example the running process's executable against the file, or the server's reported build against `llama-server --version`.

From a review of the project by a local model (gemma4-31b, 2026-09-29), the suggestions worth keeping:

6. **Log lines in the failure alert** (**built 2026-10-04**: the supervisor keeps llama-swap's last 100 output lines in memory, cleared at each launch so a failure before llama-swap runs shows none (`Supervisor.RecentOutput`, also what item 22's "not saved" needs); the alert shows the last 10, minus the app's health-check lines, in a scrollable fixed-width box (`macos.AlertWithOutput`). Checked in a harness and hands-on with a stand-in llama-swap that always fails; **done**). When llama-swap fails ("llama-swap stopped with a problem"), include the last ~10 lines of its log in the alert, which is where they're needed to diagnose it. That's cheaper than a log-viewer window, and llama-swap's web UI already has a live log viewer.
7. **"Collect Diagnostics…" in the menu** (**built 2026-10-04**, in the Logs submenu: `internal/diagnostics` redacts in layers, every line of every file (the keychain's own secret values wherever they appear, llama-swap's `redact.go` rules including blocks and inline lists under secret-named keys, token shapes with the `sk-` pattern widened for base64 keys, URL passwords) and builds the zip; the home folder becomes `~`. A Save panel suggests the Desktop, which also spares the app a Desktop access prompt. The maintainer's point, 2026-10-04: people put keys in odd places, macros and such, hence the layers and a test config with keys hidden everywhere. The user's name becomes `<user>` wherever else it appears as a word (it survived the first hands-on test in a scratchpad path); empty values aren't redacted. Verified hands-on with two real zips; **done**). It would zip both logs, the preferences, the app, llama-swap and macOS versions, and the hardware (chip and memory: `sysctl hw.model hw.memsize`, since model performance and GPU faults depend on them) onto the Desktop, for bug reports once others run the app. Redact secrets first: the `env` preference values (`HF_TOKEN`, say), and any API keys in llama-swap's config if that's included. Never upload anything automatically.
8. **Tokens/sec beside each loaded model in the menu** (**built 2026-10-04**. Checked against v262: the stream's `activity` event carries only the request's `id`, not the token counts the audit expected; those are in `/api/metrics/activity` (paginated, newest first; `?limit=10` is a few hundred bytes). The monitor looks the request up there, skips ones that generated nothing, and forgets a model's speed when it stops. Shown in the menu ("— last 114 tok/s"), in `llsl status`, and as `lastTokensPerSecond` in its JSON, all labelled "last" (the maintainer's point, 2026-10-04: it mustn't read as a live figure; it's the latest request's average, not a peak); verified with a real completion on gemma4-e2b-q4), from the model's most recent request. llama-swap tracks timings (`/api/metrics/activity`, and responses carry `timings`); check what the API actually exposes before building it (the timings are in the response body, not headers). Besides the number itself, it would show a model that fell back to running on the CPU.

Considered and declined from the same review: model profiles and auto-load (llama-swap has both: `/api/profiles` and preload hooks in its config), config templates (the config belongs to llama-swap and the hardware, and templates would go stale with llama.cpp releases), a log-viewer window (item 6 plus the web UI cover it), and a health sparkline (too much for a text menu).

From a review by Qwen3.6-35b-a3b (2026-09-29), checked against the code:

9. **Name what's holding the port** (**built 2026-10-04**: `lsof -t` finds the pids, the supervisor's process lookup gives each one's name and parent, and the message tells apart a llama-swap run by another copy of the app (dev and release on the same port, verified with the installed 0.1.2), one started some other way (with its `kill` command), and anything else. lsof only sees the user's own processes; otherwise the old message stays. Failures before llama-swap runs, this one and a binary that won't execute, are marked `supervisor.ErrNotStarted`, and their alert says "llama-swap couldn't start" rather than "stopped with a problem"; **done**). Today a busy listen address gives "something is already listening on …; is another llama-swap running?". Run `/usr/sbin/lsof -nP -iTCP:<port> -sTCP:LISTEN` to name the process and pid, and say whether it's a llama-swap started outside the app.
10. **`llsl` warns when the running app is a different version** (**done 2026-10-04**, unit-tested and checked against the dev app with an `llsl` built as 9.9.9). `llsl` and the app ship together, so they only differ when a new version is installed while the old app is still running; the new `llsl` then talks to the old app. The protocol tolerates that (unknown fields are ignored), but a new flag such as item 2's `-force` would be silently dropped. Build `llsl` with its version (`-X main.version`) and warn on stderr when `status.appVersion` differs, suggesting a restart of the app.
11. **The versions** (the maintainer noticed on 2026-09-30, after installing 0.1.2, that nothing in the GUI shows the running version; **built 2026-10-04**: `supervisor.BinaryVersion` runs `llama-swap -version` before each start, which needs no API key, unlike `/api/version`. The version goes into the status (`version` in `llsl status -json`), the "started llama-swap" log line and the About panel; `health.TestedVersion` is 262, and the real-llama-swap test says when it can be raised)**:** the app's version and llama-swap's (from `/api/version`). Decided 2026-10-03: they go in the About panel (item 21), not in a greyed-out menu line as first planned. The app's own version is always shown, even with llama-swap stopped or failing, since that's when someone asks which version they have; llama-swap's is added while it answers. Collect Diagnostics (item 7) includes the same. Also record in the code the llama-swap version the app was last tested against (v260 today, the version the API notes in Components §3 were checked on). When the app finds itself running against a newer one, it writes a `[launcher]` note to the log and includes it in diagnostics; no alert. The health monitor is coupled to llama-swap's API, so after a llama-swap upgrade breaks something, that note is the first clue. Bump the recorded version whenever the real-llama-swap test passes against a newer llama-swap.
12. **Stricter preferences loading** (done 2026-09-30 with the Preferences dialog, which needed the same checks; a misspelled key verified hands-on). A value of the wrong type already gets an alert, but a misspelled key (`healthCheckSecond`) is silently ignored and the default used. Decode with `DisallowUnknownFields`, so the alert names the unknown key, and validate `listen` as a host:port. Hand edits stay supported even after the Preferences dialog, so this still matters then.

From a blind review by gemma4-26b-a4b (2026-09-30, without CLAUDE.md or the README; it reconstructed the architecture and the Local Network reasoning from the code alone):

13. **A shorter control-socket name** (**done 2026-10-04**: `ctl`; `llsl` recognises an app from 0.1.2 or earlier still listening at `control.sock` and says so, exit 3, rather than reporting it not running or trying to launch it; verified against the old dev build). The socket path's limit is 103 bytes. The release path is 80 bytes plus the username, so `llsl` stops working for usernames over 23 characters (19 for dev builds), and managed Macs' `firstname.lastname` accounts can reach that. The app shows a clear alert, but `llsl` is then unusable. Renaming the file from `control.sock` to `ctl` raises the limit to 32 characters (28 for dev) without moving the folder, which also holds the preferences. During an upgrade, a new `llsl` won't find an old running app's socket; item 10's version warning should explain that case.

Declined from that review: keeping status updates off the main thread (already the case: they run on goroutines, systray and the shims only switch to the main thread briefly to apply changes, and the menu is capped at 8 model slots). Its API-coupling point became the tested-version note in item 11.

Declined from the Qwen review:
- An `llsl models` subcommand: `status -json` already lists loaded models with state and fault flags, and peers are item 4.
- Forwarding SIGTERM to llama-swap's children: Stop snapshots the tree before signalling and cleans up survivors right after llama-swap exits (milestone 1's `kill -9` test showed `mactop` cleaned up).
- A 30 s health-check default: 60 s was chosen deliberately and is a preference. It doesn't affect `-wait`, which polls every 250 ms until llama-swap first answers.
- `llsl logs -json`: the log mixes three timestamp formats (llama-swap's, the launcher's, `llama-server`'s relative ones), so a `ts` field would be unreliable, and plain text serves agents fine.
- `launchctl bootout` for leftovers: llama-swap is the app's child, not a launchd job, and cleanup already sends SIGTERM before any SIGKILL.
- Configurable log rotation: not worth a preference at 10 MB × 5. (Tuning the sizes stays declined; an on/off switch is a different case, noted for later under item 22.)
- `-buildvcs=false`: Go only fails when a `.git` exists but can't be read. It may matter in a CI container; note it for milestone 6.

From a full evaluation by Claude Fable 5.1 (2026-10-02), checked against llama-swap v260's source (the route table in `internal/server/server.go`, `auth.go`, `apigroup.go`, `llama-swap.go`) and the installed 0.1.2 build. `make test`, `make lint`, `make sec` and `govulncheck` were all clean, and the installed binaries have hardened runtime, no entitlements, `minos 15.0` and distinct UUIDs:

14. **An `apiKey` preference** (the most valuable item on this list; **done 2026-10-04**, as several keys plus secret environment variables, all in the keychain). In v260 every endpoint the app uses except `/health` is behind the API-key middleware: `/api/events`, `/logs/stream/*`, `/api/models/unload*`, `/api/version`, `/api/metrics/*` and `/upstream/*`. With `apiKeys` in the config the monitor never connects: the status stays "starting" for ever, Open llama-swap UI never enables, `llsl start -wait` exits 6 after the whole timeout, and unload gets a 401. The people who listen on `0.0.0.0` are the ones who set keys. Send it as `Authorization: Bearer` (llama-swap also takes Basic's password and `x-api-key`) on every monitor request and on `-load`. Needs the README's preferences table, and Collect Diagnostics (item 7) must redact it.
    - **Keep the key in the login keychain, not in `prefs.json`** (decided 2026-10-02; worth having for a public release even if the maintainer doesn't set keys). llama-swap substitutes `${env.NAME}` anywhere in its config, `apiKeys` included; its `docs/config.example.yaml` recommends it "to keep secrets out of the config". Verified 2026-10-02 against v260 with a throwaway config: with the variable unset, loading fails with "environment variable 'NAME' is not set" (`-validate` exits 1 with that); with it set, `Bearer <key>` and `x-api-key` get 200 and the literal text `${env.NAME}` gets 401. So the config refers to keys by variable (see the decision below), the app passes them to llama-swap in those variables, and sends one on its own requests. The key then exists on disk only in the keychain: not in the config, `prefs.json`, backups or a diagnostics zip. `supervisor.Validate` already runs with llama-swap's environment (`spec.Env`), so validation sees the key too. A key written literally in the config still works: the user enters the same key in the dialog, and the app uses it only for its own requests.
    - **Protection at rest only, and that's enough for now** (decided 2026-10-02). While llama-swap runs, the key is in its environment, which other processes of the same user can read, and its model servers inherit it. Anything running as the user can read their files anyway.
    - **Decided 2026-10-04: several keys, each passed as `LLSL_<NAME>`.** llama-swap's `apiKeys` is a list, so each client can have its own key and one can be revoked without the others. The forced `LLSL_` prefix keeps a key from ever overwriting a variable llama-swap or its model servers rely on (`PATH`, `HF_TOKEN`) and marks the app's keys in a config: `apiKeys: ["${env.LLSL_ADMIN}", "${env.LLSL_CLAUDE}"]`. `prefs.json` keeps the names (without the prefix) and which one the app sends on its own requests (any works; the first by default); the values live only in the keychain, one item per key. The app passes every key to llama-swap. A name in the config with no key in the app fails validation with llama-swap's own "environment variable … is not set", so a mismatch shows at Start or Restart rather than as a 401. The `env` preference rejects names starting with `LLSL_`.
    - **UI (decided 2026-10-04):** Preferences gets tabs, General (today's settings) and Secrets (named API Keys at first), with Logs to join in item 22. The keys are a table of name (with the `LLSL_` prefix shown fixed) and value (dots), with + and − below it, Generate (the example config's tip: `sk-` plus 48 random bytes, base64) and Show, which reveals the selected key to select and copy with ⌘C; no Copy button, so the app itself never writes to the clipboard. A popup chooses the key the app uses.
    - **Built 2026-10-04, keys part done**, verified hands-on: creating a key, llama-swap reaching "ready" on a config that requires it, the browser login with it, a 401 without it, removing it (validation then refuses, leaving llama-swap running), and the keychain item appearing and disappearing. Also tested: a real v262 llama-swap with `${env.LLSL_TEST}` (monitor connects with the key, not without), and a harness driving the API Keys tab. gosec's G117 on `Prefs.APIKeys` is annotated: it holds names only.
    - **Keychain Access shows long keys cut short** (checked 2026-10-04): its password field wraps at hyphens and shows only the first line, so a generated key shows as just `sk-`. An item written by Apple's `security` tool with the same value shows the same, and the shim reads every value back intact. Not ours to fix; the README needn't mention it unless people ask.
    - **The web UI:** with keys set, llama-swap answers `/ui/` with `401` and `WWW-Authenticate: Basic realm="llama-swap"` (checked 2026-10-04), so the browser asks for a login: any user name, and a key as the password. The README says so.
    - **Secret `env` values** (`HF_TOKEN`, say), a separate step after the keys (decided 2026-10-04; **done 2026-10-04** as a second table on the tab, renamed Secrets; harness-tested, and verified hands-on: a variable added in the dialog reached the restarted llama-swap's environment with the right length (checked with `ps -E`, which also shows the at-rest-only caveat), the plain-and-secret clash was refused, and removing it cleared the environment, the keychain and `prefs.json`), can use the same storage: mark a variable as secret, and its value moves to the keychain while its name stays in `prefs.json`.
    - **One model for both** (2026-10-04): an API key and a secret variable are each just a variable llama-swap gets, so the keychain item's account is the variable's name (`LLSL_ADMIN`, `HF_TOKEN`); `cmd/llama-swap-launcher/secrets.go` reads, writes and deletes them. Secret variable names aren't forced to capitals (`http_proxy` is lower case), can't start with `LLSL_`, and can't also be in `env`. In the dialog, `LSLSecretTable` (`prefs.m`) runs each table; keys add a fixed prefix, capitals and Generate.
    - **Shim:** `internal/macos/keychain.m` with the Security framework (`SecItemAdd`, `SecItemCopyMatching`, `SecItemUpdate`, `SecItemDelete`; a generic password with the bundle ID as its service), so dev and release keep separate items. `llsl` never touches the keychain: it asks the app, which makes every request to llama-swap. Verified 2026-10-04 with two different binaries signed with the Developer ID under one identifier (the legacy login keychain; the data-protection keychain would need entitlements and a provisioning profile): one stored an item and the other read and replaced it, with no prompt, in under 0.3 s each. Apple's `security find-generic-password -w`, as an outsider, got a prompt; denying it left it with nothing (exit 128). So an update keeps access silently, and other programs need the user's consent. Ad-hoc builds will probably prompt after every rebuild.
15. **Orphaned model servers after an app crash** (**done 2026-10-04**. Reproduced first with the dev app: `kill -9` of the app while a model loaded; llama-swap died within a second, before any request, since it was relaying the model's output, and the `llama-server` was left orphaned with its memory and port, which the relaunched app didn't touch; the orphaned `mactop` exited by itself. With the fix, the pidfile listed both, and the relaunch logged "stopping 1 process(es) a previous llama-swap left running" and stopped the `llama-server`. The pidfile is rewritten only when the descendants change, and not after llama-swap has exited, so a late snapshot can't bring back a removed pidfile). When the app dies, llama-swap's stdout pipe closes, and Go ends a program that writes to a broken stdout or stderr with SIGPIPE. llama-swap handles only SIGINT, SIGTERM and SIGHUP (`llama-swap.go`), so it dies without stopping its model servers, which are in their own process groups. At the next launch `CleanupLeftovers` (`internal/supervisor/supervisor.go`) finds the pidfile's llama-swap gone and returns early, so the orphans keep their memory and ports, and the new llama-swap may be handed a port one of them still holds. Fix: write the descendant snapshot that `watchTree` already refreshes every second into the pidfile as well (one `pid start name` line each), and have `CleanupLeftovers` terminate any of those still alive with a matching start time and name.
16. **A non-200 from the event stream is silent** (**built 2026-10-04**: the monitor records the refusal as `Snapshot.Problem` and logs each distinct status once; the state becomes `running` with the reason as the message, the arrow turns red, and `-wait` fails at once. Checked against the dev app with a config holding a key it lacked: `start -wait` failed in 1.2 s instead of timing out. `health.MinimumVersion` is 252, the README says so, and an older llama-swap gets a log note. The menu's explanation under the status now wraps over up to four lines of 64 characters, rather than one line cut at 90 that also widened the menu; verified hands-on; **done**). `streamEvents` (`internal/health/health.go`) returns false on any status without logging, so a 401 (item 14) or a 404 from a llama-swap too old to have `/api/events` leaves no trace, and both look like "starting". Log the status once per connection attempt, and show "needs the API key" or "llama-swap is too old" in the menu and in `status`. The README should also name the minimum llama-swap version. From llama-swap's CHANGELOG (checked 2026-10-02): `/api/events`, peer models and the unload API predate v200, where it starts; `-config-dir` arrived in v230 and `-validate` in v252 (the app tolerates its absence); v259 moved log data off `/api/events` onto `/api/events/logs`, so before that the stream the monitor reads also carried every log line. v252 is a reasonable documented floor; v260 is what it's tested against.
17. **Reject `-listen`, `-config` and `-config-dir` in the `args` preference** (**done 2026-10-04**: `prefs.Validate` rejects them, and also the TLS flags, `-validate` and `-version`, in every form Go's flag package reads, including a flag and its value written on one line). Go's flag package takes the last occurrence, so a second `-listen` makes llama-swap listen somewhere the app isn't watching ("starting" for ever), and a second `-config` changes what was validated. llama-swap also has `-config-dir` (a directory of `*.yaml` files, additive to `-config`), which the app neither offers nor forbids; `-validate` covers both, so it could become a preference later.
18. **Path-escape the model ID in `loadModel`** (**done 2026-10-04** with item 1: `escapeModel` escapes each part of the ID) (`cmd/llama-swap-launcher/control.go`). It builds `/upstream/<model>/health` from the raw ID while `Unload` escapes, so a `?` or `#` in an ID changes the request. Same-user only, so not exploitable. Escape per path segment; part of item 1.
19. **A hung `-validate` is reported as an invalid config** (**done 2026-10-04**: it's now "didn't finish within 30s", not `ErrInvalidConfig`). `supervisor.Validate` kills after 30 s, which comes back as an `exec.ExitError`, so the message says the config is invalid. Check `ctx.Err()` first and say it timed out.
20. **Say what `0.0.0.0` exposes** (**done 2026-10-04**: the dialog's hint, now wrapping, and the README's `listen` row). The dialog's hint (`prefs.m`) and the README's `listen` row nudge people onto all interfaces without saying that llama-swap's API, UI and unload endpoints are then open to the LAN unless the config sets `apiKeys` (item 14).

Noted from the same evaluation, not planned:
- `llsl` isn't gated by `inPrefs`, so a control request can restart llama-swap while the Preferences dialog is open. Acceptable, since the caller is the same user; Components §1 now says so. Gate control requests too if it ever matters.
- The menu-bar icon can take up to `StopTimeout` plus 5 s to appear after a crash, since `CleanupLeftovers` runs before `systray.Run`. The `[launcher]` log line already says why.
- The Makefile's `SIGN_IDENTITY` default: see the scrub notes under "Current status and decisions".

Notes on the earlier items, from the same evaluation:
- **Items 1 and 4 are one piece of work.** v260 has no peers API, and no handler serializes a peer's `proxy` URL (`internal/server/api.go`, `apigroup.go`), so parsing the config would be the only way to get addresses. They aren't needed: the behaviour item 1 complains about is item 4's feature. `GET /upstream/<peer>/<model>/health`, answered by the peer's llama-swap, is a reachability check through llama-swap's own proxy, and a failure carries llama-swap's "no route to host" text with its Local Network hint. So: `-load` refuses peer IDs (`health.Model.PeerID` is set), `status` gains a `peers` field from one such request per peer (the model IDs come from the events), and local loads wait for `modelStatus` to report `ready` instead of trusting HTTP 200, which also covers upstreams without a `/health`.
- **Item 2:** the `inflight` message type exists in v260 (`msgTypeInFlight` in `apigroup.go`, payload `swaputil.InFlightRequestsEvent`), with the snapshot on connect.
- **Item 5:** lowest value. If built, the supervisor's descendant snapshot already names the running `llama-server` pids; compare each one's executable path with the resolved path of the binary on disk. x/sys has no `proc_pidpath`, so it needs a small cgo call or `lsof -p`.
- **Item 7:** llama-swap ships its own redaction rules in `internal/config/redact.go` (`apiKeys`, `peers.<id>.apiKey`, any key named like a credential, Authorization headers). Copy those patterns rather than inventing a list.
- **Item 8:** cheaper than described. The event stream has an `activity` message type (`msgTypeActivity`) carrying the same `tokens` block as `/api/metrics/activity` (`input_tokens`, `cache_tokens`, draft counts and more), so no polling is needed. The endpoint answers on v260 (checked on the running instance, 2026-10-02).

From the maintainer (2026-10-03):

21. **An About panel**, with "About Llama Swap Launcher" in the menu just above Preferences…. A GUI app without one feels wrong. (**Done 2026-10-04**, verified hands-on: the details, the link, greyed out while Preferences is open, no llama-swap line while it's stopped. The copyright line is Info.plist's `NSHumanReadableCopyright`.)
    - Use macOS's standard panel (`orderFrontStandardAboutPanelWithOptions:`): icon, name and version, plus a credits area of rich text with the build date, a clickable link to the GitHub page, and "MIT License". The build date comes from the Makefile, as the version does. Activate the app first, as the dialogs do.
    - It isn't modal: an ordinary window that stays until closed and blocks nothing, which is what About is everywhere. So it doesn't go through `lsl_run_modal`.
    - It shows llama-swap's version too, while llama-swap answers, which replaces item 11's menu line.
    - Checked 2026-10-04: the panel shows its "Version" option in brackets after the version, defaulting to `CFBundleVersion`, so with both the same it read "Version 0.1.3 (0.1.3)". The shim passes an empty one, which shows just "Version 0.1.3", and `CFBundleVersion` stays as it is. If it's ever changed, keep it increasing from build to build, since Sparkle (later) compares it.
22. **Separate logs, with configurable locations, and the option not to save llama-swap's output** (**built 2026-10-04**: `cmd/llama-swap-launcher/logs.go` owns both files through `logfile.Switch` writers, which swallow write errors so a full disk can't block llama-swap's output pipe; `Rotating.OnOpen` keeps stderr on the current launcher log; a Logs tab and a Logs submenu with Show in Finder; `llsl logs` finds the file through the preferences, `-launcher`, and the in-memory lines when not saved; failure replies carry `output`. Checked with a harness and end to end in all three modes, including stderr's target, the old `llama-swap.log` no longer growing, and hand edits applying at llama-swap's next start, which the plan had missed. `llsl status`'s peer checks now carry the health-check User-Agent, so they're kept out of the log too. Verified hands-on: the Logs submenu, Log Settings…, the tab and both pickers; **done**. The maintainer noticed then that `~` worked in the log fields but not the config field: every path preference now keeps "~" as typed and is expanded where it's used (`Prefs.BinaryPath`, `ConfigPath`, `LogPaths`)). The motivation: if llama-swap's output becomes structured (JSON lines, which the maintainer has considered proposing upstream), a log consumer (Splunk, Elastic) needs it unmixed. A JSON stream must contain only JSON, so the launcher's notes can't be interleaved into it.
    - **Preferences:**
      - **Launcher log:** a path field whose greyed-out default is `launcher.log` in today's log folder, as the binary field shows the binary it found.
      - **llama-swap's output:** a popup, "in the launcher log" / "in its own file" / "not saved", with a path field enabled only for "in its own file", whose greyed-out default is today's `llama-swap.log`. Decided 2026-10-03 over two checkboxes ("enabled", "include in launcher"), one of whose four combinations (off but included) means nothing. JSON keys: `launcherLog` (a path, empty for the default), `llamaSwapLog` (`"launcher"`, `"file"` or `"off"`, default `"launcher"`) and `llamaSwapLogFile`.
    - **The defaults keep today's behaviour, under a new name:** the combined log becomes `launcher.log`, so an upgraded install's `llama-swap.log` stops growing. Say so in the release notes.
    - **`launcher.log` today is the app's stderr** (panics), connected by `dup2` in `main` before the preferences are read. Merging the notes into it means reading the preferences first (falling back to the default location if they're broken), and pointing fd 2 at the new file after every rotation, or panics land in the rotated-away file.
    - **"In its own file" is exactly llama-swap's output.** The only change is `logfile.DropLines` removing the app's own health-check request lines, whole lines, so a JSON-lines stream stays valid.
    - **"Not saved" still reads the pipe and discards it,** since closing it would end llama-swap with SIGPIPE (item 15). Keep the last ~100 lines in memory: `llsl`'s failure output (exit codes 1, 5 and 6) and item 6's alert need them exactly when something has failed. GPU-fault detection reads the API, not the file, so it's unaffected.
    - **Custom locations:**
      - Absolute paths only; the folder must exist and be writable, and the two logs can't be the same file.
      - A log may not exist yet, so a picker would be a Save panel, whose "Replace?" question is wrong for a file the app appends to. Choosing a folder may be simpler; decide when building.
      - A log in Desktop, Documents, Downloads, iCloud Drive or on an external volume makes macOS ask the app for access once, and at login nobody may be there to answer. Say so in the README.
    - **When changes apply:** see the decisions below (immediately, both).
    - **`llsl logs`** reads the locations from `prefs.json` (`internal/prefs` is pure Go) and shows llama-swap's output wherever it goes; a new `-launcher` flag shows the launcher log. With llama-swap's output not saved, it says so, and the failure output comes from the app's in-memory lines. README.
    - **The menu:** a Logs submenu replaces Open Log: Open Launcher Log, Open llama-swap Log (only while it has its own file), a separator, and Log Settings…, which opens Preferences (decided 2026-10-03, rather than a separate log dialog that would duplicate Preferences' saving, checking, restart offer and Quit-only gating). Later, Show in Finder for each log (`/usr/bin/open -R`, no Objective-C needed): the maintainer works with logs in a terminal (vim, grep), and dragging the file from Finder into a terminal gives its path with no clipboard involved, whereas Open Log shows it in Console.
    - **Decided 2026-10-04, before building:** changes apply immediately on Save, both logs, with no restart offer (the app owns the writing, so switching between lines is clean). Failure output in `llsl` (exit 1, 5, 6) comes from the app's memory, llama-swap's last 20 lines in the failure reply, the same in every mode. The Choose… pickers accept an existing file or a folder (the default name is used inside it), so there's no misleading "Replace?". Show in Finder comes now, not later.
    - **If llama-swap's output goes JSON,** the launcher's notes could follow: writing them through `log/slog` makes text or JSON lines the same calls.
    - **Later, not planned yet:** an on/off switch for rotation, default on, labelled with what it does (10 MB × 5). With a log collector, the collector or macOS's `newsyslog` may want to own rotation. Collectors such as Splunk's forwarder and Filebeat follow rename-based rotation, so leaving it on is safe for them.

**Preferences is outgrowing one form** (noted 2026-10-03): logs (item 22), the API key (item 14) and secret environment variables will make it unwieldy. Plan on tabs (General, Logs, API Key), with Log Settings… opening the Logs tab; do it with whichever of items 14 and 22 comes first. Apple's convention since macOS 13 names the menu item "Settings…" rather than "Preferences…"; a cheap rename to make alongside.

Suggested order (2026-10-02; all of them come before publishing, so this is only which to do first): items 13 (the socket path changes, and nothing depends on it yet), 10 and 11 together, 6, 9, 14, 15 and 16. Then 1 with 4, then 2; 17 to 20 as they come up; then 3, 7, 8, and 5 last. Added 2026-10-03: 21 alongside 10 and 11 (it's where the versions go); 22 after 14, sharing the move to tabs.

From a security assessment by Claude Fable 5.1 (2026-10-04), before the repository goes public. Every Go and Objective-C file was read, and each item marked "verified" was tested, not only read. Nothing found lets a remote machine or another user of the Mac take over the app or read its secrets. These are what to fix, or say, before strangers run it:

23. **The release zip breaks when unpacked with `unzip`** (verified with the 0.2.0 zip).
    - `ditto -c -k --keepParent` stores each file's `com.apple.provenance` attribute as a `._name` file beside it. Finder and `ditto -x -k` merge those back. `/usr/bin/unzip` leaves 11 of them inside the bundle, and `codesign --verify` and `spctl` then both report "a sealed resource is missing or invalid".
    - People who live in a terminal are this app's audience. Whether Homebrew unpacks a cask's zip the same way wasn't checked; check before milestone 7.
    - Fix: `ditto -c -k --norsrc --noextattr --keepParent` on both `ditto` lines of `release`. Verified: unpacked with either tool, `codesign --verify --strict`, `spctl` and `stapler validate` all pass, and nothing else is in the zip. The stapled ticket is a file (`Contents/CodeResources`), so dropping attributes loses nothing. Add an `unzip`, `codesign --verify` step to `release` so it can't come back.
24. **Collect Diagnostics leaves secrets in** (verified by running the redactor on samples). People will attach these zips to public issues. What got through:
    - a commented-out line (`# apiKey: old-key`);
    - a block scalar under a secret-named key (`apiKey: |`, with the key on the next line: the `|` is redacted, the key isn't);
    - `x-api-key: …` anywhere, in YAML or in `curl -H "x-api-key: …"`: the hint is `api_?key`, which doesn't match the hyphen, and it's a header llama-swap itself accepts;
    - keys named `key`, `peer_key`, `pass` or `pwd`, and macros with innocent names;
    - `curl -u user:password`, `MYPASS=…`, and a secret-named key inside one-line JSON (`{"api_key":"…"}`), since the key rule only matches at the start of a line;
    - token shapes not listed: `github_pat_…`, `glpat-…`, `AIza…`;
    - `prefs.json`'s `env` values under names that don't look secret (`GH_PAT`, `WANDB_KEY`).

    The first two have one cause: llama-swap's rules, which these copy, run on the parsed YAML, where comments are gone and a block scalar is one value. Here they run on the raw file's lines. Fixes:
    - blank every `env` value in the `prefs.json` copy (the app knows that map exactly; the names stay);
    - apply the rules to comment lines too, and carry a secret-named key's block scalar;
    - widen the hint to `api[-_]?key`, add `-u`, the token shapes, and secret-named keys inside a line;
    - have the alert say, as the README does, that hostnames and addresses stay, and that logs can hold prompts.

    Add each sample to `redact_test.go`.
25. **A client can keep its requests out of the saved log** (verified against v262 on a spare port).
    - `logfile.DropLines` drops every line containing `llama-swap-launcher-healthcheck`, and llama-swap's access line quotes the client's User-Agent: `[INFO] Request 127.0.0.1 "POST /api/models/unload HTTP/1.1" 200 13 "llama-swap-launcher-healthcheck" 36µs`.
    - So any client that sends that User-Agent, from the LAN included, is missing from the log, `llsl logs`, the failure alert and diagnostics, whatever it asked for. The query string isn't logged, so only the User-Agent does it.
    - Fix: drop a line only when it's also a health check, `"GET /health HTTP/` or `"GET /upstream/…/health HTTP/`, with the quoted User-Agent. `app.recentOutput` has the same test.
26. **The app's requests to llama-swap obey `HTTP_PROXY`** (verified with a stand-in proxy).
    - `health.New` makes a plain `http.Client`, and `control.go` and `peers.go` use `http.DefaultClient`. Both take a proxy from the app's environment, which `open` and `llsl` pass in from the shell.
    - Go skips the proxy for loopback, so the default `127.0.0.1`, and `0.0.0.0` (which the app dials as `127.0.0.1`), aren't affected.
    - With `listen` set to one of the Mac's own LAN addresses, or a host name, every request goes to the proxy with `Authorization: Bearer <key>`: the key leaves the Mac, and the monitor connects only if the proxy can reach the Mac back.
    - Fix: one shared client for all of the app's requests to llama-swap, with the transport's `Proxy` set to nil.
27. **Warn when llama-swap can be used without a key** (a feature, and the one worth adding before going public). The likeliest way a user gets hurt is `0.0.0.0` on a laptop, no keys, then another network. Two checks:
    - On Save: `listen` isn't loopback and the app has no API keys. Confirm, saying what's exposed.
    - Once llama-swap is ready: one request without a key for a route that needs one (`/api/version`). A 200 means llama-swap isn't asking for keys. This matters most when the app *has* keys: someone who adds a key under Secrets but never adds `apiKeys:` to llama-swap's config has a llama-swap that ignores it, and everything looks right, since the monitor's requests succeed either way. Show it in the menu and in `llsl status` when `listen` isn't loopback.

    Verified on v262 with no `apiKeys`: a cross-origin `POST /api/models/unload` gets 200 and `Access-Control-Allow-Origin: *`. So on `127.0.0.1` too, any web page open in the user's browser can use llama-swap (its config's `cors.allowedOrigins` narrows that), and so can the Mac's other users. The README's API keys section reads as if keys only matter on `0.0.0.0`; say this there. It's llama-swap's own behaviour, and worth raising with its developer.
28. **Say what the app trusts: a `SECURITY.md`, and a short Security section in the README.** There's neither, and GitHub shows no security policy. What they should say:
    - How to report a vulnerability: GitHub's private reporting (see the checklist below).
    - `prefs.json` and llama-swap's config decide what the app runs. Anything running as the user can write either and reach the control socket, and so can make the app run any program: with its Local Network permission, with any file access macOS has granted it, and with every keychain secret in its environment, without a keychain prompt. That isn't a bug; it's what a launcher is, as with a terminal. But the README's "other programs can only read it with your permission" is half the story: the keychain protects the keys on disk, in backups and from other users, not from programs already running as the user.
    - **For agents:** an agent allowed to edit llama-swap's config and run `llsl restart` can run any command, outside whatever sandbox the agent itself is in, because the app runs it. The README recommends exactly that workflow, so it should say so. `llsl`'s output also carries text from elsewhere (clients' request paths in log lines, a peer's error body), which an agent reads as input.
    - llama-swap speaks plain HTTP, so keys cross the LAN unencrypted, and the app refuses the TLS flags (`reservedFlags`), so under the app there's no way to turn TLS on. A later feature: allow them, and have the monitor speak HTTPS.
29. **`llsl`'s text output prints text from the network as it comes.** A peer's error (up to 1000 bytes of the peer's reply), `message`, and model IDs go to the terminal with any escape sequences and newlines they contain, which can redraw the terminal or forge a line such as `llama-swap: ready`. `-json` already escapes them. Replace control characters in `printStatus` and `report`; `logs` can stay as it is, like `cat`.
30. **The monitor keeps sending the key after llama-swap dies.** `Supervisor.wait` stops leftover model servers (up to 5 s) before it changes the status, which is what makes `syncMonitor` cancel the monitor. Until then the monitor's reconnects (every 250 ms at first) and the model-log watchers carry the key to whatever listens on the port next, such as another user's program on a shared Mac. Fix: let the app know the process has gone before `terminate` runs.
31. **Text that isn't valid UTF-8 makes the shims raise** (verified in a harness: `str()` returns nil for it, and `alert.messageText`, `informativeText`, a menu item's title, `NSAttributedString` and a field's `stringValue` each raise an exception on nil).
    - Such bytes can come from llama-swap: `Monitor.post` cuts an error body at 300 bytes, which can split a character, and `Validate` passes llama-swap's output on whole. Both reach `app.problem`.
    - In the app, that's either a crash, which takes llama-swap down with it, or a caller left waiting for ever. Which of the two wasn't tested.
    - Fix: `strings.ToValidUTF8` in the Go wrappers (`Alert`, `Confirm` and the rest), or a `str()` that never returns nil.
    - Today the failure alert's output box is silently left out when a line was cut mid-character (`tail` cuts at 1000 bytes).
32. **Smaller, as they come up:**
    - Without the control socket, the app carries on without its single-instance lock (`main.go`: any `Listen` error except `ErrAlreadyRunning`), and `CleanupLeftovers` then stops another instance's llama-swap. `Listen` can also lose a race between two launches: both find nothing listening, and the second bind fails. Don't clean up or start llama-swap without the lock.
    - `control.serveConn` reads a request line of any length (the same user only). Cap it at 64 KB.
    - A log in a folder others can write to (`/tmp`, `/Users/Shared`) is opened through whatever symlink is waiting there. `O_NOFOLLOW` would refuse it.
    - The GPU-fault marker is matched anywhere in a model's output, so with `unloadOnGpuFault`, a server that logs prompts unloads on a prompt containing it. Anyone who can send a prompt can call unload too, so this is only noted.
    - `-listen-tailcat` (Components §3) isn't in `reservedFlags`. Decide whether it should be; item 27's check can't see that listener.

**Publishing checklist** from the same assessment (not code):
- **Tag the release commit** (`v0.2.0`, or the next version's). The binary embeds `vcs.revision`, which should be a commit people can find; 0.2.0's is on `main`, and there are no tags yet. In the release notes: the zip's SHA-256, and how to check a download (`spctl -a -vv`, and Team ID `P56KW9H72P` in `codesign -dv`).
- **When the repository is public, turn on** private vulnerability reporting, secret scanning with push protection, Dependabot alerts, and a ruleset on `main` that refuses force-pushes. The last isn't offered for a private repository on this plan (the API said so, 2026-10-04).
- **A pull request is a way into the Mac that signs.** Agents build this project on the machine that holds the Developer ID and the notary credentials. A branch that changes `CLAUDE.md`, adds `.claude/` settings or hooks or an `.mcp.json`, or touches the `Makefile` or `packaging/`, steers or runs code there as soon as a session opens it. Read those paths by eye first, and don't check a stranger's branch out in this working copy. For milestone 6: signing secrets only in a protected environment, on tag pushes, never in a workflow a fork's pull request can trigger; pin actions by commit.
- **Forks and ad-hoc builds** should use their own bundle ID (`make BUNDLE_ID=…`): under this one, an ad-hoc build leaves Local Network entries that can't be removed, and shares this app's folder, socket and keychain service name. Say so under the README's "Building from source".
- **Not security, and not tested here:** opened straight from Downloads, a quarantined app runs from a random read-only path (Gatekeeper's app translocation), so Launch at Login and an `llsl` symlink would point at a path that disappears. The README says to move it to `/Applications`; a check at startup (the executable's path contains `/AppTranslocation/`) could say so too.

Noted, not planned: a launcher that runs whatever its preferences name can serve malware already on a Mac as a signed parent, and the same Developer ID signs all the maintainer's work. That's inherent in what the app is, and true of terminals too.

Checked and found sound, so the next review needn't repeat it:
- **The 0.2.0 release app:** hardened runtime, no entitlements (so no injected libraries, no debugger), notarized and stapled, the designated requirement is the bundle ID plus the team ID, no local paths in either binary, and `vcs.modified=false`. `make test`, `make lint`, `make sec` and `govulncheck` are clean.
- **What can reach the app:** only the control socket (mode 0600 in a 0700 folder, plus the peer-UID check; checked on the installed app). It isn't HTTP, so no web page can reach it. The app has no listening port, URL scheme or Apple Events interface.
- **What the app runs:** llama-swap's environment is built from scratch (no `DYLD_*` or proxy variables reach it), every subprocess is started without a shell, and `lsof` and `open` get fixed paths and checked arguments.
- **Secrets:** keys come from `SecRandomCopyBytes` (48 bytes); they aren't synced, never logged, and absent from `prefs.json`, the status and the diagnostics summary (names only); the dialog uses secure fields and never writes the clipboard.
- **Files:** preferences, pidfile, logs and the diagnostics zip are 0600. The pidfile cleanup checks pid, start time and name before it stops anything.
- **The repository:** the tree and `main`'s commit messages pass `CLAUDE.local.md`'s check, commits carry the noreply address, and the PNGs carry no metadata beyond a colour profile.

Suggested order: 23 (two flags), 25, 26 and 31 (a few lines each), then 24 and 27, then 28 with the README changes, then 29, 30 and 32. Items 24 to 31 change the app, so they're a new version (0.2.1), which also gets the fixed zip.

## Working in this repo

- Go module `github.com/alron/llama-swap-mac-launcher`.
  - `cmd/llama-swap-launcher/`: the app (`main.go` wiring, `app.go` menu and actions).
  - `internal/supervisor`: runs, restarts and stops llama-swap, including the process-tree cleanup and config validation.
  - `internal/health`: watches a running llama-swap through its API. `cmd/llama-swap-launcher/models.go` puts that in the menu.
  - `internal/control`: the control socket's protocol, server and client (pure Go, shared with `llsl`). `cmd/llama-swap-launcher/control.go` answers requests.
  - `cmd/llsl`: the CLI, built with `CGO_ENABLED=0` into `Contents/Helpers/llsl`. The dev build's is `"build/Llama Swap Launcher Dev.app/Contents/Helpers/llsl"`.
- **systray gotcha:** on macOS, `systray.Quit()` just makes `systray.Run` return; it doesn't call `onExit`, which only runs when macOS terminates the app (at logout, say). So shutdown work runs from both (`app.shutdown`, called after `systray.Run` returns and from `onExit`).
  - `internal/macos`: the cgo/Objective-C shims: dialogs (`dialogs.m`), the Preferences dialog (`prefs.m`), the About panel (`about.m`), launch at login (`login.m`) and the menu-bar title (`statusbar.m`), with shared helpers in `common.h`.
  - `packaging/`: the Info.plist template, the app icon (art, masters, and the `mkicon` tool) and `notarize.sh`.
  - `internal/prefs`, `internal/logfile`, `internal/paths`.
  - `packaging/Info.plist.in`: the bundle's Info.plist template.
- `make dev` builds and signs `build/Llama Swap Launcher Dev.app`; `make run` builds and launches it; `make test`, `make vet`. `make dev` refuses while the dev app is running, since it deletes and recreates the bundle; quit the app first.
- `make release` builds `build/release/Llama Swap Launcher.app` under the release bundle ID:
  - it signs with a secure timestamp and zips the app with `ditto`;
  - `packaging/notarize.sh` notarizes it (keychain profile `my-wang`) and staples the ticket, or prints Apple's log if rejected;
  - it zips the app again so the zip carries the ticket, then runs `spctl --assess` and prints the zip's SHA-256.
  
  Notarization takes a few minutes. `syspolicy_check distribution <app>` is a further readiness check.
- Builds use `-trimpath`, so the binaries don't contain local paths.
- **Checks at stable points:** `make lint` (`go vet` + `staticcheck`) and `make sec` (`gosec`, which catches things the others don't). The gopls MCP server is also available for diagnostics, references and renames.
- **gosec triage, 2026-09-28:** fixed G115 (the peer-UID comparison) and one G104 (`llsl logs -f` ignored a failed `Seek` and would reprint the whole log). Accepted as by design:
  - G204, a subprocess with variable arguments: launching the user's llama-swap is the app's job, and the rest are `/usr/bin/open` with fixed arguments.
  - G304, a file path from a variable: every path comes from the bundle ID or is the user's own config.
  - The remaining G104 unchecked errors: best-effort cleanup (`Close`, removing a stale socket or pidfile, killing an already-exiting process), `Start`/`Restart`, whose failures show in the status, and writes to stdout.
  
  These are annotated in the code, so `make sec` reports nothing until something new appears: `// #nosec G204 -- <reason>` or `G304`, and `_ =` with a short comment for deliberately ignored errors. Annotate any new accepted finding the same way, with its reason, rather than leaving it in the report.
- **This file is public** (since 2026-10-04): keep personal details out of it, such as machine names, hostnames, home-directory paths and the setup before the launcher. Write "the development Mac" or "the LAN peer"; the specifics belong in `CLAUDE.local.md`, which is git-ignored. Reviews that write findings here follow the same rule.
- **How it was made:** the README says so openly (decided 2026-10-04): the code, tests and docs are written by Claude in Claude Code under the maintainer's direction, every feature is tested on real Macs before it's committed, and this file is the working brief. Keep that section true as the process changes. It's impersonal ("the maintainer"), like the rest of the README.
- `README.md` is the user documentation, and other projects' Claude sessions read it to learn how to drive llama-swap through `llsl`. Keep it in step with any change to `llsl`'s commands, flags, exit codes or JSON, to the preferences, or to the menu.
- `LLAMA_SWAP_BIN=/opt/homebrew/bin/llama-swap go test ./internal/health -run Real -v` runs the health monitor against a real llama-swap, with a stand-in model that fakes a GPU fault. Rerun it after upgrading llama-swap to catch API changes; when it passes against a release newer than `health.TestedVersion`, it says so, and that constant (and the README's "tested with") should move up. With the same variable, `go test ./internal/supervisor -run BinaryVersionReal -v` checks that the version is still read correctly (llama-swap's `-version` wording changed between v260 and v262).
- **Build through the Makefile, or set its `CGO_CFLAGS`/`CGO_LDFLAGS`.** Without `-mmacosx-version-min=15.0`, cgo targets the build machine's macOS (27.0 here), and the app won't launch on anything older, whatever Info.plist says. Check with `otool -l <binary> | grep minos`.
- **Debugging a stuck app:** `sample <pid> 1` works on the hardened-runtime dev build, no sudo, and shows AppKit's main thread. `kill -QUIT <pid>` makes Go write every goroutine's stack to `launcher.log` (stderr is redirected there) and ends the app; its llama-swap exits on its own, and the next launch's leftover cleanup covers the rest.
- The dev app's files: preferences in `~/Library/Application Support/com.my-wang.llama-swap-launcher.dev/prefs.json` (edited by the Preferences dialog, and hand-editable), logs in `~/Library/Logs/com.my-wang.llama-swap-launcher.dev/` by default. `launcher.log` holds the `[launcher]` notes and the app's stderr (panics), and by default llama-swap's output too (TODO item 22); `llama-swap.log` is llama-swap's own file when the preferences give it one (up to 0.1.2 it was the combined log).

## The problem this solves

macOS 15+ has **Local Network privacy**: outgoing connections to LAN addresses are blocked unless the *responsible process* has the Local Network privilege. (Ref: Apple TN3179.)

- macOS auto-allows: launchd **daemons** (not agents), anything running as root, and command-line tools run from Terminal/SSH (plus their children).
- Everything else needs the privilege, and macOS attributes it to the **responsible code**, meaning the app at the top of the process tree.
- Observed behaviour:
  - llama-swap started from a **tmux** session → LAN egress blocked, denied by the kernel (visible in Console). Inside tmux, third-party binaries such as llama-swap and Homebrew's `telnet` (both ad-hoc signed) get "no route to host", while Apple's own `/usr/bin/nc` gets through. So test the tmux problem with a third-party binary, never with `nc`.
  - Started from a plain terminal shell (iTerm, not inside tmux) → works.
  - Started by the Claude desktop app → works (Claude is the responsible app), but llama-swap dies when Claude quits.
- The privilege is keyed to the program's **code signature**. Unsigned or ad-hoc builds don't give macOS a stable identity. An Apple-issued signing identity (Developer ID) keeps the grant across rebuilds and updates.

**Fix:** a properly signed app that is always the parent of llama-swap. The Local Network grant is then attached to our app once and survives updates.

Explicitly rejected alternatives:
- Allowlisting subnets via `com.apple.network.local-network` defaults. The user wants to keep the privacy protection.
- Mac App Store. It requires the App Sandbox, sandboxed children can't run arbitrary user-configured binaries, and App Review forbids executing code that changes app functionality.
- A launchd daemon (a `/Library/LaunchDaemons` plist with `UserName` set). Daemons are exempt from Local Network privacy, so this is the obvious cheap fix, but it needs sudo to install, runs outside the user's login session with no UI or status, and isn't something to hand to other users.

## Architecture

```
┌─────────────────────── Llama Swap Launcher.app ──────────────────────┐
│  Menu-bar UI (systray)                                               │
│  Supervisor ── exec.Command ──▶ llama-swap (user-installed binary)   │
│     │                               └─▶ llama-server etc. (children) │
│  Health poller ── HTTP ──▶ llama-swap API                            │
│  Control server ◀── Unix socket ── llsl CLI (bundled)                │
└──────────────────────────────────────────────────────────────────────┘
```

**Core invariant:** only the app ever spawns llama-swap. The CLI never starts llama-swap itself. It asks the running app to do it, so the Local Network responsibility always belongs to the app, no matter who issued the command.

## Components

### 1. Menu-bar app (Go)
- Use `fyne.io/systray` (maintained fork of getlantern/systray; needs cgo on macOS). `systray.Run()` must own the main thread, so supervision and polling run in goroutines. Checked 2026-09-28: actively maintained, and its methods are safe to call from any goroutine.
- **Native shims:** systray only does the menu. The file pickers (NSOpenPanel), alerts (NSAlert) and launch at login (SMAppService) need hand-written cgo/Objective-C functions. Keep them in one package, each small and commented so someone who doesn't know Objective-C can review it.
  - AppKit may only be used on the main thread. systray owns that thread and our code runs in goroutines, so every shim must switch to the main thread itself: `on_main` in `common.h`.
  - **Reach the main thread through the run loop, not GCD's main queue** (found 2026-09-30). The main queue runs one block at a time, so while a dialog shown from a `dispatch_sync` block is open, every other main-queue block waits for it: the menu-bar title froze while any alert was up, and closing dialogs at Quit never ran. `on_main` uses `CFRunLoopPerformBlock` in the common modes (which include the one dialogs run in) plus a semaphore; systray's `performSelectorOnMainThread` works the same way.
  - **Menu items and dialogs:** while a dialog runs modally, AppKit only sends a menu item's action if its target answers YES to `worksWhenModal`. systray's items all target its `SystrayAppDelegate`, a plain `NSObject`, so every menu click (Quit included) was silently dropped while any dialog was open; the menu still opened, since that goes through an event monitor. `macos.MenuWorksDuringDialogs()` (`statusbar.m`), called in `onReady`, adds `worksWhenModal` → YES to that class at runtime. Found 2026-09-30 by sampling the stuck app, after harness fixes for the wrong cause.
  - **File panels are drawn by another process** (`openAndSavePanelService`). Ending the modal session (`abortModal`) leaves the panel on screen; close one with its own `cancel:`.
  - **Closing dialogs at Quit:** every dialog runs through `lsl_run_modal`, which counts the open ones. `CloseDialogs` closes them innermost first, and any dialog shown afterwards returns at once as cancelled, so a flow like Preferences can't reopen itself while the app quits.
    - It **waits** until they've closed, and `quit()` only calls `systray.Quit()` after that. systray quits with `[NSApp stop:]`, which during a modal session ends only the dialog's loop, and its own `sync.Once` means it never tries again: the app stayed running with llama-swap already stopped (found in hands-on testing, 2026-09-30).
    - A close requested while a menu is still tracking (right after a click on Quit) takes effect once the menu closes, about 0.8 s later, which is longer than stopping llama-swap usually takes.
    - If a dialog won't close within 5 s, `quit()` shuts down and exits directly.
  - **Testing shims:** a small clang-built harness that opens the real dialog and drives it from timers works well, but mirror the app's threading (main thread in `[NSApp run]`, dialogs opened from another thread) and deliver clicks through the run loop, not a GCD block. From inside a main-queue block a file panel's `runModal` returns at once and leaves the panel orphaned on screen. Have the harness close anything still open after a timeout, rather than someone clicking Cancel, which hides failures. Test what happens *after* too: the first close harness passed while Quit still failed, because nothing checked that the main loop then ended.
  - A menu-bar-only (`LSUIElement`) app must activate itself before showing a panel, or the panel opens behind other windows. Since macOS 14, `[NSApp activate]` is only a request, which macOS sometimes refuses: dialogs don't mind, since they sit above ordinary windows, but the About panel (an ordinary window) intermittently opened behind other apps' windows (2026-10-04; not reproducible in a harness). `about.m` also calls `orderFrontRegardless` on it. Any future non-modal window needs the same.
  - **Editing shortcuts need an Edit menu.** Text fields get ⌘X/⌘C/⌘V/⌘A/⌘Z through the main menu's Edit items, and a menu-bar-only app has no main menu, so the keys just beep (right-click still works). `ensure_edit_menu()` in `common.h` installs a hidden main menu with a standard Edit menu; every shim that shows a dialog or panel calls it first. It's never visible, because menu-bar-only apps don't get a menu bar.
  - **Tab in multi-line fields:** an `NSTextView` takes Tab as a character. The Preferences dialog's delegate turns Tab and Shift-Tab into moving to the next or previous field. Checkboxes and buttons are only reached by Tab with macOS's Keyboard navigation setting on; that's the system's behaviour, not a bug.
  - **JSON booleans from Objective-C:** box them as `@YES`/`@NO` (`cond ? @YES : @NO`). `@(a == b)`, and even `@(cond ? YES : NO)`, box a *number*, which becomes JSON `1`, and Go won't decode that into a `bool`. This silently broke the Preferences dialog's Save at first, because the Go wrapper also treated an undecodable reply as Cancel. The wrapper now returns an error, and the app reports it.
  - `progrium/darwinkit` (Go bindings for AppKit) was considered and rejected: unmaintained for years, and it crashes on Go 1.25 (progrium/darwinkit#286).
- Menu: status (with llama-swap's pid), its explanation (wrapped over up to four lines), the last GPU fault, loaded models (each with an Unload submenu) and Unload All Models, Start / Stop / Restart, Logs (a submenu: open or show in the Finder each log, Log Settings…, Collect Diagnostics…), Open llama-swap UI, About Llama Swap Launcher, Settings…, Launch at Login (checkbox), Quit.
- **"Settings…", not "Preferences…"** (decided 2026-10-04, for 0.2.0): Apple's name since macOS 13, so the app fits in. The menu item, the window's title and every message say Settings; the file stays `prefs.json`, and the code keeps its `prefs` names.
- **Preferences dialog** (`internal/macos/prefs.m`, `cmd/llama-swap-launcher/prefsdialog.go`): an `NSAlert` (Save, Cancel) with an `NSTabView` as its accessory view: General, an `NSGridView` form, and Secrets, two tables (TODO item 14). Binary and config have Choose… buttons; extra arguments and environment variables are multi-line fields, one per line, so arguments containing spaces are unambiguous.
  - The values cross to Objective-C and back as JSON through one C function, so all checking stays in Go. Entries that don't check out get an alert with the reason, then the dialog comes back with them and the reason shown until it's fixed.
  - Hand edits stay supported, in the user's own editor (an Open File… button was dropped at the user's request, 2026-09-30). `prefs.Load` rejects unknown keys (usually misspellings) and checks the listen address; a broken file gets an alert instead of the dialog.
  - While the dialog or an alert it led to is open, the menu offers only Quit (`app.inPrefs`), so nothing in the menu can open a second dialog or change llama-swap underneath it (`llsl` still can; see the TODO notes of 2026-10-02). Quit closes the dialogs (`macos.CloseDialogs`) before stopping llama-swap.
  - After Save, if llama-swap is running and a setting it runs with changed, the app offers to restart it. Changed health-check settings restart the health monitor straight away.
- Preference: start llama-swap when the app launches (default on). Without it, launch at login doesn't bring llama-swap up.
- On Quit: gracefully stop llama-swap (SIGTERM, then SIGKILL after a timeout).

### 2. Supervisor
- Launch llama-swap as a **direct child** via `exec.Command`. Do not go through `open`, `launchctl`, or a detaching shell wrapper, because that can break responsibility attribution.
- Capture stdout/stderr into a rotating log file under `~/Library/Logs/<bundle-id>/` (where exactly is a preference since TODO item 22). Capture through a pipe so the app controls rotation. Side effect: if the app dies, llama-swap exits the next time it writes output (SIGPIPE). That's intended, but it doesn't replace the crash cleanup below.
- Auto-restart on unexpected exit, with exponential backoff and a crash-loop cap.
- **Binary discovery:** GUI apps don't get the shell `PATH`. Check `/opt/homebrew/bin`, `/usr/local/bin`, and `~/go/bin`, and fall back to a user-chosen path. Persist the binary path, config path, and extra args in app preferences.
- **Child environment:** build llama-swap's environment explicitly instead of passing on the app's own, because the app's own depends on how it was started. Launched with `open` (including `llsl`'s `open -b`), an app inherits the caller's environment (see `man open`). Launched from Finder or at login, it gets launchd's bare one, with `PATH=/usr/bin:/bin:/usr/sbin:/sbin`. Testing only through `open` hides PATH bugs, so test a Finder launch too. Configs usually call `llama-server` by bare name, so:
  - build a sensible `PATH` (at least the discovery directories above);
  - let the user set extra env vars (e.g. `HF_TOKEN`) in preferences;
  - set the working directory to the config file's directory so relative paths work.
- **Stopping kills the whole tree:** llama-swap's children (llama-server etc.) can hold tens of GB of memory. If SIGTERM times out and llama-swap is SIGKILLed, they're left running. Stop must find and kill all descendants, and so must the handling of a crash, where there's no chance to look first: the supervisor keeps a snapshot of llama-swap's descendants (refreshed every second) and stops any survivors after every exit, before restarting. If llama-swap puts its children in their own process groups, killing llama-swap's group won't reach them. llama-swap also starts GPU-monitoring helpers of its own: a long-running `mactop` if it's on `PATH`, otherwise repeated `ioreg` calls. It finds `mactop` through `PATH`, so with a GUI app's bare `PATH` it silently falls back to `ioreg`. That's one more reason to build a proper `PATH`.
- **Leftovers after an app crash:** llama-swap and its children keep running and hold the port. Record llama-swap's pid in a pidfile, and on startup kill leftovers from the previous run before starting a new one. Check the pid still belongs to llama-swap before killing it. Found 2026-10-02: once llama-swap has died of that SIGPIPE, its model servers are orphans the tree can't reach, since it can only be walked while llama-swap is alive. Fixed 2026-10-04 (TODO item 15): the pidfile lists llama-swap's descendants too, kept current as the tree changes, and the next launch stops those still running with the same start time and name.
- **Quarantine:** if the user downloaded llama-swap via a browser, it may carry `com.apple.quarantine` and Gatekeeper may block it the first time. Detect this failure and show a clear explanation instead of failing silently.
- **Denied Local Network access fails silently:** there's no API to check the permission. Recognise the connection errors it causes (e.g. "no route to host") in llama-swap's output and point the user to System Settings → Privacy & Security → Local Network. llama-swap's peer proxy errors already contain `no route to host` plus a hint about Local Network permissions on macOS. Don't treat timeouts as a denial: a third-party firewall such as Little Snitch holds connections while its own prompt is up.

### 3. Health / state monitoring
- llama-swap's API, checked against v260 on 2026-09-28 and against its source on 2026-10-02 (**recheck after upgrades**; the real-llama-swap test helps):
  - `GET /api/events`: server-sent events. `modelStatus` events carry every model's `id`, `state` (`stopped`, `starting`, `ready`, `stopping`) and `peerID` (set for peer models). A snapshot arrives on connect, then one per change. The event's `data` field is JSON encoded as a string. The other message types in v260 are `logData`, `activity` (per-request token counts; TODO item 8), `inflight` (item 2), `uiConfig` and `profileChanged`.
  - `GET /health` returns `OK`, with no auth middleware.
  - `GET /logs/stream/<model-id>` streams one model's output; add `?no-history` for new lines only. `/logs/stream/upstream` mixes all models; `proxy` is llama-swap's own log.
  - `POST /api/models/unload/<id>` unloads one model; `POST /api/models/unload` unloads all.
  - Also there: `/running`, `/v1/models` (with `status.value` loaded/unloaded), `/api/version`, `/metrics`.
  - **Auth:** with `apiKeys` set, every route the app uses except `/health` needs a key: `/api/*`, `/logs*`, `/upstream/*`, `/v1/models`, `/running`, `/metrics` and `/ui/` (the route table in `internal/server/server.go`). It's accepted as `Authorization: Bearer`, as Basic's password, or as `x-api-key`. The app doesn't send one yet: TODO item 14.
  - Peers: nothing beyond `peerID` in `modelStatus`; no endpoint lists peers or their URLs. `/upstream/<peer>/<model>/…` is proxied to the peer, and its `/health` is answered by the peer's llama-swap (TODO items 1 and 4).
  - Flags: `-config`, `-config-dir` (additive to `-config`), `-listen`, `-watch-config`, `-validate`, `-version`, and TLS (`-tls-cert-file`, `-tls-key-file`). TLS flags in `args` would make llama-swap serve `https://`, which the app's `http://` monitor can't reach; not supported. v262's `-h` also lists `-listen-tailcat` (a second listener, given a private-key file), which the app neither watches nor forbids (TODO item 32).
- The monitor (`internal/health`) follows `/api/events` for model states and checks `/health` periodically, which catches a llama-swap that's hung but still holds the stream open. The menu shows "starting", "ready" or "not responding", lists loaded models with an Unload submenu each, and offers Unload All Models.
- The event stream has no heartbeat (llama-swap's `serveSSE` only sends real events), so hang detection needs requests, and with `logToStdout` including `http` each one is a log line. So (decided 2026-09-28):
  - the interval is a preference, `healthCheckSeconds`, default 60;
  - `healthCheckSkipWhenActive` (default true) skips a check when events have arrived since the last one;
  - checks carry the User-Agent `llama-swap-launcher-healthcheck`, and the app drops those lines from its copy of llama-swap's log (`logfile.DropLines`). They still show in llama-swap's web UI.
  - A hang therefore shows as "not responding" within about the interval plus 3 s.
- The app passes `-listen` itself (a preference, default `127.0.0.1:8080`), so it always knows where to connect.
- `/health` only covers the proxy. A known failure it misses: after a Metal GPU error, a llama-server backend logs `backend is in error state`, and every completion against it returns 500 while `/health` still says 200. The maintainer's earlier scripts grepped per-model log files for it. The app instead follows `/logs/stream/<id>?no-history` for each ready local model (llama-server writes to its own output as well as `--log-file`, and llama-swap keeps each model's output separately), so it needs no log paths, and `no-history` keeps faults from earlier runs out. A faulted model is marked in the menu, and an alert offers to unload it: that replaces its process and backend without restarting everything.
- **Config validation:** `llama-swap -config … -validate` checks a config without starting anything (exit 0 valid, 1 invalid with the reason; 2 and "flag provided but not defined" on versions without it). Start and Restart validate first, so a broken config leaves the running llama-swap untouched.

### 4. Control socket + CLI (`llsl`)
- Unix domain socket at `~/Library/Application Support/<bundle-id>/ctl`, mode `0600` (`control.sock` up to 0.1.2; renamed for TODO item 13). Optionally verify the peer UID with `getpeereid`. Unix sockets are not IP traffic, so Local Network privacy doesn't apply to them.
- Derive the socket, preferences and log paths from the bundle ID, so a dev build and an installed release can run side by side.
- Socket paths are limited to 103 bytes on macOS. The path is 71 bytes plus the username (75 for dev builds), so usernames up to 32 characters work (28 for dev). The app checks the length at startup and fails with a clear message.
- Binding the socket doubles as a single-instance lock. On startup, remove a stale socket left by a crash (connect fails → unlink it).
- The app takes the socket before anything else at startup: as the single-instance lock, it has to come before the pidfile cleanup, or a second instance would stop the first one's llama-swap. A second instance logs and exits.
- Protocol: one request and one response per connection, each a line of JSON (`internal/control`). A request's context ends when the client hangs up, so a waiting `restart -wait` stops waiting on its behalf. Requests from other users are refused (`LOCAL_PEERCRED`).
- Commands: `status`, `start`, `stop`, `restart`, `unload MODEL | -all`, `logs [-n N] [-f]`, all with `-json` except `logs`. `logs` reads the log file directly, so it works while the app is down. Only `start` and `restart` launch the app. Control requests report problems in their response instead of showing alerts, since the caller is usually a script or an agent.
- Exit status: 0 ok (for `status`: ready and no failed model); 1 failed (for `status`: not ready, or a model's GPU backend failed); 2 usage; 3 app not running or couldn't be launched; 4 config invalid, llama-swap left as it was; 5 model didn't load; 6 timed out; 7 busy (requests in flight, nothing done; `-force` overrides). On 1, 5 and 6, llsl also prints the last 20 log lines.
- **`restart --wait`**: block until llama-swap is healthy or has failed, then exit 0 or non-zero, and print the last N log lines on failure. This lets an agent edit a config, restart, and immediately know whether the config was bad. Use the same `-validate` step as the menu's Restart, so a bad config is reported without stopping the running llama-swap.
  - llama-swap loads models on demand, so "healthy" only proves the config parsed and llama-swap is listening, not that any model's `cmd` works. Add `--load <model>`: also load that model and wait until it's ready or has failed. Big models can take minutes to load, so give it a generous default timeout.
  - Do the waiting inside the app. It knows which llama-swap process it started, so the CLI can't mistake the old instance's health for the new one's.
  - Use distinct exit codes (e.g. app unreachable, llama-swap failed to start, model failed to load, timeout) and offer `--json` output. Agents will rely on both.
- If the app isn't running, the CLI launches it with `open -b <bundle-id>` (LaunchServices), **never** by running the app executable directly. In milestone 0, running the executable directly from tmux still got the app's permission, but that's undocumented behaviour; don't rely on it.
  - The CLI reads the bundle ID of the app it ships inside, so a dev CLI opens and talks to the dev app.
  - `open -b` can launch the wrong copy when several copies share an ID (build folder, /Applications, an old download). The separate dev ID avoids this.
- Make the CLI a separate, pure-Go binary (no cgo). CLI calls then don't load AppKit, it gets its own Mach-O UUID, and the code stays simpler. It ships **inside the .app** so it's signed and notarized with everything else.
- Context: llama-swap's config-watch mode picks up most config edits on its own, but some changes need a full restart. The CLI's `restart` covers those cases, plus upgrades and crashes.

### 5. Launch at login
- `SMAppService.mainAppService` register/unregister through `internal/macos/login.m`, shown as a "Launch at Login" checkbox. macOS owns the setting (System Settings → General → Login Items), so the checkbox shows what macOS reports rather than a preference.
- Observed on macOS 27 (2026-09-28): switching it off in System Settings makes `SMAppService` report **not registered**, not the "requires approval" the header describes. The menu then shows it unticked, and ticking it re-registers it. The "requires approval" branch (a note in the menu, and an offer to open Login Items) stays for the documented case.
- The checkbox is refreshed each time the menu opens (`systray.TrayOpenedCh`), since System Settings can change it at any time. Without that, it only caught up at the next status or health update.
- The SDK header says apps using `SMAppService` must be code signed; dev builds are. Login items can be reset with `sfltool resetbtm` (unlike Local Network entries), but that resets them for every app, so use it sparingly.

### 6. Icons
- **Menu bar** (decided 2026-09-29): the text **L→S**, where the arrow's colour is the status.
  - The letters are in the menu bar's normal text colour. The arrow is the same colour when llama-swap is working, grey while something is in progress (llama-swap starting, stopping or restarting, or a model loading or unloading), and red when something needs attention (failed, not responding, a model's GPU backend failed). All of it is faded when llama-swap is stopped: letters and arrow in the busy grey (`secondaryLabelColor`), since `tertiaryLabelColor` proved too faint to see. `lookFor` in `cmd/llama-swap-launcher/icon.go` does the mapping.
  - It's an attributed title, not an image: a template image is drawn in a single colour, so it can't have a red arrow between black letters. The letters use dynamic label colours, which follow the menu bar's light or dark look; the arrow is the SF Symbol `arrowshape.right.fill`.
  - systray doesn't expose its menu-bar item, so `internal/macos/statusbar.m` finds the item's button among the app's windows by its public class, `NSStatusBarButton`. If that ever fails, the app falls back to plain "L→S" with `…` or `⚠`.
- **App icon** (real art since 2026-09-29): the source is `packaging/art/llama-swap-launcher.png`, a centre llama with arrows pointing out to eight others, on a transparent background.
  - `make icons` (`packaging/mkicon`, its own Go module so its image library stays out of the app's dependencies) makes two 1024 px masters on Apple's macOS grid (an 824 px cream rounded square, corner radius 185 px): `AppIcon.png` with the whole picture, and `AppIcon-small.png` with only the centre llama. Commit both.
  - The plate keeps the art the same size as other apps' icons, and keeps its navy arrows visible on dark backgrounds.
  - The small master is for 16 and 32 pt, where nine heads become a speckle; those are the sizes in System Settings' Local Network and Login Items lists. `make` builds `AppIcon.icns` from both with `sips` and `iconutil` (`CFBundleIconFile`).
- On macOS 26+, an Icon Composer `.icon` file would give the system's glass look; compiling one with `actool` outside an Xcode project is untested. Verify before relying on it.

## Bundle layout

```
Llama Swap Launcher.app/
  Contents/
    Info.plist
    MacOS/llama-swap-launcher
    Helpers/llsl
    Resources/AppIcon.icns
```

The bundle name contains spaces. Make can't handle targets with spaces, so don't use the `.app` path as a Make target (use phony targets or a stamp file), and quote the path in every recipe and script.

Info.plist essentials:
- `CFBundleIdentifier`: **choose once and never change it.** macOS has no way to reset a Local Network grant, so a changed ID means a messy duplicate entry. Tentative value and dev variant: see "Current status and decisions".
- `LSUIElement` = `true` (menu-bar only, no Dock icon)
- `NSLocalNetworkUsageDescription`: user-facing reason shown in the permission prompt
- Version keys, minimum OS (macOS 15+)

Also make sure the main executable has a **unique Mach-O UUID** (TN3179 notes local network privacy misbehaves without one; see TN3178). Because systray needs cgo, the app is linked by Apple's `ld`, which emits one. Confirm with `otool -l <binary> | grep -A2 LC_UUID`.

## Signing, notarization, distribution

- Apple Developer Program membership ($99/yr) → **Developer ID Application** certificate.
- Build with hardened runtime (`codesign --options runtime --timestamp`). Hardened runtime limits what loads into *our* process; it doesn't stop us from launching an unsigned llama-swap.
- Sign inner binaries first (Helpers), then the bundle. Avoid `--deep`.
- Notarize with `xcrun notarytool submit --wait`, then `xcrun stapler staple`.
- Package as a `.zip` for GitHub Releases (decided; see Current status).
- Automate via a Makefile (local) and a GitHub Actions release workflow (secrets: signing cert .p12, notarytool API key).
- **Later:** Homebrew cask. Official casks now must pass Gatekeeper, meaning signed and notarized. Use the cask `binary` stanza to symlink `llsl` onto `PATH`. The official cask repo also has popularity requirements, so start with our own tap.
- **Later / optional:** Sparkle for in-app updates.

### Development builds

- **Sign dev builds with the Developer ID certificate too**, under the `.dev` bundle ID and display name. The stable identity means the dev app gets one Local Network entry that survives rebuilds, kept separate from the release ID. Skip notarization and use `--timestamp=none` to avoid a network round trip on every build. Keep hardened runtime on so problems show up early.
- **Sign the whole bundle**, inner binaries first. Go only signs the bare executable; signing the bundle makes the bundle ID the signature's identity.
- **Select the identity by its full name** (`Developer ID Application: … (P56KW9H72P)`) in a Makefile variable. A partial match becomes ambiguous once a renewed certificate or an Apple Development certificate is also in the keychain.
- **Contributors without the certificate** can build ad-hoc by setting that variable to `-`. Local Network behaviour isn't reliable that way: ad-hoc builds get a new identity each time, and the entries they create in System Settings can't be removed.

## Non-goals

- Bundling or redistributing llama-swap or llama.cpp.
- Mac App Store distribution.
- Running as a launchd daemon or agent.
- Any change to system-wide network privacy settings.

## Suggested milestones

0. **Prove the approach before building anything.** Done 2026-09-28; see "Milestone 0 results". A throwaway `.app`, signed with the Developer ID certificate under the `.dev` bundle ID and dev display name, whose only job is to launch a child that connects to a LAN host. Launch it with `open` from inside tmux. Repeat with the real llama-swap binary as the child, since someone else's Go toolchain built it. Using the `.dev` ID means the grant carries over to the real dev app, which also shows whether a grant survives a rebuild.
1. Menu-bar skeleton + supervisor (start/stop/restart, logging, binary/config pickers). Developer ID-signed dev build with the `.dev` bundle ID. Done 2026-09-28; see "Milestone 1 results".
2. Health polling + status/model list in the menu. Done 2026-09-28, along with GPU-fault detection and config validation; see "Milestone 2 results".
3. Control socket + CLI, including `restart --wait` and `open -b` auto-launch. Done 2026-09-28; see "Milestone 3 results".
4. `.app` bundling, Info.plist, launch-at-login. Done 2026-09-28 with placeholder icons; see "Milestone 4 results".
5. Release signing (release bundle ID, secure timestamp) + notarization in the Makefile; verify the Local Network prompt appears under the app's name and the grant survives a rebuild. Done 2026-09-28; see "Milestone 5 results". The grant surviving a rebuild was shown with the Developer ID-signed dev build in milestone 0, and the release build uses the same identity scheme.
6. GitHub Actions release pipeline.
7. Homebrew cask.

## Milestone 0 results (2026-09-28)

Test app: `spike/localnet/`, Developer ID-signed under the `.dev` bundle ID, launched with `open` from tmux, connecting to the LAN peer.

- **The app's permission covers its children.** The first LAN connection came from a child (`nc`). The prompt named "Llama Swap Launcher Dev" and showed our `NSLocalNetworkUsageDescription`. Until Allow, children got "no route to host"; after it, everything got through: `nc`, a Go child and its `nc` grandchild, and llama-swap proxying a real completion from the peer through its `peers:` entry.
- **The permission survives a rebuild.** A new build under the same bundle ID got no prompt and reached everything.
- **A newly built binary's first connection can fail once.** After the rebuild, the new binary's first connection got "no route to host" and worked 3 seconds later. Unchanged binaries got through at once. Expect the same once after a llama-swap upgrade.
- **Running the executable directly from tmux also worked**, children included, although third-party binaries are otherwise blocked in that tmux. macOS applied the app's permission anyway. This is undocumented, so `llsl` still launches the app with `open -b`.
- **`open` passes on the caller's environment.** llama-swap found `mactop` on `PATH` when the app was launched with `open`. See Supervisor → Child environment.
- **Little Snitch prompts separately** for llama-swap's connection and holds it while the prompt is up, so the first request timed out rather than failing.

## Milestone 1 results (2026-09-28)

Hands-on test of the dev app with the maintainer's real config:

- **Launch:** both `make run` (`open` from a shell) and a Finder double-click work. llama-swap was found automatically, and the built PATH resolved `llama-server` and `mactop` even from Finder, where the app gets launchd's bare environment.
- **LAN:** from tmux, a request for a peer model (`peer/model`) through `127.0.0.1:8080` reached the peer with no prompt. The permission carried over from milestone 0's `.dev` build.
- **Crash:** after `kill -9` of llama-swap with a model loaded, the supervisor stopped the orphaned `llama-server` and `mactop` with SIGTERM (they had exited within 3 s), then restarted llama-swap after 1 s. No duplicate model server.
- **Stop and Quit** with a model loaded: llama-swap exited 0 after SIGTERM and stopped its own `llama-server`; nothing was left running.

## Milestone 2 results (2026-09-28)

Hands-on test of the dev app with the real config:

- **Status:** "starting" turns into "ready" too quickly to see on this machine. A large model showed "loading", then ready.
- **Models:** several models listed at once with their own states. Per-model Unload and Unload All Models both worked.
- **Hung llama-swap:** after `kill -STOP`, the menu showed "not responding" and "LS ⚠"; after `kill -CONT`, it went back to "ready" and "LS ●".
- **GPU fault:** with a stand-in model that prints the fault marker, the model got a ⚠ and the alert offered to unload it; unloading cleared it.
- **Bad config:** choosing an invalid config while running gave the validation alert with llama-swap's reason, and left the running llama-swap untouched.

## Milestone 3 results (2026-09-28)

Hands-on test of `llsl` from a tmux pane, with the real config:

- `llsl status` with the app not running: "not running", exit 3.
- `llsl start -wait` launched the app through LaunchServices and returned once llama-swap was ready (exit 0).
- **End-to-end:** from the same tmux pane, a request for a peer model through `127.0.0.1:8080` returned HTTP 200. llama-swap started from tmux via `llsl` reaches the LAN, which is the problem this project exists to solve.
- `start -load gemma4-e2b-q4` loaded the model and reported it ready; `status` listed it; `unload` unloaded it.
- `restart -wait` came back ready with a new llama-swap pid while `logs -f` followed along in another pane.
- `stop` stopped llama-swap; `status` then reported "stopped" with exit 1.

## Milestone 4 results (2026-09-28)

Hands-on test of the dev app, with placeholder icons:

- **Menu-bar icon:** the icon alone when ready, faded when stopped, `…` beside it while restarting.
- **App icon:** shown in Finder and in System Settings' Local Network list.
- **Launch at Login:**
  - Ticking it adds the app to System Settings → General → Login Items; unticking it removes it.
  - Switching it off in System Settings shows as unticked in the menu (see Components → Launch at login for why there's no note). After the menu-open refresh was added, the change shows as soon as the menu opens.
- **Launch at login after a reboot:** verified 2026-09-28 on the second Mac with the release build.

## Milestone 5 results (2026-09-28)

- `make release` built 0.1.0. Apple notarized it at the first submission (log: "Ready for distribution", no issues). The ticket is stapled, `spctl` reports "accepted, source=Notarized Developer ID", `syspolicy_check distribution` passes, and neither binary contains local paths.
- **Installed from the zip on two Macs:**
  - the development Mac, where the dev build had run;
  - the second Mac, which had never run the app. The zip was marked as downloaded (quarantine attribute), and opening it showed only the normal "downloaded from the Internet" question; Gatekeeper didn't block it.
  
  Both worked.
- **LAN traffic both ways through the release app:**
  - Outgoing: the development Mac's llama-swap proxying to the peer.
  - Incoming: a browser on the development Mac reaching the llama-swap the peer's app runs, listening on all interfaces.
- **Firewall prompts:** each Mac's firewall prompted once, naming **Llama Swap Launcher.app** (for llama-swap's connections too), and hasn't asked again. A stable signature also keeps third-party firewall rules working.

## Verify before relying on it

- Checked 2026-10-04: the real-llama-swap test passes against v262, so `health.TestedVersion` is 262.
- Checked 2026-09-28 against v260: llama-swap's HTTP endpoints and payloads (see Components §3), the config-watch flag (`-watch-config`) and `-validate`. Checked again 2026-10-02 against the v260 source: the auth-protected route list, the event message types and the flags in Components §3, and that llama-swap handles only SIGINT, SIGTERM and SIGHUP, so SIGPIPE ends it without cleanup.
- Checked 2026-09-28: llama-swap starts each model server in its own process group (`Setpgid`), so killing llama-swap's group doesn't reach them; `mactop` stays in llama-swap's group. On SIGTERM, llama-swap stops its `llama-server`s itself. On SIGKILL, the `llama-server` is orphaned (parent becomes pid 1) and keeps running. The supervisor therefore snapshots the tree by parent pid before signalling.
- `fyne.io/systray`: maintenance checked 2026-09-28 (active). Recheck its API when starting milestone 1.
- Whether each built binary carries a unique LC_UUID (`otool -l <binary> | grep -A2 LC_UUID`). Checked 2026-09-28: pure-Go binaries like `llsl` (Go 1.27's own linker) and the cgo app binary (Apple's `ld`) both get one. Pure-Go builds get the same UUID for identical builds and a different one per program.
- Checked 2026-09-28: the Developer ID designated requirement is the bundle ID plus team ID `P56KW9H72P`, with no certificate hash, so a renewed certificate keeps the Local Network permission.
- Performance: compare tokens/sec with llama-swap launched by the app vs from Terminal. If the app-launched one is slower, try `NSAppSleepDisabled` in Info.plist or holding an `NSProcessInfo` activity.
- The end-to-end test: launched from a tmux session via `llsl start`, llama-swap can reach a LAN host. Passed 2026-09-28 with the Developer ID-signed dev build; the notarized release build's LAN access also worked (milestone 5).

## Reference

- Apple TN3179, Understanding local network privacy: https://developer.apple.com/documentation/technotes/tn3179-understanding-local-network-privacy
- Homebrew Acceptable Casks: https://docs.brew.sh/Acceptable-Casks
- darwinkit crash on Go 1.25: https://github.com/progrium/darwinkit/issues/286
