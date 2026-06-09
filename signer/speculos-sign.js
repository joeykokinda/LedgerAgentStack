// speculos-sign.js
//
// The hardware-signing seam for Agent Airlock. The Go airlock invokes this as
// its --signer-cmd once an intent has cleared the policy gate. Contract:
//
//   stdin  : a TxIntent JSON  { chainId, to, valueWei, data, gasLimit, nonce, ... }
//   stdout : exactly one JSON line { "signedTx": "0x..", "deviceApproved": true, ... }
//   exit !=0 : the device rejected the transaction (or a device error occurred)
//
// It builds the unsigned EIP-1559 transaction, sends it to the Ethereum app
// running in Speculos (Ledger's emulator), walks the on-device review screens,
// captures them to ../proof, approves, and returns the broadcastable signed tx.
//
// Keep stdout pristine: the Go side parses it as JSON. All diagnostics go to
// stderr, and we route any stray library console.log to stderr too.
console.log = (...args) => console.error(...args);

const fs = require("fs");
const path = require("path");

const SpeculosHttpTransport = require("@ledgerhq/hw-transport-node-speculos-http").default;
const Eth = require("@ledgerhq/hw-app-eth").default;
const { Transaction, Signature, getAddress } = require("ethers");

const apiPort = Number(process.env.SPECULOS_API_PORT || 5000);
const apiUrl = `http://localhost:${apiPort}`;
const derivationPath = process.env.LEDGER_PATH || "44'/60'/0'/0/0";
const proofDir = path.resolve(__dirname, "..", "proof");

// Fee parameters for the demo (Sepolia). These become part of the unsigned
// transaction the human reviews on the device screen.
const maxFeePerGas = 30000000000n; // 30 gwei
const maxPriorityFeePerGas = 1500000000n; // 1.5 gwei

function readStdin() {
  return new Promise((resolve, reject) => {
    let buffer = "";
    process.stdin.setEncoding("utf8");
    process.stdin.on("data", (chunk) => (buffer += chunk));
    process.stdin.on("end", () => resolve(buffer));
    process.stdin.on("error", reject);
  });
}

function sleep(milliseconds) {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

async function currentScreenText() {
  const response = await fetch(`${apiUrl}/events?currentscreenonly=true`);
  const body = await response.json();
  return (body.events || []).map((event) => event.text).join(" ").trim();
}

async function pressButton(button) {
  await fetch(`${apiUrl}/button/${button}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ action: "press-and-release" }),
  });
}

async function saveScreenshot(filename) {
  const response = await fetch(`${apiUrl}/screenshot`);
  const bytes = Buffer.from(await response.arrayBuffer());
  fs.mkdirSync(proofDir, { recursive: true });
  fs.writeFileSync(path.join(proofDir, filename), bytes);
  console.error(`[signer] captured ${filename} (${bytes.length} bytes)`);
}

async function waitUntil(predicate, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (predicate(await currentScreenText())) {
      return true;
    }
    await sleep(150);
  }
  return false;
}

// Walk the review screens: capture each one, save the amount screen as the
// headline proof, and confirm on the Accept screen.
async function approveOnDevice() {
  const appeared = await waitUntil(
    (text) => text.length > 0 && !/app is ready/i.test(text),
    10000
  );
  if (!appeared) {
    throw new Error("device never left the home screen; signing APDU may not have arrived");
  }

  let savedHeadline = false;
  let screenIndex = 0;
  let lastText = "";

  for (let step = 0; step < 80; step++) {
    const text = await currentScreenText();

    if (text && text !== lastText) {
      screenIndex++;
      await saveScreenshot(`flow-${String(screenIndex).padStart(2, "0")}.png`);
      lastText = text;
      console.error(`[signer] screen ${screenIndex}: ${text}`);
    }

    if (!savedHeadline && /amount/i.test(text)) {
      await saveScreenshot("01-speculos-eth-review.png");
      savedHeadline = true;
    }

    if (/accept and send|approve|hold to sign|sign transaction/i.test(text)) {
      if (!savedHeadline) {
        await saveScreenshot("01-speculos-eth-review.png");
        savedHeadline = true;
      }
      await saveScreenshot("01b-speculos-accept.png");
      await pressButton("both");
      return;
    }

    await pressButton("right");
    await sleep(150);
  }

  throw new Error(`never reached the Accept screen; last screen seen: "${lastText}"`);
}

function toYParity(vHex) {
  let value = parseInt(vHex, 16);
  if (Number.isNaN(value)) {
    value = 0;
  }
  if (value >= 27) {
    value -= 27;
  }
  return value & 1;
}

async function main() {
  const intent = JSON.parse(await readStdin());

  const unsignedTx = Transaction.from({
    type: 2,
    chainId: Number(intent.chainId),
    nonce: Number(intent.nonce || 0),
    to: getAddress(intent.to),
    value: BigInt(intent.valueWei || "0"),
    gasLimit: BigInt(intent.gasLimit || 21000),
    maxFeePerGas,
    maxPriorityFeePerGas,
    data: intent.data && intent.data !== "0x" ? intent.data : "0x",
  });
  const unsignedHex = unsignedTx.unsignedSerialized.slice(2);

  console.error(`[signer] signing ${intent.valueWei} wei -> ${intent.to} on chain ${intent.chainId}`);
  const transport = await SpeculosHttpTransport.open({ apiPort });
  try {
    const ethApp = new Eth(transport);
    const signingPromise = ethApp.signTransaction(derivationPath, unsignedHex, null);
    await approveOnDevice();
    const signature = await signingPromise;

    unsignedTx.signature = Signature.from({
      r: "0x" + signature.r.padStart(64, "0"),
      s: "0x" + signature.s.padStart(64, "0"),
      yParity: toYParity(signature.v),
    });

    const result = {
      signedTx: unsignedTx.serialized,
      deviceApproved: true,
      txHash: unsignedTx.hash,
      from: unsignedTx.from,
    };
    process.stdout.write(JSON.stringify(result) + "\n", () => {
      transport.close().finally(() => process.exit(0));
    });
  } catch (error) {
    await transport.close().catch(() => {});
    throw error;
  }
}

main().catch((error) => {
  process.stderr.write(`[signer] ${(error && error.stack) || error}\n`);
  process.exit(1);
});
