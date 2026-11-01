# Roadmap

The open TODO items, grouped by the release they're meant for. Finished items are in [done.md](done.md), and [CLAUDE.md](../CLAUDE.md) is the project's brief: a section named here (Components §1, "Signing, notarization, distribution") is CLAUDE.md's. An item cited by number that isn't here is in done.md.

Items keep their numbers; a new one takes the next, from 57. When one is done, it moves to done.md, in number order, with what was built and how it was verified, and CLAUDE.md's list of open items changes with it. A review adds its findings here as a dated group, introduced by what it covered and how. Like CLAUDE.md, this file is public: keep personal details out of it.

## Next release

With item 52, which is built (CHANGELOG.md's Unreleased). The README's Settings screenshots, due to be retaken from 0.3.1, are better retaken from this release: item 52 changed the tables' height, and item 56 changes About.

54. **A Copy button for API keys and secret variables** (the maintainer, 2026-10-28; not started). In the row with + and − (and Generate and Show) under the API keys and the secret variables: select a row, click Copy, and its value is on the clipboard. Today it takes Show, selecting the revealed text, and ⌘C.
    - **Why** (the maintainer): their password manager handles Basic auth poorly, so they copy a key from the launcher every time they open llama-swap's web UI. Item 53's login page, once llama-swap has it, lets a password manager fill the key itself; this covers the time until then, and every other use of a key or secret.
    - **This reverses item 14's "no Copy button"**, which kept the app from ever writing the clipboard (the maintainer limits clipboard access where they can). Show and ⌘C put the value there anyway, with no protection at all, so a Copy button that adds the protections password managers use leaves a secret less exposed than today's way, not more:
      - **Not synced:** `prepareForNewContentsWithOptions:NSPasteboardContentsCurrentHostOnly`, so Universal Clipboard doesn't send it to the user's other Apple devices.
      - **Kept out of clipboard histories:** the `org.nspasteboard.ConcealedType` and `org.nspasteboard.TransientType` markers (nspasteboard.org's conventions), which clipboard managers such as Raycast, Alfred and Paste honour by not recording the item.
      - **Cleared after a while:** when the time is up, the app empties the clipboard, but only if nothing has been copied since. `NSPasteboard`'s `changeCount` goes up with every write by anyone, and can be read without reading the contents: the app keeps the count from its own write, and clears only if it's unchanged. So it never reads what else is on the clipboard, and anything copied since is left alone. Quitting the app clears it too, on the same condition. Clearing bumps the count itself, so a clipboard manager may record an empty entry, never the secret.
      - **How long** (the maintainer, 2026-10-28): a setting on the Tweaks tab (item 51; whichever of the two is built first creates the tab). A menu of fixed choices, in keeping with item 53's sliders: 10, 30, 60 and 90 seconds, 2 and 5 minutes. The default is 90 seconds (decided by the maintainer, 2026-10-28: time to click through an interface before pasting; the exposure is no more than the secrets already have in llama-swap's environment, item 14). No "Never" (decided by the maintainer, 2026-10-28): it would turn the protection off, and Copy is one click again when the key is needed later.
    - **The button:** "Copy", acting on the selected row like Show, greyed out with no row selected or an empty value. It copies the value as it stands in the table, unsaved edits included, as Fetch Models uses them (item 36). Nothing is logged.
    - **Also** for item 53's session secret, when that's built. Not for Environment, whose values are on screen already.
    - **Testing:** the test program mustn't touch the real clipboard (the maintainer's rule). The copy goes through a pasteboard the shim is given, so the test program passes `[NSPasteboard pasteboardWithUniqueName]` and checks it: the value, the two markers, the current-host-only option, and the clear after a shortened delay, with a newer copy left alone. The maintainer checks the real one by hand.
    - **After it's built,** the "Checked and found sound" line on secrets ("never writes the clipboard") changes to what the app writes and how, and the README's API keys section mentions the button and the 90 seconds.

