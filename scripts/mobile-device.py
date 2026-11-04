"""Create a local mobile setup file. Never print the bearer to logs/stdout."""
import hashlib
import os
from pathlib import Path
import secrets

os.umask(0o077)
root = Path(__file__).resolve().parents[1] / "secrets"
root.mkdir(exist_ok=True)
setup = root / "mobile-device.txt"
if setup.exists():
    raise SystemExit("Device setup already exists; reuse it or rotate deliberately")
token = secrets.token_urlsafe(32)
setup.write_text(f"Server: http://100.64.0.6:18084\nToken: {token}\nRequires the existing tailnet VPN.\n")
(root / "mobile.env").write_text("MOBILE_TOKEN_SHA256=" + hashlib.sha256(token.encode()).hexdigest() + "\n")
print("Device setup saved in secrets/mobile-device.txt; server hash in secrets/mobile.env")
