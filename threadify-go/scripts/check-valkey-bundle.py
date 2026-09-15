"""Fail release assembly if either supported platform's bundle is missing/wrong."""
from pathlib import Path
import struct

for target in ("linux_amd64", "darwin_arm64"):
    root = Path("bundle") / target
    binary = root / "valkey-server"
    data = binary.read_bytes()[:64]
    if target == "linux_amd64":
        assert data[:6] == b"\x7fELF\x02\x01", "Linux Valkey must be 64-bit little-endian ELF"
        assert struct.unpack_from("<H", data, 18)[0] == 62, "Linux Valkey must be amd64"
    else:
        assert data[:4] == b"\xcf\xfa\xed\xfe", "macOS Valkey must be Mach-O 64-bit"
        assert struct.unpack_from("<I", data, 4)[0] == 0x100000C, "macOS Valkey must be arm64"
    assert binary.stat().st_mode & 0o111, f"{binary} must be executable"
    notices = (root / "VALKEY-LICENSES.txt").read_text()
    assert "Valkey 8.1.10" in notices and "Copyright" in notices, "Valkey notices are missing"
print("Linux and macOS Valkey bundles verified")
