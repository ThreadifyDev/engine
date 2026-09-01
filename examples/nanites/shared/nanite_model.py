"""Shared model construction for the independently compiled Nanites agents."""

from __future__ import annotations

import os
from typing import Any

from harnest.model import LiteLLMModel


def model() -> LiteLLMModel:
    """Create the configured model without requiring a provider-specific base URL."""

    options: dict[str, Any] = {}
    if api_base := os.getenv("LITELLM_API_BASE"):
        options["api_base"] = api_base
    return LiteLLMModel(
        os.getenv("LITELLM_MODEL", "openai/gpt-4.1-mini"),
        **options,
    )


__all__ = ["model"]
