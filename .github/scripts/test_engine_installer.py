"""Exercise the installer offline with real archives, hashes and disposable destinations."""
import hashlib
import io
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
import zipfile

ROOT = Path(__file__).resolve().parents[2]


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="threadify installer ")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.commands = self.root / "commands"
        self.commands.mkdir()
        self.fixtures = self.root / "releases"
        self.fixtures.mkdir()
        self.bin = self.root / "my bin"
        self.config = self.root / "my config"
        self.env = dict(os.environ, PATH=f"{self.commands}{os.pathsep}{os.environ['PATH']}",
                        FIXTURES=str(self.fixtures), TEST_OS="Linux", TEST_ARCH="x86_64")
        self.command("uname", """import os,sys
print(os.environ['TEST_OS' if sys.argv[1] == '-s' else 'TEST_ARCH'])
""")
        self.command("curl", """import os,sys,shutil
from pathlib import Path
a=sys.argv[1:]; url=a[-1]
assert url.startswith('https://github.com/creativeJoe007/ThreadifyEngine/releases/')
assert '--proto' in a and a[a.index('--proto')+1] == '=https'
assert '--proto-redir' in a and a[a.index('--proto-redir')+1] == '=https'
if os.environ.get('FAIL_DOWNLOAD'): sys.exit(22)
if url.endswith('/latest'):
    print('https://github.com/creativeJoe007/ThreadifyEngine/releases/tag/v1.2.3',end='')
else:
    assert '/download/v1.2.3/' in url
    source=Path(os.environ['FIXTURES']) / url.rsplit('/',1)[1]
    if not source.exists(): sys.exit(22)
    shutil.copyfile(source,a[a.index('--output')+1])
""")

    def command(self, name, script):
        path = self.commands / name
        path.write_text(f"#!{sys.executable}\n" + script)
        path.chmod(0o755)

    def release(self, target="linux_amd64", binary=True, checksum="valid", bundled=False):
        is_windows = target.startswith("windows")
        name = f"threadify_1.2.3_{target}." + ("zip" if is_windows else "tar.gz")
        archive = self.fixtures / name
        files = {"config/config.yaml": b"server:\n  port: 8081\n", "config/subscription.yaml": b"tiers: []\n"}
        if binary:
            files["threadify.exe" if is_windows else "threadify"] = b"verified test binary\n"
        if bundled:
            files["libexec/valkey-server"] = b"bundled test valkey\n"
            files["libexec/VALKEY-LICENSES.txt"] = b"test notices\n"
        if is_windows:
            with zipfile.ZipFile(archive, "w") as output:
                for filename, data in files.items():
                    output.writestr(filename, data)
        else:
            with tarfile.open(archive, "w:gz") as output:
                for filename, data in files.items():
                    entry = tarfile.TarInfo(filename)
                    entry.size = len(data)
                    output.addfile(entry, io.BytesIO(data))
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        line = f"{digest}  {name}\n"
        if checksum == "wrong": line = f"{'0' * 64}  {name}\n"
        if checksum == "missing": line = f"{digest}  another-file.tar.gz\n"
        if checksum == "duplicate": line += line
        (self.fixtures / "checksums.txt").write_text(line)

    def run_installer(self, *args, success=True):
        result = subprocess.run(["sh", str(ROOT / "install.sh"), "--bin-dir", str(self.bin),
                                 "--config-dir", str(self.config), *args], env=self.env,
                                text=True, capture_output=True, timeout=20)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout)
        return result

    def test_latest_install_and_reinstall_preserve_configs(self):
        self.release()
        self.run_installer()
        self.assertEqual((self.bin / "threadify").read_bytes(), b"verified test binary\n")
        self.assertTrue(os.access(self.bin / "threadify", os.X_OK))
        (self.config / "config.yaml").write_text("existing operator settings")
        (self.config / "subscription.yaml").write_text("existing subscription")
        self.run_installer("--version", "1.2.3")
        self.assertEqual((self.config / "config.yaml").read_text(), "existing operator settings")
        self.assertEqual((self.config / "subscription.yaml").read_text(), "existing subscription")
        self.assertEqual(list(self.bin.glob(".threadify-install.*")), [])

    def test_bundled_valkey_installed_with_notices(self):
        self.release(bundled=True)
        self.run_installer()
        self.assertEqual((self.bin / "libexec/valkey-server").read_bytes(), b"bundled test valkey\n")
        self.assertTrue(os.access(self.bin / "libexec/valkey-server", os.X_OK))
        self.assertEqual((self.bin / "libexec/VALKEY-LICENSES.txt").read_bytes(), b"test notices\n")
        self.run_installer()
        self.assertEqual(list((self.bin / "libexec").glob(".threadify-install.*")), [])

    def test_platform_archive_selection(self):
        for system, arch, target, binary in [("Darwin", "arm64", "darwin_arm64", "threadify"),
                                              ("MINGW64_NT-10.0", "x86_64", "windows_amd64", "threadify.exe")]:
            with self.subTest(target=target):
                self.env.update(TEST_OS=system, TEST_ARCH=arch)
                self.release(target)
                self.run_installer("--version", "v1.2.3")
                self.assertEqual((self.bin / binary).read_bytes(), b"verified test binary\n")

    def test_unsupported_platform_does_not_install(self):
        self.env["TEST_ARCH"] = "aarch64"
        result = self.run_installer(success=False)
        self.assertIn("No release binary", result.stderr)
        self.assertFalse(self.bin.exists())

    def test_checksum_failure_preserves_existing_binary(self):
        self.bin.mkdir()
        (self.bin / "threadify").write_text("old binary")
        for mode in ["wrong", "missing", "duplicate"]:
            with self.subTest(mode=mode):
                self.release(checksum=mode)
                self.run_installer(success=False)
                self.assertEqual((self.bin / "threadify").read_text(), "old binary")
                self.assertFalse(self.config.exists())

    def test_download_failure_does_not_install(self):
        self.env["FAIL_DOWNLOAD"] = "1"
        self.run_installer("--version", "1.2.3", success=False)
        self.assertFalse(self.bin.exists())

    def test_missing_binary_does_not_install(self):
        self.release(binary=False)
        self.run_installer(success=False)
        self.assertFalse(self.bin.exists())

    def test_bad_arguments_fail(self):
        for args in [("--version", "v1.2.3/other"), ("--version", ""), ("--unknown",), ("--bin-dir",)]:
            with self.subTest(args=args):
                self.run_installer(*args, success=False)
                self.assertFalse(self.bin.exists())

    def test_help_does_not_download(self):
        self.env["FAIL_DOWNLOAD"] = "1"
        self.assertIn("Usage:", self.run_installer("--help").stdout)
        self.assertFalse(self.bin.exists())


if __name__ == "__main__":
    unittest.main()
