"""Run one write_file call through the git-multistage Hermes plugin in a fake
Hermes context, as Hermes would, and print what the model would read next.

    python3 drive_hermes.py <plugin __init__.py> <repository> <file> <content> <session>
"""

import importlib.util
import json
import os
import sys

plugin, repo, name, content, session = sys.argv[1:]
spec = importlib.util.spec_from_file_location("git_multistage_plugin", plugin)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class Context:
    def __init__(self):
        self.hooks = {}
        self.skills = []

    def register_skill(self, name, path, description=""):
        self.skills.append({"name": name, "path": str(path), "type": type(path).__name__, "description": description})

    def register_hook(self, name, callback):
        self.hooks[name] = callback


ctx = Context()
module.register(ctx)
os.chdir(repo)
path = os.path.join(repo, name)
args = {"path": path, "content": content}
ctx.hooks["pre_tool_call"](tool_name="write_file", args=args, session_id=session, tool_call_id="call-1", task_id="t")
with open(path, "w", encoding="utf-8", newline="") as f:
    f.write(content)
ctx.hooks["post_tool_call"](
    tool_name="write_file", args=args, session_id=session, tool_call_id="call-1", task_id="t", status="ok", result="ok"
)
context = ctx.hooks["pre_llm_call"](session_id=session, user_message="", is_first_turn=False)
print(json.dumps({"skills": ctx.skills, "context": (context or {}).get("context", "")}))
