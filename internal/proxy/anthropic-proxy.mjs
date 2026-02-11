#!/usr/bin/env node
// anthropic-proxy.mjs — Anthropic Messages API → OpenAI Chat Completions proxy
//
// Translates Claude Code's Anthropic Messages API format into OpenAI Chat
// Completions requests, forwarding them to OpenRouter (or any OpenAI-compatible
// endpoint). This enables Claude Code to use non-Anthropic models (Kimi K2.5,
// GLM 4.7, etc.) that are only available via OpenRouter's /chat/completions
// endpoint.
//
// Environment variables:
//   PROXY_PORT             — listen port (default 4000)
//   UPSTREAM_BASE          — upstream base URL (default https://openrouter.ai)
//   UPSTREAM_PATH          — upstream path (default /api/v1/chat/completions)
//   OPENROUTER_API_KEY     — API key for upstream authentication
//   TARGET_MODEL           — model ID to request (default moonshotai/kimi-k2.5)
//   MAX_RETRIES            — upstream retry attempts (default 3)
//   RETRY_BASE_DELAY_MS    — base delay for exponential backoff (default 1000)
//
// Zero external dependencies — uses only Node.js builtins.

import fs from 'node:fs';
import http from 'node:http';
import https from 'node:https';

const PORT = parseInt(process.env.PROXY_PORT || '4000');
const PID_FILE = process.env.PROXY_PID_FILE || '/home/sprite/.anthropic-proxy.pid';
const UPSTREAM_BASE = process.env.UPSTREAM_BASE || 'https://openrouter.ai';
const UPSTREAM_PATH = process.env.UPSTREAM_PATH || '/api/v1/chat/completions';
const API_KEY = process.env.OPENROUTER_API_KEY || '';
const TARGET_MODEL = process.env.TARGET_MODEL || 'moonshotai/kimi-k2.5';
const MAX_RETRIES = Math.max(1, parseInt(process.env.MAX_RETRIES, 10) || 3);
const RETRY_BASE_DELAY_MS = parseInt(process.env.RETRY_BASE_DELAY_MS || '1000');
const UPSTREAM_TIMEOUT_MS = 300000; // 5 minutes

// ── Retry Helpers ────────────────────────────────────────────────────

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

function isRetryableStatus(code) {
  return code === 429 || code >= 500;
}

function retryDelay(attempt) {
  return Math.min(RETRY_BASE_DELAY_MS * Math.pow(2, attempt - 1), 10000);
}

function attemptUpstreamRequest(url, payload) {
  return new Promise((resolve, reject) => {
    const upstream = https.request({
      hostname: url.hostname,
      port: url.port || 443,
      path: url.pathname,
      method: 'POST',
      timeout: UPSTREAM_TIMEOUT_MS,
      headers: {
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(payload),
        'Authorization': `Bearer ${API_KEY}`,
      },
    }, resolve);

    upstream.on('error', reject);
    upstream.on('timeout', () => {
      upstream.destroy();
      reject(new Error('upstream timeout (300s)'));
    });
    upstream.write(payload);
    upstream.end();
  });
}

function readResponseBody(response) {
  return new Promise((resolve) => {
    let body = '';
    response.on('data', (c) => { body += c; });
    response.on('end', () => resolve(body));
    response.on('error', () => resolve(body));
  });
}

