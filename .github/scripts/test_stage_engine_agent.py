"""Release staging rejects missing or substituted platform runtimes."""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("stage_agent", Path(__file__).with_name("stage-engine-agent.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
check_spec = importlib.util.spec_from_file_location("check_agent", Path(__file__).resolve().parents[2] / "threadify-go/scripts/check-agent-bundle.py")
checker = importlib.util.module_from_spec(check_spec)
check_spec.loader.exec_module(checker)



class StageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.inputs = self.root / "inputs"
        self.assets = self.root / "internal/agentbundle/assets"
        self.releases = self.root / "agent-release"
        for platform in module.PLATFORMS:
            target = platform.replace("/", "-")
            directory = self.inputs / f"agent-runtime-{target}"
            directory.mkdir(parents=True)
            archive = directory / f"runtime-{target}.tar.gz"
            archive.write_bytes(platform.encode())
            manifest = {platform: {"archive": archive.name, "sha256": hashlib.sha256(archive.read_bytes()).hexdigest(),
                                   "python": "python/python.exe" if platform.startswith("windows/") else "python/bin/python3.12"}}
            (directory / f"manifest-{target}.json").write_text(json.dumps(manifest))
        with tarfile.open(self.inputs / "agent-runtime-linux-amd64/agent.tar.gz", "w:gz") as archive:
            for name in ("harnest-manifest.json", "launch.py"):
                content = b"fixture"
                info = tarfile.TarInfo(name)
                info.size = len(content)
                archive.addfile(info, io.BytesIO(content))

    def tearDown(self):
        self.temp.cleanup()

    def stage(self):
        module.stage(self.inputs, self.assets, self.releases)

    def test_stages_all_platforms_and_only_linux_agent(self):
        self.stage()
        self.assertEqual(set(json.loads((self.assets / "runtimes.json").read_text())), set(module.PLATFORMS))
        self.assertEqual(len(list(self.releases.glob("*.tar.gz"))), 3)
        self.assertEqual((self.assets / "agent.tar.gz").read_bytes(), (self.inputs / "agent-runtime-linux-amd64/agent.tar.gz").read_bytes())

    def test_release_gate_accepts_complete_stage_and_rejects_missing_archive(self):
        self.stage()
        checker.check(self.root)
        (self.releases / "runtime-windows-amd64.tar.gz").unlink()
        with self.assertRaises(FileNotFoundError):
            checker.check(self.root)

    def test_release_gate_rejects_partial_platform_manifest(self):
        self.stage()
        manifest = self.assets / "runtimes.json"
        contents = json.loads(manifest.read_text())
        del contents["linux/amd64"]
        manifest.write_text(json.dumps(contents))
        with self.assertRaisesRegex(ValueError, "requires pinned runtimes"):
            checker.check(self.root)

    def test_tampered_runtime_is_rejected_before_staging(self):
        (self.inputs / "agent-runtime-darwin-arm64/runtime-darwin-arm64.tar.gz").write_bytes(b"substituted")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            self.stage()
        self.assertFalse(self.assets.exists())

    def test_missing_platform_is_rejected(self):
        (self.inputs / "agent-runtime-windows-amd64/manifest-windows-amd64.json").unlink()
        with self.assertRaises(FileNotFoundError):
            self.stage()
        self.assertFalse(self.assets.exists())

    def test_substituted_archive_path_is_rejected(self):
        path = self.inputs / "agent-runtime-linux-amd64/manifest-linux-amd64.json"
        manifest = json.loads(path.read_text())
        manifest["linux/amd64"]["archive"] = "../../outside.tar.gz"
        path.write_text(json.dumps(manifest))
        with self.assertRaisesRegex(ValueError, "runtime layout"):
            self.stage()

    def test_incomplete_agent_is_rejected(self):
        with tarfile.open(self.inputs / "agent-runtime-linux-amd64/agent.tar.gz", "w:gz"):
            pass
        with self.assertRaisesRegex(ValueError, "entry points"):
            self.stage()


if __name__ == "__main__":
    unittest.main()
