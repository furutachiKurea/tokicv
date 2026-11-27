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
PR = 9
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
elif args[:2] in (["issue", "comment"], ["pr", "comment"]):
    body = Path(args[args.index("--body-file") + 1]).read_text()
    with Path(os.environ["GH_POSTS"]).open("a") as posts:
        print(json.dumps({
            "kind": args[0],
            "number": int(args[2]),
            "body": body,
        }, ensure_ascii=False), file=posts)
elif args[:2] == ["pr", "list"]:
    print(json.dumps(scenario.get("pulls", []), ensure_ascii=False))
elif args[:2] == ["pr", "view"]:
    number = int(args[2])
    pull = next((pull for pull in scenario.get("pulls", []) if pull.get("number") == number), None)
    if pull is None:
        print("pull request not found", file=sys.stderr)
        sys.exit(1)
    print(json.dumps(pull, ensure_ascii=False))
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
        if path.is_file():
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
        self.event_path = self.root / "event.json"
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
            "GITHUB_EVENT_PATH": str(self.event_path),
            "EVENT_NAME": "issues",
            "TARGET_KIND": "issue",
            "TARGET_NUMBER": str(ISSUE),
            "COMMENT_ID": "",
        })
        self.set_scenario()
        self.set_event("issues")

    def tearDown(self):
        self.temp.cleanup()

    def git(self, *args):
        return subprocess.check_output(
            ["git", *args], cwd=self.repo, text=True, stderr=subprocess.STDOUT
        ).strip()

    def set_scenario(self, comments=None, pulls=None, artifacts=None, state="OPEN", labels=None):
        scenario = {
            "issue": {
                "title": "测试交付",
                "body": "实现一个可验证的测试功能",
                "state": state,
                "labels": [{"name": name} for name in labels or []],
                "comments": comments or [],
            },
            "pulls": pulls or [],
            "artifacts": artifacts or {},
        }
        self.scenario_path.write_text(json.dumps(scenario, ensure_ascii=False))
        return scenario

    def scenario(self):
        return json.loads(self.scenario_path.read_text())

    def set_event(self, event_name, body=None, comment_id=None, target_kind="issue",
                  target_number=ISSUE, issue_number=ISSUE, pull_request=False,
                  labels=None):
        payload = {}
        if event_name == "issues":
            payload = {"issue": {
                "number": issue_number,
                "labels": [{"name": name} for name in labels or []],
            }}
        elif event_name == "issue_comment":
            issue = {"number": target_number if pull_request else issue_number}
            if pull_request:
                issue["pull_request"] = {"url": "https://api.github.com/repos/owner/repo/pulls/9"}
            payload = {
                "issue": issue,
                "comment": {"id": int(comment_id or 0), "body": body or ""},
            }
        elif event_name == "workflow_dispatch":
            payload = {"inputs": {"issue": str(issue_number)}}
        elif event_name == "push":
            payload = {"ref": "refs/heads/factory/tokicv-delivery"}
        self.event_path.write_text(json.dumps(payload, ensure_ascii=False))
        self.env.update({
            "EVENT_NAME": event_name,
            "TARGET_KIND": target_kind,
            "TARGET_NUMBER": str(target_number),
            "COMMENT_ID": str(comment_id or ""),
        })

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
        values = {}
        for line in self.output_path.read_text().splitlines():
            if "=" in line:
                key, value = line.split("=", 1)
                values[key] = value
        return values

    def comment(self, author, body, minute, identifier):
        return {
            "author": {"login": author},
            "body": body,
            "createdAt": f"2026-01-01T00:{minute:02d}:00Z",
            "id": str(identifier),
        }

    def pull(self, candidate, state="OPEN", comments=None, draft=True):
        return {
            "number": PR,
            "url": f"https://github.com/owner/repo/pull/{PR}",
            "headRefName": f"factory/issue-{ISSUE}",
            "isCrossRepository": False,
            "headRefOid": candidate,
            "baseRefName": "main",
            "state": state,
            "isDraft": draft,
            "mergedAt": "2026-01-01T00:10:00Z" if state == "MERGED" else None,
            "comments": comments or [],
        }

    def make_candidate(self, push=False):
        self.git("switch", "-C", "tokicv-candidate", "main")
        (self.repo / "feature.txt").write_text("candidate\n")
        self.git("add", "feature.txt")
        self.git("commit", "-m", "docs: add recovered candidate")
        candidate = self.git("rev-parse", "HEAD")
        artifact = self.root / f"artifact-{candidate[:8]}"
        artifact.mkdir()
        self.git("bundle", "create", str(artifact / "change.bundle"),
                 "refs/heads/tokicv-candidate", f"^{self.base}")
        (artifact / "candidate.json").write_text(json.dumps({
            "version": 1,
            "issue": ISSUE,
            "base": self.base,
            "diff_base": self.base,
            "candidate": candidate,
            "title": "docs: add recovered candidate",
            "replace_branch_head": None,
            "target": {"kind": "issue", "number": ISSUE},
            "trigger": {"event": "issues", "source": "issue", "comment_id": None},
            "consumed_comment_ids": [],
        }))
        (artifact / "issue.json").write_text(json.dumps({
            "number": ISSUE,
            "title": "测试交付",
            "body": "实现一个可验证的测试功能",
            "timeline": [],
            "base": self.base,
            "base_branch": "main",
            "target": {"kind": "issue", "number": ISSUE},
            "trigger": {"event": "issues", "source": "issue", "comment_id": None},
            "consumed_comment_ids": [],
        }, ensure_ascii=False))
        self.git("switch", "main")
        if push:
            self.git("push", "origin", f"{candidate}:refs/heads/factory/issue-{ISSUE}")
        return {"artifact": artifact, "candidate": candidate, "base": self.base}

    def advance_main(self):
        (self.repo / "main-only.txt").write_text(f"new base {self.base}\n")
        self.git("add", "main-only.txt")
        self.git("commit", "-m", "docs: advance the base")
        self.base = self.git("rev-parse", "HEAD")
        self.env["BASE"] = self.base
        return self.base

    def start_fresh_runner(self):
        checkout = self.root / "runner"
        self.git("clone", "--no-local", "--single-branch", "--branch", "main",
                 str(self.repo), str(checkout))
        self.repo = checkout
        self.git("remote", "set-url", "origin", str(self.remote))

    def recovery_state(self, fixture, stage, message="需要处理", base=None, consumed=None):
        return {
            "version": 1,
            "kind": "recovery",
            "issue": ISSUE,
            "stage": stage,
            "message": message,
            "candidate": fixture["candidate"],
            "base": base or fixture["base"],
            "diff_base": fixture["base"],
            "candidate_artifact": ARTIFACT,
            "run_id": int(RUN_ID),
            "run_attempt": 1,
            "target": {"kind": "issue", "number": ISSUE},
            "trigger": {"event": "issues", "source": "issue", "comment_id": None},
            "consumed_comment_ids": consumed or [],
            "run_url": "https://github.com/owner/repo/actions/runs/499",
        }

    def delivered_state(self, fixture, pull, consumed=None, run_id=400):
        return {
            "version": 1,
            "kind": "delivered",
            "issue": ISSUE,
            "candidate": fixture["candidate"],
            "base": fixture["base"],
            "diff_base": fixture["base"],
            "pr": pull["url"],
            "run_id": run_id,
            "run_attempt": 1,
            "candidate_artifact": f"candidate-{run_id}-1",
            "target": {"kind": "issue", "number": ISSUE},
            "trigger": {"event": "issues", "source": "issue", "comment_id": None},
            "consumed_comment_ids": consumed or [],
            "run_url": f"https://github.com/owner/repo/actions/runs/{run_id}",
        }

    def copy_candidate_artifact(self, fixture):
        destination = self.work / "candidate"
        shutil.rmtree(destination, ignore_errors=True)
        shutil.copytree(fixture["artifact"], destination)

    def reported_events(self):
        if not self.posts_path.exists():
            return []
        return [json.loads(line) for line in self.posts_path.read_text().splitlines()]

    def posted(self):
        return [event["body"] for event in self.reported_events()]

    def posted_state(self):
        body = self.posted()[-1]
        start = body.index("{", body.index("tokicv-delivery:state"))
        return body, json.loads(body[start:body.index("\n-->", start)])

    def write_review(self, status="pass", message="审查通过"):
        (self.work / "review.json").write_text(json.dumps({
            "status": status,
            "message": message,
        }))

    def report_env(self, fixture, review, package):
        self.copy_candidate_artifact(fixture)
        self.write_review(review["status"], review["message"])
        return {
            "STATUS": "ready",
            "REVIEW_RESULT": "success",
            "PACKAGE_RESULT": package,
            "CANDIDATE": fixture["candidate"],
        }

    def test_intake_boundary_canonical_pr_and_discussion_label(self):
        pull = self.pull(self.base)
        self.set_scenario(pulls=[pull])
        self.set_event("issue_comment", body="/tokicv 修改文案", comment_id="2",
                       target_kind="pr", target_number=PR, pull_request=True)

        intake = self.command("intake")

        self.assertEqual(intake.returncode, 0, intake.stdout)
        self.assertEqual(self.outputs()["issue"], str(ISSUE))
        self.assertEqual(self.outputs()["target_kind"], "pr")
        self.assertEqual(self.outputs()["target_number"], str(PR))
        self.assertEqual(self.outputs()["skip"], "false")
        self.assertIn('["pr", "view", "9"', self.log_path.read_text())

        for body in ("/tokicvfoo", " /tokicv", "普通评论"):
            with self.subTest(body=body):
                self.set_event("issue_comment", body=body, comment_id="3")
                skipped = self.command("intake")
                self.assertEqual(skipped.returncode, 0, skipped.stdout)
                self.assertEqual(self.outputs()["skip"], "true")

        self.set_event("issue_comment", body="/TOKICV 继续", comment_id="4")
        accepted = self.command("intake")
        self.assertEqual(accepted.returncode, 0, accepted.stdout)
        self.assertEqual(self.outputs()["skip"], "false")

        self.set_event("issues", labels=["discussion"])
        discussion = self.command("intake")
        self.assertEqual(discussion.returncode, 0, discussion.stdout)
        self.assertEqual(self.outputs()["skip"], "true")

        self.set_event("issue_comment", body="/tokicv 开始实现", comment_id="5")
        discussion_comment = self.command("intake")
        self.assertEqual(discussion_comment.returncode, 0, discussion_comment.stdout)
        self.assertEqual(self.outputs()["skip"], "false")

        self.set_scenario(pulls=[{
            **pull, "headRefName": "feature/manual",
        }])
        self.set_event("issue_comment", body="/tokicv", comment_id="6",
                       target_kind="pr", target_number=PR, pull_request=True)
        manual_pr = self.command("intake")
        self.assertEqual(manual_pr.returncode, 0, manual_pr.stdout)
        self.assertEqual(self.outputs()["skip"], "true")

    def test_workflow_keeps_zero_runner_boundary_and_native_issue_lock(self):
        parent = (ROOT / ".github/workflows/delivery.yml").read_text()
        called = (ROOT / ".github/workflows/delivery-run.yml").read_text()

        self.assertIn("name: TokiCV delivery", parent)
        self.assertIn("startsWith(github.event.comment.body, '/tokicv')", parent)
        intake_step = parent.split("- name: Resolve the canonical issue", 1)[1].split(
            "\n  delivery:", 1,
        )[0]
        self.assertIn("DEFAULT_BRANCH: ${{ github.event.repository.default_branch }}", intake_step)
        self.assertNotIn("\nconcurrency:", parent)
        self.assertIn("uses: ./.github/workflows/delivery-run.yml", parent)
        self.assertIn("group: tokicv-delivery-${{ inputs.issue }}", called)
        self.assertIn("cancel-in-progress: false", called)
        self.assertIn("queue: max", called)

    def test_timeline_merges_issue_and_pr_comments(self):
        question = {
            "version": 1,
            "kind": "question",
            "issue": ISSUE,
            "message": "需要选择缓存策略\nA. 保留现有缓存 {cache}\nB. 重建缓存",
            "consumed_comment_ids": [],
            "run_url": "https://github.com/owner/repo/actions/runs/400",
        }
        issue_comments = [
            self.comment("github-actions", state_block(question), 1, 1),
            self.comment(OWNER, "/tokicv 选 A", 2, 2),
            self.comment("stranger", "/tokicv 选 B", 4, 4),
        ]
        pull_comments = [
            self.comment(OWNER, "/tokicv 请再补充一句", 3, 3),
            self.comment(OWNER, "/tokicv 这是 PR 的最终要求", 4, 5),
        ]
        pull = self.pull(self.base, comments=pull_comments)
        self.set_scenario(comments=issue_comments, pulls=[pull])
        self.set_event("issue_comment", body="/tokicv 这是 PR 的最终要求", comment_id="5",
                       target_kind="pr", target_number=PR, pull_request=True)
        payload = json.loads(self.event_path.read_text())
        payload["comment"]["node_id"] = "IC_review_feedback"
        self.event_path.write_text(json.dumps(payload))
        pull_comments[-1]["id"] = "IC_review_feedback"
        self.set_scenario(comments=issue_comments, pulls=[pull])
        intake = self.command("intake")
        self.assertEqual(intake.returncode, 0, intake.stdout)
        self.assertEqual(self.outputs()["comment_id"], "IC_review_feedback")
        self.env["COMMENT_ID"] = self.outputs()["comment_id"]

        prepared = self.command("prompt")

        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        context = json.loads((self.work / "issue.json").read_text())
        self.assertEqual(
            [entry["role"] for entry in context["timeline"]],
            ["factory", "owner", "owner", "owner"],
        )
        self.assertEqual(context["timeline"][1]["comment_id"], "2")
        self.assertEqual(context["timeline"][2]["source"], "pr")
        self.assertEqual(context["target"], {"kind": "pr", "number": PR})
        self.assertEqual(context["consumed_comment_ids"], ["2", "3", "IC_review_feedback"])
        prompt = (self.work / "implement.md").read_text()
        self.assertIn("这是 PR 的最终要求", prompt)
        self.assertNotIn("stranger", prompt)

    def test_review_prompt_uses_trusted_baseline_and_diff_base(self):
        for path, content in {
            "AGENTS.md": "# Agent rules\n\nRepository sentinel\n",
            "GLOSSARY.md": "# Glossary\n\nBaseline term sentinel\n",
            "docs/agents/domain.md": "# Domain\n\nDomain sentinel\n",
            "docs/adr/0001-first.md": "# First ADR\n\nFirst architecture sentinel\n",
            "docs/adr/0002-second.md": "# Second ADR\n\nSecond architecture sentinel\n",
            ".agents/skills/mygo-maintenance/SKILL.md": (
                "# MyGo rules\n\nMyGo maintenance sentinel\n"
            ),
        }.items():
            target = self.repo / path
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content)
        self.git("add", "AGENTS.md", "GLOSSARY.md", "docs", ".agents")
        self.git("commit", "-m", "docs: add trusted architecture fixture")
        self.base = self.git("rev-parse", "HEAD")
        self.env["BASE"] = self.base
        fixture = self.make_candidate()
        diff_base = fixture["base"]
        self.advance_main()
        self.copy_candidate_artifact(fixture)
        (self.work / "candidate" / "issue.json").write_text(json.dumps({
            "number": ISSUE,
            "title": "测试交付",
            "body": "实现一个可验证的测试功能",
            "timeline": [],
            "base": self.base,
            "base_branch": "main",
        }, ensure_ascii=False))

        completed = self.command("review-prompt", {
            "CANDIDATE": fixture["candidate"],
            "DIFF_BASE": diff_base,
        })

        self.assertEqual(completed.returncode, 0, completed.stdout)
        prompt = (self.work / "review.md").read_text()
        baseline, diff = prompt.split("=== 候选 diff", 1)
        self.assertIn("First architecture sentinel", baseline)
        self.assertIn("Baseline term sentinel", baseline)
        self.assertIn("Repository sentinel", baseline)
        self.assertIn("MyGo maintenance sentinel", baseline)
        self.assertIn("feature.txt", diff)
        self.assertNotIn("main-only.txt", diff)
        self.assertIn("分叉点", prompt)

    def test_missing_review_skill_degrades_cleanly(self):
        fixture = self.make_candidate()
        self.copy_candidate_artifact(fixture)

        completed = self.command("review-prompt", {
            "CANDIDATE": fixture["candidate"],
            "DIFF_BASE": fixture["base"],
        })

        self.assertEqual(completed.returncode, 0, completed.stdout)
        self.assertIn("当前基线没有 code-review 技能原文", (self.work / "review.md").read_text())

    def test_issue_rework_reuses_open_pr_and_preserves_draft(self):
        fixture = self.make_candidate(push=True)
        pull = self.pull(fixture["candidate"], comments=[])
        delivered = self.delivered_state(fixture, pull, consumed=["1"])
        self.set_scenario(comments=[
            self.comment(BOT, state_block(delivered), 1, 1),
            self.comment(OWNER, "/tokicv 修改提示文案", 2, 2),
        ], pulls=[pull])
        self.set_event("issue_comment", body="/tokicv 修改提示文案", comment_id="2")

        prepared = self.command("prompt")

        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertEqual(self.git("rev-parse", "HEAD"), fixture["candidate"])
        recovery = json.loads((self.work / "recovery.json").read_text())
        self.assertTrue(recovery["must_change"])
        self.assertEqual(recovery["diff_base"], fixture["base"])

        (self.repo / "feature.txt").write_text("candidate\nreviewed change\n")
        (self.work / "implementation.json").write_text(json.dumps({
            "status": "ready",
            "message": "调整提示文案并保留原 PR",
            "title": "docs: adjust the prompt wording",
        }))
        recorded = self.command("candidate")

        self.assertEqual(recorded.returncode, 0, recorded.stdout)
        new_candidate = self.outputs()["candidate"]
        self.assertNotEqual(new_candidate, fixture["candidate"])
        self.assertEqual(self.git("rev-parse", f"{new_candidate}^"), fixture["candidate"])
        metadata = json.loads((self.work / "candidate" / "candidate.json").read_text())
        self.assertEqual(metadata["consumed_comment_ids"], ["1", "2"])
        self.assertEqual(metadata["target"], {"kind": "issue", "number": ISSUE})

        self.write_review()
        env = {
            "STATUS": "ready", "REVIEW_RESULT": "success",
            "PACKAGE_RESULT": "success", "CANDIDATE": new_candidate,
        }
        reported = self.command("report", env)

        self.assertEqual(reported.returncode, 0, reported.stdout)
        body, state = self.posted_state()
        self.assertEqual(state["kind"], "delivered")
        self.assertEqual(state["candidate"], new_candidate)
        self.assertEqual(state["target"], {"kind": "issue", "number": ISSUE})
        self.assertEqual(state["consumed_comment_ids"], ["1", "2"])
        self.assertEqual(self.reported_events()[-1]["kind"], "issue")
        self.assertNotIn('["pr", "create"]', self.log_path.read_text())
        self.assertEqual(
            self.git("--git-dir", str(self.remote), "rev-parse",
                     f"refs/heads/factory/issue-{ISSUE}"),
            new_candidate,
        )
        self.assertIn("普通评论只作讨论", body)

    def test_pr_comment_rework_backfills_to_pr(self):
        fixture = self.make_candidate(push=True)
        delivered = self.delivered_state(fixture, self.pull(fixture["candidate"]), consumed=["1"])
        pull = self.pull(fixture["candidate"], comments=[
            self.comment(OWNER, "/tokicv PR 内继续调整", 2, 2),
        ])
        self.set_scenario(comments=[
            self.comment(BOT, state_block(delivered), 1, 1),
        ], pulls=[pull])
        self.set_event("issue_comment", body="/tokicv PR 内继续调整", comment_id="2",
                       target_kind="pr", target_number=PR, pull_request=True)

        prepared = self.command("prompt")
        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        context = json.loads((self.work / "issue.json").read_text())
        self.assertEqual(context["target"], {"kind": "pr", "number": PR})
        self.assertEqual(context["consumed_comment_ids"], ["1", "2"])

        (self.repo / "feature.txt").write_text("candidate\nPR feedback\n")
        (self.work / "implementation.json").write_text(json.dumps({
            "status": "ready", "message": "处理 PR 评论",
            "title": "docs: address PR feedback",
        }))
        recorded = self.command("candidate")
        self.assertEqual(recorded.returncode, 0, recorded.stdout)
        new_candidate = self.outputs()["candidate"]
        self.write_review()
        env = {
            "STATUS": "ready", "REVIEW_RESULT": "success",
            "PACKAGE_RESULT": "success", "CANDIDATE": new_candidate,
        }

        reported = self.command("report", env)

        self.assertEqual(reported.returncode, 0, reported.stdout)
        event = self.reported_events()[-1]
        self.assertEqual(event["kind"], "pr")
        self.assertEqual(event["number"], PR)
        _, state = self.posted_state()
        self.assertEqual(state["target"], {"kind": "pr", "number": PR})
        self.assertEqual(state["issue"], ISSUE)

    def test_main_advanced_branch_rework(self):
        branch_fixture = self.make_candidate(push=True)
        old_base = branch_fixture["base"]
        self.advance_main()
        pull = self.pull(branch_fixture["candidate"])
        delivered = self.delivered_state(branch_fixture, pull, consumed=["1"])
        self.set_scenario(comments=[
            self.comment(BOT, state_block(delivered), 1, 1),
            self.comment(OWNER, "/tokicv 在旧分支上追加修改", 2, 2),
        ], pulls=[pull])
        self.set_event("issue_comment", body="/tokicv 在旧分支上追加修改", comment_id="2")

        prepared = self.command("prompt")
        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        recovery = json.loads((self.work / "recovery.json").read_text())
        self.assertEqual(recovery["diff_base"], old_base)
        self.assertEqual(recovery["candidate"], branch_fixture["candidate"])
        self.assertTrue(recovery["must_change"])

        (self.repo / "feature.txt").write_text("candidate\nafter base advanced\n")
        (self.work / "implementation.json").write_text(json.dumps({
            "status": "ready", "message": "在旧分支追加修改",
            "title": "docs: append on the existing branch",
        }))
        recorded = self.command("candidate")

        self.assertEqual(recorded.returncode, 0, recorded.stdout)
        self.assertEqual(self.outputs()["status"], "ready")
        self.assertEqual(self.outputs()["diff_base"], old_base)
        bundle = self.work / "candidate" / "change.bundle"
        self.assertTrue(bundle.is_file())
        self.assertIn("ok", self.git("bundle", "verify", str(bundle)))

    def test_unpushed_artifact_recovery_after_main_advanced(self):
        artifact_fixture = self.make_candidate()
        artifact_base = artifact_fixture["base"]
        self.advance_main()
        state = self.recovery_state(artifact_fixture, "package_failed", "构建失败")
        self.set_scenario(
            comments=[
                self.comment(BOT, state_block(state), 3, 3),
                self.comment(OWNER, "/tokicv 补充失败路径", 4, 4),
            ],
            artifacts={ARTIFACT: str(artifact_fixture["artifact"])},
        )
        self.set_event("issue_comment", body="/tokicv 补充失败路径", comment_id="4")
        self.start_fresh_runner()

        recovered = self.command("prompt")
        self.assertEqual(recovered.returncode, 0, recovered.stdout)
        artifact_recovery = json.loads((self.work / "recovery.json").read_text())
        self.assertEqual(artifact_recovery["diff_base"], artifact_base)
        self.assertEqual(artifact_recovery["candidate"], artifact_fixture["candidate"])
        self.assertIn('["run", "download", "500"', self.log_path.read_text())

    def test_review_blocked_artifact_then_append_is_reentrant(self):
        fixture = self.make_candidate()
        state = self.recovery_state(fixture, "review_blocked", "缺少错误路径测试")
        self.set_scenario(
            comments=[
                self.comment(BOT, state_block(state), 1, 1),
                self.comment(OWNER, "/tokicv 补充错误路径测试", 2, 2),
            ],
            artifacts={ARTIFACT: str(fixture["artifact"])},
        )
        self.set_event("issue_comment", body="/tokicv 补充错误路径测试", comment_id="2")

        prepared = self.command("prompt")
        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertEqual(self.git("rev-parse", "HEAD"), fixture["candidate"])
        recovery = json.loads((self.work / "recovery.json").read_text())
        self.assertTrue(recovery["must_change"])
        self.assertIn("缺少错误路径测试", (self.work / "implement.md").read_text())

        (self.work / "implementation.json").write_text(json.dumps({
            "status": "ready", "message": "未修改", "title": "docs: add recovered candidate",
        }))
        recorded = self.command("candidate")
        self.assertEqual(recorded.returncode, 0, recorded.stdout)
        self.assertEqual(self.outputs()["status"], "needs_recovery")
        reported = json.loads((self.work / "candidate" / "recovery.json").read_text())
        self.assertEqual(reported["stage"], "review_blocked")
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
        self.assertEqual(self.git("rev-parse", f"{new_candidate}^"), fixture["candidate"])
        self.assertEqual(self.outputs()["diff_base"], fixture["base"])

    def test_empty_tokicv_refreshes_same_candidate(self):
        fixture = self.make_candidate(push=True)
        pull = self.pull(fixture["candidate"])
        delivered = self.delivered_state(fixture, pull, consumed=["1"])
        self.set_scenario(comments=[
            self.comment(BOT, state_block(delivered), 1, 1),
            self.comment(OWNER, "/tokicv", 2, 2),
        ], pulls=[pull])
        self.set_event("issue_comment", body="/tokicv", comment_id="2")

        prepared = self.command("prompt")
        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertEqual(self.outputs()["skip_implementation"], "true")
        self.assertFalse(json.loads((self.work / "recovery.json").read_text())["must_change"])
        self.assertNotIn('["run", "download"', self.log_path.read_text())

        recorded = self.command("candidate")
        self.assertEqual(recorded.returncode, 0, recorded.stdout)
        self.assertEqual(self.outputs()["status"], "ready")
        self.assertEqual(self.outputs()["candidate"], fixture["candidate"])

        self.write_review()
        reported = self.command("report", {
            "STATUS": "ready", "REVIEW_RESULT": "success",
            "PACKAGE_RESULT": "success", "CANDIDATE": fixture["candidate"],
        })
        self.assertEqual(reported.returncode, 0, reported.stdout)
        _, state = self.posted_state()
        self.assertEqual(state["kind"], "delivered")
        self.assertEqual(state["candidate"], fixture["candidate"])
        self.assertEqual(state["consumed_comment_ids"], ["1", "2"])
        self.assertEqual(len(self.posted()), 1)

    def test_same_consumed_comment_is_idempotent(self):
        fixture = self.make_candidate(push=True)
        pull = self.pull(fixture["candidate"])
        delivered = self.delivered_state(fixture, pull, consumed=["10", "11"])
        self.set_scenario(comments=[
            self.comment(BOT, state_block(delivered), 1, 1),
            self.comment(OWNER, "/tokicv 已经吸收的修改", 2, 10),
            self.comment(OWNER, "/tokicv 已经吸收的第二条", 3, 11),
        ], pulls=[pull])
        self.set_event("issue_comment", body="/tokicv 已经吸收的第二条", comment_id="11")

        prepared = self.command("prompt")

        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertEqual(self.outputs()["skip_implementation"], "true")
        self.assertTrue((self.work / "already-delivered").exists())
        self.assertEqual(self.outputs()["already_delivered"], "true")

    def test_merged_pr_notice_is_idempotent(self):
        fixture = self.make_candidate()
        pull = self.pull(fixture["candidate"], state="MERGED")
        self.set_scenario(pulls=[pull])
        self.set_event("issue_comment", body="/tokicv 测试合并后", comment_id="3",
                       target_kind="pr", target_number=PR, pull_request=True)

        prepared = self.command("prompt")
        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        notice = json.loads((self.work / "notice.json").read_text())
        self.assertIn("已合并", notice["message"])
        recorded = self.command("candidate")
        self.assertEqual(recorded.returncode, 0, recorded.stdout)
        self.assertEqual(self.outputs()["status"], "notice")
        reported = self.command("report", {"STATUS": "notice"})
        self.assertEqual(reported.returncode, 0, reported.stdout)
        first = self.reported_events()[-1]
        self.assertEqual(first["kind"], "pr")
        _, state = self.posted_state()
        self.assertEqual(state["kind"], "notice")

        pull_with_notice = self.pull(fixture["candidate"], state="MERGED", comments=[
            self.comment(BOT, self.posted()[-1], 4, 3),
        ])
        self.set_scenario(pulls=[pull_with_notice])
        repeated = self.command("prompt")
        self.assertEqual(repeated.returncode, 0, repeated.stdout)
        self.assertTrue((self.work / "already-delivered").exists())
        self.assertEqual(len(self.posted()), 1)

    def test_closed_issue_and_closed_pr_guards(self):
        self.set_scenario(state="CLOSED")
        self.set_event("issue_comment", body="/tokicv 继续", comment_id="7")
        prepared = self.command("prompt")
        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertIn("Reopen", (self.work / "notice.json").read_text())

        fixture = self.make_candidate(push=True)
        closed = self.pull(fixture["candidate"], state="CLOSED")
        self.set_scenario(pulls=[closed])
        self.set_event("issue_comment", body=RESTART, comment_id="8")
        restart = self.command("prompt")
        self.assertEqual(restart.returncode, 0, restart.stdout)
        self.assertIn("旧 PR 已关闭", (self.work / "notice.json").read_text())

    def test_explicit_rebuild_uses_exact_lease_after_gates(self):
        fixture = self.make_candidate(push=True)
        old_head = fixture["candidate"]
        pull = self.pull(old_head)
        self.advance_main()
        new_base = self.base
        self.set_scenario(pulls=[pull])
        self.set_event("issue_comment", body=RESTART, comment_id="2")

        prepared = self.command("prompt")
        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertEqual(self.git("rev-parse", "HEAD"), new_base)
        context = json.loads((self.work / "issue.json").read_text())
        self.assertEqual(context["replace_branch_head"], old_head)

        (self.repo / "feature.txt").write_text("rebuilt\n")
        (self.work / "implementation.json").write_text(json.dumps({
            "status": "ready", "message": "从当前基线重建",
            "title": "docs: rebuild from the current baseline",
        }))
        recorded = self.command("candidate")
        self.assertEqual(recorded.returncode, 0, recorded.stdout)
        rebuilt = self.outputs()["candidate"]
        metadata = json.loads((self.work / "candidate" / "candidate.json").read_text())
        self.assertEqual(metadata["replace_branch_head"], old_head)
        self.assertEqual(metadata["base"], new_base)

        self.write_review("blocked", "审查阻止重建")
        blocked = self.command("report", {
            "STATUS": "ready", "REVIEW_RESULT": "success",
            "PACKAGE_RESULT": "skipped", "CANDIDATE": rebuilt,
        })
        self.assertEqual(blocked.returncode, 0, blocked.stdout)
        self.assertEqual(
            self.git("--git-dir", str(self.remote), "rev-parse",
                     f"refs/heads/factory/issue-{ISSUE}"),
            old_head,
        )

        self.write_review("pass", "审查通过")
        self.start_fresh_runner()
        published = self.command("report", {
            "STATUS": "ready", "REVIEW_RESULT": "success",
            "PACKAGE_RESULT": "success", "CANDIDATE": rebuilt,
        })
        self.assertEqual(published.returncode, 0, published.stdout)
        self.assertEqual(
            self.git("--git-dir", str(self.remote), "rev-parse",
                     f"refs/heads/factory/issue-{ISSUE}"),
            rebuilt,
        )
        self.assertNotEqual(rebuilt, old_head)

    def test_legacy_completion_comment_with_same_pr_is_already_delivered(self):
        pull = self.pull("a" * 40)
        body = (
            "完整验证已通过。\n\n"
            f"- [候选 PR]({pull['url']})\n"
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

    def test_review_blocked_report_records_artifact_run_and_stage(self):
        fixture = self.make_candidate()
        env = self.report_env(fixture, {"status": "blocked", "message": "缺少错误路径测试"}, "skipped")
        env["STATUS"] = "ready"

        completed = self.command("report", env)

        self.assertEqual(completed.returncode, 0, completed.stdout)
        body, state = self.posted_state()
        self.assertIn("缺少错误路径测试", body)
        self.assertIn(ARTIFACT, body)
        self.assertEqual(state["stage"], "review_blocked")
        self.assertEqual(state["candidate"], fixture["candidate"])
        self.assertEqual(state["candidate_artifact"], ARTIFACT)
        self.assertEqual(state["run_id"], int(RUN_ID))
        self.assertNotIn('["pr", "create"]', self.log_path.read_text())

    def test_package_failure_report_keeps_review_evidence(self):
        fixture = self.make_candidate()
        env = self.report_env(fixture, {"status": "pass", "message": "审查通过"}, "failure")
        env["STATUS"] = "ready"

        completed = self.command("report", env)

        self.assertEqual(completed.returncode, 0, completed.stdout)
        body, state = self.posted_state()
        self.assertIn("完整验证或打包失败", body)
        self.assertEqual(state["stage"], "package_failed")
        self.assertEqual(state["candidate"], fixture["candidate"])
        self.assertNotIn('["pr", "create"]', self.log_path.read_text())

    def test_existing_pr_is_reused_before_and_after_completion_backfill(self):
        fixture = self.make_candidate(push=True)
        pull = self.pull(fixture["candidate"])
        self.set_scenario(pulls=[pull])

        prepared = self.command("prompt")
        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertEqual(self.outputs()["skip_implementation"], "true")
        self.assertEqual(self.git("rev-parse", "HEAD"), fixture["candidate"])
        recovery = json.loads((self.work / "recovery.json").read_text())
        self.assertEqual(recovery["stage"], "publication_failed")

        recorded = self.command("candidate")
        self.assertEqual(recorded.returncode, 0, recorded.stdout)
        self.write_review()
        reported = self.command("report", {
            "STATUS": "ready", "REVIEW_RESULT": "success",
            "PACKAGE_RESULT": "success", "CANDIDATE": fixture["candidate"],
        })
        self.assertEqual(reported.returncode, 0, reported.stdout)
        body, state = self.posted_state()
        self.assertEqual(state["kind"], "delivered")
        self.assertEqual(state["pr"], pull["url"])
        self.assertIn(pull["url"], body)
        self.assertNotIn('["pr", "create"]', self.log_path.read_text())

    def test_publication_recovery_without_marker_reuses_branch(self):
        fixture = self.make_candidate(push=True)
        pull = self.pull(fixture["candidate"])
        self.set_scenario(pulls=[pull])
        self.set_event("issue_comment", body="/tokicv 交付这个工单", comment_id="2")

        prepared = self.command("prompt")

        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertEqual(self.outputs()["skip_implementation"], "true")
        self.assertEqual(self.git("rev-parse", "HEAD"), fixture["candidate"])
        recovery = json.loads((self.work / "recovery.json").read_text())
        self.assertEqual(recovery["stage"], "publication_failed")
        self.assertFalse(recovery["must_change"])

        recorded = self.command("candidate")
        self.assertEqual(recorded.returncode, 0, recorded.stdout)
        self.assertEqual(self.outputs()["status"], "ready")
        self.assertEqual(self.outputs()["candidate"], fixture["candidate"])

        self.write_review()
        reported = self.command("report", {
            "STATUS": "ready", "REVIEW_RESULT": "success",
            "PACKAGE_RESULT": "success", "CANDIDATE": fixture["candidate"],
        })
        self.assertEqual(reported.returncode, 0, reported.stdout)
        _, state = self.posted_state()
        self.assertEqual(state["kind"], "delivered")
        self.assertEqual(state["candidate"], fixture["candidate"])

    def test_same_comment_retries_a_failed_stage_without_reimplementation(self):
        fixture = self.make_candidate()
        state = self.recovery_state(
            fixture, "package_failed", "完整验证或打包失败", consumed=["2"],
        )
        self.set_scenario(
            comments=[
                self.comment(BOT, state_block(state), 1, 1),
                self.comment(OWNER, "/tokicv 交付这个工单", 2, 2),
            ],
            artifacts={ARTIFACT: str(fixture["artifact"])},
        )
        self.set_event("issue_comment", body="/tokicv 交付这个工单", comment_id="2")

        prepared = self.command("prompt")

        self.assertEqual(prepared.returncode, 0, prepared.stdout)
        self.assertEqual(self.outputs()["skip_implementation"], "true")
        recovery = json.loads((self.work / "recovery.json").read_text())
        self.assertFalse(recovery["must_change"])
        self.assertEqual(recovery["candidate"], fixture["candidate"])


if __name__ == "__main__":
    unittest.main()
