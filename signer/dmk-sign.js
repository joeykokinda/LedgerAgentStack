// dmk-sign.js
//
// Genuine Ledger Device Management Kit (DMK) usage against Speculos, with no
// physical hardware. This is the "modern SDK" path, distinct from the ledgerjs
// hw-app-eth signer in speculos-sign.js. It:
//
//   1. builds a DMK with the public Speculos transport
//   2. discovers + connects to the emulated device (real session)
//   3. reads the Ethereum address from the device via device-signer-kit-ethereum
//   4. signs an EIP-1559 transaction, auto-approving on the emulator screen
//
// Proof of DMK device interaction is written to ../proof. stdout gets one JSON
// result line; all diagnostics go to stderr.
console.log = (...args) => console.error(...args);

const fs = require("fs");
const path = require("path");

const { DeviceManagementKitBuilder, DeviceActionStatus } = require("@ledgerhq/device-management-kit");
const { speculosTransportFactory } = require("@ledgerhq/device-transport-kit-speculos");
const { SignerEthBuilder } = require("@ledgerhq/device-signer-kit-ethereum");
const { firstValueFrom, filter } = require("rxjs");
const { Transaction, getBytes } = require("ethers");

const apiPort = Number(process.env.SPECULOS_API_PORT || 5000);
const apiUrl = `http://localhost:${apiPort}`;
const derivationPath = process.env.LEDGER_PATH || "44'/60'/0'/0/0";
const proofDir = path.resolve(__dirname, "..", "proof");

const sleep = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds));

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
  fs.mkdirSync(proofDir, { recursive: true });
  fs.writeFileSync(path.join(proofDir, filename), Buffer.from(await response.arrayBuffer()));
  console.error(`[dmk] captured ${filename}`);
}

// Page right until an approve screen, capture it, then confirm with both buttons.
async function autoApprove(captureName, approveRe) {
  for (let step = 0; step < 80; step++) {
    const text = await currentScreenText();
    if (approveRe.test(text)) {
      if (captureName) {
        await saveScreenshot(captureName);
      }
      await pressButton("both");
      return text;
    }
    await pressButton("right");
    await sleep(150);
  }
  return null;
}

async function deviceActionOutput(deviceActionResult) {
  const finalState = await firstValueFrom(
    deviceActionResult.observable.pipe(
      filter(
        (state) =>
          state.status === DeviceActionStatus.Completed || state.status === DeviceActionStatus.Error
      )
    )
  );
  if (finalState.status === DeviceActionStatus.Error) {
    throw new Error("DMK device action failed: " + JSON.stringify(finalState.error));
  }
  return finalState.output;
}

function normalizeHex(value) {
  return value.startsWith("0x") ? value : "0x" + value;
}

async function main() {
  const dmk = new DeviceManagementKitBuilder()
    .addTransport(speculosTransportFactory(apiUrl))
    .build();

  const discovered = await firstValueFrom(dmk.startDiscovering({}));
  const device = Array.isArray(discovered) ? discovered[0] : discovered;
  console.error("[dmk] discovered:", device && (device.name || device.deviceModel?.model || device.id));

  const sessionId = await dmk.connect({ device });
  console.error("[dmk] connected, session:", sessionId);

  const signer = new SignerEthBuilder({ dmk, sdk: dmk, sessionId }).build();

  // 1) Read the Ethereum address from the device over the DMK (no approval needed).
  const address = await deviceActionOutput(signer.getAddress(derivationPath, { checkOnDevice: false }));
  console.error("[dmk] getAddress ->", JSON.stringify(address));

  const result = {
    method: "DMK",
    transport: "SPECULOS_HTTP_TRANSPORT",
    address: address.address,
  };

  // 2) Sign an EIP-1559 transaction via device-signer-kit-ethereum (best effort:
  //    proves the full DMK signing flow when the app-open path behaves on the emulator).
  try {
    const unsignedTx = Transaction.from({
      type: 2,
      chainId: 11155111,
      nonce: 7,
      to: "0x1111111111111111111111111111111111111111",
      value: 30000000000000000n,
      gasLimit: 21000n,
      maxFeePerGas: 30000000000n,
      maxPriorityFeePerGas: 1500000000n,
      data: "0x",
    });
    const signingResult = signer.signTransaction(derivationPath, getBytes(unsignedTx.unsignedSerialized), {});
    const approvedScreen = await autoApprove("04-dmk-sign.png", /approve|sign transaction|accept|hold to sign/i);
    const signature = await deviceActionOutput(signingResult);
    console.error("[dmk] signature ->", JSON.stringify(signature), "approved at:", approvedScreen);

    unsignedTx.signature = {
      r: normalizeHex(signature.r),
      s: normalizeHex(signature.s),
      yParity: (typeof signature.v === "string" ? parseInt(signature.v, 16) : Number(signature.v)) & 1,
    };
    result.signedTx = unsignedTx.serialized;
    result.from = unsignedTx.from;
    result.txHash = unsignedTx.hash;
    result.signedViaDMK = true;
  } catch (signError) {
    console.error("[dmk] signTransaction not completed on emulator:", signError.message);
    result.signedViaDMK = false;
    result.signNote = "DMK connected + read address; sign step skipped (" + signError.message + ")";
  }

  process.stdout.write(JSON.stringify(result) + "\n");
  try {
    await Promise.resolve(dmk.close());
  } catch (closeError) {
    /* best effort */
  }
  process.exit(0);
}

main().catch(async (error) => {
  process.stderr.write(`[dmk] ${(error && error.stack) || error}\n`);
  process.exit(1);
});
