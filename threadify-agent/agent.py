import os

from harnest.agent import Agent
from harnest.model import LiteLLMModel


root_agent = Agent(
    name="threadify_agent",
    history="session",
    model=LiteLLMModel(
        model=os.getenv("LITELLM_MODEL", "openai/gpt-4o-mini"),
    ),
    description=(
        "Analyzes Threadify execution graphs and designs contracts from "
        "authenticated Threadify data."
    ),
)
