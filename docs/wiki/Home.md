# Home

[← Handbook](README.md)

## Purpose

**TunnelBypass** helps you stand up **server-side** tunnel endpoints on **Windows or Linux**: it can drive an interactive flow, emit JSON/YAML configs for engines like **Xray** and **Hysteria**, fetch helper binaries, and optionally register **OS services** (systemd, Windows service patterns).

Traffic and cryptography happen inside those engines; this tool **provisions and wires** them with consistent paths, SNI/host catalog integration, and optional Linux network hardening.

<a id="arabic-beginners"></a>

## لماذا هذا البرنامج؟ (للمبتدئين)

**ما الفكرة باختصار؟**  
TunnelBypass يساعدك تجهّز **نفقًا آمناً** على سيرفرك (ويندوز أو لينكس) **من سطر الأوامر**، بدون أن تبني كل ملفات الإعداد يدويًا من الصفر.

**1) قصة DNS (بكلمات بسيطة)**

- أحيانًا يكون **فهم أسماء المواقع** (DNS) معقدًا: إذا مرّت كل الاستعلامات داخل النفق بطريقة خاطئة، قد تحصل على **بطء** أو **دوائر غريبة**. البرنامج يضبط في الإعدادات ما يلزم بحيث تُوجَّه استعلامات DNS بطريقة **منطقية** (مثلاً عبر مسار مباشر حيث يناسب المحرك)، مع تفضيل مسارات **IPv4** حيث يُفضَّل لتفادي مشاكل شائعة على بعض الشبكات.
- على **لينكس** يمكنك أيضًا تفعيل **إصلاحات DNS على مستوى النظام** عند التثبيت عند الحاجة، حتى يبقى السلوك أوضح على السيرفر.

**2) تحسينات «تلقائية» (أي: أقل عمل يدوي)**

- **توليد ملفات جاهزة** في مجلدات ثابتة (`configs/...`) بدل نسخ لصق عشوائي.
- **جلب المساعدات** (المحركات مثل Xray أو غيرها) عند الحاجة.
- **تشغيل كخدمة نظام** (اختياري) حتى يعود النفق بعد إعادة التشغيل.
- على **لينكس**: خيارات لتحسين **إعدادات الشبكة للعبور (transit)** عندما تريد ذلك (مع الحذر؛ راجع [Linux networking](Linux-networking.md)).

**3) وتخطّي DPI (بدون وعود سحرية)**

- بعض الشبكات **تراقب** شكل الزيارات وتقيّد ما لا يشبه الزيارة العادية.
- البرنامج يعرض عليك **أنماط نقل مختلفة** (مثل TLS يشبه مواقع حقيقية، WebSocket، QUIC، …)؛ كل نمط له **مزايا وعيوب** حسب مزودك.
- الهدف أن تختار ما **يشبه حركة الويب العادية** قدر الإمكان؛ **لا يوجد ضمان** يعمل على كل شبكة في كل بلد—النتيجة تعتمد على سياسة الشبكة عندك.

**جملة أخيرة للمبتدئ:**  
ابدأ بتشغيل `tunnelbypass` **بدون معاملات** (واجهة تفاعلية)، اقرأ جدول النقل في [Transports](Transports.md)، ولا تغيّر إعدادات النظام على لينكس إلا إذا فهمت ماذا تفعل أو اختبرت على سيرفر تجريبي.

## What you get

- **Ready-made configs** under a predictable tree (`configs/<transport>/`, logs nearby).
- **Optional OS services** so listeners survive reboots when you choose that path.
- **Clear data layout** via `--data-dir`, `run portable`, or `TUNNELBYPASS_DATA_DIR` (see [Installation](Installation.md)).
- On **Linux**, optional **transit** tuning (sysctl, iptables, autopilot)—see [Linux networking](Linux-networking.md).

## Who it’s for

- Anyone who wants **one binary** and a **guided flow** instead of hand-editing raw JSON from scratch.
- Setups where **traffic shape** matters (Reality, TLS camouflage, QUIC where UDP is allowed).
- Contributors who like **docs next to code** in Git.

## What it is not

- **Not** a replacement for upstream **Xray / Hysteria / WireGuard** docs when you need deep tuning of those programs.
- **Not** legal advice: use only on networks and machines you’re allowed to configure.

## How to read this handbook

1. [Installation](Installation.md) — binaries, Docker, where files live.
2. [Transports](Transports.md) — choose a protocol and understand trade-offs.
3. [Linux networking](Linux-networking.md) — only if the server is Linux and you use transit features.
4. [Operations](Operations.md) — `status`, `uninstall`, portable runs, health checks.

The [root README](../../README.md) stays the short entry point; these pages go deeper (use `tunnelbypass run -help` for every flag).
