"""Entry-point agent which turns user requests into Threadify work."""

from harnest.agent import Agent
from harnest.lib.nanite_model import model

root_agent = Agent(
    name="nanite_dispatcher",
    history="session",
    model=model(),
    description="Creates a Threadify work thread and hands it to focused Nanites.",
)
