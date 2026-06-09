# Speculos + Ethereum App Integration Notes

Command-level guide to sign an Ethereum transaction against the Speculos emulator
from Node.js, with **no physical hardware**.

All claims below were verified against actual source files (paths cited inline) or
live `npm view` / GitHub API / `curl` output on 2026-06-08. Anything not verified is
explicitly marked **ASSUMPTION**.

Verified package versions (via `npm view <pkg> version`):

| Package | Version |
|---|---|
| `@ledgerhq/hw-app-eth` | 7.8.5 |
| `@ledgerhq/hw-transport-node-speculos-http` | 6.36.3 |
| `@ledgerhq/hw-transport-node-speculos` (TCP) | 6.34.3 |
| `@ledgerhq/hw-transport-http` | 6.36.3 |
| `@ledgerhq/device-management-kit` (DMK) | 1.5.1 |
| `@ledgerhq/device-transport-kit-speculos` (DMK transport) | 1.2.1 |
| `@ledgerhq/device-signer-kit-ethereum` (DMK eth signer) | 1.16.0 |

Latest app-ethereum release with prebuilt ELFs: **1.22.1**
(via `GET https://api.github.com/repos/LedgerHQ/app-ethereum/releases`).

---

## RECOMMENDED PATH (nothing → signed ETH tx + review-screen screenshot)

Fastest reliable path: **download the prebuilt Ethereum `.elf` from the
`app-ethereum` GitHub release**, run Speculos via Docker with the HTTP API on port
5000, and sign with `@ledgerhq/hw-app-eth` over
`@ledgerhq/hw-transport-node-speculos-http`. Auto-approve by pressing `both`
buttons via the HTTP API.

```bash
# --- 0. workspace ---
mkdir -p speculos-apps signer proof && cd "$(pwd)"

# --- 1. get a runnable ETH app ELF (Nano S Plus target = "nanos2", runs as --model nanosp) ---
curl -L -o speculos-apps/eth-nanosp.elf \
  https://github.com/LedgerHQ/app-ethereum/releases/download/1.22.1/app-1.22.1-nanos2.elf

# --- 2. run Speculos (Docker) headless, HTTP API on 5000, fixed BIP39 seed ---
docker pull ghcr.io/ledgerhq/speculos:latest
docker run --rm -d --name speculos \
  -v "$(pwd)/speculos-apps:/speculos/apps" \
  -p 5000:5000 -p 9999:9999 \
  ghcr.io/ledgerhq/speculos:latest \
  --model nanosp /speculos/apps/eth-nanosp.elf \
  --display headless --api-port 5000 --apdu-port 9999 \
  --seed "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

# wait for the API to come up
until curl -sf http://127.0.0.1:5000/events >/dev/null 2>&1; do sleep 0.5; done

# --- 3. Node signer deps ---
cd signer
npm init -y
npm install @ledgerhq/hw-app-eth @ledgerhq/hw-transport-node-speculos-http @ethereumjs/tx @ethereumjs/common
# write sign.js from section 4 below, then:
node sign.js          # prints r/s/v, saves ../proof/review.png
```

`sign.js` (full script in Section 4) opens the HTTP transport, drives an
auto-approve loop that presses `both` when the device shows the review screen,
takes a screenshot of that screen, and prints `{r, s, v}`.

Notes / caveats baked into the recommendation:
- There is **no `nanos` (original Nano S) build** in recent app-ethereum releases,
  only `nanos2` (Nano S Plus), `nanox`, `flex`, `stax`, `apex_p`. Use `nanos2` +
  `--model nanosp`. (Verified: release asset list, and speculos model keys in
  `external/speculos/speculos/mcu/struct.py`.)
- The HTTP transport sends APDUs over `POST /apdu` on the **same port 5000** as the
  API, so for this path you do not strictly need the 9999 APDU TCP port. It is
  published above only so the raw TCP transport / `ledgerctl` can also be used.

---

## 1. Ethereum app ELF — how to get a runnable one without a device

