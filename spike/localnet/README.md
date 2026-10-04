# Milestone 0: Local Network spike

Throwaway test of the launcher's core assumption: when a signed app starts
llama-swap as a child process, macOS attributes llama-swap's LAN connections
to the app, so the app's Local Network permission covers them.

It builds as `Llama Swap Launcher Dev.app` with the real dev bundle ID
(`com.my-wang.llama-swap-launcher.dev`), so the permission it gets carries
over to the real dev app later.

## What it checks

Each probe connects to the LAN target, another Mac running llama-swap
(`-target host:port`; the default, `peer.local:8080`, is a placeholder),
and reports:

- **REACHED**: the connection got to the host (including "connection
  refused", which is the host answering).
- **BLOCKED**: "no route to host", which is what Local Network privacy
  produces.
- **FAILED**: anything else, such as a timeout. Inconclusive.

The probes, in order:

1. `/usr/bin/nc` run as a child of the app.
2. A Go binary run as a child, which connects itself and runs `nc` as a
   grandchild.
3. llama-swap run as a child, with a config whose only entry is a peer at
   the target. The spike asks it for a one-token completion from the peer's
   model (`-peer-model`, default `gemma4-26b-a4b-q8`, ideally already
   loaded on the peer, so nothing new loads), which makes llama-swap connect
   to the peer.
4. The app itself.

Blocked probes are retried for 90 seconds, to leave time to answer the
permission prompt.

## Running it

Build (`SIGN_IDENTITY` overrides the certificate):

```
spike/localnet/build.sh
```

Then, **from inside a tmux session**:

1. **Launch through LaunchServices:**
   `open -W "spike/localnet/build/Llama Swap Launcher Dev.app" --args -target host:8080`
   - If macOS asks about local network access, note whose name is on the
     prompt, then click Allow.
   - Results appear in the menu bar under "LN spike" and in
     `~/Library/Logs/com.my-wang.llama-swap-launcher.dev/spike.log`.
   - Expected: all four REACHED. Quit from the menu.
2. **Run the executable directly:**
   `"spike/localnet/build/Llama Swap Launcher Dev.app/Contents/MacOS/llama-swap-launcher" -direct`
   - This was meant as a negative control, with BLOCKED expected. It got
     REACHED instead (see Results). For a real negative control, use a
     third-party binary in the same tmux pane, such as Homebrew's `telnet`.
     Apple's `/usr/bin/nc` gets through in tmux regardless.
3. **Rebuild and repeat step 1.**
   - Expected: no new prompt, and still all REACHED. That shows the
     permission survives a new build.

## Results (2026-09-28)

The approach works. Details are in the project CLAUDE.md under "Milestone 0
results". In short:

- **Step 1:** the prompt named "Llama Swap Launcher Dev" and showed our
  usage description. The probes got "no route to host" until Allow, then all
  four got through, including llama-swap returning a real completion from
  the peer. The first llama-swap attempt timed out while Little Snitch's
  own prompt was up; "Run again" passed.
- **Step 2:** everything got through, children included, even though
  Homebrew's `telnet` gets "no route to host" in the same tmux pane.
- **Step 3:** no new prompt and all four got through. The new binary's own
  first connection failed once and worked 3 seconds later.

## Cleanup

Unregister the spike from LaunchServices and delete it, so `open -b` later
finds the real dev app instead:

```
/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister \
  -u "spike/localnet/build/Llama Swap Launcher Dev.app"
rm -rf spike/localnet/build
```
