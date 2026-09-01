"""Focused analyst Nanite and its Threadify event registration."""

from harnest.agent import Agent
from harnest.lib.nanite_model import model
from harnest.lib.nanite_worker import NaniteSpec, register_nanite

register_nanite(
    NaniteSpec(
        trigger_step="work_requested",
        output_step="analysis_completed",
        role="analyst",
    )
)

root_agent = Agent(
    name="analyst_nanite",
    history="session",
    model=model(),
    description="Analyzes one bounded request and identifies risks and missing facts.",
)