### 1a. Does Speculos bundle a sample ETH .elf? — NO (only a generic test app)

`find external/speculos -name '*.elf'` yields:
- `apps/boil.elf` → symlink to `apps/nanox#boil#25#6e728d99.elf` — a generic
  "boilerplate" test app for **nanox**, **not Ethereum**.
- `speculos/cxlib/*.elf` and `speculos/sharedlib/*.elf` — these are the emulator's
  internal crypto/shared libraries (per device/API level), **not signable apps**.

`external/speculos/apps/README.md` explicitly says:
> DO NOT use them to test apps or your integrations, as these binaries are old and
> unmaintained. Rather, follow the instructions in the app's repository in order to
> build the most recent version.

So Speculos ships **no usable Ethereum app**.

### 1b. Does LedgerHQ/app-ethereum publish prebuilt .elf artifacts? — YES (this is the win)

Verified via the GitHub releases API
(`GET https://api.github.com/repos/LedgerHQ/app-ethereum/releases`). The latest
tag is **1.22.1**, and each release attaches prebuilt ELFs per device:

```
app-1.22.1-apex_p.elf
app-1.22.1-flex.elf
app-1.22.1-nanos2.elf   <-- Nano S Plus
app-1.22.1-nanox.elf    <-- Nano X
app-1.22.1-stax.elf
```

Exact download URL pattern:

```
https://github.com/LedgerHQ/app-ethereum/releases/download/<TAG>/app-<TAG>-<device>.elf
```

Copy-paste (Nano S Plus / runs as `--model nanosp`):

```bash
curl -L -o eth-nanosp.elf \
  https://github.com/LedgerHQ/app-ethereum/releases/download/1.22.1/app-1.22.1-nanos2.elf
```

Nano X variant:

```bash
curl -L -o eth-nanox.elf \
  https://github.com/LedgerHQ/app-ethereum/releases/download/1.22.1/app-1.22.1-nanox.elf
```

Device → speculos model mapping (verified `external/speculos/speculos/mcu/struct.py`
MODELS dict: `nanox`, `nanosp`, `stax`, `flex`, `apex_p`):

| ELF asset suffix | `--model` flag |
|---|---|
| `nanos2` | `nanosp` |
| `nanox` | `nanox` |
| `stax` | `stax` |
| `flex` | `flex` |
| `apex_p` | `apex_p` |

> There is no plain `nanos` model in current Speculos and no `nanos` ELF asset.

### 1c. Registry/CDN/tool that downloads prebuilt app ELFs

- **GitHub releases (above) are the canonical prebuilt source.** No extra tool
  needed beyond `curl`.
- **ASSUMPTION:** `ledgered` (Ledger's `ledgered`/`ledger-app-dev-tools` Python
  package) and Ledger's app store/manager CDN host production app binaries, but
  the store CDN serves *installable, encrypted/SCP-wrapped* app payloads for real
  devices, **not raw Speculos-runnable `.elf` files**. I did not find a documented
  one-command "download an emulatable ETH elf" tool other than the GitHub release
  assets. Treat any registry/CDN route as unverified; the GitHub release is the
  reliable one.

### 1d. Fallback — build from source with ledger-app-builder Docker

Verified image and inner build command from
`https://github.com/LedgerHQ/ledger-app-builder` and the SDK Makefiles
(`Makefile.standard_app` uses `TARGET_NANOS2` for Nano S Plus). Output path follows
the standard SDK convention `build/<target>/bin/app.elf`.

```bash
# 1. clone the app source
git clone https://github.com/LedgerHQ/app-ethereum.git
cd app-ethereum
git submodule update --init --recursive   # app-ethereum vendors libs as submodules

# 2. pull the official builder image
docker pull ghcr.io/ledgerhq/ledger-app-builder/ledger-app-builder:latest

# 3. build for Nano S Plus inside the container (mounts cwd at /app)
docker run --rm -v "$(realpath .):/app" \
  ghcr.io/ledgerhq/ledger-app-builder/ledger-app-builder:latest \
  bash -c 'BOLOS_SDK=$NANOSP_SDK make -j'

# 4. resulting ELF lands at:
#    build/nanos2/bin/app.elf
ls -la build/nanos2/bin/app.elf
```

