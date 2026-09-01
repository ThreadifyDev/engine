"""Managed Threadify SDK access and event delivery for Harnest agents."""

from __future__ import annotations

import asyncio
import inspect
import os
from collections.abc import Callable, Mapping
from dataclasses import dataclass, field
from typing import Any, Literal, TypeVar

from harnest.logging import get_logger
from harnest.plugins import Plugin, PluginContext, plugin_mutation
from threadify import (
    ArchivedStep,
    ArchivedThread,
    Connection,
    DataRetriever,
    DuplicateStepError,
    Notification,
    RefQuery,
    StepResult,
    Threadify,
    ThreadifyFactory,
    ThreadifySpanExporter,
    ThreadInstance,
    ThreadStep,
    is_duplicate_error,
)

Handler = Callable[["ThreadifyDelivery"], Any]
HandlerT = TypeVar("HandlerT", bound=Handler)
StepStatus = Literal["success", "failed", "error"]
_LOGGER = get_logger("threadify.plugin")


@dataclass(frozen=True, slots=True)
class ThreadifySettings:
    """Validated application connection settings with a redacted API key."""

    api_key: str = field(repr=False)
    service_name: str
    subscription_api_key: str | None = field(default=None, repr=False)
    ws_url: str | None = None
    graphql_url: str | None = None
    max_in_flight: int = 10
    connect_timeout: float = 10.0
    debug: bool = False

    @classmethod
    def from_environment(cls, default_service_name: str) -> ThreadifySettings:
        """Read process-owned credentials only when the plugin starts."""

        api_key = _required_text(os.getenv("THREADIFY_API_KEY"), "THREADIFY_API_KEY")
        service_name = _optional_text(
            os.getenv("THREADIFY_SERVICE_NAME")
        ) or _required_text(default_service_name, "root agent name")
        return cls(
            api_key=api_key,
            service_name=service_name,
            subscription_api_key=_optional_text(
                os.getenv("THREADIFY_SUBSCRIPTION_API_KEY")
            ),
            ws_url=_optional_text(os.getenv("THREADIFY_WS_URL")),
            graphql_url=_optional_text(os.getenv("THREADIFY_GRAPHQL_URL")),
            max_in_flight=_positive_int("THREADIFY_MAX_IN_FLIGHT", 10),
            connect_timeout=_positive_float("THREADIFY_CONNECT_TIMEOUT", 10.0),
            debug=_boolean("THREADIFY_DEBUG", False),
        )


@dataclass(frozen=True, slots=True)
class _Subscription:
    """One deterministic mapping from a Threadify event to authored code."""

    event: str
    step_name: str
    handler: Handler


class ThreadifyDelivery:
    """One event callback with managed and native SDK access."""

    __slots__ = ("_notification", "_owner")

    def __init__(
        self, owner: ThreadifyPlugin, notification: Notification
    ) -> None:
        """Bind a notification to the connection which received it."""

        self._owner = owner
        self._notification = notification

    @property
    def notification(self) -> Notification:
        """Return the original SDK notification without copying its payload."""

        return self._notification

    @property
    def connection(self) -> Connection:
        """Expose the official SDK connection for provider-specific operations."""

        return self._owner._require_connection()

    async def join(self, *, role: str = "") -> ThreadInstance:
        """Join the notification thread through the audited managed boundary."""

        return await self._owner._join_thread(
            self._notification.thread_id,
            role=role,
        )

    async def record_step(
        self,
        step_name: str,
        *,
        status: StepStatus = "success",
        context: Mapping[str, Any] | None = None,
        message: str | None = None,
        role: str = "",
        idempotency_key: str | None = None,
    ) -> StepResult:
        """Record the next step with delivery-derived idempotency by default."""

        key = idempotency_key or self._notification.notification_id
        return await self._owner._record_step(
            self._notification.thread_id,
            step_name,
            status=status,
            context=context,
            message=message,
            role=role,
            idempotency_key=key or None,
        )

    async def complete(self, reason: str | Mapping[str, Any] = "") -> Any:
        """Complete the current Threadify thread after downstream work finishes."""

        return await self._owner._complete_thread(
            self._notification.thread_id,
            reason=reason,
        )