55. **Name the app in every request it makes to llama-swap** (the maintainer, 2026-10-29, noticed in llama-swap's log; not started; to go in the same batch as item 54). Only the `/health` checks and `llsl status`'s peer checks carry a User-Agent (`health.CheckUserAgent`, `llama-swap-launcher-healthcheck`), which exists only so their lines can be dropped from the app's copy of the log (item 25). Everything else goes out as Go's default, `Go-http-client/1.1`: the event stream, the `/api/metrics/activity` lookups (item 8), the model log streams (the GPU-fault watch), unloads, `-load`, and the keyless `/api/version` probe (item 27). The `/running` poll of item 5's outdated-build check is the one that shows: once a minute while Mark models whose server program was updated is ticked, a `Go-http-client` line in the log.
    - **Every request names the app and its version:** `llama-swap-launcher/0.3.1`, set once in `health.NewClient`'s transport (a `RoundTripper` that adds the header when a request has none), so nothing can be missed. The `health` package gets the version from `main.version` (set by the Makefile; "dev" in a plain `go build`).
    - **The checks carry it too:** `llama-swap-launcher-healthcheck/0.3.1` (the maintainer: "tagging the version of the launcher in the poll is a nice touch"). `IsCheckLine` matches the tag with or without a version, so lines logged by an older app are still recognised.
    - **The `/running` poll counts as a check:** it gets the check tag, and `checkRequest` also matches `GET /running`, so the once-a-minute poll stays out of the app's copy of the log like `/health`. Item 25's rule stands: the tag and the request must match together, so a client spoofing the tag could hide only its own reads of `/running`, which change nothing.
    - **Fetch Models' requests to a peer** (another llama-swap, or a service such as OpenRouter) are the app's too, so they carry `llama-swap-launcher/<version>` as well; the peer's log then shows what's asking (the maintainer, 2026-10-29: fine to name the app to outside services). They go through their own client (`noRedirects`, item 39), so it needs the same transport.
    - **Tests:** every request through the shared client carries the tag unless it set one; `IsCheckLine` with and without the version, and with `GET /running`; a `POST` or another path with the check tag still kept. The real-llama-swap test reads the access lines back to check what llama-swap logs. The README's health-check notes, if any, and Components §3's line on the User-Agent change with it.

56. **The About panel: the credits' size, and llama-swap's whole version and build date** (the maintainer, 2026-10-31; not started; for the same batch as items 54 and 55).
    - **What's wanted, top to bottom** (below the icon and name):
      - **Version,** as AppKit draws it.
      - **The app's Built line** directly under it.
      - **llama-swap's version:** `Running: llama-swap v269` for a release, or the whole version for a build from source: `v269-2-g581d29`. The point (the maintainer, 2026-10-31) is that only a release reads as a bare `vNNN`.
      - **llama-swap's build date,** where Built is today: `llama-swap built 2026-11-01`. Its wording was confirmed by the maintainer from a mock-up (2026-10-31): a second plain "Built" would be ambiguous.
      - **A blank line,** then MIT License and the link, so the license doesn't read as llama-swap's.
      
      The two llama-swap lines show only while it runs, as today.
    - **The credits in 10 pt** (decided by the maintainer, 2026-10-31), with their colour unchanged.
      - **Why:** a test program that opens the standard panel and lists its views showed what AppKit uses. The name is bold 14 pt, and the version and the copyright line are regular 10 pt, in the primary text colour. The credits were 11 pt (`smallSystemFontSize`, item 21), larger than the version above them, which is what made the version look small.
      - **The result:** at 10 pt the whole panel is in AppKit's size. The version differs from the credits only in colour, theirs being secondary.
      - **Considered first, and dropped:** a 12 pt version, made by setting AppKit's label's font after the panel opens. It worked, but it relies on the panel's undocumented layout. This way needs only the panel's public options.
      - **The gap:** between the version and Built it's the panel's 8 points, a little more than between the credits' lines.
    - **llama-swap's version:**
      - **The problem:** `versionPattern` stops at `v269`, dropping a build's `-2-g581d29`.
      - **The fix:** widen it to git's describe form, `-<count>-g<hash>` with an optional `-dirty`.
      - **No bracketed commit in the panel** (the maintainer, 2026-10-31): the describe form names the commit already, and a release's is its tag. The panel shows the version's first word, as `llsl status`'s line already does.
      - **Elsewhere:** `llsl status -json` and diagnostics keep llama-swap's whole text, commit included. `VersionNumber` still reads 269.
      - **The width:** at 10 pt, a long one such as `Running: llama-swap v269-12-g581d29c-dirty` fits the panel's 268 points. With the bracket it would be 277 and wrap.
    - **llama-swap's build date:** `-version` already prints it (`built at 2026-11-01T03:36:54Z`, in v260's wording and v262's).
      - `BinaryVersion` returns it from the same run, and the status carries it beside the version, from the llama-swap that was started.
      - It's shown as the date in UTC, like the app's own Built line. Both read a day ahead for a build in the evening in the Americas.
      - The line is left out when the output has no date.
    - **Tests:**
      - the parser with git's describe forms and the date;
      - the test program, for the credits' size and order.

## 0.3.x

51. **The GPU's wired memory limit, kept across reboots** (the maintainer, 2026-10-26; not started; decided the same day, below). `sysctl iogpu.wired_limit_mb=N` lets the GPU wire more memory, so bigger models fit. It needs root, and it resets at every boot. Today the maintainer sets it with a LaunchDaemon written by hand in `/Library/LaunchDaemons`. The app should do it through **`SMAppService.daemon`** (the maintainer's choice, over an `osascript` password prompt at every change; SMJobBless is deprecated).
    - **What it takes** (from Apple's documentation as remembered; check it in the spike below):
      - A daemon plist in the bundle's `Contents/Library/LaunchDaemons/`, registered with `[SMAppService daemonServiceWithPlistName:]`. Its `BundleProgram` names an executable inside the bundle, and `AssociatedBundleIdentifiers` makes System Settings show the app's name.
      - The user approves it once, in System Settings → General → Login Items & Extensions (`SMAppService.openSystemSettingsLoginItems`); the status goes from `requiresApproval` to `enabled`.
      - A helper executable: a static plist can't carry a value that changes, so a small, single-purpose program in the bundle, signed with the rest, runs as root.
      - Dev and release builds get separate daemons (the label from the bundle ID, generated like `Info.plist`). Both enabled on one Mac would fight at boot.
    - **How the value reaches root** (decided: A, "more secure, more Apple style"):
      - **A:** the helper offers an XPC Mach service (`MachServices`) and accepts only the signed app, via `xpc_connection_set_peer_code_signing_requirement` (Team ID `P56KW9H72P` and the bundle ID). It keeps the value in a root-owned file, applies it on request, and again at every boot (`RunAtLoad`). The helper is Go, with a short C shim for XPC's blocks.
      - **B:** the helper reads a value the app writes in a file the user owns. Simpler, but any program running as the user could change the limit, within the bounds below.
    - **Safety** (decided by the maintainer): bounds checked in the app and again in the helper.
      - Always at least 8 GB left for macOS: the helper refuses less, whatever it's sent.
      - Refused outright on Macs with 32 GB or less.
      - The default reserve, which Settings suggests (total minus reserve) and warns below: 8 GB for 36, 48 and 64 GB; 12 GB for 96; 16 for 128; 20 for 192; 24 for 256; 32 for 512. A size between takes the row below it (384 GB: 24).
      - 0 means macOS's default. The helper retries for a few seconds when the setting doesn't exist yet early in boot, rather than the manual daemon's `sleep 10`.
    - **Reading needs no root:** `iogpu.wired_limit_mb` and `hw.memsize` with `unix.Sysctl…`, as item 5 does. Whether Metal's `recommendedMaxWorkingSetSize` follows the setting, and so could show what 0 means on this Mac, is to check.
    - **Turning it off** unregisters the daemon and sets 0 at once. Deleting the app leaves nothing in effect after the next reboot. Milestone 7's cask may need an `uninstall launchctl:` entry; check then.
    - **The maintainer's own LaunchDaemon** and the app's would both apply at boot, in no set order: remove it once the app's works. The app could note a value it didn't set.
    - **Also:** `llsl status` shows the limit, and only Settings changes it (with A, that's what keeps the change behind the signed app). Collect Diagnostics includes the current and stored values. The non-goal "Running as a launchd daemon or agent" is about the launcher itself; reword it so the helper doesn't read as a contradiction.
    - **The first root code in the project:** a security review before it ships, as before 0.2.1 and 0.3.0.
    - **Later:** with item 47's managed models, the app knows each model's files, and could warn when one won't fit under the limit.
    - **First, a spike** (validate before building): register a minimal daemon from the dev build, with the maintainer approving it in System Settings. Check that it runs as root, whether the app must be in `/Applications`, that the XPC signature check accepts the dev app and refuses another program, and, after a reboot, whether the setting exists when the daemon starts.
    - **Where:** a new **Tweaks** tab (the maintainer), for settings that are risky or fit no other tab; General has no room.
    - **When:** a 0.3.x release, before item 47's 0.4 work, is fine. Before it ships, the maintainer has Claude Fable (at Max effort) and some local models with million-token context review it hard.

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

## 0.4

47. **The next configuration push** (the maintainer, 2026-10-18; not planned in detail yet, "a lot more work").
    - **The order follows the dependencies:** macros first, then models (which use macros), then routing (groups, matrix, profiles and selectors, which name models).
    - **After those, the root single values:** `logLevel`, `logToStdout`, `logTimeFormat`, `logRequests`, `store`, `captureBuffer`, `startPort`, `globalTTL`, `healthCheckTimeout` and the rest.
    - **Things to settle then:**
      - `models`, `groups`, `profiles`, `selectors` and `matrix` merge by ID, so each entity lives wholly in one file, as peers do (item 36).
      - `macros` merge by name, and a name set differently in two files is an error.
      - Single values clash whenever the user's config sets them too, which may argue for a mode where the app writes the whole config rather than adding to the user's.
    - **The maintainer's direction (2026-10-18):**
      - Keep the folder (`llama-swap.d`) for every section except the root config, one file per section, numbered: `01-macros.yaml`, `02-api-keys.yaml`, `03-security.yaml`, `04-models.yaml`, `05-routing.yaml`, `06-peers.yaml`.
      - In fully managed mode, the root config sits beside `prefs.json` in Application Support, and so does llama-swap's SQLite `store` when it's on. The logs stay in `~/Library/Logs`.
      - Fully managed means conflicts are resolved in Settings before anything is written. llama-swap's `-validate` stays the check before every start.
    - **Checked against v265's `merge.go`:**
      - The folder's files are read in name order (`sort.Strings`), so the numbers set it.
      - The order matters only for where list entries land (API keys, origins): macros are resolved after the merge, on the whole document, so a model in `04-` can use a macro from `01-` whatever the order.
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

### Milestone 7

The Homebrew cask, from our own tap, with a `.dmg` beside the zip: the plans are in CLAUDE.md, "Signing, notarization, distribution". It waits until the repository has been public a while without major problems (the maintainer, 2026-10-04). Two things to check then: whether Homebrew unpacks a cask's zip as `unzip` does (item 23), and whether the cask needs an `uninstall launchctl:` entry once item 51's helper exists.

## Considered and set aside

Considered for 0.3 and set aside (the maintainer, 2026-10-18):
- **Preloading models at startup** (`hooks.on_startup.preload`, a list, so it would merge safely): it names models the app doesn't manage, which can change between starts and reloads. It belongs with a managed model config, if one comes. The maintainer has ideas for that, from their own configs, but it needs a fully managed config, macros included; not now.
- **Single values** such as `logLevel`: llama-swap refuses to load when two files set one differently, so they'd need a "here or in your config, not both" hint. Hold until one is needed.
