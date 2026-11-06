#!/usr/bin/env python3
"""Pinned release download, verification and safe archive extraction."""
import argparse, hashlib, pathlib, re, tarfile, urllib.request
SINGBOX = {
    'amd64': '699a1b510aa4396b2176d32d1181920399ebf1e267835da6f3b598d0f7c24968',
    'arm64': '436441f76ee2f7fe21b5b2d426d3cb261a480455a704865789ad533e4273ec26',
}
def download(url, path):
    if not url.startswith('https://github.com/'):
        raise ValueError('Only HTTPS GitHub release URLs are accepted')
    with urllib.request.urlopen(url, timeout=120) as src, open(path, 'wb') as dst:
        while block := src.read(1024 * 1024):
            dst.write(block)
def verify(path, expected):
    if not re.fullmatch(r'[a-f0-9]{64}', expected):
        raise ValueError('Invalid SHA256 checksum')
    h = hashlib.sha256()
    with open(path, 'rb') as src:
        while block := src.read(1024 * 1024): h.update(block)
    if h.hexdigest() != expected:
        raise ValueError('SHA256 mismatch; nothing installed')
def extract(path, target):
    target = pathlib.Path(target).resolve()
    with tarfile.open(path, 'r:gz') as archive:
        for item in archive.getmembers():
            dest = (target / item.name).resolve()
            if not dest.is_relative_to(target) or not (item.isfile() or item.isdir()):
                raise ValueError('Unsafe archive entry: ' + item.name)
        archive.extractall(target)
def main():
    p = argparse.ArgumentParser()
    p.add_argument('kind', choices=['manager', 'singbox'])
    p.add_argument('--arch', choices=['amd64', 'arm64'], required=True)
    p.add_argument('--directory', required=True)
    p.add_argument('--repo'); p.add_argument('--version')
    args = p.parse_args(); d = pathlib.Path(args.directory); d.mkdir(parents=True, exist_ok=True)
    if args.kind == 'singbox':
        name = f'sing-box-1.14.4-linux-{args.arch}.tar.gz'
        url = f'https://github.com/SagerNet/sing-box/releases/download/v1.14.4/{name}'
        checksum = SINGBOX[args.arch]
    else:
        if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', args.repo or ''): p.error('Explicit owner/repository required')
        if not re.fullmatch(r'v\d+\.\d+\.\d+(?:[.-][A-Za-z0-9.-]+)?', args.version or ''): p.error('Pinned vX.Y.Z version required')
        name = f'sing-box-manager-{args.version}-linux-{args.arch}.tar.gz'
        base = f'https://github.com/{args.repo}/releases/download/{args.version}'
        download(base + '/SHA256SUMS', d / 'SHA256SUMS')
        matches = [line.split()[0] for line in (d / 'SHA256SUMS').read_text().splitlines() if len(line.split()) == 2 and line.split()[1].lstrip('*') == name]
        if len(matches) != 1: raise ValueError('Missing or duplicate release checksum')
        checksum = matches[0]; url = base + '/' + name
    download(url, d / name); verify(d / name, checksum); extract(d / name, d / 'unpacked')
if __name__ == '__main__': main()