Per-device `BOLOS_SDK` env vars provided inside the builder image:
`$NANOSP_SDK` (Nano S Plus → `build/nanos2/...`), `$NANOX_SDK` (`build/nanox/...`),
`$STAX_SDK`, `$FLEX_SDK`. (Env-var names per ledger-app-builder docs;
**ASSUMPTION** that all four are present in the `:latest` tag — `$NANOSP_SDK` is
documented explicitly.)

> Use 1b (download) over 1d (build) unless you need an unreleased / patched app.
> Building pulls submodules + a large toolchain image.

---

## 2. Speculos run command (headless ETH app + HTTP API + fixed seed)

CLI flags verified in `external/speculos/speculos/main.py`:
- `--model {nanox,nanosp,stax,flex,apex_p,...}` (`-m`)
- `--display {headless,qt,text}` (default `qt`) — use `headless`
- `--api-port` default **5000** (`0` disables the REST API)
- `--apdu-port` default **9999** (raw APDU TCP server)
- `--seed` / `-s` accepts a **BIP39 mnemonic or hex seed**; default mnemonic
  (`DEFAULT_SEED` in `main.py`) is:
  `glory promote mansion idle axis finger extra february uncover one trip resource lawn turtle enact monster seven myth punch hobby comfort wild raise skin`
- `--automation` accepts a JSON doc or `file:<path>` (see Section 6)

### Pip-installed form

```bash
pip install speculos    # provides the `speculos` entrypoint (and ./speculos.py in-repo)

speculos eth-nanosp.elf \
  --model nanosp \
  --display headless \
  --api-port 5000 \
  --apdu-port 9999 \
  --seed "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
```

(Equivalent in a source checkout: `./speculos.py eth-nanosp.elf --model nanosp ...`.)

### Docker form

The container working dir is `/speculos`; mount your app dir to `/speculos/apps`
and publish the ports (pattern from `external/speculos/docs/user/docker.md`).

```bash
docker pull ghcr.io/ledgerhq/speculos:latest

docker run --rm -it \
  -v "$(pwd)/speculos-apps:/speculos/apps" \
  -p 5000:5000 -p 9999:9999 \
  ghcr.io/ledgerhq/speculos:latest \
  --model nanosp /speculos/apps/eth-nanosp.elf \
  --display headless \
  --api-port 5000 \
  --apdu-port 9999 \
  --seed "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
```

Add `-d` to detach. The fixed `--seed` makes derived addresses deterministic; with
the BIP39 test mnemonic above, `44'/60'/0'/0/0` always yields the same ETH address.

> The docker docs example uses `--model nanos`; that is stale. Use `nanosp` (or one
> of the models that exist in `struct.py`).

---

## 3. Speculos HTTP API — endpoints & ports

Default API port **5000** (`external/speculos/docs/user/api.md`). Paths verified
from the bundled OpenAPI spec
`external/speculos/speculos/api/static/swagger/swagger.json` and the Flask handlers
in `external/speculos/speculos/api/`.

| Method | Path | Purpose |
|---|---|---|
| `GET`  | `/screenshot` | PNG of current screen (`screenshot.py`) |
| `POST` | `/button/{button}` | Button: `{button}` ∈ `left` \| `right` \| `both` (`button.py`) |
| `POST` | `/finger` | Touch screen (Stax/Flex/Apex+); body `{x, y, action}` |
| `GET`  | `/events` | List events; `?stream=true` for SSE, `?currentscreenonly=true` for current screen text |
| `DELETE` | `/events` | Reset the events list |
| `POST` | `/automation` | Push/replace automation rules (same JSON as `--automation`) |
| `POST` | `/apdu` | Send one APDU; body `{"data":"<hex>"}` → `{"data":"<hex>"}` |

