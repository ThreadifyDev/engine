import asyncio
from pathlib import Path


def _implementation(tool):
    while hasattr(tool, "__wrapped__"):
        tool = tool.__wrapped__
    return tool


def test_bundled_contract_resources_exist():
    skills = Path(__file__).resolve().parents[2] / "skills"
    root = skills / "contract-design"
    assert (root / "references/gherkin.md").is_file()
    assert (root / "references/wait-for.md").is_file()
    gherkin = (root / "references/gherkin.md").read_text()
    assert "WAIT_FOR.md" not in gherkin
    assert "payment.feature" not in gherkin
    for name in ("sdk-guidance", "cli-guidance"):
        assert (skills / name / "SKILL.md").is_file()
    for language in ("javascript", "python", "go"):
        assert (skills / "sdk-guidance" / "references" / f"{language}.md").is_file()
    assert (skills / "cli-guidance" / "references/commands.md").is_file()


def test_developer_reference_rejects_unlisted_sources(tools, monkeypatch):
    implementation = _implementation(tools["get_developer_reference"])

    def unexpected(_url):
        raise AssertionError("a URL was fetched for an invalid reference")

    monkeypatch.setitem(implementation.__globals__, "_read_public_guide", unexpected)
    result = asyncio.run(tools["get_developer_reference"]("other", "api"))
    assert result["status"] == "invalid_reference"


def test_developer_reference_returns_source_and_document(tools, monkeypatch):
    implementation = _implementation(tools["get_developer_reference"])
    seen = []

    def read(url):
        seen.append(url)
        return "# Threadify CLI\n"

    monkeypatch.setitem(implementation.__globals__, "_read_public_guide", read)
    result = asyncio.run(tools["get_developer_reference"]("cli", "commands"))
    assert result["status"] == "ok"
    assert result["content"] == "# Threadify CLI\n"
    assert result["source"] == "https://docs.threadify.dev/cli.md"
    assert seen == [result["source"]]
