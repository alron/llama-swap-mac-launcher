# Roadmap

The open TODO items, grouped by the release they're meant for. Finished items are in [done.md](done.md), and [CLAUDE.md](../CLAUDE.md) is the project's brief: a section named here (Components §1, "Signing, notarization, distribution") is CLAUDE.md's. An item cited by number that isn't here is in done.md.

Items keep their numbers; a new one takes the next, from 66. When one is done, it moves to done.md, in number order, with what was built and how it was verified, and CLAUDE.md's list of open items changes with it. A review adds its findings here as a dated group, introduced by what it covered and how. Like CLAUDE.md, this file is public: keep personal details out of it.

## Next release

0.3.2 was published on 2026-11-01 with items 52 and 54 to 57 (now in [done.md](done.md)). The next, 0.3.3, is the rest of the 0.3 series, in one release (the maintainer, 2026-11-03): items 58 to 63, all done (table text centred in its row, a pause kept across the app's own restarts, macOS notifications for problems, Tab and Return in Settings' tables, paths shown with `~`, and the windows asked for from the menu taking the focus; now in done.md), and items 51, 64 and 65, below. Milestone 7 ([below](#milestone-7)) comes first, so the release tests updating the tap.

51. **The GPU's wired memory limit, kept across reboots** (the maintainer, 2026-10-26; not started; decided the same day, below; for 0.3.3, the last major feature of the 0.3 series). `sysctl iogpu.wired_limit_mb=N` lets the GPU wire more memory, so bigger models fit. It needs root, and it resets at every boot. Today the maintainer sets it with a LaunchDaemon written by hand in `/Library/LaunchDaemons`. The app should do it through **`SMAppService.daemon`** (the maintainer's choice, over an `osascript` password prompt at every change; SMJobBless is deprecated).
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

<!-- Markdown numbers a list from its first item: this one starts its own, so it shows its number. -->

64. **Check for updates** (the maintainer, 2026-11-04; not started; for 0.3.3, after item 60, whose notifications it uses; the maintainer isn't particular about the release). Once a day the app asks GitHub for the latest release, and when it's newer than the running app, tells the user, with a link to its page.
    - **A Tweaks checkbox, off by default** (the maintainer), worded something like **Check for updates daily**: `checkForUpdates` in `prefs.json`, written only when it's on, so an older app can still read a file saved with the default. Its tooltip says what a check sends: a request to GitHub naming the app and its version, from the Mac's address.
    - **The check:** `GET https://api.github.com/repos/alron/llama-swap-mac-launcher/releases/latest`, the newest release that isn't a draft or a pre-release, whose `tag_name` is `v` and the version (release titles and tags are the same, `v0.3.3`).
      - GitHub's API wants a User-Agent: the app's own, `llama-swap-launcher/<version>`, as for its other requests (item 55), with nothing personal in it.
      - Without a token GitHub allows 60 requests an hour from an address; one a day is nowhere near it.
      - The other way, to weigh when building: `https://github.com/alron/llama-swap-mac-launcher/releases/latest` redirects to the newest release's page, and the redirect alone names the tag, with no API and no JSON.
      - It's a connection to the internet, not the LAN, so Local Network privacy doesn't apply; a firewall such as Little Snitch asks about it once, naming the app.
    - **When:** a minute or so after launch, so the network is up after a login, then every 24 hours while the app runs. The time of the last check is kept beside `prefs.json` (state, like item 59's `paused`), so quitting and reopening doesn't check again within the day. A failed check (offline, GitHub down, the rate limit) is a line in the launcher log, never an alert; when to try again (in an hour, or the next day) is to decide.
    - **Newer:** the tag, without its `v`, compared with the app's version as three numbers. A tag that isn't `major.minor.patch` is ignored, as no release.
    - **Telling the user:**
      - **A notification** (item 60's), at the ordinary level, not time-sensitive, once for each new version: "Llama Swap Launcher 0.3.4 is available". Clicking it opens the release's page in the browser. With item 60's box unticked, only the menu says so: an update isn't worth an alert the user didn't ask for.
      - **A menu item** near About, something like **Version 0.3.4 Is Available…**, which opens the same page. It stays until the app is that version or newer.
      - **The link is the app's own:** `https://github.com/alron/llama-swap-mac-launcher/releases/tag/v<version>`, built from a version that checked out as three numbers, not taken from the response (its `html_url`), so a tampered response can't send the user elsewhere. It opens as **Open llama-swap UI** does.
    - **A link, nothing more** (the maintainer: "Definitely more secure that way, less chances of compromises feeding malware in an automatic update"): the app doesn't download or install the update. That would be Sparkle (CLAUDE.md, "Later / optional").
    - **Decided by the maintainer** (2026-11-04):
      - **Homebrew installs get their own note,** if the app can tell Homebrew installed it: they update with `brew upgrade` (which quits and reopens the app, milestone 7), so the notice and the menu item say that, beside the link. It's told as item 65 tells it: a link in the Caskroom that resolves to the app's own bundle. The cask needs no `auto_updates true`, which is for apps that update themselves.
      - **A Check for Updates… menu item,** which checks at once and says what it found, "up to date" included, in a dialog: the user asked, so it takes the focus (item 63). As an explicit request, it works with the daily box unticked too.
      - **`llsl` keeps parity,** if it isn't a heavy lift: `status` shows a newer version the app knows of (an `update` field in its JSON, and a line in its text), and a command checks at once, as the menu item does (name and exit status to decide; README too). The app makes the request, through the control socket, so there's one check, with one User-Agent and one record of the last check.
    - **Tests:** in Go, the comparison and the link from good and bad tags; the request against a stand-in server with GitHub's JSON; the daily timing, with the last check kept, on a fake clock.
    - **Also:** the README (the setting, and what a check sends) and Collect Diagnostics (the last check and what it found).

<!-- Markdown numbers a list from its first item: this one starts its own, so it shows its number. -->

65. **The app restarts into a new version installed over it** (the maintainer, 2026-11-04, for milestone 7; not started; for 0.3.3). Homebrew 7 can't reopen the app after `brew upgrade` without Automation access (milestone 7, below), so the app does it: when a newer version lands where it's running from, it restarts into it.
    - **Replacing a running app:** the Finder refuses ("in use"), so a zip install dragged in by the Finder still needs a Quit first. Homebrew moves the running app's bundle aside and installs the new one without complaint (seen with the stand-in, milestone 7), as `ditto` or `unzip` in Terminal would. Nothing in the app stops either; the maintainer's "file in use" error was the Finder's.
    - **Noticing:** the app watches its own bundle's path, for a different version in its `Info.plist` (a periodic check, or a watch on the folder), and ignores a copy that's older or the same.
    - **Restarting:** a Quit as usual (llama-swap stopped first), then the new copy opened by its path through Launch Services, as at login, once the old one has exited: its control socket is the single-instance lock, so a new copy started earlier would exit at once. A small detached helper (a shell command) can wait for the old process and then `open` the app.
    - **A Tweaks checkbox, on by default** (the maintainer, 2026-11-04), worded something like **Restart on update (requires a Homebrew install)**. Unticked, the app keeps running the old version until it's quit, and the menu could say a newer one is installed. In `prefs.json`, a `*bool` written only when it's off, like item 57's box, so an older app can still read a file saved with the default.
      - **Only for a copy Homebrew manages** (decided by the maintainer, 2026-11-04): in-place replacement is what Homebrew does; the Finder won't replace a running app. Elsewhere the box is greyed out, and beside it a link, **Requires a Homebrew install**, opens the README's Homebrew instructions in the browser. Where Homebrew manages the app, the box is enabled and the link isn't shown. The link changes with the install, so it's on screen like a hint, not in a tooltip (item 48).
      - **"Homebrew manages this copy," not "Homebrew is installed"** (the maintainer's point): each version's folder in the Caskroom holds a link to the app it manages (seen after the maintainer's `--adopt`: `/opt/homebrew/Caskroom/llama-swap-launcher/0.3.2/Llama Swap Launcher.app` → `/Applications/Llama Swap Launcher.app`). The app checks whether such a link resolves to its own bundle, which needs no `brew` command (slow, and it may update itself). Only `/opt/homebrew` is checked: Homebrew's prefix on Apple Silicon since 2021, where the app runs; `/usr/local` is Intel Homebrew's, which the cask's `arch: :arm64` refuses, and other prefixes are ones Homebrew doesn't support. Item 64's Homebrew note uses the same check. The dev build is never Homebrew's, so it shows the link.
    - **And under it, once item 60's notifications are in** (the maintainer): a checkbox worded something like **Notify after an update restart**, indented under the first as item 59's is (`underTitle`), and greyed out unless both it and item 60's box are ticked. It posts an ordinary notification, not a time-sensitive one: "Llama Swap Launcher restarted into 0.3.4". **Off by default** (decided by the maintainer, 2026-11-04): the user most likely started the upgrade themselves, so less noise, and someone who wants it will find it, as anyone who looks through every tab does.
    - **The Tweaks tab fills up:** the clipboard's time, two pause boxes, item 60's notifications, item 64's update check and these two. If it gets long, group them under small bold titles, as the Secrets tab does, and check the window's height in the dialog test program.
    - **To decide:** whether to wait while requests are in flight (as `llsl restart` refuses), or to ask first; whether a pause is kept across this restart even with item 59's box unticked, since the user didn't choose to restart; what the log and menu say.
    - **Gatekeeper:** Homebrew quarantines the new copy and passes on the old copy's approval (`inherit_user_approval!`), so no prompt is expected when it's opened; to check.
    - **From 0.3.3 the cask drops `on_upgrade: :signal`,** keeping the signal for uninstalling. The 0.3.2 → 0.3.3 upgrade still quits 0.3.2 (its cask decides) and needs one manual open.
    - **Tests:** the decision (a newer version at the path, not the same or older) in Go; hands-on with the dev app replaced by a newer dev build, by `ditto` and through a local tap.

### Milestone 7

The Homebrew cask, from our own tap: the plans are in CLAUDE.md, "Signing, notarization, distribution". From our own tap only, with no submission to Homebrew's own repository, so installs and upgrades are managed by `brew`. **Done before 0.3.3's release** (the maintainer, 2026-11-01 and 2026-11-03), so that release tests `make publish` updating the cask in the tap, which must be public by then.

- **The cask uses the zip** (the maintainer, 2026-11-04). The DMG comes later, as an optional download beside the zip, and the cask stays on the zip if it works well (it does, below).
- **0.3.2's cask is in the tap** (2026-11-04), written by hand: `Casks/llama-swap-launcher.rb` in `alron/homebrew-llama-swap-mac-launcher`, still private, with a README. Token `llama-swap-launcher`, so `brew install --cask alron/llama-swap-mac-launcher/llama-swap-launcher`. It has `depends_on arch: :arm64` and `macos: :sequoia`, `livecheck` from GitHub's latest release, `binary` for `llsl`, `zap` for the support folder, logs and preferences file, and caveats that llama-swap isn't included and that an upgrade quits the app. `brew style` is clean, and so is `brew audit --cask --online --strict` once the maintainer trusted it.
- **Found in Homebrew 7.0.12** (read in its source, 2026-11-04):
  - **Zips are unpacked with `unzip`**, as `check-zip.sh` does (item 23), unless they hold `__MACOSX` or `._` entries, when `ditto` is used; ours have none.
  - **The old cask in an upgrade is the copy Homebrew kept at install,** so each version's cask decides how the upgrade after it quits the app.
  - **Flight blocks are deprecated** (`preflight do … end`, with a warning now) for declarative `*_steps` (`run`, `terminate_process`, `touch`, path guards), which **run in a sandbox**: Unix sockets are always denied (`(deny network-outbound (to unix-socket))`, network access or not), and so are opening apps (`lsopen`) and Apple Events; the home folder can't be read and `HOME` is a temporary folder. So a cask can't run `llsl` (it finds the control socket through `HOME`, and couldn't connect anyway) and can't reopen the app. Signals are allowed.
  - **`uninstall quit:`** quits through Apple Events, which needs Automation access for the terminal, and is the only way Homebrew reopens an app after an upgrade.
  - **`uninstall signal:`** runs in `brew` itself, outside the sandbox: it finds the app's process by bundle ID in `launchctl list` (the dev build's longer ID doesn't match), signals it, waits 3 seconds and carries on. It's skipped in an upgrade unless the cask says `on_upgrade: :signal`. The app takes SIGTERM as Quit, stopping llama-swap first ([main.go](../cmd/llama-swap-launcher/main.go)), which can take up to 20 seconds.
  - **`--adopt`** keeps an app already in place, unmoved and unquarantined, when its Info.plist's two versions match the cask's: how a zip install of the same version moves to Homebrew without a restart.
  - **Every download is quarantined,** `file://` included, with no way to skip it. Running anything inside an app that isn't notarized then trips Gatekeeper, as the stand-in below did once, with a dialog. The release is notarized, so a cask install gets macOS's one-time "downloaded from the Internet" check.
  - **A tap that isn't Homebrew's needs `brew trust`** before Homebrew loads its casks; the tap's README says so.
- **Tested with a stand-in** (2026-11-04): an app with its own bundle ID whose SIGTERM handler takes 8 seconds, two versions zipped as a release is, and a test cask made from the real one by substitution (token, version, checksum, `file://` URL, app name, bundle ID, and the `llsl` link renamed), through a temporary local tap, into a scratch Applications folder:
  - install: unpacked, the app moved, `llsl` linked and run;
  - upgrade while running: SIGTERM, Homebrew carried on after 3 seconds, the new version installed and the old bundle purged while the old copy was still quitting; it exited 5 seconds later;
  - uninstall while running: SIGTERM, then everything removed;
  - afterwards the test tap untapped and nothing left in the Caskroom; the maintainer's running release app untouched throughout.
- **Decided by the maintainer** (2026-11-04):
  - **The cask quits the app with `uninstall signal:`,** SIGTERM by bundle ID, so uninstalling never leaves it running without its files; 0.3.2's cask also does it on upgrade (`on_upgrade: :signal`), so the 0.3.2 → 0.3.3 upgrade quits it, and it's opened again by hand, once.
  - **The app restarts itself after an upgrade** (item 65), rather than the cask reopening it. From 0.3.3, the cask drops `on_upgrade: :signal`: the upgrade replaces the running app's bundle, and the app restarts into the new version.
  - **`llsl quit` is set aside:** a cask can't run it, which was its reason.
- **Still to do:**
  - **`make publish` updates the tap:** the cask's version and SHA-256, committed as the noreply address and pushed, after the same personal-details check, with `DRY_RUN=1` showing the change. The release's tag must be on GitHub first, since the cask's URL names it.
  - **The tap made public** before 0.3.3's release, and the README's "Installing" gains Homebrew, with `brew trust`.
  - **Telling a copy Homebrew manages** (items 64 and 65): a link in `Caskroom/llama-swap-launcher/<version>/` that resolves to the app's own bundle.
  - **Item 51's helper:** whether the cask needs `uninstall launchctl:` for its daemon.
  - **The real test:** the maintainer's own 0.3.2 is now the cask's (2026-11-04: `brew trust --cask`, then `brew install --cask --adopt`, which kept the running app in place, untouched, and linked `llsl`; an install never quits the app, only an uninstall or an upgrade does). It's upgraded to 0.3.3 by `brew upgrade` once that's published.

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
