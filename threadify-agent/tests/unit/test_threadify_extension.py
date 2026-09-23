from __future__ import annotations

import asyncio
import importlib
import os
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

from harnest.extensions import ExtensionContext
from harnest.extensions import extension_namespaces
from harnest.extension_descriptors import discover_application_extensions

_ROOT = Path(__file__).resolve().parents[2]


class _Notification:
    def __init__(self, notification_id: str = "notification-1") -> None:
        self.notification_id = notification_id
        self.thread_id = "thread-1"
        self.acked = 0

    def ack(self) -> None:
        self.acked += 1


class _Step:
    def __init__(self) -> None:
        self.context = {}
        self.key = None
        self.calls = []

    def add_context(self, value):
        self.context = dict(value)
        return self

    def idempotency_key(self, value):
        self.key = value
        return self

    async def success(self, value):
        self.calls.append(("success", value))
        return {"status": "success"}

    async def failed(self, value):
        self.calls.append(("failed", value))
        return {"status": "failed"}

    async def error(self, value):
        self.calls.append(("error", value))
        return {"status": "error"}


class _Thread:
    def __init__(self) -> None:
        self.steps = []
        self.completed = []

    def step(self, name):
        step = _Step()
        self.steps.append((name, step))
        return step

    async def complete(self, reason):
        self.completed.append(reason)
        return {"status": "completed"}


class _Connection:
    def __init__(self) -> None:
        self.subscriptions = []
        self.resolved = []
        self.joined = []
        self.thread_handle = _Thread()
        self.closed = 0

    def subscribe(self, *args):
        self.subscriptions.append(args)
        return self

    async def thread(self, thread_key, options=None):
        self.resolved.append((thread_key, options))
        return self.thread_handle

    async def join(self, **kwargs):
        self.joined.append(kwargs)
        return self.thread_handle

    async def close(self):
        self.closed += 1


class ThreadifyExtensionTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        descriptors = discover_application_extensions(_ROOT)
        self.namespace = extension_namespaces(descriptors)
        activated = self.namespace.__enter__()
        self.module = importlib.import_module("harnest.extensions.threadify")
        self.extension = next(
            item.extension for item in activated if item.descriptor.name == "threadify"
        )
        # Harnest 0.12 activates extension singletons for the compiled test
        # application. Keep each unit isolated from subscriptions registered by
        # earlier tests while retaining the descriptor-bound extension identity.
        self._subscriptions = tuple(self.extension._subscriptions)
        self.extension._subscriptions.clear()
        self.connection = _Connection()

    async def asyncTearDown(self) -> None:
        if self.extension._started:
            await self.extension.stop()
        self.extension._subscriptions[:] = self._subscriptions
        self.namespace.__exit__(None, None, None)

    async def _start(self) -> None:
        env = {
            "THREADIFY_API_KEY": "private-key",
            "THREADIFY_SUBSCRIPTION_API_KEY": "",
            "THREADIFY_SERVICE_NAME": "nanite",
        }
        with (
            patch.dict(os.environ, env, clear=False),
            patch.object(
                self.module.Threadify,
                "connect",
                AsyncMock(return_value=self.connection),
            ) as connect,
        ):
            await self.extension.start(SimpleNamespace(root_agent_name="root"))
        self.assertEqual(connect.await_args.args, ("private-key",))
        self.assertEqual(connect.await_args.kwargs["service_name"], "nanite")

    async def test_separates_owner_scoped_events_from_agent_actions(self):
        action_connection = _Connection()
        event_connection = _Connection()

        @self.extension.on("step.success", "work_requested")
        async def consume(_delivery):
            return None

        env = {
            "THREADIFY_API_KEY": "analyst-key",
            "THREADIFY_SUBSCRIPTION_API_KEY": "dispatcher-event-key",
            "THREADIFY_SERVICE_NAME": "analyst",
        }
        with (
            patch.dict(os.environ, env, clear=False),
            patch.object(
                self.module.Threadify,
                "connect",
                AsyncMock(side_effect=[action_connection, event_connection]),
            ) as connect,
        ):
            await self.extension.start(SimpleNamespace(root_agent_name="root"))

        self.assertEqual(connect.await_count, 2)
        self.assertEqual(action_connection.subscriptions, [])
        self.assertEqual(len(event_connection.subscriptions), 1)

        context = self.extension.create_context(ExtensionContext(extension_name="threadify"))
        token = self.extension._bind_context(context)
        try:
            await self.extension.record_step("thread-1", "analysis_completed")
        finally:
            self.extension._reset_context(token)

        self.assertEqual(
            action_connection.joined,
            [{"thread_id": "thread-1", "role": ""}],
        )
        self.assertEqual(event_connection.joined, [])

        await self.extension.stop()
        self.assertEqual(action_connection.closed, 1)
        self.assertEqual(event_connection.closed, 1)

    async def test_exposes_sdk_and_acks_only_successful_async_deliveries(self):
        handled = []

        @self.extension.on("step.success", "work_requested")
        async def success(delivery):
            handled.append(delivery.notification.notification_id)

        @self.extension.on("step.success", "work_requested")
        async def failure(_delivery):
            raise RuntimeError("private work payload")

        await self._start()
        first = _Notification("ok")
        second = _Notification("failed")
        self.connection.subscriptions[0][-1](first)
        self.connection.subscriptions[1][-1](second)
        await asyncio.sleep(0)
        await asyncio.sleep(0)

        self.assertIs(self.module.Connection, importlib.import_module("threadify").Connection)
        self.assertEqual(handled, ["ok"])
        self.assertEqual(first.acked, 1)
        self.assertEqual(second.acked, 0)

    async def test_managed_context_resolves_joins_and_records_idempotently(self):
        await self._start()
        context = self.extension.create_context(ExtensionContext(extension_name="threadify"))
        token = self.extension._bind_context(context)
        try:
            started = await self.extension.thread(
                "job-1", {"label": "Job 1", "refs": {"request": "redacted"}}
            )
            resumed = await self.extension.thread("job-1")
            result = await self.extension.record_step(
                "thread-1",
                "triaged",
                context={"priority": "high"},
                idempotency_key="delivery-1",
            )
        finally:
            self.extension._reset_context(token)

        self.assertIs(started, self.connection.thread_handle)
        self.assertIs(resumed, started)
        self.assertEqual(self.connection.resolved, [("job-1", {"label": "Job 1", "refs": {"request": "redacted"}}), ("job-1", {})])
        self.assertEqual(result, {"status": "success"})
        self.assertEqual(self.connection.joined, [{"thread_id": "thread-1", "role": ""}])
        name, step = self.connection.thread_handle.steps[0]
        self.assertEqual(name, "triaged")
        self.assertEqual(step.context, {"priority": "high"})
        self.assertEqual(step.key, "delivery-1")

    async def test_stop_cancels_delivery_tasks_and_closes_once(self):
        release = asyncio.Event()

        @self.extension.on("step.success")
        async def blocked(_delivery):
            await release.wait()

        await self._start()
        notification = _Notification()
        self.connection.subscriptions[0][-1](notification)
        await asyncio.sleep(0)
        await self.extension.stop()

        self.assertEqual(notification.acked, 0)
        self.assertEqual(self.connection.closed, 1)


if __name__ == "__main__":
    unittest.main()
