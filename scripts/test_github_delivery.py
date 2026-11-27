import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "scripts" / "github-delivery.py"
BOT = "github-actions[bot]"
OWNER = "owner"
ISSUE = 7
RUN_ID = "500"
ARTIFACT = "candidate-500-1"
RESTART = "/tokicv 从当前基线重新实现"

FAKE_GH = r'''#!/usr/bin/env python3
import json, os, shutil, sys
from pathlib import Path

args = sys.argv[1:]
scenario = json.loads(Path(os.environ["GH_SCENARIO"]).read_text())
with Path(os.environ["GH_LOG"]).open("a") as log:
    print(json.dumps(args, ensure_ascii=False), file=log)

if args[:2] == ["auth", "git-credential"]:
    sys.stdin.read()
elif args[:2] == ["issue", "view"]:
    print(json.dumps(scenario["issue"], ensure_ascii=False))
elif args[:2] == ["issue", "comment"]:
    body = Path(args[args.index("--body-file") + 1]).read_text()
    with Path(os.environ["GH_POSTS"]).open("a") as posts:
        print(json.dumps({"body": body}, ensure_ascii=False), file=posts)
elif args[:2] == ["pr", "list"]:
    print(json.dumps(scenario.get("pulls", []), ensure_ascii=False))
elif args[:2] == ["pr", "create"]:
    print(scenario.get("pr_url", "https://github.com/owner/repo/pull/9"))
elif args[:2] == ["run", "download"]:
    name = args[args.index("--name") + 1]
    destination = Path(args[args.index("--dir") + 1])
    source = scenario.get("artifacts", {}).get(name)
    if source is None:
        print("artifact not found", file=sys.stderr)
        sys.exit(1)
    destination.mkdir(parents=True, exist_ok=True)
    for path in Path(source).iterdir():
        shutil.copy2(path, destination / path.name)
else:
    sys.exit(2)
'''


def state_block(state):
    return (
        "<!-- tokicv-delivery:state\n"
        + json.dumps(state, ensure_ascii=False, separators=(",", ":"))
        + "\n-->"
    )


class DeliveryTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        self.repo = self.root / "repo"
        self.remote = self.root / "remote.git"
        self.work = self.root / "work"
        self.bin = self.root / "bin"
        self.scenario_path = self.root / "scenario.json"
        self.log_path = self.root / "gh.log"
        self.posts_path = self.root / "posts.jsonl"
        self.output_path = self.root / "github-output"
        self.repo.mkdir()
        self.work.mkdir()
        self.bin.mkdir()
        self.output_path.touch()

        self.git("init", "-b", "main")
        self.git("config", "user.name", "TokiCV Test")
        self.git("config", "user.email", "test@example.com")
        (self.repo / "README.md").write_text("base\n")
        self.git("add", "README.md")
        self.git("commit", "-m", "docs: establish test repository")
        self.base = self.git("rev-parse", "HEAD")
        self.git("init", "--bare", str(self.remote))
        self.git("remote", "add", "origin", str(self.remote))
        self.git("push", "-u", "origin", "main")

        gh = self.bin / "gh"
        gh.write_text(FAKE_GH)
        gh.chmod(0o755)
        self.env = os.environ.copy()
        self.env.update({
            "PATH": f"{self.bin}{os.pathsep}{self.env['PATH']}",
            "GITHUB_OUTPUT": str(self.output_path),
            "GITHUB_REPOSITORY": "owner/repo",
            "GITHUB_REPOSITORY_OWNER": OWNER,
            "GITHUB_SERVER_URL": "https://github.com",
            "GITHUB_RUN_ID": RUN_ID,
            "GITHUB_RUN_ATTEMPT": "1",
            "ISSUE": str(ISSUE),
            "BASE": self.base,
            "DEFAULT_BRANCH": "main",
            "CANDIDATE_ARTIFACT_NAME": ARTIFACT,
            "GH_SCENARIO": str(self.scenario_path),
            "GH_LOG": str(self.log_path),
            "GH_POSTS": str(self.posts_path),
        })
        self.set_scenario()

    def tearDown(self):
        self.temp.cleanup()

    def git(self, *args):
        return subprocess.check_output(
            ["git", *args], cwd=self.repo, text=True, stderr=subprocess.STDOUT
        ).strip()

    def set_scenario(self, comments=None, pulls=None, artifacts=None):
        scenario = {
            "issue": {
                "title": "测试交付",
                "body": "实现一个可验证的测试功能",
                "state": "OPEN",
                "comments": comments or [],
            },
            "pulls": pulls or [],
            "artifacts": artifacts or {},
        }
        self.scenario_path.write_text(json.dumps(scenario, ensure_ascii=False))
        return scenario

    def scenario(self):
        return json.loads(self.scenario_path.read_text())

    def command(self, name, extra_env=None):
        self.output_path.write_text("")
        env = self.env.copy()
        if extra_env:
            env.update(extra_env)
        return subprocess.run(
            [sys.executable, str(SCRIPT), name, str(self.work)],
            cwd=self.repo,
            env=env,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )

    def outputs(self):
        return dict(
            line.split("=", 1)
            for line in self.output_path.read_text().splitlines()
            if "=" in line
        )

    def comment(self, author, body, minute, identifier):
        return {
            "author": {"login": author},
            "body": body,
            "createdAt": f"2026-01-01T00:0{minute}:00Z",
            "id": str(identifier),
        }

    def make_candidate(self, push=False):
        self.git("switch", "-c", "tokicv-candidate")
        (self.repo / "feature.txt").write_text("candidate\n")
        self.git("add", "feature.txt")
        self.git("commit", "-m", "docs: add recovered candidate")
        candidate = self.git("rev-parse", "HEAD")
        artifact = self.root / "artifact"
        artifact.mkdir()
        self.git("bundle", "create", str(artifact / "change.bundle"),
                 "refs/heads/tokicv-candidate", f"^{self.base}")
        (artifact / "candidate.json").write_text(json.dumps({
            "version": 1, "issue": ISSUE, "base": self.base,
            "candidate": candidate, "title": "docs: add recovered candidate",
        }))
        (artifact / "issue.json").write_text(json.dumps({
            "number": ISSUE, "title": "测试交付", "body": "实现一个可验证的测试功能",
            "timeline": [], "base": self.base, "base_branch": "main",
        }))
        self.git("switch", "main")
        if push:
            self.git("push", "origin", f"{candidate}:refs/heads/factory/issue-7")
        return {"artifact": artifact, "candidate": candidate}

    def recovery_state(self, fixture, stage, message="需要处理"):
        return {
            "version": 1, "kind": "recovery", "issue": ISSUE,
            "stage": stage, "message": message, "candidate": fixture["candidate"],
            "base": self.base, "candidate_artifact": ARTIFACT,
            "run_id": int(RUN_ID), "run_attempt": 1,
            "run_url": "https://github.com/owner/repo/actions/runs/499",
        }

    def posted(self):
        return [json.loads(line)["body"] for line in self.posts_path.read_text().splitlines()]

    def posted_state(self):
        body = self.posted()[-1]
        start = body.index("{", body.index("tokicv-delivery:state"))
        return body, json.loads(body[start:body.index("\n-->", start)])

    def copy_candidate_artifact(self, fixture):
        shutil.copytree(fixture["artifact"], self.work / "candidate")

    def report_env(self, fixture, review, package):
        self.copy_candidate_artifact(fixture)
        (self.work / "review.json").write_text(json.dumps(review))
        return {
            "STATUS": "ready", "REVIEW_RESULT": "success", "PACKAGE_RESULT": package,
            "CANDIDATE": fixture["candidate"], "CANDIDATE_ARTIFACT_NAME": ARTIFACT,
        }

    def test_timeline_keeps_bot_markers_and_owner_replies_in_order(self):
        question = {
            "version": 1, "kind": "question", "issue": ISSUE,
            "message": "需要选择缓存策略\nA. 保留现有缓存 {cache}\nB. 重建缓存",
            "run_url": "https://github.com/owner/repo/actions/runs/400",
        }
        self.set_scenario(comments=[
            self.comment(OWNER, "/tokicv 选 A", 3, 3),
            self.comment("github-actions", state_block(question), 1, 1),
            self.comment("other[bot]", state_block(question), 2, 2),
            self.comment("stranger", "/tokicv 选 B", 4, 4),
        ])

        completed = self.command("prompt")

        self.assertEqual(completed.returncode, 0, completed.stdout)
        context = json.loads((self.work / "issue.json").read_text())
        self.assertEqual([entry["role"] for entry in context["timeline"]],
                         ["factory", "owner"])
        self.assertEqual(context["timeline"][0]["message"], question["message"])
        self.assertEqual(context["timeline"][1]["body"], "/tokicv 选 A")
        prompt = (self.work / "implement.md").read_text()
        self.assertIn("保留现有缓存", prompt)
        self.assertIn("/tokicv 选 A", prompt)
        self.assertNotIn("selected_option", prompt)
        self.assertEqual(self.outputs()["skip_implementation"], "false")

    def test_implementation_prompt_requires_architecture_preflight(self):
        completed = self.command("prompt")

        self.assertEqual(completed.returncode, 0, completed.stdout)
        prompt = (self.work / "implement.md").read_text()
        self.assertIn("GLOSSARY.md", prompt)
        self.assertIn("docs/adr/", prompt)
        self.assertIn(".agents/skills/mygo-maintenance/SKILL.md", prompt)
        self.assertIn(".agents/skills/README.md", prompt)
        self.assertIn("What a good test is", prompt)
        self.assertIn("Phase 1 至 Phase 6", prompt)
        self.assertIn("Deep vs shallow", prompt)
        self.assertIn("Challenge against the glossary", prompt)
        self.assertIn("用户确认、并行子代理", prompt)
        self.assertIn("复用已有模块", prompt)
        self.assertIn("needs_decision", prompt)

    def test_review_prompt_uses_trusted_baseline_rules(self):
        code_review = (
            ROOT / ".agents/skills/code-review/SKILL.md"
        ).read_text()
        for path, content in {
            "AGENTS.md": "# Agent rules\n\nRepository sentinel\n",
            "GLOSSARY.md": "# Glossary\n\nBaseline term sentinel\n",
            "docs/agents/domain.md": "# Domain\n\nDomain sentinel\n",
            "docs/adr/0001-first.md": "# First ADR\n\nFirst architecture sentinel\n",
            "docs/adr/0002-second.md": "# Second ADR\n\nSecond architecture sentinel\n",
            ".agents/skills/mygo-maintenance/SKILL.md": (
                "# MyGo rules\n\nMyGo maintenance sentinel\n"
            ),
            ".agents/skills/code-review/SKILL.md": code_review,
        }.items():
            target = self.repo / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content)
        self.git("add", "AGENTS.md", "GLOSSARY.md", "docs", ".agents")
        self.git("commit", "-m", "docs: add trusted architecture fixture")
        self.base = self.git("rev-parse", "HEAD")
        self.env["BASE"] = self.base

        fixture = self.make_candidate()
        self.git("switch", "tokicv-candidate")
        (self.repo / "GLOSSARY.md").write_text(
            "# Glossary\n\nCandidate term sentinel\n"
        )
        (self.repo / ".agents/skills/code-review/SKILL.md").write_text(
            "# Code Review\n\nCandidate review sentinel\n"
        )
        self.git("add", "GLOSSARY.md", ".agents/skills/code-review/SKILL.md")
        self.git("commit", "-m", "docs: replace glossary")
        candidate = self.git("rev-parse", "HEAD")
        self.git("switch", "main")
        (self.repo / ".agents/skills/code-review/SKILL.md").write_text(
            "# Code Review\n\nWorktree review sentinel\n"
        )
        self.copy_candidate_artifact(fixture)
        (self.work / "candidate" / "issue.json").write_text(json.dumps({
            "number": ISSUE,
            "title": "测试交付",
            "body": "实现一个可验证的测试功能",
            "timeline": [],
            "base": self.base,
            "base_branch": "main",
        }, ensure_ascii=False))

        completed = self.command("review-prompt", {"CANDIDATE": candidate})

        self.assertEqual(completed.returncode, 0, completed.stdout)
        prompt = (self.work / "review.md").read_text()
        baseline, diff = prompt.split("=== 候选 diff", 1)
        self.assertIn("First architecture sentinel", baseline)
        self.assertIn("Second architecture sentinel", baseline)
        self.assertIn("Baseline term sentinel", baseline)
        self.assertIn("Repository sentinel", baseline)
        self.assertIn("Domain sentinel", baseline)
        self.assertIn("MyGo maintenance sentinel", baseline)
        self.assertIn("Mysterious Name", baseline)
        self.assertIn("The repo overrides", baseline)
        self.assertNotIn("ask the user where the spec is", baseline)
        self.assertNotIn("Spawn both sub-agents", baseline)
        self.assertNotIn("Candidate term sentinel", baseline)
        self.assertNotIn("Candidate review sentinel", baseline)
        self.assertNotIn("Worktree review sentinel", baseline)
        self.assertIn("Candidate term sentinel", diff)
        self.assertIn("Candidate review sentinel", diff)
        self.assertIn("重复实现", prompt)
        self.assertIn("职责错放", prompt)
        self.assertIn("具体规则违规", prompt)
        config = (self.work / "codex-review" / "config.toml").read_text()
        self.assertIn("shell_tool = false", config)
        self.assertIn("multi_agent = false", config)

    def test_review_prompt_handles_baseline_before_skills(self):
        fixture = self.make_candidate()
        self.copy_candidate_artifact(fixture)

        completed = self.command(
            "review-prompt", {"CANDIDATE": fixture["candidate"]}
        )

        self.assertEqual(completed.returncode, 0, completed.stdout)
        prompt = (self.work / "review.md").read_text()
        self.assertIn("当前基线没有 code-review 技能原文", prompt)
        self.assertIn("具体规则违规", prompt)

    def test_review_blocked_candidate_requires_append_then_accepts_it(self):
        fixture = self.make_candidate()
        state = self.recovery_state(fixture, "review_blocked", "缺少错误路径测试")
        self.set_scenario(
            comments=[
                self.comment(BOT, state_block(state), 1, 1),
                self.comment(OWNER, "/tokicv 补充错误路径测试", 2, 2),
            ],
            artifacts={ARTIFACT: str(fixture["artifact"])},
        )

        prepared = self.command("prompt")

        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertEqual(self.git("rev-parse", "HEAD"), fixture["candidate"])
        recovery = json.loads((self.work / "recovery.json").read_text())
        self.assertTrue(recovery["must_change"])
        self.assertIn("缺少错误路径测试", (self.work / "implement.md").read_text())
        self.assertIn('["run", "download", "500"', self.log_path.read_text())

        (self.work / "implementation.json").write_text(json.dumps({
            "status": "ready", "message": "未修改", "title": "docs: add recovered candidate",
        }))
        recorded = self.command("candidate")
        self.assertEqual(recorded.returncode, 0, recorded.stdout)
        self.assertEqual(self.outputs()["status"], "needs_recovery")
        reported = json.loads((self.work / "candidate" / "recovery.json").read_text())
        self.assertEqual(reported["stage"], "review_blocked")
        self.assertEqual(reported["candidate_artifact"], ARTIFACT)
        self.assertEqual(reported["run_id"], int(RUN_ID))
        self.assertIn("未在候选之上产生新提交", reported["message"])

        (self.repo / "feature.txt").write_text("candidate\nreview fix\n")
        (self.work / "implementation.json").write_text(json.dumps({
            "status": "ready", "message": "补充错误路径测试",
            "title": "docs: address review blocker",
        }))
        appended = self.command("candidate")
        self.assertEqual(appended.returncode, 0, appended.stdout)
        new_candidate = self.outputs()["candidate"]
        self.assertEqual(self.outputs()["status"], "ready")
        self.assertNotEqual(new_candidate, fixture["candidate"])
        self.assertEqual(self.git("rev-parse", f"{new_candidate}^"), fixture["candidate"])

    def test_package_failure_with_empty_command_reuses_same_sha(self):
        fixture = self.make_candidate(push=True)
        state = self.recovery_state(fixture, "package_failed", "完整验证或打包失败")
        self.set_scenario(comments=[
            self.comment(BOT, state_block(state), 1, 1),
            self.comment(OWNER, "/tokicv", 2, 2),
        ])

        prepared = self.command("prompt")

        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertEqual(self.git("rev-parse", "HEAD"), fixture["candidate"])
        self.assertFalse(json.loads((self.work / "recovery.json").read_text())["must_change"])
        self.assertNotIn('["run", "download"', self.log_path.read_text())
        self.assertEqual(self.outputs()["skip_implementation"], "true")
        recorded = self.command("candidate")
        self.assertEqual(recorded.returncode, 0, recorded.stdout)
        self.assertEqual(self.outputs()["status"], "ready")
        self.assertEqual(self.outputs()["candidate"], fixture["candidate"])

    def test_changed_baseline_blocks_until_explicit_restart(self):
        fixture = self.make_candidate()
        (self.repo / "README.md").write_text("new base\n")
        self.git("add", "README.md")
        self.git("commit", "-m", "docs: advance the base")
        new_base = self.git("rev-parse", "HEAD")
        state = self.recovery_state(fixture, "package_failed", "完整验证或打包失败")
        self.set_scenario(comments=[
            self.comment(BOT, state_block(state), 1, 1),
            self.comment(OWNER, "/tokicv", 2, 2),
        ])

        blocked = self.command("prompt", {"BASE": new_base})

        self.assertEqual(blocked.returncode, 0, blocked.stdout)
        data = json.loads((self.work / "recovery-blocked.json").read_text())
        self.assertEqual(data["stage"], "recovery_failed")
        self.assertIn(new_base, data["message"])
        self.assertIn(RESTART, data["message"])
        self.assertEqual(self.outputs()["skip_implementation"], "true")

        recovery_state = dict(state)
        recovery_state.update({"stage": "recovery_failed", "message": data["message"],
                               "base": new_base})
        self.set_scenario(comments=[
            self.comment(BOT, state_block(recovery_state), 3, 3),
            self.comment(OWNER, RESTART, 4, 4),
        ])
        restarted = self.command("prompt", {"BASE": new_base})

        self.assertEqual(restarted.returncode, 0, restarted.stdout)
        self.assertEqual(self.outputs()["skip_implementation"], "false")
        self.assertEqual(self.git("rev-parse", "HEAD"), new_base)
        context = json.loads((self.work / "issue.json").read_text())
        self.assertEqual(context["restart"]["candidate"], fixture["candidate"])

    def test_review_blocked_report_records_artifact_run_and_stage(self):
        fixture = self.make_candidate()
        env = self.report_env(fixture, {"status": "blocked", "message": "缺少错误路径测试"}, "skipped")

        completed = self.command("report", env)

        self.assertEqual(completed.returncode, 0, completed.stdout)
        body, state = self.posted_state()
        self.assertIn("缺少错误路径测试", body)
        self.assertIn(ARTIFACT, body)
        self.assertEqual(state["stage"], "review_blocked")
        self.assertEqual(state["candidate"], fixture["candidate"])
        self.assertEqual(state["base"], self.base)
        self.assertEqual(state["candidate_artifact"], ARTIFACT)
        self.assertEqual(state["run_id"], int(RUN_ID))
        self.assertEqual(state["run_attempt"], 1)
        self.assertNotIn('["pr", "create"]', self.log_path.read_text())

    def test_package_failed_report_keeps_review_evidence(self):
        fixture = self.make_candidate()
        env = self.report_env(fixture, {"status": "pass", "message": "审查通过"}, "failure")

        completed = self.command("report", env)

        self.assertEqual(completed.returncode, 0, completed.stdout)
        body, state = self.posted_state()
        self.assertIn("完整验证或打包失败", body)
        self.assertEqual(state["stage"], "package_failed")
        self.assertEqual(state["candidate"], fixture["candidate"])
        self.assertNotIn('["pr", "create"]', self.log_path.read_text())

    def test_existing_pr_is_reused_before_and_after_completion_backfill(self):
        fixture = self.make_candidate(push=True)
        pull = {
            "url": "https://github.com/owner/repo/pull/9",
            "headRefName": "factory/issue-7",
            "isCrossRepository": False,
            "headRefOid": fixture["candidate"],
            "baseRefName": "main",
            "state": "OPEN",
        }
        self.set_scenario(pulls=[pull])

        prepared = self.command("prompt")
        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertEqual(self.outputs()["skip_implementation"], "true")
        self.assertEqual(self.git("rev-parse", "HEAD"), fixture["candidate"])
        self.assertEqual(
            json.loads((self.work / "recovery.json").read_text())["stage"],
            "publication_failed",
        )

        env = self.report_env(fixture, {"status": "pass", "message": "审查通过"}, "success")
        reported = self.command("report", env)
        self.assertEqual(reported.returncode, 0, reported.stdout)
        body, state = self.posted_state()
        self.assertEqual(state["kind"], "delivered")
        self.assertEqual(state["pr"], pull["url"])
        self.assertIn(pull["url"], body)
        self.assertNotIn('["pr", "create"]', self.log_path.read_text())
        self.assertEqual(len(self.posted()), 1)

        self.set_scenario(
            comments=[self.comment(BOT, body, 3, 9)],
            pulls=[pull],
        )
        repeated = self.command("prompt")
        self.assertEqual(repeated.returncode, 0, repeated.stdout)
        self.assertEqual(self.outputs()["skip_implementation"], "true")
        self.assertTrue((self.work / "already-delivered").exists())
        self.assertEqual(len(self.posted()), 1)

    def test_legacy_completion_comment_with_same_pr_is_already_delivered(self):
        pull = {
            "url": "https://github.com/owner/repo/pull/2",
            "headRefName": "factory/issue-7",
            "isCrossRepository": False,
            "headRefOid": "a" * 40,
            "baseRefName": "main",
            "state": "MERGED",
        }
        body = (
            "完整验证已通过。\n\n"
            "- [候选 PR](https://github.com/owner/repo/pull/2)\n"
            "- [Linux 安装包和验证报告](https://github.com/owner/repo/actions/runs/1)"
        )
        self.set_scenario(
            comments=[self.comment("github-actions", body, 1, 1)],
            pulls=[pull],
        )

        completed = self.command("prompt")

        self.assertEqual(completed.returncode, 0, completed.stdout)
        self.assertEqual(self.outputs()["skip_implementation"], "true")
        self.assertTrue((self.work / "already-delivered").exists())
        self.assertFalse(self.posts_path.exists())


if __name__ == "__main__":
    unittest.main()
