#!/usr/bin/env python3
# Render the real captured demo output to a clean terminal-style PNG source.
# Faithful: it embeds the exact text from proof/agent-demo-run.txt, no edits.
import html
import pathlib

base = pathlib.Path(__file__).parent


def page(title, body_text):
    escaped = html.escape(body_text)
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


run_text = (base / "agent-demo-run.txt").read_text()
(base / "_run.html").write_text(page("agent-demo  —  Agent Airlock policy firewall", run_text))
print("wrote _run.html")
