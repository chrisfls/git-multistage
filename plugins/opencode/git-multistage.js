// The git-multistage plugin for OpenCode. Kilo Code, MiMo Code, and CodeArts
// Agent load OpenCode plugins too, so this one file serves all four.
//
// Before and after each call of a file tool, it runs `git multistage hook`,
// which records the call's edit in the session's git-multistage stage. It adds
// the hook's messages, the shared-checkout notice and the first-edit message,
// to the tool's output. It requires the git-multistage binary on PATH, and
// never blocks a tool call. This file imports nothing else from the
// repository, so it also works when copied alone into a plugin folder.

import { spawn } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

// The file tools of OpenCode and its forks.
const fileTools = new Set(["edit", "write", "multiedit", "patch", "apply_patch"]);

const skills = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../skills");

// editorName names the stages after the host: kilo, mimo, or codearts when
// the host's binary has that name, and opencode otherwise.
function editorName() {
  const exe = path.basename(process.execPath).toLowerCase();
  return ["kilo", "mimo", "codearts"].find((name) => exe.startsWith(name)) ?? "opencode";
}

let reported = false;

// runHook runs `git multistage hook <editor>` with input on stdin, and
// resolves to the text of the message that the hook prints, or "".
function runHook(editor, input) {
  return new Promise((resolve) => {
    let out = "";
    const child = spawn("git", ["multistage", "hook", editor], { stdio: ["pipe", "pipe", "ignore"] });
    child.on("error", (err) => {
      if (!reported) {
        reported = true;
        console.error(`git-multistage plugin: cannot run git multistage: ${err.message}`);
      }
      resolve("");
    });
    child.stdout.on("data", (data) => {
      out += data;
    });
    child.on("close", () => resolve(messageText(out)));
    child.stdin.on("error", () => {});
    child.stdin.end(JSON.stringify(input));
  });
}

// messageText returns the additionalContext of the hook's output.
function messageText(out) {
  for (const line of out.split("\n")) {
    try {
      const text = JSON.parse(line)?.hookSpecificOutput?.additionalContext;
      if (text) return text;
    } catch {
      // not JSON
    }
  }
  return "";
}

export const GitMultistagePlugin = async ({ directory }) => {
  const editor = editorName();
  const started = new Set();
  const notices = new Map(); // call ID -> the session start notice
  const call = (input, event, args) => ({
    hook_event_name: event,
    session_id: input.sessionID,
    tool_name: input.tool,
    tool_input: args,
    tool_use_id: input.callID,
    cwd: directory,
  });
  return {
    config: async (config) => {
      if (!existsSync(skills) || Array.isArray(config.skills)) return;
      config.skills ??= {};
      config.skills.paths ??= [];
      if (!config.skills.paths.includes(skills)) config.skills.paths.push(skills);
    },
    "tool.execute.before": async (input, output) => {
      if (!fileTools.has(input.tool)) return;
      if (!started.has(input.sessionID)) {
        started.add(input.sessionID);
        const notice = await runHook(editor, { hook_event_name: "SessionStart", session_id: input.sessionID, cwd: directory });
        if (notice) notices.set(input.callID, notice);
      }
      await runHook(editor, call(input, "PreToolUse", output.args));
    },
    "tool.execute.after": async (input, output) => {
      if (!fileTools.has(input.tool)) return;
      const message = await runHook(editor, call(input, "PostToolUse", input.args));
      const text = [notices.get(input.callID), message].filter(Boolean).join("\n\n");
      notices.delete(input.callID);
      if (text && typeof output.output === "string") output.output += `\n\n${text}`;
    },
  };
};

export default GitMultistagePlugin;