class ThreadifyContext(PluginContext):
    """Typed invocation view exposed through ``context.plugins('threadify')``."""

    __slots__ = ("_owner",)

    def __init__(self, plugin_name: str, owner: ThreadifyPlugin) -> None:
        """Bind a revocable invocation view to the application singleton."""

        super().__init__(plugin_name)
        self._owner = owner

    @property
    def connection(self) -> Connection:
        """Return the official SDK connection during this invocation only."""

        self._require_active()
        return self._owner._require_connection()

    async def start_thread(
        self,
        *,
        label: str = "",
        contract_name: str = "",
        refs: Mapping[str, Any] | None = None,
        tags: list[str] | None = None,
        role: str = "",
    ) -> ThreadInstance:
        """Start a Threadify thread through the audited managed boundary."""

        self._require_active()
        return await self._owner._start_thread(
            label=label,
            contract_name=contract_name,
            refs=refs,
            tags=tags,
            role=role,
        )

    async def join_thread(
        self, thread_id: str, *, role: str = ""
    ) -> ThreadInstance:
        """Join a Threadify thread through the audited managed boundary."""

        self._require_active()
        return await self._owner._join_thread(thread_id, role=role)

    async def record_step(
        self,
        thread_id: str,
        step_name: str,
        *,
        status: StepStatus = "success",
        context: Mapping[str, Any] | None = None,
        message: str | None = None,
        role: str = "",
        idempotency_key: str | None = None,
    ) -> StepResult:
        """Record one audited step while retaining SDK-native return types."""

        self._require_active()
        return await self._owner._record_step(
            thread_id,
            step_name,
            status=status,
            context=context,
            message=message,
            role=role,
            idempotency_key=idempotency_key,
        )

    async def complete_thread(
        self, thread_id: str, *, reason: str | Mapping[str, Any] = ""
    ) -> Any:
        """Complete a Threadify thread through the managed connection."""

        self._require_active()
        return await self._owner._complete_thread(thread_id, reason=reason)


