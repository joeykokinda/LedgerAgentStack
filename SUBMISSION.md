# Submission sheet: Ledger "Build & Show"

Fill the placeholders, then paste each value into the matching Google Form field. Nothing here is auto-submitted; this is a copy source.

## Form fields

| Form field | Value to paste |
|---|---|
| **Full Name** | `[PLACEHOLDER: your full name]` |
| **Email address** | `[PLACEHOLDER: your email; must match your contact email]` |
| **Which University & Blockchain club** | `[PLACEHOLDER: university + blockchain club name]` |
| **Link of your post** | `[PLACEHOLDER: paste the public X or LinkedIn URL after posting]` |
| **Which component did you use?** | `DMK` |
| **Proof you used the DMK/CLI** | GitHub repo `[PLACEHOLDER repo URL]` + screenshots `proof/04-dmk-sign.png` (DMK signing) and `proof/01-device-strip.png` (device review + sign) |
| **T&C accept + content reuse** | `Yes` |
| **Repo GitHub link (optional)** | `[PLACEHOLDER repo URL]` |
| **Your X / social handle** | `[PLACEHOLDER: @yourhandle]` |

## Notes for specific fields

**Which component did you use? → DMK (Ledger Device Management Kit).**
This is genuinely backed. `signer/dmk-sign.js` connects to the emulated device through the public DMK Speculos transport (`@ledgerhq/device-transport-kit-speculos`), reads the Ethereum account, and signs an EIP-1559 transaction via `@ledgerhq/device-signer-kit-ethereum`. Proof: `proof/04-dmk-sign.png`. The repo also drives the same flow through the ledgerjs device libraries (`signer/speculos-sign.js`), and installs Ledger's agent-skills (the `wallet-cli-usage` skill lands under `.agents/skills/`).
Justification to paste if the form has a text box: *"Agent Airlock gates an AI agent's transaction intents with a deterministic Go policy engine, then signs approved intents on a Ledger device through the Device Management Kit, demonstrated end to end against the Speculos emulator."*
Switch this answer to **`Both`** only if you also run the actual Ledger Wallet CLI binary; installing the wallet-cli skill alone does not back a "Wallet CLI" claim.

**Proof you used the DMK/CLI.**
Paste the GitHub repo link plus a signing-flow screenshot. One-line description to paste alongside it: *"Speculos running Ledger's Ethereum app, signing a transaction driven through the Ledger Device Management Kit, the hardware approval gate that the airlock hands a policy-cleared intent to. Repo includes the Go policy firewall and both the DMK and ledgerjs signer integrations."*

## PRE-SUBMIT CHECKLIST

- [ ] Post is public (not draft, not connections-only, not protected).
- [ ] **@Ledger** is tagged in the post body.
- [ ] **#Sponsored** (or #LedgerSponsor) is visible in the post body itself, NOT in a reply or comment.
- [ ] BOTH mandatory links are in the post:
  - [ ] https://developers.ledger.com/docs/ai-tools/overview
  - [ ] https://github.com/LedgerHQ/agent-skills
- [ ] Proof attached: signing-flow screenshot (`proof/04-dmk-sign.png` or `proof/01-device-strip.png`) and/or the repo link.
- [ ] No financial / price / "buy" / token-speculation language anywhere in the post.
- [ ] No security claim that the architecture doesn't actually back (the "what it does NOT do" honesty is intact).
- [ ] Post does NOT lead with Ledger's hardware-wallet history; Ledger is framed as an entrant in AI/agent infrastructure.
- [ ] "Link of your post" field in the form points at the live post URL.
- [ ] Submitted before the **June 12** deadline.
