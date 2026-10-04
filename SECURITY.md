# Security

## Reporting a vulnerability

Please report security problems privately, through GitHub's private
vulnerability reporting: the **Security** tab of this repository, then
**Report a vulnerability**. Please don't open a public issue for them.

Reports are read by the maintainer, who will reply there, and credit you in
the fix's release notes unless you'd rather not be named.

Only the latest release is supported: fixes go into a new release rather
than older ones.

## What's in scope

What the app trusts, and what it doesn't try to protect against, is in the
README's [Security](README.md#security) section. In short: anything already
running as your user can make the app run any program, as it can make a
terminal do, so that isn't a vulnerability. Ways in from another user of the
Mac, from the network, or from a web page are.

Problems in llama-swap itself belong with
[llama-swap](https://github.com/mostlygeek/llama-swap).
