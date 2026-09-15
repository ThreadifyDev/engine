from harnest.agent import Agent
from harnest.model import OllamaModel


root_agent = Agent(
    name="threadify_analyst_agent",
    history="session",
    model=OllamaModel.from_environment(),
)
