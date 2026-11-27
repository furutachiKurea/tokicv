import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "github_delivery_retro",
    ROOT / "scripts" / "github-delivery-retro.py",
)
RETRO = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RETRO)

REPOSITORY = "owner/tokicv"
BRANCH = "main"
RETRO_SKILL_SOURCE = (
    ROOT / ".agents/skills/retro/SKILL.md"
).read_text()


def make_run(
    identifier,
    *,
    issue=7,
    stage="implement",
    step="run tests",
    branch=BRANCH,
    repository=REPOSITORY,
    title=None,
    conclusion="failure",
):
    return (
        identifier,
        {
            "id": identifier,
            "display_title": title or f"TokiCV issue #{issue}",
            "name": "TokiCV delivery",
            "path": ".github/workflows/delivery.yml@refs/heads/factory/tokicv-delivery",
            "repository": {"full_name": repository},
            "status": "completed",
            "conclusion": conclusion,
            "head_branch": branch,
            "html_url": f"https://github.com/{repository}/actions/runs/{identifier}",
        },
        {
            "id": identifier * 10,
            "name": stage,
            "conclusion": conclusion,
            "steps": [
                {"name": "checkout", "conclusion": "success"},
                {"name": step, "conclusion": conclusion},
            ],
        },
    )


class FakeAPI:
    def __init__(self, *items, pulls=None):
        self.runs = {run["id"]: run for _, run, _ in items}
        self.jobs = {run["id"]: job for _, run, job in items}
        self.recent = [run for _, run, _ in items]
        self.pulls = pulls or {}

    def get_run(self, identifier):
        return self.runs[identifier]

    def recent_runs(self):
        return self.recent

    def get_jobs(self, identifier):
        return [self.jobs[identifier]]

    def get_pull(self, identifier):
        if identifier not in self.pulls:
            raise RETRO.RetroError("pull request not found")
        return self.pulls[identifier]


