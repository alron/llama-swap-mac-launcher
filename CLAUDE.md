# Project Brief: llama-swap Launcher & Monitor (macOS)

## Goal

Build a signed, notarized macOS menu-bar app in Go that launches, supervises, and monitors [llama-swap](https://github.com/mostlygeek/llama-swap), a local LLM model manager and router. It should also ship a small CLI so scripts and agents (e.g. Claude Code) can control it. The app will be distributed free on GitHub, and later as a Homebrew cask.

The app does **not** bundle or redistribute llama-swap. The user installs llama-swap separately, and this project is only the launcher/monitor.

## Current status and decisions

- **Stage** (2026-11-01): 0.3.2 is the latest release (2026-11-01). Milestones 0 to 6 are done; milestone 7 waits until the repository has been public a while without major problems. Next is 0.3.3, with item 58; item 51 is likely the last major feature of the 0.3 series (see "Work list"). How the project got here is in [docs/done.md](docs/done.md), "History".
- **The primary goal is met** (2026-09-28): llama-swap runs under a signed, notarized app and reaches the LAN from anywhere, tmux included. Polish and publishing followed, with the aim of sharing the project with llama-swap's developer. The repository has been public since 2026-10-04, as a single fresh commit of the scrubbed tree; the detailed history stays on a local branch that's never pushed. Personal context (machines, hostnames, the setup before the launcher) is in `CLAUDE.local.md`, which Claude Code loads alongside this file and `.gitignore` keeps out of the repo; it also has the check to run before any push. The Makefile's `SIGN_IDENTITY` default carries the maintainer's name and team ID; both are in `LICENSE` and in every signed binary already, so it stays.
- **0.3.x: managed configuration** (decided 2026-10-18). The 0.2 series meets the original goal: llama-swap runs under a signed app with the Local Network permission, much as a launchd job would, but with a UI and status. The 0.3 series adds configuration the app manages for the user, to make llama-swap easier for less technical Mac users: so far a config folder, `apiKeys`, peers and CORS (items 34, 36 and 38). The next push, item 47, is meant for 0.4.
- **Machines and the real setup:** a development Mac, and a second Mac on the LAN running its own llama-swap (the peer, also through the release app). Before the launcher, llama-swap ran from the maintainer's shell scripts, with a config whose `peers:` entry is the LAN connection that gets blocked. The details are in `CLAUDE.local.md`.
- **Signing:** Developer ID Application certificate installed and backed up (team ID `P56KW9H72P`); a hardened-runtime, timestamped test signature verified on 2026-09-28. Notarization credentials (App Store Connect API key) are saved in the keychain as notarytool profile `my-wang` (`--keychain-profile my-wang`), verified 2026-09-28. Account setup is complete.
- **Toolchain:** Go 1.27 and Xcode 27, but the active developer directory deliberately stays on the Command Line Tools. Call Apple's tools through `xcrun` so either setup works.
- **Git:** `origin` is https://github.com/alron/llama-swap-mac-launcher, public since 2026-10-04. Until then it was private, as the maintainer's safety net, since a slip could be wiped and re-pushed; a slip pushed now can't be taken back (clones, forks and caches keep it), so `CLAUDE.local.md`'s check comes before every push. `main` was pushed 2026-10-04 as one fresh commit; the old history is the local branch `private-history`, which a local pre-push hook refuses to push. Commits use the account's noreply address (`566136+alron@users.noreply.github.com`, set in this repo's config), and GitHub blocks pushes that would expose the real one. Git borrows `gh`'s login for this repo (a repo-local credential helper).
- **Language:** Go, because it's the maintainer's language. Don't propose Swift. Keep Objective-C to a few small shims (see Components §1).
- **Names:** the app is `Llama Swap Launcher.app` (dev builds: `Llama Swap Launcher Dev.app`). The executable inside is `llama-swap-launcher`. The CLI is `llsl` (no clashes on PATH or in Homebrew as of 2026-09-28).
- **Bundle ID:** `com.my-wang.llama-swap-launcher`, final as of 2026-09-28 (`com.my-wang` is the prefix for all of the maintainer's signed work). Never change it. Dev builds use `com.my-wang.llama-swap-launcher.dev`. It's defined in exactly one place, the Makefile.
- **Releases:** a notarized `.zip` (`Llama-Swap-Launcher-<version>.zip`) on GitHub Releases, built and published from the development Mac (`make release`, `make publish`) as tagged, immutable releases; later a Homebrew cask, with a `.dmg` beside the zip (milestone 7; see Signing, notarization, distribution). The Makefile's `VERSION` sets the version. 0.1.x was proof of concept, shared privately; 0.2.1 (2026-10-04) was the first public release, and anyone given 0.1.2 or 0.2.0 should move to it. [CHANGELOG.md](CHANGELOG.md) says what each release changed, and "History" in [docs/done.md](docs/done.md) how each was built and checked.
- **Architecture:** arm64 (Apple Silicon) only. Intel Macs are impractical for local models and x86_64 macOS is on its way out. Build with `GOARCH=arm64`; the Homebrew cask gets `depends_on arch: :arm64`.

## Work list

The TODO list is in two files, public like this one:
- [docs/roadmap.md](docs/roadmap.md): the open items in full, grouped by the release they're meant for. New items and reviews' findings go there.
- [docs/done.md](docs/done.md): the finished items in number order, each with its decisions and how it was verified; the declined suggestions, so they aren't proposed again; what reviews checked and found sound; the milestone results; and the project's history.

Items keep their numbers for good, since code comments and commit messages cite them ("item 25", "TODO item 14"); a new item takes the next number. An item cited by number is in the roadmap if it's open, and in done.md otherwise. When one is done, it moves to done.md, in number order, with what was built and how it was verified, and the list below changes with it. Both files are read when needed rather than loaded with this one, so link them; never `@`-import them.

Each change to `llsl` or the preferences also needs `README.md` updated.

The open items (2026-11-01):
- **Next release (0.3.3):** 58 (table text centred in its row; cosmetic).
- **0.3.x:** 51 (the GPU's wired memory limit, through a helper that runs as root; likely the last major feature of 0.3).
- **Blocked:** 53 (browser login through a form), on a llama-swap release that has it.
- **0.4:** 47 (macros, models and routing, managed by the app).
- **Later:** 35 (TLS), 37 (Tailcat) and milestone 7 (the DMG, and the Homebrew cask and tap).

## Working in this repo

- Go module `github.com/alron/llama-swap-mac-launcher`.
  - `cmd/llama-swap-launcher/`: the app (`main.go` wiring, `app.go` menu and actions).
  - `internal/supervisor`: runs, restarts and stops llama-swap, including the process-tree cleanup and config validation.
  - `internal/health`: watches a running llama-swap through its API. `cmd/llama-swap-launcher/models.go` puts that in the menu.
  - `internal/control`: the control socket's protocol, server and client (pure Go, shared with `llsl`). `cmd/llama-swap-launcher/control.go` answers requests.
  - `cmd/llsl`: the CLI, built with `CGO_ENABLED=0` into `Contents/Helpers/llsl`. The dev build's is `"build/Llama Swap Launcher Dev.app/Contents/Helpers/llsl"`.
- **systray gotcha:** on macOS, `systray.Quit()` just makes `systray.Run` return; it doesn't call `onExit`, which only runs when macOS terminates the app (at logout, say). So shutdown work runs from both (`app.shutdown`, called after `systray.Run` returns and from `onExit`).
  - `internal/macos`: the cgo/Objective-C shims: dialogs (`dialogs.m`), the Preferences dialog (`prefs.m`), the About panel (`about.m`), launch at login (`login.m`), the menu-bar title (`statusbar.m`) and copying secrets (`clipboard.m`), with shared helpers in `common.h`.
  - `packaging/`: the Info.plist template, the app icon (art, masters, and the `mkicon` tool) and `notarize.sh`.
  - `internal/prefs`, `internal/logfile`, `internal/paths`.
  - `packaging/Info.plist.in`: the bundle's Info.plist template.
- `make dev` builds and signs `build/Llama Swap Launcher Dev.app`; `make run` builds and launches it; `make test`, `make vet`. `make dev` refuses while the dev app is running, since it deletes and recreates the bundle; quit the app first.
- `make release` builds `build/release/Llama Swap Launcher.app` under the release bundle ID:
  - it signs with a secure timestamp and zips the app with `ditto`;
  - `packaging/notarize.sh` notarizes it (keychain profile `my-wang`) and staples the ticket, or prints Apple's log if rejected;
  - it zips the app again so the zip carries the ticket, then runs `spctl --assess`, `packaging/check-zip.sh` (unpacks it with `unzip` and checks the app) and prints the zip's SHA-256.
  
  Notarization takes a few minutes. `syspolicy_check distribution <app>` is a further readiness check.
- **Release titles are the tag alone** (`v0.3.1`; the maintainer, 2026-10-24): GitHub's release list cuts titles short, and "Llama Swap Launcher 0.3.1" and the rest all showed as "Llama Swap Launch…". `make publish` titles them so, and the four before were retitled with `gh release edit --title`, which immutable releases allow (their tag and zip unchanged). GitHub reports 0.2.1 as immutable too, though it was published before the setting was turned on.
- **`make publish`** (`packaging/publish.sh`) puts the built release on GitHub: it checks the zip again, that the app is `VERSION` built from a clean tree whose commit is on `origin/main`, the tag (creating it if missing) and the changelog, then pushes the tag and runs `gh release create`. `DRY_RUN=1` stops after the checks and shows the notes. It's outward-facing: ask the maintainer before running it for real.
- **`CHANGELOG.md`** has a section per release (`## <version> (<date>)`), written for users; changes since the last release collect under `## Unreleased`, renamed when `VERSION` moves up. Links use full URLs (release notes don't resolve relative ones). Commit and push it before `make publish`, which takes the release notes from `origin/main`'s copy and adds how to check the download.
- Builds use `-trimpath`, so the binaries don't contain local paths. `packaging/check-build.sh` runs after every build (dev, release, CI) and fails if either binary targets a macOS newer than `MIN_MACOS`, lacks an `LC_UUID`, or contains a path from the build machine.
- **Go patch releases:** `go.mod`'s `go` line is the minimum Go and the version CI installs, and with Go's default `GOTOOLCHAIN=auto` a local build with an older Go fetches it. When govulncheck reports standard-library vulnerabilities (CI first caught some on 2026-10-15: eight, fixed in Go 1.27.2, none reachable in the app; then GO-2026-6809 on 2026-10-20, fixed in 1.27.3, a denylist of compromised intermediate certificates, which only Fetch Models' HTTPS requests reach), raise it to the fixed release, and build the next release with it. A Go release can also need newer checkers: staticcheck v0.8.1 couldn't read Go 1.27.2's compiled packages ("export data version 5"), and v0.8.2 could. CI pins their versions; local ones come from Homebrew.
- **Checks at stable points:** `make lint` (`go vet` + `staticcheck`), `make sec` (`gosec`, which catches things the others don't) and `make vuln` (`govulncheck`). CI runs all of them, plus `make test` and an ad-hoc build, for every push to `main` and every pull request (`.github/workflows/ci.yml`). The gopls MCP server is also available for diagnostics, references and renames.
- **gosec triage, 2026-09-28:** fixed G115 (the peer-UID comparison) and one G104 (`llsl logs -f` ignored a failed `Seek` and would reprint the whole log). Accepted as by design:
  - G204, a subprocess with variable arguments: launching the user's llama-swap is the app's job, and the rest are `/usr/bin/open` with fixed arguments.
  - G304, a file path from a variable: every path comes from the bundle ID or is the user's own config.
  - The remaining G104 unchecked errors: best-effort cleanup (`Close`, removing a stale socket or pidfile, killing an already-exiting process), `Start`/`Restart`, whose failures show in the status, and writes to stdout.
  
  These are annotated in the code, so `make sec` reports nothing until something new appears: `// #nosec G204 -- <reason>` or `G304`, and `_ =` with a short comment for deliberately ignored errors. Annotate any new accepted finding the same way, with its reason, rather than leaving it in the report.
- **This file is public** (since 2026-10-04), and so are `docs/roadmap.md` and `docs/done.md`: keep personal details out of them, such as machine names, hostnames, home-directory paths and the setup before the launcher. Write "the development Mac" or "the LAN peer"; the specifics belong in `CLAUDE.local.md`, which is git-ignored. Reviews that write findings into them follow the same rule.
- **How it was made:** the README says so openly (decided 2026-10-04): the code, tests and docs are written by Claude in Claude Code under the maintainer's direction, every feature is tested on real Macs before it's committed, and this file is the working brief. Keep that section true as the process changes. It's impersonal ("the maintainer"), like the rest of the README.
- **The README's screenshots** (`docs/images/`, retaken 2026-11-01 with 0.3.2, twelve of them now: the menu in five states (paused added), the About panel, and each Settings tab (Tweaks added), on a plain black desktop; before, 2026-10-18 with 0.3.0, and first 2026-10-04 with 0.2.1) are the maintainer's own ⌘⇧4 captures: Claude can't take them (that needs Screen Recording permission, which the maintainer doesn't grant, and macOS draws the menu itself). Each was converted to sRGB (`sips --matchTo "/System/Library/ColorSync/Profiles/sRGB Profile.icc"`) and re-encoded with Go's `image/png`, which drops the display profile, EXIF and XMP; in 0.2.1's menu shots, other apps' menu-bar items to the right of **L→S** were blurred out; for 0.3.0 the maintainer moved **L→S** beside Apple's own items and hid the desktop icons, so nothing needed blurring. Fields that would show personal details were edited for the shot and the dialog cancelled; the maintainer's own name and home folder may show (the maintainer, 2026-11-01: they're on GitHub and in the About panel already), but not machine names, hostnames or the like. In 0.3.2's paused menu, an open menu pushed other apps' items into the shot, and they were filled with black, which the black desktop hides. Two retakes were asked for because they'd have taught the wrong thing: a peer whose key was `HF_TOKEN`, and llama-swap's own address listed as an allowed web page. Retake them with the next major feature, not for every small change (the maintainer, 2026-10-04), ideally on a Mac without other menu-bar items, so nothing needs blurring; look each one over before committing it: the text check before a push can't read images.
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

Seen in testing (details in the milestone results in [docs/done.md](docs/done.md)):
- The app's permission covers its children and theirs. The prompt names the app and shows its `NSLocalNetworkUsageDescription` (milestone 0).
- A newly built binary's first LAN connection can fail once with "no route to host" and work seconds later; expect it once after a llama-swap upgrade (milestone 0).
- macOS's firewall and Little Snitch each ask once, naming the app, llama-swap's connections included. Little Snitch holds the connection while it asks, so the first request can time out (milestones 0 and 5).

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
  - **The dialog's width and the tab labels** (2026-10-18):
    - The dialog is as wide as its widest tab (the Peers tab, measured at 0.3.1). Show in Finder sits beside the Config folder menu so the folder's row is no wider than the binary's and config's.
    - Tabs with lists (General, for Environment, then Secrets, Peers and Security) fill that width: `padded(view, YES)` pins the page to both sides, `stretched()` makes the lists as wide as their stack, and `minWidth()` keeps 540 (420 for text boxes) as the minimum. General is a column: a grid for the paths and the address, then Environment's title and table (stretched, as wide as the Secrets tables: 580 points, measured), the Health check row and the checkboxes, all from the same left edge. A grid in a stretched column is stretched too and gives some of the width to its label column, pushing the labels right, so `hugLabels` fixes that column at its widest label, measured as the grid does (a text field's alignment rect, 2 points in from its frame on each side). Logs keeps its form's width.
    - The tab view's frame is rounded up to whole points, since the pages' fitting sizes can be fractional, and made a point wider when the centred labels would sit on a half point across. AppKit always puts them 4.5 points below the tab view's top, as 0.2.2 had them. On a non-Retina display a fractional position blurs them; the dialog test program checks the horizontal one.
  - **Don't replace a table's cells to restyle them** (found 2026-10-18): change the views on screen in place (`viewAtColumn:row:makeIfNecessary:NO`) and let `tableView:viewForTableColumn:row:` style new ones. Cells replaced while the window's Tab order includes them (after a tab switch, say) leave pointers AppKit loses track of: it logs "has N views pointing at it that can't be found", and closing the window can crash. A test program should check its log for that line.
  - **Testing shims:** a small clang-built harness that opens the real dialog and drives it from timers works well, but mirror the app's threading (main thread in `[NSApp run]`, dialogs opened from another thread) and deliver clicks through the run loop, not a GCD block. From inside a main-queue block a file panel's `runModal` returns at once and leaves the panel orphaned on screen. Have the harness close anything still open after a timeout, rather than someone clicking Cancel, which hides failures. Test what happens *after* too: the first close harness passed while Quit still failed, because nothing checked that the main loop then ended.
  - A menu-bar-only (`LSUIElement`) app must activate itself before showing a panel, or the panel opens behind other windows. Since macOS 14, `[NSApp activate]` is only a request, which macOS sometimes refuses: dialogs don't mind, since they sit above ordinary windows, but the About panel (an ordinary window) intermittently opened behind other apps' windows (2026-10-04; not reproducible in a harness). `about.m` also calls `orderFrontRegardless` on it. Any future non-modal window needs the same.
  - **Editing shortcuts need an Edit menu.** Text fields get ⌘X/⌘C/⌘V/⌘A/⌘Z through the main menu's Edit items, and a menu-bar-only app has no main menu, so the keys just beep (right-click still works). `ensure_edit_menu()` in `common.h` installs a hidden main menu with a standard Edit menu; every shim that shows a dialog or panel calls it first. It's never visible, because menu-bar-only apps don't get a menu bar.
  - **Tab in multi-line fields:** an `NSTextView` takes Tab as a character. The Preferences dialog's delegate turns Tab and Shift-Tab into moving to the next or previous field. Checkboxes and buttons are only reached by Tab with macOS's Keyboard navigation setting on; that's the system's behaviour, not a bug.
  - **Explanations are tooltips, warnings are hints** (TODO item 48): what a field is for goes in a tooltip on the field and its label (`explain`); grey text under a field (`hint`) is only for warnings and for text that changes with the settings. Tooltips work in the modal dialog.
  - **Copying secrets** (TODO item 54): `clipboard.m` writes one value with the current-host-only option and the concealed and transient markers, keeps the clipboard's `changeCount` from its write, and clears only if it's unchanged, from a timer in the common modes (so it fires during and after the dialog) and at Quit. It never reads the clipboard's contents. Test programs must never touch the real clipboard (the maintainer's rule): `lsl_use_pasteboard` points the app at a `[NSPasteboard pasteboardWithUniqueName]`. The current-host-only option leaves no trace on the pasteboard, so a test records the `prepareForNewContentsWithOptions:` call itself.
  - **JSON booleans from Objective-C:** box them as `@YES`/`@NO` (`cond ? @YES : @NO`). `@(a == b)`, and even `@(cond ? YES : NO)`, box a *number*, which becomes JSON `1`, and Go won't decode that into a `bool`. This silently broke the Preferences dialog's Save at first, because the Go wrapper also treated an undecodable reply as Cancel. The wrapper now returns an error, and the app reports it.
  - `progrium/darwinkit` (Go bindings for AppKit) was considered and rejected: unmaintained for years, and it crashes on Go 1.25 (progrium/darwinkit#286).
- Menu: status (with llama-swap's pid), its explanation (wrapped over up to four lines), the last GPU fault, loaded models (each with an Unload submenu) and Unload All Models, Start or Stop (one item, retitled with `SetTitle`; TODO item 57), Restart, Pause All or Resume All (one item; hidden for a llama-swap older than v268), Logs (a submenu: open or show in the Finder each log, Log Settings…, Collect Diagnostics…), Open llama-swap UI, About Llama Swap Launcher, Settings…, Launch at Login (checkbox), Quit.
- **"Settings…", not "Preferences…"** (decided 2026-10-04, for 0.2.0): Apple's name since macOS 13, so the app fits in. The menu item, the window's title and every message say Settings; the file stays `prefs.json`, and the code keeps its `prefs` names.
- **Preferences dialog** (`internal/macos/prefs.m`, `cmd/llama-swap-launcher/prefsdialog.go`): an `NSAlert` (Save, Cancel) with an `NSTabView` as its accessory view: General, an `NSGridView` form; Secrets, two tables (TODO item 14); Peers; Security; Logs; and Tweaks, for settings that fit no other tab (TODO item 54 made it). Binary and config have Choose… buttons; the config folder is a menu (TODO item 34); environment variables are a table like the Secrets tab's, with plain values (TODO item 48). The `args` preference isn't shown (item 34): a field the dialog doesn't return keeps the value it was given, so Save keeps them.
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
  - `GET /api/events`: server-sent events. `modelStatus` events carry every model's `id`, `state` (`stopped`, `starting`, `ready`, `stopping`) and `peerID` (set for peer models). A snapshot arrives on connect, then one per change. The event's `data` field is JSON encoded as a string. The other message types in v260 are `logData`, `activity` (per-request token counts; TODO item 8), `inflight` (item 2), `uiConfig` and `profileChanged`; v268 added `pauseChanged` (`{"paused": …}`, on connect and with every change; item 57).
  - `GET /health` returns `OK`, with no auth middleware.
  - `GET /logs/stream/<model-id>` streams one model's output; add `?no-history` for new lines only. `/logs/stream/upstream` mixes all models; `proxy` is llama-swap's own log.
  - `POST /api/models/unload/<id>` unloads one model; `POST /api/models/unload` unloads all.
  - `POST /api/models/pause` and `/resume` (v268 on; item 57): a pause unloads every local model and answers requests for them with 503 until resumed; `/health` still says 200. A reload keeps it, a restart doesn't.
  - Also there: `/running`, `/v1/models` (with `status.value` loaded/unloaded), `/api/version`, `/metrics`.
  - **Auth:** with `apiKeys` set, every route the app uses except `/health` needs a key: `/api/*`, `/logs*`, `/upstream/*`, `/v1/models`, `/running`, `/metrics` and `/ui/` (the route table in `internal/server/server.go`). It's accepted as `Authorization: Bearer`, as Basic's password, or as `x-api-key`. The app doesn't send one yet: TODO item 14.
  - Peers: nothing beyond `peerID` in `modelStatus`; no endpoint lists peers or their URLs. `/upstream/<peer>/<model>/…` is proxied to the peer, and its `/health` is answered by the peer's llama-swap (TODO items 1 and 4).
  - Flags: `-config`, `-config-dir` (additive to `-config`; a setting since TODO item 34), `-listen`, `-watch-config` (a setting since item 34), `-validate`, `-version`, and TLS (`-tls-cert-file`, `-tls-key-file`). TLS flags in `args` would make llama-swap serve `https://`, which the app's `http://` monitor can't reach; not supported. v262's `-h` also lists `-listen-tailcat` (a second listener, given a private-key file), which the app neither watches nor forbids (TODO item 32).
- The monitor (`internal/health`) follows `/api/events` for model states and checks `/health` periodically, which catches a llama-swap that's hung but still holds the stream open. The menu shows "starting", "ready" or "not responding", lists loaded models with an Unload submenu each, and offers Unload All Models.
- The event stream has no heartbeat (llama-swap's `serveSSE` only sends real events), so hang detection needs requests, and with `logToStdout` including `http` each one is a log line. So (decided 2026-09-28):
  - the interval is a preference, `healthCheckSeconds`, default 60;
  - `healthCheckSkipWhenActive` (default true) skips a check when events have arrived since the last one;
  - checks (`/health`, the peer checks, and item 5's `/running` poll) carry the User-Agent `llama-swap-launcher-healthcheck/<version>` (`health.MarkCheck`), and the app drops those lines from its copy of llama-swap's log (`logfile.DropLines` with `health.IsCheckLine`, which needs the tag and a `GET` of one of those paths, and takes the tag without a version too, as apps up to 0.3.1 sent it). They still show in llama-swap's web UI.
  - every other request through `health.NewClient` (the event stream, unloads, `-load`, the probe, Fetch Models) names the app as `llama-swap-launcher/<version>`; `main` sets `health.AppVersion` first thing (TODO item 55).
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
- Commands: `status`, `start`, `stop`, `restart`, `unload MODEL | -all`, `pause`, `resume`, `logs [-n N] [-f]`, all with `-json` except `logs`. `logs` reads the log file directly, so it works while the app is down. Only `start` and `restart` launch the app. Control requests report problems in their response instead of showing alerts, since the caller is usually a script or an agent.
- Exit status: 0 ok (for `status`: ready and no failed model); 1 failed (for `status`: not ready, or a model's GPU backend failed); 2 usage; 3 app not running or couldn't be launched; 4 config invalid, llama-swap left as it was; 5 model didn't load; 6 timed out; 7 busy (requests in flight, nothing done; `-force` overrides); 8 paused (for `status`: inference is paused; for `-load`: refused because of it; item 57). On 1, 5 and 6, llsl also prints the last 20 log lines.
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
- **Releases are built, signed, notarized and published on the development Mac** (decided 2026-10-04, milestone 6): `make release`, then `make publish`. GitHub Actions runs CI only, with no secrets.
  - The maintainer's reasoning: a leak through GitHub (a fault there, an intrusion, a compromised action) is likelier than a remote compromise of their own Mac, so the private halves of the Developer ID certificate and the notary key stay off it.
  - That may change if releases become frequent. Then: a second Developer ID Application certificate and App Store Connect API key issued for GitHub alone, each revocable without touching the others. Builds signed with the second certificate keep users' Local Network grants, since the designated requirement is the bundle ID plus the team ID, with no certificate hash. The signing job would run only on tags, in a protected environment that waits for the maintainer's approval, with actions pinned by commit.
- **Later:** Homebrew cask. Official casks now must pass Gatekeeper, meaning signed and notarized. Use the cask `binary` stanza to symlink `llsl` onto `PATH`. The official cask repo also has popularity requirements, so start with our own tap.
  - **A `.dmg` alongside the zip** (the maintainer, 2026-10-18). The zip stays: it's the quick download for terminal users, the maintainer's own preference, and it already unpacks cleanly with `unzip` (item 23). The DMG is the Apple-ecosystem staple:
    - it holds the app, the drag-to-`/Applications` shortcut, `README.md` and `LICENSE`, which are useful there rather than left on disk as they would be from a zip;
    - it's built from the already stapled app, then signed, notarized and stapled itself, so a release has two notarizations;
    - `make publish` uploads both, with both SHA-256s in the notes;
    - the cask uses the DMG. Homebrew copies only what the cask names (`app`, and `binary` for `llsl`), so the README and LICENSE are for people installing by hand.
  - **The tap** (the maintainer, 2026-10-24): https://github.com/alron/homebrew-llama-swap-mac-launcher, created empty and private, which makes it `alron/llama-swap-mac-launcher` to `brew tap`. It must be public before anyone else can use it.
    - llama-swap's own tap (`mostlygeek/homebrew-llama-swap`) is one formula, `Formula/llama-swap.rb`, with a version and a SHA-256 per platform. A workflow in the tap rewrites them when llama-swap's release sends it a `repository_dispatch`, which needs a token on GitHub.
    - Ours is a cask (`Casks/…rb`), since it installs an app. Releases are published from the Mac (see above), so `make publish` can update the cask's version and SHA-256 and push it to the tap itself, with no token on GitHub.
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

The results of milestones 0 to 5 are in [docs/done.md](docs/done.md); milestone 6's are below.

0. **Prove the approach before building anything.** Done 2026-09-28; see "Milestone 0 results". A throwaway `.app`, signed with the Developer ID certificate under the `.dev` bundle ID and dev display name, whose only job is to launch a child that connects to a LAN host. Launch it with `open` from inside tmux. Repeat with the real llama-swap binary as the child, since someone else's Go toolchain built it. Using the `.dev` ID means the grant carries over to the real dev app, which also shows whether a grant survives a rebuild.
1. Menu-bar skeleton + supervisor (start/stop/restart, logging, binary/config pickers). Developer ID-signed dev build with the `.dev` bundle ID. Done 2026-09-28; see "Milestone 1 results".
2. Health polling + status/model list in the menu. Done 2026-09-28, along with GPU-fault detection and config validation; see "Milestone 2 results".
3. Control socket + CLI, including `restart --wait` and `open -b` auto-launch. Done 2026-09-28; see "Milestone 3 results".
4. `.app` bundling, Info.plist, launch-at-login. Done 2026-09-28 with placeholder icons; see "Milestone 4 results".
5. Release signing (release bundle ID, secure timestamp) + notarization in the Makefile; verify the Local Network prompt appears under the app's name and the grant survives a rebuild. Done 2026-09-28; see "Milestone 5 results". The grant surviving a rebuild was shown with the Developer ID-signed dev build in milestone 0, and the release build uses the same identity scheme.
6. GitHub Actions: CI, and publishing releases from the Mac. Done 2026-10-04; see "Milestone 6 results".
7. Homebrew cask, from our own tap, with a `.dmg` alongside the zip (see Signing, notarization, distribution). After the repository has been public a while without major problems.

## Milestone 6 results (2026-10-04)

Releases stay on the development Mac (see Signing, notarization, distribution); GitHub Actions only checks.

- **CI** (`.github/workflows/ci.yml`): `make test`, `make lint`, `make sec`, `make vuln` and `make dev SIGN_IDENTITY=-`, for pushes to `main`, pull requests, and by hand; not for changes to Markdown files alone. The first run passed in under two minutes, with every package's tests run on macOS 15.
  - It runs on `macos-15`: the oldest macOS the app supports, with Xcode 16.4 and the macOS 15 SDK, so a shim calling a newer API without checking fails to build there.
  - It holds no secrets: `contents: read`, the checkout keeps no credentials, and actions are pinned by commit. `.github/dependabot.yml` proposes action updates monthly. The checkers' versions are pinned in the workflow; raise them by hand.
  - Fork pull requests get the same read-only run. GitHub's default asks the maintainer to approve a first-time contributor's run.
  - While the repository was private, macOS minutes counted ten times against the plan's free allowance; since it went public, its runs are free.
  - The `-buildvcs` worry (declined from the Qwen review) didn't arise: the checkout's `.git` is readable.
- **Publishing:** `make publish` and `CHANGELOG.md` (see Working in this repo). 0.2.1 was the first GitHub Release, published this way on 2026-10-04: the zip downloaded from it has the same SHA-256 and passes `check-zip.sh`.

## Verify before relying on it

- Checked 2026-11-01: the real-llama-swap tests pass against v269, so `health.TestedVersion` is 269. Between v266 and v269: the capacity router (v267), pause and resume (v268: `POST /api/models/pause` and `/resume`, which unload every local model and refuse requests until resumed; the app doesn't show or use them yet: item 57), and in v269 a `modalities` field in `modelStatus` events (ignored) and new usage parsing, with the activity fields the menu's speeds read (`tokens.output_tokens`, `tokens.tokens_per_second`) unchanged in its source. (Before: v266, 2026-10-24, whose UI paths became relative, with `/ui` still redirecting to the UI; v265, 2026-10-15; v262, 2026-10-04.)
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
