# Transports and stealth

[← Handbook](README.md)

## How TunnelBypass models transports

TunnelBypass **writes configs** under a predictable tree (`configs/<transport>/`, logs beside them) and can install **one OS service** per flow when you ask for it. Choosing a transport means picking a **recipe**: the interactive flow or `tunnelbypass run …` with the right flags.

## Stealth ladder (rule of thumb)

Stricter networks often tolerate traffic that **looks like ordinary HTTPS** to a real hostname more than raw or obvious proxy shapes. A coarse ordering (see also the [root README](../../README.md) table):

1. **VLESS + REALITY (TCP / Vision)** — strong TLS camouflage when tuned with a plausible SNI.
2. **VLESS + REALITY + gRPC** — HTTP/2 framing; REALITY + gRPC is the supported gRPC path here (see project notes in code).
3. **WebSocket / WSS paths** — useful where HTTP upgrades are normal; different trade-offs than QUIC.
4. **QUIC / Hysteria** — excellent when UDP is not throttled; weaker when UDP is degraded.

This is **not** a guarantee against every DPI stack—only a practical ordering for many ISP conditions.

## Engines and clients

| Engine family | Typical client side |
|---------------|---------------------|
| Xray (VLESS, WSS, …) | v2rayN, Nekoray, sing-box, etc. |
| Hysteria v2 | Official / compatible Hy2 clients |
| WireGuard | OS WireGuard apps |
| SSH / stunnel / wstunnel | SSH clients, custom HTTP stacks |
| Shadowsocks + v2ray-plugin (SS + SNI) | ss:// importers, shadowsocks-rust + plugin, NekoBox, sing-box (manual profile), etc. |

**SS + SNI listen port:** TunnelBypass defaults to **40000** (TCP+UDP) when no port is set (wizard, `run`, spec). Open that port on the VPS if you use the default. Choose **443** in the wizard instead if you want ordinary-HTTPS port camouflage.

<a id="tls-allowinsecure"></a>

### Self-signed TLS, `allowInsecure`, and what the server does

Several stacks (**VLESS WSS/SSH-TLS, Hysteria URIs, Shadowsocks + v2ray-plugin**, …) use **TLS** between the **client and your TunnelBypass host**. When that TLS uses a **self-signed** (or private-CA) certificate, many clients need an explicit **“allow insecure” / `allowInsecure` / `insecure`** toggle so they skip **verifying only that hop**.

**Important:** there is **no `allowInsecure` knob in TunnelBypass server configs** that changes how the daemon “trusts” downstream sites. The server **presents** the certificate files it generated (or you installed). Whether the **client** accepts that certificate is entirely a **client** setting.

- **`teddysun/v2ray-plugin` + v2fly/v2ray-core (v5):** upstream plugin code builds `tls.Config` from flags like `tls`, `host`, `cert`, `key`, and **`certRaw`**. There is **no** `AllowInsecure` / `allowInsecure` switch wired into that config — a bare **`allowInsecure`** token in `plugin_opts` is **ignored**, so TLS verification still runs and self-signed certs fail with **`x509: unknown authority`** unless you pin trust another way.
- **TunnelBypass (provisioned Shadowsocks + v2ray-plugin):** after generating `configs/shadowsocks/cert.crt`, the **base64(DER)** leaf is copied into **`SSV2rayClientCertRaw`** and written to **`server.json`** under **`_tunnelbypass.v2rayClientCertRaw`** so sharing links and **`client.json`** always embed **`certRaw=…`** — the **remote client never needs a cert file**, only the URI / JSON. The server process still uses **`cert=` / `key=`** paths in **`server.json`** `plugin_opts`. Each provision also picks a random **WebSocket `path=/tb-…`** (not `/`) and stores it under **`_tunnelbypass.v2rayWsPath`** so server and client **`plugin_opts`** stay in sync.
- **Manual / third-party configs** with no embedded cert raw: TunnelBypass may still emit **`tls;host=<SNI>;allowInsecure`** as a **best-effort** hint for clients that interpret it — **real v2ray-plugin binaries will not**. Paste **`certRaw`** yourself or use a client that skips verification for the plugin hop.
- **Sharing links / URIs:** the **`ss://` query** encodes **every** inner `;` as **`%3B`** — import the URI **exactly** as printed. With **`certRaw`**, links are **long** (QR codes may truncate — prefer **`client.json`** or the text file under `configs/shadowsocks/`). **`allowInsecure` / `insecure` in VLESS / Hysteria** URLs use those stacks’ own rules.
- **Go-based clients (sing-box 1.19+, etc.):** the leaf certificate must include **Subject Alternative Name (DNS)** entries, not only **Subject CN**. TunnelBypass always adds **SAN** when it generates or **re**generates TLS certs (OpenSSL path included). If you still see **`x509: certificate relies on legacy Common Name field, use SANs instead`**, run **Setup again** or delete `configs/shadowsocks/cert.crt` and `private.key` so a new cert is written; then re-import **`client.json`** / the new **`ss://`** (new **`certRaw`**).

