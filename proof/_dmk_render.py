#!/usr/bin/env python3
# Render the real DMK run (command + logs + JSON result) to a terminal-style PNG.
# Faithful: it embeds the exact stderr/stdout captured from node signer/dmk-sign.js.
import html
import json
import pathlib

base = pathlib.Path(__file__).parent
err = pathlib.Path("/tmp/dmk.err").read_text().strip()
out = pathlib.Path("/tmp/dmk.out").read_text().strip()

try:
    pretty = json.dumps(json.loads(out), indent=2)
except Exception:
    pretty = out

body = "$ node signer/dmk-sign.js\n" + err + "\n\n" + pretty
(base / "05-dmk-run.txt").write_text(body + "\n")


def page(title, text):
    escaped = html.escape(text)
    return f"""<!doctype html><html><head><meta charset="utf-8"><style>
html,body{{margin:0;background:#0b0e14}}
.wrap{{padding:34px 40px}}
.bar{{display:flex;gap:9px;align-items:center;margin-bottom:20px}}
.dot{{width:13px;height:13px;border-radius:50%}}
.r{{background:#ff5f56}}.y{{background:#ffbd2e}}.g{{background:#27c93f}}
.t{{color:#6b7280;font:600 14px 'JetBrains Mono',monospace;margin-left:12px}}
pre{{margin:0;color:#d7dce5;font:18px/1.5 'JetBrains Mono',monospace;white-space:pre-wrap;overflow-wrap:anywhere}}
</style></head><body><div class="wrap">
<div class="bar"><span class="dot r"></span><span class="dot y"></span>
<span class="dot g"></span><span class="t">{html.escape(title)}</span></div>
<pre>{escaped}</pre></div></body></html>"""


(base / "_dmk.html").write_text(page("node signer/dmk-sign.js  —  Ledger Device Management Kit on Speculos", body))
print("wrote 05-dmk-run.txt and _dmk.html")
