#!/usr/bin/env node
// Sindri agent bridge — long-running Claude session with bidirectional comms.
// Uses V2 Session API: send() to inject messages, stream() to read output.
//
// Output: podman logs -f <container>
// Send:   podman exec <container> curl --unix-socket /tmp/sindri.sock -X POST -d '{"message":"do this"}' http://localhost/send
// Status: podman exec <container> curl --unix-socket /tmp/sindri.sock http://localhost/status

import { createRequire } from "node:module";
const require = createRequire("/opt/sindri/node_modules/");
const { unstable_v2_createSession } = require("@anthropic-ai/claude-agent-sdk");
import { createServer } from "node:http";
import { readFileSync, unlinkSync } from "node:fs";

const SOCKET_PATH = "/tmp/sindri.sock";
const initialPrompt = process.argv[2] || readFileSync("/home/sindri/.claude/system-prompt.txt", "utf8").trim() || "You are a Sindri agent.";

console.log("=== sindri agent bridge starting ===");

// Create long-lived session
// Permission request queue — bridge emits events, waits for approval via HTTP
let permissionResolve = null;

// Don't use `await using` — we want the session to live forever
const session = unstable_v2_createSession({
  cwd: "/workspace",
  permissionMode: "acceptEdits",
  allowedTools: [
    "Bash", "Read", "Edit", "Write", "Glob", "Grep",
    "NotebookEdit", "WebFetch", "WebSearch", "AskUserQuestion",
    "TodoWrite", "EnterPlanMode", "ExitPlanMode", "LSP",
    "Skill", "Task", "TaskOutput",
  ],
  permissionTool: async (req) => {
    // Emit permission request as JSON event for watch/TUI to see
    console.log(JSON.stringify({
      type: "sindri_permission_request",
      tool: req.tool_name || req.toolName || "unknown",
      input: req.tool_input || req.input || {},
      message: req.message || "",
    }));

    // Wait for approval via HTTP /approve endpoint
    const result = await new Promise(r => { permissionResolve = r; });
    return result;
  },
});

console.log("=== session created ===");

// Send initial prompt
await session.send(initialPrompt);
console.log("=== initial prompt sent ===");

// Stream output forever in background
(async () => {
  while (true) {
    for await (const event of session.stream()) {
      console.log(JSON.stringify(event));
    }
    // stream() ends after each turn — loop to catch the next turn
  }
})();

// HTTP server for sending messages
const server = createServer((req, res) => {
  if (req.method === "GET" && req.url === "/status") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ status: "running", pendingPermission: permissionResolve !== null }));
    return;
  }

  if (req.method === "POST" && req.url === "/approve") {
    if (permissionResolve) {
      permissionResolve({ behavior: "allow" });
      permissionResolve = null;
      res.writeHead(200);
      res.end(JSON.stringify({ ok: true, action: "approved" }));
    } else {
      res.writeHead(404);
      res.end(JSON.stringify({ error: "no pending permission request" }));
    }
    return;
  }

  if (req.method === "POST" && req.url === "/deny") {
    if (permissionResolve) {
      permissionResolve({ behavior: "deny" });
      permissionResolve = null;
      res.writeHead(200);
      res.end(JSON.stringify({ ok: true, action: "denied" }));
    } else {
      res.writeHead(404);
      res.end(JSON.stringify({ error: "no pending permission request" }));
    }
    return;
  }

  if (req.method === "POST" && req.url === "/send") {
    let body = "";
    req.on("data", chunk => { body += chunk; });
    req.on("end", async () => {
      try {
        const { message } = JSON.parse(body);
        if (!message) {
          res.writeHead(400);
          res.end(JSON.stringify({ error: "missing 'message' field" }));
          return;
        }
        console.error(`=== received: ${message.slice(0, 80)} ===`);
        await session.send(message);
        res.writeHead(200);
        res.end(JSON.stringify({ ok: true }));
      } catch (e) {
        console.error(`=== send error: ${e.message} ===`);
        res.writeHead(500);
        res.end(JSON.stringify({ error: e.message }));
      }
    });
    return;
  }

  res.writeHead(404);
  res.end("not found");
});

try { unlinkSync(SOCKET_PATH); } catch {}
server.listen(SOCKET_PATH, () => {
  console.error(`=== listening on ${SOCKET_PATH} ===`);
});
