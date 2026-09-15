const code = `// Start tracking
const thread = await threadify.start();
thread.step("payment_captured")

// Add context
const step = thread.step("fraud_check")
step.addContext(data).success()`;

let tokens = [];
let processed = code
  .replace(/</g, "&lt;")
  .replace(/>/g, "&gt;");

// 1. Extract strings
processed = processed.replace(/(["'])(.*?)\1/g, (match) => {
  tokens.push(`<span class="text-emerald-300">${match}</span>`);
  return `__TOKEN_${tokens.length - 1}__`;
});

// 2. Extract comments
processed = processed.replace(/(\/\/.*)/g, (match) => {
  tokens.push(`<span class="text-slate-500 italic">${match}</span>`);
  return `__TOKEN_${tokens.length - 1}__`;
});

// 3. Keywords
processed = processed.replace(/\b(const|await|async|let|var|function|return|import|from)\b/g, '<span class="text-pink-400">$1</span>');

// 4. Method calls
processed = processed.replace(/\.(\w+)\s*\(/g, '.<span class="text-blue-400">$1</span>(');

// 5. Object keys
processed = processed.replace(/([a-zA-Z0-9_]+):/g, '<span class="text-indigo-300">$1</span>:');

// 6. Known variables
processed = processed.replace(/\b(threadify|thread|step|connection|notification|console|data|invitation|invitationToken)\b/g, '<span class="text-sky-300">$1</span>');

// Restore tokens
tokens.forEach((token, i) => {
  processed = processed.replace(`__TOKEN_${i}__`, token);
});

console.log(processed);
