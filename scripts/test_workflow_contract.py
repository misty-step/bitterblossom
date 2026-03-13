from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


def test_workflow_contract_exists_and_is_versioned() -> None:
    workflow = ROOT / "WORKFLOW.md"
    text = workflow.read_text(encoding="utf-8")

    assert text.startswith("---\nversion: 1\n")
    assert "# Bitterblossom Workflow Contract" in text
    assert "shape" in text
    assert "merge" in text


def test_runtime_prompts_reference_workflow_contract() -> None:
    builder = (ROOT / "scripts" / "prompts" / "conductor-builder-template.md").read_text(encoding="utf-8")
    reviewer = (ROOT / "scripts" / "prompts" / "conductor-reviewer-template.md").read_text(encoding="utf-8")
    ralph = (ROOT / "scripts" / "ralph-prompt-template.md").read_text(encoding="utf-8")

    assert "repo `WORKFLOW.md`" in builder
    assert "repo `WORKFLOW.md`" in reviewer
    assert "WORKFLOW.md" in ralph
    assert "unresolved PR review threads as merge blockers" not in builder


def test_repo_guidance_references_workflow_contract() -> None:
    readme = (ROOT / "README.md").read_text(encoding="utf-8")
    agents = (ROOT / "AGENTS.md").read_text(encoding="utf-8")
    repo_claude = (ROOT / "CLAUDE.md").read_text(encoding="utf-8")
    conductor = (ROOT / "docs" / "CONDUCTOR.md").read_text(encoding="utf-8")
    base_claude = (ROOT / "base" / "CLAUDE.md").read_text(encoding="utf-8")
    skills_glance = (ROOT / "base" / "skills" / "glance.md").read_text(encoding="utf-8")
    pr_fix = (ROOT / "base" / "skills" / "pr-fix" / "SKILL.md").read_text(encoding="utf-8")

    assert "WORKFLOW.md" in readme
    assert "WORKFLOW.md" in agents
    assert "WORKFLOW.md" in repo_claude
    assert "WORKFLOW.md" in conductor
    assert "WORKFLOW.md" in base_claude
    assert "WORKFLOW.md" in skills_glance
    assert "WORKFLOW.md" in pr_fix


def test_phase_workers_reference_contract_and_required_skills() -> None:
    bramble = (ROOT / "sprites" / "bramble.md").read_text(encoding="utf-8")
    moss = (ROOT / "sprites" / "moss.md").read_text(encoding="utf-8")
    thorn = (ROOT / "sprites" / "thorn.md").read_text(encoding="utf-8")
    willow = (ROOT / "sprites" / "willow.md").read_text(encoding="utf-8")
    fern = (ROOT / "sprites" / "fern.md").read_text(encoding="utf-8")
    foxglove = (ROOT / "sprites" / "foxglove.md").read_text(encoding="utf-8")

    assert "WORKFLOW.md" in bramble
    assert "WORKFLOW.md" in moss
    assert "WORKFLOW.md" in thorn
    assert "WORKFLOW.md" in willow
    assert "- pr-fix\n" in willow
    assert "WORKFLOW.md" in fern
    assert "- pr\n" in fern
    assert "WORKFLOW.md" in foxglove
    assert "- debug\n" in foxglove
