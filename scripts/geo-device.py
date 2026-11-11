"""Create separate private credentials for Notes -> Location; never print them."""
import hashlib
import os
from pathlib import Path
import secrets

os.umask(0o077)
root = Path(__file__).resolve().parents[1] / 'secrets'
root.mkdir(exist_ok=True)
for name in ('geo-location.env', 'geo-notes.env'):
    if (root / name).exists():
        raise SystemExit('Geo credentials already exist; reuse them or rotate deliberately')
token = secrets.token_urlsafe(32)
(root / 'geo-location.env').write_text('GEO_TOKEN_SHA256=' + hashlib.sha256(token.encode()).hexdigest() + '\n')
(root / 'geo-notes.env').write_text('GEO_LOCATION_TOKEN=' + token + '\n')
print('Saved private credentials in secrets/geo-location.env and secrets/geo-notes.env')
