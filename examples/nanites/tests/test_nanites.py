from __future__ import annotations

import importlib.util
import os
import sys
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

_ROOT = Path(__file__).resolve().parents[1]


def _load(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class _Plugin:
    def __init__(self) -> None:
        self.handlers = []

    def on(self, event, step):
        def decorate(handler):
            self.handlers.append((event, step, handler))
            return handler

        return decorate


class _Delivery:
    def __init__(self) -> None:
        self.notification = SimpleNamespace(
            thread_id="thread-1",
            to_dict=lambda: {"details": {"request": "review rollout"}},
        )
        self.recorded = []
        self.completed = []

    async def record_step(self, name, **kwargs):
        self.recorded.append((name, kwargs))

    async def complete(self, reason):
        self.completed.append(reason)


class NaniteWorkerTests(unittest.IsolatedAsyncioTestCase):
    async def test_event_invokes_agent_then_advances_terminal_step(self):
        module = _load("nanite_worker_test", _ROOT / "shared" / "nanite_worker.py")
        plugin = _Plugin()
        module.register_nanite(
            module.NaniteSpec("analysis_completed", "review_completed", "reviewer"),
            plugin=plugin,
        )
        delivery = _Delivery()
        with patch.object(module, "_invoke_self", AsyncMock(return_value="final plan")):
            await plugin.handlers[0][2](delivery)

        self.assertEqual(plugin.handlers[0][:2], ("step.success", "analysis_completed"))
        self.assertEqual(delivery.recorded[0][0], "review_completed")
        self.assertEqual(
            delivery.recorded[0][1]["context"],
            {"output": "final plan", "nanite": "reviewer"},
        )
        self.assertEqual(delivery.completed, [])

    async def test_failed_agent_does_not_advance_or_ack_in_adapter(self):
        module = _load("nanite_worker_failure", _ROOT / "shared" / "nanite_worker.py")
        plugin = _Plugin()
        module.register_nanite(
            module.NaniteSpec("work_requested", "analysis_completed", "analyst"),
            plugin=plugin,
        )
        delivery = _Delivery()
        with (
            patch.object(module, "_invoke_self", AsyncMock(side_effect=RuntimeError("failed"))),
            self.assertRaises(RuntimeError),
        ):
            await plugin.handlers[0][2](delivery)
        self.assertEqual(delivery.recorded, [])
        self.assertEqual(delivery.completed, [])


class MaterializationTests(unittest.TestCase):
    def test_storage_import_defers_runtime_credentials(self):
        with patch.dict(os.environ, {"HARNEST_NANITES_POSTGRES_URL": ""}):
            module = _load(
                "nanite_storage_without_credentials",
                _ROOT / "shared" / "nanite_storage.py",
            )
            with self.assertRaisesRegex(
                RuntimeError, "HARNEST_NANITES_POSTGRES_URL is required"
            ):
                module._store()

    def test_materializes_one_plugin_source_and_detects_staleness(self):
        module = _load("nanites_prepare_test", _ROOT / "prepare.py")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            plugin = root / "source-plugin"
            (plugin / "lib").mkdir(parents=True)
            (plugin / "plugin.yaml").write_text("kind: RuntimePlugin\n", encoding="utf-8")
            (plugin / "lib" / "client.py").write_text("VALUE = 1\n", encoding="utf-8")
            shared = root / "shared"
            shared.mkdir()
            (shared / "nanite_model.py").write_text("MODEL = 1\n", encoding="utf-8")
            (shared / "nanite_worker.py").write_text("WORKER = 1\n", encoding="utf-8")
            (shared / "nanite_storage.py").write_text("STORE = 1\n", encoding="utf-8")

            module.materialize(root, plugin)
            self.assertTrue(module.check(root, plugin))
            (root / "analyst" / "plugins" / "threadify" / "plugin.yaml").write_text(
                "changed\n", encoding="utf-8"
            )
            self.assertFalse(module.check(root, plugin))


if __name__ == "__main__":
    unittest.main()
