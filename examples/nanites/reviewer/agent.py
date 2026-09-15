"""Focused reviewer Nanite and its Threadify event registration."""

from harnest.agent import Agent
from harnest.lib.nanite_model import model
from harnest.lib.nanite_worker import NaniteSpec, register_nanite

register_nanite(
    NaniteSpec(
        trigger_step="analysis_completed",
        output_step="review_completed",
        role="reviewer",
    )
)

root_agent = Agent(
    name="reviewer_nanite",
    history="session",
    model=model(),
    description="Reviews a bounded analysis and returns an actionable final plan.",
)
