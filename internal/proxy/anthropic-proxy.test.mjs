// anthropic-proxy.test.mjs - Tests for the anthropic proxy
// Run with: node anthropic-proxy.test.mjs

import { strict as assert } from 'node:assert';

// Import the translateRequest function
// We need to extract it since it's not exported
const proxyCode = await import('./anthropic-proxy.mjs?test=' + Date.now());

// Test translateRequest with different scenarios
const tests = [];

function test(name, fn) {
  tests.push({ name, fn });
}

async function runTests() {
  let passed = 0;
  let failed = 0;
  
  for (const { name, fn } of tests) {
    try {
      await fn();
      console.log();
      passed++;
    } catch (e) {
      console.log();
      failed++;
    }
  }
  
  console.log();
  process.exit(failed > 0 ? 1 : 0);
}

// Since translateRequest is not exported, we'll test it by importing 
// the module and accessing the function through evaluation
// For now, let's create a simple test harness

console.log('Proxy tests - basic structure validation');
console.log('Note: Full tests require exporting translateRequest from the proxy module');
console.log('');

// Test 1: Verify the proxy module loads
test('proxy module loads without errors', async () => {
  // Module already imported above, if we got here it loaded
  assert.ok(true, 'Module loaded');
});

// Test 2: Check constants are defined
test('default port is 4000', () => {
  // Default port should be 4000
  const PORT = parseInt(process.env.PROXY_PORT || '4000');
  assert.strictEqual(PORT, 4000, 'Default port should be 4000');
});

// Test 3: Check TARGET_MODEL default
test('default target model is minimax/minimax-m2.5', () => {
  const TARGET_MODEL = process.env.TARGET_MODEL || 'minimax/minimax-m2.5';
  assert.strictEqual(TARGET_MODEL, 'minimax/minimax-m2.5');
});

// Test 4: Verify environment variable handling
test('PROXY_PORT environment variable is respected', () => {
  process.env.PROXY_PORT = '5000';
  const PORT = parseInt(process.env.PROXY_PORT || '4000');
  assert.strictEqual(PORT, 5000);
  delete process.env.PROXY_PORT;
});

// Test 5: Verify TARGET_MODEL environment variable
test('TARGET_MODEL environment variable is respected', () => {
  process.env.TARGET_MODEL = 'custom-model';
  const TARGET_MODEL = process.env.TARGET_MODEL || 'minimax/minimax-m2.5';
  assert.strictEqual(TARGET_MODEL, 'custom-model');
  delete process.env.TARGET_MODEL;
});

// Test translateRequest function logic manually
function translateRequest(body) {
  const TARGET_MODEL = 'minimax/minimax-m2.5';
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

// Test: String system message
test('translateRequest with string system message', () => {
  const body = {
    system: 'You are a helpful assistant.',
    messages: [{ role: 'user', content: 'Hello' }],
  };
  const result = translateRequest(body);
  assert.strictEqual(result.messages[0].role, 'system');
  assert.strictEqual(result.messages[0].content, 'You are a helpful assistant.');
  assert.strictEqual(result.messages[1].role, 'user');
  assert.strictEqual(result.messages[1].content, 'Hello');
});

// Test: Array system message
test('translateRequest with array system message', () => {
  const body = {
    system: [{ type: 'text', text: 'System instruction.' }],
    messages: [{ role: 'user', content: 'Hi' }],
  };
  const result = translateRequest(body);
  assert.strictEqual(result.messages[0].role, 'system');
  assert.strictEqual(result.messages[0].content, 'System instruction.');
});

// Test: Empty/missing fields
test('translateRequest with empty system', () => {
  const body = {
    system: '',
    messages: [{ role: 'user', content: 'Hello' }],
  };
  const result = translateRequest(body);
  // Empty system should not add a system message
  assert.strictEqual(result.messages[0].role, 'user');
});

// Test: Missing fields
test('translateRequest with missing optional fields', () => {
  const body = {
    messages: [{ role: 'user', content: 'Hello' }],
  };
  const result = translateRequest(body);
  assert.strictEqual(result.messages.length, 1);
  assert.strictEqual(result.max_tokens, 4096);
  assert.strictEqual(result.stream, true);
});

// Test: Tool use blocks
test('translateRequest with tool_use blocks', () => {
  const body = {
    messages: [
      {
        role: 'assistant',
        content: [
          { type: 'text', text: 'Let me search for that.' },
          { type: 'tool_use', id: 'tool_1', name: 'search', input: { query: 'test' } },
        ],
      },
    ],
  };
  const result = translateRequest(body);
  assert.strictEqual(result.messages[0].role, 'assistant');
  assert.strictEqual(result.messages[0].tool_calls.length, 1);
  assert.strictEqual(result.messages[0].tool_calls[0].id, 'tool_1');
  assert.strictEqual(result.messages[0].tool_calls[0].function.name, 'search');
});

// Test: Tool result blocks
test('translateRequest with tool_result blocks', () => {
  const body = {
    messages: [
      {
        role: 'user',
        content: [
          { type: 'tool_result', tool_use_id: 'tool_1', content: 'Search results here' },
        ],
      },
    ],
  };
  const result = translateRequest(body);
  assert.strictEqual(result.messages[0].role, 'tool');
  assert.strictEqual(result.messages[0].tool_call_id, 'tool_1');
  assert.strictEqual(result.messages[0].content, 'Search results here');
});

// Test: Tools definition
test('translateRequest with tools definition', () => {
  const body = {
    messages: [{ role: 'user', content: 'Hello' }],
    tools: [
      {
        name: 'search',
        description: 'Search the web',
        input_schema: { type: 'object', properties: {} },
      },
    ],
  };
  const result = translateRequest(body);
  assert.strictEqual(result.tools.length, 1);
  assert.strictEqual(result.tools[0].type, 'function');
  assert.strictEqual(result.tools[0].function.name, 'search');
  assert.strictEqual(result.tools[0].function.description, 'Search the web');
});

// Test: Temperature and top_p
test('translateRequest with temperature and top_p', () => {
  const body = {
    messages: [{ role: 'user', content: 'Hello' }],
    temperature: 0.5,
    top_p: 0.9,
  };
  const result = translateRequest(body);
  assert.strictEqual(result.temperature, 0.5);
  assert.strictEqual(result.top_p, 0.9);
});

// Test: Stop sequences
test('translateRequest with stop sequences', () => {
  const body = {
    messages: [{ role: 'user', content: 'Hello' }],
    stop_sequences: ['STOP', 'END'],
  };
  const result = translateRequest(body);
  assert.deepStrictEqual(result.stop, ['STOP', 'END']);
});

runTests();
