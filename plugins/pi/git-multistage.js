// The git-multistage extension for Pi. Oh My Pi loads it too, from the
// package's "pi" field, and its stages get the prefix omp.
//
// Before and after each call of a file tool, it runs `git multistage hook`,
// which records the call's edit in the session's git-multistage stage. It adds
// the hook's messages, the shared-checkout notice and the first-edit message,
// to the tool's result. It requires the git-multistage binary on PATH, and
// never blocks a tool call.

import { spawn } from "node:child_process";

// The file tools of Pi and Oh My Pi.
const fileTools = new Set(["edit", "write", "apply_patch"]);

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
        console.error(`git-multistage extension: cannot run git multistage: ${err.message}`);
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

export default function gitMultistage(pi, editor = process.title === "omp" ? "omp" : "pi") {
  const started = new Set();
  const notices = new Map(); // session ID -> the session start notice, until a call result carries it
  const call = (event, ctx, eventName) => {
    const agent = ctx.agent?.kind === "sub" ? ctx.agent.id : undefined; // Oh My Pi subagents
    return {
      hook_event_name: eventName,
      session_id: ctx.sessionManager?.getSessionId?.() ?? "",
      ...(agent ? { agent_id: agent } : {}),
      tool_name: event.toolName,
      tool_input: event.input,
      tool_use_id: event.toolCallId,
      cwd: ctx.cwd,
    };
  };

  pi.on("tool_call", async (event, ctx) => {
    if (!fileTools.has(event.toolName)) return undefined;
    const input = call(event, ctx, "PreToolUse");
    if (!started.has(input.session_id)) {
      started.add(input.session_id);
      const notice = await runHook(editor, { hook_event_name: "SessionStart", session_id: input.session_id, cwd: ctx.cwd });
      if (notice) notices.set(input.session_id, notice);
    }
    await runHook(editor, input);
    return undefined;
  });

  pi.on("tool_result", async (event, ctx) => {
    if (!fileTools.has(event.toolName)) return undefined;
    const input = call(event, ctx, event.isError ? "PostToolUseFailure" : "PostToolUse");
    const message = await runHook(editor, input);
    const text = [notices.get(input.session_id), message].filter(Boolean).join("\n\n");
    notices.delete(input.session_id);
    if (!text) return undefined;
    return { content: [...event.content, { type: "text", text }], structuredContent: event.structuredContent };
  });
}
