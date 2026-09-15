const code = `// Start tracking
const thread = await threadify.start();
thread.step("payment_captured")

// Add context
const step = thread.step("fraud_check")
step.addContext(data).success()`;

let tokens = [];

// Helper to push a token and return placeholder
const tokenize = (html) => {
  tokens.push(html);
  return `__TOKEN_${tokens.length - 1}__`;
};

let processed = code
  .replace(/</g, "&lt;")
  .replace(/>/g, "&gt;");

// 1. Strings
processed = processed.replace(/(["'])(.*?)\1/g, (match) => tokenize(`<span class="text-emerald-300">${match}</span>`));

// 2. Comments
processed = processed.replace(/(\/\/.*)/g, (match) => tokenize(`<span class="text-slate-500 italic">${match}</span>`));

// 3. Method calls
processed = processed.replace(/\.(\w+)(\s*\()/g, (_, method, paren) => {
  return `.${tokenize(`<span class="text-blue-400">${method}</span>`)}${paren}`;
});

// 4. Object keys
processed = processed.replace(/([a-zA-Z0-9_]+):/g, (_, key) => {
  return `${tokenize(`<span class="text-indigo-300">${key}</span>`)}:`;
});

// 5. Keywords
processed = processed.replace(/\b(const|await|async|let|var|function|return|import|from)\b/g, (match) => tokenize(`<span class="text-pink-400">${match}</span>`));

// 6. Known variables
processed = processed.replace(/\b(threadify|thread|step|connection|notification|console|data|invitation|invitationToken)\b/g, (match) => tokenize(`<span class="text-sky-300">${match}</span>`));

// Restore tokens
tokens.forEach((token, i) => {
  processed = processed.replace(`__TOKEN_${i}__`, token);
});

console.log(processed);
