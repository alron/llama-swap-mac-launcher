# Changelog

What changed in each release. Versions before 0.2.1 were shared privately,
not published; they're listed for people who had them.

## 0.2.1 (2026-10-04)

The first public release.

Llama Swap Launcher is a menu-bar app that runs
[llama-swap](https://github.com/mostlygeek/llama-swap) for you, so its
connections to other machines on your LAN work under macOS's Local Network
privacy however it's started: from tmux, a script or an AI agent too. It
comes with `llsl`, a command-line tool for scripts and agents. See the
[README](https://github.com/alron/llama-swap-mac-launcher#readme) to install and use it.

It needs macOS 15 or later on Apple silicon, and llama-swap v252 or later
(tested with v262), installed separately.

This release fixes what a security assessment found in 0.2.0:

- **The zip works with `unzip`.** Unpacked with `/usr/bin/unzip` rather than
  the Finder, 0.2.0's app failed its signature check.
- **Collect Diagnostics redacts more:** secrets in comment lines, multi-line
  values, `x-api-key` and other credential headers, `curl -u`, `NAME=value`,
  JSON inside a line, and more token formats. Every environment variable's
  value is blanked in the copy of the settings.
- **A warning when llama-swap can be used without a key:** in the menu and
  in `llsl status`, when the app has API keys that llama-swap's config
  doesn't ask for, or llama-swap listens beyond this Mac with none. Settings
  asks before a change that would open it to the network without a key.
- The app's requests to llama-swap no longer go through a proxy set in the
  environment, which could have sent the API key off the Mac.
- A client could keep its requests out of the saved log by sending the app's
  health-check User-Agent. Only the app's own health checks are left out
  now.
- The health monitor stops as soon as llama-swap exits, rather than sending
  the API key to whatever takes the port next.
- `llsl status` shows control characters in text from the network as
  escapes, so it can't redraw the terminal.
- Alerts no longer fail on text that isn't valid UTF-8.
- A second copy of the app can't stop the first one's llama-swap. Requests
  to the control socket are limited in size, and logs aren't opened through
  a symbolic link.
- A warning when the app is run straight from Downloads, where macOS runs it
  from a temporary location that Launch at Login can't use.
- [SECURITY.md](https://github.com/alron/llama-swap-mac-launcher/blob/main/SECURITY.md), and a Security section in the README: how to
  report a problem, and what the app trusts.

Upgrading: quit the running app before replacing it. Settings, keychain
items and the Local Network permission carry over.

## 0.2.0 (2026-10-04)

- **Settings** (renamed from Preferences, as macOS names it) has tabs:
  General; Secrets, for llama-swap API keys and secret environment
  variables, kept in your login keychain rather than in a file; and Logs.
- **API keys:** the app passes them to llama-swap as `LLSL_<NAME>`
  variables for its config's `apiKeys`, and sends one with its own
  requests, so it works with a llama-swap that requires keys.
- **Logs:** the launcher's notes and llama-swap's output share
  `launcher.log` by default; llama-swap's output can have its own file or
  not be saved. Upgrading from 0.1.x, `llama-swap.log` stops growing.
- **About Llama Swap Launcher**, with the app's and llama-swap's versions.
- **Collect Diagnostics…** saves a redacted zip for bug reports.
- **The menu** shows the last request's speed beside each model, the last
  GPU fault, and why llama-swap is refusing the app (a missing key, a
  llama-swap too old). Failure alerts include llama-swap's last lines, and a
  busy port names the program holding it.
- **`llsl`:**
  - `restart`, `stop` and `unload` refuse while requests are in flight
    (exit 7), unless given `-force`;
  - `-load` waits until the model is really ready, and refuses a peer's
    model;
  - `status` checks each peer;
  - `logs -launcher` shows the launcher log;
  - `llsl` warns when the app running is a different version.
- **Optional:** unload a model automatically when its GPU backend fails,
  and mark a model whose server was updated on disk since it loaded.
- If the app crashes, the next launch also stops the model servers its
  llama-swap left running.
- The control socket is now `ctl`, so longer user names work. `llsl` from
  0.2 recognises an app from 0.1.x still running and says to quit it.

## 0.1.2 (2026-09-30)

The Preferences dialog.

## 0.1.1 (2026-09-29)

The app icon, and the status-coloured **L→S** in the menu bar.

## 0.1.0 (2026-09-28)

The first build: the menu-bar app, the health monitor and `llsl`.
