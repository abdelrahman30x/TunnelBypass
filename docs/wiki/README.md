# Handbook

In-repo documentation for TunnelBypass: Markdown in **`docs/wiki/`**, versioned with the repo and rendered on GitHub (not a separate wiki host).

**CLI-only:** there is no built-in web UI—everything runs from the terminal, config files, and optional OS services.

New here? Read **[Home](Home.md)** first, then use the table below.

---

## At a glance

1. **Run the tool** on your server (**Linux** or **Windows**).
2. **Pick a transport** (overview table in the [root README](../../README.md)).
3. **Use the generated configs or sharing links** on clients; on **Linux** only, optional **transit** steps (sysctl / firewall) are documented in [Linux networking](Linux-networking.md).

For install commands and quick copy-paste, start from the [root README](../../README.md).

---

## Pages

| Page | What’s inside | Read when |
|------|----------------|-----------|
| [Home](Home.md) | Purpose, what you get, boundaries; [Arabic for beginners](Home.md#arabic-beginners) | First time here |
| [Installation](Installation.md) | Build from source, install scripts, Docker, data dirs | You need binaries or paths |
| [Transports](Transports.md) | Choosing a stack, stealth trade-offs, clients | You’re picking a protocol |
| [Linux networking](Linux-networking.md) | Transit sysctl, iptables, autopilot, safe rollback | Server is Linux and you use network tuning |
| [Operations](Operations.md) | `status`, `health`, uninstall, portable mode | Day-to-day commands |

Use `tunnelbypass run -help` for the full flag list.
