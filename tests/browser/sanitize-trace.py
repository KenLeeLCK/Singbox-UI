"""Replace known fixture secrets in every trace member before retaining artifacts."""
import json
import os
import sys
import zipfile

source = sys.argv[1]
secrets = [s.encode() for s in json.load(sys.stdin) if s]
try:
    with zipfile.ZipFile(source) as archive, zipfile.ZipFile(source + '.safe', 'w', zipfile.ZIP_DEFLATED) as out:
        for item in archive.infolist():
            data = archive.read(item)
            for secret in secrets:
                data = data.replace(secret, b'x' * len(secret))
            out.writestr(item, data)
    os.replace(source + '.safe', source)
except Exception:
    # Never leave an unsanitized trace eligible for CI artifact upload.
    os.unlink(source)
    raise
