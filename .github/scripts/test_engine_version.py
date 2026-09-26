"""Exercise version decisions against disposable Git histories."""
import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("engine_version", Path(__file__).with_name("engine-version.py"))
version = importlib.util.module_from_spec(spec)
spec.loader.exec_module(version)


class EngineVersionTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.previous_dir = os.getcwd()
        os.chdir(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Release test")
        self.git("config", "user.email", "release@example.test")

    def tearDown(self):
        os.chdir(self.previous_dir)
        self.temp.cleanup()

    def git(self, *args):
        return subprocess.check_output(["git", *args], text=True).strip()

    def commit(self, message, path="threadify-go/main.go"):
        target = Path(path)
        target.parent.mkdir(parents=True, exist_ok=True)
        with target.open("a") as stream:
            stream.write(message + "\n")
        self.git("add", path)
        self.git("commit", "-qm", message)

    def test_first_engine_release_ignores_sdk_tags(self):
        self.commit("feat: initial engine")
        self.git("tag", "threadify-sdk-go-v9.0.0")
        self.assertEqual(version.next_release(), ("v0.1.0", ""))

    def test_patch_minor_and_breaking_footer(self):
        self.commit("feat: initial")
        self.git("tag", "v1.2.3")
        self.commit("fix: persistence")
        self.assertEqual(version.next_release(), ("v1.2.4", "v1.2.3"))
        self.commit("feat(otel): ingestion")
        self.assertEqual(version.next_release(), ("v1.3.0", "v1.2.3"))
        self.commit("refactor: schema\n\nBREAKING CHANGE: new schema")
        self.assertEqual(version.next_release(), ("v2.0.0", "v1.2.3"))

    def test_breaking_subject_and_retry(self):
        self.commit("fix(engine)!: remove old protocol")
        self.assertEqual(version.next_release(), ("v1.0.0", ""))
        self.git("tag", "v1.0.0")
        self.assertEqual(version.next_release(), ("v1.0.0", ""))
        self.commit("fix: next")
        self.git("tag", "v1.0.1")
        self.assertEqual(version.next_release(), ("v1.0.1", "v1.0.0"))

    def test_unrelated_changes_do_not_release_or_bump(self):
        self.commit("feat: initial")
        self.git("tag", "v1.0.0")
        for path in (
            "homepage/main.ts", "threadify-sdk-go/sdk.go", "README.md", "threadify-go/README.md",
            "threadify-go/tests/e2e/README.md",
        ):
            self.commit("feat!: unrelated", path)
        self.assertEqual(version.next_release(), ("", "v1.0.0"))
        self.commit("fix: engine")
        self.assertEqual(version.next_release(), ("v1.0.1", "v1.0.0"))

    def test_dashboard_changes_create_engine_releases(self):
        self.commit("feat: initial")
        self.git("tag", "v1.0.0")
        self.commit("fix: dashboard", "web/app/root.tsx")
        self.assertEqual(version.next_release(), ("v1.0.1", "v1.0.0"))
        self.git("tag", "v1.0.1")
        self.commit("build: node version", ".nvmrc")
        self.assertEqual(version.next_release(), ("v1.0.2", "v1.0.1"))

    def test_bundled_agent_changes_create_engine_releases(self):
        self.commit("feat: initial")
        self.git("tag", "v1.0.0")
        self.commit("fix: agent tool", "threadify-agent/tools/get_thread.py")
        self.assertEqual(version.next_release(), ("v1.0.1", "v1.0.0"))

    def test_installer_changes_create_engine_releases(self):
        self.commit("feat: initial")
        self.git("tag", "v1.0.0")
        self.commit("feat: binary installer", "install.sh")
        self.assertEqual(version.next_release(), ("v1.1.0", "v1.0.0"))
        self.git("tag", "v1.1.0")
        self.commit("test: installer validation", ".github/scripts/test_engine_installer.py")
        self.assertEqual(version.next_release(), ("v1.1.1", "v1.1.0"))

    def test_shared_build_action_and_packaged_docs_create_releases(self):
        self.commit("feat: initial")
        self.git("tag", "v1.0.0")
        self.commit("fix: build cache", ".github/actions/setup-engine-go/action.yml")
        self.assertEqual(version.next_release(), ("v1.0.1", "v1.0.0"))
        self.git("tag", "v1.0.1")
        self.commit("docs: deployment", "threadify-go/SELF_HOSTING.md")
        self.assertEqual(version.next_release(), ("v1.0.2", "v1.0.1"))

    def test_unmerged_and_prerelease_tags_do_not_set_version(self):
        self.commit("feat: initial")
        self.git("tag", "v1.0.0")
        branch = self.git("branch", "--show-current")
        self.git("checkout", "-qb", "other")
        self.commit("feat: future")
        self.git("tag", "v9.0.0")
        self.git("checkout", "-q", branch)
        self.commit("fix: stable")
        self.git("tag", "v2.0.0-rc.1")
        self.assertEqual(version.next_release(), ("v1.0.1", "v1.0.0"))

    def test_explicit_tag_can_start_a_new_release_series(self):
        self.commit("feat: original engine")
        self.git("tag", "v1.5.2")
        self.commit("fix: release validation")
        self.git("tag", "v0.1.0")
        old_type, old_name = os.environ.get("REF_TYPE"), os.environ.get("REF_NAME")
        os.environ.update(REF_TYPE="tag", REF_NAME="v0.1.0")
        try:
            self.assertEqual(version.next_release(), ("v0.1.0", ""))
        finally:
            for key, value in (("REF_TYPE", old_type), ("REF_NAME", old_name)):
                if value is None:
                    os.environ.pop(key, None)
                else:
                    os.environ[key] = value


if __name__ == "__main__":
    unittest.main()
