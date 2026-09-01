from __future__ import annotations

import asyncio
import importlib
import os
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import AsyncMock, patch

from harnest.plugins import PluginContext, runtime_plugin_namespaces
from harnest.runtime_plugins import discover_runtime_plugins

_PLUGINS = Path(__file__).resolve().parents[2] / "plugins"


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
        self.started = []
        self.joined = []
        self.thread = _Thread()
        self.closed = 0

    def subscribe(self, *args):
        self.subscriptions.append(args)
        return self

    async def start(self, **kwargs):
        self.started.append(kwargs)
        return self.thread

    async def join(self, **kwargs):
        self.joined.append(kwargs)
        return self.thread

    async def close(self):
        self.closed += 1


class ThreadifyPluginTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self) -> None:
        descriptors = discover_runtime_plugins(_PLUGINS)
        self.namespace = runtime_plugin_namespaces(descriptors)
        activated = self.namespace.__enter__()
        self.module = importlib.import_module("harnest.plugins.threadify")
        self.plugin = activated[0].plugin
        # Harnest 0.9 activates runtime-plugin singletons for the compiled test
        # application. Keep each unit isolated from subscriptions registered by
        # earlier tests while retaining the descriptor-bound plugin identity.
        self._subscriptions = tuple(self.plugin._subscriptions)
        self.plugin._subscriptions.clear()
        self.connection = _Connection()

    async def asyncTearDown(self) -> None:
        if self.plugin._started:
            await self.plugin.stop()
        self.plugin._subscriptions[:] = self._subscriptions
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
            await self.plugin.start(SimpleNamespace(root_agent_name="root"))
        self.assertEqual(connect.await_args.args, ("private-key",))
        self.assertEqual(connect.await_args.kwargs["service_name"], "nanite")

    async def test_separates_owner_scoped_events_from_agent_actions(self):
        action_connection = _Connection()
        event_connection = _Connection()

        @self.plugin.on("step.success", "work_requested")
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
            await self.plugin.start(SimpleNamespace(root_agent_name="root"))

        self.assertEqual(connect.await_count, 2)
        self.assertEqual(action_connection.subscriptions, [])
        self.assertEqual(len(event_connection.subscriptions), 1)

        context = self.plugin.create_context(PluginContext("threadify"))
        token = self.plugin._bind_context(context)
        try:
            await self.plugin.record_step("thread-1", "analysis_completed")
        finally:
            self.plugin._reset_context(token)

        self.assertEqual(
            action_connection.joined,
            [{"thread_id": "thread-1", "role": ""}],
        )
        self.assertEqual(event_connection.joined, [])

        await self.plugin.stop()
        self.assertEqual(action_connection.closed, 1)
        self.assertEqual(event_connection.closed, 1)

    async def test_exposes_sdk_and_acks_only_successful_async_deliveries(self):
        handled = []

        @self.plugin.on("step.success", "work_requested")
        async def success(delivery):
            handled.append(delivery.notification.notification_id)

        @self.plugin.on("step.success", "work_requested")
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

    async def test_managed_context_starts_joins_and_records_idempotently(self):
        await self._start()
        context = self.plugin.create_context(PluginContext("threadify"))
        token = self.plugin._bind_context(context)
        try:
            started = await self.plugin.start_thread(
                label="job-1", refs={"request": "redacted"}
            )
            result = await self.plugin.record_step(
                "thread-1",
                "triaged",
                context={"priority": "high"},
                idempotency_key="delivery-1",
            )
        finally:
            self.plugin._reset_context(token)

        self.assertIs(started, self.connection.thread)
        self.assertEqual(result, {"status": "success"})
        self.assertEqual(self.connection.joined, [{"thread_id": "thread-1", "role": ""}])
        name, step = self.connection.thread.steps[0]
        self.assertEqual(name, "triaged")
        self.assertEqual(step.context, {"priority": "high"})
        self.assertEqual(step.key, "delivery-1")

    async def test_stop_cancels_delivery_tasks_and_closes_once(self):
        release = asyncio.Event()

        @self.plugin.on("step.success")
        async def blocked(_delivery):
            await release.wait()

        await self._start()
        notification = _Notification()
        self.connection.subscriptions[0][-1](notification)
        await asyncio.sleep(0)
        await self.plugin.stop()

        self.assertEqual(notification.acked, 0)
        self.assertEqual(self.connection.closed, 1)


if __name__ == "__main__":
    unittest.main()