async function forwardWithRetry(res, openaiBody, requestModel) {
  const payload = JSON.stringify(openaiBody);
  const url = new URL(UPSTREAM_BASE + UPSTREAM_PATH);

  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    try {
      const upstreamRes = await attemptUpstreamRequest(url, payload);

      if (upstreamRes.statusCode === 200) {
        res.writeHead(200, {
          'Content-Type': 'text/event-stream',
          'Cache-Control': 'no-cache',
          'Connection': 'keep-alive',
        });
        streamAnthropicResponse(res, upstreamRes, requestModel);
        return;
      }

      // Non-200 response
      const errBody = await readResponseBody(upstreamRes);
      console.error(`[proxy] attempt ${attempt}/${MAX_RETRIES}: upstream HTTP ${upstreamRes.statusCode}: ${errBody.slice(0, 500)}`);

      // Non-retryable or final attempt — return error to client
      if (!isRetryableStatus(upstreamRes.statusCode) || attempt === MAX_RETRIES) {
        const suffix = attempt > 1 ? ` after ${attempt} attempts` : '';
        res.writeHead(upstreamRes.statusCode, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({
          type: 'error',
          error: { type: 'api_error', message: `Upstream API error (HTTP ${upstreamRes.statusCode})${suffix}` },
        }));
        return;
      }
    } catch (err) {
      console.error(`[proxy] attempt ${attempt}/${MAX_RETRIES}: ${err.message}`);

      if (attempt === MAX_RETRIES) {
        if (!res.headersSent) {
          res.writeHead(502, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({
            type: 'error',
            error: { type: 'api_error', message: `Upstream connection failed after ${MAX_RETRIES} attempts: ${err.message}` },
          }));
        }
        return;
      }
    }

    // Retryable error, not final attempt — backoff before next try
    const delay = retryDelay(attempt);
    console.error(`[proxy] retrying in ${delay}ms...`);
    await sleep(delay);
  }
}

// ── Request Translation ──────────────────────────────────────────────

function translateRequest(body) {
  const messages = [];

  // System message(s)
  if (body.system) {
    const text = typeof body.system === 'string'
      ? body.system
      : (Array.isArray(body.system)
          ? body.system.filter(b => b.type === 'text').map(b => b.text).join('\n')
          : '');
    if (text) messages.push({ role: 'system', content: text });
  }

  // Conversation messages
  for (const msg of body.messages || []) {
    if (typeof msg.content === 'string') {
      messages.push({ role: msg.role, content: msg.content });
      continue;
    }
    if (!Array.isArray(msg.content)) continue;

    const textParts = [];
    const toolCalls = [];
    const toolResults = [];

    for (const block of msg.content) {
      if (block.type === 'text') {
        textParts.push(block.text);
      } else if (block.type === 'tool_use') {
        toolCalls.push({
          id: block.id,
          type: 'function',
          function: { name: block.name, arguments: JSON.stringify(block.input) },
        });
      } else if (block.type === 'tool_result') {
        const content = typeof block.content === 'string'
          ? block.content
          : (Array.isArray(block.content)
              ? block.content.filter(b => b.type === 'text').map(b => b.text).join('\n')
              : '');
        toolResults.push({ role: 'tool', tool_call_id: block.tool_use_id, content });
      }
    }

    if (toolCalls.length > 0) {
      messages.push({
        role: 'assistant',
        content: textParts.join('\n') || null,
        tool_calls: toolCalls,
      });
    } else if (toolResults.length > 0) {
      for (const tr of toolResults) messages.push(tr);
    } else if (textParts.length > 0) {
      messages.push({ role: msg.role, content: textParts.join('\n') });
    }
  }

  const result = {
    model: TARGET_MODEL,
    messages,
    max_tokens: body.max_tokens || 4096,
    stream: true,
    stream_options: { include_usage: true },
  };

  if (body.temperature != null) result.temperature = body.temperature;
  if (body.top_p != null) result.top_p = body.top_p;
  if (body.stop_sequences) result.stop = body.stop_sequences;

  // Translate tools
  if (body.tools?.length > 0) {
    result.tools = body.tools.map(t => ({
      type: 'function',
      function: {
        name: t.name,
        description: t.description || '',
        parameters: t.input_schema || {},
      },
    }));
  }

  return result;
}

// ── Response Translation (Streaming) ─────────────────────────────────

