// drive.mjs runs one edit through a git-multistage JavaScript plugin in a
// fake host, as the host would: the plugin's before-tool handler, the edit on
// disk, then its after-tool handler. It prints the tool result that the
// model would read.
//
//   node drive.mjs <host> <plugin file> <repository> <file> <new content> <session>
//
// host is opencode, pi, omp, or amp. Before the edit, the session makes one
// call of a file tool that fails before it runs, so the host never reports
// its result; the session start notice must still reach the model.

import { writeFileSync, readFileSync } from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";

const [host, plugin, repo, file, content, session] = process.argv.slice(2);
const abs = path.join(repo, file);
const before = readFileSync(abs, "utf8");
const mod = await import(pathToFileURL(plugin).href);
process.chdir(repo);

const edit = () => writeFileSync(abs, content);
const handlers = {};
const on = (name, handler) => {
  handlers[name] = handler;
};

let text;
switch (host) {
  case "opencode": {
    const hooks = await mod.default({ directory: repo });
    const input = { tool: "edit", sessionID: session, callID: "call-1" };
    await hooks["tool.execute.before"]({ ...input, callID: "call-0" }, { args: { filePath: abs } });
    const output = { args: { filePath: abs, oldString: before, newString: content } };
    await hooks["tool.execute.before"](input, output);
    edit();
    const result = { title: "", output: "Edit applied.", metadata: {} };
    await hooks["tool.execute.after"]({ ...input, args: output.args }, result);
    text = result.output;
    break;
  }
  case "pi":
  case "omp": {
    if (host === "omp") process.title = "omp"; // as Oh My Pi's process is named
    mod.default({ on });
    const ctx = { cwd: repo, sessionManager: { getSessionId: () => session } };
    const input = { path: abs, edits: [{ oldText: before, newText: content }] };
    await handlers.tool_call({ toolName: "edit", toolCallId: "call-0", input: { path: abs } }, ctx);
    await handlers.tool_call({ toolName: "edit", toolCallId: "call-1", input }, ctx);
    edit();
    const content0 = [{ type: "text", text: "Edit applied." }];
    const result = await handlers.tool_result({ toolName: "edit", toolCallId: "call-1", input, content: content0, isError: false }, ctx);
    text = (result?.content ?? content0).map((part) => part.text).join("\n\n");
    break;
  }
  case "amp": {
    mod.default({ on });
    const event = { tool: "edit_file", toolUseID: "call-1", input: { path: abs, old_str: before, new_str: content }, thread: { id: session } };
    await handlers["tool.call"]({ ...event, toolUseID: "call-0", input: { path: abs } });
    const allow = await handlers["tool.call"](event);
    if (allow?.action !== "allow") throw new Error(`tool.call returned ${JSON.stringify(allow)}`);
    edit();
    const result = await handlers["tool.result"]({ ...event, status: "done", output: "Edit applied." });
    text = result?.output ?? "Edit applied.";
    break;
  }
  default:
    throw new Error(`unknown host ${host}`);
}
process.stdout.write(JSON.stringify({ text }));