**Critical — broken `ss://` after copy/edit:** In SIP002, the **whole** `plugin` query value must be **percent-encoded**. If you see **raw semicolons** right after `plugin=`, e.g.

`?plugin=v2ray-plugin;tls;host%3D...`

many clients (including sing-box) treat `;` as a **query separator**, so they only read `plugin=v2ray-plugin` and **drop** the rest → TLS still verified → **`x509: unknown authority`**. Correct shape uses **`%3B`** instead of **`;`**. Regenerate from TunnelBypass or fix the link by encoding every `;` in the plugin argument as `%3B`.

<a id="sing-box-client-tls"></a>

### sing-box / Throne / NekoRay / NekoBox and `x509: certificate signed by unknown authority`

Two different problems share the **same error text**. Use the **rest of the log line** to tell them apart.

#### 1) Outer TLS to **your TunnelBypass** (self-signed) — most common with `ss://` + v2ray-plugin

**Typical log shape (sing-box):**

`inbound/mixed[mixed-in]: process connection from 127.0.0.1:…: tls: failed to verify certificate: x509: certificate signed by unknown authority`

Here **mixed** is the local SOCKS/HTTP inbound (same idea as Throne’s `127.0.0.1:2080` style layouts). A browser or app connects to **127.0.0.1**; sing-box then opens the **Shadowsocks + v2ray-plugin** session to your VPS. The **first TLS layer** uses TunnelBypass’s **self-signed** cert → the plugin must **trust that cert** (TunnelBypass **`certRaw`** in `plugin_opts`, or your client’s own “insecure” UI if it supports the plugin hop — **not** the unused `allowInsecure` token for stock v2ray-plugin). This is **not** fixed on the server alone; the **importer / JSON** must pass a pinning or skip-verify path **into v2ray-plugin**.

**What to do:**

1. **Regenerate** the sharing link / **`client.json`** from TunnelBypass — ensure **`certRaw=…`** appears in decoded `plugin_opts` when using the bundled self-signed cert, and the URI has **`%3B`** between parts, not raw `;` (see warning above).
2. **Inspect the imported profile:** after decode, plugin args should include **`tls;host=<YOUR_SNI>;path=/tb-…;certRaw=…`** (TunnelBypass-generated) or your client’s equivalent skip-verify for the plugin.
3. **NekoRay / NekoBox:** open the server profile → **Plugin** / **TLS / certificate** options → enable **skip verification** / **allow insecure** for **this server** if the app did not honor the URI (wording varies by version).
4. **Throne (sing-box):** edit the **shadowsocks** outbound in JSON; set `plugin` to `v2ray-plugin` and match TunnelBypass **`plugin_opts`** (including **`certRaw`** if you copy from `client.json`). If the UI strips `plugin_opts`, paste **raw** config.
5. **Isolation test:** on PC, run **shadowsocks-rust** with TunnelBypass’s `configs/shadowsocks/client.json`
   (already aligned with the link). If that works but Throne/NekoRay fails, the GUI is not forwarding **`certRaw`** / TLS options to the plugin → use JSON / another build / report to that app.

#### 2) TLS to a **public** site (e.g. WhatsApp) or **DoH** inside sing-box

