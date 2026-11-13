# Roadmap

The open TODO items, grouped by the release they're meant for. Finished items are in [done.md](done.md), and [CLAUDE.md](../CLAUDE.md) is the project's brief: a section named here (Components §1, "Signing, notarization, distribution") is CLAUDE.md's. An item cited by number that isn't here is in done.md.

Items keep their numbers; a new one takes the next, from 94. When one is done, it moves to done.md, in number order, with what was built and how it was verified, and CLAUDE.md's list of open items changes with it. A review adds its findings here as a dated group, introduced by what it covered and how. Like CLAUDE.md, this file is public: keep personal details out of it.

## Next release

0.3.6 was published on 2026-11-10: the app waits at launch for llama-swap's files on a disk that mounts after login (item 91). 0.3.5 (2026-11-08): Fetch Models removes models a peer no longer serves (item 88), a config reload no longer flashes a notification (item 89), and small fixes from three security reviews of 0.3.4 (items 81 to 87). 0.3.4 (2026-11-06) let item 51's GPU memory limit be turned on where it had never been used. 0.3.3 finished the 0.3 series: items 51 (after two security reviews, items 66 to 72 and 74 to 80) and 58 to 65, with milestone 7, the Homebrew cask from our own tap. The finished ones are in [done.md](done.md). The next big step is 0.4 ([below](#04), item 47).

<!-- Item 92 starts a list of its own: the items above it are done. -->
92. **Don't show Metal's figure as macOS's default** (the maintainer, 2026-11-10; found with item 90's check).
    - **The problem:** with the limit at 0, Settings → Tweaks and `llsl status` say "macOS's default, about N GB". N is Metal's `recommendedMaxWorkingSetSize`, read at launch. On the Mac with 256 GB it said about 240 GB, which the maintainer doubts is the real default wired limit; on the development Mac it said 79626 MB of 96 GB (81%). The two don't fit one rule, and nothing exposes the true default: `sysctl iogpu` only shows the 0. Metal's figure is its estimate of what the GPU can use without hurting performance; it follows a limit once one is set, but at 0 nothing says it equals the kernel's default.
    - **It decides nothing:** the suggestion is the Mac's memory less a reserve. It only shows in the Tweaks line, in `llsl status`, in the JSON's `defaultMB` and in the diagnostics.
    - **Decided (the maintainer):** drop it. "It doesn't buy anything keeping it there other than a number that may or may not be valid. The number we set, we absolutely can display." So: "Now macOS's default." in Tweaks, "macOS's default" in `llsl status`, no `defaultMB` in the JSON (the README's field list follows, and the changelog says so), and none in the diagnostics. Remove `noteDefault`, `GPUWorkingSetMB` and the Metal framework, if nothing else uses them.

## Blocked

53. **Browser login through a form, on the Security tab** (the maintainer, 2026-10-28; **blocked** on a pull request the maintainer is submitting to llama-swap, from their own `form-auth` branch; build only against a llama-swap release that has it, since the options may change in review). With API keys, llama-swap asks a browser for one with the Basic auth dialog; the branch adds `security.auth`, whose `mode: form` sends the browser to a login page at `/ui/login` instead, which password managers fill.
    - **The options** (read in the branch's `internal/config/security.go` and `docs/config.example.yaml`, 2026-10-28):
      - `mode`: `basic` (the default) or `form`. Form needs `apiKeys`, or llama-swap refuses to load ("mode \"form\" requires apiKeys"). In form mode a browser never gets the Basic dialog, Basic credentials it sends are ignored, and other sites can't frame llama-swap's pages.
      - `sessionTTL`: seconds a login lasts unused. Default 604800 (7 days); at most 17280000 (200 days), and 0 means that maximum. Loading a page or doing something renews it; background polling doesn't.
      - `sessionMaxLifetime`: the longest renewal can keep one login alive, from logging in. Default 2592000 (30 days); 0 means no cap; otherwise at least `sessionTTL`, and at most 10 years.
      - `sessionSecret`: signs the sessions; at least 32 characters. Without one, llama-swap makes a random secret per process: a reload keeps browsers logged in, a restart logs them out (`processSessionSecret`, `internal/server/session.go`). Changing it logs every browser out.
      - The last three are ignored in basic mode. The maintainer tests with `mode: form`, the secret from an environment variable, `sessionTTL: 86400` and `sessionMaxLifetime: 0`.
    - **In the app:** an Authentication section on the Security tab (named after llama-swap's `security:` block, item 38), written to `security.yaml` in the managed folder, which it needs, like the rest of the tab.
      - All four are single values, which clash when the user's own config sets them differently (item 34), so each is written only when it differs from llama-swap's default: `mode` only for `form`, the two lifetimes only when changed (pointers, like the peer timeouts and `maxAge`), the secret only when set.
      - **The lifetimes** (decided by the maintainer, 2026-10-28): sliders showing minutes, hours and days, each with fixed bounds, and a checkbox that sets 0 and greys the slider out.
        - `sessionTTL`: 5 minutes to 200 days; the checkbox is "the maximum" (0, which llama-swap reads as 200 days).
        - `sessionMaxLifetime`: 3 hours to 2 years; the checkbox is "no limit" (0). The app's bounds are narrower than llama-swap's (1 second up, and 10 years), and `prefs.Validate` holds hand edits to them too.
        - A linear slider over 5 minutes to 200 days can't land on anything useful, so it moves between fixed stops (5, 15 and 30 minutes, 1, 2, 6 and 12 hours, 1, 2, 3, 7, 14 and 30 days, and so on up to the bound; `NSSlider`'s tick marks with `allowsTickMarkValuesOnly`), with the value written beside it ("7 days"). Both defaults are stops. A hand-edited value between stops is shown as it is until the slider moves.
        - llama-swap refuses a `sessionMaxLifetime` shorter than `sessionTTL`, "the maximum" included, so moving one slider pushes the other along (decided by the maintainer, 2026-10-28): the session length up past the lifetime raises the lifetime with it, and the lifetime down below the session length lowers it. Ticking "the maximum" with a lifetime under 200 days raises the lifetime to 200 days. The dialog can't be set to something llama-swap refuses.
      - **The secret** (decided by the maintainer, 2026-10-28): kept in the keychain, and easy for the user to see and copy, so the same secret can go on several llama-swaps (by hand or on another Mac's app). So it works like an API key: dots, with Show to reveal it, item 54's Copy button, Generate for a new one (`SecRandomCopyBytes`, comfortably over 32 characters), and pasting one in from another instance. llama-swap gets it in an environment variable the app reserves, such as `LLSL_SESSION_SECRET`, with an API key named `SESSION_SECRET` refused so they can't collide. Save checks the 32 characters and refuses `${` (item 42). A fixed secret matters more under the app than by hand: without one, every restart the app offers logs browsers out.
        - **Sharing a login** (read in the branch's `session.go`): a cookie is accepted by every instance with the same secret and the same key, since the cookie's name and signature come from both. The browser still sends a cookie only to the host name it came from, any port: instances on one Mac, or behind one host name (a reverse proxy), share a login; Macs at different host names each need their own login, with the same key as the password.
      - **Form needs keys** (decided by the maintainer, 2026-10-28): a hard requirement, as for Basic login: the form choice is enabled only with Require these keys on (and so the managed folder), and Save refuses form with it off.
    - **The app's own requests are unaffected** (read): llama-swap treats a request as a browser's only with `Sec-Fetch-Mode`, `Origin`, `Referer`, a page load or one of its session cookies (`isBrowserRequest`), and the app's Go client sends none, so its Bearer key works as before, and item 27's keyless probe still gets a 401. Check against the release.
    - **An older llama-swap ignores the section silently** (unknown keys are ignored, item 34), so browsers would keep the Basic dialog with no error. Record the first release with it (beside `health.TestedVersion`), and grey the section out, with a hint, when `BinaryVersion` is older.
    - **With item 50:** a change here is a file change, so a watching llama-swap reloads it without a restart, and a reload keeps logins. A secret kept in an environment variable is read only at start, so changing it is a restart.
    - **Also:** Open llama-swap UI lands on the login page; the README's note that the browser asks for "any user name, and a key as the password" gets a form-mode version. Collect Diagnostics must redact a literal `sessionSecret` (check the redactor's names cover it). `llsl status` could show the mode. Session cookies cross the LAN unencrypted over plain HTTP, as keys do (item 28); llama-swap sets `Secure` only over HTTPS, which item 35 would bring.
    - **For the security review:** how `allowCredentials` (item 38) combines with session cookies, letting a listed page make requests with the browser's login.
    - **Tested by:** a `TestManagedAuthReal` against the release, like `TestManagedCORSReal`: the login page for a browser-like request, the app's Bearer and the keyless 401 unchanged, a session kept across a reload and, without a secret, lost across a restart.


<!-- Item 93 starts a list of its own: 54 to 92 are elsewhere. -->
93. **Keys for model servers, through llama-swap's `proxyApiKey`** (the maintainer, 2026-11-13; **blocked** on the maintainer's `proxy-api-key` branch of their llama-swap fork reaching a llama-swap release; it cites llama-swap's issue #1103, with no pull request yet).
    - **What the branch adds:** `models.<id>.proxyApiKey`, the key llama-swap presents to a model's own server, for one started with a key of its own (llama-server reads `LLAMA_API_KEY`).
      - It's sent as `Authorization: Bearer` and `x-api-key` on every request llama-swap makes to that server, health checks and capability detection included, and replaces any key the client sent.
      - So a browser logged in with Basic auth (or item 53's form) can use the Playground and `/upstream/<model>/` against a server that requires a key.
      - A key with whitespace is refused when the config loads, and llama-swap's redaction covers it.
    - **Already fine in the app (checked 2026-11-13):**
      - **Collect Diagnostics' redaction:** a literal `proxyApiKey` value is redacted, a `${env.…}` reference kept, and `LLAMA_API_KEY=…` in `env` redacted.
      - **Usable by hand, and in use:** the maintainer's own config, run with a llama-swap built from the branch, gives every model one shared key. It's a secret variable under Settings → Secrets, which the app passes to llama-swap's environment. Each model passes it to its server under the engine's own variable in `env` (`LLAMA_API_KEY=${env.NAME}` for llama-server, `SPLASH_API_KEY=${env.NAME}` for Inco AI Splash, both checked by the maintainer), with `proxyApiKey: ${env.NAME}` beside it. The README's Secret environment variables section can say so once it's released.
    - **For item 47, models managed by the app:**
      - **A per-model option, "the server requires a key":** the app generates the key, keeps it in the keychain as it does its own API keys, and writes both lines.
      - **Through `env`, never `--api-key` in `cmd`:** a command line is visible to every user on the Mac; a process's environment only to its owner and root.
      - **Engine-specific:** the variable's name belongs in item 47's description of each engine: `LLAMA_API_KEY` for llama-server, `SPLASH_API_KEY` for Splash; an engine without one doesn't offer the option.
      - **One shared key to start with,** as the maintainer uses: one keychain item for every model's server. A key per model only if someone asks for it.
    - **Version gate:** llama-swap ignores config keys it doesn't know (checked with v272: a config with `proxyApiKey` validates), so on an older llama-swap a key-protected model would just fail with 401. Offer the option only for a llama-swap version that has it, as the menu hides Pause before v268. The version is known once it's released.

## 0.4

47. **The next configuration push** (the maintainer, 2026-10-18; not planned in detail yet, "a lot more work").
    - **The order follows the dependencies:** models, with the app's presets, then routing (groups, matrix, profiles and selectors, which name models).
    - **No macros** (decided by the maintainer, 2026-11-02). The app writes no `macros:` section. Shared settings are the app's own **presets**, written out in full into each model that uses one. llama-swap expands macros when it loads the config, so what runs is the same either way; only the file differs.
      - **Why:** macros keep hand edits consistent, and nobody hand-edits files the app writes. A preset can do more than a macro: set keys such as `ttl` or `capacities`, which a macro can't add, and override a flag another preset sets, where `${a} ${b}` would pass it twice. Each model's `cmd` then shows exactly what runs, `-validate`'s errors point at the model, and the app's names can't clash with the user's own macros.
      - **What stays:** llama-swap's own `${PORT}`, `${MODEL_ID}` and `${PID}`, which it assigns, and `${env.NAME}` for secrets, so their values never reach the file (the app already passes the Secrets tab's as environment variables; llama-swap refuses a config naming one that isn't set).
      - **Importing a config that has macros** is where the app has to understand them: it reads them, never writes them. The maintainer's own show three kinds. Machine paths through `${env.…}` (the server binary, the models folder) become settings. Half an argument (`--model '${models_home}'`, with the path after it) becomes a file picker. Only groups of flags become presets.
      - **A preset is a model without the model's own parts** (its file, aliases, capacities), so one editor serves both. A model lists its presets in order, its own settings override theirs, and a preview shows the final command, which preset each flag came from, and what was overridden: what macros gave a person reading the YAML.
    - **Not only llama.cpp** (the maintainer, 2026-11-02). Other inference engines run under llama-swap on macOS, MLX's among them, so the form needs enough that's generic not to be tied to llama-server. Only a model's `cmd` depends on the engine: everything else llama-swap has per model (aliases, `ttl`, `capacities`, `filters`, `useModelName`, `checkEndpoint`, …) means the same for any of them.
    - **The form's shape** (agreed as the starting design, 2026-11-02; the maintainer expects to adjust it once it's built and seen):
      - **The model's llama-swap settings,** the same for every engine: its ID, description, aliases, `ttl`, `capacities`, `evict_cost`, `useModelName`, `filters`, `capabilities`, `concurrencyLimit`, `env`, `unlisted`, timeouts.
      - **An engine,** the only part that knows a particular server:
        - its program, with any fixed arguments before the rest (Splash's `serve`), or a launcher such as `uv run`. Finding it also searches `~/.local/bin`, where uv and pipx put tools;
        - how it's given the port and host (`--port ${PORT}`, …);
        - how it's given the model, and what it takes: a file (GGUF), a folder (MLX's safetensors), or a Hugging Face name it downloads itself. That picks the form's field: a file picker, a folder picker or text. An engine can take more than one way, each with its own flags;
        - what llama-swap needs for it: the health path (`checkEndpoint`), and `useModelName` when the server expects its own name for the model in requests.
        
        The app ships llama-server's, and others once checked against the real program; **Other** lets the user answer the same questions for anything else.
      - **Arguments,** generic underneath:
        - an ordered list of flags and values (`--ctx-size 131072`, `--jinja`). A later preset, or the model's own setting, replaces an earlier one by the flag's name, which works for any program taking `--flag value` without the app knowing what the flag means. Free text at the end takes anything else, such as positional arguments.
        - **An engine's flag table,** optional, adds what the generic list can't: aliases (`-c` is `--ctx-size`), flags that add up rather than replace (`--lora`), and each value's type (a number, on or off, a choice, a file; Splash's `--max-context` takes `100K`), for proper fields and checking. llama-server's comes first; an engine without one works as plain rows.
      - **Presets:** one holding arguments belongs to an engine (llama.cpp's flags mean nothing to MLX); one holding only llama-swap settings (`ttl`, `filters`) suits every engine.
      - **llama.cpp only, as now:** the GPU-fault watch and measuring capacity read llama-server's log.
    - **Engines to check when building** (only llama-server is installed on the development Mac):
      - **MLX's `mlx_lm.server`:** its flags for port, host and model, its health path, and whether it serves whichever model a request names, which would make `useModelName` matter.
      - **Splash** (https://github.com/incoai/splash; Homebrew, `incoai/tap/splash`; Apple silicon M3 or newer on macOS 26.4 or later, so not every Mac the app supports): `splash serve`, listening on `127.0.0.1:8000` by default. `--model` takes a Hugging Face `OWNER/REPO:VARIANT`, or, with `--model-dir` (a GGUF file, or a GGUF or MLX folder), just the name the API reports. Also `--draft-model`, `--mmproj` and `--max-context` (`100K`, up to `1M`; sized automatically by default). Its `--port` and any host flag, its health path, and whether it checks the model name in requests, are in `splash serve --help`, not on its page (read 2026-11-02).
    - **No finding or downloading models** (the maintainer, 2026-11-03). Other Mac apps already do it, in a crowded field, and it isn't worth competing. The app uses the folders those apps download into instead, for its model pickers. Seen on the development Mac:
      - **LM Studio** records its folder in `~/.lmstudio/settings.json` as `downloadsFolder` (by default `~/.lmstudio/models`), laid out as publisher, repository, file. Read the setting rather than assume the default: the maintainer's is on another drive.
      - **The Hugging Face cache,** `~/.cache/huggingface/hub` (or wherever `HF_HUB_CACHE` or `HF_HOME` puts it), as `models--<owner>--<repo>/snapshots/<revision>/`. MLX models land there too.
      - **Unsloth Studio** keeps its own llama.cpp build in `~/.unsloth/llama.cpp`, so it's somewhere to find an engine, as well as models.
      - Which others (Jan, Ollama's blobs, llama.cpp's own `-hf` downloads) is to check when building.
    - **Routing: the capacity router only, at first** (the maintainer, 2026-11-01). It's the easiest of llama-swap's routers to manage from a UI: no expressions such as `(model1 | model2) & +list`, only numbers. The maintainer's own large-model Mac runs it, and their other Macs are to move to it from the matrix.
      - **Pools:** `routing.router.settings.capacity.pools`, named by the user, each with a size the user chooses: memory in MB, say, for a pool such as `unified`.
      - **Each model's size:** `models.<id>.capacities.<pool>`, its real requirement: as llama-server reports it, configured context included.
      - **Each model's `evict_cost`,** optional: by default its size in the first pool in GB (MB with the last three digits dropped), and left out when that's 0, which is llama-swap's own default. A cost in proportion to size fits the router's plan: it evicts the set with the lowest total cost, so it keeps big models, which are slow to reload, rather than small ones.
      - **What llama-swap enforces** (read in v269's `internal/config/capacity.go`, `model_config.go` and `docs/config.example.yaml`):
        - pools are named and have positive integer sizes, at least one;
        - a model's sizes and `evict_cost` are non-negative integers (YAML `!!int`: a fraction is refused, not rounded);
        - a model's size can't exceed its pool's, nor name a pool that isn't defined;
        - **a model with no `capacities` uses nothing**, so it loads on top of a full pool. The app should require a size in every pool for each model it manages.
        - The router plans to minimise the evicted models' total cost, then their number, then their IDs, and only among the models already loaded.
        - `capacity` can't be mixed with groups or a matrix, and `routing.router.use` is a single value, so the user's own config can't keep a matrix beside it: this belongs in the fully managed mode below.
      - **With item 51:** the app knows the GPU's wired memory limit, so it could suggest a `unified` pool of that size, and warn when a pool is bigger than the limit.
      - **Sizing a model,** to fill in its capacity rather than the user typing it. From research the maintainer had another Claude session do (2026-11-03, kept outside the repository), checked on the development Mac the same day.
        - **By hand so far** (the maintainer, 2026-11-01): load the model and read llama-server's log, which prints its projected use (`common_memory_breakdown_print`, "projected to use N MiB") when `--fit` is on, as it is by default.
        - **`llama-fit-params --fit-print on`** prints the same projection without loading anything. It runs llama-server's own fitting code on the file's metadata, takes llama-server's arguments (`-c`, `-ngl`, `-fa`, the cache types, `-np`, …), and prints `MTL0 <model> <context> <compute>` and `Host …` in MiB on stdout. Homebrew's llama.cpp installs it beside `llama-server`; feature-detect `--fit-print` in its `--help`.
          - **Measured:** 0.25 to 0.5 s a run, for a 12B and a 35B model at 32k and 128k context, so it can run as settings change. Gemma 4 31B's model figure, 33,382 MiB, is the one llama-server logged when it loaded it.
        - **The plain KV formula is far off for current models.** 2 × layers × context × KV heads × head size × bytes gives 24,576 MiB of context for Gemma 4 12B at 32k, where llama.cpp projects 992, because its sliding-window layers cache only their window. For Qwen3.6-35B-A3B it gives 2,624 MiB, where llama.cpp projects 702: it's a hybrid, and only every fourth layer keeps a cache that grows.
          - Reading the metadata layer by layer matched llama.cpp to the MiB for both, at 32k and 128k: `head_count_kv` as a per-layer array, `sliding_window_pattern` and `key_length_swa`, `full_attention_interval`, and the `ssm.*` state sizes. But those rules differ by architecture, and new architectures keep arriving.
        - **Neither counts a draft model or a vision projector.** For an MTP draft, llama-server's log says "failed to measure the memory of the extra model, fitting without it", and `llama-fit-params` refuses `-md` outright. So the app adds them itself, at least their file sizes (Gemma 4 31B's MTP draft is 491 MiB, its F32 projector 2,196 MiB), and passes `llama-fit-params` only the arguments it takes.
        - **`gguf-parser`** (gpustack/gguf-parser-go, MIT) estimates from a file's header, even remotely before a download.
          - As a Go library it would bring about a dozen direct dependencies (gonum, json-iterator, `x/crypto`, `x/exp`, `x/tools`, …) to an app that has two. As its 9 MB binary, it's someone else's code signed with the maintainer's Developer ID.
          - What it alone offers is estimating before a download, and the app won't download models (below).
        - **Decided by the maintainer** (2026-11-03, after the measurements above; the first take had been the formula by default, with `llama-fit-params` behind a Tweaks checkbox):
          - **`llama-fit-params` by default,** whenever it's beside the engine's `llama-server`. It's then the same build as the server, so its projection matches what runs. A model's size is its projection, plus the file sizes of any draft model and vision projector. There's no checkbox.
          - **The formula when it's missing** (LM Studio's bundled `llama-server`, say, or a custom build): read layer by layer for the architectures it knows, labelled an estimate, and otherwise the plain formula, labelled rough, which always errs high.
          - **No `gguf-parser`,** since the app won't help find or download models (below).
        - **`--fit` and models that share memory:** llama-server's `--fit`, on by default, adjusts arguments that aren't set, context included, to fit the GPU's free memory. On Metal that's counted per process, so (per the research, from llama.cpp's source; to check) each llama-server sees the whole budget free, whatever else is loaded. The app should therefore write `-c` and `-ngl` for every llama.cpp model it manages, so that what runs matches the size the router was given.
        - **The budget:** Metal's `recommendedMaxWorkingSetSize` (79,626 MiB on the development Mac's 96 GB, as llama-server's log also shows), or the wired limit when item 51 sets one. A pool sized to the GPU's limit compares with the `MTL0` figures; `Host` is ordinary memory.
        - **Other engines** (MLX, Splash, …) report no projection. Their sizes are typed in, or estimated from the size of their weights and labelled rough.
      - Groups, the matrix, profiles and selectors wait until the capacity router is in, if they're wanted at all.
    - **After those, the root single values:** `logLevel`, `logToStdout`, `logTimeFormat`, `logRequests`, `store`, `captureBuffer`, `startPort`, `globalTTL`, `healthCheckTimeout` and the rest.
    - **Things to settle then:**
      - `models`, `groups`, `profiles`, `selectors` and `matrix` merge by ID, so each entity lives wholly in one file, as peers do (item 36).
      - `macros` merge by name, and a name set differently in two files is an error. The app writes none, so the user's own can't clash with it.
      - Single values clash whenever the user's config sets them too, which may argue for a mode where the app writes the whole config rather than adding to the user's.
    - **The maintainer's direction (2026-10-18):**
      - Keep the folder (`llama-swap.d`) for every section except the root config, one file per section, numbered: `01-api-keys.yaml`, `02-security.yaml`, `03-models.yaml`, `04-routing.yaml`, `05-peers.yaml` (`01-macros.yaml` dropped with the macros, 2026-11-02).
      - In fully managed mode, the root config sits beside `prefs.json` in Application Support, and so does llama-swap's SQLite `store` when it's on. The logs stay in `~/Library/Logs`.
      - Fully managed means conflicts are resolved in Settings before anything is written. llama-swap's `-validate` stays the check before every start.
    - **Checked against v265's `merge.go`:**
      - The folder's files are read in name order (`sort.Strings`), so the numbers set it.
      - The order matters only for where list entries land (API keys, origins): macros are resolved after the merge, on the whole document, so the user's own macros work from any file, whatever the order.
      - The numbers are mainly for whoever opens the folder.
    - **Renaming today's files** (`api-keys.yaml`, `peers.yaml`, `security.yaml`) needs the app to delete its own old-named ones, those starting with its note, when it switches. Otherwise llama-swap reads both, and a peer defined twice stops it loading.
    - **Preloading models at startup** (set aside below) belongs with the managed models.
    - **Tailcat** stays later (item 37).

## Later

### HTTPS

35. **TLS: `-tls-cert-file` and `-tls-key-file` as settings** (the maintainer, 2026-10-18; later, not planned yet). Some people will want llama-swap on HTTPS, which the app refuses today (item 28).
    - **First:** a checkbox to turn it on, with a file picker for each file. Both flags leave the refused list and become settings, lifted out of `args` like `-watch-config` (item 34).
    - **The app's own requests** (the monitor, `-load`, the peer checks) must then speak HTTPS. Simplest: trust exactly the configured certificate, which also works when it doesn't name `127.0.0.1`. Open llama-swap UI uses `https://`. Other machines, and peers that point at this llama-swap, need to trust the certificate themselves.
    - **Later:** generating a self-signed certificate. Go's `crypto/x509` can make the key and certificate itself, so no `openssl` is needed. Store them `0600` in the app's folder.
    - **Pie in the sky:** Let's Encrypt. It needs a public DNS name. For a llama-swap on a LAN, only the DNS-01 challenge works, through a DNS provider's API. Renewals would need a llama-swap reload or restart.

### Tailcat

37. **Tailcat** (the maintainer, 2026-10-18; later, not planned yet, as the maintainer doesn't use it). llama-swap's own tunnel: a peer whose `proxy` is `tailcat://<token>`, with an optional `tailcatKey` file, and the `-listen-tailcat` flag (refused in `args` today, item 32). Peers (item 36) start with `http://` and `https://` only; Tailcat peers, and perhaps the listener, can follow.

## Considered and set aside

Considered for 0.3 and set aside (the maintainer, 2026-10-18):
- **Preloading models at startup** (`hooks.on_startup.preload`, a list, so it would merge safely): it names models the app doesn't manage, which can change between starts and reloads. It belongs with a managed model config, if one comes. The maintainer has ideas for that, from their own configs, but it needs a fully managed config, macros included; not now.
- **Single values** such as `logLevel`: llama-swap refuses to load when two files set one differently, so they'd need a "here or in your config, not both" hint. Hold until one is needed.