class DeliveryRetroTests(unittest.TestCase):
    def test_auto_accepts_second_same_ticket_and_failed_step(self):
        api = FakeAPI(make_run(1), make_run(2))

        evidence, reason = RETRO.auto_evidence(api, REPOSITORY, BRANCH, 2)

        self.assertIsNone(reason)
        self.assertEqual(evidence["issue"], 7)
        self.assertEqual([run["step"] for run in evidence["runs"]], ["run tests"] * 2)
        self.assertEqual(
            set(evidence["runs"][0]),
            {"id", "issue", "url", "conclusion", "job", "step"},
        )
        self.assertTrue(evidence["runs"][1]["url"].endswith("/runs/2"))

    def test_auto_requires_second_matching_failure(self):
        cases = [
            (FakeAPI(make_run(1), make_run(2, step="race")), 2),
            (FakeAPI(make_run(1), make_run(2, branch="feature")), 2),
            (FakeAPI(make_run(1), make_run(2, repository="other/repo")), 2),
            (FakeAPI(make_run(1), make_run(2, title="TokiCV smoke")), 2),
            (FakeAPI(make_run(1), make_run(2), make_run(3)), 3),
            (FakeAPI(make_run(1)), 1),
        ]
        for api, current in cases:
            with self.subTest(current=current, recent=len(api.recent)):
                evidence, reason = RETRO.auto_evidence(
                    api, REPOSITORY, BRANCH, current
                )
                self.assertIsNone(evidence)
                self.assertTrue(reason)

    def test_recent_runs_are_limited_to_thirty(self):
        api = RETRO.GitHubAPI(REPOSITORY)
        response = {"workflow_runs": [{"id": value} for value in range(31)]}
        with mock.patch.object(api, "get", return_value=response) as request:
            self.assertEqual(len(api.recent_runs()), 30)
        self.assertIn("per_page=30", request.call_args.args[0])

    def test_auto_groups_issue_and_pr_titles_by_canonical_issue(self):
        pulls = {
            9: {
                "base": {"ref": BRANCH},
                "head": {
                    "ref": "factory/issue-7",
                    "repo": {"full_name": REPOSITORY},
                },
            }
        }
        api = FakeAPI(
            make_run(1),
            make_run(2, title="TokiCV PR #9"),
            pulls=pulls,
        )

        evidence, reason = RETRO.auto_evidence(api, REPOSITORY, BRANCH, 2)

        self.assertIsNone(reason)
        self.assertEqual(evidence["issue"], 7)
        self.assertEqual([run["id"] for run in evidence["runs"]], [1, 2])

    def test_canonical_issue_rejects_nonfactory_pr(self):
        api = FakeAPI(
            make_run(1),
            make_run(2, title="TokiCV PR #9"),
            pulls={
                9: {
                    "base": {"ref": BRANCH},
                    "head": {
                        "ref": "feature/manual",
                        "repo": {"full_name": REPOSITORY},
                    },
                }
            },
        )

        evidence, reason = RETRO.auto_evidence(api, REPOSITORY, BRANCH, 2)

        self.assertIsNone(evidence)
        self.assertTrue(reason)

    def test_manual_accepts_two_default_branch_runs(self):
        api = FakeAPI(
            make_run(1, issue=7, stage="implement"),
            make_run(2, issue=8, stage="review"),
        )

        evidence = RETRO.manual_evidence(api, REPOSITORY, BRANCH, [2, 1])

        self.assertEqual(evidence["mode"], "manual")
        self.assertEqual([run["id"] for run in evidence["runs"]], [1, 2])

    def test_manual_rejects_duplicate_or_wrong_branch(self):
        cases = [
            (FakeAPI(make_run(1)), [1, 1]),
            (FakeAPI(make_run(1), make_run(2, branch="feature")), [1, 2]),
        ]
        for api, identifiers in cases:
            with self.subTest(recent=len(api.recent)):
                with self.assertRaises(RETRO.RetroError):
                    RETRO.manual_evidence(api, REPOSITORY, BRANCH, identifiers)

    def test_insufficient_is_valid_without_improvement(self):
        evidence, _ = RETRO.auto_evidence(
            FakeAPI(make_run(1), make_run(2)), REPOSITORY, BRANCH, 2
        )
        proposal = {
            "status": "insufficient",
            "problem": "两次运行只有 job/step 相同，缺少共同的错误细节",
            "evidence": [
                {"run_id": 1, "observation": "第一次运行没有更多结构化证据"},
                {"run_id": 2, "observation": "第二次运行没有更多结构化证据"},
            ],
            "improvement": {"summary": "", "files": [], "acceptance": []},
        }

        RETRO.check_proposal(proposal, evidence)
        report = RETRO.render_report(evidence, proposal)

        self.assertIn("未形成提案", report)
        self.assertNotIn("验收条件", report)

    def test_proposal_needs_evidence_for_both_runs(self):
        evidence, _ = RETRO.auto_evidence(
            FakeAPI(make_run(1), make_run(2)), REPOSITORY, BRANCH, 2
        )
        proposal = {
            "status": "proposal",
            "problem": "相同失败",
            "evidence": [{"run_id": 1, "observation": "只有一次证据"}],
            "improvement": {
                "summary": "增加检查",
                "files": ["scripts/example.py"],
                "acceptance": ["两次失败都能识别"],
            },
        }

        with self.assertRaises(RETRO.RetroError):
            RETRO.check_proposal(proposal, evidence)

    def test_collect_and_validate_commands(self):
        api = FakeAPI(make_run(1), make_run(2))
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary) / "retro"
            output = Path(temporary) / "output"
            summary = Path(temporary) / "summary"
            environment = {
                "TARGET_REPOSITORY": REPOSITORY,
                "EVENT_NAME": "workflow_run",
                "DEFAULT_BRANCH": BRANCH,
                "CURRENT_RUN_ID": "2",
                "GITHUB_OUTPUT": str(output),
                "GITHUB_STEP_SUMMARY": str(summary),
            }
            proposal = {
                "status": "proposal",
                "problem": "相同失败",
                "evidence": [
                    {"run_id": 1, "observation": "第一次运行失败"},
                    {"run_id": 2, "observation": "第二次运行失败"},
                ],
                "improvement": {
                    "summary": "增加检查",
                    "files": ["scripts/example.py"],
                    "acceptance": ["两次失败都能生成报告"],
                },
            }
            with mock.patch.object(RETRO, "GitHubAPI", return_value=api):
                with mock.patch.object(
                    RETRO, "document", return_value=RETRO_SKILL_SOURCE
                ):
                    with mock.patch.dict(os.environ, environment, clear=False):
                        RETRO.collect(directory)
                        (directory / "proposal.json").write_text(
                            json.dumps(proposal, ensure_ascii=False)
                        )
                        RETRO.validate(directory)

            evidence = json.loads((directory / "evidence.json").read_text())
            self.assertNotIn("log", evidence["runs"][0])
            self.assertIn("eligible=true", output.read_text())
            self.assertIn("验收条件", summary.read_text())
            prompt = (directory / "prompt.md").read_text()
            self.assertIn("证据", prompt)
            self.assertIn("Navigation", prompt)
            self.assertIn("implementation agent has the most **context pressure**", prompt)
            self.assertIn("`AGENTS.md`: these files are pushed", prompt)
            self.assertNotIn("Call the Skill tool", prompt)
            self.assertNotIn("Present these candidates", prompt)
            self.assertEqual(
                (directory / "report.md").read_text(), summary.read_text()
            )
            config = (directory / "codex" / "config.toml").read_text()
            self.assertIn("shell_tool = false", config)
            self.assertIn("multi_agent = false", config)

    def test_retro_guidance_degrades_without_trusted_skill(self):
        with mock.patch.object(RETRO, "document", return_value=""):
            guidance = RETRO.retro_guidance()

        self.assertIn("没有 retro 技能原文", guidance)


if __name__ == "__main__":
    unittest.main()
