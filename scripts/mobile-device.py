"""Create a local mobile setup file. Never print the bearer to logs/stdout."""
import hashlib
import os
from pathlib import Path
import secrets
import sys

os.umask(0o077)
if sys.argv[1:] not in ([], ["ios"]):
    raise SystemExit("Usage: mobile-device.py [ios]")
device = "ios" if sys.argv[1:] else "mobile"
hash_key = "IOS_TOKEN_SHA256" if device == "ios" else "MOBILE_TOKEN_SHA256"
root = Path(__file__).resolve().parents[1] / "secrets"
root.mkdir(exist_ok=True)
setup = root / f"{device}-device.txt"
host_config = root / f"{device}.env"
if setup.exists() or host_config.exists():
    raise SystemExit("Device setup already exists; reuse it or rotate deliberately")
token = secrets.token_urlsafe(32)
with setup.open("x") as setup_file:
    setup_file.write(f"Server: http://100.64.0.6:18084\nToken: {token}\nRequires the existing tailnet VPN.\n")
with host_config.open("x") as config_file:
    config_file.write(f"{hash_key}={hashlib.sha256(token.encode()).hexdigest()}\n")
print(f"Device setup saved in secrets/{setup.name}; server hash in secrets/{host_config.name}")
