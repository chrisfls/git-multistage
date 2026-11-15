// The git-multistage plugin for Amp. Install it with:
//
//   amp plugins add https://raw.githubusercontent.com/chrisfls/git-multistage/master/plugins/amp/git-multistage.js
//
// Before and after each call of a file tool, it runs `git multistage hook`,
// which records the call's edit in the thread's git-multistage stage. It adds
// the hook's messages, the shared-checkout notice and the first-edit message,
// to the tool's output. It requires the git-multistage binary on PATH, and
// never blocks a tool call. This file imports nothing else from the
// repository, because Amp installs a plugin as one file.

import { spawn } from "node:child_process";

// The file tools of Amp.
const fileTools = new Set(["edit_file", "create_file", "apply_patch"]);

let reported = false;

// runHook runs `git multistage hook amp` with input on stdin, and resolves
// to the text of the message that the hook prints, or "".
function runHook(input) {
  return new Promise((resolve) => {
    let out = "";
    const child = spawn("git", ["multistage", "hook", "amp"], { stdio: ["pipe", "pipe", "ignore"] });
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

export default function gitMultistage(amp) {
  const started = new Set();
  const notices = new Map(); // session ID -> the session start notice, until a call result carries it
  const call = (event, eventName) => ({
    hook_event_name: eventName,
    session_id: event.thread.id,
    tool_name: event.tool,
    tool_input: event.input,
    tool_use_id: event.toolUseID,
    cwd: process.cwd(),
  });

  amp.on("tool.call", async (event) => {
    if (!fileTools.has(event.tool)) return { action: "allow" };
    if (!started.has(event.thread.id)) {
      started.add(event.thread.id);
      const notice = await runHook({ hook_event_name: "SessionStart", session_id: event.thread.id, cwd: process.cwd() });
      if (notice) notices.set(event.thread.id, notice);
    }
    await runHook(call(event, "PreToolUse"));
    return { action: "allow" };
  });

  amp.on("tool.result", async (event) => {
    if (!fileTools.has(event.tool)) return undefined;
    const message = await runHook(call(event, event.status === "done" ? "PostToolUse" : "PostToolUseFailure"));
    const text = [notices.get(event.thread.id), message].filter(Boolean).join("\n\n");
    if (!text || typeof event.output !== "string") return undefined;
    notices.delete(event.thread.id);
    const result = { status: event.status, output: `${event.output}\n\n${text}` };
    return event.error ? { ...result, error: event.error } : result;
  });
}