If the log names a **real hostname** (e.g. `web.whatsapp.com:5222`) or a **DoH** URL, verification uses **public** PKI. Then fix **DNS / `fake-ip`**, **sing-box & OS CA store/device time**, or **DoT/DoH outbound TLS** — not TunnelBypass `ss://` tunnel flags. More detail: [DoH / dnscrypt-proxy](#doh-dnscrypt-tls).

**DNS tip:** Throne’s WireGuard template in code uses **plain UDP DNS** on `1.1.1.1:53` through the tunnel to avoid DoH TLS issues. If you hand-write sing-box for Shadowsocks, prefer **UDP DNS** in `dns.servers` while debugging, instead of DoH, so a broken DoH trust path is not confused with the SS tunnel cert.

A log like

`connection: open connection to web.whatsapp.com:5222 using outbound/shadowsocks[proxy]: tls: ... unknown authority`

means **that hostname’s** chain failed verification (or sing-box mishandled the stream) — **not** “add allowInsecure on the TunnelBypass server.”

Typical checks for this case:

1. **sing-box build / CA store** — official build; valid OS trust store (or sing-box bundled CAs).
2. **DNS + `fake-ip`** — try **real DNS**, simpler rules.
3. **No double TLS** — one clean shadowsocks + v2ray-plugin outbound to TunnelBypass.
4. **Device time**, antivirus / corporate HTTPS inspection.

Tunnel-hop insecure settings (**section 1** above) are separate from errors that **name a public host** in the dial.

<a id="nekobox-dns"></a>

### NekoBox / sing-box: `dns: lookup failed … context deadline exceeded` (Shadowsocks + v2ray-plugin)

Typical log lines:

- `outbound/shadowsocks[proxy]: outbound packet connection to 8.8.8.8:53`
- `dns: lookup failed for www.google.com: context deadline exceeded`

**Cause:** DNS (**UDP** to e.g. `8.8.8.8:53`) is being sent **through the Shadowsocks outbound**. With **v2ray-plugin** (outer WebSocket + TLS), **UDP relay** to public resolvers is often **slow or unreliable**, so lookups **time out** while normal **HTTPS (TCP)** may still work intermittently.

**What to do:**

1. **NekoBox:** In **Preferences** (or per-profile DNS options, depending on version), turn on **Direct connection for DNS** / use **system DNS**, so resolver traffic **does not** go through the tunnel.
2. **Raw sing-box:** In `dns.servers`, set **`detour`** to your **direct** outbound (tag) for the resolver, not `proxy`. Only application traffic should use the Shadowsocks outbound; **bootstrap DNS** must be reachable without waiting on the tunnel.
3. **Why not “fix the server”?** TunnelBypass already sets shadowsocks-rust **`mode: tcp_and_udp`**. The usual failure is **client** routing DNS via the plugin path, not a missing UDP flag on the VPS.

<a id="nekobox-mtu-mux"></a>

### NekoBox: MTU and Mux (optional hardening)

If the tunnel is flaky on some networks (packet loss, strict middleboxes), try **lowering MTU** in NekoBox (e.g. **1280**). Enabling **Mux** / multiplexing for the profile (when the client supports it) can collapse many short-lived connections into fewer plugin sessions, which sometimes helps with DPI that targets connection churn. These are **client-only** tweaks; TunnelBypass does not set them in `ss://` (see your app’s advanced / sing-box options).

<a id="doh-dnscrypt-tls"></a>

### DoH / dnscrypt-proxy and the same TLS error (not fixable via sharing URL)

Tools like **dnscrypt-proxy** can log `tls: failed to verify certificate: x509: certificate signed by unknown authority` when **HTTPS/TLS to a DNS resolver** (e.g. Cloudflare DoH) goes through a **proxy or perimeter that uses its own CA** (corporate MITM, “main-proxy” with private roots). That is the same **x509** string as in other apps, but the fix is **not** something you add to TunnelBypass **`ss://` / `vless://` / `hysteria2://`** links.

What usually works (as in [DNSCrypt discussion #2774](https://github.com/DNSCrypt/dnscrypt-proxy/issues/2774)):

1. **Install the proxy’s root CA** into the **OS trust store** on the machine running dnscrypt-proxy / DoH client.
2. Or run the resolver on a **middle hop** where full public CAs are already trusted and connectivity is clean.

Maintainers generally **do not** recommend a permanent “`curl -k` for everything” mode for a long‑running DNS daemon; prefer trusting the right CA. For **sing-box**, if the failure is on a **DoH/DoT outbound**, adjust that outbound’s **TLS** block in JSON (or trust store)—again **separate** from the TunnelBypass subscription URI.

## Spec files and non-interactive runs

For automation, prefer **`--spec`** / config flags documented in `tunnelbypass run -help` so CI or config management can call the same paths as the interactive flow without a TTY.