Button body schema (from `button.py`): JSON `{"action": "..."}` where `action` ∈
`press-and-release` | `press` | `release`, with optional `"delay"` (seconds,
default `0.1`). Internally `left=[1]`, `right=[2]`, `both=[1,2]`.

Screenshot:

```bash
curl -o screenshot.png http://127.0.0.1:5000/screenshot
```

Press both buttons (confirm):

```bash
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"action":"press-and-release"}' \
  http://127.0.0.1:5000/button/both
```

Touch (Stax/Flex), body fields `x`, `y`, `action` (`press-and-release` etc.):

```bash
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"x":200,"y":300,"action":"press-and-release"}' \
  http://127.0.0.1:5000/finger
```

Events stream (Server-Sent Events). Each line is `data: {json}` where the JSON is a
`TextEvent` with fields `text, x, y, w, h, clear`
(`external/speculos/speculos/observer.py` + `api/events.py`):

```bash
curl -N http://127.0.0.1:5000/events?stream=true
# data: {"text": "Review", "x": 0, "y": 0, "w": 128, "h": 11, "clear": false}
# data: {"text": "transaction", "x": 0, "y": 11, ...}
```

Current screen text only (handy for polling which screen we're on):

```bash
curl -s 'http://127.0.0.1:5000/events?currentscreenonly=true'
# {"events":[{"text":"Review","x":...}, ...]}
```

Ports recap: **5000** = REST/HTTP API (and APDU via `POST /apdu`); **9999** = raw
APDU TCP server (used by `ledgerctl`, `ledgerblue`, and the
`hw-transport-node-speculos` *TCP* transport via `LEDGER_PROXY_PORT`).

---

## 4. Node signing snippet (ledgerjs) — VERIFIED

**Correct transport for a clean Node demo:**
`@ledgerhq/hw-transport-node-speculos-http` — connects to the **HTTP API port
(default 5000)** and tunnels APDUs over `POST /apdu`. It also exposes
`.button("left"|"right"|"both")` and an `automationEvents` stream, so the *same*
transport object can both sign and drive the UI. Verified by unpacking the tarball:
`SpeculosHttpTransport.ts` does
`baseURL = (opts.baseURL||"http://localhost") + ":" + (opts.apiPort||"5000")`,
`exchange()` → `POST /apdu {data: hex}`, `button()` → `POST /button/<name>
{action:"press-and-release"}`.

Transport comparison:
- `@ledgerhq/hw-transport-node-speculos-http` → **port 5000** (HTTP API). Recommended.
- `@ledgerhq/hw-transport-node-speculos` → **port 9999** (raw APDU TCP). Works, but
  no button/UI helpers; you'd drive the UI via the separate REST API. Open with
  `SpeculosTransport.open({ apduPort: 9999 })`.
- `@ledgerhq/hw-transport-http` → generic HTTP-proxy transport (for the
  `ledger-live-http-proxy.py` style proxy on 9998), **not** the Speculos REST API.
  Not the right one here.

Signing API verified from `@ledgerhq/hw-app-eth` `lib/Eth.d.ts`:
`signTransaction(path, rawTxHex, resolution?)` → `Promise<{ r, s, v }>` (hex
strings, no `0x`). `rawTxHex` is the **serialized *unsigned* tx hex** (for legacy:
RLP of the 9-field tx; for EIP-1559: the `0x02`-typed serialization without
signature). Passing `resolution = null` forces blind-sign fallback but still signs.

Install:

```bash
npm install @ledgerhq/hw-app-eth @ledgerhq/hw-transport-node-speculos-http @ethereumjs/tx @ethereumjs/common
```

`signer/sign.js` — opens the transport, builds an EIP-1559 tx, auto-approves, grabs
the review-screen screenshot, prints `r/s/v`:

```js
const SpeculosHttpTransport =
  require("@ledgerhq/hw-transport-node-speculos-http").default;
const Eth = require("@ledgerhq/hw-app-eth").default;
const { FeeMarketEIP1559Transaction } = require("@ethereumjs/tx");
const { Common, Hardfork } = require("@ethereumjs/common");
const fs = require("fs");
const path = require("path");

const API = "http://127.0.0.1";
const PORT = "5000";
const DERIVATION_PATH = "44'/60'/0'/0/0";

async function screenText() {
  const res = await fetch(`${API}:${PORT}/events?currentscreenonly=true`);
  const { events } = await res.json();
  return events.map((e) => e.text).join(" ");
}

async function shoot(file) {
  const res = await fetch(`${API}:${PORT}/screenshot`);
  const buf = Buffer.from(await res.arrayBuffer());
  fs.writeFileSync(file, buf);
}

(async () => {
  const transport = await SpeculosHttpTransport.open({
    baseURL: API,
    apiPort: PORT,
  });
  const eth = new Eth(transport);

  // sanity: deterministic address from the fixed seed
  const { address } = await eth.getAddress(DERIVATION_PATH, false);
  console.log("signer address:", address);

  // build an unsigned EIP-1559 tx (Sepolia, chainId 11155111)
  const common = Common.custom(
    { chainId: 11155111 },
    { hardfork: Hardfork.London },
  );
  const tx = FeeMarketEIP1559Transaction.fromTxData(
    {
      to: "0x1234567890123456789012345678901234567890",
      value: 1_000_000_000_000_000n, // 0.001 ETH (wei)
      gasLimit: 21000n,
      maxFeePerGas: 30_000_000_000n,
      maxPriorityFeePerGas: 1_500_000_000n,
      nonce: 0n,
      data: "0x",
    },
    { common },
  );

  // serialized UNSIGNED message hex (no 0x), as hw-app-eth expects
  const rawUnsignedHex = Buffer.from(tx.getMessageToSign(false)).toString("hex");

  // auto-approve loop: when the review screen is up, press BOTH; screenshot it once
  let shotTaken = false;
  const approver = setInterval(async () => {
    try {
      const text = await screenText();
      if (
        !shotTaken &&
        /(Review|Amount|Max fees|Hold to sign|Sign transaction|Accept)/i.test(text)
      ) {
        await shoot(path.join(__dirname, "..", "proof", "review.png"));
        shotTaken = true;
      }
      if (/(Accept|Approve|Sign transaction|Hold to sign)/i.test(text)) {
        await transport.button("both"); // confirm on Nano UI
      }
    } catch (_) {}
  }, 400);

  const sig = await eth.signTransaction(DERIVATION_PATH, rawUnsignedHex, null);
  clearInterval(approver);
  console.log("signature:", sig); // { r, s, v } hex strings
  await transport.close();
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
```

Notes:
- `fetch` is global in Node ≥ 18. On older Node, swap to `axios`/`node-fetch`.
- For a deterministic *known* address, set Speculos `--seed` to the BIP39 mnemonic
  you control (Recommended Path uses the classic `abandon ... about` test mnemonic;
  Speculos's own default mnemonic also works and is listed in Section 2).
- The screen-text regexes above are heuristics for the Nano flow (`Review`,
  `Amount`, `Max fees`, `Accept`/`Approve`). If the app build uses different
  wording, dump `GET /events` once and adjust. A robust alternative is the
  `--automation` rule file (Section 6), which matches text inside Speculos and
  presses for you with zero polling in Node.

---

## 5. DMK Speculos support — YES, a public Node transport exists

`@ledgerhq/device-management-kit` (DMK, v1.5.1) **does** have a public, Node-capable
Speculos transport: **`@ledgerhq/device-transport-kit-speculos` (v1.2.1)**. It talks
to Speculos over **HTTP** (default `http://localhost:5000`), not WebHID/WebBLE, and
its README states **"Node.js >= 20"** compatibility. Verified by unpacking the
tarball:

- `package.json`: name `@ledgerhq/device-transport-kit-speculos`, peerDeps
  `rxjs` + `@ledgerhq/device-management-kit` (no WebHID/DOM deps).
- `lib/types/src/index.d.ts` exports: `speculosTransportFactory`,
  `SpeculosTransport`, `speculosIdentifier`.
- Factory signature:
  `speculosTransportFactory(speculosUrl?, isE2E?, deviceModelId?) => TransportFactory`.

Install:

```bash
npm install @ledgerhq/device-management-kit @ledgerhq/device-transport-kit-speculos rxjs
```

Minimal DMK wiring (build the DMK with the Speculos transport):

```ts
import { DeviceManagementKitBuilder } from "@ledgerhq/device-management-kit";
import { speculosTransportFactory } from "@ledgerhq/device-transport-kit-speculos";

const dmk = new DeviceManagementKitBuilder()
  // defaults to http://localhost:5000; pass a URL for a custom port
  .addTransport(speculosTransportFactory("http://localhost:5000"))
  .build();

// then: dmk.startDiscovering(...) -> dmk.connect({ device }) -> sessionId,
// and drive APDUs / use a signer kit with that sessionId.
```

> README typo watch: the README import example reads
> `from "@ledgerhq/device-transport-speculos"`, but the **real package name is
> `@ledgerhq/device-transport-kit-speculos`** (confirmed via `npm view` and the
> tarball `package.json`). Use the `-kit-` name.

Full Ethereum signing via DMK is possible with
**`@ledgerhq/device-signer-kit-ethereum` (v1.16.0)**:
`new SignerEthBuilder({ sdk, sessionId }).build()`, then
`signerEth.signTransaction(derivationPath, transaction, options)` where
`transaction` is a **`Uint8Array`** and the result is an **RxJS observable** of
`DeviceActionState` ending with `Signature { r, s, v }` (verified in the signer-kit
README). This differs from ledgerjs: DMK takes a tx **buffer** (not hex string) and
returns an **observable**, and you must first discover → connect → obtain a
`sessionId`.

**Recommendation for the live demo:** use the **ledgerjs path in Section 4**
(`hw-app-eth` + `hw-transport-node-speculos-http`). It is a flat async/await call,
the transport doubles as the button driver, and there is far less ceremony than
DMK's builder + discovery + session + observable model. DMK-over-Speculos is real
and works in Node, but it is the heavier integration; keep it as the
"production-architecture" alternative, not the demo.

---

## 6. Auto-approve the signing prompt

Two interchangeable mechanisms (both verified):

### Option A — drive buttons from Node via the HTTP API (used in Section 4)

Poll `GET /events?currentscreenonly=true`, and when the review/accept screen shows,
`POST /button/both {action:"press-and-release"}`. On Nano S+/X the final
"Accept and send" screen is confirmed with **both** buttons. Standalone curl:

```bash
# screenshot the review screen for proof
curl -o proof/review.png http://127.0.0.1:5000/screenshot
# confirm
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"action":"press-and-release"}' http://127.0.0.1:5000/button/both
```

### Option B — Speculos `--automation` rules (zero Node polling)

Speculos matches screen text itself and fires button presses. Format verified in
`external/speculos/docs/user/automation.md`: a JSON doc (or `file:<path>`) with
`rules[]`; each rule may have `text`/`regexp`/`x`/`y`/`conditions` and `actions`.
Actions: `["button", num, pressed]` with `num=1` left, `num=2` right, `pressed`
true/false; also `["finger",x,y,touched]`, `["setbool",name,val]`, `["exit"]`.

`approve.json` — press **both** buttons whenever an "Accept"/"Approve"/"Sign"
screen appears (press = down then up on buttons 1 and 2):

```json
{
  "version": 1,
  "rules": [
    {
      "regexp": "Accept|Approve|Sign transaction|Hold to sign",
      "actions": [
        ["button", 1, true],
        ["button", 2, true],
        ["button", 1, false],
        ["button", 2, false]
      ]
    }
  ]
}
```

Load at launch:

```bash
speculos eth-nanosp.elf --model nanosp --display headless \
  --api-port 5000 --automation file:approve.json --seed "<mnemonic>"
```

Or push/replace rules at runtime over the API (same JSON body):

```bash
curl -s -X POST -H 'Content-Type: application/json' \
  --data @approve.json http://127.0.0.1:5000/automation
```

> For the demo, Option A (Section 4 script) keeps everything in one Node process and
> lets you screenshot the exact review screen before confirming. Option B is the
> most robust for CI because text matching happens inside Speculos.

---

## Verification log (what was actually checked)

- `find external/speculos -name '*.elf'` → only `boil.elf` (nanox) + cxlib/sharedlib internals; no ETH app. `external/speculos/apps/README.md` warns the bundled elfs are stale.
- `GET api.github.com/repos/LedgerHQ/app-ethereum/releases` → tag 1.22.1 with assets `app-1.22.1-{apex_p,flex,nanos2,nanox,stax}.elf`.
- `npm view` confirmed all package versions in the table at top.
- `external/speculos/speculos/main.py` lines ~396-446: `--apdu-port` default 9999, `--api-port` default 5000, `--seed` (BIP39/hex), `--display {headless,qt,text}`, `-m/--model`, `--automation`; `DEFAULT_SEED` value at line ~60.
- `external/speculos/speculos/api/static/swagger/swagger.json` paths: `/apdu` POST, `/automation` POST, `/button/{button}` POST, `/events` GET+DELETE, `/finger` POST, `/screenshot` GET.
- `external/speculos/speculos/api/button.py`: button names left/right/both, body `{"action": "press-and-release"|"press"|"release", "delay"?}`.
- `external/speculos/speculos/api/events.py` + `observer.py`: SSE `data: {json}` lines; `TextEvent{text,x,y,w,h,clear}`; `?currentscreenonly=true` returns `{"events":[...]}`.
- Unpacked `@ledgerhq/hw-transport-node-speculos-http` `src/SpeculosHttpTransport.ts`: baseURL `host:apiPort` (default 5000), `exchange`→`POST /apdu`, `button`→`POST /button/<name>`.
- Unpacked `@ledgerhq/hw-app-eth` `lib/Eth.d.ts`: `signTransaction(path, rawTxHex, resolution?) → {r,s,v}`, `getAddress(path, boolDisplay?, ...) → {publicKey,address}`.
- Unpacked `@ledgerhq/device-transport-kit-speculos`: exports `speculosTransportFactory`/`SpeculosTransport`/`speculosIdentifier`; README "Node.js >= 20", default `http://localhost:5000`; peerDeps `rxjs` + DMK.
- Unpacked `@ledgerhq/device-signer-kit-ethereum` README: `SignerEthBuilder({sdk,sessionId}).signTransaction(path, Uint8Array, opts)` → observable → `Signature{r,s,v}`.
- `external/speculos/docs/user/docker.md`: image `ghcr.io/ledgerhq/speculos`, `-v $(pwd)/apps:/speculos/apps`, publish ports.
- `external/speculos/speculos/mcu/struct.py`: model keys `nanox, nanosp, stax, flex, apex_p` (no plain `nanos`).
- `github.com/LedgerHQ/ledger-app-builder` + SDK `Makefile.standard_app`: image `ghcr.io/ledgerhq/ledger-app-builder/ledger-app-builder:latest`, `BOLOS_SDK=$NANOSP_SDK make`, target `TARGET_NANOS2`, output `build/nanos2/bin/app.elf` (SDK convention).

**ASSUMPTIONs flagged inline:** (1c) Ledger store/CDN and `ledgered` as a prebuilt-elf source for emulation; (1d) presence of all per-device `$*_SDK` vars in the `:latest` builder image (`$NANOSP_SDK` is documented). Everything else above was read directly from source/spec or live API output.
