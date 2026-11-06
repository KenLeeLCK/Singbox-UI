"""Verify the service PID owns the selected loopback listener, not a stale port.

Linux procfs links socket file descriptors to the TCP table by inode. Run as
root from the installer; inability to inspect a process fails health closed.
"""
import os
import pathlib
import sys


def owns_listener(pid, port, proc=pathlib.Path('/proc')):
    if pid <= 0 or not 1 <= port <= 65535:
        return False
    process = proc / str(pid)
    try:
        sockets = set()
        for fd in (process / 'fd').iterdir():
            try:
                target = os.readlink(fd)
            except FileNotFoundError:
                continue  # An unrelated descriptor can close during inspection.
            if target.startswith('socket:[') and target.endswith(']'):
                sockets.add(target[8:-1])
        # Supported Linux amd64/arm64 targets use little-endian IPv4 addresses.
        address = f'0100007F:{port:04X}'
        for line in (process / 'net/tcp').read_text().splitlines()[1:]:
            fields = line.split()
            if (len(fields) >= 10 and fields[1] == address
                    and fields[3] == '0A' and fields[9] in sockets):
                return True
    except OSError:
        return False
    return False


if __name__ == '__main__':
    try:
        healthy = len(sys.argv) == 3 and owns_listener(int(sys.argv[1]), int(sys.argv[2]))
    except ValueError:
        healthy = False
    sys.exit(0 if healthy else 1)