class ThreadifyPlugin(Plugin[ThreadifyContext]):
    """Own one Threadify connection and async delivery tasks per agent process."""

    def __init__(self) -> None:
        """Retain declarations without connecting during compiler imports."""

        self._connection: Connection | None = None
        self._subscription_connection: Connection | None = None
        self._subscriptions: list[_Subscription] = []
        self._deliveries: set[asyncio.Task[None]] = set()
        self._started = False

    def on(
        self, event: str, step_name: str = ""
    ) -> Callable[[HandlerT], HandlerT]:
        """Register a Threadify subscription before or during application startup."""

        event_name = _required_text(event, "Threadify event")
        selected_step = _optional_text(step_name) or ""

        def decorate(handler: HandlerT) -> HandlerT:
            """Retain authored order so callback activation is deterministic."""

            if not callable(handler):
                raise TypeError("Threadify event handler must be callable")
            subscription = _Subscription(event_name, selected_step, handler)
            if subscription in self._subscriptions:
                raise ValueError("Threadify event handler is already registered")
            self._subscriptions.append(subscription)
            if self._started:
                self._activate_subscription(subscription)
            return handler

        return decorate

    async def start(self, start_context: Any) -> None:
        """Connect once, then activate every authored subscription in order."""

        settings = ThreadifySettings.from_environment(start_context.root_agent_name)
        connection = await Threadify.connect(
            settings.api_key,
            service_name=settings.service_name,
            ws_url=settings.ws_url,
            graphql_url=settings.graphql_url,
            debug=settings.debug,
            max_in_flight=settings.max_in_flight,
            connect_timeout=settings.connect_timeout,
        )
        self._connection = connection
        subscription_connection = connection
        if (
            settings.subscription_api_key
            and settings.subscription_api_key != settings.api_key
        ):
            subscription_connection = await Threadify.connect(
                settings.subscription_api_key,
                service_name=f"{settings.service_name}-events",
                ws_url=settings.ws_url,
                graphql_url=settings.graphql_url,
                debug=settings.debug,
                max_in_flight=settings.max_in_flight,
                connect_timeout=settings.connect_timeout,
            )
        self._subscription_connection = subscription_connection
        self._started = True
        try:
            for subscription in self._subscriptions:
                self._activate_subscription(subscription)
        except BaseException:
            self._started = False
            self._connection = None
            self._subscription_connection = None
            if subscription_connection is not connection:
                await subscription_connection.close()
            await connection.close()
            raise

    async def stop(self) -> None:
        """Stop local deliveries before closing their shared SDK connection."""

        self._started = False
        deliveries = tuple(self._deliveries)
        for delivery in deliveries:
            delivery.cancel()
        if deliveries:
            await asyncio.gather(*deliveries, return_exceptions=True)
        self._deliveries.clear()
        connection, self._connection = self._connection, None
        subscription_connection, self._subscription_connection = (
            self._subscription_connection,
            None,
        )
        if (
            subscription_connection is not None
            and subscription_connection is not connection
        ):
            await subscription_connection.close()
        if connection is not None:
            await connection.close()

    def create_context(self, base: PluginContext) -> ThreadifyContext:
        """Create a fresh revocable SDK view only after connection startup."""

        self._require_connection()
        return ThreadifyContext(base.plugin_name, self)

    @property
    def connection(self) -> Connection:
        """Return the SDK connection through the current invocation context."""

        return self.context.connection

    async def start_thread(self, **kwargs: Any) -> ThreadInstance:
        """Start a thread through the current invocation context."""

        return await self.context.start_thread(**kwargs)

    async def join_thread(
        self, thread_id: str, *, role: str = ""
    ) -> ThreadInstance:
        """Join a thread through the current invocation context."""

        return await self.context.join_thread(thread_id, role=role)

    async def record_step(
        self, thread_id: str, step_name: str, **kwargs: Any
    ) -> StepResult:
        """Record a step through the current invocation context."""

        return await self.context.record_step(thread_id, step_name, **kwargs)

    async def complete_thread(
        self, thread_id: str, *, reason: str | Mapping[str, Any] = ""
    ) -> Any:
        """Complete a thread through the current invocation context."""

        return await self.context.complete_thread(thread_id, reason=reason)

    def _activate_subscription(self, subscription: _Subscription) -> None:
        """Bridge the synchronous SDK callback into one tracked async task."""

        connection = self._require_subscription_connection()

        def receive(notification: Notification) -> None:
            self._schedule_delivery(subscription, notification)

        if subscription.step_name:
            connection.subscribe(
                subscription.event,
                subscription.step_name,
                receive,
            )
        else:
            connection.subscribe(subscription.event, receive)

    def _schedule_delivery(
        self, subscription: _Subscription, notification: Notification
    ) -> None:
        """Give each notification one task whose lifetime the plugin owns."""

        if not self._started:
            return
        task = asyncio.create_task(
            self._deliver(subscription, notification),
            name=f"harnest-threadify-{notification.notification_id or 'event'}",
        )
        self._deliveries.add(task)
        task.add_done_callback(self._deliveries.discard)

    async def _deliver(
        self, subscription: _Subscription, notification: Notification
    ) -> None:
        """Acknowledge only callbacks which reached successful completion."""

        try:
            result = subscription.handler(ThreadifyDelivery(self, notification))
            if inspect.isawaitable(result):
                await result
        except asyncio.CancelledError:
            raise
        except Exception as error:  # noqa: BLE001
            # Provider payloads and authored exception text may contain private
            # work context, so logs retain only stable routing and error type.
            _LOGGER.error(
                "threadify.delivery.failed",
                subscription_event=subscription.event,
                step=subscription.step_name or "global",
                error_type=type(error).__name__,
            )
            return
        _ack(notification, subscription)

    async def _start_thread(
        self,
        *,
        label: str,
        contract_name: str,
        refs: Mapping[str, Any] | None,
        tags: list[str] | None,
        role: str,
    ) -> ThreadInstance:
        """Start one thread and emit a privacy-safe durable mutation signal."""

        connection = self._require_connection()
        async with plugin_mutation("threadify", "thread.start", trigger="agent"):
            return await connection.start(
                label=label,
                contract_name=contract_name,
                refs=dict(refs or {}),
                tags=tags,
                role=role,
            )

    async def _join_thread(
        self, thread_id: str, *, role: str = ""
    ) -> ThreadInstance:
        """Join one thread and audit the service participation mutation."""

        selected_id = _required_text(thread_id, "Threadify thread ID")
        connection = self._require_connection()
        async with plugin_mutation("threadify", "thread.join", trigger="agent"):
            return await connection.join(thread_id=selected_id, role=role)

    async def _record_step(
        self,
        thread_id: str,
        step_name: str,
        *,
        status: StepStatus,
        context: Mapping[str, Any] | None,
        message: str | None,
        role: str,
        idempotency_key: str | None,
    ) -> StepResult:
        """Record one idempotent step after joining through the managed client."""

        selected_step = _required_text(step_name, "Threadify step name")
        selected_status = _step_status(status)
        thread = await self._join_thread(thread_id, role=role)
        step = thread.step(selected_step)
        if context:
            step.add_context(dict(context))
        if idempotency_key:
            step.idempotency_key(idempotency_key)
        operation = f"step.{selected_status}"
        async with plugin_mutation("threadify", operation, trigger="agent"):
            return await _stop_step(step, selected_status, message)

    async def _complete_thread(
        self, thread_id: str, *, reason: str | Mapping[str, Any]
    ) -> Any:
        """Complete one joined thread and audit only the committed mutation."""

        thread = await self._join_thread(thread_id)
        payload = dict(reason) if isinstance(reason, Mapping) else reason
        async with plugin_mutation("threadify", "thread.complete", trigger="agent"):
            return await thread.complete(payload)

    def _require_connection(self) -> Connection:
        """Fail closed when code escapes the managed application lifetime."""

        if not self._started or self._connection is None:
            raise RuntimeError("Threadify plugin is not started")
        return self._connection

    def _require_subscription_connection(self) -> Connection:
        """Return the optional owner-scoped event connection."""

        if not self._started or self._subscription_connection is None:
            raise RuntimeError("Threadify plugin is not started")
        return self._subscription_connection