function streamAnthropicResponse(res, upstream, requestModel) {
  const msgId = 'msg_proxy_' + Date.now();
  let buffer = '';
  let blockIndex = 0;
  let inTextBlock = false;
  let inToolBlock = false;
  let currentToolCallId = null;
  let outputTokens = 0;
  let inputTokens = 0;

  // Emit Anthropic SSE event
  const emit = (event, data) => {
    res.write(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
  };

  // Send message_start
  emit('message_start', {
    type: 'message_start',
    message: {
      id: msgId, type: 'message', role: 'assistant', content: [],
      model: requestModel, stop_reason: null, stop_sequence: null,
      usage: { input_tokens: 0, output_tokens: 0 },
    },
  });

  function startTextBlock() {
    if (!inTextBlock && !inToolBlock) {
      emit('content_block_start', {
        type: 'content_block_start', index: blockIndex,
        content_block: { type: 'text', text: '' },
      });
      inTextBlock = true;
    }
  }

  function endCurrentBlock() {
    if (inTextBlock) {
      emit('content_block_stop', { type: 'content_block_stop', index: blockIndex });
      inTextBlock = false;
      blockIndex++;
    }
    if (inToolBlock) {
      emit('content_block_stop', { type: 'content_block_stop', index: blockIndex });
      inToolBlock = false;
      currentToolCallId = null;
      blockIndex++;
    }
  }

  function finish(stopReason) {
    endCurrentBlock();
    emit('message_delta', {
      type: 'message_delta',
      delta: { stop_reason: stopReason || 'end_turn', stop_sequence: null },
      usage: { output_tokens: outputTokens },
    });
    emit('message_stop', { type: 'message_stop' });
    res.end();
  }

  upstream.on('data', (chunk) => {
    buffer += chunk.toString();
    const lines = buffer.split('\n');
    buffer = lines.pop() || '';

    for (const line of lines) {
      if (line.startsWith(':') || !line.startsWith('data: ')) continue;
      const data = line.slice(6).trim();
      if (data === '[DONE]') return finish();

      let parsed;
      try { parsed = JSON.parse(data); } catch (e) {
        console.error('[proxy] JSON parse error:', e.message);
        continue;
      }

      if (parsed.usage) {
        inputTokens = parsed.usage.prompt_tokens || inputTokens;
        outputTokens = parsed.usage.completion_tokens || outputTokens;
      }

      const choice = parsed.choices?.[0];
      if (!choice) continue;
      const delta = choice.delta || {};

      // Tool calls
      if (delta.tool_calls) {
        for (const tc of delta.tool_calls) {
          if (tc.function?.name) {
            // End any existing block, start new tool_use block
            endCurrentBlock();
            currentToolCallId = tc.id || `toolu_${Date.now()}_${tc.index ?? 0}`;
            inToolBlock = true;
            emit('content_block_start', {
              type: 'content_block_start', index: blockIndex,
              content_block: { type: 'tool_use', id: currentToolCallId, name: tc.function.name, input: {} },
            });
            if (tc.function.arguments) {
              emit('content_block_delta', {
                type: 'content_block_delta', index: blockIndex,
                delta: { type: 'input_json_delta', partial_json: tc.function.arguments },
              });
            }
          } else if (tc.function?.arguments) {
            emit('content_block_delta', {
              type: 'content_block_delta', index: blockIndex,
              delta: { type: 'input_json_delta', partial_json: tc.function.arguments },
            });
          }
        }
        continue;
      }

      // Text content (skip empty strings from reasoning-model thinking phase)
      if (delta.content != null && delta.content !== '') {
        if (inToolBlock) endCurrentBlock();
        startTextBlock();
        emit('content_block_delta', {
          type: 'content_block_delta', index: blockIndex,
          delta: { type: 'text_delta', text: delta.content },
        });
      }

      // Finish reason
      if (choice.finish_reason) {
        const reason = choice.finish_reason === 'tool_calls' ? 'tool_use' : 'end_turn';
        return finish(reason);
      }
    }
  });

  upstream.on('end', () => { if (!res.writableEnded) finish(); });
  upstream.on('error', (err) => {
    console.error('[proxy] upstream stream error:', err.message);
    if (!res.writableEnded) res.end();
  });
}

// ── HTTP Server ──────────────────────────────────────────────────────

const server = http.createServer((req, res) => {
  // Health check (shallow — local server only)
  if (req.method === 'GET' && (req.url === '/health' || req.url === '/')) {
    res.writeHead(200, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ status: 'ok', model: TARGET_MODEL, port: PORT }));
    return;
  }

  // Deep health check — tests upstream reachability
  if (req.method === 'GET' && req.url === '/health/deep') {
    const url = new URL(UPSTREAM_BASE);
    const checkReq = https.request({
      hostname: url.hostname,
      port: url.port || 443,
      path: '/',
      method: 'HEAD',
      timeout: 5000,
    }, (checkRes) => {
      res.writeHead(200, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        status: 'ok', model: TARGET_MODEL, port: PORT,
        upstream: { reachable: true, status: checkRes.statusCode },
      }));
    });
    checkReq.on('error', (err) => {
      res.writeHead(503, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        status: 'degraded', model: TARGET_MODEL, port: PORT,
        upstream: { reachable: false, error: err.message },
      }));
    });
    checkReq.on('timeout', () => {
      checkReq.destroy();
      res.writeHead(503, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({
        status: 'degraded', model: TARGET_MODEL, port: PORT,
        upstream: { reachable: false, error: 'timeout' },
      }));
    });
    checkReq.end();
    return;
  }

  // Only POST /v1/messages
  if (req.method !== 'POST' || !req.url?.startsWith('/v1/messages')) {
    res.writeHead(404, { 'Content-Type': 'application/json' });
    res.end(JSON.stringify({ type: 'error', error: { type: 'not_found', message: 'Not found' } }));
    return;
  }

  let body = '';
  let bodySize = 0;
  const MAX_BODY_SIZE = 10 * 1024 * 1024; // 10MB max request size

  req.on('data', (c) => {
    bodySize += Buffer.byteLength(c);
    if (bodySize > MAX_BODY_SIZE) {
      res.writeHead(413, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ type: 'error', error: { type: 'request_too_large', message: 'Request body exceeds 10MB limit' } }));
      req.destroy();
      return;
    }
    body += c;
  });
  req.on('end', () => {
    let anthropicBody;
    try { anthropicBody = JSON.parse(body); } catch (e) {
      res.writeHead(400, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ type: 'error', error: { type: 'invalid_request_error', message: e.message } }));
      return;
    }

    const openaiBody = translateRequest(anthropicBody);
    const requestModel = anthropicBody.model || TARGET_MODEL;

    console.log(`[proxy] ${requestModel} → ${TARGET_MODEL} | ${openaiBody.messages.length} msgs | ${openaiBody.tools?.length || 0} tools`);

    forwardWithRetry(res, openaiBody, requestModel).catch((err) => {
      console.error('[proxy] unhandled forward error:', err.message);
      if (!res.headersSent) {
        res.writeHead(500, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ type: 'error', error: { type: 'api_error', message: 'Internal proxy error' } }));
      }
    });
  });
});


server.listen(PORT, '127.0.0.1', () => {
  try {
    // Ensure directory exists (create if using /run/sprite/)
    const pidDir = PID_FILE.substring(0, PID_FILE.lastIndexOf('/'));
    if (!fs.existsSync(pidDir)) {
      fs.mkdirSync(pidDir, { recursive: true, mode: 0o755 });
    }
    fs.writeFileSync(PID_FILE, String(process.pid), { mode: 0o644 });
    console.log(`[anthropic-proxy] pid=${process.pid} port=${PORT} model=${TARGET_MODEL} retries=${MAX_RETRIES} pidfile=${PID_FILE}`);
  } catch (err) {
    console.error('[anthropic-proxy] failed to write PID file:', err.message);
  }
});

// Cleanup PID file on shutdown
function cleanup() {
  try {
    if (fs.existsSync(PID_FILE)) {
      fs.unlinkSync(PID_FILE);
      console.log('[anthropic-proxy] cleaned up PID file');
    }
  } catch (err) {
    // Ignore cleanup errors
  }
  process.exit(0);
}

process.on('SIGINT', cleanup);
process.on('SIGTERM', cleanup);
process.on('SIGQUIT', cleanup);
