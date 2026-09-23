"""Submission retries must recover a thread created before its entry step."""
import importlib.util
import sys
import unittest
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import AsyncMock, patch


class Duplicate(Exception):
    pass


class SubmitWorkTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        self.existing = SimpleNamespace(id="thread-1", steps=AsyncMock(return_value=[]))
        self.extension = SimpleNamespace(
            connection=SimpleNamespace(get_thread_by_ref=AsyncMock(return_value=self.existing)),
            thread=AsyncMock(return_value=SimpleNamespace(thread_id="thread-1")),
            record_step=AsyncMock(),
        )
        modules = {name: ModuleType(name) for name in (
            "harnest", "harnest.context", "harnest.extensions", "harnest.extensions.threadify", "harnest.tool"
        )}
        modules["harnest.context"].context = SimpleNamespace(current=lambda: SimpleNamespace(invocation_id="invocation-1"))
        modules["harnest.extensions.threadify"].threadify = self.extension
        modules["harnest.extensions.threadify"].is_duplicate_error = lambda error: isinstance(error, Duplicate)
        modules["harnest.tool"].tool = lambda function: function
        source = Path(__file__).resolve().parents[1] / "dispatcher/tools/submit_work.py"
        spec = importlib.util.spec_from_file_location("submission_regression", source)
        self.module = importlib.util.module_from_spec(spec)
        with patch.dict(sys.modules, modules):
            spec.loader.exec_module(self.module)

    async def test_existing_thread_without_entry_step_is_resumed_and_submitted(self):
        result = await self.module.submit_work("request")
        self.assertEqual(result, {"thread_id": "thread-1", "status": "submitted"})
        self.extension.thread.assert_awaited_once_with("nanite:invocation-1", None)
        self.assertEqual(self.extension.record_step.await_args.kwargs["idempotency_key"], "invocation-1")

    async def test_persisted_submission_avoids_writing_a_closed_thread(self):
        self.existing.steps.return_value = [SimpleNamespace(status="success")]
        result = await self.module.submit_work("request")
        self.assertEqual(result["status"], "already_submitted")
        self.existing.steps.assert_awaited_once_with(step_name="work_requested", idempotency_key="invocation-1", status="success")
        self.extension.thread.assert_not_awaited()

    async def test_retry_racing_archival_uses_idempotent_submission(self):
        self.extension.connection.get_thread_by_ref.return_value = None
        self.extension.record_step.side_effect = Duplicate()
        result = await self.module.submit_work("request")
        self.assertEqual(result["status"], "already_submitted")
        self.assertEqual(self.extension.thread.await_args.args[0], "nanite:invocation-1")
        self.assertIn("contract", self.extension.thread.await_args.args[1])

    async def test_failed_submission_is_retried_on_same_key(self):
        self.extension.record_step.side_effect = [RuntimeError("transport closed"), None]
        with self.assertRaisesRegex(RuntimeError, "transport closed"):
            await self.module.submit_work("request")
        self.assertEqual((await self.module.submit_work("request"))["status"], "submitted")
        self.assertEqual(self.extension.record_step.await_count, 2)