async def _stop_step(
    step: ThreadStep, status: StepStatus, message: str | None
) -> StepResult:
    """Map the bounded public status set to the SDK's explicit methods."""

    if status == "success":
        return await step.success(message)
    if status == "failed":
        return await step.failed(message)
    return await step.error(message)


def _ack(notification: Notification, subscription: _Subscription) -> None:
    """Treat token-less batch events as delivered without hiding real ACK errors."""

    try:
        notification.ack()
    except RuntimeError:
        _LOGGER.debug(
            "threadify.delivery.ack_unavailable",
            subscription_event=subscription.event,
            step=subscription.step_name or "global",
        )


def _step_status(value: Any) -> StepStatus:
    """Reject invalid statuses before joining or changing a remote thread."""

    if value not in {"success", "failed", "error"}:
        raise ValueError("Threadify step status must be success, failed, or error")
    return value


def _required_text(value: Any, label: str) -> str:
    """Normalize required configuration without returning secret-bearing errors."""

    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{label} must be a non-empty string")
    return value.strip()


def _optional_text(value: Any) -> str | None:
    """Normalize optional environment text while treating whitespace as absent."""

    if value is None:
        return None
    if not isinstance(value, str):
        raise TypeError("optional Threadify setting must be a string")
    normalized = value.strip()
    return normalized or None


def _positive_int(name: str, default: int) -> int:
    """Parse one positive bounded integer from the process environment."""

    value = _optional_text(os.getenv(name))
    if value is None:
        return default
    try:
        parsed = int(value)
    except ValueError as error:
        raise ValueError(f"{name} must be a positive integer") from error
    if parsed <= 0 or parsed > 10_000:
        raise ValueError(f"{name} must be between 1 and 10000")
    return parsed


def _positive_float(name: str, default: float) -> float:
    """Parse one positive finite timeout from the process environment."""

    value = _optional_text(os.getenv(name))
    if value is None:
        return default
    try:
        parsed = float(value)
    except ValueError as error:
        raise ValueError(f"{name} must be a positive number") from error
    if not 0 < parsed <= 300:
        raise ValueError(f"{name} must be greater than 0 and at most 300")
    return parsed


def _boolean(name: str, default: bool) -> bool:
    """Accept conventional environment booleans and reject ambiguous values."""

    value = _optional_text(os.getenv(name))
    if value is None:
        return default
    normalized = value.lower()
    if normalized in {"1", "true", "yes", "on"}:
        return True
    if normalized in {"0", "false", "no", "off"}:
        return False
    raise ValueError(f"{name} must be a boolean")


plugin = ThreadifyPlugin()
threadify = plugin


__all__ = [
    "ArchivedStep",
    "ArchivedThread",
    "Connection",
    "DataRetriever",
    "DuplicateStepError",
    "Notification",
    "RefQuery",
    "StepResult",
    "ThreadInstance",
    "ThreadStep",
    "Threadify",
    "ThreadifyContext",
    "ThreadifyDelivery",
    "ThreadifyFactory",
    "ThreadifyPlugin",
    "ThreadifySettings",
    "ThreadifySpanExporter",
    "is_duplicate_error",
    "plugin",
    "threadify",
]
