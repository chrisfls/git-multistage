"""The git-multistage plugin for Hermes Agent.

Before and after each call of a file tool, it runs `git multistage hook`,
which records the call's edit in the session's git-multistage stage. It
gives the hook's messages, the shared-checkout notice and the first-edit
message, to the model before its next call. It requires the git-multistage
binary on PATH, and never blocks a tool call.
"""

import json
import os
import subprocess
import sys
from pathlib import Path

# The file tools of Hermes Agent.
FILE_TOOLS = {"write_file", "patch"}

_started = set()
_pending = {}  # session ID -> messages for the model
_reported = False


def _skill_file():
    """Return the skill file for a git-clone install or a flat copy."""
    here = Path(__file__).resolve().parent
    for root in (here.parent, here):
        skill = root / "skills" / "git-multistage" / "SKILL.md"
        if skill.is_file():
            return skill
    return None


def _description(skill):
    """Return the description in the front matter of a skill file. Hermes
    shows a skill without one with an empty description."""
    text = skill.read_text(encoding="utf-8").replace("\r\n", "\n")
    front = text.split("---\n", 2)[1] if text.startswith("---\n") else ""
    for line in front.splitlines():
        key, _, value = line.partition(":")
        if key == "description":
            return value.strip()
    return ""


def _run(event, session_id, **fields):
    """Run `git multistage hook hermes` and return the text of its message."""
    global _reported
    hook_input = {"hook_event_name": event, "session_id": session_id or "", "cwd": os.getcwd(), **fields}
    try:
        out = subprocess.run(
            ["git", "multistage", "hook", "hermes"],
            input=json.dumps(hook_input),
            capture_output=True,
            text=True,
            timeout=60,
            check=False,
        ).stdout
    except (OSError, subprocess.SubprocessError) as err:
        if not _reported:
            _reported = True
            print(f"git-multistage plugin: cannot run git multistage: {err}", file=sys.stderr)
        return ""
    for line in out.splitlines():
        try:
            text = json.loads(line).get("hookSpecificOutput", {}).get("additionalContext")
        except (ValueError, AttributeError):
            continue
        if text:
            return text
    return ""


def _queue(session_id, text):
    if text:
        _pending.setdefault(session_id, []).append(text)


def _pre_tool_call(tool_name=None, args=None, session_id=None, tool_call_id=None, **_):
    if tool_name not in FILE_TOOLS:
        return None
    if session_id not in _started:
        _started.add(session_id)
        _queue(session_id, _run("SessionStart", session_id))
    _run("PreToolUse", session_id, tool_name=tool_name, tool_input=args or {}, tool_use_id=tool_call_id or "")
    return None


def _post_tool_call(tool_name=None, args=None, session_id=None, tool_call_id=None, status=None, **_):
    if tool_name not in FILE_TOOLS:
        return
    event = "PostToolUse" if status in (None, "ok") else "PostToolUseFailure"
    text = _run(event, session_id, tool_name=tool_name, tool_input=args or {}, tool_use_id=tool_call_id or "")
    _queue(session_id, text)


def _pre_llm_call(session_id=None, **_):
    texts = _pending.pop(session_id, [])
    return {"context": "\n\n".join(texts)} if texts else None


def register(ctx):
    skill = _skill_file()
    if skill is not None:
        ctx.register_skill("git-multistage", skill, description=_description(skill))
    ctx.register_hook("pre_tool_call", _pre_tool_call)
    ctx.register_hook("post_tool_call", _post_tool_call)
    ctx.register_hook("pre_llm_call", _pre_llm_call)
