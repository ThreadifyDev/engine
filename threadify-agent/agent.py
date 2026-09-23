from harnest.agent import Agent
from harnest.lib.ai_gateway import configured_model


root_agent = Agent(
    name="threadify_agent",
    history="session",
    model=configured_model(),
    description=(
        "Analyzes Threadify execution graphs and designs contracts from "
        "authenticated Threadify data."
    ),
)
