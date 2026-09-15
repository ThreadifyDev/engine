"""Threadify-backed execution policy for Harnest-managed tools."""

from __future__ import annotations

from collections.abc import Callable, Mapping
from dataclasses import dataclass
import inspect
from typing import Any, TypeVar

from harnest.extensions import Extension


F = TypeVar("F", bound=Callable[..., Any])


@dataclass(frozen=True, slots=True)
class GuardPolicy:
    """Map one tool call to a Threadify contract step and thread argument."""

    tool_name: str
    thread_id_argument: str
    thread_id_position: int
    step_name: str | None = None
    step_name_argument: str | None = None
    step_name_position: int | None = None

    def resolve(self, args: tuple[Any, ...], kwargs: Mapping[str, Any]) -> tuple[str, str]:
        """Resolve the guarded Threadify identifiers from one provider-neutral call."""

        thread_id = _argument(
            self.thread_id_argument,
            self.thread_id_position,
            args,
            kwargs,
        )
        if self.step_name is not None:
            step_name = self.step_name
        else:
            step_name = _argument(
                self.step_name_argument or "step_name",
                self.step_name_position,
                args,
                kwargs,
            )
        return _required_text(thread_id, "thread ID"), _required_text(
            step_name, "step name"
        )


class ThreadifyGuardExtension(Extension):
    """Own the deterministic registry of tools protected by Threadify context."""

    def __init__(self) -> None:
        self._policies: dict[str, GuardPolicy] = {}
        self._observations: dict[str, GuardPolicy] = {}

    def register(
        self,
        function: F,
        *,
        step_name: str | None,
        step_name_argument: str | None,
        thread_id_argument: str,
    ) -> F:
        """Register a function during deterministic agent compilation."""

        if not callable(function):
            raise TypeError("guarded Threadify step must decorate a callable")
        tool_name = _required_text(getattr(function, "__name__", ""), "tool name")
        thread_argument = _required_text(thread_id_argument, "thread ID argument")
        if (step_name is None) == (step_name_argument is None):
            raise ValueError(
                "guarded step must declare exactly one of step_name or "
                "step_name_argument"
            )
        signature = inspect.signature(function)
        positions = {
            name: index for index, name in enumerate(signature.parameters)
        }
        if thread_argument not in positions:
            raise ValueError(
                f"guarded tool {tool_name!r} has no {thread_argument!r} argument"
            )
        dynamic_argument = None
        dynamic_position = None
        if step_name_argument is not None:
            dynamic_argument = _required_text(step_name_argument, "step name argument")
            if dynamic_argument not in positions:
                raise ValueError(
                    f"guarded tool {tool_name!r} has no {dynamic_argument!r} argument"
                )
            dynamic_position = positions[dynamic_argument]
        policy = GuardPolicy(
            tool_name=tool_name,
            thread_id_argument=thread_argument,
            thread_id_position=positions[thread_argument],
            step_name=(
                _required_text(step_name, "step name") if step_name is not None else None
            ),
            step_name_argument=dynamic_argument,
            step_name_position=dynamic_position,
        )
        existing = self._policies.get(tool_name)
        if existing is not None and existing != policy:
            raise ValueError(f"guarded tool {tool_name!r} has conflicting policies")
        self._policies[tool_name] = policy
        self._register_observation(policy)
        return function

    def observe(
        self,
        function: F,
        *,
        step_name: str,
        thread_id_argument: str,
    ) -> F:
        """Map an unguarded prerequisite tool to an acknowledged Threadify step."""

        if not callable(function):
            raise TypeError("observed Threadify step must decorate a callable")
        tool_name = _required_text(getattr(function, "__name__", ""), "tool name")
        thread_argument = _required_text(thread_id_argument, "thread ID argument")
        signature = inspect.signature(function)
        positions = {
            name: index for index, name in enumerate(signature.parameters)
        }
        if thread_argument not in positions:
            raise ValueError(
                f"observed tool {tool_name!r} has no {thread_argument!r} argument"
            )
        policy = GuardPolicy(
            tool_name=tool_name,
            thread_id_argument=thread_argument,
            thread_id_position=positions[thread_argument],
            step_name=_required_text(step_name, "step name"),
        )
        self._register_observation(policy)
        return function

    def _register_observation(self, policy: GuardPolicy) -> None:
        """Reject ambiguous exhaust mappings during deterministic compilation."""

        existing = self._observations.get(policy.tool_name)
        if existing is not None and existing != policy:
            raise ValueError(
                f"observed tool {policy.tool_name!r} has conflicting policies"
            )
        self._observations[policy.tool_name] = policy

    def policy_for(self, tool_name: str) -> GuardPolicy | None:
        """Return the policy for a tool without exposing the mutable registry."""

        return self._policies.get(tool_name)

    def observation_for(self, tool_name: str) -> GuardPolicy | None:
        """Return the explicit exhaust mapping for a process action tool."""

        return self._observations.get(tool_name)


extension = ThreadifyGuardExtension()


def guarded_step(
    step_name: str | None = None,
    *,
    step_name_argument: str | None = None,
    thread_id_argument: str = "thread_id",
) -> Callable[[F], F]:
    """Declare the Threadify execution fact required by an authored tool."""

    def decorate(function: F) -> F:
        return extension.register(
            function,
            step_name=step_name,
            step_name_argument=step_name_argument,
            thread_id_argument=thread_id_argument,
        )

    return decorate


def observed_step(
    step_name: str,
    *,
    thread_id_argument: str = "thread_id",
) -> Callable[[F], F]:
    """Declare an agent-executed prerequisite that must reach Threadify first."""

    def decorate(function: F) -> F:
        return extension.observe(
            function,
            step_name=step_name,
            thread_id_argument=thread_id_argument,
        )

    return decorate


def _argument(
    name: str,
    position: int | None,
    args: tuple[Any, ...],
    kwargs: Mapping[str, Any],
) -> Any:
    """Read a named call argument while preserving positional tool compatibility."""

    if name in kwargs:
        return kwargs[name]
    if position is not None and position < len(args):
        return args[position]
    raise ValueError(f"guarded tool call is missing {name!r}")


def _required_text(value: Any, label: str) -> str:
    """Normalize bounded identifiers before they reach the GraphQL boundary."""

    if not isinstance(value, str) or not value.strip():
        raise ValueError(f"{label} must be a non-empty string")
    normalized = value.strip()
    if len(normalized) > 500:
        raise ValueError(f"{label} exceeds the 500 character limit")
    return normalized


__all__ = [
    "GuardPolicy",
    "ThreadifyGuardExtension",
    "guarded_step",
    "observed_step",
    "extension",
]
