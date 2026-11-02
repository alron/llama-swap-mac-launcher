# Roadmap

The open TODO items, grouped by the release they're meant for. Finished items are in [done.md](done.md), and [CLAUDE.md](../CLAUDE.md) is the project's brief: a section named here (Components §1, "Signing, notarization, distribution") is CLAUDE.md's. An item cited by number that isn't here is in done.md.

Items keep their numbers; a new one takes the next, from 57. When one is done, it moves to done.md, in number order, with what was built and how it was verified, and CLAUDE.md's list of open items changes with it. A review adds its findings here as a dated group, introduced by what it covered and how. Like CLAUDE.md, this file is public: keep personal details out of it.

## Next release

0.3.2 was published on 2026-11-01 with items 52 and 54 to 57 (now in [done.md](done.md)). The next, 0.3.3, has:

58. **Table text centred in its row** (the maintainer, 2026-11-01, from 0.3.2's screenshots; not started; cosmetic, for 0.3.3). In Environment, the secret variables, the peers and the models list, a cell's text sits at the top of its row, against the header and the next row's stripe. In the API keys, the key's value does too, but its name doesn't.
    - **Why** (read in `prefs.m`): every text cell is a bare `NSTextField`, which the table stretches to the row's height (24 points) and which draws its text at the top. The API keys' name is the exception: its fixed `LLSL_` prefix and the name are in an `NSStackView`, which centres them.
    - **The fix:** the same centring for every text cell: in `LSLSecretTable` (both columns) and in `LSLPeers` (the peers' name and URL, and the models list). That's a container, a stack or an `NSTableCellView` with a centre-Y constraint, and the field inside it. The code that finds a cell's field (`rowForView:`, and the restyling in place, which mustn't replace cells: Components §1) has to look through the container.
    - **Tests:** the dialog test program checks that each text cell's text is centred in its row on every table, edits still reach the rows, and nothing turns up in AppKit's log ("pointing at it"). The README's Settings screenshots then show it, at the next retake.

## 0.3.x

Item 51 is likely the last major feature of the 0.3 series (the maintainer, 2026-11-01), for a release after 0.3.2.

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

The Homebrew cask, from our own tap, with a `.dmg` beside the zip: the plans are in CLAUDE.md, "Signing, notarization, distribution". It waited until the repository had been public a while without major problems (the maintainer, 2026-10-04). **Now planned alongside 0.3.3** (the maintainer, 2026-11-01): from our own tap only, with no submission to Homebrew's own repository, so installs and upgrades are managed by `brew`. Two things to check: whether Homebrew unpacks a cask's zip as `unzip` does (item 23), and whether the cask needs an `uninstall launchctl:` entry once item 51's helper exists.

**Upgrading while the app runs** (the maintainer, 2026-11-01: no "quit the app before `brew upgrade`"). Replacing the running app by hand fails because its files are in use, and the app has to stop and start anyway: llama-swap is its child.
- **Homebrew's own way** (read in Homebrew 7.0.11's `cask/artifact/abstract_uninstall.rb` and `cask/upgrade.rb`): `uninstall quit: "<bundle id>"` quits the app before an upgrade and reopens it afterwards (`lsregister -f`, then `open -b`). But it quits through `osascript` (`Application(id).quit()`), an Apple Event, which needs Automation access for Terminal ("Terminal → System Events"). The maintainer never grants that, and nor will many people, so it would time out after 10 seconds and carry on with the app running.
- **The plan: the cask quits and reopens the app through `llsl`,** the control socket, which needs no permission.
  - **During an upgrade** (read in `cask/installer.rb`): the old cask's `uninstall_preflight` block runs before the old app is removed (`start_upgrade` → `uninstall_artifacts`, flight blocks before the `Uninstall` directives), and the new cask's `postflight` runs once the new one is in place.
  - **`uninstall_preflight`:** runs the installed app's `Contents/Helpers/llsl quit`, and notes for `postflight` that the app was running.
  - **`postflight`:** reopens the app with `open -b` if it was running.
  - **Needs a new `llsl quit`:** stops llama-swap as Quit does, then quits the app. Proposed: it refuses while requests are in flight (exit 7), like `stop`, so `brew upgrade` stops with that message rather than cutting someone off; `-force` overrides.
  - **What's lost:** a pause is in memory only (item 57), so an upgrade resumes inference. To settle when building: whether `llsl quit` hands it to the next launch.
- **To validate first, with a local tap and a `file://` URL:**
  - `system_command` in both blocks during `brew upgrade --cask`;
  - `appdir` in `uninstall_preflight`;
  - how a block's failure (exit 7) stops the upgrade;
  - whether replacing the running bundle ever succeeds without quitting, leaving the old app running.
- **Zip installs** still need the app quit first. The app could notice its own bundle being replaced and restart itself, which would serve both; noted, not planned.

## Considered and set aside

Considered for 0.3 and set aside (the maintainer, 2026-10-18):
- **Preloading models at startup** (`hooks.on_startup.preload`, a list, so it would merge safely): it names models the app doesn't manage, which can change between starts and reloads. It belongs with a managed model config, if one comes. The maintainer has ideas for that, from their own configs, but it needs a fully managed config, macros included; not now.
- **Single values** such as `logLevel`: llama-swap refuses to load when two files set one differently, so they'd need a "here or in your config, not both" hint. Hold until one is needed.
