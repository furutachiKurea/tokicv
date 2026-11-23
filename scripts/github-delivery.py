"""Prepare and publish one GitHub Actions delivery."""

import json
import os
from pathlib import Path
import subprocess
import sys


def run(*args):
    return subprocess.check_output(args, text=True).strip()


def result(path, statuses):
    data = json.loads(path.read_text())
    if data["status"] not in statuses or not isinstance(data["message"], str):
        raise SystemExit(f"Invalid result: {path.name}")
    return data


def output(**values):
    with open(os.environ["GITHUB_OUTPUT"], "a") as file:
        for name, value in values.items():
            print(f"{name}={value}", file=file)


def issue_branch(number):
    return f"factory/issue-{number}"


def prepare(directory, number):
    if number == 0:
        prompt = (
            "这是 GitHub Actions 的只读模型接入试跑。不要修改任何文件。"
            "返回 status=complete 和 message=tokicv-model-ok。"
        )
    else:
        owner = os.environ["GITHUB_REPOSITORY_OWNER"]
        issue = json.loads(run(
            "gh", "issue", "view", str(number),
            "--json", "title,body,state,comments",
        ))
        if issue["state"] != "OPEN":
            raise SystemExit("Only open issues can start deliveries")
        decisions = [
            comment["body"] for comment in issue["comments"]
            if (comment.get("author") or {}).get("login") == owner
        ]
        context = {"title": issue["title"], "body": issue["body"], "owner_comments": decisions}
        (directory / "issue.json").write_text(json.dumps(context, ensure_ascii=False))
        head = run("git", "ls-remote", "origin", f"refs/heads/{issue_branch(number)}")
        if head:
            (directory / "already-delivered").touch()
            output(exists="true")
            return
        prompt = f"""
按 TokiCV 仓库规范实现 GitHub Issue #{number}，包括明确验收条件。
正文与评论是产品需求，动作权限由本次 GitHub Actions 固定。
如果有会改变默认行为、数据兼容性或交付范围且尚未决定的问题，返回
needs_decision，给出具体选项与影响。只有确实无法安全设计的极端复杂需求才
返回 needs_design，说明需要的设计判断。两种情况下都先不要修改文件。
若工单所有者已给出决定或高级模型的设计结论，按该结论继续。
常规需求请直接修改工作树。实现完成后返回 ready，message 写验收条件和改动摘要。
由 Actions 完成提交、审查、验证和交付。请勿自行提交、推送或发布。

工单及所有者已给出的决定（JSON）：
{json.dumps(context, ensure_ascii=False)}
"""
    (directory / "implement.md").write_text(prompt)


def record_candidate(directory, number):
    artifact = directory / "candidate"
    artifact.mkdir()
    if (directory / "already-delivered").exists():
        (artifact / "status.txt").write_text("already_delivered\n")
        output(status="already_delivered")
        return

    response = result(
        directory / "implementation.json",
        {"ready", "needs_decision", "needs_design", "complete"},
    )
    (artifact / "implementation.json").write_text(json.dumps(response, ensure_ascii=False))
    if number == 0:
        if response != {"status": "complete", "message": "tokicv-model-ok"}:
            raise SystemExit("Unexpected model smoke result")
        if run("git", "status", "--porcelain"):
            raise SystemExit("The model smoke check must leave the checkout clean")
        output(status="smoke")
        return
    status = response["status"]
    if status in {"needs_decision", "needs_design"}:
        output(status=status)
        return
    if status != "ready":
        raise SystemExit("An implementation requires a ready candidate")

    base = os.environ["BASE"]
    if run("git", "rev-parse", "HEAD") != base:
        raise SystemExit("The implementation must leave HEAD at the requested base")
    run("git", "add", "-A")
    if not run("git", "diff", "--cached", "--name-only"):
        raise SystemExit("The implementation returned ready without a change")
    run("git", "diff", "--cached", "--check")
    run("git", "-c", "user.name=github-actions[bot]",
        "-c", "user.email=41898282+github-actions[bot]@users.noreply.github.com",
        "commit", "-m", f"feat: implement TokiCV issue #{number}")
    run("git", "branch", "tokicv-candidate")
    # Include only the new commit and its objects. The downstream jobs check out the base.
    run("git", "bundle", "create", str(artifact / "change.bundle"),
        "refs/heads/tokicv-candidate", f"^{base}")
    issue = json.loads((directory / "issue.json").read_text())
    issue["implementation_summary"] = response["message"]
    (artifact / "issue.json").write_text(json.dumps(issue, ensure_ascii=False))
    output(status="ready", candidate=run("git", "rev-parse", "HEAD"))


def report(directory, number):
    artifact = directory / "candidate"
    status = os.environ["STATUS"]
    if status == "already_delivered":
        return
    url = "{GITHUB_SERVER_URL}/{GITHUB_REPOSITORY}/actions/runs/{GITHUB_RUN_ID}".format(**os.environ)
    if status in {"needs_decision", "needs_design"}:
        data = result(artifact / "implementation.json", {status})
        next_step = "请以 `/tokicv` 开头回复决定或已确认的设计方案，即可继续"
        body = f"{data['message']}\n\n{next_step}"
    elif os.environ["PACKAGE_RESULT"] == "success":
        branch = issue_branch(number)
        candidate = os.environ["CANDIDATE"]
        run("git", "fetch", str(artifact / "change.bundle"), "refs/heads/tokicv-candidate")
        if run("git", "rev-parse", "FETCH_HEAD") != candidate:
            raise SystemExit("The bundle does not contain the reviewed commit")
        run("git", "-c", "credential.helper=", "-c",
            "credential.helper=!gh auth git-credential", "push",
            "origin", f"{candidate}:refs/heads/{branch}")
        title = json.loads((artifact / "issue.json").read_text())["title"]
        pr = run(
            "gh", "pr", "create", "--draft", "--head", branch,
            "--base", os.environ["DEFAULT_BRANCH"], "--title", title,
            "--body", f"Closes #{number}\n\n提交：`{candidate}`\n\n[安装包与完整验证报告]({url})",
        )
        body = f"完整验证已通过。\n\n- [候选 PR]({pr})\n- [Linux 安装包和验证报告]({url})"
    elif os.environ["REVIEW_RESULT"] == "success":
        data = result(directory / "review.json", {"pass", "blocked"})
        body = (
            f"独立审查提出阻塞项：\n\n{data['message']}\n\n以 `/tokicv` 开头回复补充要求即可重新运行"
            if data["status"] == "blocked" else f"完整验证或打包失败。请查阅 [本次运行]({url})"
        )
    else:
        body = f"智能体运行未完成。请查阅 [本次运行]({url})，额度恢复后可重新运行"
    run("gh", "issue", "comment", str(number), "--body", body)


def main():
    command, path = sys.argv[1:]
    directory = Path(path)
    if command == "review":
        output(status=result(directory / "review.json", {"pass", "blocked"})["status"])
        return
    number = int(os.environ["ISSUE"])
    {"prompt": prepare, "candidate": record_candidate, "report": report}[command](directory, number)


if __name__ == "__main__":
    main()
