import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
from unittest import mock
import unittest


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "architecture_audit",
    ROOT / "scripts" / "architecture-audit.py",
)
AUDIT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(AUDIT)


class ArchitectureAuditTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.repo = self.root / "repo"
        self.repo.mkdir()
        self.output = self.root / "github-output"
        self.summary = self.root / "github-summary"
        self.output.touch()
        self.summary.touch()
        self.env = os.environ.copy()
        self.env.update({
            "GITHUB_OUTPUT": str(self.output),
            "GITHUB_STEP_SUMMARY": str(self.summary),
            "TARGET_REPOSITORY": "owner/tokicv",
            "DEFAULT_BRANCH": "main",
        })
        self.git("init", "-b", "main")
        self.git("config", "user.name", "TokiCV Test")
        self.git("config", "user.email", "test@example.com")

    def tearDown(self):
        self.temp.cleanup()

    def git(self, *args, env=None):
        return subprocess.check_output(
            ["git", *args],
            cwd=self.repo,
            env=env,
            text=True,
            stderr=subprocess.STDOUT,
        ).strip()

    def commit(self, files, message, date=None):
        for path, content in files.items():
            target = self.repo / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content)
        self.git("add", "-A")
        commit_env = os.environ.copy()
        if date:
            commit_env.update({
                "GIT_AUTHOR_DATE": date,
                "GIT_COMMITTER_DATE": date,
            })
        self.git("commit", "-m", message, env=commit_env)
        return self.git("rev-parse", "HEAD")

    def run_audit(self, function, directory):
        previous = Path.cwd()
        os.chdir(self.repo)
        try:
            with mock.patch.object(AUDIT, "git", side_effect=self.git):
                with mock.patch.dict(os.environ, self.env, clear=False):
                    function(directory)
        finally:
            os.chdir(previous)

    def test_collect_skips_when_no_recent_substantive_change(self):
        self.commit(
            {"README.md": "old\n"},
            "docs: establish old baseline",
            date="2020-01-01T00:00:00+00:00",
        )
        directory = self.root / "audit"

        self.run_audit(AUDIT.collect, directory)

        self.assertIn("eligible=false", self.output.read_text())
        self.assertIn("跳过模型", self.summary.read_text())
        self.assertFalse((directory / "prompt.md").exists())

    def test_collect_includes_bounded_commits_and_architecture_rules(self):
        self.commit({
            "GLOSSARY.md": "# Glossary\n\nArchitecture glossary sentinel\n",
            "AGENTS.md": "# Rules\n\nRepository rule sentinel\n",
            "docs/agents/domain.md": "# Domain\n\nDomain rule sentinel\n",
            "docs/adr/0001-one.md": "# ADR\n\nArchitecture rule sentinel\n",
            ".agents/skills/mygo-maintenance/SKILL.md": (
                "# MyGo\n\nMyGo rule sentinel\n"
            ),
            ".agents/skills/improve-codebase-architecture/SKILL.md": (
                ROOT / ".agents/skills/improve-codebase-architecture/SKILL.md"
            ).read_text(),
            ".agents/skills/codebase-design/SKILL.md": (
                ROOT / ".agents/skills/codebase-design/SKILL.md"
            ).read_text(),
            ".agents/skills/domain-modeling/SKILL.md": (
                ROOT / ".agents/skills/domain-modeling/SKILL.md"
            ).read_text(),
            "src/main.txt": "start\n",
        }, "src: establish audit fixture")
        for index in range(30):
            self.commit(
                {f"src/change-{index}.txt": f"{index}\n"},
                f"src: change {index}",
            )
        (self.repo / ".agents/skills/codebase-design/SKILL.md").write_text(
            "# Design\n\n## Glossary\n\nWorktree sentinel.\n"
        )
        directory = self.root / "audit"

        self.run_audit(AUDIT.collect, directory)

        evidence = json.loads((directory / "evidence.json").read_text())
        self.assertIn("eligible=true", self.output.read_text())
        self.assertEqual(len(evidence["commits"]), AUDIT.MAX_COMMITS)
        self.assertTrue(evidence["bounds"]["commits_truncated"])
        self.assertEqual(evidence["adrs"][0]["path"], "docs/adr/0001-one.md")
        self.assertEqual(len(evidence["repository_rules"]), 4)
        self.assertLessEqual(
            sum(len(item["patch"].encode()) for item in evidence["commits"]),
            AUDIT.MAX_TOTAL_PATCH_BYTES,
        )
        prompt = (directory / "prompt.md").read_text()
        self.assertIn("Architecture rule sentinel", prompt)
        self.assertIn("Scope before you scan: YAGNI", prompt)
        self.assertIn("Deep module", prompt)
        self.assertIn("The deletion test", prompt)
        self.assertIn("Challenge against the glossary", prompt)
        self.assertIn(
            "When the user states how something works, check whether the code agrees",
            prompt,
        )
        self.assertNotIn("Worktree sentinel", prompt)
        self.assertNotIn("### 2. Present candidates as an HTML report", prompt)
        self.assertNotIn("### 3. Grilling loop", prompt)
        self.assertNotIn("### Sharpen fuzzy language", prompt)
        self.assertNotIn("### Update GLOSSARY.md inline", prompt)
        self.assertIn("最多提出三条建议", prompt)
        self.assertIn("只读", prompt)
        schema = json.loads((directory / "schema.json").read_text())
        self.assertEqual(
            schema["properties"]["suggestions"]["maxItems"],
            3,
        )
        config = (directory / "codex" / "config.toml").read_text()
        self.assertIn("shell_tool = false", config)
        self.assertIn("multi_agent = false", config)

    def test_collect_truncates_large_patch(self):
        self.commit(
            {"large.txt": "x" * 50_000 + "\n"},
            "src: add a large fixture",
        )
        directory = self.root / "audit"

        self.run_audit(AUDIT.collect, directory)

        evidence = json.loads((directory / "evidence.json").read_text())
        self.assertTrue(evidence["bounds"]["patches_truncated"])
        self.assertTrue(evidence["commits"][0]["patch_truncated"])
        self.assertLessEqual(
            len(evidence["commits"][0]["patch"].encode()),
            AUDIT.MAX_COMMIT_PATCH_BYTES,
        )
        prompt = (directory / "prompt.md").read_text()
        self.assertIn("没有巡检技能原文", prompt)

    def test_validate_accepts_evidence_backed_suggestions(self):
        commit = "a" * 40
        evidence = {
            "schema": 1,
            "default_branch": "main",
            "head": commit,
            "window_start": "2026-11-20T00:00:00+00:00",
            "window_end": "2026-11-27T00:00:00+00:00",
            "commits": [{"commit": commit}],
            "bounds": {"max_total_patch_bytes": 120000},
        }
        proposal = {
            "summary": "发现一个可验证的架构问题",
            "suggestions": [{
                "title": "复用已有导出模块",
                "rule": "ADR 0002 要求复用同一渲染路径",
                "evidence": [{
                    "commit": commit,
                    "observation": "新增了第二条导出路径",
                }],
                "recommendation": "改为调用已有导出模块并补回归测试",
            }],
        }
        directory = self.root / "audit"
        directory.mkdir()
        (directory / "evidence.json").write_text(json.dumps(evidence))
        (directory / "proposal.json").write_text(json.dumps(proposal))

        self.run_audit(AUDIT.validate, directory)

        report = (directory / "report.md").read_text()
        self.assertIn("复用已有导出模块", report)
        self.assertIn("由仓库所有者决定", report)
        self.assertEqual(report, self.summary.read_text())

    def test_validate_rejects_unknown_commit_and_more_than_three_suggestions(self):
        evidence = {"schema": 1, "commits": [{"commit": "a" * 40}]}
        valid = {
            "title": "标题",
            "rule": "规则",
            "evidence": [{"commit": "a" * 40, "observation": "观察"}],
            "recommendation": "建议",
        }
        with self.assertRaises(AUDIT.AuditError):
            AUDIT.check_proposal({
                "summary": "摘要",
                "suggestions": [dict(valid, evidence=[{
                    "commit": "b" * 40,
                    "observation": "未知提交",
                }])],
            }, evidence)
        with self.assertRaises(AUDIT.AuditError):
            AUDIT.check_proposal({
                "summary": "摘要",
                "suggestions": [valid, valid, valid, valid],
            }, evidence)

    def test_workflow_is_weekly_owner_manual_and_read_only(self):
        workflow = (
            ROOT / ".github" / "workflows" / "architecture-audit.yml"
        ).read_text()

        self.assertIn('cron: "17 1 * * 1"', workflow)
        self.assertIn("workflow_dispatch:", workflow)
        self.assertIn("github.repository_owner", workflow)
        self.assertNotIn("contents: write", workflow)
        self.assertNotIn("issues: write", workflow)
        self.assertNotIn("pull-requests: write", workflow)


if __name__ == "__main__":
    unittest.main()
